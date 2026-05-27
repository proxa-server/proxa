# Runtime contract

The `internal/runtime.Runtime` interface is the boundary between Proxa
control-plane logic and the container runtime that actually executes
containers. Today the only implementation is `runtime/docker` (the
Docker Engine API client). Future backends (podman, containerd shim,
firecracker, the future agent-proxied remote runtime in v0.5) MUST
satisfy this contract verbatim, with no per-backend deviations leaking
into reconciler / probe / ingress code.

This document is the contract Proxa expects every implementation to
honor. The contract is intentionally minimal: anything not specified
here is a future addition, not implementation freedom.

---

## General rules

1. **`context.Context` is the first parameter on every method.**
   Cancellation must abort the in-flight runtime call within ~100ms.
   Deadlines must be honored.

2. **Errors wrap with package prefix.** Format:
   `fmt.Errorf("runtime/<impl>: <op>: %w", err)`. Sentinel errors live
   in `internal/runtime.Errors` (currently only `ErrNotImplemented`).

3. **Methods are safe for concurrent use** from multiple goroutines —
   reconciler ticks, probe waves, log-stream goroutines, and the
   user-initiated container API all hit the runtime in parallel.

4. **Idempotency where the underlying API allows.** `StartContainer`
   on an already-running container returns nil. `RemoveContainer` on a
   missing container returns nil. `StopContainer` on an already-stopped
   container returns nil. Callers rely on these for retry safety.

5. **No global state.** Implementations construct a per-instance client
   via a `New(ctx, nodeID)` constructor (or similar). The reconciler
   wires one instance per process.

6. **`security.Apply` is called by CreateContainer.** Constitution §II
   demands the security profile is enforced at the runtime boundary, not
   by callers. Implementations MUST apply `spec.Security` (nonroot,
   read-only rootfs, no-new-privileges, dropped caps) and reject specs
   that the backend cannot satisfy with a clear error.

## Method-by-method contract

### `Name() string`
Short stable identifier (`"docker"`, `"podman"`, `"firecracker"`).
Used in logs + `proxa system info` for operator visibility.

### `Version(ctx) (string, error)`
The backend's own version string (e.g. Docker Engine 25.0.3). Used by
the dashboard's System Info card. Implementations MAY cache.

### `PullImage(ctx, ref)`
Pull `ref` (a `repo:tag` or `repo@sha256:...` reference) and block
until the image is available locally. Already-cached images return nil
without touching the network — implementations that don't have a
"cached" concept MUST simulate one by checking InspectImage first.

### `InspectImage(ctx, ref) -> *ImageInfo`
Return metadata for a locally-cached image. Returns `(nil, nil)` for
"not present" (NOT an error). Returns an error only for genuine I/O or
runtime failures.

### `CreateContainer(ctx, spec) -> id`
Create a container per `spec` but DO NOT start it. Implementations MUST:
- apply `spec.Security` (constitution §II) before submitting to the
  backend.
- attach every label in `spec.Labels` (`proxa.project`, `proxa.service`,
  `proxa.replica`, `proxa.spec_hash` — defined in
  `internal/runtime/docker/labels.go`).
- use `spec.Name` verbatim — the reconciler computes deterministic
  container names that include the project + service + replica index.

Returns the runtime's canonical container ID (Docker: 64-char hex;
other backends may use other IDs).

### `StartContainer(ctx, id)`
Start a previously-created container. Idempotent on already-running.

### `StopContainer(ctx, id, gracePeriod)`
Send SIGTERM, wait `gracePeriod`, then SIGKILL if still running. Default
grace period when `gracePeriod == 0` is implementation-defined (Docker:
10s). Idempotent on already-stopped.

### `RemoveContainer(ctx, id, force)`
Remove the container record (and its writable rootfs layer) from the
backend. `force=true` removes even a running container (after a stop).
Idempotent on missing.

### `RenameContainer(ctx, id, newName)`
Used by `start-first` deploy strategy to relabel the old replica
before the new one inherits its canonical name. Implementations that
don't support renaming MUST return `ErrNotImplemented` so the strategy
falls back to `stop-first`.

### `InspectContainer(ctx, id) -> *ContainerInfo`
Single-container detail lookup. Populates `IPAddress` from the bridge
network when present (empty otherwise — host-network or Linux + no
bridge attached). Returns `(nil, err)` when the container is missing.

### `ListContainers(ctx, filter) -> []ContainerInfo`
List containers matching the filter. `filter.Project` is **required**
(empty Project returns an error). Implementations MUST filter by the
`proxa.project` label, not by name prefix, so foreign containers don't
appear and proxa containers from other projects don't leak in.

### `StreamLogs(ctx, id, opts) -> io.ReadCloser`
Return a reader over the container's stdout+stderr stream. `opts.Follow`
keeps the stream open across container restarts (for tail -f UX);
`opts.Tail` controls historic line count. Closing the reader OR
cancelling `ctx` MUST stop the stream within ~100ms.

### `Stats(ctx, id) -> *ContainerStats`
One-shot resource snapshot. Used by the dashboard's per-service stats
card + future Prometheus exporter. Implementations on backends without
native stats (e.g. firecracker MicroVM) MAY synthesize from cgroup
readings.

### `Exec(ctx, id, cmd, opts) -> *ExecResult`
Run `cmd` inside a running container, capture stdout/stderr/exit-code.
Used by exec-style health probes + `proxa exec`. `opts.Timeout` MUST be
honored; an exec that exceeds the timeout returns a non-nil error AND
a partially-populated `ExecResult`.

## Optional capabilities (not yet on the interface)

These are explicitly out of scope for v0.4.3 — captured here so future
implementations don't paint themselves into a corner:

- **Volume management.** Today the reconciler uses `spec.Volumes` and
  expects the runtime to handle bind mounts + named volumes
  transparently. v0.5+ may add `CreateVolume` / `RemoveVolume` for
  declarative volume lifecycle.

- **Network management.** Bridge network creation is left to the
  backend's auto-management. v0.5+ multi-host may add `CreateNetwork`
  / `JoinNetwork` for cluster-wide overlay networking.

- **Image signing verification.** Image signing (cosign) is deferred
  until first supply-chain ask per spec 006. When it lands, it'll be
  a new `VerifyImage(ctx, ref) error` method.

## Implementation checklist (when adding a new runtime backend)

When the agent ships (v0.5) and we add a remote-runtime backend that
proxies to `proxa-agent` over mTLS, the agent-backed Runtime must:

- [ ] Implement every method above.
- [ ] Pass the integration tests under `internal/runtime/<impl>/*_test.go`
      with `//go:build dockerd` for the live-daemon ones.
- [ ] Pass the same e2e tests under `tests/e2e/*.go` that the docker
      backend passes (the e2e tests don't care which runtime is wired —
      they invoke `proxa server` and hit the API).
- [ ] Honor the OpenTelemetry trace context propagation contract (when
      v0.6 lands distributed tracing).
- [ ] Provide a `Capabilities()` accessor when v0.5+ introduces feature
      flags (e.g., "supports image-streaming logs", "supports stats").

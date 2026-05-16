# Feature Spec: Health Checks + Real Deploy Strategies

**Feature ID**: 002-health-checks
**Status**: Ready for Planning
**Created**: 2026-05-14
**Depends on**: 001-core-loop (v0.1.0 merged)

## Overview

Wire actual health probes into the reconciliation loop so Proxa knows whether a container is *working*, not just *running*. Today the reconciler treats `State == "running"` as healthy — a process that started and immediately got stuck or returns 500s on every request still counts as "actual = desired" and never gets restarted. The TaskDef already has a `[health]` section (path/port/command/interval/timeout/retries) but the runtime ignores it.

This feature also unlocks the deploy strategies that 001 left naive (Complexity Tracking deviation #2). With probes in hand, `start-first` can wait for the new container to pass a probe before stopping the old one (real zero-downtime), and `stop-first` continues to do its data-safety dance for stateful workloads.

By the end of this feature: `proxa up` against a service that crashes on second request results in the reconciler restarting the bad replica within `interval × retries` seconds, the dashboard shows the replica flipping to "degraded" then back to "healthy", and a stateless service upgrade has no detectable outside downtime.

## User Scenarios

### SC-002-1: Operator declares an HTTP probe

The operator's TOML includes `[health] path = "/healthz" port = 80 interval = "5s" timeout = "2s" retries = 3`. After `proxa up`, the reconciler probes `GET /healthz` on each replica every 5 seconds. As long as the response is `2xx` within 2s, `proxa ps` and the dashboard show `healthy`.

### SC-002-2: A replica starts failing probes

Container is up but its `/healthz` starts returning 500. After 3 consecutive failures (~15s with 5s interval), the reconciler marks the replica unhealthy, then removes and recreates it on the next tick. Service status reads `degraded` while the bad replica is being replaced; returns to `healthy` once the new replica passes its first probe.

### SC-002-3: Exec probe for non-HTTP workloads

A redis service uses `[health] command = ["redis-cli", "ping"]`. Reconciler runs the command inside the container every interval; non-zero exit code counts as a failure same as a 500.

### SC-002-4: Multi-replica service partially degraded

A service with `replicas = 3`. Replica 1's `/healthz` starts 500ing. While the reconciler is rotating it, the service shows `degraded` with `2/3` healthy. The remaining two replicas keep serving traffic. Once replica 1 is replaced and passing, status returns to `healthy`.

### SC-002-5: Stateless start-first upgrade — zero downtime

The operator changes `image = nginx:1.27` → `nginx:1.28` and re-runs `proxa up`. Service is stateless (`stateful = false`, default), so `strategy = start-first` applies. For each replica the reconciler:

1. Creates the new container with the new image.
2. Probes it until it passes (or `interval × retries` exhausted, in which case it removes the new container and leaves the old running).
3. Once the new one is healthy, removes the old container.

External observers see continuous availability throughout the upgrade.

### SC-002-6: Stateful stop-first upgrade — data safety

The operator upgrades a postgres service with `stateful = true`. `strategy = stop-first` applies. For each replica: stop the old container first, wait for it to fully exit, then create the new one with the new image. Brief downtime per replica is acceptable; data corruption from two writers touching the same volume is not.

## Functional Requirements

- FR-001: System MUST run an HTTP probe (`GET <path>` on the container's first declared `expose` port) every `interval` for any service whose TaskDef declares `[health].path`. Response status `2xx` within `timeout` is success; anything else is failure.
- FR-002: System MUST run an exec probe (`Runtime.Exec(container, command)`) for any service whose TaskDef declares `[health].command`. Exit code 0 is success; anything else is failure.
- FR-003: A service MAY declare both probes; both must succeed for a replica to count as healthy. A service MAY declare neither, in which case the replica is healthy iff the container is `State=running` (current v0.1.0 behavior, preserved).
- FR-004: Probe `interval` defaults to 10s if unspecified. `timeout` defaults to half the interval. `retries` defaults to 3.
- FR-005: After `retries` consecutive failures, a replica is marked `HealthOK = false` in the StateStore. The reconciler then schedules a remove + recreate on the next tick.
- FR-006: `Service.Status` is derived per tick from replica health: all healthy → `healthy`; some healthy + some unhealthy or in-progress → `degraded`; zero healthy and replicas exist → `failed`; replicas == 0 → `stopped`.
- FR-007: When `strategy = start-first` (default for `stateful = false`), an upgrade replaces replicas one at a time: create-new → probe-new-until-healthy → remove-old. New container's probe must pass at least once before old is removed.
- FR-008: When `strategy = stop-first` (default for `stateful = true`), an upgrade replaces replicas one at a time: stop-old → wait-for-exit → remove-old → create-new → probe-new. New container must pass at least one probe before reconciler advances to the next replica.
- FR-009: A start-first replacement that exhausts `retries` on the new container MUST roll back: remove the failing new container and leave the old one running. The replacement is retried on the next reconcile tick (with backoff documented as a known follow-up).
- FR-010: Per-replica probes MUST be parallel-safe — running concurrently, no shared mutable state outside the StateStore (which already serializes writes via SQLite WAL).
- FR-011: Probe outcomes MUST be logged via `slog`: `level=DEBUG` on every probe, `level=INFO` on a state transition (healthy ↔ unhealthy), `level=WARN` when a replica is being replaced due to probe failures.
- FR-012: `Replica.HealthOK`, `Replica.LastProbeAt`, and `Service.Status` MUST be persisted to the StateStore after each tick so `proxa ps` and the dashboard reflect probe outcomes immediately.
- FR-013: Probes MUST NOT block the reconciler's main tick — each replica's probes run in their own goroutine; the tick reads the latest health snapshot. Probe goroutines MUST exit when the container is removed or when the reconciler ctx cancels.

## Entities

(All already declared in `pkg/types/` from feature 000.)

- **HealthCheck** (`TaskDef.Health`) — the probe definition: `path`, `port`, `command`, `interval`, `timeout`, `retries`.
- **ReplicaState.HealthOK** — boolean populated by the probe loop.
- **ReplicaState.LastProbeAt** — RFC 3339 timestamp updated on every probe.
- **Service.Status** — `pending | reconciling | healthy | degraded | failed | stopped` (the v0.1.0 stub used `pending` and `reconciling`; this feature populates the rest).
- **DeployStrategy** (`TaskDef.Strategy`) — `start-first` (stateless default) or `stop-first` (stateful default).

## Success Criteria

- SC-001: A service with `[health] path = "/healthz"` and a working endpoint shows `healthy` in `proxa ps` and the dashboard within one tick of becoming reachable.
- SC-002: A workload that returns 500 on every probe is removed and recreated by the reconciler within `interval × (retries + 1)` seconds of the first failure.
- SC-003: A service with 3 replicas, one of which starts failing probes, shows `degraded` in `proxa ps` while the bad replica is rotated, then returns to `healthy` once the replacement passes a probe.
- SC-004: An exec probe (`["sh", "-c", "exit 1"]`) triggers the same restart cycle as a failing HTTP probe.
- SC-005: An in-place image upgrade of a stateless 2-replica service results in zero failed external requests during the rollover (verified by a 1-second curl loop against the host port).
- SC-006: An in-place image upgrade of a stateful service with `strategy = stop-first` never has two containers in `running` state for the same replica index simultaneously (verified by `docker ps` snapshots during the rollover).
- SC-007: A start-first replacement whose new container fails every probe is rolled back: the new container is removed, the old container stays `running`, service status remains `healthy` (or `degraded` until next retry), `slog` emits `WARN` with rollback reason.
- SC-008: With no `[health]` block declared, behavior is identical to v0.1.0 — `State=running` containers count as healthy. No regressions for existing services.

## Assumptions

- HTTP probes target the first port in `expose[]` if `[health].port` is unset. If neither `expose[]` nor `[health].port` is set, the parser rejects the TaskDef at `proxa up` time with error code `health-probe-needs-port`.
- HTTP probes go through the container's IP on the Docker bridge network, NOT through any host port mapping. Direct probe → container, no hairpin.
- Exec probe timeout is enforced by `Runtime.Exec`'s `ExecOpts.Timeout`, which is already declared in the foundation contract.
- Default values (interval 10s, timeout = interval/2, retries 3) match common Docker / Kubernetes defaults.
- Probes run from the Proxa server process. In single-node v0.x this is the same host as the containers, so latency is negligible. Multi-node v1.0 will need to probe from the agent on the container's host.

## Dependencies

- 001-core-loop merged at v0.1.0 (reconciler, runtime, store, server all functional).
- `Runtime.Exec` from 000-foundation (currently a stub in `runtime/docker/exec.go` that returns ErrNotImplemented). This feature includes wiring the real Exec implementation against `docker/docker/client`.
- `Runtime.StreamLogs` from 000 (still stubbed; this feature does NOT wire it — that's Feature 004 dashboard log viewer).

## Out of Scope

- HTTP/TCP ingress (Feature 003) — probes are internal-only here.
- Dashboard log viewer (Feature 004).
- Per-replica metrics dashboard / Stats wiring (later, ties into the Dashboard feature).
- TCP-only and gRPC probe types (could add, deferred until a real workload asks for them).
- Automatic rollback to the previous deployment on health failure (recorded in `DeploymentRecord.Outcome` as "failed" but no rollback machinery yet — Feature 005 or later).
- Probe authentication / mTLS to user workloads — assume probes are unauthenticated against `/healthz`-style endpoints.
- Backoff between failed start-first replacement attempts — every reconcile tick retries; if the new image is fundamentally broken this loops forever. Polish item documented as a known follow-up.

## Testing Strategy

- **Unit tests**: probe-result aggregation function (replicas → service status), HTTP probe with `httptest.Server` returning configurable status codes, exec probe against a mock Runtime, parallel-safety with `-race`.
- **Integration tests** (`//go:build dockerd`): real container with a `/healthz` endpoint that toggles 200 ↔ 500; verify the reconciler restarts it.
- **End-to-end tests** (`//go:build e2e`): SC-005 zero-downtime upgrade — deploy nginx, replace image, run a curl loop in a goroutine, assert no failures during the upgrade window.

## References

- Technical Spec Section 6: Security Model (probes do NOT bypass security profile).
- Technical Spec Section 8: Deployment Strategies.
- Constitution §VIII: Zero-Downtime by Default — this feature delivers the substance of that principle (001 only delivered the partial scaffolding).
- `specs/001-core-loop/plan.md` Complexity Tracking deviation #2 — this feature closes it.

# Phase 0 — Research: Health Checks + Real Deploy Strategies

Six decisions resolved before Phase 1 design.

---

## R-001: Probe transport — direct container IP vs. host port hop

**Decision**: HTTP probes dial the container's bridge-network IP directly (resolved via `docker inspect`'s `NetworkSettings.IPAddress`), NOT through any `expose[].host` host-port mapping.

**Rationale**: Many services declare `host = 0` (ingress-only) per our PortSpec semantics — no host port to hit. Even when `host > 0`, the host-port hop adds NAT latency and can fail when the host port is in `TIME_WAIT` from a previous probe. Container IP is stable (assigned at create-time) and reachable from the Proxa server which sits on the same Docker daemon.

**Alternatives rejected**:
- Host-port hop — fails when no host port is declared; brittle on macOS Docker Desktop (vpnkit NAT).
- DNS lookup `proxa-default-web-0.docker.internal` — non-portable; not all Docker setups expose this.

**Implementation**: custom `http.Transport` with `DialContext: func(ctx, network, addr) → net.Dial("tcp", containerIP:port)`. The container IP is fetched once when the probe goroutine starts (via `Runtime.InspectContainer`) and refreshed on container ID change.

---

## R-002: Probe interval semantics — fixed vs. drifting

**Decision**: Fixed interval anchored to wall-clock — every `interval` from probe-start, not from probe-end. If a probe takes 3s and interval is 5s, the next probe fires 2s after the previous returned (5s after it started). If a probe takes 6s (longer than interval), the next probe fires immediately when the prior returns; intervals don't queue up.

**Rationale**: Predictable for operators ("probes every 10s" means what it says). Drift-from-end (interval = idle time between probes) makes the actual probe rate dependent on workload latency, which surprises people debugging slow services.

**Alternatives rejected**:
- Drift-from-end — surprising; slow workload lowers probe rate.
- Strict ticker (every interval regardless) — can pile up if probes are slow; risks goroutine leak.

**Implementation**: `time.NewTicker(interval)` plus a "skip if a probe is in flight" guard. Ticker channel drains if the previous probe is still running.

---

## R-003: Failure consecutiveness — strict streak vs. sliding window

**Decision**: Strict consecutive streak. After `retries` consecutive failures, mark unhealthy. A single success resets the counter to 0.

**Rationale**: Matches Kubernetes/Docker Compose semantics, what operators expect. Sliding-window logic ("3 of last 5") is harder to explain and adds tunable knobs we don't need yet.

**Alternatives rejected**:
- Sliding window — extra config knob (`window_size`), hard to reason about.
- Exponential backoff between failures — useful for noisy upstreams but premature for v0.x.

---

## R-004: Status aggregation — per-tick vs. event-driven

**Decision**: Per-tick aggregation. Every reconciler tick (5s default), recompute `Service.Status` from the current snapshot of replica `HealthOK` values. Persist to SQLite only if status changed since the last tick.

**Rationale**: Keeps SQLite write rate bounded (one write per service per tick at most) regardless of probe rate. Probe results are written to in-memory state by the probe manager; the reconciler reads that snapshot. No write-amplification.

**Alternatives rejected**:
- Per-probe status update — N×M writes per tick where M is probes-per-replica-per-tick. Fine at our scale but unnecessary.
- Pub/sub from probes to reconciler — over-engineered; the per-tick read of in-memory snapshot is simpler and good enough.

---

## R-005: Strategy abstraction — interface vs. switch in action.go

**Decision**: A `Strategy` interface with `StartFirst` and `StopFirst` impls in `internal/reconciler/strategy.go`.

**Rationale**: Two reasons. First, future deploy strategies (canary, blue-green, rolling-percentage) are an obvious extension lane and a switch statement gets unwieldy fast. Second, testability — fake Strategy impls let us assert "the reconciler called Strategy.Apply(...) with these args" without spinning up real Docker.

The interface is small (just one method, `Apply(ctx, replicaCtx, oldID, newSpec, runtime, probe) error`), so the abstraction cost is low.

**Alternatives rejected**:
- Switch statement on `spec.Strategy` inside `action.go` — works for two cases, breaks down at three.
- Pure functions instead of interface methods — fine, but interface lets us inject a fake for tests without function-pointer plumbing.

---

## R-006: Probe goroutine lifecycle — pool vs. per-replica

**Decision**: One goroutine per replica, owned by the `internal/probe.Manager`. Started when the manager learns about a new container (via `manager.Track(containerID, spec)`); stopped when the manager learns the container is gone (`manager.Untrack(containerID)`).

**Rationale**: Per-replica goroutines map naturally to per-replica state. The alternative (a worker pool with a queue of "probe this container") adds queue management and complicates "stop probes for container X" semantics. With ≤100 containers per node, 100 lightweight goroutines is fine.

**Alternatives rejected**:
- Worker pool with bounded concurrency — overkill at our scale; complicates the "stop probing X" path.
- Single timer goroutine that probes everything in sequence — serializes; bad worst-case latency for a slow probe.

---

## License audit

No new third-party deps in this feature. License posture unchanged from 001's `docs/licenses.md`.

A T-task in Polish phase will run the audit script anyway and confirm `go.sum` is unchanged or only adds Docker SDK transitives (which were already audited in 001).

# Phase 1 — Data Model: Probes, Replica Health, Service Status

No SQLite schema change. The `services.replicas_json` and `services.status` columns added in 001 are sufficient — this feature populates fields that already exist on `pkg/types.ReplicaState` and `pkg/types.Service` from foundation.

---

## In-memory probe state (`internal/probe/`)

```go
package probe

// Result is one probe outcome.
type Result struct {
    At       time.Time
    Healthy  bool
    Latency  time.Duration
    Err      error  // nil on Healthy=true
}

// History is a per-replica capped ring buffer of recent results.
// Used by the manager to compute the consecutive-failure streak (R-003)
// and surface the last error to the dashboard.
type History struct {
    mu       sync.Mutex
    entries  []Result // capped at ~32; oldest dropped
}

// Snapshot is what the reconciler reads each tick.
// Immutable copy — callers do NOT hold the manager's lock.
type Snapshot struct {
    ContainerID string
    HealthOK    bool   // true if the replica is currently healthy
    LastProbeAt time.Time
    LastErr     string // empty when healthy
    Streak      int    // consecutive failures (0 when healthy)
}
```

The `Manager` owns one goroutine per tracked container and an `sync.Map[containerID]*replicaState` for snapshots. Reads from the reconciler are O(1) per replica.

---

## Replica health state machine

```
                       probe success
                ┌───────────────────────────┐
                │                           │
                ↓                           │
        ┌──────────────┐  probe failure  ┌──┴────────┐
        │  healthy     │ ──────────────► │ failing   │
        │ (HealthOK=T) │                 │ (streak<R)│
        └──────────────┘ ◄─────────────  └─────┬─────┘
                ▲       probe success          │
                │                              │ streak == R
                │                              ↓
                │                       ┌──────────────┐
                │                       │ unhealthy    │
                │                       │ (HealthOK=F) │
                │                       └──────┬───────┘
                │                              │
                │                              │ reconciler removes container
                │                              ↓
                │                       ┌──────────────┐
                │                       │  removed     │
                │                       └──────┬───────┘
                │                              │
                │                              │ reconciler creates replacement
                │                              ↓
                │                       ┌──────────────┐
                └─────────────────────  │  starting    │
                  first probe success   │ (HealthOK=F  │
                                        │  initially)  │
                                        └──────────────┘
```

States:
- **starting** — container just created; `HealthOK = false` until first probe succeeds. Service status: `reconciling`.
- **healthy** — `HealthOK = true`. Service status (per replica): contributes to `healthy`.
- **failing** — at least one consecutive failure but `streak < retries`. Service status (per replica): contributes to `degraded`.
- **unhealthy** — `streak >= retries`. Service status (per replica): contributes to `degraded` until removed; `failed` if it's the only replica.
- **removed** — reconciler has actioned the unhealthy replica; container ID disappears from actual state.

Transitions are driven solely by probe results + reconciler actions. The probe manager owns the in-memory streak; the reconciler reads `HealthOK` and decides whether to schedule a Replace action.

---

## Service status aggregation

```go
// In internal/reconciler/status.go
func aggregate(desired int, replicas []SnapshotByReplica) types.ServiceStatus {
    if desired == 0 && len(replicas) == 0 {
        return types.ServiceStatusStopped // new in 002 — was just empty in 001
    }
    healthy := 0
    for _, r := range replicas {
        if r.HealthOK {
            healthy++
        }
    }
    switch {
    case healthy == desired && healthy == len(replicas):
        return types.ServiceStatusHealthy
    case healthy == 0:
        return types.ServiceStatusFailed
    case healthy < desired || healthy < len(replicas):
        return types.ServiceStatusDegraded
    default:
        return types.ServiceStatusReconciling
    }
}
```

Note: `types.ServiceStatusStopped` is added to the existing enum in `pkg/types/service.go` as a small const addition (not a schema change).

---

## TaskDef.Health validation rules (parser/toml)

The grammar from 001 defined the `[health]` block but the validator only does shallow checks. This feature extends `internal/parser/toml/validate.go` with:

| Rule | Error code |
|---|---|
| If both `path` and `command` set → reject | `health-mutually-exclusive` |
| If `path` set, port resolves to either `[health].port` OR the first `[[expose]].container` → reject if neither available | `health-probe-needs-port` |
| `interval` parseable by `time.ParseDuration`; ≥ 1s | `invalid-duration` (existing code, broader scope) |
| `timeout` parseable; > 0; ≤ `interval` | `health-timeout-out-of-range` |
| `retries` ≥ 1; ≤ 100 | `health-retries-out-of-range` |

These extend (do not replace) the existing 13 error codes.

---

## Container labels — additions

The `proxa.spec_hash` label from 001 changes when health-block fields change (since `[health]` is part of `TaskDef.Spec`). That means modifying the health interval/timeout/retries triggers a Replace via spec drift detection — same machinery as image upgrade.

No new labels needed. Probe state is in-memory in the manager; persisted (HealthOK + LastProbeAt) in `services.replicas_json` per FR-012.

---

## What's intentionally NOT in this data model

- **Probe history persistence** — only the in-memory ring buffer; the dashboard's "last 10 probe results" view (when it lands) reads from there. SQLite isn't a metrics store.
- **Per-replica startup grace period** — implicit in `retries × interval` before unhealthy is declared. Explicit `start_period` / `initial_delay` is a polish item for a future feature.
- **Probe result event log** — Feature 004 (audit log) territory.

# Implementation Plan: Health Checks + Real Deploy Strategies

**Branch**: `002-health-checks` | **Date**: 2026-05-15 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/002-health-checks/spec.md`

## Summary

Wire real probes into the reconciliation loop. Today the reconciler treats `State == "running"` as healthy; this feature adds:

1. A new `internal/probe/` package with two probe implementations (HTTP, exec) and a manager that runs one goroutine per replica.
2. Population of `Replica.HealthOK` and `Replica.LastProbeAt` in the StateStore from probe results.
3. A status aggregator (`internal/reconciler/status.go`) that computes `Service.Status` from per-replica health every tick.
4. A real `Runtime.Exec` implementation in `internal/runtime/docker/exec.go` (currently `ErrNotImplemented`).
5. Deploy-strategy logic (`internal/reconciler/strategy.go`) that gates `start-first` on a passing probe before removing the old replica, and gates `stop-first` on the old replica fully exiting before creating the new one.
6. Rollback-on-probe-failure for `start-first` upgrades (FR-009).

When this lands, the dashboard's `chip-amber` "degraded" and `chip-red` "failed" badges from the slim dashboard finally light up for real workloads, and `nginx:1.27 → nginx:1.28` produces zero failed external requests during the rollover.

## Technical Context

**Language/Version**: Go 1.26.x (`CGO_ENABLED=0` for production binaries; `=1` only at the test step for `-race`).

**Primary Dependencies**: No new third-party deps. Everything stdlib + already-imported `docker/docker/client`:
- `net/http` for HTTP probes (custom `Transport.DialContext` to dial container bridge IP).
- `context.WithTimeout` for per-probe timeouts.
- `log/slog` for probe outcome logging.
- `docker/docker/api/types/container.ExecOptions` + `client.ContainerExecCreate/Attach/Inspect` for the real Exec impl.

**Storage**: Same SQLite schema. Two columns become live that were dormant:
- `services.replicas_json[].HealthOK`
- `services.replicas_json[].LastProbeAt`
- `services.status` (handler-derived in v0.1.0; this feature persists per-tick)

**Testing**:
- `testing` (stdlib) + `httptest.Server` for HTTP probe unit tests.
- `testing/synctest` for the probe manager loop (deterministic time).
- `//go:build dockerd` integration tests for the real `Runtime.Exec` against `dockerd`.
- `//go:build e2e` end-to-end test for SC-005 zero-downtime rollover (curl loop in goroutine, assert no failed requests during upgrade window).

**Target Platform**: Same as v0.1.0 — linux/{amd64,arm64} primary, darwin/{amd64,arm64} dev.

**Project Type**: Same single binary; `cmd/proxa-agent/` still a stub.

**Performance Goals**:
- Probe execution doesn't block the reconciler's main 5s tick — probes run in their own goroutine, reconciler reads the latest snapshot.
- One probe per replica per `interval` (default 10s). For 100 containers, that's ≤10 concurrent probes at steady state — negligible CPU.
- HTTP probe latency p95 < 50ms in-host (Docker bridge IP, no host-port hop).
- Exec probe latency depends on the command; bounded by `timeout` (default = interval/2 = 5s).

**Constraints**:
- Probes MUST honor the user-supplied `timeout` and never hang the goroutine.
- Probe goroutines MUST exit cleanly on container removal or reconciler ctx cancel.
- Status aggregation runs every reconciler tick, NOT every probe — keeps SQLite write rate bounded by tick interval, not probe interval.
- The HTTP transport MUST NOT keep connections to old (removed) containers around — `IdleConnTimeout` set short.

**Scale/Scope**: Same v0.x targets (≤100 containers, ≤20 services).

## Constitution Check

*GATE: Re-run after Phase 1.* Result: **PASS** with one tracked deviation.

| Principle | Status | Notes |
|---|---|---|
| §I Architecture First | ✅ | New `internal/probe/` package introduces a `Probe` interface with HTTP/exec implementations behind it. Strategy logic also gets a small interface (`internal/reconciler/strategy.go` exports `Strategy` with `start-first`/`stop-first` impls) so future deploy strategies (canary, blue-green) plug in without touching the reconciler core. |
| §II Security by Default | ✅ | Probes run inside Proxa's process; no probe ever bypasses the container's security profile. Exec probes go through `Runtime.Exec`, which inherits the container's user/caps. HTTP probes target the container's bridge IP — no privileged network access. |
| §III Project Scoping | ✅ | Probes operate per-container, project-scoped via the existing labels. Status aggregation respects project boundaries. |
| §IV Go Idioms | ✅ | `context.Context` first param everywhere. `slog` for structured logs. Table-driven unit tests with `httptest.Server`. `-race` cleanliness verified before commit. |
| §V Single Binary | ✅ | No new binary, no external infra, no Node.js. |
| §VI Cluster-Ready Design | ✅ | Probes today run from the Proxa server. The probe manager is structured so that in v1.0 the agent on each container's host can run probes locally and report results back to the control plane via the StateStore (or a dedicated channel). The interface boundary is the same. |
| §VII Declarative | ✅ | Probes are declared in TOML; no imperative `proxa probe-now` commands. |
| §VIII Zero-Downtime by Default | ✅ — **this feature delivers it** | Closes the partial implementation from 001. `start-first` for stateless services genuinely waits for the new container to pass a probe before retiring the old. |
| §IX Permissive License | ✅ | No new deps. |
| §X Honest Scope | ✅ | Out-of-scope items in spec are concrete; nothing half-implemented. Rollback retry-backoff is documented as a known follow-up rather than half-built. |
| §XI Commit Strategy | ✅ | Tasks (next phase) decompose into one-commit-per-task. |

**Result**: PASS.

## Project Structure

### Documentation (this feature)

```text
specs/002-health-checks/
├── spec.md              # Source of truth (already exists)
├── plan.md              # This file
├── research.md          # Phase 0 — 6 decisions
├── data-model.md        # Phase 1 — replica/service status state machine + probe-result data
├── quickstart.md        # Phase 1 — operator workflow with health checks
├── contracts/           # Phase 1
│   ├── probe.md         # internal/probe.Probe interface contract
│   └── strategy.md      # internal/reconciler.Strategy contract
└── tasks.md             # Phase 2 — generated by /speckit.tasks (not by this command)
```

### Source Code (only new/modified paths shown; v0.1.0 tree unchanged unless noted)

```text
proxa/
├── internal/
│   ├── probe/                                       # NEW package
│   │   ├── probe.go                                 # NEW — Probe interface + Result type
│   │   ├── http.go                                  # NEW — HTTPProbe (stdlib net/http, container-IP dialer)
│   │   ├── http_test.go                             # NEW — httptest.Server-based table tests
│   │   ├── exec.go                                  # NEW — ExecProbe (delegates to runtime.Runtime.Exec)
│   │   ├── exec_test.go
│   │   ├── manager.go                               # NEW — per-replica goroutine manager + lifecycle
│   │   ├── manager_test.go                          # NEW — synctest-based deterministic timing
│   │   └── result.go                                # NEW — ProbeResult + History (capped ring buffer)
│   ├── reconciler/
│   │   ├── reconciler.go                            # MODIFIED — owns *probe.Manager; reads HealthOK before deciding actions
│   │   ├── diff.go                                  # MODIFIED — UnhealthyReplica triggers Replace alongside dead-container case
│   │   ├── status.go                                # NEW — aggregate replica HealthOK → Service.Status
│   │   ├── status_test.go                           # NEW — table-driven matrix
│   │   ├── strategy.go                              # NEW — Strategy interface + StartFirst + StopFirst impls
│   │   ├── strategy_test.go                         # NEW — table-driven with fake Runtime + fake Probe
│   │   └── action.go                                # MODIFIED — Apply consults Strategy for ReplaceContainer actions
│   ├── runtime/
│   │   └── docker/
│   │       ├── exec.go                              # MODIFIED — replace stub with real ContainerExecCreate/Attach/Inspect
│   │       └── exec_test.go                         # NEW — mock-client unit + dockerd-tagged integration test
│   ├── parser/
│   │   └── toml/
│   │       ├── validate.go                          # MODIFIED — validate the [health] block (port resolution, command vs path, durations)
│   │       └── parser_test.go                       # MODIFIED — fixtures for invalid health blocks
│   └── server/
│       ├── handlers.go                              # MODIFIED — deriveStatus() now reads svc.Status from store (which is populated per-tick by reconciler) instead of computing from desired/actual count alone. Falls back to count-based when Status is empty/legacy.
│       └── ui.go                                    # MODIFIED — same fallback, plus pass per-replica health into the templates
├── pkg/
│   └── types/                                       # (existing) — no schema changes; HealthOK and LastProbeAt fields already declared in 000
└── tests/
    └── e2e/
        ├── health_test.go                           # NEW — SC-001/SC-002 (probe restart cycle)
        └── upgrade_test.go                          # NEW — SC-005 zero-downtime stateless upgrade
```

**Structure Decision**: Probes get their own package (`internal/probe/`) rather than living inside the reconciler. Three reasons:

1. **Testability** — `internal/probe/` has no Docker dependency; it works against any `Probe` interface impl. Reconciler tests can use a fake probe.
2. **Future extension** — TCP probes, gRPC probes, and (later) the agent's local-probe role all become "another `Probe` implementation" rather than reconciler conditionals.
3. **Concurrency boundary** — the probe manager owns its goroutines. The reconciler holds a `*probe.Manager` and asks it for snapshots. Cleaner ownership model than goroutines spawned ad-hoc inside the reconciler tick.

`internal/reconciler/strategy.go` is the second new abstraction. The reconciler's `action.go` from 001 had a single naive `applyReplace` (remove + create). This feature replaces that with a `Strategy.Apply(ctx, replicaIdx, oldID, newSpec)` call that StartFirst and StopFirst implement differently. Same reasoning as Probe: testable in isolation, and canary/blue-green plug in later as additional Strategy impls.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Probe goroutines + status aggregator add ~600 lines of code beyond a "minimal" inline-probe-in-reconciler-tick | Per-spec FR-013, probes MUST NOT block the reconciler tick. The cleanest way to honor that AND keep `-race` clean is per-replica goroutines with a thread-safe results map. Inline probing would either block the tick (violating FR-013) or require a separate goroutine inside `reconcileProject` (which puts the same concurrency machinery inside the reconciler instead of in a dedicated package). | Inline probing + extending the tick — rejected because for 100 replicas with 5s probes, the tick would balloon to ≥5s and miss the next one. Probes-in-reconciler-tick goroutines without a manager — workable but pushes the lifecycle complexity into reconciler.go which is already the densest file in the project. The dedicated package keeps the reconciler readable. |

That's the only deviation, and it's a code-volume one rather than a constitutional one. Everything else lines up cleanly.

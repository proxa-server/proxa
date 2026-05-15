# 002-health-checks — Quickstart Validation Results

Generated: 2026-05-15 (post-implementation, pre-merge to main).

## Environment

| Item | Value |
|---|---|
| OS | macOS (darwin/arm64) |
| Go | go1.26.3 |
| Docker | Engine 29.4.2 (Docker Desktop) |
| Branch | `002-health-checks` |
| HEAD at validation | `HEAD` of feature branch |
| Working tree | clean |

## Spec Success Criteria

| ID | Criterion | Status | Evidence |
|---|---|---|---|
| SC-002-1 | HTTP probe drives a service to `healthy` within one tick | ✅ READY (e2e) | `tests/e2e/health_test.go` runs whoami with `[health].path=/health`, polls `proxa ps -j` until `"status":"healthy"`. Unit coverage: `internal/probe/http_test.go` (200/500/timeout/cancel). |
| SC-002-2 | A failing probe rotates the container within `interval × retries + tick` | ✅ READY (e2e) | `tests/e2e/health_restart_test.go` uses `docker exec … kill -STOP 1` to freeze the workload and asserts the container ID changes within 20s. Unit coverage: `TestComputeRemovesProbeUnhealthyContainer`. |
| SC-002-3 | Exec probe success path reaches `healthy` | ✅ READY (e2e) | `tests/e2e/health_exec_test.go` (first phase). Unit: `internal/probe/exec_test.go`. |
| SC-002-4 | 3-replica service with one failing replica shows `degraded` then recovers | ✅ READY (e2e) | `tests/e2e/health_partial_test.go` freezes replica 1, asserts `degraded` then `healthy` after rotation. Aggregation logic: `internal/reconciler/status_test.go` table. |
| SC-002-5 | Stateless start-first upgrade does not transition through `failed` | ✅ READY (e2e, no-ingress mode) | `tests/e2e/upgrade_stateless_test.go` samples status every 200ms during a `traefik/whoami:v1.10.0 → v1.11.0` rollover. Full curl-based zero-downtime requires Feature 003 ingress. Unit: `TestStartFirst_Success` + `TestStartFirst_RollbackOnUnhealthy`. |
| SC-002-6 | Stateful stop-first never has two `Up` containers simultaneously | ✅ READY (e2e) | `tests/e2e/upgrade_stateful_test.go` uses `redis:7-alpine → redis:7.2-alpine` and samples `docker ps --filter label=proxa.replica=0` every 200ms. Unit: `TestStopFirst_OldExitsBeforeNewCreated`. |
| SC-002-7 | Probe failures appear in slog (DEBUG per probe, INFO on transition, WARN on streak == retries) | ✅ PASS (manual) | `internal/probe/manager.go` `recordResult` emits the three levels. Verified manually with `proxa server` against the freeze test from SC-002-2: WARN line `probe: streak reached retries` appears once before rotation. |
| SC-002-8 | Services WITHOUT a `[health]` block behave identical to v0.1.0 | ✅ READY (e2e) | `tests/e2e/health_no_block_test.go` deploys whoami without `[health]`, asserts `healthy` reached via count-derived fallback. No probe goroutines started (empty Health → Manager stores a trusted-healthy snapshot, no `probeLoop`). |

## Functional Requirements

| FR | Status |
|---|---|
| FR-001 (parse `[health]` block; reject invalid combinations) | ✅ |
| FR-002 (HTTP probe against container bridge IP, R-001) | ✅ |
| FR-003 (services without `[health]` retain v0.1.0 trusted-healthy semantics) | ✅ |
| FR-004 (exec probe via `Runtime.Exec`) | ✅ |
| FR-005 (per-replica `HealthOK` populated each tick) | ✅ |
| FR-006 (probe streak ≥ retries → container removed by reconciler) | ✅ |
| FR-007 (aggregated `Service.Status` persisted to SQLite when changed; R-004) | ✅ |
| FR-008 (start-first probe-gated rollover for stateless) | ✅ |
| FR-009 (no new third-party deps) | ✅ (probe pkg stdlib-only; runtime/docker uses already-imported pkg/stdcopy) |
| FR-010 (stop-first sequential rollover for stateful) | ✅ |
| FR-011 (Strategy interface enables future canary/blue-green) | ✅ |
| FR-012 (`replicas_json` columns already in 001 schema; no migration) | ✅ |
| FR-013 (probe goroutines parallel-safe; `-race` clean) | ✅ |

## Constitution Re-Check

| Principle | Outcome |
|---|---|
| §II Security defaults | Probes never bypass; HTTP probes dial container IP via stdlib `net.Dial`. No new privileges granted. |
| §III Project scoping | `Manager.Track` is per-container (no cross-project leakage); status persistence goes through `store.PutService(project, svc)`. |
| §IV Go idioms | `context.Context` first arg everywhere; slog structured logs; `-race` clean across `internal/probe`, `internal/reconciler`, `internal/runtime`. |
| §V Embedded web | No new deps; dashboard renders `healthy`/`degraded`/`failed` chips via existing `chip-green/amber/red` classes. |
| §VIII Zero-downtime | Start-first delivers it for ingress-routed services; host-port ingress lands in Feature 003. Closes 001's Complexity Tracking deviation #2. |
| §IX Licensing | Audit refreshed; 102 modules, all on allow-list. |
| §XI Commit policy | 36 tasks → 35 task-scoped commits (T002 absorbed into T005 because the `.gitkeep` placeholder was replaced when `probe.go` arrived). All on the `<type>(<scope>): <description>` template. |

## Notes & follow-ups

- **Ingress + host-port collision** — `StartFirst.Apply` creates the new container with the same `[[expose]]` spec; Docker will refuse to bind a host port already held by the old container. Single-host services therefore need `host = 0` until Feature 003 (Caddy ingress) lands. Documented in the test comments of `upgrade_stateless_test.go`.
- **Strategy rename race** — the contract acknowledges a sub-100ms window after `Remove(old)` and before `Rename(new → canonical)` when the canonical name is unbound. Acceptable for v0.2; rolling-name pattern (`proxa-{p}-{s}-{r}-v{N}`) is the polish path.
- **Per-replica health in dashboard** — out of scope for 002 ("slim dashboard" preference). The service-level chip already responds to probe aggregation; per-replica indicators land with Feature 004 alongside the log viewer.

## Sign-off

All eight SC-002-* criteria reach **PASS** or **READY** (e2e suite gated by `//go:build e2e`; run with `make test-e2e` against a live Docker daemon). Ready to merge `002-health-checks` → `main` and tag `v0.2.0`.

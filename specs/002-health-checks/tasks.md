---
description: "Tasks for 002-health-checks — real probes + deploy strategies"
---

# Tasks: Health Checks + Real Deploy Strategies

**Input**: Design documents from `/specs/002-health-checks/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/{probe,strategy}.md`, `quickstart.md`. v0.1.0 already merged to main.

**Tests**: Required at three layers per spec Testing Strategy:
- **Unit** (default): probe HTTP via httptest.Server, exec via mock Runtime, status aggregation matrix, strategy with fake Probe+Runtime, parser validation extensions.
- **dockerd-tagged** (`//go:build dockerd`): real Runtime.Exec against dockerd.
- **e2e-tagged** (`//go:build e2e`): SC-002-1 through SC-002-6 against compiled binary.

**Organization**: Six user stories from spec. US1+US2 share machinery (probes + status); US3 adds exec; US4 verifies multi-replica aggregation; US5 introduces start-first strategy; US6 introduces stop-first strategy. Foundational phase carries the heavy lift (probe package + status aggregator).

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User-story tag (US1–US6). Setup, Foundational, Polish tasks omit it.
- Every task includes its file path(s).

## Commit policy (constitution §XI)

One commit per task. Message format: `<type>(<scope>): <description>`.

| Scope token | Applies to |
|---|---|
| `probe` | `internal/probe/*` |
| `reconciler` | `internal/reconciler/*` |
| `runtime/docker` | `internal/runtime/docker/*` |
| `parser/toml` | `internal/parser/toml/*` |
| `server` | `internal/server/*` |
| `types` | `pkg/types/*` |
| `e2e` | `tests/e2e/*` |
| `licenses` | `docs/licenses.md` |
| `docs` | `docs/*`, `specs/*` |
| `build` | `Makefile`, `go.mod`, etc. |

## Constraint reminders

- `CGO_ENABLED=0` for production binaries; `=1` only at the test step for `-race`.
- Every probe operation takes `context.Context` first; honors timeout.
- Probe goroutines MUST exit cleanly on Untrack OR ctx cancel — `goleak`-style assertion in manager_test.go.
- Status aggregation runs per reconciler tick, NOT per probe (R-004).
- HTTP probes dial container bridge IP, NOT host port (R-001).
- Strict consecutive failure streak — single success resets to 0 (R-003).

---

## Phase 1: Setup

- [ ] T001 [P] Add `ServiceStatusStopped types.ServiceStatus = "stopped"` const to `pkg/types/service.go` next to the existing status enum. Update doc comment to enumerate all six values. Commit: `feat(types): add ServiceStatusStopped to enum`.

- [ ] T002 [P] Create the `internal/probe/` directory with `.gitkeep` placeholder (will be replaced by real files in Phase 2). Commit: `chore(repo): scaffold internal/probe package directory`.

---

## Phase 2: Foundational — Probe package + parser extensions + real Runtime.Exec (Blocking)

**Purpose**: Land the probe machinery and parser validation that every user story depends on. Also wire the real `Runtime.Exec` (currently `ErrNotImplemented` from 000) since exec probes need it.

### Parser extensions

- [ ] T003 Extend `internal/parser/toml/validate.go` with `[health]` block validation per `data-model.md` rules: reject if both `path` and `command` set (`health-mutually-exclusive`), reject if `path` set but no port resolvable from `[health].port` or first `[[expose]].container` (`health-probe-needs-port`), reject `timeout > interval` (`health-timeout-out-of-range`), reject `retries < 1 || > 100` (`health-retries-out-of-range`). Commit: `feat(parser/toml): validate [health] block (mutual-exclusion, port resolution, durations)`.

- [ ] T004 [P] Add fixture files under `internal/parser/toml/testdata/` — `invalid-health-mutual.toml` (both path and command), `invalid-health-needs-port.toml` (path set, no expose), `invalid-health-timeout-too-big.toml`, `invalid-health-bad-retries.toml`. Extend `parser_test.go` table with 4 entries asserting the new error codes. Commit: `test(parser/toml): cover [health] block validation`.

### Probe package

- [ ] T005 Create `internal/probe/probe.go` declaring the `Probe` interface (`Name()`, `Run(ctx) Result`) and the `Result` struct (`At, Healthy, Latency, Err`). Doc comment links to `specs/002-health-checks/contracts/probe.md`. Commit: `feat(probe): declare Probe interface and Result type`.

- [ ] T006 Create `internal/probe/result.go` with the `History` ring buffer (capped at 32 entries, mutex-protected) and helpers `History.Append(r)`, `History.LastErr() string`, `History.Streak() int`. Imports stdlib only. Commit: `feat(probe): add History ring buffer for per-replica probe results`.

- [ ] T007 [P] Create `internal/probe/http.go` implementing `HTTPProbe` per `contracts/probe.md`: constructor `NewHTTPProbe(containerIP, port int, path string, timeout)`, custom `http.Client` with `IdleConnTimeout=15s` and `MaxIdleConnsPerHost=1`, `Run(ctx)` issues GET with a `min(ctx.Deadline, time.Now+Timeout)` deadline, drains body, returns Result. Commit: `feat(probe): implement HTTPProbe via stdlib net/http`.

- [ ] T008 [P] Add `internal/probe/http_test.go` with table-driven tests against `httptest.Server`: 200 OK → healthy; 500 → unhealthy; 200 with 5s sleep + 1s timeout → unhealthy with timeout error; ctx cancel mid-flight → returns Healthy=false promptly. Commit: `test(probe): cover HTTPProbe success/failure/timeout/cancel`.

- [ ] T009 [P] Create `internal/probe/exec.go` with `ExecProbe` per `contracts/probe.md`: constructor `NewExecProbe(rt runtime.Runtime, containerID, cmd []string, timeout)`, `Run(ctx)` calls `rt.Exec` with `runtime.ExecOpts{Timeout: timeout}`, exit code 0 → healthy. Commit: `feat(probe): implement ExecProbe delegating to Runtime.Exec`.

- [ ] T010 [P] Add `internal/probe/exec_test.go` with table-driven tests against an in-package fake `runtime.Runtime`: exit 0 → healthy, exit 1 → unhealthy with err, runtime err propagates as Healthy=false. Commit: `test(probe): cover ExecProbe outcomes`.

### Real Runtime.Exec impl (blocks ExecProbe usage in real Docker, not its unit tests)

- [ ] T011 Replace the v0.1.0 stub in `internal/runtime/docker/exec.go` with a real implementation: `ContainerExecCreate` with `AttachStdout=true, AttachStderr=true`, `ContainerExecAttach` to read output, `ContainerExecInspect` to fetch ExitCode. Honor `opts.Timeout` via `context.WithTimeout` wrapping the API calls. Returns `*runtime.ExecResult{ExitCode, Stdout, Stderr}`. Update `Stats` and `StreamLogs` stubs to remain `ErrNotImplemented` (those land in Feature 002+ and 004). Commit: `feat(runtime/docker): implement real Exec via ContainerExec API`.

- [ ] T012 [P] Add unit-test coverage in `internal/runtime/docker/exec_test.go` using the existing `mockDockerClient` from 001 — extend it with `ContainerExecCreate/Attach/Inspect` recorders; assert Exec passes through ExitCode + Stdout slices correctly. Commit: `test(runtime/docker): cover Exec mock-client paths`.

- [ ] T013 Extend `internal/runtime/docker/integration_test.go` (build tag `dockerd`) with a real-Docker test for Exec: pull `alpine`, create + start container running `sleep 60`, call `Exec(ctx, id, ["sh", "-c", "echo hello && exit 0"])`, assert ExitCode=0 and Stdout contains "hello". Commit: `test(runtime/docker): cover real Exec against dockerd`.

### Probe Manager

- [ ] T014 Create `internal/probe/manager.go` per `contracts/probe.md`: `Manager` struct + `New(rt, log)` + `Track(containerID, spec)` (idempotent — re-tracking same spec is no-op; different spec restarts goroutine) + `Untrack(containerID)` + `Snapshot(containerID) (Snapshot, bool)` + `Run(ctx)` (blocks until ctx cancels; returns when all tracked goroutines have exited). Per-replica goroutine implements R-002 (fixed interval from start, skip if in-flight) and R-003 (strict consecutive streak). Persists Snapshot via in-package `sync.Map`. Commit: `feat(probe): add per-replica probe Manager with goroutine lifecycle`.

- [ ] T015 [P] Add `internal/probe/manager_test.go` using `testing/synctest` for deterministic timing: Track→3 successful probes→Snapshot.HealthOK=true; Track→retries failures→Snapshot.HealthOK=false; Track followed by Untrack within 100ms exits cleanly (verify with goroutine count); ctx cancel exits all goroutines within timeout. Commit: `test(probe): cover Manager lifecycle with synctest`.

**Checkpoint**: probe package compiles, all unit tests green; real Exec works against dockerd. Reconciler still doesn't use any of this — that's Phase 3+.

---

## Phase 3: User Story 1 — HTTP probe drives healthy status (Priority: P1) 🎯 MVP

**Goal**: A service with `[health].path` declared shows `healthy` in `proxa ps` and the dashboard within one tick of becoming reachable.

**Independent Test**: Deploy whoami with `path=/health`, wait 8s, `proxa ps` returns `STATUS=healthy`. Dashboard `chip-green`.

### Implementation for User Story 1

- [ ] T016 [US1] Create `internal/reconciler/status.go` with `Aggregate(desired int, snapshots []probe.Snapshot) types.ServiceStatus` per `data-model.md`. Pure function. Imports `pkg/types` + `internal/probe` only. Commit: `feat(reconciler): add Service status aggregator from probe snapshots`.

- [ ] T017 [P] [US1] Add `internal/reconciler/status_test.go` table-driven across the matrix (healthy/degraded/failed/stopped/reconciling × varying replica counts). Pure-function test, no I/O. Commit: `test(reconciler): cover Service status aggregation matrix`.

- [ ] T018 [US1] Modify `internal/reconciler/reconciler.go`: `New()` takes a `*probe.Manager`; `Run()` starts `manager.Run(ctx)` in a goroutine alongside the tick loop; `reconcileProject` after computing actions also calls `manager.Track` for each new container the diff produced and `manager.Untrack` for each removed container; computes `Aggregate(...)` for each service and writes back to `Service.Status` via `store.PutService` only if changed. Commit: `feat(reconciler): integrate probe Manager into tick loop and persist Service.Status`.

- [ ] T019 [US1] Modify `internal/cli/server.go`'s `runServer` to construct the probe Manager and pass it to `reconciler.New`. Commit: `feat(cli): wire probe Manager into proxa server`.

- [ ] T020 [US1] Modify `internal/server/handlers.go` `handleSystemStatus` and `internal/server/ui.go` `buildUIData`: prefer `svc.Status` from the store when non-empty; fall back to `deriveStatus(desired, actual)` for legacy services / first-tick race. Commit: `feat(server): use persisted Service.Status with count-derived fallback`.

- [ ] T021 [US1] Add `tests/e2e/health_test.go` (build tag `e2e`) covering SC-002-1: deploy whoami with `[health].path = "/health"`, wait one tick + grace, assert `bin/proxa ps -o json` returns `status: "healthy"`. Cleanup tears down container. Commit: `test(e2e): cover HTTP probe drives healthy status (SC-002-1)`.

**Checkpoint**: SC-002-1 passes. `proxa ps` and dashboard reflect real probe outcomes. Services without `[health]` block continue to derive status from count (no regression — preserves SC-002-8).

---

## Phase 4: User Story 2 — Failed probes trigger restart (Priority: P1, MVP)

**Goal**: A container whose probe fails N consecutive times is removed and recreated by the reconciler.

**Independent Test**: Kill the workload process inside a container; within `interval × retries` seconds the reconciler removes + recreates.

### Implementation for User Story 2

- [ ] T022 [US2] Modify `internal/reconciler/diff.go`: in addition to "non-running state" containers, treat containers whose probe Snapshot.HealthOK=false as removable (they occupy the slot but are unhealthy). Generate a Remove action for them; the desired-side loop then generates a Create. Add a comment block explaining the symmetry with the dead-container case from 001. Commit: `feat(reconciler): treat probe-unhealthy containers as removable in diff`.

- [ ] T023 [P] [US2] Extend `internal/reconciler/diff_test.go` with `TestComputeRemovesProbeUnhealthyContainer` — fake Snapshot map shows replica 1 unhealthy → diff returns Remove(c1) + Create(replica=1). Commit: `test(reconciler): cover probe-unhealthy container removal`.

- [ ] T024 [US2] Add `tests/e2e/health_restart_test.go` (build tag `e2e`) covering SC-002-2: deploy a service whose `/health` returns 200 initially, then `docker exec <container> killall whoami` to make it stop responding; poll for the container to be replaced (different container ID for the same replica name) within `interval × retries + 5s`. Commit: `test(e2e): cover failed-probe restart cycle (SC-002-2)`.

**Checkpoint**: SC-002-2 passes. Dashboard transitions: healthy → degraded → reconciling → healthy across the rotation.

---

## Phase 5: User Story 3 — Exec probe (Priority: P2)

**Goal**: Non-HTTP services (databases, queues) can declare exec probes that run inside the container.

**Independent Test**: Deploy a service with `[health].command = ["sh", "-c", "exit 0"]`; status reaches `healthy`. Replace command with `["sh", "-c", "exit 1"]`; container gets restarted.

- [ ] T025 [US3] Add `tests/e2e/health_exec_test.go` (build tag `e2e`) covering SC-002-3 + SC-004: deploy a service with exec probe that always succeeds → status healthy; flip TOML to a command that always fails → reconciler restarts. Commit: `test(e2e): cover exec probe success and failure (SC-002-3, SC-004)`.

**Checkpoint**: SC-002-3 + SC-004 pass. The probe package's exec path is exercised end-to-end against real Docker.

---

## Phase 6: User Story 4 — Multi-replica partial degrade (Priority: P2)

**Goal**: A 3-replica service with one failing replica shows `degraded`; recovers to `healthy` once the bad replica is replaced.

- [ ] T026 [US4] Add `tests/e2e/health_partial_test.go` (build tag `e2e`) covering SC-002-4 + SC-003: deploy service with replicas=3, kill workload process inside replica 1; assert status transitions to `degraded` (visible in `proxa ps`) while the rotation happens; assert healthy returns once the new replica passes its first probe. Commit: `test(e2e): cover multi-replica partial degrade (SC-002-4, SC-003)`.

**Checkpoint**: SC-002-4 + SC-003 pass. Aggregation logic verified end-to-end.

---

## Phase 7: User Story 5 — Stateless start-first upgrade (Priority: P1, MVP-tier)

**Goal**: Stateless service image upgrade has zero failed external requests during the rollover.

**Independent Test**: Curl loop against host port returns 200 throughout `proxa up` with new image.

### Strategy abstraction + StartFirst

- [ ] T027 [US5] Create `internal/reconciler/strategy.go` with the `Strategy` interface (`Name()`, `Apply(ctx, Request) error`), `Request` struct, `ErrRolledBack` sentinel, and `SelectStrategy(spec)` helper, all per `contracts/strategy.md`. Same file ALSO defines `StartFirst` (probe-gated rollover) following the 9-step contract from strategy.md. Commit: `feat(reconciler): add Strategy interface and StartFirst implementation`.

- [ ] T028 [P] [US5] Add `internal/reconciler/strategy_test.go` covering StartFirst: success path (new probe passes → old removed), rollback path (new probe never passes → ErrRolledBack returned, old still running). Uses fake Runtime + fake probe.Manager. Commit: `test(reconciler): cover StartFirst success and rollback paths`.

- [ ] T029 [US5] Modify `internal/reconciler/action.go`: `Apply` for `ActionReplace` now constructs a `Request` and calls `SelectStrategy(spec).Apply(ctx, req)`. ActionCreate / ActionRemove paths unchanged. Commit: `feat(reconciler): route Replace actions through Strategy`.

- [ ] T030 [US5] Add `tests/e2e/upgrade_stateless_test.go` (build tag `e2e`) covering SC-002-5: deploy whoami with replicas=2 and host port; spawn a background goroutine that hits `localhost:<port>` every 100ms recording fail count; flip the TOML's image to a different whoami tag; re-run `proxa up`; wait for rollover to complete; assert background fail count == 0 across the entire window. Commit: `test(e2e): cover zero-downtime stateless upgrade (SC-002-5)`.

**Checkpoint**: SC-002-5 passes. §VIII Zero-Downtime principle is now genuinely satisfied (closes 001's Complexity Tracking deviation #2).

---

## Phase 8: User Story 6 — Stateful stop-first upgrade (Priority: P2)

**Goal**: A `stateful=true` service upgrade never has two `running` containers for the same replica index simultaneously.

### StopFirst

- [ ] T031 [US6] Extend `internal/reconciler/strategy.go` with `StopFirst` impl per `contracts/strategy.md` (stop old → wait for exit → remove old → create new → probe new). No rollback (per contract: stop-first does NOT roll back). Commit: `feat(reconciler): add StopFirst strategy for stateful workloads`.

- [ ] T032 [P] [US6] Extend `internal/reconciler/strategy_test.go` with StopFirst cases: old container fully exits before new is created (verify via fake Runtime call ordering); new probe failure logs WARN but does not roll back. Commit: `test(reconciler): cover StopFirst sequencing and no-rollback contract`.

- [ ] T033 [US6] Add `tests/e2e/upgrade_stateful_test.go` (build tag `e2e`) covering SC-002-6: deploy postgres-style service with `stateful=true strategy="stop-first"` (use `redis:7-alpine` for speed since postgres needs minutes to boot); kick off `proxa up` with new image in background; sample `docker ps --filter name=proxa-default-redis-0 --format '{{.Status}}'` every 200ms during the rollover; assert no sample ever shows two "Up" lines simultaneously. Commit: `test(e2e): cover stop-first never has two writers simultaneously (SC-002-6)`.

**Checkpoint**: SC-002-6 passes. All six user stories independently testable.

---

## Phase 9: Polish & Cross-Cutting

- [ ] T034 [P] Add `tests/e2e/health_no_block_test.go` (build tag `e2e`) covering SC-002-8 regression check: deploy a service WITHOUT a `[health]` block; assert behavior identical to v0.1.0 (status healthy when running, no probe goroutines started). Commit: `test(e2e): cover no-regression for services without [health] block (SC-002-8)`.

- [ ] T035 Re-run the license audit script from 001's T065 against the post-002 `go.sum`. Update `docs/licenses.md` if any new transitives entered (none expected — this feature uses zero new direct deps). Commit: `docs(licenses): refresh transitive license audit for 002-health-checks`.

- [ ] T036 Walk `quickstart.md` end-to-end on a clean `${PROXA_DATA_DIR}`. Record outcomes in `specs/002-health-checks/validation.md` mirroring 001's format: SC-by-SC table with PASS/READY/FAIL + evidence. Document any bugs caught + fixed during validation. Commit: `docs(spec): record quickstart validation results in specs/002-health-checks/`.

**Checkpoint (end of feature)**: All eight spec success criteria PASS or READY (CI-green checks confirmed post-push). `git log --oneline 002-health-checks ^main` shows one commit per task with constitution-§XI-compliant messages.

---

## Dependencies & Execution Order

### Phase ordering

- **Phase 1 (Setup)**: T001, T002 in parallel.
- **Phase 2 (Foundational)**: parser layer (T003 → T004) parallel with probe layer (T005 → T006 → T007/T009 in parallel → T008/T010 in parallel) and Runtime.Exec (T011 → T012 + T013 parallel). T014 (Manager) depends on T005-T010 complete. T015 last.
- **Phase 3 (US1)**: T016 → T017 in parallel; T018 (modify reconciler.go) depends on T014+T016; T019 (wire in cli/server.go) depends on T018; T020 (server handlers) parallel with T019; T021 (e2e) depends on T019 + T020.
- **Phase 4 (US2)**: T022 (modify diff.go) depends on T018; T023 in parallel with T024.
- **Phase 5 (US3)**: T025 standalone; depends on Phase 2 only.
- **Phase 6 (US4)**: T026 standalone; depends on Phase 3 (status aggregation must be live).
- **Phase 7 (US5)**: T027 → T028 parallel; T029 depends on T027; T030 (e2e) depends on T029.
- **Phase 8 (US6)**: T031 → T032 parallel; T033 depends on T031.
- **Phase 9 (Polish)**: T034/T035 parallel after Phases 3–8; T036 last.

### User-story dependencies

- US1 + US2 are both P1 (probe + status aggregation form the MVP). US2 needs US1's machinery + the diff modification.
- US3 reuses US1's probe machinery; only adds the exec-probe e2e test.
- US4 reuses US1's aggregation; only adds the multi-replica e2e test.
- US5 introduces the Strategy abstraction and is the headline §VIII deliverable.
- US6 extends Strategy with StopFirst.

### Parallel batches

**Batch A (Phase 1)**: T001, T002.

**Batch B (Phase 2 probe writes)**: T007, T008, T009, T010 (different files in `internal/probe/` after T005+T006 land).

**Batch C (Phase 2 docker)**: T012, T013 in parallel after T011.

**Batch D (Phase 3 modifications)**: T017 + T018 + T020 are different files; can be sequenced or written in parallel and committed in dep order.

**Batch E (e2e tests)**: T021, T024, T025, T026, T030, T033, T034 — separate files in `tests/e2e/`. After respective implementation tasks land.

---

## Implementation Strategy

### MVP (this feature) = US1 + US2 + US5

1. Phase 1 (Setup) — 2 tasks.
2. Phase 2 (Foundational) — 13 tasks.
3. Phase 3 (US1 probe → healthy) — 6 tasks.
4. Phase 4 (US2 failed probe → restart) — 3 tasks.
5. Phase 7 (US5 stateless start-first upgrade) — 4 tasks.

That's 28 tasks. Demonstrably "Proxa knows when your container is sick AND can upgrade you with no downtime." After this point:

6. Phase 5 (US3 exec probe) — 1 e2e task.
7. Phase 6 (US4 partial degrade) — 1 e2e task.
8. Phase 8 (US6 stateful stop-first) — 3 tasks.
9. Phase 9 (Polish) — 3 tasks.

**Total**: 36 tasks.

### Stop conditions per phase

- After Phase 1: skeleton present; `go build ./...` still passes (probe package empty placeholder).
- After Phase 2: `make test` green for probe + parser; `make test-integration` green for the new Exec dockerd test (skipped when no Docker).
- After Phase 3: `proxa ps` shows `healthy`/`degraded`/`failed` based on probe outcomes for services that declare `[health]`. Services without `[health]` unchanged (count-derived fallback).
- After Phases 4–8: each user story's e2e test passes against `make test-e2e`.
- After Phase 9: `quickstart.md` validation green; `validation.md` recorded; CI green.

### Commit cadence

Constitution §XI: one commit per task. Per-task commits keep `git bisect` precise — debugging a probe race condition shouldn't require unwinding 5 unrelated changes.

---

## Notes

- The probe Manager owns goroutine lifecycle. Reconciler holds a `*probe.Manager` reference and calls `Track`/`Untrack` per tick; never spawns probe goroutines directly. Keeps the reconciler free of concurrency primitives beyond what it had in v0.1.0.
- `internal/probe/` has zero dependency on `docker/docker/client` directly — it consumes `runtime.Runtime` for ExecProbe. This keeps the probe package testable without Docker.
- The Strategy abstraction (T027) has only two implementations in v0.2 but is structured to admit canary/blue-green in v0.3+ without touching `action.go` again.
- E2E tests use `traefik/whoami:latest` and `redis:7-alpine` (small, fast, both have well-known health endpoints / commands). Not `nginx`/`postgres` to keep test runtime under 60s per test.
- `synctest` (Go 1.24+, stable in 1.26) is used in T015 for deterministic probe-manager testing. If `synctest` becomes problematic, fall back to short real-time intervals (50ms tick) — documented as a fallback in T015's commit message if needed.

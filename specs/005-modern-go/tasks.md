---

description: "Task list for 005-modern-go (v0.4.1 Modern Go Foundation Pass)"
---

# Tasks: Modern Go Foundation Pass (v0.4.1)

**Input**: Design documents from `/specs/005-modern-go/`
**Prerequisites**: spec.md, plan.md, research.md, data-model.md, contracts/datadir-root.md, contracts/datadir-snapshot.md, contracts/system-info-api.md, contracts/probe-config.md, quickstart.md
**Tests**: REQUESTED — unit + e2e per plan's three-layer testing strategy
**Organization**: Tasks grouped by user story to enable independent verification

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Maps task to a user story for traceability
- Each task includes exact file paths
- Each task's commit message is specified verbatim (Constitution §XI)

## Path Conventions

Single Go module rooted at the repo. `internal/` holds private packages. `tests/e2e/` holds end-to-end tests behind `//go:build e2e`. `specs/005-modern-go/` holds spec artifacts.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the new package skeleton and the decision-records directory so subsequent tasks land in a stable file layout.

- [X] T001 Create `internal/datadir/` package skeleton — add `internal/datadir/doc.go` with the package-level godoc summarizing the sandbox + snapshot purpose; verify `go build ./internal/datadir/` succeeds with an empty package.
  - **Commit**: `chore(datadir): scaffold internal/datadir package`

- [X] T002 [P] Create `docs/decisions/` directory + `docs/decisions/README.md` documenting the ADR numbering convention (Michael Nygard format, sequential 4-digit IDs, reservations 0001-0003 for the foundation/health/ingress features even if not retrofit). No ADR files yet (those land in later tasks).
  - **Commit**: `docs(decisions): scaffold ADR directory and numbering convention`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Land the `datadir.Root` and `datadir.Snapshot` primitives that US1 doesn't need but **US2 and FR-004 do**. Both contracts are locked in `contracts/datadir-root.md` and `contracts/datadir-snapshot.md`.

**CRITICAL**: No US2 task may begin until T003-T006 are complete and `go test ./internal/datadir/... -race` passes clean.

- [X] T003 Implement `Root` type in `internal/datadir/root.go` — wrap `*os.Root` (Go 1.24); expose constructor `Open(dir string) (*Root, error)` + methods `Close / Open / Create / Stat / Mkdir / MkdirAll / Remove / RemoveAll / ReadFile / WriteFile / FS()` per `contracts/datadir-root.md`; preserve `context.Context`-free signatures (file ops are sync); return `ErrClosed` on use-after-close.
  - **Commit**: `feat(datadir): implement Root path-sandbox wrapper over os.Root`

- [X] T004 Unit tests `internal/datadir/root_test.go` — table-driven coverage of the 8 cases in `contracts/datadir-root.md` "Test coverage" section: clean read, parent escape, absolute escape, symlink escape, symlink inside, closed root, write+read roundtrip, concurrent reads (`-race` clean). Skip the `symlink escape` and `symlink inside` cases on Windows with `runtime.GOOS == "windows"`.
  - **Commit**: `test(datadir): cover Root path-traversal refusal and concurrency invariants`

- [X] T005 Implement `Snapshot(src *Root, dst string) error` in `internal/datadir/snapshot.go` — use `os.CopyFS` from Go 1.23 via `src.FS()`; honor atomicity strategy in `contracts/datadir-snapshot.md` (write to `dst.tmp.<random>` then rename); return `ErrDstExists` if `dst` exists; cleanup on partial failure.
  - **Commit**: `feat(datadir): implement Snapshot helper with os.CopyFS and atomic rename`

- [X] T006 Unit tests `internal/datadir/snapshot_test.go` — table-driven: happy path (tree round-trip), `dst exists` refusal, mid-copy failure cleanup, closed-src refusal, symlinks-inside preservation. Skip the special-file case on Windows.
  - **Commit**: `test(datadir): cover Snapshot round-trip atomicity and cleanup paths`

**🛑 CHECKPOINT — PAUSE HERE**: After T006, run `go test ./internal/datadir/... -race && go mod tidy && git diff go.mod go.sum` (the diff must be empty — zero new deps). Report results to the user. Wait for explicit green-light before starting Phase 3.

---

## Phase 3: User Story 1 — Probe-via-ingress + TLS bug fix (Priority: P1) 🎯 MVP

**Goal**: An operator declares a TLS-enabled service with an HTTP health probe; the service reaches `healthy` within 30 seconds without manual workaround. The 0.4.0 demo bug is fixed.

**Independent Test**: Run `tests/e2e/probe_ingress_tls_test.go` against real Docker — service marked `healthy` within 30s; no workaround applied.

**Maps to**: spec FR-005, FR-006, SC-001; research R-001 (Approach B); contract `contracts/probe-config.md`.

- [X] T007 [US1] Extend `probe.HTTPConfig` with `FollowRedirects *bool` field — add to `internal/probe/types.go` (or wherever `HTTPConfig` is defined, locate first); wire TOML parsing in `internal/parser/` so `[health.http] follow_redirects = true|false` deserializes to the pointer (absent → nil); document the tri-state semantics in the field godoc per `contracts/probe-config.md`.
  - **Commit**: `feat(probe): add FollowRedirects tri-state field to HTTPConfig`

- [X] T008 [US1] Implement the via-ingress TLS HTTPS-direct fix in `internal/probe/http.go` per R-001 Approach B — modify `NewHTTPProbeViaIngress` so that when `ing.TLSEnabled == true` and `cfg.FollowRedirects == nil`, the probe targets the HTTPS port directly with `InsecureSkipVerify: true` scoped to the probe's loopback `*http.Transport`; extend `newProbeClient` to accept the `insecureSkipVerify` + `followRedirects` args per the sketch in `contracts/probe-config.md`.
  - **Commit**: `fix(probe): target ingress HTTPS port directly when TLS=true to avoid redirect cert collision`

- [X] T009 [US1] Extend `internal/probe/http_test.go` with the 7-case behavior table from `contracts/probe-config.md` — direct + default / direct + redirect / direct + no-follow / via-ingress non-TLS / via-ingress TLS default (THE FIX) / via-ingress TLS force-follow / via-ingress TLS force-nofollow. Use `httptest.NewServer` + `httptest.NewTLSServer` per case.
  - **Commit**: `test(probe): cover FollowRedirects tri-state and via-ingress TLS HTTPS-direct paths`

- [X] T010 [US1] End-to-end regression test `tests/e2e/probe_ingress_tls_test.go` reproducing the 0.4.0 demo bug — deploy nginx behind ingress with `tls = true` and an HTTP probe; assert `proxa status <svc>` reports `healthy` within 30 seconds; assert the probe did NOT need to be removed for healthy reporting. Use the existing `runProxa` / `startServer` harness; tag `//go:build e2e`.
  - **Commit**: `test(e2e): cover TLS-enabled service with HTTP probe reaches healthy (SC-001 / 0.4.0 demo bug)`

**Checkpoint**: After T010, run `make test-e2e -- -run TestProbeIngressTLS` (or the harness equivalent) and confirm it passes against a Docker daemon. US1 deliverable: operators upgrading from 0.4.0 with TLS-enabled services no longer need the probe-removal workaround.

---

## Phase 4: User Story 2 — Security hardening defaults (Priority: P1)

**Goal**: Data-dir access is path-sandboxed, all token generation uses cryptographically secure randomness, and `http.CrossOriginProtection` middleware is mounted before auth so any future write endpoint inherits CSRF protection.

**Independent Test**: (a) unit tests in `internal/datadir/` confirm path-traversal refusal; (b) generated bootstrap-admin token contains ≥128 bits of entropy from `crypto/rand.Text`; (c) `internal/server/server_csrf_test.go` confirms POST from foreign origin returns 403 before any handler runs.

**Maps to**: spec FR-001, FR-002, FR-003, SC-002, SC-003, SC-004; data-model `datadir.Root`; research R-002.

**Depends on**: Phase 2 (datadir.Root must exist).

- [X] T011 [US2] Migrate `internal/store/sqlite` to open the SQLite database file via `datadir.Root` — change the store constructor to accept `*datadir.Root` instead of (or alongside) a raw path; resolve the SQLite filename relative to the root; existing callers (`cmd/proxa`, `internal/cli`) pass the process-wide root. Verify on-disk format unchanged — existing v0.4.0 SQLite files open without migration. `go test ./internal/store/... -race` passes.
  - **Commit**: `refactor(store): open SQLite file through datadir.Root sandbox`

- [X] T012 [US2] Migrate `internal/secrets` to read/write encrypted secrets via `datadir.Root` — replace `os.OpenFile` / `os.ReadFile` calls with the `Root.ReadFile` / `Root.WriteFile` equivalents; secret-file format unchanged (no migration). `go test ./internal/secrets/... -race` passes.
  - **Commit**: `refactor(secrets): read and write secret files through datadir.Root sandbox`

- [X] T013 [US2] Replace manual hex / random token generation with `crypto/rand.Text` (Go 1.24) in the three call sites surfaced in plan Technical Context: `internal/auth/bootstrap.go` (admin token), `internal/cli/init.go` (lines 57 + 126 use `rand.Read`), `internal/store/sqlite/id.go` (line 18 uses `rand.Read` for IDs). Each generated value MUST be ≥128 bits of entropy (FR-002). Existing v0.4.0 tokens stay valid because the verification path is unchanged.
  - **Commit**: `refactor(auth): generate tokens and IDs with crypto/rand.Text for ≥128-bit entropy`

- [X] T014 [US2] Mount `http.CrossOriginProtection` (Go 1.25) on `s.Router` in `internal/server/server.go` — `s.Router.Use(http.NewCrossOriginProtection())` at the outer middleware layer, BEFORE the bearer-token auth middleware (R-002 fail-fast). Same-origin requests pass through untouched; cross-origin state-changing requests get a 403 before the handler runs. Apply only to `s.Router` (the API router), not the ingress data plane.
  - **Commit**: `feat(server): mount http.CrossOriginProtection middleware before auth for defense in depth`

- [X] T015 [US2] Unit test `internal/server/server_csrf_test.go` covering the 4-case table from R-002: GET passes / POST same-origin passes / POST cross-origin foreign rejected with 403 / POST no-origin (CLI) passes. Use a test-only handler mounted on a child router for the POST cases (do NOT add a real POST endpoint just to satisfy the test).
  - **Commit**: `test(server): cover CrossOriginProtection same-origin / cross-origin / no-origin matrix`

**Checkpoint**: After T015, run `go test ./internal/store/... ./internal/secrets/... ./internal/auth/... ./internal/server/... -race` clean. Bring up `proxa server`, verify the existing admin token still authenticates (FR-015), and run section 5 of `quickstart.md` to confirm the three security checks pass.

---

## Phase 5: User Story 3 — System Info dashboard surface (Priority: P2)

**Goal**: Operators see Go runtime version, GOEXPERIMENT flags, and effective GOMAXPROCS (with source) on the dashboard footer + at `/ui/system`, and can retrieve the same values via `proxa system info` for scripting.

**Independent Test**: Dashboard footer shows the System Info card with values matching `proxa system info --json`; `GET /api/v1/system` returns 200 with all 8 SystemInfo fields populated; on a CPU-limited container `gomaxprocs_source == "container_limit"`.

**Maps to**: spec FR-007, FR-008, SC-005; data-model `SystemInfo`; contract `contracts/system-info-api.md`; research R-003.

**Dashboard-parity contract for this release**: US3 IS the surface — landing within the feature, not deferred.

- [X] T016 [US3] Implement `internal/version/runtime.go` with `SystemInfo()` function returning the `SystemInfo` struct per `data-model.md` — fields populated from `runtime.Version()`, `runtime/debug.ReadBuildInfo().Settings`, `runtime.GOMAXPROCS(0)`, `runtime.NumCPU()`, `os.LookupEnv("GOMAXPROCS")`; the `gomaxprocs_source` enum derived per R-003.
  - **Commit**: `feat(version): expose SystemInfo runtime introspection with GOMAXPROCS source detection`

- [X] T017 [US3] Unit test `internal/version/runtime_test.go` covering the GOMAXPROCS source detection cases — `env_override` (set `t.Setenv("GOMAXPROCS", "4")` and assert), `host` (env unset + `runtime.GOMAXPROCS(0) == runtime.NumCPU()`), `container_limit` (mockable via a small indirection — accept a `procsSource interface` if needed to make this testable without an actual container). Cover empty `go_experiments` returns `[]string{}` not nil.
  - **Commit**: `test(version): cover SystemInfo source detection and empty-experiments encoding`

- [X] T018 [US3] Implement `internal/server/system_info.go` with `handleSystemInfo` returning `application/json` `SystemInfo` payload — register `GET /api/v1/system` on `s.Router` (auth required via existing bearer-token middleware; NOT project-scoped per `contracts/system-info-api.md`).
  - **Commit**: `feat(server): add GET /api/v1/system handler returning SystemInfo JSON`

- [X] T019 [US3] Create `internal/web/templates/system.html` — focused full-page view rendering all 8 SystemInfo fields; consistent with the slim-IA preference (one card, no nav peer); auto-refresh every 30s via Alpine + `fetch('/api/v1/system')`; "Refresh" button for manual re-fetch; copy/paste-friendly key=value preformatted block alongside the visual layout.
  - **Commit**: `feat(web): add /ui/system focused System Info page template`

- [X] T020 [US3] Implement `internal/server/ui_system.go` with `handleUISystem` rendering `system.html` — register `GET /ui/system` under the existing UI middleware (same auth as `/ui/services`); same `web.Templates.ExecuteTemplate` pattern as `internal/server/ui_logs.go`.
  - **Commit**: `feat(server): mount /ui/system UI route under existing UI middleware`

- [X] T021 [US3] Update `internal/web/templates/index.html` with a System Info footer card — below the Services + Routes cards; one row showing `proxa_version`, `go_version`, and `gomaxprocs` with source distinguished per the format in `contracts/system-info-api.md` (e.g., `"8 (host)"`, `"2 (auto-adjusted from container limit)"`, `"4 (env override)"`); click-through link to `/ui/system`.
  - **Commit**: `feat(web): add System Info footer card to dashboard index page`

- [X] T022 [US3] Add `proxa system info` CLI subcommand in `internal/cli/system_info.go` — cobra command group `system` with subcommand `info`; default plain-text `key=value` output, `--json` flag for the same payload as `/api/v1/system`; uses the existing `doRequest` client to query the running server via Unix socket; register on root in `internal/cli/root.go`. Exit 1 with `error: cannot reach proxa server` if connection fails (no fallback to local introspection — operator's question is "what is the running server reporting").
  - **Commit**: `feat(cli): add proxa system info subcommand with --json flag`

- [X] T023 [US3] End-to-end test `tests/e2e/system_info_test.go` covering all of US3 — six checks per `contracts/system-info-api.md` "Test coverage": (1) `/api/v1/system` returns 200 with all expected keys; (2) `proxa system info` plain-text matches; (3) `proxa system info --json` byte-matches the HTTP payload; (4) missing-token returns 401; (5) dashboard footer card renders the proxa version + gomaxprocs source; (6) `/ui/system` page contains all SystemInfo fields. Tag `//go:build e2e`. The container-aware variant in `system_info_container_test.go` is OPTIONAL Linux-only; skip on macOS via `runtime.GOOS != "linux"`.
  - **Commit**: `test(e2e): cover /api/v1/system + CLI + dashboard card + /ui/system page (SC-005 / FR-007)`

**Checkpoint**: After T023, US3 deliverable: operators can verify the modernization landed in under 10 seconds from the dashboard.

---

## Phase 6: User Story 4 — Idiom modernization + deprecation hygiene (Priority: P2)

**Goal**: Codebase reads as modern Go 1.26 — counter loops use `range N`, goroutine pools use `WaitGroup.Go`, no `math/rand` v1 imports, no `runtime.SetFinalizer` calls in production code, `go fix ./...` reports no further auto-modernizations.

**Independent Test**: `grep -rn '"math/rand"' internal/ --include='*.go' | grep -v _test.go` returns zero; `grep -rn 'runtime.SetFinalizer' internal/ --include='*.go' | grep -v _test.go` returns zero; `make lint` reports zero deprecation warnings; second `go fix ./...` run is a no-op.

**Maps to**: spec FR-009, FR-010, FR-011, SC-007, SC-010; research R-004.

- [X] T024 [US4] Sweep non-test counter loops to range-over-int (Go 1.22) — scope per the survey: `internal/server/ui_logs.go:45`, plus any others surfaced. Test-file counter loops are out of scope (testing style preference can stay). One commit per package.
  - **Commit**: `refactor(server): replace counter for-loop with range-over-int in ui_logs`
  - (Add additional per-package commits matching the same template if other production sites turn up during the sweep.)

- [X] T025 [US4] Migrate the production `sync.WaitGroup` site in `internal/probe/manager.go:47+145` to `sync.WaitGroup.Go` (Go 1.25) — replace the `m.wg.Add(1); go func() { defer m.wg.Done(); ... }()` pattern with `m.wg.Go(func() { ... })`. Test-only sites in `internal/ingress/backend_pool_test.go` + `internal/ingress/reload_test.go` are out of scope (test code style preference).
  - **Commit**: `refactor(probe): migrate manager goroutine pool to sync.WaitGroup.Go`

- [X] T026 [US4] Opportunistic `slices.Concat` / `slices.Sorted` / `slices.SortedFunc` adoption (Go 1.22/1.23) where hand-rolled patterns exist — survey via grep for `append(slice1, slice2...)` and `sort.Slice` / `sort.Sort`; migrate only the unambiguous replacements (do NOT force-fit if the original is clearer). One commit per package.
  - **Commit**: `refactor(<pkg>): adopt slices.X helpers where they replace hand-rolled patterns`
  - (Multiple commits permitted, one per package touched.)

- [X] T027 [US4] Run `go fix ./...` modernizers (Go 1.26) on a scratch worktree, review per-package diff per the R-004 triage rules, then commit accepted changes per package. Each accepted package gets one commit with the per-package diff. Any modernizer NOT accepted gets documented in T028.
  - **Commit**: `refactor(<pkg>): apply go fix modernizers (range-over-int / errors.Join / etc.)`
  - (Multiple commits permitted, one per package whose diff is accepted.)

- [X] T028 [US4] Create `docs/decisions/0005-deferred-modernizers.md` ADR listing every `go fix` modernizer NOT taken in T027, with a one-line rationale and target release per item. Also record the verification findings: `math/rand` v1 imports = 0, `runtime.SetFinalizer` calls = 0 (no migration needed for these per the pre-survey).
  - **Commit**: `docs(decisions): record deferred modernizers and verification findings (ADR-0005)`

**Checkpoint**: After T028, run `make lint && go fix ./... && git diff --quiet` (no diff = no further auto-modernizations remain). US4 deliverable: codebase reads as modern Go 1.26.

---

## Phase 7: User Story 5 — Reproducible build tooling (Priority: P3)

**Goal**: A new contributor can run `make lint` on a fresh clone without manual install of `staticcheck` or other tools. The `tool` directive in `go.mod` pins the version.

**Independent Test**: `tests/e2e/tool_directive_test.go` performs a fresh `go mod download && go tool staticcheck -h` cycle and asserts success.

**Maps to**: spec FR-012, SC-006; research R-006.

- [X] T029 [US5] Add `tool` directive to `go.mod` for `honnef.co/go/tools/cmd/staticcheck` — `go get -tool honnef.co/go/tools/cmd/staticcheck@v0.7.0`; verify `go mod tidy` round-trip stable and `go tool staticcheck -version` works.
  - **Commit**: `chore(deps): pin staticcheck via go.mod tool directive`

- [X] T030 [US5] Update `Makefile` `lint` target to invoke `go tool staticcheck ./...` instead of `go run $(STATICCHECK) ./...`; keep the `STATICCHECK` variable for backward compatibility (set to the same value) so external tooling that reads it doesn't break. Verify `make lint` still works on a fresh clone.
  - **Commit**: `chore(make): use go tool staticcheck for reproducible lint invocation`

- [X] T031 [US5] Update `docs/operations.md` with two short sections: (a) the new `go tool staticcheck` invocation pattern (1 paragraph); (b) container-aware GOMAXPROCS verification — how to check `proxa system info` confirms `gomaxprocs_source == "container_limit"` when running inside a CPU-limited container (1 paragraph).
  - **Commit**: `docs(operations): document go tool staticcheck and container-aware GOMAXPROCS verification`

- [X] T032 [US5] End-to-end test `tests/e2e/tool_directive_test.go` for fresh-clone lint reproducibility — `os.MkdirTemp` a scratch dir, copy the repo into it (skip `.git/`), run `go mod download && go tool staticcheck -version` from the temp dir using a fresh `GOMODCACHE`; assert exit 0 and output contains `staticcheck`. Tag `//go:build e2e`. Skip if `go` is not on PATH.
  - **Commit**: `test(e2e): cover fresh-clone go tool staticcheck reproducibility (SC-006)`

**Checkpoint**: After T032, US5 deliverable: contributors run `make lint` on a fresh clone with no `go install` step.

---

## Phase 8: Polish & Cross-Cutting

**Purpose**: Decision records, license refresh, and validation report. Lands at the very end so the validation table reflects the complete state.

- [X] T033 Create `docs/decisions/0004-router.md` ADR — Status: Accepted. Context: chi router is in use; Go 1.22 `net/http.ServeMux` now supports method+path patterns. Decision: keep chi until v1.0; document the stdlib alternative for future reconsideration. Consequences: ~300 LOC mechanical conversion deferred; one dependency stays in `go.mod` until v1.0.
  - **Commit**: `docs(decisions): record keep-chi-until-v1.0 router decision (ADR-0004)`

- [X] T034 Refresh `docs/licenses.md` with a 2026-05-20 log entry confirming no-op dependency audit for the 005-modern-go feature (zero new third-party deps added; matches the 004-logs no-op refresh pattern).
  - **Commit**: `docs(licenses): refresh transitive license audit for 005-modern-go (no-op confirmation)`

- [X] T035 Create `specs/005-modern-go/validation.md` with SC-by-SC PASS/FAIL table covering SC-001..SC-010; for each criterion, record the test that validated it (unit / e2e file + test name) or the manual verification step (quickstart section #). Include the audit findings: `math/rand` v1 = 0, `runtime.SetFinalizer` = 0, `go fix` second run = no-op (per T028). Note any deferred items per `0005-deferred-modernizers.md`.
  - **Commit**: `docs(spec): record validation results for 005-modern-go with SC-by-SC table`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 Setup**: No dependencies — starts immediately.
- **Phase 2 Foundational**: Depends on Phase 1. **BLOCKS Phase 4 (US2 needs `datadir.Root`)**. Phases 3, 5, 6, 7 don't strictly depend on Phase 2 but the **🛑 CHECKPOINT after T006** halts everything for verification.
- **Phase 3 US1**: Depends on Phase 1 complete. Independent of `datadir`.
- **Phase 4 US2**: Depends on Phase 2 complete (T011, T012 import `datadir.Root`).
- **Phase 5 US3**: Depends on Phase 1 only; can interleave with US2 if multiple contributors.
- **Phase 6 US4**: Depends on Phase 1 only.
- **Phase 7 US5**: Depends on Phase 1 only.
- **Phase 8 Polish**: Depends on all desired user stories being complete (T035 validation report needs all tests passing).

### Within-Phase Order

- Within each phase, tests follow their corresponding implementation task (TDD-flavored where impl + test are paired, but committed separately per Constitution §XI).
- T011 + T012 are in the same package family (`internal/store/sqlite` + `internal/secrets`); land sequentially to keep `go test ./...` green at every intermediate commit.

### Parallel Opportunities

- T001 + T002 can run in parallel (`internal/datadir/doc.go` vs `docs/decisions/README.md` — disjoint files).
- T017 (version unit test) can run in parallel with T018 (server handler) once T016 is done.
- T019 (system.html template) + T022 (CLI subcommand) are disjoint and can be developed in parallel.
- T029 + T030 + T031 + T032 are sequential because they all touch the build/lint surface.
- Polish tasks T033 / T034 are independent and parallelizable.

### MVP Scope

The MVP is **Phase 2 + Phase 3 (US1)** — datadir primitives committed + probe-via-ingress TLS bug fix shipped. Even at MVP, an operator with TLS-enabled services + probes is unblocked.

---

## Parallel Example: After T006 Checkpoint (with multiple contributors)

```bash
# Developer A: US1 (probe fix) — Phase 3 in sequence: T007 → T008 → T009 → T010
# Developer B: US2 (security hardening) — Phase 4: T011 → T012 → T013, T014 → T015
# Developer C: US3 (dashboard surface) — Phase 5: T016 → T017, T018, T019, T020, T021, T022, T023
# Developer D: US4 (idioms) — Phase 6: T024 → T025 → T026 → T027 → T028
# Developer E: US5 (tool directive) — Phase 7: T029 → T030 → T031 → T032
```

Solo execution: phase order P1 (US1) → P1 (US2) → P2 (US3) → P2 (US4) → P3 (US5) → Polish.

---

## Notes

- **Commit timestamps** per `feedback_commit_timestamps`: 2026-05-20 (Wed). Commits authored 8am-5pm Wed MUST backdate via `GIT_AUTHOR_DATE` + `GIT_COMMITTER_DATE` env vars to a recent weekend or evening (e.g., previous Sunday evening). Commits after 5pm Wed or on the weekend can be real-time.
- **Constitution §XI compliance**: every commit message MUST follow `<type>(<scope>): <description>` per the templates above. Do not aggregate tasks into one commit unless the spec explicitly groups them (e.g., T013 groups three crypto/rand.Text migration sites because they share an atomic security guarantee).
- **§V verification after each phase**: run `go mod tidy && git diff go.mod go.sum`; the diff MUST be empty for every phase except T029 (which legitimately edits `go.mod` to add the tool directive).
- **§VIII zero-downtime check**: after T015 (end of US2), the running server MUST accept v0.4.0-issued tokens — verify before proceeding to Phase 5.
- **Dashboard-parity verification**: T021 (footer card) + T020 (full-page view) together satisfy the `feedback_dashboard_parity` rule for this release.
- Avoid: re-running `/speckit.implement` partially without setting up the GIT_AUTHOR_DATE env var; cross-story dependencies that break independence; deleting `STATICCHECK` Makefile variable in T030 (kept for back-compat).

---
description: "Tasks for 004-logs — Runtime.StreamLogs + proxa logs CLI + dashboard log viewer"
---

# Tasks: Logs — Stream container output to CLI and dashboard

**Input**: Design documents from `/specs/004-logs/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/{streamlogs,sse-endpoint,cli-logs}.md`, `quickstart.md`. v0.3.0 merged to main.

**Tests**: Required at three layers per spec Testing Strategy:

- **Unit** (default): `parseSinceFlag` table-driven, SSE framing one-event-per-line, CLI flag validation table, mock-client log demux.
- **dockerd-tagged** (`//go:build dockerd`): real container produces 3 timed lines via `sh -c sleep loop`, `StreamLogs` returns them in order with < 2s latency.
- **e2e-tagged** (`//go:build e2e`): SC-001 (tail), SC-002 (follow), SC-006 (cross-project 403), SC-007 (dashboard SSE end-to-end).

**Organization**: Five user stories from spec. US1 (tail) + US2 (follow) form the MVP — both CLI-side. US3 is the dashboard viewer (full-page log streaming). US4 (`--since`) and US5 (`--replica`) extend the existing flags. Foundational phase carries the heavy lift: `Runtime.StreamLogs` concrete + SSE encoder.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User-story tag (US1–US5). Foundational and Polish omit it.
- Every task includes its file path(s).

## Commit policy (constitution §XI)

One commit per task. Message format: `<type>(<scope>): <description>`.

| Scope token | Applies to |
|---|---|
| `runtime/docker` | `internal/runtime/docker/*` |
| `server` | `internal/server/*` |
| `cli` | `internal/cli/*` |
| `web` | `internal/web/static/*`, `internal/web/templates/*` |
| `e2e` | `tests/e2e/*` |
| `licenses` | `docs/licenses.md` |
| `docs` | `docs/*`, `specs/*` |

## Constraint reminders

- ZERO new third-party deps. Pure stdlib + already-imported `docker/docker`.
- `CGO_ENABLED=0` for production; `=1` only at the `-race` test step.
- `context.Context` first arg on every log-streaming op; cancel propagates from HTTP request all the way to the daemon stream.
- Project-scoped auth (§III) enforced BEFORE any daemon connection opens.
- Graceful shutdown: in-flight streams drain on SIGTERM; no 503.
- CLI Ctrl-C closes TCP within 1s (no orphaned ESTABLISHED).
- Dashboard parity: logs icon in Services + Routes table rows lands in US1 (the link is a no-op-but-not-broken hint until US3 lands).

---

## Phase 2: Foundational — `Runtime.StreamLogs` + SSE encoder (Blocking)

**Purpose**: Land the Runtime concrete + the SSE/chunked writer that every user story depends on. NO operator-visible behavior yet.

> Phase 1 (Setup) is empty for this feature — zero new third-party deps, all package directories already exist. Implementation starts at Phase 2.

- [ ] T001 Create `internal/runtime/docker/logs.go` implementing `Runtime.StreamLogs(ctx, id, opts)`. Body: call `cli.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout, ShowStderr, Follow, Tail, Since, Timestamps})`, pipe the response through `stdcopy.StdCopy` into an `io.Pipe`, return the pipe reader as `io.ReadCloser`. Closing the reader cancels the demux goroutine. Commit: `feat(runtime/docker): implement StreamLogs via ContainerLogs + stdcopy demux`.

- [ ] T002 Remove the `StreamLogs` stub from `internal/runtime/docker/exec.go` (the line that returns `ErrNotImplemented`). Add a one-line doc comment to that file pointing at `logs.go`. Commit: `refactor(runtime/docker): move StreamLogs stub-removal to logs.go`.

- [ ] T003 Extend `internal/runtime/docker/mockclient_test.go` with a `ContainerLogs` method on `mockDockerClient`. Mock returns a configurable `io.ReadCloser` containing pre-built stdcopy-framed bytes so unit tests can verify demux output. Commit: `test(runtime/docker): add ContainerLogs to mockDockerClient`.

- [ ] T004 [P] Add `internal/runtime/docker/logs_test.go` with unit cases: (a) one stdout frame returns one demuxed line; (b) interleaved stdout+stderr frames return interleaved demuxed bytes in input order; (c) closing the returned reader stops the demux goroutine (verified with `goleak`-style goroutine count). Commit: `test(runtime/docker): cover StreamLogs demux and shutdown`.

- [ ] T005 Add `internal/runtime/docker/logs_integration_test.go` (build tag `dockerd`). Pull `alpine:3.19`, create + start a container running `sh -c 'for i in 1 2 3; do echo line $i; sleep 0.2; done'`, call `Runtime.StreamLogs(ctx, id, LogOpts{Follow: true})`, assert all 3 lines arrive in the expected order within 2 seconds, then exit. Commit: `test(runtime/docker): cover StreamLogs against real dockerd with timed lines`.

- [ ] T006 Create `internal/server/sse.go` with `writeSSEEvent(w, event, data string)` (writes `event: <e>\ndata: <d>\n\n` and flushes if w is `http.Flusher`), `writeSSEData(w, line string)` (writes `data: <line>\n\n`), and `writePlainLine(w, line string)` (writes `<line>\n`). All three call `Flush()` after the write. Commit: `feat(server): add tiny SSE encoder helpers (writeSSE / writePlain + Flush)`.

- [ ] T007 [P] Add `internal/server/sse_test.go` table-driven: each input line produces exactly one well-formed SSE event with a trailing blank line, lines with embedded `\n` are split into multiple events, `Flush` is called per line (verified via a mock ResponseWriter that counts flushes). Commit: `test(server): cover SSE encoder framing and flush count`.

**Checkpoint**: `Runtime.StreamLogs` works against the docker daemon; SSE writer is ready. No HTTP endpoint or CLI yet — that's Phase 3.

---

## Phase 3: User Story 1 — `proxa logs --tail` (Priority: P1) 🎯 MVP

**Goal**: An operator can run `proxa logs <service> --tail N` and see the last N log lines from replica 0.

**Independent Test**: Deploy whoami, generate 200 log lines via curl, run `proxa logs whoami --tail 50`, get ≤ 50 lines on stdout, exit 0.

### Implementation for User Story 1

- [ ] T008 [US1] Create `internal/server/handlers_logs.go` with `handleStreamServiceLogs`. Order of operations per `data-model.md` lifecycle: (1) parse query params (`tail`, `follow`, `since`, `replica`) — validate with stable error codes (`invalid-tail`, `invalid-since`, `invalid-replica`); (2) auth check: resolve subject from ctx, check project allow-list via `s.authz`, return 403 `logs-cross-project` if denied; (3) resolve replica index → container ID by listing containers with `proxa.service` label and sorting by `proxa.replica` — return 404 `service-not-found` or 503 `replica-not-found` / `replica-not-available`; (4) write headers (Content-Type per Accept, X-Proxa-Container, X-Proxa-Replica); (5) for SSE write the meta event; (6) call `Runtime.StreamLogs(r.Context(), id, opts)`, range over `bufio.Scanner` on the returned reader, call `writeSSEData` or `writePlainLine` per line; (7) on EOF write SSE end event or close conn. Commit: `feat(server): add handleStreamServiceLogs with project-scoped auth and replica resolution`.

- [ ] T009 [US1] Modify `internal/server/routes.go` to mount `GET /api/v1/projects/{project}/services/{name}/logs` → `s.handleStreamServiceLogs`. Auth middleware honors the same Bearer + Unix-socket bypass as other API routes. Commit: `feat(server): mount /api/v1/projects/{p}/services/{s}/logs`.

- [ ] T010 [US1] Create `internal/cli/logs.go` with the `proxa logs <service>` subcommand. Flags: `-f, --follow`, `-n, --tail int` (default -1), `--since duration` (default ""), `--replica int` (default 0), `--project string` (default "default"). Body: validate flags client-side (return exit 2 on invalid), build URL with query params (CLI converts `--since` duration → RFC3339 abs timestamp before sending), open the HTTP request with `Accept: text/plain`, write the meta header to stderr, copy `resp.Body` to stdout line-by-line. Honor `--follow` by simply not setting a request timeout. Register the subcommand on the root command (`cli.go` rootCmd.AddCommand). Commit: `feat(cli): add proxa logs subcommand with --tail flag plumbing`.

- [ ] T011 [P] [US1] Add `internal/cli/logs_test.go` table-driven for client-side flag validation: `--tail -99` → exit 2 + stable error message; `--since broken` → exit 2 + stable error message; `--replica -1` → exit 2 + stable error message; `--project ""` → exit 2. Commit: `test(cli): cover logs flag validation table`.

### Dashboard parity (lands in US1 per feedback memory)

- [ ] T012 [US1] [P] Modify `internal/web/templates/services_table.html` AND `internal/web/templates/routes_table.html`: add a small "📜 logs" anchor at the end of each row pointing at `/ui/logs/{{.Project}}/{{.Service}}` (services_table) or the route's service (routes_table). Style: inline-flex chip, neutral background, monospace text. The link will return 404 until US3 lands T017 — that's intentional (the icon is a visible hint that the feature is coming) and not user-confusing because the link clearly points at a real path. Commit: `feat(web): add logs-icon link to services and routes table rows`.

### Test gate for User Story 1

- [ ] T013 [US1] Add `tests/e2e/logs_tail_test.go` (build tag `e2e`) covering SC-001: deploy whoami with `replicas = 1` and `host = <picked>`, generate ~10 log lines by curling the host port, run `proxa logs whoami --tail 5` via the binary, assert exit 0 and ≤ 5 lines on stdout. ALSO assert `proxa logs nosuch` exits non-zero with `service "nosuch" not found`. Commit: `test(e2e): cover proxa logs --tail returns N lines (SC-001)`.

- [ ] T014 [US1] Add `tests/e2e/logs_crossproject_test.go` (build tag `e2e`) covering SC-006: create two projects (`a`, `b`) each with a service, request `/api/v1/projects/a/services/<svc>/logs` with a token scoped only to project `b`, assert HTTP 403 with `code: "logs-cross-project"` in the JSON body. Commit: `test(e2e): cover cross-project 403 on logs endpoint (SC-006)`.

**Checkpoint**: SC-001 + SC-006 pass. `proxa logs <svc>` works one-shot. Dashboard rows show the (currently 404) logs icon hint — that becomes a real link in US3.

---

## Phase 4: User Story 2 — `proxa logs --follow` (Priority: P1) 🎯 MVP

**Goal**: `proxa logs <svc> -f` streams new lines in real time until Ctrl-C.

**Independent Test**: Deploy whoami, in one terminal `proxa logs whoami -f`, in another `curl whoami`, observe the request log line within 1 second; Ctrl-C exits with no orphan TCP connection.

### Implementation for User Story 2

- [ ] T015 [US2] Modify `internal/cli/logs.go`: in `--follow` mode, install a `signal.NotifyContext(SIGINT, SIGTERM)` so Ctrl-C cancels the request context (which closes the HTTP connection cleanly); on exit print nothing extra and return exit 0 (or 130 on signal). The HTTP request already has no client-side timeout when `--follow`, so the server-side `Runtime.StreamLogs(Follow: true)` keeps the daemon connection open until cancel. Commit: `feat(cli): handle SIGINT cleanly under --follow (exit 130, no orphan conn)`.

- [ ] T016 [US2] Add `tests/e2e/logs_follow_test.go` (build tag `e2e`) covering SC-002 + SC-003: deploy whoami, exec `proxa logs whoami -f` as a subprocess capturing stdout, in a goroutine `curl` the whoami port and timestamp the moment of the request, scan the subprocess stdout for the new request line, assert the line appears within 1 second of the curl (SC-002). Then send the subprocess SIGINT, wait for exit, assert exit code 0 or 130 within 1 second (SC-003). Commit: `test(e2e): cover proxa logs --follow latency and clean Ctrl-C exit (SC-002, SC-003)`.

**Checkpoint**: SC-002 + SC-003 pass. CLI follow mode works end-to-end.

---

## Phase 5: User Story 3 — Dashboard log viewer (Priority: P2)

**Goal**: Click the "📜 logs" icon in a Services row → focused full-page log viewer at `/ui/logs/{project}/{service}` with replica dropdown, follow toggle, and pause-on-scroll auto-scroll.

**Independent Test**: Deploy a service with ≥ 1 replica, navigate to `/ui/logs/default/<svc>`, see logs streaming live for replica 0 within 2s, pick a different replica → new stream replaces old within 2s, scroll up → auto-scroll pauses → scroll down → auto-scroll resumes.

### Implementation for User Story 3

- [ ] T017 [US3] Create `internal/web/templates/logs.html` — full-page log viewer. Header: service name + project chip + replica dropdown (`<select x-model="replica" @change="reconnect()">`) + follow toggle (`<input type="checkbox" x-model="follow" @change="reconnect()">`) + back link to `/ui/`. Body: `<pre x-ref="output">` for log lines. Embedded Alpine controller (`x-data="logsController()"`): opens `EventSource('/api/v1/projects/<p>/services/<s>/logs?follow=true&replica=' + replica)`, appends each `event.data` to `output.textContent`, auto-scrolls to bottom unless `output.scrollTop` is more than 100px from the bottom (tracked via scroll event), closes the EventSource on `beforeunload` and on dropdown change before reopening. Commit: `feat(web): add full-page log viewer template with Alpine + EventSource controller`.

- [ ] T018 [US3] Create `internal/server/ui_logs.go` with `handleUILogs` that renders the `logs.html` template. URL pattern: `/ui/logs/{project}/{service}`. Pulls path params, validates the service exists (404 if not), passes `uiLogsData{Project, Service, Replicas []int}` to the template. The `Replicas` slice is computed from the current backend count (so the dropdown shows the right options). Commit: `feat(server): add /ui/logs/{project}/{service} full-page handler`.

- [ ] T019 [US3] Modify `internal/server/ui.go` `MountUI`: register `s.Router.With(mw).Get("/ui/logs/{project}/{service}", s.handleUILogs)` next to the existing `/ui/services` and `/ui/routes` routes. Commit: `feat(server): mount /ui/logs route under the existing UI middleware`.

### Test gate for User Story 3

- [ ] T020 [US3] Add `tests/e2e/logs_dashboard_test.go` (build tag `e2e`) covering SC-007: deploy whoami with one replica and a host-port route, generate a few log lines, `curl` the `/ui/logs/default/whoami` URL and assert the response has Content-Type `text/html` and the body contains the service name + the replica dropdown markup. Then `curl` the SSE endpoint (`Accept: text/event-stream`) and read for 3 seconds: assert Content-Type `text/event-stream`, that at least one `event: meta` line appears, AND that at least one `data: ` log line appears (triggered by a concurrent curl to the whoami port during the read). Commit: `test(e2e): cover dashboard log viewer page + SSE stream (SC-007)`.

**Checkpoint**: SC-007 passes. Dashboard log viewer is the operator's debug surface.

---

## Phase 6: User Story 4 — `--since` filter (Priority: P2)

**Goal**: `proxa logs <svc> --since 5m` returns only the last 5 minutes of lines.

### Implementation for User Story 4

- [ ] T021 [US4] Add `parseSinceFlag(raw string) (time.Time, error)` helper to `internal/cli/logs.go`. Parses Go duration strings (`5m`, `1h30m`, `48h`), returns `time.Now().Add(-d)`. Rejects negative durations + non-parseable inputs with explicit error message including the original input. Commit: `feat(cli): add parseSinceFlag duration helper for proxa logs --since`.

- [ ] T022 [US4] [P] Add `internal/cli/logs_since_test.go` table: valid (`5m`, `1h30m`, `1h`, `60s` → expected delta), invalid (`broken`, `5`, `-1m`, empty → error code 2 with message containing the input). Commit: `test(cli): cover parseSinceFlag duration parsing`.

- [ ] T023 [US4] Wire `--since` in `proxa logs` CLI → query param `?since=<RFC3339>`; server-side query parser in `handleStreamServiceLogs` (T008 already validates `invalid-since`) MUST construct `time.Time` from RFC3339 and pass into `runtime.LogOpts.Since`. Commit: `feat(server,cli): plumb --since duration into Runtime.LogOpts.Since`.

- [ ] T024 [US4] Add `tests/e2e/logs_since_test.go` (build tag `e2e`) covering SC-004-adjacent: deploy whoami, generate one log line, sleep 5s, generate another, `proxa logs <svc> --since 3s` → assert only the second line appears. Commit: `test(e2e): cover --since filter window (US4)`.

**Checkpoint**: US4 done. Time-window debug is straightforward.

---

## Phase 7: User Story 5 — `--replica` pick (Priority: P3)

**Goal**: `proxa logs <svc> --replica 2` streams logs from replica 2 only.

### Implementation for User Story 5

- [ ] T025 [US5] Verify (and extend if needed) `internal/cli/logs.go` to pass `?replica=N` query param when `--replica N` is non-zero (default 0). Server-side replica resolution already covers this from T008. Add one validation case in `logs_test.go`: `--replica -1` → exit 2 + "must be >= 0". Commit: `feat(cli): wire --replica flag into proxa logs query`.

- [ ] T026 [US5] Add `tests/e2e/logs_replica_test.go` (build tag `e2e`) covering SC-005: deploy a service with `replicas = 3` and host ports per replica (use `host = 18091 + i`-style), `proxa logs <svc> --replica 1 --tail 5` → assert stderr meta header mentions `proxa-default-<svc>-1 (replica 1)`. Also assert `--replica 99` exits non-zero with `replica 99 not found (service has 3 replicas)`. Skip on macOS Docker Desktop (multi-replica + bridge IPs — same limitation as 002 SC-002-4); covered on Linux CI. Commit: `test(e2e): cover --replica pick and out-of-range error (SC-005)`.

**Checkpoint**: US5 done. Multi-replica debugging works (Linux CI).

---

## Phase 8: Polish & Cross-Cutting

- [ ] T027 Re-run the license audit from 003's polish phase against the post-004 `go.sum`. Expected: NO change (this feature added zero direct deps, zero new transitives). Update `docs/licenses.md` Refresh log with a one-line entry noting "feature 004 logs: no go.sum churn — stdlib + already-imported docker/docker only". Commit: `docs(licenses): refresh transitive license audit for 004-logs (no-op confirmation)`.

- [ ] T028 Walk `quickstart.md` end-to-end on a clean `${PROXA_DATA_DIR}` against a local proxa server. Record outcomes in `specs/004-logs/validation.md` mirroring the 001 / 002 / 003 format: SC-by-SC table with PASS/READY/FAIL + evidence. Document any bugs caught + fixed during validation. Commit: `docs(spec): record quickstart validation results for 004-logs`.

**Checkpoint (end of feature)**: All 8 SCs PASS or READY. `git log --oneline 004-logs ^main` shows one commit per task with constitution-§XI-compliant messages.

---

## Dependencies & Execution Order

### Phase ordering

- **Phase 2 (Foundational)**: T001 → T002 sequential (same file area). T003 in parallel with T001 (different file). T004 [P] depends on T001+T003. T005 standalone after T001. T006 → T007 [P].
- **Phase 3 (US1)**: T008 (handler) depends on T001 + T006. T009 (route mount) depends on T008. T010 (CLI) parallel with T008 (no shared file). T011 [P] depends on T010. T012 (dashboard hint) parallel with T008. T013 (e2e tail) depends on T009 + T010 + T012. T014 (e2e cross-project) depends on T009.
- **Phase 4 (US2)**: T015 depends on T010. T016 depends on T015.
- **Phase 5 (US3)**: T017 standalone (template). T018 depends on T017. T019 depends on T018. T020 depends on T009 + T019.
- **Phase 6 (US4)**: T021 → T022 [P]. T023 depends on T021. T024 depends on T023.
- **Phase 7 (US5)**: T025 depends on T010. T026 depends on T025.
- **Phase 8 (Polish)**: T027 + T028 after all SCs verified.

### User-story dependencies

- US1 + US2 are both P1 (MVP). US2 builds on US1's handler + CLI (just adds signal handling and a follow-mode loop). Independent test of US2 requires US1's binary, but US1 is independent of US2.
- US3 (dashboard) depends on US1's handler being live; can be done in parallel with US2.
- US4 + US5 are both flag extensions on top of US1's CLI; can be done in any order.

### Parallel execution batches

**Batch A (Phase 2)**: T001 + T003 in parallel. T002 sequential after T001. T004 [P] after T001+T003. T005 sequential. T006 → T007 [P].

**Batch B (Phase 3 — US1)**: T008 + T010 + T012 in parallel after Phase 2 (different files). T009 + T011 [P] + T013 + T014 sequence into integration.

**Batch C (Phase 5 — US3)**: T017 → T018 → T019 → T020 sequential (each depends on prior).

**Batch D (e2e tests)**: T013, T014, T016, T020, T024, T026 — separate files in `tests/e2e/`. After respective implementation tasks land.

---

## Implementation Strategy

**MVP scope**: Phase 2 + Phase 3 (US1) + Phase 4 (US2) = the CLI debugging loop. ~14 tasks. After this, an operator can `proxa logs <svc> --tail N` and `proxa logs <svc> -f` from any terminal — Docker CLI becomes optional.

**Incremental delivery beyond MVP**: US3 (dashboard) is the next high-value bundle (4 tasks). US4 + US5 are small extensions.

**Stop-and-validate points**:

- After Phase 2: `Runtime.StreamLogs` works against real dockerd (T005 PASS); SSE writer unit-tested. Internal-only, no operator visible yet.
- After Phase 3 (US1): SC-001 + SC-006 PASS. `proxa logs <svc> --tail N` is real. Pause for user verification before US2-5.
- After Phases 4-5: SC-002 + SC-003 + SC-007 PASS. Full debug loop end-to-end including dashboard.
- After Phase 8: cut v0.4.0.

---

## Notes & follow-ups

- `--timestamps` flag (Docker's `Timestamps=true`) deliberately left out of v0.4 per spec's Out of Scope — small polish PR after if asked.
- ANSI color stripping in the dashboard: v0.4 strips (browsers don't decode); a future polish renders via `\x1b[Nm` → `<span>`.
- 10-concurrent-streams isn't an explicit SC but the architecture supports it; document a known limit in operations.md if a real user hits a wall.
- Multi-replica merged tail is explicitly out of scope — v0.5+ if a real workload asks.

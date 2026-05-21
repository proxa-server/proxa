# Implementation Plan: Logs — Stream container output

**Branch**: `004-logs` | **Date**: 2026-05-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/004-logs/spec.md`

## Summary

Wire `Runtime.StreamLogs` (the last unimplemented Runtime method) against `docker/docker/client.ContainerLogs`. Add a `proxa logs <service>` CLI subcommand with `--follow / --tail / --since / --replica` flags. Expose `/api/v1/projects/{project}/services/{name}/logs` that delivers logs as Server-Sent Events (for the dashboard) or chunked transfer (for the CLI), based on the `Accept` header. Land a focused full-page dashboard log viewer at `/ui/logs/{project}/{service}` with a replica dropdown, follow toggle, and pause-on-scroll auto-scroll. Per the dashboard-parity rule from memory: every Services + Routes row gains a small "logs" icon linking to the viewer; dashboard work is in-scope for this feature, not deferred.

## Technical Context

**Language/Version**: Go 1.26.x. `CGO_ENABLED=0` for the production binary.

**Primary Dependencies**:

- Standard library `net/http`, `bufio`, `time`, `context`.
- Already-imported `github.com/docker/docker/client` (`ContainerLogs` API).
- Already-imported `github.com/docker/docker/pkg/stdcopy` (stdout/stderr demux — same as Exec from 002).
- **No new third-party deps.** SSE encoding is `"data: <line>\n\n"` — pure stdlib.
- Already-vendored frontend assets: `htmx.min.js` + `alpine.min.js` under `internal/web/static/`. Native browser `EventSource` API drives the dashboard log viewer (no new JS dep — see research R-001).

**Storage**: None new. Logs live in Docker's existing log driver (json-file by default). Proxa is a pass-through reader.

**Testing**:

- Unit: `parseSinceFlag`, SSE framing helper, CLI flag validation table.
- `//go:build dockerd`: real container produces 3 timed lines, `StreamLogs` returns them in order under 2s.
- `//go:build e2e`: SC-001 (tail), SC-002 (follow latency), SC-006 (cross-project 403), SC-007 (dashboard SSE end-to-end).
- `-race` on every package that touches the streaming goroutine pair (reader + writer).

**Target Platform**: Linux server (production), macOS Docker Desktop (developer). Logs work identically on both — Docker daemon mediates.

**Project Type**: Single Go binary embedding API + reconciler + ingress + dashboard. Logs is a thin layer on top.

**Performance Goals**:

- p95 first-byte latency for `proxa logs --tail 100`: under 500 ms (Docker daemon retrieval + 100 lines copy).
- `--follow` per-line latency from container write to client receive: under 1 second (SC-002).
- 10 concurrent log streams against the same service: no goroutine leaks, no daemon-side connection exhaustion.

**Constraints**:

- Project-scoped auth (§III) enforced BEFORE any daemon connection opens — failed auth wastes zero daemon resources.
- Graceful shutdown: in-flight streams drain on `proxa server` SIGTERM; clients see clean EOF, not a 503 or connection reset.
- CLI Ctrl-C exits within 1 second AND closes the HTTP connection (no orphaned `ESTABLISHED` in `lsof`).

**Scale/Scope**: Single-node v0.4. ~10 concurrent streams per service is the tested upper bound. Multi-node log aggregation is a v1.0 cluster concern.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | How this feature complies |
|---|---|
| §I Architecture First | `Runtime.StreamLogs` interface already declared from 000; this feature provides the concrete `docker` implementation. SSE writer is a small package-private helper in `internal/server/` — not a new interface (zero callers besides the handler). |
| §II Security by Default | Cross-project access returns 403 BEFORE opening any daemon connection (`logs-cross-project` stable error code). Logs are not secret material per FR-013 — they live in the daemon's existing storage which already inherits the daemon's permission model. |
| §III Project Scoping | The SSE handler validates `{project}` against the authenticated subject's allowed projects on the very first line of code; daemon resources only open after auth passes. |
| §IV Go Idioms | `context.Context` first arg threaded through StreamLogs + handler + CLI. SSE writer flushes after each line; reader scanner runs in its own goroutine canceled by ctx. `slog` JSON for the structured events (request received, stream opened, stream closed, error). `-race` clean. No CGO. |
| §V Single Binary | No new Go deps. No new vendored JS — native `EventSource` is in every browser since 2009. HTMX + Alpine already bundled cover the rest. |
| §VI Cluster-Ready | Multi-node would need a node-local log source (the agent's local Runtime) and either a fan-out at the API layer or a direct redirect to the node. Today's single-node code routes all reads through the same Runtime; the v1.0 fan-out plugs in behind the same handler signature. |
| §VIII Zero-Downtime by Default | Graceful shutdown drains in-flight streams up to a deadline; new connections during shutdown get a clean refused-not-503. Hot route reload from 003 doesn't affect log streams (different HTTP server). |
| §IX Permissive License | Zero new deps. License audit refresh in Polish phase is a no-op (no go.sum change expected). |
| §XI Commit Strategy | Scopes: `runtime/docker`, `server`, `cli`, `web`, `e2e`, `docs`. One commit per task. |

**No deviations require a Complexity Tracking entry.** The only judgment call is "SSE vs WebSockets vs polling" — resolved in research.md R-002 in favor of SSE (one-way server→client matches the use case; native browser support; no upgrade dance).

## Project Structure

### Documentation (this feature)

```text
specs/004-logs/
├── plan.md              # This file
├── research.md          # Phase 0 — transport choice, JS approach, demux pattern, ctx-cancel pattern
├── data-model.md        # Phase 1 — LogStream, LogLine, request/response shapes (mostly transient)
├── quickstart.md        # Phase 1 — operator walkthrough (proxa logs + dashboard click-through)
├── contracts/
│   ├── streamlogs.md    # The Runtime.StreamLogs contract (re-state from 000 + concrete semantics)
│   ├── sse-endpoint.md  # GET /api/v1/.../logs query params + response shapes
│   └── cli-logs.md      # proxa logs subcommand grammar + exit codes
└── tasks.md             # Phase 2 — produced by /speckit.tasks (NOT this command)
```

### Source Code (repository root)

New files:

```text
internal/
├── runtime/docker/
│   └── logs.go                 # NEW — Runtime.StreamLogs concrete (ContainerLogs + stdcopy demux)
├── server/
│   ├── handlers_logs.go        # NEW — handleStreamServiceLogs (SSE OR chunked based on Accept)
│   ├── ui_logs.go              # NEW — /ui/logs/{project}/{service} full-page handler
│   └── sse.go                  # NEW — tiny SSE encoder ("data: ...\n\n" + Flush)
└── cli/
    └── logs.go                 # NEW — proxa logs subcommand + flag plumbing

internal/web/
└── templates/
    └── logs.html               # NEW — full-page log viewer with Alpine.js EventSource controller

tests/e2e/
├── logs_tail_test.go           # NEW — SC-001
├── logs_follow_test.go         # NEW — SC-002
├── logs_crossproject_test.go   # NEW — SC-006
└── logs_dashboard_test.go      # NEW — SC-007 (asserts page + SSE content-type + data: lines flow)

Modified files:

internal/
├── runtime/docker/
│   ├── exec.go                 # Remove StreamLogs ErrNotImplemented stub (moved to logs.go)
│   └── mockclient_test.go      # Add ContainerLogs to the dockerClient mock
├── server/
│   ├── routes.go               # Mount GET /api/v1/projects/{project}/services/{name}/logs
│   ├── ui.go                   # Mount /ui/logs/{project}/{service}
│   └── ui_routes.go            # (no change — routes table reuses the new logs-icon link)
└── web/
    └── templates/
        ├── index.html          # (no change — logs link lives in the per-row templates)
        ├── services_table.html # Add logs icon button in each row
        └── routes_table.html   # Add logs icon button in each row (link by service)
```

**Structure Decision**: Stay flat — no new top-level package. `internal/server/handlers_logs.go` is a thin handler that orchestrates `Runtime.StreamLogs` and the SSE writer; `internal/server/sse.go` is a 30-line helper. The CLI subcommand lives next to existing CLI commands. The dashboard page is one template + an inline Alpine.js controller (no separate JS file — matches the existing pattern in `index.html`).

## Complexity Tracking

> Empty — Constitution Check passes without deviations.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _(none)_  | —          | —                                   |

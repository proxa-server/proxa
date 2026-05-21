---
description: "Log streaming — proxa logs CLI + dashboard log viewer + SSE API"
---

# Feature Specification: Logs — Stream container output to CLI and dashboard

**Feature Branch**: `004-logs`

**Created**: 2026-05-17

**Status**: Draft

**Input**: Wire `Runtime.StreamLogs` (the last unimplemented Runtime method) and surface container logs to operators via a `proxa logs <service>` CLI, a `/api/v1/.../logs` streaming HTTP endpoint, and a dedicated dashboard log viewer page. Operators stop having to shell into Docker directly to debug their workloads.

## Overview

Today an operator can deploy a service, watch its status, see its routes — but the moment something prints an error and the service goes `degraded`, the only debug path is `docker logs proxa-<project>-<service>-<replica>`. That violates the "single binary, opinionated defaults" promise and forces the operator to know container names + remember Docker's CLI flags.

This feature closes that loop: `proxa logs <service>` streams tail-style; the dashboard's per-service "logs" button opens a focused page that streams the same bytes via Server-Sent Events; a JSON API powers both. No new dependencies — `Runtime.StreamLogs` already declared in the interface from 000, just unimplemented; HTMX has SSE support already bundled in the dashboard.

After this lands, the operator's day-2 debug loop runs entirely through Proxa: deploy → check dashboard → click logs → grep for the stack trace → re-deploy. Docker CLI becomes optional.

## User Scenarios *(mandatory)*

### User Story 1 — Tail the last N lines of a service (Priority: P1, MVP)

The operator runs `proxa logs web --tail 100` and sees the last 100 log lines from replica 0 of the `web` service. Command exits cleanly.

**Why this priority**: This is the single most common debug action — "what did it print before it crashed?" — and the most asked-for feature in any container runtime tooling.

**Independent Test**: Deploy whoami (`replicas = 1`), trigger it a few times to produce log lines, run `proxa logs whoami --tail 50`. Get up to 50 lines of whoami request logs on stdout.

**Acceptance Scenarios**:

1. **Given** a service `web` with one running replica that has produced 200 log lines, **When** the operator runs `proxa logs web --tail 50`, **Then** stdout shows the most recent 50 lines (no more, no less) and the command exits 0.
2. **Given** the same service, **When** the operator runs `proxa logs web` with no flags, **Then** stdout shows all available lines (Docker's default behavior) and the command exits 0.
3. **Given** a service that does not exist, **When** the operator runs `proxa logs nosuch`, **Then** the command exits non-zero with `service "nosuch" not found in project "default"`.

---

### User Story 2 — Follow logs in real-time (Priority: P1, MVP)

The operator runs `proxa logs web --follow` and the command keeps streaming new log lines as they arrive, until Ctrl-C.

**Why this priority**: Same use case as `tail -f` for any developer — watch the workload while triggering it from a second terminal. Required for any meaningful debugging session.

**Independent Test**: Deploy whoami, run `proxa logs whoami -f` in one terminal, `curl` the service in another, observe each request's log line appear within 1 second.

**Acceptance Scenarios**:

1. **Given** a service producing intermittent log output, **When** the operator runs `proxa logs <svc> --follow` and a new log line is written by the container, **Then** the line appears on stdout within 1 second.
2. **Given** the same follow session, **When** the operator presses Ctrl-C, **Then** the command exits cleanly (exit 0 or 130 — standard SIGINT exit) without leaving the underlying API connection orphaned.
3. **Given** the underlying container exits while `--follow` is active, **When** the API connection closes, **Then** the CLI prints a clear "container exited" line and exits 0.

---

### User Story 3 — View logs in the dashboard (Priority: P2)

The operator clicks the "logs" icon next to a service in the Services card. A new full-page log viewer opens that streams logs from replica 0 in real time, with a dropdown to switch replicas and a "follow" toggle.

**Why this priority**: Closes the dashboard debug loop without forcing terminal context-switching. The CLI covers the "I'm in a shell anyway" path; the dashboard covers "I'm already in the browser checking status".

**Independent Test**: Deploy a service with multiple replicas via the dashboard's existing flow, click the "logs" icon, see logs streaming live for replica 0; switch the dropdown to replica 1 and see its log stream replace replica 0's.

**Acceptance Scenarios**:

1. **Given** a service with 2+ replicas, **When** the operator clicks the "logs" icon in the Services card row, **Then** the browser navigates to `/ui/logs/<project>/<service>` and the latest lines from replica 0 appear within 2 seconds.
2. **Given** the log viewer is open with follow enabled, **When** the container produces a new log line, **Then** the line appears in the viewer within 1 second and the viewer auto-scrolls to the bottom.
3. **Given** the viewer is open, **When** the operator picks a different replica from the dropdown, **Then** the existing stream closes and a fresh stream from the chosen replica opens within 2 seconds.

---

### User Story 4 — Filter by time window (Priority: P2)

The operator runs `proxa logs web --since 5m` and sees only the log lines produced in the last 5 minutes.

**Why this priority**: Production debugging often starts with "since when". Without `--since`, the operator dumps the entire log buffer.

**Independent Test**: Deploy a service, let it produce log lines over time, run `proxa logs <svc> --since 1m` and observe only the most recent minute's lines.

**Acceptance Scenarios**:

1. **Given** a service whose log buffer spans 1 hour, **When** the operator runs `proxa logs <svc> --since 10m`, **Then** the command prints only lines whose timestamps are within the last 10 minutes and exits 0.
2. **Given** the `--since` value cannot be parsed as a duration (e.g., `--since notanumber`), **When** the command runs, **Then** it exits non-zero with a clear "invalid --since value" message.

---

### User Story 5 — Pick a specific replica (Priority: P3)

The operator runs `proxa logs web --replica 2` to see only the logs from replica 2 of the `web` service.

**Why this priority**: Multi-replica debugging is uncommon for hobby workloads; matters more once a service scales out. Useful but not blocking.

**Independent Test**: Deploy a service with replicas=3, run `proxa logs <svc> --replica 1` and confirm the output mentions replica 1's container name in the connection log line.

**Acceptance Scenarios**:

1. **Given** a service with 3 replicas, **When** the operator runs `proxa logs <svc> --replica 2`, **Then** only replica 2's log lines appear (the test inspects the container name printed at the top of the stream).
2. **Given** the operator requests `--replica 99` on a 3-replica service, **When** the command runs, **Then** it exits non-zero with `replica 99 not found (service has 3 replicas)`.

---

### Edge Cases

- **Service has zero running replicas** (just scaled to 0, or all crashed): the API returns 503 with a `service has no running replicas` body; the CLI prints the error and exits non-zero.
- **Log buffer is empty** (just-started container, no output yet): the API returns an empty stream; the CLI exits 0 with no output (`--follow` continues waiting).
- **Cross-project access denied**: a bearer token scoped to project `socio-do` requesting logs for project `kut-do` gets 403 with `unauthorized for project "kut-do"` and the CLI prints the error.
- **Container removed mid-stream**: the open SSE/stream connection closes from the daemon side; the dashboard shows a "stream ended" line in italics; the CLI prints "container exited" and exits 0.
- **Very long line** (e.g., a 64 KiB JSON dump on one line): the dashboard wraps; the CLI lets the terminal handle wrapping (no truncation).
- **ANSI color codes** in the log output: the CLI passes them through (terminals decode); the dashboard strips them (browser doesn't decode ANSI), OR renders them via a minimal ANSI→span conversion. Deferred decision: see Assumptions.
- **Operator left the dashboard tab open overnight**: SSE connections stay alive but consume one daemon connection per viewer. Acceptable for v0.4; rate-limiting / max-viewers comes later if a real abuse surfaces.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST stream container log output from a chosen replica of a service to a CLI consumer (`proxa logs`) and an HTTP consumer (`/api/v1/projects/{project}/services/{name}/logs`) on demand.
- **FR-002**: The CLI subcommand `proxa logs <service>` MUST accept and honor `--tail N` (return the last N lines, default = all available), `--follow` (stream new lines until Ctrl-C), `--since DURATION` (only lines from the last DURATION; same format as `time.ParseDuration`), and `--replica I` (pick replica index, default 0).
- **FR-003**: The HTTP endpoint MUST accept query parameters mirroring the CLI flags: `tail`, `follow`, `since`, `replica` — with identical defaults and identical validation error messages.
- **FR-004**: When `follow=true`, the HTTP response MUST be a streaming response that delivers new log lines to the client with at most 1 second of buffering between the daemon producing the line and the client receiving it.
- **FR-005**: When `follow=false`, the HTTP response MUST be a one-shot response that closes after the last line is sent — no half-open connection waiting for the client.
- **FR-006**: The CLI MUST exit cleanly on Ctrl-C in `--follow` mode, with no leaked HTTP connection to the proxa server (verifiable by `netstat` after the command exits).
- **FR-007**: The dashboard MUST provide a "logs" icon button in every Services-card row that links to `/ui/logs/{project}/{service}`. The Routes card SHOULD also link to the same page for the service backing the route.
- **FR-008**: The dashboard log-viewer page MUST stream logs from replica 0 by default and provide a dropdown to switch to any other replica of the same service; switching MUST close the prior stream within 1 second.
- **FR-009**: The dashboard log-viewer page MUST auto-scroll to the bottom when new lines arrive AND the user has not scrolled up; if the user has scrolled up, auto-scroll MUST pause until they scroll to the bottom again.
- **FR-010**: All log-streaming endpoints MUST be authenticated via the existing Bearer-token / Unix-socket bypass scheme; cross-project access MUST return 403.
- **FR-011**: The HTTP endpoint MUST include the source replica's container ID and replica index in a non-streaming header / preamble so the consumer can verify which replica it's reading.
- **FR-012**: The CLI MUST print a clear single-line header (e.g., `=== proxa-default-web-0 (replica 0) ===`) before the log lines begin, so the operator knows which container's output they're seeing.
- **FR-013**: The system MUST NOT persist log lines anywhere new — logs remain in Docker's existing json-file driver (or whatever the daemon is configured for); this feature is read-only on the daemon's existing log storage.

### Key Entities *(include if feature involves data)*

- **LogStream**: An ephemeral, per-request bidirectional connection from the proxa server to the Docker daemon for one container's logs. Lives for the duration of the HTTP request / CLI session. Not persisted.
- **LogLine**: One newline-terminated line of container output, with optional timestamp (Docker's `--timestamps` flag passes through). The system does NOT parse, structure, or filter lines — passes them through as bytes.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can run `proxa logs <service>` against a deployed service and see the last log lines on stdout within 2 seconds of command start.
- **SC-002**: With `--follow` active, a log line written by the container reaches the operator's terminal within 1 second of being written (measured by timestamping a log line at write and at print time).
- **SC-003**: Pressing Ctrl-C during `--follow` exits within 1 second and leaks no open TCP connections (verified by `lsof -p <pid>` returning before exit + post-exit no `ESTABLISHED` entry for the proxa port).
- **SC-004**: A dashboard user clicking the "logs" icon sees the first log line in the viewer within 2 seconds of clicking.
- **SC-005**: Switching the replica dropdown in the dashboard log viewer produces logs from the new replica within 2 seconds, with no lines from the prior replica leaking into the new view.
- **SC-006**: A bearer-token request to `/api/v1/projects/X/services/Y/logs` from a subject scoped to a different project returns 403 with a clear error body.
- **SC-007**: The dashboard log viewer auto-scrolls to the bottom when new lines arrive (verified by browser scrollTop reaching scrollHeight - clientHeight after a new line) AND pauses auto-scroll when the user manually scrolls up (verified by scrollTop staying put when a new line arrives 1 second after a user scroll).
- **SC-008**: Running `proxa logs` on the same service from 10 concurrent CLI sessions produces consistent output to all 10 without one session's stream affecting another's (no cross-talk, no half-tails).

## Assumptions

- **Docker default log driver is json-file** (the engine default). Operators using `--log-driver=none` or other drivers will see empty streams; this feature does not validate the daemon's log driver configuration.
- **Log timestamps come from Docker, not Proxa.** The daemon's `--timestamps` flag is passed through; if the operator wants RFC3339 timestamps prefixed to each line they set `proxa logs --timestamps` (deferred: see Out of Scope).
- **Streaming via Server-Sent Events** is the default dashboard transport; chunked transfer encoding is the default for `--follow`. The CLI and the dashboard share the same endpoint but switch transport based on the `Accept` header (`text/event-stream` → SSE; otherwise → chunked).
- **ANSI color handling for the dashboard:** v0.4 strips ANSI codes in the rendered HTML (browsers don't decode them). A future polish can render colors via a minimal `\x1b[Nm` → `<span style="color:...">` converter — out of scope here.
- **One replica per stream** in v0.4; merging logs from multiple replicas into a single timestamp-sorted view is a v0.5+ idea if a real workload asks for it.
- **No log persistence layer in Proxa** — Docker's existing log driver is authoritative. Proxa is a thin pass-through.
- **Bearer-token auth scheme** from earlier features applies unchanged. Unix-socket bypass also unchanged.

## Dependencies

- **003-ingress merged at v0.3.0** — the dashboard's navigation pattern (HTMX poll + chip routing) is the foundation for the log-viewer page.
- **`Runtime.StreamLogs`** from 000-foundation — currently `ErrNotImplemented`; this feature provides the real implementation against the docker/docker client's `ContainerLogs` API.
- **`runtime.LogOpts`** already declared in `internal/runtime/runtime.go` — carries `Follow`, `Tail`, `Timestamps`, `Since` fields.
- **HTMX's SSE extension** (`htmx-ext-sse`) is already bundled in `internal/web/static/`. If not, vendor it during implementation — it's MIT-licensed and `< 5 KiB`.

## Out of Scope

- **Structured log parsing / search / filter by log level** — operators use grep on the CLI output. Web-side search is a v0.5+ observability concern.
- **Persistent log storage** — Docker's existing log driver is authoritative. Proxa does not duplicate.
- **Audit log for Proxa operations** (who deployed / killed what) — separate future feature (probably v0.5).
- **Log shipping** to external systems (Loki, Datadog, S3) — a future Feature might add log-driver configuration on the TaskDef; not here.
- **Multi-replica merged tail** — pick one replica at a time. The future merging feature must sort by timestamp and is non-trivial; not v0.4.
- **`--timestamps` flag** — Docker supports it via `ContainerLogs(opts.Timestamps=true)`; deferred to a small polish PR after v0.4 ships, IF an operator asks.
- **ANSI color rendering in the dashboard** — strip in v0.4; render in v0.5+ if requested.
- **Container exec / shell-in** (`proxa exec`) — `Runtime.Exec` is already wired since 002; the `proxa exec` CLI wrapper is a different feature.
- **Log download** (one-shot bulk export) — `--follow=false` + redirect to file (`proxa logs svc > out.txt`) already covers the use case; no dedicated download endpoint.

## Testing Strategy

- **Unit tests**: `runtime.LogOpts` round-trip in the docker client mock; the `parseSinceFlag` helper; the SSE encoder framing (one event per line); the CLI's flag validation table (covering `--since invalid`, `--replica negative`, `--tail negative`).
- **`//go:build dockerd` integration test**: spin up a real `alpine` container with `sh -c 'for i in 1 2 3; do echo line $i; sleep 0.2; done'`, call `Runtime.StreamLogs`, assert 3 lines arrive in the expected order within 2 seconds.
- **`//go:build e2e` tests**:
  - SC-001: `proxa logs <svc> --tail N` returns ≤ N lines and exits 0.
  - SC-002: `proxa logs <svc> --follow` shows a new line within 1s of the container writing it.
  - SC-006: cross-project token gets 403.
  - SC-007: dashboard log viewer auto-scrolls when new lines arrive (Playwright-style assertion; defer to "manual verification" if Playwright isn't a dep we want).
- **Race coverage**: the SSE handler maintains a per-request scanner goroutine + a write goroutine; `go test -race` must be clean.

## References

- Technical Spec Section 9: Observability (logs are the v0.4 slice; metrics + audit log come later).
- Constitution §IV Go idioms — stdlib + already-imported docker/docker; no new deps.
- Constitution §V single binary — HTMX SSE extension already bundled; dashboard viewer page uses it.
- Feature 002 spec, §"Dependencies" — "Runtime.StreamLogs from 000 (still stubbed; this feature does NOT wire it — that's Feature 004 dashboard log viewer)." This is that feature.
- Feature 003 spec, §"Out of Scope" — "Dashboard log viewer (Feature 004)." This is that feature.

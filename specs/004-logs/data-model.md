# Phase 1 — Data Model: Logs

No SQLite schema change. No persistent log storage. This feature is a pass-through reader over Docker's existing log driver — the data model is request/response shapes and transient in-process state.

---

## In-memory state (`internal/server/handlers_logs.go`)

```go
// logStreamRequest is the parsed inbound request — the result of
// validating query params + path params + Accept header.
type logStreamRequest struct {
    Project   string
    Service   string
    Replica   int        // 0..N-1; default 0
    Follow    bool       // default false
    Tail      int        // -1 = all available; default -1
    Since     time.Time  // zero = no since filter
    Wire      wireFormat // SSE or Plain (from Accept header)
}

type wireFormat int

const (
    wirePlain wireFormat = iota // chunked transfer, raw lines (CLI default)
    wireSSE                     // text/event-stream (dashboard)
)
```

---

## On-the-wire response shapes

### Plain chunked (CLI / curl)

```text
HTTP/1.1 200 OK
Content-Type: text/plain; charset=utf-8
X-Proxa-Container: proxa-default-web-0
X-Proxa-Replica: 0
Transfer-Encoding: chunked

2026-05-17T11:23:45Z hello from whoami
2026-05-17T11:23:46Z another line
...
```

### SSE (dashboard)

```text
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
X-Proxa-Container: proxa-default-web-0
X-Proxa-Replica: 0

event: meta
data: {"container":"proxa-default-web-0","replica":0}

data: 2026-05-17T11:23:45Z hello from whoami

data: 2026-05-17T11:23:46Z another line

event: end
data: {"reason":"container exited"}
```

The `event: meta` preamble lets the dashboard JS confirm which replica it's reading before any log lines flow (useful when the dropdown changes mid-stream — the JS can ignore residual events from the prior stream).

The `event: end` final event lets the dashboard distinguish "container exited cleanly" from "connection dropped" without parsing HTTP-level signals.

---

## Error response shapes (all wire formats)

| HTTP | Body / SSE event | When |
|---|---|---|
| 400 `invalid-since` | `{"error":"invalid --since duration"}` | `?since=notanumber` |
| 400 `invalid-tail` | `{"error":"tail must be >= -1"}` | `?tail=-99` |
| 400 `invalid-replica` | `{"error":"replica must be >= 0"}` | `?replica=-5` |
| 403 `logs-cross-project` | `{"error":"unauthorized for project \"X\""}` | bearer token's subject not allowed in `{project}` |
| 404 `service-not-found` | `{"error":"service \"Y\" not found in project \"X\""}` | unknown service |
| 503 `replica-not-found` | `{"error":"replica N not found (service has M replicas)"}` | replica index out of range |
| 503 `replica-not-available` | `{"error":"replica N not running yet"}` | container exists in store but not yet started |

For SSE responses where the error happens BEFORE the stream opens, the server writes a final `event: error\ndata: <json>\n\n` and closes. After the stream is open, errors become `event: end` with a reason field.

---

## CLI flag → query param mapping

| CLI flag | Query param | Server-side default | Notes |
|---|---|---|---|
| `--follow`, `-f` | `follow=true` | `false` | omit for one-shot |
| `--tail N` | `tail=N` | `-1` (all) | maps directly |
| `--since DUR` | `since=<RFC3339-now-minus-DUR>` | none | CLI converts duration → absolute timestamp before sending |
| `--replica I` | `replica=I` | `0` | int |
| `--timestamps` | _(future polish)_ | _(future polish)_ | not in v0.4 |

The CLI does the duration-to-timestamp conversion so the server doesn't need a per-server `time.Now()` interpretation (clock skew safety + simpler server-side code).

---

## Project-scoping enforcement (FR-010, §III)

```go
// At the top of handleStreamServiceLogs, BEFORE any daemon call:
subject := s.authn.SubjectFromContext(r.Context())
allowed, err := s.authz.SubjectAllowedInProject(r.Context(), subject, project)
if err != nil || !allowed {
    writeError(w, http.StatusForbidden, "logs-cross-project",
        fmt.Sprintf("unauthorized for project %q", project))
    return
}
```

No `Runtime.ListContainers` call, no `StreamLogs` call, no daemon resource consumed.

---

## Lifecycle of one log request

```
client GET /api/v1/.../logs?follow=true
        │
        ├─► auth check (project-scoped) ─── 403 if denied ─────────────────►
        │
        ├─► resolve replica index → containerID
        │           │
        │           ├─ service-not-found ─────── 404 ──────────────────────►
        │           ├─ replica-not-found ─────── 503 ──────────────────────►
        │           └─ replica-not-available ─── 503 ──────────────────────►
        │
        ├─► write headers + (SSE) meta event
        │
        ├─► open Runtime.StreamLogs(ctx, containerID, opts)
        │           │
        │           └─ daemon down ────────────── 502 (one-shot) / event:error (SSE) ►
        │
        ├─► copy demuxed lines to client
        │           │
        │           ├─ each line: writeSSE() / writePlain() + Flush
        │           └─ client disconnect (ctx cancel) → reader exits → conn closes
        │
        └─► container exits → reader EOF → write event:end (SSE) / close (Plain) ─►
```

---

## What's intentionally NOT in this data model

- **Log line schema** — Proxa does not parse, structure, or filter lines. Bytes in, bytes out.
- **Persistent retention metadata** — Docker owns retention; we don't track it.
- **Rate limits per subject** — out of scope; a future security/quota feature.
- **Multi-replica aggregation** — out of scope per spec; one stream per request.

# Phase 0 — Research: Logs Streaming

Five decisions resolved before Phase 1 design.

---

## R-001: Dashboard streaming JS — HTMX-ext-sse vs Alpine + EventSource vs custom

**Decision**: Alpine.js + native browser `EventSource` API. Zero new vendored JS.

**Rationale**:

- `EventSource` is a standard browser API since 2009; works in every browser we care about. Zero new bytes.
- HTMX-ext-sse (~3 KiB) would be more declarative but adds a license-audit row and a vendored file to maintain. The declarativeness savings are tiny — Alpine's `x-init` + `x-data` cover the controller in ~25 lines of JS inline in the template.
- A custom WebSocket protocol would require server-side `gorilla/websocket` (new dep, ~30 KiB transitive). Overkill for one-way streaming.
- HTMX without SSE-ext (just `hx-trigger="every 1s"`) is poll-based — adds load and skips lines between polls. SSE is push-based.

**Alternatives rejected**:

- **HTMX-ext-sse**: +1 vendored file, +1 license entry; declarative win not worth it for ~25 lines of saved JS.
- **WebSockets via `gorilla/websocket`**: bidirectional protocol for a one-way use case; new dep.
- **Plain HTMX polling**: drops lines between polls; not "follow" semantics.

**Implementation**: The `logs.html` template embeds an Alpine controller (`x-data="logsController"`) that:

1. Opens an `EventSource('/api/v1/.../logs?follow=true')` on mount.
2. Appends each `event.data` line to a `<pre>` element.
3. Auto-scrolls to bottom unless the user has scrolled up (tracked via `scroll` event on the `<pre>`).
4. Closes the EventSource and reopens with the new replica index when the dropdown changes.
5. Closes the EventSource on `beforeunload` so the daemon connection drains promptly.

---

## R-002: Wire format — SSE vs chunked transfer vs WebSockets

**Decision**: Dual-format per Accept header. `text/event-stream` → SSE (dashboard). Anything else → chunked transfer with raw `\n`-separated lines (CLI + curl-style consumers).

**Rationale**:

- SSE for browsers because `EventSource` only speaks SSE.
- Chunked transfer with raw newlines for CLI because Go's `http.Client` makes it trivial: copy `resp.Body` to stdout. No SSE framing to strip. Curl users get the same thing.
- One handler, branch on `r.Header.Get("Accept")`. Both formats share the same upstream reader.

**Alternatives rejected**:

- **SSE for everyone**: forces the CLI to strip `data: ` prefixes and parse `\n\n` events. Unnecessary friction.
- **Raw chunked for everyone**: dashboard's `EventSource` won't accept non-SSE responses.
- **WebSockets**: see R-001.

**Implementation**: `internal/server/sse.go` exports two writers — `writeSSE(w, line)` and `writePlain(w, line)`. Both call `w.(http.Flusher).Flush()` after every line so the client sees lines immediately.

---

## R-003: Context cancellation — how the HTTP request cancel reaches the daemon stream

**Decision**: Use `http.Request.Context()` directly as the ctx passed to `Runtime.StreamLogs`. When the HTTP server cancels the request context (client disconnect, server shutdown, Ctrl-C-on-CLI dropping the TCP connection), the daemon stream's underlying `io.Reader` returns immediately because the Docker client honors ctx.

**Rationale**:

- `docker/docker/client.Client.ContainerLogs(ctx, ...)` cancels its underlying connection when ctx cancels — no additional plumbing needed.
- `net/http.Server` cancels the request context on client disconnect (`http.CloseNotifier` is the legacy way; ctx cancellation is the modern one and Go's stdlib does this automatically).
- The reader goroutine inside the handler ranges over a bufio Scanner on `resp.Body`; when ctx cancels, the daemon closes the body, the scanner errors out, the goroutine exits.

**Alternatives rejected**:

- **Manual heartbeat ping**: unnecessary; ctx cancellation is the canonical mechanism.
- **Wrapping the body in a context-aware reader**: re-implements what the docker client does internally.

**Implementation**: in `handlers_logs.go`:

```go
ctx := r.Context()
rc, err := s.runtime.StreamLogs(ctx, containerID, opts)
defer rc.Close()
// scanner loop... ctx cancels → rc reads errs → loop exits → goroutine returns.
```

---

## R-004: stdout/stderr demultiplexing — stdcopy vs raw

**Decision**: Demux via `github.com/docker/docker/pkg/stdcopy.StdCopy`. Stream stdout AND stderr to the client interleaved (no separation in the wire output — the CLI / dashboard treats them as one stream).

**Rationale**:

- Docker's `ContainerLogs` returns a multiplexed stream when the container was started WITHOUT `tty: true`. Raw bytes look like garbage (8-byte headers per chunk).
- `stdcopy.StdCopy(out, errOut, reader)` is the canonical demux helper, already imported by us for `Exec` in 002.
- Operators almost always want stdout + stderr interleaved (the order they were written). Separating into two channels is rare and adds complexity. We'd revisit if a real workload asks.

**Alternatives rejected**:

- **Raw pass-through**: looks broken (8-byte multiplex headers visible).
- **Two separate SSE event types** (`event: stdout`, `event: stderr`): doubles client-side complexity for almost no win.

**Implementation**: pipe `stdcopy.StdCopy(combined, combined, dockerResponseBody)` where `combined` is an `io.Writer` that funnels both streams into a single line-emitting writer. We use a custom writer that buffers until newline, then emits one SSE/chunked line.

---

## R-005: Replica selection — by index vs by container ID

**Decision**: Operator selects by replica INDEX (0..N-1). The server resolves index → container ID via `Runtime.ListContainers` filtered by `proxa.service` label, sorted by `proxa.replica` label.

**Rationale**:

- Container IDs are opaque hex strings — bad UX for `proxa logs --replica`.
- Replica index is the user-facing identity from day 1 (`proxa-default-web-0`, `-1`, `-2`); operators already know it.
- Resolution happens once per request before the daemon stream opens; cheap.
- If a replica is mid-rotation (old removed, new not yet up), the request returns 503 `replica-not-available` and the client retries naturally.

**Alternatives rejected**:

- **Container ID in CLI**: operator-hostile.
- **Auto-pick "newest" replica**: surprising; multi-replica services want deterministic selection.

**Implementation**: Helper `resolveReplicaContainerID(ctx, project, service, replicaIdx) (containerID string, err error)` in `handlers_logs.go`. Errors: `service-not-found`, `replica-not-found` (out of range), `replica-not-available` (container not running yet).

---

## What's intentionally NOT in this research

- **WebSocket support** — see R-002 rejection. No bidirectional channel needed.
- **Log persistence** — Docker's existing log driver owns it; out of scope per spec.
- **ANSI color rendering** — strip in v0.4, defer color HTML to v0.5+ if asked.
- **Multi-replica merged tail** — out of scope; v0.5+ if a real workload asks.
- **Structured log search** — operators grep; v0.5+ observability feature.

# Contract: `GET /api/v1/projects/{project}/services/{name}/logs`

The log-streaming HTTP endpoint. Delivers SSE or chunked-plain based on the `Accept` header.

## Request

```text
GET /api/v1/projects/{project}/services/{name}/logs?{params}
Authorization: Bearer <token>      # required over TCP listener; bypassed over Unix socket
Accept: text/event-stream           # SSE; default is text/plain (chunked)
```

### Path params

| Name | Type | Notes |
|---|---|---|
| `project` | string | must match an existing project; auth-scoped |
| `name` | string | must match an existing service in `{project}` |

### Query params

| Name | Type | Default | Notes |
|---|---|---|---|
| `follow` | bool | `false` | when true, response stays open and streams new lines |
| `tail` | int | `-1` | `-1` or absent = all available; `N >= 0` = last N lines |
| `since` | RFC3339 timestamp | none | only lines with timestamps after this point |
| `replica` | int | `0` | replica index; service must have at least `replica+1` replicas |

## Response — happy path (SSE)

```text
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
X-Proxa-Container: proxa-default-web-0
X-Proxa-Replica: 0
Connection: keep-alive

event: meta
data: {"container":"proxa-default-web-0","replica":0}

data: 2026-05-17T11:23:45Z first log line

data: 2026-05-17T11:23:46Z second log line

...

event: end
data: {"reason":"container exited"}
```

Empty lines between events are required by the SSE spec.

## Response — happy path (plain chunked)

```text
HTTP/1.1 200 OK
Content-Type: text/plain; charset=utf-8
X-Proxa-Container: proxa-default-web-0
X-Proxa-Replica: 0
Transfer-Encoding: chunked

2026-05-17T11:23:45Z first log line
2026-05-17T11:23:46Z second log line
```

## Error responses

For pre-stream errors (auth, validation, resolution), the server writes a standard JSON error body and the appropriate status code. For mid-stream errors with SSE clients, the server writes an `event: error` and closes.

| Status | Body | Error code | When |
|---|---|---|---|
| 400 | `{"error":"...", "code":"invalid-tail"}` | `invalid-tail` | non-integer or `< -1` |
| 400 | `{"error":"...", "code":"invalid-since"}` | `invalid-since` | not a parseable RFC3339 |
| 400 | `{"error":"...", "code":"invalid-replica"}` | `invalid-replica` | negative |
| 401 | `{"error":"...", "code":"unauthenticated"}` | `unauthenticated` | no bearer token on a TCP listener |
| 403 | `{"error":"...", "code":"logs-cross-project"}` | `logs-cross-project` | subject not allowed in `{project}` |
| 404 | `{"error":"...", "code":"service-not-found"}` | `service-not-found` | service does not exist |
| 503 | `{"error":"...", "code":"replica-not-found"}` | `replica-not-found` | replica index out of range |
| 503 | `{"error":"...", "code":"replica-not-available"}` | `replica-not-available` | container not running yet |
| 502 | `{"error":"...", "code":"runtime-error"}` | `runtime-error` | docker daemon unreachable |

For SSE clients, error 400/401/403/404/503/502 still come back as standard HTTP responses (the `EventSource` API exposes the error via `.onerror`). Mid-stream errors after the 200 is already sent:

```text
event: error
data: {"error":"...", "code":"runtime-error"}
```

## Performance guarantees

- p95 first-byte latency for `follow=false, tail=100`: < 500 ms (FR-001-adjacent).
- Each `data:` event flushes to the client before the next line is read (no batching). Verified by `bufio.Writer` followed by `http.Flusher.Flush()` per line.
- Server-side memory per stream: O(1) — single line buffer, no in-process accumulation.

## Concurrency

10 concurrent streams against the same service produce 10 independent daemon connections. No cross-talk, no shared buffer. The daemon may push back at very high concurrency; document in operations.md if a real user hits this.

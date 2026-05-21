# Contract — System Info HTTP endpoint + CLI command

Implements **FR-007** (dashboard surface) and **FR-008** (CLI parity). Single source of truth for runtime introspection.

## HTTP — `GET /api/v1/system`

### Request

```
GET /api/v1/system HTTP/1.1
Host: proxa.example.com
Authorization: Bearer <token>
Accept: application/json
```

- **Auth**: bearer token required, same middleware as the rest of `/api/v1/*`. Unix-socket bypass MAY apply (matches existing behavior for read-only endpoints).
- **Scope**: NOT project-scoped — this is per-node runtime metadata, not per-project resource state. Any authenticated subject can read it; no `Authorize(VerbX, ServiceRef)` check.

### Response — 200 OK

```json
{
  "go_version": "go1.26.0",
  "commit": "22dda33",
  "build_date": "2026-05-19T20:45:00Z",
  "proxa_version": "v0.4.1",
  "go_experiments": [],
  "gomaxprocs": 8,
  "gomaxprocs_source": "host",
  "numcpu_host": 8
}
```

- `go_experiments` is `[]` (NOT `null`, NOT `["none"]`) when none active.
- `gomaxprocs_source` is one of: `"host"`, `"container_limit"`, `"env_override"`.
- All keys MUST be present in every response — no optional fields.
- Content-Type: `application/json; charset=utf-8`.

### Response — 401 Unauthorized

```json
{"error": "unauthorized", "code": "missing-token"}
```

Standard error envelope, matches existing API.

### Stability contract

- Adding new top-level fields is non-breaking.
- Renaming or removing top-level fields is a breaking change requiring a major version bump.
- The enum values of `gomaxprocs_source` are stable.

## CLI — `proxa system info`

### Default invocation (plain text)

```
$ proxa system info
proxa_version=v0.4.1
go_version=go1.26.0
commit=22dda33
build_date=2026-05-19T20:45:00Z
go_experiments=
gomaxprocs=8
gomaxprocs_source=host
numcpu_host=8
```

- One `key=value` per line, lowercased keys (matching JSON tags).
- `go_experiments` is the comma-separated list (empty when none).
- Suitable for `eval $(proxa system info)` and `awk -F= '/^gomaxprocs=/ {print $2}'`.

### JSON mode

```
$ proxa system info --json
{"go_version":"go1.26.0", ... }
```

Output matches the HTTP endpoint payload exactly (round-trippable).

### Flag reference

| Flag | Default | Meaning |
|---|---|---|
| `--json` | false | emit the same JSON payload as the HTTP endpoint |
| `--server <unix-path>` | from config | unix socket to query the running proxa server |

### Exit codes

- `0` — success.
- `1` — connection refused, auth error, or any other failure. Error printed to stderr.

### Behavior when no server is running

The CLI fails with exit 1 and `error: cannot reach proxa server at <socket>`. The command does NOT fall back to computing the values from the CLI process itself — operator's question is "what is the *running server* reporting", not "what would *I* report".

## UI — `/ui/system` + dashboard footer card

### `/ui/system` page

- Same data, rendered as a focused full-page view consistent with the slim-IA preference (one card, no nav peer).
- Polled every 30s (slower than Services/Routes — these values rarely change at runtime).
- Includes a "Refresh" button for immediate re-fetch.

### Dashboard footer card

- Appears at the bottom of `/ui/` (the index page), below Services + Routes cards.
- Shows: proxa version, Go version, gomaxprocs (with the source distinguished — e.g., `"8 (host)"` or `"2 (auto-adjusted from container limit)"` or `"4 (env override)"`).
- Click-through to the full `/ui/system` page.
- Does NOT poll independently — refreshes when the page does (every page load + manual refresh).

## Test coverage

`tests/e2e/system_info_test.go`:

1. `proxa init && proxa server` → poll `/api/v1/system` returns 200 with all expected keys.
2. `proxa system info` returns plain-text output with matching values.
3. `proxa system info --json` returns JSON matching the HTTP payload byte-for-byte.
4. `curl http://socket/api/v1/system` without bearer token returns 401.
5. Dashboard footer card renders the proxa version + gomaxprocs source.
6. `/ui/system` page loads and contains all SystemInfo fields.

`tests/e2e/system_info_container_test.go` (optional, Linux-only):

1. Run proxa inside a container with `--cpus=2`; assert `gomaxprocs_source == "container_limit"` and `gomaxprocs == 2`.
2. Run proxa with `GOMAXPROCS=4` env var; assert `gomaxprocs_source == "env_override"` and `gomaxprocs == 4`.

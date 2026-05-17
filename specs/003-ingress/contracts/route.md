# Contract: `[[route]]` TOML grammar + validation

The new TOML construct that lets operators declare reachability.

## Grammar

```toml
# A service may declare zero or more routes.
[[route]]
host = "api.example.com"            # required for L7; FQDN
path = "/v1/*"                      # optional; prefix match with optional trailing *
lb_strategy = "random"              # optional; "random" (default) or "round-robin"

# L4 example — TCP forward
[[route]]
host = "db.example.com"             # used for logging + dashboard; not routed by Host header at L4
l4 = "tcp"
port = 5432

# Probe routing override (lives in [health], not [[route]])
[health]
path = "/health"
port = 80
interval = "5s"
retries = 3
via = "ingress"                     # "" or "direct" (default), or "ingress"
```

## Validation rules

See `data-model.md` § "Validation rules" for the full table.

### Rule details

#### `route-needs-host`

L7 routes (`l4 = ""`) MUST declare `host`. L4 routes MAY declare `host` for display purposes but the field is informational (L4 doesn't speak HTTP, can't read Host header).

#### `route-bad-host`

Hostname must match RFC 1035 syntax for DNS labels: lowercase letters, digits, hyphens, dots. No leading hyphen, no empty labels, no trailing dot in the canonical form Proxa stores.

In v0.3, wildcard hostnames (`*.example.com`) are explicitly rejected for clarity — they will land with DNS-01 ACME challenge in a future release.

#### `route-bad-path`

If declared, `path` MUST start with `/`. The only allowed wildcard is a single trailing `*`:

- `/v1/*` ✓
- `/v1/users` ✓
- `*/v1/*` ✗ (`route-bad-path`)
- `/v1*/users` ✗ (`route-bad-path`)

Path matching semantics:

- `/v1/*` matches `/v1`, `/v1/`, `/v1/anything`.
- `/v1/users` matches `/v1/users` exactly; not `/v1/users/123`.
- An empty/absent `path` matches every request to the matching `host` (catch-all for that hostname).

#### `route-invalid-protocol`

`l4` ∈ {"", "tcp", "udp"}. Anything else rejected at parse time.

#### `route-needs-port`

If `l4` is "tcp" or "udp", `port` MUST be an integer between 1 and 65535. The proxa server binds this port directly — the operator is responsible for ensuring no other process owns it.

#### `route-bad-lb`

`lb_strategy` ∈ {"", "random", "round-robin"}. Defaults to "random" when empty.

#### `route-conflict`

Two failure modes:

1. **In-project conflict**: within one project, two services declare routes with the same `(host, path)` pair.
2. **Cross-project conflict**: in two different projects, two routes declare the same `host` (any path). This protects against tenant spoofing (§III).

Both conditions fire the same `route-conflict` error code. The error message identifies the conflicting service + project so the operator can fix the right TOML.

#### `health-bad-via`

`via` ∈ {"", "direct", "ingress"}. Empty string is equivalent to "direct" (preserves v0.2 behavior).

## Validation timing

- **Parse time** (`proxa up <file.toml>`): all per-route rules above + the in-project + cross-project `route-conflict` check.
- **Reconcile time**: no extra validation (the StateStore is the authoritative source of routes; if it round-trips JSON correctly it's already valid).

## Multiple routes per service

A service may declare multiple `[[route]]` blocks. All routes for a service share the same backend pool (the service's replicas). This lets operators expose the same backend on multiple hostnames or both an HTTP and a TCP route without duplicating containers.

```toml
name = "api"
image = "ghcr.io/foo/api:1.0"
replicas = 3

[[expose]]
container = 80
host = 0
protocol = "http"

[[route]]
host = "api.example.com"

[[route]]
host = "api-legacy.example.com"
path = "/v1/*"
```

## What's intentionally NOT in this contract

- **Per-route timeouts, retries, header rewrites** — out of scope for v0.3.
- **WAF rules per route** — Coraza or similar in a future feature.
- **TLS per-route override** (e.g., one route uses TLS, another doesn't) — `[ingress].tls` is server-wide in v0.3; all TLS-enabled hosts share the same ACME issuer.
- **Custom 404 / 503 bodies** — the ingress returns minimal text bodies; HTML customization comes with the future observability/dashboard work.

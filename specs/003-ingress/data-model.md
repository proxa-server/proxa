# Phase 1 — Data Model: Routes, Backend Pools, TLS Certificates, Ingress Config

No SQLite schema change. Routes live inside `services.spec_json` as a new `routes` field. The parser+serializer already round-trip the entire `types.TaskDef` to JSON; we extend the type, and persistence comes for free.

---

## `pkg/types/taskdef.go` additions

```go
// Route is one operator-declared mapping of (hostname, optional path)
// → this service. Multiple routes per service are allowed (e.g., a
// service serving both api.example.com and admin.example.com).
type Route struct {
    Host       string `toml:"host"        json:"host"`              // FQDN; required for L7
    Path       string `toml:"path"        json:"path,omitempty"`     // prefix; "*" trailing wildcard only
    L4         string `toml:"l4"          json:"l4,omitempty"`       // "" (L7), "tcp", "udp"
    Port       int    `toml:"port"        json:"port,omitempty"`     // L4 only — required for tcp/udp routes
    LBStrategy string `toml:"lb_strategy" json:"lbStrategy,omitempty"` // "random" (default), "round-robin"
}

// HealthCheck (existing) — add Via field
type HealthCheck struct {
    // ...existing fields (Path, Port, Command, Interval, Timeout, Retries) ...
    Via string `toml:"via" json:"via,omitempty"` // "" or "direct" → bridge IP (default); "ingress" → loopback via our ingress
}

// TaskDef (existing) — add Routes
type TaskDef struct {
    // ...existing fields...
    Routes []Route `toml:"route" json:"routes,omitempty"`   // [[route]] blocks in TOML
}
```

---

## In-memory ingress state (`internal/ingress/`)

```go
package ingress

// Router holds the immutable routing snapshot. Stored in an
// atomic.Pointer[*Router] so updates are lock-free for readers.
type Router struct {
    // L7: project + host + path-prefix → service identity
    l7Routes []l7Route // sorted by path-prefix length descending → longest-match wins

    // L4: (proto, port) → service identity. Ports are global across
    // projects; the parser enforces no two services bind the same port.
    l4Routes map[l4Key]l4Route
}

type l7Route struct {
    Project  string
    Host     string
    PathGlob string  // "" or "/v1/" (trailing slash semantics described in contracts/route.md)
    Service  serviceID
    LB       string
}

type l4Route struct {
    Project string
    Service serviceID
    LB      string
}

type l4Key struct{ Proto string; Port int }

type serviceID struct{ Project, Service string }

// BackendPool is the ingress-side view of a service's reachable
// replicas. Updated by the reconciler via Ingress.UpdateBackends after
// each tick when probe snapshots stabilize.
type BackendPool struct {
    mu       sync.RWMutex
    backends []Backend
    cursor   uint64 // round-robin index (atomic)
}

type Backend struct {
    ContainerID string
    IPAddress   string // bridge IP from runtime.ContainerInfo.IPAddress
    Healthy     bool   // mirrors probe.Snapshot.HealthOK
}

// Pick returns one healthy backend per the LB strategy, or nil when
// no backend is healthy (caller serves 503).
func (p *BackendPool) Pick(strategy string) *Backend { ... }
```

---

## TLS certificate state (CertMagic-owned, surfaced for the dashboard)

```go
// CertStatus is what the dashboard renders for each route's TLS chip.
type CertStatus string

const (
    CertStatusOff       CertStatus = "off"       // tls disabled in [ingress] config
    CertStatusPending   CertStatus = "pending"   // route declared, ACME not yet succeeded
    CertStatusValid     CertStatus = "valid"     // cert issued, > 30 days remain
    CertStatusRenewing  CertStatus = "renewing"  // < 30 days remain, ACME job in flight
    CertStatusFailed    CertStatus = "failed"    // last ACME attempt failed; cert may still be valid
)

// CertInfo is read by buildUIData via Ingress.CertInfo(host) for the dashboard.
type CertInfo struct {
    Host           string
    Status         CertStatus
    NotAfter       time.Time // zero when no cert yet
    LastRenewalErr string    // empty when no error
}
```

---

## State machines

### Route lifecycle

```
   parsed → in_store → in_ingress_router → matched_by_request
                              │
                              ▼
                       (route edit / removal)
                              │
                              ▼
                        drained → gone
```

- **parsed**: TOML accepted; route appears in the proxa store after `proxa up`.
- **in_store**: persisted in `services.spec_json`; not yet in the live router.
- **in_ingress_router**: next reconciler tick has called `Ingress.UpdateRoutes`; live traffic is matched.
- **matched_by_request**: handler is running; the snapshot Router is pinned via the closure.
- **drained**: route removed/edited; old Router still pinned by in-flight requests; new Router serves new requests.
- **gone**: all in-flight requests holding the old Router have returned; GC reclaims it.

### Certificate lifecycle

```
   off ──tls=true──► pending ──ACME ok──► valid ──30d before expiry──► renewing
                       │                    ▲                            │
                       │                    └──── ACME ok ───────────────┘
                       └──ACME fail──► failed (cert may still be valid)
```

Transitions are driven by CertMagic's internal timers + ACME challenge results; ingress surfaces the current state via `CertInfo` for the dashboard.

---

## Validation rules (`internal/parser/toml/validate.go` extension)

| Rule | Error code |
|---|---|
| `[[route]]` with `l4=""` (L7) requires non-empty `host` | `route-needs-host` |
| `host` must be a valid hostname (RFC 1035 chars, optional leading `*` rejected in v0.3) | `route-bad-host` |
| `path`, when set, must start with `/` and may end with one `*` (no `*` in the middle) | `route-bad-path` |
| `l4` ∈ {"", "tcp", "udp"} | `route-invalid-protocol` |
| `l4 != ""` requires `port` between 1 and 65535 | `route-needs-port` |
| `lb_strategy` ∈ {"", "random", "round-robin"} | `route-bad-lb` |
| Within one project, no two routes share the same (host, path) pair | `route-conflict` |
| Across projects, no two routes share the same `host` | `route-conflict` |
| `[health].via` ∈ {"", "direct", "ingress"} | `health-bad-via` |

`route-conflict` is detected at parse time when a service's `[[route]]` block is added or modified — the parser fetches all currently-registered routes from the StateStore via the existing `routesByHost(ctx)` helper (added in this feature) and compares.

---

## Container labels — additions

None. Routes are operator metadata, not container metadata. The ingress identifies backends by `(project, service, replica)` labels that already exist on every Proxa-managed container (since 001).

---

## Storage

| What | Where |
|---|---|
| Routes | `services.spec_json` → `routes` field. No migration. |
| Health.Via | `services.spec_json` → `health.via` field. No migration. |
| ACME account key | `${PROXA_DATA_DIR}/certs/acme/acme-v02.api.letsencrypt.org/users/<email>/<email>.key` (CertMagic default layout, mode 0600). |
| Issued certs | `${PROXA_DATA_DIR}/certs/certificates/acme-v02.api.letsencrypt.org-directory/<host>/<host>.crt` (CertMagic default, mode 0600). |
| Ingress config | `${PROXA_DATA_DIR}/config.toml` `[ingress]` section + `PROXA_INGRESS_*` env vars (read by `internal/config/config.go`). |

---

## What's intentionally NOT in this data model

- **Cert history / audit log** — Feature 004 (audit log) territory.
- **Per-route metrics persistence** — Metrics feature; would live in a time-series sidecar or in-memory ring.
- **Route weights / canary splits** — Feature 005.
- **Per-backend health snapshots in the route table** — the BackendPool is derived state, not authoritative; rebuilt every reconciler tick from probe.Snapshot.

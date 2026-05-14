# Contract: `internal/ingress.IngressController`

Public-facing traffic ingress. L7 (HTTP/HTTPS) via reverse proxy with automatic TLS (CertMagic); L4 (raw TCP/UDP) via stdlib `net`. Routes are derived from `TaskDef.Expose` plus per-project ingress config.

## Go signature (v0.0)

```go
package ingress

import (
    "context"
    "errors"
    "net"
)

// IngressController accepts external connections and forwards to backend replicas.
type IngressController interface {
    // Lifecycle
    Start(ctx context.Context, cfg Config) error
    Stop(ctx context.Context) error

    // Route management — called by the reconciler whenever a service's
    // expose list or replica set changes.
    UpsertRoute(ctx context.Context, r Route) error
    DeleteRoute(ctx context.Context, project, name string) error
    ListRoutes(ctx context.Context, project string) ([]Route, error)
}

type Config struct {
    HTTPAddr   string   // ":80"
    HTTPSAddr  string   // ":443"
    L4Listeners []L4Listener
    ACMEEmail  string   // CertMagic contact email
    ACMECache  string   // disk path for cert cache
}

type L4Listener struct {
    Addr     string // ":5432"
    Protocol string // tcp | udp
}

type Route struct {
    Project   string
    Service   string                // proxa service name
    Protocol  string                // http | https | tcp | udp
    Match     RouteMatch
    Backends  []net.Addr            // current healthy replicas; updated on reconcile
}

type RouteMatch struct {
    Host       string   // L7: e.g., "api.example.com"
    PathPrefix string   // L7: optional; "/" if empty
    Port       int      // L4: required
}

var (
    ErrNotFound       = errors.New("ingress: route not found")
    ErrNotImplemented = errors.New("ingress: not implemented")
)
```

## Behavioral contract

1. **TLS is automatic for L7.** ACME issuance happens on-demand for hosts that have at least one matching route. Manual cert upload is supported via `Config.ACMECache` but not required.
2. **Backend health is the reconciler's job.** The ingress only forwards to backends present in `Route.Backends`; if a replica goes unhealthy, the reconciler calls `UpsertRoute` again with the trimmed list.
3. **Zero-downtime route swaps**: `UpsertRoute` MUST atomically replace the backend pool for an existing route; no requests dropped during the swap.
4. **L4 listeners are per-port.** Each `L4Listener` in `Config` binds at `Start`; runtime port changes require `Stop` + reconfigure + `Start`.
5. **Routes are project-scoped.** No cross-project route conflicts even with same hostname (impl detail: route key is `(project, host, pathPrefix)` for L7, `(protocol, port)` for L4).

## Foundation deliverable

`internal/ingress/ingress.go` declares the interface + types + `ErrNotImplemented`. No CertMagic dep imported yet (FR-009).

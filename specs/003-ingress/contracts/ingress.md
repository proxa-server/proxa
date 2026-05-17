# Contract: `internal/ingress.Ingress`

The ingress abstraction. One concrete implementation in v0.3 (`certMagicIngress`); the v1.0 cluster-mode replacement plugs in behind the same interface.

## Go signature

```go
package ingress

import (
    "context"

    "github.com/proxa-server/proxa/internal/probe"
    "github.com/proxa-server/proxa/pkg/types"
)

// Ingress is the contract every routing-layer implementation satisfies.
type Ingress interface {
    // Name identifies the implementation ("certmagic", "fake", ...).
    Name() string

    // Run blocks until ctx cancels. Binds the configured HTTP/HTTPS/L4
    // listeners and serves traffic until shutdown. Returns the first
    // non-nil listener error or nil on clean shutdown.
    //
    // Caller (cli/server.go runServer) launches this in a goroutine
    // alongside reconciler.Run.
    Run(ctx context.Context) error

    // UpdateRoutes replaces the entire routing table atomically. The
    // reconciler calls this each tick after probe snapshots stabilize.
    // routes maps a serviceID to the slice of Route declarations on
    // that service's TaskDef.
    //
    // Errors here are configuration errors (conflicting routes that
    // somehow slipped past the parser) — the caller logs and continues
    // serving with the previous table.
    UpdateRoutes(ctx context.Context, routes map[ServiceID][]types.Route) error

    // UpdateBackends replaces the backend pool for a single service.
    // Called by the reconciler each tick with the current
    // (containerID, IP, healthy) tuples derived from probe.Snapshot.
    UpdateBackends(ctx context.Context, svc ServiceID, backends []Backend)

    // CertInfo returns the current TLS certificate state for a
    // hostname. Used by buildUIData to render the dashboard chip.
    // Returns (zero, false) when the hostname is not in the router or
    // when TLS is disabled globally.
    CertInfo(host string) (CertInfo, bool)

    // IngressInfo returns server-wide ingress metadata for the dashboard
    // header (HTTP port, HTTPS port, TLS on/off, cert count).
    IngressInfo() IngressInfo
}

type ServiceID struct {
    Project string
    Service string
}

type Backend struct {
    ContainerID string
    IPAddress   string
    Healthy     bool
}

type IngressInfo struct {
    HTTPPort  int
    HTTPSPort int
    TLSEnabled bool
    CertCount  int
}
```

## Behavioral contract

1. **Atomic route table swap** — `UpdateRoutes` returns only after the new table is live; subsequent requests use the new table. In-flight requests continue against the snapshot they captured at start. Per R-003.
2. **Backend pool freshness** — `UpdateBackends` is called per service per tick. The pool snapshot is also an atomic swap (RWMutex used only by the cursor for round-robin, see backend_pool.go).
3. **No 5xx on reload** — verified by tests/e2e/ingress_reload_test.go (SC-003).
4. **TLS lifecycle is internal** — operators never call CertMagic directly; `Run` constructs CertMagic with the config, then `UpdateRoutes` tells CertMagic which hostnames need certs.
5. **L4 connections are pinned for life** — once a TCP backend is chosen at `Accept`, the goroutine that copies bytes between client and backend uses that backend until either side closes the conn or the context cancels.
6. **Graceful shutdown** — on ctx cancel, the implementation MUST stop accepting new connections and drain in-flight requests up to a deadline (default 30 s), then close.

## Error handling

The ingress NEVER panics on a request. Backend selection failures (no healthy replica) return `503 Service Unavailable` with `Retry-After: 5`. TLS handshake failures (bad SNI) return the standard TLS alert. L4 forwarder errors (backend down mid-connection) result in a connection close, logged at INFO level.

## Concurrency

- `UpdateRoutes` and `UpdateBackends` are called from the reconciler goroutine, never concurrently with themselves.
- Request handlers are called by `net/http` goroutines, one per request.
- The Router pointer is atomically loaded at request start; the BackendPool's `Pick` uses an `atomic.Uint64` cursor for round-robin so no lock is held in the hot path.

# Contract: `internal/ingress.Router`

```go
// Router resolves (host, path) → ServiceID for L7 traffic, and
// (proto, port) → ServiceID for L4. Immutable — replaced atomically
// on UpdateRoutes.
type Router struct {
    // ...see data-model.md for the internal layout...
}

// LookupL7 returns the matched service + LB strategy for an HTTP
// request, or (zero, false) when no route matches.
func (r *Router) LookupL7(host, path string) (ServiceID, string, bool)

// LookupL4 returns the matched service for an inbound (proto, port).
func (r *Router) LookupL4(proto string, port int) (ServiceID, string, bool)
```

Lookup is O(N) over L7 routes (sorted longest-path-first; first match wins) and O(1) over L4 (map lookup). For N up to ~100 routes the linear L7 scan is faster than a trie due to cache locality; revisit if a single deployment ever has > 500 routes.

# Contract: `internal/ingress.BackendPool`

See data-model.md. The pool is internal state owned by the Ingress implementation, never returned to callers — `Pick` returns a copy of one `Backend` value, never a pointer that could outlive the pool snapshot.

// Package ingress is the public-facing traffic ingress. L7 (HTTP/HTTPS)
// is served via a reverse proxy with automatic TLS via CertMagic; L4
// (raw TCP/UDP) is served via stdlib net listeners.
//
// Routes are derived from TaskDef.Expose plus per-project ingress config
// and updated by the reconciler whenever a service's expose list or
// replica set changes.
//
// See specs/000-foundation/contracts/ingresscontroller.md for the full
// behavioral contract.
package ingress

import (
	"context"
	"errors"
	"net"
)

// IngressController accepts external connections and forwards them to
// backend replicas. Implementations: defaultController (v0.x).
//
// Behavioral rules (full contract in
// specs/000-foundation/contracts/ingresscontroller.md):
//
//   - TLS is automatic for L7 routes via on-demand ACME issuance.
//   - Backend health is the reconciler's job; the ingress only forwards
//     to backends present in Route.Backends.
//   - UpsertRoute MUST atomically replace the backend pool for an
//     existing route — no requests dropped during the swap.
//   - L4 listeners bind at Start; runtime port changes require restart.
//   - Routes are project-scoped; key is (project, host, pathPrefix) for
//     L7, (protocol, port) for L4.
type IngressController interface {
	Start(ctx context.Context, cfg Config) error
	Stop(ctx context.Context) error

	UpsertRoute(ctx context.Context, r Route) error
	DeleteRoute(ctx context.Context, project, name string) error
	ListRoutes(ctx context.Context, project string) ([]Route, error)
}

// Config is the ingress's startup configuration.
type Config struct {
	HTTPAddr    string // ":80"
	HTTPSAddr   string // ":443"
	L4Listeners []L4Listener
	ACMEEmail   string // CertMagic contact email
	ACMECache   string // disk path for cert cache
}

// L4Listener describes one raw TCP/UDP listener.
type L4Listener struct {
	Addr     string // ":5432"
	Protocol string // tcp | udp
}

// Route is one ingress route — bound to a single project + service.
type Route struct {
	Project  string
	Service  string
	Protocol string // http | https | tcp | udp
	Match    RouteMatch
	Backends []net.Addr // current healthy replicas, set by the reconciler
}

// RouteMatch describes how a request maps to a route. L7 uses Host +
// PathPrefix; L4 uses Port (the listener Addr selects the protocol).
type RouteMatch struct {
	Host       string
	PathPrefix string
	Port       int
}

var (
	// ErrNotFound is returned when the requested route does not exist.
	ErrNotFound = errors.New("ingress: route not found")

	// ErrNotImplemented is returned by stub implementations.
	ErrNotImplemented = errors.New("ingress: not implemented")
)

// noopIngressController satisfies [IngressController] with
// ErrNotImplemented for every method. Useful as a placeholder in unit
// tests.
type noopIngressController struct{}

// Compile-time assertion that noopIngressController satisfies
// IngressController.
var _ IngressController = noopIngressController{}

func (noopIngressController) Start(context.Context, Config) error             { return ErrNotImplemented }
func (noopIngressController) Stop(context.Context) error                      { return ErrNotImplemented }
func (noopIngressController) UpsertRoute(context.Context, Route) error        { return ErrNotImplemented }
func (noopIngressController) DeleteRoute(context.Context, string, string) error {
	return ErrNotImplemented
}
func (noopIngressController) ListRoutes(context.Context, string) ([]Route, error) {
	return nil, ErrNotImplemented
}

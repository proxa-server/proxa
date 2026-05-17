// Package ingress is Proxa's L7/L4 routing layer. It owns the HTTP/HTTPS
// listener, the ACME-driven TLS certificate lifecycle, the per-service
// backend pool, and the TCP/UDP forwarders.
//
// Architecture (see specs/003-ingress/):
//
//   - Routes are project-scoped. (host, path) collisions inside a project
//     and any cross-project host collision are rejected at parse time
//     (FR-017, §III).
//   - Backend pools mirror probe.Snapshot.HealthOK — unhealthy replicas
//     are excluded from selection (FR-013).
//   - The routing table is held in an atomic.Pointer[*Router] so reads
//     are lock-free; hot-reload is a pointer swap (R-003, FR-009).
//   - TLS is auto-provisioned via CertMagic (R-001). HTTP-01 challenge
//     only in v0.3 (R-002).
//
// Interface name `IngressController` is preserved from feature 000 per
// constitution §I (interfaces declared from day one); the method shape
// evolved from CRUD-per-route to snapshot-replace as the reconciler-
// driven design crystallized in 003.
package ingress

import (
	"context"
	"time"

	"github.com/proxa-server/proxa/pkg/types"
)

// IngressController is the contract every routing-layer implementation
// satisfies. One concrete implementation in v0.3 (certMagicIngress);
// the v1.0 cluster-mode replacement plugs in behind the same interface.
//
// See specs/003-ingress/contracts/ingress.md for the full behavioral
// contract.
type IngressController interface {
	// Name identifies the implementation ("certmagic", "fake", ...).
	Name() string

	// Run blocks until ctx cancels. Binds the configured HTTP/HTTPS/L4
	// listeners and serves traffic until shutdown. Returns the first
	// non-nil listener error or nil on clean shutdown.
	Run(ctx context.Context) error

	// UpdateRoutes replaces the entire routing table atomically. The
	// reconciler calls this each tick after probe snapshots stabilize.
	// Errors here are configuration errors (conflicts that slipped past
	// the parser); caller logs and continues serving with the previous
	// table.
	UpdateRoutes(ctx context.Context, routes map[ServiceID][]types.Route) error

	// UpdateBackends replaces the backend pool for a single service.
	// Called by the reconciler each tick with the current
	// (containerID, IP, healthy) tuples derived from probe.Snapshot.
	UpdateBackends(ctx context.Context, svc ServiceID, backends []Backend)

	// CertInfo returns the current TLS certificate state for a hostname.
	// Used by the dashboard's TLS chip. Returns (zero, false) when the
	// hostname is unknown or when TLS is disabled globally.
	CertInfo(host string) (CertInfo, bool)

	// IngressInfo returns server-wide ingress metadata for the dashboard
	// header (HTTP port, HTTPS port, TLS on/off, cert count).
	IngressInfo() IngressInfo
}

// ServiceID uniquely identifies a service across the cluster.
type ServiceID struct {
	Project string
	Service string
}

// Backend is one reachable replica behind a route. Updated by the
// reconciler each tick from runtime.ContainerInfo + probe.Snapshot.
type Backend struct {
	ContainerID string
	IPAddress   string // bridge IP from runtime.ContainerInfo.IPAddress
	Port        int    // container-side port (from PortSpec.Container)
	Healthy     bool   // mirrors probe.Snapshot.HealthOK
}

// CertStatus is what the dashboard renders for each route's TLS chip.
type CertStatus string

const (
	CertStatusOff      CertStatus = "off"      // [ingress].tls = false
	CertStatusPending  CertStatus = "pending"  // route declared, ACME not yet succeeded
	CertStatusValid    CertStatus = "valid"    // cert issued, > 30 days remain
	CertStatusRenewing CertStatus = "renewing" // < 30 days remain, ACME job in flight
	CertStatusFailed   CertStatus = "failed"   // last ACME attempt failed
)

// CertInfo is read by buildUIData via IngressController.CertInfo(host)
// for the dashboard TLS chip rendering.
type CertInfo struct {
	Host           string
	Status         CertStatus
	NotAfter       time.Time // zero when no cert yet
	LastRenewalErr string    // empty when no error
}

// IngressInfo is the server-wide widget shown in the dashboard header.
type IngressInfo struct {
	HTTPPort   int
	HTTPSPort  int
	TLSEnabled bool
	CertCount  int
}

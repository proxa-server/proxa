// Package server hosts the HTTP API and the slim dashboard for the
// Proxa control plane. Wires StateStore + Runtime + Reconciler + Auth
// together behind a chi router.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/proxa-server/proxa/internal/auth"
	"github.com/proxa-server/proxa/internal/config"
	"github.com/proxa-server/proxa/internal/events"
	"github.com/proxa-server/proxa/internal/ingress"
	"github.com/proxa-server/proxa/internal/reconciler"
	rt "github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/internal/store"
)

// Server hosts the HTTP API and slim dashboard.
type Server struct {
	cfg       *config.Config
	store     store.StateStore
	runtime   rt.Runtime
	recon     *reconciler.Reconciler
	ingress   ingress.IngressController // optional; UI shows "off" widget when nil
	events    *events.Store             // optional; v0.4.3+; nil = no events endpoints registered
	authn     auth.Authenticator
	authz     auth.PolicyEngine
	Router    *chi.Mux       // exported so callers can attach more routes
	listener  net.Listener
	httpsrv   *http.Server
}

// New returns a Server ready to Start.
//
// http.CrossOriginProtection (Go 1.25) is mounted as the outermost
// middleware so every route — current and future — inherits the
// defense by default. The check is no-op for GET/HEAD/OPTIONS (the
// dashboard's HTMX polling path) and only rejects state-changing
// requests from a foreign Origin. Mounted BEFORE auth so a malicious
// cross-origin POST fails fast on the header check before any DB
// lookup happens.
func New(cfg *config.Config, st store.StateStore, runtime rt.Runtime, recon *reconciler.Reconciler,
	authn auth.Authenticator, authz auth.PolicyEngine) *Server {
	router := chi.NewMux()
	router.Use(http.NewCrossOriginProtection().Handler)
	return &Server{
		cfg:     cfg,
		store:   st,
		runtime: runtime,
		recon:   recon,
		authn:   authn,
		authz:   authz,
		Router:  router,
	}
}

// WithIngress installs an IngressController so the dashboard + API can
// surface routes and TLS state. Optional — when nil the routes table
// renders empty and the Ingress header widget shows TLS:off.
func (s *Server) WithIngress(i ingress.IngressController) { s.ingress = i }

// WithEvents installs an events.Store so the dashboard + API can surface
// the audit log. Optional — when nil the events API returns 503 and the
// dashboard Events card renders an empty state.
func (s *Server) WithEvents(e *events.Store) { s.events = e }

// IsUnixListener reports whether the server is listening on a Unix
// socket. Used by middleware to decide whether to bypass token auth on
// /ui/ routes.
func (s *Server) IsUnixListener() bool {
	return s.cfg.IsUnixListener()
}

// Start binds the configured listener and serves until ctx cancels or
// Shutdown is called. Blocks until the server stops.
func (s *Server) Start(ctx context.Context) error {
	listener, err := s.openListener()
	if err != nil {
		return err
	}
	s.listener = listener

	s.httpsrv = &http.Server{
		Handler:           s.Router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Shutdown on ctx cancel.
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = s.httpsrv.Shutdown(shutdownCtx)
	}()

	if err := s.httpsrv.Serve(listener); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server: serve: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the server within the given context's deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpsrv == nil {
		return nil
	}
	return s.httpsrv.Shutdown(ctx)
}

// openListener creates the configured net.Listener (Unix or TCP).
func (s *Server) openListener() (net.Listener, error) {
	addr := s.cfg.ListenAddr
	switch {
	case strings.HasPrefix(addr, "unix://"):
		path := strings.TrimPrefix(addr, "unix://")
		// Remove stale socket from prior runs.
		_ = os.Remove(path)
		l, err := net.Listen("unix", path)
		if err != nil {
			return nil, fmt.Errorf("server: listen unix %q: %w", path, err)
		}
		// Tighten socket perms — owner + group can read/write.
		_ = os.Chmod(path, 0o660)
		return l, nil
	case strings.HasPrefix(addr, "tcp://"):
		hostport := strings.TrimPrefix(addr, "tcp://")
		l, err := net.Listen("tcp", hostport)
		if err != nil {
			return nil, fmt.Errorf("server: listen tcp %q: %w", hostport, err)
		}
		return l, nil
	default:
		return nil, fmt.Errorf("server: unsupported listen scheme %q (want unix:// or tcp://)", addr)
	}
}

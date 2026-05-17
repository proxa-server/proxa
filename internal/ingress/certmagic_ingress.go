package ingress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/proxa-server/proxa/internal/config"
	"github.com/proxa-server/proxa/pkg/types"
)

// certMagicIngress is the v0.3 concrete IngressController. It binds:
//   - one HTTP listener on cfg.HTTPPort (ACME challenge + HTTP→HTTPS
//     redirect when TLS is enabled, or plain proxy when TLS is off)
//   - one HTTPS listener on cfg.HTTPSPort (only when TLS=true)
//
// L4 forwarder lifecycle lands with T030 (US3); for now the L4 hook is
// a noop so UpdateRoutes can be called with mixed L7/L4 declarations.
type certMagicIngress struct {
	cfg     config.IngressConfig
	dataDir string
	logger  *slog.Logger

	router *RouterPtr
	pools  *poolRegistry
	tls    *tlsProvider

	proxy   *proxyHandler
	httpSrv *http.Server
	tlsSrv  *http.Server

	mu          sync.RWMutex
	allowedHost map[string]bool // populated from UpdateRoutes for ACME on-demand
}

// New constructs a certMagicIngress with the given config. The
// ingress is not running yet — call Run to bind listeners.
func New(cfg config.IngressConfig, dataDir string, logger *slog.Logger) IngressController {
	if logger == nil {
		logger = slog.Default()
	}
	router := NewRouterPtr()
	pools := newPoolRegistry()
	return &certMagicIngress{
		cfg:         cfg,
		dataDir:     dataDir,
		logger:      logger,
		router:      router,
		pools:       pools,
		tls:         newTLSProvider(cfg, dataDir, logger),
		proxy:       newProxyHandler(router, pools, logger),
		allowedHost: make(map[string]bool),
	}
}

func (i *certMagicIngress) Name() string { return "certmagic" }

// Run binds the listeners and blocks until ctx cancels.
func (i *certMagicIngress) Run(ctx context.Context) error {
	httpMux := http.NewServeMux()
	if i.cfg.TLS {
		// 301 to HTTPS for everything except ACME challenges (which the
		// HTTPS handler doesn't see — they're HTTP-only by ACME design).
		httpMux.HandleFunc("/", i.redirectToHTTPS)
	} else {
		// TLS off → HTTP is the primary listener; route everything.
		httpMux.Handle("/", i.proxy)
	}

	i.httpSrv = &http.Server{
		Addr:              fmt.Sprintf(":%d", i.cfg.HTTPPort),
		Handler:           httpMux,
		ReadHeaderTimeout: 30 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		i.logger.Info("ingress: HTTP listener", "port", i.cfg.HTTPPort, "tls", i.cfg.TLS)
		if err := i.httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("ingress: http listener: %w", err)
		}
	}()

	if i.cfg.TLS {
		tlsConfig, err := i.tls.TLSConfig(ctx)
		if err != nil {
			return fmt.Errorf("ingress: tls config: %w", err)
		}
		i.tlsSrv = &http.Server{
			Addr:              fmt.Sprintf(":%d", i.cfg.HTTPSPort),
			Handler:           i.proxy,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 30 * time.Second,
		}
		go func() {
			i.logger.Info("ingress: HTTPS listener", "port", i.cfg.HTTPSPort)
			// Empty cert/key strings → use Server.TLSConfig.
			if err := i.tlsSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("ingress: https listener: %w", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		// Graceful shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = i.httpSrv.Shutdown(shutdownCtx)
		if i.tlsSrv != nil {
			_ = i.tlsSrv.Shutdown(shutdownCtx)
		}
		i.logger.Info("ingress: stopped")
		return nil
	case err := <-errCh:
		return err
	}
}

// UpdateRoutes replaces the routing table atomically.
func (i *certMagicIngress) UpdateRoutes(ctx context.Context, routes map[ServiceID][]types.Route) error {
	r, err := BuildRouter(routes)
	if err != nil {
		return err
	}
	i.router.Swap(r)

	// Refresh the ACME on-demand allow-list.
	hosts := make(map[string]bool)
	for _, svcRoutes := range routes {
		for _, route := range svcRoutes {
			if route.L4 == "" && route.Host != "" {
				hosts[route.Host] = true
			}
		}
	}
	i.mu.Lock()
	i.allowedHost = hosts
	i.mu.Unlock()
	return nil
}

// UpdateBackends replaces the backend pool for a single service.
func (i *certMagicIngress) UpdateBackends(ctx context.Context, svc ServiceID, backends []Backend) {
	if len(backends) == 0 {
		i.pools.Forget(svc)
		return
	}
	i.pools.Upsert(svc, backends)
}

// CertInfo delegates to the TLS provider.
func (i *certMagicIngress) CertInfo(host string) (CertInfo, bool) {
	return i.tls.CertInfo(host)
}

// IngressInfo returns server-wide ingress metadata for the dashboard.
func (i *certMagicIngress) IngressInfo() IngressInfo {
	return IngressInfo{
		HTTPPort:   i.cfg.HTTPPort,
		HTTPSPort:  i.cfg.HTTPSPort,
		TLSEnabled: i.cfg.TLS,
		CertCount:  i.tls.CertCount(),
	}
}

// redirectToHTTPS issues a 301 to the same path on the HTTPS port,
// preserving the query string.
func (i *certMagicIngress) redirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	target := "https://" + stripPort(r.Host)
	if i.cfg.HTTPSPort != 443 {
		target = fmt.Sprintf("%s:%d", target, i.cfg.HTTPSPort)
	}
	target += r.URL.RequestURI()
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

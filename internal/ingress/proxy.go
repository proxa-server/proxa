package ingress

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

// proxyHandler builds the per-service ReverseProxy graph and serves L7
// HTTP requests via the current Router snapshot + BackendPool selection.
//
// One *httputil.ReverseProxy is created lazily per service the first
// time we route to it. The Director is route-aware (it re-resolves the
// backend per request), so the same underlying ReverseProxy can serve
// many backends as the pool changes.
type proxyHandler struct {
	router *RouterPtr
	pools  *poolRegistry
	logger *slog.Logger

	mu       sync.Mutex
	perSvc   map[ServiceID]*httputil.ReverseProxy
}

func newProxyHandler(router *RouterPtr, pools *poolRegistry, logger *slog.Logger) *proxyHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &proxyHandler{
		router: router,
		pools:  pools,
		logger: logger,
		perSvc: make(map[ServiceID]*httputil.ReverseProxy),
	}
}

func (h *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Snapshot the router pointer ONCE per request. Even if UpdateRoutes
	// publishes a new router mid-request, this request stays on its
	// captured snapshot (FR-009 + SC-003).
	rt := h.router.Load()
	host := stripPort(r.Host)
	svc, lb, ok := rt.LookupL7(host, r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	pool := h.pools.Get(svc)
	if pool == nil {
		writeRetryAfter503(w, "no backend pool for service")
		return
	}
	backend := pool.Pick(lb)
	if backend == nil {
		writeRetryAfter503(w, "no healthy backends")
		return
	}

	proxy := h.getOrCreate(svc)
	// Stash the chosen backend on the request context so Director uses it.
	r2 := r.WithContext(withBackend(r.Context(), backend))
	proxy.ServeHTTP(w, r2)
}

func (h *proxyHandler) getOrCreate(svc ServiceID) *httputil.ReverseProxy {
	h.mu.Lock()
	defer h.mu.Unlock()
	if rp, ok := h.perSvc[svc]; ok {
		return rp
	}
	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			backend, ok := backendFrom(req.Context())
			if !ok || backend == nil {
				// Director can't refuse a request; downstream RoundTripper
				// will fail and ErrorHandler runs. Leave URL empty so dial fails fast.
				req.URL = &url.URL{Scheme: "http", Host: "0.0.0.0:0"}
				return
			}
			req.URL.Scheme = "http"
			req.URL.Host = fmt.Sprintf("%s:%d", backend.IPAddress, backend.Port)
			// Preserve incoming Host header so the backend sees the
			// original hostname (vhosted apps need this).
			if req.Header.Get("X-Forwarded-Host") == "" {
				req.Header.Set("X-Forwarded-Host", req.Host)
			}
			req.Header.Set("X-Forwarded-Proto", "https")
		},
		Transport: &http.Transport{
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   16,
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			h.logger.Warn("proxy: backend error",
				"service", svc.Project+"/"+svc.Service,
				"host", r.Host,
				"path", r.URL.Path,
				"err", err)
			writeRetryAfter503(w, err.Error())
		},
	}
	h.perSvc[svc] = rp
	return rp
}

func writeRetryAfter503(w http.ResponseWriter, reason string) {
	w.Header().Set("Retry-After", "5")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("503 Service Unavailable\n" + reason + "\n"))
}

func stripPort(host string) string {
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
		if host[i] < '0' || host[i] > '9' {
			return host
		}
	}
	return host
}

// --- context plumbing for the chosen backend ---

type backendCtxKey struct{}

func withBackend(ctx context.Context, b *Backend) context.Context {
	return context.WithValue(ctx, backendCtxKey{}, b)
}

func backendFrom(ctx context.Context) (*Backend, bool) {
	v := ctx.Value(backendCtxKey{})
	if v == nil {
		return nil, false
	}
	b, ok := v.(*Backend)
	return b, ok
}

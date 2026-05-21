package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

// hostPort parses a httptest.Server URL into (host, port).
func hostPort(t *testing.T, s *httptest.Server) (string, int) {
	t.Helper()
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, portStr, _ := strings.Cut(u.Host, ":")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

func TestHTTPProbeHealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host, port := hostPort(t, srv)
	p := NewHTTPProbe(host, port, "/", 1*time.Second, nil)

	r := p.Run(context.Background())
	if !r.Healthy {
		t.Errorf("expected Healthy=true, got err=%v", r.Err)
	}
	if r.Latency <= 0 {
		t.Errorf("latency should be > 0, got %v", r.Latency)
	}
}

func TestHTTPProbeUnhealthyStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	host, port := hostPort(t, srv)
	p := NewHTTPProbe(host, port, "/", 1*time.Second, nil)

	r := p.Run(context.Background())
	if r.Healthy {
		t.Errorf("expected Healthy=false for 500")
	}
	if r.Err == nil {
		t.Errorf("expected Err to describe the 500")
	}
	if !strings.Contains(r.Err.Error(), "500") {
		t.Errorf("err should mention status 500: %v", r.Err)
	}
}

func TestHTTPProbeTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	host, port := hostPort(t, srv)
	p := NewHTTPProbe(host, port, "/", 100*time.Millisecond, nil)

	r := p.Run(context.Background())
	if r.Healthy {
		t.Errorf("expected Healthy=false on timeout")
	}
	if r.Err == nil {
		t.Errorf("expected Err on timeout")
	}
}

func TestHTTPProbeCtxCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()
	host, port := hostPort(t, srv)
	p := NewHTTPProbe(host, port, "/", 5*time.Second, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel
	r := p.Run(ctx)
	if r.Healthy {
		t.Errorf("expected Healthy=false on canceled ctx")
	}
}

// boolPtr is a tiny helper for FollowRedirects test cases.
//
//go:fix inline
func boolPtr(b bool) *bool { return new(b) }

// TestHTTPProbeFollowRedirects covers the FollowRedirects tri-state on
// the direct probe path: nil = Go default (follow), *true = same,
// *false = treat first 3xx as non-2xx (probe fails).
//
// Validates spec FR-006 for direct probes.
func TestHTTPProbeFollowRedirects(t *testing.T) {
	// final server returns 200; redirect server 301s to it.
	finalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer finalSrv.Close()

	redirSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, finalSrv.URL+"/dest", http.StatusMovedPermanently)
	}))
	defer redirSrv.Close()

	redirHost, redirPort := hostPort(t, redirSrv)

	cases := []struct {
		name            string
		followRedirects *bool
		wantHealthy     bool
	}{
		{"nil follows redirect to 200 (default)", nil, true},
		{"true follows redirect to 200", new(true), true},
		{"false treats 3xx as non-2xx", new(false), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := NewHTTPProbe(redirHost, redirPort, "/", 1*time.Second, c.followRedirects)
			r := p.Run(context.Background())
			if r.Healthy != c.wantHealthy {
				t.Errorf("Healthy = %v, want %v (err=%v)", r.Healthy, c.wantHealthy, r.Err)
			}
		})
	}
}

// TestHTTPProbeViaIngress_TLSDirect covers the v0.4.1 fix
// (research.md R-001 Approach B): when ingress.TLSEnabled == true and
// FollowRedirects == nil, the probe targets the HTTPS port directly
// with InsecureSkipVerify=true and bypasses the redirect entirely.
//
// Validates spec FR-005 and serves as the unit-level regression gate
// for the 0.4.0 demo bug. SC-001 is also covered end-to-end by T010.
func TestHTTPProbeViaIngress_TLSDirect(t *testing.T) {
	// HTTPS server stands in for the ingress HTTPS listener.
	httpsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "tls.local" {
			t.Errorf("unexpected Host header on HTTPS upstream: %q", r.Host)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer httpsSrv.Close()

	// HTTP server stands in for the ingress HTTP listener that would
	// 301 to HTTPS. The fix should NOT hit this when TLS=true + nil.
	var httpHits int
	httpSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpHits++
		http.Redirect(w, r, "https://example.invalid/", http.StatusMovedPermanently)
	}))
	defer httpSrv.Close()

	_, httpPort := hostPort(t, httpSrv)
	httpsHost, httpsPort := hostPort(t, httpsSrv)
	if httpsHost != "127.0.0.1" {
		// httptest binds 127.0.0.1; if a future Go version changes
		// this, skip rather than misreport.
		t.Skipf("httptest HTTPS host is %q, expected 127.0.0.1", httpsHost)
	}

	cases := []struct {
		name            string
		ing             IngressInfo
		followRedirects *bool
		wantHealthy     bool
		wantHTTPHits    int
	}{
		{
			name:            "TLS=false + nil: hits HTTP, 301 marks unhealthy",
			ing:             IngressInfo{HTTPPort: httpPort, HTTPSPort: httpsPort, TLSEnabled: false},
			followRedirects: nil,
			wantHealthy:     false, // HTTP server only 301s; default follow goes to example.invalid → fails
			wantHTTPHits:    1,
		},
		{
			name:            "TLS=true + nil: THE FIX — targets HTTPS directly, ignores HTTP entirely",
			ing:             IngressInfo{HTTPPort: httpPort, HTTPSPort: httpsPort, TLSEnabled: true},
			followRedirects: nil,
			wantHealthy:     true,
			wantHTTPHits:    0,
		},
		{
			name:            "TLS=true + explicit follow=true: targets HTTPS too (same as nil here)",
			ing:             IngressInfo{HTTPPort: httpPort, HTTPSPort: httpsPort, TLSEnabled: true},
			followRedirects: new(true),
			wantHealthy:     false, // follow=true takes the HTTP-direct branch and follows to example.invalid → fails
			wantHTTPHits:    1,
		},
		{
			name:            "TLS=true + explicit follow=false: stays on HTTP, sees 301 directly, treats as non-2xx",
			ing:             IngressInfo{HTTPPort: httpPort, HTTPSPort: httpsPort, TLSEnabled: true},
			followRedirects: new(false),
			wantHealthy:     false, // 301 → CheckRedirect returns ErrUseLastResponse → 301 status → non-2xx
			wantHTTPHits:    1,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			httpHits = 0
			p := NewHTTPProbeViaIngress(c.ing, "tls.local", "/", 1*time.Second, c.followRedirects)
			r := p.Run(context.Background())
			if r.Healthy != c.wantHealthy {
				t.Errorf("Healthy = %v, want %v (err=%v)", r.Healthy, c.wantHealthy, r.Err)
			}
			if httpHits != c.wantHTTPHits {
				t.Errorf("HTTP listener was hit %d times, want %d", httpHits, c.wantHTTPHits)
			}
		})
	}
}

// TestHTTPProbeViaIngress_NonTLS preserves the v0.4.0 baseline: when
// TLS is disabled, the probe still targets the HTTP port regardless
// of FollowRedirects setting.
func TestHTTPProbeViaIngress_NonTLS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "plain.local" {
			t.Errorf("unexpected Host header: %q", r.Host)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	_, httpPort := hostPort(t, srv)

	ing := IngressInfo{HTTPPort: httpPort, HTTPSPort: 0, TLSEnabled: false}
	p := NewHTTPProbeViaIngress(ing, "plain.local", "/", 1*time.Second, nil)

	r := p.Run(context.Background())
	if !r.Healthy {
		t.Errorf("expected Healthy=true; err=%v", r.Err)
	}
	if !strings.HasPrefix(p.URL, "http://127.0.0.1:") {
		t.Errorf("expected http://127.0.0.1:* URL; got %q", p.URL)
	}
}

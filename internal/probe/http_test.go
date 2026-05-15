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
	p := NewHTTPProbe(host, port, "/", 1*time.Second)

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
	p := NewHTTPProbe(host, port, "/", 1*time.Second)

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
	p := NewHTTPProbe(host, port, "/", 100*time.Millisecond)

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
	p := NewHTTPProbe(host, port, "/", 5*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancel
	r := p.Run(ctx)
	if r.Healthy {
		t.Errorf("expected Healthy=false on canceled ctx")
	}
}

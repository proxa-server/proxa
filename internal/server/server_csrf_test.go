package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestCrossOriginProtection_MountedBeforeAuth covers R-002 (Cross-Origin
// Protection middleware must be mounted at the outermost layer and run
// before the bearer-token auth middleware, so a malicious cross-origin
// POST is rejected on the header check before any DB lookup happens).
//
// Four-case table per the research decision:
//
//	GET / same-origin             → pass (CSRF check is no-op on safe methods)
//	POST / same-origin            → pass (matching Origin)
//	POST / foreign Origin         → 403 (rejected before auth)
//	POST / no Origin (CLI / curl) → pass (no Origin header = no cross-origin claim)
//
// Validates spec FR-003 and SC-004. Uses a test-only POST handler
// mounted on a child router; the production routes don't change.
func TestCrossOriginProtection_MountedBeforeAuth(t *testing.T) {
	// authCalled is set when the auth middleware runs; the CSRF check
	// MUST run before auth (R-002 fail-fast), so for a rejected cross-
	// origin POST authCalled stays false.
	var authCalled bool
	authMW := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authCalled = true
			next.ServeHTTP(w, r)
		})
	}
	handlerCalled := false

	s := New(nil, newMemStore(), noopRuntime{}, nil, fakeAuth{}, nil)
	// Mount a test-only POST endpoint behind the existing CORS-then-auth
	// chain so we can observe what runs.
	s.Router.Route("/test", func(r chi.Router) {
		r.Use(authMW)
		r.Post("/echo", func(w http.ResponseWriter, _ *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusNoContent)
		})
		r.Get("/echo", func(w http.ResponseWriter, _ *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})
	})

	ts := httptest.NewServer(s.Router)
	t.Cleanup(ts.Close)

	cases := []struct {
		name           string
		method         string
		origin         string // "" = don't send Origin header
		wantStatus     int
		wantAuthCalled bool
		wantHandler    bool
	}{
		{
			name:           "GET with foreign origin: safe method, pass-through",
			method:         http.MethodGet,
			origin:         "http://evil.example.com",
			wantStatus:     http.StatusOK,
			wantAuthCalled: true,
			wantHandler:    true,
		},
		{
			name:           "POST same-origin: pass",
			method:         http.MethodPost,
			origin:         ts.URL, // matches server origin
			wantStatus:     http.StatusNoContent,
			wantAuthCalled: true,
			wantHandler:    true,
		},
		{
			name:           "POST foreign origin: rejected BEFORE auth",
			method:         http.MethodPost,
			origin:         "http://evil.example.com",
			wantStatus:     http.StatusForbidden,
			wantAuthCalled: false, // CRITICAL: auth must not run
			wantHandler:    false,
		},
		{
			name:           "POST no Origin (CLI / curl): pass",
			method:         http.MethodPost,
			origin:         "",
			wantStatus:     http.StatusNoContent,
			wantAuthCalled: true,
			wantHandler:    true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			authCalled = false
			handlerCalled = false

			req, err := http.NewRequest(c.method, ts.URL+"/test/echo", strings.NewReader(""))
			if err != nil {
				t.Fatal(err)
			}
			if c.origin != "" {
				req.Header.Set("Origin", c.origin)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != c.wantStatus {
				t.Errorf("status = %d, want %d", resp.StatusCode, c.wantStatus)
			}
			if authCalled != c.wantAuthCalled {
				t.Errorf("authCalled = %v, want %v (CSRF must run BEFORE auth on cross-origin POST)",
					authCalled, c.wantAuthCalled)
			}
			if handlerCalled != c.wantHandler {
				t.Errorf("handlerCalled = %v, want %v", handlerCalled, c.wantHandler)
			}
		})
	}
}

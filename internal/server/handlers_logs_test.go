package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/proxa-server/proxa/internal/auth"
	"github.com/proxa-server/proxa/pkg/types"
)

// forbidAuthz always denies — used to gate the cross-project 403 test.
type forbidAuthz struct{}

func (forbidAuthz) Authorize(context.Context, auth.AuthzRequest) error {
	return auth.ErrForbidden
}
func (forbidAuthz) PoliciesFor(context.Context, string) ([]types.Policy, error) {
	return nil, nil
}

// allowAuthz always allows.
type allowAuthz struct{}

func (allowAuthz) Authorize(context.Context, auth.AuthzRequest) error { return nil }
func (allowAuthz) PoliciesFor(context.Context, string) ([]types.Policy, error) {
	return nil, nil
}

// TestSC_006_LogsCrossProject403 covers SC-006: a request to project A
// from a subject denied by policy returns 403 with stable code
// logs-cross-project. Unit-level because multi-subject support lands
// in a later feature (currently bootstrap-admin is the only subject).
func TestSC_006_LogsCrossProject403(t *testing.T) {
	s := New(nil, newMemStore(), noopRuntime{}, nil, fakeAuth{}, forbidAuthz{})
	s.Router.Route("/api/v1", func(r chi.Router) {
		r.Use(RequireAuth(fakeAuth{}))
		r.Get("/projects/{project}/services/{name}/logs", s.handleStreamServiceLogs)
	})
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/projects/other/services/web/logs", nil)
	req.Header.Set("Authorization", "Bearer fake-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var doc struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("parse body: %v\n%s", err, body)
	}
	if doc.Error != "logs-cross-project" {
		t.Errorf("body error code = %q, want logs-cross-project; body=%s", doc.Error, body)
	}
}

func TestHandleStreamServiceLogsAllowAuthzReachesResolution(t *testing.T) {
	// With allow-all authz, the handler proceeds past auth and tries to
	// resolve the replica. noopRuntime returns no containers → expects
	// 404 service-not-found. This proves the auth gate doesn't block
	// legitimate requests.
	s := New(nil, newMemStore(), noopRuntime{}, nil, fakeAuth{}, allowAuthz{})
	s.Router.Route("/api/v1", func(r chi.Router) {
		r.Use(RequireAuth(fakeAuth{}))
		r.Get("/projects/{project}/services/{name}/logs", s.handleStreamServiceLogs)
	})
	ts := httptest.NewServer(s.Router)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/projects/default/services/missing/logs", nil)
	req.Header.Set("Authorization", "Bearer fake-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (service-not-found, post-auth)", resp.StatusCode)
	}
}

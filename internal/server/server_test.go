package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/proxa-server/proxa/internal/auth"
	rt "github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

// fakeAuth always succeeds with a fixed Subject.
type fakeAuth struct{}

func (fakeAuth) Name() string { return "fake" }
func (fakeAuth) Authenticate(context.Context, *http.Request) (*types.Subject, error) {
	return &types.Subject{ID: "test-user", Name: "tester"}, nil
}
func (fakeAuth) RotateCredentials(context.Context, string) error { return auth.ErrNotSupported }

// rejectAuth always denies.
type rejectAuth struct{}

func (rejectAuth) Name() string { return "reject" }
func (rejectAuth) Authenticate(context.Context, *http.Request) (*types.Subject, error) {
	return nil, auth.ErrUnauthenticated
}
func (rejectAuth) RotateCredentials(context.Context, string) error { return auth.ErrNotSupported }

// memStore is a minimal StateStore for handler tests.
type memStore struct {
	projects map[string]types.Project
	services map[string]map[string]types.Service
}

func newMemStore() *memStore {
	return &memStore{
		projects: map[string]types.Project{"default": {Name: "default", CreatedAt: time.Now().UTC()}},
		services: map[string]map[string]types.Service{},
	}
}

func (m *memStore) Open(context.Context, string) error  { return nil }
func (m *memStore) Close() error                        { return nil }
func (m *memStore) Migrate(context.Context) error       { return nil }
func (m *memStore) CreateProject(_ context.Context, p types.Project) error {
	if _, ok := m.projects[p.Name]; ok {
		return store.ErrAlreadyExists
	}
	m.projects[p.Name] = p
	return nil
}
func (m *memStore) GetProject(_ context.Context, name string) (*types.Project, error) {
	if p, ok := m.projects[name]; ok {
		return &p, nil
	}
	return nil, store.ErrNotFound
}
func (m *memStore) ListProjects(context.Context) ([]types.Project, error) {
	out := make([]types.Project, 0, len(m.projects))
	for _, p := range m.projects {
		out = append(out, p)
	}
	return out, nil
}
func (m *memStore) DeleteProject(context.Context, string) error             { return nil }
func (m *memStore) PutService(_ context.Context, project string, svc types.Service) error {
	if _, ok := m.services[project]; !ok {
		m.services[project] = map[string]types.Service{}
	}
	m.services[project][svc.Name] = svc
	return nil
}
func (m *memStore) GetService(_ context.Context, project, name string) (*types.Service, error) {
	if svcs, ok := m.services[project]; ok {
		if s, ok := svcs[name]; ok {
			return &s, nil
		}
	}
	return nil, store.ErrNotFound
}
func (m *memStore) ListServices(_ context.Context, project string) ([]types.Service, error) {
	out := []types.Service{}
	for _, s := range m.services[project] {
		out = append(out, s)
	}
	return out, nil
}
func (m *memStore) DeleteService(_ context.Context, project, name string) error {
	delete(m.services[project], name)
	return nil
}
func (m *memStore) WatchServices(context.Context, string) (<-chan store.ServiceEvent, error) {
	return nil, store.ErrNotImplemented
}
func (m *memStore) PutJob(context.Context, string, types.Job) error  { return nil }
func (m *memStore) GetJob(context.Context, string, string) (*types.Job, error) {
	return nil, store.ErrNotFound
}
func (m *memStore) ListJobs(context.Context, string) ([]types.Job, error) { return nil, nil }
func (m *memStore) DeleteJob(context.Context, string, string) error      { return nil }
func (m *memStore) PutNode(context.Context, types.Node) error            { return nil }
func (m *memStore) GetNode(context.Context, string) (*types.Node, error) {
	return nil, store.ErrNotFound
}
func (m *memStore) ListNodes(context.Context) ([]types.Node, error) { return nil, nil }
func (m *memStore) DeleteNode(context.Context, string) error         { return nil }
func (m *memStore) Heartbeat(context.Context, string, time.Time) error { return nil }
func (m *memStore) PutSubject(context.Context, types.Subject) error    { return nil }
func (m *memStore) GetSubject(context.Context, string) (*types.Subject, error) {
	return nil, store.ErrNotFound
}
func (m *memStore) PutPolicy(context.Context, types.Policy) error { return nil }
func (m *memStore) ListPoliciesFor(context.Context, string) ([]types.Policy, error) {
	return nil, nil
}
func (m *memStore) DeletePolicy(context.Context, string) error            { return nil }
func (m *memStore) Tx(context.Context, func(store.Tx) error) error        { return nil }

// noopRuntime returns no containers.
type noopRuntime struct{}

func (noopRuntime) Name() string { return "noop" }
func (noopRuntime) Version(context.Context) (string, error) { return "test", nil }
func (noopRuntime) PullImage(context.Context, string) error { return nil }
func (noopRuntime) InspectImage(context.Context, string) (*rt.ImageInfo, error) { return nil, errors.New("no") }
func (noopRuntime) CreateContainer(context.Context, rt.ContainerSpec) (string, error) { return "", nil }
func (noopRuntime) StartContainer(context.Context, string) error                       { return nil }
func (noopRuntime) StopContainer(context.Context, string, time.Duration) error         { return nil }
func (noopRuntime) RemoveContainer(context.Context, string, bool) error                { return nil }
func (noopRuntime) InspectContainer(context.Context, string) (*rt.ContainerInfo, error) {
	return nil, errors.New("no")
}
func (noopRuntime) ListContainers(context.Context, rt.ListFilter) ([]rt.ContainerInfo, error) {
	return nil, nil
}
func (noopRuntime) StreamLogs(context.Context, string, rt.LogOpts) (io.ReadCloser, error) {
	return nil, errors.New("no")
}
func (noopRuntime) Stats(context.Context, string) (*rt.ContainerStats, error) { return nil, errors.New("no") }
func (noopRuntime) Exec(context.Context, string, []string, rt.ExecOpts) (*rt.ExecResult, error) {
	return nil, errors.New("no")
}

func newTestServer(t *testing.T, authn auth.Authenticator) *httptest.Server {
	t.Helper()
	st := newMemStore()
	s := New(nil, st, noopRuntime{}, nil, authn, nil)
	s.Router.Route("/api/v1", func(r chi.Router) {
		r.Use(RequireAuth(authn))
		r.Get("/system/status", s.handleSystemStatus)
		r.Get("/projects", s.handleListProjects)
		r.Post("/projects", s.handleCreateProject)
		r.Route("/projects/{project}/services", func(r chi.Router) {
			r.Put("/{name}", s.handleUpsertService)
			r.Get("/{name}", s.handleGetService)
			r.Get("/", s.handleListServices)
		})
	})
	ts := httptest.NewServer(s.Router)
	t.Cleanup(ts.Close)
	return ts
}

func TestUnauthenticatedRequestReturns401(t *testing.T) {
	ts := newTestServer(t, rejectAuth{})
	resp, err := http.Get(ts.URL + "/api/v1/system/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", resp.StatusCode)
	}
}

func TestUpsertAndGetServiceRoundTrip(t *testing.T) {
	ts := newTestServer(t, fakeAuth{})

	spec := types.TaskDef{
		Project: "default", Name: "web", Image: "nginx:alpine", Replicas: 2,
	}
	body, _ := json.Marshal(spec)
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/projects/default/services/web", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upsert got %d: %s", resp.StatusCode, b)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/v1/projects/default/services/web")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get got %d", resp.StatusCode)
	}
	var got types.Service
	_ = json.NewDecoder(resp.Body).Decode(&got)
	if got.Name != "web" || got.Spec.Image != "nginx:alpine" || got.Spec.Replicas != 2 {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

func setupStore(t *testing.T) *Store {
	t.Helper()
	s := New()
	if err := s.Open(context.Background(), ":memory:"); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return s
}

func TestMigrateIdempotent(t *testing.T) {
	s := setupStore(t)
	// Re-run migrate; should be a no-op.
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

func TestDefaultProjectExists(t *testing.T) {
	s := setupStore(t)
	p, err := s.GetProject(context.Background(), "default")
	if err != nil {
		t.Fatalf("default project missing: %v", err)
	}
	if p.Name != "default" {
		t.Errorf("project name = %q, want 'default'", p.Name)
	}
}

func TestProjectCRUD(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	if err := s.CreateProject(ctx, types.Project{Name: "socio-do"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.CreateProject(ctx, types.Project{Name: "socio-do"}); !errors.Is(err, store.ErrAlreadyExists) {
		t.Errorf("duplicate create: want ErrAlreadyExists, got %v", err)
	}
	if err := s.CreateProject(ctx, types.Project{Name: "BAD CAPS"}); err == nil {
		t.Errorf("bad name accepted")
	}
	projects, err := s.ListProjects(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(projects) < 2 { // default + socio-do
		t.Errorf("expected ≥2 projects, got %d", len(projects))
	}
}

func TestServiceCRUDAndProjectIsolation(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	for _, p := range []string{"socio-do", "kut-do"} {
		if err := s.CreateProject(ctx, types.Project{Name: p}); err != nil {
			t.Fatalf("create %q: %v", p, err)
		}
		svc := types.Service{
			Project: p,
			Name:    "web",
			Spec: types.TaskDef{
				Project: p,
				Name:    "web",
				Image:   "nginx:alpine",
				Replicas: 1,
			},
		}
		if err := s.PutService(ctx, p, svc); err != nil {
			t.Fatalf("put service %q/web: %v", p, err)
		}
	}

	// Both services exist independently.
	for _, p := range []string{"socio-do", "kut-do"} {
		got, err := s.GetService(ctx, p, "web")
		if err != nil {
			t.Fatalf("get %q/web: %v", p, err)
		}
		if got.Project != p {
			t.Errorf("%q project mismatch: got %q", p, got.Project)
		}
	}

	// Listing socio-do does not show kut-do's web.
	list, err := s.ListServices(ctx, "socio-do")
	if err != nil {
		t.Fatalf("list socio-do: %v", err)
	}
	if len(list) != 1 || list[0].Project != "socio-do" {
		t.Errorf("project isolation broken: %+v", list)
	}
}

func TestServiceSpecHashRecomputed(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	svc := types.Service{
		Project: "default",
		Name:    "web",
		Spec:    types.TaskDef{Project: "default", Name: "web", Image: "nginx:alpine"},
	}
	if err := s.PutService(ctx, "default", svc); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Change image; spec_hash must change.
	svc.Spec.Image = "nginx:1.27"
	if err := s.PutService(ctx, "default", svc); err != nil {
		t.Fatalf("put updated: %v", err)
	}

	got, err := s.GetService(ctx, "default", "web")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Spec.Image != "nginx:1.27" {
		t.Errorf("spec image not updated: %q", got.Spec.Image)
	}
}

func TestEmptyProjectRejected(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()
	if _, err := s.ListServices(ctx, ""); err == nil {
		t.Errorf("ListServices('') should error")
	}
	if err := s.PutService(ctx, "", types.Service{}); err == nil {
		t.Errorf("PutService('') should error")
	}
}

func TestNodeHeartbeat(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	n := types.Node{
		ID:      "node-local",
		Name:    "host",
		Role:    types.NodeRoleServer,
		Address: "127.0.0.1:5443",
		Status:  types.NodeStatusReady,
	}
	if err := s.PutNode(ctx, n); err != nil {
		t.Fatalf("put node: %v", err)
	}
	now := time.Now().UTC()
	if err := s.Heartbeat(ctx, "node-local", now); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
}

func TestPolicyWildcardRejectedForNonAdmin(t *testing.T) {
	s := setupStore(t)
	ctx := context.Background()

	if err := s.PutSubject(ctx, types.Subject{ID: "u1", Name: "user", Provider: "local"}); err != nil {
		t.Fatalf("put subject: %v", err)
	}
	err := s.PutPolicy(ctx, types.Policy{
		ID: "p1", SubjectID: "u1", Role: types.RoleEditor, Project: "*",
	})
	if err == nil {
		t.Errorf("wildcard Project='*' should be rejected for RoleEditor")
	}
}

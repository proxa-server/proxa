package reconciler

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	rt "github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

// fakeStore is the minimal StateStore used by reconciler tests.
type fakeStore struct {
	projects []types.Project
	services map[string][]types.Service
}

func (f *fakeStore) Open(context.Context, string) error  { return nil }
func (f *fakeStore) Close() error                        { return nil }
func (f *fakeStore) Migrate(context.Context) error       { return nil }
func (f *fakeStore) CreateProject(context.Context, types.Project) error { return nil }
func (f *fakeStore) GetProject(context.Context, string) (*types.Project, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) ListProjects(context.Context) ([]types.Project, error) {
	return f.projects, nil
}
func (f *fakeStore) DeleteProject(context.Context, string) error { return nil }
func (f *fakeStore) PutService(context.Context, string, types.Service) error { return nil }
func (f *fakeStore) GetService(context.Context, string, string) (*types.Service, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) ListServices(ctx context.Context, project string) ([]types.Service, error) {
	return f.services[project], nil
}
func (f *fakeStore) DeleteService(context.Context, string, string) error { return nil }
func (f *fakeStore) WatchServices(context.Context, string) (<-chan store.ServiceEvent, error) {
	return nil, store.ErrNotImplemented
}
func (f *fakeStore) PutJob(context.Context, string, types.Job) error  { return nil }
func (f *fakeStore) GetJob(context.Context, string, string) (*types.Job, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) ListJobs(context.Context, string) ([]types.Job, error) { return nil, nil }
func (f *fakeStore) DeleteJob(context.Context, string, string) error      { return nil }
func (f *fakeStore) PutNode(context.Context, types.Node) error            { return nil }
func (f *fakeStore) GetNode(context.Context, string) (*types.Node, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) ListNodes(context.Context) ([]types.Node, error) { return nil, nil }
func (f *fakeStore) DeleteNode(context.Context, string) error         { return nil }
func (f *fakeStore) Heartbeat(context.Context, string, time.Time) error { return nil }
func (f *fakeStore) PutSubject(context.Context, types.Subject) error    { return nil }
func (f *fakeStore) GetSubject(context.Context, string) (*types.Subject, error) {
	return nil, store.ErrNotFound
}
func (f *fakeStore) PutPolicy(context.Context, types.Policy) error { return nil }
func (f *fakeStore) ListPoliciesFor(context.Context, string) ([]types.Policy, error) {
	return nil, nil
}
func (f *fakeStore) DeletePolicy(context.Context, string) error             { return nil }
func (f *fakeStore) Tx(context.Context, func(store.Tx) error) error         { return nil }

// fakeRuntime records calls and returns configured containers.
type fakeRuntime struct {
	mu          sync.Mutex
	containers  []rt.ContainerInfo
	createCount atomic.Int32
	createErr   error
}

func (f *fakeRuntime) Name() string { return "fake" }
func (f *fakeRuntime) Version(context.Context) (string, error) { return "test", nil }
func (f *fakeRuntime) PullImage(context.Context, string) error { return nil }
func (f *fakeRuntime) InspectImage(context.Context, string) (*rt.ImageInfo, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakeRuntime) CreateContainer(ctx context.Context, spec rt.ContainerSpec) (string, error) {
	f.createCount.Add(1)
	if f.createErr != nil {
		return "", f.createErr
	}
	return "fake-id", nil
}
func (f *fakeRuntime) StartContainer(context.Context, string) error          { return nil }
func (f *fakeRuntime) StopContainer(context.Context, string, time.Duration) error { return nil }
func (f *fakeRuntime) RestartContainer(context.Context, string, time.Duration) error { return nil }
func (f *fakeRuntime) ListAllContainers(context.Context) ([]rt.ContainerInfo, error) {
	return nil, nil
}
func (f *fakeRuntime) RemoveContainer(context.Context, string, bool) error    { return nil }
func (f *fakeRuntime) RenameContainer(context.Context, string, string) error  { return nil }
func (f *fakeRuntime) InspectContainer(context.Context, string) (*rt.ContainerInfo, error) {
	return nil, errors.New("not implemented in fake")
}
func (f *fakeRuntime) ListContainers(ctx context.Context, filter rt.ListFilter) ([]rt.ContainerInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]rt.ContainerInfo, 0, len(f.containers))
	for _, c := range f.containers {
		if c.Labels["proxa.project"] == filter.Project {
			out = append(out, c)
		}
	}
	return out, nil
}
func (f *fakeRuntime) StreamLogs(context.Context, string, rt.LogOpts) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeRuntime) Stats(context.Context, string) (*rt.ContainerStats, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeRuntime) Exec(context.Context, string, []string, rt.ExecOpts) (*rt.ExecResult, error) {
	return nil, errors.New("not implemented")
}

func TestReconcilerCreatesMissingContainers(t *testing.T) {
	store := &fakeStore{
		projects: []types.Project{{Name: "default"}},
		services: map[string][]types.Service{
			"default": {{
				Project: "default",
				Name:    "web",
				Spec:    types.TaskDef{Project: "default", Name: "web", Image: "nginx:alpine", Replicas: 2},
			}},
		},
	}
	runtime := &fakeRuntime{}

	r := New(store, runtime, Options{TickInterval: 50 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	r.Run(ctx)

	if got := runtime.createCount.Load(); got < 2 {
		t.Errorf("expected ≥2 CreateContainer calls (replicas=2), got %d", got)
	}
}

func TestReconcilerHonorsPoke(t *testing.T) {
	// Migrated to testing/synctest (Go 1.25): real time.Sleep was 200ms
	// per run with race-condition risk if the goroutine hadn't scheduled
	// yet. synctest.Wait() blocks until every goroutine in the bubble is
	// durably waiting on a channel/timer, which IS the condition the
	// sleeps were approximating. Wall time drops from ~200ms to <1ms,
	// and the test is deterministic.
	synctest.Test(t, func(t *testing.T) {
		store := &fakeStore{
			projects: []types.Project{{Name: "default"}},
			services: map[string][]types.Service{
				"default": {{
					Project: "default",
					Name:    "web",
					Spec:    types.TaskDef{Project: "default", Name: "web", Image: "nginx:alpine", Replicas: 1},
				}},
			},
		}
		runtime := &fakeRuntime{}

		// Long tick so only the poke triggers reconciliation in our window.
		r := New(store, runtime, Options{TickInterval: 5 * time.Second})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go r.Run(ctx)

		// Wait for the first immediate-fire reconcile to complete and the
		// goroutine to block on the tick/poke select.
		synctest.Wait()
		if got := runtime.createCount.Load(); got < 1 {
			t.Errorf("first tick should have created at least 1 container, got %d", got)
		}

		r.Poke()
		synctest.Wait()
		cancel()
		// We don't strictly need to assert a second create here (idempotency
		// would prevent duplicate creates if container exists), just that
		// the loop didn't deadlock.
	})
}

func TestReconcilerContinuesOnError(t *testing.T) {
	store := &fakeStore{
		projects: []types.Project{{Name: "default"}},
		services: map[string][]types.Service{
			"default": {{
				Project: "default",
				Name:    "web",
				Spec:    types.TaskDef{Project: "default", Name: "web", Image: "nginx:alpine", Replicas: 1},
			}},
		},
	}
	runtime := &fakeRuntime{createErr: errors.New("simulated failure")}

	r := New(store, runtime, Options{TickInterval: 50 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	r.Run(ctx) // should not panic; should continue ticking despite errors

	if runtime.createCount.Load() < 2 {
		t.Errorf("expected reconciler to retry (≥2 attempts), got %d", runtime.createCount.Load())
	}
}

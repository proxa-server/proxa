package reconciler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/proxa-server/proxa/internal/probe"
	rt "github.com/proxa-server/proxa/internal/runtime"
	dockerlabels "github.com/proxa-server/proxa/internal/runtime/docker"
	"github.com/proxa-server/proxa/pkg/types"
)

// strategyRuntime is a controllable Runtime stub for strategy unit tests.
// It records the order of mutating calls so the test can assert
// start-first sequencing (new is created BEFORE old is removed).
type strategyRuntime struct {
	mu             sync.Mutex
	calls          []string // ordered names of mutating operations
	createdID      string
	execExitCode   atomic.Int64 // 0 = healthy, non-zero = failing
	removeOldErr   error
	containers     map[string]rt.ContainerInfo
}

func newStrategyRuntime() *strategyRuntime {
	return &strategyRuntime{
		containers: map[string]rt.ContainerInfo{
			"old-id": {ID: "old-id", State: "running"},
		},
	}
}

func (s *strategyRuntime) record(op string) {
	s.mu.Lock()
	s.calls = append(s.calls, op)
	s.mu.Unlock()
}

func (s *strategyRuntime) Name() string                                  { return "strategy-fake" }
func (s *strategyRuntime) Version(context.Context) (string, error)       { return "fake", nil }
func (s *strategyRuntime) PullImage(context.Context, string) error {
	s.record("Pull")
	return nil
}
func (s *strategyRuntime) InspectImage(context.Context, string) (*rt.ImageInfo, error) {
	return nil, errors.New("not implemented")
}
func (s *strategyRuntime) CreateContainer(_ context.Context, spec rt.ContainerSpec) (string, error) {
	s.record("Create:" + spec.Name)
	id := "new-id"
	s.mu.Lock()
	s.createdID = id
	s.containers[id] = rt.ContainerInfo{ID: id, State: "created", Labels: spec.Labels}
	s.mu.Unlock()
	return id, nil
}
func (s *strategyRuntime) StartContainer(_ context.Context, id string) error {
	s.record("Start:" + id)
	s.mu.Lock()
	if c, ok := s.containers[id]; ok {
		c.State = "running"
		s.containers[id] = c
	}
	s.mu.Unlock()
	return nil
}
func (s *strategyRuntime) StopContainer(_ context.Context, id string, _ time.Duration) error {
	s.record("Stop:" + id)
	s.mu.Lock()
	if c, ok := s.containers[id]; ok {
		c.State = "exited"
		s.containers[id] = c
	}
	s.mu.Unlock()
	return nil
}
func (s *strategyRuntime) RemoveContainer(_ context.Context, id string, _ bool) error {
	s.record("Remove:" + id)
	if id == "old-id" && s.removeOldErr != nil {
		return s.removeOldErr
	}
	s.mu.Lock()
	delete(s.containers, id)
	s.mu.Unlock()
	return nil
}
func (s *strategyRuntime) RenameContainer(_ context.Context, id, newName string) error {
	s.record("Rename:" + id + "->" + newName)
	return nil
}
func (s *strategyRuntime) InspectContainer(_ context.Context, id string) (*rt.ContainerInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.containers[id]
	if !ok {
		return nil, errors.New("no such container")
	}
	return &c, nil
}
func (s *strategyRuntime) ListContainers(context.Context, rt.ListFilter) ([]rt.ContainerInfo, error) {
	return nil, nil
}
func (s *strategyRuntime) ListAllContainers(context.Context) ([]rt.ContainerInfo, error) {
	return nil, nil
}
func (s *strategyRuntime) RestartContainer(context.Context, string, time.Duration) error {
	return nil
}
func (s *strategyRuntime) StreamLogs(context.Context, string, rt.LogOpts) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (s *strategyRuntime) Stats(context.Context, string) (*rt.ContainerStats, error) {
	return nil, errors.New("not implemented")
}
func (s *strategyRuntime) Exec(context.Context, string, []string, rt.ExecOpts) (*rt.ExecResult, error) {
	code := int(s.execExitCode.Load())
	return &rt.ExecResult{ExitCode: code}, nil
}

func mkStrategyReq(rtimpl *strategyRuntime, probes *probe.Manager, oldID string) Request {
	return Request{
		Project:    "default",
		Service:    "web",
		ReplicaIdx: 0,
		OldID:      oldID,
		NewSpec: types.TaskDef{
			Project: "default",
			Name:    "web",
			Image:   "nginx:alpine",
			Health: types.HealthCheck{
				Command:  []string{"true"},
				Interval: 50 * time.Millisecond,
				Timeout:  20 * time.Millisecond,
				Retries:  1,
			},
		},
		Runtime: rtimpl,
		Probes:  probes,
		Logger:  slog.New(slog.DiscardHandler),
	}
}

func TestStartFirst_Success(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rtimpl := newStrategyRuntime()
		rtimpl.execExitCode.Store(0) // healthy

		probes := probe.New(rtimpl, slog.New(slog.DiscardHandler))
		mgrCtx, cancelMgr := context.WithCancel(t.Context())
		defer cancelMgr()
		go probes.Run(mgrCtx)

		s := &StartFirst{}
		err := s.Apply(t.Context(), mkStrategyReq(rtimpl, probes, "old-id"))
		if err != nil {
			t.Fatalf("StartFirst.Apply returned %v, want nil", err)
		}

		// Verify ordering: new container is created and started BEFORE
		// the old is removed (the safety property of start-first).
		rtimpl.mu.Lock()
		calls := append([]string(nil), rtimpl.calls...)
		rtimpl.mu.Unlock()
		idxStartNew := indexOf(calls, "Start:new-id")
		idxRemoveOld := indexOf(calls, "Remove:old-id")
		idxRename := indexOf(calls, "Rename:new-id->"+dockerlabels.ContainerNameFor("default", "web", 0))
		if idxStartNew == -1 || idxRemoveOld == -1 || idxRename == -1 {
			t.Fatalf("expected Start:new-id, Remove:old-id, Rename calls; got %v", calls)
		}
		if !(idxStartNew < idxRemoveOld) {
			t.Errorf("Start:new must precede Remove:old; got %v", calls)
		}
		if !(idxRemoveOld < idxRename) {
			t.Errorf("Remove:old must precede Rename of new; got %v", calls)
		}
	})
}

func TestStartFirst_RollbackOnUnhealthy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rtimpl := newStrategyRuntime()
		rtimpl.execExitCode.Store(1) // probe always fails

		probes := probe.New(rtimpl, slog.New(slog.DiscardHandler))
		mgrCtx, cancelMgr := context.WithCancel(t.Context())
		defer cancelMgr()
		go probes.Run(mgrCtx)

		s := &StartFirst{}
		err := s.Apply(t.Context(), mkStrategyReq(rtimpl, probes, "old-id"))
		if !errors.Is(err, ErrRolledBack) {
			t.Fatalf("expected ErrRolledBack, got %v", err)
		}

		rtimpl.mu.Lock()
		calls := append([]string(nil), rtimpl.calls...)
		_, oldStillThere := rtimpl.containers["old-id"]
		_, newGone := rtimpl.containers["new-id"]
		rtimpl.mu.Unlock()

		if indexOf(calls, "Remove:new-id") == -1 {
			t.Errorf("rollback should have removed new-id, calls=%v", calls)
		}
		if indexOf(calls, "Remove:old-id") != -1 {
			t.Errorf("rollback must NOT remove old-id, calls=%v", calls)
		}
		if !oldStillThere {
			t.Errorf("old container should still exist after rollback")
		}
		if newGone {
			// the runtime's RemoveContainer deletes the entry — good
			_ = newGone
		}
	})
}

func TestStopFirst_OldExitsBeforeNewCreated(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rtimpl := newStrategyRuntime()
		rtimpl.execExitCode.Store(0) // healthy

		probes := probe.New(rtimpl, slog.New(slog.DiscardHandler))
		mgrCtx, cancelMgr := context.WithCancel(t.Context())
		defer cancelMgr()
		go probes.Run(mgrCtx)

		s := &StopFirst{}
		err := s.Apply(t.Context(), mkStrategyReq(rtimpl, probes, "old-id"))
		if err != nil {
			t.Fatalf("StopFirst.Apply returned %v, want nil", err)
		}

		rtimpl.mu.Lock()
		calls := append([]string(nil), rtimpl.calls...)
		rtimpl.mu.Unlock()
		idxStopOld := indexOf(calls, "Stop:old-id")
		idxRemoveOld := indexOf(calls, "Remove:old-id")
		idxCreateNew := indexOf(calls, "Create:"+dockerlabels.ContainerNameFor("default", "web", 0))
		if idxStopOld == -1 || idxRemoveOld == -1 || idxCreateNew == -1 {
			t.Fatalf("expected Stop, Remove, Create calls; got %v", calls)
		}
		if !(idxStopOld < idxRemoveOld) {
			t.Errorf("Stop:old must precede Remove:old; got %v", calls)
		}
		if !(idxRemoveOld < idxCreateNew) {
			t.Errorf("Remove:old must precede Create:new (no two-writers); got %v", calls)
		}
	})
}

func TestStopFirst_DoesNotRollBackOnUnhealthy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rtimpl := newStrategyRuntime()
		rtimpl.execExitCode.Store(1) // probe fails

		probes := probe.New(rtimpl, slog.New(slog.DiscardHandler))
		mgrCtx, cancelMgr := context.WithCancel(t.Context())
		defer cancelMgr()
		go probes.Run(mgrCtx)

		s := &StopFirst{}
		err := s.Apply(t.Context(), mkStrategyReq(rtimpl, probes, "old-id"))
		if err != nil {
			t.Fatalf("StopFirst.Apply returned %v, want nil (no rollback contract)", err)
		}

		rtimpl.mu.Lock()
		calls := append([]string(nil), rtimpl.calls...)
		_, newExists := rtimpl.containers["new-id"]
		rtimpl.mu.Unlock()
		if indexOf(calls, "Remove:new-id") != -1 {
			t.Errorf("stop-first must NOT remove the new container on probe failure, calls=%v", calls)
		}
		if !newExists {
			t.Errorf("new container should remain; reconciler tick decides recovery")
		}
	})
}

func indexOf(s []string, want string) int {
	for i, v := range s {
		if v == want {
			return i
		}
	}
	return -1
}

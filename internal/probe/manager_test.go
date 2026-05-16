package probe

import (
	"context"
	"log/slog"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/proxa-server/proxa/internal/runtime"
	"github.com/proxa-server/proxa/pkg/types"
)

// countingRuntime wraps fakeRuntime so tests can drive different exit
// codes over time. Probe count is read with atomic load.
type countingRuntime struct {
	fakeRuntime
	calls atomic.Int64
}

func (c *countingRuntime) Exec(ctx context.Context, id string, cmd []string, opts runtime.ExecOpts) (*runtime.ExecResult, error) {
	c.calls.Add(1)
	return c.fakeRuntime.Exec(ctx, id, cmd, opts)
}

func discardLog() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestManagerInitialProbeMarksHealthy(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := &countingRuntime{fakeRuntime: fakeRuntime{exitCode: 0}}
		m := New(rt, discardLog())

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go m.Run(ctx)

		spec := types.TaskDef{Health: types.HealthCheck{
			Command:  []string{"true"},
			Interval: 5 * time.Second,
			Timeout:  1 * time.Second,
			Retries:  3,
		}}
		if err := m.Track("c1", spec); err != nil {
			t.Fatalf("Track: %v", err)
		}

		synctest.Wait()
		snap, ok := m.Snapshot("c1")
		if !ok {
			t.Fatal("Snapshot returned ok=false after Track")
		}
		if !snap.HealthOK {
			t.Errorf("expected HealthOK=true after first successful probe, got %+v", snap)
		}
		if rt.calls.Load() == 0 {
			t.Errorf("expected at least 1 Exec call, got 0")
		}
	})
}

func TestManagerStreakOnConsecutiveFailures(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := &countingRuntime{fakeRuntime: fakeRuntime{exitCode: 1}}
		m := New(rt, discardLog())

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go m.Run(ctx)

		spec := types.TaskDef{Health: types.HealthCheck{
			Command:  []string{"false"},
			Interval: 5 * time.Second,
			Timeout:  1 * time.Second,
			Retries:  3,
		}}
		_ = m.Track("c2", spec)

		synctest.Wait()
		s, _ := m.Snapshot("c2")
		if s.HealthOK {
			t.Errorf("expected HealthOK=false after first failure")
		}
		if s.Streak != 1 {
			t.Errorf("Streak = %d after 1 failure, want 1", s.Streak)
		}

		// Advance 10s: ticker fires at +5s, +10s — two more failures.
		time.Sleep(10 * time.Second)
		synctest.Wait()
		s2, _ := m.Snapshot("c2")
		if s2.Streak < 3 {
			t.Errorf("Streak = %d after 10s, want >= 3", s2.Streak)
		}
	})
}

func TestManagerUntrackStopsGoroutine(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := &countingRuntime{fakeRuntime: fakeRuntime{exitCode: 0}}
		m := New(rt, discardLog())

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go m.Run(ctx)

		spec := types.TaskDef{Health: types.HealthCheck{
			Command:  []string{"true"},
			Interval: 5 * time.Second,
			Retries:  3,
		}}
		_ = m.Track("c3", spec)
		synctest.Wait()

		callsBefore := rt.calls.Load()
		m.Untrack("c3")

		if _, ok := m.Snapshot("c3"); ok {
			t.Errorf("Snapshot still ok after Untrack")
		}

		// Advance well past the interval; goroutine is gone so no more probes.
		time.Sleep(30 * time.Second)
		synctest.Wait()

		if got := rt.calls.Load(); got != callsBefore {
			t.Errorf("Exec called %d times after Untrack (was %d before)", got, callsBefore)
		}
	})
}

func TestManagerEmptyHealthIsTrustedHealthy(t *testing.T) {
	m := New(&countingRuntime{}, discardLog())
	if err := m.Track("c4", types.TaskDef{}); err != nil {
		t.Fatalf("Track: %v", err)
	}
	s, ok := m.Snapshot("c4")
	if !ok || !s.HealthOK {
		t.Errorf("empty Health should produce HealthOK=true snapshot, got %+v ok=%v", s, ok)
	}
}

func TestManagerTrackIsIdempotent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := &countingRuntime{fakeRuntime: fakeRuntime{exitCode: 0}}
		m := New(rt, discardLog())

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		go m.Run(ctx)

		spec := types.TaskDef{Health: types.HealthCheck{
			Command:  []string{"true"},
			Interval: 5 * time.Second,
		}}
		_ = m.Track("c5", spec)
		synctest.Wait()
		callsAfterFirst := rt.calls.Load()

		// Re-Track with same spec — should be a no-op (no restart).
		_ = m.Track("c5", spec)
		synctest.Wait()

		if got := rt.calls.Load(); got != callsAfterFirst {
			t.Errorf("re-Track with same spec triggered new probe(s): before=%d after=%d", callsAfterFirst, got)
		}
	})
}

func TestManagerRunReturnsAfterCtxCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := New(&countingRuntime{fakeRuntime: fakeRuntime{exitCode: 0}}, discardLog())

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		go func() {
			m.Run(ctx)
			close(done)
		}()

		spec := types.TaskDef{Health: types.HealthCheck{Command: []string{"true"}, Interval: 5 * time.Second}}
		_ = m.Track("c6", spec)
		synctest.Wait()

		cancel()
		synctest.Wait()
		select {
		case <-done:
			// good
		default:
			t.Errorf("Run did not return after ctx cancel")
		}
	})
}

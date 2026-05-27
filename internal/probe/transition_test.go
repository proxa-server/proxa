package probe

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// recordingSink captures every event for assertion.
type recordingSink struct {
	mu  sync.Mutex
	evs []EventRecord
}

func (r *recordingSink) Append(_ context.Context, e EventRecord) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.evs = append(r.evs, e)
	return int64(len(r.evs)), nil
}

func (r *recordingSink) snapshot() []EventRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]EventRecord, len(r.evs))
	copy(out, r.evs)
	return out
}

// TestEmitTransition verifies the emitTransition path directly — it
// doesn't require a full probe loop.
func TestEmitTransition_HealthyToUnhealthy(t *testing.T) {
	sink := &recordingSink{}
	m := NewWithOptions(nil, discardLog(), Options{Events: sink})

	m.emitTransition("abc1234567890ef", true, false, 4)

	evs := sink.snapshot()
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	e := evs[0]
	if e.Type != "probe.transition" {
		t.Errorf("type = %q, want probe.transition", e.Type)
	}
	if e.Actor != "reconciler" {
		t.Errorf("actor = %q, want reconciler", e.Actor)
	}
	if e.Target != "container:abc123456789" {
		t.Errorf("target = %q, want container:abc123456789 (short id)", e.Target)
	}
	if !strings.Contains(e.Payload, `"from":"healthy"`) ||
		!strings.Contains(e.Payload, `"to":"unhealthy"`) ||
		!strings.Contains(e.Payload, `"streak":4`) {
		t.Errorf("payload missing expected fields: %s", e.Payload)
	}
}

func TestEmitTransition_NilSinkIsSilent(t *testing.T) {
	m := NewWithOptions(nil, discardLog(), Options{}) // Events: nil
	// Must not panic. There's nothing to assert beyond that.
	m.emitTransition("abc", false, true, 1)
}

func TestEmitTransition_UnhealthyToHealthy(t *testing.T) {
	sink := &recordingSink{}
	m := NewWithOptions(nil, discardLog(), Options{Events: sink})

	m.emitTransition("xyz", false, true, 1)

	evs := sink.snapshot()
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	if !strings.Contains(evs[0].Payload, `"from":"unhealthy"`) ||
		!strings.Contains(evs[0].Payload, `"to":"healthy"`) {
		t.Errorf("recovery payload wrong: %s", evs[0].Payload)
	}
}

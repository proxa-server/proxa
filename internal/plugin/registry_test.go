package plugin_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/proxa-server/proxa/internal/events"
	"github.com/proxa-server/proxa/internal/plugin"
)

// discardLog returns a silent slog.Logger for tests.
func discardLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// countingHook records every event it sees.
type countingHook struct {
	name    string
	mu      sync.Mutex
	count   atomic.Int64
	events  []events.Event
	onError error
	panicOn string // when event.Type == this, panic
}

func (h *countingHook) Name() string { return h.name }
func (h *countingHook) OnEvent(_ context.Context, e events.Event) error {
	if h.panicOn != "" && e.Type == h.panicOn {
		panic("intentional panic: " + e.Type)
	}
	h.mu.Lock()
	h.events = append(h.events, e)
	h.mu.Unlock()
	h.count.Add(1)
	return h.onError
}
func (h *countingHook) snapshot() int64 { return h.count.Load() }

func TestRegistry_PublishDelivers(t *testing.T) {
	r := plugin.NewRegistry(discardLog())
	defer r.Close()

	h := &countingHook{name: "h1"}
	if err := r.Register(h); err != nil {
		t.Fatal(err)
	}

	r.Publish(events.Event{Type: events.TypeReconcilerCreate, Actor: "reconciler", Target: "service:default/api"})
	r.Publish(events.Event{Type: events.TypeProbeTransition, Actor: "reconciler", Target: "container:abc"})

	// Wait briefly for async delivery.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && h.snapshot() < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := h.snapshot(); got != 2 {
		t.Errorf("hook received %d events, want 2", got)
	}
}

func TestRegistry_RegisterDuplicateNameRejected(t *testing.T) {
	r := plugin.NewRegistry(discardLog())
	defer r.Close()
	_ = r.Register(&countingHook{name: "dup"})
	if err := r.Register(&countingHook{name: "dup"}); !errors.Is(err, plugin.ErrHookAlreadyRegistered) {
		t.Errorf("got %v, want ErrHookAlreadyRegistered", err)
	}
}

func TestRegistry_RegisterAfterCloseRejected(t *testing.T) {
	r := plugin.NewRegistry(discardLog())
	_ = r.Close()
	if err := r.Register(&countingHook{name: "h"}); !errors.Is(err, plugin.ErrRegistryClosed) {
		t.Errorf("got %v, want ErrRegistryClosed", err)
	}
}

func TestRegistry_PanicInHookDoesNotAffectOthers(t *testing.T) {
	r := plugin.NewRegistry(discardLog())
	defer r.Close()

	bad := &countingHook{name: "bad", panicOn: events.TypeReconcilerCreate}
	good := &countingHook{name: "good"}
	if err := r.Register(bad); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(good); err != nil {
		t.Fatal(err)
	}

	// Trigger panic in bad + normal delivery in good.
	r.Publish(events.Event{Type: events.TypeReconcilerCreate, Actor: "reconciler", Target: "service:default/api"})
	r.Publish(events.Event{Type: events.TypeProbeTransition, Actor: "reconciler", Target: "container:abc"})

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && good.snapshot() < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := good.snapshot(); got != 2 {
		t.Errorf("good hook got %d events, want 2 (panic in another hook must not affect this one)", got)
	}
}

func TestRegistry_BackPressureDropsEvents(t *testing.T) {
	r := plugin.NewRegistry(discardLog())
	defer r.Close()

	// A hook whose OnEvent blocks forever — fills the buffered channel
	// and forces back-pressure on subsequent Publishes.
	blockCh := make(chan struct{})
	slow := &slowHook{name: "slow", block: blockCh}
	if err := r.Register(slow); err != nil {
		t.Fatal(err)
	}
	defer close(blockCh) // release on test cleanup

	// Publish 200 events — channel buffer is 64, so beyond that
	// publishes drop without blocking.
	for i := 0; i < 200; i++ {
		r.Publish(events.Event{Type: events.TypeReconcilerCreate, Actor: "x", Target: "t"})
	}
	// Test asserts the Publish loop didn't block. If we got here, it
	// completed in bounded time.
}

type slowHook struct {
	name  string
	block chan struct{}
}

func (h *slowHook) Name() string { return h.name }
func (h *slowHook) OnEvent(context.Context, events.Event) error {
	<-h.block
	return nil
}

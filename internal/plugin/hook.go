// Package plugin defines Proxa's extension contract. Substrate-level
// positioning per project_competitive_landscape: PaaS-layer tools
// (Coolify-on-Proxa, internal Socio.do utility, etc.) hook into
// Proxa's event stream + state without forking the source.
//
// v0.4.3 ships the Hook + Registry interfaces with a buffered-channel
// async delivery + panic recovery. No production Hook implementations
// ship in v0.4.3 — first consumer arrives with v0.7 webhook egress or
// later, demand-driven.
//
// Design contract:
//   - Hook.OnEvent runs asynchronously; the publisher (reconciler /
//     server) is never blocked by a slow Hook.
//   - A Hook panic does NOT crash the publisher — recovered + logged.
//   - Back-pressure (subscriber queue full) drops events for that
//     subscriber only.
package plugin

import (
	"context"
	"log/slog"
	"sync"

	"github.com/proxa-server/proxa/internal/events"
)

// Hook is the extension point. Implementations subscribe to the
// Registry and receive every event the Registry publishes.
type Hook interface {
	// Name returns a human identifier — used in logs + diagnostics
	// when the Hook errors or panics.
	Name() string
	// OnEvent is invoked once per published event. Blocking inside
	// OnEvent is allowed but blocks ONLY this Hook's delivery queue,
	// not the publisher or other Hooks.
	OnEvent(ctx context.Context, e events.Event) error
}

// Registry dispatches events to registered Hooks.
type Registry interface {
	// Register adds a Hook. Returns an error if a Hook with the same
	// Name is already registered.
	Register(Hook) error
	// Publish delivers e to every registered Hook. Non-blocking;
	// returns immediately after the event is queued.
	Publish(events.Event)
	// Close stops all delivery goroutines and waits for in-flight
	// Hooks to finish (best-effort, bounded).
	Close() error
}

// NewRegistry returns a Registry backed by per-Hook buffered channels.
// log defaults to slog.Default() if nil.
func NewRegistry(log *slog.Logger) Registry {
	if log == nil {
		log = slog.Default()
	}
	return &registry{
		log:   log,
		hooks: map[string]*hookSub{},
	}
}

// --- impl ---------------------------------------------------------------

type hookSub struct {
	hook Hook
	ch   chan events.Event
	done chan struct{}
}

type registry struct {
	log *slog.Logger

	mu    sync.RWMutex
	hooks map[string]*hookSub

	closed bool
}

func (r *registry) Register(h Hook) error {
	name := h.Name()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return ErrRegistryClosed
	}
	if _, ok := r.hooks[name]; ok {
		return ErrHookAlreadyRegistered
	}
	sub := &hookSub{
		hook: h,
		ch:   make(chan events.Event, 64),
		done: make(chan struct{}),
	}
	r.hooks[name] = sub
	go r.deliver(sub)
	return nil
}

func (r *registry) Publish(e events.Event) {
	r.mu.RLock()
	subs := make([]*hookSub, 0, len(r.hooks))
	for _, s := range r.hooks {
		subs = append(subs, s)
	}
	r.mu.RUnlock()

	for _, s := range subs {
		select {
		case s.ch <- e:
		default:
			r.log.Warn("plugin: hook dropped event (back-pressure)",
				"hook", s.hook.Name(), "event_type", e.Type)
		}
	}
}

func (r *registry) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	subs := r.hooks
	r.hooks = nil
	r.mu.Unlock()

	for _, s := range subs {
		close(s.ch)
		<-s.done
	}
	return nil
}

// deliver is the per-Hook dispatch goroutine.
func (r *registry) deliver(s *hookSub) {
	defer close(s.done)
	for e := range s.ch {
		r.callHook(s.hook, e)
	}
}

func (r *registry) callHook(h Hook, e events.Event) {
	defer func() {
		if rec := recover(); rec != nil {
			r.log.Error("plugin: hook panicked",
				"hook", h.Name(), "event_type", e.Type, "recover", rec)
		}
	}()
	ctx := context.Background()
	if err := h.OnEvent(ctx, e); err != nil {
		r.log.Warn("plugin: hook returned error",
			"hook", h.Name(), "event_type", e.Type, "err", err)
	}
}

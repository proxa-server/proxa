package events

import (
	"context"
	"sync"
)

// Bus is an in-process pub/sub for Event consumers. Subscribers
// receive events asynchronously via a buffered channel; back-pressure
// (slow subscriber) results in dropped events for that subscriber only
// — the publisher path is never blocked.
//
// Used by the dashboard Events panel (subscribes for live updates),
// the future v0.7 webhook egress (subscribes + POSTs), and the
// internal/plugin Registry (subscribes + dispatches to Hooks).
type Bus struct {
	mu          sync.RWMutex
	subscribers map[chan Event]struct{}
}

// NewBus returns an empty Bus.
func NewBus() *Bus {
	return &Bus{subscribers: map[chan Event]struct{}{}}
}

// Subscribe returns a channel that receives every Publish'd Event
// until ctx is canceled. Buffer size 64 — slow subscribers drop events
// rather than block the publisher.
func (b *Bus) Subscribe(ctx context.Context) <-chan Event {
	ch := make(chan Event, 64)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subscribers, ch)
		close(ch)
		b.mu.Unlock()
	}()

	return ch
}

// Publish delivers e to every subscriber. Non-blocking — if a
// subscriber's channel is full, the event is dropped for that
// subscriber. Returns the number of subscribers that received the event.
func (b *Bus) Publish(e Event) int {
	b.mu.RLock()
	subs := make([]chan Event, 0, len(b.subscribers))
	for ch := range b.subscribers {
		subs = append(subs, ch)
	}
	b.mu.RUnlock()

	delivered := 0
	for _, ch := range subs {
		select {
		case ch <- e:
			delivered++
		default:
			// Subscriber is slow; drop and move on. Logged at the
			// publishing site if needed.
		}
	}
	return delivered
}

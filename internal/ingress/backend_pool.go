package ingress

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
)

// BackendPool is the ingress-side view of a service's reachable replicas.
// Updated by the reconciler each tick via UpdateBackends; read by request
// handlers via Pick. Lock-free on the hot path for round-robin (atomic
// cursor); random uses math/rand/v2's lock-free PCG.
//
// The pool is concurrency-safe — Pick may be called from any goroutine.
type BackendPool struct {
	mu       sync.RWMutex
	backends []Backend
	cursor   atomic.Uint64 // round-robin index
}

// NewBackendPool returns an empty pool.
func NewBackendPool() *BackendPool { return &BackendPool{} }

// Replace swaps the backend slice atomically. Callers should pass a
// fresh slice; the pool does NOT defensive-copy.
func (p *BackendPool) Replace(backends []Backend) {
	p.mu.Lock()
	p.backends = backends
	p.mu.Unlock()
}

// Healthy returns a copy of the currently healthy backends. Empty when
// the pool is empty or all backends are unhealthy.
func (p *BackendPool) Healthy() []Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Backend, 0, len(p.backends))
	for _, b := range p.backends {
		if b.Healthy {
			out = append(out, b)
		}
	}
	return out
}

// Pick returns one healthy backend per the LB strategy, or nil when no
// healthy backend exists (caller is expected to return 503).
//
// Strategy values:
//   - ""           → "random" (default)
//   - "random"     → uniform pick across healthy backends
//   - "round-robin" → strict rotation; an atomic cursor advances per pick
//
// Returns a COPY of the Backend, never a pointer into the pool — callers
// must not mutate the returned value expecting it to affect pool state.
func (p *BackendPool) Pick(strategy string) *Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Gather healthy indices into a small scratch slice. For typical pool
	// sizes (≤ 10 replicas per service) this is faster than maintaining a
	// parallel "healthy" index list.
	healthy := make([]int, 0, len(p.backends))
	for i, b := range p.backends {
		if b.Healthy {
			healthy = append(healthy, i)
		}
	}
	if len(healthy) == 0 {
		return nil
	}

	switch strategy {
	case "round-robin":
		// cursor advances exactly once per pick; modulo by healthy count
		// so the rotation visits each healthy backend in order.
		n := p.cursor.Add(1) - 1
		b := p.backends[healthy[n%uint64(len(healthy))]]
		return &b
	default: // "" or "random"
		idx := healthy[rand.IntN(len(healthy))]
		b := p.backends[idx]
		return &b
	}
}

// Size returns the total backend count (healthy + unhealthy).
func (p *BackendPool) Size() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.backends)
}

// HealthyCount returns the number of currently-healthy backends.
func (p *BackendPool) HealthyCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	n := 0
	for _, b := range p.backends {
		if b.Healthy {
			n++
		}
	}
	return n
}

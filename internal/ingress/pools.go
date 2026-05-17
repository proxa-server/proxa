package ingress

import "sync"

// poolRegistry holds one BackendPool per service. Concurrency-safe; the
// registry's RWMutex protects the map only, not the pools themselves
// (each BackendPool has its own RWMutex).
type poolRegistry struct {
	mu    sync.RWMutex
	pools map[ServiceID]*BackendPool
}

func newPoolRegistry() *poolRegistry {
	return &poolRegistry{pools: make(map[ServiceID]*BackendPool)}
}

// Get returns the pool for svc, or nil when no pool exists yet.
func (r *poolRegistry) Get(svc ServiceID) *BackendPool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pools[svc]
}

// Upsert replaces the backend list for svc. If the pool didn't exist,
// it is created.
func (r *poolRegistry) Upsert(svc ServiceID, backends []Backend) {
	r.mu.Lock()
	pool, ok := r.pools[svc]
	if !ok {
		pool = NewBackendPool()
		r.pools[svc] = pool
	}
	r.mu.Unlock()
	pool.Replace(backends)
}

// Forget removes the pool for svc.
func (r *poolRegistry) Forget(svc ServiceID) {
	r.mu.Lock()
	delete(r.pools, svc)
	r.mu.Unlock()
}

// Count returns the number of pools (= services with backends).
func (r *poolRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.pools)
}

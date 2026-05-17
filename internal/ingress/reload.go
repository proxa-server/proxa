package ingress

import "sync/atomic"

// RouterPtr is a goroutine-safe holder for the current *Router. Readers
// take a snapshot via Load (lock-free, single atomic load); writers
// publish a new snapshot via Swap.
//
// This is the core of FR-009 + SC-003 (hot-reload with ZERO 5xx):
// in-flight request handlers read the pointer ONCE at request entry and
// use that snapshot for the entire request — even if UpdateRoutes
// publishes a new table mid-request. The old *Router stays alive until
// every closure pinning it returns; then GC reclaims it.
//
// L4 connections similarly hold a reference to the backend they were
// initially routed to; reloads don't mid-stream-rebalance an established
// TCP connection.
//
// See specs/003-ingress/research.md R-003 for the design rationale.
type RouterPtr struct {
	v atomic.Pointer[Router]
}

// NewRouterPtr returns a pointer initialized with an empty router. The
// empty router rejects every lookup (LookupL7 / LookupL4 → false), so
// the ingress can safely accept connections before the first
// UpdateRoutes call without panicking.
func NewRouterPtr() *RouterPtr {
	p := &RouterPtr{}
	empty, _ := BuildRouter(nil)
	p.v.Store(empty)
	return p
}

// Load returns the currently-published router snapshot. Lock-free.
// The returned pointer is safe to hold for the lifetime of a request.
func (p *RouterPtr) Load() *Router { return p.v.Load() }

// Swap publishes a new router snapshot atomically and returns the
// previous one. The caller may continue to hold the previous pointer
// (e.g., for graceful drain), but typically discards it.
func (p *RouterPtr) Swap(next *Router) *Router { return p.v.Swap(next) }

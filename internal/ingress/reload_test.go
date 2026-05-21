package ingress

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/proxa-server/proxa/pkg/types"
)

func TestRouterPtrInitialLoadIsEmptyRouter(t *testing.T) {
	p := NewRouterPtr()
	r := p.Load()
	if r == nil {
		t.Fatal("Load() returned nil; should be empty router")
	}
	if _, _, ok := r.LookupL7("anything", "/"); ok {
		t.Errorf("empty router should miss")
	}
}

func TestRouterPtrSwapAndLoad(t *testing.T) {
	p := NewRouterPtr()
	r1, _ := BuildRouter(map[ServiceID][]types.Route{
		{Project: "default", Service: "a"}: {{Host: "a.example.com"}},
	})
	prev := p.Swap(r1)
	if prev == nil {
		t.Errorf("Swap should return prior pointer, got nil")
	}
	if p.Load() != r1 {
		t.Errorf("Load after Swap should return r1")
	}

	r2, _ := BuildRouter(map[ServiceID][]types.Route{
		{Project: "default", Service: "b"}: {{Host: "b.example.com"}},
	})
	prev2 := p.Swap(r2)
	if prev2 != r1 {
		t.Errorf("Swap should return r1; got %p (r1=%p)", prev2, r1)
	}
}

// TestRouterPtrConcurrentReadersAndSwaps is the SC-003 unit-level
// gate: while 100 goroutines do Load() in a loop, a writer Swaps 50
// new routers. Every Load MUST return a non-nil, fully-built router —
// any read of a partially-published value would be a torn pointer.
func TestRouterPtrConcurrentReadersAndSwaps(t *testing.T) {
	p := NewRouterPtr()
	var reads atomic.Int64
	var stop atomic.Bool

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			for !stop.Load() {
				r := p.Load()
				if r == nil {
					t.Errorf("Load returned nil")
					return
				}
				// Exercise the snapshot so the race detector sees the
				// read of internal fields.
				_, _, _ = r.LookupL7("x", "/")
				reads.Add(1)
			}
		})
	}

	for range 50 {
		fresh, err := BuildRouter(map[ServiceID][]types.Route{
			{Project: "default", Service: "a"}: {{Host: "a.example.com"}},
		})
		if err != nil {
			t.Fatalf("BuildRouter: %v", err)
		}
		p.Swap(fresh)
	}
	stop.Store(true)
	wg.Wait()

	if reads.Load() == 0 {
		t.Errorf("readers never observed a Load — concurrency setup broken")
	}
}

package ingress

import (
	"sync"
	"sync/atomic"
	"testing"
)

func mkBackends(spec ...bool) []Backend {
	out := make([]Backend, 0, len(spec))
	for i, healthy := range spec {
		out = append(out, Backend{
			ContainerID: string(rune('a' + i)),
			IPAddress:   "10.0.0." + string(rune('1'+i)),
			Port:        80,
			Healthy:     healthy,
		})
	}
	return out
}

func TestBackendPoolPickEmpty(t *testing.T) {
	p := NewBackendPool()
	if got := p.Pick("random"); got != nil {
		t.Errorf("Pick on empty pool = %+v, want nil", got)
	}
}

func TestBackendPoolPickAllUnhealthy(t *testing.T) {
	p := NewBackendPool()
	p.Replace(mkBackends(false, false, false))
	if got := p.Pick(""); got != nil {
		t.Errorf("Pick on all-unhealthy pool = %+v, want nil", got)
	}
}

func TestBackendPoolPickRandomHitsHealthyOnly(t *testing.T) {
	p := NewBackendPool()
	p.Replace(mkBackends(true, false, true)) // a, b unhealthy, c healthy → a, c only
	seen := map[string]bool{}
	for i := range 200 {
		b := p.Pick("random")
		if b == nil {
			t.Fatalf("Pick returned nil at iteration %d", i)
		}
		if !b.Healthy {
			t.Errorf("Pick returned unhealthy backend: %+v", b)
		}
		seen[b.ContainerID] = true
	}
	if !seen["a"] || !seen["c"] || seen["b"] {
		t.Errorf("expected to see only a + c, got: %v", seen)
	}
}

func TestBackendPoolRoundRobinExactDistribution(t *testing.T) {
	p := NewBackendPool()
	p.Replace(mkBackends(true, true, true))
	counts := map[string]int{}
	for i := range 9 {
		b := p.Pick("round-robin")
		if b == nil {
			t.Fatalf("Pick returned nil at %d", i)
		}
		counts[b.ContainerID]++
	}
	for _, id := range []string{"a", "b", "c"} {
		if counts[id] != 3 {
			t.Errorf("round-robin distribution off: %v (want each = 3)", counts)
			break
		}
	}
}

func TestBackendPoolRoundRobinSkipsUnhealthy(t *testing.T) {
	p := NewBackendPool()
	p.Replace(mkBackends(true, false, true)) // a + c healthy
	for i := range 20 {
		b := p.Pick("round-robin")
		if b == nil {
			t.Fatalf("Pick returned nil at %d", i)
		}
		if b.ContainerID == "b" {
			t.Errorf("round-robin returned unhealthy backend b")
		}
	}
}

func TestBackendPoolConcurrentPick(t *testing.T) {
	// Race test: 100 goroutines × 100 picks each on a healthy pool. Must
	// never panic, deadlock, or return nil.
	p := NewBackendPool()
	p.Replace(mkBackends(true, true, true))

	var wg sync.WaitGroup
	var nils atomic.Int64
	for range 100 {
		wg.Go(func() {
			for range 100 {
				if p.Pick("round-robin") == nil {
					nils.Add(1)
				}
			}
		})
	}
	wg.Wait()
	if nils.Load() != 0 {
		t.Errorf("%d nil picks under concurrency; pool was fully healthy", nils.Load())
	}
}

func TestBackendPoolReplaceDuringPick(t *testing.T) {
	// Replace concurrently with Pick; must not crash. We don't assert
	// which view each Pick sees — only that no panic/data race.
	p := NewBackendPool()
	p.Replace(mkBackends(true, true))

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 10 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					_ = p.Pick("random")
				}
			}
		})
	}
	for range 50 {
		p.Replace(mkBackends(true, false, true, true))
	}
	close(stop)
	wg.Wait()
}

func TestBackendPoolCounts(t *testing.T) {
	p := NewBackendPool()
	p.Replace(mkBackends(true, false, true))
	if p.Size() != 3 {
		t.Errorf("Size=%d, want 3", p.Size())
	}
	if p.HealthyCount() != 2 {
		t.Errorf("HealthyCount=%d, want 2", p.HealthyCount())
	}
	if len(p.Healthy()) != 2 {
		t.Errorf("Healthy() len=%d, want 2", len(p.Healthy()))
	}
}

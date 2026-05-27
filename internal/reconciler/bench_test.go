package reconciler

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/proxa-server/proxa/pkg/types"
)

// BenchmarkReconciler_TickThroughput measures Poke-driven cycle
// throughput with N services under management. Uses the existing
// fakeStore + fakeRuntime helpers in reconciler_test.go (package-local,
// so this bench file lives next to them).
//
// Reports "ticks/sec" + "services/sec" (= ticks/sec × N).
func BenchmarkReconciler_TickThroughput(b *testing.B) {
	const numServices = 50

	st := &fakeStore{services: map[string][]types.Service{
		"default": makeBenchServices(numServices),
	}}
	rt := &fakeRuntime{}

	r := New(st, rt, Options{
		TickInterval: time.Hour, // never auto-ticks; we Poke
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx)

	// Warmup so first measured iteration doesn't pay init cost.
	r.Poke()
	time.Sleep(20 * time.Millisecond)

	b.ResetTimer()
	start := time.Now()
	for i := 0; i < b.N; i++ {
		r.Poke()
	}
	// Drain — reconciler coalesces pokes; brief sleep is coarse but
	// adequate for relative throughput tracking across releases.
	time.Sleep(50 * time.Millisecond)
	elapsed := time.Since(start)

	ticksPerSec := float64(b.N) / elapsed.Seconds()
	b.ReportMetric(ticksPerSec*float64(numServices), "services/sec")
	b.ReportMetric(ticksPerSec, "ticks/sec")
}

// makeBenchServices builds N synthetic services for the bench's fakeStore.
func makeBenchServices(n int) []types.Service {
	out := make([]types.Service, n)
	for i := 0; i < n; i++ {
		name := "svc-" + strconv.Itoa(i)
		out[i] = types.Service{
			Name:    name,
			Project: "default",
			Spec: types.TaskDef{
				Name:     name,
				Image:    "nginxinc/nginx-unprivileged:alpine-slim",
				Replicas: 1,
			},
		}
	}
	return out
}

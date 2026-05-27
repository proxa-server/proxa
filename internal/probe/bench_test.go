package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/proxa-server/proxa/pkg/types"
)

// BenchmarkProbe_WaveCapacity measures how many container probes the
// probe.Manager can sustain concurrently per cycle. We spin up an
// httptest.Server that returns 200 instantly, configure the Manager
// in "via=ingress" mode pointing at that server, and Track N synthetic
// containers — each one's HTTP probe loops against the httptest URL.
//
// Reports "containers/cycle" as N — the manager-capacity number the
// operator cares about for sizing a cluster. Cross-release regression
// shows up when N at fixed CPU drops.
//
// Reuses the existing fakeRuntime from exec_test.go in this package
// (no new type needed).
func BenchmarkProbe_WaveCapacity(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Extract the port from httptest.Server URL — Manager's
	// IngressHTTPPort field is an int.
	u, _ := url.Parse(srv.URL)
	srvPort, _ := strconv.Atoi(u.Port())

	const numContainers = 100

	m := NewWithOptions(fakeRuntime{}, nil, Options{
		IngressHTTPPort: srvPort, // probe via "ingress" hits the httptest URL
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	// Track N "containers" each with a fast probe interval and a route
	// so the via=ingress path activates.
	for i := 0; i < numContainers; i++ {
		spec := types.TaskDef{
			Name: "svc-" + strconv.Itoa(i),
			Routes: []types.Route{{Host: "bench.local"}},
			Health: types.HealthCheck{
				Path:     "/",
				Port:     80,
				Via:      "ingress",
				Interval: 100 * time.Millisecond,
				Timeout:  50 * time.Millisecond,
				Retries:  1,
			},
		}
		if err := m.Track("ctr-"+strconv.Itoa(i), spec); err != nil {
			b.Fatalf("Track ctr-%d: %v", i, err)
		}
	}

	// Settle: let one probe cycle complete before measuring.
	time.Sleep(150 * time.Millisecond)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Keep the loop alive long enough for probe goroutines to keep
		// firing. The metric below captures sustained capacity, not
		// per-op latency.
		time.Sleep(10 * time.Microsecond)
	}
	b.StopTimer()

	b.ReportMetric(float64(numContainers), "containers/cycle")
}

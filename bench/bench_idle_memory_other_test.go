//go:build !linux

package bench_test

import (
	"runtime"
	"runtime/debug"
	"testing"
	"time"
)

// BenchmarkIdleMemoryRSS_Other is the portable fallback for non-Linux
// hosts (macOS, Windows, BSD). VmRSS via /proc is Linux-only and macOS
// process memory accounting is sufficiently different (virtual +
// dirty + compressed) that comparing the two would be misleading.
//
// Instead we measure THIS Go test process's heap + stack footprint as
// a coarse proxy for what the proxa-server process would consume —
// they share the Go runtime + stdlib, so heap+stack growth tracks
// reasonably well across releases for relative regression detection.
//
// Reports "MB-rss" with the same metric name as the Linux variant so
// `make bench` aggregates a consistent column. Operators should NOT
// compare Linux numbers vs macOS numbers directly — only same-platform
// trends are meaningful.
func BenchmarkIdleMemoryRSS_Other(b *testing.B) {
	// Force a GC + brief settle so the measurement is steady-state.
	runtime.GC()
	debug.FreeOSMemory()
	time.Sleep(50 * time.Millisecond)

	for i := 0; i < b.N; i++ {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		// Sys is "total bytes obtained from OS"; closer to RSS-ish than
		// HeapAlloc alone, includes runtime metadata + stacks.
		mb := float64(stats.Sys) / (1024 * 1024)
		b.ReportMetric(mb, "MB-rss")
	}
}

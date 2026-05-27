package bench_test

import (
	"fmt"
	"io"
	"testing"
)

// BenchmarkSSE_Throughput measures the SSE encoder hot path: writing
// `data: <line>\n\n` + flush. The production implementation lives at
// internal/server/sse.writeSSEData but is package-private; we mirror
// the 3-line function here. Same fmt.Fprintf + flush cost characteristic.
//
// Reports "lines/sec" — the per-connection ceiling for log streaming.
// Real-world workloads see lower numbers because they share with other
// goroutines + network egress, but this baseline detects regressions
// in fmt.Fprintf / io.Writer overhead between Go releases.
func BenchmarkSSE_Throughput(b *testing.B) {
	const line = "2026-05-26T13:00:00Z stdout 192.168.1.1 - - \"GET /api/v1/health HTTP/1.1\" 200 124 \"-\" \"curl/7.84\""

	w := io.Discard

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writeSSEDataMirror(w, line)
	}
	b.StopTimer()

	linesPerSec := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(linesPerSec, "lines/sec")
}

// writeSSEDataMirror replicates the production internal/server/sse.writeSSEData
// 3-liner so the bench measures equivalent work without crossing the
// internal package boundary. If the production function gains new
// behavior, update this mirror alongside it.
func writeSSEDataMirror(w io.Writer, line string) {
	_, _ = fmt.Fprintf(w, "data: %s\n", line)
	_, _ = fmt.Fprint(w, "\n")
}

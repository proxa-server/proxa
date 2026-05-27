// Package bench holds Proxa's performance benchmark suite. It is NOT
// runtime code — the proxa and proxa-agent binaries do not link this
// package.
//
// The suite establishes baseline numbers for every hot path that
// future releases might inadvertently regress. Run via:
//
//	make bench
//
// (which expands to `go test -bench=. -benchmem -run=^$ -count=3 ./bench/...`).
//
// Categories (one benchmark file per category):
//
//   - bench_reconciler_test.go — reconciler tick throughput (services/sec)
//   - bench_ingress_test.go    — L7 ingress request latency (µs/req-p50, µs/req-p99)
//   - bench_probe_test.go      — probe.Manager wave capacity (containers/cycle)
//   - bench_l4_test.go         — L4 proxy throughput (MB/sec)
//   - bench_sse_test.go        — SSE encoder throughput (lines/sec)
//   - bench_idle_memory_test.go — idle RSS of `proxa server` (MB-rss)
//
// The companion file bench/binary-size-baseline.txt records the
// reference binary size (used by `make build-check` for ±2% drift
// detection). Update it in the SAME commit that intentionally changes
// the binary size; otherwise CI flags the drift.
package bench

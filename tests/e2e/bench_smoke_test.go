//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

// TestSC_004_BenchSuiteEmitsAllMetrics validates SC-004 / FR-001: a
// contributor runs `make bench` on a clean checkout and the suite
// emits all six named metrics. Smoke-level — we only check that the
// suite exits 0 within a reasonable budget and that every expected
// metric name appears in the output. Actual numeric values are
// platform-specific and not asserted here.
//
// Skipped on hosts where `make` is unavailable (rare).
func TestSC_004_BenchSuiteEmitsAllMetrics(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-004")

	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make not on PATH")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not on PATH")
	}

	repoRoot, err := harness.FindRepoRoot()
	if err != nil {
		t.Fatalf("FindRepoRoot: %v", err)
	}

	// 5 min budget — bench targets six categories with `-count=3`.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Use -benchtime=10x to keep the smoke run quick (full bench in real
	// release validation uses the default 1s/op).
	cmd := exec.CommandContext(ctx, "go", "test",
		"-bench=.", "-benchmem", "-run=^$", "-count=1", "-benchtime=10x",
		"./bench/...",
		"./internal/reconciler/...",
		"./internal/probe/...",
		"./internal/ingress/...",
	)
	cmd.Dir = repoRoot
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("bench run failed: %v\noutput:\n%s", err, out.String())
	}

	output := out.String()
	expectedMetrics := []string{
		"services/sec",
		"µs/req-p50",
		"containers/cycle",
		"MB/sec",
		"lines/sec",
		"MB-rss",
	}
	for _, m := range expectedMetrics {
		if !strings.Contains(output, m) {
			t.Errorf("bench output missing metric %q\n--- last 400 chars of output ---\n%s",
				m, tailString(output, 400))
		}
	}
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

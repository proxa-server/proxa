//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

// TestSC_005_LogsReplicaPick covers US5 / SC-005: --replica N streams
// logs from the chosen replica only. Out-of-range returns a clear error.
//
// Skipped on Docker Desktop (macOS): multi-replica services with a
// shared host port can't all bind. On a Linux server with bridge-
// routable hosts the test runs normally.
func TestSC_005_LogsReplicaPick(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-005")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
	harness.SkipIfHTTPProbeUnreachable(t)

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "logreplica.toml")
	// Bridge IPs only — multi-replica + shared host port = collision.
	tomlContent := `
name     = "logreplica"
image    = "nginxinc/nginx-unprivileged:alpine-slim"
replicas = 3

[security]
user = "101:101"

[[expose]]
container = 8080
host      = 0
protocol  = "http"
`
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		for i := 0; i < 3; i++ {
			_ = exec.Command("docker", "rm", "-f", fmt.Sprintf("proxa-default-logreplica-%d", i)).Run()
		}
	})
	harness.WaitForCount(t, "logreplica", 3, 45*time.Second)

	// Each replica has logged the nginx startup line; --replica 1 should
	// only return logs from replica 1, evidenced by the meta header on
	// stderr (=== /proxa-default-logreplica-1 (replica 1) ===).
	out, err := harness.RunProxa(t, dir, "logs", "logreplica", "--replica", "1", "--tail", "20")
	if err != nil {
		t.Fatalf("logs --replica 1: %v\n%s", err, out)
	}
	if !strings.Contains(out, "logreplica-1") {
		t.Errorf("expected meta header mentioning logreplica-1; got:\n%s", out)
	}
	if strings.Contains(out, "logreplica-0") || strings.Contains(out, "logreplica-2") {
		t.Errorf("output mentions other replicas; expected only -1:\n%s", out)
	}

	// Out-of-range replica → non-zero exit + clear message.
	out, err = harness.RunProxa(t, dir, "logs", "logreplica", "--replica", "99")
	if err == nil {
		t.Errorf("expected non-zero exit for --replica 99; got success:\n%s", out)
	}
	if !strings.Contains(out, "replica 99 not found") {
		t.Errorf("expected 'replica 99 not found' in error; got:\n%s", out)
	}
}

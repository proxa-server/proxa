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

// TestSC_001_LogsTail covers SC-001: deploy a service, generate log
// lines, run `proxa logs <svc> --tail N` and assert ≤ N lines on
// stdout with exit 0.
func TestSC_001_LogsTail(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-001")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := harness.StartServer(t, dir)
	defer stop()

	hostPort, _ := harness.PickTwoFreeTCPPorts(t)
	tomlPath := filepath.Join(dir, "logtail.toml")
	tomlContent := fmt.Sprintf(`
name     = "logtail"
image    = "traefik/whoami:latest"
replicas = 1

[[expose]]
container = 80
host      = %d
protocol  = "http"
`, hostPort)
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-logtail-0").Run()
	})

	harness.WaitForCount(t, "logtail", 1, 30*time.Second)

	// Generate ~10 log lines by curling whoami.
	for i := 0; i < 10; i++ {
		_ = exec.Command("curl", "-sf", fmt.Sprintf("http://127.0.0.1:%d/", hostPort)).Run()
	}
	time.Sleep(1 * time.Second) // let logs flush

	out, err := harness.RunProxa(t, dir, "logs", "logtail", "--tail", "5")
	if err != nil {
		t.Fatalf("logs --tail 5: %v\n%s", err, out)
	}
	lines := nonEmptyLines(out)
	if len(lines) > 5 {
		t.Errorf("got %d lines, want ≤ 5\n--- output ---\n%s", len(lines), out)
	}
	if len(lines) == 0 {
		t.Errorf("got 0 lines; expected some recent whoami output\n--- output ---\n%s", out)
	}

	// Unknown service → non-zero exit + clear error.
	out, err = harness.RunProxa(t, dir, "logs", "nosuch")
	if err == nil {
		t.Errorf("expected non-zero exit for missing service; got success\nout=%s", out)
	}
	if !strings.Contains(out, "not found") {
		t.Errorf("missing-service error should contain 'not found', got: %s", out)
	}
}

func nonEmptyLines(s string) []string {
	out := []string{}
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

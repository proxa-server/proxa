//go:build e2e
// +build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

// TestSC_002_1_HTTPProbeMarksHealthy deploys a single-replica whoami
// service with an HTTP health probe and verifies that proxa ps reports
// status="healthy" after a few reconciler ticks. whoami returns 200 on
// any path so /health is a viable probe target without a custom handler.
func TestSC_002_1_HTTPProbeMarksHealthy(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-002.1")
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

	tomlPath := filepath.Join(dir, "health.toml")
	tomlContent := `
name     = "healthsvc"
image    = "traefik/whoami:latest"
replicas = 1

[[expose]]
container = 80
host      = 0
protocol  = "http"

[health]
path     = "/health"
port     = 80
interval = "2s"
timeout  = "1s"
retries  = 3
`
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}

	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-healthsvc-0").Run()
	})

	harness.WaitForCount(t, "healthsvc", 1, 20*time.Second)

	// Give the probe loop time to run at least 2-3 cycles and the
	// reconciler one more tick to fold that into svc.Status.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		out, err := harness.RunProxa(t, dir, "ps", "-j")
		if err != nil {
			t.Fatalf("ps -j: %v\n%s", err, out)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(out), &doc); err != nil {
			t.Fatalf("parse ps json: %v\n%s", err, out)
		}
		status := findServiceStatus(doc, "healthsvc")
		if status == "healthy" {
			return // SC-002-1 satisfied
		}
		time.Sleep(2 * time.Second)
	}

	out, _ := harness.RunProxa(t, dir, "ps", "-j")
	t.Fatalf("healthsvc never reached healthy within 30s; last ps -j output:\n%s", out)
}

// findServiceStatus walks the SystemStatus JSON tree for the named service.
func findServiceStatus(doc map[string]any, service string) string {
	projects, _ := doc["projects"].([]any)
	for _, p := range projects {
		proj, _ := p.(map[string]any)
		svcs, _ := proj["services"].([]any)
		for _, s := range svcs {
			svc, _ := s.(map[string]any)
			name, _ := svc["name"].(string)
			if name != service {
				continue
			}
			if status, ok := svc["status"].(string); ok {
				return strings.TrimSpace(status)
			}
		}
	}
	return ""
}

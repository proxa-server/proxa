//go:build e2e
// +build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// TestSC_002_5_StatelessUpgrade_NoFailedStatus verifies the start-first
// rollover keeps the service from ever entering "failed" status during
// an image upgrade.
//
// Note: true curl-based zero-downtime requires ingress routing (Feature
// 003) so the new container can come up on a different host port. For
// this test we use host=0 (ingress-only port semantics from v0.1.0) and
// poll the service status via `proxa ps -j` to verify no failed window.
func TestSC_002_5_StatelessUpgrade_NoFailedStatus(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "upgrade.toml")
	tomlV1 := `
name     = "upgrade"
image    = "traefik/whoami:v1.10.0"
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
retries  = 2

strategy = "start-first"
`
	if err := os.WriteFile(tomlPath, []byte(tomlV1), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up v1: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-upgrade-0").Run()
	})
	waitForCount(t, "upgrade", 1, 30*time.Second)
	if !waitForServiceStatus(t, dir, "upgrade", "healthy", 30*time.Second) {
		t.Fatalf("upgrade service never reached healthy on v1")
	}

	// Background sampler: poll status every 200ms; flag if it ever says "failed".
	var sawFailed atomic.Bool
	stopSampler := make(chan struct{})
	doneSampler := make(chan struct{})
	go func() {
		defer close(doneSampler)
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopSampler:
				return
			case <-ticker.C:
				out, err := runProxa(t, dir, "ps", "-j")
				if err != nil {
					continue
				}
				var doc map[string]any
				if json.Unmarshal([]byte(out), &doc) != nil {
					continue
				}
				if findServiceStatus(doc, "upgrade") == "failed" {
					sawFailed.Store(true)
				}
			}
		}
	}()

	// Flip to v1.11.0 and re-up.
	tomlV2 := `
name     = "upgrade"
image    = "traefik/whoami:v1.11.0"
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
retries  = 2

strategy = "start-first"
`
	if err := os.WriteFile(tomlPath, []byte(tomlV2), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up v2: %v\n%s", err, out)
	}

	// Allow rollover to complete (interval × retries + tick + grace).
	time.Sleep(20 * time.Second)
	close(stopSampler)
	<-doneSampler

	if sawFailed.Load() {
		t.Errorf("service status reached 'failed' at least once during start-first rollover (SC-002-5)")
	}
}

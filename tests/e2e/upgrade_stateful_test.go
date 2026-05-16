//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSC_002_6_StatefulStopFirst_NoConcurrentWriters verifies that a
// stateful service rollover never has two "Up" containers for the same
// replica index simultaneously. We use redis (single-process,
// fast-boot) as a stand-in for any stateful workload — postgres would
// take minutes to converge and exceed test budgets.
func TestSC_002_6_StatefulStopFirst_NoConcurrentWriters(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "redis.toml")
	tomlV1 := `
name     = "redis"
image    = "redis:7-alpine"
replicas = 1
stateful = true
strategy = "stop-first"

[health]
command  = ["redis-cli", "ping"]
interval = "2s"
timeout  = "1s"
retries  = 3
`
	if err := os.WriteFile(tomlPath, []byte(tomlV1), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up v1: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-redis-0").Run()
	})

	waitForCount(t, "redis", 1, 30*time.Second)

	// Sampler: every 200ms count "Up" containers matching the replica-0 name.
	var maxConcurrent int
	var samplerMu sync.Mutex
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
				out, err := exec.Command("docker", "ps",
					"--filter", "label=proxa.managed=true",
					"--filter", "label=proxa.service=redis",
					"--filter", "label=proxa.replica=0",
					"--format", "{{.Status}}").Output()
				if err != nil {
					continue
				}
				up := 0
				for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
					if strings.HasPrefix(line, "Up") {
						up++
					}
				}
				samplerMu.Lock()
				if up > maxConcurrent {
					maxConcurrent = up
				}
				samplerMu.Unlock()
			}
		}
	}()

	// Flip image and trigger rollover.
	tomlV2 := `
name     = "redis"
image    = "redis:7.2-alpine"
replicas = 1
stateful = true
strategy = "stop-first"

[health]
command  = ["redis-cli", "ping"]
interval = "2s"
timeout  = "1s"
retries  = 3
`
	if err := os.WriteFile(tomlPath, []byte(tomlV2), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up v2: %v\n%s", err, out)
	}

	// Allow stop-first to complete: 30s grace + tick + 5s margin.
	time.Sleep(45 * time.Second)
	close(stopSampler)
	<-doneSampler

	samplerMu.Lock()
	got := maxConcurrent
	samplerMu.Unlock()
	if got > 1 {
		t.Errorf("stop-first violated: saw %d concurrent Up containers for replica 0 (SC-002-6)", got)
	}
}

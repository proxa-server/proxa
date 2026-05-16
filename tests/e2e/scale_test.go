//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSC_003_ScaleViaTOMLEdit verifies that re-running `proxa up` with
// a changed replicas count converges actual to desired.
func TestSC_003_ScaleViaTOMLEdit(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "scale.toml")
	tomlV1 := "name = \"scaler\"\nimage = \"traefik/whoami:latest\"\nreplicas = 1\n"
	if err := os.WriteFile(tomlPath, []byte(tomlV1), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up v1: %v\n%s", err, out)
	}

	t.Cleanup(func() {
		_ = exec.Command("docker", "ps", "-aq", "--filter", "label=proxa.service=scaler").
			Run()
		out, _ := exec.Command("docker", "ps", "-aq", "--filter", "label=proxa.service=scaler").Output()
		for _, id := range strings.Fields(strings.TrimSpace(string(out))) {
			_ = exec.Command("docker", "rm", "-f", id).Run()
		}
	})

	// Wait for replica 0 to come up.
	waitForCount(t, "scaler", 1, 30*time.Second)

	// Scale up to 3.
	tomlV2 := "name = \"scaler\"\nimage = \"traefik/whoami:latest\"\nreplicas = 3\n"
	if err := os.WriteFile(tomlPath, []byte(tomlV2), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up v2: %v\n%s", err, out)
	}
	waitForCount(t, "scaler", 3, 30*time.Second)

	// Scale down to 1.
	tomlV3 := "name = \"scaler\"\nimage = \"traefik/whoami:latest\"\nreplicas = 1\n"
	if err := os.WriteFile(tomlPath, []byte(tomlV3), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up v3: %v\n%s", err, out)
	}
	waitForCount(t, "scaler", 1, 30*time.Second)
}

// waitForCount blocks until the running-container count for the named
// service reaches `want`, or the deadline passes.
func waitForCount(t *testing.T, service string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := exec.Command("docker", "ps",
			"--filter", "label=proxa.managed=true",
			"--filter", "label=proxa.service="+service,
			"--format", "{{.Names}}").Output()
		if err == nil {
			n := len(strings.Fields(strings.TrimSpace(string(out))))
			if n == want {
				return
			}
		}
		time.Sleep(1 * time.Second)
	}
	t.Errorf("service %q never reached %d running containers within %s", service, want, timeout)
}

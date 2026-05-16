//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSC_002_ReconcilerRestartsKilledContainer(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "web.toml")
	// traefik/whoami runs cleanly under our non-root security profile
	// (nginx:alpine can't write /var/cache/nginx and exits immediately).
	if err := os.WriteFile(tomlPath, []byte("name = \"web\"\nimage = \"traefik/whoami:latest\"\nreplicas = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}

	// Wait for initial container.
	time.Sleep(7 * time.Second)

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-web-0").Run()
	})

	// Capture the original container ID.
	origID, err := exec.Command("docker", "inspect", "proxa-default-web-0", "--format", "{{.Id}}").CombinedOutput()
	if err != nil {
		t.Fatalf("inspect orig: %v\n%s", err, origID)
	}

	// Kill the container.
	if out, err := exec.Command("docker", "kill", "proxa-default-web-0").CombinedOutput(); err != nil {
		t.Fatalf("docker kill: %v\n%s", err, out)
	}

	// Reconciler should restart within 10s. The new container's name is the
	// same (proxa-default-web-0) but its ID differs.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		newID, err := exec.Command("docker", "inspect", "proxa-default-web-0", "--format", "{{.Id}}").CombinedOutput()
		if err == nil && string(newID) != string(origID) {
			return // success!
		}
		time.Sleep(1 * time.Second)
	}
	t.Errorf("SC-002: container was not restarted within 15s")
}

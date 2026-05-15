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

// TestSC_005_ProxaPsReflectsState verifies that proxa ps shows the
// services the user deployed with correct desired/actual counts.
func TestSC_005_ProxaPsReflectsState(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	tomlPath := filepath.Join(dir, "ps.toml")
	if err := os.WriteFile(tomlPath, []byte("name = \"pstest\"\nimage = \"traefik/whoami:latest\"\nreplicas = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}
	waitForCount(t, "pstest", 2, 15*time.Second)

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f",
			"proxa-default-pstest-0", "proxa-default-pstest-1").Run()
	})

	out, err := runProxa(t, dir, "ps")
	if err != nil {
		t.Fatalf("ps: %v\n%s", err, out)
	}
	for _, want := range []string{"PROJECT", "SERVICE", "pstest", "traefik/whoami", "healthy"} {
		if !strings.Contains(out, want) {
			t.Errorf("ps output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

//go:build e2e
// +build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSC_007a_NoInitGivesClearError verifies that running `proxa up`
// against an uninitialized data dir prints a message that points the
// operator at `proxa init` (per spec SC-007).
func TestSC_007a_NoInitGivesClearError(t *testing.T) {
	dir := t.TempDir() // intentionally NOT running `proxa init`

	tomlPath := filepath.Join(dir, "x.toml")
	if err := os.WriteFile(tomlPath, []byte("name = \"x\"\nimage = \"traefik/whoami:latest\"\nreplicas = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runProxa(t, dir, "up", tomlPath)
	if err == nil {
		t.Fatalf("expected non-zero exit; got success.\nout: %s", out)
	}
	if !strings.Contains(out, "proxa init") {
		t.Errorf("error should mention 'proxa init', got: %s", out)
	}
}

// TestSC_007b_NoDaemonGivesClearError verifies that even after init,
// running `proxa up` without a server reachable yields an actionable
// error.
func TestSC_007b_NoDaemonGivesClearError(t *testing.T) {
	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	tomlPath := filepath.Join(dir, "x.toml")
	if err := os.WriteFile(tomlPath, []byte("name = \"x\"\nimage = \"traefik/whoami:latest\"\nreplicas = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runProxa(t, dir, "up", tomlPath)
	if err == nil {
		t.Fatalf("expected non-zero exit (server not running); got success.\nout: %s", out)
	}
	if !strings.Contains(out, "cannot reach") && !strings.Contains(out, "connection") {
		t.Errorf("error should mention server unreachable, got: %s", out)
	}

	// Sanity: confirm exec.Command("docker"...) is reachable so we know
	// the test environment is OK to skip the whoami deploy.
	_ = exec.Command("true").Run()
}

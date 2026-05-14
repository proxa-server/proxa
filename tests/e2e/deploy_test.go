//go:build e2e
// +build e2e

// Run with: make test-e2e
//
// Requires Docker daemon + a built `bin/proxa` binary. These tests
// exercise the compiled binary as a subprocess against a temporary
// data dir and verify SC-001 (security defaults) end-to-end.

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// runProxa invokes the proxa binary with the given args and the
// PROXA_DATA_DIR pointing at dir. Returns combined stdout+stderr.
func runProxa(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("./bin/proxa", args...)
	cmd.Env = append(os.Environ(), "PROXA_DATA_DIR="+dir)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// startServer spawns `proxa server` in the background and returns a
// cancel function the test should defer.
func startServer(t *testing.T, dir string) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "./bin/proxa", "server")
	cmd.Env = append(os.Environ(), "PROXA_DATA_DIR="+dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start server: %v", err)
	}
	// Give the server a moment to bind its socket.
	time.Sleep(500 * time.Millisecond)
	return func() {
		cancel()
		_ = cmd.Wait()
	}
}

// dockerInspect returns the result of `docker inspect <container> --format <fmt>`.
func dockerInspect(t *testing.T, container, format string) string {
	t.Helper()
	out, err := exec.Command("docker", "inspect", container, "--format", format).CombinedOutput()
	if err != nil {
		t.Fatalf("docker inspect: %v\n%s", err, out)
	}
	return string(out)
}

func TestSC_001_DeployWithSecurityDefaults(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := runProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	stop := startServer(t, dir)
	defer stop()

	// Write a TaskDef.
	tomlPath := filepath.Join(dir, "web.toml")
	tomlContent := `
name     = "web"
image    = "nginx:alpine"
replicas = 1

[[expose]]
container = 80
host      = 0
protocol  = "http"
`
	if err := os.WriteFile(tomlPath, []byte(tomlContent), 0o600); err != nil {
		t.Fatal(err)
	}

	if out, err := runProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up: %v\n%s", err, out)
	}

	// Wait for reconciler to converge.
	time.Sleep(7 * time.Second)

	// Cleanup: best-effort tear down the container.
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-web-0").Run()
	})

	// SC-001: verify security defaults.
	caps := dockerInspect(t, "proxa-default-web-0", "{{json .HostConfig.CapDrop}}")
	if caps == "" || caps == "null\n" {
		t.Errorf("CapDrop is empty: %q", caps)
	}
	user := dockerInspect(t, "proxa-default-web-0", "{{.Config.User}}")
	if user == "" || user == "\n" || user == "root\n" || user == "0\n" {
		t.Errorf("User should be non-root, got %q", user)
	}
	secOpt := dockerInspect(t, "proxa-default-web-0", "{{json .HostConfig.SecurityOpt}}")
	if !contains(secOpt, "no-new-privileges:true") {
		t.Errorf("SecurityOpt missing no-new-privileges:true, got %q", secOpt)
	}
}

func contains(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}

// _ is an unused-symbol guard.
var _ = fmt.Sprintf

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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// runProxa invokes the proxa binary with the given args and the
// PROXA_DATA_DIR pointing at dir. Returns combined stdout+stderr.
func runProxa(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(proxaBinary(t), args...)
	cmd.Env = append(os.Environ(), proxaEnv(t, dir)...)
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
	cmd := exec.CommandContext(ctx, proxaBinary(t), "server")
	cmd.Env = append(os.Environ(), proxaEnv(t, dir)...)
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

// proxaEnv builds the env slice (PROXA_DATA_DIR + PROXA_LISTEN) for a
// proxa subprocess. Uses a short Unix socket path under /tmp because
// macOS caps sun_path at 104 bytes — t.TempDir()'s /var/folders/...
// path easily blows past that. The socket name is derived from the
// data dir's basename so server/client agree without extra plumbing.
func proxaEnv(t *testing.T, dir string) []string {
	t.Helper()
	socket := socketPath(t, dir)
	return []string{
		"PROXA_DATA_DIR=" + dir,
		"PROXA_LISTEN=unix://" + socket,
	}
}

// socketPath derives a short, deterministic socket path from the data
// dir. Same dir → same socket within a test run.
func socketPath(t *testing.T, dir string) string {
	t.Helper()
	// SHA-12 of the (already-unique) data dir gives a stable short id.
	h := sha256.Sum256([]byte(dir))
	id := hex.EncodeToString(h[:6])
	tmp := os.TempDir()
	if runtime.GOOS == "darwin" {
		tmp = "/tmp" // macOS $TMPDIR lives under /var/folders/... — too long
	}
	return filepath.Join(tmp, "proxa-e2e-"+id+".sock")
}

// skipIfHTTPProbeUnreachable skips the test when the host cannot route
// directly to docker bridge IPs. This happens on Docker Desktop (macOS
// / Windows) where the bridge network lives inside the Docker VM —
// 172.17.0.x is unreachable from the host. On a Linux server (target
// production environment for Proxa) the bridge IS routable and these
// tests run normally.
//
// Exec probes still work everywhere because they go through the docker
// daemon. The unit suite (internal/probe/http_test.go) exercises the
// HTTP probe logic against httptest.Server independently of any daemon.
func skipIfHTTPProbeUnreachable(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return
	}
	// Heuristic: if the Docker context is "desktop-linux" / "default" on
	// macOS, we are on Docker Desktop. Cheaper than starting a real
	// container — just check the platform.
	t.Skip("HTTP probe e2e tests require a host that can route to docker bridge IPs " +
		"(direct from the host). Docker Desktop on " + runtime.GOOS + " runs the bridge " +
		"inside a VM, so the host cannot reach 172.17.0.x. Run this test on a Linux server, " +
		"or rely on the unit-level HTTP probe coverage in internal/probe/http_test.go.")
}

// proxaBinary returns an absolute path to the built proxa binary. Honors
// $PROXA_BIN if set; otherwise walks up from the test file's directory
// looking for `bin/proxa`. Tests live under tests/e2e/, so the walk-up
// finds the repo-root `bin/` placed there by `make build`.
func proxaBinary(t *testing.T) string {
	t.Helper()
	if env := os.Getenv("PROXA_BIN"); env != "" {
		return env
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for {
		candidate := filepath.Join(dir, "bin", "proxa")
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			return abs
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate bin/proxa from %s — run `make build` first", mustGetwd(t))
		}
		dir = parent
	}
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
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

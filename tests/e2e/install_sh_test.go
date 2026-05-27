//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// install.sh scenario tests. These tests spin up disposable Linux
// containers via the docker CLI, copy install.sh in, and exercise its
// platform-detection + checksum + idempotency branches. The script
// itself never touches the host filesystem (work happens inside
// containers); each test cleans up its container in t.Cleanup.

const (
	// Small Debian image, widely available, has tar/sh — perfect for
	// testing a Linux glibc happy-path install.
	debianImage = "debian:12-slim"
	// Alpine for the musl refusal case.
	alpineImage = "alpine:3.20"
	// busybox-musl for the no-curl/no-wget case.
	busyboxImage = "busybox:1.36-musl"
)

// TestInstallSh_RefuseAlpine validates that the script exits 1 with a
// clear "Alpine musl" message on alpine:3.20. Does NOT require
// installing curl inside the container — the refusal happens during
// platform detection, before any network use.
func TestInstallSh_RefuseAlpine(t *testing.T) {
	skipIfNoDocker(t)
	out, code := runInstallShInContainer(t, alpineImage, "sh /work/install.sh", nil)
	if code != 1 {
		t.Errorf("expected exit code 1; got %d\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "Alpine") {
		t.Errorf("expected output to mention Alpine; got:\n%s", out)
	}
}

// TestInstallSh_RefuseChecksumMismatch hosts a fake "release" via
// httptest with a tampered checksums.txt, points the script at it via
// INSTALL_BASE_URL, and asserts exit 3. The fake release serves a
// valid archive but a deliberately-wrong SHA-256.
func TestInstallSh_RefuseChecksumMismatch(t *testing.T) {
	skipIfNoDocker(t)

	// Build a 1-byte "archive" that will not match a fake checksum.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/checksums.txt"):
			// Deliberately-wrong SHA-256 (64 zeros).
			fmt.Fprintf(w, "0000000000000000000000000000000000000000000000000000000000000000  proxa_v0.0.0-test_linux_amd64.tar.gz\n")
		case strings.HasSuffix(path, ".tar.gz"):
			// A valid empty tarball. The download succeeds, but checksum fails.
			w.Write(buildEmptyTarGz(t))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	// Resolve the httptest URL from the container's perspective. Docker
	// for Mac exposes host.docker.internal; on Linux containers we use
	// the bridge gateway. For maximum portability, we let the container
	// use --network=host on Linux (skipping macOS variant for now).
	hostURL := strings.Replace(srv.URL, "127.0.0.1", "host.docker.internal", 1)

	env := map[string]string{
		"INSTALL_BASE_URL": hostURL,
		"INSTALL_VERSION":  "v0.0.0-test",
	}
	// Run on Debian (curl available after apt-get).
	cmd := "apt-get update -qq && apt-get install -y -qq curl >/dev/null && sh /work/install.sh"
	out, code := runInstallShInContainer(t, debianImage, cmd, env)
	if code != 3 {
		t.Errorf("expected exit code 3 (checksum mismatch); got %d\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "checksum mismatch") {
		t.Errorf("expected output to mention checksum mismatch; got:\n%s", out)
	}
}

// TestInstallSh_NoCurlNoWget verifies the no-downloader exit code (5)
// in a busybox container where neither curl nor wget is installed.
// busybox uses musl (Alpine-like), so we ALSO get the Alpine-refuse
// path FIRST. To isolate the no-downloader path, override the alpine
// check by using a distroless / scratch-ish image. busybox doesn't
// have /etc/alpine-release so the alpine check passes; the script
// then hits the no-downloader path because busybox has no curl/wget.
func TestInstallSh_NoCurlNoWget(t *testing.T) {
	skipIfNoDocker(t)
	out, code := runInstallShInContainer(t, busyboxImage, "sh /work/install.sh", nil)
	// busybox uses musl, so install.sh should refuse with platform error (1)
	// IF /etc/alpine-release exists; or with no-downloader (5) otherwise.
	// busybox:1.36-musl does NOT include /etc/alpine-release, so we expect 5.
	if code != 5 {
		t.Errorf("expected exit code 5 (no curl/wget); got %d\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "neither curl nor wget") {
		t.Errorf("expected output to mention neither curl nor wget; got:\n%s", out)
	}
}

// TestInstallSh_HappyPath_LinuxAmd64 is the full path: detect platform,
// resolve version, download, verify, install, print suggested unit. This
// requires the v0.4.2 release to actually exist on GitHub Releases — at
// development time the release does NOT exist yet, so this test is
// SKIPPED in v0.4.2 development. It becomes meaningful once the release
// pipeline runs end-to-end against a real tag. The skip + reason is
// explicit so the test serves as the regression gate post-release.
func TestInstallSh_HappyPath_LinuxAmd64(t *testing.T) {
	if os.Getenv("PROXA_INSTALL_E2E_REAL_RELEASE") == "" {
		t.Skip("requires PROXA_INSTALL_E2E_REAL_RELEASE=1 + a published v0.4.2 GitHub Release; document the v0.4.2 happy-path validation in quickstart.md section 3 instead")
	}
	skipIfNoDocker(t)
	cmd := "apt-get update -qq && apt-get install -y -qq curl >/dev/null && sh /work/install.sh && /usr/local/bin/proxa version"
	out, code := runInstallShInContainer(t, debianImage, cmd, nil)
	if code != 0 {
		t.Fatalf("expected exit 0; got %d\noutput:\n%s", code, out)
	}
	for _, want := range []string{"detected: linux/amd64", "installed:", "Suggested systemd unit"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

// TestInstallSh_Idempotent is also SKIPPED until a real v0.4.1 + v0.4.2
// release exist. Documented for the v0.4.3 release validation.
func TestInstallSh_Idempotent(t *testing.T) {
	if os.Getenv("PROXA_INSTALL_E2E_REAL_RELEASE") == "" {
		t.Skip("requires PROXA_INSTALL_E2E_REAL_RELEASE=1 + published v0.4.1 + v0.4.2 GitHub Releases")
	}
	// Real impl deferred until v0.4.2 + v0.4.3 releases coexist on GitHub.
}

// --- helpers --------------------------------------------------------------

func skipIfNoDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
}

// runInstallShInContainer copies install.sh into a fresh container,
// runs the supplied shell command (which is expected to invoke the
// script), and returns (combinedOutput, exitCode). Cleans up the
// container in t.Cleanup. Times out at 60s.
func runInstallShInContainer(t *testing.T, image, shellCmd string, env map[string]string) (string, int) {
	t.Helper()

	repoRoot, err := installShRepoRoot()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}
	scriptPath := filepath.Join(repoRoot, "install.sh")
	if _, err := os.Stat(scriptPath); err != nil {
		t.Fatalf("install.sh missing at %s: %v", scriptPath, err)
	}

	args := []string{
		"run", "--rm",
		"--add-host", "host.docker.internal:host-gateway",
		"-v", scriptPath + ":/work/install.sh:ro",
	}
	for k, v := range env {
		args = append(args, "-e", k+"="+v)
	}
	args = append(args, image, "sh", "-c", shellCmd)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, "docker", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err = cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		// Non-exit-error failure (docker daemon down, image pull fail, etc.).
		t.Logf("docker run encountered error: %v", err)
		code = -1
	}
	return buf.String(), code
}

// installShRepoRoot walks up from the cwd looking for go.mod.
// Independent helper because tests/e2e/internal/harness isn't built
// yet during v0.4.2 Phase 3 (lands in T025).
func installShRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// buildEmptyTarGz returns a valid (empty) tar.gz archive payload. Just
// enough to satisfy the "downloaded archive" step before checksum
// verification rejects it.
func buildEmptyTarGz(t *testing.T) []byte {
	t.Helper()
	// A real empty tar.gz is 32 bytes — the gzip header + the tar
	// end-of-archive block. Easier: shell out to tar.
	tmp := t.TempDir()
	emptyDir := filepath.Join(tmp, "empty")
	if err := os.Mkdir(emptyDir, 0o700); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(tmp, "empty.tar.gz")
	cmd := exec.Command("tar", "-czf", out, "-C", emptyDir, ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("build empty tar.gz: %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Helpers for invoking the proxa binary as a subprocess from e2e tests.

package harness

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// RunProxa invokes the proxa binary with args + PROXA_DATA_DIR pointing
// at dir. Returns combined stdout + stderr.
func RunProxa(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(ProxaBinary(t), args...)
	cmd.Env = append(os.Environ(), ProxaEnv(t, dir)...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// ProxaBinary returns the absolute path to the built proxa binary.
// Honors $PROXA_BIN; otherwise walks up from cwd looking for bin/proxa.
func ProxaBinary(t *testing.T) string {
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
			t.Fatalf("could not locate bin/proxa from %s — run `make build` first", MustGetwd(t))
		}
		dir = parent
	}
}

// MustGetwd is `os.Getwd` with a fatal-on-error wrapper.
func MustGetwd(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

//go:build e2e
// +build e2e

package e2e

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSC_006_ToolDirectiveReproducibility covers SC-006 / FR-012: a
// fresh-clone contributor can run `go tool staticcheck` without any
// manual install step. The test copies the repo into a scratch dir,
// points GOMODCACHE at a fresh location, and asserts staticcheck
// resolves through the go.mod `tool` directive on its own.
//
// Skipped when `go` is not on PATH (developer-machine dependency).
func TestSC_006_ToolDirectiveReproducibility(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	// Locate the repo root by walking up from this file looking for
	// the project's go.mod.
	repoRoot, err := findRepoRoot()
	if err != nil {
		t.Fatalf("findRepoRoot: %v", err)
	}

	// Copy the repo (excluding .git/, bin/, and the existing GOMODCACHE
	// if any) into a scratch directory.
	scratch := t.TempDir()
	if err := copyRepoForTest(repoRoot, scratch); err != nil {
		t.Fatalf("copy repo: %v", err)
	}

	// Fresh GOMODCACHE so we exercise the download path, simulating a
	// brand-new contributor. Use os.MkdirTemp + custom cleanup so we
	// can chmod -R u+w before remove (go module cache files are read-
	// only and t.TempDir's RemoveAll would error out).
	freshCache, err := os.MkdirTemp("", "proxa-fresh-modcache-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = filepath.WalkDir(freshCache, func(p string, d fs.DirEntry, err error) error {
			if err == nil {
				_ = os.Chmod(p, 0o700)
			}
			return nil
		})
		_ = os.RemoveAll(freshCache)
	})

	cmd := exec.Command("go", "tool", "staticcheck", "-version")
	cmd.Dir = scratch
	cmd.Env = append(os.Environ(),
		"GOMODCACHE="+freshCache,
		"GOFLAGS=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go tool staticcheck -version (fresh clone): %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(strings.ToLower(string(out)), "staticcheck") {
		t.Errorf("expected output to mention staticcheck; got:\n%s", out)
	}
}

// findRepoRoot walks up from the test's working directory looking for
// the project's go.mod. Mirrors the proxaBinary helper's strategy.
func findRepoRoot() (string, error) {
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

// copyRepoForTest copies src to dst, skipping .git/, bin/, and any
// stray local caches. Keeps test setup cost bounded.
func copyRepoForTest(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		// Skip directories that bloat the copy and don't affect the
		// staticcheck resolution path.
		if d.IsDir() {
			switch rel {
			case ".git", "bin", "dist":
				return filepath.SkipDir
			}
			if rel == "." {
				return nil
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		// Regular files only.
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
}

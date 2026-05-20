package datadir_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/proxa-server/proxa/internal/datadir"
)

func TestSnapshot_HappyPath(t *testing.T) {
	src := t.TempDir()
	// Build a small tree under src.
	mustWrite(t, filepath.Join(src, "top.txt"), []byte("top"))
	mustMkdir(t, filepath.Join(src, "sub"))
	mustWrite(t, filepath.Join(src, "sub", "nested.txt"), []byte("nested"))
	mustWrite(t, filepath.Join(src, "sub", "another.bin"), []byte{0x00, 0x01, 0x02, 0xff})

	r := openRoot(t, src)
	dst := filepath.Join(t.TempDir(), "snapshot")

	if err := datadir.Snapshot(r, dst); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	// Verify dst contains the tree.
	for path, want := range map[string][]byte{
		"top.txt":          []byte("top"),
		"sub/nested.txt":   []byte("nested"),
		"sub/another.bin":  {0x00, 0x01, 0x02, 0xff},
	} {
		got, err := os.ReadFile(filepath.Join(dst, path))
		if err != nil {
			t.Errorf("ReadFile(%s): %v", path, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%s content mismatch: got %q, want %q", path, got, want)
		}
	}
}

func TestSnapshot_DstExists(t *testing.T) {
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "x.txt"), []byte("x"))
	r := openRoot(t, src)

	dst := filepath.Join(t.TempDir(), "preexisting")
	if err := os.MkdirAll(dst, 0o700); err != nil {
		t.Fatal(err)
	}

	err := datadir.Snapshot(r, dst)
	if !errors.Is(err, datadir.ErrDstExists) {
		t.Errorf("got %v, want ErrDstExists", err)
	}
	// dst should remain unchanged (still empty, untouched).
	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("dst was modified despite refusal: %d entries", len(entries))
	}
}

func TestSnapshot_ClosedSrc(t *testing.T) {
	src := t.TempDir()
	r := openRoot(t, src)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "snapshot")
	if err := datadir.Snapshot(r, dst); !errors.Is(err, datadir.ErrClosed) {
		t.Errorf("got %v, want ErrClosed", err)
	}
	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dst exists despite closed-src error: %v", err)
	}
}

func TestSnapshot_NilSrc(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "snapshot")
	if err := datadir.Snapshot(nil, dst); !errors.Is(err, datadir.ErrClosed) {
		t.Errorf("got %v, want ErrClosed", err)
	}
}

func TestSnapshot_SpecialFileCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Mkfifo not available on Windows")
	}
	src := t.TempDir()
	mustWrite(t, filepath.Join(src, "regular.txt"), []byte("ok"))
	// Mkfifo creates a named pipe — os.CopyFS errors on these.
	if err := syscall.Mkfifo(filepath.Join(src, "fifo"), 0o600); err != nil {
		t.Skipf("Mkfifo unavailable on this filesystem: %v", err)
	}
	r := openRoot(t, src)

	dst := filepath.Join(t.TempDir(), "snapshot")
	if err := datadir.Snapshot(r, dst); err == nil {
		t.Errorf("Snapshot succeeded with FIFO in src; expected error")
	}
	// dst MUST NOT exist after a failed snapshot (cleanup ran).
	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dst exists after failed snapshot: %v", err)
	}
	// The tmp directory MUST NOT exist either.
	parent := filepath.Dir(dst)
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(dst) {
			t.Errorf("stray entry in parent after failed snapshot: %q", e.Name())
		}
	}
}

func TestSnapshot_EmptyTree(t *testing.T) {
	src := t.TempDir()
	r := openRoot(t, src)
	dst := filepath.Join(t.TempDir(), "empty-snap")

	if err := datadir.Snapshot(r, dst); err != nil {
		t.Fatalf("Snapshot of empty tree: %v", err)
	}
	info, err := os.Stat(dst)
	if err != nil {
		t.Fatalf("Stat dst: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("dst is not a directory: mode %v", info.Mode())
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty snapshot; got %d entries", len(entries))
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

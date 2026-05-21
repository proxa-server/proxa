package datadir_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/proxa-server/proxa/internal/datadir"
)

func TestRoot_OpenAndDir(t *testing.T) {
	dir := t.TempDir()
	r, err := datadir.Open(dir)
	if err != nil {
		t.Fatalf("Open(%q): %v", dir, err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if r.Dir() != dir {
		t.Errorf("Dir() = %q, want %q", r.Dir(), dir)
	}
}

func TestRoot_OpenNonexistent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	r, err := datadir.Open(missing)
	if err == nil {
		_ = r.Close()
		t.Fatalf("Open(%q) succeeded; want error", missing)
	}
}

func TestRoot_CleanRead(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("world"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := openRoot(t, dir)
	got, err := r.ReadFile("hello.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "world" {
		t.Errorf("got %q, want %q", got, "world")
	}
}

func TestRoot_ParentEscape(t *testing.T) {
	dir := t.TempDir()
	// Place a file outside the root and try to reach it via ../
	parent := filepath.Dir(dir)
	outside := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })

	r := openRoot(t, dir)
	if _, err := r.Open("../outside.txt"); err == nil {
		t.Errorf("Open(\"../outside.txt\") succeeded; want refusal")
	}
}

func TestRoot_AbsoluteEscape(t *testing.T) {
	r := openRoot(t, t.TempDir())
	if _, err := r.Open("/etc/passwd"); err == nil {
		t.Errorf("Open(/etc/passwd) succeeded; want refusal")
	}
}

func TestRoot_SymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; skipping")
	}
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside-target.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link-out")); err != nil {
		t.Fatal(err)
	}

	r := openRoot(t, dir)
	if _, err := r.Open("link-out"); err == nil {
		t.Errorf("Open(link-out → outside target) succeeded; want refusal")
	}
}

func TestRoot_SymlinkInside(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows; skipping")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target.txt"), []byte("inside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", filepath.Join(dir, "link-in")); err != nil {
		t.Fatal(err)
	}

	r := openRoot(t, dir)
	got, err := r.ReadFile("link-in")
	if err != nil {
		t.Fatalf("ReadFile(link-in): %v", err)
	}
	if string(got) != "inside" {
		t.Errorf("got %q, want %q", got, "inside")
	}
}

func TestRoot_ClosedRoot(t *testing.T) {
	r := openRoot(t, t.TempDir())
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	cases := []struct {
		name string
		fn   func() error
	}{
		{"Open", func() error { _, err := r.Open("x"); return err }},
		{"Create", func() error { _, err := r.Create("x"); return err }},
		{"Stat", func() error { _, err := r.Stat("x"); return err }},
		{"Mkdir", func() error { return r.Mkdir("x", 0o700) }},
		{"MkdirAll", func() error { return r.MkdirAll("x/y", 0o700) }},
		{"Remove", func() error { return r.Remove("x") }},
		{"RemoveAll", func() error { return r.RemoveAll("x") }},
		{"ReadFile", func() error { _, err := r.ReadFile("x"); return err }},
		{"WriteFile", func() error { return r.WriteFile("x", nil, 0o600) }},
		{"Close-twice", func() error { return r.Close() }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.fn(); !errors.Is(err, datadir.ErrClosed) {
				t.Errorf("got %v, want ErrClosed", err)
			}
		})
	}
	// FS() on a closed Root returns an fs.FS that errors on every op.
	t.Run("FS-after-close", func(t *testing.T) {
		f := r.FS()
		_, err := f.Open(".")
		if !errors.Is(err, datadir.ErrClosed) {
			t.Errorf("FS.Open returned %v; want ErrClosed", err)
		}
	})
}

func TestRoot_WriteReadRoundtrip(t *testing.T) {
	r := openRoot(t, t.TempDir())
	want := []byte("roundtrip content with\nnewlines and \x00 bytes\n")
	if err := r.WriteFile("data.bin", want, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := r.ReadFile("data.bin")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("roundtrip mismatch: got %q, want %q", got, want)
	}
}

func TestRoot_MkdirAndStat(t *testing.T) {
	r := openRoot(t, t.TempDir())
	if err := r.MkdirAll("a/b/c", 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	info, err := r.Stat("a/b/c")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected directory; got mode %v", info.Mode())
	}
}

func TestRoot_ConcurrentReads(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shared.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := openRoot(t, dir)

	var wg sync.WaitGroup
	const N = 100
	errs := make(chan error, N)
	for range N {
		wg.Go(func() {
			got, err := r.ReadFile("shared.txt")
			if err != nil {
				errs <- err
				return
			}
			if string(got) != "payload" {
				errs <- errors.New("unexpected content")
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent read failure: %v", err)
	}
}

func TestRoot_FSWalk(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "top.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "nested.txt"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := openRoot(t, dir)

	var paths []string
	if err := fs.WalkDir(r.FS(), ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	want := map[string]bool{".": true, "top.txt": true, "sub": true, "sub/nested.txt": true}
	for _, p := range paths {
		if !want[p] {
			t.Errorf("unexpected path %q in walk", p)
		}
		delete(want, p)
	}
	for missing := range want {
		t.Errorf("path %q not visited", missing)
	}
}

func openRoot(t *testing.T, dir string) *datadir.Root {
	t.Helper()
	r, err := datadir.Open(dir)
	if err != nil {
		t.Fatalf("Open(%q): %v", dir, err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

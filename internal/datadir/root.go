package datadir

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// ErrClosed is returned by methods called on a *Root after Close.
var ErrClosed = errors.New("datadir: root is closed")

// Root is a goroutine-safe handle to a sandboxed data directory.
// All file operations are restricted to paths inside the directory
// it was opened with — relative paths above, absolute paths outside,
// and symlinks resolving outside are refused by the underlying
// *os.Root with a *PathError wrapping fs.ErrInvalid.
//
// One *Root per process is the expected usage. The zero value is
// unusable; obtain instances via Open.
type Root struct {
	r   *os.Root
	dir string
}

// Open returns a Root anchored at dir. dir MUST be an absolute path
// that already exists and is a directory.
func Open(dir string) (*Root, error) {
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("datadir: open root %q: %w", dir, err)
	}
	return &Root{r: r, dir: dir}, nil
}

// Dir returns the absolute path the Root was opened at.
func (r *Root) Dir() string { return r.dir }

// Close releases the underlying OS resources. Subsequent calls return
// ErrClosed.
func (r *Root) Close() error {
	if r.r == nil {
		return ErrClosed
	}
	err := r.r.Close()
	r.r = nil
	return err
}

// Open opens name for reading. name is relative to the anchor.
func (r *Root) Open(name string) (*os.File, error) {
	if r.r == nil {
		return nil, ErrClosed
	}
	return r.r.Open(name)
}

// Create creates or truncates name. name is relative to the anchor.
func (r *Root) Create(name string) (*os.File, error) {
	if r.r == nil {
		return nil, ErrClosed
	}
	return r.r.Create(name)
}

// OpenFile opens name with the given flag and permissions. name is
// relative to the anchor.
func (r *Root) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	if r.r == nil {
		return nil, ErrClosed
	}
	return r.r.OpenFile(name, flag, perm)
}

// Stat returns FileInfo for name. name is relative to the anchor.
func (r *Root) Stat(name string) (os.FileInfo, error) {
	if r.r == nil {
		return nil, ErrClosed
	}
	return r.r.Stat(name)
}

// Mkdir creates a directory at name with the given permissions.
// Parent directories are NOT created (use MkdirAll for that).
func (r *Root) Mkdir(name string, perm os.FileMode) error {
	if r.r == nil {
		return ErrClosed
	}
	return r.r.Mkdir(name, perm)
}

// MkdirAll creates name and any missing parents within the anchor.
func (r *Root) MkdirAll(name string, perm os.FileMode) error {
	if r.r == nil {
		return ErrClosed
	}
	return r.r.MkdirAll(name, perm)
}

// Remove removes the named file or empty directory.
func (r *Root) Remove(name string) error {
	if r.r == nil {
		return ErrClosed
	}
	return r.r.Remove(name)
}

// RemoveAll removes name and any children. Use with caution.
func (r *Root) RemoveAll(name string) error {
	if r.r == nil {
		return ErrClosed
	}
	return r.r.RemoveAll(name)
}

// ReadFile is a convenience wrapper: Open + ReadAll + Close.
func (r *Root) ReadFile(name string) ([]byte, error) {
	if r.r == nil {
		return nil, ErrClosed
	}
	return r.r.ReadFile(name)
}

// WriteFile is a convenience wrapper: Create + Write + Close, applying
// perm to the new file. Existing files are truncated.
func (r *Root) WriteFile(name string, data []byte, perm os.FileMode) error {
	if r.r == nil {
		return ErrClosed
	}
	return r.r.WriteFile(name, data, perm)
}

// FS returns an fs.FS view of the rooted directory, suitable for
// passing to os.CopyFS or fs.WalkDir.
func (r *Root) FS() fs.FS {
	if r.r == nil {
		return errFS{err: ErrClosed}
	}
	return r.r.FS()
}

// errFS is a tiny fs.FS that returns the same error from every op.
// Used so FS() can return a non-nil value when the Root is closed,
// matching the rest of the API surface (callers always get an error,
// never a nil dereference).
type errFS struct{ err error }

func (e errFS) Open(string) (fs.File, error) { return nil, e.err }

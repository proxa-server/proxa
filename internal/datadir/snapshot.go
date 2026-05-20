package datadir

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrDstExists is returned by Snapshot when the destination path
// already exists. Snapshot refuses to overwrite.
var ErrDstExists = errors.New("datadir: snapshot destination already exists")

// Snapshot copies the contents of src to dst as a new directory.
// dst MUST NOT exist; it is created. The operation is atomic from
// the caller's perspective: either dst is the complete snapshot, or
// dst does not exist (cleanup on failure).
//
// src is a Root, so the source is implicitly path-sandboxed.
// dst is an absolute filesystem path supplied by the caller and is
// NOT sandboxed — the caller is responsible for choosing a safe dst.
//
// Snapshot uses os.CopyFS under the hood, preserving file modes and
// regular-file content. Symlinks inside src are copied as symlinks
// when CopyFS supports them on the host filesystem. Special files
// (devices, sockets, FIFOs) cause Snapshot to return an error.
//
// Snapshot does NOT take any cluster-wide consistency lock. The
// caller is responsible for quiescing writes against src for the
// duration if a point-in-time snapshot is required. For SQLite,
// callers SHOULD use the SQLite VACUUM INTO / backup API to capture
// a consistent DB snapshot rather than relying on this helper.
//
// Atomicity strategy: copy into a sibling temp directory, then
// rename to dst. The rename is atomic on POSIX filesystems within
// the same mount; cross-mount falls back to a copy + remove and is
// no longer atomic.
func Snapshot(src *Root, dst string) error {
	if src == nil || src.r == nil {
		return ErrClosed
	}
	if _, err := os.Stat(dst); err == nil {
		return ErrDstExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("datadir: snapshot stat dst: %w", err)
	}

	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("datadir: snapshot mkdir parent: %w", err)
	}

	suffix, err := randomSuffix()
	if err != nil {
		return fmt.Errorf("datadir: snapshot random suffix: %w", err)
	}
	tmp := dst + ".tmp." + suffix

	if err := os.CopyFS(tmp, src.FS()); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("datadir: snapshot copyfs: %w", err)
	}

	if err := os.Rename(tmp, dst); err != nil {
		_ = os.RemoveAll(tmp)
		return fmt.Errorf("datadir: snapshot rename: %w", err)
	}
	return nil
}

func randomSuffix() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

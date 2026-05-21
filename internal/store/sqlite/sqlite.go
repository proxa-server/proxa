// Package sqlite is the SQLite-backed implementation of
// [github.com/proxa-server/proxa/internal/store.StateStore]. Uses
// modernc.org/sqlite (pure Go, no CGO).
//
// See specs/000-foundation/contracts/statestore.md for the behavioral
// contract; specs/001-core-loop/data-model.md for the schema.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/proxa-server/proxa/internal/datadir"
	_ "modernc.org/sqlite" // register the SQL driver
)

// Store implements store.StateStore against a local SQLite database.
type Store struct {
	db  *sql.DB
	dsn string
}

// New returns an unopened Store. Call [Store.Open] before any other method.
func New() *Store {
	return &Store{}
}

// OpenInRoot opens the SQLite database at filename relative to root.
// The path is validated against the root's sandbox before sql.Open is
// called — a name that resolves outside root returns an error and the
// database is not opened. Use this from production code paths.
//
// The SQLite driver still receives an absolute path (it needs a path
// string, not a *os.File), but the absolute path is the one root.Dir()
// composes with filename — guaranteed by the prior sandbox check to
// resolve inside the data directory.
//
// Production callers (proxa init, proxa server) MUST use OpenInRoot.
// The legacy [Store.Open] is preserved for the in-memory test idiom
// (`":memory:"`) and for callers that already have a validated path.
func (s *Store) OpenInRoot(ctx context.Context, root *datadir.Root, filename string) error {
	if root == nil {
		return fmt.Errorf("store/sqlite: OpenInRoot: nil root")
	}
	// Sandbox check: open + close immediately. O_CREATE so first-init
	// (file does not yet exist) succeeds. If filename escapes root,
	// the underlying *os.Root refuses with a *PathError.
	f, err := root.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("store/sqlite: sandbox %q: %w", filename, err)
	}
	_ = f.Close()
	return s.Open(ctx, filepath.Join(root.Dir(), filename))
}

// Open establishes the connection pool and applies WAL + busy-timeout pragmas.
// dsn is the SQLite DSN (path or ":memory:"). Open is idempotent within a
// process — re-opening after Close is allowed.
//
// Production code SHOULD use [Store.OpenInRoot] instead — Open bypasses
// the data-dir sandbox and accepts any path string.
func (s *Store) Open(ctx context.Context, dsn string) error {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("store/sqlite: open(%q): %w", dsn, err)
	}
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
	}
	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return fmt.Errorf("store/sqlite: pragma %q: %w", p, err)
		}
	}
	s.db = db
	s.dsn = dsn
	return nil
}

// Close releases the underlying connection pool.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("store/sqlite: close: %w", err)
	}
	s.db = nil
	return nil
}

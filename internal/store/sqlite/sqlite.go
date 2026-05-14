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

// Open establishes the connection pool and applies WAL + busy-timeout pragmas.
// dsn is the SQLite DSN (path or ":memory:"). Open is idempotent within a
// process — re-opening after Close is allowed.
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

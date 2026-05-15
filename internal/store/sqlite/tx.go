package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/proxa-server/proxa/internal/store"
)

// txn is the transaction-scoped wrapper exposed to Tx() callers.
// In v0.0 it provides the same surface as Store; future extensions
// can add tx-only helpers without changing the interface.
type txn struct {
	*Store
	sqlTx *sql.Tx
}

// Tx runs fn inside a single SQLite transaction. Commits on nil error,
// rolls back otherwise.
func (s *Store) Tx(ctx context.Context, fn func(store.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: begin tx: %w", err)
	}
	if err := fn(&txn{Store: s, sqlTx: tx}); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: commit tx: %w", err)
	}
	return nil
}

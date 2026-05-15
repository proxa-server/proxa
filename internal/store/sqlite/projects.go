package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

// projectNameRe is the allowed project-name pattern.
// Mirrors the rule documented in toml-grammar.md.
var projectNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// CreateProject inserts a new project row. Returns ErrAlreadyExists
// if the name collides.
func (s *Store) CreateProject(ctx context.Context, p types.Project) error {
	if !projectNameRe.MatchString(p.Name) {
		return fmt.Errorf("store/sqlite: invalid project name %q", p.Name)
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO projects(name, created_at) VALUES (?, ?)`,
		p.Name, p.CreatedAt.Format(time.RFC3339))
	if err != nil {
		if isUniqueViolation(err) {
			return store.ErrAlreadyExists
		}
		return fmt.Errorf("store/sqlite: create project %q: %w", p.Name, err)
	}
	return nil
}

// GetProject returns the project row by name, or store.ErrNotFound.
func (s *Store) GetProject(ctx context.Context, name string) (*types.Project, error) {
	row := s.db.QueryRowContext(ctx, `SELECT name, created_at FROM projects WHERE name = ?`, name)
	var p types.Project
	var createdAt string
	if err := row.Scan(&p.Name, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("store/sqlite: get project %q: %w", name, err)
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: parse project created_at: %w", err)
	}
	p.CreatedAt = t
	return &p, nil
}

// ListProjects returns every project, sorted by name.
func (s *Store) ListProjects(ctx context.Context) ([]types.Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, created_at FROM projects ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list projects: %w", err)
	}
	defer rows.Close()

	var out []types.Project
	for rows.Next() {
		var p types.Project
		var createdAt string
		if err := rows.Scan(&p.Name, &createdAt); err != nil {
			return nil, fmt.Errorf("store/sqlite: scan project: %w", err)
		}
		t, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: parse project created_at: %w", err)
		}
		p.CreatedAt = t
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteProject removes a project. Fails if any service/job still references it
// (ON DELETE RESTRICT on the FK).
func (s *Store) DeleteProject(ctx context.Context, name string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM projects WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete project %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store/sqlite: delete project rows affected: %w", err)
	}
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// isUniqueViolation returns true if err looks like a SQLite UNIQUE constraint
// failure. modernc.org/sqlite returns errors whose Error() contains
// "UNIQUE constraint failed" — checked structurally rather than by typed
// extraction to avoid coupling to an internal type.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "UNIQUE constraint failed") || contains(msg, "PRIMARY KEY constraint failed")
}

// contains is a tiny strings.Contains alternative to keep imports tight.
func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

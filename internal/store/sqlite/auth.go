package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

// PutSubject inserts or updates a subject row. The secret_hash column
// holds the bcrypt hash of either the bootstrap token or a local
// password — never plaintext (constitution §II).
func (s *Store) PutSubject(ctx context.Context, sub types.Subject) error {
	if sub.ID == "" {
		return fmt.Errorf("store/sqlite: PutSubject requires non-empty ID")
	}
	metaJSON, err := json.Marshal(sub.Metadata)
	if err != nil {
		return fmt.Errorf("store/sqlite: marshal subject metadata: %w", err)
	}
	if sub.Metadata == nil {
		metaJSON = []byte("{}")
	}
	now := time.Now().UTC().Format(time.RFC3339)

	// secret_hash and email are sourced from Metadata for v0.0
	// (the type doesn't expose them as first-class fields).
	var secretHash sql.NullString
	if h, ok := sub.Metadata["secret_hash"]; ok && h != "" {
		secretHash = sql.NullString{String: h, Valid: true}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO subjects(id, name, email, provider, metadata_json, secret_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			email = excluded.email,
			provider = excluded.provider,
			metadata_json = excluded.metadata_json,
			secret_hash = excluded.secret_hash
	`, sub.ID, sub.Name, nullableString(sub.Email), sub.Provider, string(metaJSON), secretHash, now)
	if err != nil {
		return fmt.Errorf("store/sqlite: upsert subject %q: %w", sub.ID, err)
	}
	return nil
}

// GetSubject fetches a subject by ID.
func (s *Store) GetSubject(ctx context.Context, id string) (*types.Subject, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, email, provider, metadata_json, secret_hash FROM subjects WHERE id = ?
	`, id)
	var (
		sub        types.Subject
		email      sql.NullString
		metaJSON   string
		secretHash sql.NullString
	)
	if err := row.Scan(&sub.ID, &sub.Name, &email, &sub.Provider, &metaJSON, &secretHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, store.ErrNotFound
		}
		return nil, fmt.Errorf("store/sqlite: get subject %q: %w", id, err)
	}
	if email.Valid {
		sub.Email = email.String
	}
	if metaJSON != "" && metaJSON != "{}" {
		_ = json.Unmarshal([]byte(metaJSON), &sub.Metadata)
	}
	if sub.Metadata == nil {
		sub.Metadata = map[string]string{}
	}
	if secretHash.Valid {
		sub.Metadata["secret_hash"] = secretHash.String
	}
	return &sub, nil
}

// PutPolicy inserts a policy row. Rejects Project="*" for non-admin roles
// per data-model.md.
func (s *Store) PutPolicy(ctx context.Context, p types.Policy) error {
	if p.ID == "" {
		return fmt.Errorf("store/sqlite: PutPolicy requires non-empty ID")
	}
	if p.Project == "*" && p.Role != types.RoleAdmin {
		return fmt.Errorf("store/sqlite: wildcard Project='*' is only valid for RoleAdmin")
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO policies(id, subject_id, role, project, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			subject_id = excluded.subject_id,
			role = excluded.role,
			project = excluded.project
	`, p.ID, p.SubjectID, string(p.Role), p.Project, p.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store/sqlite: upsert policy %q: %w", p.ID, err)
	}
	return nil
}

// ListPoliciesFor returns every policy bound to a subject.
func (s *Store) ListPoliciesFor(ctx context.Context, subjectID string) ([]types.Policy, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, subject_id, role, project, created_at FROM policies WHERE subject_id = ?`,
		subjectID)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list policies for %q: %w", subjectID, err)
	}
	defer rows.Close()
	var out []types.Policy
	for rows.Next() {
		var (
			p         types.Policy
			role      string
			createdAt string
		)
		if err := rows.Scan(&p.ID, &p.SubjectID, &role, &p.Project, &createdAt); err != nil {
			return nil, err
		}
		p.Role = types.Role(role)
		if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
			p.CreatedAt = t
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePolicy removes a policy by ID.
func (s *Store) DeletePolicy(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM policies WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete policy %q: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

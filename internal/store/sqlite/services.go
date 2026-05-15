package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/proxa-server/proxa/internal/hash"
	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

// PutService inserts or updates a service row. spec_hash is recomputed
// from the spec on every call; callers cannot fake it. Rejects mismatched
// project/name between the row and the spec.
func (s *Store) PutService(ctx context.Context, project string, svc types.Service) error {
	if project == "" {
		return fmt.Errorf("store/sqlite: PutService requires non-empty project")
	}
	if svc.Project != project || svc.Name == "" {
		return fmt.Errorf("store/sqlite: service project/name mismatch (path=%q row=%q/%q)",
			project, svc.Project, svc.Name)
	}

	specJSON, err := json.Marshal(svc.Spec)
	if err != nil {
		return fmt.Errorf("store/sqlite: marshal service spec: %w", err)
	}
	replicasJSON, err := json.Marshal(svc.Replicas)
	if err != nil {
		return fmt.Errorf("store/sqlite: marshal replicas: %w", err)
	}
	if svc.Replicas == nil {
		replicasJSON = []byte("[]")
	}
	historyJSON, err := json.Marshal(svc.History)
	if err != nil {
		return fmt.Errorf("store/sqlite: marshal history: %w", err)
	}
	if svc.History == nil {
		historyJSON = []byte("[]")
	}

	specHash := hash.Hash(svc.Spec)
	now := time.Now().UTC().Format(time.RFC3339)
	if svc.CreatedAt.IsZero() {
		svc.CreatedAt = time.Now().UTC()
	}
	createdAt := svc.CreatedAt.Format(time.RFC3339)
	if svc.ID == "" {
		svc.ID = newID("svc")
	}
	if svc.Status == "" {
		svc.Status = types.ServiceStatusPending
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO services (project, name, id, spec_json, spec_hash, status, replicas_json, history_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project, name) DO UPDATE SET
			spec_json = excluded.spec_json,
			spec_hash = excluded.spec_hash,
			status = excluded.status,
			replicas_json = excluded.replicas_json,
			history_json = excluded.history_json,
			updated_at = excluded.updated_at
	`, project, svc.Name, svc.ID, string(specJSON), specHash, string(svc.Status),
		string(replicasJSON), string(historyJSON), createdAt, now)
	if err != nil {
		return fmt.Errorf("store/sqlite: upsert service %q/%q: %w", project, svc.Name, err)
	}
	return nil
}

// GetService returns the service row by (project, name), or store.ErrNotFound.
func (s *Store) GetService(ctx context.Context, project, name string) (*types.Service, error) {
	if project == "" {
		return nil, fmt.Errorf("store/sqlite: GetService requires non-empty project")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT project, name, id, spec_json, status, replicas_json, history_json, created_at, updated_at
		FROM services WHERE project = ? AND name = ?
	`, project, name)
	svc, err := scanService(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: get service %q/%q: %w", project, name, err)
	}
	return svc, nil
}

// ListServices returns every service in a project, sorted by name.
func (s *Store) ListServices(ctx context.Context, project string) ([]types.Service, error) {
	if project == "" {
		return nil, fmt.Errorf("store/sqlite: ListServices requires non-empty project")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT project, name, id, spec_json, status, replicas_json, history_json, created_at, updated_at
		FROM services WHERE project = ? ORDER BY name
	`, project)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list services in %q: %w", project, err)
	}
	defer rows.Close()

	var out []types.Service
	for rows.Next() {
		svc, err := scanService(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan service: %w", err)
		}
		out = append(out, *svc)
	}
	return out, rows.Err()
}

// DeleteService removes a service row by (project, name).
func (s *Store) DeleteService(ctx context.Context, project, name string) error {
	if project == "" {
		return fmt.Errorf("store/sqlite: DeleteService requires non-empty project")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM services WHERE project = ? AND name = ?`, project, name)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete service %q/%q: %w", project, name, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

// scanService decodes one row into types.Service. Accepts a Scan function
// so it works with both *sql.Row and *sql.Rows.
func scanService(scan func(...any) error) (*types.Service, error) {
	var (
		svc                          types.Service
		specJSON, replicasJSON, historyJSON string
		createdAt, updatedAt         string
		status                       string
	)
	if err := scan(&svc.Project, &svc.Name, &svc.ID, &specJSON, &status, &replicasJSON, &historyJSON, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	svc.Status = types.ServiceStatus(status)
	if err := json.Unmarshal([]byte(specJSON), &svc.Spec); err != nil {
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}
	if err := json.Unmarshal([]byte(replicasJSON), &svc.Replicas); err != nil {
		return nil, fmt.Errorf("unmarshal replicas: %w", err)
	}
	if err := json.Unmarshal([]byte(historyJSON), &svc.History); err != nil {
		return nil, fmt.Errorf("unmarshal history: %w", err)
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		svc.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		svc.UpdatedAt = t
	}
	return &svc, nil
}

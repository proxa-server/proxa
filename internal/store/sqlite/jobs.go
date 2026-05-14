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

// PutJob inserts or updates a job row, scoped to project.
func (s *Store) PutJob(ctx context.Context, project string, j types.Job) error {
	if project == "" {
		return fmt.Errorf("store/sqlite: PutJob requires non-empty project")
	}
	if j.Project != project || j.Name == "" {
		return fmt.Errorf("store/sqlite: job project/name mismatch")
	}
	specJSON, err := json.Marshal(j.Spec)
	if err != nil {
		return fmt.Errorf("store/sqlite: marshal job spec: %w", err)
	}
	var lastRunJSON sql.NullString
	if j.LastRun != nil {
		b, err := json.Marshal(j.LastRun)
		if err != nil {
			return fmt.Errorf("store/sqlite: marshal last run: %w", err)
		}
		lastRunJSON = sql.NullString{String: string(b), Valid: true}
	}
	var nextRunAt sql.NullString
	if !j.NextRunAt.IsZero() {
		nextRunAt = sql.NullString{String: j.NextRunAt.Format(time.RFC3339), Valid: true}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now().UTC()
	}
	if j.ID == "" {
		j.ID = newID("job")
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO jobs(project, name, id, spec_json, last_run_json, next_run_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project, name) DO UPDATE SET
			spec_json = excluded.spec_json,
			last_run_json = excluded.last_run_json,
			next_run_at = excluded.next_run_at,
			updated_at = excluded.updated_at
	`, project, j.Name, j.ID, string(specJSON), lastRunJSON, nextRunAt,
		j.CreatedAt.Format(time.RFC3339), now)
	if err != nil {
		return fmt.Errorf("store/sqlite: upsert job %q/%q: %w", project, j.Name, err)
	}
	return nil
}

// GetJob fetches a job by (project, name).
func (s *Store) GetJob(ctx context.Context, project, name string) (*types.Job, error) {
	if project == "" {
		return nil, fmt.Errorf("store/sqlite: GetJob requires non-empty project")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT project, name, id, spec_json, last_run_json, next_run_at, created_at, updated_at
		FROM jobs WHERE project = ? AND name = ?
	`, project, name)
	j, err := scanJob(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, store.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: get job %q/%q: %w", project, name, err)
	}
	return j, nil
}

// ListJobs returns every job in a project, sorted by name.
func (s *Store) ListJobs(ctx context.Context, project string) ([]types.Job, error) {
	if project == "" {
		return nil, fmt.Errorf("store/sqlite: ListJobs requires non-empty project")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT project, name, id, spec_json, last_run_json, next_run_at, created_at, updated_at
		FROM jobs WHERE project = ? ORDER BY name
	`, project)
	if err != nil {
		return nil, fmt.Errorf("store/sqlite: list jobs: %w", err)
	}
	defer rows.Close()

	var out []types.Job
	for rows.Next() {
		j, err := scanJob(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store/sqlite: scan job: %w", err)
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// DeleteJob removes a job row.
func (s *Store) DeleteJob(ctx context.Context, project, name string) error {
	if project == "" {
		return fmt.Errorf("store/sqlite: DeleteJob requires non-empty project")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM jobs WHERE project = ? AND name = ?`, project, name)
	if err != nil {
		return fmt.Errorf("store/sqlite: delete job %q/%q: %w", project, name, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return store.ErrNotFound
	}
	return nil
}

func scanJob(scan func(...any) error) (*types.Job, error) {
	var (
		j                                types.Job
		specJSON                         string
		lastRunJSON, nextRunAt           sql.NullString
		createdAt, updatedAt             string
	)
	if err := scan(&j.Project, &j.Name, &j.ID, &specJSON, &lastRunJSON, &nextRunAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(specJSON), &j.Spec); err != nil {
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}
	if lastRunJSON.Valid {
		var lr types.JobRun
		if err := json.Unmarshal([]byte(lastRunJSON.String), &lr); err == nil {
			j.LastRun = &lr
		}
	}
	if nextRunAt.Valid {
		if t, err := time.Parse(time.RFC3339, nextRunAt.String); err == nil {
			j.NextRunAt = t
		}
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		j.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		j.UpdatedAt = t
	}
	return &j, nil
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// schemaMigrations is the ordered list of Go-coded migrations. Each migration
// runs in its own transaction. Adding a new schema version means appending
// a new entry here; Migrate then applies it on next start.
var schemaMigrations = []migration{
	{version: 1, apply: migrateV1},
	{version: 2, apply: migrateV2},
}

type migration struct {
	version int
	apply   func(ctx context.Context, tx *sql.Tx) error
}

// Migrate runs every migration whose version is greater than the currently-
// recorded schema_version. Idempotent: re-running on the latest schema is
// a no-op. Each migration is atomic per its own transaction.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("store/sqlite: bootstrap schema_version: %w", err)
	}

	var current int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store/sqlite: read current schema version: %w", err)
	}

	for _, m := range schemaMigrations {
		if m.version <= current {
			continue
		}
		if err := s.runMigration(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) runMigration(ctx context.Context, m migration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store/sqlite: begin migration v%d: %w", m.version, err)
	}
	if err := m.apply(ctx, tx); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store/sqlite: apply migration v%d: %w", m.version, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version(version, applied_at) VALUES (?, ?)`,
		m.version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store/sqlite: record migration v%d: %w", m.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store/sqlite: commit migration v%d: %w", m.version, err)
	}
	return nil
}

// migrateV1 is the initial schema per specs/001-core-loop/data-model.md.
func migrateV1(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE projects (
			name        TEXT PRIMARY KEY,
			created_at  TEXT NOT NULL
		) WITHOUT ROWID`,

		`CREATE TABLE services (
			project       TEXT NOT NULL REFERENCES projects(name) ON DELETE RESTRICT,
			name          TEXT NOT NULL,
			id            TEXT NOT NULL UNIQUE,
			spec_json     TEXT NOT NULL,
			spec_hash     TEXT NOT NULL,
			status        TEXT NOT NULL,
			replicas_json TEXT NOT NULL DEFAULT '[]',
			history_json  TEXT NOT NULL DEFAULT '[]',
			created_at    TEXT NOT NULL,
			updated_at    TEXT NOT NULL,
			PRIMARY KEY (project, name)
		) WITHOUT ROWID`,
		`CREATE INDEX idx_services_status ON services(status)`,

		`CREATE TABLE jobs (
			project       TEXT NOT NULL REFERENCES projects(name) ON DELETE RESTRICT,
			name          TEXT NOT NULL,
			id            TEXT NOT NULL UNIQUE,
			spec_json     TEXT NOT NULL,
			last_run_json TEXT,
			next_run_at   TEXT,
			created_at    TEXT NOT NULL,
			updated_at    TEXT NOT NULL,
			PRIMARY KEY (project, name)
		) WITHOUT ROWID`,
		`CREATE INDEX idx_jobs_next_run ON jobs(next_run_at)`,

		`CREATE TABLE nodes (
			id              TEXT PRIMARY KEY,
			name            TEXT NOT NULL,
			role            TEXT NOT NULL,
			address         TEXT NOT NULL,
			cpu_cores       INTEGER NOT NULL,
			memory_mb       INTEGER NOT NULL,
			disk_gb         INTEGER NOT NULL,
			container_count INTEGER NOT NULL DEFAULT 0,
			last_heartbeat  TEXT NOT NULL,
			status          TEXT NOT NULL,
			labels_json     TEXT NOT NULL DEFAULT '{}'
		) WITHOUT ROWID`,
		`CREATE INDEX idx_nodes_status ON nodes(status)`,

		`CREATE TABLE subjects (
			id            TEXT PRIMARY KEY,
			name          TEXT NOT NULL,
			email         TEXT,
			provider      TEXT NOT NULL,
			metadata_json TEXT NOT NULL DEFAULT '{}',
			secret_hash   TEXT,
			created_at    TEXT NOT NULL
		) WITHOUT ROWID`,
		`CREATE INDEX idx_subjects_provider ON subjects(provider)`,

		`CREATE TABLE policies (
			id          TEXT PRIMARY KEY,
			subject_id  TEXT NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
			role        TEXT NOT NULL,
			project     TEXT NOT NULL,
			created_at  TEXT NOT NULL
		) WITHOUT ROWID`,
		`CREATE INDEX idx_policies_subject ON policies(subject_id)`,
		`CREATE INDEX idx_policies_project_role ON policies(project, role)`,

		// Seed the default project that constitution §III implies always exists.
		`INSERT INTO projects(name, created_at) VALUES ('default', ?)`,
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, stmt := range stmts {
		var err error
		if stmt == `INSERT INTO projects(name, created_at) VALUES ('default', ?)` {
			_, err = tx.ExecContext(ctx, stmt, now)
		} else {
			_, err = tx.ExecContext(ctx, stmt)
		}
		if err != nil {
			return fmt.Errorf("v1 stmt failed: %w", err)
		}
	}
	return nil
}

// migrateV2 adds the events table (specs/007-architectural-foundations).
// Lands in v0.4.3 — keystone for the audit log + v0.6 time-travel + v0.5
// rollback metadata + v0.5 webhook event payloads.
//
// Schema:
//   - ts in unix milliseconds (sortable, compact, easy to bench)
//   - type: short dotted identifier like "reconciler.create" / "user.container.stop"
//   - actor: "reconciler", "subject:<id>", "system:<name>"
//   - target: "service:<project>/<name>" / "container:<id>" / "node:<id>"
//   - payload: JSON blob — type-specific fields
//
// Two indexes: ts-only for chronological scans, (target, ts) for per-target tail.
func migrateV2(ctx context.Context, tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE events (
			id      INTEGER PRIMARY KEY AUTOINCREMENT,
			ts      INTEGER NOT NULL,
			type    TEXT NOT NULL,
			actor   TEXT NOT NULL,
			target  TEXT NOT NULL,
			payload TEXT NOT NULL
		)`,
		`CREATE INDEX idx_events_ts ON events(ts)`,
		`CREATE INDEX idx_events_target_ts ON events(target, ts)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("v2 stmt failed: %w", err)
		}
	}
	return nil
}

package events

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Store appends + tails events from the SQLite events table.
// Concurrency-safe; the SQLite driver serializes writes internally.
type Store struct {
	db *sql.DB
}

// NewStore returns a Store backed by an already-opened *sql.DB. Caller
// owns the DB lifecycle. Migrations (which create the events table)
// must have run before any Append or Tail call.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Append writes one event row. Sets At = time.Now() if zero. Returns
// the auto-incremented ID.
func (s *Store) Append(ctx context.Context, e Event) (int64, error) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if e.Type == "" {
		return 0, fmt.Errorf("events: append: empty type")
	}
	if e.Actor == "" {
		return 0, fmt.Errorf("events: append: empty actor")
	}
	if e.Target == "" {
		return 0, fmt.Errorf("events: append: empty target")
	}
	if e.Payload == "" {
		e.Payload = "{}"
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO events(ts, type, actor, target, payload) VALUES (?, ?, ?, ?, ?)`,
		e.At.UnixMilli(), e.Type, e.Actor, e.Target, e.Payload)
	if err != nil {
		return 0, fmt.Errorf("events: append: %w", err)
	}
	return res.LastInsertId()
}

// Tail returns the most recent N events matching the optional filter.
// Empty target = all targets. since == zero = no lower bound.
func (s *Store) Tail(ctx context.Context, filter TailFilter) ([]Event, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	var (
		rows *sql.Rows
		err  error
	)
	switch {
	case filter.Target != "" && !filter.Since.IsZero():
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, ts, type, actor, target, payload
			FROM events
			WHERE target = ? AND ts >= ?
			ORDER BY ts DESC, id DESC
			LIMIT ?`, filter.Target, filter.Since.UnixMilli(), limit)
	case filter.Target != "":
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, ts, type, actor, target, payload
			FROM events
			WHERE target = ?
			ORDER BY ts DESC, id DESC
			LIMIT ?`, filter.Target, limit)
	case !filter.Since.IsZero():
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, ts, type, actor, target, payload
			FROM events
			WHERE ts >= ?
			ORDER BY ts DESC, id DESC
			LIMIT ?`, filter.Since.UnixMilli(), limit)
	default:
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, ts, type, actor, target, payload
			FROM events
			ORDER BY ts DESC, id DESC
			LIMIT ?`, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("events: tail: %w", err)
	}
	defer rows.Close()

	out := make([]Event, 0, limit)
	for rows.Next() {
		var e Event
		var tsMs int64
		if err := rows.Scan(&e.ID, &tsMs, &e.Type, &e.Actor, &e.Target, &e.Payload); err != nil {
			return nil, fmt.Errorf("events: scan: %w", err)
		}
		e.At = time.UnixMilli(tsMs).UTC()
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("events: rows iter: %w", err)
	}
	return out, nil
}

// TailFilter narrows a Tail query.
type TailFilter struct {
	Target string    // exact match; empty = all targets
	Since  time.Time // lower bound on ts; zero = no lower bound
	Limit  int       // default 50; max 1000
}

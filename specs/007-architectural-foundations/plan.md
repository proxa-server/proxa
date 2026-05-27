# Implementation Plan: Architectural Foundations + proxa.toml v1 (v0.4.3)

**Branch**: `007-architectural-foundations` | **Date**: 2026-05-26 | **Spec**: [spec.md](./spec.md)

## Summary

Third release of the v0.4.x Foundation Train. Lands the architectural primitives every post-v0.4.x feature consumes: events table + audit log, SQLite migrations framework, `internal/state.Snapshot` interface (stub), `internal/cluster` interfaces (single-node stub), `internal/plugin` extension interface, proxa.toml v1 formal spec with `[meta] proxa_version` gating, dashboard Events panel, and `docs/runtime-contract.md`.

Infrastructure-only — no operator-visible runtime behavior changes other than the new Events dashboard panel. Zero new production dependencies.

## Technical Context

**Language/Version**: Go 1.26.x

**Primary Dependencies**: unchanged. modernc.org/sqlite continues to back the events table + schema_migrations.

**Storage**: NEW tables in existing SQLite db: `events` (id PK, ts, type, actor, target, payload JSON, indexed by ts) + `schema_migrations` (version PK, applied_at, description).

**Testing**: unit tests for migrations idempotency, events append throughput (bench), state.Snapshot stub error behavior, cluster stub queries, plugin Hook panic recovery, TOML meta-version gating.

**Constraints**: zero new prod deps; binary size ±2%; zero-downtime upgrade from v0.4.2.

## Constitution Check

All 11 principles pass. §I (interfaces locked first), §III (events table is project-scoped via the `target` field encoding project/service), §IV (stdlib-first SQLite migrations), §V (no new deps), §VI (cluster interfaces enable v1.0 multi-host), §VII (events are declarative records of reconciliation outcomes), §VIII (additive-only schema changes), §IX (no deps means no license deltas), §X (out-of-scope list explicit in spec), §XI (one commit per logical task).

## Project Structure

```text
internal/
├── events/                           # NEW package
│   ├── doc.go
│   ├── event.go                      # Event struct + EventType constants
│   ├── store.go                      # SQLite-backed append + tail
│   ├── store_test.go
│   ├── bench_test.go                 # SC-003 sub-millisecond append
│   └── bus.go                        # pub/sub for in-process subscribers
│
├── state/                            # NEW package
│   ├── doc.go
│   ├── snapshot.go                   # Snapshot interface + ErrSnapshotNotImplemented
│   ├── stub.go                       # stub returning ErrSnapshotNotImplemented
│   └── stub_test.go
│
├── cluster/                          # NEW package
│   ├── doc.go
│   ├── cluster.go                    # StateStore + Membership + Scheduler interfaces
│   ├── singlenode.go                 # single-node stub impls
│   └── singlenode_test.go
│
├── plugin/                           # NEW package
│   ├── doc.go
│   ├── hook.go                       # Hook interface
│   ├── registry.go                   # Registry with panic recovery
│   └── registry_test.go
│
├── store/sqlite/
│   ├── migrations.go                 # MODIFIED — promote scattered CREATE TABLEs to versioned migrations
│   ├── migrations_test.go            # NEW — idempotency + lock + ordering tests
│   └── events_schema.sql             # embedded migration file (events + schema_migrations)
│
├── parser/toml/
│   ├── meta.go                       # NEW — [meta] block parser + version gating
│   ├── meta_test.go
│   └── validate.go                   # MODIFIED — surface unknown-field warnings
│
├── reconciler/
│   └── reconciler.go                 # MODIFIED — emit events on every action + transition
│
├── server/
│   ├── events_api.go                 # NEW — GET /api/v1/events
│   ├── ui_events.go                  # NEW — /ui/events handler
│   └── routes.go                     # MODIFIED — wire the two new endpoints
│
└── web/templates/
    ├── events.html                   # NEW — full-page Events table
    └── index.html                    # MODIFIED — add Events card with HTMX polling

docs/
├── proxa-toml-v1.md                  # NEW — formal TOML spec
└── runtime-contract.md               # NEW — Runtime interface contract for future backends

pkg/types/
└── taskdef.go                        # MODIFIED — add optional Meta struct with ProxaVersion field
```

## Phase 0 — Research

Three decisions to lock:

- **R-001 Events schema**: single `events` table for system + user events (vs separate audit_log table). Decision: single table; distinguished by `actor` field prefix (`reconciler:`, `subject:`, `system:`). Simpler queries, single index on ts.
- **R-002 Migrations framework**: hand-rolled in-tree (~150 LOC) vs an existing library (e.g., golang-migrate, goose). Decision: hand-rolled — §V forbids new prod deps, and our migration needs are simple (linear version-numbered SQL files embedded via `go:embed`).
- **R-003 Plugin Hook delivery**: synchronous in-line vs async via channel + goroutine. Decision: async via buffered channel; Registry owns the dispatch goroutine. Plugins can't block reconciler/server hot path. Lost events under back-pressure get logged but dropped (acceptable for v0.4.3 stub).

## Phase 1 — Design

### Events schema (migration 001_events)

```sql
CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,        -- unix milliseconds
  type TEXT NOT NULL,         -- e.g. "reconciler.create", "probe.transition", "user.container.stop"
  actor TEXT NOT NULL,        -- e.g. "reconciler", "subject:bootstrap-admin"
  target TEXT NOT NULL,       -- e.g. "service:default/api", "container:abc123"
  payload TEXT NOT NULL       -- JSON blob
);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts);
CREATE INDEX IF NOT EXISTS idx_events_target_ts ON events(target, ts);

CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at INTEGER NOT NULL,
  description TEXT NOT NULL
);
```

### Interface signatures

```go
// internal/state/snapshot.go
type SnapshotID string

type Snapshot interface {
    Take(ctx context.Context, project string) (SnapshotID, error)
    Load(ctx context.Context, id SnapshotID) (*ProjectState, error)
    Diff(a, b SnapshotID) (Diff, error)
    Restore(ctx context.Context, id SnapshotID) error
}

var ErrSnapshotNotImplemented = errors.New("state: snapshot not implemented (v0.4.3 stub; real impl in v0.5)")

// internal/cluster/cluster.go
type Node struct { ID string; Addr string; Labels map[string]string; LastSeen time.Time }

type Membership interface {
    Self(ctx context.Context) (Node, error)
    List(ctx context.Context) ([]Node, error)
    Subscribe(ctx context.Context) (<-chan MembershipEvent, error)
}

type StateStore interface {
    Get(ctx context.Context, key string) ([]byte, error)
    Put(ctx context.Context, key string, value []byte) error
    Delete(ctx context.Context, key string) error
    Watch(ctx context.Context, prefix string) (<-chan WatchEvent, error)
}

type Scheduler interface {
    Place(ctx context.Context, taskID string, constraints map[string]string) (nodeID string, err error)
}

// internal/plugin/hook.go
type Hook interface {
    Name() string
    OnEvent(ctx context.Context, e Event) error
}

type Registry interface {
    Register(Hook) error
    Publish(Event)
    Close() error
}
```

## Phase 2 — Tasks

See tasks.md for the per-task breakdown.

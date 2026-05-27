---
description: "Task list for 007-architectural-foundations (v0.4.3)"
---

# Tasks: Architectural Foundations + proxa.toml v1 (v0.4.3)

Pragmatic batching — this release is wide (many small interfaces) so commit logically by package.

## Phase 1: Schema + migrations (US2 foundation)

- [ ] T001 Implement `internal/store/sqlite/migrations.go` framework + `schema_migrations` table + embedded SQL files via `go:embed` + exclusive-lock on apply.
  - **Commit**: `feat(store): add SQLite schema migrations framework with embedded SQL files`
- [ ] T002 Add migration 0001_events.sql creating `events` table + indices on ts and target.
  - **Commit**: `feat(store): add migration 0001_events for the events table`
- [ ] T003 Unit tests for migrations: idempotency, version ordering, partial-failure rollback, exclusive-lock contention.
  - **Commit**: `test(store): cover migrations idempotency + lock contention`

## Phase 2: Events package (US1)

- [ ] T004 [US1] Create `internal/events` package: `Event` struct + EventType constants + `Store` interface backed by SQLite.
  - **Commit**: `feat(events): add events package with SQLite-backed append + tail`
- [ ] T005 [US1] Unit tests for events append + tail + benchmark for sub-millisecond append at 100k rows.
  - **Commit**: `test(events): cover append/tail + bench append throughput (SC-003)`
- [ ] T006 [US1] In-process `Bus` for pub/sub subscribers (reconciler + dashboard subscribe; plugins via registry).
  - **Commit**: `feat(events): add in-process Bus pub/sub for subscribers`

## Phase 3: State interface stub (US3)

- [ ] T007 [US3] Create `internal/state` package: Snapshot interface + ErrSnapshotNotImplemented + ProjectState/Diff/SnapshotID types.
  - **Commit**: `feat(state): add Snapshot interface + stub returning ErrSnapshotNotImplemented`
- [ ] T008 [US3] Unit tests for stub behavior (every method returns sentinel error; never panics).
  - **Commit**: `test(state): cover snapshot stub sentinel-error behavior`

## Phase 4: Cluster interfaces + single-node stub (US4)

- [ ] T009 [US4] Create `internal/cluster` package: Node + Membership + StateStore + Scheduler interfaces + types.
  - **Commit**: `feat(cluster): add Membership/StateStore/Scheduler interfaces`
- [ ] T010 [US4] Single-node stub implementations of each interface (Self returns local node, List returns [self], Place always returns local node id).
  - **Commit**: `feat(cluster): add single-node stub implementations`
- [ ] T011 [US4] Unit tests for stubs.
  - **Commit**: `test(cluster): cover single-node stub queries`

## Phase 5: Plugin extension interface (US5)

- [ ] T012 [US5] Create `internal/plugin` package: Hook + Registry interfaces + buffered-channel async delivery + panic recovery.
  - **Commit**: `feat(plugin): add Hook/Registry interfaces with panic-safe async delivery`
- [ ] T013 [US5] Unit tests for Registry (publish/subscribe roundtrip, panic recovery, back-pressure drop).
  - **Commit**: `test(plugin): cover registry pub/sub, panic recovery, back-pressure`

## Phase 6: proxa.toml v1 spec (US6)

- [ ] T014 [US6] Extend `pkg/types/taskdef.go` with optional `Meta` struct (`ProxaVersion string` field).
  - **Commit**: `feat(types): add optional [meta] block to TaskDef`
- [ ] T015 [US6] Update `internal/parser/toml` to parse `[meta]` + enforce version gating with clear errors.
  - **Commit**: `feat(parser/toml): version-gate TOML against running binary`
- [ ] T016 [US6] Update parser to emit warning (not error) for unknown fields, naming the field.
  - **Commit**: `feat(parser/toml): warn on unknown fields without failing the parse`
- [ ] T017 [US6] Unit tests for meta version gating + unknown-field warning.
  - **Commit**: `test(parser/toml): cover [meta] version gating + unknown-field warnings`
- [ ] T018 [US6] Author `docs/proxa-toml-v1.md` with the formal spec — every block, every field, version-introduced column.
  - **Commit**: `docs(toml): publish proxa-toml-v1 formal spec`

## Phase 7: Reconciler event emission (US1 integration)

- [ ] T019 [US1] Wire `events.Store` into reconciler; emit events on every create/start/stop/remove + service status change + probe transition.
  - **Commit**: `feat(reconciler): emit events for actions + status changes + probe transitions`

## Phase 8: Dashboard Events panel (FR-019, dashboard parity)

- [ ] T020 Create `internal/server/events_api.go` with `GET /api/v1/events?target=&since=&limit=` handler.
  - **Commit**: `feat(server): add GET /api/v1/events handler`
- [ ] T021 Create `internal/server/ui_events.go` + `internal/web/templates/events.html` for full-page events viewer.
  - **Commit**: `feat(web): add /ui/events full-page events viewer`
- [ ] T022 Extend `internal/web/templates/index.html` dashboard with an Events card (last 10, polls every 5s).
  - **Commit**: `feat(web): add Events card to dashboard index polling /api/v1/events`
- [ ] T023 End-to-end test for events surface: deploy a service, query /api/v1/events, verify reconciler.create event present.
  - **Commit**: `test(e2e): cover events table + /api/v1/events + dashboard Events card (SC-001 / SC-011)`

## Phase 9: Runtime contract polish (FR-018)

- [ ] T024 Author `docs/runtime-contract.md` documenting every Runtime interface method, expected error semantics, expected concurrency safety.
  - **Commit**: `docs(runtime): publish Runtime interface contract for future backends`

## Phase 10: Polish + validation

- [ ] T025 Refresh `docs/licenses.md` — no-op refresh confirming zero new modules for 007.
  - **Commit**: `docs(licenses): refresh transitive license audit for 007 (no-op)`
- [ ] T026 Refresh `docs/operations.md` with Events panel usage + retention guidance + migrations operator-visible state.
  - **Commit**: `docs(operations): add v0.4.3 sections — Events panel + migrations operator notes`
- [ ] T027 Create `specs/007-architectural-foundations/validation.md` with SC-by-SC pass/fail table.
  - **Commit**: `docs(spec): record validation results for 007-architectural-foundations`

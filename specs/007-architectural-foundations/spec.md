# Feature Specification: Architectural Foundations + proxa.toml v1 (v0.4.3)

**Feature Branch**: `007-architectural-foundations`

**Created**: 2026-05-26

**Status**: Draft

**Input**: User description: v0.4.3 — third release of the v0.4.x Foundation Train, and the LAST one before v0.5 Multi-host MVP arrives. This release lands the architectural primitives that v0.5 + v0.6 + v0.7 all depend on, so that none of those releases has to refactor the codebase mid-feature. It is internal infrastructure — no operator-visible behavior changes — but it is the most important release in the foundation train because EVERY downstream feature consumes one of its contracts.

Per the project roadmap (`project_roadmap_v04_to_v10`): event store table in SQLite (single table for both reconciler events AND user-action audit log), SQLite schema migrations framework, `internal/state` Snapshot interface (stub here, real impl in v0.5), Runtime interface contract polish + `docs/runtime-contract.md`, `internal/cluster` StateStore/Membership/Scheduler interfaces with single-node stub (prep for v0.5 multi-host), `internal/plugin` extension interface (substrate needs this from day one per the competitive landscape memory), `[meta] proxa_version` config versioning + compatibility warnings, `internal/events/bus.go` pub/sub, **proxa.toml v1 formal spec with versioned ABI**.

This is intentionally a LARGE release in code volume but SMALL in operator-visible surface. Every change is a foundation other features will build on.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reconciler emits events that survive process restart (Priority: P1)

An operator does a deploy via `proxa up myservice.toml`. The reconciler creates a container; a probe transitions it to healthy; some time later, an operator wonders "what happened to my-service in the last 30 minutes?" Today, the only answer is `journalctl -u proxa | grep my-service`. After this release, there's an `events` table in SQLite recording every reconciler action + probe transition + user write, queryable via SQL or (in v0.6 Post-Mortem Mode) via a dashboard scrub-bar. The event store also serves as the audit log for v0.4.4's container-action writes (per `feedback_container_actions_scope`).

**Why this priority**: Without the events table, v0.4.4 cannot emit audit log entries (FR-007 of feature 008), v0.5 cannot record deploy snapshots for rollback, v0.6 cannot do time-travel logs, and v0.7 cannot record GitOps webhook deliveries. This is the keystone of the post-v0.4.x feature work.

**Independent Test**: Run `proxa server`, `proxa up svc.toml`, wait for healthy, run `proxa events tail --service=svc` (or query the table directly via `sqlite3 ~/.proxa/proxa.db 'SELECT * FROM events ORDER BY ts DESC LIMIT 10'`). At least three events visible: reconciler.create, probe.transition, service.status_change.

**Acceptance Scenarios**:

1. **Given** a fresh `proxa init` data dir, **When** an operator runs `proxa up` for a new service, **Then** the events table contains at least one `reconciler.create` event referencing the service.
2. **Given** a service whose probe transitions from failing to healthy, **When** the operator queries the events table, **Then** a `probe.transition` event records the timestamp + container ID + new state.
3. **Given** the events table contains 10,000+ rows, **When** the reconciler appends a new event, **Then** the append is sub-millisecond (no index rebuild or table scan).
4. **Given** the events table approaches a configurable retention limit, **When** the retention sweeper runs, **Then** events older than the limit are pruned without blocking writes.

---

### User Story 2 - SQLite schema evolves without manual operator intervention (Priority: P1)

An operator upgrades from v0.4.2 to v0.4.3. The v0.4.2 data dir has the existing schema (services, subjects, etc.); v0.4.3 needs to ADD the `events` table + an extra column on `service` for the active snapshot ID (preparation for v0.5 rollback). After `proxa server` starts, the schema is automatically migrated to v0.4.3's shape, the existing data is preserved verbatim, and the migration version is recorded so the operator can audit the upgrade path.

**Why this priority**: Without a migrations framework, every schema change becomes a manual data-dir wipe-and-reinit cycle for operators. v0.5+ will accumulate schema changes (snapshots, rollback metadata, multi-host cluster state). Lock the framework now; every subsequent release defines one migration file.

**Independent Test**: Take a `proxa.db` from v0.4.2 (with services + subjects + projects). Start v0.4.3 against it. Verify (a) old data preserved, (b) `schema_migrations` table records the migration ran, (c) the new events table exists and is empty.

**Acceptance Scenarios**:

1. **Given** a v0.4.2 data dir with services + subjects, **When** `proxa server` starts on v0.4.3, **Then** existing rows are preserved AND new tables/columns are added in a single transaction.
2. **Given** a partial migration failure (e.g., disk full mid-migration), **When** the migration is rolled back, **Then** the schema returns to its pre-migration state and the server refuses to start with a clear error.
3. **Given** a future v0.5 that adds another migration, **When** the operator upgrades v0.4.3 → v0.5, **Then** only the new migration runs (idempotent — never re-runs already-applied migrations).
4. **Given** a fresh `proxa init` on v0.4.3, **When** the database is created, **Then** all migrations apply in order and the `schema_migrations` table shows every version landed.

---

### User Story 3 - Future v0.5 Snapshot consumers compile against a stable interface (Priority: P1)

A contributor working on v0.5 Confidence Mode wants to implement `proxa rollback`. The `internal/state.Snapshot` interface ALREADY exists in v0.4.3 (stub implementation only — capture returns ErrNotImplemented). The contributor adds the real implementation; every caller (reconciler, CLI rollback command, dashboard timeline) compiles unchanged because the interface signature was locked in v0.4.3.

**Why this priority**: This is the "lock the contract before the consumer arrives" pattern. v0.5 Confidence Mode (rollback + diff) consumes Snapshot. If we shipped v0.5 with the interface and the impl simultaneously, the interface might be shaped wrong and we'd refactor mid-release. Lock now.

**Independent Test**: `grep -rn "state.Snapshot" internal/` shows the interface is imported by at least one downstream package (e.g., the future `internal/cli/rollback.go` stub). The interface methods + their godoc lock the contract.

**Acceptance Scenarios**:

1. **Given** the `internal/state` package post-v0.4.3, **When** a contributor inspects `state.Snapshot`, **Then** the interface defines `Take`, `Load`, `Diff`, `Restore` methods with the exact signatures v0.5 will implement.
2. **Given** the stub implementation, **When** any caller invokes a Snapshot method in v0.4.3, **Then** the call returns a sentinel error (`state.ErrSnapshotNotImplemented`) — never panics, never silently no-ops.
3. **Given** v0.5 lands the real implementation, **When** existing v0.4.3 callers run unchanged against it, **Then** they compile without source changes — only the imported package's binary changes.

---

### User Story 4 - Future v1.0 Multi-host work compiles against stable cluster interfaces (Priority: P2)

A contributor (or future-Jearel) starting v1.0 cluster work finds `internal/cluster` with StateStore + Membership + Scheduler interfaces ALREADY defined. The single-node stub implementation exists; v1.0 swaps in embedded-etcd + agent gossip without touching the consumers. The "Multi-host MVP" of v0.5 uses the SAME interfaces with a slim implementation (central SQLite as the cluster state).

**Why this priority**: Multi-host is THE killer feature per `feedback_multi_host_is_killer_feature`. Locking the cluster interfaces now lets v0.5 ship multi-host without redesigning the abstractions. v1.0 then upgrades the IMPLEMENTATIONS without re-shaping the consumers.

**Independent Test**: `grep -rn "cluster.StateStore\|cluster.Membership\|cluster.Scheduler" internal/` shows the interfaces are referenced (even if only by the single-node stub) — the package exists and exports the expected types.

**Acceptance Scenarios**:

1. **Given** the `internal/cluster` package post-v0.4.3, **When** a contributor inspects the exported types, **Then** StateStore + Membership + Scheduler interfaces exist with documented signatures.
2. **Given** the single-node stub, **When** the reconciler queries cluster membership in single-node mode, **Then** the stub returns "one node, self" without surprises.
3. **Given** v0.5 multi-host MVP, **When** the agent connects, **Then** the Membership.Add call works through the same interface (different implementation).

---

### User Story 5 - PaaS-layer tools build on top of Proxa via a stable plugin interface (Priority: P2)

A future PaaS-layer tool (Coolify-on-Proxa hypothetically, or an internal Socio.do utility) wants to hook into Proxa's event stream + state without modifying Proxa source. The `internal/plugin` interface (defined in v0.4.3 with stub support, fleshed out in later releases) gives them a stable extension point: subscribe to events, register custom subjects, expose health endpoints. Substrate positioning (per `project_competitive_landscape`) requires this exists from day one — otherwise every PaaS-layer integration is a fork.

**Why this priority**: Substrate-vs-PaaS is the strategic positioning. Without a plugin interface, "PaaS layers can build on top" is marketing not architecture. P2 because no PaaS layer exists yet — but locking the interface now means when one does, integration is days not months.

**Independent Test**: `grep -rn "plugin.Hook\|plugin.Registry" internal/` shows the interface exists. The README's substrate-positioning paragraph can point to it as evidence.

**Acceptance Scenarios**:

1. **Given** the `internal/plugin` package post-v0.4.3, **When** a contributor inspects the exported types, **Then** Hook + Registry interfaces exist with godoc describing the extension model.
2. **Given** the stub Registry, **When** an internal subsystem (reconciler, server) emits an event, **Then** the event is delivered to all registered Hooks without affecting the originating subsystem's hot path.
3. **Given** a hypothetical PaaS-layer tool, **When** they inspect Proxa's plugin contract, **Then** the contract is sufficient to build event-driven integrations without forking Proxa.

---

### User Story 6 - proxa.toml format is versioned and forward-compatible (Priority: P2)

An operator wrote a service TOML against v0.4.0. After upgrading through v0.4.1 → v0.4.2 → v0.4.3 their TOML still works. v0.5 adds an optional `[autoscale]` block — operators who don't set it stay on v0.4.x semantics. If an operator sets a v1.0 field on a v0.5 binary, they get a clear "field requires v1.0+" error, not a silent ignore. The proxa.toml format has a formal versioned ABI starting in v0.4.3.

**Why this priority**: Without ABI versioning, every TOML field addition is a potential silent regression for operators on older binaries. Lock the format spec NOW (one file: `docs/proxa-toml-v1.md`) so the rules are clear: outer block shape locked, inner fields evolve.

**Independent Test**: `docs/proxa-toml-v1.md` exists with the formal spec; the parser logs a warning when it encounters an unknown TOML field above the supported version.

**Acceptance Scenarios**:

1. **Given** a v0.4.0 service TOML, **When** the operator runs `proxa up` on v0.4.3, **Then** the deploy succeeds with no warnings (backwards compatible).
2. **Given** a TOML with `[meta] proxa_version = "0.4.3"`, **When** the operator runs against a binary older than v0.4.3, **Then** the operator sees a clear "TOML requires proxa v0.4.3+ but running v0.4.2" error before any container creation.
3. **Given** a TOML field unknown to the running binary, **When** the operator runs `proxa up`, **Then** they see a warning naming the unknown field + the recommendation to either remove it or upgrade.

---

### Edge Cases

- What happens when two `proxa server` processes start against the same data dir simultaneously? **Expected**: schema migrations use a `BEGIN EXCLUSIVE` lock; second process waits for first to finish or fails with a clear "another proxa-server is initializing this data dir" error. No double-migration, no corruption.
- What happens when the events table grows to 1M rows? **Expected**: writes stay sub-millisecond (INSERT with auto-incrementing PK + indexed timestamp). Reads use `LIMIT N` + the timestamp index. Retention sweeper can be invoked but is opt-in (no auto-prune in v0.4.3).
- What happens when a contributor adds a Snapshot method without updating the stub? **Expected**: stub's compile breaks. Tests for the stub catch the missing method.
- What happens when an operator opens a v0.4.3 data dir with a v0.4.2 binary? **Expected**: v0.4.2 doesn't know about the new tables / columns, but reads existing tables fine. The `[meta] proxa_version` warning fires if their TOML claims v0.4.3.
- What happens when a plugin Hook panics? **Expected**: recovered with a logged error; the originating subsystem is not affected.

## Requirements *(mandatory)*

### Functional Requirements

**Event store + audit log (US1):**

- **FR-001**: System MUST persist every reconciler action (create/start/stop/remove/scale), probe transition, user-initiated write, and service status change as a row in an `events` table.
- **FR-002**: Each event MUST record: ts, type, actor (`reconciler` or `subject:<id>`), target (e.g., `container:<id>` or `service:<project>/<name>`), and a JSON payload with type-specific fields.
- **FR-003**: Event appends MUST be sub-millisecond at p99 even with 100,000+ existing rows.
- **FR-004**: System MUST provide a retention sweeper that removes events older than a configurable cutoff. Retention is opt-in via config; default is "never prune".

**SQLite schema migrations (US2):**

- **FR-005**: System MUST apply schema migrations on `proxa server` startup, in version order, idempotently.
- **FR-006**: System MUST refuse to start if a migration fails, leaving the schema in its pre-failure state.
- **FR-007**: System MUST record applied migrations in a `schema_migrations` table so operators can audit the upgrade path.
- **FR-008**: System MUST hold an exclusive lock during migrations so concurrent `proxa server` startups cannot race.

**State Snapshot interface (US3):**

- **FR-009**: System MUST export `state.Snapshot` interface from `internal/state` with `Take`, `Load`, `Diff`, `Restore` methods whose signatures are committed for v0.5 consumption.
- **FR-010**: System MUST provide a stub implementation that returns `state.ErrSnapshotNotImplemented` for every method, so callers can compile against the interface immediately.

**Cluster interfaces (US4):**

- **FR-011**: System MUST export `cluster.StateStore`, `cluster.Membership`, `cluster.Scheduler` interfaces from `internal/cluster` with documented signatures.
- **FR-012**: System MUST provide single-node stub implementations that return the local node for membership queries + accept all schedule decisions for the local node.

**Plugin interface (US5):**

- **FR-013**: System MUST export `plugin.Hook` and `plugin.Registry` types from `internal/plugin` with documented event-delivery semantics.
- **FR-014**: The plugin Registry MUST recover from panics in Hook callbacks without affecting the publishing subsystem.

**proxa.toml v1 spec (US6):**

- **FR-015**: System MUST publish a formal proxa.toml v1 specification at `docs/proxa-toml-v1.md` documenting every supported block + field + version-introduced.
- **FR-016**: The TOML parser MUST recognize an optional `[meta] proxa_version` field and refuse to run against a TOML that requires a newer binary than the running one.
- **FR-017**: The TOML parser MUST log a warning (NOT fail) when it encounters an unknown field, naming the field + suggesting to remove it or upgrade.

**Runtime contract polish (cross-cutting):**

- **FR-018**: System MUST publish a `docs/runtime-contract.md` documenting the `internal/runtime.Runtime` interface contract for the apple/container + podman backends arriving in v0.9 + v1.0.

**Dashboard parity (cross-cutting):**

- **FR-019**: The dashboard MUST surface a read-only Events panel showing the last 50 events with type + actor + target + age. Refreshes every 5s like the existing Services/Routes cards.

**Cross-cutting:**

- **FR-020**: This release MUST NOT introduce any new third-party module dependencies (no new tool-directives either, unless required for the migrations framework).
- **FR-021**: The release MUST preserve zero-downtime upgrade from v0.4.2: existing tokens stay valid, the bootstrap admin token unchanged, on-disk format is ADDITIVE only.

### Key Entities

- **Event** (NEW table `events`): id INT PK, ts INT (unix ms), type TEXT, actor TEXT, target TEXT, payload TEXT (JSON). Indexed by ts.
- **SchemaMigration** (NEW table `schema_migrations`): version INT PK, applied_at INT, description TEXT.
- **state.Snapshot interface** (NEW): Take/Load/Diff/Restore methods locked.
- **cluster.* interfaces** (NEW): StateStore + Membership + Scheduler signatures locked.
- **plugin.Hook / plugin.Registry** (NEW): event-delivery extension point.
- **TOML meta block** (NEW field on existing TaskDef): optional `[meta] proxa_version` for version gating.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can query the events table after a deploy and see at least one reconciler.create event within 10 seconds.
- **SC-002**: An operator upgrading from v0.4.2 → v0.4.3 sees their existing services + subjects intact, the schema_migrations table populated, and the events table present and empty.
- **SC-003**: An automated benchmark confirms event appends at p99 < 1ms with 100k existing rows.
- **SC-004**: A contributor inspecting `state.Snapshot` finds all four method signatures committed.
- **SC-005**: A contributor inspecting `internal/cluster` finds StateStore + Membership + Scheduler interfaces with single-node stub implementations.
- **SC-006**: A contributor inspecting `internal/plugin` finds Hook + Registry types with documented event-delivery contract.
- **SC-007**: An operator with a v0.4.0 service TOML can run `proxa up` against v0.4.3 successfully (backwards-compatible).
- **SC-008**: An operator with a TOML containing an unknown field sees a warning naming the field, NOT a fatal error.
- **SC-009**: `docs/proxa-toml-v1.md` exists and documents every currently-supported block.
- **SC-010**: `docs/runtime-contract.md` exists and documents every method of the Runtime interface.
- **SC-011**: The dashboard's Events panel renders the most recent events with the existing 5s polling cadence.
- **SC-012**: The release adds zero new production binary dependencies; binary size delta within ±2% of v0.4.2.

## Assumptions

- Go toolchain stays at 1.26.x.
- No new third-party module dependencies in the production binary.
- The `events` table replaces the need for a separate audit log table (single events table for both system + user actions, distinguished by actor field).
- Retention policy in v0.4.3 is "never prune by default"; opt-in retention rule arrives with v0.6 Post-Mortem Mode.
- The Snapshot stub returns `ErrSnapshotNotImplemented`; real implementation lands with v0.5 Confidence Mode.
- The cluster single-node stub satisfies v0.5 Multi-host MVP's needs for the central-control-plane mode; v1.0 swaps in embedded etcd.
- The plugin interface is stub-only in v0.4.3 (no Hook implementations ship); usable by v0.9+ when first PaaS-layer interest emerges.
- proxa.toml v1 spec doc captures the CURRENT format; future versions are appended sections.
- Dashboard Events panel is read-only; clicking an event link to "show this event in context" arrives with v0.6 Post-Mortem Mode.

Out-of-scope for v0.4.3 (documented to prevent reopening):
- Real Snapshot implementation (v0.5)
- Real cluster.StateStore impl with embedded etcd (v1.0; v0.5 ships a different slim impl)
- Real Hook implementations / plugin marketplace (future, demand-driven)
- Event retention sweeper auto-prune (v0.6)
- Event-to-webhook delivery (v0.5 webhook egress)
- Schema migration rollback (one-way migrations only in v0.4.3)
- TOML v2 with breaking changes (no breaking changes planned for v0.x)

## Dependencies

- Builds on v0.4.2 (Test Foundation + Public Images).
- **Prerequisite for v0.4.4** Container UI: events table is what audit-log entries write into.
- **Prerequisite for v0.5.0** Multi-user slim: events table is the audit log destination for write actions.
- **Prerequisite for v0.5** Multi-host MVP: cluster interfaces define the multi-node abstractions.
- **Prerequisite for v0.5.1** Confidence Mode: state.Snapshot interface is what rollback consumes.
- **Prerequisite for v0.6** Post-Mortem Mode: events table is the time-travel data source.
- **Prerequisite for v0.7** GitOps + DX: events emit webhook payloads.

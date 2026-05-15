# Phase 1 — Data Model: SQLite Schema + Runtime Labels

The seven entities from `pkg/types/` already define the in-memory shape (see `specs/000-foundation/data-model.md`). This document captures their *persistence representation* in SQLite and their *runtime representation* in Docker container labels.

---

## SQLite schema (v1)

All tables sit in a single file at `${PROXA_DATA_DIR}/proxa.db` opened in WAL mode (`PRAGMA journal_mode = WAL`) with `PRAGMA foreign_keys = ON`.

```sql
-- ─── schema_version ────────────────────────────────────────────────────────
CREATE TABLE schema_version (
    version  INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL  -- RFC 3339
);

-- ─── projects ──────────────────────────────────────────────────────────────
CREATE TABLE projects (
    name        TEXT PRIMARY KEY,           -- ^[a-z0-9][a-z0-9-]{0,62}$
    created_at  TEXT NOT NULL
) WITHOUT ROWID;

-- The "default" project is inserted by Migrate() on the v1 migration.

-- ─── services ──────────────────────────────────────────────────────────────
CREATE TABLE services (
    project     TEXT NOT NULL REFERENCES projects(name) ON DELETE RESTRICT,
    name        TEXT NOT NULL,
    id          TEXT NOT NULL UNIQUE,        -- KSUID
    spec_json   TEXT NOT NULL,               -- canonical JSON of types.TaskDef
    spec_hash   TEXT NOT NULL,               -- sha256:... matches container label
    status      TEXT NOT NULL,               -- ServiceStatus enum value
    replicas_json TEXT NOT NULL DEFAULT '[]',-- []ReplicaState
    history_json  TEXT NOT NULL DEFAULT '[]',-- []DeploymentRecord (capped at 20 entries)
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (project, name)
) WITHOUT ROWID;

CREATE INDEX idx_services_status ON services(status);

-- ─── jobs ──────────────────────────────────────────────────────────────────
CREATE TABLE jobs (
    project     TEXT NOT NULL REFERENCES projects(name) ON DELETE RESTRICT,
    name        TEXT NOT NULL,
    id          TEXT NOT NULL UNIQUE,
    spec_json   TEXT NOT NULL,
    last_run_json TEXT,                      -- nullable; types.JobRun
    next_run_at TEXT,                        -- nullable; RFC 3339
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (project, name)
) WITHOUT ROWID;

CREATE INDEX idx_jobs_next_run ON jobs(next_run_at);

-- ─── nodes ─────────────────────────────────────────────────────────────────
CREATE TABLE nodes (
    id              TEXT PRIMARY KEY,        -- KSUID; matches proxa.node label
    name            TEXT NOT NULL,
    role            TEXT NOT NULL,           -- 'server' | 'agent'
    address         TEXT NOT NULL,
    cpu_cores       INTEGER NOT NULL,
    memory_mb       INTEGER NOT NULL,
    disk_gb         INTEGER NOT NULL,
    container_count INTEGER NOT NULL DEFAULT 0,
    last_heartbeat  TEXT NOT NULL,
    status          TEXT NOT NULL,           -- 'ready' | 'draining' | 'offline'
    labels_json     TEXT NOT NULL DEFAULT '{}'
) WITHOUT ROWID;

CREATE INDEX idx_nodes_status ON nodes(status);

-- ─── subjects ──────────────────────────────────────────────────────────────
CREATE TABLE subjects (
    id            TEXT PRIMARY KEY,           -- KSUID
    name          TEXT NOT NULL,
    email         TEXT,
    provider      TEXT NOT NULL,              -- 'local' | 'bootstrap' | 'oidc:...' | 'agent'
    metadata_json TEXT NOT NULL DEFAULT '{}',
    secret_hash   TEXT,                        -- bcrypt(token) for bootstrap; bcrypt(password) for local
    created_at    TEXT NOT NULL
) WITHOUT ROWID;

CREATE INDEX idx_subjects_provider ON subjects(provider);

-- ─── policies ──────────────────────────────────────────────────────────────
CREATE TABLE policies (
    id          TEXT PRIMARY KEY,             -- KSUID
    subject_id  TEXT NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
    role        TEXT NOT NULL,                -- 'admin' | 'editor' | 'viewer' | 'agent'
    project     TEXT NOT NULL,                -- '*' for admin only; otherwise must exist in projects.name
    created_at  TEXT NOT NULL
) WITHOUT ROWID;

CREATE INDEX idx_policies_subject ON policies(subject_id);
CREATE INDEX idx_policies_project_role ON policies(project, role);
```

### Migration v1 inserts

```sql
INSERT INTO schema_version (version, applied_at) VALUES (1, '<RFC3339-now>');
INSERT INTO projects (name, created_at) VALUES ('default', '<RFC3339-now>');
```

The admin subject + bootstrap-token subject are inserted by `proxa init`, NOT by the schema migration (so re-running `Migrate` on an existing DB doesn't clobber them).

### Constraints honored at the StateStore impl level (not DB)

- `Project="*"` is allowed in `policies` only when `Role="admin"`. Enforced in `sqliteStore.PutPolicy`; CHECK constraint avoided to keep migration v1 simple.
- `Service.Spec.Project` (inside `spec_json`) must equal the row's `project` column. Enforced at write time.
- The `spec_hash` column is recomputed by the impl on every PutService — never trusted from the caller.

### Why `WITHOUT ROWID`?

For tables keyed on a string PK that we always look up by exact key (projects, services by `(project, name)`, etc.), `WITHOUT ROWID` saves one B-tree level and one row-id column. Standard SQLite optimization for catalog-style tables. `nodes` and `subjects` use `WITHOUT ROWID` for the same reason; `jobs` and `policies` use it for consistency.

---

## Container labels (the runtime side)

Every container created by Proxa carries the label set defined in `internal/runtime/docker/labels.go`:

```go
package labels

const (
    Managed    = "proxa.managed"     // value: always "true"
    Project    = "proxa.project"     // value: project name
    Service    = "proxa.service"     // value: service name within project
    Replica    = "proxa.replica"     // value: 0-based replica index, decimal string
    SpecHash   = "proxa.spec_hash"   // value: "sha256:..." matches services.spec_hash
    NodeID     = "proxa.node"        // value: nodes.id; "node-local" in single-node v0.0
    CreatedAt  = "proxa.created_at"  // value: RFC 3339 UTC; informational only
)
```

`Runtime.ListContainers(ctx, ListFilter{Project: "X"})` adds:
- Filter `label=proxa.managed=true` (excludes foreign containers)
- Filter `label=proxa.project=<X>` (project scoping)

The Docker name follows FR-015: `proxa-{project}-{service}-{replica}`. Naming and labels are *both* set so that:
- Lookup by name is O(1) for the reconciler ("does container `proxa-default-web-2` exist?")
- Bulk filtering by label is O(N) but indexed by Docker ("list all containers in project default")

### Why both name and labels?

The Docker daemon enforces unique names within the local engine, so name collisions surface immediately. Labels alone could collide (an operator could conceivably manually create a container with the same labels). Name uniqueness is our authoritative ownership claim; labels are the queryable index.

---

## State transitions

### Service lifecycle (in `services.status`)

```
                ┌─────────┐
                │ pending │   PutService(...) inserts here
                └────┬────┘
                     │ first reconcile tick reads desired
                     ↓
              ┌─────────────┐
              │ reconciling │ ← any time replicas != desired
              └─────┬───────┘
              ┌─────┴─────┬──────────────────┐
              ↓           ↓                  ↓
       ┌──────────┐ ┌──────────┐ ┌──────────┐
       │ healthy  │ │ degraded │ │  failed  │
       └────┬─────┘ └────┬─────┘ └────┬─────┘
            │            │            │
            └────────────┴────────────┘
                         │ replica count drifts → reconciling
                         ↓
                   (loops back)
```

- `healthy`: actual replicas == desired AND every replica's `Phase=running`.
- `degraded`: actual replicas == desired BUT at least one replica `Phase != running` (e.g., `starting`).
- `failed`: actual < desired AND the last reconciler tick was unable to bring it back up (e.g., image pull failure). Recoverable; next tick retries.

### Job lifecycle (in `jobs.last_run_json.status`)

`running` → (`succeeded` | `failed` | `timeout`). One JobRun at a time per job in v0.0.

### Node lifecycle (in `nodes.status`)

`ready` → `draining` (operator initiated, drain workloads) → `offline` (heartbeat gap > N intervals).

---

## Cardinality + capacity

Per node in v0.0 (per Plan's Scale/Scope):
- ≤10 projects
- ≤20 services per project (so ≤200 services total)
- Replicas per service typically ≤10
- ≤100 containers running simultaneously

SQLite handles this trivially; no partitioning or indexing concerns at this scale.

---

## What's intentionally NOT in the schema

These will land with their respective features:

- `secrets` — Feature 005.
- `config_maps` — Feature 006.
- `ingress_routes` — Feature 003.
- `health_check_status` — Feature 002 (will be a column added to `services.replicas_json[].HealthOK`, already present in the type but unused in 001).
- `audit_log` — separate concern; possibly its own feature or a cross-cutting addition during hardening.
- `event_stream` — for `WatchServices` push notifications. v0.0 implementation polls; etcd in v1.0 enables real watches.

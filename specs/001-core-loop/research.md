# Phase 0 — Research: Core Loop

Ten decisions resolved before Phase 1 design. Each captures the *why* and the rejected alternatives so future contributors don't re-litigate.

---

## R-001: Daemon model — `proxa server` foreground process

**Decision**: A long-running `proxa server` subcommand hosts the reconciler and the HTTP API. CLI commands (`up`, `down`, `ps`) are clients of the API.

**Rationale**: Spec SC-002 ("reconciler restarts crashed container within 10s") is unsatisfiable without a persistent process. The daemon is not optional; making it explicit (named `server`, runs in the foreground or under systemd/launchd) keeps the architecture transparent.

**Alternatives rejected**:
- **CLI commands invoke a one-shot reconcile and exit** — fails SC-002. A crashed container would never restart.
- **Daemonize via fork/setsid inside `proxa init`** — hides the daemon's lifecycle from the operator. Hard to debug, no unified log destination, no way to integrate with system supervisors.
- **systemd timer + `proxa reconcile-once`** — pushes lifecycle management out of the binary, violates §V (single binary, zero external infra), and adds a 5s+ jitter on top of the OS timer.

The operator choice for production: run `proxa server` under systemd (`/etc/systemd/system/proxa.service`) or launchd. For dev: just run it in a terminal. Documented in `quickstart.md`.

---

## R-002: API listener — Unix socket by default, TCP opt-in

**Decision**: API listens on a Unix domain socket at `${PROXA_DATA_DIR}/proxa.sock` (mode `0660`, owner = the user who ran `proxa server`) by default. TCP listening is opt-in via `--listen tcp://0.0.0.0:5443` (or via the `listen` field in `~/.proxa/config.toml`).

**Rationale**:
- No port conflicts on first install.
- File-system permissions provide zero-config local auth (only users with read+write on the socket can talk to the API).
- TCP is needed for remote management and (eventually) cluster control-plane traffic, but it's an opt-in security boundary.

**Alternatives rejected**:
- **TCP only** — opens a port that might collide; requires firewall config on first install; no zero-config local auth.
- **Both by default** — increases attack surface unnecessarily.

The CLI client tries the Unix socket first (resolved from `PROXA_SOCKET` env or `${PROXA_DATA_DIR}/proxa.sock`); if absent or unreachable, falls back to `https://localhost:5443` if `PROXA_URL` is set.

---

## R-003: Reconciler architecture — pure diff function + ticker loop

**Decision**: The reconciler is two layers:

1. `diff.Compute(desired []types.Service, actual []runtime.ContainerInfo) []Action` — a pure function. Input: snapshot of desired state (from StateStore) and actual state (from Runtime). Output: list of typed actions (`CreateContainer{spec}`, `RemoveContainer{id}`, `ReplaceContainer{id, spec}`).
2. `Loop` — a goroutine that ticks every `tickInterval`, fetches snapshots, calls `diff.Compute`, then applies each action via the Runtime. Errors per-action are logged and the loop continues.

**Rationale**:
- Pure diff is trivially testable: `(desired, actual) → actions` with no I/O, no goroutines, no time. Table-driven tests cover all SC scenarios (scale up, scale down, replace, delete).
- Ticker-driven loop is the simplest correct design. Event-driven (e.g., Docker event stream) would be lower-latency but adds connection management and makes failure modes harder to reason about. 5s tick is fast enough for SC-002 ("within 10s").

**Alternatives rejected**:
- **Event-driven via Docker event stream** — saved for v0.1+ as an optimization. The poll-based loop is the correctness baseline; events become a freshness improvement layered on top.
- **Reconciler holds long-lived state** — rejected. Stateless reconciler means restarting the daemon is cheap; correctness comes from the StateStore + Runtime, not from in-memory bookkeeping.

---

## R-004: Container labeling scheme

**Decision**: Every container created by Proxa carries these labels:

| Label | Example | Purpose |
|---|---|---|
| `proxa.managed` | `"true"` | Mark as Proxa-owned (vs. unrelated containers on the host). Also doubles as the discovery filter. |
| `proxa.project` | `"socio-do"` | Project scoping (constitution §III). |
| `proxa.service` | `"web"` | Service name within the project. |
| `proxa.replica` | `"0"` | Replica index, 0-based. Stable across reconciles. |
| `proxa.spec_hash` | `"sha256:abc123..."` | Hash of the canonicalized TaskDef spec at create time. Used by the reconciler to detect spec drift (FR-007). |
| `proxa.node` | `"node-local"` | Node ID. Single value in v0.0; the cluster scheduler in v1.0 uses this for placement. |
| `proxa.created_at` | `"2026-05-14T10:23:00Z"` | RFC 3339 timestamp. Useful for human debugging via `docker ps`. |

**Rationale**: Labels are the canonical Docker mechanism for tracking ownership. Filtering by `proxa.managed=true` lets `Runtime.ListContainers` cleanly exclude foreign containers. The `proxa.spec_hash` makes change detection a label comparison (no need to introspect every env var or volume mount).

Container name follows FR-015: `proxa-{project}-{service}-{replica}`. Both name and labels are required because Docker name-based lookup is exact-match and we need both lookup speed (by name) and bulk filtering (by project, by service).

**Alternatives rejected**:
- **Tracking via state-store-only** (no labels) — rejected. Labels make `docker ps` informative and let an operator audit Proxa's footprint without going through the API.
- **Storing the full spec in a label** — rejected. Docker labels have a soft size limit; spec_hash gives us change detection in 64 bytes.

---

## R-005: Spec hash for change detection

**Decision**: SHA-256 over the *canonical JSON encoding* of the TaskDef. Canonical means: keys sorted, no whitespace, omitempty fields preserved as their zero value (so `replicas: 0` and an absent `replicas` produce the same hash as far as semantics allow).

Implementation: `internal/reconciler/hash.go` provides `Hash(types.TaskDef) string` returning `"sha256:" + hex.EncodeToString(...)`.

**Rationale**: Determinism is everything. Two TaskDefs that semantically describe the same workload must hash identically across runs, hosts, and Go versions. Canonical JSON (sorted keys, no extraneous whitespace) is the simplest standard achieving this. SHA-256 because we already need `crypto/sha256` indirectly via Docker's image-digest comparisons.

**Alternatives rejected**:
- **Reflection-based field walk** — fragile across Go upgrades; doesn't handle map ordering.
- **TOML re-encoding then hash** — depends on the TOML library's output stability across versions. JSON is more conservative.
- **CRC32 / FNV** — too short; collision risk over the lifetime of a long-running cluster (image upgrades happen often).

---

## R-006: SQLite schema versioning

**Decision**: A single `schema_version` table with one row tracks the current schema number. `Migrate(ctx)` runs all migrations from `current_version + 1` to the highest known, in order, in a single transaction per migration. Migrations are Go code in `internal/store/sqlite/migrations.go`, not external `.sql` files.

The first migration (v1) creates: `projects`, `services`, `jobs`, `nodes`, `subjects`, `policies`, `schema_version` tables, all with `WITHOUT ROWID` where appropriate, indexes on (`project`, `name`) for the project-scoped tables.

**Rationale**:
- Go-coded migrations keep the schema and the migration code in one binary. No external `.sql` files to ship or version-control separately.
- Single-transaction-per-migration is safe: SQLite's atomic schema changes mean a failed migration leaves the DB at the previous version cleanly.
- `Migrate` is idempotent (already in the interface contract) — re-running on the latest schema is a no-op.

**Alternatives rejected**:
- **External SQL migration files via `golang-migrate`** — adds a dependency for a problem we can solve in 200 lines. Also makes `go:embed` dance.
- **Inferring schema from struct tags (auto-migrate)** — fragile; loses control over indexes and constraint ordering.

---

## R-007: TOML parser library

**Decision**: `github.com/BurntSushi/toml` v1.x.

**Rationale**: De facto standard for Go TOML. Maintained, supports TOML 1.0.0, struct-tag-driven decoding maps cleanly to `pkg/types/TaskDef`, sensible error messages with line numbers (the user-facing error path matters for `proxa up`).

**Alternatives rejected**:
- **`github.com/pelletier/go-toml/v2`** — also widely used; slightly different ergonomics. BurntSushi has more battle-time; we have no specific reason to prefer pelletier.
- **stdlib (none exists)** — Go has no stdlib TOML.

**License**: MIT. §IX-compliant.

---

## R-008: Dependency license audit (constitution §IX)

All seven direct deps from Technical Context audited against §IX (Apache, MIT, BSD, MPL 2.0 only):

| Direct dep | License | OK |
|---|---|---|
| `modernc.org/sqlite` | BSD-3-Clause | ✅ |
| `github.com/docker/docker` | Apache-2.0 | ✅ |
| `github.com/BurntSushi/toml` | MIT | ✅ |
| `github.com/go-chi/chi/v5` | MIT | ✅ |
| `github.com/spf13/cobra` | Apache-2.0 | ✅ |
| `github.com/spf13/viper` | MIT | ✅ |
| `golang.org/x/crypto/bcrypt` | BSD-3-Clause | ✅ |

**Transitive risk**: `docker/docker` pulls in a substantial dep tree (containerd-types, distribution, opencontainers/go-digest, etc.). All known transitives are Apache/MIT/BSD per the docker/docker `vendor/modules.txt` audit at v27. After `go mod tidy` runs in T-task X (TBD in `tasks.md`), the implementer MUST run a script (or eyeball `go.sum` + `pkg.go.dev` license info) to confirm no AGPL/SSPL/BSL surfaces.

A simple audit command for any contributor:

```sh
go list -deps -m -json all | jq -r 'select(.Module != null) | .Module.Path' | sort -u | xargs -I {} echo {}
# then cross-reference against pkg.go.dev license metadata
```

This audit is folded into a Polish-phase task in `tasks.md`.

---

## R-009: Bootstrap token — generation, storage, transport

**Decision**:
- Generated by `proxa init` using `crypto/rand.Read(32 bytes)` → base64url-encoded → 43 chars without padding.
- Stored two places at init time:
  - `${PROXA_DATA_DIR}/token` (mode `0600`) — the operator's reference copy.
  - SQLite `subjects` table with `provider="bootstrap"`, hashed via bcrypt (cost 12) — the server's verification copy. (Plaintext token is shown to the operator on stdout exactly once.)
- Transport: `Authorization: Bearer <token>` HTTP header on every API call.
- The CLI client reads the token from `PROXA_TOKEN` env var, then `${PROXA_DATA_DIR}/token`, then prompts via stdin if both are absent.

**Rationale**: Bcrypt at the verifier side means even a database-leak doesn't expose the token. Random 32 bytes = 256 bits of entropy; brute-force resistant. base64url so it's safe in URLs and shell args.

**Alternatives rejected**:
- **Static token derived from a passphrase** — weakens with poor passphrase choice; no way to rotate without changing the passphrase.
- **JWT** — overkill for v0.0; symmetric secret + simple format avoids the JWT pitfalls (alg confusion, missing exp checks).

The local-password authenticator (`internal/auth/password`) lands in this feature too — it's used by the future dashboard's login flow but is unused by the CLI (which uses the bootstrap token). Implementing both now keeps RBAC structurally complete from v0.0.

---

## R-010: Testing strategy for Docker-dependent code

**Decision**: Three layers.

1. **Unit tests against a mock Docker client** — exercise `internal/runtime/docker/` package's *spec construction* logic (label assignment, security profile application, name generation). Mock = a tiny in-package interface that mirrors the methods we actually call on the docker client. Lives in `internal/runtime/docker/mockclient_test.go`.
2. **Pure-Go integration tests, build-tag `//go:build dockerd`** — talk to a real `dockerd` over `/var/run/docker.sock`. Run by contributor with `make test-integration`. Skipped in CI by default (CI runs only the unit layer). Tests cover: create real `nginx:alpine` container with `security.Default()` applied, verify `docker inspect` reports `CapDrop=[ALL]`, `User != "root"`, etc.
3. **End-to-end black-box tests, build-tag `//go:build e2e`** — invoke the compiled `proxa` binary as a subprocess, run `init` → `server` → `up` → `ps` → `down` → kill the server. Runs in `quickstart.md` as the final validation step.

**Rationale**: CI must be reliable on hosted runners that don't have Docker. Layer 1 covers most of our logic without requiring Docker. Layer 2 catches Docker-API integration bugs but only on dev machines with `dockerd`. Layer 3 catches wiring bugs across the whole stack but is slowest.

**Alternatives rejected**:
- **Testcontainers / dockertest in CI** — fragile (Docker-in-Docker on hosted runners is flaky), slow, and triples CI cost. Not worth the marginal coverage gain.
- **All-mock testing** — gives false confidence; a Docker API contract change between the mock and real `dockerd` would slip through.

The CI workflow grows a `make test` step (already exists) which runs only the default-tagged tests. A separate optional GitHub Actions workflow `integration.yml` (added in this feature or deferred to 002) can run the `dockerd`-tagged tests on a self-hosted runner if/when one exists.

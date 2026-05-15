# Implementation Plan: Core Loop — Reconciliation Engine

**Branch**: `001-core-loop` | **Date**: 2026-05-14 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/001-core-loop/spec.md`

## Summary

Bring the seven foundation interfaces to life: the SQLite-backed `StateStore`, the Docker-backed `Runtime`, two `Authenticator` implementations (token + local password), a TOML parser, a long-running reconciliation loop, and a cobra-driven CLI plus a small HTTP API that ties everything together. By the end of this feature, an operator can:

```sh
proxa init                         # creates data dir, SQLite, master key, admin user, bootstrap token
proxa server                       # foreground daemon; runs reconciler + HTTP API on Unix socket
# (in another shell)
proxa up service.toml              # parses, validates, stores TaskDef, kicks reconciler
proxa ps                           # lists services with desired/actual replicas
proxa down api                     # sets replicas=0, reconciler removes containers
```

Foundation (000) shipped only stubs and the `internal/security` real-logic package. Everything else returns `ErrNotImplemented`. This feature replaces those stubs with concrete code, adds `internal/reconciler/`, `internal/parser/toml/`, `internal/server/`, `internal/cli/`, `internal/config/`, `internal/hash/`, `internal/auth/dbpolicy/`, and `internal/web/`, and wires `cmd/proxa/main.go` to the cobra root command.

This feature also ships a **slim read-only dashboard** at `/ui/` (per FR-017 added during /speckit.analyze). Frontend trio (HTMX + Alpine + Tailwind) embedded via `go:embed`. Auth: Unix-socket access bypasses the token check; TCP listeners require auth (or return 401 in v0.0). The full dashboard with login + write ops + log viewer remains scoped to Feature 004.

## Technical Context

**Language/Version**: Go 1.26.x (`CGO_ENABLED=0` for production binaries; `=1` only at the test step for `-race`).

**Primary Dependencies** — first real third-party deps in this project (FR-009 from 000 was honored only because there was no concrete code yet; this feature genuinely needs them):

| Module | Version pin | License | Purpose |
|---|---|---|---|
| `modernc.org/sqlite` | latest 1.x | BSD-3 | Pure-Go SQLite for `StateStore` |
| `github.com/docker/docker` | v27+ | Apache 2.0 | Docker Engine API client (`client` subpackage) |
| `github.com/BurntSushi/toml` | latest 1.x | MIT | TOML parser for TaskDef files |
| `github.com/go-chi/chi/v5` | latest 5.x | MIT | HTTP router for the API server |
| `github.com/spf13/cobra` | latest 1.x | Apache 2.0 | CLI subcommands |
| `github.com/spf13/viper` | latest 1.x | MIT | Config (env vars + flags + files) |
| `golang.org/x/crypto/bcrypt` | latest | BSD-3 | Password hashing for local admin |

All §IX-compliant. Indirect deps (Docker pulls in many) audited in `research.md` after `go mod tidy` lands.

**Storage**: SQLite via `modernc.org/sqlite` in WAL mode. Single file at `${PROXA_DATA_DIR}/proxa.db` (default `~/.proxa/proxa.db`). Schema migrations run idempotently at server startup.

**Frontend assets** (slim dashboard): vendored under `web/static/` and embedded via `go:embed`. No Node.js at build or runtime (constitution §V).
- `htmx.min.js` (~14KB) — pinned to a specific HTMX release version, downloaded once.
- `alpine.min.js` (~16KB) — pinned to a specific Alpine release version, downloaded once.
- `app.css` — generated locally via the standalone `tailwindcss` CLI binary (one-shot, not in CI), checked in. Theme extends with the project's sage/teal palette + Geist/Instrument-Serif fonts from the dashboard mockup at `specs/_reference/dashboard-mockup.html`. Regeneration steps documented in `CLAUDE.md` (added by T072).

**Testing**:
- `testing` (stdlib) for table-driven unit tests.
- `testing/synctest` (Go 1.24+, stable in 1.26) for the reconciler loop — gives us deterministic time without `time.Sleep` in tests.
- Integration tests build-tagged `//go:build integration` that hit a live `dockerd` (skipped in CI by default; runs locally during dev).

**Target Platform**: linux/{amd64,arm64} primary (Docker daemon assumed); darwin/{amd64,arm64} for development. Multi-arch via existing goreleaser config from 000.

**Project Type**: Single binary still — `cmd/proxa` gains subcommands; `cmd/proxa-agent` stays a stub (agent loop arrives later).

**Performance Goals**:
- Reconciliation tick: 5 seconds default; configurable down to 1 second.
- Reconciler diff computation + Docker calls for ≤100 containers: <500ms p95 per tick.
- API endpoint p95: <50ms for read, <200ms for write (write blocks on SQLite + reconcile-trigger).
- `proxa init` end-to-end: <2 seconds on warm filesystem.

**Constraints**:
- Reconciler MUST NOT panic on transient Docker errors. Log via `slog` and continue.
- SQLite WAL: only the server holds the writer connection; CLI commands are read-only against SQLite directly (or use the API, which is the recommended path).
- API listens on a Unix socket by default (`${PROXA_DATA_DIR}/proxa.sock` mode 0600). TCP listen is opt-in via `--listen tcp://0.0.0.0:5443`.
- Every container created MUST go through `security.Apply()` before `Runtime.CreateContainer` (constitution §II — enforced inside the runtime impl, not relying on caller discipline).
- Every state-store call carries a `project string` (constitution §III — already in the interface from 000).

**Scale/Scope**:
- Single-node v0.0. ≤100 containers, ≤20 services, ≤10 projects per node.
- Multi-node clustering is explicitly out of scope (v1.0).

## Constitution Check

*GATE: Re-run after Phase 1.* Result below: **PASS** with one justified deviation tracked in Complexity Tracking.

| Principle | Status | How this plan complies |
|---|---|---|
| §I Architecture First | ✅ | Every new concrete impl sits behind a 000-foundation interface. The reconciler is new code (no interface yet), justified in Complexity Tracking. |
| §II Security by Default | ✅ | `dockerRuntime.CreateContainer` calls `security.Apply()` on every spec before submitting to the daemon. Test verifies that constructing a container without explicit security yields `CapDrop=[ALL]` and `NoNewPrivileges=true` in the actual Docker spec. API has no anonymous endpoints; the bootstrap token is required for every `/api/v1/*` call. Secrets master key is generated at `proxa init` with `0600` permissions. |
| §III Project Scoping from v0.0 | ✅ | Container labels carry `proxa.project=<name>`. Every SQL query has a `WHERE project = ?` clause (or scope-by-id for cluster-scoped tables like `nodes`). Test fixture: two services named `web` in projects `socio-do` and `kut-do` coexist (SC-006). |
| §IV Go Idioms | ✅ | `context.Context` first param everywhere. Errors wrap with `fmt.Errorf("pkg/<impl>: %w", err)`. Logging via `slog` JSON to stderr. Table-driven tests. `CGO_ENABLED=0` for production binary. |
| §V Single Binary | ✅ | Still one binary (`proxa`). The "daemon" is `proxa server`, not a separate executable. |
| §VI Cluster-Ready Design | ✅ | `StateStore`, `Runtime`, `Authenticator` all interface-mediated. Container labels include `proxa.node=<node-id>` (single value in v0.0; ready for the cluster scheduler in v1.0). The reconciler operates on a single node's view today; the diff function is pure (input: desired set + actual set → action set), trivial to slot behind a multi-node coordinator later. |
| §VII Declarative, Not Imperative | ✅ | TOML in, reconciliation loop computes diff, applies action. There is no imperative "create container N" CLI; even `proxa up` is "set the desired state then let the reconciler converge". |
| §VIII Zero-Downtime by Default | ⚠️ Partial — see Complexity Tracking | This feature implements basic create/remove/replace. The full start-first vs stop-first strategy with health-check gating arrives in Feature 002 (per spec "Out of Scope: Health checks"). For v0.0 of the reconciler, "replace" is a naive remove-then-create per replica index. Documented in Complexity Tracking. |
| §IX Permissive License | ✅ | All seven new deps are Apache/MIT/BSD. Transitive deps audited post-`go mod tidy` in `research.md` Section R-008. |
| §X Honest Scope | ✅ | Out-of-scope items (health checks, ingress, dashboard, secrets CRUD, config maps, deploy strategies, multi-node) are documented in spec and re-stated here. Each will get its own feature; nothing is half-implemented. |
| §XI Commit Strategy | ✅ | Tasks (next phase) are decomposed so each one fits a single `<type>(<scope>): <description>` commit. |

## Project Structure

### Documentation (this feature)

```text
specs/001-core-loop/
├── spec.md              # Source of truth (already exists)
├── plan.md              # This file
├── research.md          # Phase 0 — 8 decisions + dep audit
├── data-model.md        # Phase 1 — SQLite schema + container-label conventions
├── quickstart.md        # Phase 1 — operator workflow walkthrough
├── contracts/           # Phase 1 — REST API contract + TOML grammar
│   ├── rest-api.md
│   └── toml-grammar.md
└── tasks.md             # Phase 2 — generated by /speckit.tasks
```

### Source Code (only new/modified paths shown; foundation tree unchanged unless noted)

```text
proxa/
├── cmd/
│   └── proxa/
│       └── main.go                            # MODIFIED — wires internal/cli root command
├── internal/
│   ├── auth/
│   │   ├── auth.go                            # (existing)
│   │   ├── policy.go                          # (existing — interface only; noop deleted by T021)
│   │   ├── token/
│   │   │   ├── token.go                       # NEW — TokenAuthenticator (bearer token)
│   │   │   └── token_test.go
│   │   ├── password/
│   │   │   ├── password.go                    # NEW — LocalPasswordAuthenticator (bcrypt)
│   │   │   └── password_test.go
│   │   └── dbpolicy/
│   │       ├── dbpolicy.go                    # NEW — StateStore-backed PolicyEngine impl
│   │       └── dbpolicy_test.go
│   ├── cli/
│   │   ├── root.go                            # NEW — cobra root, global flags
│   │   ├── init.go                            # NEW — proxa init
│   │   ├── server.go                          # NEW — proxa server (the daemon)
│   │   ├── up.go                              # NEW — proxa up
│   │   ├── down.go                            # NEW — proxa down
│   │   ├── ps.go                              # NEW — proxa ps
│   │   ├── client.go                          # NEW — HTTP client wrapper for the API
│   │   └── *_test.go
│   ├── config/
│   │   └── config.go                          # NEW — viper config (paths, listen addr, tick interval)
│   ├── parser/
│   │   └── toml/
│   │       ├── parser.go                      # NEW — TOML → TaskDef + validation
│   │       ├── parser_test.go
│   │       └── testdata/
│   │           ├── valid.toml
│   │           ├── missing-image.toml
│   │           └── ...
│   ├── reconciler/
│   │   ├── reconciler.go                      # NEW — main loop, ticker, error handling
│   │   ├── diff.go                            # NEW — pure function: (desired, actual) → action set
│   │   ├── action.go                          # NEW — apply one action via Runtime
│   │   └── *_test.go                          # (hash moved to internal/hash/ to avoid layering inversion)
│   ├── runtime/
│   │   ├── runtime.go                         # (existing)
│   │   └── docker/
│   │       ├── docker.go                      # NEW — dockerRuntime: New, Name, Version
│   │       ├── container.go                   # NEW — Create/Start/Stop/Remove/Inspect/List
│   │       ├── image.go                       # NEW — Pull/Inspect
│   │       ├── exec.go                        # NEW — Exec, Stats, StreamLogs
│   │       ├── labels.go                      # NEW — proxa.project, .service, .replica, .spec_hash
│   │       ├── security.go                    # NEW — applies SecurityProfile to Docker config
│   │       └── *_test.go
│   ├── server/
│   │   ├── server.go                          # NEW — http.Server lifecycle, mux
│   │   ├── routes.go                          # NEW — chi route registration
│   │   ├── handlers.go                        # NEW — REST handlers
│   │   ├── middleware.go                      # NEW — auth middleware
│   │   └── *_test.go
│   ├── store/
│   │   ├── store.go                           # (existing)
│   │   └── sqlite/
│   │       ├── sqlite.go                      # NEW — sqliteStore: Open/Close/Migrate
│   │       ├── migrations.go                  # NEW — versioned schema
│   │       ├── projects.go                    # NEW — CreateProject/Get/List/Delete
│   │       ├── services.go                    # NEW — Put/Get/List/Delete/WatchServices
│   │       ├── jobs.go                        # NEW — Put/Get/List/Delete
│   │       ├── nodes.go                       # NEW — Put/Get/List/Delete + Heartbeat
│   │       ├── auth.go                        # NEW — PutSubject/Get + PutPolicy/ListFor/Delete
│   │       ├── tx.go                          # NEW — Tx implementation
│   │       └── *_test.go
│   ├── hash/
│   │   ├── hash.go                            # NEW — canonical-JSON SHA-256 (leaf utility)
│   │   └── hash_test.go
│   └── web/
│       └── web.go                             # NEW — go:embed FS for web/static + web/templates
├── web/
│   ├── static/
│   │   ├── htmx.min.js                        # NEW — vendored, pinned version
│   │   ├── alpine.min.js                      # NEW — vendored, pinned version
│   │   └── app.css                            # NEW — Tailwind production build (locally generated)
│   ├── templates/
│   │   ├── index.html                         # NEW — slim dashboard landing
│   │   └── services_table.html                # NEW — HTMX-swappable services-table fragment
│   └── README.md                              # (existing) — updated by T072
├── pkg/
│   └── types/                                 # (existing) — no changes; entities already cover this feature
├── tests/
│   └── e2e/                                   # NEW — //go:build e2e end-to-end tests
│       └── *_test.go
└── go.mod                                     # MODIFIED — gains the seven listed deps
```

**Structure Decision**: Keep the foundation's monorepo layout from §4.2. New code lives under existing `internal/` packages plus nine new packages: `parser/toml`, `reconciler`, `server`, `cli`, `config`, `hash`, `web`, and the concrete-impl subpackages (`auth/token`, `auth/password`, `auth/dbpolicy`, `runtime/docker`, `store/sqlite`). The decision to put concrete impls under sibling subpackages (e.g., `internal/store/sqlite/`, `internal/auth/dbpolicy/`) rather than mixing them into the parent's interface file keeps the import path readable and the layering clean: `import "github.com/proxa-server/proxa/internal/store"` for the interface, `import "github.com/proxa-server/proxa/internal/store/sqlite"` for the impl. `internal/hash/` is a leaf utility imported by both `store/sqlite` and `reconciler` — putting it standalone avoids a layering inversion where the low-level store would otherwise reach up into the high-level coordinator. The new `tests/e2e/` directory holds `//go:build e2e` end-to-end tests that exercise the compiled binary as a subprocess.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Reconciler is concrete code, not behind an interface | The reconciler is the *coordinator* — it consumes `Runtime` and `StateStore` interfaces and calls `security.Apply`. It is not itself a swappable backend; there will only ever be one reconciler implementation per Proxa version. Adding a `Reconciler` interface would be ceremony without a second implementation in sight. | A `Reconciler` interface — rejected because §I Architecture First is about backend swappability (Docker→containerd, SQLite→etcd), not about every internal coordinator. The reconciler's *output* (calls into Runtime/StateStore) is already interface-mediated. |
| §VIII zero-downtime is partially implemented | `internal/reconciler/action.go` does naive remove-then-create per replica, not the full start-first/stop-first strategy. | Strategy implementation requires health checks (Feature 002) to be useful — a "start-first" replacement only buys zero-downtime if you can confirm the new container is healthy before stopping the old one. Implementing strategy without health checks gives the appearance of zero-downtime without the substance. Better to ship naive replace in 001 and add real strategy in 002 once health probes exist. Documented in spec Out of Scope. |
| Daemon (`proxa server`) added to scope despite spec FR list omitting it | SC-002 ("reconciler restarts crashed container within 10s") is unsatisfiable without a long-running process. The spec describes the daemon's behavior in success criteria but never names it in functional requirements. | A polling cron / launchd shim — rejected because it duplicates timer logic outside the binary, violates §V (single binary, zero deps), and complicates `proxa init`'s deliverables. The daemon is not optional; it is the reconciler. Calling it `proxa server` makes it visible. |

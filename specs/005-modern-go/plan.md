# Implementation Plan: Modern Go Foundation Pass (v0.4.1)

**Branch**: `005-modern-go` | **Date**: 2026-05-19 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/005-modern-go/spec.md`

## Summary

First release of the v0.4.x Foundation Train. Systematic modernization pass applying Go 1.22 → 1.26 idioms and stdlib hardening primitives across the whole codebase, gated by three justifications: security (US2: data-dir sandbox via `os.Root`, cryptographic token gen via `crypto/rand.Text`, cross-origin protection via `http.CrossOriginProtection`), deprecation hygiene (US4: any legacy `math/rand` import → `math/rand/v2`; any `runtime.SetFinalizer` → `runtime.AddCleanup`), and idiom modernization (US4: counter loops → `range N`, manual goroutine pool → `sync.WaitGroup.Go`, hand-rolled slice ops → `slices` package, `go fix ./...` modernizers). Plus the probe-via-ingress + TLS=true bug fix discovered in the 004-logs demo (US1). Plus the System Info dashboard card so operators can verify the modernization landed (US3 / dashboard-parity). Plus the `tool` directive in go.mod so contributors can run `go tool staticcheck ./...` on a fresh clone (US5). Plus a `internal/datadir/snapshot.go` helper that the v0.4.3 event store and v0.5.0 rollback will both depend on (FR-004). Zero new third-party module dependencies — entire release is stdlib upgrades + internal refactors + one bug fix.

## Technical Context

**Language/Version**: Go 1.26.x (toolchain pin in `go.mod`); CGO_ENABLED=0; targets darwin/amd64, darwin/arm64, linux/amd64, linux/arm64.

**Primary Dependencies**: existing only — chi router, cobra+viper CLI, modernc.org/sqlite, docker/docker client, caddyserver/certmagic, BurntSushi/toml. **No new dependencies added by this release.**

**Storage**: existing SQLite under data dir + `secrets/` flat files; this release adds `os.Root` sandboxing of all data-dir access but does not change the schema or on-disk format.

**Testing**: `go test ./...` (unit + race), `//go:build dockerd` integration against real Docker daemon, `//go:build e2e` against the full binary; existing harness in `tests/e2e/` reused. New tests added per US — see Phase 1 contracts.

**Target Platform**: same as v0.4.0 — macOS dev loop (Colima / Rancher Desktop / Docker Desktop), Linux self-hosted production.

**Project Type**: single Go binary embedding API server, scheduler, ingress, dashboard, DNS, CLI.

**Performance Goals**: no regression vs v0.4.0. Specifically: binary size ±2% (SC-008), reconciler tick latency unchanged, probe latency unchanged. Container-aware GOMAXPROCS may *improve* CPU efficiency for in-container Proxa deployments (free win, no code change beyond verification).

**Constraints**: zero new third-party deps (FR-013); zero-downtime upgrade from v0.4.0 (FR-015); tokens issued by v0.4.0 stay valid post-upgrade (assumptions section); the dashboard's slim-IA preference preserved (System Info is one card, not a new top-level peer).

**Scale/Scope**: roughly 14-16 commits across these scopes: `probe`, `store`, `secrets`, `auth`, `server`, `web`, `cli`, `ingress`, `reconciler`, `version`, `docs`, `e2e`. No new top-level packages except `internal/datadir/` (one new path-sandbox + snapshot helper). One new docs directory `docs/decisions/` for ADRs 0004 (router) and 0005 (deferred modernizers).

## Constitution Check

*Gate evaluated before Phase 0 research. Re-evaluated after Phase 1 design — see "Post-Design Constitution Check" below.*

| Principle | Compliance | Notes |
|-----------|------------|-------|
| §I Architecture First | ✅ pass | No new components; existing interfaces (`Runtime`, `StateStore`, `Authenticator`, etc.) untouched. New `datadir.Root` helper is internal infrastructure, not a swap point. |
| §II Security by Default | ✅ pass (HARDENS) | US2 is pure §II reinforcement: data-dir sandbox closes a path-traversal class; `crypto/rand.Text` hardens token entropy; `http.CrossOriginProtection` adds defense-in-depth before any write endpoint exists. |
| §III Project Scoping | ✅ pass | No state-model changes. `datadir.Root` operates below the project layer (it sandboxes the on-disk location regardless of project). |
| §IV Go Idioms | ✅ pass (REINFORCES) | This release IS the §IV reinforcement: stdlib-first (literally only stdlib changes), `context.Context` preserved on every modified function, `slog` unchanged, no CGO, table-driven tests for new helpers. |
| §V Single Binary, Zero Deps | ✅ pass | No new modules added. Verified by `go mod tidy` round-trip in the final commit. Dashboard remains HTMX+Alpine+Tailwind embedded; the System Info card is a small HTML fragment, not a new asset class. |
| §VI Cluster-Ready Design | ✅ pass | The new `datadir.Root` helper is per-node infrastructure — multi-node v1.0 will have one Root per node, each scoped to that node's data dir. No design choice in this release blocks clustering. |
| §VII Declarative | ✅ pass | No reconciliation-loop changes. The probe fix preserves the reconciler contract; it only changes probe-internal redirect handling. |
| §VIII Zero-Downtime | ✅ pass | FR-015 explicitly mandates zero-downtime upgrade from v0.4.0. Tokens stay valid; on-disk format unchanged; rolling upgrade works. |
| §IX Permissive License | ✅ pass | Zero new dependencies. License audit log gets a refresh entry confirming no-op (matches the 004-logs pattern). |
| §X Honest Scope | ✅ pass | Spec's Assumptions section explicitly enumerates out-of-scope items (chi→stdlib router, range-over-func iterators, json v2, Green Tea GC, weak.Pointer caches) so they aren't half-implemented here. |
| §XI Commit Strategy | ✅ pass | Plan budgets ~14-16 commits each with `<type>(<scope>): <description>` and a single focused diff. Constitution-mandated. |

**Verdict**: No violations. No Complexity Tracking entry needed.

## Project Structure

### Documentation (this feature)

```text
specs/005-modern-go/
├── plan.md              # This file
├── research.md          # Phase 0 — probe-fix design decision, CSRF mount placement, deferred modernizers list
├── data-model.md        # Phase 1 — minimal: SystemInfo payload + datadir.Root contract
├── quickstart.md        # Phase 1 — operator + contributor walk-through
├── contracts/
│   ├── datadir-root.md       # internal/datadir/root.go API
│   ├── datadir-snapshot.md   # internal/datadir/snapshot.go API
│   ├── system-info-api.md    # GET /api/v1/system + CLI: proxa system info
│   └── probe-config.md       # probe.HTTPConfig.FollowRedirects + via-ingress redirect handling
├── checklists/
│   └── requirements.md       # already created in /speckit.specify
└── tasks.md             # Phase 2 output (created by /speckit-tasks — NOT this command)
```

### Source Code (repository root, deltas only)

```text
internal/
├── datadir/                          # NEW package
│   ├── root.go                       # os.Root wrapper for all data-dir file access
│   ├── root_test.go                  # path-traversal refusal table
│   ├── snapshot.go                   # os.CopyFS-based snapshot helper (FR-004)
│   └── snapshot_test.go              # round-trip assertion
├── store/sqlite/
│   └── store.go                      # MODIFIED — open SQLite file via datadir.Root
├── secrets/
│   └── store.go                      # MODIFIED — read/write secrets via datadir.Root
├── auth/
│   └── bootstrap.go                  # MODIFIED — crypto/rand.Text for token gen
├── cli/
│   ├── init.go                       # MODIFIED — crypto/rand.Text for any token-style randomness
│   └── system_info.go                # NEW — `proxa system info` command (FR-008)
├── server/
│   ├── server.go                     # MODIFIED — mount http.CrossOriginProtection middleware
│   ├── system_info.go                # NEW — GET /api/v1/system handler + /ui/system page
│   └── ui.go                         # MODIFIED — embed System Info card link in main dashboard footer
├── probe/
│   ├── http.go                       # MODIFIED — CheckRedirect policy for via-ingress TLS redirect
│   └── http_test.go                  # MODIFIED — unit table for redirect behavior
├── version/
│   └── runtime.go                    # NEW — runtime introspection (GOMAXPROCS source, GOEXPERIMENT, Go version)
├── ingress/proxy.go                  # MODIFIED — sync.WaitGroup.Go migration (US4)
├── reconciler/reconciler.go          # MODIFIED — sync.WaitGroup.Go migration + range-over-int counters
└── probe/manager.go                  # MODIFIED — sync.WaitGroup.Go migration

web/_src/                             # CSS source (unchanged this release; templates updated)
internal/web/
├── templates/
│   ├── index.html                    # MODIFIED — System Info card footer
│   └── system.html                   # NEW — focused /ui/system page (slim-IA preserved)
└── static/                           # NO new assets; CSS classes reused

tests/
└── e2e/
    ├── probe_ingress_tls_test.go     # NEW — US1 reproducer + regression gate
    ├── system_info_test.go           # NEW — US3 dashboard card + CLI parity
    └── tool_directive_test.go        # NEW — US5 fresh-clone lint reproducibility

docs/
├── decisions/                        # NEW directory
│   ├── 0004-router.md                # NEW — keep chi until v1.0, document stdlib alternative
│   └── 0005-deferred-modernizers.md  # NEW — modernizers NOT taken + rationale
├── licenses.md                       # MODIFIED — refresh log entry (no-op confirmation)
└── operations.md                     # MODIFIED — document `go tool staticcheck`, container-aware GOMAXPROCS verification

go.mod                                # MODIFIED — add `tool` directive for staticcheck
Makefile                              # MODIFIED — `make lint` uses `go tool staticcheck` instead of `go run`
```

**Structure Decision**: Existing single-binary `internal/` layout preserved. Two new minor packages introduced: `internal/datadir/` (path-sandbox + snapshot helper, deliberately under `internal/` not `pkg/` because it's Proxa-private infrastructure) and `internal/version/runtime.go` (runtime introspection as a sibling to existing build-info vars). No external API surface changes beyond the one new `GET /api/v1/system` endpoint and the one new `proxa system info` CLI command.

## Phase 0 — Research (output: research.md)

Three open questions to resolve before tasks:

1. **Probe-via-ingress fix shape** — two viable approaches:
   - **A**: Teach `probe.HTTPProbe.client` to follow up to 1 redirect with a `CheckRedirect` function; treat the final 2xx as success. Minimal change. Risk: if the redirect target is the HTTPS port but the probe didn't trust the cert, it still fails — need to handle self-signed staging certs.
   - **B**: When the probe is constructed via `NewHTTPProbeViaIngress` and ingress has TLS enabled, target the HTTPS port directly instead of HTTP. More invasive but semantically cleaner — probe goes to the same port operator traffic uses.
   - Decision in research.md after reading `internal/probe/http.go` and `internal/ingress/router.go` in detail.

2. **`http.CrossOriginProtection` mount semantics** — Go 1.25's middleware rejects cross-origin state-changing requests by default. Open question: does the dashboard's HTMX same-origin polling count as "same origin" without explicit allowlist? Decision in research.md by reading `internal/server/server.go` and `internal/web/templates/index.html`.

3. **`runtime.SetDefaultGOMAXPROCS` introspection** — Go 1.25 exposes container-aware GOMAXPROCS adjustment. Open question: how do we *detect* whether the auto-adjust took effect (vs explicit env override, vs host CPU count)? Likely via `runtime.GOMAXPROCS(0)` compared against host detection (`runtime.NumCPU()` for the un-adjusted view), with the env-var presence as the override signal. Decision in research.md after reading Go 1.25 runtime docs.

4. **Modernizer triage (US4)** — after running `go fix ./...` on a scratch branch, which auto-applied changes do we accept, and which do we defer (and why)? Decision in research.md feeds the per-package commit list and `docs/decisions/0005-deferred-modernizers.md`.

## Phase 1 — Design & Contracts (output: data-model.md, contracts/, quickstart.md)

### Data model

Minimal — this release introduces no persistent entities. Two in-memory contracts:

- **SystemInfo** — read-only payload returned by `GET /api/v1/system` and rendered by `/ui/system` and the dashboard footer card. Fields: `go_version`, `commit`, `build_date`, `go_experiments []string`, `gomaxprocs int`, `gomaxprocs_source enum{"host", "container_limit", "env_override"}`, `numcpu_host int`.
- **datadir.Root** — wrapper over `*os.Root` exposing the subset of file ops Proxa uses (`Open`, `Create`, `Stat`, `Mkdir`, `Remove`, `ReadFile`, `WriteFile`). Path-traversal refusal is the invariant; tested via table-driven `root_test.go`.

### Contracts

- `contracts/datadir-root.md` — Go API contract for `internal/datadir.Root`: constructor signature, allowed ops, traversal-refusal guarantee, error taxonomy.
- `contracts/datadir-snapshot.md` — Go API contract for `Snapshot(src datadir.Root, dst string) error` using `os.CopyFS`; what happens on partial failure (atomic rename to dst final path); not wired to any CLI in this release.
- `contracts/system-info-api.md` — HTTP contract for `GET /api/v1/system` (200 JSON SystemInfo; 401 with no token) and CLI contract for `proxa system info` (plain text key=value; `--json` for scripting).
- `contracts/probe-config.md` — extension to `probe.HTTPConfig`: new field for explicit redirect-follow policy; default value choice from research.md; backwards compatibility (existing probes without the field behave the new way iff via-ingress + TLS, else unchanged).

### Quickstart

`quickstart.md` walks an operator through:

1. Upgrading from v0.4.0 to v0.4.1 (no config changes needed; tokens stay valid).
2. Opening the dashboard and finding the System Info card.
3. Running `proxa system info` from CLI and comparing values.
4. Deploying a TLS-enabled service with a health probe and confirming healthy state in <30s.
5. Reading `docs/decisions/0004-router.md` and `docs/decisions/0005-deferred-modernizers.md` to understand the modernization scope.

### Agent context

The `<!-- SPECKIT START -->` / `<!-- SPECKIT END -->` markers in `CLAUDE.md` are updated to reference `specs/005-modern-go/plan.md` for the active feature.

## Post-Design Constitution Check

Re-evaluated after Phase 1 design. **No new violations introduced.** The `SystemInfo` endpoint adds attack surface (one more HTTP route) but is read-only, behind the same bearer-token authn as every other API endpoint, and exposes no sensitive data (Go version is already in `runtime/debug.ReadBuildInfo`). §II compliance maintained. The `datadir.Root` migration MUST be a full sweep — no `os.OpenFile`-style data-dir access may remain after this release; verified by a one-off grep in the final validation step.

## Complexity Tracking

*Not applicable — no constitution violations to justify.*

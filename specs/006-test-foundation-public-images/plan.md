# Implementation Plan: Test Foundation + Public Images (v0.4.2)

**Branch**: `006-test-foundation-public-images` | **Date**: 2026-05-26 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/006-test-foundation-public-images/spec.md`

## Summary

Second release of the v0.4.x Foundation Train. Two complementary themes:

1. **Test Foundation** — formalize CI/test infrastructure so v0.5+ killer features land on a stable platform. Concrete deliverables: testing/synctest adoption in 6 reconciler+probe tests (kills time.Sleep flakiness), T.Attr tagging in e2e tests for spec/SC traceability, `tests/e2e/internal/harness/` consolidation of ~15 helpers, content-addressed digest pinning of every test image, new `bench/` suite with 6 benchmark categories (reconciler tick / ingress L7 / probe wave / L4 throughput / SSE / idle RSS), formalized `make {test, test-quick, test-integ, test-e2e, bench, cover}` matrix, reporting-only coverage gate ≥60% per package.

2. **Public Images** — make Proxa installable in production. Concrete deliverables: GHCR multi-arch images (`ghcr.io/proxa-server/proxa` + `ghcr.io/proxa-server/proxa-agent`, linux/amd64 + linux/arm64), Dockerfile.proxa + Dockerfile.proxa-agent (both FROM scratch), POSIX `install.sh` published via GitHub Pages, GitHub Actions `release.yml` workflow on `v*` tags that runs Goreleaser + buildx + pushes images + attaches checksums to GitHub Release. Goreleaser already half-configured (`.goreleaser.yml` exists with the build matrix); v0.4.2 finishes the manifest+publish parts.

Plus the dashboard-parity contract: extend the existing System Info card (from v0.4.1) with a `Distribution` field (binary / docker / unknown) so operators can verify their distribution channel from the dashboard.

Both themes are infrastructure-only — no runtime behavior changes for end users. **v0.4.2 is the last release before multi-host work begins**; it MUST leave the project in a state where v0.5 can ship the `proxa-agent` functional impl without rebuilding the release pipeline.

## Technical Context

**Language/Version**: Go 1.26.x (toolchain pin in `go.mod`). `testing/synctest` and `T.Attr` are stdlib Go 1.25+ — already available.

**Primary Dependencies**: existing only — chi router, cobra+viper CLI, modernc.org/sqlite, docker/docker client, caddyserver/certmagic, BurntSushi/toml. **Zero new production dependencies.** Build-time tooling: Goreleaser added via Go 1.24 `tool` directive (MIT license, same pattern as staticcheck in v0.4.1).

**Storage**: no schema changes. SystemInfo gets one new in-memory field (`distribution`). On-disk format unchanged; existing v0.4.1 data dirs work post-upgrade.

**Testing**:
- Unit (`go test ./...` + race) — extended with synctest in 6 tests
- `//go:build dockerd` — no new dockerd tests in v0.4.2
- `//go:build e2e` — 3 new e2e tests (install.sh smoke, docker image smoke, bench-suite smoke)
- New `bench/...` benchmarks (run separately from unit suite via `make bench`)
- Coverage script: small Go binary at `cmd/coverage-gate/` — decision in R-005

**Target Platform**: same as v0.4.0/v0.4.1 — macOS dev loop (Colima / Rancher Desktop / Docker Desktop), Linux self-hosted production. Container images target linux/amd64 + linux/arm64.

**Project Type**: single Go binary embedding everything (CLI + API server + dashboard + ingress + DNS + future agent).

**Performance Goals**: bench suite establishes baseline numbers; no specific perf target this release (target is the baseline itself). Existing perf characteristics preserved.

**Constraints**:
- Zero new third-party deps in production (FR-015)
- Binary size within ±2% of v0.4.1 (FR-016; v0.4.1 = 27,450,050 bytes recorded)
- Zero-downtime upgrade from v0.4.1 (FR-017)
- v0.5 multi-host MUST be able to consume v0.4.2's release infrastructure (verified in Post-Design Constitution Check)

**Scale/Scope**: roughly 15-18 commits across scopes: `bench`, `tests/e2e/harness`, `internal/probe`, `internal/reconciler`, `internal/version`, `runtime/docker` (Dockerfiles), `ci`, `docs`, `make`. No new top-level packages except `tests/e2e/internal/harness/` and `bench/`. No new `internal/` packages.

## Constitution Check

| Principle | Compliance | Notes |
|-----------|------------|-------|
| §I Architecture First | ✅ pass | No new components. Existing interfaces (`Runtime`, `StateStore`, `Authenticator`, etc.) untouched. |
| §II Security by Default | ✅ pass | Container images run as nonroot user (USER 65532:65532); install.sh refuses on checksum mismatch (FR-010); no auth changes. |
| §III Project Scoping | ✅ pass | No state-model changes. Distribution field is per-node runtime metadata, not project-scoped. |
| §IV Go Idioms | ✅ pass (REINFORCES) | synctest + T.Attr are stdlib adoption — exactly the §IV pattern. |
| §V Single Binary, Zero Deps | ✅ pass | Goreleaser via `tool` directive (same precedent as staticcheck v0.4.1). NO new production dependencies. Verified by `go mod tidy` round-trip in final commit. |
| §VI Cluster-Ready Design | ✅ pass (ENABLES) | Public agent image (FR-008) is the **gate** for v0.5 multi-host. Without v0.4.2, multi-host cannot ship the bootstrap flow. |
| §VII Declarative | ✅ pass | No reconciliation-loop changes. |
| §VIII Zero-Downtime | ✅ pass | FR-017 explicitly mandates. No on-disk/wire format changes. |
| §IX Permissive License | ✅ pass | Goreleaser is MIT. License audit log gets refresh entry. No new prod deps means no new transitive surface either. |
| §X Honest Scope | ✅ pass | Spec's Assumptions section enumerates 8 explicit out-of-scope items (auto-systemd-install, SBOM, image signing, agent functional impl, Docker Hub mirror, custom CNAME, Brew/AUR packaging, synctest beyond reconciler/probe). |
| §XI Commit Strategy | ✅ pass | Plan budgets ~15-18 commits each with `<type>(<scope>): <description>`. |

**Verdict**: No violations. No Complexity Tracking entry needed.

## Project Structure

### Documentation (this feature)

```text
specs/006-test-foundation-public-images/
├── plan.md              # This file
├── research.md          # Phase 0 — R-001 release tool, R-002 tool directive, R-003 install.sh hosting, R-004 synctest scope, R-005 coverage script lang, R-006 bench layout
├── data-model.md        # Phase 1 — minimal: SystemInfo Distribution field + binary-size-baseline + coverage allowlist
├── quickstart.md        # Phase 1 — operator + contributor walk-throughs
├── contracts/
│   ├── install-sh.md         # POSIX shell contract: env vars, exit codes, output format
│   ├── release-pipeline.md   # Goreleaser + Actions contract: triggers, artifacts, registry layout
│   ├── dockerfiles.md        # Image LABELS, EXPOSE, VOLUME, USER, ENTRYPOINT contracts
│   ├── make-targets.md       # Makefile matrix: each target's exact behavior + exit codes
│   └── coverage-gate.md      # Coverage tool contract: input format, output, allowlist
├── checklists/
│   └── requirements.md       # Already created in /speckit.specify
└── tasks.md             # Phase 2 output (created by /speckit-tasks — NOT this command)
```

### Source Code (repository root, deltas only)

```text
bench/                                # NEW directory
├── doc.go                            # package godoc explaining benchmark categories
├── bench_reconciler_test.go          # tick throughput (services/sec)
├── bench_ingress_test.go             # L7 latency (p50/p99) under load
├── bench_probe_test.go               # probe wave capacity (containers/cycle)
├── bench_l4_test.go                  # L4 proxy throughput (MB/sec)
├── bench_sse_test.go                 # SSE message throughput (lines/sec)
├── bench_idle_memory_test.go         # idle RSS (Linux; runtime/debug fallback elsewhere)
└── binary-size-baseline.txt          # v0.4.1 baseline 27,450,050 bytes (committed)

tests/e2e/internal/harness/           # NEW package
├── doc.go                            # godoc
├── proxa.go                          # runProxa, proxaBinary, mustGetwd, findRepoRoot
├── server.go                         # startServer (background subprocess)
├── socket.go                         # socketPath, getViaSocket, sseRequest
├── docker.go                         # waitForCount, waitForServiceStatus, skipIfHTTPProbeUnreachable
├── token.go                          # readToken
├── ports.go                          # pickTwoFreeTCPPorts
├── snippet.go                        # snippet (string truncation for error output)
├── images.go                         # NEW — pinned image digests
├── attrs.go                          # SCAttrs(t, spec, sc) helper for T.Attr tagging
└── copyrepo.go                       # findRepoRoot, copyRepoForTest (from v0.4.1 tool_directive_test)

internal/version/
├── runtime.go                        # MODIFIED — add Distribution field to SystemInfo + detector
└── runtime_test.go                   # MODIFIED — new test cases for detector

internal/web/templates/
├── system.html                       # MODIFIED — add Distribution row in System Info table
└── index.html                        # MODIFIED — footer card shows Distribution alongside existing fields

internal/reconciler/
└── reconciler_test.go                # MODIFIED — 2 time.Sleep calls migrated to synctest (lines 162, 168)

internal/probe/
├── manager_test.go                   # MODIFIED — 2 time.Sleep calls migrated to synctest (lines 89, 123)
└── http_test.go                      # MODIFIED — 2 time.Sleep calls migrated to synctest (lines 69, 87)

tests/e2e/                            # all *_test.go files
├── *_test.go (existing)              # MODIFIED — import harness package, drop inline duplicates
├── install_sh_test.go                # NEW — //go:build e2e + scratch Linux container smoke
├── docker_image_test.go              # NEW — //go:build e2e + pull local image + verify boot
└── bench_smoke_test.go               # NEW — //go:build e2e + `make bench` exits 0 with ≥6 metrics

Dockerfile.proxa                      # NEW — FROM scratch, USER 65532, EXPOSE 8080 80 443, VOLUME /data
Dockerfile.proxa-agent                # NEW — FROM scratch, ENTRYPOINT /usr/local/bin/proxa-agent

install.sh                            # NEW — POSIX shell, downloads + verifies + installs binary
docs/install/install.sh               # NEW — published via GitHub Pages (symlink or mirror of install.sh)

cmd/coverage-gate/                    # NEW — small Go binary that parses coverage.out
└── main.go                           # ~80 LOC; per-package summary, highlight <60%, always exits 0

.coverage-allowlist                   # NEW — empty file with comment header explaining purpose

.github/workflows/
└── release.yml                       # NEW — on tag v*, run goreleaser + buildx + push to GHCR + attach checksums

.goreleaser.yml                       # MODIFIED — add dockers + docker_manifests blocks for multi-arch

Makefile                              # MODIFIED — add test-quick, test-integ, test-e2e, bench, cover targets

go.mod                                # MODIFIED — add `tool github.com/goreleaser/goreleaser/v2/cmd/goreleaser`

docs/
├── operations.md                     # MODIFIED — add sections: container deploy, bench expectations, coverage allowlist, future SBOM/cosign work
└── licenses.md                       # MODIFIED — refresh log entry for v0.4.2 (Goreleaser MIT confirmed)
```

**Structure Decision**: Existing single-binary `internal/` layout preserved. Two new top-level support directories: `bench/` (benchmark harness, not Proxa runtime code) and `tests/e2e/internal/harness/` (consolidated test helpers as a real Go package). No new packages under `internal/`. The new `cmd/coverage-gate/` is a tiny dev tool — alternative is shell script (decided in R-005 to use Go for cross-platform).

## Phase 0 — Research (output: research.md)

Six open questions to lock before tasks:

1. **R-001 — Release tool choice**: Extend existing Goreleaser config vs replace with hand-rolled Actions matrix. Decision in research.md (recommend extend — zero migration cost, idiomatic for Go).
2. **R-002 — Goreleaser via Go tool directive**: Confirm pattern from v0.4.1 (staticcheck precedent). License audit (MIT). Exact `go get -tool` command + Makefile invocation.
3. **R-003 — install.sh hosting**: `docs/install/install.sh` via GitHub Pages from main vs gh-pages orphan branch. Recommend (A) for less moving parts.
4. **R-004 — Synctest scope**: 6 specific sleeps in 3 files locked. Verify synctest + httptest.Server interaction in http_test.go before locking implementation pattern.
5. **R-005 — Coverage script language**: Go binary (cross-platform) vs shell (awk portability issues). Recommend Go.
6. **R-006 — Bench layout + Linux-only memory bench**: `bench/` top-level, idle-memory via `//go:build linux` + portable fallback elsewhere.

## Phase 1 — Design & Contracts (output: data-model.md, contracts/, quickstart.md)

### Data model

Minimal additions (full detail in `data-model.md`):
- **SystemInfo.Distribution** — new in-memory enum field (`"binary" | "docker" | "unknown"`).
- **bench/binary-size-baseline.txt** — single integer, committed.
- **.coverage-allowlist** — newline-delimited paths, comment-friendly.

No persistent entity changes. No SQLite schema migration.

### Contracts

Five contracts in `contracts/` covering every external/operator-visible surface this release adds:
- `install-sh.md`, `release-pipeline.md`, `dockerfiles.md`, `make-targets.md`, `coverage-gate.md`.

### Quickstart

8 validation walk-throughs covering each FR + SC. Detail in `quickstart.md`.

### Agent context

CLAUDE.md has no SPECKIT markers — per v0.4.1 precedent, skip insertion.

## Post-Design Constitution Check

Re-evaluated after Phase 1 design. **No new violations introduced.**

**Critical multi-host gate check** (per user input architectural note):

> "After v0.4.2, can v0.5 reasonably add `proxa-agent` real impl without re-doing CI/release pipeline?"

**Answer: YES.** v0.4.2 establishes:
- `Dockerfile.proxa-agent` exists with correct image shape (LABELS, ENTRYPOINT). v0.5 only changes the binary contents (real impl vs stub), not the Dockerfile.
- GHCR registry path `ghcr.io/proxa-server/proxa-agent:vX.Y.Z` is live and discoverable.
- Multi-arch buildx pipeline produces the agent image automatically on every tag.
- `install.sh` pattern is ready to be cloned for an `agent-bootstrap.sh` in v0.5.

v0.5 multi-host work CAN be 100% feature engineering, with zero release-infrastructure refactor. **The architectural gate is met.**

## Complexity Tracking

*Not applicable — no constitution violations to justify.*

# 000-foundation — Quickstart Validation Results

Generated: 2026-05-13 — final task (T030) of the foundation feature.

## Environment

| Item | Value |
|---|---|
| OS | macOS (darwin/arm64) |
| Go | `go1.26.3` (go.mod pins `go 1.26`) |
| Branch | `000-foundation` |
| HEAD at validation | commit `1a3b512` (T029 — goreleaser scaffold) |
| Working tree | clean (no untracked, no unstaged) |

## Spec Success Criteria

| ID | Criterion | Status | Evidence |
|---|---|---|---|
| SC-001 | `go build ./...` succeeds with zero errors on a clean clone | ✅ PASS | `CGO_ENABLED=0 go build ./...` exits 0; `make build` produces `bin/proxa` and `bin/proxa-agent` (≈2.5 MB each, statically linked). |
| SC-002 | `go vet ./...` reports zero issues | ✅ PASS | `make lint` runs `go vet ./...` and `staticcheck ./...` (v0.7.0); both exit 0. |
| SC-003 | Every interface has at least a stub implementation that returns `ErrNotImplemented` | ✅ PASS | Each interface package ships a private `noop*` stub with a compile-time `var _ Iface = noopT{}` assertion. Files: `internal/runtime/runtime.go`, `internal/store/store.go`, `internal/secrets/secrets.go`, `internal/ingress/ingress.go`, `internal/auth/auth.go`, `internal/auth/policy.go`. (`internal/security` ships real logic per the contract.) |
| SC-004 | `proxa version` prints the version string | ✅ PASS | `bin/proxa version` → `proxa version 1a3b512 (commit 1a3b512, built 2026-05-14T01:02:09Z)`. `bin/proxa-agent version` works identically. Version vars are injected from `git describe`/`git rev-parse` via `Makefile` `-ldflags`. |
| SC-005 | GitHub Actions CI passes on push to main | ⏳ READY (verify after push) | `.github/workflows/ci.yml` is in place: matrix `{ubuntu-latest, macos-latest} × {go: 1.26.x}` runs `go mod tidy` round-trip, `go vet`, `staticcheck@v0.7.0`, `go test -race -count=1`, `go build` — all under `CGO_ENABLED=0`. Actual green status confirms only after the branch is pushed and the workflow runs on GitHub-hosted runners. |

## Functional Requirements

| ID | Requirement | Status |
|---|---|---|
| FR-001 | Repository is a Go module at `github.com/proxa-server/proxa` | ✅ |
| FR-002 | Monorepo layout per Tech Spec §4.2 | ✅ |
| FR-003 | All core interfaces defined in their `internal/` packages with documentation comments referencing the contract | ✅ |
| FR-004 | Shared types (`TaskDef`, `Service`, `Job`, `Node`, `Project`, `Policy`, `Subject`) defined in `pkg/types/` | ✅ |
| FR-005 | `cmd/proxa/main.go` compiles and prints version | ✅ |
| FR-006 | `Makefile` with `build`, `test`, `lint`, `clean` (plus `tidy`, `help`) | ✅ |
| FR-007 | CI configured (`go vet`, `staticcheck`, `go test`, `go build`) | ✅ |
| FR-008 | `.goreleaser.yml` scaffolded for future multi-arch release | ✅ |
| FR-009 | `go.mod` contains only dependencies actually imported | ✅ — `go.mod` has zero third-party `require` entries; `go.sum` does not exist. The only external tool reference is `staticcheck@v0.7.0` invoked via `go run`, which fetches into the module cache without modifying `go.mod`. |

## Constitution Alignment

All eleven principles honored at this checkpoint. Notable evidence:

- **§I Architecture First** — Seven interfaces declared before any concrete implementation. Every later feature implements against these contracts.
- **§II Security by Default** — `internal/security` ships real logic. `SecurityProfile{}` zero value passes `Validate()`; `Apply()` injects `CapDrop=["ALL"]` and `NoNewPrivileges=true`. Validated by 14 table-driven subtests.
- **§III Project Scoping from v0.0** — Every state-store and runtime method that touches a service/job/secret takes `project string` as a required parameter. `Project` entity exists in `pkg/types/`.
- **§IV Go Idioms** — Stdlib-only. Every interface method that does I/O takes `context.Context` as the first parameter. Errors wrap with `fmt.Errorf("pkg/<impl>: %w", err)` per the contract docs.
- **§V Single Binary** — Two binaries (`proxa`, `proxa-agent`), both `CGO_ENABLED=0`, both statically linked.
- **§VI Cluster-Ready Design** — `cmd/proxa-agent/` exists from this feature. `RoleAgent` defined in `pkg/types/policy.go`. `NodeRoleServer` and `NodeRoleAgent` enumerated.
- **§IX Permissive License** — Apache 2.0 `LICENSE` at root. Zero third-party deps means zero license risk at this checkpoint.
- **§X Honest Scope** — No half-implemented features. `internal/security` is real logic; everything else returns `ErrNotImplemented`. Reserved slots (`proto/`, `web/`) ship a `README.md` describing intent.
- **§XI Commit Strategy** — `git log --oneline 000-foundation` shows one commit per task with `<type>(<scope>): <description>` messages. (Two follow-up commits from `/speckit.analyze` and `/speckit.implement` — F1–F5 fixes and the staticcheck v0.7.0 bump — are scoped as `docs(spec)` and `chore(spec)` respectively.)

## Known follow-ups (not blockers for closing 000-foundation)

1. SC-005 ("CI green on push") cannot be confirmed locally; mark this success criterion definitively PASS only after the first push to GitHub fires the workflow and it returns green. If it fails, root-cause and patch the workflow as a `chore(ci): fix …` commit.
2. The original `tasks.md` Phase 4 contracts (T020–T025) called for unexported `noop*` stubs. During implementation we added one-line `var _ Iface = noopT{}` compile-time assertions to (a) confirm the stub satisfies the interface and (b) make staticcheck consider the stub methods reachable. This is a Go idiom; documented in the package comments.
3. The staticcheck pin moved from `v0.5.1` (chosen during `/speckit.analyze` remediation) to `v0.7.0` because `v0.5.1` does not compile against Go 1.26. Recorded in commit `91878d2`.

## Outcome

Foundation feature is **DONE**. Ready for `001-core-loop` planning.

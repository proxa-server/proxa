---
description: "Tasks for 000-foundation — project scaffolding"
---

# Tasks: Foundation — Project Scaffolding

**Input**: Design documents from `/specs/000-foundation/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`

**Tests**: Spec does not request TDD. The only test task in this feature is `internal/security/profile_test.go` (SC-000-2 cannot be claimed PASS without it; `security` is the only real-logic component).

**Organization**: Two user stories from spec — **US1** (SC-000-1: structure is discoverable) and **US2** (SC-000-2: interfaces are implementable). US1 ships the directory skeleton + reserved-slot READMEs + agent stub; US2 ships the seven interface contracts + the security package's real logic. Both share the Setup and Foundational phases.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User-story tag (US1, US2). Setup, Foundational, and Polish tasks omit it.
- Every task includes its file path(s).

## Commit policy (constitution §XI)

One commit per task. Message format: `<type>(<scope>): <description>`. Types: `feat`, `fix`, `test`, `docs`, `refactor`, `chore`. Scope = `internal/<pkg>` for tasks under `internal/`, `types` for `pkg/types/`, `cmd` for `cmd/*`, `ci` for `.github/workflows/`, `build` for `Makefile`/`.goreleaser.yml`/`go.mod`, `docs` for top-level docs, `chore` for everything else.

## Constraint reminders

- `CGO_ENABLED=0` (constitution §IV).
- `go.mod` MUST NOT contain dependencies that are not actually imported (FR-009). For this feature that means **no third-party deps**; every file imports stdlib + this module's own packages only.
- Every interface method that does I/O takes `context.Context` as first param (§IV).
- `SecurityProfile{}` zero-value MUST be the safest configuration (§II).
- Every state-store/secrets/runtime operation is project-scoped (§III).

---

## Phase 1: Setup

**Purpose**: Module identity, repo boilerplate, directory skeleton.

- [X] T001 Create `go.mod` at repo root with `module github.com/proxa-server/proxa` and `go 1.26` directive. Run `go mod tidy` (will be a no-op; no deps). Commit: `chore(build): initialize go module at go 1.26`.

- [X] T002 [P] Add Apache-2.0 `LICENSE` file at repo root (verbatim Apache License 2.0 text, copyright `Proxa contributors`). Commit: `chore(repo): add Apache 2.0 license`.

- [X] T003 [P] Add `.gitignore` at repo root covering `bin/`, `*.test`, `*.out`, `coverage.*`, `.DS_Store`, `proxa.db*`, and editor crud (`.idea/`, `.vscode/settings.json`). Commit: `chore(repo): add .gitignore`.

- [X] T004 Create the empty directory skeleton with `.gitkeep` placeholders where needed: `cmd/proxa/`, `cmd/proxa-agent/`, `internal/auth/`, `internal/ingress/`, `internal/runtime/`, `internal/secrets/`, `internal/security/`, `internal/store/`, `internal/version/`, `pkg/types/`, `proto/`, `web/`, `.github/workflows/`. Commit: `chore(repo): create monorepo directory skeleton`.

**Checkpoint**: `go build ./...` runs (compiles zero files; no errors).

---

## Phase 2: Foundational — Shared Types + Security (Blocking Prerequisites)

**Purpose**: Public types (`pkg/types/`) and the `internal/security` package. Every other package — and both binaries — depend on at least one of these. `internal/security` is the only place in this feature with real logic; it ships the constitution-§II defaults and the validator that downstream callers will invoke.

**⚠️ CRITICAL**: US1 and US2 work cannot start until this phase is complete.

- [X] T005 Create `internal/security/profile.go` with the `SecurityProfile` struct (fields per `contracts/securityprofile.md`) plus the `Default()`, `Apply()`, and `Validate()` functions. Doc comments on each exported symbol. Imports: `errors` only. Commit: `feat(security): add SecurityProfile struct with safe defaults and validation`.

- [X] T006 Create `internal/security/profile_test.go` with table-driven tests covering: (a) `Default()` returns `CapDrop=["ALL"]` and `*NoNewPrivileges==true`, (b) `Apply()` fills zero-valued fields without overriding caller choices, (c) `Validate()` rejects `User=root` without `AllowRoot=true` and rejects `NoNewPrivileges=false` without `AllowRoot=true`, (d) the zero-value `SecurityProfile{}` passes `Validate()`. Imports: `testing` only. Commit: `test(security): cover Default/Apply/Validate with table-driven tests`.

- [X] T007 Create `pkg/types/security.go` exposing the alias `type SecurityProfile = security.SecurityProfile` so public callers reach the type through `pkg/types`. File-level comment notes the cross-package design from `contracts/securityprofile.md`. Imports: `github.com/proxa-server/proxa/internal/security`. Depends on T005. Commit: `feat(types): re-export SecurityProfile from internal/security`.

- [X] T008 [P] Create `pkg/types/project.go` with the `Project` struct (`Name`, `CreatedAt`) per `data-model.md`. JSON tags only (TOML not needed — projects are API-created, not TOML-declared). Imports: `time`. Commit: `feat(types): add Project entity`.

- [X] T009 [P] Create `pkg/types/subject.go` with the `Subject` struct (`ID`, `Name`, `Email`, `Provider`, `Metadata`). JSON tags only. Imports: none. Commit: `feat(types): add Subject entity`.

- [X] T010 [P] Create `pkg/types/node.go` with `Node`, `NodeRole`, `NodeStatus`, `NodeResources` per `data-model.md`. JSON tags only. Imports: `time`. Commit: `feat(types): add Node entity with role and resource fields`.

- [X] T011 [P] Create `pkg/types/policy.go` with `Policy` and the `Role` enum (`admin`, `editor`, `viewer`, `agent`). JSON tags only. Imports: `time`. Commit: `feat(types): add Policy with RBAC role enum`.

- [X] T012 [P] Create `pkg/types/taskdef.go` with `TaskDef` plus its sub-types `VolumeMount`, `PortSpec`, `DeployStrategy` enum, `HealthCheck`, and `ResourceLimits` per `data-model.md`. Both TOML and JSON tags. The `Security` field references `SecurityProfile` (the alias from T007). Imports: `time`. Depends on T007. Commit: `feat(types): add TaskDef and its sub-types`.

- [X] T013 [P] Create `pkg/types/service.go` with `Service`, `ServiceStatus` enum, `ReplicaState`, `DeploymentRecord` per `data-model.md`. JSON tags only. Imports: `time`. Depends on T012 (uses `TaskDef`). Commit: `feat(types): add Service entity with replica and deployment-history fields`.

- [X] T014 [P] Create `pkg/types/job.go` with `Job` and `JobRun` per `data-model.md`. JSON tags only. Imports: `time`. Depends on T012 (uses `TaskDef`). Commit: `feat(types): add Job entity with run history`.

**Checkpoint**: `go test ./internal/security/...` is green; `go build ./pkg/types/...` succeeds. The constitution-§II defaults are now enforceable; downstream interfaces can rely on `types.SecurityProfile` and the entity types.

---

## Phase 3: User Story 1 — Structure is discoverable (Priority: P1) 🎯 MVP

**Goal**: A developer or AI agent clones the repo and can locate `cmd/proxa`, `cmd/proxa-agent`, `internal/`, `pkg/types/`, `proto/`, `web/` without guessing. The agent stub compiles. Reserved-slot READMEs explain what *will* live there.

**Independent Test**: Run `tree -L 2 -I 'specs|.specify|.git'` and confirm the layout matches Technical Spec §4.2. `go build ./cmd/proxa-agent` succeeds and prints a version string. Every reserved directory (`proto/`, `web/`) contains a `README.md` whose first line states the directory's purpose.

### Implementation for User Story 1

- [X] T015 [P] [US1] Create `internal/version/version.go` exporting `var Version = "dev"`, `var Commit = "unknown"`, `var BuildDate = "unknown"` (mutable so `-ldflags "-X internal/version.Version=..."` can override at build time). Doc comment on each var. Imports: none. Commit: `feat(version): add build-time version vars`.

- [X] T016 [US1] Create `cmd/proxa-agent/main.go` with a minimal entrypoint: when invoked as `proxa-agent version` it prints `proxa-agent version <Version> (commit <Commit>, built <BuildDate>)`; any other arg prints usage to stderr and exits 2. Imports: `fmt`, `os`, `github.com/proxa-server/proxa/internal/version`. Depends on T015. Commit: `feat(cmd): add proxa-agent binary stub with version subcommand`.

- [X] T017 [P] [US1] Create `proto/README.md` (single section, two short paragraphs): purpose is the gRPC schema for control-plane↔agent communication; landing in feature 001 or 002. Commit: `docs(proto): add placeholder README`.

- [X] T018 [P] [US1] Create `web/README.md`: purpose is HTMX + Alpine.js + Tailwind dashboard assets, embedded into the binary via `go:embed`; landing in the dashboard feature. Reference `specs/_reference/dashboard-mockup.html` as directional but non-authoritative. Commit: `docs(web): add placeholder README with mockup reference`.

- [X] T019 [US1] Update `README.md` at repo root: short overview (one paragraph), repo layout table mirroring `quickstart.md`, link to `specs/000-foundation/quickstart.md` and to the constitution. Replace whatever currently exists from the legacy Python project. Commit: `docs(repo): rewrite README for Go orchestrator structure`.

**Checkpoint**: `go build ./cmd/proxa-agent && ./proxa-agent version` works. Every reserved directory has a README. README at repo root reflects current project.

---

## Phase 4: User Story 2 — Interfaces are implementable (Priority: P1, equal MVP)

**Goal**: An AI coding agent (or a contributor) handed any of the seven interface contracts can implement it without ambiguity — file location is unique, signature is fixed, behavioral contract is documented in `contracts/`, and a stub returning `ErrNotImplemented` already compiles.

**Independent Test**: For each interface package, `go doc ./internal/<pkg>.<Interface>` returns a non-empty doc comment that references `specs/000-foundation/contracts/<pkg>.md`. `go vet ./...` and `staticcheck ./...` report zero issues. A noop stub in each package satisfies the interface and returns `<Pkg>.ErrNotImplemented` for every method.

### Implementation for User Story 2

- [X] T020 [P] [US2] Create `internal/runtime/runtime.go` with the `Runtime` interface, all parameter/return types from `contracts/runtime.md` (`ContainerSpec`, `ImageInfo`, `ContainerInfo`, `ListFilter`, `LogOpts`, `ContainerStats`, `ExecOpts`, `ExecResult`), and `var ErrNotImplemented = errors.New("runtime: not implemented")`. Doc comment on `Runtime` interface points at `specs/000-foundation/contracts/runtime.md`. Add a private `noopRuntime` type implementing every method as `return …, ErrNotImplemented`. Imports: `context`, `errors`, `io`, `time`, `github.com/proxa-server/proxa/pkg/types`. Depends on T012. Commit: `feat(runtime): declare Runtime interface and noop stub`.

- [X] T021 [P] [US2] Create `internal/store/store.go` with the `StateStore` interface plus `Tx`, `ServiceEvent`, and the three sentinel errors (`ErrNotFound`, `ErrAlreadyExists`, `ErrNotImplemented`) from `contracts/statestore.md`. Doc comment on the interface points at the contract. Private `noopStore` returns `ErrNotImplemented` from every method. Imports: `context`, `errors`, `time`, `github.com/proxa-server/proxa/pkg/types`. Depends on T008, T010, T011, T013, T014. Commit: `feat(store): declare StateStore interface and noop stub`.

- [X] T022 [P] [US2] Create `internal/secrets/secrets.go` with the `SecretsStore` interface and `SecretMeta` type plus `ErrNotFound`, `ErrNotImplemented` per `contracts/secretsstore.md`. Doc comment notes the §II hard rules (no plaintext on the wire, no secret material in logs). Private `noopSecretsStore`. Imports: `context`, `errors`, `time`. Commit: `feat(secrets): declare SecretsStore interface and noop stub`.

- [X] T023 [P] [US2] Create `internal/ingress/ingress.go` with the `IngressController` interface, `Config`, `L4Listener`, `Route`, `RouteMatch`, and `ErrNotFound`/`ErrNotImplemented` per `contracts/ingresscontroller.md`. Private `noopIngressController`. Imports: `context`, `errors`, `net`. Commit: `feat(ingress): declare IngressController interface and noop stub`.

- [ ] T024 [P] [US2] Create `internal/auth/auth.go` with the `Authenticator` interface, the `Chain` helper (implementation included — it's trivial, not a stub), and the sentinel errors `ErrUnauthenticated`, `ErrNotSupported`, `ErrNotImplemented` per `contracts/authenticator.md`. Doc comment on `Authenticator` references the contract; doc on `Chain` notes first-match-wins. Imports: `context`, `errors`, `net/http`, `github.com/proxa-server/proxa/pkg/types`. Depends on T009. Commit: `feat(auth): declare Authenticator interface and Chain composer`.

- [ ] T025 [US2] Create `internal/auth/policy.go` with the `PolicyEngine` interface, `AuthzRequest`, `ResourceRef`, `Verb` and `ResourceKind` enums, and `ErrForbidden`/`ErrNotImplemented` per `contracts/policyengine.md`. Private `noopPolicyEngine`. Same package as T024 — must follow T024 (single-file conflict on package-level naming). Imports: `context`, `errors`, `github.com/proxa-server/proxa/pkg/types`. Depends on T024. Commit: `feat(auth): declare PolicyEngine with RBAC verb and kind enums`.

- [ ] T026 [US2] Create `cmd/proxa/main.go` mirroring the agent stub from T016 but for the control-plane binary: `proxa version` prints version string; any other arg prints usage and exits 2. This is the file referenced by SC-004. Imports: `fmt`, `os`, `github.com/proxa-server/proxa/internal/version`. Depends on T015. Commit: `feat(cmd): add proxa control-plane binary stub with version subcommand`.

**Checkpoint**: `go build ./...` succeeds. `go vet ./...` and `staticcheck ./...` report zero issues. `./proxa version` and `./proxa-agent version` both print a string. Every interface has a doc comment that points at its contract file.

---

## Phase 5: Polish & Cross-Cutting

**Purpose**: Tooling and release scaffolding so the success criteria (SC-001 through SC-005) can be claimed PASS.

- [ ] T027 Create `Makefile` at repo root with phony targets `build` (builds both binaries into `bin/` with `CGO_ENABLED=0` and `-ldflags` injecting version/commit/date from `git describe`/`git rev-parse`), `test` (`go test ./...`), `lint` (`go vet ./... && staticcheck ./...` — `staticcheck` invoked via `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0` so it doesn't need to be globally installed), `clean` (`rm -rf bin/`), and `tidy` (`go mod tidy`). Commit: `chore(build): add Makefile with build/test/lint/clean/tidy targets`.

- [ ] T028 [P] Create `.github/workflows/ci.yml`: on push to any branch and on PR to `main`, run a matrix of `{os: ubuntu-latest, macos-latest}` × `{go: '1.26.x'}` executing `go vet ./...`, `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...`, `go test -race -count=1 ./...`, `go build ./...` (with `CGO_ENABLED=0`). Cache the module cache. Commit: `chore(ci): add GitHub Actions workflow (vet, staticcheck, test, build)`.

- [ ] T029 [P] Create `.goreleaser.yml` scaffolding multi-arch builds for `linux/{amd64,arm64}` and `darwin/{amd64,arm64}` of both `cmd/proxa` and `cmd/proxa-agent`. `CGO_ENABLED=0`. `archives` block produces tar.gz with `LICENSE` + `README.md` included. **Do not** wire a `release` workflow yet (no tag triggers); this is scaffolding only. Commit: `chore(build): scaffold goreleaser config for multi-arch release`.

- [ ] T030 Run the quickstart validation checklist from `quickstart.md` end-to-end: clean clone of `000-foundation` branch, `make build`, `make test`, `make lint`, `./bin/proxa version`, `./bin/proxa-agent version`, confirm CI green on push. If any item fails, file a follow-up task and do not mark this complete. Commit: `docs(spec): record quickstart validation results in specs/000-foundation/`.

**Checkpoint (end of feature)**: All five spec success criteria are demonstrably met. `git log --oneline 000-foundation` shows one commit per task with constitution-§XI-compliant messages.

---

## Dependencies & Execution Order

### Phase ordering

- **Phase 1 (Setup)**: T001 → T002, T003, T004 may run in parallel after T001.
- **Phase 2 (Foundational)**: T005 → T006 → T007. Once T007 lands, T008–T014 may run in parallel (they touch different files in `pkg/types/`, though T012 must precede T013 and T014 because both reference `TaskDef`).
- **Phase 3 (US1)**: Starts after Phase 2 complete. T015 → T016; T017, T018, T019 in parallel.
- **Phase 4 (US2)**: Starts after Phase 2 complete. T020, T021, T022, T023, T024 in parallel (different packages, all depend on `pkg/types/`). T025 follows T024 (same package). T026 depends on T015.
- **Phase 5 (Polish)**: Starts after Phases 3 and 4 complete. T027 first (Makefile is consumed by T028 indirectly); T028, T029 in parallel after T027. T030 last.

### Cross-story note

US1 and US2 are both Priority P1 and together constitute the MVP for this feature. There is no "ship US1 alone" path because `go build ./...` (SC-001) requires the US2 interface files to exist. The split exists for parallelism and traceability against SC-000-1 and SC-000-2, not for incremental release.

### Parallel batches

**Batch A (Phase 1, after T001):** T002, T003, T004.

**Batch B (Phase 2, after T007):** T008, T009, T010, T011, T012. Then T013 + T014 in parallel.

**Batch C (Phase 3):** T017, T018, T019 (after T015 + T016 for completeness, though T017–T019 don't strictly depend on T015/T016).

**Batch D (Phase 4, after T012/T013/T014 from Phase 2):** T020, T021, T022, T023, T024 — five interface packages, no shared files.

**Batch E (Phase 5, after T027):** T028, T029.

---

## Parallel Example: Phase 2 foundational entity files

```bash
# After T005/T006/T007 land, kick off the pkg/types/ wave:
Task: "Create pkg/types/project.go (T008)"
Task: "Create pkg/types/subject.go (T009)"
Task: "Create pkg/types/node.go (T010)"
Task: "Create pkg/types/policy.go (T011)"
Task: "Create pkg/types/taskdef.go (T012)"

# Then once T012 is in:
Task: "Create pkg/types/service.go (T013)"
Task: "Create pkg/types/job.go (T014)"
```

## Parallel Example: Phase 4 interface declarations

```bash
# After Phase 2 complete, the five interface packages can land together
# (each is a distinct package; no shared symbols):
Task: "Create internal/runtime/runtime.go (T020)"
Task: "Create internal/store/store.go (T021)"
Task: "Create internal/secrets/secrets.go (T022)"
Task: "Create internal/ingress/ingress.go (T023)"
Task: "Create internal/auth/auth.go (T024)"

# Then T025 (same package as T024), then T026 (uses internal/version).
```

---

## Implementation Strategy

### MVP (this feature is the MVP)

1. Phase 1 (Setup) — module file + LICENSE + dir tree.
2. Phase 2 (Foundational) — `internal/security` with real logic + tests; `pkg/types/` field definitions.
3. Phases 3 + 4 in parallel — US1 (structure discoverable) + US2 (interfaces implementable).
4. Phase 5 (Polish) — Makefile, CI, goreleaser, README, quickstart validation.

### Stop conditions per phase

- After Phase 1: `go build ./...` is a no-op success.
- After Phase 2: `go test ./internal/security/...` green; `go build ./pkg/types/...` green.
- After Phases 3+4: `go build ./...` succeeds, both `version` subcommands work, `go vet ./...` and `staticcheck ./...` clean.
- After Phase 5: CI green on push; all five spec success criteria satisfied.

### Commit cadence

Constitution §XI: one commit per task, scoped to the package being touched. Never bundle two tasks into one commit. Hooks `before_implement` / `after_implement` (optional) may be used to snapshot before/after each task during `/speckit.implement`.

---

## Notes

- No third-party dependencies enter `go.mod` in this feature. `go.sum` will exist (empty or near-empty) only after `go mod tidy` resolves the stdlib indirect graph.
- The `noopRuntime`, `noopStore`, `noopSecretsStore`, `noopIngressController`, `noopPolicyEngine` types in T020–T025 are private (`noop*`). They exist for unit tests in future features (callers can pass them to satisfy the interface). They are NOT exported.
- `staticcheck` is invoked via `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0` from the Makefile and CI to avoid requiring a global install. `go run <pkg>@<version>` does not modify `go.mod` (the tool is fetched into the module cache and executed in isolation), so FR-009 is preserved. The version is pinned for reproducibility; bump deliberately by editing the Makefile and `ci.yml` together. Staticcheck is BSD-3-licensed (constitution §IX compliant).
- T030 is the only task that produces a documentation artifact rather than code. Its commit captures the validation evidence so the spec's success criteria are traceable in git.

# Implementation Plan: Foundation — Project Scaffolding

**Branch**: `000-foundation` | **Date**: 2026-05-13 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/000-foundation/spec.md`

## Summary

Establish the Go monorepo scaffolding, module identity, shared type definitions, and core interface declarations that every subsequent feature depends on. This is a structural foundation: nothing user-facing ships in this feature, but every later feature loses ambiguity about where files go and what contracts they implement.

Concrete deliverables: `go.mod` at `github.com/proxa-server/proxa`; the monorepo layout from Technical Spec §4.2; seven core interfaces (`Runtime`, `StateStore`, `SecretsStore`, `IngressController`, `Authenticator`, `PolicyEngine`, plus the `SecurityProfile` struct) each in its own `internal/<pkg>/`; shared types in `pkg/types/`; `cmd/proxa/main.go` that compiles and prints `proxa version`; Makefile with `build`/`test`/`lint`/`clean`; GitHub Actions CI running `go vet`, `staticcheck`, `go test`, `go build`; scaffolded `.goreleaser.yml`.

## Technical Context

**Language/Version**: Go 1.26.x (`CGO_ENABLED=0`, single static binary; verified by Makefile and CI matrix; `go.mod` pins `go 1.26`)

**Primary Dependencies** (all stdlib-first; only added when stdlib genuinely insufficient):
- `github.com/go-chi/chi/v5` — HTTP router (later features)
- `github.com/spf13/cobra`, `github.com/spf13/viper` — CLI + config (later features)
- `modernc.org/sqlite` — pure-Go SQLite, no CGO (later features)
- `github.com/docker/docker/client` — runtime backend behind `Runtime` interface (later features)
- `github.com/caddyserver/certmagic` — L7 TLS automation (later features)
- `filippo.io/age` — secret encryption (later features)
- HTMX + Alpine.js + Tailwind — checked into `web/` and embedded via `go:embed` (later features)

For Phase 000, **only modules actually imported by scaffolding code land in `go.mod`** (FR-009). Most of the above show up in `go.mod` only when the feature that needs them is implemented. Foundation imports the stdlib + nothing else, except where a stub demands a single import (none expected).

**Storage**: N/A for this feature. Interface declared (`StateStore`) but not implemented beyond `ErrNotImplemented` stub.

**Testing**: `testing` (stdlib) with table-driven tests; `github.com/stretchr/testify/assert` added only when stdlib comparisons become noisy (not in this feature).

**Target Platform**: Linux/macOS/Windows server. Multi-arch binary (`amd64`, `arm64`) via goreleaser. Single-node v0.x; cluster-ready seams via interfaces.

**Project Type**: CLI + server, single binary (`cmd/proxa`) + agent binary (`cmd/proxa-agent`). Monorepo layout per Technical Spec §4.2.

**Performance Goals**: Not applicable for scaffolding. Carried forward to feature `001-core-loop`.

**Constraints**:
- `CGO_ENABLED=0` enforced in Makefile and CI.
- No transitive AGPL/SSPL/BSL dependencies (constitution §IX).
- Every interface ships with documentation comments describing the contract.
- No speculative `go get` — `go.mod` must round-trip through `go mod tidy` clean.

**Scale/Scope**: Foundation only. ~25 files: module file, 2 main.go files, 7 interface files (each with stub), 7 type files, Makefile, CI workflow, goreleaser config, basic README/LICENSE.

## Constitution Check

*GATE: PASS pre-Phase-0. Re-checked post-Phase-1 — still PASS.*

| Principle | Status | How this plan complies |
|---|---|---|
| §I Architecture First | ✅ | This feature *defines* the seven non-negotiable interfaces before any concrete implementation lands. Every later feature imports from these packages. |
| §II Security by Default | ✅ | No runtime behavior shipped. Stubs return `ErrNotImplemented`; cannot be used insecurely. `SecurityProfile` struct's zero value (`CapDrop: ALL`, `NoNewPrivileges: true`) is documented as the default. |
| §III Project Scoping from v0.0 | ✅ | `Project` is in the first `pkg/types/` cut. `Service`, `Job`, `Policy`, and `Subject` all carry a `Project` field from the start. No retrofit needed. |
| §IV Go Idioms | ✅ | Stdlib-first (`slog`, `net/http`, `context`, `errors`). No dependency added unless imported. `context.Context` is the first param of every I/O method on every interface. Error sentinel pattern: `var ErrNotImplemented = errors.New("not implemented")` in each package. |
| §V Single Binary | ✅ | One `cmd/proxa/main.go` + one `cmd/proxa-agent/main.go`. No Compose, no sidecars. Dashboard reserved for later features but slot exists at `web/`. |
| §VI Cluster-Ready Design | ✅ | `StateStore` is an interface from day one — SQLite slot in v0.x, etcd slot in v1.0. `Runtime`, `Authenticator`, etc. all the same. |
| §VII Declarative, Not Imperative | N/A (no behavior) | TOML parsing arrives in `001-core-loop`. Foundation just declares `TaskDef` as the target type. |
| §VIII Zero-Downtime by Default | N/A (no deploys) | Strategy enum reserved in `TaskDef`; honored when reconciler lands. |
| §IX Permissive License | ✅ | Apache 2.0 LICENSE file in root. Every dependency reviewed (see `research.md` — License Audit). No AGPL/SSPL/BSL. |
| §X Honest Scope | ✅ | Spec lists exactly what's in scope. Stubs `panic`-free; they return `ErrNotImplemented`. Nothing half-implemented. |
| §XI Commit Strategy | ✅ | Tasks (next phase) will be one-commit-per-task, scope = `internal/<pkg>` name. |

**Result**: PASS. `Complexity Tracking` section below remains empty.

## Project Structure

### Documentation (this feature)

```text
specs/000-foundation/
├── spec.md              # Source of truth (already exists)
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (one .md per interface)
│   ├── runtime.md
│   ├── statestore.md
│   ├── secretsstore.md
│   ├── ingresscontroller.md
│   ├── authenticator.md
│   ├── policyengine.md
│   └── securityprofile.md
└── tasks.md             # Phase 2 — generated by /speckit.tasks (not by this command)
```

### Source Code (repository root)

```text
proxa/
├── cmd/
│   ├── proxa/                      # Control-plane binary entrypoint
│   │   └── main.go                 # Prints `proxa version`; later wires server
│   └── proxa-agent/                # Per-node agent binary (later features)
│       └── main.go                 # Stub that prints `proxa-agent version`
├── internal/
│   ├── auth/
│   │   ├── auth.go                 # Authenticator interface + ErrNotImplemented
│   │   └── policy.go               # PolicyEngine interface + ErrNotImplemented
│   ├── ingress/
│   │   └── ingress.go              # IngressController interface
│   ├── runtime/
│   │   └── runtime.go              # Runtime interface (Docker behind it later)
│   ├── secrets/
│   │   └── secrets.go              # SecretsStore interface
│   ├── security/
│   │   └── profile.go              # SecurityProfile struct + safe-defaults helper
│   ├── store/
│   │   └── store.go                # StateStore interface
│   └── version/
│       └── version.go              # Version, Commit, BuildDate (set via -ldflags)
├── pkg/
│   └── types/                      # Public shared types
│       ├── taskdef.go
│       ├── service.go
│       ├── job.go
│       ├── node.go
│       ├── project.go
│       ├── policy.go
│       ├── security.go             # type alias to internal/security.SecurityProfile
│       └── subject.go
├── proto/                          # gRPC schema (reserved; empty in this feature)
│   └── README.md                   # "Reserved for control-plane↔agent gRPC"
├── web/                            # Dashboard assets (reserved; empty in this feature)
│   └── README.md                   # "Reserved for HTMX/Alpine/Tailwind"
├── .github/
│   └── workflows/
│       └── ci.yml                  # vet, staticcheck, test, build
├── .goreleaser.yml                 # Scaffolded; not yet wired to a release
├── Makefile                        # build, test, lint, clean
├── go.mod                          # github.com/proxa-server/proxa
├── go.sum
├── LICENSE                         # Apache 2.0
└── README.md                       # Existing; minor edit to point at structure
```

**Structure Decision**: Monorepo per Technical Spec §4.2 — one Go module, two binaries under `cmd/`, internal packages under `internal/`, public types under `pkg/types/`. Reserved slots (`proto/`, `web/`, `cmd/proxa-agent/`) ship with a `README.md` placeholder so directory intent is discoverable without code yet living there. The agent binary stub is included now (vs. deferred) because constitution §VI demands clustering seams be present from v0.

## Complexity Tracking

*Empty — no constitutional violations to justify.*

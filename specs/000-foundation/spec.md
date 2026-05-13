# Feature Spec: Foundation — Project Scaffolding

**Feature ID**: 000-foundation
**Status**: Ready for Planning
**Created**: 2026-05-13

## Overview

Set up the Go monorepo structure, module initialization, core type definitions, and interface declarations that all subsequent features build on. This is not a user-facing feature — it is the structural foundation that enables the architect + agent workflow.

## User Scenarios

### SC-000-1: Developer clones repo and understands structure

A developer (or AI agent) clones the repo and can immediately identify where each component lives: `cmd/proxa` for the control plane binary, `cmd/proxa-agent` for the agent binary, `internal/` for implementation packages, `pkg/types/` for shared types, and `proto/` for gRPC definitions.

### SC-000-2: Agent implements against interfaces

An AI coding agent receives an interface definition (e.g., `Runtime`) and can implement it in the correct package (`internal/runtime/`) without ambiguity about where files go or how packages import each other.

## Functional Requirements

- FR-001: Repository MUST be a Go module at `github.com/proxa-server/proxa`
- FR-002: Repository MUST use the monorepo layout defined in the technical spec (Section 4.2): `cmd/proxa/`, `cmd/proxa-agent/`, `internal/`, `pkg/types/`, `proto/`, `web/`
- FR-003: All core interfaces MUST be defined in their respective `internal/` packages with documentation comments
- FR-004: Shared types (`TaskDef`, `Service`, `Job`, `Node`, `Project`, `Policy`, `Subject`) MUST be defined in `pkg/types/`
- FR-005: The `cmd/proxa/main.go` MUST compile and print version info (`proxa version`)
- FR-006: A `Makefile` MUST exist with targets: `build`, `test`, `lint`, `clean`
- FR-007: CI MUST be configured (GitHub Actions) with: `go vet`, `staticcheck`, `go test ./...`, `go build ./...`
- FR-008: `.goreleaser.yml` MUST be scaffolded for future multi-arch release
- FR-009: `go.mod` MUST include only dependencies that are actually imported — no speculative `go get`

## Entities

- **TaskDef**: the parsed representation of a TOML service/job definition. Contains: project, name, image, replicas, stateful flag, security profile, env vars, volumes, expose/ports, strategy, health check, resources.
- **Service**: a running workload tracked in the state store. Contains: TaskDef reference, current status, replica states, deployment history, created/updated timestamps.
- **Job**: a one-shot or cron workload. Contains: TaskDef reference, schedule, last run status, duration.
- **Node**: a cluster member. Contains: ID, role (server/agent), address, resources (CPU/mem/disk), container count, last heartbeat, status.
- **Project**: a logical grouping. Contains: name, created timestamp.
- **Policy**: an RBAC binding. Contains: subject ID, role, project scope.
- **Subject**: an authenticated identity. Contains: ID, name, email, provider, metadata.

## Interfaces to Declare

```go
// internal/runtime/runtime.go
type Runtime interface { ... }

// internal/store/store.go
type StateStore interface { ... }

// internal/secrets/secrets.go
type SecretsStore interface { ... }

// internal/ingress/ingress.go
type IngressController interface { ... }

// internal/auth/auth.go
type Authenticator interface { ... }

// internal/auth/policy.go
type PolicyEngine interface { ... }

// internal/security/profile.go
type SecurityProfile struct { ... }
```

Full signatures are defined in the technical spec, Section 14 (Core Interfaces).

## Success Criteria

- SC-001: `go build ./...` succeeds with zero errors on a clean clone
- SC-002: `go vet ./...` reports zero issues
- SC-003: Every interface has at least a stub implementation that returns `ErrNotImplemented`
- SC-004: `proxa version` prints the version string
- SC-005: GitHub Actions CI passes on push to main

## Assumptions

- Go 1.26.x is available in the development environment
- The repo is at `github.com/proxa-server/proxa`
- No external services are needed for this feature (no Docker, no database)

## Dependencies

None — this is the first feature.

## References

- Technical Spec Section 4.2: Monorepo Structure
- Technical Spec Section 14: Core Interfaces (Go)
- Technical Spec Constitution (this repo's `.specify/memory/constitution.md`)

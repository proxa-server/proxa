# Feature Spec: Core Loop — Reconciliation Engine

**Feature ID**: 001-core-loop
**Status**: Ready for Planning
**Created**: 2026-05-13
**Depends on**: 000-foundation

## Overview

Implement the core reconciliation loop that makes Proxa an orchestrator: read a TOML task definition, create Docker containers to match the desired state, and continuously reconcile actual state back to desired state. This is the heartbeat of Proxa — everything else (ingress, dashboard, clustering) builds on top of this loop.

By the end of this feature, `proxa up service.toml` starts N replicas of a container with security hardening, and if any container dies, the reconciler restarts it automatically.

## User Scenarios

### SC-001-1: Deploy a service from a TOML file

An operator writes a TOML file defining a service (name, image, replicas, security profile). They run `proxa up service.toml`. Proxa parses the file, validates it, stores the desired state in SQLite, and creates the specified number of containers via the Docker API. Each container has the security profile applied (non-root, cap-drop-all, no-new-privileges).

### SC-001-2: Reconciler restarts crashed containers

A running container crashes (OOM, application error, `docker kill`). The reconciliation loop detects the discrepancy between desired replicas (3) and actual running containers (2) within its tick interval. It creates a replacement container with the same spec. The operator sees the replica count recover without manual intervention.

### SC-001-3: Scale by editing the file

The operator edits the TOML file to change `replicas = 3` to `replicas = 5` and runs `proxa up service.toml` again. Proxa detects the desired state change and creates 2 additional containers. If replicas are reduced, excess containers are stopped and removed.

### SC-001-4: Stop a service

The operator runs `proxa down api`. Proxa sets the desired replica count to 0 in the state store, and the reconciler stops and removes all containers for that service.

### SC-001-5: List running services

The operator runs `proxa ps`. Proxa queries the state store and the Docker runtime, then displays a table showing: service name, image, desired/actual replicas, status, and project.

## Functional Requirements

- FR-001: System MUST parse TOML task definitions conforming to the spec (Section 11) with validation of required fields and type checking
- FR-002: System MUST create containers via the Docker API with the SecurityProfile applied: non-root user (configurable UID:GID, default 1000:1000), `CapDrop: ALL`, `NoNewPrivileges: true`, read-only rootfs (configurable), seccomp default profile
- FR-003: System MUST store desired state (TaskDef) in SQLite via the StateStore interface, scoped by project
- FR-004: System MUST run a reconciliation loop on a configurable tick interval (default: 5 seconds) that compares desired vs actual state and acts on the diff
- FR-005: Reconciler MUST create containers when actual < desired
- FR-006: Reconciler MUST remove containers when actual > desired
- FR-007: Reconciler MUST replace containers when the image or config has changed (detected via hash comparison)
- FR-008: System MUST authenticate API/CLI access via bootstrap token generated on `proxa init`
- FR-009: System MUST create a local admin user on `proxa init` with bcrypt-hashed password
- FR-010: System MUST store the bootstrap token in `~/.proxa/token` with 0600 permissions
- FR-011: `proxa init` MUST create the data directory, SQLite database, master key (for future secrets), and bootstrap credentials
- FR-012: `proxa up <file>` MUST parse, validate, store, and trigger reconciliation for the given task definition
- FR-013: `proxa down <service>` MUST set desired replicas to 0 and trigger reconciliation
- FR-014: `proxa ps` MUST display service name, project, image, desired/actual replicas, and status
- FR-015: Container naming convention MUST be `proxa-{project}-{service}-{replica-index}`
- FR-016: All operations MUST respect project scoping — a service in project "socio-do" is independent from a same-named service in "kut-do"

## Entities

- **TaskDef**: parsed from TOML. Key fields for this feature: project (default: "default"), name, image, replicas, security (user, read_only_fs, cap_add)
- **DesiredState**: the set of TaskDefs stored in SQLite representing what should be running
- **ActualState**: the set of containers reported by the Docker API representing what is running
- **Diff**: the computed difference between desired and actual (containers to create, remove, or replace)

## Success Criteria

- SC-001: `proxa up` starts 3 replicas of `nginx:alpine` with security hardening applied, verifiable via `docker inspect` showing non-root user, empty capabilities, and no-new-privileges
- SC-002: Killing a container (`docker kill proxa-default-web-1`) results in the reconciler creating a replacement within 10 seconds
- SC-003: Changing replicas in the TOML and re-running `proxa up` adjusts the container count within one reconciliation tick
- SC-004: `proxa down` removes all containers for the service within one reconciliation tick
- SC-005: `proxa ps` accurately reflects the current state of all managed services
- SC-006: Two services with the same name in different projects coexist without conflict
- SC-007: Running `proxa up` without `proxa init` first produces a clear error message

## Assumptions

- Docker daemon is running and accessible via the default socket (`/var/run/docker.sock`)
- The operator has permission to create containers (member of `docker` group or root)
- Network configuration of containers is not in scope — default Docker bridge is sufficient
- Health checks are NOT in scope for this feature (added in 002)
- Ingress/TLS is NOT in scope for this feature (added in 003)
- Dashboard is NOT in scope for this feature (added in 004)

## Dependencies

- 000-foundation (monorepo structure, interfaces, types)
- Docker Engine 24+ running on the host
- SQLite via `modernc.org/sqlite` (pure Go, no CGO)
- Docker client via `github.com/docker/docker/client`
- TOML parsing via `github.com/BurntSushi/toml`
- CLI via `github.com/spf13/cobra` + `github.com/spf13/viper`
- Password hashing via `golang.org/x/crypto/bcrypt`

## Out of Scope

- Health checks (Feature 002)
- HTTP/TCP ingress (Feature 003)
- Admin dashboard (Feature 004)
- Secrets management (Feature 005)
- Config maps (Feature 006)
- Deployment strategies — canary/blue-green (Feature 007)
- Multi-node / clustering (v1.0)

## Testing Strategy

- **Unit tests**: TOML parser with valid/invalid inputs, diff calculation (desired vs actual), SecurityProfile application to Docker container config
- **Integration tests**: full cycle with Docker — `proxa init` → `proxa up` → verify containers exist → kill one → verify reconciler restarts it → `proxa down` → verify containers removed
- **Security tests**: verify via `docker inspect` that containers have empty capabilities, non-root user, and no-new-privileges flag

## References

- Technical Spec Section 4.3: Control Plane Flow (Single Node)
- Technical Spec Section 6: Security Model
- Technical Spec Section 11: Task Definition Specification
- Technical Spec Section 14: Core Interfaces (Runtime, StateStore)
- Technical Spec Section 15.1: v0.0 Release Phase

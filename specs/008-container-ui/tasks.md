---
description: "Task list for 008-container-ui (v0.4.4)"
---

# Tasks: Container UI (v0.4.4)

## Phase 1: Runtime extension

- [ ] T001 Add `RestartContainer(ctx, id, gracePeriod)` to the `Runtime` interface; implement in `internal/runtime/docker` (single `ContainerRestart` call); `noopRuntime` returns `ErrNotImplemented`.
  - **Commit**: `feat(runtime): add RestartContainer to Runtime interface`

## Phase 2: API + audit

- [ ] T002 `internal/server/host_containers_api.go` with five handlers (list + start + stop + restart + remove). Managed-check returns 409 via the `proxa.project` label; emits `user.container.<verb>` event on success.
  - **Commit**: `feat(server): add /api/v1/host-containers handlers with managed-guard + audit`
- [ ] T003 Register the routes in `internal/server/routes.go`.
  - **Commit**: `feat(server): mount /api/v1/host-containers under existing auth middleware`

## Phase 3: Dashboard

- [ ] T004 `internal/web/templates/containers.html` — full-page table with action buttons (Alpine x-data, two-stage Remove confirmation).
  - **Commit**: `feat(web): add /ui/containers full-page host containers viewer`
- [ ] T005 `internal/server/ui_containers.go` + route registration in `internal/server/ui.go`.
  - **Commit**: `feat(web): wire /ui/containers route under existing auth-or-unix middleware`
- [ ] T006 `internal/web/templates/index.html` — add footer link to Containers.
  - **Commit**: `feat(web): add Containers footer link to dashboard index`

## Phase 4: E2E

- [ ] T007 `tests/e2e/host_containers_test.go` — covers SC-001..SC-006 with `//go:build e2e`. Boots a host container via `docker run`, asserts API + UI + audit emission.
  - **Commit**: `test(e2e): cover host-containers API + UI + audit (SC-001..SC-006)`

## Phase 5: Docs

- [ ] T008 `docs/operations.md` — Containers section.
  - **Commit**: `docs(operations): add v0.4.4 host-containers section`
- [ ] T009 `README.md` — scope note: token holders have docker.sock-equivalent access until v0.5 multi-user.
  - **Commit**: `docs(readme): document host-container write scope for token holders`
- [ ] T010 `docs/licenses.md` — no-op refresh entry.
  - **Commit**: `docs(licenses): refresh transitive license audit for 008 (no-op)`

## Phase 6: Validation

- [ ] T011 `specs/008-container-ui/validation.md` — SC pass/fail table.
  - **Commit**: `docs(spec): record validation results for 008-container-ui`

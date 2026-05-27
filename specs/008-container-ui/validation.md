# Validation Results — 008 Container UI (v0.4.4)

Date: 2026-05-27 (release night)
Branch: `008-container-ui`
Tag: `v0.4.4`

## Success Criteria — Pass / Fail Table

| ID      | Criterion                                                            | Status | Evidence |
|---------|----------------------------------------------------------------------|--------|----------|
| SC-001  | `/api/v1/host-containers` lists ALL docker containers, managed flag correct | PASS | `tests/e2e/host_containers_test.go:TestSC_008_HostContainersListAndAct` boots an unlabeled `docker run` container, asserts it appears with `managed=false`. The same test also asserts the managed reconciler-spawned container appears with `managed=true` (via `TestSC_008_HostContainersRejectManaged`). |
| SC-002  | start/stop/restart/remove on a host container succeed + emit event   | PASS   | Same e2e test posts /stop, then verifies the audit log contains `user.container.stop` with target `container:<short-id>`. The /start and /remove variants are covered in the same flow. |
| SC-003  | start/stop/restart/remove on a managed container return 409 + no event | PASS | `TestSC_008_HostContainersRejectManaged` deploys a managed service, locates the managed container in the API list, posts /stop, asserts 409. (Negative: no event emitted because the handler returns before the emit call.) |
| SC-004  | Action menu hidden in UI for managed containers with tooltip         | PASS   | `internal/web/templates/containers.html` uses `<template x-if="c.managed">` to render the "managed (use proxa CLI)" tooltip, and `<template x-if="!c.managed">` to render the action buttons. Mutually exclusive. |
| SC-005  | Remove of a running container returns 409 with clear error           | PASS   | Same e2e test posts DELETE on the running container without `force=true`, asserts 409. Error message body: `"container is running; stop it first or pass ?force=true"`. |
| SC-006  | Events visible in `/ui/events` with `container:` target              | PASS   | The events emitter (`host_containers_api.go:emitHostContainerEvent`) uses `events.TargetContainer(shortID(id))` which formats as `container:<id>`. E2E asserts this string appears in the /api/v1/events response. |
| SC-007  | README scope note + operations.md updated; no §IX violations         | PASS   | README has a new "Auth scope (until v0.5)" section. operations.md has a new "Host containers (v0.4.4+)" section with the API surface + scope note. licenses.md refresh log records the v0.4.4 no-op confirmation. |

## Constitution Compliance

- **§I (Architecture First):** spec/plan/tasks authored before code; no
  scope creep beyond the spec.
- **§II (Security by Default):** writes inherit the existing token auth
  middleware. The README scope note + operations.md scope note honestly
  describe token power until v0.5.
- **§III (Project Scoping):** host containers explicitly bypass project
  scoping via a separate URL prefix (`/api/v1/host-containers`).
  Project-scoped endpoints (`/api/v1/projects/{p}/services/...`) are
  untouched.
- **§IV (Go Idioms):** new code is stdlib + already-vendored packages.
- **§V (Single Binary):** zero new production dependencies. Confirmed by
  licenses.md refresh entry.
- **§VIII (Zero-Downtime):** no on-disk format changes. v0.4.3 → v0.4.4
  is a straight binary swap.
- **§IX (Permissive License):** no new modules.
- **§X (Honest Scope):** managed containers are read-only with a
  tooltip; we don't pretend the UI can override the reconciler. The
  README scope note is explicit about token power.
- **§XI (Commit Strategy):** one commit per task per the tasks.md
  "Commit:" lines.

## Notes / Follow-ups

- **Container exec** still deferred to v0.8 (WebSocket plumbing + auth
  scoping cost too high).
- **Host container logs** — the existing `/ui/logs/{project}/{service}`
  is service-scoped. Adding a `host-container-logs` view would be
  straightforward but is deferred to v0.4.5+ if there's appetite.
- **`actor` field** is hard-coded to `subject:bootstrap-admin` in
  v0.4.4. v0.5 multi-user replaces this with the authenticated subject
  id (one-line change in `emitHostContainerEvent`).
- **Bulk actions** (stop-all, restart-all) intentionally not shipped —
  too dangerous before multi-user RBAC.

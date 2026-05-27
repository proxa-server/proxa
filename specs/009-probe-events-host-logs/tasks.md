---
description: "Task list for 009-probe-events-host-logs (v0.4.5)"
---

# Tasks: Probe events + host-container logs (v0.4.5)

## Phase 1: Probe events

- [ ] T001 Add `Events *events.Store` to `probe.Options`; wire transition-detection in `manager.go` to emit `probe.transition` event on every healthy⇄unhealthy flip.
  - **Commit**: `feat(probe): emit probe.transition events on health-streak transitions`
- [ ] T002 Wire `events.Store` into probe construction in `cli/server.go`.
  - **Commit**: `feat(cli): wire events.Store into probe.Manager construction`
- [ ] T003 Unit test for the transition emit (probe goes healthy → unhealthy → healthy with a recording events.Store).
  - **Commit**: `test(probe): cover probe.transition event emission on streak flips`

## Phase 2: Host-container logs API

- [ ] T004 `internal/server/host_containers_logs.go` — `GET /api/v1/host-containers/{id}/logs` handler that reuses StreamLogs + the existing SSE encoder.
  - **Commit**: `feat(server): add GET /api/v1/host-containers/{id}/logs`
- [ ] T005 Register the route in `internal/server/routes.go`.
  - **Commit**: `feat(server): mount /api/v1/host-containers/{id}/logs`

## Phase 3: Dashboard

- [ ] T006 Add `/ui/logs/host/{id}` route + handler reusing the existing logs template with a host-container context.
  - **Commit**: `feat(web): add /ui/logs/host/{id} host container logs viewer`
- [ ] T007 Add "Logs" link per row in `containers.html`.
  - **Commit**: `feat(web): add Logs link to host containers row`

## Phase 4: E2E + docs

- [ ] T008 E2E test for /api/v1/host-containers/{id}/logs (boots an unmanaged container, asserts tail returns container output).
  - **Commit**: `test(e2e): cover host-container logs API (SC-002 / SC-003)`
- [ ] T009 docs/operations.md update for the new endpoint.
  - **Commit**: `docs(operations): document host-container logs endpoint`
- [ ] T010 docs/licenses.md no-op refresh.
  - **Commit**: `docs(licenses): refresh transitive license audit for 009 (no-op)`
- [ ] T011 specs/009/validation.md.
  - **Commit**: `docs(spec): record validation results for 009-probe-events-host-logs`

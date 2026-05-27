# Feature: Container UI (v0.4.4)

**Status:** draft → ready
**Owner:** Jearel (architect) + agent (implementation)
**Predecessor:** 007-architectural-foundations (v0.4.3 — events table)
**Successor gate:** 009-multi-host (v0.5)

## Overview

Today operators see Proxa-*managed* containers in the dashboard but
cannot see, start, stop, restart, or remove the *other* containers
running on the host. v0.4.4 closes that gap: a Containers page lists
every container Docker knows about; for the ones Proxa didn't create,
operators get a slim action menu.

Containers Proxa manages remain read-only in the dashboard. Trying to
stop a managed container from the UI would just race the reconciler.

Every write hits the v0.4.3 events table with a `user.container.*`
type, so the audit log shows who did what to which container.

## Why now

Two-fold:

1. **Operator UX** — the most common dashboard request from
   self-hosters is "let me restart the prometheus container without
   ssh'ing into the box". v0.4.4 makes that a click.
2. **Validates v0.4.3 plumbing** — the events table needs a
   user-initiated emitter to prove the schema works end-to-end before
   v0.5 multi-user RBAC layers on top.

## Constitution + memory checks

- **§II Security by Default:** every write requires the existing token
  auth. Token holders today get docker.sock-equivalent access on the
  host — that's documented in v0.4.2 install docs and re-stated in this
  feature's README scope note.
- **§III Project Scoping:** host containers don't have a Proxa project.
  We surface them under a synthetic `host` project namespace in the
  UI; the API uses an explicit `host_only=true` filter.
- **§X Honest Scope:** managed containers are read-only with a
  tooltip — we don't pretend the UI can override the reconciler.
- **`feedback_container_actions_scope`:** v0.4.4 meets the three
  conditions (host-only writes, audit log per action, README scope
  note).
- **`feedback_dashboard_parity`:** the feature ships its dashboard
  surface in the same release.
- **`feedback_dashboard_cannot_self_shutdown`:** the `proxa` and
  `proxa-agent` containers (if running) are themselves managed and
  therefore read-only in the UI — operator can never stop the control
  plane from inside it.

## User Stories

### US1 — List every container on the host

As an operator I want to see every container Docker knows about, not
just the ones Proxa manages, so I can audit drift without `docker ps`.

**Acceptance:**
- A new `/ui/containers` page renders a table of all containers.
- Each row shows: name, image, state, uptime, owner (`proxa` or
  `host`).
- Proxa-managed rows show project + service + replica.
- Host rows show only the docker container name + first label set.
- The page polls every 10s.

### US2 — Start / stop / restart a host container

As an operator I want to start, stop, or restart a host container
from the dashboard.

**Acceptance:**
- Each host-row exposes a slim action menu: Start, Stop, Restart,
  Remove. (Start is enabled only when state ∈ {exited, created};
  others enabled only when state == running.)
- Each action POSTs to a `/api/v1/host-containers/{id}/{action}`
  endpoint, which calls the Runtime interface directly.
- Each action emits a `user.container.{start,stop,restart,remove}`
  event with `actor=subject:bootstrap-admin` (will be replaced with
  the authenticated subject id in v0.5 multi-user).
- Action menu is HIDDEN (not disabled) on Proxa-managed rows with a
  tooltip: "Managed by Proxa — use `proxa scale` or update the TOML".

### US3 — Remove a host container

As an operator I want to remove a stopped host container.

**Acceptance:**
- Remove requires a confirmation modal: "remove container `<name>`?
  this cannot be undone." (Alpine x-data + native dialog or simple
  inline confirmation.)
- Behind the scenes, Remove uses `RemoveContainer(id, force=false)`.
  An attempt to remove a running container surfaces the runtime error
  ("container is running") in the UI; the operator must Stop first.

### US4 — Audit trail visible in /ui/events

As an operator I want every host-container action recorded in the
audit log so I can answer "who restarted nginx last week?".

**Acceptance:**
- Every successful action lands an event row.
- The Events viewer (from v0.4.3) shows them with target
  `container:<short-id>` and payload `{"name":"<name>","prev_state":"<state>"}`.
- Filtering by target `container:` in `/ui/events` works.

## Functional Requirements

- **FR-001** `GET /api/v1/host-containers` returns a JSON list of every
  container Docker knows about, with a `managed` boolean indicating
  Proxa ownership. Auth: token (same as `/api/v1/*`).
- **FR-002** `POST /api/v1/host-containers/{id}/start` starts a
  host container. Refuses (`409 Conflict`) when `managed == true`.
- **FR-003** `POST /api/v1/host-containers/{id}/stop` stops a host
  container. Refuses (`409 Conflict`) when `managed == true`.
- **FR-004** `POST /api/v1/host-containers/{id}/restart` restarts a
  host container. Refuses (`409 Conflict`) when `managed == true`.
- **FR-005** `DELETE /api/v1/host-containers/{id}` removes a host
  container. Refuses (`409 Conflict`) when `managed == true`. Refuses
  (`409 Conflict`) when the container is still running and
  `?force=true` is not set.
- **FR-006** Every successful write emits an event row in the v0.4.3
  events table with type `user.container.<verb>`, actor
  `subject:bootstrap-admin`, target `container:<short-id>`, payload
  including the container name + prior state.
- **FR-007** `/ui/containers` renders the table per US1 + US2 + US3.
- **FR-008** Dashboard index footer adds a "Containers" link
  (alongside System).
- **FR-009** README + docs/operations.md document the scope:
  "host-container writes assume the token holder has the equivalent
  of docker.sock access; multi-user RBAC lands in v0.5".
- **FR-010** `proxa-server` and `proxa-agent` containers (if running)
  are managed-equivalent: writes refused with the standard tooltip.
  The detection is by `proxa.*` label OR by the container being
  identified as the current process's parent (best-effort).

## Success Criteria

| ID      | Criterion                                                            |
|---------|----------------------------------------------------------------------|
| SC-001  | `/api/v1/host-containers` lists ALL docker containers, managed flag correct |
| SC-002  | start/stop/restart/remove on a host container succeed + emit event   |
| SC-003  | start/stop/restart/remove on a managed container return 409 + no event |
| SC-004  | Action menu hidden in UI for managed containers with tooltip         |
| SC-005  | Remove of a running container returns 409 with clear error           |
| SC-006  | Events visible in `/ui/events` with `container:` target              |
| SC-007  | README scope note + operations.md updated; no §IX violations         |

## Out of scope

- Multi-user RBAC (v0.5).
- Container exec / TTY (v0.8 — too much WebSocket plumbing).
- Container logs viewer for host containers (existing `/ui/logs/...`
  works for Proxa-managed; host extension deferred to v0.4.5+).
- Image / volume / network management (v0.8 with RBAC).
- Bulk actions (start all, stop all) — too dangerous without RBAC.

## Assumptions

- `Runtime.ListContainers` already returns labels; we filter by
  presence of `proxa.project` to detect managed.
- The host container's first non-`proxa.*` label is used as a hint in
  the UI (e.g., `com.docker.compose.service` for compose-managed
  containers — informational only).
- v0.4.3 events.Store + events.Bus are in place. v0.4.4 only adds
  emitters, no schema changes.

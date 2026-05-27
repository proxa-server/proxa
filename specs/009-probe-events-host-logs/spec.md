# Feature: Probe events + host-container logs (v0.4.5)

**Status:** ready
**Predecessor:** 008-container-ui (v0.4.4 Container UI)
**Theme:** close documented v0.4.3 / v0.4.4 follow-ups before v0.5

## Overview

Two small, contained additions that close two documented follow-ups
from earlier 0.4.x releases:

1. **Probe events** — `probe.Manager` emits `probe.transition` events
   on streak transitions (healthy → unhealthy and back). Closes the
   v0.4.3 validation.md follow-up.
2. **Host-container logs** — `GET /api/v1/host-containers/{id}/logs`
   reuses the existing `Runtime.StreamLogs` to expose any host
   container's logs (managed or unmanaged). Closes the v0.4.4
   validation.md follow-up. The dashboard's Containers page gains a
   "Logs" link per row that opens the existing `/ui/logs/...` page
   adapted for host containers.

Both are foundation polish: probe events are needed before v0.5
multi-host (the dashboard's node-health surface depends on
cross-host transition aggregation), and host-container logs are the
last gap between "Proxa as docker.sock replacement" and the operator
not needing to ssh into the box.

## User Stories

### US1 — See health transitions in the audit log

As an operator I want to see `probe.transition` rows in the audit log
so I can answer "when did api start flapping?" without grep'ing logs.

**Acceptance:**
- Every time a tracked container's probe streak crosses zero (became
  healthy after being unhealthy, or vice versa) → one event row.
- Payload includes `{"from":"<prev>","to":"<new>","streak":<n>}`.
- Target: `container:<short-id>`. Actor: `reconciler`.
- Visible in `/ui/events` with no UI change needed (already supports
  arbitrary types).

### US2 — Tail a host container's logs

As an operator I want to tail the logs of a host container from the
dashboard without ssh'ing into the box.

**Acceptance:**
- `GET /api/v1/host-containers/{id}/logs?tail=N&follow=true` streams
  via SSE (matching the existing per-service logs API).
- Works for both managed and host containers — managed containers
  already have `/api/v1/projects/{p}/services/{n}/logs`, but the
  host-containers route is the only path that works for unmanaged
  ones.
- Containers page row gets a "Logs" link that opens the existing
  `/ui/logs/...` page (generalized for the host-container case).

## Functional Requirements

- **FR-001** `probe.Manager` accepts an optional `*events.Store` (set
  via a new `Options.Events` field; nil = silent).
- **FR-002** On every probe streak transition, `probe.Manager` emits
  one `probe.transition` event with the payload schema above.
- **FR-003** `GET /api/v1/host-containers/{id}/logs` accepts `tail`,
  `follow`, and `since` query params. Streams JSON (default) or SSE
  (when `Accept: text/event-stream`).
- **FR-004** The Containers dashboard page adds a "Logs" button per
  row that links to `/ui/logs/host/{id}` (a thin wrapper around the
  existing logs template, parameterized for host containers).

## Success Criteria

| ID      | Criterion                                                            |
|---------|----------------------------------------------------------------------|
| SC-001  | Probe streak transition emits one event row with correct payload     |
| SC-002  | `GET /api/v1/host-containers/{id}/logs` returns tail lines           |
| SC-003  | `/ui/logs/host/{id}` renders the existing logs viewer for host containers |
| SC-004  | No regression in existing per-service `/api/v1/projects/.../logs`    |

## Out of scope

- Cross-host probe aggregation (waits for v0.5).
- Logs since-timestamp filtering for `Runtime.StreamLogs` itself —
  reuses whatever the runtime backend already supports.
- Log search / filter UI — current page is tail-only.

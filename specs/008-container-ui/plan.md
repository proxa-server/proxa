# Implementation Plan — 008 Container UI (v0.4.4)

## Tech context

Pure stdlib + already-vendored deps. Zero new modules.

Runtime calls re-use the existing `internal/runtime.Runtime` interface
methods: `ListContainers`, `StartContainer`, `StopContainer`,
`RestartContainer` (NEW — but trivial: stop + start with same opts),
`RemoveContainer`, `InspectContainer`.

Audit emission re-uses v0.4.3 `internal/events.Store`.

## Constitution check

Reviewed all 11 principles; v0.4.4 introduces:

- §II: writes require token auth (existing middleware). No new auth
  surface; the README scope note (FR-009) is the operator-facing
  honesty about token power until v0.5.
- §III: host containers don't have a Proxa project. The API uses an
  explicit `host-containers` URL prefix to make the namespace
  separation obvious. No service/project tables touched.
- §VIII: zero on-disk format changes.

No deviations.

## Project structure

New:

- `internal/server/host_containers_api.go` — handler for the 5 endpoints
- `internal/server/ui_containers.go` — handler for `/ui/containers`
- `internal/web/templates/containers.html` — full-page table + actions
- `tests/e2e/host_containers_test.go` — e2e covering SC-001..SC-007

Touched:

- `internal/runtime/runtime.go` — add `RestartContainer(ctx, id, gracePeriod) error` to the interface
- `internal/runtime/docker/runtime.go` — implement RestartContainer (one-liner over the docker client)
- `internal/runtime/noop.go` (or wherever the noop lives) — return `ErrNotImplemented`
- `internal/server/routes.go` — register the 5 new routes
- `internal/server/ui.go` — register `/ui/containers`
- `internal/web/templates/index.html` — footer link to `/ui/containers`
- `docs/operations.md` — Containers section
- `README.md` — scope note for host-container writes (FR-009)

## Research entries

### R-001: Managed-detection — label vs name?

Use the `proxa.project` label (set by reconciler) as authoritative.
Container name prefix (`proxa-<project>-<service>-<replica>`) is a
fallback only — operators MAY have renamed managed containers, but
they cannot remove the label without re-creating.

### R-002: Restart as stop+start vs runtime.RestartContainer

Docker has a native ContainerRestart API; use it. Cost: one new
interface method. Benefit: atomic w.r.t. cgroup teardown. Noop
runtime returns ErrNotImplemented.

### R-003: Confirmation modal for Remove

Inline confirmation via Alpine x-data — no new JS deps. Pattern:
two-stage button (first click → "click again to confirm", second click
within 3s → actually remove). Cheaper than a modal dialog.

## Phases

- Phase 1: Runtime extension (RestartContainer)
- Phase 2: API endpoints + audit emission
- Phase 3: Dashboard page + index footer link
- Phase 4: E2E test
- Phase 5: Docs (operations.md + README scope note)
- Phase 6: Validation (SC table)

## Out-of-scope verification

- Container exec: no WebSocket plumbing added.
- Image/volume/network: no new endpoints touch these.
- Bulk actions: no multi-id endpoints.
- Container logs for host containers: the existing logs API stays
  service-scoped.

## Post-design constitution recheck

After v0.4.4, can v0.5 add multi-host without re-doing this work?
Yes — the host-containers API is node-local; multi-host will layer a
node-id parameter on top (`?node=<id>`) routed through the agent. The
audit log already records actor + target; switching `actor` from
`subject:bootstrap-admin` to the authenticated subject is a one-line
change in v0.5.

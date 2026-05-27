# Validation Results — 009 Probe events + Host-container logs (v0.4.5)

Date: 2026-05-27 (release night)
Branch: `009-probe-events-host-logs`
Tag: `v0.4.5`

## Success Criteria — Pass / Fail Table

| ID      | Criterion                                                            | Status | Evidence |
|---------|----------------------------------------------------------------------|--------|----------|
| SC-001  | Probe streak transition emits one event row with correct payload     | PASS   | `internal/probe/transition_test.go` covers healthy→unhealthy + unhealthy→healthy + nil-sink-silent paths. Transition payload schema asserted: `{"from":"healthy","to":"unhealthy","streak":4}`. |
| SC-002  | `GET /api/v1/host-containers/{id}/logs` returns tail lines           | PASS   | `tests/e2e/host_container_logs_test.go` boots an alpine container that echoes sentinels, asserts each sentinel appears in the response body. |
| SC-003  | `/ui/logs/host/{id}` renders the existing logs viewer for host containers | PASS | Same e2e test fetches the page, asserts the "Container Logs" badge + container id appear. |
| SC-004  | No regression in existing per-service `/api/v1/projects/.../logs`    | PASS   | `internal/server/handlers_logs_test.go` continues to pass with race detector. |

## Constitution Compliance

- **§IV (Go Idioms):** all-new code is stdlib + already-vendored
  packages. The `probe.EventSink` interface keeps `probe/` from
  importing `events/` (avoids a tight coupling and keeps the package
  graph clean).
- **§V (Single Binary):** zero new production dependencies.
- **§VIII (Zero-Downtime):** no on-disk format changes. v0.4.4 → v0.4.5
  is a straight binary swap.
- **§XI (Commit Strategy):** one commit per task per tasks.md.

## Notes / Follow-ups

- **Cross-host probe aggregation** still deferred to v0.5 multi-host.
  The events log gives operators per-host visibility today; v0.5 will
  add a "cluster-wide health timeline" view.
- **`follow=true`** on the host-container logs API works at the
  network layer (SSE) but the dashboard's host-logs page is
  tail-only (one-shot fetch + manual refresh). A live-tail UI is
  deferred to v0.4.5.x or v0.5.

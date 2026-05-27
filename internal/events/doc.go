// Package events is Proxa's audit + observability event store.
//
// Every reconciler action, probe transition, service status change,
// user-initiated write, and (later) GitOps webhook delivery emits one
// Event row into the SQLite `events` table (migration v2). Consumers
// query the store via SQL or via the in-process Bus pub/sub.
//
// Used by:
//   - v0.4.3 dashboard Events panel (read-only viewer)
//   - v0.4.4 Container UI (audit log for host-container start/stop writes)
//   - v0.5.0 multi-user (granular actor attribution)
//   - v0.5.1 Confidence Mode (rollback consumes the deploy-event chain)
//   - v0.6 Post-Mortem Mode (time-travel scrub-bar)
//   - v0.7+ webhook egress (events become wire-format payloads)
//
// Design contract:
//   - Append is sub-millisecond at p99 even with 100k+ existing rows
//   - Reads use the (target, ts) index for per-target tails
//   - No automatic pruning in v0.4.3 — retention sweeper arrives v0.6
//   - Bus subscribers receive events asynchronously; back-pressure drops
package events

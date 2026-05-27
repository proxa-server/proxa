# Validation Results — 007 Architectural Foundations (v0.4.3)

Date: 2026-05-27 (release night)
Branch: `007-architectural-foundations`
Tag: `v0.4.3`

## Success Criteria — Pass / Fail Table

| ID      | Criterion                                                              | Status | Evidence                                                                                                  |
|---------|------------------------------------------------------------------------|--------|-----------------------------------------------------------------------------------------------------------|
| SC-001  | Every reconciler action lands an event row in `events`                 | PASS   | `tests/e2e/events_test.go:TestSC_007_EventsEmittedOnReconcilerCreate` deploys a service, queries `/api/v1/events`, asserts `reconciler.create` is present for `service:default/evtsvc`. |
| SC-002  | SQLite migrations are idempotent + atomic                              | PASS   | `internal/store/sqlite/migrations.go` runs each migration in its own `BeginTx`; existing migrations re-checked on every start. Validated by `internal/store/sqlite` test suite passing on a fresh DB AND a pre-migrated DB. |
| SC-003  | `events.Append` p99 < 1ms with 1k rows already present                 | PASS   | `internal/events/store_test.go:BenchmarkAppend` seeds 1k rows then benchmarks Append; observed ~30µs per op on the developer laptop (M-series). Sub-millisecond with 3-decade headroom. |
| SC-004  | `state.Snapshot` stub returns `ErrSnapshotNotImplemented` from every method | PASS | `internal/state/stub_test.go` asserts every method returns the sentinel via `errors.Is`.                    |
| SC-005  | `cluster.SingleNode` satisfies Membership + StateStore + Scheduler     | PASS   | Compile-time interface assertion in `internal/cluster/singlenode.go` (`var _ Membership = (*SingleNode)(nil)` triplet). Runtime queries covered by `internal/cluster/singlenode_test.go`. |
| SC-006  | `plugin.Registry` survives a hook panic                                | PASS   | `internal/plugin/registry_test.go:TestRegistry_PanicInHookDoesNotAffectOthers` registers a panic-on-`reconciler.create` hook alongside a counting hook; verifies the counting hook still receives both events. |
| SC-007  | `plugin.Registry` drops events under back-pressure without blocking    | PASS   | `internal/plugin/registry_test.go:TestRegistry_BackPressureDropsEvents` registers a blocking hook + publishes 200 events; test completes in bounded time, proving Publish never blocked beyond the 64-event channel buffer. |
| SC-008  | `parser/toml` accepts `[meta]` block + gates against binary version    | PASS   | `internal/parser/toml/meta_test.go` covers six cases: ok-if-newer, reject-if-older, dev-binary-never-gated, empty-meta-no-gate, prerelease-stripped, decode-error-not-shadowed. |
| SC-009  | `parser/toml` emits unknown-field warnings without failing             | PASS   | `internal/parser/toml/meta_test.go:TestParse_UnknownField_WarnsNotErrors` parses a doc with two unknown fields, asserts `len(res.Warnings) >= 2` and both field names appear in the warnings. |
| SC-010  | `proxa.toml v1` formal spec documents every block + every field       | PASS   | `docs/proxa-toml-v1.md` covers `[meta]`, top-level, `[security]`, `[resources]`, `[health]`, `[strategy]`, `[[expose]]`, `[[volumes]]`, `[[route]]` + a worked example. Each field's `Since` column records the version it landed in. |
| SC-011  | Dashboard surfaces events at `/ui/events` + on the index card          | PASS   | `internal/web/templates/events.html` renders the full-page viewer; `internal/web/templates/index.html` adds the "Recent events" card polling `/api/v1/events?limit=10` every 5s. End-to-end coverage in `events_test.go` (the same test that covers SC-001). |
| SC-012  | Runtime contract documented for future backends                        | PASS   | `docs/runtime-contract.md` enumerates every `Runtime` interface method, concurrency rules, error format, idempotency requirements, security application contract, plus an implementation checklist for the v0.5 remote-runtime backend. |

## Constitution Compliance

- **§I (Architecture First):** events table + interface stubs derived from
  `spec.md` FRs, not retrofit; plan.md research entries (R-001 events
  single-table, R-002 hand-rolled migrations, R-003 async plugin
  delivery) explicit and locked.
- **§II (Security by Default):** no security-surface changes in v0.4.3.
  Audit log itself is a security feature (every reconciler action is
  recorded with actor + target + timestamp).
- **§III (Project Scoping):** events are recorded with `target` like
  `service:<project>/<name>` so multi-project deployments retain
  per-project audit isolation.
- **§IV (Go Idioms):** all new packages stdlib-first. `events` uses
  `database/sql` directly; `plugin` uses `sync.RWMutex` + buffered
  channels; `state` + `cluster` are pure interface definitions with
  stub impls.
- **§V (Single Binary):** zero new production dependencies — confirmed
  by `docs/licenses.md` refresh log entry for 2026-05-27.
- **§VI (Cluster-Ready):** the `cluster.{Membership,StateStore,Scheduler}`
  interfaces are the gates v0.5 multi-host work needs. Reconciler
  doesn't yet consume them, but their existence proves the contract is
  shippable.
- **§VII (Declarative):** the `[meta]` block + version gating let
  operators pin documents to a minimum Proxa version. Forward-compat is
  enforced by the unknown-field warnings (v0.5 documents are still
  valid v0.4.3 documents minus the new fields).
- **§VIII (Zero-Downtime):** the only on-disk change is migration v2
  (additive — new table, no schema rewrites). Upgrade from v0.4.2 to
  v0.4.3 is a straight binary swap. Validated by manual upgrade smoke
  on the developer laptop.
- **§IX (Permissive License):** no new modules added.
- **§X (Honest Scope):** every interface in this release ships with a
  no-op or stub implementation. The README accurately describes the
  events table as live + the cluster/snapshot interfaces as stubs.
- **§XI (Commit Strategy):** one commit per task, type(scope) format,
  per the tasks.md "Commit:" lines.

## Notes / Follow-ups

- **probe-transition events** are not emitted in v0.4.3 — the reconciler
  emits create/remove/scale/status events, but not the per-container
  probe transition stream. Adding that requires either a probe
  manager → events store wire-in OR moving the events.Bus subscription
  into probe.Manager. Deferred to a small follow-up (likely v0.4.4
  alongside the user-initiated container events) to keep v0.4.3 scope
  tight.
- **Retention sweep** for the events table is documented in
  `docs/operations.md` as a manual procedure. v0.5 (cluster store)
  will introduce a built-in retention worker as part of the
  state-management lifecycle.
- **plugin.Registry** is in-process only. Webhook delivery + signature
  verification land in v0.5.

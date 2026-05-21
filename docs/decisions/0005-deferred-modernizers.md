# 5. Deferred Go Modernizers and Audit Findings (v0.4.1)

## Status

Accepted (2026-05-21)

## Context

Feature 005-modern-go (v0.4.1) applied a systematic modernization pass across Go 1.22 → 1.26 features. As part of `Phase 6 US4` we ran `go fix ./...` and accepted every modernizer the tool proposed. This ADR records both (a) the audit findings for the deprecations we expected to need to migrate, and (b) the small set of modernizations we deliberately did NOT take and why.

This decision record exists so a future contributor (or future-you, six months from now) does not waste cycles re-litigating choices made here.

## Audit findings — no migration needed

These deprecations were called out in the v0.4.1 spec as potential migration targets. Audit at landing time:

- **`math/rand` v1 imports**: 0 occurrences in non-test production code. `internal/ingress/backend_pool.go` already uses `math/rand/v2` (`rand.IntN`). FR-009 satisfied with zero code changes.
- **`runtime.SetFinalizer` calls**: 0 occurrences in production code. FR-010 satisfied with zero code changes; the `runtime.AddCleanup` migration target is empty.
- **`fmt.Errorf("%s", err)` deprecation pattern**: 0 occurrences. The codebase already uses `%w` for error wrapping (Constitution §IV).

## Modernizers accepted

The `go fix ./...` run produced changes in 22 files across 8 packages, committed as four scope-grouped commits (`refactor(ingress)`, `refactor(runtime/docker)`, `refactor(server)`, `refactor: across remaining packages`). The modernizations taken were uniformly mechanical safe transforms:

- `strings.Split(s, sep)` followed by `range` → `strings.SplitSeq(s, sep)` directly in `range` (Go 1.24 iterator).
- `strings.HasPrefix(s, p)` + `strings.TrimPrefix(s, p)` pair → `strings.CutPrefix(s, p)` (Go 1.20+, modernizer surfaces in 1.26).
- `&b` for `*bool` literal → `new(b)` with expression argument (Go 1.26 built-in `new()` extension).
- `interface{}` → `any` (Go 1.18+ alias, modernizer enforces consistency).
- `for i := 0; i < N; i++` → `for i := range N` (Go 1.22 range-over-integer) — landed as a separate dedicated commit (`refactor(server): replace counter for-loop with range-over-int in ui_logs`).
- Goroutine pool boilerplate `wg.Add(1); go func() { defer wg.Done(); ... }()` → `wg.Go(func() { ... })` (Go 1.25 `sync.WaitGroup.Go`) — landed as a dedicated commit (`refactor(probe): migrate manager goroutine pool to sync.WaitGroup.Go`).
- `sort.Slice(s, less)` → `slices.SortFunc(s, cmp)` (Go 1.21 + `cmp.Compare`) — three sites in `internal/ingress/router.go`, `internal/server/handlers_logs.go`, `internal/reconciler/diff.go`, each landed as its own commit per the per-package rule.

After all modernizers landed, a second `go fix ./...` run produced no further changes — SC-010 satisfied.

## Modernizers deferred

- **`encoding/json/v2` adoption (Go 1.25 experimental)**: Deferred until upstream stabilizes the package. The current `encoding/json` works correctly for every Proxa wire-format use case; the v2 package adds (1) a streaming API we don't currently exploit, (2) different error semantics that would require auditing every JSON-decoding call site, and (3) the risk that the experimental API changes shape before stabilizing. **Re-evaluate when Go marks `encoding/json/v2` non-experimental.**

- **`weak.Pointer[T]` caches (Go 1.24)**: Deferred until v0.9 Observability. No current code path has cache pressure that warrants weak references. The natural first consumer is the cost-attribution container-info cache in v0.9; introducing the pattern earlier would be speculative.

- **`omitempty` → `omitzero` migration (Go 1.24)**: `go fix` flagged this as an "alternative fix (behavior change)" and the maintainers explicitly skipped it for now. `omitzero` is more correct semantically (omits the zero value for the type rather than the empty representation for the encoding) but the behavior change can break wire-format compatibility with clients that distinguish "field absent" from "field present with zero value". Sweep this when we own all clients of the affected types (likely v0.4.3 with the schema migration framework).

- **`sync/v2`-style WaitGroup.Go in test code**: We migrated the one production site (`internal/probe/manager.go`). Test files (`internal/ingress/backend_pool_test.go`, `internal/ingress/reload_test.go`) still use the classic `wg.Add(1) / defer wg.Done()` pattern. Test code is style preference, not deprecation hygiene; not worth churning the diff. Future test changes in those files may opportunistically migrate.

- **`testing/synctest` adoption** (Go 1.25 graduated): Deferred to **v0.4.2 Test Foundation** by design. Synctest is a testing concern that fits the dedicated test-infrastructure release; landing it here would mix scopes.

- **Green Tea GC experiment**: Deferred to **v0.4.2 Test Foundation**. The experiment requires baseline benchmarks to measure against, and v0.4.2 adds the `bench/` suite.

- **`tool` directive in `go.mod` for tools beyond staticcheck**: Deferred to **v0.4.2 Test Foundation** where the full lint/CI tooling consolidation happens. v0.4.1 covers only staticcheck as the proof-of-concept (T029-T032).

- **`crypto/hpke` and `crypto/mlkem` (Go 1.26 post-quantum primitives)**: Deferred indefinitely. No current code path requires hybrid public-key encapsulation or post-quantum KEM. Likely consumers are future backup-encryption work (v0.4.4+) or telemetry-push channels (v1.0+). Re-evaluate when one of those features actually lands.

- **chi router → stdlib `net/http.ServeMux`**: Deferred to v1.0. Documented separately in [ADR-0004](./0004-router.md).

- **Range-over-func iterators on `Runtime.ListContainers` / `ListServices`**: Deferred to **v0.4.3 Architectural Foundations**. The migration is non-trivial (every caller site needs to change shape) and pairs naturally with the state/event refactor that v0.4.3 will do.

## Consequences

- The codebase reads as modern Go 1.26 idiomatic. `make lint` reports zero deprecation warnings.
- A future contributor running `go fix ./...` against a clean v0.4.1 checkout will see no diff — SC-010 holds at v0.4.1's commit, and any drift becomes a visible PR diff.
- Six modernizers are explicitly deferred. Each has a named target release. Sweeping them at the right time keeps the v0.4.x train short and focused.
- The `omitempty` → `omitzero` decision is the highest-risk deferred item because the wire-format change could break older clients. The decision to defer is conservative — re-evaluate before any breaking schema change.

## Related

- [ADR-0004 — Keep chi router until v1.0](./0004-router.md)
- [specs/005-modern-go/research.md R-004](../../specs/005-modern-go/research.md) (modernizer triage)
- [specs/005-modern-go/plan.md](../../specs/005-modern-go/plan.md) (the v0.4.1 modernization pass)

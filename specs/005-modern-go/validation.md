# Validation Report — 005-modern-go (v0.4.1)

**Date**: 2026-05-21
**Branch**: `005-modern-go`
**Approach**: Walk every success criterion + every functional requirement and record how it was validated. Cross-reference the test that proves it.

## Audit findings (deprecation hygiene)

Before tasks executed:

| Migration target | Sites found | Action taken |
|---|---|---|
| `math/rand` v1 imports (non-test) | **0** | None needed (FR-009 satisfied at v0.4.0) |
| `runtime.SetFinalizer` (production) | **0** | None needed (FR-010 satisfied at v0.4.0) |
| Production `sync.WaitGroup` Add/Done pairs | **1** (`internal/probe/manager.go`) | Migrated to `sync.WaitGroup.Go` in T025 |
| Non-test `for i := 0; i < N; i++` | **1** (`internal/server/ui_logs.go`) | Migrated to `range N` in T024 |
| `sort.Slice` production sites | **3** (ingress / server / reconciler) | Migrated to `slices.SortFunc` in T026 |
| `go fix ./...` modernizers | **22 files / 8 packages** | Accepted all; per-package commits |
| `go fix ./...` second-pass diff | **empty** | SC-010 satisfied |

## Success Criteria — SC-by-SC

| SC | Statement | Status | Evidence |
|---|---|---|---|
| **SC-001** | TLS-enabled service + HTTP probe reaches `healthy` within 30s without manual workaround | ✅ PASS | `tests/e2e/probe_ingress_tls_test.go:TestSC_001_ProbeViaIngress_TLS` (ran in ~7s end-to-end) |
| **SC-002** | Path-traversal input refused by data-dir helpers | ✅ PASS | `internal/datadir/root_test.go:TestRoot_ParentEscape / _AbsoluteEscape / _SymlinkEscape` (8 sub-cases) |
| **SC-003** | Every token-generation path uses crypto-secure source with ≥128 bits entropy | ✅ PASS | `crypto/rand.Text` (130 bits, base32) wired in `internal/cli/init.go` for admin pwd + bootstrap token; verified by manual `proxa init` smoke test post-T013 |
| **SC-004** | Cross-origin state-changing request rejected before handler runs | ✅ PASS | `internal/server/server_csrf_test.go:TestCrossOriginProtection_MountedBeforeAuth` (4-case table: GET pass, POST same-origin pass, POST foreign 403 BEFORE auth, POST no-origin pass) |
| **SC-005** | Dashboard shows Go version + GOMAXPROCS + source in under 10s; container-aware adjustment distinguishable | ✅ PASS | `tests/e2e/system_info_test.go:TestSC_005_SystemInfo` (6 checks: HTTP API + CLI plain + CLI JSON + 401 + footer card + /ui/system page); `internal/version/runtime_test.go` covers the source detection enum (4 sub-tests) |
| **SC-006** | Fresh-clone `make lint` works without manual install | ✅ PASS | `tests/e2e/tool_directive_test.go:TestSC_006_ToolDirectiveReproducibility` (copy repo → fresh GOMODCACHE → `go tool staticcheck -version`) |
| **SC-007** | `make lint` reports zero deprecation warnings | ✅ PASS | `make lint` exits clean post-T030 (two pre-existing unused-function warnings removed in `refactor: remove dead helpers surfaced by staticcheck`) |
| **SC-008** | Binary size within ±2% of v0.4.0 | ✅ PASS | v0.4.0 binary: 27,211,842 bytes. v0.4.1 binary: 27,450,050 bytes. Delta: +238,208 bytes = **+0.88%**. Within budget. |
| **SC-009** | v0.4.0-issued tokens still authenticate post-upgrade; zero observable downtime | ✅ PASS | Manually smoke-tested at end of Phase 4: existing data dir + new binary; `curl /api/v1/system/status` with the v0.4.0 token returned 200. On-disk format unchanged (SQLite + secrets.key + token file). |
| **SC-010** | Second `go fix ./...` run is a no-op (no drift) | ✅ PASS | Verified at end of T027: `go fix ./...` after the per-package commits produced empty `git diff`. |

## Functional Requirements — FR-by-FR

| FR | Implementation | Test |
|---|---|---|
| **FR-001** datadir path-traversal sandbox | `internal/datadir/root.go` wraps `*os.Root` | `root_test.go` (12 cases, race-clean) |
| **FR-002** crypto-secure tokens ≥128 bits | `crypto/rand.Text` in `cli/init.go` | smoke-tested post-T013 |
| **FR-003** CrossOriginProtection mounted | `internal/server/server.go:New` `Router.Use(http.NewCrossOriginProtection().Handler)` | `server_csrf_test.go` 4-case table |
| **FR-004** data-dir snapshot helper | `internal/datadir/snapshot.go` (os.CopyFS + atomic rename) | `snapshot_test.go` (6 cases, atomicity verified) |
| **FR-005** probe via-ingress TLS fix | `internal/probe/http.go:NewHTTPProbeViaIngress` direct HTTPS branch + ServerName SNI | `http_test.go:TestHTTPProbeViaIngress_TLSDirect` (4 cases) + `probe_ingress_tls_test.go` e2e |
| **FR-006** explicit redirect-follow override | `HealthCheck.FollowRedirects *bool` tri-state in `pkg/types/taskdef.go` | `http_test.go:TestHTTPProbeFollowRedirects` + `TestSC_001_ProbeViaIngress_TLS_FollowRedirectsOverride` e2e |
| **FR-007** dashboard System Info surface | `internal/web/templates/system.html` + `index.html` footer card | `system_info_test.go` checks 5 & 6 |
| **FR-008** CLI parity for system info | `internal/cli/system_info.go` with `--json` flag | `system_info_test.go` checks 2 & 3 |
| **FR-009** zero legacy `math/rand` imports | confirmed by audit; `go vet`/staticcheck clean | `make lint` |
| **FR-010** zero `runtime.SetFinalizer` in prod | confirmed by audit | `make lint` |
| **FR-011** no deprecation warnings | `make lint` clean post-T030 | `make lint` exit 0 |
| **FR-012** tool directive reproducibility | `go.mod` `tool` directive for staticcheck + Makefile `go tool staticcheck` | `tool_directive_test.go` e2e |
| **FR-013** no new third-party deps (functional code) | The `tool` directive entry for staticcheck IS a new module (MIT) but it does not link into the binary — it's developer tooling. Production binary closure unchanged. | `licenses.md` refresh log entry |
| **FR-014** binary size ±2% | +0.88% measured | T035 build comparison |
| **FR-015** zero-downtime v0.4.0 → v0.4.1 upgrade | Manual smoke test | post-Phase-4 verification |

## Deviations from the plan

- **+1 unplanned commit**: `fix(probe): set TLS ServerName to route host for via-ingress HTTPS probe` (commit `7e46e81`). The e2e regression test surfaced that the TLS handshake also needed `Config.ServerName` set to the route host (not just `InsecureSkipVerify: true`). Cleaner as a follow-up commit than amending T008.
- **+2 unplanned commits from staticcheck cleanup**: `refactor: remove dead helpers surfaced by staticcheck (recordCertResult, boolPtr)` (commit `44556d4`). Staticcheck found two unused functions when `make lint` first ran on the v0.4.1 baseline; one pre-existing (`recordCertResult` in ingress/tls.go), one introduced by go-fix inlining (`boolPtr` in probe/http_test.go). Removed both rather than suppress.
- **FR-013 nuance**: The release ships ZERO new modules in the production binary closure, but adds `honnef.co/go/tools` as a `tool`-directive dependency. This is consistent with the spec's intent (developer tooling pinned without manual install) — documented in `licenses.md`.

## Out-of-scope confirmations (no scope creep)

The Assumptions section of `spec.md` enumerated 5 explicit out-of-scope items. Each remains out-of-scope and is documented in an ADR for future contributors:

- chi → stdlib router migration → [ADR-0004](../../docs/decisions/0004-router.md)
- range-over-func iterators on Runtime APIs → [ADR-0005 — deferred to v0.4.3](../../docs/decisions/0005-deferred-modernizers.md)
- `encoding/json/v2` → [ADR-0005](../../docs/decisions/0005-deferred-modernizers.md)
- Green Tea GC benchmarking → [ADR-0005 — deferred to v0.4.2](../../docs/decisions/0005-deferred-modernizers.md)
- `weak.Pointer` caches → [ADR-0005 — deferred to v0.9](../../docs/decisions/0005-deferred-modernizers.md)

## Commit history

35 tasks landed across 8 phases. Total commit count for the release: **~40 commits** (35 task-aligned + ~5 ad-hoc fixes/groupings). See `git log v0.4.0..HEAD` on the merge to main for the final ordered list.

## Sign-off

All success criteria PASS. All functional requirements implemented and tested. Ready to merge to `main` and tag `v0.4.1`.

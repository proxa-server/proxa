# Research — 005-modern-go (v0.4.1)

Phase 0 decisions that resolve the open questions in `plan.md`. Each decision has rationale + alternatives considered, so future maintainers can re-litigate from the same footing.

## R-001 — Probe-via-ingress + TLS bug fix shape

**Question**: When a health probe targets a service exposed through ingress with TLS enabled, the probe receives an HTTP 301 redirect to HTTPS and treats it as a non-2xx failure. What's the correct fix?

**Code surveyed**: `internal/probe/http.go` (HTTPProbe struct + newProbeClient + DoOnce); `internal/ingress/router.go` (HTTP→HTTPS redirect handler when TLS enabled).

**Observed behavior**: The current `newProbeClient` builds an `http.Client` with no `CheckRedirect` override. Default Go behavior is "follow up to 10 redirects" — but the redirect target is the ingress HTTPS port, which serves a self-signed or staging CertMagic cert that the probe's default `Transport` will reject. So the probe doesn't fail because of "non-2xx" — it fails because of cert verification on the redirect. The error surfaces as a probe timeout / TLS handshake error, which the operator-facing diagnostic flattens to "unhealthy".

**Decision — Approach B (target the right port directly)**: When the probe is constructed via `NewHTTPProbeViaIngress` AND the resolved ingress has TLS enabled, the probe MUST target the HTTPS port directly with an `*http.Transport` configured to skip cert verification *only for the ingress loopback target* (`InsecureSkipVerify: true` scoped to the probe client, NOT global). This preserves end-to-end semantics — the probe asserts the same thing the load balancer asserts: "does the upstream return 2xx when reached the way real traffic reaches it?"

**Why B over A**:
- **A (CheckRedirect + final-2xx)** is simpler but doesn't actually solve the cert problem — the redirect target's TLS handshake still fails. Patching cert verification *and* adding redirect-following is more surface area for the same outcome.
- **B (direct HTTPS target)** is one branch in `NewHTTPProbeViaIngress`: "if TLS, swap URL scheme to https and port to ingress.HTTPSPort, set client transport InsecureSkipVerify=true". Three lines of meaningful logic.
- B matches the existing probe semantics for non-ingress probes (which target the container bridge IP directly — no redirect dance).

**Spec compatibility — FR-006 "explicit opt-out of redirect following"**: Approach B sidesteps the redirect entirely, so the FR-006 opt-out applies to the *non-ingress probe* path (any probe configured with an explicit URL the operator chose, where they may want to assert the redirect themselves). Default for non-ingress probes preserved: "follow redirects up to 10, treat final response status as outcome" (Go default). New field `HTTPConfig.FollowRedirects *bool` (pointer for tri-state nil=default/true/false) lets the operator opt out.

**Alternatives considered**:
- (A) `CheckRedirect` + final-2xx — rejected, doesn't solve TLS handshake.
- (C) Disable HTTP→HTTPS redirect on ingress entirely when a probe is configured — rejected, breaks operator-traffic security model (real users would skip HTTPS).
- (D) Have the reconciler tell the probe "skip me, ingress will tell you" — rejected, breaks probe/reconciler decoupling and assumes ingress has the same health signal (it doesn't, ingress only sees TCP-level reachability).

**Test footprint**: `tests/e2e/probe_ingress_tls_test.go` reproduces the 0.4.0 demo bug; unit test `internal/probe/http_test.go` adds a table-driven case for `FollowRedirects` tri-state behavior.

## R-002 — `http.CrossOriginProtection` middleware mount semantics

**Question**: Go 1.25's `http.CrossOriginProtection` rejects state-changing requests from foreign origins by default. Will mounting it on `s.Router` break the dashboard's same-origin HTMX polling?

**Code surveyed**: `internal/server/server.go` (chi.Mux setup at `s.Router`); `internal/web/templates/index.html` (HTMX polling targets `/ui/services`, `/ui/routes` via same-origin); `internal/server/ui.go` (UI routes).

**Decision**: Mount `http.CrossOriginProtection` at the outer `s.Router.Use(...)` level. Same-origin requests pass through unaffected — the middleware checks the `Origin` (or `Sec-Fetch-Site`) header and only rejects when the origin is explicitly cross-origin. HTMX polls have `Origin: <same-host>` by default → pass. Bearer-token API calls from `curl`/`proxa` CLI typically have no `Origin` header → pass (the middleware's policy is "reject when Origin is set AND mismatches", not "require Origin header").

**Critical subtlety**: state-changing methods (`POST`/`PUT`/`PATCH`/`DELETE`) are the ones the middleware guards. v0.4.1 ships *no* such write endpoints (the API is read-only + log-stream). So the middleware is effectively a no-op for current traffic — but ANY future write endpoint (v0.5 `proxa rollback`, v0.8 dashboard exec, etc.) inherits the protection automatically without per-handler opt-in. This is the point: defense-in-depth, mounted now, paying dividends later.

**Mount placement**: BEFORE the bearer-token auth middleware. The CSRF check is cheaper than a DB lookup (header-only) — fail fast.

**Test footprint**: `internal/server/server_csrf_test.go` — table-driven: (a) GET passes, (b) POST same-origin passes, (c) POST cross-origin foreign rejected with 403, (d) POST no-origin (CLI) passes. The "POST" tests use a test-only handler mounted on a child router for the test only; we don't add a real POST endpoint just to satisfy the test.

**Alternatives considered**:
- Mount after auth — rejected, wastes a token verification on a request that's already cross-origin.
- Skip mounting until v0.5 (when first POST endpoint lands) — rejected, the whole point is to land the gate BEFORE the write endpoint so we never ship one without protection.
- Per-handler opt-in — rejected, exactly the failure mode we're avoiding (a contributor adds a write endpoint and forgets the middleware).

## R-003 — `runtime.SetDefaultGOMAXPROCS` introspection

**Question**: How does Proxa detect whether the container-aware GOMAXPROCS adjustment took effect?

**Code surveyed**: Go 1.25 `runtime` package docs; existing `internal/version/version.go`.

**Decision**: Add `internal/version/runtime.go` with a single exported function `SystemInfo() Info`. The `Info` struct includes `GOMAXPROCSSource` enum with three values:

- `"env_override"` — `GOMAXPROCS` env var is set explicitly (read via `os.LookupEnv("GOMAXPROCS")`).
- `"container_limit"` — env var unset AND `runtime.GOMAXPROCS(0) < runtime.NumCPU()` (i.e., Go runtime detected a CPU cgroup limit lower than the host).
- `"host"` — env var unset AND `runtime.GOMAXPROCS(0) == runtime.NumCPU()` (default, no cgroup limit).

**Edge case**: A host with N CPUs and a container limit of exactly N would report `host` instead of `container_limit`. Acceptable — the observable effect is identical (using all host CPUs), and the operator's question ("did container-awareness save me from a runaway?") is honestly answered: it didn't need to.

**Alternatives considered**:
- Read `runtime/debug.GCStats` to check for runtime adjustment — rejected, no API exposes "was I adjusted from cgroup". The heuristic above is the documented detection pattern.
- Probe `/sys/fs/cgroup/...` directly — rejected, platform-specific, fragile, and gives the wrong answer on hosts where cgroups exist but Proxa isn't subject to them.

## R-004 — Modernizer triage (US4)

**Question**: After running `go fix ./...`, which auto-applied changes do we accept, which do we defer (and why)?

**Method**: On a scratch worktree, run `go fix ./...` and review the diff per-package. Categorize each change as (a) accept-and-commit-with-rationale, (b) defer-with-rationale-in-decision-record, (c) reject-as-spurious.

**Pre-decisions** (we know upfront before running):

- **Accept**: any `for i := 0; i < N; i++` → `for i := range N` in non-test code where `i` is used only as a counter.
- **Accept**: any `sort.Slice(s, func(i, j int) ...)` → `slices.SortFunc(s, cmp)` if the comparison is straightforward.
- **Accept**: any `fmt.Errorf("%s", err)` → `fmt.Errorf("%w", err)` modernizer (the deprecated formatting modernizer).
- **Defer + document**: any modernizer that touches a hot path the benchmark in v0.4.2 will measure (e.g., the reconciler loop) — defer to v0.4.2 after a baseline is captured.
- **Reject**: any modernizer that imports an experimental package (e.g., `encoding/json/v2`).

**Final list lives in** `docs/decisions/0005-deferred-modernizers.md` — written *after* the scratch run, in the actual implementation pass. Each deferred item has a one-line "why deferred" and a target release.

**Test footprint**: A small unit test `internal/version/modernizers_test.go` asserts that the "deferred" list documented in `0005-deferred-modernizers.md` matches a constant in the source (so the doc and code can't drift out of sync). Optional — decide during implementation.

## R-005 — Decision record format

**Question**: We're creating `docs/decisions/` for the first time. What format do ADRs use?

**Decision**: Lightweight Markdown ADRs in the Michael Nygard style — `# N. Title`, `## Status`, `## Context`, `## Decision`, `## Consequences`. Numbered sequentially: `0004` for router (next free number after 0001-foundation, 0002-health-checks, 0003-ingress — even though those aren't ADRs in this directory, the numbering reserves space for retrofit if we ever document them); `0005` for deferred modernizers.

**Alternatives considered**: MADR template — rejected as heavier than needed for this size of project; the project doesn't have decision-review stakeholders beyond the maintainer.

## R-006 — `tool` directive scope (US5)

**Question**: Which tools get the `tool` directive treatment in this release vs deferred to v0.4.2 Test Foundation?

**Decision**: Only `honnef.co/go/tools/cmd/staticcheck` (currently pinned via `go run honnef.co/go/tools/cmd/staticcheck@v0.7.0` in the Makefile). Defer `golangci-lint`, `gofumpt`, etc. to v0.4.2 when the broader test/CI tooling consolidation happens. Rationale: staticcheck is the only one currently in the dev loop (`make lint`); migrating it is the proof-of-concept for the pattern, and v0.4.2 sweeps the rest.

**Makefile change**: `STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.7.0` becomes a no-op variable (kept for backward-compat in case something external reads it), and the `lint` target invokes `go tool staticcheck ./...` instead of `go run $(STATICCHECK) ./...`. Document the migration in `docs/operations.md`.

**Alternatives considered**: Sweep all tools at once — rejected, broader sweep risks scope creep into v0.4.2's territory.

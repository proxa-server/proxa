---
description: "Tasks for 003-ingress — L7/L4 routing with auto-TLS, hot-reload, dashboard surfacing"
---

# Tasks: Ingress — L7/L4 Routing with Auto-TLS

**Input**: Design documents from `/specs/003-ingress/`

**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/{ingress,route}.md`, `quickstart.md`. v0.2.0 merged to main.

**Tests**: Required at three layers per spec Testing Strategy:

- **Unit** (default): router lookup table-driven, backend pool LB strategies, parser route validation cases (including project-scoped `route-conflict`), L4 forwarder loopback echo, atomic-pointer reload safety with `-race`.
- **dockerd-tagged** (`//go:build dockerd`): ACME issuance against Pebble (Let's Encrypt's local test CA spun up as a sibling container).
- **e2e-tagged** (`//go:build e2e`): SC-001 (self-signed mode), SC-002 (LB across 3 replicas), SC-003 (hot-reload no 5xx), SC-006 (TCP forward to redis), SC-007 (re-enable the 4 macOS-skipped 002 tests via `[health].via = "ingress"`).

**Organization**: Six user stories from spec. US1 + US2 share machinery (router, proxy, LB) and form the MVP. US3 adds L4. US4 closes the macOS dev-experience gap from 002. US5 confirms non-privileged ports (already supported via config; one test). US6 verifies hot-reload safety end-to-end. Foundational phase carries the heavy lift (parser, types, ingress package primitives, reload glue).

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User-story tag (US1–US6). Setup, Foundational, Polish tasks omit it.
- Every task includes its file path(s).

## Commit policy (constitution §XI)

One commit per task. Message format: `<type>(<scope>): <description>`.

| Scope token | Applies to |
|---|---|
| `ingress` | `internal/ingress/*` |
| `web` | `internal/web/static/*`, `internal/web/templates/*` |
| `parser/toml` | `internal/parser/toml/*` |
| `types` | `pkg/types/*` |
| `server` | `internal/server/*` |
| `cli` | `internal/cli/*` |
| `config` | `internal/config/*` |
| `reconciler` | `internal/reconciler/*` |
| `probe` | `internal/probe/*` |
| `e2e` | `tests/e2e/*` |
| `licenses` | `docs/licenses.md` |
| `docs` | `docs/*`, `specs/*` |
| `build` | `Makefile`, `go.mod`, `go.sum` |

## Constraint reminders

- `CGO_ENABLED=0` for production binaries; `=1` only at the test step for `-race`.
- Every ingress operation takes `context.Context` first; honors shutdown.
- Route table reload MUST be lock-free for readers (atomic.Pointer swap — R-003).
- Backend pool reflects probe.Snapshot.HealthOK; unhealthy replicas excluded (FR-013).
- Cross-project hostname uniqueness enforced at parse time (§III + FR-017).
- Hot-reload during sustained traffic MUST NOT produce any 5xx (FR-009 + SC-003).
- Dashboard land WITH US1, not in Polish (incremental-landing preference).

---

## Phase 1: Setup

- [X] T001 Add `github.com/caddyserver/certmagic` (Apache-2.0) to `go.mod` via `go get github.com/caddyserver/certmagic@latest`, run `go mod tidy`, commit the resulting `go.mod` + `go.sum`. Verify total module count grows by ≤ 20 (research R-001 budget). Commit: `build(deps): add caddyserver/certmagic for ACME-driven TLS`.

- [X] T002 [P] Create the `internal/ingress/` directory with `.gitkeep` placeholder (will be replaced by real files in Phase 2). Commit: `chore(repo): scaffold internal/ingress package directory`.

---

## Phase 2: Foundational — types + parser + ingress primitives (Blocking)

**Purpose**: Land the route + Health.Via types, parser validation, and the ingress package primitives every user story depends on. NO behavior visible to users yet.

### Types extension

- [X] T003 Extend `pkg/types/taskdef.go`: add `Route` struct (`Host`, `Path`, `L4`, `Port`, `LBStrategy`) and a `Routes []Route` field on `TaskDef`; add `Via string` field to `HealthCheck` with `toml:"via" json:"via,omitempty"`. Commit: `feat(types): add Route struct and Health.Via field for ingress`.

### Parser extensions

- [X] T004 Extend `internal/parser/toml/validate.go` with `[[route]]` block validation per `contracts/route.md` rules: reject empty `host` for L7 (`route-needs-host`), bad hostname syntax (`route-bad-host`), bad path glob (`route-bad-path`), invalid `l4` value (`route-invalid-protocol`), missing port for L4 (`route-needs-port`), bad `lb_strategy` (`route-bad-lb`), and `[health].via` ∉ {"", "direct", "ingress"} (`health-bad-via`). Commit: `feat(parser/toml): validate [[route]] block and [health].via field`.

- [X] T005 Add cross-service `route-conflict` detection to `internal/parser/toml/validate.go`: when `proxa up` parses a TOML, the validator MUST consult the StateStore for already-registered routes via a new `routesByHost(store)` helper and reject (host, path) collisions in the same project AND `host` collisions across projects. Commit: `feat(parser/toml): reject conflicting routes in project-scoped + cross-project mode`.

- [X] T006 [P] Add fixture files under `internal/parser/toml/testdata/` — `invalid-route-needs-host.toml`, `invalid-route-bad-host.toml`, `invalid-route-bad-path.toml`, `invalid-route-bad-l4.toml`, `invalid-route-needs-port.toml`, `invalid-route-bad-lb.toml`, `invalid-health-bad-via.toml`, `valid-route-tls.toml`, `valid-route-l4-tcp.toml`, `valid-route-multi.toml`. Extend `parser_test.go` table with 7 entries asserting the new error codes + 3 entries asserting the valid fixtures round-trip. Commit: `test(parser/toml): cover [[route]] validation and Health.Via`.

### Ingress package primitives

- [X] T007 Delete `internal/ingress/.gitkeep` and create `internal/ingress/ingress.go` declaring the `Ingress` interface (`Name`, `Run`, `UpdateRoutes`, `UpdateBackends`, `CertInfo`, `IngressInfo`), the `ServiceID`, `Backend`, `CertInfo`, `CertStatus` (const enum), and `IngressInfo` types. Doc comment links to `specs/003-ingress/contracts/ingress.md`. Commit: `feat(ingress): declare Ingress interface and shared types`.

- [X] T008 Create `internal/ingress/backend_pool.go` with `BackendPool` struct (RWMutex + `[]Backend` + atomic round-robin cursor) and methods `Replace(backends []Backend)`, `Pick(strategy string) *Backend`, `Healthy() []Backend`. The `Pick` method returns nil when no healthy backend exists; LB strategies `random` (default) and `round-robin`. Commit: `feat(ingress): add BackendPool with random and round-robin LB`.

- [X] T009 [P] Add `internal/ingress/backend_pool_test.go` table-driven across LB strategies × pool composition (empty / 1 / 3 healthy / mixed healthy+unhealthy). Round-robin assertion: 9 picks across 3 backends = exactly 3 each. Concurrency assertion: `go test -race` with 100 parallel `Pick` calls. Commit: `test(ingress): cover BackendPool LB strategies and concurrency`.

- [X] T010 Create `internal/ingress/router.go` with `Router` struct (immutable snapshot), `LookupL7(host, path string) (ServiceID, string, bool)` (longest-prefix-first), `LookupL4(proto string, port int) (ServiceID, string, bool)` (map lookup), and a `BuildRouter(routes map[ServiceID][]types.Route) (*Router, error)` constructor that returns `route-conflict` errors for any duplicate. Commit: `feat(ingress): add Router with longest-prefix L7 lookup and O(1) L4 lookup`.

- [X] T011 [P] Add `internal/ingress/router_test.go` table-driven for LookupL7 (path glob matching with and without trailing `*`, multi-route services, missing matches → false) and LookupL4 (proto+port keying, missing → false). Commit: `test(ingress): cover Router L7/L4 lookup edge cases`.

- [X] T012 Create `internal/ingress/reload.go` with `RouterPtr` wrapping `atomic.Pointer[*Router]` plus a `Swap(*Router)` method that returns the previous pointer. Doc comment justifies R-003 atomic-swap pattern. Commit: `feat(ingress): add atomic Router swap for lock-free hot-reload`.

- [X] T013 [P] Add `internal/ingress/reload_test.go` using `testing/synctest`: launch 100 reader goroutines doing `Load()` in a loop and 1 writer doing `Swap` 50 times; assert no reader observes a partially-built router (every `Load` returns a non-nil, fully-built `*Router`). Run with `-race`. Commit: `test(ingress): cover atomic Router swap under concurrent readers`.

**Checkpoint**: parser accepts and validates `[[route]]` and `[health].via`; ingress package primitives compile and pass unit + race tests. NO listener bound yet — that's Phase 3.

---

## Phase 3: User Story 1 — HTTPS on a domain + dashboard surfacing (Priority: P1) 🎯 MVP

**Goal**: A service with one `[[route]]` block and `[ingress].tls = true` becomes reachable at `https://<host>/` with a valid auto-issued certificate, AND the dashboard's new Routes card shows it.

**Independent Test**: Deploy `whoami` with `[[route]] host = "whoami.local"`, run `proxa up`, `curl --resolve whoami.local:8443:127.0.0.1 https://whoami.local:8443/` returns 200 with the self-signed cert; `curl http://127.0.0.1:8443/ui/routes` returns HTML containing `whoami.local`.

### Implementation for User Story 1

- [X] T014 [US1] Create `internal/ingress/proxy.go` with `Proxy` struct wrapping `net/http/httputil.ReverseProxy`. The `Director` function: (1) look up the service via the currently-loaded `*Router`, (2) `Pick` a backend from the service's `BackendPool` using the route's LB strategy, (3) rewrite the request URL host to `<backendIP>:<containerPort>`. On no-healthy-backend, the proxy's `ErrorHandler` writes `503 Service Unavailable` + `Retry-After: 5` per FR-012. Commit: `feat(ingress): add ReverseProxy with backend selection and 503 fallback`.

- [X] T015 [US1] Create `internal/ingress/tls.go` with `NewTLSConfig(cfg IngressConfig) (*tls.Config, *certmagic.Config, error)` that constructs CertMagic with `FileStorage` rooted at `${PROXA_DATA_DIR}/certs/`, ACME endpoint from `cfg.ACMEDirectoryURL` (defaults to Let's Encrypt prod), and HTTP-01 challenge enabled. Self-signed mode (`cfg.TLS == false || cfg.Email == ""`) skips ACME and uses CertMagic's on-the-fly self-signed cert generator (test-mode). Commit: `feat(ingress): wire CertMagic for ACME HTTP-01 with self-signed fallback`.

- [X] T016 [US1] Create `internal/ingress/certmagic_ingress.go` implementing the `Ingress` interface: `Run(ctx)` binds the HTTP listener (serves ACME challenges + 301 to HTTPS), the HTTPS listener (TLS via CertMagic + reverse proxy), and the L4 listeners (placeholder; full impl in US3). `UpdateRoutes` rebuilds the `*Router` via `BuildRouter` and `Swap`s it. `UpdateBackends` replaces the named service's `BackendPool`. `CertInfo` queries CertMagic's storage for cert metadata. `IngressInfo` returns the configured ports + TLS state + cert count. Commit: `feat(ingress): add certMagicIngress implementation of the Ingress interface`.

- [X] T017 [US1] Extend `internal/config/config.go` with `IngressConfig` struct (`HTTPPort int`, `HTTPSPort int`, `TLS bool`, `Email string`, `ACMEDirectoryURL string`) read from `[ingress]` section + `PROXA_INGRESS_*` env vars. Defaults: HTTPPort=8080, HTTPSPort=8443, TLS=false, Email="", ACMEDirectoryURL=Let's Encrypt prod. Commit: `feat(config): add IngressConfig with non-privileged defaults`.

- [X] T018 [US1] Modify `internal/cli/server.go` `runServer`: construct `ingress.NewCertMagicIngress(cfg.Ingress, logger)`, pass it to `reconciler.New` via a new `Options.Ingress` field, and run `ingress.Run(ctx)` in a sibling goroutine to `reconciler.Run` and `probes.Run`. Commit: `feat(cli): wire ingress component into proxa server`.

- [X] T019 [US1] Modify `internal/reconciler/reconciler.go` `updateProbesAndStatus`: after computing per-service backends from probe snapshots, call `r.ingress.UpdateBackends(ctx, ServiceID{...}, backends)` for each service. At end of each `reconcileOnce`, build the project's route map and call `r.ingress.UpdateRoutes(ctx, routes)`. Snapshot-and-swap, not incremental. Commit: `feat(reconciler): push routes and backends to ingress each tick`.

### Dashboard (lands WITH US1 per user instruction)

- [X] T020 [US1] [P] Add `chip-blue` (TLS valid) and `chip-purple` (TLS renewing) classes to `internal/web/static/app.css` matching the existing `chip-*` color palette (border + background + foreground); reuse `chip-amber` for TLS pending, `chip-red` for TLS failed, `chip-neutral` for TLS off. Source CSS at `web/_src/app.css` updated in lockstep per CLAUDE.md note. Commit: `feat(web): add chip-blue and chip-purple for TLS certificate state`.

- [X] T021 [US1] [P] Create `internal/web/templates/routes_table.html` mirroring `services_table.html`: HTMX poll `hx-get="/ui/routes" hx-trigger="every 5s" hx-target="this" hx-swap="outerHTML"`. Columns: Project, Host, Path, Service, TLS chip (template chooses chip class from `.TLSStatus`), Backends count. Empty state: "No routes yet. Add a [[route]] block to a service TOML." Commit: `feat(web): add Routes HTMX card template`.

- [X] T022 [US1] Extend `internal/server/ui.go` `buildUIData`: add `TotalRoutes int`, `IngressInfo IngressInfoRow`, and `RouteRow{Project, Host, Path, Service, TLSStatus, BackendCount}`. Per-route TLSStatus comes from `s.ingress.CertInfo(host)`. Cluster-status header row gains `IngressInfo` (ports + TLS state + cert count from `s.ingress.IngressInfo()`). Commit: `feat(server): surface routes and ingress state in buildUIData`.

- [X] T023 [US1] [P] Create `internal/server/ui_routes.go` with `handleUIRoutes` that renders just the `routes_table.html` fragment with `buildUIData(ctx).Routes`. Register at `/ui/routes` next to the existing `/ui/services`. Commit: `feat(server): add /ui/routes HTMX poll endpoint`.

- [X] T024 [US1] Modify `internal/web/templates/index.html` to include the Routes card (between Services and the cluster-status footer) and to render the Ingress info widget in the cluster-status header row. Commit: `feat(web): mount Routes card and Ingress widget in dashboard layout`.

- [X] T025 [US1] Add `GET /api/v1/routes` and `GET /api/v1/ingress` handlers in `internal/server/handlers.go` returning JSON shaped like the `RouteRow` and `IngressInfoRow` types from T022. Bearer-token + Unix-socket bypass, same as other API endpoints. Commit: `feat(server): expose /api/v1/routes and /api/v1/ingress`.

- [X] T026 [US1] Add `tests/e2e/ingress_https_test.go` (build tag `e2e`) covering SC-001 in self-signed mode (`tls = false` to skip Let's Encrypt rate limits): deploy `whoami` with `[[route]] host = "whoami.local"`, wait for convergence, `curl --resolve whoami.local:<httpsport>:127.0.0.1 -k https://whoami.local:<httpsport>/` returns 200; ALSO `curl http://127.0.0.1:<httpport>/api/v1/routes` with the bearer token returns JSON containing the new route AND `curl http://127.0.0.1:<httpport>/ui/routes` returns HTML containing "whoami.local". Cleanup tears down the container. Commit: `test(e2e): cover HTTPS route reachability and dashboard surfacing (SC-001)`.

**Checkpoint**: SC-001 passes. Operators can declare one route and reach it over HTTPS; dashboard reflects the route and ingress state.

---

## Phase 4: User Story 2 — Replica load balancing (Priority: P1, MVP)

**Goal**: Scaling a service to 3 replicas distributes inbound requests across the replicas; the ingress backend pool tracks reconciler additions/removals.

**Independent Test**: Deploy 3 whoami replicas under one route. Hit the route 30 times. At least 2 distinct backends respond.

### Implementation for User Story 2

- [X] T027 [US2] Add `tests/e2e/ingress_lb_test.go` (build tag `e2e`) covering SC-002: deploy whoami with `replicas = 3` and one `[[route]]`, wait for `proxa ps` to show 3/3 healthy, then issue 30 sequential `curl` requests through the route and parse the `Hostname:` line from each response. Assert ≥ 2 distinct hostnames seen. Optional follow-up sub-test asserts `lb_strategy = "round-robin"` produces an exact 10/10/10 distribution over 30 requests. Commit: `test(e2e): cover replica LB across 3 backends (SC-002)`.

**Checkpoint**: SC-002 passes. No new code outside the e2e test — US1's `Proxy` + `BackendPool` already handle LB; this phase confirms it end-to-end.

---

## Phase 5: User Story 3 — TCP/UDP L4 forward (Priority: P2)

**Goal**: A non-HTTP workload (postgres / redis) is reachable on a declared `l4 = "tcp"` route.

**Independent Test**: Deploy redis with `[[route]] host = "cache.local" l4 = "tcp" port = 6379`. `redis-cli -h 127.0.0.1 -p 6379 ping` returns `PONG`.

### Implementation for User Story 3

- [X] T028 [US3] Create `internal/ingress/l4.go` with `tcpForwarder` and `udpForwarder` types. TCP: `net.Listen` per declared L4 route, accept loop spawns a goroutine per connection that `Pick`s a backend at accept-time, dials it, and `io.Copy` in both directions until either side closes. UDP: `net.ListenPacket`, source-IP+port → backend cached for 30s per R-004, forward packets. Commit: `feat(ingress): implement L4 TCP and UDP forwarders`.

- [X] T029 [US3] [P] Add `internal/ingress/l4_test.go` with a loopback echo server: start an in-process TCP echo server on a random port, register it as a backend, start the L4 forwarder on another random port, dial the forwarder, write+read 1 KiB of random bytes, assert echo. Same shape for UDP with a small datagram. Commit: `test(ingress): cover L4 TCP/UDP forwarders with loopback echo`.

- [X] T030 [US3] Wire L4 forwarder lifecycle into `certmagic_ingress.go` `Run`: launch one forwarder goroutine per declared L4 route during `UpdateRoutes`; on subsequent `UpdateRoutes`, diff old vs new L4 set and start/stop forwarders accordingly. Commit: `feat(ingress): manage L4 forwarder lifecycle on UpdateRoutes`.

- [X] T031 [US3] Add `tests/e2e/ingress_tcp_test.go` (build tag `e2e`) covering SC-006: deploy `redis:7-alpine` with `[[route]] host = "cache.local" l4 = "tcp" port = 6379`, `redis-cli -h 127.0.0.1 -p 6379 ping` returns `PONG`. Cleanup. Commit: `test(e2e): cover L4 TCP forward to redis (SC-006)`.

**Checkpoint**: SC-006 passes. Non-HTTP workloads exposable.

---

## Phase 6: User Story 4 — Probes via ingress (Priority: P2)

**Goal**: Services on macOS Docker Desktop can use HTTP probes by routing them through ingress (`[health].via = "ingress"`); the 4 macOS-skipped 002 tests stop skipping.

**Independent Test**: Deploy a service with `[health].path = "/health"` AND `[health].via = "ingress"` AND a `[[route]]`. `proxa ps` reports `healthy` within `interval × retries + tick` on macOS.

### Implementation for User Story 4

- [X] T032 [US4] Modify `internal/probe/manager.go` `probeLoop`: when `spec.Health.Via == "ingress"`, construct `NewHTTPProbe` with URL = `http://127.0.0.1:<httpport><path>` and inject `Host` header = the route's host. New helper `lookupRouteHost(spec)` walks `spec.Routes` and returns the first L7 route's host (errors if none). Pass IngressConfig.HTTPPort to the Manager via `probe.Options{IngressHTTPPort int}` so the loopback URL is constructed correctly. Commit: `feat(probe): route HTTP probes through ingress loopback when Via=ingress`.

- [X] T033 [US4] Extend `internal/cli/server.go` `runServer` to pass `IngressHTTPPort` into `probe.Options` so the Manager knows where to dial. Commit: `feat(cli): pass ingress HTTPPort into probe Manager options`.

- [X] T034 [US4] Add `tests/e2e/ingress_probe_test.go` (build tag `e2e`) covering SC-007: deploy whoami with `[[route]] host = "whoami.local"` AND `[health].path = "/health" via = "ingress"`. The test does NOT call `skipIfHTTPProbeUnreachable` — it must pass on macOS Docker Desktop too. Assert `proxa ps -j` reports `healthy` within 30s. Commit: `test(e2e): cover HTTP probe via ingress loopback (SC-007)`.

**Checkpoint**: SC-007 passes. macOS dev story unblocked.

---

## Phase 7: User Story 5 — Non-privileged ports (Priority: P3)

**Goal**: Operator without root can start proxa with `[ingress].http_port = 18080` and `[ingress].https_port = 18443` and serve traffic.

### Implementation for User Story 5

- [X] T035 [US5] Add `internal/ingress/certmagic_ingress_test.go` covering `Run` with non-privileged ports — start the ingress on `:18080` + `:18443` in a unit test (no Docker, no real backend), connect with a TCP `net.Dial`, assert the listener accepts. Demonstrates SC-008 without needing an unprivileged test user. Commit: `test(ingress): verify non-privileged port binding`.

- [X] T036 [US5] Update `docs/operations.md` (create if absent) with the privileged-port story: CAP_NET_BIND_SERVICE on Linux, systemd AmbientCapabilities snippet, or the high-port + external LB pattern. Cross-link from `README.md`. Commit: `docs: document privileged-port options for ingress (CAP_NET_BIND, systemd, high ports)`.

**Checkpoint**: SC-008 evidence captured.

---

## Phase 8: User Story 6 — Hot-reload no 5xx (Priority: P2)

**Goal**: Editing a route mid-traffic does not cause any 5xx; in-flight HTTP/1.1 keep-alive sessions complete their next request successfully.

**Independent Test**: Sustained `curl` loop against a route; edit the TOML's `lb_strategy`; `proxa up`. No `curl: (` or non-2xx in the loop output.

### Implementation for User Story 6

- [X] T037 [US6] Add `tests/e2e/ingress_reload_test.go` (build tag `e2e`) covering SC-003: deploy whoami with `[[route]]`, spawn a background goroutine that hits the route every 50ms recording failures; in the foreground re-issue `proxa up` with a modified TOML (toggle `lb_strategy`) at t=2s; let the loop run for 15s total; assert zero failures (no `curl` errors, no 5xx responses). Commit: `test(e2e): cover hot-reload safety under sustained traffic (SC-003)`.

**Checkpoint**: SC-003 passes. Live edits during business hours are safe.

---

## Phase 9: Polish & Cross-Cutting

- [X] T038 [P] Add `internal/ingress/ingress_integration_test.go` (`//go:build dockerd`) that brings up a Pebble container (ghcr.io/letsencrypt/pebble), points `ACMEDirectoryURL` at `https://localhost:14000/dir`, issues a cert for `proxa-test.local`, asserts the cert is present and not expired. Cleanup tears down Pebble. Commit: `test(ingress): cover ACME issuance against Pebble test CA`.

- [X] T039 Re-run the license audit script from 002's polish phase against the post-003 `go.sum`. Update `docs/licenses.md` with: new direct dep `caddyserver/certmagic` (Apache-2.0), new transitive deps from CertMagic's tree (`mholt/acmez/v3`, `libdns/libdns`, `miekg/dns`, `zeebo/blake3`, etc. — confirm each is Apache-2.0/MIT/BSD/MPL-2.0). Commit: `docs(licenses): refresh transitive license audit for 003-ingress`.

- [X] T040 Walk `quickstart.md` end-to-end on a clean `${PROXA_DATA_DIR}` (use self-signed mode to avoid Let's Encrypt rate limits). Record outcomes in `specs/003-ingress/validation.md` mirroring the 001 / 002 format: SC-by-SC table with PASS/READY/FAIL + evidence. Document any bugs caught + fixed during validation. Commit: `docs(spec): record quickstart validation results for 003-ingress`.

**Checkpoint (end of feature)**: All 8 SCs PASS or READY. `git log --oneline 003-ingress ^main` shows one commit per task with constitution-§XI-compliant messages.

---

## Dependencies & Execution Order

### Phase ordering

- **Phase 1 (Setup)**: T001 → T002 in parallel after T001 finishes (T002 just makes the directory).
- **Phase 2 (Foundational)**: T003 (types) blocks T004 (parser) → T005 (cross-service); T006 [P] tests after T004+T005. T007 (interface) blocks T008–T013 which run in chains:
  - T008 (BackendPool) → T009 [P] (test)
  - T010 (Router) → T011 [P] (test)
  - T012 (atomic reload) → T013 [P] (test)
- **Phase 3 (US1)**: T014 (proxy) + T015 (tls) + T017 (config) parallel after Phase 2. T016 (certmagic_ingress) depends on T014+T015+T017. T018 (cli wiring) depends on T016. T019 (reconciler push) depends on T018. Dashboard: T020 [P] + T021 [P] + T023 [P] can run in parallel; T022 (buildUIData) depends on T016 (it queries CertInfo); T024 (index.html) depends on T021; T025 (handlers) depends on T022. T026 (SC-001 e2e) depends on EVERYTHING above in US1.
- **Phase 4 (US2)**: T027 standalone after US1.
- **Phase 5 (US3)**: T028 (l4) → T029 [P] test; T030 wires into certmagic_ingress (depends on T016 + T028); T031 (e2e) depends on T030.
- **Phase 6 (US4)**: T032 (probe via ingress) depends on T016 (needs ingress to be running); T033 (cli) depends on T032; T034 (e2e) depends on T033.
- **Phase 7 (US5)**: T035 standalone after T016. T036 docs standalone.
- **Phase 8 (US6)**: T037 standalone after US1 (the reload mechanism is already in T012; US6 just verifies).
- **Phase 9 (Polish)**: T038 standalone after T016. T039 after every code commit. T040 last.

### User-story dependencies

- US1 + US2 are both P1 (MVP). US2 depends on US1's proxy + LB code being in place.
- US3 depends on Phase 2 + US1's certmagic_ingress shell.
- US4 depends on US1 (needs ingress listening to route probes through).
- US5 is essentially free given Phase 3's IngressConfig — just a test + docs.
- US6 depends on Phase 2's atomic reload + US1's wiring.

### Parallel execution batches

**Batch A (Phase 1)**: T001 sequential. T002 parallel after T001.

**Batch B (Phase 2 — parser branch)**: T003 → T004 → T005 sequential. T006 [P] after T005.

**Batch C (Phase 2 — ingress primitives)**: After T007, three parallel chains:
- T008 → T009 [P]
- T010 → T011 [P]
- T012 → T013 [P]

**Batch D (Phase 3 — US1 dashboard chunk)**: T020 [P] + T021 [P] + T023 [P] all touch different files, all can land in parallel. T022 + T024 + T025 sequence into the dashboard wiring; T026 is the final e2e gate.

**Batch E (e2e tests)**: T026, T027, T031, T034, T037 — separate files in `tests/e2e/`. After respective implementation tasks land.

---

## Implementation Strategy

**MVP scope**: Phases 1 + 2 + 3 = Setup + Foundational + US1 (HTTPS-on-a-domain with dashboard). Delivers the headline feature ("one TOML edit, HTTPS in 2 minutes") AND surfaces it in the dashboard so operators see what they got. ~26 tasks.

**Incremental delivery beyond MVP**: US2 (1 task, validates LB), US4 (3 tasks, unblocks macOS dev), US6 (1 task, validates reload safety) are all small and high-value — recommended as the next bundle. US3 (4 tasks, L4) and US5 (2 tasks, polish) round out v0.3.

**Stop-and-validate points**:

- After Phase 2: parser + ingress primitives compile and pass unit tests. No user-visible behavior yet; integration smoke test = `bin/proxa server` starts without panicking.
- After Phase 3 (US1): SC-001 passes; dashboard shows real routes. Stop here for a feedback round before US2-6.
- After Phases 4–6: all P1 + P2 user stories pass. Cut a v0.3-rc1 tag.
- After Phase 9: cut v0.3.0.

---

## Notes & follow-ups

- `synctest` (Go 1.26 stable) is used in T013 for deterministic atomic-pointer reload testing. Same pattern as 002's `probe.Manager` tests.
- Pebble (T038) needs network access to pull the image; documented as a `make test-integration` requirement.
- ACME against real Let's Encrypt is NEVER part of automated tests — would hit rate limits and require a real domain. Manual validation only (T040 quickstart walk).
- Coraza WAF integration is noted in research R-001 as a future v0.5+ option; explicitly not in 003 scope.

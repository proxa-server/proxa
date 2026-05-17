# Implementation Plan: Ingress — L7/L4 Routing with Auto-TLS

**Branch**: `003-ingress` | **Date**: 2026-05-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/003-ingress/spec.md`

## Summary

Land L7 HTTPS/HTTP routing and L4 TCP/UDP forwarding inside the existing `proxa server` binary. HTTPS termination is automated via ACME (Let's Encrypt by default) using CertMagic. Reverse-proxy and routing logic is hand-rolled on top of `net/http/httputil.ReverseProxy` so we avoid the full Caddy v2 binary surface but still get production-grade TLS automation. L4 forwarding is pure stdlib `net`. The ingress is a sibling goroutine to the reconciler, watches the StateStore for route changes, and hot-swaps its routing table without breaking in-flight connections. Dashboard ships with this feature — a new Routes card mirrors the Services card so operators see what is actually reachable.

## Technical Context

**Language/Version**: Go 1.26.x (existing project pin per `feedback_commit_timestamps`-adjacent memory and CLAUDE.md). `CGO_ENABLED=0` for production binaries; `=1` only at the `-race` test step.

**Primary Dependencies**:

- `github.com/caddyserver/certmagic` (Apache-2.0) — ACME client + cert storage + renewal timers. Justified in research.md R-001.
- Standard library `net/http`, `net/http/httputil` (`ReverseProxy`), `net` (L4 forwarder).
- Existing direct deps stay: chi (routing for the API), docker/docker (runtime), modernc.org/sqlite (state).
- Already-imported indirectly via CertMagic: `github.com/mholt/acmez/v3`, `github.com/libdns/libdns`, `github.com/zeebo/blake3`. License audit refresh required (§IX).

**Storage**:

- ACME account state + issued certificates under `${PROXA_DATA_DIR}/certs/` (CertMagic's default `FileStorage` backend, mode 0700 / 0600 per FR-014).
- Routes persist in the existing SQLite store as part of `Service.Spec` JSON — no schema migration; the parser extension serializes a new `routes` field.

**Testing**:

- Unit tests in each package — table-driven, `testing` stdlib.
- `//go:build dockerd` for ACME against the Pebble test CA (a local container the test brings up).
- `//go:build e2e` for SC-001/2/3/6 end-to-end via the compiled binary and a real Docker daemon.
- `go test -race` on every package that touches shared ingress state.

**Target Platform**: Linux server (production), macOS Docker Desktop (developer ergonomics). The HTTP-probe-via-ingress path (FR-010 + SC-007) closes the macOS bridge-IP gap.

**Project Type**: Single Go binary embedding API + reconciler + ingress + dashboard.

**Performance Goals**:

- p95 routing overhead < 1 ms above bare reverse-proxy (i.e., adding ingress in front of a service must not visibly slow it down).
- Hot-reload of the routing table < 100 ms wall-clock with zero dropped in-flight requests (FR-009, SC-003).
- 100 connections/s sustained on a single VPS-sized node (1 vCPU) without saturating CPU.

**Constraints**:

- Strict zero-downtime: no 5xx during route edits (SC-003). No connection reset during cert renewal (FR-003).
- Project scoping (§III): hostname uniqueness enforced across all services in the same project AND across projects.
- §V single binary: CertMagic must NOT spawn subprocesses or require external CA tooling. No sidecar.
- §IX license: every new direct and transitive must be Apache-2.0 / MIT / BSD / MPL-2.0 — re-audit in polish.

**Scale/Scope**: Single-node v0.3. Routes per node: ~50 (well below CertMagic's tested limit). Replicas per route: ~10. Multi-node ingress synchronization is explicitly out of scope (deferred to v1.0 cluster mode).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | How this feature complies |
|---|---|
| §I Architecture First | New `Ingress` interface in `internal/ingress/` — concrete `CaddyMagicIngress` (or similar) behind it. Test suite uses a fake `Ingress` for unit-testing the reconciler+ingress coupling. |
| §II Security by Default | Ingress listeners bind only what is configured. ACME-issued certs stored 0600; ACME account key 0600. No secret material in slog. HTTP→HTTPS redirect mandatory when TLS is enabled (FR-006). |
| §III Project Scoping | FR-017 makes hostname uniqueness a project-scoped property AND a cross-project property (operator cannot accidentally hijack another project's hostname). Routes table validation in `internal/parser/toml/validate.go` runs the cross-project check at parse time. |
| §IV Go Idioms | `context.Context` first arg on every public function. `slog` JSON for ingress events. Table-driven unit tests. `-race` clean. No CGO. |
| §V Single Binary | CertMagic is a library, not a subprocess. L7 reverse proxy is `net/http/httputil.ReverseProxy`. L4 is `net`. Dashboard remains HTMX + Alpine via `go:embed`. |
| §VI Cluster-Ready | `Ingress` is an interface; the cluster-mode implementation (v1.0) will be a different concrete that coordinates routing tables across nodes. The interface intentionally exposes "set routes" + "lookup backend", not "bind port 80" — abstractions that survive multi-node. |
| §VIII Zero-Downtime by Default | FR-009 (hot-reload) and SC-003 (no 5xx during reload) operationalize §VIII for the routing layer. CertMagic's renewal happens in-place — no listener restart. Old in-flight connections drain on `Shutdown(ctx)` with a generous deadline. |
| §IX Permissive License | CertMagic + acmez + libdns all Apache-2.0. License audit refresh task in Polish phase. |
| §XI Commit Strategy | Scopes: `ingress`, `parser/toml`, `types`, `cli`, `server`, `web`, `licenses`, `e2e`. One commit per task. |

**No deviations require a Complexity Tracking entry.** The closest call is "do we ship the full Caddy v2 library or just CertMagic + custom router" — resolved in research.md R-001 in favor of the latter for §V binary-size reasons.

## Project Structure

### Documentation (this feature)

```text
specs/003-ingress/
├── plan.md              # This file
├── research.md          # Phase 0 — library choice + ACME strategy + dashboard touchpoints
├── data-model.md        # Phase 1 — Route, BackendPool, TLSCertificate, IngressConfig
├── quickstart.md        # Phase 1 — operator walkthrough (DNS → up → curl https://)
├── contracts/
│   ├── ingress.md       # The Ingress interface + Router + BackendPool contracts
│   └── route.md         # The Route TOML grammar extension + validation rules
└── tasks.md             # Phase 2 — produced by /speckit.tasks (NOT this command)
```

### Source Code (repository root)

New files:

```text
internal/
├── ingress/                   # NEW package
│   ├── ingress.go             # Ingress interface + Server lifecycle (Run/Shutdown)
│   ├── router.go              # Hostname/path → service lookup, project-scoped
│   ├── backend_pool.go        # Service → []replicaIP, updated from probe.Snapshot
│   ├── proxy.go               # httputil.ReverseProxy wrapper with LB strategy
│   ├── tls.go                 # CertMagic wiring + cert storage path resolver
│   ├── l4.go                  # TCP + UDP forwarders (pure net.Listen / net.ListenPacket)
│   ├── reload.go              # Atomic routing-table swap; drains old proxy
│   ├── *_test.go              # Unit + race coverage
│   └── ingress_integration_test.go  # //go:build dockerd — ACME vs Pebble
│
└── web/
    └── templates/
        └── routes_table.html  # NEW — HTMX-polled Routes card

Modified files:

internal/
├── cli/server.go              # Construct ingress.New, pass to reconciler.New
├── parser/toml/
│   ├── parse.go               # Extend grammar with [[route]] block + [health].via
│   ├── validate.go            # Add route-conflict / route-needs-host / route-invalid-protocol
│   └── testdata/              # +invalid-route-conflict.toml, +invalid-route-needs-host.toml, +invalid-route-bad-l4.toml, +valid-route-tls.toml, +valid-route-l4-tcp.toml
├── server/
│   ├── handlers.go            # New GET /api/v1/routes, GET /api/v1/ingress
│   ├── ui.go                  # buildUIData → add TotalRoutes, IngressInfo, RouteRow{TLSState, BackendCount}
│   └── ui_routes.go           # NEW — /ui/routes HTMX endpoint mirroring /ui/services
└── reconciler/
    └── reconciler.go          # Call ingress.UpdateRoutes(...) inside reconcileProject after probe snapshots are stable

pkg/types/
├── taskdef.go                 # Add Route struct + Health.Via field
└── service.go                 # No change (routes live inside Spec.Routes)

config (existing internal/config/config.go):
└── Extend with IngressConfig {HTTPPort, HTTPSPort, TLS, Email, ACMEDirectoryURL}.
   Reads from config.toml [ingress] section + PROXA_INGRESS_* env vars.

tests/e2e/
├── ingress_https_test.go      # NEW — SC-001 (self-signed mode, skip Let's Encrypt rate limits)
├── ingress_lb_test.go         # NEW — SC-002 (LB across 3 replicas)
├── ingress_reload_test.go     # NEW — SC-003 (hot-reload, no 5xx)
├── ingress_tcp_test.go        # NEW — SC-006 (TCP forward to redis on a route)
└── ingress_probe_test.go      # NEW — SC-007 (re-enable the 4 macOS-skipped 002 tests)

web/_src/app.css               # Tailwind-style additions for chip-blue (TLS valid) +
internal/web/static/app.css    # chip-purple (TLS renewing) if Tailwind v4 path is used,
                               # otherwise hand-CSS in the v0.0 file.
```

**Structure Decision**: The `internal/ingress/` package is sibling to `internal/reconciler/` and `internal/probe/` — runs as a goroutine in `runServer` alongside them, sharing the same shutdown ctx. The reconciler pushes route + backend updates to ingress via the `UpdateRoutes` method on the `Ingress` interface; ingress never reaches back into the reconciler. Dashboard additions live next to the existing `services_table.html` so HTMX's polling model is preserved (one card = one fragment URL = `every 5s`).

## Complexity Tracking

> Empty — Constitution Check passes without deviations.

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| _(none)_  | —          | —                                   |

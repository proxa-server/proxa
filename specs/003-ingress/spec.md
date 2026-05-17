---
description: "L7/L4 ingress — HTTPS via auto-TLS, hostname/path routing, TCP/UDP forwarding"
---

# Feature Specification: Ingress — L7/L4 Routing with Auto-TLS

**Feature Branch**: `003-ingress`

**Created**: 2026-05-16

**Status**: Draft

**Input**: Deliver L7 HTTPS/HTTP routing (hostname + path) with auto-provisioned TLS certificates and L4 TCP/UDP forwarding. This closes the gap between "a container is running" and "an external user can reach it" — the missing layer in v0.2 that forced operators to choose between direct host-port binding (collides on rollover) or ingress-only ports (had no route).

## Overview

Today an operator can deploy a service and the reconciler keeps the right number of containers alive, but reaching them from outside requires either binding a host port directly (one host, one port, breaks during start-first rollover) or running an external reverse proxy and pointing it at container IPs by hand. Both paths defeat the "single-binary, opinionated defaults" promise.

This feature gives Proxa its own routing layer: HTTPS termination with automatic Let's Encrypt certificates, hostname-based virtual hosting, simple path-prefix routing, load balancing across replicas, and L4 forwarding for non-HTTP workloads. It runs in the same proxa binary as the rest of the control plane — no separate process, no per-host nginx, no copy-pasted Caddyfile.

The downstream effects: start-first rollover finally works for host-port services (the new container comes up behind ingress on its own bridge IP, then the ingress backend pool swaps), HTTPS becomes the default (not a polish item), and HTTP probes can optionally route through the ingress layer so macOS dev environments work without a VM workaround.

## User Scenarios *(mandatory)*

### User Story 1 — Expose an HTTP service on a domain (Priority: P1, MVP)

A solo operator deploys `whoami` and wants `https://whoami.example.com` to serve it — no DNS challenge dance, no certificate renewal cron, no nginx config.

**Why this priority**: This is the single largest reason new users abandon container orchestrators — getting HTTPS working on a custom domain is the "real" first-day task. Delivering it as a one-line declaration is Proxa's main competitive promise.

**Independent Test**: With DNS pointing `whoami.example.com` → the Proxa host, deploy a service with `[[route]] host = "whoami.example.com"` and `[[expose]] container = 80 protocol = "http"`. Within two minutes (cert issuance budget), `curl https://whoami.example.com/` returns the whoami response over a valid TLS connection.

**Acceptance Scenarios**:

1. **Given** a service declared with `[[route]] host = "api.example.com"` and `tls = true` enabled in `[ingress]` config, **When** `proxa up service.toml` runs and the operator waits two minutes, **Then** `curl https://api.example.com/` returns the service's HTTP response with a Let's Encrypt-issued certificate trusted by the system CA bundle.
2. **Given** the same service, **When** `curl http://api.example.com/` is issued, **Then** the response is `301 Moved Permanently` to the `https://` equivalent.
3. **Given** the cert is approaching expiry (30 days), **When** the ingress's renewal timer fires, **Then** a new cert is issued and hot-swapped without dropping connections.

---

### User Story 2 — Scale replicas behind a single ingress endpoint (Priority: P1, MVP)

The operator scales `web` from 1 to 3 replicas. External requests should distribute across the 3 backends; killing one replica should not drop in-flight requests on the survivors.

**Why this priority**: Bare reverse-proxy mode is not interesting if it means manually editing routes when scaling. Backend pool management is what makes ingress useful for production-shaped workloads.

**Independent Test**: Deploy 3 replicas of whoami behind one route. Hit the route 30 times with `curl`. Verify the response (which contains the container hostname) cycles across at least 2 distinct backends.

**Acceptance Scenarios**:

1. **Given** a service with `replicas = 3` and one `[[route]]`, **When** 30 sequential requests hit the route, **Then** responses originate from at least 2 distinct replicas (random selection — exact 1/3 split is not required).
2. **Given** an in-flight request being served by replica 1, **When** the operator runs `proxa down` on a different service that triggers a tick that removes replica 1 of the first service (simulated by `docker kill`), **Then** the in-flight response completes successfully (no truncation, no connection reset visible to the client).
3. **Given** `lb_strategy = "round-robin"` is set on a route, **When** 9 sequential requests hit the route against 3 replicas, **Then** each replica receives exactly 3 requests.

---

### User Story 3 — Forward a raw TCP service (Priority: P2)

A user deploys a Postgres service and wants `psql -h db.example.com -p 5432` to work. No TLS termination — just a transparent TCP forward to one of the postgres replicas.

**Why this priority**: Databases and queues are the next-most-common workload after web services. Without L4 forwarding, Proxa is "HTTP-only" and feels half-finished.

**Independent Test**: Deploy postgres with a `[[route]] host = "db.example.com" l4 = "tcp" port = 5432`. Run `psql -h db.example.com -p 5432 -U postgres -c "SELECT 1"` and get `1`.

**Acceptance Scenarios**:

1. **Given** a TCP route declared for a single-replica postgres, **When** an external `psql` client connects, **Then** the connection succeeds and a query returns expected results.
2. **Given** a TCP route with 2 replicas (stateless service like a connection-pooled cache), **When** the operator establishes a long-lived connection, **Then** that connection stays pinned to one backend for its lifetime (no mid-connection rebalancing).

---

### User Story 4 — Route HTTP probes through the ingress (Priority: P2)

A developer on macOS Docker Desktop deploys a service with an HTTP `[health]` probe and wants the probe to actually run — bridge IPs are not routable from the macOS host, so the direct-dial path silently fails.

**Why this priority**: This unblocks the four e2e tests (SC-002-1, 2, 4, 5) that currently skip on macOS, and removes the dev-environment caveat in the project's README.

**Independent Test**: On a macOS dev box (or any host where docker bridge IPs are not routable), deploy a service with `[health]` probe and `via = "ingress"`. After one tick + grace, `proxa ps` reports `healthy`.

**Acceptance Scenarios**:

1. **Given** an environment where the docker bridge subnet (172.17.0.0/16) is NOT routable from the host, **When** a service is deployed with `[health].path = "/health"` and `[health].via = "ingress"`, **Then** within `interval × retries + tick`, the service reaches `healthy` status — the probe routes via the ingress layer instead of dialing the container IP.
2. **Given** the same service with `[health].via = "direct"` (the existing default), **When** the probe runs on a routable Linux host, **Then** the existing behavior is unchanged (direct bridge-IP dial).

---

### User Story 5 — Operate on a host without root (Priority: P3)

The operator runs Proxa on a managed VPS without sudo for port 80/443 binding. They configure `[ingress].http_port = 8080` and `[ingress].https_port = 8443` and put a managed load balancer in front.

**Why this priority**: Cloud VPS environments routinely block low ports for non-root users; forcing root is hostile. Configurable ports is a small ask that unblocks a real user class.

**Independent Test**: Start the proxa server as a non-root user with `[ingress].http_port = 8080`. Deploy a service with a route. Verify ingress accepts connections on `:8080` and routes correctly.

**Acceptance Scenarios**:

1. **Given** `[ingress].http_port = 8080` and `[ingress].https_port = 8443` in the config, **When** the proxa server starts as a non-root user, **Then** it binds the configured ports without elevated permissions and serves routes from them.
2. **Given** TLS is enabled but the configured `http_port` is not 80, **When** an HTTP request comes in, **Then** the redirect to HTTPS targets the configured `https_port`, not the standard 443.

---

### User Story 6 — Hot-reload routes without dropping connections (Priority: P2)

The operator edits a route, runs `proxa up`, and existing connections must complete on the old route configuration before the new one takes effect.

**Why this priority**: Config edits during business hours should not require a maintenance window. This is a baseline zero-downtime expectation matching §VIII of the constitution.

**Independent Test**: Establish an HTTP/1.1 keep-alive session against a route. Edit the route (e.g., change `lb_strategy`). Re-run `proxa up`. Continue the keep-alive session — the next request still succeeds and gets the new strategy.

**Acceptance Scenarios**:

1. **Given** a route is being actively served, **When** the route definition is updated via `proxa up`, **Then** no in-flight request returns a 5xx error caused by the reload.
2. **Given** a route is deleted, **When** the next request arrives for that hostname, **Then** the response is `404 Not Found` (not `503 Service Unavailable` and not a connection reset).

---

### Edge Cases

- **Wildcard hostnames** (`*.example.com`) — out of scope for v0.3; one route = one hostname.
- **Conflicting routes** (two routes claiming `api.example.com /v1/*`) — the parser rejects with `route-conflict`; second `proxa up` fails with a clear message.
- **Empty backend pool** (all replicas exited, none replaced yet) — ingress returns `503 Service Unavailable` with a `Retry-After: 5` header.
- **ACME challenge fails** (DNS not pointing at the host yet) — TLS for that host stays self-signed until the next renewal attempt succeeds; HTTP path keeps serving; an INFO-level slog event captures the failure.
- **L4 backend down mid-connection** — the existing TCP connection is severed (no transparent failover); new connections route to surviving backends.
- **Operator deploys a route for a non-existent service** — parser accepts (forward-compat for staged deploys), ingress returns `503` until the service appears.
- **Bridge-routable probe on a non-routable host** — clear error message in the slog log pointing the operator at `[health].via = "ingress"`.
- **TLS without a public domain** — explicitly enabling `tls = true` for a hostname like `localhost` or an IP fails fast with a config error (Let's Encrypt cannot issue for those).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide an L7 ingress component that accepts inbound HTTP and HTTPS connections on configurable ports (defaults: 8080 / 8443 in dev; operator picks 80 / 443 in production).
- **FR-002**: The system MUST automatically provision TLS certificates for any hostname declared in a `[[route]]` block when `[ingress].tls = true` is set and `[ingress].email` is populated.
- **FR-003**: The system MUST renew certificates before expiry without operator action and without dropping in-flight connections during the swap.
- **FR-004**: The system MUST route inbound requests to a service's replica pool based on the request's `Host` header matching the route's `host` field, with optional path-prefix matching via the route's `path` field (a single trailing `*` is the only wildcard form supported).
- **FR-005**: The system MUST distribute requests across replicas of a service using a configurable strategy per route — `random` (default), `round-robin` — and treat replicas as a pool that automatically updates when the reconciler adds or removes replicas.
- **FR-006**: The system MUST issue an HTTP-to-HTTPS redirect (`301 Moved Permanently`) for any hostname that has TLS enabled, preserving path and query string.
- **FR-007**: The system MUST provide an L4 TCP forwarder for routes declared with `l4 = "tcp"`, transparently forwarding connection bytes between the client and a chosen backend replica with no protocol-level inspection.
- **FR-008**: The system MUST provide an L4 UDP forwarder for routes declared with `l4 = "udp"` with the same packet-forwarding semantics as TCP.
- **FR-009**: The system MUST hot-reload the routing table when the StateStore reports a route change, with a strict guarantee that no in-flight HTTP request or established TCP connection is interrupted by the reload.
- **FR-010**: The system MUST support routing HTTP `[health]` probes through the ingress layer when the service's `[health].via = "ingress"` field is set, as an alternative to the existing direct-bridge-IP probe path.
- **FR-011**: The system MUST reject malformed `[[route]]` declarations at parse time, including conflicting (hostname, path) pairs across services in the same project — with explicit error codes the operator can search for.
- **FR-012**: The system MUST return `503 Service Unavailable` with a `Retry-After: 5` header when a matched route has zero healthy replicas; it MUST NOT proxy to an unhealthy replica.
- **FR-013**: The system MUST respect `Service.Status` from the reconciler — replicas whose probe Snapshot reports `HealthOK = false` are excluded from the backend pool until they recover.
- **FR-014**: The system MUST persist ACME account state and issued certificates under `${PROXA_DATA_DIR}/certs/` with restrictive filesystem permissions (mode 0700 on the directory, 0600 on key material).
- **FR-015**: The system MUST allow the operator to disable TLS entirely (`tls = false`, the default) for local development and air-gapped deployments, in which case all routes serve HTTP only on the configured `http_port`.
- **FR-016**: The system MUST log a structured slog event for every route hot-reload, every ACME challenge attempt (success and failure), every backend selection failure (no healthy replicas), and every connection reset triggered by a backend disappearing.
- **FR-017**: The system MUST scope routes by project — a route declared in project `socio-do` cannot match traffic intended for project `kut-do` even if they share a hostname, and the parser rejects cross-project hostname collisions with the same `route-conflict` code as in-project ones.
- **FR-018**: The system MUST NOT require any new third-party process or sidecar — the ingress runs in the same `proxa server` binary as the reconciler and API server, sharing the same lifecycle and shutdown semantics.

### Key Entities *(include if feature involves data)*

- **Route**: An operator-declared mapping of (project, hostname, optional path-prefix, L4-protocol) → service. Lives inside the service's TOML as a `[[route]]` block; persisted in the StateStore as part of the service's spec.
- **BackendPool**: The in-memory, ingress-side view of a route's reachable replicas. Updated each reconciler tick from `Service.Replicas` + the probe Manager's snapshots. Not persisted — derivable from authoritative state.
- **TLSCertificate**: An ACME-issued (or self-signed, for non-public hostnames) certificate + private key pair stored under `${PROXA_DATA_DIR}/certs/`. Lifecycle owned by the ingress component, not the operator.
- **IngressConfig**: Server-wide ingress settings — listen ports, TLS toggle, ACME email, ACME directory URL (defaults to Let's Encrypt production). Lives in the existing `${PROXA_DATA_DIR}/config.toml`, not in per-service TOMLs.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A new operator, starting from a fresh `proxa init` and an A-record pointing at the host, can deploy a service and reach it at `https://service.example.com` with a valid certificate within 3 minutes of running `proxa up`.
- **SC-002**: When a service is scaled from 1 to 3 replicas, 30 sequential requests through the ingress route hit at least 2 distinct replicas (load balancing observable from the outside).
- **SC-003**: When the operator runs `proxa up` to modify a route on a live system, 100 % of HTTP/1.1 keep-alive sessions established before the edit complete their next request without a 5xx error.
- **SC-004**: When a backend replica is killed mid-request, the in-flight request that landed on that replica returns an error to the client, but the next request from the same client lands on a healthy replica within 1 reconciler tick (≤ 5 seconds).
- **SC-005**: TLS certificates renew automatically — a service deployed 90 days before the test still serves a non-expired certificate without operator action.
- **SC-006**: A TCP route to a single-backend postgres replica supports a `psql` session that runs `SELECT 1` and returns the expected result, indistinguishable from connecting to the container directly.
- **SC-007**: The four HTTP-probe e2e tests that currently skip on macOS Docker Desktop (SC-002-1, SC-002-2, SC-002-4, SC-002-5 of feature 002) PASS when the service TOML opts into `[health].via = "ingress"` and the ingress component is enabled.
- **SC-008**: When the configured ingress ports are non-privileged (e.g., 8080 / 8443), the proxa server binary started by a non-root user successfully binds those ports and serves traffic — verified by an integration test that runs under a dedicated unprivileged test user.

## Assumptions

- **DNS is the operator's problem**: this feature does not provision DNS records; the operator is expected to point an A or CNAME at the host before declaring a route with a public hostname.
- **One ingress instance per node**: multi-node load balancing is a v1.0 concern (cluster mode); for v0.3 the ingress runs in the same server process and listens on the local node's IP only.
- **ACME HTTP-01 challenge is sufficient**: this feature does not implement DNS-01 for wildcard certs; operators needing wildcards stand up their own ACME tooling outside Proxa (deferred to v0.4+).
- **Project-scoped hostname uniqueness**: a hostname may appear in at most one project; cross-project routing requires distinct hostnames. This matches §III project-scoping and avoids tenant-spoofing concerns.
- **Backend health = the probe Manager's truth**: the ingress consumes the same `Snapshot.HealthOK` values the reconciler uses for status aggregation; there is no parallel ingress-side health check.
- **`Strategy` is service-internal, `lb_strategy` is route-side**: `TaskDef.Strategy` (start-first / stop-first) governs how the reconciler replaces containers; `[[route]].lb_strategy` (random / round-robin) governs how the ingress picks among healthy backends. They never interact.
- **No protocol upgrade** for L4: a route is either L7 (HTTP/HTTPS) or L4 (TCP/UDP), declared at definition time. Switching requires a redeploy.
- **Default TLS off**: out-of-the-box, ingress serves plain HTTP on the configured port — opt-in to ACME by setting `tls = true` and an `email`. This keeps the "I just want to try Proxa" path zero-cost on first run.

## Dependencies

- **002-health-checks merged at v0.2.0** — ingress reads `probe.Snapshot.HealthOK` to decide whether to include a replica in the backend pool.
- **`runtime.ContainerInfo.IPAddress`** — already populated by `InspectContainer` since 002 (was added to support HTTP probes); ingress reuses it to dial bridge IPs.
- **Persistent storage** — the existing StateStore handles route persistence as part of `Service.Spec`; no schema migration anticipated beyond serializing the new `[[route]]` block.
- **A library that implements ACME and an L7 reverse proxy** — to avoid hand-rolling HTTP/2 routing and TLS state machines. The chosen library must be Apache-2.0, MIT, BSD, or MPL-2.0 to satisfy §IX, and must be embeddable in a single Go binary (no sidecar, no CGO).
- **Operator awareness** — the README must document the privileged-port story (CAP_NET_BIND_SERVICE / systemd / non-standard ports) before this feature ships, otherwise SC-008 cannot be reproduced by a first-time user.

## Out of Scope

- **gRPC routing** (HTTP/2 with `application/grpc` content type and `Trailer` semantics) — deferred until a real workload asks; the HTTPS layer will pass gRPC traffic through but Proxa-side load balancing aware of gRPC frame boundaries is not promised.
- **Weighted routing / canary** — Feature 005 territory; in v0.3 a backend is either in the pool or out, no per-backend weight.
- **WebSocket sticky sessions** — connections upgrade normally and stay pinned to the chosen backend for the connection's lifetime (FR-005 semantics already cover this), but explicit cookie- or IP-based stickiness for HTTP traffic is out of scope.
- **L7 request rewrites** (header injection, URL rewriting, body transforms) — beyond path-prefix matching for routing, the ingress is a transparent proxy.
- **IPv6-only deployments** — IPv6 traffic on dual-stack hosts is forwarded normally, but a host with no IPv4 binding is not a tested configuration in v0.3.
- **DNS-01 ACME challenge** (and therefore wildcard certificates) — HTTP-01 only.
- **Rate limiting, IP allow/deny lists, geofencing** — observability and security policy live in v1.x; v0.3 is "make routes work".
- **Multi-node ingress synchronization** — a single proxa server's ingress only knows about its own node's containers; multi-node coordination is a v1.0 cluster concern.
- **Wildcard hostnames** (`*.example.com`) — explicit hostnames only.

## Testing Strategy

- **Unit tests**: route table builder (given N services with M routes, produce the right ingress library config), backend pool updater (probe snapshot → pool membership), config validator (`[[route]]` parser extension), L4 connection forwarder (loopback echo server end-to-end), and parser rejection cases for `route-conflict` / `route-needs-host` / `route-invalid-protocol`.
- **`//go:build dockerd` integration tests**: ACME against the Pebble staging server (Let's Encrypt's local test CA) to verify the cert issuance + renewal flow end-to-end without hitting Let's Encrypt rate limits.
- **`//go:build e2e` tests**: SC-001 (one-route HTTPS reachability — local self-signed mode to skip DNS), SC-002 (load balancing across 3 replicas), SC-003 (hot-reload safety with a sustained-request load during a route edit), SC-006 (TCP forward end-to-end with a tiny Go TCP client), SC-007 (re-enable the 002 skipped tests with `via = "ingress"`).
- **Race coverage**: ingress lives in the same process as the reconciler — the backend pool is read by ingress goroutines (one per inbound request) and written by the reconciler tick goroutine. `go test -race` must pass on every commit touching ingress state.

## References

- Technical Spec Section 7: Ingress (delivered here; the section title in the original docx).
- Constitution §V Single Binary Zero Dependencies — the ingress library must be embeddable, not a sidecar.
- Constitution §VIII Zero-Downtime by Default — FR-009 (hot-reload) and SC-003 (no 5xx during reload) operationalize this principle for the routing layer.
- Constitution §IX Permissive License — any new direct dependency must be re-audited; `docs/licenses.md` to be updated as part of polish.
- Feature 002 spec, §"Out of Scope" — "HTTP/TCP ingress (Feature 003)" is the entry that this spec answers.

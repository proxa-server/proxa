# Phase 0 — Research: Ingress Library + ACME + Routing

Six decisions resolved before Phase 1 design.

---

## R-001: Ingress library — full Caddy v2 vs. CertMagic + stdlib

**Decision**: Import `github.com/caddyserver/certmagic` for ACME automation and cert storage. Use `net/http`, `net/http/httputil.ReverseProxy` for the L7 reverse-proxy + load balancer. Use stdlib `net` for L4 TCP/UDP forwarding. **Do NOT** import the full `github.com/caddyserver/caddy/v2`.

**Rationale**:

- Full Caddy v2 pulls roughly 150–200 transitive modules (modules system, Caddyfile parser, every built-in HTTP handler, JSON config loader, admin API). We would use a small fraction of that surface — just the reverse proxy. The binary size + license-audit cost is disproportionate to the win.
- CertMagic alone is `~15` new transitive modules (acmez, libdns, miekg/dns, zeebo/blake3, mholt/acmez/v3, and the standard x/crypto / x/sys family already in our tree). Manageable, all Apache-2.0 / MIT / BSD per `pkg.go.dev/<m>?tab=licenses` quick check.
- `httputil.ReverseProxy` is mature, supports `Director` (mutate outbound request) and `Transport` (custom dial / round-trip) — both of which we need for backend selection.
- Hot-reload becomes our own code (a `sync.Map[host]*Route` swap inside a single atomic.Pointer load — see contracts/ingress.md), not a JSON-config replay. Simpler to reason about and to test.

**Alternatives rejected**:

- **Full Caddy v2 library** — too much surface for the slice we'd use. Embedding it would also force us into Caddy's module system idioms, which clash with our straightforward Go interface approach.
- **mholt/acmez directly (no CertMagic)** — saves ~5 transitives but we'd reimplement cert storage, renewal timers, OCSP stapling, and on-the-fly issuance retries. CertMagic is the project's own opinionated wrapper around acmez and is the right altitude.
- **Traefik library or libnetwork ingress** — both are framework-shaped (config-driven, plugin-loaded). Not embeddable as a Go library in a clean way.
- **External nginx + filesystem reload** — violates §V "single binary, no sidecar".
- **Coraza WAF** — orthogonal concern (security policy / WAF rules). Out-of-scope per spec §"Out of Scope": "Rate limiting, IP allow/deny lists, geofencing — observability and security policy live in v1.x". Coraza is a strong fit for a future security feature (probably v0.5+), where it would slot in as a middleware in the ingress pipeline. Noted in memory for future consideration; not pulled into 003.

**Implementation**: CertMagic's `magic.HTTPS(domains, handler)` is the one-call setup we WILL NOT use because it owns the listener loop. Instead, we use `magic.Config` + `magic.New(cache, cfg)` to drive cert issuance ourselves, then mount the returned `*tls.Config` on our own `http.Server` that we own end-to-end (including shutdown semantics).

---

## R-002: ACME challenge type — HTTP-01 vs DNS-01 vs TLS-ALPN-01

**Decision**: HTTP-01 only. The proxa server's HTTP listener serves the `/.well-known/acme-challenge/` path before forwarding the rest to the routing layer.

**Rationale**:

- HTTP-01 requires only that the server be reachable on port 80 of the claimed hostname — matches the operator's mental model ("I pointed DNS at this box").
- DNS-01 requires API credentials for the operator's DNS provider, plus per-provider plugins (libdns/cloudflare, libdns/route53, etc.). Out of scope for v0.3 per spec assumptions.
- TLS-ALPN-01 is the most elegant on paper but needs the operator to point port 443 at us BEFORE we have a cert — chicken-and-egg awkward for new users.

**Alternatives rejected**:

- DNS-01 — deferred to v0.4+ (also unblocks wildcard certs which the spec leaves out of scope).
- TLS-ALPN-01 — feasible but less first-day-friendly.

**Implementation**: CertMagic supports HTTP-01 by default; we just give it a chance to handle the `/.well-known/acme-challenge/*` path on port 80 before our router sees it. The router does this with a high-priority match.

---

## R-003: Hot-reload mechanism — config replay vs atomic pointer swap

**Decision**: Atomic pointer swap. The routing table is held in an `atomic.Pointer[*Router]`. `UpdateRoutes(...)` builds a fresh `*Router` (all hostname/path indexes precomputed), then stores it. Request handlers `.Load()` the pointer at the top of each request and use that snapshot for the lifetime of the request.

**Rationale**:

- Reload cost is one pointer store. No lock held while requests are being served. No 5xx during reload (SC-003).
- An "old" router that a long-lived request is still using stays alive in memory until the request ends (Go's GC keeps it pinned via the request closure). Once all requests holding the old pointer finish, the old router becomes unreachable and is collected.
- L4 connections are unaffected — they hold their own backend reference for the connection's lifetime (FR-007 / FR-008).

**Alternatives rejected**:

- **Mutex around the routing table** — fine for low-rate edits, but holds writers vs readers contention. Pointer swap is lock-free on the read path.
- **CertMagic's reload-by-replaying-config** — would force us to adopt CertMagic's Caddy-style JSON config and replay it on every change. Too much ceremony for a key-value swap.

---

## R-004: L4 backend selection at connection setup — per-connection vs per-packet

**Decision**: Per-connection for TCP, per-source-address for UDP. Once a TCP connection is accepted on the ingress listener, the chosen backend is pinned for the connection's lifetime (FR-007 acceptance scenario 2). For UDP, source-IP+port → backend mapping cached for 30 seconds, then re-resolved.

**Rationale**:

- TCP per-connection pinning is the only viable model — mid-connection rebalancing breaks every protocol that has session state.
- UDP "stickiness" via a short cache mimics what L4 load balancers (haproxy, lvs) do for DNS, QUIC handshake, gaming — keeps packets from the same source going to the same backend for protocol continuity.
- The 30-second TTL is short enough to converge on backend pool changes without operator action.

**Alternatives rejected**:

- True stateless UDP (every packet gets a fresh backend choice) — breaks QUIC, DTLS, any UDP protocol with sequence numbers.
- TCP MPTCP-style rebalancing — irrelevant at our scale and not portable.

---

## R-005: Probe routing through ingress — HTTP probe rewrite vs new transport

**Decision**: When `[health].via = "ingress"`, the probe Manager constructs an `HTTPProbe` whose `URL` targets `http://<ingress-bind-ip>:<ingress-http-port>/<path>` AND injects the route's expected `Host` header. The probe is sent from inside the same process as the ingress (loopback), so no DNS resolution is needed.

**Rationale**:

- Loopback HTTP is always routable, even on macOS Docker Desktop — fixes the dev-environment gap from feature 002.
- The Host-header injection lets the existing ingress router resolve the request to the right service+backend without changing the probe interface.
- Keeps the `HTTPProbe` contract unchanged (it still issues one `GET` with a deadline); only the URL construction logic in `probe.Manager` shifts based on the spec field.

**Alternatives rejected**:

- **New `IngressProbe` type** — code duplication; the wire behavior is identical to HTTPProbe, only the URL builder differs.
- **Re-dial ingress via the public hostname** — requires real DNS resolution of the operator's hostname from inside the proxa process, which is fragile (split-horizon DNS, /etc/hosts edits).

**Implementation note**: The default for `[health].via` is `"direct"` — exactly preserves v0.2 behavior. Operators on macOS dev opt into `"ingress"` per service.

---

## R-006: Dashboard layout — separate Routes card vs merged Services+Routes view

**Decision**: Add a separate Routes card to the dashboard, polled via HTMX every 5 s at `/ui/routes`, sibling to the existing Services card. The cluster-status header row gains an Ingress widget showing HTTP port, HTTPS port, TLS on/off, and active cert count.

**Rationale**:

- Mirrors the existing pattern (one card = one HTMX poll fragment), preserving the slim-dashboard preference from the user-feedback memory.
- A merged view would create a cross-product table (service × route) that gets noisy as routes-per-service grow.
- Operators usually scan "what services exist" and "what is reachable from outside" as separate questions — the layout matches the mental model.

**Alternatives rejected**:

- **Single combined table** — gets ugly with multiple routes per service or services with no routes.
- **Defer dashboard to a later feature** — explicitly rejected per user instruction in /speckit.plan invocation ("dashboard MUST also land in this feature, not be deferred"). Matches the incremental-landing preference in memory.

**Implementation**: Two new chip colors needed in `internal/web/static/app.css`:

- `chip-blue` for `TLS valid` (cert present, not in renewal window)
- `chip-purple` for `TLS renewing` (within 30 days of expiry, ACME job in flight or queued)
- Reuse `chip-amber` for `TLS pending` (route declared, cert not yet issued)
- Reuse `chip-red` for `TLS failed` (ACME challenge failure)
- Reuse `chip-neutral` for `TLS off` (route declared but `[ingress].tls = false`)

The Ingress widget in the header looks like:

```text
Ingress  8080 / 8443  TLS:on  2 certs
```

Or, when TLS is off:

```text
Ingress  8080  TLS:off
```

---

## What's intentionally NOT in this research

- **Coraza WAF integration** — orthogonal to routing; tracked as a future v0.5+ option (see R-001 paragraph).
- **HTTP/3 / QUIC** — Go stdlib doesn't ship QUIC yet; would force a `net/http3` dep. Not asked for in any user story.
- **Per-replica observability (request rate, p95 latency)** — Metrics is a separate feature.
- **gRPC-aware load balancing** — out of scope per spec.

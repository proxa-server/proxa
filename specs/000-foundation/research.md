# Phase 0 — Research: Foundation

All "NEEDS CLARIFICATION" items resolved before Phase 1. Most decisions inherit directly from the constitution and CLAUDE.md; this document records the *why* and the rejected alternatives so future contributors don't re-litigate.

---

## R-001: SQLite driver — `modernc.org/sqlite` vs `mattn/go-sqlite3`

**Decision**: `modernc.org/sqlite`.

**Rationale**: Constitution §IV mandates `CGO_ENABLED=0`. `modernc.org/sqlite` is a pure-Go transpilation of SQLite; it links statically with no C toolchain. Performance is within ~15-25% of cgo-based drivers, which is irrelevant for the single-node control-plane workload (low write QPS, dominated by reconciliation loop cadence, not DB throughput).

**Alternatives rejected**:
- `mattn/go-sqlite3` — requires CGO. Disqualified by §IV.
- `crawshaw.io/sqlite` — CGO. Disqualified by §IV.
- Embedded BoltDB / bbolt — single-writer, no SQL, would force a hand-rolled query layer. SQLite gives us migrations, schema, and ad-hoc inspection via `sqlite3 proxa.db`.

**License**: BSD-3-Clause. Compliant with §IX.

---

## R-002: HTTP router — chi vs gorilla/mux vs stdlib

**Decision**: `github.com/go-chi/chi/v5`.

**Rationale**: chi has middleware composition, sub-router mounting (needed for `/api/v1/projects/{project}/...`), and parameter parsing without reflection. Tiny dep tree (no transitive surprises). Stable since 2015, maintained.

**Alternatives rejected**:
- `net/http.ServeMux` (stdlib pattern matching, since 1.22) — viable, and we use it where chi adds no value. But chi's middleware chain and route mounting save real code in the auth + project-scoping middleware that lands in 001+.
- `gorilla/mux` — reflection-based routing; archived in 2022 then revived under new maintainers; uncertain long-term posture.

**License**: MIT. Compliant with §IX.

---

## R-003: CLI framework — cobra+viper vs kong vs urfave/cli

**Decision**: `cobra` + `viper`.

**Rationale**: The CLI surface area (server start, agent join, service inspect, project CRUD, login, etc.) is large enough to justify a real framework. cobra handles subcommands + completions + man pages; viper handles 12-factor config (env vars + flags + TOML) with no glue code.

**Alternatives rejected**:
- `urfave/cli` — fine for small CLIs; loses to cobra on completion generation and subcommand depth.
- `kong` — elegant struct-tag DSL; smaller ecosystem; harder for AI agents to scaffold from examples.
- `flag` (stdlib) — would require us to hand-roll the entire command tree. Not justified.

**License**: cobra Apache 2.0, viper MIT. Compliant with §IX.

---

## R-004: TLS automation — CertMagic vs autocert vs manual ACME

**Decision**: `github.com/caddyserver/certmagic`.

**Rationale**: When the L7 ingress lands (later feature), we need ACME-based auto-issuance for user-supplied domains. CertMagic is the proven Caddy core, supports DNS challenges, on-demand TLS, and certificate sharing across cluster nodes (v1.0 multi-node).

**Alternatives rejected**:
- `golang.org/x/crypto/acme/autocert` — only HTTP-01, no DNS-01; tied to in-process cache; not cluster-friendly.
- Manual ACME implementation — multi-month project; reinvents a solved problem.

**License**: Apache 2.0. Compliant with §IX.

---

## R-005: Secret encryption — age vs NaCl box vs custom AEAD

**Decision**: `filippo.io/age`.

**Rationale**: Modern (2019), audited, simple file format. X25519 + ChaCha20-Poly1305. We need a way to encrypt secrets at rest in the state store and unwrap them inside the runtime sandbox. age's library API supports streaming encryption without temp files.

**Alternatives rejected**:
- `golang.org/x/crypto/nacl/secretbox` — symmetric only; key management becomes our problem.
- HashiCorp Vault — external service. Violates §V (single binary).
- AWS KMS / similar — vendor lock-in; not self-hosted.

**License**: BSD-3-Clause. Compliant with §IX.

---

## R-006: Logging — slog vs zap vs zerolog

**Decision**: `log/slog` (stdlib, Go 1.21+).

**Rationale**: Constitution §IV mandates `slog`. JSON handler to stderr by default. No reason to add a dependency for a problem the stdlib now solves.

**Alternatives rejected**: zap, zerolog — predate `slog`; faster on microbenchmarks but irrelevant at our log volume.

---

## R-007: Container runtime client — docker/docker/client vs containerd vs OCI exec

**Decision**: `github.com/docker/docker/client` initially, behind the `Runtime` interface.

**Rationale**: Docker is the most-installed local runtime; lowest friction for v0.x users. The `Runtime` interface (per §I) lets us swap in containerd or a direct OCI runtime later without touching handlers. Constitution §VI is satisfied because the interface is the only seam handlers see.

**Alternatives rejected for now**:
- `containerd` direct — fewer installed-bases for laptop dev; revisit in v0.x late or v1.0.
- `runc` direct — too low level for v0; we'd be reimplementing image pulls, networking, volumes.

**License**: Apache 2.0. Compliant with §IX.

---

## R-008: Linter — `staticcheck` vs `golangci-lint`

**Decision**: `staticcheck` for v0.0; revisit `golangci-lint` later if signal/noise warrants it.

**Rationale**: `staticcheck` is the canonical Go static analyzer (also bundled inside golangci-lint). Adding only `staticcheck` keeps CI fast and the dep surface narrow. `go vet` + `staticcheck` catches the vast majority of real defects.

**Alternatives rejected**: `golangci-lint` — bundles 50+ linters; configuration overhead, frequent rule churn, slower CI. Not worth it for a young codebase.

---

## R-009: Frontend bundling — embedded HTMX/Alpine/Tailwind

**Decision**: Check pre-built CSS + JS into `web/static/` and embed via `go:embed`. No Node.js at build or runtime.

**Rationale**: Constitution §V. Tailwind CLI is available as a single static binary (downloadable, not invoked from `go build`); we run it manually when CSS changes and commit the result. HTMX and Alpine ship as single JS files.

**Alternatives rejected**:
- `esbuild`-via-Go + Tailwind-as-Go-module — exists (`b0v1k/tailwind`-style ports) but immature and not maintained by the upstream.
- Server-side rendering only — loses HTMX's progressive enhancement story.

**Not in this feature**: `web/` only gets a placeholder README. Real assets land with the dashboard feature.

---

## R-010: Go 1.26 stdlib — what we'll actually use

Bumping `go.mod` to `go 1.26` unlocks several stdlib affordances downstream features will use. Recorded here so they don't get re-litigated later:

- **`errors.AsType[T]()`** — generic, type-safe `errors.As`. Will replace the boilerplate `var e *MyErr; errors.As(err, &e)` dance in handlers and middleware.
- **`slog.NewMultiHandler`** — multiplex logs to stderr + an in-memory ring buffer for the dashboard's "live tail" view. Saves us from writing a custom Tee handler.
- **`net.Dialer.DialIP/TCP/UDP/Unix`** context-aware variants — used by the agent for control-plane reconnection with deadline propagation.
- **`bytes.Buffer.Peek`** — non-advancing peek; useful for the log streaming endpoint when sniffing newlines.
- **`os.Process.WithHandle`** — pidfd/Handle access. Lets the runtime (eventually) signal containers via pidfd rather than racing through `kill(pid)` lookups.
- **`testing.T.ArtifactDir` + `-artifacts`** — CI can collect debug dumps from failing tests without ad-hoc temp paths.
- **`runtime/secret`** (experimental, `GOEXPERIMENT=runtimesecret`, Linux amd64/arm64) — secure temporary erasure for unwrapped secret bytes. **Strong candidate** for the age-backed `SecretsStore.Get` return path once the experiment stabilizes. Not enabled in v0.x (binary must build on all targets without GOEXPERIMENT), but noted for re-evaluation in v1.0.
- **Hybrid PQ TLS in `crypto/tls`** (`SecP256r1MLKEM768` enabled by default) — ingress TLS gets post-quantum-resistant key exchange for free.
- **`net/url.Parse` rejects malformed colon-in-host**: relevant when the ingress controller parses user-supplied hostnames. We'll accept the new strict behavior; do NOT set `GODEBUG=urlstrictcolons=0`.

Behavioral gotcha for API design: **`ServeMux` trailing-slash redirects are now 307 (Temporary), not 301**. Document this in the API spec when REST endpoints land — clients that cached 301s under earlier Go will need to refresh.

Constitution §IV ("standard library first") is *more* satisfied by 1.26 than by 1.22: every item above replaces something we'd otherwise have hand-rolled or pulled in a dependency for.

---

## R-011: License audit (constitution §IX gate)

All dependencies listed in Technical Context audited against §IX (Apache, MIT, BSD, MPL 2.0 only; no AGPL/SSPL/BSL/proprietary).

| Dependency | License | Status |
|---|---|---|
| go-chi/chi | MIT | ✅ |
| spf13/cobra | Apache 2.0 | ✅ |
| spf13/viper | MIT | ✅ |
| modernc.org/sqlite | BSD-3-Clause | ✅ |
| docker/docker/client | Apache 2.0 | ✅ |
| caddyserver/certmagic | Apache 2.0 | ✅ |
| filippo.io/age | BSD-3-Clause | ✅ |
| HTMX (htmx.org) | BSD-2-Clause (Zero-Clause BSD also offered) | ✅ |
| Alpine.js | MIT | ✅ |
| Tailwind CSS | MIT | ✅ |

For Foundation specifically, `go.mod` will be **empty of third-party deps** (FR-009: no speculative `go get`). The audit is recorded here so future PRs that add a dependency can be reviewed against this baseline.

---

## R-012: Reserved-slot strategy (proto/, web/, cmd/proxa-agent/)

**Decision**: Reserved directories exist in this feature with a `README.md` placeholder and (for `cmd/proxa-agent/`) a minimal `main.go` that compiles and prints version.

**Rationale**: Constitution §VI requires clustering seams from v0. Having `cmd/proxa-agent/` and `proto/` visible from the first commit signals that the agent + gRPC contract are real artifacts of the architecture, not afterthoughts. The README placeholders prevent confused "what goes here?" PRs.

**Alternatives rejected**: Creating these directories only when 001+ needs them — loses the architectural signal and risks layout drift between features.

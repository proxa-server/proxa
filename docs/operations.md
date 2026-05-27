# Proxa Operations

Practical notes for running `proxa server` in production.

## Privileged ports (80 / 443)

The ingress (Feature 003) defaults to `:8080` / `:8443` so a non-root
process can run `proxa server` without any host setup. In production
most operators want the standard ports — three options, pick one:

### Option 1 — Linux `CAP_NET_BIND_SERVICE` (recommended)

Grant the binary the capability that lets non-root processes bind
ports below 1024:

```sh
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/proxa
```

Then set the ports in `~/.proxa/config.toml`:

```toml
[ingress]
http_port  = 80
https_port = 443
tls        = true
email      = "ops@example.com"
```

Re-apply `setcap` after every binary upgrade — capabilities live on
the inode, not the executable name.

### Option 2 — systemd `AmbientCapabilities`

Skip `setcap` and let systemd grant the cap at process spawn:

```ini
[Service]
ExecStart=/usr/local/bin/proxa server
User=proxa
Group=proxa
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
```

### Option 3 — High ports + external 80/443 redirector

Keep `proxa server` on `:8080` / `:8443` and put a tiny `iptables`
PREROUTING rule (or a managed load balancer / cloud LB) in front:

```sh
iptables -t nat -A PREROUTING -p tcp --dport 80  -j REDIRECT --to-port 8080
iptables -t nat -A PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 8443
```

Operators on managed VPSes (DigitalOcean App Platform, Render,
Fly.io) usually pick this path because the platform's load balancer
already terminates TLS or forwards traffic.

## ACME / Let's Encrypt

With `tls = true` and a real `email`, the ingress requests certs via
the HTTP-01 challenge. Pre-requisites:

- DNS A or AAAA record points at the host running `proxa server`.
- Port 80 is reachable from the public internet (HTTP-01 specifically
  hits `/.well-known/acme-challenge/...` over port 80).
- The host's clock is accurate (ACME signs JWTs against current time).

Certs land under `${PROXA_DATA_DIR}/certs/` (mode 0700 on the dir,
0600 on key material). Renewal is automatic — the ingress checks
expiry every few hours and re-runs the challenge ~30 days before the
cert expires.

## Self-signed mode (development)

Set `tls = true` and leave `email = ""`. The ingress generates one
in-memory self-signed cert covering every SNI and serves it. Useful
for testing the HTTPS path without hitting Let's Encrypt rate limits.

## Probe routing on Docker Desktop (macOS / Windows)

HTTP probes default to dialing the container's bridge IP, which is
unreachable from the host on Docker Desktop. Workaround: opt the
service into probe-via-ingress per `[health]` block:

```toml
[health]
path = "/health"
port = 80
via  = "ingress"
```

The probe routes through `127.0.0.1:<http_port>` with the route's
hostname as the `Host` header — always reachable, regardless of
where the container's bridge lives.

On a Linux server with bridge-routable hosts this flag has no
effect (the default direct-dial path works); harmless to set.

## Lint tooling (v0.4.1+)

Lint is wired through Go 1.24's `tool` directive in `go.mod`. On a
fresh clone, contributors run `make lint` and `staticcheck` resolves
automatically through `go tool` — no manual `go install` or `brew
install` step.

The pinned version is visible in `go.mod`:

```
tool honnef.co/go/tools/cmd/staticcheck
```

To run staticcheck directly outside the Makefile:

```sh
go tool staticcheck ./...
```

To bump the version: `go get -tool honnef.co/go/tools/cmd/staticcheck@<new-version>`.
This pattern will extend to `gofumpt`, `golangci-lint`, etc. in
v0.4.2 (Test Foundation).

## Container-aware GOMAXPROCS (v0.4.1+)

Go 1.25 auto-detects CPU limits when Proxa runs inside a container.
No code change is required — `runtime.GOMAXPROCS(0)` reports the
effective CPU count, not the host's full core count.

To verify the auto-adjustment took effect, run `proxa system info`
against a running server (or open the dashboard's System Info
footer card / `/ui/system` page):

```
$ proxa system info
...
gomaxprocs=2
gomaxprocs_source=container_limit
numcpu_host=16
...
```

- `gomaxprocs_source=container_limit` — Go auto-adjusted from a
  cgroup CPU quota.
- `gomaxprocs_source=env_override` — the `GOMAXPROCS` env var is
  explicitly set; the explicit value wins.
- `gomaxprocs_source=host` — no env var, no container limit detected;
  Proxa uses the full host CPU count.

If you're deploying Proxa inside a CPU-limited container and the
source reports `host`, double-check the cgroup setup — Proxa is
probably running with full host scheduling rights, which may cause
noisy-neighbor issues.

## install.sh hosting via GitHub Pages (v0.4.2+)

The canonical operator one-liner is:

```sh
curl -fsSL https://proxa-server.github.io/proxa/install/install.sh | sh
```

`docs/install/install.sh` is a verbatim mirror of the repo-root
`install.sh`, kept in sync by the release workflow (`make
mirror-install` after each release).

### One-time setup (manual, repo admin only)

GitHub Pages cannot be enabled from a workflow — this is a one-time
manual setup. Once done it persists for the life of the repo.

1. Open `https://github.com/proxa-server/proxa/settings/pages`
2. **Source**: `Deploy from a branch`
3. **Branch**: `main`, folder `/docs`
4. Save

Within ~30 seconds the install.sh becomes reachable at the canonical
URL above.

### Verifying the published install.sh

```sh
curl -fsSL https://proxa-server.github.io/proxa/install/install.sh | head -1
# Expected: #!/bin/sh
```

If the URL 404s, the Pages setup step above was probably skipped.

### Future migration to a custom CNAME

If the project picks up a domain like `proxa.sh`, the canonical URL
can shift to `https://get.proxa.sh/install.sh` via a Pages CNAME
record. Until then the GitHub Pages URL is the official answer.

## Coverage gate (`make cover`, v0.4.2+)

`make cover` runs the test suite with `-coverprofile`, generates an
HTML report at `coverage.html`, and runs `cmd/coverage-gate` which
prints a per-package coverage table.

### Reading the output

```
Coverage report (threshold: 60%, 24 packages, 2 allowlisted)

  Package                                                          Coverage
  -------                                                          --------
⚠ github.com/proxa-server/proxa/internal/server                       20.1%
⚠ github.com/proxa-server/proxa/internal/cli                          24.4%
~ github.com/proxa-server/proxa/internal/web                       n/a (allowlisted)
  github.com/proxa-server/proxa/internal/security                  100.0%
```

- `⚠` prefix → below the 60% baseline (action: write tests or add to
  allowlist with a rationale)
- `~` prefix → on the `.coverage-allowlist` (no action; suppressed
  from the warning set)
- no prefix → at-or-above baseline

### Allowlist usage

The `.coverage-allowlist` file at repo root holds packages exempt from
the gate. Comment-friendly format:

```
# Reason: HTML templates — coverage meaningless for embedded assets.
github.com/proxa-server/proxa/internal/web

# Reason: wire-format types only — no executable behavior to cover.
github.com/proxa-server/proxa/pkg/types
```

When adding an entry, include a one-line `# Reason:` comment so future
maintainers know why the package is exempt.

### Reporting-only in v0.4.2

`cmd/coverage-gate` always exits 0 in v0.4.2. The threshold is purely
informational. A future release (likely v0.4.3 or v0.5) flips the
exit code to signal threshold violations, gating CI. That change
requires:

1. Per-package coverage stabilization across releases (no surprise
   drops from refactors).
2. Allowlist seed for genuinely-untestable packages.
3. Workflow change to honor the exit code.

Until then `make cover` is a diagnostic tool, not a gate.

### JSON mode for CI integration

```sh
go run ./cmd/coverage-gate -format=json -threshold=60 \
  -allowlist=.coverage-allowlist coverage.out
```

Suitable for piping to `jq` in CI scripts for trend dashboards.

## Container deployment (v0.4.2+)

Proxa publishes multi-arch container images on every release:

```sh
# control plane
docker run -d --name proxa \
  -v proxa-data:/data \
  -p 8080:8080 -p 80:80 -p 443:443 \
  ghcr.io/proxa-server/proxa:v0.4.2 server

# agent (v0.5+ functionality; v0.4.2 ships the stub image as gate)
docker run -d --name proxa-agent \
  -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/proxa-server/proxa-agent:v0.4.2 connect <control-url>
```

### Image properties

- `FROM scratch` (single static binary, no shell, no libc, no package manager)
- Runs as nonroot user `65532:65532`
- Multi-arch: `linux/amd64` + `linux/arm64`
- OCI labels: `org.opencontainers.image.source/version/revision/licenses=Apache-2.0/title/description/url`
- Size budgets (soft): `proxa` < 80 MB compressed, `proxa-agent` < 40 MB
- `PROXA_DISTRIBUTION=docker` env baked in — System Info reports `"distribution":"docker"`

### Persisted state

The control-plane image mounts `/data` as a volume:

- `/data/proxa.db` — SQLite (services, projects, subjects, policies, events)
- `/data/secrets.key` — age master key (mode 0600)
- `/data/admin-token` — bootstrap admin token (mode 0600)
- `/data/certs/` — CertMagic cache (when ingress TLS=true)

Use a named volume or bind mount for `/data` — otherwise data is
lost on container restart.

## Bench expectations (v0.4.2+)

`make bench` runs the benchmark suite across `bench/` + per-package
benches in reconciler/probe/ingress. Six named metrics:

| Metric | What it measures |
|---|---|
| `services/sec` | Reconciler tick throughput (services-per-second the reconciler can converge under no-op state) |
| `µs/req-p50` + `µs/req-p99` | Ingress L7 routing decision latency (p50 / p99 from BuildRouter + LookupL7) |
| `containers/cycle` | Probe Manager sustained capacity (concurrent containers probed per cycle) |
| `MB/sec` | L4 proxy throughput (loopback TCP echo through io.Copy splice) |
| `lines/sec` | SSE encoder per-connection ceiling (fmt.Fprintf + flush) |
| `MB-rss` | Idle proxa-server RSS — Linux reads `/proc/<pid>/status` VmRSS; macOS uses `runtime.MemStats.Sys` as portable approximation (NOT cross-platform comparable) |

### When to run

- Before opening a PR that touches reconciler / ingress / probe / SSE hot paths
- As part of release sign-off (capture baseline, compare to previous tag)
- In CI on a stable runner class (consistent CPU model required for trend tracking)

### Reading regression signals

A 10-20% drift in any metric is usually noise. >50% drift indicates a
real regression. The bench files use `b.ReportMetric` so output is
machine-parseable with `go test -bench=. -json ./bench/...` for trend
dashboards.

### Updating bench/binary-size-baseline.txt

When a feature intentionally grows the binary (e.g., a new internal
package or a new transitive dep), update the baseline in the SAME
commit:

```sh
make build
stat -f%z bin/proxa > bench/binary-size-baseline.txt   # macOS
# or
stat -c%s bin/proxa > bench/binary-size-baseline.txt   # Linux
```

Then commit the updated baseline. CI's `make build-check` ensures the
baseline tracks intentional growth rather than silently inflating.

## Future supply-chain work (deferred)

The following are intentionally out-of-scope for v0.4.2 and tracked
for a future release:

- **Image signing via cosign keyless** — wait for first supply-chain
  concern from a real user. Workflow integration: add `cosign sign` to
  `.github/workflows/release.yml` after the docker push step.
- **SBOM publishing per release** — generate with `syft packages` and
  attach to the GitHub Release. Useful for enterprise compliance asks.
- **Docker Hub mirror** — GHCR-only is fine for v0.4.x. Mirror to
  Docker Hub when broader operator visibility is needed.
- **Custom CNAME (get.proxa.sh)** for the install.sh URL — operational
  polish, requires domain registration first.

Each can be added without breaking existing operators; the contract
of the release pipeline (binaries + checksums + images + manifests)
stays stable.

---

## Audit log + Events panel (v0.4.3+)

Every reconciler action (`create`, `remove`, replica replace) plus every
service status change is recorded in the SQLite `events` table.

**Where to look:**

- **Dashboard:** `/ui/events` is the full-page audit log viewer.
  Supports a target filter (e.g. `service:default/api`,
  `container:abc123…`) and a result-limit selector (50 / 100 / 250 /
  1000).
- **Dashboard index:** the "Recent events" card on `/ui/` shows the
  last 10 events, polling every 5 seconds.
- **API:** `GET /api/v1/events?target=<>&since=<RFC3339>&limit=<1..1000>`
  returns `{"events":[{id,at,type,actor,target,payload},...]}` as JSON.
  Returns `503` when the server was started without an events store
  wired (unit-test paths; production `proxa server` always wires one).

**Event type vocabulary** (see `internal/events/event.go` for the canonical list):

| Type                       | Emitter      | Triggered by                                         |
|----------------------------|--------------|------------------------------------------------------|
| `reconciler.create`        | reconciler   | a new container was created + started                |
| `reconciler.remove`        | reconciler   | a container was removed (scale-down, replace cleanup) |
| `reconciler.scale`         | reconciler   | a Replace action (rolling restart / spec change)     |
| `service.status_changed`   | reconciler   | aggregated status flipped (e.g. degraded → ready)    |

Future emitters (v0.4.4+): `user.container.{start,stop,restart,remove}`
land with the Container UI; webhook plugins (v0.5+) subscribe to all
event types via the `internal/plugin` bus.

**Retention guidance:**

The events table grows monotonically — there is **no built-in retention
sweep** in v0.4.3 (deferred to v0.5 along with the cluster store). For
high-churn deployments, run a manual purge against the SQLite database
during a maintenance window:

```sh
sqlite3 /var/lib/proxa/proxa.db \
    "DELETE FROM events WHERE ts < strftime('%s','now','-30 days')*1000;"
sqlite3 /var/lib/proxa/proxa.db "VACUUM;"
```

A 30-day window keeps the table small (~10 MB per 100k events) without
losing recent troubleshooting context.

## Host containers (v0.4.4+)

The `/ui/containers` page lists every container the Docker daemon
knows about — Proxa-managed alongside host containers. Operators can
start, stop, restart, and remove **host** containers from the UI.
Proxa-managed containers are read-only with a tooltip explaining
that `proxa scale` (or a TOML edit) is the right tool.

**API:** `GET /api/v1/host-containers` returns the full list as JSON,
each entry tagged with `managed: bool`. Five write endpoints:

```
POST   /api/v1/host-containers/{id}/start
POST   /api/v1/host-containers/{id}/stop
POST   /api/v1/host-containers/{id}/restart
DELETE /api/v1/host-containers/{id}        # ?force=true to remove running
```

Each write that targets a managed container returns `409 Conflict`.

**Audit trail:** every successful write lands a `user.container.<verb>`
event in the v0.4.3 events table, with the actor field hard-coded to
`subject:bootstrap-admin` in v0.4.4. (Multi-user RBAC + per-subject
attribution lands in v0.5.)

**Scope note (token power):**

Any holder of the bootstrap token can start/stop/restart/remove any
host container on the box. That is approximately equivalent to giving
them docker-socket access. v0.4.4 is intended for single-operator
homelab deployments. Multi-operator deployments should wait for v0.5
multi-user RBAC, or restrict token distribution accordingly.

## SQLite migrations (v0.4.3+)

`proxa server` runs schema migrations idempotently on every start.
Migrations are versioned (`schema_version` table) and atomic (each
migration runs inside one transaction). The migration registry lives
in `internal/store/sqlite/migrations.go`; v0.4.3 introduces the events
table as migration v2.

**Operator-visible:**

- A first-time start runs every migration in order. The server logs each
  one at INFO level: `store/sqlite: applied migration vN`.
- A re-start on the same schema is a no-op.
- A migration failure aborts startup (the data file is left at the prior
  schema version — never partially-migrated).
- Downgrade is not supported. To roll back from v0.4.3 → v0.4.2, restore
  a pre-upgrade backup of `proxa.db`; the events table will simply be
  ignored by the older binary.

**Future-proofing for v0.5+ multi-host:**

The single-node `internal/cluster.SingleNode` stub satisfies the
Membership + StateStore + Scheduler interfaces, returning the local node
for every query. v0.5 will swap in an embedded-etcd implementation
without any reconciler-side change. The Snapshot interface in
`internal/state` is similarly stubbed and lands its real implementation
alongside time-travel in v0.6.

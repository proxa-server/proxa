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

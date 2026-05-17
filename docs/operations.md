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

# Quickstart — Ingress

The minimal walkthrough that proves SC-001 through SC-007.

## Pre-requisites

- Linux server with Docker installed and a public IPv4.
- A domain you control with an A-record pointing at the server's IP. Examples below assume `whoami.example.com` and `db.example.com`.
- Port 80 + 443 reachable from the public internet (Let's Encrypt's HTTP-01 challenge needs port 80).
- Proxa binary built from main (`make build`) at `/usr/local/bin/proxa`.

## 1. Bring up the control plane with TLS enabled

```bash
proxa init
cat >> ~/.proxa/config.toml <<'EOF'
[ingress]
http_port  = 80
https_port = 443
tls        = true
email      = "ops@example.com"
EOF
proxa server &
```

On macOS / non-root dev, swap to `http_port = 8080`, `https_port = 8443`, `tls = false` first. SC-008 covers this path.

## 2. Declare a route (SC-001)

```bash
cat > whoami.toml <<'EOF'
name     = "whoami"
image    = "traefik/whoami:latest"
replicas = 1

[[expose]]
container = 80
host      = 0
protocol  = "http"

[[route]]
host = "whoami.example.com"
EOF

proxa up whoami.toml
```

Within ~60 seconds of `proxa up`:

```bash
curl -sI https://whoami.example.com/
# HTTP/2 200
# server: ... whatever whoami returns ...
```

The cert is Let's Encrypt-issued and trusted by the system CA bundle.

The first HTTP request also returns a 301 to https:

```bash
curl -sI http://whoami.example.com/
# HTTP/1.1 301 Moved Permanently
# location: https://whoami.example.com/
```

## 3. Scale and watch the load balance (SC-002)

```bash
sed -i 's/replicas = 1/replicas = 3/' whoami.toml
proxa up whoami.toml
sleep 10

for i in {1..30}; do
  curl -s https://whoami.example.com/ | grep '^Hostname:'
done | sort | uniq -c
# Expected: at least 2 distinct hostnames out of 30 requests
```

## 4. Hot-reload without 5xx (SC-003)

In one terminal:

```bash
while :; do
  curl -sf https://whoami.example.com/ > /dev/null && echo -n "."
  sleep 0.05
done
```

In another terminal, edit the route's `lb_strategy` and re-up:

```bash
sed -i 's/host = "whoami.example.com"/host = "whoami.example.com"\nlb_strategy = "round-robin"/' whoami.toml
proxa up whoami.toml
```

The dots in the first terminal MUST keep flowing. Any blank line or a `curl: (` error fails SC-003.

## 5. TCP forward to Postgres (SC-006)

```bash
cat > db.toml <<'EOF'
name     = "db"
image    = "postgres:16-alpine"
replicas = 1
stateful = true
strategy = "stop-first"

[env]
POSTGRES_PASSWORD = "demo"

[[route]]
host = "db.example.com"
l4   = "tcp"
port = 5432
EOF

proxa up db.toml
sleep 30
PGPASSWORD=demo psql -h db.example.com -U postgres -c "SELECT 1"
# Expected: ?column?
#           ----------
#                   1
```

## 6. macOS dev — probe via ingress (SC-007)

On a macOS Docker Desktop host, the direct-bridge-IP HTTP probe path fails because 172.17.0.x is unreachable from the host. Opt into ingress routing per service:

```toml
[health]
path = "/health"
port = 80
interval = "3s"
retries  = 3
via      = "ingress"        # this line is the new bit
```

`proxa ps` reports `healthy` within ~15 seconds. The previously-skipped e2e tests (SC-002-1, -2, -4, -5 of feature 002) PASS when this flag is set.

## 7. Inspect via the dashboard

`https://<your-server>/?token=<token>` shows two cards in v0.3:

- **Services** — same as v0.2 (status chip per service).
- **Routes** — new in v0.3:

  | Host                | Path | Service | TLS    | Backends |
  |---------------------|------|---------|--------|----------|
  | whoami.example.com  | /    | whoami  | valid  | 3        |
  | db.example.com      | —    | db      | off    | 1        |

The header row shows: `Ingress  80 / 443  TLS:on  1 cert`.

## What success looks like

- `curl https://...` returns 200 with a valid cert (SC-001).
- 30 requests → ≥ 2 backends (SC-002).
- Sustained curl loop survives `proxa up` edit (SC-003).
- `psql` over TCP returns the expected result (SC-006).
- `[health].via = "ingress"` makes macOS probes work (SC-007).
- Dashboard shows real route + TLS state, not dummy data.

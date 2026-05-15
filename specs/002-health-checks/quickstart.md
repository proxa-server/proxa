# Quickstart — Health Checks (v0.2)

After this feature lands, an operator declaring a `[health]` block in their TOML gets real probe-driven status, automatic restart of unhealthy replicas, and zero-downtime stateless upgrades.

## Prerequisites

- Built `bin/proxa` (`make build`)
- Docker daemon running
- Token from `proxa init` (or already-running `proxa server`)

## End-to-end demo: HTTP probe + auto-restart

```sh
export PROXA_DATA_DIR=/tmp/proxa-health
export PROXA_LISTEN=tcp://127.0.0.1:5443
rm -rf $PROXA_DATA_DIR && mkdir -p $PROXA_DATA_DIR

bin/proxa init
nohup bin/proxa server > /tmp/proxa-server.log 2>&1 &

cat > /tmp/web.toml <<'EOF'
name     = "web"
image    = "traefik/whoami:latest"
replicas = 2

[[expose]]
container = 80
host      = 9999
protocol  = "http"

[health]
path     = "/health"     # whoami responds 200 here
interval = "5s"
timeout  = "2s"
retries  = 3
EOF

bin/proxa up /tmp/web.toml
sleep 8
bin/proxa ps
# expected: STATUS=healthy, ACTUAL=2

# simulate a misbehaving replica: kill it inside the container
docker exec proxa-default-web-0 killall -9 whoami 2>&1 || true
# (whoami exits; reconciler picks up via probe failures within ~15s)

sleep 18
bin/proxa ps
# during the rotation: STATUS=degraded
# after the new replica passes its first probe: STATUS=healthy
```

## Demo: zero-downtime stateless upgrade (SC-005)

In one terminal, kick a curl loop that records failures:

```sh
i=0; fails=0
while true; do
  i=$((i+1))
  curl -sf http://localhost:9999/ >/dev/null || fails=$((fails+1))
  sleep 0.5
  printf "\r tries=%d fails=%d" $i $fails
done
```

In another terminal, upgrade the image:

```sh
sed -i '' 's|traefik/whoami:latest|traefik/whoami:v1.10.4|' /tmp/web.toml
PROXA_DATA_DIR=/tmp/proxa-health PROXA_LISTEN=tcp://127.0.0.1:5443 bin/proxa up /tmp/web.toml
```

Watch the curl-loop terminal during the rollover. Expected: `fails` stays at 0 while the new replicas come up and old ones drain. SC-005 verified.

## Demo: stateful upgrade prevents two writers (SC-006)

```sh
cat > /tmp/db.toml <<'EOF'
name     = "db"
image    = "postgres:16-alpine"
replicas = 1
stateful = true
strategy = "stop-first"

[security]
allowRoot = true
capDrop   = ["MKNOD"]   # postgres needs more than ALL-drop

[env]
POSTGRES_PASSWORD = "test"

[[volumes]]
source = "pgdata"
target = "/var/lib/postgresql/data"

[health]
command  = ["pg_isready", "-U", "postgres"]
interval = "5s"
timeout  = "3s"
retries  = 5
EOF

bin/proxa up /tmp/db.toml
sleep 30  # postgres takes a moment to become ready

# Upgrade
sed -i '' 's|postgres:16-alpine|postgres:17-alpine|' /tmp/db.toml
bin/proxa up /tmp/db.toml &

# Snapshot docker ps every second during the rollover; verify
# we never see two "running" containers for proxa-default-db-0.
for i in $(seq 1 60); do
  docker ps --filter name=proxa-default-db-0 --format '{{.Names}}\t{{.Status}}' >> /tmp/db-snapshots.log
  sleep 1
done

# In /tmp/db-snapshots.log, no row should ever show two "Up" lines
# for proxa-default-db-0 simultaneously.
```

## Verifying behavior

| Behavior | How to check |
|---|---|
| Probe goroutines exit on container removal | `bin/proxa down web && sleep 6 && grep "probe stopped" /tmp/proxa-server.log` |
| Probe respects timeout | Deploy a slow `/healthz` (sleep 5 in handler); set `timeout = "1s"`; verify timeout-driven failures in logs |
| `proxa ps` reflects degraded state | Mid-rotation, status=degraded; check `bin/proxa ps -o json` for the raw value |
| Dashboard chip turns amber | Open `/ui/?token=...`; degraded service shows `chip-amber` |
| Service with no `[health]` block | Behaves identically to v0.1.0 — `State=running` containers count as healthy. No regressions. |

## Cleanup

```sh
docker ps -aq --filter label=proxa.managed=true | xargs -r docker rm -f
kill $(cat /tmp/proxa-server.pid 2>/dev/null) 2>/dev/null
rm -rf /tmp/proxa-health /tmp/web.toml /tmp/db.toml /tmp/db-snapshots.log
```

## Out of scope (deferred to later features)

- TCP / gRPC probes (current spec only HTTP + exec)
- Per-replica startup grace period (use `retries × interval` for now)
- Auto-rollback retry backoff (each tick retries; broken images loop forever)
- Probe credentials / mTLS
- Per-probe metrics dashboard

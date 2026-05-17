# Quickstart — Logs

The minimal walkthrough that proves SC-001 through SC-007.

## Pre-requisites

- A running `proxa server` (see operations.md for setup).
- A deployed service producing some log output. We'll use whoami because it logs each HTTP request to stderr.

## 1. Deploy a noisy service

```bash
cat > whoami.toml <<'EOF'
name = "whoami"
image = "traefik/whoami:latest"
replicas = 1

[[expose]]
container = 80
host = 18091
protocol = "http"

[[route]]
host = "whoami.local"
EOF

proxa up whoami.toml
sleep 5
# Generate some log output:
for i in {1..10}; do curl -s http://127.0.0.1:18091/ > /dev/null; done
```

## 2. Tail the last 5 lines (SC-001)

```bash
proxa logs whoami --tail 5
# === proxa-default-whoami-0 (replica 0) ===
# Starting up on port 80
# 127.0.0.1 - "GET / HTTP/1.1" 200
# 127.0.0.1 - "GET / HTTP/1.1" 200
# ...
```

## 3. Follow live (SC-002)

In one terminal:

```bash
proxa logs whoami -f
```

In another terminal:

```bash
curl http://127.0.0.1:18091/
```

The `proxa logs -f` terminal shows the new request line within 1 second.

Ctrl-C exits cleanly (exit 130). `lsof -p <previous-pid>` would have shown one open ESTABLISHED connection to the proxa server socket; after exit, no leftover.

## 4. Filter by time (SC-004)

```bash
# Hit the service to produce known-recent lines:
curl -s http://127.0.0.1:18091/ > /dev/null
sleep 3
curl -s http://127.0.0.1:18091/ > /dev/null

# Now ask for the last 2 seconds — should see only the most recent line:
proxa logs whoami --since 2s
```

## 5. Pick a specific replica (SC-005)

Scale up first:

```bash
sed -i '' 's/replicas = 1/replicas = 3/' whoami.toml
proxa up whoami.toml
sleep 10
proxa ps   # should show 3/3 healthy

proxa logs whoami --replica 2 --tail 10
# === proxa-default-whoami-2 (replica 2) ===
# ...lines from replica 2 only...
```

Bad replica index:

```bash
proxa logs whoami --replica 99
# error: replica 99 not found (service has 3 replicas)
# exit code 1
```

## 6. View in the dashboard (SC-003, SC-007)

Open the dashboard (e.g., `https://proxa.example.com/?token=$TOKEN`).

- The Services card shows a small "logs" icon (📜 or similar) on each row.
- Click the icon for `whoami` → navigates to `/ui/logs/default/whoami`.
- The full-page viewer shows the last lines from replica 0 within 2 seconds.
- A dropdown lets you switch replicas (0 / 1 / 2).
- A "follow" toggle starts / stops live streaming.
- The viewer auto-scrolls to the bottom; if you scroll up to read older lines, auto-scroll pauses until you scroll back down.

In another terminal, trigger the service:

```bash
curl http://127.0.0.1:18091/
```

The dashboard log viewer shows the new request line within 1 second.

## 7. Cross-project authorization (SC-006)

Assuming you have a bearer token for project `other`:

```bash
curl -s -H "Authorization: Bearer $OTHER_TOKEN" \
  https://proxa.example.com/api/v1/projects/default/services/whoami/logs
# {"error":"unauthorized for project \"default\"","code":"logs-cross-project"}
# HTTP 403
```

## 8. Cleanup

```bash
proxa down whoami
docker rm -f proxa-default-whoami-0 proxa-default-whoami-1 proxa-default-whoami-2 || true
```

## What success looks like

- `proxa logs <svc>` shows recent lines and exits 0 (SC-001).
- `proxa logs -f` streams new lines within 1s and exits cleanly on Ctrl-C (SC-002, SC-003).
- The dashboard log viewer opens within 2s of clicking, streams live, and auto-scrolls smartly (SC-004, SC-007).
- Cross-project requests get 403 (SC-006).
- Multi-replica services route correctly to the requested replica (SC-005).

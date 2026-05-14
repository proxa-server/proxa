# Quickstart — Core Loop

After this feature lands, an operator can deploy and reconcile a containerized service end-to-end on a single node. This document is the validation checklist that the final task (`/speckit.implement`'s polish phase) walks through.

## Prerequisites

- A built `proxa` binary (`make build` produces `bin/proxa`).
- Docker Engine 24+ running locally; user is in the `docker` group.
- An empty data directory (or set `PROXA_DATA_DIR=/tmp/proxa-test` for the walkthrough).

## End-to-end flow

### 1. Initialize

```sh
proxa init
```

Expected output (paths/values redacted):

```
created  ~/.proxa/
created  ~/.proxa/proxa.db (SQLite, schema v1)
created  ~/.proxa/secrets.key (mode 0600)
created  admin user 'admin' (provider=local; password printed below — STORE IT)
   admin password: <one-time printed string>
created  bootstrap token (saved to ~/.proxa/token, mode 0600)
   bootstrap token: <43-char base64url string>

Run `proxa server` in a separate terminal (or under systemd) to start the
control plane, then use `proxa up <file>` from any other terminal.
```

Verifications:

- `[ -f ~/.proxa/proxa.db ]` ✓
- `stat -c %a ~/.proxa/secrets.key` returns `600` ✓
- `stat -c %a ~/.proxa/token` returns `600` ✓
- `sqlite3 ~/.proxa/proxa.db "SELECT name FROM projects"` includes `default` ✓

### 2. Start the daemon

In a second terminal:

```sh
proxa server
```

Expected log output (JSON to stderr):

```json
{"time":"...","level":"INFO","msg":"server starting","listen":"unix:///home/.../proxa.sock"}
{"time":"...","level":"INFO","msg":"reconciler started","tickInterval":"5s"}
```

The process blocks. SIGINT shuts down cleanly (server.Shutdown + reconciler ctx cancel).

### 3. Define and deploy a service

In the original terminal, write a TOML file:

```sh
cat > /tmp/web.toml <<'EOF'
name     = "web"
image    = "nginx:alpine"
replicas = 3

[[expose]]
container = 80
host      = 0
protocol  = "http"
EOF

proxa up /tmp/web.toml
```

Expected output:

```
upserted service "default/web" (3 replicas requested)
reconciler will converge within ~5s
```

Within 5-10 seconds (one tick), three containers exist:

```sh
docker ps --filter label=proxa.managed=true --format '{{.Names}}\t{{.Image}}\t{{.Status}}'
```

```
proxa-default-web-0    nginx:alpine    Up 3 seconds
proxa-default-web-1    nginx:alpine    Up 3 seconds
proxa-default-web-2    nginx:alpine    Up 3 seconds
```

### 4. Verify security defaults applied (SC-001)

```sh
docker inspect proxa-default-web-0 \
  --format '{{json .HostConfig.CapDrop}} {{.HostConfig.SecurityOpt}} {{.Config.User}}'
```

Expected (constitution §II enforced):

```
["ALL"] [no-new-privileges:true] 1000:1000     # or whatever security.user resolves to
```

If `User` is empty, the runtime picked a non-root UID at container start; verify with `docker exec proxa-default-web-0 id -u` returns non-zero.

### 5. Verify reconciler restarts a killed container (SC-002)

```sh
docker kill proxa-default-web-1
sleep 11
docker ps --filter name=proxa-default-web-1 --format '{{.Names}}\t{{.Status}}'
```

Expected: a new container with the same name, `Up X seconds` where X < 10.

### 6. Scale up by editing the TOML (SC-003)

```sh
sed -i 's/replicas = 3/replicas = 5/' /tmp/web.toml
proxa up /tmp/web.toml
sleep 6
docker ps --filter label=proxa.service=web --format '{{.Names}}' | wc -l
```

Expected: `5`.

### 7. Tear down (SC-004)

```sh
proxa down web
sleep 6
docker ps --filter label=proxa.service=web --format '{{.Names}}' | wc -l
```

Expected: `0`.

### 8. List view (SC-005)

```sh
proxa ps
```

Expected (formatted table to stdout):

```
PROJECT   SERVICE   IMAGE          DESIRED   ACTUAL   STATUS
default   web       nginx:alpine   0         0        healthy
```

(`replicas=0` services remain visible until `DELETE /api/v1/.../services/web` is called; Feature 003 may add a `--clean` filter.)

### 9. Multi-project isolation (SC-006)

```sh
cat > /tmp/web-socio.toml <<'EOF'
project  = "socio-do"
name     = "web"
image    = "nginx:alpine"
replicas = 1
[[expose]]
container = 80
host      = 0
protocol  = "http"
EOF

cat > /tmp/web-kut.toml <<'EOF'
project  = "kut-do"
name     = "web"
image    = "nginx:alpine"
replicas = 1
[[expose]]
container = 80
host      = 0
protocol  = "http"
EOF

proxa up /tmp/web-socio.toml
proxa up /tmp/web-kut.toml
sleep 6
docker ps --filter label=proxa.managed=true \
  --format '{{.Names}}' | grep web | sort
```

Expected:

```
proxa-kut-do-web-0
proxa-socio-do-web-0
```

Two services named `web` coexist because their projects differ.

### 10. Clarify error when daemon not running (SC-007)

In a fresh shell (no `proxa server` running):

```sh
proxa up /tmp/web.toml
```

Expected stderr:

```
error: cannot reach proxa server at unix:///home/.../proxa.sock
hint:  start it with `proxa server` or set PROXA_URL to a TCP listener.
exit status 1
```

### 11. Clean shutdown

In the daemon terminal, Ctrl-C. Expected log:

```
{"time":"...","level":"INFO","msg":"shutdown signal received"}
{"time":"...","level":"INFO","msg":"reconciler stopped"}
{"time":"...","level":"INFO","msg":"server stopped","duration":"..."}
```

Containers keep running (`docker ps` still shows them). The next `proxa server` start picks them up via the labels and resumes reconciliation.

## Success Criteria — verifier mapping

| SC | Verified by |
|---|---|
| SC-001 (security defaults) | Step 4 |
| SC-002 (restart in 10s) | Step 5 |
| SC-003 (scale via TOML edit) | Step 6 |
| SC-004 (proxa down) | Step 7 |
| SC-005 (proxa ps) | Step 8 |
| SC-006 (project isolation) | Step 9 |
| SC-007 (clear error w/o init) | Step 10 |

## What's intentionally NOT validated here

- Health check probes (Feature 002).
- Ingress / TLS (Feature 003).
- Dashboard (Feature 004).
- Secrets resolution (Feature 005).
- Multi-node behavior (v1.0).
- Performance under sustained load (a separate feature, post-v0.0).

## Cleanup after the walkthrough

```sh
proxa down web                                       # if not already done
docker ps -aq --filter label=proxa.managed=true | xargs -r docker rm -f
rm -rf ~/.proxa                                       # nukes the data dir
```

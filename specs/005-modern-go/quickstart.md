# Quickstart — v0.4.1 Modern Go

Walkthroughs to validate the spec's user stories on a real installation. Each section maps to one or more user stories and success criteria.

## Prerequisites

- Proxa v0.4.0 already installed and running (existing deployment), OR a fresh v0.4.1 binary.
- A Docker daemon reachable (Colima / Rancher Desktop / Docker Desktop / Docker Engine on Linux).
- `curl` available for raw API checks.

## 1. Upgrade from v0.4.0 → v0.4.1 (validates FR-015, SC-009)

```sh
# Stop the v0.4.0 binary gracefully — DO NOT kill -9.
proxa server stop  # or systemctl stop proxa

# Replace the binary in place.
sudo install -m 0755 /path/to/proxa-v0.4.1 /usr/local/bin/proxa

# Start v0.4.1 — same config dir, same data dir.
proxa server start  # or systemctl start proxa

# Verify the existing v0.4.0 bearer token still authenticates.
PROXA_TOKEN=$(cat ~/.config/proxa/admin-token)
curl -H "Authorization: Bearer $PROXA_TOKEN" http://localhost:8080/api/v1/services
# Expected: 200 with your existing services. NOT 401.
```

**Pass criteria**: the API request succeeds without re-issuing a token. No re-init step.

## 2. Verify the modernization landed (validates US3, FR-007, SC-005)

```sh
# CLI mode:
proxa system info
# Expected output:
#   proxa_version=v0.4.1
#   go_version=go1.26.0
#   commit=<hash>
#   build_date=2026-05-19T...
#   go_experiments=
#   gomaxprocs=<N>
#   gomaxprocs_source=host         # or container_limit or env_override
#   numcpu_host=<N>

# JSON mode:
proxa system info --json | jq .

# Dashboard:
open http://localhost:8080/ui/
# Scroll to the System Info card at the bottom of the page.
# Click through to /ui/system for the focused full-page view.
```

**Pass criteria**: System Info card visible on `/ui/`, full page renders at `/ui/system`, CLI returns matching values within 10 seconds.

## 3. Verify container-aware GOMAXPROCS (validates SC-005 second clause)

Only meaningful when Proxa runs inside a CPU-limited container.

```sh
# Run Proxa inside a container with --cpus=2 on a host with more cores:
docker run --rm --cpus=2 -p 8080:8080 -v $(pwd)/data:/data proxa:v0.4.1 server
# In another shell:
PROXA_TOKEN=$(docker exec ... cat /data/admin-token)
curl -H "Authorization: Bearer $PROXA_TOKEN" http://localhost:8080/api/v1/system | jq .gomaxprocs_source
# Expected: "container_limit"
curl ... | jq .gomaxprocs
# Expected: 2 (not the host's full core count)
```

```sh
# Run Proxa with explicit env var:
GOMAXPROCS=4 proxa server
# In another shell:
proxa system info --json | jq .gomaxprocs_source
# Expected: "env_override"
```

**Pass criteria**: `gomaxprocs_source` distinguishes the three modes correctly; `gomaxprocs` reports the effective value.

## 4. Verify the TLS+probe fix (validates US1, FR-005, SC-001)

This is the regression test for the 0.4.0 demo bug.

```sh
# Create a TOML that exposes nginx through TLS-enabled ingress AND has an HTTP probe:
cat > tlsapi.toml <<'EOF'
name     = "tlsapi"
image    = "nginxinc/nginx-unprivileged:alpine-slim"
replicas = 1

[security]
user = "101:101"

[[expose]]
container = 8080
host      = 0
protocol  = "http"

[[route]]
host = "tlsapi.local"

[health.http]
path = "/"
port = 8080
interval = "5s"
timeout = "3s"
EOF

# Enable ingress TLS in proxa config:
# [ingress]
# tls = true

proxa up tlsapi.toml

# Wait up to 30 seconds and check status:
proxa status tlsapi
# Expected: status=healthy, no probe failures.
```

**Pass criteria** (SC-001): service reaches `healthy` within 30 seconds. No manual workaround required (no need to delete the `[health.http]` section).

**Failure expected before this release**: in v0.4.0 the probe would hit the HTTP→HTTPS redirect, fail cert verification on the redirected request, and mark the service unhealthy indefinitely. The 0.4.0 demo workaround was to remove `[health.http]` from the TOML — a documented loss of probe coverage.

## 5. Verify security hardening (validates US2, FR-001..003, SC-002..004)

```sh
# (a) Path-traversal refusal — exercised by unit tests; verify with:
go test -v ./internal/datadir/...
# Expected: TestRoot_PathTraversalRefusal passes.

# (b) Token entropy — inspect any newly-generated token:
proxa init  # in a fresh empty data dir
TOKEN=$(cat ~/.config/proxa/admin-token)
echo "${#TOKEN}"
# Expected: token length >= 22 chars (base64 of 16+ bytes = >=128 bits entropy).
# Sanity: the token MUST NOT contain obvious patterns (timestamp, sequential, etc.).

# (c) Cross-origin protection — issue a state-changing request from a foreign origin:
curl -X POST http://localhost:8080/api/v1/services/dummy \
  -H "Origin: http://evil.example.com" \
  -H "Authorization: Bearer $PROXA_TOKEN"
# Expected: 403 forbidden (rejected by CrossOriginProtection middleware
#           BEFORE the handler runs, even though /api/v1/services/dummy
#           POST doesn't exist as a real endpoint in v0.4.1).
```

**Pass criteria** (SC-002, SC-003, SC-004): all three checks behave as expected.

## 6. Verify the new datadir snapshot helper (validates FR-004)

The helper is library-only in v0.4.1 (not wired to a CLI). Validate via unit test:

```sh
go test -v ./internal/datadir/... -run Snapshot
# Expected: TestSnapshot_HappyPath, TestSnapshot_DstExists, TestSnapshot_Atomicity pass.
```

**Pass criteria**: the snapshot helper round-trips a tree of files correctly and refuses to overwrite an existing destination.

## 7. Verify the tool directive (validates US5, SC-006)

On a fresh clone:

```sh
git clone https://github.com/proxa-server/proxa.git
cd proxa
git checkout v0.4.1
make lint
# Expected: staticcheck runs and reports no warnings. NO `go install` or
# `brew install` step required first.
```

**Pass criteria**: `make lint` runs end-to-end on a fresh clone with only the Go toolchain installed.

## 8. Verify the decision records exist (validates spec's out-of-scope contract)

```sh
ls docs/decisions/
# Expected:
#   0004-router.md
#   0005-deferred-modernizers.md

cat docs/decisions/0004-router.md
# Expected: documents the chi → stdlib router defer to v1.0 with rationale.

cat docs/decisions/0005-deferred-modernizers.md
# Expected: lists any go-fix modernizers NOT taken in this release with rationale.
```

**Pass criteria**: both decision records exist and contain the documented rationale.

## 9. Verify the binary-size budget (validates SC-008)

```sh
ls -la /usr/local/bin/proxa-v0.4.0 /usr/local/bin/proxa-v0.4.1
# Compute the size delta as a percentage:
#   (size_v0.4.1 - size_v0.4.0) / size_v0.4.0 * 100
# Expected: within ±2%.
```

**Pass criteria** (SC-008): the new binary is within ±2% of the v0.4.0 binary's size on the same target.

## 10. Verify the deprecation-clean baseline (validates US4, SC-007)

```sh
make lint
# Expected: zero deprecation warnings reported.

grep -rn "\"math/rand\"" internal/ --include="*.go" | grep -v _test.go
# Expected: zero results.

grep -rn "runtime.SetFinalizer" internal/ --include="*.go" | grep -v _test.go
# Expected: zero results.
```

**Pass criteria**: all three checks return clean.

## When this quickstart fails

- **Section 1 fails (token rejected)**: the on-disk token format changed — bug, file an issue. FR-015 violation.
- **Section 2 fails (no System Info card)**: dashboard template not deployed, or `/api/v1/system` not mounted. Re-check the build.
- **Section 4 fails (probe still marks unhealthy)**: the via-ingress + TLS fix didn't land or the test setup is missing TLS enablement.
- **Section 5(c) fails (CrossOriginProtection allows the foreign request)**: middleware not mounted at the right level, or short-circuited by another middleware. Re-check `internal/server/server.go`.
- **Section 7 fails (`make lint` needs install)**: `tool` directive not added to `go.mod`, or `Makefile` still uses `go run` instead of `go tool`.

# Quickstart — v0.4.2 Test Foundation + Public Images

Validation walk-throughs for every FR + SC. Each section maps to one or more spec items.

## Prerequisites

- Proxa source checkout at v0.4.2
- Go 1.26.x
- Docker daemon (Colima / Rancher Desktop / Docker Desktop) — required for image + e2e tests
- `curl` (for install.sh validation)
- ~10 GB free disk for full validation (images + benchmarks + e2e workdirs)

## 1. Upgrade from v0.4.1 → v0.4.2 (validates FR-017, SC-009)

Per the same pattern as v0.4.1's upgrade smoke:

```sh
# With a v0.4.1 binary running:
proxa server stop  # or systemctl stop

# Replace binary in place
sudo install -m 0755 /path/to/proxa-v0.4.2 /usr/local/bin/proxa

# Start v0.4.2
proxa server start

# Verify existing token still authenticates
PROXA_TOKEN=$(cat ~/.config/proxa/admin-token)
curl -H "Authorization: Bearer $PROXA_TOKEN" http://localhost:8080/api/v1/services
# Expected: 200 with existing services
```

**Pass criteria**: existing token authenticates without re-issuance; existing services still tracked.

## 2. Run the benchmark suite (validates US3, FR-001, SC-004)

```sh
make bench
```

Expected runtime: 3-5 minutes. Output includes named metrics:

```
BenchmarkReconciler_TickThroughput      120     8423105 ns/op    118.7 services/sec
BenchmarkIngress_L7Latency_p50          2000    540123 ns/op     540 µs/req-p50
BenchmarkIngress_L7Latency_p99          2000    540123 ns/op    2100 µs/req-p99
BenchmarkProbe_WaveCapacity              30   38421253 ns/op     260 containers/cycle
BenchmarkL4_Throughput                   10  202841523 ns/op    1245 MB/sec
BenchmarkSSE_Throughput                 500    2003841 ns/op    498000 lines/sec
BenchmarkIdleMemoryRSS_Linux              1 10000000000 ns/op     52.3 MB-rss
```

**Pass criteria**: all 6 named benchmarks emit non-zero metrics; total runtime < 5 minutes.

## 3. Install via the canonical one-liner (validates US1, FR-009, SC-001)

On a fresh Ubuntu 22.04 amd64 VM (or `docker run --rm -it ubuntu:22.04 sh`):

```sh
apt-get update && apt-get install -y curl
curl -fsSL https://proxa-server.github.io/proxa/install/install.sh | sh

# Expected output ends with:
#   >>> installed: proxa v0.4.2 (commit <sha>, built <date>)
#   >>> done
#   Suggested systemd unit (copy to /etc/systemd/system/proxa.service):
#   [unit text]

proxa version
# Expected: v0.4.2 + commit + build date
```

Repeat on Debian 12 arm64 (or `docker run --rm --platform linux/arm64 -it debian:12 sh`).

**Pass criteria**: install completes in < 60s; binary at `/usr/local/bin/proxa`; `proxa version` returns expected version.

## 4. Pull and run the GHCR image (validates US2, FR-008, SC-002, SC-003)

```sh
# On amd64 Linux (or Mac via Docker Desktop / Colima):
docker run --rm -d --name proxa-test \
  -v proxa-test-data:/data \
  -p 8080:8080 \
  ghcr.io/proxa-server/proxa:v0.4.2 server

# Verify it's responding
curl http://localhost:8080/api/v1/system | jq .
# Expected: SystemInfo JSON with distribution="docker"

# Cleanup
docker stop proxa-test && docker rm proxa-test
docker volume rm proxa-test-data
```

Same command on arm64 (Apple Silicon, Graviton, RPi):
```sh
docker run --rm --platform linux/arm64 ghcr.io/proxa-server/proxa:v0.4.2 version
```

Agent image smoke (forward-compat for v0.5):
```sh
docker run --rm ghcr.io/proxa-server/proxa-agent:v0.4.2 version
# Expected: agent stub version (only `version` subcommand works in v0.4.2)
```

**Pass criteria**: image pulls + boots on both architectures; `distribution=docker` shows in SystemInfo; agent image is multi-arch + responds to version.

## 5. Run `make cover` + understand allowlist (validates US5, FR-006, SC-006)

```sh
make cover
```

Expected output: per-package coverage table sorted ascending. Packages below 60% prefixed with `⚠`. Allowlisted packages (web, types) prefixed with `~`. HTML report at `coverage.html`.

To exclude a package from the gate:

```sh
echo "github.com/proxa-server/proxa/internal/some-thin-wrapper" >> .coverage-allowlist
make cover
# Now that package appears with ~ prefix instead of ⚠
```

**Pass criteria**: every package appears in output with a numeric % or `n/a (allowlisted)`; `make cover` always exits 0 in v0.4.2.

## 6. Verify Distribution field in dashboard + CLI (validates FR-014, SC-010)

CLI:
```sh
proxa system info
# Expected output includes:
#   distribution=binary    (if running native binary)
#   distribution=docker    (if running inside container)

proxa system info --json | jq .distribution
# Expected: "binary" or "docker"
```

Dashboard:
```sh
open http://localhost:8080/ui/
# Scroll to footer card — Distribution shown alongside Proxa version + Go version + GOMAXPROCS

open http://localhost:8080/ui/system
# Full table — Distribution row visible
```

**Pass criteria**: CLI + footer card + full /ui/system page all show the same distribution value; binary install shows "binary"; container install shows "docker".

## 7. Verify zero remaining `time.Sleep` in synctest-migrated tests (validates US4, FR-002, SC-005)

```sh
# Pre-check: verify the targeted files no longer have time.Sleep
grep -n "time.Sleep" \
  internal/reconciler/reconciler_test.go \
  internal/probe/manager_test.go
# Expected: zero output (sleeps migrated to synctest)

# http_test.go still has 2 time.Sleep calls (inside httptest handlers; documented limitation)
grep -c "time.Sleep" internal/probe/http_test.go
# Expected: 2 (NOT 0 — these are inside httptest.Server handlers, not migratable)

# Wall-time measurement
time go test -count=1 -race ./internal/reconciler/... ./internal/probe/...
# Compare to v0.4.1 baseline (recorded in tasks.md during impl)
# Target: ≥50% faster on the synctest-migrated tests
```

**Pass criteria**: zero `time.Sleep` in reconciler_test.go + manager_test.go; 2 remaining in http_test.go (documented); test wall time measurably faster.

## 8. Release pipeline dry-run (validates FR-012, SC-011)

```sh
make release-dry
# Equivalent to:
go tool goreleaser release --snapshot --skip=publish --clean

# Verify outputs in dist/
ls dist/
# Expected:
#   proxa_*_linux_amd64.tar.gz
#   proxa_*_linux_arm64.tar.gz
#   proxa_*_darwin_amd64.tar.gz
#   proxa_*_darwin_arm64.tar.gz
#   proxa-agent_*_* (same matrix)
#   checksums.txt
#   install.sh
#   docker images locally tagged as ghcr.io/proxa-server/proxa:*-amd64 / *-arm64

docker image ls | grep ghcr.io/proxa-server
# Expected: 4 entries (2 binaries × 2 arches)

docker run --rm ghcr.io/proxa-server/proxa:<snapshot>-amd64 version
# Expected: snapshot version reported
```

**Pass criteria**: all artifacts produced; no errors; local Docker images bootable.

## 9. End-to-end test suite (full validation)

```sh
make test         # ~30s unit + race
make lint         # ~10s vet + staticcheck
make test-integ   # ~1-2 min (requires Docker)
make test-e2e     # ~5-10 min (requires Docker; includes install.sh smoke + docker image smoke + bench smoke)
make bench        # ~3-5 min (benchmarks, not part of e2e but useful pre-release)
make cover        # ~45s
```

**Pass criteria**: every target exits 0; no flakes across 3 consecutive runs of `make test` + `make test-integ`.

## 10. Binary size sanity (validates FR-016, SC-012)

```sh
make build
ls -la bin/proxa | awk '{print $5}'   # bytes
cat bench/binary-size-baseline.txt    # baseline bytes

# Compute delta:
# (current_size - baseline_size) / baseline_size * 100
# Expected: within ±2%
```

**Pass criteria**: binary size within ±2% of baseline (if not, baseline file is updated in the SAME commit with the size-changing feature, NOT silently).

## When this quickstart fails

- **Section 3 fails (install.sh refuses)**: probably running on Alpine/musl or BSD; that's the correct refusal behavior. Use manual download.
- **Section 4 fails (image pull denied)**: GHCR package may not be public yet — check `gh api repos/proxa-server/proxa/packages/container/proxa` and run the publicization step.
- **Section 6 fails (distribution="unknown")**: `PROXA_DISTRIBUTION` env var not set in Dockerfile AND `/.dockerenv` not present — re-check the image Dockerfile.
- **Section 7 fails (sleeps remain)**: synctest migration incomplete; check the 4 specific call sites locked in research R-004.
- **Section 8 fails (Goreleaser dry-run errors)**: probably a Dockerfile syntax issue or missing buildx. Run `docker buildx ls` to verify buildx is available.
- **Section 10 fails (size out of budget)**: NEW feature added bytes — that's fine, but update `bench/binary-size-baseline.txt` in the same commit and explain in the commit message why size grew.

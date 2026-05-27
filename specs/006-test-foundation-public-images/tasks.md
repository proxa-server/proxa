---

description: "Task list for 006-test-foundation-public-images (v0.4.2 Test Foundation + Public Images)"
---

# Tasks: Test Foundation + Public Images (v0.4.2)

**Input**: Design documents from `/specs/006-test-foundation-public-images/`
**Prerequisites**: spec.md, plan.md, research.md, data-model.md, contracts/{install-sh,release-pipeline,dockerfiles,make-targets,coverage-gate}.md, quickstart.md
**Tests**: REQUESTED — unit (coverage-gate, version detector, synctest migrations) + e2e (install.sh, docker image, bench smoke)
**Organization**: Tasks grouped by user story to enable independent verification

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: Maps task to a user story for traceability
- Each task includes exact file paths
- Each task's commit message is specified verbatim (Constitution §XI)

## Path Conventions

Single Go module rooted at the repo. `bench/` is a NEW top-level package for performance benchmarks. `tests/e2e/internal/harness/` is a NEW package consolidating e2e test helpers. `cmd/coverage-gate/` is a NEW dev tool. Other changes target existing packages.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Create the directory scaffolding for the two new packages so subsequent tasks land in a stable file layout.

- [X] T001 Create `bench/` package skeleton — add `bench/doc.go` with package godoc explaining the benchmark categories (reconciler / ingress / probe / L4 / SSE / idle memory) + `bench/binary-size-baseline.txt` containing the single integer `27450050` (v0.4.1 binary size recorded 2026-05-21). Verify `go build ./bench/` succeeds with an empty test-only package.
  - **Commit**: `chore(bench): scaffold benchmark package + record v0.4.1 binary-size baseline`

- [X] T002 [P] Create `tests/e2e/internal/harness/` package skeleton — add `tests/e2e/internal/harness/doc.go` with package godoc summarizing what helpers live here. No implementation yet (real consolidation in T025). Verify `go build -tags=e2e ./tests/e2e/internal/harness/` succeeds.
  - **Commit**: `chore(tests/e2e): scaffold internal/harness package`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Land the cross-cutting infrastructure — Goreleaser tool directive, `cmd/coverage-gate` Go binary, coverage allowlist file, and the Makefile target matrix. ALL user-story work depends on these being in place.

**CRITICAL**: No US task may begin until T003-T007 are complete, `make help` lists all new targets, `make cover` runs `cmd/coverage-gate` end-to-end, and existing tests still pass.

- [X] T003 Add Goreleaser via Go 1.24 tool directive — run `go get -tool github.com/goreleaser/goreleaser/v2/cmd/goreleaser@latest` (verify resulting version pinned in `go.mod`); verify `go tool goreleaser -v` works; verify `go mod tidy` round-trip stable (tool directive line added, transitive go.sum entries added, **NOT** linked into proxa binary). License verified MIT per [research.md R-002](./research.md).
  - **Commit**: `chore(deps): pin goreleaser via go.mod tool directive`

- [X] T004 Implement `cmd/coverage-gate/main.go` (~80 LOC) per [contracts/coverage-gate.md](./contracts/coverage-gate.md) — flags: `-threshold N` (default 60), `-allowlist FILE` (default `.coverage-allowlist`), `-format FMT` (default `text`). Parse `coverage.out` (positional arg), compute per-package %, sort ascending, print table (text mode) or JSON (json mode), mark below-threshold with `⚠`, allowlisted with `~`. Always exit 0 in v0.4.2 (reporting-only). Missing allowlist file is NOT an error.
  - **Commit**: `feat(coverage-gate): implement per-package coverage reporter with allowlist support`

- [X] T005 Unit test `cmd/coverage-gate/main_test.go` covering the 9 cases in [contracts/coverage-gate.md test coverage section](./contracts/coverage-gate.md): TestParseCoverageOut_HappyPath, TestParseCoverageOut_Empty, TestParseCoverageOut_Malformed, TestAllowlist_LoadsAndIgnoresComments, TestAllowlist_MissingFileIsNotAnError, TestThreshold_HighlightsBelow, TestThreshold_AllowlistedNotHighlighted, TestOutput_JSONMode, TestExit_AlwaysZeroInV042. Use fixture files in `cmd/coverage-gate/testdata/`.
  - **Commit**: `test(coverage-gate): cover parser, allowlist, threshold highlighting, JSON mode`

- [X] T006 Create `.coverage-allowlist` at repo root with comment-friendly format and initial seeds per [data-model.md](./data-model.md): `internal/web` (HTML templates, coverage meaningless), `pkg/types` (wire-format types, no behavior to test). Include header comment explaining the rationale.
  - **Commit**: `chore(coverage): seed .coverage-allowlist with web templates and pkg/types`

- [X] T007 Update `Makefile` with the full target matrix per [contracts/make-targets.md](./contracts/make-targets.md). New / changed targets: `test-quick` (no race), `bench` (`-bench=. -benchmem -run=^$ -count=3 ./bench/...`), `cover` (test -coverprofile + tool cover html + run `./cmd/coverage-gate`), `release-dry` (`go tool goreleaser release --snapshot --skip=publish --clean`), `release` (refuses unless on a `vX.Y.Z` tag — uses `git describe --exact-match --tags HEAD`), `build-check` (build + size comparison to `bench/binary-size-baseline.txt` within ±2%). Existing `test`, `test-integ`, `test-e2e`, `lint`, `tidy`, `clean`, `help` preserved.
  - **Commit**: `chore(make): formalize test/bench/cover/release target matrix`

**🛑 CHECKPOINT — PAUSE HERE**: After T007, run `make help` (verify all targets), `make cover` (verify coverage-gate runs end-to-end), `go mod tidy && git diff go.mod go.sum` (expect only the tool-directive change for goreleaser + its transitive entries — production deps unchanged), `make test` (verify existing tests still green). Report results to the user. Wait for explicit green-light before starting Phase 3.

---

## Phase 3: User Story 1 — Install via one-line script (Priority: P1)

**Goal**: An operator on a fresh Linux VM runs `curl <pages-url>/install.sh | sh` and gets a working Proxa binary in `/usr/local/bin/proxa` within 60 seconds. Refuses on unsupported platforms with a clear message.

**Independent Test**: Run `tests/e2e/install_sh_test.go:TestInstallSh_HappyPath_LinuxAmd64` against real Docker — binary installed, version reports correctly, suggested systemd unit printed.

**Maps to**: spec FR-009, FR-010, FR-011, SC-001, SC-009; contract [contracts/install-sh.md](./contracts/install-sh.md).

- [X] T008 [US1] Create `install.sh` at repo root per [contracts/install-sh.md](./contracts/install-sh.md). POSIX shell (tested with `dash`); env vars `INSTALL_VERSION` (default `latest`), `INSTALL_DIR` (default `/usr/local/bin`), `INSTALL_VERIFY` (default `1`); exit codes 0-5 per contract; platform matrix Linux glibc + macOS yes / Alpine musl + FreeBSD + Windows refuse with `error: ... / hint: ...` format; `>>>` event lines on stdout; curl-with-wget-fallback (exit 5 if both missing); SHA-256 verify against `checksums.txt` from the GitHub Release (skipable via `INSTALL_VERIFY=0` with a stern warning); atomic install via temp-file + mv; suggested systemd unit text printed AFTER successful install (not auto-installed); idempotent re-run with same version is a no-op.
  - **Commit**: `feat(install): add POSIX install.sh with checksum verify, platform refusal, suggested systemd unit`

- [X] T009 [US1] Create `docs/install/install.sh` as a verbatim mirror of the repo-root `install.sh`. File-copy via Makefile target (`make mirror-install`) executed by CI in the release workflow. Also update [docs/operations.md](../../docs/operations.md) with a one-time manual step: enable GitHub Pages from `/docs` path in repo settings (cannot be automated from a workflow). Mirror is what the canonical `curl https://proxa-server.github.io/proxa/install/install.sh | sh` URL serves.
  - **Commit**: `docs(install): mirror install.sh under docs/install for GitHub Pages hosting`

- [X] T010 [US1] End-to-end test `tests/e2e/install_sh_test.go` covering the 5 scenarios per [contracts/install-sh.md test coverage](./contracts/install-sh.md): TestInstallSh_HappyPath_LinuxAmd64 (debian:12 container, `apt-get install -y curl`, run installer, verify binary + version), TestInstallSh_RefuseAlpine (alpine:3.20 container, assert exit 1 + "Alpine musl" message), TestInstallSh_RefuseChecksumMismatch (httptest serving tampered `checksums.txt`, override `INSTALL_BASE_URL`, assert exit 3), TestInstallSh_NoCurlNoWget (busybox-minimal container without either, assert exit 5), TestInstallSh_Idempotent (install v0.4.1 then v0.4.2 via env override, verify final version + no orphan files). Tag `//go:build e2e`. Skip when Docker unavailable.
  - **Commit**: `test(e2e): cover install.sh happy path, platform refusal, checksum mismatch, idempotency (SC-001 / SC-009)`

**Checkpoint**: After T010, US1 deliverable: operators can install Proxa via one-line `curl … | sh` on supported Linux distros.

---

## Phase 4: User Story 2 — Run as container from GHCR (Priority: P1)

**Goal**: `docker run -d -v proxa-data:/data ghcr.io/proxa-server/proxa:v0.4.2 server` works on amd64 + arm64. Same image multi-arch; agent image exists in parallel as the v0.5 gate.

**Independent Test**: `tests/e2e/docker_image_test.go:TestDockerImage_BootsAndRespondsOnAmd64` — pull local snapshot image, run container, verify `/api/v1/system` responds 200 with `distribution=docker`.

**Maps to**: spec FR-008, FR-012, FR-013, SC-002, SC-003, SC-011, SC-012; contracts [contracts/dockerfiles.md](./contracts/dockerfiles.md) + [contracts/release-pipeline.md](./contracts/release-pipeline.md).

- [X] T011 [US2] Create `Dockerfile.proxa` at repo root per [contracts/dockerfiles.md](./contracts/dockerfiles.md). `FROM scratch`, `ARG TARGETOS / TARGETARCH`, `COPY proxa /usr/local/bin/proxa`, `USER 65532:65532` (nonroot), `EXPOSE 8080 80 443`, `VOLUME ["/data"]`, `ENV PROXA_DISTRIBUTION=docker`, `ENTRYPOINT ["/usr/local/bin/proxa"]`, `CMD ["server", "--data-dir", "/data"]`. No LABELS in the Dockerfile — Goreleaser adds them at build time.
  - **Commit**: `feat(runtime/docker): add Dockerfile.proxa for control-plane image`

- [X] T012 [US2] Create `Dockerfile.proxa-agent` at repo root per [contracts/dockerfiles.md](./contracts/dockerfiles.md). `FROM scratch`, `COPY proxa-agent /usr/local/bin/proxa-agent`, `USER 65532:65532`, NO `EXPOSE` (agent outbound-only), NO `VOLUME` (operator mounts `/var/run/docker.sock`), `ENV PROXA_DISTRIBUTION=docker`, `ENTRYPOINT ["/usr/local/bin/proxa-agent"]`, `LABEL org.opencontainers.image.documentation="Mount /var/run/docker.sock from host at runtime."`. No default CMD (agent stub only supports `version`).
  - **Commit**: `feat(runtime/docker): add Dockerfile.proxa-agent for worker-node image`

- [X] T013 [US2] Extend `.goreleaser.yml` with `dockers` block (4 entries: proxa-amd64, proxa-arm64, proxa-agent-amd64, proxa-agent-arm64) + `docker_manifests` block (6 entries: 3 aliases × 2 binaries — full `v{Version}`, `v{Major}.{Minor}` stream, `latest`). Each `dockers` entry uses `use: buildx`, platform flag, OCI labels (org.opencontainers.image.{source,version,revision,licenses,title,description,url}) per [contracts/release-pipeline.md](./contracts/release-pipeline.md). Verify `make release-dry` builds all 4 images locally without error.
  - **Commit**: `chore(ci): extend goreleaser with multi-arch image build + 6 manifests`

- [X] T014 [US2] Create `.github/workflows/release.yml` per [contracts/release-pipeline.md](./contracts/release-pipeline.md). Trigger: `push.tags: ['v*']`. Permissions: `contents: write` + `packages: write`. Steps: checkout (fetch-depth 0), setup-go 1.26.x, setup-docker-buildx, login to ghcr.io with `GITHUB_TOKEN`, run `go tool goreleaser release --clean`, verify image LABELS via `docker manifest inspect ghcr.io/proxa-server/proxa:${{ github.ref_name }}`, mark GHCR packages public via `gh api -X PATCH /user/packages/container/proxa --field visibility=public` (idempotent), also run `make mirror-install` to copy `install.sh` → `docs/install/install.sh` and commit if changed (or skip if unchanged).
  - **Commit**: `chore(ci): add release.yml workflow for tag-triggered goreleaser + GHCR publish`

- [X] T015 [US2] End-to-end test `tests/e2e/docker_image_test.go` covering the 7 scenarios per [contracts/dockerfiles.md test coverage](./contracts/dockerfiles.md). TestMain builds local snapshot images once via `go tool goreleaser release --snapshot --skip=publish --clean` and caches their tags. Test cases: TestDockerImage_BootsAndRespondsOnAmd64 (run + curl /api/v1/system/status 200), TestDockerImage_BootsAndRespondsOnArm64 (skip if `runtime.GOARCH != "amd64"` to avoid emulation flakes), TestDockerImage_RunsAsNonroot (`docker inspect` shows User: "65532:65532"), TestDockerImage_DistributionFieldIsDocker (`proxa system info --json` from inside container reports `"distribution":"docker"`), TestDockerImage_SizeWithinBudget (`docker image inspect ... .Size` < 80MB proxa / < 40MB agent — warn not fail), TestDockerAgentImage_RunsVersionSubcommand (`docker run agent version`), TestDockerImage_LabelsPresent (`docker inspect ... .Config.Labels` includes all 7 OCI labels). Tag `//go:build e2e`. Skip when Docker unavailable.
  - **Commit**: `test(e2e): cover docker image boot, nonroot user, distribution field, labels, size budget (SC-002 / SC-003 / SC-012)`

**Checkpoint**: After T015, US2 deliverable: GHCR images discoverable + multi-arch + bootable. v0.5 multi-host has its image-distribution gate met.

---

## Phase 5: User Story 3 — Bench baseline (Priority: P1)

**Goal**: `make bench` produces baseline performance numbers across 6 categories. Future PRs detect regression by comparing to these baselines.

**Independent Test**: `make bench` exits 0 and emits ≥6 named metrics (services/sec, µs/req-p50, µs/req-p99, containers/cycle, MB/sec, lines/sec, MB-rss).

**Maps to**: spec FR-001, SC-004; depends on T001 (bench/ scaffold) + T002 (harness scaffold for ProxaBinary()) — but T025 not required since bench files use harness lazily.

- [X] T016 [P] [US3] `bench/bench_reconciler_test.go` — `BenchmarkReconciler_TickThroughput`. Spin up `reconciler.New` with a synthetic in-memory state store + N (=100) synthetic services (no docker; use a stub Runtime). Measure tick throughput as `b.N` ticks processed. `b.ReportMetric(float64(N)/sec, "services/sec")` on the wrapping wall time. Pure unit; no docker.
  - **Commit**: `bench(bench): add reconciler tick-throughput baseline`

- [X] T017 [P] [US3] `bench/bench_ingress_test.go` — `BenchmarkIngress_L7Latency`. Start a httptest.Server backend that returns 200 immediately; configure ingress to forward to it; hammer with N concurrent goroutines doing GET requests. Record latency samples in a slice; compute p50 + p99 after the bench; `b.ReportMetric(float64(p50.Microseconds()), "µs/req-p50")` + `b.ReportMetric(float64(p99.Microseconds()), "µs/req-p99")`. Pure unit.
  - **Commit**: `bench(bench): add ingress L7 latency p50/p99 baseline`

- [X] T018 [P] [US3] `bench/bench_probe_test.go` — `BenchmarkProbe_WaveCapacity`. Start a synthetic httptest backend that returns 200 with N ms delay. Spin up `probe.Manager` with N tracked container IDs (use a stub Runtime that returns each ID's "container info" pointing at the httptest URL). Measure how many full probe cycles complete in the bench wall time. `b.ReportMetric(float64(N), "containers/cycle")`. Pure unit.
  - **Commit**: `bench(bench): add probe wave capacity baseline`

- [X] T019 [P] [US3] `bench/bench_l4_test.go` — `BenchmarkL4_Throughput`. Start a TCP echo server on loopback (port 0); configure L4 proxy in front of it; client opens connection through proxy, writes N MB, reads echoes. Measure sustained MB/sec. `b.ReportMetric(mbPerSec, "MB/sec")`. Pure unit (loopback only).
  - **Commit**: `bench(bench): add L4 proxy throughput baseline`

- [X] T020 [P] [US3] `bench/bench_sse_test.go` — `BenchmarkSSE_Throughput`. Call `writeSSEData(io.Discard, line)` in a tight loop with realistic log line sizes. Measure lines/sec. `b.ReportMetric(linesPerSec, "lines/sec")`. Pure unit (no network).
  - **Commit**: `bench(bench): add SSE encoder throughput baseline`

- [X] T021 [US3] Two-file split for the idle-memory bench:
  - `bench/bench_idle_memory_linux_test.go` (`//go:build linux`) — spawn `proxa server` as a subprocess via `harness.ProxaBinary(b)`, sleep 10s for settle, read `/proc/<pid>/status` VmRSS line, kill subprocess. `b.ReportMetric(rssMB, "MB-rss")`.
  - `bench/bench_idle_memory_other_test.go` (`//go:build !linux`) — same shape but uses `runtime/debug.ReadMemStats` inside an in-process server (no subprocess); reports `(stats.HeapAlloc + stats.StackInuse)/1MB` as approximation. Same metric name "MB-rss" for unit consistency.
  - **Commit**: `bench(bench): add idle-memory baseline (Linux VmRSS + portable runtime/debug fallback)`

---

## Phase 6: User Story 4 — Synctest adoption + e2e harness consolidation (Priority: P2)

**Goal**: Reconciler + probe time-based unit tests run deterministically and ≥50% faster via `testing/synctest`. e2e helpers consolidated into a real package; per-test SC tagging available via `harness.SCAttrs`.

**Independent Test**: Wall-time of `go test ./internal/reconciler/... ./internal/probe/...` post-migration is ≥50% lower than v0.4.1 baseline; zero flakes across 10 consecutive runs.

**Maps to**: spec FR-002, FR-004, FR-005, FR-007, SC-005, SC-007, SC-008; research [R-004 synctest scope](./research.md).

- [X] T022 [US4] Migrate `internal/reconciler/reconciler_test.go` time-based tests — 2 `time.Sleep(100*time.Millisecond)` calls at lines 162 + 168. Wrap the relevant test bodies in `synctest.Run(func() { ... })` (Go 1.25 graduated). Replace `time.Sleep` with `synctest.Wait()` after the goroutine-bound operation. Document the pattern in a small comment for future contributors.
  - **Commit**: `test(reconciler): migrate tick-interval tests to testing/synctest`

- [X] T023 [US4] Migrate `internal/probe/manager_test.go` time-based tests — 2 `time.Sleep` calls at lines 89 (10s) + 123 (30s). These are the biggest wall-time wins: synctest makes them effectively zero. Wrap test bodies in `synctest.Run`; replace `time.Sleep` with `synctest.Wait()`.
  - **Commit**: `test(probe): migrate manager streak/retry tests to testing/synctest`

- [X] T024 [US4] Audit `internal/probe/http_test.go` — 2 `time.Sleep` calls at lines 69 (500ms) + 87 (2s) STAY (inside `httptest.Server` handler goroutines, which run outside any synctest bubble per [research.md R-004](./research.md)). Add a 3-line comment block above each documenting why these are real waits. Do NOT migrate.
  - **Commit**: `docs(probe): document why http_test.go sleeps stay (httptest handlers, not migratable)`

- [X] T025 [US4] Implement `tests/e2e/internal/harness/` package contents — move ~15 helpers from scattered `tests/e2e/*.go` files into purpose-named harness files per [plan.md project structure](./plan.md):
  - `proxa.go` — runProxa, proxaBinary, mustGetwd, findRepoRoot
  - `server.go` — startServer (background subprocess)
  - `socket.go` — socketPath, getViaSocket, sseRequest
  - `docker.go` — waitForCount, waitForServiceStatus, skipIfHTTPProbeUnreachable
  - `token.go` — readToken
  - `ports.go` — pickTwoFreeTCPPorts
  - `snippet.go` — snippet (string truncation)
  - `copyrepo.go` — copyRepoForTest (from v0.4.1 tool_directive_test)
  - `images.go` — NEW — pinned image digests for `nginxinc/nginx-unprivileged` + `traefik/whoami` via `@sha256:...` constants per FR-005/SC-008
  - `attrs.go` — NEW — `SCAttrs(t *testing.T, spec, sc string)` helper that calls `t.Attr("spec", ...)` + `t.Attr("sc", ...)` (Go 1.25 stdlib) for FR-007/T.Attr tagging
  - All helpers exported with godoc explaining purpose + when to use.
  - **Commit**: `test(tests/e2e): consolidate harness helpers + pin test images by digest (FR-004 / FR-005)`

- [X] T026 [US4] Update every `tests/e2e/*_test.go` file to import `github.com/proxa-server/proxa/tests/e2e/internal/harness` and replace inline helper calls with `harness.X(...)` form. For tests whose name matches `TestSC_NNN_...`, add `harness.SCAttrs(t, "<feature>", "<SC-NNN>")` as the first line of the test body. Measure total LOC in `tests/e2e/*.go` before + after; record in commit message. Target: ≥30% reduction (SC-007).
  - **Commit**: `test(tests/e2e): replace inline helpers with harness package imports (SC-007)`

---

## Phase 7: User Story 5 — Coverage reporting (Priority: P3)

**Goal**: Contributors run `make cover` and see per-package coverage with packages below 60% highlighted. Allowlist exempts genuinely-untestable packages. Hard-fail gate deferred to a later release.

**Independent Test**: `make cover` exits 0 and produces both an HTML report at `coverage.html` and a per-package table on stdout.

**Maps to**: spec FR-006, SC-006. Most of US5 already landed in Phase 2 (T004 binary, T005 tests, T006 allowlist, T007 Makefile target). Phase 7 only adds operator-facing docs.

- [X] T027 [US5] Update `docs/operations.md` with the Coverage section per [contracts/coverage-gate.md](./contracts/coverage-gate.md): how to read `make cover` output (text + JSON modes), allowlist file format + rationale-comment convention, when to add an entry, future hard-fail plan (v0.4.3+ flips exit code based on threshold violation).
  - **Commit**: `docs(operations): document make cover output and .coverage-allowlist usage`

---

## Phase 8: Dashboard Parity + Cross-Cutting Polish

**Purpose**: Land the dashboard-parity surface (Distribution field on existing System Info), plus all release-polish work (operations docs refresh, licenses audit, validation report).

- [X] T028 Extend `internal/version/runtime.go` SystemInfo with `Distribution string` field — per [data-model.md](./data-model.md). Detection logic: check `PROXA_DISTRIBUTION` env var FIRST (set in Dockerfiles), then `/.dockerenv` presence OR `os.Getpid() == 1` fallback, default `"binary"`. Computed once at startup, cached in a package-level var, returned by every `System()` call (no file I/O on the request path). JSON tag: `distribution`.
  - **Commit**: `feat(version): add Distribution field to SystemInfo with env+fs detection`

- [X] T029 Unit tests for Distribution detector in `internal/version/runtime_test.go` — 4 cases per [data-model.md](./data-model.md): TestSystem_Distribution_EnvOverride_Docker (`t.Setenv("PROXA_DISTRIBUTION", "docker")` → `"docker"`), TestSystem_Distribution_NotInContainer (default → `"binary"`), TestSystem_Distribution_DefaultUnknownOnError (detector parameterized; force error path → `"unknown"`), TestSystem_DistributionRoundtripJSON (encode + decode preserves the value). Use the parameterized-detector indirection from v0.4.1 if cleaner.
  - **Commit**: `test(version): cover Distribution detector enum + JSON roundtrip`

- [X] T030 Update dashboard templates to surface the Distribution field — `internal/web/templates/system.html` adds a Distribution row to the full System Info table (after the GOMAXPROCS row); `internal/web/templates/index.html` extends the existing footer card one-liner so it reads e.g. `v0.4.2 · go1.26.0 · 10 (host) · docker`.
  - **Commit**: `feat(web): surface Distribution on dashboard footer card and /ui/system table (FR-014)`

- [X] T031 End-to-end test `tests/e2e/bench_smoke_test.go` — `//go:build e2e`, invoke `make bench` via `os/exec` against a temp data dir, assert exit 0, assert stdout contains all 6 named metrics: `services/sec`, `µs/req-p50`, `containers/cycle`, `MB/sec`, `lines/sec`, `MB-rss`. Validates US3 SC-004 end-to-end. Skip on hosts where `make` is unavailable (rare; document the skip).
  - **Commit**: `test(e2e): cover make bench produces all 6 named metrics (SC-004)`

- [X] T032 Refresh `docs/operations.md` with v0.4.2 sections: **Container deployment** (`docker run` pattern, volume mount for `/data`, port mapping for `:8080`/`:80`/`:443`, env vars including `PROXA_DISTRIBUTION`), **Bench expectations** (when to run, what regression looks like, how to update `bench/binary-size-baseline.txt` with rationale), **Future SBOM / cosign work** (deferred until first supply-chain ask, when to revisit), **GitHub Pages setup** (one-time manual step: repo Settings → Pages → Source = "Deploy from a branch" → main / docs).
  - **Commit**: `docs(operations): add v0.4.2 sections (container deploy, bench, SBOM future, Pages setup)`

- [X] T033 Refresh `docs/licenses.md` log entry for 006-test-foundation-public-images. ONE new tool-directive dep: `github.com/goreleaser/goreleaser/v2` (MIT, verified). Add transitive surface count from `go list -m all`. Confirm zero new production-binary deps. Match the format of prior refresh entries.
  - **Commit**: `docs(licenses): refresh transitive license audit for 006 (+goreleaser MIT)`

- [X] T034 Create `specs/006-test-foundation-public-images/validation.md` with SC-by-SC PASS/FAIL table covering SC-001..SC-012. For each criterion, record the test that validates it (unit / e2e file + test name) OR the manual verification step (quickstart section #). Include audit findings: `tests/e2e/*.go` LOC drop measured for SC-007, synctest migration count (4 sleeps migrated, 2 documented-stay) for SC-005, binary size delta for SC-012, image size budgets for FR-013.
  - **Commit**: `docs(spec): record validation results for 006-test-foundation-public-images`

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 Setup**: No dependencies — starts immediately.
- **Phase 2 Foundational**: Depends on Phase 1. **BLOCKS all US work** (Goreleaser tool directive needed for T013/T014/T015; cmd/coverage-gate needed for T027/T032; Makefile targets needed throughout).
- **Phase 3 US1 install.sh**: Depends on Phase 2 (uses Makefile `make mirror-install` from T007 + the GHCR release infra prepared in T013/T014).
- **Phase 4 US2 Public Images**: Depends on Phase 2 (Goreleaser tool directive from T003).
- **Phase 5 US3 Bench Suite**: Depends on Phase 1 only (bench scaffold from T001). Independent of Phase 2 unless T021 uses `harness.ProxaBinary` (which is moved in T025 — for v0.4.2, T021 can use the existing `proxaBinary` helper still in tests/e2e/ until T025 lands, then refactor in T026).
- **Phase 6 US4 Synctest + Harness**: Depends on Phase 1 + Phase 2 (harness scaffold from T002; T024 documents-only stays).
- **Phase 7 US5 Coverage docs**: Depends on Phase 2 (T004 cmd/coverage-gate + T007 Makefile cover target).
- **Phase 8 Polish**: Depends on all desired US phases being complete (T034 validation needs all tests passing).

### Within-Phase Order

- Within Phase 2, T003 (tool directive) MUST land before T007 (Makefile uses `go tool goreleaser`).
- Within Phase 4, T011 + T012 (Dockerfiles) MUST land before T013 (Goreleaser config references them) which MUST land before T014 (workflow runs Goreleaser).
- Within Phase 6, T025 (harness implementation) MUST land before T026 (e2e files import harness).
- Within Phase 8, T028 (Distribution field on SystemInfo) MUST land before T029 (its tests) which MUST land before T030 (templates render the field).

### Parallel Opportunities

- T001 + T002 (different directories — scaffold in parallel)
- T004 + T005 (different file; impl + test can land sequentially but if test-first style is preferred, both in one impl run)
- T011 + T012 (independent Dockerfiles — parallelize)
- T016 + T017 + T018 + T019 + T020 (independent benchmark files — full parallel — marked [P])
- T022 + T023 + T024 (independent test files — parallel)
- T031 + T033 + T034 (independent docs — parallel)

### MVP Scope

The MVP is **Phase 2 (foundational tooling) + Phase 4 (US2 public images)** — 12 tasks. Even at MVP, an operator can `docker run ghcr.io/proxa-server/proxa:v0.4.2 server` and the v0.5 multi-host gate is met. install.sh polish (US1) and bench/synctest/coverage (US3-US5) build on top but aren't strictly required for v0.5.

---

## Parallel Example: Phase 5 (US3 Bench Suite)

```bash
# All 5 simple bench files can be developed in parallel
# (T016, T017, T018, T019, T020 marked [P] in the task list)
# T021 sequential (two-file split with build tags)
```

---

## Notes

- **Commit timestamps** per `feedback_commit_timestamps`: implementation may straddle work-hours. Commits authored 8am-5pm Mon-Fri MUST backdate via `GIT_AUTHOR_DATE` + `GIT_COMMITTER_DATE` env vars to a recent weekend / evening (e.g., previous Sunday evening). After 5pm Mon-Fri or weekends = real-time OK.
- **NO COMMITS YET** — user explicitly instructed working-tree-only until they say "commit now". The spec + plan + tasks themselves are uncommitted as of tasks.md creation. When user authorizes, backdating may apply to ALL pending commits in one batch.
- **Constitution §V verification** after each phase: `go mod tidy && git diff go.mod go.sum` — diff MUST stay limited to the goreleaser tool directive added in T003 + its transitive entries. NO production-binary deps added.
- **Constitution §VIII zero-downtime check**: after T028-T030 (Distribution field), the running server MUST still accept v0.4.1-issued tokens (verified by upgrade smoke test in quickstart section 1) — verify before Phase 8 ends.
- **Dashboard-parity verification**: T030 (templates) + T029 (tests) + T031 (e2e bench smoke) together satisfy the `feedback_dashboard_parity` rule for this release. The Distribution field is the operator-visible artifact proving v0.4.2 landed.
- **Multi-host gate verification** per `feedback_multi_host_is_killer_feature` memory: at end of Phase 4 (T015), `ghcr.io/proxa-server/proxa-agent:dev` image must be bootable (`docker run agent version` works). This proves v0.5 multi-host can consume the release infrastructure without rework.
- **Avoid**: rewriting tests during the harness migration (T025/T026 is a MOVE refactor, not a behavior change); accidentally adding production deps when configuring goreleaser (the tool directive is OK, regular `require` is NOT); committing the dev `ghcr.io/...:dev` snapshot tag (it's a local build artifact, not for push).

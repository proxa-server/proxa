# Feature Specification: Test Foundation + Public Images (v0.4.2)

**Feature Branch**: `006-test-foundation-public-images`

**Created**: 2026-05-26

**Status**: Draft

**Input**: User description: v0.4.2 — second release of the Foundation Train (after 005-modern-go shipped at v0.4.1). Two complementary themes share one release because both are "infrastructure that future features need":

1. **Test Foundation** — formalize the test/CI infrastructure so v0.5 Multi-host MVP and v0.6 Auto-scaling land on a stable platform with fast deterministic tests + performance baselines + a coverage gate.
2. **Public Images** — ship Proxa as installable Docker images via GHCR + a one-line installer at a stable public URL. This GATES all v0.5 multi-host work because `proxa-agent` must be bootstrap-able on remote machines via `curl … | sh`.

Both themes are infrastructure-only — no operator-visible runtime behavior changes. The dashboard-parity contract is satisfied trivially by extending the existing System Info card (from v0.4.1) with a single new field describing how the running Proxa was distributed.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator installs Proxa via a one-line installer (Priority: P1)

A new operator hears about Proxa from a blog post or HN thread, opens their fresh Linux VM (Ubuntu or Debian, amd64 or arm64), and pastes a single `curl … | sh` command. The installer detects their OS and CPU architecture, downloads the right `proxa` binary from the official release, verifies its SHA-256 checksum, installs it to `/usr/local/bin`, and prints a suggested `systemd` unit text they can copy. Within 60 seconds of pasting the command, `proxa version` reports a real version string.

**Why this priority**: This is THE distribution UX that determines whether Proxa feels like a "real product" or "a Go project on GitHub". Without it, operators are reading release notes, downloading tarballs, and manually verifying checksums. With it, Proxa joins the modern installable-tools tier alongside `rustup`, `k3s`, `tailscale`, `nvm`. It also unblocks v0.5 multi-host because the same installer will eventually bootstrap remote agents.

**Independent Test**: On a clean Ubuntu/amd64 VM (no Go, no Docker, no curl history of Proxa), run `curl <published-url>/install.sh | sh`. Verify (a) exit 0, (b) `/usr/local/bin/proxa` exists, (c) `proxa version` returns the expected version, (d) suggested systemd unit was printed to stdout. Repeat on an arm64 VM. Repeat on Debian.

**Acceptance Scenarios**:

1. **Given** a fresh Ubuntu 22.04 amd64 VM with `curl` available, **When** the operator runs `curl <published-url>/install.sh | sh`, **Then** the script completes successfully within 60 seconds, the binary is installed to `/usr/local/bin/proxa`, and `proxa version` prints the expected version + commit + build date.
2. **Given** a fresh Debian 12 arm64 VM, **When** the operator runs the same one-liner, **Then** the arm64 binary is downloaded and installed successfully.
3. **Given** an existing `/usr/local/bin/proxa` from a prior install, **When** the operator re-runs the installer, **Then** the existing binary is overwritten with the latest, version reports update accordingly, and no orphan files remain.
4. **Given** a network where the GitHub Release tarball is reachable but the SHA-256 checksum file is tampered, **When** the installer downloads and verifies, **Then** the installer refuses to install the binary and prints a clear "checksum mismatch" error.
5. **Given** an OS that the installer does NOT yet support (e.g., FreeBSD or Alpine musl in v0.4.2 scope), **When** the operator runs the installer, **Then** the installer prints a clear "unsupported platform" message identifying the detected OS + arch and suggesting manual download.

---

### User Story 2 - Operator runs Proxa as a container from GHCR (Priority: P1)

An operator who already has Docker (or Colima, or Rancher Desktop) wants to try Proxa without installing a binary. They run `docker run -d --name proxa -v proxa-data:/data -p 8080:8080 ghcr.io/proxa-server/proxa:v0.4.2 server`. The container starts, the API listens on port 8080, and the dashboard is reachable. The same image works on amd64 (Linux x86 VPS) and arm64 (Apple Silicon Mac, Graviton VPS) — the right architecture is auto-selected by the Docker daemon.

**Why this priority**: Containerized distribution is the modern default for self-hosted infrastructure. It also gates v0.5 multi-host directly: the `proxa-agent` runs as a container on each remote host, and that requires `ghcr.io/proxa-server/proxa-agent` to exist as a published image. Multi-arch is required because Apple Silicon dev loops + Graviton/Ampere production hosts are common — single-arch is a regression vs. competitors.

**Independent Test**: On a Mac (arm64) with Docker available, pull and run `ghcr.io/proxa-server/proxa:v0.4.2`. Verify the API responds on 8080. Repeat on a Linux amd64 host. Confirm the image manifest lists both architectures via `docker manifest inspect`.

**Acceptance Scenarios**:

1. **Given** Docker (or compatible runtime) on an amd64 Linux host, **When** the operator runs `docker run ghcr.io/proxa-server/proxa:v0.4.2 server`, **Then** the container starts cleanly and `proxa system info` reports the expected version.
2. **Given** the same command on an arm64 macOS Docker Desktop or Colima, **When** the operator runs it, **Then** the arm64 variant is pulled and runs without "exec format error".
3. **Given** the same operator wants the agent image (forward-looking), **When** they pull `ghcr.io/proxa-server/proxa-agent:v0.4.2`, **Then** the image exists, is multi-arch, and contains the stub agent binary that responds to `proxa-agent version`.
4. **Given** the GHCR image is pulled without authentication, **When** the operator runs it, **Then** the pull succeeds (images are publicly readable; no GitHub token required for read).

---

### User Story 3 - Contributor measures performance baselines before changes (Priority: P1)

A contributor working on v0.5+ features wants to know whether their change degrades performance. They run `make bench` on a clean checkout. Within a few minutes, the suite emits baseline numbers for: reconciler tick throughput (services-per-second the reconciler can converge), ingress L7 request latency (p50/p99 under sustained load), L4 proxy throughput (megabits-per-second sustained), probe wave latency (containers-checked-per-cycle), SSE message throughput (log lines streamed per second), and the idle memory footprint of `proxa server` (RSS bytes). They can compare these numbers against a previous baseline captured by another contributor or by CI.

**Why this priority**: Today Proxa has zero benchmarks. Any performance regression that lands silently between v0.4.2 and v1.0 is invisible until an operator notices in production. With a bench suite baseline established now, every later PR has a measurable "is this faster or slower?" answer. Multi-host (v0.5) will dramatically change reconciler workload — a baseline before that change is captured forever.

**Independent Test**: On any developer machine, run `make bench`. Output includes at least one measurement per category named above, with reproducible numbers (within ~10% variance across runs on the same machine).

**Acceptance Scenarios**:

1. **Given** a clean checkout of v0.4.2, **When** the contributor runs `make bench`, **Then** the suite completes within 5 minutes and emits structured numeric results for each named category.
2. **Given** the suite has been run once and the contributor makes a code change, **When** they re-run `make bench` and compare, **Then** any change beyond noise tolerance (say ±10%) is visible as a delta in the output.
3. **Given** the bench suite is run on CI, **When** results diverge significantly from the previous run, **Then** the divergence is surfaced (initially as a report; hard-fail gating is a future release).

---

### User Story 4 - Contributor sees fast, deterministic, non-flaky tests (Priority: P2)

A contributor runs the test suite. The reconciler and probe tests — which today rely on real `time.Sleep` calls and can take 5-30 seconds per test plus occasional flakes — now use synthetic time. The entire reconciler + probe unit test suite runs in under a second. No `time.Sleep`-based race conditions remain. The contributor's dev loop becomes "edit → save → tests in 5s" instead of "edit → save → tests in 90s and one flake".

**Why this priority**: Slow flaky tests are the #1 silent productivity killer in any project. Today there are observable wait-based flakes in reconciler/probe tests. Synthetic time eliminates the entire class. Once landed, every future test in those packages inherits the pattern — payoff compounds.

**Independent Test**: Measure wall time of `go test ./internal/reconciler/... ./internal/probe/... -count=1` before and after the change. The "after" time should be at least 50% lower for synctest-applicable tests. Run the suite 10 times consecutively — zero flakes.

**Acceptance Scenarios**:

1. **Given** the reconciler + probe test packages post-change, **When** a contributor runs them 10 times in a row, **Then** there are zero flakes and the total wall time is at least 50% less than the pre-change baseline.
2. **Given** a contributor writes a new reconciler or probe test, **When** they need to simulate the passage of time, **Then** the synthetic-time pattern is documented and obvious to follow.
3. **Given** an existing test that DID use `time.Sleep`, **When** the change lands, **Then** all such sleeps in the reconciler/probe test files are eliminated (zero `time.Sleep` calls remain in those test files post-change, except for explicit "wait for real OS process" scenarios where synthetic time doesn't apply).

---

### User Story 5 - Contributor sees per-package coverage reports (Priority: P3)

A contributor runs `make cover`. They see a per-package coverage table with percentages. Any package below the 60% baseline is flagged in the output (highlighted, sorted to the top, or otherwise made obvious). The contributor can also open an HTML report for line-by-line coverage. The 60% threshold is documented; how to override it for genuinely-low-coverage packages (e.g., a package whose only purpose is to wrap external APIs) is documented.

**Why this priority**: Coverage is a weak signal in isolation but useful as a regression detector. Today coverage is uncomputed. Setting a baseline now means future PRs that drop coverage are visible. The 60% baseline is intentionally low — it's a floor, not a target. Hard CI failure is deferred to a later release once the threshold is calibrated.

**Independent Test**: Run `make cover` on a clean checkout. Verify (a) output includes per-package percentages, (b) packages below 60% are obvious in the output, (c) an HTML report is generated and openable.

**Acceptance Scenarios**:

1. **Given** v0.4.2 just landed, **When** a contributor runs `make cover`, **Then** they see per-package coverage and any package below 60% is clearly highlighted.
2. **Given** a contributor adds a new feature without tests, **When** they run `make cover`, **Then** the new package's coverage is visibly low and flagged.
3. **Given** a package that genuinely cannot reach 60% (e.g., a thin OS wrapper), **When** the contributor wants to suppress it from the alert, **Then** there is a documented mechanism to do so (e.g., an opt-out file or comment).

---

### Edge Cases

- **GHCR rate limiting** — what happens when an operator pulls the image from a CI cluster that hits GHCR rate limits? **Expected**: documented in operations.md with the recommendation to use a registry mirror or authenticated pull for high-volume CI.
- **install.sh against an unsupported OS** — Alpine musl, FreeBSD, Windows. **Expected**: detect and refuse with a clear "unsupported platform" message + manual-download fallback link. Does NOT attempt a partial install.
- **install.sh against a host without `curl` or `wget`** — bootstrap problem. **Expected**: documented in operations.md that the host needs `curl` OR `wget`; the script chooses whichever is available; both missing = clear error.
- **Image tag pinning** — operator pulls `ghcr.io/proxa-server/proxa:v0.4.2` six months later when v0.6 is current. **Expected**: the v0.4.2 image is still available (GHCR keeps tags forever unless explicitly deleted). Operator stays on the version they pinned.
- **Multi-arch image manifest mismatch** — operator on an unusual architecture (e.g., riscv64) pulls the multi-arch tag. **Expected**: Docker daemon returns "no matching manifest" with a clear error; operator falls back to building from source.
- **Coverage on packages with no test files at all** — `internal/web`, `pkg/types`, etc. **Expected**: report shows them as "no tests" (not 0%) and they are excluded from the 60% gate. Documented.
- **Synctest on tests that need real OS interactions** — a test that probes a real `httptest.Server`. **Expected**: synctest does NOT apply to tests that interact with real network/processes; documented as scope.

## Requirements *(mandatory)*

### Functional Requirements

**Test Foundation (US3, US4, US5):**

- **FR-001**: System MUST provide a Makefile target that runs all benchmarks (reconciler tick throughput, ingress L7 latency, L4 proxy throughput, probe wave latency, SSE throughput, idle memory baseline) and emits structured numeric output suitable for tracking over time.
- **FR-002**: System MUST eliminate real `time.Sleep`-based waits in reconciler and probe unit tests, replacing them with synthetic-time primitives such that those tests run deterministically and at least 50% faster on a developer machine.
- **FR-003**: System MUST provide a Makefile matrix of test targets covering fast-feedback (unit no-race), default (unit + race), integration (against real container runtime), end-to-end (against built binary), benchmark, and coverage modes.
- **FR-004**: System MUST consolidate end-to-end test helpers (process startup, socket dialing, HTTP/SSE clients, port allocation, token reading) into a single internal harness package so individual test files focus on the scenario, not the plumbing.
- **FR-005**: System MUST pin every container image used in end-to-end tests by content-addressed digest, so a remote image change cannot silently alter test behavior.
- **FR-006**: System MUST provide per-package test coverage reporting accessible via a Makefile target, with packages below a documented baseline percentage clearly highlighted. The baseline is reporting-only in this release; hard enforcement is deferred to a later release.
- **FR-007**: System MUST allow individual tests to attach machine-readable attributes (such as the spec / success-criterion identifier the test validates) so test reports can be cross-referenced against specifications.

**Public Images (US1, US2):**

- **FR-008**: System MUST publish multi-architecture container images (amd64 and arm64 at minimum) for both the control plane and the agent stub binaries to a publicly-readable container registry on each tagged release.
- **FR-009**: System MUST provide a one-line shell installer script published at a stable public URL that detects the operator's OS and CPU architecture, downloads the correct binary from the official release, verifies its SHA-256 checksum, and installs it to a standard system location.
- **FR-010**: The installer script MUST refuse to install when the SHA-256 checksum does not match the published value, with a clear error message identifying the failure.
- **FR-011**: The installer script MUST print (not auto-execute) a suggested service-manager unit configuration after a successful install, so the operator can copy-paste it for their preferred service manager.
- **FR-012**: System MUST provide an automated release pipeline that, on each tagged release, builds all binaries, builds multi-arch images, pushes the images to the registry, and attaches binaries + checksums to the published release.
- **FR-013**: Both control-plane and agent images MUST be small (under a documented size budget — e.g., based on a minimal base image), MUST run as a non-root user where possible, and MUST document the expected volume mounts and exposed ports.

**Dashboard parity (cross-cutting):**

- **FR-014**: The dashboard's existing System Info surface MUST show how the running Proxa was distributed (e.g., "binary", "docker", "unknown") so operators can verify their distribution channel at a glance.

**Cross-cutting:**

- **FR-015**: This release MUST NOT introduce any new third-party module dependencies in the production binary. Build-time/release tooling MAY add dependencies via the existing tool-directive pattern, subject to the project's permissive-license allow list.
- **FR-016**: The release MUST NOT increase the published binary size by more than 2% relative to v0.4.1.
- **FR-017**: The release MUST preserve zero-downtime upgrade from v0.4.1: tokens issued by v0.4.1 remain valid; on-disk state remains readable; the dashboard renders on first load after upgrade.

### Key Entities

This feature introduces no new persistent entities. The runtime data model is unchanged. The only in-memory addition is a new field on the existing System Info payload describing the distribution channel.

- **Distribution channel** (new field on existing System Info): a small string indicating how the running Proxa binary was distributed — "binary" (downloaded from GitHub Releases or built locally), "docker" (running inside a container built from the official image), or "unknown" (could not detect). Used by operators to verify they're running a known-good distribution.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can install a working Proxa binary on a fresh supported Linux VM in under 60 seconds using a single one-line command. (Validates US1 / FR-009.)
- **SC-002**: An operator can pull and run the official Proxa container image on both amd64 and arm64 hosts with a single command, without authentication, and the API responds successfully. (Validates US2 / FR-008.)
- **SC-003**: The published container image for the agent stub exists on the registry and is multi-architecture, so v0.5 multi-host work can build on it without re-publishing infrastructure. (Validates FR-008 forward-compatibility.)
- **SC-004**: A contributor can run the full benchmark suite in under 5 minutes and receive numeric results across all six measurement categories. (Validates US3 / FR-001.)
- **SC-005**: The reconciler + probe unit test packages run at least 50% faster post-release compared to the v0.4.1 baseline, with zero flakes across 10 consecutive runs. (Validates US4 / FR-002.)
- **SC-006**: A contributor can view per-package coverage and any package below the 60% baseline is highlighted in the output. (Validates US5 / FR-006.)
- **SC-007**: The end-to-end test files import a consolidated harness package and the average end-to-end test file is at least 30% shorter in lines than its v0.4.1 counterpart. (Validates FR-004.)
- **SC-008**: Every container image used in end-to-end tests is pinned by SHA-256 digest, verified by an automated check in the test suite or CI. (Validates FR-005.)
- **SC-009**: The installer script refuses to install when given a tampered checksum file, with a clear error message identifying checksum mismatch. (Validates FR-010.)
- **SC-010**: An operator inspecting the dashboard's System Info surface can identify within 10 seconds whether the running Proxa was distributed as a binary or a container. (Validates US (cross-cutting dashboard parity) / FR-014.)
- **SC-011**: The release pipeline executes end-to-end on a test tag and produces all expected artifacts (binaries, checksums, multi-arch images for both control plane and agent) without manual intervention. (Validates FR-012.)
- **SC-012**: The published binary size is within ±2% of the v0.4.1 binary size. (Validates FR-016.)

## Assumptions

- The Go toolchain pin remains 1.26.x. Synthetic-time and structured test-attribute primitives are stdlib-available; no toolchain bump required.
- The release adds no new third-party module dependencies to the production binary. Build/release tooling (e.g., a release automation tool) MAY be added via the existing tool-directive pattern, subject to the §IX permissive-license allow list (target tooling is MIT-licensed and on the allow list).
- Image signing (e.g., cosign keyless) is intentionally deferred to a future release. The first supply-chain concern report will trigger that work. Documented in operations.md as future work.
- A Software Bill of Materials (SBOM) per release is intentionally deferred to a future release. Reconsider when the first enterprise user requests it.
- The installer's published URL uses GitHub Pages (zero infrastructure) in this release. A custom-CNAME upgrade path is documented in operations.md but not blocking.
- The installer supports only the two most common Linux distributions on amd64 and arm64 in this release: Ubuntu LTS (20.04, 22.04, 24.04) and Debian (11, 12). FreeBSD, Alpine musl, and Windows are documented as unsupported with a clear "manual download" fallback. Brew/AUR/deb-rpm packaging is deferred to later releases.
- The agent IMAGE ships in this release wrapping the existing stub binary (which only implements `version`). The agent's real functional implementation lands in v0.5 Multi-host MVP. The image shape (mounts, entrypoint) is the contract for v0.5.
- The container registry is GitHub Container Registry (GHCR) only in this release. Docker Hub mirror is deferred. Operators with private registries can re-tag and push to their own.
- The bench suite establishes the baseline; hard CI failure on regression is reporting-only in this release. Threshold tuning for hard-fail happens in a later release.
- The coverage gate is reporting-only at 60% baseline in this release. Hard CI failure is deferred to v0.4.3 or later once the threshold is calibrated against the actual codebase distribution.
- The synctest adoption is scoped to reconciler and probe packages where time-based flakes are observed. Other packages do not adopt synctest in this release — adoption is opportunistic in future releases when a flake is observed or a new time-based test is added.

## Dependencies

- Builds on v0.4.1 (005-modern-go) which shipped the System Info card this release extends.
- **Pre-requirement for v0.5** (Multi-host MVP): the published agent image (FR-008) is what v0.5's bootstrap script pulls onto remote hosts. Without v0.4.2, v0.5 cannot ship the agent loop.
- **Pre-requirement for v0.6** (Auto-scaling): the bench suite (FR-001) establishes the performance baseline against which auto-scaling controller overhead is measured.
- No external infrastructure dependencies new to this release. The container registry is the same GitHub-provided registry the project's source repository already uses. The installer URL uses the same GitHub-hosted documentation pages the project may already publish.

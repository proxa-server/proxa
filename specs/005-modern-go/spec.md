# Feature Specification: Modern Go Foundation Pass (v0.4.1)

**Feature Branch**: `005-modern-go`

**Created**: 2026-05-19

**Status**: Draft

**Input**: User description: v0.4.1 "Modern Go" foundation release — systematic modernization pass across Go 1.22 → 1.26 features applied to the entire codebase. Every change is justified by one of: (a) security hardening, (b) deprecation hygiene, (c) idiom modernization. Also fixes the probe-via-ingress + TLS collision bug discovered in the 0.4.0 logs demo.

This is the first release of the **v0.4.x Foundation Train**. v0.4.2 (Test Foundation) and v0.4.3 (Architectural Foundations) follow before v0.5.0 Confidence Mode.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - TLS-enabled service with health probe deploys cleanly (Priority: P1)

An operator declares a service exposed through ingress with `tls = true` AND a health probe (HTTP). They run `proxa up`, and within the normal reconciliation window the service is reported as healthy on both the CLI and the dashboard. They do not need to disable the probe, rewrite the manifest, or apply a workaround.

**Why this priority**: This is a concrete bug from the 0.4.0 logs demo — a real operator hit it within a few minutes of using the product, and the workaround (deleting the probe) silently weakens production posture. A foundation-train release that does not fix it would mean the next operator hits the same wall.

**Independent Test**: Deploy a single TLS-enabled HTTP service with a probe; assert service reaches "healthy" state within 30 seconds; assert no manual workaround was applied. Can be validated entirely from CLI + dashboard observation.

**Acceptance Scenarios**:

1. **Given** a TOML with `[ingress.tls] enabled = true` and a service with `[health.http] path = "/"`, **When** the operator runs `proxa up`, **Then** the service is reported as `healthy` within 30 seconds and the dashboard shows it as healthy.
2. **Given** the same configuration with TLS disabled, **When** the operator runs `proxa up`, **Then** the service continues to report healthy as it did in v0.4.0 (no regression of the non-TLS path).
3. **Given** the same configuration with TLS enabled but a probe that should genuinely fail (path returns 500), **When** the operator runs `proxa up`, **Then** the service is reported as unhealthy with the actual upstream status code in the operator-visible diagnostic.

---

### User Story 2 - Proxa hardens its own security defaults (Priority: P1)

A security reviewer evaluates Proxa for production use. They examine three areas: (1) whether Proxa can be tricked into reading or writing outside its declared data directory through path traversal, (2) whether authentication tokens are generated with cryptographically strong randomness, and (3) whether write endpoints are protected against cross-origin attack vectors. In all three areas, Proxa's defaults are hardened — no operator action required.

**Why this priority**: Constitution §II ("Security by Default") is non-negotiable. The current codebase pre-dates modern stdlib hardening primitives — wiring them up before the v0.5+ killer features land is far cheaper than retrofitting later. Also: this release adds no new attack surface, only removes existing surface — pure win.

**Independent Test**: (1) Run a unit test that asks Proxa's data-dir helpers to resolve a path containing `../../etc/passwd` and assert the helper refuses. (2) Inspect token-generation code paths and assert they all derive from a cryptographically secure source. (3) Issue a state-changing request from a foreign origin and assert it is rejected by default.

**Acceptance Scenarios**:

1. **Given** Proxa is running with a data directory at `/var/lib/proxa`, **When** any internal helper is given a relative path that resolves above `/var/lib/proxa`, **Then** the helper returns an error and does not open the file.
2. **Given** the bootstrap-admin flow generates a new token, **When** the token is inspected, **Then** it contains at least 128 bits of cryptographically secure entropy (no math-random, no hex-of-counter, no timestamp-only seeding).
3. **Given** the dashboard's write endpoints exist or will exist in future releases, **When** a request arrives with a foreign `Origin` header and no explicit per-handler opt-out, **Then** the request is rejected before the handler runs.

---

### User Story 3 - Operator confirms the modernization landed (Priority: P2)

An operator who has just upgraded to v0.4.1 wants to confirm that the modernization is in effect on their running instance — particularly the runtime version and whether the new container-aware CPU detection took effect when Proxa runs inside a CPU-limited container. They open the dashboard and find a small "System Info" surface that shows the Go runtime version, any active runtime experiments, and the effective parallelism (and whether it was auto-adjusted from a container limit).

**Why this priority**: This is the dashboard-parity contract for this release. The modernization is mostly invisible by design — but operators (especially the security-focused ones from US2) need a single place to verify it's active. A "did the upgrade actually land?" question must be answerable in under 10 seconds without shelling into the host.

**Independent Test**: Deploy Proxa, open the dashboard, locate the System Info surface, read the values, compare to expected (Go version matches the build; if running inside a Docker container with `--cpus=2`, the surface shows effective parallelism = 2, not the host's core count).

**Acceptance Scenarios**:

1. **Given** Proxa is running on bare metal or VM, **When** the operator opens the dashboard, **Then** they can locate a "System Info" surface showing Go runtime version, active runtime experiments (or "none"), and effective parallelism (= host CPU count).
2. **Given** Proxa is running inside a container with a CPU limit of 2 cores on a 16-core host, **When** the operator opens the dashboard, **Then** the System Info surface shows effective parallelism = 2 and indicates the value was auto-adjusted from a container limit.
3. **Given** the operator wants the same information from the CLI, **When** they run a status/info-style command, **Then** the same three values are returned in plain text suitable for scripting.

---

### User Story 4 - Codebase reads as modern Go (Priority: P2)

A contributor reads Proxa source for the first time. They expect to find idioms current as of Go 1.26: counter loops use the range-over-integer form instead of three-clause loops; slice operations use the standard `slices` package helpers instead of hand-rolled loops; goroutine pools use the modern `WaitGroup.Go` helper instead of manual `Add` / `defer Done` boilerplate; randomness uses the v2 random package; finalizers use the modern cleanup helper. No deprecated standard-library APIs are referenced anywhere in the project source.

**Why this priority**: Lowers the contributor on-ramp cost, keeps the codebase a recruiting / reference asset, and prevents the kind of drift that requires a costly "modernization release" in two years. Also, static analysis remains clean — no slow accumulation of deprecation noise that hides real issues.

**Independent Test**: Run the project's vet command and assert zero deprecation warnings. Inspect the diff after this release: the absolute count of legacy-random imports, legacy-finalizer calls, and manual goroutine-pool boilerplate drops to zero in non-test code.

**Acceptance Scenarios**:

1. **Given** the post-release source tree, **When** a contributor searches for the legacy random import, **Then** zero results are returned in non-test, non-vendored code.
2. **Given** the post-release source tree, **When** a contributor searches for the legacy finalizer API, **Then** zero results are returned in production code (test fixtures may use it).
3. **Given** the post-release source tree, **When** the project's standard static-analysis tooling is run, **Then** no deprecation warnings are reported.

---

### User Story 5 - Build tooling is reproducible (Priority: P3)

A new contributor clones Proxa and runs the standard lint / static-analysis commands. The tooling Just Works — no manual install of `staticcheck` or other binaries, no version-skew bugs, identical output on their machine and in CI. The set of tools and their pinned versions is visible in the project's main module manifest.

**Why this priority**: Removes a recurring contributor friction ("it works on my machine, fails in CI") and aligns Proxa with the modern idiomatic approach to tool dependencies. Low cost, lasting payoff. Tagged P3 because no functional behavior changes — purely developer ergonomics.

**Independent Test**: On a fresh clone, run the project's lint command and verify it produces output without requiring any explicit install of tools first.

**Acceptance Scenarios**:

1. **Given** a fresh clone of Proxa on a machine with only the Go toolchain installed, **When** the contributor runs the project's standard lint command, **Then** the lint tool runs successfully without manual install steps.
2. **Given** the same setup, **When** the contributor inspects the main module manifest, **Then** the set of pinned tools and their versions is plainly visible.

---

### Edge Cases

- What happens when the data-dir sandbox helper is asked to open a symlink whose target lies outside the rooted directory? **Expected**: refused with an error; documented in the data-dir helper's contract.
- What happens when Proxa is started with the parallelism environment variable set explicitly? **Expected**: the explicit value wins (no auto-adjust); the System Info surface reflects "explicit override" rather than "auto-adjusted".
- What happens when an operator has a TLS-enabled service AND wants the probe to assert the redirect itself (e.g., security-testing a redirect)? **Expected**: the probe configuration supports an explicit "do not follow redirects" opt-in; default remains "follow the ingress HTTP-to-HTTPS redirect cycle once".
- What happens when an operator upgrades from v0.4.0 to v0.4.1 with an active deployment? **Expected**: zero observable downtime; tokens generated by v0.4.0 remain valid; the System Info surface appears on first dashboard load post-upgrade.
- What happens when the dashboard's cross-origin protection challenges a same-origin write from the dashboard itself? **Expected**: same-origin requests are recognized and allowed; only cross-origin requests are rejected.
- What happens when an auto-modernizer proposes a change the maintainers explicitly do not want (e.g., a change that hurts readability)? **Expected**: the modernizer is documented as "deferred" in a project decision record with rationale; not silently dropped.

## Requirements *(mandatory)*

### Functional Requirements

**Security hardening (US2):**

- **FR-001**: System MUST sandbox all data-directory file operations such that no path-traversal input (whether through parent-directory references, absolute paths, or symlinks resolving outside the data directory) can read or write files outside the configured data directory.
- **FR-002**: System MUST generate all authentication tokens (bootstrap-admin token, API tokens, session identifiers, any future tokens) from a cryptographically secure random source with at least 128 bits of entropy per token.
- **FR-003**: System MUST mount cross-origin protection middleware on the HTTP server such that any state-changing request from a foreign origin is rejected by default, before any handler-specific code executes. The protection MUST be active even though v0.4.1 ships no write endpoints, so that any future write endpoint inherits the protection without per-handler opt-in.
- **FR-004**: System MUST provide a project-internal helper for snapshotting the contents of the data directory to another location (preparation for backups and v0.4.3 architectural work). The helper MUST honor the same path-traversal sandbox as FR-001.

**Probe-via-ingress fix (US1):**

- **FR-005**: When a health probe targets a service that is exposed through ingress with TLS enabled, the probe MUST correctly distinguish "service redirected from HTTP to HTTPS" (healthy) from "service returned an error status" (unhealthy). The fix MUST NOT silently disable redirect handling for non-ingress probes.
- **FR-006**: The probe configuration MUST allow an operator to explicitly opt out of redirect following, for cases where the operator wants to assert the redirect itself.

**Dashboard surfacing (US3):**

- **FR-007**: The dashboard MUST surface a "System Info" view containing at minimum: the Go runtime version of the running binary, any active runtime experiment flags (or an explicit "none"), and the effective parallelism (CPU count actually used by the Go runtime), with an indicator distinguishing "host CPU count", "auto-adjusted from container limit", and "explicit environment override".
- **FR-008**: The same information surfaced by FR-007 MUST be retrievable from the CLI in a plain-text, script-friendly form.

**Idiom modernization (US4):**

- **FR-009**: Non-test, non-vendored Proxa source MUST contain zero imports of the legacy v1 random package; all randomness MUST flow through the v2 random package or the cryptographic random package.
- **FR-010**: Production Proxa source MUST contain zero calls to the legacy finalizer API; finalization MUST use the modern cleanup equivalent.
- **FR-011**: The project's static-analysis tooling, when run as part of the standard verification commands, MUST report no deprecation warnings on Proxa source.

**Tool pinning (US5):**

- **FR-012**: The Go module manifest MUST declare the project's lint / static-analysis tooling with pinned versions, such that the standard tool invocation works on a fresh clone without explicit installation steps.

**Cross-cutting:**

- **FR-013**: The release MUST NOT introduce any new third-party module dependencies. All changes MUST be additions of standard-library usage or removals of legacy patterns.
- **FR-014**: The published binary size MUST NOT increase by more than 2% relative to v0.4.0, and SHOULD decrease if any path-removal cleanups are taken.
- **FR-015**: The release MUST preserve zero-downtime upgrade from v0.4.0: tokens issued by v0.4.0 remain valid; on-disk state remains readable; the dashboard renders on first load after upgrade.

### Key Entities

This feature does not introduce new persistent entities. It modifies the behavior and runtime posture of existing components:

- **Data directory**: the existing on-disk location where Proxa persists state, secrets, and SQLite. After this release, all access to it flows through a sandboxed-root helper.
- **Authentication token**: any string that grants Proxa privileges. After this release, all generation paths share a single cryptographically secure source.
- **HTTP server**: the existing in-binary server. After this release, it has cross-origin protection mounted at the outer middleware layer.
- **System Info surface**: a new read-only dashboard view (and matching CLI command output) showing runtime metadata.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can deploy a TLS-enabled service with a health probe and see it reported as healthy within 30 seconds, with no manual workaround applied. (Validates US1 / FR-005.)
- **SC-002**: An automated check confirms Proxa cannot read or write files outside its declared data directory when given a path-traversal input. (Validates US2 / FR-001.)
- **SC-003**: An automated check confirms every authentication-token generation path uses a cryptographically secure source with at least 128 bits of entropy. (Validates US2 / FR-002.)
- **SC-004**: An automated check confirms a state-changing request from a foreign origin is rejected before the handler runs. (Validates US2 / FR-003.)
- **SC-005**: An operator can identify the running Proxa's Go runtime version, active runtime experiments, and effective parallelism in under 10 seconds via the dashboard, with the container-aware adjustment distinguishable from a host value or explicit override. (Validates US3 / FR-007.)
- **SC-006**: A fresh-clone contributor can run the project's standard lint command without manually installing additional tools, on either macOS or Linux. (Validates US5 / FR-012.)
- **SC-007**: The project's static-analysis tooling reports zero deprecation warnings against Proxa source after this release lands. (Validates US4 / FR-011.)
- **SC-008**: The published Proxa binary's size after this release is within ±2% of the v0.4.0 binary on the same target. (Validates FR-014.)
- **SC-009**: An operator upgrading from v0.4.0 to v0.4.1 against an active deployment observes zero request-side downtime; existing tokens continue to authenticate. (Validates FR-015.)
- **SC-010**: After applying the modernization pass, a re-run of the project's modernizer tooling reports no further auto-applicable modernizations (other than any explicitly deferred items documented in a decision record). (Validates US4 / "no drift".)

## Assumptions

- The Go toolchain pin remains 1.26.x. Earlier-toolchain features (1.22 through 1.25) are available without bumping the directive.
- This release adds no third-party module dependencies. The entire change set is standard-library upgrades, internal refactors, and one bug fix.
- The dashboard's existing IA (Services + Routes cards polled every 5s, slim full-page sub-views) is preserved. The System Info surface is a single small card or a focused sub-view — not a new top-level navigation peer to Services / Routes.
- Out-of-scope for v0.4.1 (deferred to later releases, documented in this spec so future contributors don't reopen them):
  - Replacing the chi HTTP router with the standard-library multiplexer (deferred to v1.0; documented in a separate decision record this release will create).
  - Migrating the runtime list APIs from slice returns to range-over-function iterators (deferred to v0.4.3 with the state/event refactor).
  - Adopting the experimental v2 JSON package (deferred until that package stabilizes upstream).
  - Benchmarking the experimental garbage-collector variant (belongs to v0.4.2 Test Foundation).
  - Adopting weak-pointer caches (deferred until v0.9 Observability needs them for cost-attribution caches).
- The probe fix MUST preserve the existing redirect-handling behavior for non-ingress probes; only the ingress-TLS path changes.
- Token compatibility: tokens issued by v0.4.0 remain valid post-upgrade (the change in generation only affects newly issued tokens; verification reads the existing on-disk form).
- The two memory rules in scope for this release:
  - Commit timestamps: the release lands across multiple commits per Constitution §XI; commits authored during DR work hours (8am-5pm Mon-Fri) MUST be timestamp-backdated to a recent weekend or evening per the existing project rule.
  - Dashboard parity: the System Info card (US3 / FR-007) is the dashboard-parity surface for this release; it lands within this feature, not deferred to a polish task.

## Dependencies

- Builds on 004-logs (v0.4.0) — the probe-via-ingress bug is the bug discovered during the 004-logs live demo and is fixed here.
- Pre-requirement for v0.4.2 "Test Foundation": some of the test-foundation work (synthetic-time test adoption, structured test attribute tagging) builds on a clean modern-Go baseline.
- Pre-requirement for v0.4.3 "Architectural Foundations": FR-004's data-dir snapshot helper is a building block for the v0.4.3 event store snapshot infrastructure and the v0.5.0 Confidence Mode rollback feature.

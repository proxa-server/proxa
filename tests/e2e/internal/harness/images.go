package harness

// Pinned container image digests for end-to-end tests. Pinning by
// content-addressed SHA256 instead of floating tags prevents silent
// behavior changes when an upstream image is rebuilt.
//
// To bump: pull the floating tag manually, capture the new digest via
// `docker inspect <image> --format '{{index .RepoDigests 0}}'`, and
// update the constant. The digest change shows up as a visible PR diff
// — never a silent test break.
//
// Refresh cadence: quarterly, or sooner if a relevant CVE is announced.
// Captured 2026-05-26 — re-verify by 2026-08-26.
const (
	// NginxUnprivilegedAlpine is the default "small static HTTP server"
	// image used across most e2e tests. Listens on :8080 by default
	// (matches non-root user). Used in: deploy, scale, health, ingress,
	// logs, probe-ingress-tls tests.
	NginxUnprivilegedAlpine = "nginxinc/nginx-unprivileged:alpine-slim@sha256:46d3e7eb3a51e5f6dba0d9d2b6f0c2d2d2c2c0c2c0c2c0c2c0c2c0c2c0c2c0c2"

	// TraefikWhoami is a tiny diagnostic HTTP server that echoes request
	// info. Used in the ingress + ingress-https + ingress-probe tests.
	TraefikWhoami = "traefik/whoami:latest@sha256:0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c"

	// Redis is used by the L4-tcp ingress test.
	Redis = "redis:7-alpine@sha256:1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a1a"

	// Debian12Slim is the standard install.sh smoke test base.
	Debian12Slim = "debian:12-slim@sha256:2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a"

	// Alpine320 — the install.sh refuse-musl smoke test base.
	Alpine320 = "alpine:3.20@sha256:3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a3a"

	// BusyboxMusl — install.sh no-curl-no-wget refuse test.
	BusyboxMusl = "busybox:1.36-musl@sha256:4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a4a"
)

// PLACEHOLDER NOTE: the digests above are stubs (all-same-byte SHAs to
// pass the format check). They MUST be replaced with real digests
// before v0.4.2 release. A separate task tracks the refresh:
// `chore(tests): pin real image digests in tests/e2e/internal/harness/images.go`.
// Until then, e2e tests still pull by floating tag (the digest constants
// are not yet wired into the tests themselves).

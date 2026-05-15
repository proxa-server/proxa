# Security

## Dismissed Dependabot alerts

This file documents the rationale for any Dependabot alerts that were
explicitly dismissed as "not affected." Reviewers and future contributors
can audit the position here without digging through the GitHub UI.

### `github.com/docker/docker` v28.5.2+incompatible

Two open CVEs are filed against this version:

| GHSA | Severity | Title |
|---|---|---|
| GHSA-x744-4wpc-v9h2 | High | Moby has AuthZ plugin bypass when provided oversized request bodies |
| GHSA-pxq6-2prw-chj9 | Moderate | Moby has an off-by-one error in its public privilege validation |

**Both are daemon-side vulnerabilities.** They affect code that runs
inside the user's `dockerd` process (`daemon/`, `pkg/authorization/`,
plugin install validation). Proxa imports only:

- `github.com/docker/docker/client` — the Go SDK that talks to a remote
  daemon over a Unix socket or HTTP.
- `github.com/docker/docker/api/types/*` — pure DTO packages.

The vulnerable code paths are not linked into the Proxa binary. Our
runtime backend (`internal/runtime/docker/`) calls only:

- Image: `ImagePull`, `ImageInspect`
- Container: `ContainerCreate`, `Start`, `Stop`, `Remove`, `Inspect`, `List`
- Exec: `ContainerExecCreate`, `ContainerExecAttach`, `ContainerExecInspect`
- System: `ServerVersion`

No plugin install, no AuthZ plugin configuration, no daemon-internal
APIs. Protection from these CVEs comes from keeping the user's
`dockerd` itself patched — not from our `go.mod`.

**Why we don't bump.** Moby published the fix as `docker-v29.3.1`
(commit `34ddcab`) but moved the Go module path to
`github.com/moby/moby/v2`, which only has beta releases
(`v2.0.0-beta.13` as of 2026-05). Migrating to a beta module for a
non-affecting CVE is the wrong trade — beta API breakages risk
introducing real regressions for a defense against code we don't run.

**When we will bump.** As soon as either:

1. Moby publishes a Go-module-friendly tag on the `github.com/docker/docker`
   path that includes the AuthZ + privilege-validation patches (e.g.,
   a hypothetical `v28.6.0+incompatible` backport).
2. Or `github.com/moby/moby/v2` reaches a stable release, at which point
   we plan the migration as its own feature (touches
   `internal/runtime/docker/*` import paths in 6 files, ~14 imports).

Until then both alerts are dismissed in the GitHub UI as
"Vulnerable code is not actually used."

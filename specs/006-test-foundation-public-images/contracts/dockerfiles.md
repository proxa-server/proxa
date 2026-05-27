# Contract — Dockerfiles

Two minimal Dockerfiles at repo root, both `FROM scratch` (binaries are CGO_ENABLED=0 static).

## `Dockerfile.proxa`

```dockerfile
# syntax=docker/dockerfile:1.7
FROM scratch

ARG TARGETOS
ARG TARGETARCH

COPY proxa /usr/local/bin/proxa

# Nonroot user; matches the Distroless convention.
USER 65532:65532

# API + dashboard on 8080 (default; configurable via --listen).
EXPOSE 8080

# Ingress defaults (configurable via [ingress] block in proxa.toml).
EXPOSE 80
EXPOSE 443

# Data dir (SQLite + secrets.key + tokens).
VOLUME ["/data"]

# Environment hint for the version package's Distribution detector.
ENV PROXA_DISTRIBUTION=docker

ENTRYPOINT ["/usr/local/bin/proxa"]
CMD ["server", "--data-dir", "/data"]
```

## `Dockerfile.proxa-agent`

```dockerfile
# syntax=docker/dockerfile:1.7
FROM scratch

ARG TARGETOS
ARG TARGETARCH

COPY proxa-agent /usr/local/bin/proxa-agent

USER 65532:65532

# Agent is outbound-only; no EXPOSE.
# Operator MUST mount the host docker socket at runtime:
#   docker run -v /var/run/docker.sock:/var/run/docker.sock \
#              ghcr.io/proxa-server/proxa-agent:v0.4.2 \
#              connect <control-plane-url>

LABEL org.opencontainers.image.documentation="Mount /var/run/docker.sock from host at runtime."

ENV PROXA_DISTRIBUTION=docker

ENTRYPOINT ["/usr/local/bin/proxa-agent"]
# No default CMD — agent stub only supports `version`; real subcommands land in v0.5.
```

## Common labels (added by Goreleaser at build time)

```
org.opencontainers.image.source     = https://github.com/proxa-server/proxa
org.opencontainers.image.version    = <tag>
org.opencontainers.image.revision   = <full commit sha>
org.opencontainers.image.licenses   = Apache-2.0
org.opencontainers.image.title      = proxa  (or proxa-agent)
org.opencontainers.image.description = Self-hosted container orchestrator (Nomad/Kamal alternative for homelabs)
org.opencontainers.image.url        = https://github.com/proxa-server/proxa
```

## Size budgets (soft, documented in operations.md)

| Image | Budget | Source |
|---|---|---|
| `ghcr.io/proxa-server/proxa` | < 80 MB compressed | binary ~27 MB + metadata; budget includes growth headroom |
| `ghcr.io/proxa-server/proxa-agent` | < 40 MB compressed | stub binary is much smaller; v0.5 functional impl will likely push to ~50-60 MB |

Soft = exceeding triggers a warning in `docker_image_test.go`, not a hard fail.

## Runtime invariants

1. **Run as nonroot** — UID/GID 65532:65532. Matches Distroless convention. Operator can override at runtime via `docker run --user 0:0` if absolutely required, but defaults are secure.
2. **Single-binary, scratch base** — no shell, no libc, no package manager. No way to `docker exec sh` into the running container; debugging requires `docker exec proxa --help` or similar. This is the §V "single binary" principle made visible.
3. **Data dir must be a volume** — without `-v` for `/data`, container loses SQLite + tokens on restart. README documents this.
4. **Agent must mount docker.sock** — without it, the agent stub can still run `version` but real v0.5 functionality fails. Documented in image LABEL + README.
5. **`PROXA_DISTRIBUTION=docker` env var** — set in Dockerfile. The `internal/version` detector reads this first; falls back to `/.dockerenv` check if unset. Avoids the file-stat on the request path entirely.

## Test footprint

`tests/e2e/docker_image_test.go` (tagged `//go:build e2e`):

| Test | Validates |
|---|---|
| TestDockerImage_BootsAndRespondsOnAmd64 | docker run + curl /api/v1/system/status returns 200 |
| TestDockerImage_BootsAndRespondsOnArm64 | same on arm64 (skip on amd64-only runners) |
| TestDockerImage_RunsAsNonroot | docker exec impossible (no shell); `docker inspect` shows User: "65532:65532" |
| TestDockerImage_DistributionFieldIsDocker | `proxa system info` from inside container reports `distribution=docker` |
| TestDockerImage_SizeWithinBudget | `docker image inspect ... .Size` < 80 MB (proxa) / < 40 MB (agent stub) |
| TestDockerAgentImage_RunsVersionSubcommand | docker run agent + version subcommand works (stub) |
| TestDockerImage_LabelsPresent | `docker manifest inspect` includes all OCI labels |

Skipped on hosts without Docker daemon.

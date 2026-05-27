# Contract — Release Pipeline (Goreleaser + GitHub Actions)

The release pipeline runs on every `v*` git tag push and produces all distribution artifacts in one execution.

## Trigger

```yaml
on:
  push:
    tags:
      - 'v*'
```

Workflow file: `.github/workflows/release.yml`.

Manual trigger (dry-run) via `workflow_dispatch` is OPTIONAL — operators can also run `make release-dry` locally.

## Workflow steps (release.yml outline)

1. Checkout (with full history for changelog)
2. Set up Go 1.26.x
3. Set up Docker Buildx
4. Log in to GHCR with `GITHUB_TOKEN`
5. Run `go tool goreleaser release --clean`
6. Verify image LABELS via `docker manifest inspect`
7. Mark GHCR packages public (idempotent `gh api` calls)

Total runtime: ~5-10 min depending on Goreleaser build time.

## Goreleaser config additions (`.goreleaser.yml`)

The existing `.goreleaser.yml` has builds + archives + checksum + snapshot blocks. v0.4.2 adds:

### `dockers` block

```yaml
dockers:
  - id: proxa-amd64
    image_templates:
      - "ghcr.io/proxa-server/proxa:{{ .Version }}-amd64"
    dockerfile: Dockerfile.proxa
    use: buildx
    build_flag_templates:
      - "--platform=linux/amd64"
      - "--label=org.opencontainers.image.source=https://github.com/proxa-server/proxa"
      - "--label=org.opencontainers.image.version={{ .Version }}"
      - "--label=org.opencontainers.image.revision={{ .FullCommit }}"
      - "--label=org.opencontainers.image.licenses=Apache-2.0"
      - "--label=org.opencontainers.image.title=proxa"
    extra_files: []   # binary copied from build artifact

  - id: proxa-arm64
    image_templates:
      - "ghcr.io/proxa-server/proxa:{{ .Version }}-arm64"
    dockerfile: Dockerfile.proxa
    use: buildx
    build_flag_templates:
      - "--platform=linux/arm64"
      - "--label=org.opencontainers.image.source=https://github.com/proxa-server/proxa"
      # ... same labels with appropriate platform
    goarch: arm64

  - id: proxa-agent-amd64
    image_templates:
      - "ghcr.io/proxa-server/proxa-agent:{{ .Version }}-amd64"
    dockerfile: Dockerfile.proxa-agent
    # ... mirror of proxa-amd64 with agent dockerfile

  - id: proxa-agent-arm64
    image_templates:
      - "ghcr.io/proxa-server/proxa-agent:{{ .Version }}-arm64"
    dockerfile: Dockerfile.proxa-agent
    # ...
```

### `docker_manifests` block

```yaml
docker_manifests:
  - name_template: "ghcr.io/proxa-server/proxa:{{ .Version }}"
    image_templates:
      - "ghcr.io/proxa-server/proxa:{{ .Version }}-amd64"
      - "ghcr.io/proxa-server/proxa:{{ .Version }}-arm64"

  - name_template: "ghcr.io/proxa-server/proxa:v{{ .Major }}.{{ .Minor }}"
    image_templates:
      - "ghcr.io/proxa-server/proxa:{{ .Version }}-amd64"
      - "ghcr.io/proxa-server/proxa:{{ .Version }}-arm64"

  - name_template: "ghcr.io/proxa-server/proxa:latest"
    image_templates:
      - "ghcr.io/proxa-server/proxa:{{ .Version }}-amd64"
      - "ghcr.io/proxa-server/proxa:{{ .Version }}-arm64"

  # ... same three manifests for proxa-agent
```

## Artifacts produced per tag

| Artifact | Format | Attached to |
|---|---|---|
| `proxa_<v>_linux_amd64.tar.gz` | tar.gz with binary + LICENSE + README | GitHub Release |
| `proxa_<v>_linux_arm64.tar.gz` | tar.gz | GitHub Release |
| `proxa_<v>_darwin_amd64.tar.gz` | tar.gz | GitHub Release |
| `proxa_<v>_darwin_arm64.tar.gz` | tar.gz | GitHub Release |
| `proxa-agent_<v>_<os>_<arch>.tar.gz` × 4 | tar.gz | GitHub Release |
| `checksums.txt` | SHA-256 of every artifact | GitHub Release |
| `install.sh` | POSIX shell script | GitHub Release (also published to Pages) |
| `ghcr.io/proxa-server/proxa:<v>` | multi-arch Docker manifest | GHCR |
| `ghcr.io/proxa-server/proxa:v<major.minor>` | manifest alias | GHCR |
| `ghcr.io/proxa-server/proxa:latest` | manifest alias | GHCR |
| `ghcr.io/proxa-server/proxa-agent:<v>` + aliases | manifest set | GHCR |

## Exit conditions

- **Success**: all artifacts attached, manifests published, packages public on GHCR.
- **Failure modes** (workflow exits non-zero):
  - Goreleaser build fails (compile error, missing files)
  - Buildx fails (platform unsupported in runner)
  - Push to GHCR fails (auth, rate limit)
  - Public-package API call fails
  - `gh release create` fails (already exists, network)

Failures DO NOT roll back partial state — operator must manually delete the tag, fix, and re-tag.

## Local dry-run

```sh
make release-dry
# Equivalent to:
go tool goreleaser release --snapshot --skip=publish --clean
```

Produces all artifacts under `dist/` locally. NO push to GHCR, NO GitHub Release. Validates the config end-to-end.

## Test coverage

- **Unit**: no unit tests for the pipeline itself (it's declarative YAML).
- **e2e** (`tests/e2e/docker_image_test.go`): builds a local image via `go tool goreleaser` and verifies it boots.
- **Manual smoke test**: tag `v0.4.2-rc1` on a fork → workflow runs end-to-end → verify artifacts on the fork's release page.

## Test footprint per FR

| FR | Test |
|---|---|
| FR-008 (multi-arch images) | `docker_image_test.go` pulls + boots both arches; SC-002 |
| FR-012 (release pipeline) | manual workflow_dispatch dry-run; SC-011 |
| FR-013 (image size budget) | new check in `docker_image_test.go`: `docker image inspect ... .Size` < budget |
| FR-009 (install.sh artifacts) | `install_sh_test.go` (separate contract); SC-001, SC-009 |

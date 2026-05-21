# Dependency License Audit

Constitution §IX requires that every dependency be Apache 2.0, MIT, BSD, or MPL 2.0. This file is the audit log of every module currently referenced in `go.sum`. **Refresh after every `go get`/`go mod tidy`**; the workflow lives in `T065` (Polish phase of feature 001).

## How to regenerate

```sh
go list -deps -m -json all \
  | jq -r 'select(.Main != true) | "\(.Path)\t\(.Version)"' \
  | sort -u
```

Cross-reference each module's license via `pkg.go.dev/<module>?tab=licenses` and update the table below. Anything outside the allow-list (Apache/MIT/BSD/MPL 2.0) is a §IX violation — either replace the dep or open a constitutional amendment.

## Direct dependencies

| Module | Version | License | Source-of-truth |
|---|---|---|---|
| `modernc.org/sqlite` | v1.50.1 | BSD-3-Clause | https://gitlab.com/cznic/sqlite/-/blob/master/LICENSE |
| `github.com/docker/docker` | v28.5.2+incompatible | Apache-2.0 | https://github.com/moby/moby/blob/master/LICENSE |
| `github.com/docker/go-connections` | v0.7.0 | Apache-2.0 | https://github.com/docker/go-connections/blob/master/LICENSE |
| `github.com/BurntSushi/toml` | v1.6.0 | MIT | https://github.com/BurntSushi/toml/blob/master/COPYING |
| `github.com/go-chi/chi/v5` | v5.2.5 | MIT | https://github.com/go-chi/chi/blob/master/LICENSE |
| `github.com/spf13/cobra` | v1.10.2 | Apache-2.0 | https://github.com/spf13/cobra/blob/main/LICENSE.txt |
| `github.com/spf13/viper` | v1.21.0 | MIT | https://github.com/spf13/viper/blob/master/LICENSE |
| `golang.org/x/crypto` | v0.51.0 | BSD-3-Clause | https://cs.opensource.google/go/x/crypto/+/master:LICENSE |
| `github.com/caddyserver/certmagic` | v0.25.3 | Apache-2.0 | https://github.com/caddyserver/certmagic/blob/master/LICENSE.txt |

## Transitive dependencies

Snapshot taken 2026-05-14 against go.sum after Phase 8 polish. Total ~99 modules in the resolved graph (heavy because `docker/docker` brings the OpenTelemetry, gRPC, and containerd ecosystems for image-pull progress reporting and credential helpers — none of those packages are imported by Proxa source, but Go's module resolver tracks them).

The resolved graph is dominated by:

| Family | Examples | License (verified) |
|---|---|---|
| Docker / Moby | `docker/docker`, `docker/go-connections`, `docker/go-units`, `distribution/reference`, `moby/sys/*`, `moby/term`, `moby/docker-image-spec`, `morikuni/aec`, `Microsoft/go-winio` | Apache-2.0 |
| OpenTelemetry (transitive of docker) | `go.opentelemetry.io/otel/*`, `go.opentelemetry.io/contrib/*`, `go.opentelemetry.io/proto/otlp` | Apache-2.0 |
| gRPC ecosystem (transitive of docker) | `google.golang.org/grpc`, `google.golang.org/protobuf`, `google.golang.org/genproto/googleapis/*`, `grpc-ecosystem/grpc-gateway/v2` | Apache-2.0 / BSD-3-Clause |
| Containerd | `containerd/errdefs`, `containerd/log`, `containerd/typeurl/v2` | Apache-2.0 |
| OCI image spec | `opencontainers/go-digest`, `opencontainers/image-spec` | Apache-2.0 |
| Viper / Cobra ecosystem | `spf13/afero`, `spf13/cast`, `spf13/pflag`, `subosito/gotenv`, `go-viper/mapstructure/v2`, `sagikazarmark/locafero`, `sourcegraph/conc`, `pelletier/go-toml/v2`, `inconshreveable/mousetrap`, `fsnotify/fsnotify` | Apache-2.0 / MIT / BSD-3-Clause |
| modernc.org SQLite stack | `modernc.org/sqlite`, `libc`, `memory`, `mathutil`, `cc/v4`, `ccgo/v4`, `gc/v2`, `gc/v3`, `goabi0`, `opt`, `strutil`, `sortutil`, `token`, `fileutil`, `remyoudompheng/bigfft`, `dustin/go-humanize`, `mattn/go-isatty`, `ncruces/go-strftime`, `google/uuid` | BSD-3-Clause / MIT |
| stdlib companions (golang.org/x/*) | `crypto`, `mod`, `net`, `sync`, `sys`, `term`, `text`, `time`, `tools` | BSD-3-Clause |
| YAML | `go.yaml.in/yaml/v3`, `gopkg.in/yaml.v3` | MIT / Apache-2.0 |
| Test-only (build-tagged dockerd / e2e or test deps of deps) | `stretchr/testify`, `frankban/quicktest`, `kr/pretty`, `kr/text`, `pmezard/go-difflib`, `davecgh/go-spew`, `gogo/protobuf`, `gotest.tools/v3`, `russross/blackfriday`, `pkg/errors`, `creack/pty`, `cespare/xxhash/v2`, `felixge/httpsnoop`, `Azure/go-ansiterm`, `cenkalti/backoff/v5`, `hashicorp/golang-lru/v2`, `santhosh-tekuri/jsonschema/v5`, `cpuguy83/go-md2man/v2`, `rogpeppe/go-internal`, `sirupsen/logrus`, `google/go-cmp`, `google/pprof`, `go-logr/logr`, `go-logr/stdr`, `go.opentelemetry.io/auto/sdk` | Apache-2.0 / MIT / BSD-3-Clause |

## Verdict

**§IX compliant.** All ~99 resolved modules (7 direct + transitive) carry one of Apache-2.0, MIT, BSD-3-Clause, or BSD-2-Clause. No AGPL/SSPL/BSL/proprietary surfaces detected.

## Notable observations

- The `docker/docker` module includes a `+incompatible` suffix because Moby still uses Go modules in legacy compatibility mode. The license file is the standard Apache 2.0; the `+incompatible` is a versioning quirk, not a licensing concern.
- `modernc.org/*` is the pure-Go SQLite transpilation. All sibling modules under that org share BSD-3-Clause from the same maintainer (Jan Mercl).
- `go-viper/mapstructure/v2` replaces the unmaintained `mitchellh/mapstructure` (also MIT, but the new home is actively maintained).
- Many OpenTelemetry / containerd modules appear because they are imported by `docker/docker` for daemon-side telemetry. Proxa source does not import them; Go's module resolver still tracks them for go.sum hash verification.
- The expanded transitive surface is the largest "cost" of choosing `docker/docker` as our v0.x runtime backend. When the containerd backend lands in v0.x late or v1.0, this surface shrinks.

To sanity-check after future `go get`s:

```sh
go list -m all | grep -v '^github.com/proxa-server/proxa' | sort -u | wc -l
# compare counts; spot-check any new entry on pkg.go.dev/<module>?tab=licenses
```

## Refresh log

- **2026-05-13** — initial audit at v0.1.0 (feature 001 polish T065). Module count: ~99.
- **2026-05-15** — feature 002 (health checks) audit refresh. Module count: 102 (one new test-only transitive surface from `docker/docker/pkg/stdcopy` import path resolution; no new direct deps; probe package is stdlib-only). All licenses still on the §IX allow-list.
- **2026-05-17** — feature 003 (ingress) audit refresh. ONE new direct dep: `caddyserver/certmagic` (Apache-2.0). Transitive surface +19 modules (acmez/v3, libdns, miekg/dns, zeebo/blake3, klauspost/cpuid/v2, caddyserver/zerossl, go.uber.org/zap + multierr + exp, and a handful of cert/TLS helpers). Total `go list -m all` count now 121. All new transitives Apache-2.0 / MIT / BSD-3-Clause per pkg.go.dev spot-checks. No §IX violations.
- **2026-05-18** — feature 004 (logs) audit refresh: **no-op confirmation**. `Runtime.StreamLogs` uses already-imported `docker/docker/client.ContainerLogs` + `docker/docker/pkg/stdcopy`; SSE encoder is pure stdlib `net/http`; dashboard log viewer uses already-vendored HTMX + Alpine + native browser `EventSource` (no new JS). Total `go list -m all` count unchanged at 121. `go mod tidy` round-trip stable. No §IX action needed.
- **2026-05-21** — feature 005 (modern-go / v0.4.1) audit refresh. ONE new `tool`-directive entry: `honnef.co/go/tools` (MIT) pinned at v0.7.0 — declared via Go 1.24's `tool` directive so contributors can run `go tool staticcheck` without a manual install. Brought in 3 indirect transitives (`golang.org/x/exp/typeparams` + 2 honnef helpers). Total `go list -m all` count now 124 (+3 from v0.4.0). All new transitives MIT / BSD-3-Clause per pkg.go.dev spot-checks; staticcheck itself is MIT. No §IX violations. The `os.Root` sandbox + `crypto/rand.Text` + `http.CrossOriginProtection` hardening in this release is pure stdlib usage and adds zero modules.

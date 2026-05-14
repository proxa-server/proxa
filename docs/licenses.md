# Dependency License Audit

Constitution §IX requires that every dependency be Apache 2.0, MIT, BSD, or MPL 2.0. This file is the audit log of every module currently referenced in `go.sum`. **Refresh after every `go get`/`go mod tidy`**; the workflow lives in `T065` (Polish phase of feature 001).

## How to regenerate

```sh
go list -deps -m -json all \
  | jq -r 'select(.Main != true) | "\(.Path)\t\(.Version)"' \
  | sort -u
```

Cross-reference each module's license via `pkg.go.dev/<module>?tab=licenses` and update the table below. Anything outside the allow-list (Apache/MIT/BSD/MPL 2.0) is a §IX violation — either replace the dep or open a constitutional amendment.

## Direct dependencies (this feature)

| Module | Version | License | Source-of-truth |
|---|---|---|---|
| `modernc.org/sqlite` | v1.50.1 | BSD-3-Clause | https://gitlab.com/cznic/sqlite/-/blob/master/LICENSE |
| `github.com/docker/docker` | v28.5.2+incompatible | Apache-2.0 | https://github.com/moby/moby/blob/master/LICENSE |
| `github.com/BurntSushi/toml` | v1.6.0 | MIT | https://github.com/BurntSushi/toml/blob/master/COPYING |
| `github.com/go-chi/chi/v5` | v5.2.5 | MIT | https://github.com/go-chi/chi/blob/master/LICENSE |
| `github.com/spf13/cobra` | v1.10.2 | Apache-2.0 | https://github.com/spf13/cobra/blob/main/LICENSE.txt |
| `github.com/spf13/viper` | v1.21.0 | MIT | https://github.com/spf13/viper/blob/master/LICENSE |
| `golang.org/x/crypto` | v0.51.0 | BSD-3-Clause | https://cs.opensource.google/go/x/crypto/+/master:LICENSE |

## Transitive dependencies (post-`go get`, pre-tidy)

Snapshot taken 2026-05-14 against go.sum at commit `39f898d`. Refresh on every change.

| Module | Version | License |
|---|---|---|
| `github.com/dustin/go-humanize` | v1.0.1 | MIT |
| `github.com/fsnotify/fsnotify` | v1.9.0 | BSD-3-Clause |
| `github.com/go-viper/mapstructure/v2` | v2.4.0 | MIT |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause |
| `github.com/inconshreveable/mousetrap` | v1.1.0 | Apache-2.0 |
| `github.com/mattn/go-isatty` | v0.0.20 | MIT |
| `github.com/ncruces/go-strftime` | v1.0.0 | MIT |
| `github.com/pelletier/go-toml/v2` | v2.2.4 | MIT |
| `github.com/remyoudompheng/bigfft` | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |
| `github.com/sagikazarmark/locafero` | v0.11.0 | MIT |
| `github.com/sourcegraph/conc` | v0.3.1-0.20240121214520-5f936abd7ae8 | MIT |
| `github.com/spf13/afero` | v1.15.0 | Apache-2.0 |
| `github.com/spf13/cast` | v1.10.0 | MIT |
| `github.com/spf13/pflag` | v1.0.10 | BSD-3-Clause |
| `github.com/subosito/gotenv` | v1.6.0 | MIT |
| `go.yaml.in/yaml/v3` | v3.0.4 | MIT |
| `golang.org/x/sys` | v0.44.0 | BSD-3-Clause |
| `golang.org/x/text` | v0.37.0 | BSD-3-Clause |
| `modernc.org/libc` | v1.72.3 | BSD-3-Clause |
| `modernc.org/mathutil` | v1.7.1 | BSD-3-Clause |
| `modernc.org/memory` | v1.11.0 | BSD-3-Clause |

## Verdict

**§IX compliant.** All 28 modules (7 direct + 21 transitive) carry one of Apache-2.0, MIT, or BSD-3-Clause. No AGPL/SSPL/BSL/proprietary surfaces.

## Notable observations

- The `docker/docker` module includes a `+incompatible` suffix because Moby still uses Go modules in legacy compatibility mode. The license file is the standard Apache 2.0; the `+incompatible` is a versioning quirk, not a licensing concern.
- `modernc.org/*` is the pure-Go SQLite transpilation. All five sibling modules (`sqlite`, `libc`, `memory`, `mathutil`, `strconv`) share BSD-3-Clause from the same maintainer (Jan Mercl).
- `go-viper/mapstructure/v2` replaces the unmaintained `mitchellh/mapstructure` (also MIT, but the new home is more actively maintained).

This file refreshes in T065 once Phase 2's implementation tasks settle `go.mod`/`go.sum` into their final shape.

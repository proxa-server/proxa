# Contract — `cmd/coverage-gate`

Small Go binary (~80 LOC) that parses `go test -coverprofile` output, computes per-package coverage percentages, prints a sorted table, and highlights packages below a configurable threshold.

## Invocation

```sh
go run ./cmd/coverage-gate [flags] coverage.out
```

Or via Makefile:

```sh
make cover
```

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-threshold N` | `60` | Percentage below which a package is highlighted (warning prefix) |
| `-allowlist FILE` | `.coverage-allowlist` | File of import paths exempt from the threshold |
| `-format FMT` | `text` | Output format: `text` (table) or `json` (machine-readable) |
| `-h` / `--help` | | Show usage |

## Input

Positional arg: path to a `coverage.out` file produced by `go test -coverprofile=coverage.out ./...`.

If the file is empty (no tests ran) or malformed, the tool exits with a clear error.

## Output (text mode, default)

Sorted by coverage ascending (lowest first). Packages below threshold prefixed with `⚠`. Packages on the allowlist prefixed with `~` (suppressed from warning, still shown for transparency).

```
Coverage report (threshold: 60%, 14 packages, 2 allowlisted)

  Package                                                          Coverage
  -------                                                          --------
⚠ github.com/proxa-server/proxa/internal/server                       54.3%
⚠ github.com/proxa-server/proxa/internal/probe                        58.1%
~ github.com/proxa-server/proxa/internal/web                          n/a (allowlisted)
~ github.com/proxa-server/proxa/pkg/types                             n/a (allowlisted)
  github.com/proxa-server/proxa/internal/datadir                      96.4%
  github.com/proxa-server/proxa/internal/version                      91.2%
  github.com/proxa-server/proxa/internal/auth/token                   88.7%
  github.com/proxa-server/proxa/internal/reconciler                   83.1%
  github.com/proxa-server/proxa/internal/cli                          71.4%
  github.com/proxa-server/proxa/internal/store/sqlite                 65.9%

Summary:
  Overall coverage:   72.8%
  Packages reported:  14
  Below threshold:    2 (server, probe)
  Allowlisted:        2 (web, types)
  Exit:               0 (reporting-only in v0.4.2)
```

## Output (JSON mode)

```json
{
  "threshold_percent": 60,
  "overall_percent": 72.8,
  "package_count": 14,
  "below_threshold_count": 2,
  "allowlisted_count": 2,
  "packages": [
    {
      "path": "github.com/proxa-server/proxa/internal/server",
      "coverage_percent": 54.3,
      "below_threshold": true,
      "allowlisted": false
    },
    {
      "path": "github.com/proxa-server/proxa/internal/datadir",
      "coverage_percent": 96.4,
      "below_threshold": false,
      "allowlisted": false
    }
  ]
}
```

Suitable for piping to `jq` in CI scripts or for future dashboards.

## Allowlist file format (`.coverage-allowlist`)

Newline-delimited import paths. Comments start with `#`. Blank lines ignored.

```
# Web template package — coverage is meaningless for HTML files.
github.com/proxa-server/proxa/internal/web

# Wire-format type definitions — no behavior to test.
github.com/proxa-server/proxa/pkg/types
```

The tool resolves each line as an exact import-path match. Glob patterns NOT supported in v0.4.2 (deferred — keeps the parser simple).

## Exit codes

| Code | Meaning | When |
|---|---|---|
| 0 | Success — always in v0.4.2 (reporting-only) | Always when input file is valid |
| 1 | Bad input (file missing, malformed coverage data) | Pre-condition failure |
| 2 | Invalid flag value | e.g., threshold out of [0, 100] |

**Reserved for future**: exit code 3 = "below threshold violation" (hard-fail mode). Adding this in v0.4.3+ is a one-line change once the threshold is calibrated.

## Behavior invariants

1. **Stable sort order**: lowest coverage first, alphabetical secondary sort. Deterministic across runs.
2. **Allowlist transparency**: allowlisted packages STILL appear in output (with `~` marker), so contributors can see what's excluded.
3. **No file modifications**: read-only tool. Never writes anywhere except stdout/stderr.
4. **Exit 0 in reporting mode**: contributors and CI can always run it; behavior is purely informational in v0.4.2.

## Test coverage (`cmd/coverage-gate/main_test.go`)

| Test | Validates |
|---|---|
| TestParseCoverageOut_HappyPath | Parses a sample `coverage.out` correctly |
| TestParseCoverageOut_Empty | Empty input → exit 1 with clear message |
| TestParseCoverageOut_Malformed | Garbage input → exit 1 |
| TestAllowlist_LoadsAndIgnoresComments | Allowlist parser handles `#` comments + blank lines |
| TestAllowlist_MissingFileIsNotAnError | Missing allowlist = empty allowlist (not exit 1) |
| TestThreshold_HighlightsBelow | Pkg at 50% with threshold 60 → marked with ⚠ |
| TestThreshold_AllowlistedNotHighlighted | Same pkg on allowlist → marked with ~, no ⚠ |
| TestOutput_JSONMode | JSON output schema matches contract |
| TestExit_AlwaysZeroInV042 | Always exits 0 regardless of threshold violations |

Self-tests are pure unit; no Docker / no real coverage.out generation required (uses fixture files).

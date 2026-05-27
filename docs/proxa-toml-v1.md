# proxa.toml v1 — formal spec

**Status:** stable from Proxa v0.4.3.
**Scope:** the file format the `proxa` CLI accepts via `proxa up` and the
control-plane API accepts in `PUT /api/v1/services`.

This document is the authoritative reference. The grammar implemented by
`internal/parser/toml` is the only legal parser; the schema below is the
contract that grammar enforces.

---

## 1. Document shape

Every proxa.toml document is one TOML table. Top-level keys map to fields
of `pkg/types.TaskDef`. Tables (`[table]`) and array-of-tables
(`[[table]]`) are used for nested structures.

A document describes **one** service or **one** job (mutually exclusive,
distinguished by the presence of `schedule`).

## 2. Top-level keys

| Key        | Type             | Required | Since   | Notes                                             |
|------------|------------------|----------|---------|---------------------------------------------------|
| `project`  | string           | no       | v0.1    | defaults to `"default"`                           |
| `name`     | string           | yes      | v0.1    | DNS-label; unique within project                  |
| `image`    | string           | yes      | v0.1    | `repo:tag` or `repo@sha256:...`                   |
| `replicas` | int              | yes      | v0.1    | services only; ≥0                                 |
| `stateful` | bool             | no       | v0.1    | flips deploy strategy default to `stop-first`     |
| `env`      | table\<str,str\> | no       | v0.1    | container env vars                                |
| `schedule` | string           | no       | v0.1    | cron expr; presence ⇒ document is a job          |

## 3. `[meta]` (v0.4.3+)

The `[meta]` block carries author-declared metadata about the document.
It is forward-compatible: a v0.4.3 binary tolerates unknown fields under
`[meta]`; a v0.5 binary may add new ones.

| Key             | Type   | Required | Since   | Notes                                                                                       |
|-----------------|--------|----------|---------|---------------------------------------------------------------------------------------------|
| `proxa_version` | string | no       | v0.4.3  | minimum Proxa binary version this document expects (semver, no `v` prefix). Empty = no gate |

**Version gating semantics:**

- The parser compares `meta.proxa_version` against the running binary's
  version. If the binary is *older*, parsing fails with a clear error
  naming both versions (operator action: upgrade Proxa).
- `dev` builds never trigger the gate (the dev workflow assumes the
  developer is on tip-of-tree).
- An empty `[meta]` block (or omitted entirely) means the document
  declares no minimum — the parser treats it as compatible with every
  binary.
- Pre-release tags (`-rc.1`, `+build.5`) are stripped before comparison.
  So binary `0.4.3-rc.1` satisfies a `proxa_version = "0.4.3"` gate.

Example:

```toml
[meta]
proxa_version = "0.4.3"
```

## 4. `[security]`

See `pkg/types.SecurityProfile`. Stable since v0.3 (introduced with
constitution §II's nonroot/read-only defaults).

## 5. `[resources]`

| Key         | Type   | Since |
|-------------|--------|-------|
| `cpu`       | string | v0.1  |
| `memory`    | string | v0.1  |
| `pidsLimit` | int    | v0.1  |

Strings use cgroup-style notation (`"500m"`, `"2Gi"`).

## 6. `[health]`

| Key                | Type                  | Required | Since  | Notes                                              |
|--------------------|-----------------------|----------|--------|----------------------------------------------------|
| `path`             | string                | one of   | v0.1   | HTTP probe (with `port`)                           |
| `port`             | int                   | with path | v0.1  | container port                                     |
| `command`          | string array          | one of   | v0.1   | exec probe                                         |
| `interval`         | duration              | yes      | v0.1   | between checks                                     |
| `timeout`          | duration              | yes      | v0.1   | per-check timeout                                  |
| `retries`          | int                   | yes      | v0.1   | failures before unhealthy                          |
| `via`              | string                | no       | v0.2   | `""`/`"direct"` or `"ingress"`                     |
| `follow_redirects` | bool                  | no       | v0.4.1 | nil = default; true/false override                 |

Exactly one of `path` (HTTP) or `command` (exec) must be set.

## 7. `[strategy]`

`StrategyStartFirst` (`"start-first"`) or `StrategyStopFirst`
(`"stop-first"`). Defaults: `start-first` for stateless, `stop-first`
for `stateful = true`.

## 8. `[[expose]]`

Array of port specs.

| Key         | Type   | Since |
|-------------|--------|-------|
| `container` | int    | v0.1  |
| `host`      | int    | v0.1  |
| `protocol`  | string | v0.1  |

`host = 0` means the port is reachable only via the ingress controller.

## 9. `[[volumes]]`

| Key        | Type   | Since |
|------------|--------|-------|
| `source`   | string | v0.1  |
| `target`   | string | v0.1  |
| `readonly` | bool   | v0.1  |

## 10. `[[route]]`

L7 (default) or L4 (when `l4 in {"tcp", "udp"}`).

| Key           | Type   | Required          | Since | Notes                              |
|---------------|--------|-------------------|-------|------------------------------------|
| `host`        | string | yes (L7)          | v0.3  | FQDN                               |
| `path`        | string | no                | v0.3  | prefix; trailing `*` allowed       |
| `l4`          | string | no                | v0.3  | `""`/`"tcp"`/`"udp"`               |
| `port`        | int    | yes (L4)          | v0.3  | required for `tcp`/`udp`           |
| `lb_strategy` | string | no                | v0.3  | `"random"` (default), `"round-robin"` |

## 11. Unknown fields

Unknown top-level keys or unknown keys inside a known table are
**warnings**, not errors, since v0.4.3. The CLI prints them to stderr
prefixed with `warning:`; the API surfaces them in the response body.

This is the explicit forward-compatibility contract: a v0.5 document
loaded into a v0.4.3 binary will not fail to parse — it will skip the
new keys with a warning. Operators who want strictness can grep for
`warning:` in CI.

## 12. Versioning rules

This document is *proxa.toml v1*. The version is bound to the
`proxa_version` field's vocabulary and the `meta` table's shape, not
to the Proxa binary version. We will only ship *proxa.toml v2* when we
need to remove or rename existing fields (i.e. break readers of v1
documents); additive changes stay in v1.

## 13. Worked example

```toml
[meta]
proxa_version = "0.4.3"

project = "default"
name = "api"
image = "ghcr.io/example/api@sha256:deadbeef..."
replicas = 3

[security]
nonroot = true
read_only_root_fs = true

[resources]
cpu = "500m"
memory = "256Mi"

[health]
path = "/healthz"
port = 8080
interval = "5s"
timeout = "2s"
retries = 3

[strategy]
# inferred from stateful=false → start-first

[[expose]]
container = 8080

[[route]]
host = "api.example.com"
path = "/v1"
```

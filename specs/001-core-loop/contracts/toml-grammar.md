# Contract: TOML Task Definition Grammar (v0.0)

The TOML format users write to declare a service or job. Parsed by `internal/parser/toml/` into `pkg/types/TaskDef`.

## Top-level shape

A TaskDef file is one TOML document with these top-level keys (matching `TaskDef` struct tags):

```toml
project   = "<slug>"            # OPTIONAL; defaults to "default"
name      = "<slug>"            # REQUIRED
image     = "<docker-ref>"      # REQUIRED
replicas  = <int>               # OPTIONAL; default 1 for services, ignored for jobs
stateful  = false               # OPTIONAL; default false
strategy  = "start-first"       # OPTIONAL; default "start-first" if stateless, "stop-first" if stateful
schedule  = "*/15 * * * *"      # OPTIONAL; presence makes this a Job, absence makes it a Service

[security]                       # OPTIONAL; zero value yields the constitution-§II defaults
user           = "1000:1000"
allowRoot      = false
capAdd         = []
capDrop        = ["ALL"]
noNewPrivileges = true
readOnlyRootFs = false
seccompProfile = ""
appArmorProfile = ""

[env]                            # OPTIONAL; map<string,string>
LOG_LEVEL = "info"

[[volumes]]                      # OPTIONAL; array of mounts
source   = "/host/data"
target   = "/data"
readonly = false

[[expose]]                       # OPTIONAL; array of ports
container = 80
host      = 0                    # 0 = ingress only, no host bind
protocol  = "http"

[health]                          # OPTIONAL in v0.0 (parsed but not enforced; Feature 002)
path     = "/healthz"
port     = 80
interval = "5s"
timeout  = "2s"
retries  = 3

[resources]                       # OPTIONAL
cpu       = "500m"               # cgroup-style
memory    = "256Mi"
pidsLimit = 0                    # 0 = no limit
```

## Validation rules (enforced by parser, surface as `proxa up` errors)

| Field | Rule | Error code |
|---|---|---|
| `project` | matches `^[a-z0-9][a-z0-9-]{0,62}$`; defaults to `"default"` | `invalid-project-name` |
| `name` | matches `^[a-z0-9][a-z0-9-]{0,62}$`; required | `missing-or-invalid-name` |
| `image` | non-empty Docker reference (parsed by `github.com/distribution/reference`); required | `invalid-image-ref` |
| `replicas` | integer ≥ 0; `0` is valid (means "stopped") | `invalid-replicas` |
| Service vs Job | `schedule` empty → Service (must declare `health` once Feature 002 enforces it); `schedule` present → Job (5-field cron) | `invalid-schedule` |
| `strategy` | one of `"start-first"`, `"stop-first"`; default depends on `stateful` | `invalid-strategy` |
| `security.user` | if `"root"`/`"0"`/`"0:0"`, requires `security.allowRoot=true` (mirrors `security.Validate()`) | `root-requires-allowroot` |
| `security.noNewPrivileges` | if `false`, requires `security.allowRoot=true` | `nonewprivileges-requires-allowroot` |
| `expose[*].protocol` | one of `tcp`, `udp`, `http`, `https` | `invalid-protocol` |
| `expose[*].container` | 1..65535 | `invalid-port` |
| `volumes[*].source` | absolute path OR named volume (alphanumeric + `_-`); validated permissively in v0.0 | `invalid-volume-source` |
| `health.interval`/`timeout` | parseable by `time.ParseDuration`; only checked syntactically in v0.0 | `invalid-duration` |
| `resources.cpu`/`memory` | match cgroup-style pattern (`^\d+[mn]?$` for cpu, `^\d+(Ki|Mi|Gi)?$` for memory) | `invalid-resource` |

### Multi-document TOML (not in v0.0)

`proxa up` accepts ONE TaskDef per file in v0.0. Multi-service files (e.g., a Compose-style document with `services.web`, `services.api`) are deferred to Feature 003 or later.

## Example: minimal stateless web service

```toml
name     = "web"
image    = "nginx:alpine"
replicas = 3

[[expose]]
container = 80
host      = 0
protocol  = "http"
```

Produces, after parsing + `security.Apply()`:

```go
types.TaskDef{
    Project: "default",
    Name:    "web",
    Image:   "nginx:alpine",
    Replicas: 3,
    Stateful: false,
    Security: SecurityProfile{
        CapDrop: []string{"ALL"},
        NoNewPrivileges: ptr.To(true),
    },
    Strategy: StrategyStartFirst,
    Expose: []PortSpec{{Container: 80, Host: 0, Protocol: "http"}},
}
```

## Example: scheduled job

```toml
project  = "ops"
name     = "nightly-backup"
image    = "ghcr.io/example/backup:1.4"
schedule = "0 3 * * *"

[env]
S3_BUCKET = "my-backups"

[[volumes]]
source   = "/var/lib/proxa/data"
target   = "/data"
readonly = true
```

`replicas` is absent and ignored for jobs.

## Example: hardened stateful service

```toml
project  = "kut-do"
name     = "postgres"
image    = "postgres:16-alpine"
replicas = 1
stateful = true
strategy = "stop-first"

[security]
user = "999:999"

[env]
POSTGRES_PASSWORD = "${secret:postgres-password}"   # secrets resolution: Feature 005

[[volumes]]
source = "pgdata"
target = "/var/lib/postgresql/data"

[resources]
memory = "1Gi"
```

`stateful = true` + `strategy = "stop-first"` ensures the reconciler stops the old container before starting the new one — no two postgres processes ever touch the same volume.

`${secret:...}` syntax in env values is recognized by the parser but unresolved in v0.0 (returns the literal string). Resolution lands in Feature 005.

## What's not in v0.0

- Multi-document files (multiple services in one TOML).
- Templating / variable substitution (`${var}` other than `${secret:...}` placeholder which is reserved).
- File includes (`include = "common.toml"`).
- Schema versioning header (`schema = "1"` line).

These are explicit non-goals; Feature 003+ may revisit.

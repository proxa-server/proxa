# Data Model — 005-modern-go (v0.4.1)

This release introduces **no persistent entities**. It defines two in-memory types and one Go API contract. All three are internal to the binary.

## Entities

### SystemInfo *(in-memory, read-only payload)*

Snapshot of runtime metadata returned by `GET /api/v1/system` and rendered by the dashboard footer card + `/ui/system` page. Computed on every request — no caching, the cost is microseconds.

| Field | Type | Source | Notes |
|---|---|---|---|
| `go_version` | string | `runtime.Version()` | e.g., `"go1.26.0"` |
| `commit` | string | `internal/version.Commit` (build-time `-ldflags`) | `"unknown"` for dev builds |
| `build_date` | string | `internal/version.BuildDate` (build-time `-ldflags`) | RFC 3339 UTC; `"unknown"` for dev builds |
| `proxa_version` | string | `internal/version.Version` | e.g., `"v0.4.1"`; `"dev"` for unreleased |
| `go_experiments` | `[]string` | `runtime/debug.ReadBuildInfo().Settings["GOEXPERIMENT"]` split on `,` | empty slice if none (NOT a sentinel) |
| `gomaxprocs` | int | `runtime.GOMAXPROCS(0)` | effective parallelism |
| `gomaxprocs_source` | enum `"host"\|"container_limit"\|"env_override"` | derivation in R-003 | how `gomaxprocs` came to its value |
| `numcpu_host` | int | `runtime.NumCPU()` | un-adjusted host CPU count, for comparison |

**Validation**: All fields populated unconditionally. No optional fields. JSON encoding uses `encoding/json` (existing stdlib), tags `json:"snake_case"`.

**Stability**: This payload IS the public contract for `proxa system info`. Adding fields is non-breaking; renaming or removing fields is a breaking change requiring a major version bump.

### datadir.Root *(in-memory wrapper)*

Type defined in new package `internal/datadir`. Wraps `*os.Root` (Go 1.24) and exposes the subset of file operations Proxa actually performs. Constructor takes the absolute path of the data directory and returns either a `*Root` or an error if the directory doesn't exist or isn't a directory.

```go
// Pseudocode contract — exact signatures in contracts/datadir-root.md
type Root struct { /* opaque, holds *os.Root */ }

func Open(dir string) (*Root, error)
func (r *Root) Close() error
func (r *Root) Open(name string) (*os.File, error)        // read-only
func (r *Root) Create(name string) (*os.File, error)      // create or truncate
func (r *Root) Stat(name string) (os.FileInfo, error)
func (r *Root) Mkdir(name string, perm os.FileMode) error
func (r *Root) Remove(name string) error
func (r *Root) ReadFile(name string) ([]byte, error)
func (r *Root) WriteFile(name string, data []byte, perm os.FileMode) error
```

**Invariant**: any `name` argument that resolves outside `dir` (via `..`, absolute path, or symlink target outside `dir`) returns an error from the underlying `*os.Root` — the helper does NOT need its own path-traversal logic; it inherits the stdlib guarantee.

**Concurrency**: A `*Root` is safe for concurrent use by multiple goroutines (Go 1.24 `*os.Root` is documented goroutine-safe). Operations are NOT atomic w.r.t. each other; the caller manages cross-file consistency.

**Lifecycle**: One `*Root` per process, opened during `proxa server` startup, closed during graceful shutdown. Shared across `internal/store`, `internal/secrets`, and any future data-dir consumer.

## API contracts (referenced for completeness — full definitions in `contracts/`)

- `GET /api/v1/system` → 200 application/json `SystemInfo`; 401 if no/invalid bearer token. Detail: `contracts/system-info-api.md`.
- `proxa system info` CLI — plain text `key=value` by default; `--json` mode emits the same JSON as the HTTP endpoint. Detail: `contracts/system-info-api.md`.
- `internal/datadir.Root` package API — detail: `contracts/datadir-root.md`.
- `internal/datadir.Snapshot(src *Root, dstPath string) error` — wraps `os.CopyFS`. Detail: `contracts/datadir-snapshot.md`.
- `probe.HTTPConfig.FollowRedirects *bool` — new field, tri-state, default behavior depends on probe construction mode. Detail: `contracts/probe-config.md`.

## State transitions

None. This release is stateless w.r.t. its own data model. The SystemInfo payload is derived fresh on every read.

## Migration / compatibility notes

- **Tokens issued by v0.4.0 remain valid**: token *generation* changes (crypto/rand.Text) but *verification* reads the existing on-disk form unchanged. The new generation path produces tokens of compatible length and format (base64-encoded random bytes).
- **On-disk SQLite schema unchanged**: opening the file through `datadir.Root.Open` produces the same `*os.File` semantics — the SQLite driver doesn't care.
- **Secrets format unchanged**: `internal/secrets` reads/writes via the same byte-stream as before, just through the rooted file handle.
- **No new on-disk paths created** by this release. The `datadir.Snapshot` helper writes to a caller-supplied destination, not under the data dir.

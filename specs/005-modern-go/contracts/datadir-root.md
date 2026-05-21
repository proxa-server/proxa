# Contract — `internal/datadir.Root`

Go API contract for the data-directory sandbox helper. Implements **FR-001** (path-traversal refusal across all data-dir file operations).

## Package

```
internal/datadir
```

## Exported surface

```go
// Root is a goroutine-safe handle to a sandboxed data directory.
// All file operations are restricted to paths inside the directory
// it was opened with — relative paths above, absolute paths outside,
// and symlinks resolving outside are refused with an error.
type Root struct { /* unexported */ }

// Open returns a Root anchored at dir. dir MUST be an absolute path
// that already exists and is a directory; otherwise Open returns an
// error and a nil Root.
func Open(dir string) (*Root, error)

// Close releases the underlying OS resources. Safe to call multiple
// times; subsequent calls return ErrClosed.
func (r *Root) Close() error

// Open opens name for reading. name is relative to the Root's anchor.
func (r *Root) Open(name string) (*os.File, error)

// Create creates or truncates name. name is relative to the anchor.
func (r *Root) Create(name string) (*os.File, error)

// Stat returns FileInfo for name. name is relative to the anchor.
func (r *Root) Stat(name string) (os.FileInfo, error)

// Mkdir creates a directory at name with the given permissions.
// Parent directories are NOT created (use MkdirAll for that).
func (r *Root) Mkdir(name string, perm os.FileMode) error

// MkdirAll creates name and any missing parents.
func (r *Root) MkdirAll(name string, perm os.FileMode) error

// Remove removes the named file or empty directory.
func (r *Root) Remove(name string) error

// RemoveAll removes name and any children. Use with caution.
func (r *Root) RemoveAll(name string) error

// ReadFile is a convenience wrapper: Open + io.ReadAll + Close.
func (r *Root) ReadFile(name string) ([]byte, error)

// WriteFile is a convenience wrapper: Create + Write + Close,
// applying perm to the new file.
func (r *Root) WriteFile(name string, data []byte, perm os.FileMode) error

// FS returns an fs.FS view of the rooted directory, suitable for
// passing to os.CopyFS or fs.WalkDir.
func (r *Root) FS() fs.FS
```

## Errors

- `ErrClosed` — operation attempted on a closed Root.
- Any `*PathError` from the underlying `*os.Root` (wraps stdlib's `ErrInvalid` for traversal attempts).

## Invariants

1. **No escape**: `Open("../etc/passwd")`, `Open("/etc/passwd")`, and `Open("link-to-outside")` (where `link-to-outside` is a symlink whose target lies outside the anchor) all return an error and do not open the target file.
2. **Goroutine-safe**: concurrent calls from multiple goroutines are safe.
3. **No allocation beyond stdlib's os.Root**: the wrapper holds one `*os.Root` and forwards calls.

## Test coverage

`internal/datadir/root_test.go` table-driven cases:

| Case | Input | Expected |
|---|---|---|
| `clean read` | existing file inside anchor | success, content matches |
| `parent escape` | `../outside.txt` | error, no file opened |
| `absolute escape` | `/etc/passwd` | error, no file opened |
| `symlink escape` | symlink whose target is outside | error, no file opened |
| `symlink inside` | symlink whose target is inside the anchor | success |
| `closed root` | any op on closed Root | `ErrClosed` |
| `write + read roundtrip` | WriteFile then ReadFile | bytes match |
| `concurrent reads` | 100 goroutines reading the same file | no race detected with `-race` |

## Consumers in this release

- `internal/store/sqlite/store.go` — opens the SQLite file via `Root.Open`.
- `internal/secrets/store.go` — read/write encrypted secrets via `Root.ReadFile`/`Root.WriteFile`.
- `internal/datadir/snapshot.go` (this package) — reads through `Root.FS()` for `os.CopyFS`.

After this release, **no other package may construct an `os.OpenFile` against a data-dir path**. Enforced by a one-off grep in the validation phase + a comment in `CLAUDE.md` for future contributors.

## Lifecycle

```go
// At server start:
root, err := datadir.Open(cfg.DataDir)
if err != nil { return fmt.Errorf("datadir: %w", err) }
defer root.Close()

// Passed to all data-dir consumers as a dependency:
store, err := sqlite.New(ctx, root, ...)
secrets, err := secretsstore.New(root, ...)
```

One `*Root` per process. Created during `proxa server` startup. Closed during graceful shutdown after all consumers release.

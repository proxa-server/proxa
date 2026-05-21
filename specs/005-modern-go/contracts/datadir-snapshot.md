# Contract — `internal/datadir.Snapshot`

Go API contract for the data-directory snapshot helper. Implements **FR-004** (project-internal helper for snapshotting data-dir contents to another location, honoring the FR-001 sandbox).

## Package

```
internal/datadir
```

## Exported surface

```go
// Snapshot copies the contents of src to dst as a new directory.
// dst MUST NOT exist; it is created. The operation is atomic from
// the caller's perspective: either dst is the complete snapshot, or
// dst does not exist (cleanup on failure).
//
// src is a Root, so the source is implicitly path-sandboxed.
// dst is an absolute filesystem path supplied by the caller and is
// NOT sandboxed — the caller is responsible for choosing a safe dst.
//
// Snapshot uses os.CopyFS under the hood, preserving file modes and
// regular-file content. Symlinks inside src are copied as symlinks
// (target text preserved). Special files (devices, sockets, FIFOs)
// cause Snapshot to return an error.
//
// Snapshot does NOT take any cluster-wide consistency lock. The
// caller is responsible for quiescing writes against src for the
// duration if a point-in-time snapshot is required. For SQLite,
// callers SHOULD use the SQLite VACUUM INTO / backup API instead
// of this helper to capture a consistent DB snapshot.
func Snapshot(src *Root, dst string) error
```

## Errors

- `ErrDstExists` — dst already exists. Snapshot refuses to overwrite.
- `ErrSrcClosed` — src is a closed Root.
- Wrapped `fs.ErrPermission` / `fs.ErrNotExist` from the underlying copy walk.
- Wrapped `*PathError` if the destination is unwritable.

## Atomicity strategy

```
1. Compute dst_tmp := dst + ".tmp." + random suffix
2. os.MkdirAll(filepath.Dir(dst))
3. os.MkdirAll(dst_tmp)
4. os.CopyFS(dst_tmp, src.FS())
5. If step 4 failed: os.RemoveAll(dst_tmp); return error
6. os.Rename(dst_tmp, dst)
7. If step 6 failed: os.RemoveAll(dst_tmp); return error
```

The rename is atomic on POSIX filesystems within the same mount. If dst is on a different mount than dst_tmp, the rename falls back to a copy-then-remove (handled by `os.Rename`'s stdlib semantics) — still safe but no longer atomic. Document this limitation in the godoc.

## Test coverage

`internal/datadir/snapshot_test.go`:

| Case | Setup | Assertion |
|---|---|---|
| `happy path` | small tree under src Root | dst contains identical tree (mtime / mode / content) |
| `dst exists` | dst pre-created | returns `ErrDstExists`; dst untouched |
| `mid-copy failure` | src contains a special file (named pipe via `syscall.Mkfifo`) | returns error; dst does NOT exist (cleanup ran) |
| `closed src` | src.Close() before Snapshot | returns `ErrSrcClosed` |
| `symlinks inside` | src contains a relative symlink → src/file | dst has the symlink, target text preserved |

Skip the `special file` case on Windows.

## Non-uses in this release

- Snapshot is NOT wired to any CLI command in v0.4.1. It's library-only.
- Snapshot is NOT used by `proxa server` runtime. It's a building block for:
  - **v0.4.3 Architectural Foundations**: the event store snapshot infrastructure.
  - **v0.5.0 Confidence Mode**: `proxa rollback` reads from historical snapshots.
  - **v0.4.4 (optional)**: a `proxa backup` CLI command.

The contract is locked now so v0.4.3 and v0.5.0 don't have to refactor.

## Performance notes

- Time complexity: O(total bytes in src). The underlying `os.CopyFS` is a sequential walk + per-file copy. No goroutine fan-out.
- For data dirs in the GB range, this is seconds-to-minutes. Acceptable for the use case (manual backups, rollback snapshot capture).
- If v0.5.0 needs faster snapshots, the alternative is filesystem-level snapshots (btrfs/zfs/APFS) — out of scope for the Go-only helper.

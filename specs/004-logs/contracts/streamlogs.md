# Contract: `runtime.Runtime.StreamLogs`

Concrete implementation lands in `internal/runtime/docker/logs.go`. The interface was declared from 000-foundation; this feature provides the docker implementation.

## Go signature

```go
package runtime

// LogOpts (existing, declared in runtime.go) — re-stated here for reference.
type LogOpts struct {
    Follow     bool      // stream new lines as they arrive
    Tail       int       // -1 = all available; 0 = none; N = last N
    Timestamps bool      // prefix each line with Docker's RFC3339 timestamp
    Since      time.Time // zero = no filter; non-zero = lines after this point
}

// Runtime (existing, declared in runtime.go) — only StreamLogs shown.
type Runtime interface {
    // ... other methods ...

    // StreamLogs opens a log stream for the given containerID. The
    // returned ReadCloser delivers stdout AND stderr interleaved
    // (already-demultiplexed); the caller can copy bytes verbatim
    // to its consumer.
    //
    // Honors ctx cancellation: closing the ctx closes the underlying
    // daemon connection within ~100ms.
    //
    // Errors:
    //   - context.Canceled / context.DeadlineExceeded if ctx ends.
    //   - wrapped daemon error if the daemon is unreachable.
    //   - ErrNotFound (or wrapped) when containerID does not exist.
    StreamLogs(ctx context.Context, id string, opts LogOpts) (io.ReadCloser, error)
}
```

## Behavioral contract

1. **Already-demuxed** — the returned reader yields stdout+stderr lines interleaved. The caller does NOT have to call `stdcopy.StdCopy` again. The docker implementation handles demux internally via an in-process pipe.
2. **Honors ctx cancel** — closing ctx closes the daemon connection within ~100 ms (the docker client's own ctx propagation).
3. **Follow vs one-shot** — `opts.Follow=true` keeps the connection open and writes lines as the container produces them; `Follow=false` returns the snapshot and closes after the last historical line.
4. **Tail semantics** — `Tail=-1` (or `0` in Docker's vocabulary, but we normalize to `-1` at the call site) returns all available lines; `Tail=N` returns the most recent N. Negative non-`-1` values are an error before the call.
5. **Since semantics** — `Since` non-zero filters to lines AFTER that timestamp. Resolution is Docker's (~seconds).
6. **Timestamps** — `Timestamps=true` prefixes each line with Docker's RFC3339 timestamp (a string like `2026-05-17T11:23:45.123456789Z`). Default false (v0.4 leaves this off; the CLI flag lands as a polish later).
7. **No retry** — transient daemon errors propagate to the caller; this method is not the right place to add a retry loop. CLI / handler decide based on the error.

## Lifecycle

```
caller: rc, err := rt.StreamLogs(ctx, id, opts)
        ├─ daemon unreachable → err returned, rc = nil
        ├─ container not found → err with ErrNotFound semantics
        └─ otherwise → rc returned (open)

caller: io.Copy(w, rc) ... or scanner over rc
        ├─ container exits → rc returns io.EOF
        ├─ ctx cancels → rc returns ctx.Err()
        └─ daemon dies → rc returns wrapped error

caller: rc.Close()
        └─ MUST be called to release the daemon connection
```

## Mock surface (for unit tests)

`internal/runtime/docker/mockclient_test.go` extends `dockerClient` mock with:

```go
ContainerLogs(ctx context.Context, container string, options container.LogsOptions) (io.ReadCloser, error)
```

Tests can stuff the returned ReadCloser with a buffer containing pre-built stdcopy-framed bytes and assert the demux output.

## Concurrency

- One `StreamLogs` call per (containerID, opts) per request. No global cache.
- Multiple concurrent calls for the same container are independent — each opens its own daemon connection.
- The daemon may impose its own connection limits; document but don't workaround in v0.4.

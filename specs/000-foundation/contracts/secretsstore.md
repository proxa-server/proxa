# Contract: `internal/secrets.SecretsStore`

Encrypted-at-rest secret material. Backed by `age` (X25519 + ChaCha20-Poly1305) in v0.x. Secret values are never persisted in plaintext, never returned in API responses, never logged.

## Go signature (v0.0)

```go
package secrets

import (
    "context"
    "errors"
    "time"
)

// SecretsStore manages encrypted secrets, scoped per project.
type SecretsStore interface {
    // Open initializes the store with the master key material location.
    Open(ctx context.Context, keyPath string) error
    Close() error

    // Put encrypts and stores a secret. Overwrites if exists.
    Put(ctx context.Context, project, name string, value []byte) error

    // Get returns the decrypted value. Caller MUST scrub the slice when done.
    Get(ctx context.Context, project, name string) ([]byte, error)

    // List returns names only (no values).
    List(ctx context.Context, project string) ([]SecretMeta, error)

    // Delete removes a secret. ErrNotFound if absent.
    Delete(ctx context.Context, project, name string) error

    // Rotate re-encrypts all secrets under a new key. Atomic per project.
    Rotate(ctx context.Context, newKeyPath string) error
}

type SecretMeta struct {
    Project    string
    Name       string
    CreatedAt  time.Time
    UpdatedAt  time.Time
    BytesLen   int       // length of plaintext; safe to expose
}

var (
    ErrNotFound       = errors.New("secrets: not found")
    ErrNotImplemented = errors.New("secrets: not implemented")
)
```

## Behavioral contract (constitution §II hard rules)

1. **Plaintext never crosses an API boundary.** `Get` is callable only from in-process Runtime code at container-start time. The HTTP handlers MUST NOT expose a "read secret" endpoint.
2. **No secret material in logs, ever.** Implementations MUST NOT log `value`, the raw bytes, or any derivative. Error messages reference `project/name` only.
3. **Names follow project naming rules**: `^[a-z0-9][a-z0-9-]{0,62}$`.
4. **`Rotate` is atomic per project**: either all secrets in the project are re-encrypted with the new key or none are.
5. **Master key is on disk by default** at `${PROXA_DATA_DIR}/secrets.key` with `0600` perms. Implementations enforce mode bits.
6. **`Get` returns a freshly-allocated slice each call.** No reused buffers; caller is responsible for `crypto/subtle`-style scrubbing if they care.

## Foundation deliverable

`internal/secrets/secrets.go` declares the interface + `ErrNotImplemented`. **No implementation.** The age-based impl arrives in a later feature when the API actually accepts secret CRUD.

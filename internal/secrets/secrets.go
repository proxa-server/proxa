// Package secrets is the encrypted-at-rest secret store. Backed by age
// (X25519 + ChaCha20-Poly1305) in v0.x. Plaintext secret material never
// crosses an API boundary, never appears in logs, and never appears in
// the dashboard.
//
// See specs/000-foundation/contracts/secretsstore.md for the full
// behavioral contract.
package secrets

import (
	"context"
	"errors"
	"time"
)

// SecretsStore manages encrypted secrets, scoped per project.
//
// Constitution §II hard rules (enforced by every implementation):
//
//   - Plaintext is never returned by an API handler. Get is callable only
//     from in-process Runtime code at container-start time.
//   - No secret material is logged. Error messages reference project/name
//     only — never the value or any derivative.
//   - Master key is on disk at ${PROXA_DATA_DIR}/secrets.key with mode 0600.
//   - All file operations MUST flow through [internal/datadir.Root]
//     (added in v0.4.1) — no direct os.OpenFile / os.ReadFile against
//     the data directory. Implementations should accept *datadir.Root
//     in their constructor.
type SecretsStore interface {
	Open(ctx context.Context, keyPath string) error
	Close() error

	// Put encrypts and stores a secret. Overwrites if exists.
	Put(ctx context.Context, project, name string, value []byte) error

	// Get returns the freshly-allocated, decrypted value. Caller is
	// responsible for scrubbing the slice when done.
	Get(ctx context.Context, project, name string) ([]byte, error)

	// List returns names and metadata, never values.
	List(ctx context.Context, project string) ([]SecretMeta, error)

	// Delete removes a secret. Returns ErrNotFound if absent.
	Delete(ctx context.Context, project, name string) error

	// Rotate re-encrypts all secrets under a new key. Atomic per project.
	Rotate(ctx context.Context, newKeyPath string) error
}

// SecretMeta is the safe-to-expose description of a secret.
type SecretMeta struct {
	Project   string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	BytesLen  int // length of plaintext; safe to expose
}

var (
	// ErrNotFound is returned when the requested secret does not exist.
	ErrNotFound = errors.New("secrets: not found")

	// ErrNotImplemented is returned by stub implementations.
	ErrNotImplemented = errors.New("secrets: not implemented")
)

// noopSecretsStore satisfies [SecretsStore] with ErrNotImplemented for
// every method. Useful as a placeholder in unit tests.
type noopSecretsStore struct{}

// Compile-time assertion that noopSecretsStore satisfies SecretsStore.
var _ SecretsStore = noopSecretsStore{}

func (noopSecretsStore) Open(context.Context, string) error                { return ErrNotImplemented }
func (noopSecretsStore) Close() error                                      { return ErrNotImplemented }
func (noopSecretsStore) Put(context.Context, string, string, []byte) error { return ErrNotImplemented }
func (noopSecretsStore) Get(context.Context, string, string) ([]byte, error) {
	return nil, ErrNotImplemented
}
func (noopSecretsStore) List(context.Context, string) ([]SecretMeta, error) {
	return nil, ErrNotImplemented
}
func (noopSecretsStore) Delete(context.Context, string, string) error { return ErrNotImplemented }
func (noopSecretsStore) Rotate(context.Context, string) error         { return ErrNotImplemented }

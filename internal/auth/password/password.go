// Package password implements [auth.Authenticator] using local
// usernames + bcrypt-hashed passwords. Reserved for future dashboard
// login flow (Feature 004); the v0.0 CLI authenticates via bearer
// token instead. Wired through [auth.Chain] so the same middleware
// handles both.
package password

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/proxa-server/proxa/internal/auth"
	"github.com/proxa-server/proxa/pkg/types"
)

// SubjectLookup is the store surface this authenticator needs.
// Returns the subject whose Name matches `username`, or store.ErrNotFound.
type SubjectLookup interface {
	GetSubjectByName(ctx context.Context, name string) (*types.Subject, error)
}

// Authenticator verifies Basic-auth credentials against bcrypt hashes
// in the StateStore.
type Authenticator struct {
	lookup SubjectLookup
}

// New returns an Authenticator backed by the given subject lookup.
func New(lookup SubjectLookup) *Authenticator {
	return &Authenticator{lookup: lookup}
}

// Name returns "local-password".
func (a *Authenticator) Name() string { return "local-password" }

// Authenticate parses the Basic-auth header and verifies the password
// against the stored bcrypt hash.
func (a *Authenticator) Authenticate(ctx context.Context, r *http.Request) (*types.Subject, error) {
	user, pass, ok := basicAuth(r)
	if !ok {
		return nil, auth.ErrUnauthenticated
	}
	sub, err := a.lookup.GetSubjectByName(ctx, user)
	if err != nil {
		return nil, auth.ErrUnauthenticated
	}
	hash := sub.Metadata["secret_hash"]
	if hash == "" {
		return nil, auth.ErrUnauthenticated
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(pass)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, auth.ErrUnauthenticated
		}
		return nil, fmt.Errorf("auth/password: verify: %w", err)
	}
	out := *sub
	out.Metadata = stripSecret(sub.Metadata)
	return &out, nil
}

// RotateCredentials regenerates the bcrypt hash for `subjectID` against
// `newPassword`. The caller passes the new password as a base64-encoded
// string in the metadata channel; in v0.0 this method returns
// ErrNotSupported (rotation surfaced by the future dashboard).
func (a *Authenticator) RotateCredentials(ctx context.Context, subjectID string) error {
	return auth.ErrNotSupported
}

// HashPassword produces a bcrypt hash of the plaintext password using
// cost 12. Used by `proxa init` when creating the local admin user.
func HashPassword(plaintext string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plaintext), 12)
	if err != nil {
		return "", fmt.Errorf("auth/password: hash: %w", err)
	}
	return string(h), nil
}

// basicAuth extracts (user, pass) from an Authorization: Basic header.
func basicAuth(r *http.Request) (string, string, bool) {
	const prefix = "Basic "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return "", "", false
	}
	dec, err := base64.StdEncoding.DecodeString(h[len(prefix):])
	if err != nil {
		return "", "", false
	}
	pair := string(dec)
	before, after, ok := strings.Cut(pair, ":")
	if !ok {
		return "", "", false
	}
	return before, after, true
}

func stripSecret(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if k == "secret_hash" {
			continue
		}
		out[k] = v
	}
	return out
}

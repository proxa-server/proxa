// Package token implements [auth.Authenticator] using a bearer token
// stored as a bcrypt hash in the [store.StateStore]. Used for the
// bootstrap admin token created by `proxa init`.
package token

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"github.com/proxa-server/proxa/internal/auth"
	"github.com/proxa-server/proxa/pkg/types"
)

// SubjectGetter is the minimum store surface this authenticator needs.
// Decouples from the full StateStore interface for testability.
type SubjectGetter interface {
	GetSubject(ctx context.Context, id string) (*types.Subject, error)
}

// Authenticator verifies Bearer tokens against bcrypt hashes stored in
// the StateStore subject row whose ID matches BootstrapSubjectID.
type Authenticator struct {
	store              SubjectGetter
	BootstrapSubjectID string
}

// New returns an Authenticator that resolves bootstrap tokens against
// the subject identified by BootstrapSubjectID (default "bootstrap-admin").
func New(store SubjectGetter) *Authenticator {
	return &Authenticator{store: store, BootstrapSubjectID: "bootstrap-admin"}
}

// Name returns "token".
func (a *Authenticator) Name() string { return "token" }

// Authenticate extracts the Bearer token from the request and verifies it
// against the bcrypt hash stored on the bootstrap subject.
func (a *Authenticator) Authenticate(ctx context.Context, r *http.Request) (*types.Subject, error) {
	token, ok := bearerToken(r)
	if !ok {
		return nil, auth.ErrUnauthenticated
	}
	sub, err := a.store.GetSubject(ctx, a.BootstrapSubjectID)
	if err != nil {
		return nil, fmt.Errorf("auth/token: lookup bootstrap subject: %w", err)
	}
	hash := sub.Metadata["secret_hash"]
	if hash == "" {
		return nil, auth.ErrUnauthenticated
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)); err != nil {
		if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
			return nil, auth.ErrUnauthenticated
		}
		return nil, fmt.Errorf("auth/token: verify: %w", err)
	}
	// Strip the secret_hash before returning the subject to handlers.
	out := *sub
	out.Metadata = stripSecret(sub.Metadata)
	return &out, nil
}

// RotateCredentials returns ErrNotSupported for v0.0; rotation lands
// when the dashboard's settings page exists (Feature 004).
func (a *Authenticator) RotateCredentials(ctx context.Context, subjectID string) error {
	return auth.ErrNotSupported
}

// bearerToken parses an "Authorization: Bearer <token>" header.
// Falls back to the proxa_token cookie (set by the UI middleware
// when the user lands at /ui/?token=...) and finally to the
// ?token= query parameter (browser-friendly first-visit URL).
// Returns the token and true on success.
func bearerToken(r *http.Request) (string, bool) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		if tok := strings.TrimSpace(h[len("Bearer "):]); tok != "" {
			return tok, true
		}
	}
	if c, err := r.Cookie("proxa_token"); err == nil && c.Value != "" {
		return c.Value, true
	}
	if tok := r.URL.Query().Get("token"); tok != "" {
		return tok, true
	}
	return "", false
}

// stripSecret returns a copy of m with the "secret_hash" key removed.
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

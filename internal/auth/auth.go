// Package auth holds the identity ([Authenticator]) and authorization
// ([PolicyEngine]) interfaces. Both are consumed together by the API
// middleware: authenticate, then authorize.
//
// Constitution §II: every API endpoint authenticates — there are no
// anonymous routes.
//
// See specs/000-foundation/contracts/authenticator.md and
// specs/000-foundation/contracts/policyengine.md for the full
// behavioral contracts.
package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/proxa-server/proxa/pkg/types"
)

// Authenticator turns transport-level credentials into a [types.Subject].
//
// Implementations expected in v0.x: TokenAuthenticator (bearer tokens),
// LocalPasswordAuthenticator (bcrypt), AgentTokenAuthenticator (per-node
// identity tokens — constitution §VI). OIDCAuthenticator arrives later.
//
// Behavioral rules:
//
//   - Authenticate is read-only against the request; it MUST NOT mutate
//     beyond what context propagation requires.
//   - A nil Subject with nil error is forbidden.
//   - Errors do not echo credentials.
type Authenticator interface {
	Name() string
	Authenticate(ctx context.Context, r *http.Request) (*types.Subject, error)
	RotateCredentials(ctx context.Context, subjectID string) error
}

// Chain composes multiple [Authenticator]s; the first one whose
// Authenticate returns a non-nil Subject without an error wins. If all
// fail, the last error is returned (or [ErrUnauthenticated] if none ran).
type Chain []Authenticator

// Name returns "chain".
func (c Chain) Name() string { return "chain" }

// Authenticate iterates the chain in order.
func (c Chain) Authenticate(ctx context.Context, r *http.Request) (*types.Subject, error) {
	var lastErr error
	for _, a := range c {
		s, err := a.Authenticate(ctx, r)
		if err == nil {
			return s, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		return nil, ErrUnauthenticated
	}
	return nil, lastErr
}

// RotateCredentials returns [ErrNotSupported]; chains do not own a single
// identity store, so rotation is the underlying [Authenticator]'s job.
func (Chain) RotateCredentials(context.Context, string) error { return ErrNotSupported }

var (
	// ErrUnauthenticated is returned when no credentials are present or
	// they fail validation.
	ErrUnauthenticated = errors.New("auth: unauthenticated")

	// ErrNotSupported is returned by Authenticator implementations that
	// do not implement an operation (e.g., RotateCredentials).
	ErrNotSupported = errors.New("auth: operation not supported")

	// ErrNotImplemented is returned by stub implementations.
	ErrNotImplemented = errors.New("auth: not implemented")
)

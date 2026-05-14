# Contract: `internal/auth.Authenticator`

Identity resolution at the API boundary. Returns a `*types.Subject` for an authenticated request or an error. Per constitution §II, every API endpoint authenticates — there are no anonymous routes.

## Go signature (v0.0)

```go
package auth

import (
    "context"
    "errors"
    "net/http"

    "github.com/proxa-server/proxa/pkg/types"
)

// Authenticator turns transport-level credentials into a Subject.
type Authenticator interface {
    // Name identifies the auth method ("token", "oidc", "local-password", ...).
    Name() string

    // Authenticate inspects the request and returns the Subject.
    // Returns ErrUnauthenticated if no credentials are present or they are invalid.
    Authenticate(ctx context.Context, r *http.Request) (*types.Subject, error)

    // RotateCredentials is impl-specific (token refresh, password change, etc.).
    // Implementations that do not support rotation return ErrNotSupported.
    RotateCredentials(ctx context.Context, subjectID string) error
}

// Chain composes multiple Authenticators; first match wins.
type Chain []Authenticator

func (c Chain) Name() string { return "chain" }
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

var (
    ErrUnauthenticated = errors.New("auth: unauthenticated")
    ErrNotSupported    = errors.New("auth: operation not supported")
    ErrNotImplemented  = errors.New("auth: not implemented")
)
```

## Behavioral contract

1. **No anonymous access.** A nil `Subject` with nil error is forbidden; impls must either return `(*Subject, nil)` or `(nil, error)`.
2. **`Authenticate` is read-only.** It MUST NOT mutate the request beyond what `r.Context()` propagation requires.
3. **Errors do not leak credentials.** Returned errors describe the failure class ("invalid token", "expired", "missing header") without echoing the credential value.
4. **Implementations expected in v0.x**: `TokenAuthenticator` (bearer tokens), `LocalPasswordAuthenticator` (bcrypt-hashed). `OIDCAuthenticator` in v0.x late or v1.0.
5. **`agent` role authentication** (constitution §VI): cluster agents authenticate via a dedicated `AgentTokenAuthenticator` that maps a node-identity token to a `Subject` with provider `"agent"`.

## Foundation deliverable

`internal/auth/auth.go` declares the interface + `Chain` helper + sentinel errors. No concrete authenticator implementation in this feature.

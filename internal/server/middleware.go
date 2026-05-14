package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/proxa-server/proxa/internal/auth"
	"github.com/proxa-server/proxa/pkg/types"
)

type ctxKey int

const subjectCtxKey ctxKey = 1

// errorEnvelope mirrors the JSON envelope from contracts/rest-api.md.
type errorEnvelope struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorEnvelope{Error: code, Message: msg})
}

// RequireAuth returns a middleware that runs the configured
// Authenticator on each request and stores the resulting Subject
// in the request context.
func RequireAuth(authn auth.Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sub, err := authn.Authenticate(r.Context(), r)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthenticated", "missing or invalid credentials")
				return
			}
			ctx := context.WithValue(r.Context(), subjectCtxKey, sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuthOrUnix is the dashboard-flavored auth middleware: bypass
// the token check when the listener is a Unix socket (file-system
// permissions do the gating). Over TCP, the same token check as
// RequireAuth applies. v0.0 dashboard has no login form, so over TCP
// /ui/ returns 401 without a Bearer header.
func RequireAuthOrUnix(authn auth.Authenticator, isUnix bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isUnix {
				next.ServeHTTP(w, r)
				return
			}
			sub, err := authn.Authenticate(r.Context(), r)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "unauthenticated",
					"dashboard over TCP requires a Bearer token in v0.0; login form arrives in Feature 004")
				return
			}
			ctx := context.WithValue(r.Context(), subjectCtxKey, sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SubjectFromContext retrieves the authenticated subject placed by
// RequireAuth/RequireAuthOrUnix. Returns nil if the request was not
// authenticated.
func SubjectFromContext(ctx context.Context) *types.Subject {
	sub, _ := ctx.Value(subjectCtxKey).(*types.Subject)
	return sub
}

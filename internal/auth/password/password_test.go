package password

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/proxa-server/proxa/internal/auth"
	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

type fakeLookup struct {
	users map[string]*types.Subject
}

func (f *fakeLookup) GetSubjectByName(ctx context.Context, name string) (*types.Subject, error) {
	s, ok := f.users[name]
	if !ok {
		return nil, store.ErrNotFound
	}
	return s, nil
}

func basicAuthHeader(user, pass string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
}

func newReq(headerValue string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	if headerValue != "" {
		r.Header.Set("Authorization", headerValue)
	}
	return r
}

func TestHashPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" {
		t.Fatalf("empty hash")
	}
}

func TestAuthenticate(t *testing.T) {
	hash, _ := HashPassword("hunter2")
	users := map[string]*types.Subject{
		"admin": {
			ID:       "local-admin",
			Name:     "admin",
			Provider: "local",
			Metadata: map[string]string{"secret_hash": hash},
		},
	}
	a := New(&fakeLookup{users: users})

	tests := []struct {
		name    string
		header  string
		wantErr error
	}{
		{"valid creds", basicAuthHeader("admin", "hunter2"), nil},
		{"wrong password", basicAuthHeader("admin", "wrong"), auth.ErrUnauthenticated},
		{"unknown user", basicAuthHeader("nobody", "anything"), auth.ErrUnauthenticated},
		{"missing header", "", auth.ErrUnauthenticated},
		{"bearer scheme", "Bearer xyz", auth.ErrUnauthenticated},
		{"malformed b64", "Basic !!!", auth.ErrUnauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sub, err := a.Authenticate(context.Background(), newReq(tt.header))
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if sub == nil || sub.Name != "admin" {
					t.Errorf("subject mismatch: %+v", sub)
				}
				if _, leaked := sub.Metadata["secret_hash"]; leaked {
					t.Errorf("secret_hash leaked into returned subject")
				}
			} else {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("err = %v, want %v", err, tt.wantErr)
				}
			}
		})
	}
}

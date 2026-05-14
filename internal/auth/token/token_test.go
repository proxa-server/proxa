package token

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/proxa-server/proxa/internal/auth"
	"github.com/proxa-server/proxa/internal/store"
	"github.com/proxa-server/proxa/pkg/types"
)

// fakeStore implements SubjectGetter for tests.
type fakeStore struct {
	subject *types.Subject
	err     error
}

func (f *fakeStore) GetSubject(ctx context.Context, id string) (*types.Subject, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.subject == nil {
		return nil, store.ErrNotFound
	}
	return f.subject, nil
}

func newRequestWithAuth(header string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	return req
}

func TestAuthenticate(t *testing.T) {
	const validToken = "test-token-1234567890abcdef"
	hash, err := bcrypt.GenerateFromPassword([]byte(validToken), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	bootstrap := &types.Subject{
		ID:       "bootstrap-admin",
		Name:     "admin",
		Provider: "bootstrap",
		Metadata: map[string]string{"secret_hash": string(hash)},
	}

	tests := []struct {
		name    string
		header  string
		store   SubjectGetter
		wantErr error
	}{
		{
			name:    "valid token",
			header:  "Bearer " + validToken,
			store:   &fakeStore{subject: bootstrap},
			wantErr: nil,
		},
		{
			name:    "wrong token",
			header:  "Bearer wrong",
			store:   &fakeStore{subject: bootstrap},
			wantErr: auth.ErrUnauthenticated,
		},
		{
			name:    "missing header",
			header:  "",
			store:   &fakeStore{subject: bootstrap},
			wantErr: auth.ErrUnauthenticated,
		},
		{
			name:    "non-bearer scheme",
			header:  "Basic dXNlcjpwYXNz",
			store:   &fakeStore{subject: bootstrap},
			wantErr: auth.ErrUnauthenticated,
		},
		{
			name:    "subject missing",
			header:  "Bearer " + validToken,
			store:   &fakeStore{subject: nil},
			wantErr: store.ErrNotFound, // wrapped, but the chain contains it
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := New(tt.store)
			req := newRequestWithAuth(tt.header)
			sub, err := a.Authenticate(context.Background(), req)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if sub == nil || sub.ID != "bootstrap-admin" {
					t.Errorf("unexpected subject: %+v", sub)
				}
				if _, ok := sub.Metadata["secret_hash"]; ok {
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

func TestRotateCredentialsUnsupported(t *testing.T) {
	a := New(&fakeStore{})
	if err := a.RotateCredentials(context.Background(), "x"); !errors.Is(err, auth.ErrNotSupported) {
		t.Errorf("got %v, want ErrNotSupported", err)
	}
}

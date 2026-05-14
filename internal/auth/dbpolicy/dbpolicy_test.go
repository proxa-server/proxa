package dbpolicy

import (
	"context"
	"errors"
	"testing"

	"github.com/proxa-server/proxa/internal/auth"
	"github.com/proxa-server/proxa/pkg/types"
)

type fakeStore struct {
	policies map[string][]types.Policy
}

func (f *fakeStore) ListPoliciesFor(ctx context.Context, subjectID string) ([]types.Policy, error) {
	return f.policies[subjectID], nil
}

func TestAuthorize(t *testing.T) {
	subjectAdmin := &types.Subject{ID: "u-admin"}
	subjectEditor := &types.Subject{ID: "u-editor"}
	subjectViewer := &types.Subject{ID: "u-viewer"}
	subjectAgent := &types.Subject{ID: "u-agent"}

	store := &fakeStore{policies: map[string][]types.Policy{
		"u-admin":  {{Role: types.RoleAdmin, Project: "*"}},
		"u-editor": {{Role: types.RoleEditor, Project: "socio-do"}},
		"u-viewer": {{Role: types.RoleViewer, Project: "kut-do"}},
		"u-agent":  {{Role: types.RoleAgent, Project: "*"}},
	}}
	e := New(store)

	tests := []struct {
		name string
		req  auth.AuthzRequest
		want error
	}{
		// Admin: anything allowed
		{"admin/create-service-anywhere", auth.AuthzRequest{
			Subject:  subjectAdmin,
			Verb:     auth.VerbCreate,
			Resource: auth.ResourceRef{Kind: auth.KindService, Project: "any-project"},
		}, nil},
		{"admin/delete-policy", auth.AuthzRequest{
			Subject:  subjectAdmin,
			Verb:     auth.VerbDelete,
			Resource: auth.ResourceRef{Kind: auth.KindPolicy},
		}, nil},

		// Editor: CRUD on service in scoped project
		{"editor/create-service-in-scope", auth.AuthzRequest{
			Subject:  subjectEditor,
			Verb:     auth.VerbCreate,
			Resource: auth.ResourceRef{Kind: auth.KindService, Project: "socio-do"},
		}, nil},
		{"editor/create-service-out-of-scope", auth.AuthzRequest{
			Subject:  subjectEditor,
			Verb:     auth.VerbCreate,
			Resource: auth.ResourceRef{Kind: auth.KindService, Project: "kut-do"},
		}, auth.ErrForbidden},
		{"editor/delete-project-rejected", auth.AuthzRequest{
			Subject:  subjectEditor,
			Verb:     auth.VerbDelete,
			Resource: auth.ResourceRef{Kind: auth.KindProject, Project: "socio-do"},
		}, auth.ErrForbidden},

		// Viewer: read-only in scoped project
		{"viewer/get-service-in-scope", auth.AuthzRequest{
			Subject:  subjectViewer,
			Verb:     auth.VerbGet,
			Resource: auth.ResourceRef{Kind: auth.KindService, Project: "kut-do"},
		}, nil},
		{"viewer/create-service-rejected", auth.AuthzRequest{
			Subject:  subjectViewer,
			Verb:     auth.VerbCreate,
			Resource: auth.ResourceRef{Kind: auth.KindService, Project: "kut-do"},
		}, auth.ErrForbidden},
		{"viewer/get-out-of-scope", auth.AuthzRequest{
			Subject:  subjectViewer,
			Verb:     auth.VerbGet,
			Resource: auth.ResourceRef{Kind: auth.KindService, Project: "socio-do"},
		}, auth.ErrForbidden},

		// Agent: get on services
		{"agent/get-service", auth.AuthzRequest{
			Subject:  subjectAgent,
			Verb:     auth.VerbGet,
			Resource: auth.ResourceRef{Kind: auth.KindService, Project: "any"},
		}, nil},
		{"agent/update-node", auth.AuthzRequest{
			Subject:  subjectAgent,
			Verb:     auth.VerbUpdate,
			Resource: auth.ResourceRef{Kind: auth.KindNode},
		}, nil},
		{"agent/create-service-rejected", auth.AuthzRequest{
			Subject:  subjectAgent,
			Verb:     auth.VerbCreate,
			Resource: auth.ResourceRef{Kind: auth.KindService, Project: "any"},
		}, auth.ErrForbidden},

		// Empty project on project-scoped kind always denied.
		{"editor/empty-project-rejected", auth.AuthzRequest{
			Subject:  subjectEditor,
			Verb:     auth.VerbList,
			Resource: auth.ResourceRef{Kind: auth.KindService, Project: ""},
		}, auth.ErrForbidden},

		// Nil subject denied.
		{"nil-subject", auth.AuthzRequest{
			Subject:  nil,
			Verb:     auth.VerbGet,
			Resource: auth.ResourceRef{Kind: auth.KindService, Project: "default"},
		}, auth.ErrForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Authorize(context.Background(), tt.req)
			if tt.want == nil && got != nil {
				t.Errorf("got %v, want nil", got)
			}
			if tt.want != nil && !errors.Is(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

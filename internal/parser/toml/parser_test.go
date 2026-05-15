package toml

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/proxa-server/proxa/pkg/types"
)

func TestParseValidMinimal(t *testing.T) {
	td := parseFile(t, "valid-minimal.toml")
	if td.Name != "web" {
		t.Errorf("name = %q, want 'web'", td.Name)
	}
	if td.Image != "nginx:alpine" {
		t.Errorf("image = %q, want 'nginx:alpine'", td.Image)
	}
	if td.Replicas != 3 {
		t.Errorf("replicas = %d, want 3", td.Replicas)
	}
	if td.Project != "default" {
		t.Errorf("project = %q, want 'default' (auto-applied)", td.Project)
	}
	if len(td.Expose) != 1 || td.Expose[0].Protocol != "http" {
		t.Errorf("expose mismatch: %+v", td.Expose)
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		fixture  string
		wantCode string
	}{
		{"invalid-missing-image.toml", "invalid-image-ref"},
		{"invalid-bad-name.toml", "missing-or-invalid-name"},
		{"invalid-bad-protocol.toml", "invalid-protocol"},
	}

	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			_, err := tryParse(t, tt.fixture)
			if err == nil {
				t.Fatalf("expected error for %s", tt.fixture)
			}
			var ve *validationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *validationError, got %T: %v", err, err)
			}
			if ve.Code != tt.wantCode {
				t.Errorf("code = %q, want %q (err: %v)", ve.Code, tt.wantCode, err)
			}
		})
	}
}

func parseFile(t *testing.T, name string) types.TaskDef {
	t.Helper()
	td, err := tryParse(t, name)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return td
}

func tryParse(t *testing.T, name string) (types.TaskDef, error) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	return Parse(f)
}

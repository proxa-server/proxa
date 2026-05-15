package docker

import (
	"strings"
	"testing"

	"github.com/proxa-server/proxa/internal/runtime"
)

func TestContainerNameFor(t *testing.T) {
	tests := []struct {
		project, service string
		replica          int
		want             string
	}{
		{"default", "web", 0, "proxa-default-web-0"},
		{"socio-do", "api", 7, "proxa-socio-do-api-7"},
		{"a", "b", 99, "proxa-a-b-99"},
	}
	for _, tt := range tests {
		got := ContainerNameFor(tt.project, tt.service, tt.replica)
		if got != tt.want {
			t.Errorf("ContainerNameFor(%q, %q, %d) = %q, want %q",
				tt.project, tt.service, tt.replica, got, tt.want)
		}
	}
}

func TestContainerNameUnderDockerLimit(t *testing.T) {
	// Worst-case: project + service each at the 63-char regex max.
	long := strings.Repeat("a", 63)
	got := ContainerNameFor(long, long, 999)
	if len(got) > 253 {
		t.Errorf("name length %d exceeds Docker limit of 253", len(got))
	}
}

func TestBuildContainerLabelsPopulatesReserved(t *testing.T) {
	spec := runtime.ContainerSpec{
		Labels: map[string]string{
			LabelProject: "default",
			LabelService: "web",
			"custom":     "value",
		},
	}
	labels := BuildContainerLabels(spec, 2, "sha256:abc123", "node-local")

	checks := map[string]string{
		LabelManaged:  "true",
		LabelProject:  "default",
		LabelService:  "web",
		LabelReplica:  "2",
		LabelSpecHash: "sha256:abc123",
		LabelNodeID:   "node-local",
		"custom":      "value",
	}
	for k, want := range checks {
		if got := labels[k]; got != want {
			t.Errorf("label %q = %q, want %q", k, got, want)
		}
	}
	if labels[LabelCreatedAt] == "" {
		t.Errorf("LabelCreatedAt should be populated")
	}
}

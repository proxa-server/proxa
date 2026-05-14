package reconciler

import (
	"testing"

	"github.com/proxa-server/proxa/internal/hash"
	rt "github.com/proxa-server/proxa/internal/runtime"
	dockerlabels "github.com/proxa-server/proxa/internal/runtime/docker"
	"github.com/proxa-server/proxa/pkg/types"
)

func mkSvc(project, name, image string, replicas int) types.Service {
	return types.Service{
		Project: project,
		Name:    name,
		Spec: types.TaskDef{
			Project:  project,
			Name:     name,
			Image:    image,
			Replicas: replicas,
		},
	}
}

func mkContainer(id, project, service string, replica int, specHash string) rt.ContainerInfo {
	return rt.ContainerInfo{
		ID:    id,
		Name:  "/proxa-" + project + "-" + service,
		Image: "nginx:alpine",
		State: "running",
		Labels: map[string]string{
			dockerlabels.LabelProject:  project,
			dockerlabels.LabelService:  service,
			dockerlabels.LabelReplica:  itoa(replica),
			dockerlabels.LabelSpecHash: specHash,
		},
	}
}

func itoa(i int) string {
	switch i {
	case 0:
		return "0"
	case 1:
		return "1"
	case 2:
		return "2"
	case 3:
		return "3"
	case 4:
		return "4"
	}
	return "?"
}

func TestComputeNoChange(t *testing.T) {
	svc := mkSvc("default", "web", "nginx:alpine", 2)
	specHash := hash.Hash(svc.Spec)
	actual := []rt.ContainerInfo{
		mkContainer("c0", "default", "web", 0, specHash),
		mkContainer("c1", "default", "web", 1, specHash),
	}
	actions := Compute([]types.Service{svc}, actual)
	if len(actions) != 0 {
		t.Errorf("expected 0 actions, got %d: %+v", len(actions), actions)
	}
}

func TestComputeScaleUp(t *testing.T) {
	svc := mkSvc("default", "web", "nginx:alpine", 3)
	specHash := hash.Hash(svc.Spec)
	actual := []rt.ContainerInfo{
		mkContainer("c0", "default", "web", 0, specHash),
	}
	actions := Compute([]types.Service{svc}, actual)
	creates := 0
	for _, a := range actions {
		if a.Type == ActionCreate {
			creates++
		}
	}
	if creates != 2 {
		t.Errorf("expected 2 creates (replicas 1 and 2), got %d (all=%+v)", creates, actions)
	}
}

func TestComputeScaleDown(t *testing.T) {
	svc := mkSvc("default", "web", "nginx:alpine", 1)
	specHash := hash.Hash(svc.Spec)
	actual := []rt.ContainerInfo{
		mkContainer("c0", "default", "web", 0, specHash),
		mkContainer("c1", "default", "web", 1, specHash),
		mkContainer("c2", "default", "web", 2, specHash),
	}
	actions := Compute([]types.Service{svc}, actual)
	removes := 0
	for _, a := range actions {
		if a.Type == ActionRemove {
			removes++
		}
	}
	if removes != 2 {
		t.Errorf("expected 2 removes, got %d (all=%+v)", removes, actions)
	}
}

func TestComputeSpecDrift(t *testing.T) {
	svc := mkSvc("default", "web", "nginx:1.27", 1)
	currentHash := hash.Hash(svc.Spec)
	staleHash := "sha256:stale"
	actual := []rt.ContainerInfo{
		mkContainer("c0", "default", "web", 0, staleHash),
	}
	actions := Compute([]types.Service{svc}, actual)
	if len(actions) != 1 || actions[0].Type != ActionReplace {
		t.Fatalf("expected 1 replace, got %+v", actions)
	}
	if actions[0].ContainerID != "c0" {
		t.Errorf("ContainerID = %q, want c0", actions[0].ContainerID)
	}
	if actions[0].SpecHash != currentHash {
		t.Errorf("SpecHash = %q, want %q", actions[0].SpecHash, currentHash)
	}
	if actions[0].Reason != "spec_hash drift" {
		t.Errorf("Reason = %q", actions[0].Reason)
	}
}

func TestComputeServiceDeleted(t *testing.T) {
	specHash := "sha256:any"
	actual := []rt.ContainerInfo{
		mkContainer("c0", "default", "old-svc", 0, specHash),
		mkContainer("c1", "default", "old-svc", 1, specHash),
	}
	actions := Compute(nil, actual) // no desired services
	if len(actions) != 2 {
		t.Errorf("expected 2 removes for deleted service, got %d", len(actions))
	}
	for _, a := range actions {
		if a.Type != ActionRemove {
			t.Errorf("expected all removes, got %+v", a)
		}
	}
}

func TestComputeMultiProjectIndependence(t *testing.T) {
	socio := mkSvc("socio-do", "web", "nginx:alpine", 1)
	kut := mkSvc("kut-do", "web", "nginx:alpine", 1)
	actions := Compute([]types.Service{socio, kut}, nil)
	if len(actions) != 2 {
		t.Fatalf("expected 2 creates (one per project), got %d", len(actions))
	}
	projects := map[string]bool{}
	for _, a := range actions {
		projects[a.Project] = true
	}
	if !projects["socio-do"] || !projects["kut-do"] {
		t.Errorf("project independence broken: %+v", actions)
	}
}

func TestComputeDeterministicOrdering(t *testing.T) {
	svcs := []types.Service{
		mkSvc("kut-do", "web", "nginx:alpine", 1),
		mkSvc("socio-do", "api", "nginx:alpine", 2),
	}
	a1 := Compute(svcs, nil)
	a2 := Compute(svcs, nil)
	if len(a1) != len(a2) {
		t.Fatalf("length mismatch")
	}
	for i := range a1 {
		if a1[i].Project != a2[i].Project || a1[i].Service != a2[i].Service || a1[i].Replica != a2[i].Replica {
			t.Errorf("non-deterministic at index %d: %+v vs %+v", i, a1[i], a2[i])
		}
	}
}

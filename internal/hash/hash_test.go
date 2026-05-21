package hash

import (
	"maps"
	"testing"

	"github.com/proxa-server/proxa/pkg/types"
)

func TestHashDeterministic(t *testing.T) {
	spec := types.TaskDef{
		Project:  "default",
		Name:     "web",
		Image:    "nginx:alpine",
		Replicas: 3,
		Env: map[string]string{
			"LOG_LEVEL": "info",
			"PORT":      "8080",
		},
	}

	// Hash a second TaskDef with env entries in different insertion order.
	// Map iteration in Go is randomized; canonical encoding must sort.
	spec2 := types.TaskDef{
		Project:  "default",
		Name:     "web",
		Image:    "nginx:alpine",
		Replicas: 3,
		Env: map[string]string{
			"PORT":      "8080",
			"LOG_LEVEL": "info",
		},
	}

	if Hash(spec) != Hash(spec2) {
		t.Errorf("Hash not deterministic across map ordering:\n  spec1 = %s\n  spec2 = %s",
			Hash(spec), Hash(spec2))
	}
}

func TestHashChangesOnFieldChange(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*types.TaskDef)
		wantSame bool
	}{
		{
			name:     "no change",
			mutate:   func(t *types.TaskDef) {},
			wantSame: true,
		},
		{
			name:     "image change",
			mutate:   func(t *types.TaskDef) { t.Image = "nginx:1.27" },
			wantSame: false,
		},
		{
			// Replicas is deployment-scale, not per-container config:
			// scaling 1→5 must NOT trigger a rolling replace of the
			// existing replicas. See Hash() doc for the full exclusion list.
			name:     "replicas change is ignored",
			mutate:   func(t *types.TaskDef) { t.Replicas = 5 },
			wantSame: true,
		},
		{
			name:     "strategy change is ignored",
			mutate:   func(t *types.TaskDef) { t.Strategy = types.StrategyStopFirst },
			wantSame: true,
		},
		{
			name:     "env value change",
			mutate:   func(t *types.TaskDef) { t.Env["LOG_LEVEL"] = "debug" },
			wantSame: false,
		},
		{
			name:     "new env key",
			mutate:   func(t *types.TaskDef) { t.Env["NEW"] = "x" },
			wantSame: false,
		},
	}

	base := types.TaskDef{
		Project:  "default",
		Name:     "web",
		Image:    "nginx:alpine",
		Replicas: 3,
		Env:      map[string]string{"LOG_LEVEL": "info"},
	}
	baseHash := Hash(base)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := base
			if mutated.Env != nil {
				envCopy := make(map[string]string, len(mutated.Env))
				maps.Copy(envCopy, mutated.Env)
				mutated.Env = envCopy
			}
			tt.mutate(&mutated)

			got := Hash(mutated)
			if tt.wantSame && got != baseHash {
				t.Errorf("expected unchanged hash, got %s vs base %s", got, baseHash)
			}
			if !tt.wantSame && got == baseHash {
				t.Errorf("expected hash to change after %q mutation, got identical", tt.name)
			}
		})
	}
}

func TestHashFormat(t *testing.T) {
	got := Hash(types.TaskDef{Name: "x", Image: "y"})
	// "sha256:" + 64 hex chars = 71 chars total
	if len(got) != 71 {
		t.Errorf("Hash length = %d, want 71 (sha256: prefix + 64 hex)", len(got))
	}
	if got[:7] != "sha256:" {
		t.Errorf("Hash prefix = %q, want 'sha256:'", got[:7])
	}
}

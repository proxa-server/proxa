package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/proxa-server/proxa/internal/config"
)

func TestRunUpFailsWhenNotInitialized(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, ListenAddr: "unix://" + filepath.Join(dir, "proxa.sock")}

	// Write a valid TOML to a temp file.
	tomlPath := filepath.Join(dir, "svc.toml")
	if err := os.WriteFile(tomlPath, []byte("name = \"web\"\nimage = \"nginx:alpine\"\nreplicas = 1\n[[expose]]\ncontainer = 80\nhost = 0\nprotocol = \"http\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := runUp(context.Background(), cfg, tomlPath)
	if err == nil {
		t.Fatalf("expected error when not initialized")
	}
	if !strings.Contains(err.Error(), "proxa init") {
		t.Errorf("error should mention 'proxa init', got: %v", err)
	}
}

func TestRunUpFailsWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	// Touch the token file so the precheck passes.
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("dummy\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{DataDir: dir, ListenAddr: "unix://" + filepath.Join(dir, "proxa.sock")}

	err := runUp(context.Background(), cfg, filepath.Join(dir, "does-not-exist.toml"))
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}

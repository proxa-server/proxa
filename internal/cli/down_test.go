package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/proxa-server/proxa/internal/config"
)

// fakeAPI returns a *httptest.Server that records POST .../scale calls
// and answers per the configured statusCode.
type fakeAPI struct {
	*httptest.Server
	receivedReplicas []int
	statusCode       int
}

func newFakeAPI(t *testing.T, statusCode int) *fakeAPI {
	t.Helper()
	f := &fakeAPI{statusCode: statusCode}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/projects/default/services/web/scale", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Replicas int `json:"replicas"` }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.receivedReplicas = append(f.receivedReplicas, body.Replicas)
		w.WriteHeader(f.statusCode)
	})
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func writeToken(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunDownPostsZeroReplicas(t *testing.T) {
	api := newFakeAPI(t, http.StatusOK)
	dataDir := writeToken(t)
	cfg := &config.Config{DataDir: dataDir, ListenAddr: api.URL}

	if err := runDown(context.Background(), cfg, "default", "web"); err != nil {
		t.Fatalf("runDown: %v", err)
	}
	if len(api.receivedReplicas) != 1 || api.receivedReplicas[0] != 0 {
		t.Errorf("expected one scale call with replicas=0, got %v", api.receivedReplicas)
	}
}

func TestRunDownIdempotentOnMissingService(t *testing.T) {
	api := newFakeAPI(t, http.StatusNotFound)
	dataDir := writeToken(t)
	cfg := &config.Config{DataDir: dataDir, ListenAddr: api.URL}

	if err := runDown(context.Background(), cfg, "default", "web"); err != nil {
		t.Errorf("down on missing service should be idempotent, got: %v", err)
	}
}

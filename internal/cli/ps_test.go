package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/proxa-server/proxa/internal/config"
)

func newPsAPI(t *testing.T, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/system/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func psCfg(t *testing.T, listenURL string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return &config.Config{DataDir: dir, ListenAddr: listenURL}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = orig
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	return buf.String()
}

func TestRunPsRendersTable(t *testing.T) {
	api := newPsAPI(t, `{
		"node": {"id":"node-local","status":"ready","containerCount":2},
		"projects": [
			{"name":"default","services":[
				{"name":"web","image":"nginx:alpine","desiredReplicas":2,"actualReplicas":2,"status":"healthy"}
			]}
		]
	}`)
	cfg := psCfg(t, api.URL)

	out := captureStdout(t, func() {
		if err := runPs(context.Background(), cfg, false); err != nil {
			t.Errorf("runPs: %v", err)
		}
	})

	for _, want := range []string{"PROJECT", "SERVICE", "default", "web", "nginx:alpine", "healthy"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestRunPsEmptyCluster(t *testing.T) {
	api := newPsAPI(t, `{"node":{"id":"node-local","status":"ready","containerCount":0},"projects":[]}`)
	cfg := psCfg(t, api.URL)

	out := captureStdout(t, func() {
		_ = runPs(context.Background(), cfg, false)
	})

	if !strings.Contains(out, "PROJECT") {
		t.Errorf("expected header even on empty cluster, got: %s", out)
	}
}

func TestRunPsJSONOutput(t *testing.T) {
	api := newPsAPI(t, `{"node":{"id":"x","status":"ready","containerCount":0},"projects":[]}`)
	cfg := psCfg(t, api.URL)

	out := captureStdout(t, func() {
		_ = runPs(context.Background(), cfg, true)
	})

	if !strings.Contains(out, `"node"`) || !strings.Contains(out, `"projects"`) {
		t.Errorf("expected JSON envelope, got: %s", out)
	}
}

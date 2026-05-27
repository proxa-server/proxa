//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/proxa-server/proxa/tests/e2e/internal/harness"
)

// TestSC_003_HotReloadNo5xx covers SC-003: while a sustained curl-
// equivalent loop hits the ingress, the operator re-runs `proxa up`
// with a modified TOML. The atomic.Pointer router swap (FR-009 +
// R-003) must produce ZERO 5xx responses in the loop.
func TestSC_003_HotReloadNo5xx(t *testing.T) {
	harness.SCAttrs(t, "006-test-foundation-public-images", "SC-003")
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}

	dir := t.TempDir()
	if out, err := harness.RunProxa(t, dir, "init"); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	httpPort, httpsPort := harness.PickTwoFreeTCPPorts(t)
	cfg := fmt.Sprintf("[ingress]\nhttp_port = %d\nhttps_port = %d\ntls = true\nemail = \"\"\n", httpPort, httpsPort)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	stop := harness.StartServer(t, dir)
	defer stop()

	backendHostPort, _ := harness.PickTwoFreeTCPPorts(t)
	tomlPath := filepath.Join(dir, "reload.toml")
	tomlV1 := fmt.Sprintf(`
name     = "reloadsvc"
image    = "traefik/whoami:latest"
replicas = 1

[[expose]]
container = 80
host      = %d
protocol  = "http"

[[route]]
host = "reload.local"
`, backendHostPort)
	if err := os.WriteFile(tomlPath, []byte(tomlV1), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up v1: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", "proxa-default-reloadsvc-0").Run()
	})

	harness.WaitForCount(t, "reloadsvc", 1, 30*time.Second)
	if !harness.WaitForServiceStatus(t, dir, "reloadsvc", "healthy", 30*time.Second) {
		t.Fatalf("reloadsvc never healthy")
	}

	// Background load: hit the ingress every 50ms for 15s.
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true, ServerName: "reload.local"},
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 1 * time.Second}).DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", httpsPort))
			},
		},
	}

	var (
		fiveXX atomic.Int64
		errs   atomic.Int64
		ok     atomic.Int64
	)
	loopCtx, loopCancel := context.WithCancel(context.Background())
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-loopCtx.Done():
				return
			case <-ticker.C:
				req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("https://reload.local:%d/", httpsPort), nil)
				req.Host = "reload.local"
				resp, err := client.Do(req)
				if err != nil {
					errs.Add(1)
					continue
				}
				resp.Body.Close()
				switch {
				case resp.StatusCode >= 500:
					fiveXX.Add(1)
				case resp.StatusCode == 200:
					ok.Add(1)
				}
			}
		}
	}()

	// Let the loop warm up.
	time.Sleep(2 * time.Second)

	// Hot-edit: flip lb_strategy and re-up.
	tomlV2 := fmt.Sprintf(`
name     = "reloadsvc"
image    = "traefik/whoami:latest"
replicas = 1

[[expose]]
container = 80
host      = %d
protocol  = "http"

[[route]]
host = "reload.local"
lb_strategy = "round-robin"
`, backendHostPort)
	if err := os.WriteFile(tomlPath, []byte(tomlV2), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := harness.RunProxa(t, dir, "up", tomlPath); err != nil {
		t.Fatalf("up v2: %v\n%s", err, out)
	}

	// Keep hitting for another 10s so the reload window is fully covered.
	time.Sleep(10 * time.Second)

	loopCancel()
	<-loopDone

	gotOK := ok.Load()
	got5xx := fiveXX.Load()
	gotErr := errs.Load()
	t.Logf("loop summary: 2xx=%d 5xx=%d transport-err=%d", gotOK, got5xx, gotErr)

	if got5xx > 0 {
		t.Errorf("got %d 5xx responses during reload; SC-003 requires zero", got5xx)
	}
	if gotOK == 0 {
		t.Errorf("got 0 2xx responses; loop infrastructure broken")
	}
}

//go:build dockerd

package ingress

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/proxa-server/proxa/internal/config"
)

// TestIntegration_PebbleACMEDirectory brings up Pebble (Let's Encrypt's
// local test CA) and verifies the tlsProvider can reach its ACME
// directory endpoint. This is the lightweight gate for the ACME path
// — full HTTP-01 challenge requires the ingress to bind a public port
// for Pebble to dial back, which only works on a routable host.
//
// Run with:
//
//	go test -tags dockerd -count=1 ./internal/ingress/...
func TestIntegration_PebbleACMEDirectory(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pebble integration in -short mode")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker CLI not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Spin up Pebble with PEBBLE_VA_NOSLEEP=1 for fast challenges.
	const pebbleName = "proxa-test-pebble"
	_ = exec.Command("docker", "rm", "-f", pebbleName).Run()
	cmd := exec.CommandContext(ctx, "docker", "run", "-d",
		"--name", pebbleName,
		"-p", "14000:14000",
		"-p", "15000:15000",
		"-e", "PEBBLE_VA_NOSLEEP=1",
		"ghcr.io/letsencrypt/pebble:latest",
		"pebble", "-dnsserver", "127.0.0.1:8053")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("could not start pebble (likely no image pulled / no network): %v\n%s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", pebbleName).Run() })

	// Wait for Pebble's directory endpoint to come up.
	pebbleURL := "https://localhost:14000/dir"
	client := &http.Client{
		Timeout:   3 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	deadline := time.Now().Add(30 * time.Second)
	var got *http.Response
	for time.Now().Before(deadline) {
		resp, err := client.Get(pebbleURL)
		if err == nil && resp.StatusCode == 200 {
			got = resp
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if got == nil {
		t.Fatalf("pebble directory %s never responded 200 within 30s", pebbleURL)
	}
	got.Body.Close()

	// Construct the tlsProvider configured against Pebble.
	cfg := config.IngressConfig{
		HTTPPort:         pickFreePort(t),
		HTTPSPort:        pickFreePort(t),
		TLS:              true,
		Email:            "ops@proxa-test.local",
		ACMEDirectoryURL: pebbleURL,
	}
	dataDir := t.TempDir()
	provider := newTLSProvider(cfg, dataDir, slog.New(slog.DiscardHandler))

	// Just verifying acmeConfig() returns without error is enough for
	// the directory-reachability gate. Full cert issuance with HTTP-01
	// requires Pebble to dial back into our ingress on port 80, which
	// only works on a public IP; out of scope for this lightweight test.
	tlsConf, err := provider.TLSConfig(ctx)
	if err != nil {
		t.Fatalf("TLSConfig against pebble: %v", err)
	}
	if tlsConf == nil {
		t.Fatalf("nil TLSConfig — provider didn't construct ACME path")
	}

	// Sanity: provider.CertCount should be 0 (no cert yet) and CertInfo
	// for an unknown host should report pending (route declared, ACME
	// not yet succeeded).
	if got := provider.CertCount(); got != 0 {
		t.Errorf("CertCount = %d, want 0 before issuance", got)
	}
	info, ok := provider.CertInfo("proxa-test.local")
	if !ok || info.Status != CertStatusPending {
		t.Errorf("CertInfo before issuance = %+v ok=%v; want Pending", info, ok)
	}
	_ = net.IPv4 // keep net import used in build-tagged file even when not exercised
}

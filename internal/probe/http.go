package probe

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// HTTPProbe performs an HTTP GET against a container's bridge-network
// IP (NOT the host port — see specs/002-health-checks/research.md R-001).
// 2xx response within Timeout = success.
type HTTPProbe struct {
	URL     string
	Timeout time.Duration
	client  *http.Client
}

// NewHTTPProbe builds a probe targeting http://<containerIP>:<port><path>.
// The HTTP client uses a tight IdleConnTimeout so connections to a
// removed container don't linger.
func NewHTTPProbe(containerIP string, port int, path string, timeout time.Duration) *HTTPProbe {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if path == "" {
		path = "/"
	}
	url := fmt.Sprintf("http://%s:%d%s", containerIP, port, path)

	return &HTTPProbe{
		URL:     url,
		Timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 1,
				IdleConnTimeout:     15 * time.Second,
				DialContext: (&net.Dialer{
					Timeout: timeout,
				}).DialContext,
			},
		},
	}
}

// Name returns "http".
func (p *HTTPProbe) Name() string { return "http" }

// Run issues GET <URL> with the configured timeout. 2xx = healthy.
func (p *HTTPProbe) Run(ctx context.Context) Result {
	start := time.Now()
	probeCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, p.URL, nil)
	if err != nil {
		return Result{At: start, Healthy: false, Latency: time.Since(start), Err: fmt.Errorf("probe/http: build request: %w", err)}
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return Result{At: start, Healthy: false, Latency: time.Since(start), Err: fmt.Errorf("probe/http: %s: %w", p.URL, err)}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return Result{At: start, Healthy: true, Latency: time.Since(start)}
	}
	return Result{At: start, Healthy: false, Latency: time.Since(start),
		Err: fmt.Errorf("probe/http: %s returned status %d", p.URL, resp.StatusCode)}
}

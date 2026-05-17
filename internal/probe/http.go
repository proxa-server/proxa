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
// IP (NOT the host port — see specs/002-health-checks/research.md R-001)
// OR a loopback ingress port with a route-host header injected when
// constructed via NewHTTPProbeViaIngress.
// 2xx response within Timeout = success.
type HTTPProbe struct {
	URL     string
	Host    string // when non-empty, overrides Request.Host (probe-via-ingress)
	Timeout time.Duration
	client  *http.Client
}

// newProbeClient builds the same client both HTTPProbe constructors use.
func newProbeClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1,
			IdleConnTimeout:     15 * time.Second,
			DialContext: (&net.Dialer{
				Timeout: timeout,
			}).DialContext,
		},
	}
}

// NewHTTPProbeViaIngress builds a probe targeting 127.0.0.1:<ingressPort>
// with the route's hostname injected as the Host header. Used when a
// service declares [health].via = "ingress" — bypasses direct
// bridge-IP dial (works on macOS Docker Desktop).
//
// The underlying http.Client mutates the Host header on each request
// via Request.Host; the URL is always loopback.
func NewHTTPProbeViaIngress(ingressPort int, routeHost, path string, timeout time.Duration) *HTTPProbe {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if path == "" {
		path = "/"
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", ingressPort, path)
	return &HTTPProbe{
		URL:     url,
		Host:    routeHost,
		Timeout: timeout,
		client:  newProbeClient(timeout),
	}
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
		client:  newProbeClient(timeout),
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
	if p.Host != "" {
		req.Host = p.Host
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

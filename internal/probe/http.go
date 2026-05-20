package probe

import (
	"context"
	"crypto/tls"
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

// IngressInfo carries the loopback ports + TLS state the via-ingress
// probe needs to construct its target URL. Populated by the probe
// Manager from probe.Options at construction time.
//
// When TLSEnabled is true, NewHTTPProbeViaIngress targets HTTPSPort
// directly (with InsecureSkipVerify=true scoped to the probe's loopback
// client) instead of HTTPPort, to avoid the HTTP→HTTPS redirect cycle
// that broke probes in v0.4.0 (specs/005-modern-go/research.md R-001).
type IngressInfo struct {
	HTTPPort   int
	HTTPSPort  int
	TLSEnabled bool
}

// newProbeClient builds the *http.Client used by both HTTPProbe
// constructors. insecureSkipVerify is scoped to the returned client —
// it does NOT propagate to any other client in the codebase, and
// applies only to the probe's loopback target (127.0.0.1:<ingress>).
//
// followRedirects controls the client's CheckRedirect:
//
//	nil          → Go default (follow up to 10).
//	*true        → same as nil.
//	*false       → return http.ErrUseLastResponse on the first 3xx,
//	               so the caller observes the redirect status directly.
func newProbeClient(timeout time.Duration, insecureSkipVerify bool, followRedirects *bool) *http.Client {
	c := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1,
			IdleConnTimeout:     15 * time.Second,
			DialContext: (&net.Dialer{
				Timeout: timeout,
			}).DialContext,
			TLSClientConfig: &tls.Config{
				// Loopback-only: probe targets 127.0.0.1:<ingress-https-port>
				// where CertMagic may have issued a self-signed staging cert.
				// Real operator traffic still validates the cert normally;
				// this skip is bounded to *this* probe client instance.
				InsecureSkipVerify: insecureSkipVerify, //nolint:gosec // see comment above
			},
		},
	}
	if followRedirects != nil && !*followRedirects {
		c.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return c
}

// NewHTTPProbeViaIngress builds a probe targeting Proxa's own ingress
// loopback. Used when a service declares [health].via = "ingress" —
// bypasses direct bridge-IP dial (works on macOS Docker Desktop).
//
// Target selection (specs/005-modern-go/research.md R-001 Approach B):
//
//	ing.TLSEnabled && followRedirects == nil → https://127.0.0.1:HTTPSPort
//	                                           with cert verify off (loopback)
//	otherwise                                → http://127.0.0.1:HTTPPort
//
// The underlying http.Client mutates the Host header on each request
// via Request.Host so the ingress sees the operator-declared route host.
func NewHTTPProbeViaIngress(ing IngressInfo, routeHost, path string, timeout time.Duration, followRedirects *bool) *HTTPProbe {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	if path == "" {
		path = "/"
	}

	scheme := "http"
	port := ing.HTTPPort
	insecureSkipVerify := false

	// The v0.4.1 fix: when ingress is TLS-enabled and the operator
	// did not explicitly override redirect behavior, probe the HTTPS
	// port directly. Real traffic terminates TLS at the ingress, so
	// the probe is asserting the same thing real traffic asserts:
	// "does the upstream return 2xx when reached through ingress?".
	if ing.TLSEnabled && followRedirects == nil && ing.HTTPSPort > 0 {
		scheme = "https"
		port = ing.HTTPSPort
		insecureSkipVerify = true
	}

	url := fmt.Sprintf("%s://127.0.0.1:%d%s", scheme, port, path)
	return &HTTPProbe{
		URL:     url,
		Host:    routeHost,
		Timeout: timeout,
		client:  newProbeClient(timeout, insecureSkipVerify, followRedirects),
	}
}

// NewHTTPProbe builds a probe targeting http://<containerIP>:<port><path>.
// The HTTP client uses a tight IdleConnTimeout so connections to a
// removed container don't linger.
//
// followRedirects: nil = Go default (follow up to 10); *true = same;
// *false = treat first 3xx as non-2xx (probe fails). Used when the
// operator wants to assert a redirect explicitly on a direct probe.
func NewHTTPProbe(containerIP string, port int, path string, timeout time.Duration, followRedirects *bool) *HTTPProbe {
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
		client:  newProbeClient(timeout, false, followRedirects),
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

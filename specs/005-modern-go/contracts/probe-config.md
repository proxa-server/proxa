# Contract — `probe.HTTPConfig` extension + via-ingress TLS handling

Implements **FR-005** (probe correctly distinguishes redirect from error when ingress has TLS enabled) and **FR-006** (operator can opt out of redirect following). Decision rationale: see `research.md` R-001.

## Package

```
internal/probe
```

## New field on `HTTPConfig`

```go
type HTTPConfig struct {
    // ... existing fields preserved ...

    // FollowRedirects controls HTTP redirect handling. Tri-state:
    //   nil   → use default for the probe's construction mode (see below)
    //   *true → follow up to 10 redirects, treat final response as outcome
    //   *false → do not follow; treat any 3xx as non-2xx (probe fails)
    //
    // Operator-supplied in TOML via `follow_redirects = true|false`.
    // Default behavior depends on construction:
    //   - NewHTTPProbe(...)            → nil means "follow up to 10" (Go default)
    //   - NewHTTPProbeViaIngress(...)  → nil means "follow up to 1 redirect
    //                                    only when ingress TLS=true; ignore
    //                                    cert verification on the redirect
    //                                    target" (the fix; see R-001 Approach B)
    FollowRedirects *bool
}
```

**Pointer-to-bool for tri-state** — Go idiom: `nil` = unset (use default), `*true`/`*false` = explicit. Allows additive default behavior without breaking call sites that don't set the field.

## Behavior matrix

| Probe mode | `FollowRedirects` | Behavior |
|---|---|---|
| Direct (`NewHTTPProbe`) | `nil` | Follow up to 10 (Go default) |
| Direct | `*true` | Same as nil |
| Direct | `*false` | No redirect; 3xx = failure |
| Via-ingress, TLS=false | `nil` | Follow up to 10 (no redirect expected — bridge IP, no ingress in the loop) |
| Via-ingress, TLS=true | `nil` | **THE FIX**: target HTTPS port directly, skip cert verify (loopback only), follow no redirects (none expected when targeting HTTPS directly) |
| Via-ingress, TLS=true | `*true` | Force redirect-following from HTTP→HTTPS port; cert verify off |
| Via-ingress, TLS=true | `*false` | Force HTTP-only probe; the 301 will mark the service unhealthy. Operator explicitly chose this to assert the redirect. |

## TOML surface

```toml
[health.http]
path = "/health"
port = 8080
interval = "10s"
timeout = "3s"
follow_redirects = true   # optional, tri-state via TOML presence
```

When `follow_redirects` is absent from TOML, the parser leaves `HTTPConfig.FollowRedirects` as `nil`, triggering the construction-mode default above.

## Implementation sketch

```go
// internal/probe/http.go

func NewHTTPProbeViaIngress(ing IngressInfo, host, path string, cfg HTTPConfig) *HTTPProbe {
    port := ing.HTTPPort
    scheme := "http"
    insecureSkipVerify := false
    
    // The fix: if ingress is TLS-enabled and operator didn't override,
    // probe the HTTPS port directly (avoids the redirect dance entirely).
    if ing.TLSEnabled && cfg.FollowRedirects == nil {
        port = ing.HTTPSPort
        scheme = "https"
        insecureSkipVerify = true  // loopback to our own ingress, cert is self-signed in dev
    }
    
    return &HTTPProbe{
        URL:     fmt.Sprintf("%s://127.0.0.1:%d%s", scheme, port, path),
        Host:    host,
        Timeout: cfg.Timeout,
        client:  newProbeClient(cfg.Timeout, insecureSkipVerify, cfg.FollowRedirects),
    }
}

func newProbeClient(timeout time.Duration, insecureSkipVerify bool, followRedirects *bool) *http.Client {
    c := &http.Client{
        Timeout: timeout,
        Transport: &http.Transport{
            MaxIdleConnsPerHost: 1,
            IdleConnTimeout:     15 * time.Second,
            TLSClientConfig: &tls.Config{InsecureSkipVerify: insecureSkipVerify},
        },
    }
    if followRedirects != nil && !*followRedirects {
        c.CheckRedirect = func(*http.Request, []*http.Request) error {
            return http.ErrUseLastResponse  // surface the 3xx to the caller
        }
    }
    return c
}
```

## Security note on `InsecureSkipVerify`

The skip applies ONLY to the probe client targeting `127.0.0.1:HTTPS_PORT` (our own ingress, loopback). It does NOT propagate to:
- The real ingress client serving operator traffic (uses CertMagic-issued certs, verified normally).
- Any other HTTP client in the codebase.
- The dashboard's API client (browser-side, uses the real cert).

This is loopback-only, scope-bounded, justified by: the probe is asserting "does the upstream return 2xx when reached the way real traffic reaches it" — and real traffic terminates TLS at the ingress, not the upstream. The probe is testing the *upstream*, not the TLS cert.

## Test coverage

`internal/probe/http_test.go` table extension:

| Case | Mode | `FollowRedirects` | Ingress TLS | Server | Expected |
|---|---|---|---|---|---|
| direct + default | Direct | nil | n/a | 200 | success |
| direct + redirect | Direct | nil | n/a | 301→200 | success (follow) |
| direct + no-follow | Direct | *false | n/a | 301 | failure (treats 3xx as non-2xx) |
| via-ingress non-TLS | ViaIngress | nil | false | 200 | success |
| via-ingress TLS default | ViaIngress | nil | true | https://...→200 | success (THE FIX) |
| via-ingress TLS force-follow | ViaIngress | *true | true | http://301→https://200 | success |
| via-ingress TLS force-nofollow | ViaIngress | *false | true | http://301 | failure (operator opted in to assert the redirect) |

`tests/e2e/probe_ingress_tls_test.go` reproduces the 0.4.0 demo bug end-to-end: nginx behind ingress with TLS=true + HTTP probe; assert service reaches `healthy` within 30s.

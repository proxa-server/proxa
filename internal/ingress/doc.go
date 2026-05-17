// Package ingress is Proxa's L7/L4 routing layer. It owns the HTTP/HTTPS
// listener, the ACME-driven TLS certificate lifecycle, the per-service
// backend pool, and the TCP/UDP forwarders. See specs/003-ingress/ for
// the full contract.
//
// The blank import below anchors the github.com/caddyserver/certmagic
// dependency in go.mod until the real tls.go lands (T015 of the
// implementation plan).
package ingress

import _ "github.com/caddyserver/certmagic"

package ingress

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"log/slog"
	"math/big"
	"path/filepath"
	"sync"
	"time"

	"github.com/caddyserver/certmagic"

	"github.com/proxa-server/proxa/internal/config"
)

// tlsProvider produces the *tls.Config for the HTTPS listener and
// reports certificate state for the dashboard.
//
// Three modes:
//
//   - cfg.TLS == false              → no TLS, returns (nil, nil). Caller
//                                     skips the HTTPS listener entirely.
//   - cfg.TLS == true && Email != ""→ ACME via Let's Encrypt (CertMagic).
//   - cfg.TLS == true && Email == ""→ self-signed (single in-memory cert
//                                     issued for the duration of the run).
//                                     Useful for tests and air-gapped dev.
type tlsProvider struct {
	cfg     config.IngressConfig
	dataDir string
	logger  *slog.Logger

	// ACME mode
	magic *certmagic.Config

	// Self-signed mode
	selfSigned *tls.Certificate

	// CertInfo cache
	mu        sync.RWMutex
	certCache map[string]CertInfo
}

// newTLSProvider constructs a tlsProvider for the given config.
func newTLSProvider(cfg config.IngressConfig, dataDir string, logger *slog.Logger) *tlsProvider {
	if logger == nil {
		logger = slog.Default()
	}
	return &tlsProvider{
		cfg:       cfg,
		dataDir:   dataDir,
		logger:    logger,
		certCache: make(map[string]CertInfo),
	}
}

// TLSConfig returns the *tls.Config to install on the HTTPS listener,
// or (nil, nil) when TLS is disabled. Initializes ACME / self-signed
// lazily on first call.
func (p *tlsProvider) TLSConfig(ctx context.Context) (*tls.Config, error) {
	if !p.cfg.TLS {
		return nil, nil
	}
	if p.cfg.Email == "" {
		return p.selfSignedConfig()
	}
	return p.acmeConfig(ctx)
}

func (p *tlsProvider) acmeConfig(ctx context.Context) (*tls.Config, error) {
	if p.magic != nil {
		return p.magic.TLSConfig(), nil
	}

	storage := &certmagic.FileStorage{Path: filepath.Join(p.dataDir, "certs")}
	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			return p.magic, nil
		},
	})
	p.magic = certmagic.New(cache, certmagic.Config{Storage: storage})

	acmeIssuer := certmagic.NewACMEIssuer(p.magic, certmagic.ACMEIssuer{
		Email:  p.cfg.Email,
		Agreed: true,
		CA:     p.cfg.ACMEDirectoryURL, // empty → Let's Encrypt prod default
	})
	p.magic.Issuers = []certmagic.Issuer{acmeIssuer}

	// On-demand: only attempt issuance for hostnames the operator has
	// declared via [[route]]. The router holds the authoritative set.
	p.magic.OnDemand = &certmagic.OnDemandConfig{
		DecisionFunc: func(ctx context.Context, name string) error {
			// Caller installs the actual hostname-allow check in
			// certMagicIngress.Run via SetAllowedHosts.
			return nil
		},
	}

	p.logger.Info("ingress: ACME enabled",
		"email", p.cfg.Email,
		"ca", p.cfg.ACMEDirectoryURL,
		"storage", storage.Path)
	return p.magic.TLSConfig(), nil
}

// selfSignedConfig generates one ECDSA cert with a wildcard-friendly CN
// (good for localhost + arbitrary test hostnames) and returns a tls.Config
// that serves it for every SNI lookup.
func (p *tlsProvider) selfSignedConfig() (*tls.Config, error) {
	if p.selfSigned == nil {
		cert, err := generateSelfSigned()
		if err != nil {
			return nil, fmt.Errorf("ingress: self-signed cert: %w", err)
		}
		p.selfSigned = cert
		p.logger.Info("ingress: self-signed TLS enabled (set [ingress].email to switch to ACME)")
	}
	return &tls.Config{
		GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			return p.selfSigned, nil
		},
		MinVersion: tls.VersionTLS12,
	}, nil
}

// generateSelfSigned creates one short-lived self-signed cert that covers
// localhost, 127.0.0.1, and any SNI (CN = "proxa-self-signed").
func generateSelfSigned() (*tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, err
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "proxa-self-signed"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost", "*"},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  priv,
		Leaf:        &tmpl,
	}, nil
}

// CertInfo returns the dashboard's TLS chip view for a hostname.
func (p *tlsProvider) CertInfo(host string) (CertInfo, bool) {
	if !p.cfg.TLS {
		return CertInfo{Host: host, Status: CertStatusOff}, true
	}
	if p.cfg.Email == "" {
		// Self-signed: always valid for the lifetime of the process.
		return CertInfo{Host: host, Status: CertStatusValid, NotAfter: time.Now().Add(365 * 24 * time.Hour)}, true
	}
	p.mu.RLock()
	info, ok := p.certCache[host]
	p.mu.RUnlock()
	if ok {
		return info, true
	}
	return CertInfo{Host: host, Status: CertStatusPending}, true
}

// recordCertResult updates the cache when an ACME attempt completes.
func (p *tlsProvider) recordCertResult(host string, info CertInfo) {
	p.mu.Lock()
	p.certCache[host] = info
	p.mu.Unlock()
}

// CertCount returns the number of hostnames with a known cert state.
func (p *tlsProvider) CertCount() int {
	if !p.cfg.TLS {
		return 0
	}
	if p.cfg.Email == "" {
		// Self-signed serves one cert covering all hostnames.
		return 1
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.certCache)
}

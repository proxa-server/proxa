// Package config loads Proxa runtime configuration via viper.
//
// Read order: defaults → ${HOME}/.proxa/config.toml → env vars
// (PROXA_*) → CLI flags. The viper layer keeps each source's values
// separate so the highest-priority source wins per key.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the resolved runtime configuration.
type Config struct {
	DataDir      string        // ${HOME}/.proxa
	ListenAddr   string        // unix:///{DataDir}/proxa.sock
	TickInterval time.Duration // reconciler cadence; 5s default
	LogLevel     string        // debug | info | warn | error
	Ingress      IngressConfig // L7/L4 routing layer; see [ingress] block
}

// IngressConfig holds the server-wide ingress settings. Read from the
// [ingress] section of config.toml + PROXA_INGRESS_* env vars.
//
// Defaults are dev-friendly (non-privileged ports, TLS off). Production
// operators are expected to set http_port=80, https_port=443, tls=true,
// email="ops@example.com".
type IngressConfig struct {
	HTTPPort         int    // [ingress].http_port; default 8080
	HTTPSPort        int    // [ingress].https_port; default 8443
	TLS              bool   // [ingress].tls; default false
	Email            string // [ingress].email; required when TLS=true for ACME (else self-signed)
	ACMEDirectoryURL string // [ingress].acme_directory; default Let's Encrypt prod
}

// Load reads configuration from defaults, ~/.proxa/config.toml, env
// vars, and viper-bound flags.
func Load() (*Config, error) {
	v := viper.New()

	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("config: home dir: %w", err)
	}
	defaultDataDir := filepath.Join(home, ".proxa")
	defaultListen := "unix://" + filepath.Join(defaultDataDir, "proxa.sock")

	v.SetDefault("data_dir", defaultDataDir)
	v.SetDefault("listen", defaultListen)
	v.SetDefault("tick_interval", "5s")
	v.SetDefault("log_level", "info")
	v.SetDefault("ingress.http_port", 8080)
	v.SetDefault("ingress.https_port", 8443)
	v.SetDefault("ingress.tls", false)
	v.SetDefault("ingress.email", "")
	v.SetDefault("ingress.acme_directory", "")

	v.SetConfigName("config")
	v.SetConfigType("toml")
	// Look in the env-overridden data dir FIRST (lets tests + dev set
	// $PROXA_DATA_DIR to a tempdir and drop a config.toml there);
	// fall back to the default ${HOME}/.proxa.
	if envDir := os.Getenv("PROXA_DATA_DIR"); envDir != "" {
		v.AddConfigPath(envDir)
	}
	v.AddConfigPath(defaultDataDir)
	v.SetEnvPrefix("PROXA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Don't error if config.toml is absent — defaults + env are enough.
	_ = v.ReadInConfig()

	tick, err := time.ParseDuration(v.GetString("tick_interval"))
	if err != nil {
		return nil, fmt.Errorf("config: tick_interval %q: %w", v.GetString("tick_interval"), err)
	}

	dataDir := v.GetString("data_dir")
	listen := v.GetString("listen")
	// If listen is the default Unix socket but data_dir was overridden,
	// re-derive listen so it points inside the new data_dir.
	if strings.HasPrefix(listen, "unix://"+filepath.Join(home, ".proxa")) && dataDir != defaultDataDir {
		listen = "unix://" + filepath.Join(dataDir, "proxa.sock")
	}

	return &Config{
		DataDir:      dataDir,
		ListenAddr:   listen,
		TickInterval: tick,
		LogLevel:     v.GetString("log_level"),
		Ingress: IngressConfig{
			HTTPPort:         v.GetInt("ingress.http_port"),
			HTTPSPort:        v.GetInt("ingress.https_port"),
			TLS:              v.GetBool("ingress.tls"),
			Email:            v.GetString("ingress.email"),
			ACMEDirectoryURL: v.GetString("ingress.acme_directory"),
		},
	}, nil
}

// SocketPath extracts the path from a unix:// listen address.
// Returns empty string for non-Unix listeners.
func (c *Config) SocketPath() string {
	if !strings.HasPrefix(c.ListenAddr, "unix://") {
		return ""
	}
	return strings.TrimPrefix(c.ListenAddr, "unix://")
}

// IsUnixListener reports whether ListenAddr is a Unix-socket listener.
func (c *Config) IsUnixListener() bool {
	return strings.HasPrefix(c.ListenAddr, "unix://")
}

package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/proxa-server/proxa/internal/auth/dbpolicy"
	"github.com/proxa-server/proxa/internal/auth/token"
	"github.com/proxa-server/proxa/internal/config"
	"github.com/proxa-server/proxa/internal/ingress"
	"github.com/proxa-server/proxa/internal/probe"
	"github.com/proxa-server/proxa/internal/reconciler"
	"github.com/proxa-server/proxa/internal/runtime/docker"
	"github.com/proxa-server/proxa/internal/server"
	"github.com/proxa-server/proxa/internal/store/sqlite"
)

func newServerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "Run the Proxa control plane (HTTP API + reconciler loop) in the foreground",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return runServer(cmd.Context(), cfg)
		},
	}
}

func runServer(ctx context.Context, cfg *config.Config) error {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Verify the data dir exists (proxa init ran).
	dbPath := filepath.Join(cfg.DataDir, "proxa.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return fmt.Errorf("server: %s missing — run `proxa init` first", dbPath)
	}

	// Open store.
	st := sqlite.New()
	if err := st.Open(ctx, dbPath); err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	// Open Docker runtime.
	rt, err := docker.New(ctx, "node-local")
	if err != nil {
		return err
	}
	defer rt.Close()

	// Auth: token only in v0.0 (password lands with the dashboard).
	authn := token.New(st)
	authz := dbpolicy.New(st)

	// Probe manager (health checks per replica). Pass the ingress
	// HTTP/HTTPS ports + TLS state so probes can opt into via=ingress
	// and (when TLS is on) bypass the HTTP→HTTPS redirect dance that
	// broke probes in v0.4.0.
	probes := probe.NewWithOptions(rt, logger, probe.Options{
		IngressHTTPPort:   cfg.Ingress.HTTPPort,
		IngressHTTPSPort:  cfg.Ingress.HTTPSPort,
		IngressTLSEnabled: cfg.Ingress.TLS,
	})

	// Ingress (L7/L4 routing layer).
	ingressCtl := ingress.New(cfg.Ingress, cfg.DataDir, logger)

	// Reconciler.
	recon := reconciler.New(st, rt, reconciler.Options{
		TickInterval: cfg.TickInterval,
		Logger:       logger,
		Probes:       probes,
		Ingress:      ingressCtl,
	})

	// Server.
	srv := server.New(cfg, st, rt, recon, authn, authz)
	srv.WithIngress(ingressCtl)
	srv.MountRoutes()
	srv.MountUI()

	// Run reconciler + ingress in background, server in foreground.
	reconCtx, cancelRecon := context.WithCancel(ctx)
	defer cancelRecon()
	go recon.Run(reconCtx)
	go func() {
		if err := ingressCtl.Run(reconCtx); err != nil {
			logger.Error("ingress: run failed", "err", err)
		}
	}()

	// Catch SIGINT/SIGTERM.
	sigCtx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("server starting", "listen", cfg.ListenAddr, "dataDir", cfg.DataDir)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start(sigCtx) }()

	select {
	case <-sigCtx.Done():
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

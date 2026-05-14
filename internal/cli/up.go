package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/proxa-server/proxa/internal/config"
	parsertoml "github.com/proxa-server/proxa/internal/parser/toml"
)

func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up <file.toml>",
		Short: "Parse a TOML task definition and apply it (creates or updates the service)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return runUp(cmd.Context(), cfg, args[0])
		},
	}
}

func runUp(ctx interface{ Done() <-chan struct{} }, cfg *config.Config, path string) error {
	// FR-007 / SC-007 precheck: token file presence implies `proxa init` ran.
	tokenPath := filepath.Join(cfg.DataDir, "token")
	if _, err := os.Stat(tokenPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("proxa not initialized; run `proxa init` first")
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cli: open %q: %w", path, err)
	}
	defer f.Close()

	td, err := parsertoml.Parse(f)
	if err != nil {
		return fmt.Errorf("invalid TOML: %w", err)
	}

	client, err := NewClient(cfg.ListenAddr, cfg.DataDir)
	if err != nil {
		return err
	}

	upCtx := ctxAdapter{ctx}
	svc, err := client.UpsertService(upCtx, td)
	if err != nil {
		return err
	}
	fmt.Printf("upserted service %q/%q (%d replicas requested)\n", svc.Project, svc.Name, svc.Spec.Replicas)
	fmt.Printf("reconciler will converge within ~%s\n", cfg.TickInterval)
	return nil
}

// ctxAdapter satisfies context.Context against the cmd.Context() Done()-only
// shape we accept above. Cobra's ctx is a real context.Context already; the
// adapter exists only because runUp's interface{Done} signature predates a
// later refactor and we keep it stable.
type ctxAdapter struct{ inner interface{ Done() <-chan struct{} } }

func (a ctxAdapter) Deadline() (time.Time, bool)       { return time.Time{}, false }
func (a ctxAdapter) Done() <-chan struct{}             { return a.inner.Done() }
func (a ctxAdapter) Err() error                        { return nil }
func (a ctxAdapter) Value(any) any                     { return nil }

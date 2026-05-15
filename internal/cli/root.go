// Package cli is the cobra-based command tree for the proxa binary.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/proxa-server/proxa/internal/version"
)

// NewRoot builds the proxa root command with all subcommands attached.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:     "proxa",
		Short:   "Proxa — self-hosted container orchestrator",
		Version: version.Version + " (commit " + version.Commit + ", built " + version.BuildDate + ")",
	}

	root.PersistentFlags().String("data-dir", "", "data directory (default ~/.proxa, env PROXA_DATA_DIR)")
	root.PersistentFlags().String("listen", "", "API listener (default unix://${data-dir}/proxa.sock)")
	root.PersistentFlags().String("log-level", "info", "log level (debug|info|warn|error)")

	root.AddCommand(newInitCmd())
	root.AddCommand(newServerCmd())
	root.AddCommand(newUpCmd())
	root.AddCommand(newPsCmd())

	return root
}

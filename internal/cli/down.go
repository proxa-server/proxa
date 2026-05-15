package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/proxa-server/proxa/internal/config"
)

func newDownCmd() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "down <service>",
		Short: "Scale a service to zero replicas (idempotent)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return runDown(cmd.Context(), cfg, project, args[0])
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "default", "project the service lives in")
	return cmd
}

func runDown(ctx context.Context, cfg *config.Config, project, service string) error {
	client, err := NewClient(cfg.ListenAddr, cfg.DataDir)
	if err != nil {
		return err
	}
	if err := client.Scale(ctx, project, service, 0); err != nil {
		return err
	}
	fmt.Printf("scaled %q/%q to 0 replicas; reconciler will remove containers within ~%s\n",
		project, service, cfg.TickInterval)
	return nil
}

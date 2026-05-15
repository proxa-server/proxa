package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/proxa-server/proxa/internal/config"
)

func newPsCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "ps",
		Short: "List running services across all projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return runPs(cmd.Context(), cfg, jsonOut)
		},
	}
	cmd.Flags().BoolVarP(&jsonOut, "json", "j", false, "emit JSON instead of a table")
	return cmd
}

func runPs(ctx context.Context, cfg *config.Config, jsonOut bool) error {
	client, err := NewClient(cfg.ListenAddr, cfg.DataDir)
	if err != nil {
		return err
	}
	status, err := client.SystemStatus(ctx)
	if err != nil {
		return err
	}

	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(status)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROJECT\tSERVICE\tIMAGE\tDESIRED\tACTUAL\tSTATUS")

	projects, _ := status["projects"].([]any)
	rows := 0
	for _, p := range projects {
		proj, _ := p.(map[string]any)
		name, _ := proj["name"].(string)
		svcs, _ := proj["services"].([]any)
		for _, s := range svcs {
			svc, _ := s.(map[string]any)
			fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%v\t%v\n",
				name,
				stringField(svc, "name"),
				stringField(svc, "image"),
				numField(svc, "desiredReplicas"),
				numField(svc, "actualReplicas"),
				stringField(svc, "status"),
			)
			rows++
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if rows == 0 {
		fmt.Fprintln(os.Stderr, "(no services — deploy one with `proxa up <file.toml>`)")
	}
	return nil
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func numField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return 0
}

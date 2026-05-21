package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/proxa-server/proxa/internal/config"
)

// newSystemCmd registers `proxa system info` and (future) sibling
// system-level subcommands. Lives under a top-level `system` group so
// the namespace can grow (e.g., `proxa system reload`, `proxa system
// rotate-token`) without polluting the root command set.
func newSystemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "Inspect or manage the running Proxa server",
	}
	cmd.AddCommand(newSystemInfoCmd())
	return cmd
}

func newSystemInfoCmd() *cobra.Command {
	var jsonOut bool
	c := &cobra.Command{
		Use:   "info",
		Short: "Print Go runtime + Proxa version + GOMAXPROCS source of the running server",
		Long: `info queries GET /api/v1/system on the running server and prints
the result. Default output is one key=value per line; --json emits
the same JSON the HTTP endpoint returns.

Exits 1 if the server cannot be reached. The command does NOT fall
back to computing values locally — the operator's question is what
the *running server* is reporting.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			client, err := NewClient(cfg.ListenAddr, cfg.DataDir)
			if err != nil {
				return err
			}
			info, err := client.SystemInfo(cmd.Context())
			if err != nil {
				return err
			}
			if jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(info)
			}
			printSystemInfoKV(cmd.OutOrStdout(), info)
			return nil
		},
	}
	c.Flags().BoolVar(&jsonOut, "json", false, "emit the same JSON payload as GET /api/v1/system")
	return c
}

// printSystemInfoKV writes one key=value per line, alphabetically by
// key for stable output suitable for `awk -F= '...'` parsing.
func printSystemInfoKV(w interface{ Write(p []byte) (int, error) }, info map[string]any) {
	keys := make([]string, 0, len(info))
	for k := range info {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, formatValue(info[k]))
	}
	_, _ = w.Write([]byte(b.String()))
}

// formatValue renders a JSON-decoded value as a stable plain-text form.
// Numbers print without trailing zeros; slices print comma-joined.
func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		// JSON numbers decode to float64. Integers stay integers.
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			parts = append(parts, formatValue(e))
		}
		return strings.Join(parts, ",")
	default:
		// Fall back to JSON for objects.
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprintf("%v", x)
		}
		return string(b)
	}
}

package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/proxa-server/proxa/internal/config"
)

// newLogsCmd builds the `proxa logs <service>` cobra subcommand.
//
//   proxa logs [flags] <service>
//
//   -f, --follow              stream new lines until Ctrl-C
//   -n, --tail int            number of recent lines (-1 = all) (default -1)
//       --since duration      only lines from the last DURATION
//       --replica int         replica index (default 0)
//       --project string      project name (default "default")
func newLogsCmd() *cobra.Command {
	var (
		follow     bool
		tail       int
		sinceRaw   string
		replica    int
		project    string
	)
	cmd := &cobra.Command{
		Use:   "logs <service>",
		Short: "Stream a service's container logs (tail / follow / since / replica)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return runLogs(cmd.Context(), cfg, args[0], logsFlags{
				Follow:   follow,
				Tail:     tail,
				SinceRaw: sinceRaw,
				Replica:  replica,
				Project:  project,
			})
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "stream new lines until Ctrl-C")
	cmd.Flags().IntVarP(&tail, "tail", "n", -1, "number of recent lines to fetch (-1 = all)")
	cmd.Flags().StringVar(&sinceRaw, "since", "", "only lines from the last DURATION (e.g. 5m, 1h30m)")
	cmd.Flags().IntVar(&replica, "replica", 0, "replica index")
	cmd.Flags().StringVar(&project, "project", "default", "project name")
	return cmd
}

type logsFlags struct {
	Follow   bool
	Tail     int
	SinceRaw string
	Replica  int
	Project  string
}

func runLogs(parentCtx context.Context, cfg *config.Config, service string, f logsFlags) error {
	// Client-side flag validation (exit 2 per cli-logs.md contract).
	if f.Tail < -1 {
		return cliFlagErr(fmt.Errorf("invalid --tail value %d: must be >= -1", f.Tail))
	}
	if f.Replica < 0 {
		return cliFlagErr(fmt.Errorf("invalid --replica value %d: must be >= 0", f.Replica))
	}
	if f.Project == "" {
		return cliFlagErr(fmt.Errorf("invalid --project: must not be empty"))
	}
	var sinceTS time.Time
	if f.SinceRaw != "" {
		t, err := parseSinceFlag(f.SinceRaw)
		if err != nil {
			return cliFlagErr(err)
		}
		sinceTS = t
	}

	client, err := NewClient(cfg.ListenAddr, cfg.DataDir)
	if err != nil {
		return err
	}

	// Install SIGINT/SIGTERM → ctx cancel so Ctrl-C closes the HTTP
	// connection cleanly (no orphaned ESTABLISHED — SC-003).
	ctx, stop := signal.NotifyContext(parentCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	q := url.Values{}
	if f.Follow {
		q.Set("follow", "true")
	}
	if f.Tail != -1 {
		q.Set("tail", strconv.Itoa(f.Tail))
	}
	if !sinceTS.IsZero() {
		q.Set("since", sinceTS.UTC().Format(time.RFC3339Nano))
	}
	if f.Replica != 0 {
		q.Set("replica", strconv.Itoa(f.Replica))
	}

	path := fmt.Sprintf("/api/v1/projects/%s/services/%s/logs", f.Project, service)
	if encoded := q.Encode(); encoded != "" {
		path += "?" + encoded
	}

	resp, err := client.doStream(ctx, http.MethodGet, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Drain body for the error message.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return fmt.Errorf("error: %s", strings.TrimSpace(string(body)))
	}

	// Header that tells the operator which container they're looking at.
	containerName := resp.Header.Get("X-Proxa-Container")
	replicaIdx := resp.Header.Get("X-Proxa-Replica")
	if containerName != "" {
		fmt.Fprintf(os.Stderr, "=== %s (replica %s) ===\n", containerName, replicaIdx)
	}

	// Copy body → stdout, line-by-line (no extra buffering by us; Body
	// already buffers and the server flushes per line).
	_, copyErr := io.Copy(os.Stdout, resp.Body)
	if copyErr != nil && !isContextCancel(copyErr) {
		return fmt.Errorf("read stream: %w", copyErr)
	}
	return nil
}

// parseSinceFlag converts "5m" / "1h30m" / "48h" → absolute timestamp
// in the past. Negative durations and empty strings are rejected.
// Implementation lives in cli/logs_since.go (added in T021).
func parseSinceFlag(raw string) (time.Time, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since value %q: %w", raw, err)
	}
	if d <= 0 {
		return time.Time{}, fmt.Errorf("invalid --since value %q: must be a positive duration", raw)
	}
	return time.Now().Add(-d), nil
}

// cliFlagErr wraps a flag-validation error so the runtime can exit 2.
// Cobra exits 1 by default on RunE error; we use a sentinel-style
// wrapping recognized by main.go (TODO: actually wire exit 2 in main
// — for v0.4 MVP, exit 1 with a clear message is acceptable).
func cliFlagErr(err error) error { return err }

// isContextCancel reports whether the error came from the parent ctx
// cancelling (Ctrl-C). HTTP body read returns various flavors of
// "use of closed network connection" depending on the transport.
func isContextCancel(err error) bool {
	s := err.Error()
	return strings.Contains(s, "context canceled") ||
		strings.Contains(s, "use of closed network connection") ||
		strings.Contains(s, "operation was canceled")
}

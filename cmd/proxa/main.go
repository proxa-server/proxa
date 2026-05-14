// proxa is the Proxa control-plane binary. It hosts the API server,
// scheduler, ingress, dashboard, DNS, and CLI in a single process
// (constitution §V).
//
// Subcommands are wired by [internal/cli.NewRoot]; this main is just
// the entrypoint that delegates and reports the cobra error if any.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/proxa-server/proxa/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	root := cli.NewRoot()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

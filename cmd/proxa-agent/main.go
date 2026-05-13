// proxa-agent is the per-node Proxa agent binary. It registers the host
// with the control plane, reports heartbeats, and executes container
// operations on behalf of the scheduler.
//
// In feature 000-foundation this binary only supports the `version`
// subcommand. The full agent loop arrives in a later feature.
package main

import (
	"fmt"
	"os"

	"github.com/proxa-server/proxa/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Printf("proxa-agent version %s (commit %s, built %s)\n",
			version.Version, version.Commit, version.BuildDate)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: proxa-agent <command>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  version    print agent version and exit")
}

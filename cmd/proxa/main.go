// proxa is the Proxa control-plane binary. It hosts the API server,
// scheduler, ingress, dashboard, DNS, and CLI in a single process
// (constitution §V).
//
// In feature 000-foundation this binary only supports the `version`
// subcommand. The full server arrives in a later feature.
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
		fmt.Printf("proxa version %s (commit %s, built %s)\n",
			version.Version, version.Commit, version.BuildDate)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: proxa <command>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "commands:")
	fmt.Fprintln(os.Stderr, "  version    print version and exit")
}

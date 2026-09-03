// Command timetracker is a local-first time tracking CLI.
package main

import (
	"fmt"
	"os"

	"github.com/acramatte/timetracker-cli/internal/cli"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "timetracker:", err)
		os.Exit(1)
	}
}

// run is the testable entry point. It delegates argument parsing and
// dispatch to the Cobra command tree.
func run(args []string) error {
	root := cli.NewRootCommand()
	root.SetArgs(args)
	return root.Execute()
}

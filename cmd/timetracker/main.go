// Command timetracker is a local-first time tracking CLI.
package main

import (
	"fmt"
	"os"

	"github.com/acramatte/timetracker-cli/internal/cli"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is the testable entry point. It delegates argument parsing and
// dispatch to the Cobra command tree, then maps the outcome to the
// documented exit-code categories (spec §6.3).
func run(args []string) int {
	root := cli.NewRootCommand()
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		fmt.Fprintln(os.Stderr, "timetracker:", err)
	}
	return cli.ExitCode(err)
}

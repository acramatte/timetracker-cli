// Command timetracker is a local-first time tracking CLI.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "timetracker:", err)
		os.Exit(1)
	}
}

// run is the testable entry point. Phase 0 provides no commands yet;
// subsequent phases wire argument parsing and use cases here.
func run(args []string) error {
	_ = args
	return nil
}

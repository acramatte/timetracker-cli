// Package cli wires the timetracker command tree.
package cli

import "github.com/spf13/cobra"

// NewRootCommand builds the timetracker root command. Subsequent phases
// register use-case commands as children here.
func NewRootCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "timetracker",
		Short:         "Local-first time tracking CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}

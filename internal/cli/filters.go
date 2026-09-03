package cli

import (
	"github.com/spf13/cobra"

	"github.com/acramatte/timetracker-cli/internal/app"
)

// entryFilterFlags is the shared history filter set (entries list, report,
// export csv): inclusive local calendar dates, project, description text,
// and entry state.
type entryFilterFlags struct {
	from, to, project, query, status string
}

func (f *entryFilterFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.from, "from", "", "first local calendar date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&f.to, "to", "", "last local calendar date (YYYY-MM-DD)")
	cmd.Flags().StringVar(&f.project, "project", "", "project ID")
	cmd.Flags().StringVar(&f.query, "query", "", "case-insensitive description text")
	cmd.Flags().StringVar(&f.status, "status", "", "entry state: active, completed, or all")
}

func (f *entryFilterFlags) values() app.EntryFilters {
	return app.EntryFilters{
		From: f.from, To: f.to, ProjectID: f.project, Query: f.query, Status: f.status,
	}
}

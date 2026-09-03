package store

import (
	"context"
	"fmt"
)

// DoctorReport summarises storage health for the doctor command
// (spec 001-local-first-cli §11, task B7).
type DoctorReport struct {
	SchemaVersion int64
	Timezone      string
	TablesPresent []string
	Err           error
}

// Doctor checks the database: schema version, configured timezone, and the
// presence of expected tables. It is read-only and never mutates data.
func Doctor(ctx context.Context, s *Store) DoctorReport {
	report := DoctorReport{}

	version, err := SchemaVersion(ctx, s.DB)
	if err != nil {
		report.Err = fmt.Errorf("read schema version: %w", err)
		return report
	}
	report.SchemaVersion = version

	tz, err := s.LocalTimezone(ctx)
	if err != nil {
		report.Err = fmt.Errorf("read timezone: %w", err)
		return report
	}
	report.Timezone = tz.String()

	for _, table := range []string{"settings", "projects", "time_entries"} {
		var name string
		err := s.DB.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).
			Scan(&name)
		if err != nil {
			report.Err = fmt.Errorf("expected table %s missing: %w", table, err)
			return report
		}
		report.TablesPresent = append(report.TablesPresent, table)
	}

	return report
}

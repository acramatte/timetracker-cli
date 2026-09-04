package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// EntryFilter is the repository-level filter. From/To are UTC instants;
// the application converts inclusive local dates before calling the store.
type EntryFilter struct {
	From      *time.Time
	To        *time.Time // exclusive upper bound
	ProjectID string
	Query     string
	Status    string // active, completed, all; empty means all
}

// EntryResult includes the denormalised project name needed by reports and
// CSV export while retaining the canonical project ID on TimeEntry.
type EntryResult struct {
	Entry       TimeEntry
	ProjectName string
}

// entryResultColumns is the entry column list shared by ListEntries' query,
// kept in sync with scanEntry's expectations (project name appended last).
const entryResultColumns = `
	e.id, e.description, e.started_at, e.stopped_at, e.project_id,
	e.pomodoro, e.pomodoro_duration_seconds, e.pomodoro_ends_at,
	e.created_at, e.updated_at`

// ListEntries returns entries matching one filter definition. This is the
// shared query path for list, report, and CSV export (spec §10, task D1).
func (s *Store) ListEntries(ctx context.Context, filter EntryFilter) ([]EntryResult, error) {
	query := `
		SELECT ` + entryResultColumns + `, COALESCE(p.name, '')
		FROM time_entries e
		LEFT JOIN projects p ON p.id = e.project_id`
	var clauses []string
	args := make([]any, 0, 6)
	if filter.From != nil {
		clauses = append(clauses, "e.started_at >= ?")
		args = append(args, filter.From.UTC().Format(time.RFC3339))
	}
	if filter.To != nil {
		clauses = append(clauses, "e.started_at < ?")
		args = append(args, filter.To.UTC().Format(time.RFC3339))
	}
	if filter.ProjectID != "" {
		clauses = append(clauses, "e.project_id = ?")
		args = append(args, filter.ProjectID)
	}
	if filter.Query != "" {
		clauses = append(clauses, "LOWER(e.description) LIKE LOWER(?)")
		args = append(args, "%"+filter.Query+"%")
	}
	switch filter.Status {
	case "active":
		clauses = append(clauses, "e.stopped_at IS NULL")
	case "completed":
		clauses = append(clauses, "e.stopped_at IS NOT NULL")
	case "", "all":
	default:
		return nil, fmt.Errorf("invalid entry status %q", filter.Status)
	}
	if len(clauses) > 0 {
		query += " WHERE " + strings.Join(clauses, " AND ")
	}
	query += " ORDER BY e.started_at DESC, e.id DESC"

	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list entries: %w", err)
	}
	defer rows.Close()

	var results []EntryResult
	for rows.Next() {
		result, err := scanEntryResult(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate entries: %w", err)
	}
	return results, nil
}

// Report contains totals over a selected entry set.
type Report struct {
	Count             int
	CompletedDuration int64
}

// ReportEntries computes completed duration while sharing ListEntries' exact
// filter semantics.
func (s *Store) ReportEntries(ctx context.Context, filter EntryFilter) (Report, error) {
	entries, err := s.ListEntries(ctx, filter)
	if err != nil {
		return Report{}, err
	}
	var report Report
	report.Count = len(entries)
	for _, result := range entries {
		if result.Entry.StoppedAt != nil {
			report.CompletedDuration += int64(result.Entry.StoppedAt.Sub(result.Entry.StartedAt).Seconds())
		}
	}
	return report, nil
}

// Backup creates a consistent SQLite snapshot using SQLite's VACUUM INTO.
// The destination must not already exist, matching SQLite's behavior.
func (s *Store) Backup(ctx context.Context, path string) error {
	if _, err := s.DB.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return fmt.Errorf("backup database: %w", err)
	}
	return nil
}

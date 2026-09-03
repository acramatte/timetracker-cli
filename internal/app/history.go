package app

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/acramatte/timetracker-cli/internal/store"
)

// EntryFilters are user-facing filters. Local calendar dates are interpreted
// in the persisted timezone before conversion to UTC repository bounds.
type EntryFilters struct {
	From      string
	To        string
	ProjectID string
	Query     string
	Status    string
}

// Resolve converts inclusive YYYY-MM-DD dates to UTC bounds. The upper bound
// is exclusive at the start of the day after To (spec §10, task D2).
func Resolve(ctx context.Context, s *store.Store, filters EntryFilters) (store.EntryFilter, error) {
	loc, err := s.LocalTimezone(ctx)
	if err != nil {
		return store.EntryFilter{}, err
	}
	result := store.EntryFilter{
		ProjectID: filters.ProjectID,
		Query:     filters.Query,
		Status:    filters.Status,
	}
	if filters.From != "" {
		from, err := time.ParseInLocation("2006-01-02", filters.From, loc)
		if err != nil {
			return store.EntryFilter{}, fmt.Errorf("%w: invalid --from date %q", ErrValidation, filters.From)
		}
		from = from.UTC()
		result.From = &from
	}
	if filters.To != "" {
		to, err := time.ParseInLocation("2006-01-02", filters.To, loc)
		if err != nil {
			return store.EntryFilter{}, fmt.Errorf("%w: invalid --to date %q", ErrValidation, filters.To)
		}
		to = to.AddDate(0, 0, 1).UTC()
		result.To = &to
	}
	if result.From != nil && result.To != nil && !result.From.Before(*result.To) {
		return store.EntryFilter{}, fmt.Errorf("%w: --from must not be after --to", ErrValidation)
	}
	return result, nil
}

// List returns the entries selected by one shared filter definition.
func List(ctx context.Context, s *store.Store, filters EntryFilters) ([]store.EntryResult, error) {
	resolved, err := Resolve(ctx, s, filters)
	if err != nil {
		return nil, err
	}
	return s.ListEntries(ctx, resolved)
}

// Report returns totals selected through the same repository filter path.
func Report(ctx context.Context, s *store.Store, filters EntryFilters) (store.Report, error) {
	resolved, err := Resolve(ctx, s, filters)
	if err != nil {
		return store.Report{}, err
	}
	return s.ReportEntries(ctx, resolved)
}

// ExportCSV writes the canonical CSV columns from the shared list result.
func ExportCSV(ctx context.Context, s *store.Store, filters EntryFilters, w io.Writer) error {
	entries, err := List(ctx, s, filters)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(w)
	if err := writer.Write([]string{"id", "description", "project", "start_time", "end_time", "duration_seconds", "duration_formatted", "pomodoro"}); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, result := range entries {
		e := result.Entry
		end, duration, durationFormatted := "", "", ""
		if e.StoppedAt != nil {
			seconds := int64(e.StoppedAt.Sub(e.StartedAt).Seconds())
			end = e.StoppedAt.UTC().Format(time.RFC3339)
			duration = strconv.FormatInt(seconds, 10)
			durationFormatted = formatDuration(seconds)
		}
		if err := writer.Write([]string{
			e.ID, e.Description, result.ProjectName,
			e.StartedAt.UTC().Format(time.RFC3339), end, duration,
			durationFormatted, strconv.FormatBool(e.Pomodoro),
		}); err != nil {
			return fmt.Errorf("write csv entry: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("flush csv: %w", err)
	}
	return nil
}

func formatDuration(seconds int64) string {
	minutes, remainder := seconds/60, seconds%60
	hours, minutes := minutes/60, minutes%60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, remainder)
}

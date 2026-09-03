package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"path/filepath"
	"testing"
	"time"

	"github.com/acramatte/timetracker-cli/internal/store"
	"github.com/stretchr/testify/require"
)

func TestHistoryFiltersAndReportShareSelection(t *testing.T) {
	_, _, projects, entries := newTestApp(t)
	ctx := context.Background()
	project, err := projects.Add(ctx, AddOptions{Name: "project"})
	require.NoError(t, err)

	addCompleted := func(id, description string, start time.Time, duration time.Duration) {
		projectID := project.ID
		_, err := entries.Store.StartEntry(ctx, store.TimeEntry{
			ID: id, Description: description, StartedAt: start, ProjectID: &projectID,
			CreatedAt: start, UpdatedAt: start,
		})
		require.NoError(t, err)
		_, err = entries.Store.StopEntry(ctx, id, start.Add(duration))
		require.NoError(t, err)
	}
	addCompleted("e1", "deep work", time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC), time.Hour)
	addCompleted("e2", "planning", time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC), 30*time.Minute)

	active, err := entries.Store.StartEntry(ctx, store.TimeEntry{
		ID: "e3", Description: "deep active", StartedAt: time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	require.NoError(t, err)
	require.Nil(t, active.StoppedAt)

	filters := EntryFilters{From: "2026-09-01", To: "2026-09-02", Query: "DEEP"}
	listed, err := List(ctx, entries.Store, filters)
	require.NoError(t, err)
	require.Len(t, listed, 2)

	report, err := Report(ctx, entries.Store, filters)
	require.NoError(t, err)
	require.Equal(t, 2, report.Count)
	require.Equal(t, int64(3600), report.CompletedDuration)
}

func TestHistoryDateBoundsUseConfiguredTimezone(t *testing.T) {
	_, _, _, entries := newTestApp(t)
	ctx := context.Background()
	require.NoError(t, entries.Store.SetSetting(ctx, store.SettingTimezone, "Europe/Zurich"))

	add := func(id string, start time.Time) {
		_, err := entries.Store.StartEntry(ctx, store.TimeEntry{
			ID: id, Description: id, StartedAt: start.UTC(),
			CreatedAt: start.UTC(), UpdatedAt: start.UTC(),
		})
		require.NoError(t, err)
		_, err = entries.Store.StopEntry(ctx, id, start.UTC().Add(time.Minute))
		require.NoError(t, err)
	}
	zurich := func(hour int) time.Time {
		loc, err := time.LoadLocation("Europe/Zurich")
		require.NoError(t, err)
		return time.Date(2026, 3, 29, hour, 0, 0, 0, loc)
	}
	add("before", zurich(0).Add(-time.Minute))
	add("inside", zurich(3))
	add("after", zurich(4).Add(24*time.Hour))

	listed, err := List(ctx, entries.Store, EntryFilters{From: "2026-03-29", To: "2026-03-29"})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, "inside", listed[0].Entry.ID)
}

func TestExportCSVParsesSpecialCharacters(t *testing.T) {
	_, _, _, entries := newTestApp(t)
	ctx := context.Background()
	start := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	_, err := entries.Store.StartEntry(ctx, store.TimeEntry{
		ID: "e1", Description: "quoted, line\nbreak", StartedAt: start,
		CreatedAt: start, UpdatedAt: start,
	})
	require.NoError(t, err)
	_, err = entries.Store.StopEntry(ctx, "e1", start.Add(90*time.Second))
	require.NoError(t, err)

	var output bytes.Buffer
	require.NoError(t, ExportCSV(ctx, entries.Store, EntryFilters{}, &output))
	rows, err := csv.NewReader(&output).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "quoted, line\nbreak", rows[1][1])
	require.Equal(t, "00:01:30", rows[1][6])
}

func TestBackupIsIndependentlyReadable(t *testing.T) {
	_, _, _, entries := newTestApp(t)
	ctx := context.Background()
	start := time.Now().UTC().Truncate(time.Second)
	_, err := entries.Store.StartEntry(ctx, store.TimeEntry{
		ID: "e1", Description: "backup", StartedAt: start,
		CreatedAt: start, UpdatedAt: start,
	})
	require.NoError(t, err)
	_, err = entries.Store.StopEntry(ctx, "e1", start.Add(time.Minute))
	require.NoError(t, err)

	backupPath := filepath.Join(t.TempDir(), "backup.db")
	require.NoError(t, entries.Store.Backup(ctx, backupPath))

	db, err := store.Open(ctx, backupPath)
	require.NoError(t, err)
	defer db.Close()
	backedUp, err := (&store.Store{DB: db}).GetEntry(ctx, "e1")
	require.NoError(t, err)
	require.Equal(t, "backup", backedUp.Description)
}

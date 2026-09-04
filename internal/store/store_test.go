package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "timetracker.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return &Store{DB: db}
}

func entry(id, desc string, start time.Time) TimeEntry {
	return TimeEntry{
		ID:          id,
		Description: desc,
		StartedAt:   start,
		CreatedAt:   start,
		UpdatedAt:   start,
	}
}

func TestCreateAndListProjects(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	color := "#2563eb"
	created, err := s.CreateProject(ctx, Project{
		ID: "p1", Name: "timetracker", Color: &color, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	require.NoError(t, err)
	assert.Equal(t, "timetracker", created.Name)

	// Archived project is hidden by default.
	_, err = s.CreateProject(ctx, Project{
		ID: "p2", Name: "archived-proj", Archived: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	require.NoError(t, err)

	active, err := s.ListProjects(ctx, false)
	require.NoError(t, err)
	require.Len(t, active, 1)
	assert.Equal(t, "timetracker", active[0].Name)

	all, err := s.ListProjects(ctx, true)
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestArchiveProjectKeepsHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := s.CreateProject(ctx, Project{ID: "p1", Name: "proj", CreatedAt: now, UpdatedAt: now})
	require.NoError(t, err)
	pid := "p1"
	start := now.Add(-time.Hour)
	_, err = s.StartEntry(ctx, TimeEntry{
		ID: "e1", Description: "work", StartedAt: start, ProjectID: &pid,
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	_, err = s.StopEntry(ctx, "e1", now)
	require.NoError(t, err)

	archiveAt := now.Add(time.Hour).Truncate(time.Second)
	archived, err := s.ArchiveProject(ctx, "p1", archiveAt)
	require.NoError(t, err)
	assert.True(t, archived.Archived)
	assert.Equal(t, archiveAt, archived.UpdatedAt)

	all, err := s.ListProjects(ctx, true)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.True(t, all[0].Archived, "project archived")

	// Historical entry keeps its project identity (spec §9.5).
	var projectID *string
	err = s.DB.QueryRowContext(ctx,
		`SELECT project_id FROM time_entries WHERE id = 'e1'`).Scan(&projectID)
	require.NoError(t, err)
	require.NotNil(t, projectID)
	assert.Equal(t, "p1", *projectID)
}

func TestArchiveUnknownProjectIsNotFound(t *testing.T) {
	s := testStore(t)
	_, err := s.ArchiveProject(context.Background(), "nope", time.Now())
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestStartAndActiveEntry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(-time.Minute)

	created, err := s.StartEntry(ctx, entry("e1", "deep work", start))
	require.NoError(t, err)
	assert.Nil(t, created.StoppedAt)

	active, err := s.ActiveEntry(ctx)
	require.NoError(t, err)
	assert.Equal(t, "e1", active.ID)
	assert.Equal(t, "deep work", active.Description)
	assert.False(t, active.StartedAt.IsZero())
}

func TestSecondStartIsConflict(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := s.StartEntry(ctx, entry("e1", "first", now))
	require.NoError(t, err)
	_, err = s.StartEntry(ctx, entry("e2", "second", now))
	require.ErrorIs(t, err, ErrActiveEntryExists, "spec §9.1: exactly one active entry")
}

func TestStopEntryAndNoActive(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := s.StartEntry(ctx, entry("e1", "work", now.Add(-time.Hour)))
	require.NoError(t, err)

	stopped, err := s.StopEntry(ctx, "", now)
	require.NoError(t, err)
	require.NotNil(t, stopped.StoppedAt)
	assert.True(t, stopped.StoppedAt.Equal(now.Truncate(time.Second)) ||
		stopped.StoppedAt.After(stopped.StartedAt))

	_, err = s.ActiveEntry(ctx)
	require.ErrorIs(t, err, ErrNoActiveEntry)

	// Stopping again with no active entry fails clearly (spec §9.7).
	_, err = s.StopEntry(ctx, "", now)
	require.ErrorIs(t, err, ErrNoActiveEntry)
}

func TestStopByNameUnknown(t *testing.T) {
	s := testStore(t)
	_, err := s.StopEntry(context.Background(), "missing", time.Now())
	require.ErrorIs(t, err, ErrNotFound)
}

func TestStopNamedEntryWhileAnotherActive(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	_, err := s.StartEntry(ctx, entry("e1", "older", now.Add(-2*time.Hour)))
	require.NoError(t, err)
	// Can't have two active; close e1 first, then start e2 and stop by ID.
	_, err = s.StopEntry(ctx, "", now.Add(-time.Hour))
	require.NoError(t, err)
	_, err = s.StartEntry(ctx, entry("e2", "current", now.Add(-time.Minute)))
	require.NoError(t, err)

	stopped, err := s.StopEntry(ctx, "e2", now)
	require.NoError(t, err)
	assert.Equal(t, "e2", stopped.ID)
}

func TestPomodoroFieldsRoundTrip(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	start := time.Now().UTC().Add(-5 * time.Minute)
	ends := start.Add(30 * time.Minute)
	dur := int64(1800)

	_, err := s.StartEntry(ctx, TimeEntry{
		ID: "e1", Description: "pomodoro", StartedAt: start,
		Pomodoro: true, PomodoroDurationSeconds: &dur, PomodoroEndsAt: &ends,
		CreatedAt: start, UpdatedAt: start,
	})
	require.NoError(t, err)

	active, err := s.ActiveEntry(ctx)
	require.NoError(t, err)
	assert.True(t, active.Pomodoro)
	require.NotNil(t, active.PomodoroDurationSeconds)
	assert.Equal(t, int64(1800), *active.PomodoroDurationSeconds)
	require.NotNil(t, active.PomodoroEndsAt)
	assert.True(t, active.PomodoroEndsAt.Equal(ends.Truncate(time.Second)))
}

func TestReplaceEntryRollsBackStopWhenInsertFails(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := s.StartEntry(ctx, entry("e1", "protected", now.Add(-time.Hour)))
	require.NoError(t, err)
	_, err = s.DB.ExecContext(ctx, `
		CREATE TRIGGER fail_replacement
		BEFORE INSERT ON time_entries
		WHEN NEW.id = 'e2'
		BEGIN
			SELECT RAISE(FAIL, 'forced replacement failure');
		END`)
	require.NoError(t, err)

	_, _, err = s.ReplaceEntry(ctx, entry("e2", "replacement", now), now)
	require.Error(t, err)

	active, err := s.ActiveEntry(ctx)
	require.NoError(t, err)
	assert.Equal(t, "e1", active.ID)
	assert.Nil(t, active.StoppedAt, "failed replacement must roll the stop back")
}

func TestListEntriesMapsEveryEntryField(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(4 * time.Hour)
	startedAt := createdAt.Add(time.Hour)
	stoppedAt := startedAt.Add(2 * time.Hour)
	pomodoroEndsAt := startedAt.Add(25 * time.Minute)
	duration := int64(1500)
	projectID := "p-map"

	_, err := s.CreateProject(ctx, Project{
		ID: projectID, Name: "mapped project", CreatedAt: createdAt, UpdatedAt: createdAt,
	})
	require.NoError(t, err)
	_, err = s.StartEntry(ctx, TimeEntry{
		ID: "e-map", Description: "mapped entry", StartedAt: startedAt,
		StoppedAt: &stoppedAt, ProjectID: &projectID, Pomodoro: true,
		PomodoroDurationSeconds: &duration, PomodoroEndsAt: &pomodoroEndsAt,
		CreatedAt: createdAt, UpdatedAt: updatedAt,
	})
	require.NoError(t, err)

	results, err := s.ListEntries(ctx, EntryFilter{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	result := results[0]
	assert.Equal(t, "mapped project", result.ProjectName)
	assert.Equal(t, "e-map", result.Entry.ID)
	assert.Equal(t, "mapped entry", result.Entry.Description)
	assert.Equal(t, startedAt, result.Entry.StartedAt)
	require.NotNil(t, result.Entry.StoppedAt)
	assert.Equal(t, stoppedAt, *result.Entry.StoppedAt)
	require.NotNil(t, result.Entry.ProjectID)
	assert.Equal(t, projectID, *result.Entry.ProjectID)
	assert.True(t, result.Entry.Pomodoro)
	require.NotNil(t, result.Entry.PomodoroDurationSeconds)
	assert.Equal(t, duration, *result.Entry.PomodoroDurationSeconds)
	require.NotNil(t, result.Entry.PomodoroEndsAt)
	assert.Equal(t, pomodoroEndsAt, *result.Entry.PomodoroEndsAt)
	assert.Equal(t, createdAt, result.Entry.CreatedAt)
	assert.Equal(t, updatedAt, result.Entry.UpdatedAt)
}

func TestDescriptionConstraint(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	_, err := s.StartEntry(ctx, entry("e1", "   ", time.Now()))
	require.Error(t, err, "whitespace-only description violates CHECK")
}

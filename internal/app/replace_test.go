package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/acramatte/timetracker-cli/internal/store"
)

// TestReplaceValidatesBeforeMutating proves a rejected replacement leaves
// the previous entry active (spec §9.6): validation must run before the
// stop mutation, not after it.
func TestReplaceValidatesBeforeMutating(t *testing.T) {
	tk, _, pr, _ := newTestApp(t)
	ctx := context.Background()

	old, err := tk.Start(ctx, StartOptions{Description: "protected task"})
	require.NoError(t, err)

	// Case 1: blank description is rejected.
	_, _, err = tk.Replace(ctx, StartOptions{Description: "   "})
	assert.ErrorIs(t, err, ErrValidation)
	active, err := tk.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, active, "old entry must remain active after rejected replace")
	assert.Equal(t, old.ID, active.ID)

	// Case 2: unknown project reference is rejected.
	_, _, err = tk.Replace(ctx, StartOptions{Description: "new task", ProjectID: "p-ghost"})
	assert.ErrorIs(t, err, store.ErrNotFound)
	active, err = tk.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, active, "old entry must remain active after rejected replace")
	assert.Equal(t, old.ID, active.ID)

	// Sanity: a valid replace still works afterwards.
	proj, err := pr.Add(ctx, AddOptions{Name: "demo"})
	require.NoError(t, err)
	stopped, started, err := tk.Replace(ctx, StartOptions{Description: "new task", ProjectID: proj.ID})
	require.NoError(t, err)
	assert.Equal(t, old.ID, stopped.ID)
	require.NotNil(t, stopped.StoppedAt)
	assert.Equal(t, "new task", started.Description)
	assert.Equal(t, proj.ID, *started.ProjectID)
}

// TestStopAtBeforeStartTime rejects a stop instant earlier than the entry
// started: validation error, not a SQLite CHECK storage error.
func TestStopAtBeforeStartTime(t *testing.T) {
	tk, _, _, _ := newTestApp(t)
	ctx := context.Background()

	e, err := tk.Start(ctx, StartOptions{Description: "task"})
	require.NoError(t, err)

	beforeStart := e.StartedAt.Add(-time.Hour)
	_, err = tk.Stop(ctx, "", &beforeStart)
	assert.ErrorIs(t, err, ErrValidation, "backdated stop before start must be a validation error")

	active, err := tk.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, active, "rejected stop must leave the entry active")
	assert.Equal(t, e.ID, active.ID)
}

// TestStopNamedEntryAtExplicitInstant allows an explicit --at that is after
// the start time (the legitimate --at correction path).
func TestStopNamedEntryAtExplicitInstant(t *testing.T) {
	tk, _, _, _ := newTestApp(t)
	ctx := context.Background()

	e, err := tk.Start(ctx, StartOptions{Description: "task"})
	require.NoError(t, err)

	at := e.StartedAt.Add(10 * time.Minute)
	stopped, err := tk.Stop(ctx, "", &at)
	require.NoError(t, err)
	require.NotNil(t, stopped.StoppedAt)
	assert.True(t, stopped.StoppedAt.Equal(at.Truncate(time.Second)))
}

// newTestAppWithEntries builds an app plus a completed fixture entry.
func newTestAppWithEntries(t *testing.T) (*TrackingService, *EntriesService, *ProjectsService, context.Context) {
	tk, _, pr, en := newTestApp(t)
	return tk, en, pr, context.Background()
}

// TestEditRejectedRangeLeavesEntryUntouched verifies Edit re-validation
// failure does not mutate the stored entry.
func TestEditRejectedRangeLeavesEntryUntouched(t *testing.T) {
	_, en, pr, ctx := newTestAppWithEntries(t)

	proj, err := pr.Add(ctx, AddOptions{Name: "p"})
	require.NoError(t, err)

	start := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	stop := start.Add(time.Hour)
	added, err := en.Add(ctx, AddEntryOptions{Description: "original", Start: start, Stop: stop, ProjectID: proj.ID})
	require.NoError(t, err)

	// New stop before new start: rejected, nothing changes.
	badStop := added.StartedAt.Add(-time.Hour)
	_, err = en.Edit(ctx, added.ID, EditOptions{Stop: &badStop})
	assert.ErrorIs(t, err, ErrValidation)

	current, err := en.Store.GetEntry(ctx, added.ID)
	require.NoError(t, err)
	require.NotNil(t, current.StoppedAt)
	assert.True(t, current.StoppedAt.Equal(stop), "entry must be untouched after rejected edit")
	assert.Equal(t, "original", current.Description)
}

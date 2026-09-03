package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/acramatte/timetracker-cli/internal/store"
)

// newTestApp builds a full app over an isolated temporary database.
func newTestApp(t *testing.T) (*TrackingService, *PomodoroService, *ProjectsService, *EntriesService) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "timetracker.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	s := &store.Store{DB: db}
	require.NoError(t, s.Initialise(context.Background(), "Etc/UTC"))
	tracking := &TrackingService{Store: s}
	pom := &PomodoroService{Store: s, Notifier: NoopNotifier{}}
	projects := &ProjectsService{Store: s}
	entries := &EntriesService{Store: s}
	return tracking, pom, projects, entries
}

// ---- Tracking: start/status/stop (C2-C4) ----

func TestTrackingLifecycle(t *testing.T) {
	tk, _, _, _ := newTestApp(t)
	ctx := context.Background()

	// No active entry initially.
	active, err := tk.Status(ctx)
	require.NoError(t, err)
	assert.Nil(t, active)

	e, err := tk.Start(ctx, StartOptions{Description: "  deep work  "})
	require.NoError(t, err)
	assert.Equal(t, "deep work", e.Description, "description is normalised")
	assert.Nil(t, e.StoppedAt)

	active, err = tk.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, e.ID, active.ID)

	stopped, err := tk.Stop(ctx, "", nil)
	require.NoError(t, err)
	assert.Equal(t, e.ID, stopped.ID)
	require.NotNil(t, stopped.StoppedAt)

	// Stop again with nothing active fails cleanly (AT-22).
	_, err = tk.Stop(ctx, "", nil)
	require.ErrorIs(t, err, store.ErrNoActiveEntry)
}

func TestTrackingConflict(t *testing.T) {
	tk, _, _, _ := newTestApp(t)
	ctx := context.Background()

	_, err := tk.Start(ctx, StartOptions{Description: "first"})
	require.NoError(t, err)
	_, err = tk.Start(ctx, StartOptions{Description: "second"})
	assert.ErrorIs(t, err, store.ErrActiveEntryExists, "AT-19")
}

func TestReplaceClosesOldAndStartsNew(t *testing.T) {
	tk, _, _, _ := newTestApp(t)
	ctx := context.Background()

	old, err := tk.Start(ctx, StartOptions{Description: "old task"})
	require.NoError(t, err)

	newEntry, started, err := tk.Replace(ctx, StartOptions{Description: "new task"})
	require.NoError(t, err)
	assert.Equal(t, old.ID, newEntry.ID)
	assert.NotNil(t, newEntry.StoppedAt)
	assert.Equal(t, "new task", started.Description)

	active, err := tk.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, active)
	assert.Equal(t, started.ID, active.ID)
}

func TestReplaceWithoutActiveFails(t *testing.T) {
	tk, _, _, _ := newTestApp(t)
	_, _, err := tk.Replace(context.Background(), StartOptions{Description: "x"})
	assert.ErrorIs(t, err, store.ErrNoActiveEntry)
}

// ---- Projects (C1) ----

func TestProjectsLifecycle(t *testing.T) {
	tk, _, pr, _ := newTestApp(t)
	ctx := context.Background()

	proj, err := pr.Add(ctx, AddOptions{Name: "demo", Color: "#2563eb"})
	require.NoError(t, err)
	assert.NotEmpty(t, proj.ID)

	// Blank name rejected.
	_, err = pr.Add(ctx, AddOptions{Name: "   "})
	assert.ErrorIs(t, err, ErrValidation)

	// Start an entry against the project, then archive it.
	_, err = tk.Start(ctx, StartOptions{Description: "work", ProjectID: proj.ID})
	require.NoError(t, err)
	_, err = tk.Stop(ctx, "", nil)
	require.NoError(t, err)
	require.NoError(t, pr.Archive(ctx, proj.ID))

	active, err := pr.List(ctx, false)
	require.NoError(t, err)
	assert.Empty(t, active, "archived hidden by default (AT-14)")

	all, err := pr.List(ctx, true)
	require.NoError(t, err)
	assert.Len(t, all, 1)

	// Unknown project cannot be referenced (AT-16).
	_, err = tk.Start(ctx, StartOptions{Description: "work", ProjectID: "missing"})
	assert.ErrorIs(t, err, store.ErrNotFound)
}

// ---- Pomodoro (C6-C8) ----

func TestPomodoroDefaultThirtyMinutes(t *testing.T) {
	_, pom, _, _ := newTestApp(t)
	ctx := context.Background()

	e, err := pom.Start(ctx, PomodoroStartOptions{Description: "focus"})
	require.NoError(t, err)
	require.NotNil(t, e.PomodoroDurationSeconds)
	assert.Equal(t, int64(1800), *e.PomodoroDurationSeconds, "AT-43: 30-minute default")
	require.NotNil(t, e.PomodoroEndsAt)
	assert.Equal(t, 30*time.Minute, e.PomodoroEndsAt.Sub(e.StartedAt))
}

func TestPomodoroOverride(t *testing.T) {
	_, pom, _, _ := newTestApp(t)
	ctx := context.Background()

	e, err := pom.Start(ctx, PomodoroStartOptions{Description: "sprint", Minutes: 25})
	require.NoError(t, err)
	assert.Equal(t, int64(1500), *e.PomodoroDurationSeconds, "AT-44")
}

func TestPomodoroReconcileOverdue(t *testing.T) {
	_, pom, _, _ := newTestApp(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	pom.Now = func() time.Time { return base }

	e, err := pom.Start(ctx, PomodoroStartOptions{Description: "abandoned"})
	require.NoError(t, err)

	// Move the clock past the deadline.
	later := base.Add(2 * time.Hour)
	pom.Now = func() time.Time { return later }

	reconciled, err := pom.ReconcileOverdue(ctx)
	require.NoError(t, err)
	require.NotNil(t, reconciled, "AT-48: overdue pomodoro is completed")
	assert.Equal(t, e.ID, reconciled.ID)
	assert.True(t, reconciled.StoppedAt.Equal(*e.PomodoroEndsAt),
		"recovery completes at the scheduled end, not the recovery instant")
}

func TestReconcileNoopWhenNothingOverdue(t *testing.T) {
	_, pom, _, _ := newTestApp(t)
	ctx := context.Background()

	_, err := pom.ReconcileOverdue(ctx)
	require.NoError(t, err, "no active entry: no-op")

	// Active non-pomodoro entry: also a no-op.
	tk := &TrackingService{Store: pom.Store, Now: pom.Now}
	_, err = tk.Start(ctx, StartOptions{Description: "ordinary"})
	require.NoError(t, err)
	_, err = pom.ReconcileOverdue(ctx)
	require.NoError(t, err)
}

// notifierFunc adapts a plain function to the Notifier interface.
type notifierFunc func(ctx context.Context, title, msg string) error

func (f notifierFunc) Notify(ctx context.Context, title, msg string) error {
	return f(ctx, title, msg)
}

func TestPomodoroRunDeadlineWithFakeNotifier(t *testing.T) {
	_, pom, _, _ := newTestApp(t)
	ctx := context.Background()

	var notified int
	pom.Notifier = notifierFunc(func(ctx context.Context, title, msg string) error {
		notified++
		return nil
	})

	// Pre-create an entry whose deadline is 1s in the past, so the runner
	// completes immediately instead of sleeping for a real minute.
	start := time.Now().UTC().Add(-2 * time.Minute)
	ends := start.Add(time.Minute)
	dur := int64(60)
	e, err := pom.Store.StartEntry(ctx, store.TimeEntry{
		ID: "e-test", Description: "deadline test", StartedAt: start,
		Pomodoro: true, PomodoroDurationSeconds: &dur, PomodoroEndsAt: &ends,
		CreatedAt: start, UpdatedAt: start,
	})
	require.NoError(t, err)

	stopped, err := pom.RunDeadline(ctx, e, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, notified, "AT-45: exactly one notification attempt")
	require.NotNil(t, stopped.StoppedAt)
	// Both values round-trip through RFC3339 (second precision).
	assert.True(t, stopped.StoppedAt.Truncate(time.Second).Equal(ends.Truncate(time.Second)))
}

func TestPomodoroRunDeadlineReportsProgress(t *testing.T) {
	_, pom, _, _ := newTestApp(t)
	ctx := context.Background()

	updates := []time.Duration(nil)
	progress := func(remaining time.Duration) {
		updates = append(updates, remaining)
	}

	start := time.Now().UTC()
	ends := start.Add(1100 * time.Millisecond)
	dur := int64(1)
	e, err := pom.Store.StartEntry(ctx, store.TimeEntry{
		ID: "e-progress", Description: "progress test", StartedAt: start,
		Pomodoro: true, PomodoroDurationSeconds: &dur, PomodoroEndsAt: &ends,
		CreatedAt: start, UpdatedAt: start,
	})
	require.NoError(t, err)

	stopped, err := pom.RunDeadline(ctx, e, progress)
	require.NoError(t, err)
	require.NotNil(t, stopped.StoppedAt)
	require.NotEmpty(t, updates, "foreground runner must report countdown progress")
	assert.Zero(t, updates[len(updates)-1], "runner reports zero at completion")
}

// failingNotifier always fails: completion must still succeed (AT-49).
type failingNotifier struct{}

func (failingNotifier) Notify(context.Context, string, string) error {
	return errors.New("no notification daemon")
}

func TestNotificationFailureIsNonFatal(t *testing.T) {
	_, pom, _, _ := newTestApp(t)
	ctx := context.Background()
	pom.Notifier = failingNotifier{}

	// Past-deadline entry: the runner completes immediately.
	start := time.Now().UTC().Add(-2 * time.Minute)
	ends := start.Add(time.Minute)
	dur := int64(60)
	e, err := pom.Store.StartEntry(ctx, store.TimeEntry{
		ID: "e-notify-fail", Description: "notify fails",
		StartedAt: start, Pomodoro: true,
		PomodoroDurationSeconds: &dur, PomodoroEndsAt: &ends,
		CreatedAt: start, UpdatedAt: start,
	})
	require.NoError(t, err)

	stopped, err := pom.RunDeadline(ctx, e, nil)
	require.NoError(t, err, "notification failure must not fail completion")
	require.NotNil(t, stopped.StoppedAt)
}

// ---- Manual entries and edits (C9-C10) ----

func TestManualEntryAdd(t *testing.T) {
	_, _, _, en := newTestApp(t)
	ctx := context.Background()
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	stop := start.Add(90 * time.Minute)

	e, err := en.Add(ctx, AddEntryOptions{
		Description: "backdated work", Start: start, Stop: stop, Pomodoro: true,
	})
	require.NoError(t, err)
	require.NotNil(t, e.StoppedAt, "AT-23: completed entry")
	assert.True(t, e.Pomodoro)
	assert.Equal(t, int64(5400), int64(e.StoppedAt.Sub(e.StartedAt).Seconds()))
}

func TestManualEntryRejectsInvertedRange(t *testing.T) {
	_, _, _, en := newTestApp(t)
	ctx := context.Background()
	start := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	stop := start.Add(-time.Hour)

	_, err := en.Add(ctx, AddEntryOptions{Description: "bad", Start: start, Stop: stop})
	assert.ErrorIs(t, err, ErrValidation, "AT-10")
}

func TestEditMetadataAndTiming(t *testing.T) {
	_, _, pr, en := newTestApp(t)
	ctx := context.Background()

	proj, err := pr.Add(ctx, AddOptions{Name: "target"})
	require.NoError(t, err)
	start := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	stop := start.Add(time.Hour)

	e, err := en.Add(ctx, AddEntryOptions{Description: "original", Start: start, Stop: stop})
	require.NoError(t, err)

	newStop := stop.Add(30 * time.Minute)
	edited, err := en.Edit(ctx, e.ID, EditOptions{
		Description: "corrected",
		ProjectID:   &proj.ID,
		Stop:        &newStop,
	})
	require.NoError(t, err)
	assert.Equal(t, e.ID, edited.ID, "AT-24: same ID")
	assert.Equal(t, "corrected", edited.Description)
	require.NotNil(t, edited.ProjectID)
	assert.Equal(t, proj.ID, *edited.ProjectID)
	require.NotNil(t, edited.StoppedAt)
	assert.True(t, edited.StoppedAt.Equal(newStop), "AT-25: corrected timing")
}

func TestEditUnknownEntry(t *testing.T) {
	_, _, _, en := newTestApp(t)
	_, err := en.Edit(context.Background(), "missing", EditOptions{Description: "x"})
	assert.ErrorIs(t, err, store.ErrNotFound, "AT-26")
}

// ---- JSON envelopes (C11) ----

func TestJSONEnvelopeShapes(t *testing.T) {
	tk, _, _, _ := newTestApp(t)
	ctx := context.Background()

	e, err := tk.Start(ctx, StartOptions{Description: "json test"})
	require.NoError(t, err)

	payload, err := MarshalEntryEnvelope(e)
	require.NoError(t, err)
	assert.Contains(t, payload, `"entry":`)
	assert.Contains(t, payload, `"id":`)
	assert.Contains(t, payload, `"stopped_at":null`)

	payload, err = MarshalActive(&e)
	require.NoError(t, err)
	assert.Contains(t, payload, `"active":`)

	payload, err = MarshalActive(nil)
	require.NoError(t, err)
	assert.Equal(t, `{"active":null}`, payload)
}

func TestMapErrorCategories(t *testing.T) {
	code, _ := MapError(ErrValidation)
	assert.Equal(t, ExitValidation, code)

	code, _ = MapError(store.ErrActiveEntryExists)
	assert.Equal(t, ExitConflict, code)

	code, _ = MapError(store.ErrNotFound)
	assert.Equal(t, ExitNotFound, code)

	code, _ = MapError(errors.New("mystery"))
	assert.Equal(t, ExitStorage, code)
}

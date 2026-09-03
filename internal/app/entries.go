package app

import (
	"context"
	"fmt"
	"time"

	"github.com/acramatte/timetracker-cli/internal/store"
)

// EntriesService implements manual-entry creation and edit (tasks C9–C10).
type EntriesService struct {
	Store *store.Store
	// Now is injectable for tests.
	Now func() time.Time
}

func (e *EntriesService) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// AddEntryOptions carries validated manual-entry inputs (task C9, spec §5.3).
type AddEntryOptions struct {
	Description string
	Start       time.Time
	Stop        time.Time
	ProjectID   string
	Pomodoro    bool
}

// Add creates a completed, manually dated entry.
func (e *EntriesService) Add(ctx context.Context, opts AddEntryOptions) (store.TimeEntry, error) {
	desc, err := normalizeDescription(opts.Description)
	if err != nil {
		return store.TimeEntry{}, err
	}
	if err := validateRange(opts.Start, opts.Stop); err != nil {
		return store.TimeEntry{}, err
	}
	projectID, err := resolveProject(ctx, e.Store, opts.ProjectID)
	if err != nil {
		return store.TimeEntry{}, err
	}

	now := e.now().UTC().Truncate(time.Second)
	id, err := newID()
	if err != nil {
		return store.TimeEntry{}, err
	}

	stop := opts.Stop.UTC()
	return e.Store.StartEntry(ctx, store.TimeEntry{
		ID:          id,
		Description: desc,
		StartedAt:   opts.Start.UTC(),
		StoppedAt:   &stop,
		ProjectID:   projectID,
		Pomodoro:    opts.Pomodoro,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

// EditOptions carries validated edit inputs (task C10). Zero-value fields
// are left unchanged; supplied ones are applied and re-validated.
type EditOptions struct {
	Description string
	ProjectID   *string // nil = leave; pointer to "" = clear
	Start       *time.Time
	Stop        *time.Time
}

// Edit corrects an existing entry's metadata or timing (§9.2, §9.4).
func (e *EntriesService) Edit(ctx context.Context, id string, opts EditOptions) (store.TimeEntry, error) {
	current, err := e.Store.GetEntry(ctx, id)
	if err != nil {
		return store.TimeEntry{}, err
	}

	desc := current.Description
	if opts.Description != "" {
		desc, err = normalizeDescription(opts.Description)
		if err != nil {
			return store.TimeEntry{}, err
		}
	}

	start := current.StartedAt
	if opts.Start != nil {
		start = opts.Start.UTC()
	}
	stop := current.StoppedAt
	if opts.Stop != nil {
		s := opts.Stop.UTC()
		stop = &s
	}
	if stop != nil {
		if err := validateRange(start, *stop); err != nil {
			return store.TimeEntry{}, err
		}
	}

	projectID := current.ProjectID
	if opts.ProjectID != nil {
		if *opts.ProjectID == "" {
			projectID = nil
		} else {
			projectID, err = resolveProject(ctx, e.Store, *opts.ProjectID)
			if err != nil {
				return store.TimeEntry{}, err
			}
		}
	}

	return e.Store.UpdateEntry(ctx, store.TimeEntry{
		ID:                      current.ID,
		Description:             desc,
		StartedAt:               start,
		StoppedAt:               stop,
		ProjectID:               projectID,
		Pomodoro:                current.Pomodoro,
		PomodoroDurationSeconds: current.PomodoroDurationSeconds,
		PomodoroEndsAt:          current.PomodoroEndsAt,
		CreatedAt:               current.CreatedAt,
		UpdatedAt:               e.now().UTC().Truncate(time.Second),
	})
}

var _ = fmt.Sprintf // keep fmt import if unused in future edits

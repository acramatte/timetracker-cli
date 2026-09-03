package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/acramatte/timetracker-cli/internal/store"
)

// TrackingService implements the start/status/stop/replace use cases
// (tasks C2–C5).
type TrackingService struct {
	Store *store.Store
	// Now is injectable for tests; defaults to time.Now.
	Now func() time.Time
}

func (t *TrackingService) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// Status returns the active entry, or nil when none is active.
func (t *TrackingService) Status(ctx context.Context) (*store.TimeEntry, error) {
	e, err := t.Store.ActiveEntry(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNoActiveEntry) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

// StartOptions carries validated start inputs.
type StartOptions struct {
	Description string
	ProjectID   string
}

// Start creates a new active entry. It fails with
// store.ErrActiveEntryExists when another entry is active (spec §9.6).
func (t *TrackingService) Start(ctx context.Context, opts StartOptions) (store.TimeEntry, error) {
	desc, err := normalizeDescription(opts.Description)
	if err != nil {
		return store.TimeEntry{}, err
	}
	projectID, err := resolveProject(ctx, t.Store, opts.ProjectID)
	if err != nil {
		return store.TimeEntry{}, err
	}

	now := t.now().UTC().Truncate(time.Second)
	id, err := newID()
	if err != nil {
		return store.TimeEntry{}, err
	}

	return t.Store.StartEntry(ctx, store.TimeEntry{
		ID:          id,
		Description: desc,
		StartedAt:   now,
		ProjectID:   projectID,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

// Stop closes the single active entry (or a named one) at the current
// time or an explicit --at instant (spec §9.7).
func (t *TrackingService) Stop(ctx context.Context, entryID string, at *time.Time) (store.TimeEntry, error) {
	stopAt := t.now().UTC().Truncate(time.Second)
	if at != nil {
		stopAt = at.UTC()
		if stopAt.Before(t.now().Add(-24 * time.Hour)) {
			// Guard against obviously wrong backdated stops; the spec
			// allows --at for corrections but not years in the past.
			return store.TimeEntry{}, fmt.Errorf("%w: --at is more than 24h in the past", ErrValidation)
		}
	}

	// Validate the resulting range before mutating so a bad --at surfaces
	// as a validation error (exit 2), not a storage CHECK violation (exit 5).
	if entryID != "" {
		e, err := t.Store.GetEntry(ctx, entryID)
		if err != nil {
			return store.TimeEntry{}, err
		}
		if err := validateRange(e.StartedAt, stopAt); err != nil {
			return store.TimeEntry{}, fmt.Errorf("%w: entry %s %v", ErrValidation, entryID, err)
		}
	} else if e, err := t.Store.ActiveEntry(ctx); err == nil {
		if err := validateRange(e.StartedAt, stopAt); err != nil {
			return store.TimeEntry{}, fmt.Errorf("%w: active entry %s %v", ErrValidation, e.ID, err)
		}
	}

	return t.Store.StopEntry(ctx, entryID, stopAt)
}

// Replace validates the new entry's inputs first, then closes the active
// entry and starts the new one. Validation runs before any mutation so a
// rejected replacement never leaves the previous entry stopped (spec §9.6).
func (t *TrackingService) Replace(ctx context.Context, opts StartOptions) (store.TimeEntry, store.TimeEntry, error) {
	// Pre-flight validation: description and project must be accepted for
	// the new entry before the old one is touched.
	desc, err := normalizeDescription(opts.Description)
	if err != nil {
		return store.TimeEntry{}, store.TimeEntry{}, err
	}
	projectID, err := resolveProject(ctx, t.Store, opts.ProjectID)
	if err != nil {
		return store.TimeEntry{}, store.TimeEntry{}, err
	}

	old, err := t.Store.ActiveEntry(ctx)
	if err != nil {
		return store.TimeEntry{}, store.TimeEntry{}, err // ErrNoActiveEntry propagates
	}

	now := t.now().UTC().Truncate(time.Second)
	stopped, err := t.Store.StopEntry(ctx, old.ID, now)
	if err != nil {
		return store.TimeEntry{}, store.TimeEntry{}, err
	}

	id, err := newID()
	if err != nil {
		return stopped, store.TimeEntry{}, err
	}
	started, err := t.Store.StartEntry(ctx, store.TimeEntry{
		ID:          id,
		Description: desc,
		StartedAt:   now,
		ProjectID:   projectID,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		return stopped, store.TimeEntry{}, err
	}
	return stopped, started, nil
}

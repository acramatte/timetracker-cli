package app

import (
	"context"
	"fmt"
	"time"

	"github.com/acramatte/timetracker-cli/internal/store"
)

// PomodoroService implements tasks C6–C8: durable Pomodoro start/stop,
// notification boundary, and overdue reconciliation.
type PomodoroService struct {
	Store *store.Store
	// Now is injectable for tests.
	Now func() time.Time
	// Notifier is attempted once at deadline; failure is non-fatal (§6.1).
	Notifier Notifier
	// Progress receives remaining time updates from the foreground runner.
	// It is nil for callers that do not need display updates (including tests).
	Progress func(remaining time.Duration)
	// DefaultMinutes is read from settings at first use if nil.
	DefaultMinutes int
}

// Notifier abstracts the desktop-notification attempt (spec §6.1). A
// command-line implementation calls an OS helper; a no-op is valid.
type Notifier interface {
	Notify(ctx context.Context, title, message string) error
}

// NOPNotifier is the terminal-fallback/no-op notifier.
type NoopNotifier struct{}

func (NoopNotifier) Notify(context.Context, string, string) error { return nil }

func (p *PomodoroService) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *PomodoroService) defaultMinutes(ctx context.Context) int {
	if p.DefaultMinutes > 0 {
		return p.DefaultMinutes
	}
	v, err := p.Store.GetSetting(ctx, store.SettingPomodoroDefaultMins)
	if err != nil || v == "" {
		return store.DefaultPomodoroMinutes
	}
	mins := store.DefaultPomodoroMinutes
	fmt.Sscanf(v, "%d", &mins)
	if mins <= 0 {
		return store.DefaultPomodoroMinutes
	}
	return mins
}

// StartOptions carries validated pomodoro inputs.
type PomodoroStartOptions struct {
	Description string
	ProjectID   string
	Minutes     int // 0 = use configured default (30)
}

// Start creates an active Pomodoro entry with a persisted scheduled end
// (spec §9.8: end = start + duration).
func (p *PomodoroService) Start(ctx context.Context, opts PomodoroStartOptions) (store.TimeEntry, error) {
	desc, err := normalizeDescription(opts.Description)
	if err != nil {
		return store.TimeEntry{}, err
	}
	minutes := opts.Minutes
	if minutes <= 0 {
		minutes = p.defaultMinutes(ctx)
	}
	projectID, err := resolveProject(ctx, p.Store, opts.ProjectID)
	if err != nil {
		return store.TimeEntry{}, err
	}

	now := p.now().UTC().Truncate(time.Second)
	ends := now.Add(time.Duration(minutes) * time.Minute)
	duration := durationSeconds(minutes)

	id, err := newID()
	if err != nil {
		return store.TimeEntry{}, err
	}

	return p.Store.StartEntry(ctx, store.TimeEntry{
		ID:                      id,
		Description:             desc,
		StartedAt:               now,
		ProjectID:               projectID,
		Pomodoro:                true,
		PomodoroDurationSeconds: &duration,
		PomodoroEndsAt:          &ends,
		CreatedAt:               now,
		UpdatedAt:               now,
	})
}

func durationSeconds(minutes int) int64 { return int64(minutes) * 60 }

// StopEarly finishes an active Pomodoro early (task C6) — same rules as
// ordinary stop, but the foreground runner suppresses the expiry alert.
func (p *PomodoroService) StopEarly(ctx context.Context, entryID string, at *time.Time) (store.TimeEntry, error) {
	t := &TrackingService{Store: p.Store, Now: p.Now}
	return t.Stop(ctx, entryID, at)
}

// ReconcileOverdue completes an expired Pomodoro at its stored scheduled
// end (spec §9.9: never at the recovery instant). Callers invoke this
// before active-entry operations (task C8).
func (p *PomodoroService) ReconcileOverdue(ctx context.Context) (*store.TimeEntry, error) {
	e, err := p.Store.ActiveEntry(ctx)
	if err != nil {
		return nil, nil // no active entry: nothing to reconcile
	}
	if !e.Pomodoro || e.PomodoroEndsAt == nil || !e.PomodoroEndsAt.Before(p.now()) {
		return nil, nil
	}

	stopped, err := p.Store.StopEntry(ctx, e.ID, *e.PomodoroEndsAt)
	if err != nil {
		return nil, err
	}
	return &stopped, nil
}

// RunDeadline fires the notification when a foreground runner observes the
// scheduled end (task C7). Completion is durable before notification.
func (p *PomodoroService) RunDeadline(ctx context.Context, e store.TimeEntry) (store.TimeEntry, error) {
	if e.PomodoroEndsAt == nil {
		return store.TimeEntry{}, fmt.Errorf("%w: entry %s has no scheduled end", ErrValidation, e.ID)
	}

	// Sleep until deadline (bounded by context cancellation), reporting a
	// countdown to the interactive command when requested.
	remaining := time.Until(*e.PomodoroEndsAt)
	if remaining > 0 {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		if p.Progress != nil {
			p.Progress(remaining)
		}
		for {
			select {
			case <-ctx.Done():
				return store.TimeEntry{}, ctx.Err()
			case <-ticker.C:
				remaining = time.Until(*e.PomodoroEndsAt)
				if remaining <= 0 {
					if p.Progress != nil {
						p.Progress(0)
					}
					goto deadline
				}
				if p.Progress != nil {
					p.Progress(remaining)
				}
			}
		}
	}

deadline:
	stopped, err := p.Store.StopEntry(ctx, e.ID, *e.PomodoroEndsAt)
	if err != nil {
		return store.TimeEntry{}, err
	}

	if p.Notifier != nil {
		// Best-effort (§6.1): failure is a warning, never fatal.
		_ = p.Notifier.Notify(ctx, "Pomodoro complete", e.Description)
	}
	return stopped, nil
}

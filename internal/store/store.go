package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Store is the handwritten SQLite repository (ADR 0002). It owns
// transaction boundaries, parameterised queries, row mapping, and error
// translation. Command handlers and future TUI code depend on this type,
// never on database/sql directly.
type Store struct {
	DB *sql.DB
}

// Close releases the underlying database handle.
func (s *Store) Close() error { return s.DB.Close() }

// Known error categories (spec §6.3): commands map these to exit behavior.
var (
	// ErrActiveEntryExists: another entry is already active.
	ErrActiveEntryExists = errors.New("an entry is already active")
	// ErrNoActiveEntry: stop/status found no active entry.
	ErrNoActiveEntry = errors.New("no active entry")
	// ErrNotFound: a named entry or project does not exist.
	ErrNotFound = errors.New("not found")
)

// Project is a local project (spec §7.1).
type Project struct {
	ID        string
	Name      string
	Color     *string
	Archived  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TimeEntry is a tracked entry (spec §7.2). Nil StoppedAt means active.
type TimeEntry struct {
	ID                      string
	Description             string
	StartedAt               time.Time
	StoppedAt               *time.Time
	ProjectID               *string
	Pomodoro                bool
	PomodoroDurationSeconds *int64
	PomodoroEndsAt          *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// CreateProject inserts a project; the caller supplies the generated ID.
func (s *Store) CreateProject(ctx context.Context, p Project) (Project, error) {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO projects (id, name, color, archived, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Color, boolToInt(p.Archived),
		p.CreatedAt.UTC().Format(time.RFC3339), p.UpdatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return p, nil
}

// ListProjects returns projects, optionally including archived ones.
func (s *Store) ListProjects(ctx context.Context, includeArchived bool) ([]Project, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, name, color, archived, created_at, updated_at
		FROM projects
		WHERE ? OR archived = 0
		ORDER BY name`, boolToInt(includeArchived))
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var out []Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ProjectExists reports whether a project with the given ID exists.
func (s *Store) ProjectExists(ctx context.Context, id string) (bool, error) {
	var one int
	err := s.DB.QueryRowContext(ctx,
		`SELECT 1 FROM projects WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check project %s: %w", id, err)
	}
	return true, nil
}

// ArchiveProject marks a project archived; historical entries keep their
// project identity (spec §9.5). Returns ErrNotFound for unknown IDs. The
// caller supplies updatedAt so tests can inject the clock.
func (s *Store) ArchiveProject(ctx context.Context, id string, updatedAt time.Time) error {
	res, err := s.DB.ExecContext(ctx,
		`UPDATE projects SET archived = 1, updated_at = ? WHERE id = ?`,
		updatedAt.UTC().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("archive project %s: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// StartEntry creates a new active entry. The single-active-entry invariant
// is enforced by the partial unique index; the race loser is translated to
// ErrActiveEntryExists (spec §9.1, §9.6).
func (s *Store) StartEntry(ctx context.Context, e TimeEntry) (TimeEntry, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return TimeEntry{}, fmt.Errorf("begin start transaction: %w", err)
	}
	defer tx.Rollback()

	stoppedAt := nullTimeString(e.StoppedAt)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO time_entries
			(id, description, started_at, stopped_at, project_id, pomodoro,
			 pomodoro_duration_seconds, pomodoro_ends_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Description, e.StartedAt.UTC().Format(time.RFC3339), stoppedAt,
		e.ProjectID, boolToInt(e.Pomodoro),
		e.PomodoroDurationSeconds, nullTimeString(e.PomodoroEndsAt),
		e.CreatedAt.UTC().Format(time.RFC3339), e.UpdatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		if strings.Contains(err.Error(), "one_active_time_entry") {
			return TimeEntry{}, ErrActiveEntryExists
		}
		return TimeEntry{}, fmt.Errorf("insert active entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return TimeEntry{}, fmt.Errorf("commit start: %w", err)
	}
	return e, nil
}

// ActiveEntry returns the single active entry or ErrNoActiveEntry.
func (s *Store) ActiveEntry(ctx context.Context) (TimeEntry, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, description, started_at, stopped_at, project_id, pomodoro,
		       pomodoro_duration_seconds, pomodoro_ends_at, created_at, updated_at
		FROM time_entries
		WHERE stopped_at IS NULL
		ORDER BY started_at DESC
		LIMIT 1`)
	e, err := scanEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return TimeEntry{}, ErrNoActiveEntry
	}
	if err != nil {
		return TimeEntry{}, err
	}
	return e, nil
}

// StopEntry closes an active entry at stopAt. With entryID == "" it acts on
// the single active entry (spec §9.7); otherwise on the named entry.
// Returns ErrNoActiveEntry / ErrNotFound respectively.
func (s *Store) StopEntry(ctx context.Context, entryID string, stopAt time.Time) (TimeEntry, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return TimeEntry{}, fmt.Errorf("begin stop transaction: %w", err)
	}
	defer tx.Rollback()

	var id string
	if entryID == "" {
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM time_entries WHERE stopped_at IS NULL ORDER BY started_at DESC LIMIT 1`).
			Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return TimeEntry{}, ErrNoActiveEntry
		}
		if err != nil {
			return TimeEntry{}, fmt.Errorf("find active entry: %w", err)
		}
	} else {
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM time_entries WHERE id = ?`, entryID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return TimeEntry{}, ErrNotFound
		}
		if err != nil {
			return TimeEntry{}, fmt.Errorf("find entry %s: %w", entryID, err)
		}
	}

	stopped := stopAt.UTC().Format(time.RFC3339)
	if _, err := tx.ExecContext(ctx, `
		UPDATE time_entries
		SET stopped_at = ?, updated_at = ?
		WHERE id = ? AND stopped_at IS NULL`,
		stopped, stopped, id); err != nil {
		return TimeEntry{}, fmt.Errorf("stop entry %s: %w", id, err)
	}

	e, err := scanEntry(tx.QueryRowContext(ctx, `
		SELECT id, description, started_at, stopped_at, project_id, pomodoro,
		       pomodoro_duration_seconds, pomodoro_ends_at, created_at, updated_at
		FROM time_entries WHERE id = ?`, id))
	if err != nil {
		return TimeEntry{}, fmt.Errorf("reload stopped entry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return TimeEntry{}, fmt.Errorf("commit stop: %w", err)
	}
	return e, nil
}

// GetEntry fetches any entry (active or completed) by ID.
func (s *Store) GetEntry(ctx context.Context, id string) (TimeEntry, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, description, started_at, stopped_at, project_id, pomodoro,
		       pomodoro_duration_seconds, pomodoro_ends_at, created_at, updated_at
		FROM time_entries WHERE id = ?`, id)
	e, err := scanEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return TimeEntry{}, ErrNotFound
	}
	if err != nil {
		return TimeEntry{}, err
	}
	return e, nil
}

// UpdateEntry rewrites an existing entry's fields (task C10 edit path).
// The ID must exist; updated_at is taken from the supplied entry.
func (s *Store) UpdateEntry(ctx context.Context, e TimeEntry) (TimeEntry, error) {
	res, err := s.DB.ExecContext(ctx, `
		UPDATE time_entries
		SET description = ?, started_at = ?, stopped_at = ?, project_id = ?,
		    pomodoro = ?, pomodoro_duration_seconds = ?, pomodoro_ends_at = ?,
		    updated_at = ?
		WHERE id = ?`,
		e.Description, e.StartedAt.UTC().Format(time.RFC3339), nullTimeString(e.StoppedAt),
		e.ProjectID, boolToInt(e.Pomodoro), e.PomodoroDurationSeconds,
		nullTimeString(e.PomodoroEndsAt), e.UpdatedAt.UTC().Format(time.RFC3339), e.ID)
	if err != nil {
		if strings.Contains(err.Error(), "one_active_time_entry") {
			return TimeEntry{}, ErrActiveEntryExists
		}
		return TimeEntry{}, fmt.Errorf("update entry %s: %w", e.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return TimeEntry{}, ErrNotFound
	}
	return e, nil
}

// scanner abstracts *sql.Row and *sql.Rows for shared row mapping.
type scanner interface {
	Scan(dest ...any) error
}

func scanProject(rows scanner) (Project, error) {
	var p Project
	var created, updated string
	var archived int
	err := rows.Scan(&p.ID, &p.Name, &p.Color, &archived, &created, &updated)
	if err != nil {
		return Project{}, fmt.Errorf("scan project: %w", err)
	}
	p.Archived = archived == 1
	p.CreatedAt, err = parseTime(created, err)
	p.UpdatedAt, err = parseTime(updated, err)
	return p, err
}

func scanEntry(rows scanner) (TimeEntry, error) {
	return scanEntryExtra(rows)
}

// scanEntryResult maps one ListEntries row: the 10 canonical entry columns
// via the shared mapping, plus the denormalised project name appended as an
// extra destination.
func scanEntryResult(rows scanner) (EntryResult, error) {
	var projectName string
	e, err := scanEntryExtra(rows, &projectName)
	if err != nil {
		return EntryResult{}, err
	}
	return EntryResult{Entry: e, ProjectName: projectName}, nil
}

// scanEntryExtra scans the 10 canonical entry columns and then any extra
// destinations appended by the caller (e.g. a joined project name).
func scanEntryExtra(rows scanner, extra ...any) (TimeEntry, error) {
	var e TimeEntry
	var started, created, updated string
	var stoppedAt, pomodoroEndsAt *string
	var pomodoro int
	dests := []any{&e.ID, &e.Description, &started, &stoppedAt, &e.ProjectID,
		&pomodoro, &e.PomodoroDurationSeconds, &pomodoroEndsAt,
		&created, &updated}
	dests = append(dests, extra...)
	err := rows.Scan(dests...)
	if err != nil {
		return TimeEntry{}, fmt.Errorf("scan entry: %w", err)
	}
	e.Pomodoro = pomodoro == 1
	e.StartedAt, err = parseTime(started, nil)
	if err != nil {
		return TimeEntry{}, err
	}
	e.CreatedAt, err = parseTime(created, nil)
	if err != nil {
		return TimeEntry{}, err
	}
	e.UpdatedAt, err = parseTime(updated, nil)
	if err != nil {
		return TimeEntry{}, err
	}
	if stoppedAt != nil {
		var stopped time.Time
		stopped, err = parseTime(*stoppedAt, nil)
		if err != nil {
			return TimeEntry{}, err
		}
		e.StoppedAt = &stopped
	}
	if pomodoroEndsAt != nil {
		var ends time.Time
		ends, err = parseTime(*pomodoroEndsAt, nil)
		if err != nil {
			return TimeEntry{}, err
		}
		e.PomodoroEndsAt = &ends
	}
	return e, nil
}

func parseTime(s string, err error) (time.Time, error) {
	if err != nil {
		return time.Time{}, err
	}
	t, perr := time.Parse(time.RFC3339, s)
	if perr != nil {
		return time.Time{}, fmt.Errorf("parse timestamp %q: %w", s, perr)
	}
	return t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullTimeString(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Setting keys persisted at initialisation (spec 001-local-first-cli §7.3).
const (
	SettingTimezone            = "timezone"
	SettingPomodoroDefaultMins = "pomodoro_default_minutes"
)

// DefaultPomodoroMinutes matches the Phoenix application's 30-minute Pomodoro.
const DefaultPomodoroMinutes = 30

// Initialise persists default settings on first use and is idempotent:
// existing values are never overwritten (spec task B5).
func (s *Store) Initialise(ctx context.Context, defaultTimezone string) error {
	if defaultTimezone == "" {
		defaultTimezone = "Etc/UTC"
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin settings transaction: %w", err)
	}
	defer tx.Rollback()

	// INSERT OR IGNORE: initialisation never clobbers user-changed values.
	stmt, err := tx.PrepareContext(ctx,
		`INSERT OR IGNORE INTO settings (key, value) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare settings insert: %w", err)
	}
	defer stmt.Close()

	for key, value := range map[string]string{
		SettingTimezone:            defaultTimezone,
		SettingPomodoroDefaultMins: fmt.Sprintf("%d", DefaultPomodoroMinutes),
	} {
		if _, err := stmt.ExecContext(ctx, key, value); err != nil {
			return fmt.Errorf("persist setting %s: %w", key, err)
		}
	}

	return tx.Commit()
}

// GetSetting returns a single setting value; sql.ErrNoRows when unset.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.DB.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

// SetSetting upserts one setting value.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("set setting %s: %w", key, err)
	}
	return nil
}

// LocalTimezone returns the configured timezone, falling back to UTC when
// the stored value is missing or not a valid IANA zone.
func (s *Store) LocalTimezone(ctx context.Context) (*time.Location, error) {
	name, err := s.GetSetting(ctx, SettingTimezone)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.UTC, nil
		}
		return nil, err
	}
	loc, loadErr := time.LoadLocation(name)
	if loadErr != nil {
		return time.UTC, nil
	}
	return loc, nil
}

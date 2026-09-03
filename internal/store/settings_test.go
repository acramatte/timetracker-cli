package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) (*Store, func()) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "timetracker.db"))
	require.NoError(t, err)
	return &Store{DB: db}, func() { db.Close() }
}

func TestInitialisePersistsDefaults(t *testing.T) {
	s, closeDB := openTestDB(t)
	defer closeDB()
	ctx := context.Background()

	require.NoError(t, s.Initialise(ctx, "Europe/Zurich"))

	tz, err := s.GetSetting(ctx, SettingTimezone)
	require.NoError(t, err)
	assert.Equal(t, "Europe/Zurich", tz)

	pom, err := s.GetSetting(ctx, SettingPomodoroDefaultMins)
	require.NoError(t, err)
	assert.Equal(t, "30", pom)
}

func TestInitialiseIsIdempotent(t *testing.T) {
	s, closeDB := openTestDB(t)
	defer closeDB()
	ctx := context.Background()

	require.NoError(t, s.Initialise(ctx, "Europe/Zurich"))
	require.NoError(t, s.SetSetting(ctx, SettingTimezone, "America/New_York"))
	// Second init must NOT clobber the user-changed value.
	require.NoError(t, s.Initialise(ctx, "Europe/Zurich"))

	tz, err := s.GetSetting(ctx, SettingTimezone)
	require.NoError(t, err)
	assert.Equal(t, "America/New_York", tz)
}

func TestInitialiseEmptyTimezoneFallsBackToUTC(t *testing.T) {
	s, closeDB := openTestDB(t)
	defer closeDB()
	ctx := context.Background()

	require.NoError(t, s.Initialise(ctx, ""))

	tz, err := s.GetSetting(ctx, SettingTimezone)
	require.NoError(t, err)
	assert.Equal(t, "Etc/UTC", tz)
}

func TestLocalTimezoneValidAndInvalid(t *testing.T) {
	s, closeDB := openTestDB(t)
	defer closeDB()
	ctx := context.Background()

	require.NoError(t, s.Initialise(ctx, "Europe/Zurich"))
	loc, err := s.LocalTimezone(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Europe/Zurich", loc.String())

	// Invalid stored zone falls back to UTC rather than failing commands.
	require.NoError(t, s.SetSetting(ctx, SettingTimezone, "Not/AZone"))
	loc, err = s.LocalTimezone(ctx)
	require.NoError(t, err)
	assert.Equal(t, time.UTC.String(), loc.String())
}

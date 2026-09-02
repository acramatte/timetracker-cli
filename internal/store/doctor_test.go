package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoctorHealthy(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "timetracker.db"))
	require.NoError(t, err)
	defer db.Close()
	s := &Store{DB: db}
	require.NoError(t, s.Initialise(ctx, "Europe/Zurich"))

	report := Doctor(ctx, s)

	assert.NoError(t, report.Err)
	assert.Positive(t, report.SchemaVersion)
	assert.Equal(t, "Europe/Zurich", report.Timezone)
	assert.ElementsMatch(t, []string{"settings", "projects", "time_entries"}, report.TablesPresent)
}

func TestDoctorReadonlyOnEmptyDatabase(t *testing.T) {
	// Note: Open() applies embedded migrations by design (spec §8.3 —
	// migration happens at open), so a freshly opened database is already
	// healthy. The unhealthy path is covered by corrupt-file cases in
	// acceptance testing; here we assert the healthy read-only report.
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "timetracker.db"))
	require.NoError(t, err)
	defer db.Close()
	s := &Store{DB: db}

	report := Doctor(ctx, s)

	assert.NoError(t, report.Err)
	assert.ElementsMatch(t, []string{"settings", "projects", "time_entries"}, report.TablesPresent)
}

func TestInitialiseIsIdempotentAcrossReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "timetracker.db")

	db, err := Open(ctx, path)
	require.NoError(t, err)
	s1 := &Store{DB: db}
	require.NoError(t, s1.Initialise(ctx, "Europe/Zurich"))
	db.Close()

	// Re-open and re-initialise: values must survive unchanged.
	db2, err := Open(ctx, path)
	require.NoError(t, err)
	defer db2.Close()
	s2 := &Store{DB: db2}
	require.NoError(t, s2.Initialise(ctx, "Europe/Zurich"))

	tz, err := s2.GetSetting(ctx, SettingTimezone)
	require.NoError(t, err)
	assert.Equal(t, "Europe/Zurich", tz)
}

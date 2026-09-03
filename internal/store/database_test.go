package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenMigratesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "timetracker.db")

	db, err := Open(ctx, dbPath)
	require.NoError(t, err)

	version, err := SchemaVersion(ctx, db)
	require.NoError(t, err)
	assert.Positive(t, version, "migrations must have been applied")
	db.Close()

	// Re-open: idempotent, no duplicate schema, no error.
	db2, err := Open(ctx, dbPath)
	require.NoError(t, err)
	defer db2.Close()

	version2, err := SchemaVersion(ctx, db2)
	require.NoError(t, err)
	assert.Equal(t, version, version2, "re-open must not re-apply or roll back migrations")
}

func TestOpenCreatesMissingDatabaseDirectory(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "nested", "data", "timetracker.db")

	db, err := Open(ctx, dbPath)
	require.NoError(t, err, "first-run open must create the platform data directory")
	defer db.Close()

	_, err = SchemaVersion(ctx, db)
	require.NoError(t, err)
}

func TestOpenSetsWALMode(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "timetracker.db"))
	require.NoError(t, err)
	defer db.Close()

	var mode string
	require.NoError(t, db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode))
	assert.Equal(t, "wal", mode, "spec §8.3 requires WAL mode")
}

func TestSchemaTablesExist(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "timetracker.db"))
	require.NoError(t, err)
	defer db.Close()

	for _, table := range []string{"settings", "projects", "time_entries"} {
		var name string
		err := db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		require.NoError(t, err, "table %s must exist", table)
	}
}

func TestOneActiveEntryInvariant(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "timetracker.db"))
	require.NoError(t, err)
	defer db.Close()

	// The invariant is enforced by the partial unique index on
	// stopped_at IS NULL; verify a second active insert fails.
	insert := `INSERT INTO time_entries
		(id, description, started_at, stopped_at, created_at, updated_at)
		VALUES (?, 'd', '2026-09-01T10:00:00Z', NULL, '2026-09-01T10:00:00Z', '2026-09-01T10:00:00Z')`
	_, err = db.ExecContext(ctx, insert, "e1")
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, insert, "e2")
	require.Error(t, err, "second active entry must violate the unique index")
}

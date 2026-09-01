-- +goose Up
-- Initial schema for the local-first timetracker (spec 001-local-first-cli §7).
-- All instants are stored as UTC RFC 3339 text; only the application
-- converts for display and local-date filtering.

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE projects (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    color      TEXT,
    archived   INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE time_entries (
    id                        TEXT PRIMARY KEY,
    description               TEXT NOT NULL CHECK (TRIM(description) <> ''),
    started_at                TEXT NOT NULL,
    stopped_at                TEXT,
    project_id                TEXT REFERENCES projects(id),
    pomodoro                  INTEGER NOT NULL DEFAULT 0 CHECK (pomodoro IN (0, 1)),
    pomodoro_duration_seconds INTEGER,
    pomodoro_ends_at          TEXT,
    created_at                TEXT NOT NULL,
    updated_at                TEXT NOT NULL,
    CHECK (stopped_at IS NULL OR stopped_at >= started_at)
);

CREATE INDEX idx_time_entries_user_started
    ON time_entries (started_at DESC);

CREATE INDEX idx_time_entries_project
    ON time_entries (project_id)
    WHERE project_id IS NOT NULL;

-- Invariant: at most one active time entry (spec §9.1).
CREATE UNIQUE INDEX one_active_time_entry
    ON time_entries ((1))
    WHERE stopped_at IS NULL;

-- Invariant support: an active Pomodoro carries its planned duration and
-- scheduled end (spec §9.8). Completed Pomodoros may omit them.
CREATE INDEX idx_time_entries_active_pomodoro
    ON time_entries (pomodoro_ends_at)
    WHERE stopped_at IS NULL AND pomodoro = 1;

-- +goose Down
DROP INDEX IF EXISTS idx_time_entries_active_pomodoro;
DROP INDEX IF EXISTS one_active_time_entry;
DROP INDEX IF EXISTS idx_time_entries_project;
DROP INDEX IF EXISTS idx_time_entries_user_started;
DROP TABLE IF EXISTS time_entries;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS settings;

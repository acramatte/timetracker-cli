# Local-First TimeTracker CLI — Product Specification

**Status:** Proposed
**Target:** `timetracker-cli`
**Implementation language:** Go
**Primary interface:** non-interactive CLI; interactive TUI is a later interface over the same domain layer.

## 1. Problem

Humans and agents need to record time spent on tasks without signing into a browser or relying on a hosted service. The existing Phoenix TimeTracker establishes useful domain concepts—projects, timed entries, reporting, CSV export, and pomodoro entries—but the new CLI must operate independently of that application.

## 2. Product decision

Version 1 is a self-contained, local-first Go command-line application backed by SQLite. It has no account, server, network, API token, or browser requirement.

The distributed `timetracker` executable contains its SQLite driver. It does **not** require a separately installed `sqlite3` executable or database service.

## 3. Goals

1. One portable CLI binary can initialise and use a local tracker without setup beyond installation.
2. Both humans and agents can start, stop, inspect, search, correct, report on, and export time entries.
3. Commands are safe for non-interactive invocation and provide stable JSON output.
4. The database preserves the useful semantics of the existing TimeTracker domain while removing web-account concerns.
5. Data is durable, inspectable, exportable, and straightforward to back up.
6. An interactive TUI can be added without duplicating domain rules or persistence logic.

## 4. Non-goals for version 1

- Connecting to, syncing with, or authenticating against the existing Phoenix application.
- Multi-device synchronisation, sharing, team workspaces, or hosted accounts.
- A browser UI, daemon, or background service.
- A required TUI.
- Full parity with every web-screen interaction.
- Billing, rate calculation, invoicing, or calendar integration.

## 5. User and agent workflows

### 5.1 Track current work

```text
start → inspect status → stop
```

The default `start` operation creates one active entry with the system time as its start. `stop` closes that entry using the system time.

### 5.2 Complete a focused Pomodoro

```text
pomodoro start → foreground countdown → notify and complete
```

`pomodoro start` creates the same kind of active entry as ordinary tracking, marked as a Pomodoro and given a scheduled end. Its default duration is **30 minutes**, matching the Phoenix application. The command displays a countdown while it owns the foreground terminal; on expiry it completes the entry at the scheduled end and sends a best-effort local notification. A caller can stop the Pomodoro early with `pomodoro stop` from another terminal.

If the countdown process exits unexpectedly, the entry remains durable. The next command reconciles an overdue Pomodoro by completing it at its scheduled end; reconciliation must not extend the recorded duration to the later recovery time.

### 5.3 Record a completed entry

A caller can create a manually dated, completed entry with explicit start and stop values. This supports corrections and work recorded after the fact.

### 5.3 Find and report past work

A caller can list entries filtered by date range, project, description text, and active/completed state; calculate totals for a selected set; and export the same selected set as CSV.

### 5.4 Manage projects

A caller can create, list, and archive local projects. Archived projects remain attached to historical entries and are excluded from normal active-project selection unless explicitly requested.

## 6. Command contract

Commands shown here are the target public contract, not implementation instructions.

```text
timetracker init
timetracker status [--json]
timetracker start <description> [--project <name-or-id>] [--replace] [--json]
timetracker stop [--entry <id>] [--at <RFC3339>] [--json]
timetracker pomodoro start <description> [--project <name-or-id>] [--minutes <positive-integer>] [--replace] [--json]
timetracker pomodoro stop [--entry <id>] [--at <RFC3339>] [--json]
timetracker entries add <description> --start <RFC3339> --stop <RFC3339> [--project <name-or-id>] [--pomodoro] [--json]
timetracker entries list [--from <YYYY-MM-DD>] [--to <YYYY-MM-DD>] [--project <name-or-id>] [--query <text>] [--status active|completed|all] [--json]
timetracker entries edit <id> [--description <text>] [--project <name-or-id>] [--start <RFC3339>] [--stop <RFC3339>] [--json]
timetracker projects add <name> [--color <#RRGGBB>] [--json]
timetracker projects list [--all] [--json]
timetracker projects archive <id> [--json]
timetracker report [filters] [--json]
timetracker export csv [filters] [--output <path>|-]
timetracker backup <path>
timetracker doctor [--json]
```

### 6.1 Pomodoro interaction and notification

- `pomodoro start` defaults to 30 minutes; `--minutes` explicitly overrides that duration for one session.
- A running Pomodoro is a foreground countdown command, not a background daemon. It shows remaining time in an interactive terminal and completes the entry at its scheduled end.
- At completion, the command attempts an OS-native desktop notification and falls back to a terminal bell plus a clear completion message when desktop delivery is unavailable.
- Notification failure is a warning only: the database completion is still successful.
- With `--json`, the command emits only the final completed entry to stdout; it must not interleave countdown frames or notification diagnostics with JSON.
- Before evaluating a new active-entry operation, the CLI reconciles any overdue Pomodoro at its stored scheduled end. A later recovery never creates a duplicate completion notification.

### 6.2 Output rules

- Human-readable command results go to stdout by default.
- With `--json`, stdout contains exactly one JSON document and no decorative text.
- Diagnostics, warnings, and failures go to stderr.
- A successful mutation returns the resulting resource in JSON mode.
- `export csv --output -` writes only CSV to stdout.
- Commands that might prompt in a later interface must fail with a clear actionable error when essential input is absent; the v1 commands must not prompt.

### 6.3 Exit rules

- `0`: command succeeded.
- Non-zero: the requested operation was not completed.
- The implementation will document stable categories for validation, not-found, conflict, and storage errors before the first release.

## 7. Domain model

The model is deliberately similar to the existing TimeTracker application, but is single-user and local.

### 7.1 Project

| Field | Meaning |
|---|---|
| `id` | Application-generated, globally portable identifier (ULID or UUID string). |
| `name` | Required project name. |
| `color` | Optional display hint; no effect on reports. |
| `archived` | Whether new entries should normally avoid this project. |
| `created_at`, `updated_at` | UTC audit timestamps. |

### 7.2 Time entry

| Field | Meaning |
|---|---|
| `id` | Application-generated, globally portable identifier. |
| `description` | Required task/work description. |
| `started_at` | Required UTC instant. |
| `stopped_at` | Optional UTC instant; `NULL` means active. |
| `project_id` | Optional local project reference. |
| `pomodoro` | Boolean marker for a Pomodoro-created entry. |
| `pomodoro_duration_seconds` | Nullable planned duration. Required for an active Pomodoro created by `pomodoro start`. |
| `pomodoro_ends_at` | Nullable scheduled UTC completion instant. Required for an active Pomodoro created by `pomodoro start`. |
| `created_at`, `updated_at` | UTC audit timestamps. |

### 7.3 Settings

A small key/value settings store holds local preferences, beginning with `timezone` and `pomodoro_default_minutes`. The timezone default is resolved at initialisation from the host environment, with `Etc/UTC` as the safe fallback. The Pomodoro default is 30 minutes.

## 8. Persistence and portability

### 8.1 Default locations

The database belongs in user data storage, separate from executable installation and configuration.

| Concern | Linux default | macOS default | Windows default |
|---|---|---|---|
| Executable | Package-manager or user-selected `PATH` location | Package-manager or user-selected `PATH` location | Package-manager or user-selected `PATH` location |
| Data directory | `$XDG_DATA_HOME/timetracker`, defaulting to `~/.local/share/timetracker` | `~/Library/Application Support/timetracker` | `%LOCALAPPDATA%\\timetracker` |
| Database | `<data-dir>/timetracker.db` | `<data-dir>/timetracker.db` | `<data-dir>\\timetracker.db` |
| Configuration | `$XDG_CONFIG_HOME/timetracker`, defaulting to `~/.config/timetracker` | `~/Library/Application Support/timetracker` | `%APPDATA%\\timetracker` |

SQLite may create `timetracker.db-wal` and `timetracker.db-shm` beside the database. They are live database files and must be included in operational guidance.

### 8.2 Overrides

The implementation must support both of these deterministic overrides:

```text
--data-dir <directory>
TIMETRACKER_DATA_DIR=<directory>
```

The command-line flag wins over the environment variable; the environment variable wins over the platform default. This supports tests, portable installations, and agent sandboxing.

### 8.3 Database policy

- Use SQLite in WAL mode with a bounded busy timeout.
- Create the data directory with user-only permissions where the host platform supports it.
- Store instants in UTC; convert only for display, date filtering, and export according to the configured timezone.
- Version the schema with forward-only SQL migrations managed by Goose and embedded in the distributed executable. Users do not need a `goose` executable or a separate migration step.
- Use a handwritten, parameterised `database/sql` SQLite repository for version 1. It owns transaction boundaries, query composition, error translation, and mapping persistence rows into domain values.
- Defer `sqlc` until the schema and query surface have stabilised and generated query plumbing demonstrably reduces repetition without obscuring domain transaction boundaries. If adopted, Goose migrations remain the schema source of truth and generated query methods remain behind the repository boundary.

## 9. Invariants and safety rules

1. A database has at most one active time entry. This is a database-enforced invariant, not merely CLI behavior.
2. A completed entry must not stop before it starts.
3. Entry descriptions are required and must contain non-whitespace content after normalisation.
4. Project references, when supplied, must resolve to an existing project.
5. Archiving a project must not remove or rewrite historical entries.
6. `start` fails when another entry is active. Only `start --replace` may close the current entry and start another, and it must do both in one transaction.
7. `stop` without `--entry` acts only on the single active entry; it must fail clearly when there is none.
8. An active Pomodoro has a positive planned duration and a scheduled end equal to its start plus that duration.
9. A Pomodoro expiry or overdue recovery completes at its stored scheduled end, not at the notification or recovery instant.
10. Date filters are interpreted as inclusive local calendar dates in the configured timezone.
11. CSV export uses exactly the same filter semantics as `entries list` and `report`.

## 10. Reporting and export

`report` returns the selected entry count and total completed duration. Active entries are visible in listings but are excluded from completed-duration totals unless a future explicitly named option says otherwise.

The CSV export must include at least:

```text
id,description,project,start_time,end_time,duration_seconds,duration_formatted,pomodoro
```

CSV must be encoded by a compliant CSV writer so descriptions, project names, quotes, commas, and line breaks remain valid and round-trip safely.

## 11. Backup, recovery, and diagnosis

- `backup <path>` creates a consistent SQLite backup without requiring a user to manually copy a live database and journal files.
- `export csv` and a future JSON export are portable data exits, not substitutes for a database backup.
- `doctor` reports the resolved data directory, database path, schema version, configured timezone, and actionable storage/migration health errors.
- No command sends data off the host in version 1.

## 12. Future compatibility

Application-generated string identifiers, UTC timestamps, explicit embedded migrations, and a repository boundary are intentional preparation for a later import/export or synchronisation adapter. They do not imply that v1 must implement remote sync.

## 13. Open decisions to settle before implementation

1. Choose ULID versus UUID for identifiers; both satisfy the portable-ID requirement.
2. Choose the Go SQLite driver after a short reproducible spike, preferring a single-binary, cross-platform distribution path.
3. Decide the exact default timezone detection and how `timetracker config set timezone` is exposed.
4. Choose the implementation boundary for OS-native notifications while preserving the required desktop-notification attempt and terminal fallback.

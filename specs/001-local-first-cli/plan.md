# Local-First TimeTracker CLI — Delivery Plan

**Status:** Proposed
**Implementation status:** No code has been started.

## Delivery strategy

Build vertically from durable local storage to agent-safe commands, then reporting/backup, and only then optional interactive presentation. Each phase has an independently demonstrable outcome.

## Phase 0 — Foundation decisions and repository setup

### Status: implemented on `feat/phase-0-foundation` (PR #1); migrations fix (`cec1bb8`) included.

### Outcome

A Go project can build a single local executable and has a documented technical baseline.

### Work

1. Initialise the Go module and project layout.
2. Choose and document the SQLite driver through a small portability spike:
   - no separately installed SQLite executable;
   - supported on intended Linux, macOS, and Windows targets;
   - compatible with the chosen release build approach.
3. Add embedded Goose support for forward-only SQL migrations and define the local data-directory resolver. The executable, not a user-installed `goose` command, owns migration application.
4. Record the v1 data-access decision: use a handwritten, parameterised `database/sql` repository; defer `sqlc` until schema and query shapes stabilise.
5. Add developer commands for formatting, linting, unit tests, and integration tests.
6. Add a short root README covering local-first intent and development entry points.

### Exit criteria

- A clean checkout builds one executable on the development platform.
- The selected driver/distribution choice is documented with its trade-offs.
- Migrations can be applied by the executable without a separately installed migration tool.
- The handwritten-repository boundary and deferred `sqlc` adoption criteria are documented.
- No tracker behavior exists yet beyond groundwork.

## Phase 1 — Durable database and domain core

### Outcome

The application creates and migrates a private local database whose schema enforces the key domain invariants, and exposes a typed repository boundary for all future commands.

### Status: implemented on `feat/phase-1-domain-core` (PR #2, stacked on PR #1)

### Work

1. Implement platform data/config directory resolution and command/environment overrides.
2. Create embedded Goose SQL migrations for settings, projects, and time entries, including persisted Pomodoro duration and scheduled-end fields.
3. Enable SQLite WAL and configure bounded contention behavior.
4. Persist default timezone and 30-minute Pomodoro configuration at initialisation.
5. Implement handwritten `database/sql` repository transactions, dynamic query composition, row mapping, and error translation.
6. Enforce one active entry, valid time ranges, and persisted Pomodoro duration/end invariants at the database layer.
7. Implement `doctor` use case that exposes resolved paths, schema version, and storage health.

### Exit criteria

- Fresh initialisation is repeatable and idempotent.
- Concurrent starts cannot leave two active entries.
- A database remains usable after an application restart.
- Path overrides are deterministic and isolated for tests.

## Phase 2 — Agent-first tracking and project commands

### Status: implemented on `feat/phase-2-commands` (PR #3, stacked on PR #2). Shell completion generation deferred to Phase 4 (E4) per task order.

### Outcome

Agents can manage active work and projects entirely through non-interactive commands.

### Work

1. Implement project add/list/archive application services and commands.
2. Implement status, start, stop, and explicit `start --replace` behavior.
3. Implement dedicated Pomodoro start/early-stop commands with a default 30-minute duration, one-session `--minutes` override, and scheduled-end persistence.
4. Implement a foreground countdown runner plus best-effort desktop notification and terminal fallback; make notification failure non-fatal.
5. Implement overdue-Pomodoro reconciliation at the scheduled end before a new active-entry operation.
6. Implement completed/manual-entry creation and entry edit.
7. Define stable JSON resource shapes and error responses.
8. Ensure stdout/stderr separation and non-zero failures.
9. Add shell completion generation only after command names/flags stabilise.

### Exit criteria

- The complete start/status/stop and Pomodoro countdown loop works with both human and JSON output.
- A Pomodoro completes at its scheduled end, survives interruption through accurate overdue recovery, and reports notification failure without losing the entry.
- An active-entry conflict is visible and safe by default.
- All mutations return the persisted result in JSON mode.
- No command opens a prompt or depends on a TUI.

## Phase 3 — Search, reporting, export, and backup

### Outcome

A caller can trust the CLI to retrieve, total, export, and protect local time data.

### Work

1. Implement consistent date-range, project, text, and status filters.
2. Implement timezone-aware local calendar date filtering.
3. Implement reports and total completed duration.
4. Implement standards-compliant CSV export using the same filters.
5. Implement a SQLite-consistent backup command.
6. Add a structured diagnostic report for corruption, migration, and permission failures.
7. Design a JSON export format, even if its CLI command ships in a subsequent small release.

### Exit criteria

- `entries list`, `report`, and `export csv` select the same logical entries for identical filters.
- Export is valid for special characters and can be consumed by an external CSV parser.
- Backup/restoration is demonstrated against a known fixture database.

## Phase 4 — Hardening and distribution

### Outcome

The tool is ready for ordinary local installation and unattended agent use.

### Work

1. Establish build matrix and release artifact naming for Linux, macOS, and Windows.
2. Add migration-upgrade regression fixtures.
3. Add lock-contention and abrupt-process-interruption tests.
4. Add command reference, examples, recovery guide, and data-location documentation.
5. Add a release smoke-test script that uses an isolated data directory.
6. Verify that uninstall guidance preserves user data.

### Exit criteria

- A release artifact passes the same smoke flow on every supported platform.
- Version upgrades preserve historical entries.
- Documentation explains data location, backup, recovery, and agent-safe JSON use.

## Phase 5 — Optional TUI

### Outcome

Humans can use a focused terminal UI without changing the domain/persistence architecture.

### Work

1. Select and spike a Go TUI library.
2. Render active timer, quick start/stop, projects, history, and filter state.
3. Reuse Phase 1–3 application services directly.
4. Keep every capability accessible without the TUI.

### Exit criteria

- TUI and CLI produce indistinguishable persisted results for equivalent actions.
- The non-interactive commands remain fully supported and documented.

## Deferred follow-up: import, export, or sync

Only after local behavior is established, assess interoperability with the Phoenix app. Any future sync effort needs its own specification covering identity, conflict resolution, ordering, auth, privacy, offline behavior, and migration ownership.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| A driver complicates cross-platform release | Run the driver/build spike before domain implementation. |
| Two local processes start entries simultaneously | Enforce the invariant in SQLite and test it under contention. |
| Timezone/date filters surprise users | Store UTC instants, persist one explicit timezone, and test DST boundaries. |
| Users copy a live database incorrectly | Provide a backup command and document WAL sidecar files. |
| The future TUI duplicates business behavior | Keep TUI as an adapter over application services only. |
| Premature remote compatibility distorts v1 | Use portable IDs and exports, but defer sync machinery. |

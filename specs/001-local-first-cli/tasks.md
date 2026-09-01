# Local-First TimeTracker CLI — Implementation Task Breakdown

**Status:** Proposed
**Rule:** Do not begin a later task until its stated dependencies and acceptance references are satisfied.

## Milestone A — Project foundation

- [ ] **A1** Initialise the Go module, executable entry point, and conventional internal package layout.
- [ ] **A2** Add developer tooling commands for format, lint, unit test, integration test, and release smoke test.
- [ ] **A3** Run a SQLite-driver portability spike; record the selected driver and distribution rationale.
- [ ] **A4** Implement platform data/config directory resolution plus `--data-dir` and `TIMETRACKER_DATA_DIR` precedence.
- [ ] **A5** Add root documentation for installation assumptions and local-first scope.

**Depends on:** none
**Acceptance references:** AT-01 through AT-04.

## Milestone B — Database and migration boundary

- [ ] **B1** Add embedded Goose ownership for forward-only SQL schema migrations and database-version tracking; require no user-installed `goose` executable.
- [ ] **B2** Add settings, projects, and time-entry tables with application-generated string IDs, UTC audit fields, and Pomodoro duration/scheduled-end fields.
- [ ] **B3** Add database constraints for required descriptions, valid stop/start ordering, project references, one active entry, and valid active-Pomodoro duration/end facts.
- [ ] **B4** Configure WAL mode, short transactions, and bounded busy timeout on every database open.
- [ ] **B5** Implement idempotent database initialisation plus persisted default timezone and 30-minute Pomodoro settings.
- [ ] **B6** Implement a handwritten parameterised `database/sql` SQLite repository for reads, writes, transactions, dynamic filters, and known error categories; keep `sqlc` deferred until its adoption criteria are met.
- [ ] **B7** Implement `doctor` data-path, schema, timezone, and basic health reporting.

**Depends on:** A1, A3, A4
**Acceptance references:** AT-05 through AT-12.

## Milestone C — Projects and tracking commands

- [ ] **C1** Implement project add/list/archive application services.
- [ ] **C2** Implement the active-entry status query.
- [ ] **C3** Implement safe timer start with description/project validation.
- [ ] **C4** Implement timer stop, including no-active and named-entry cases.
- [ ] **C5** Implement explicit transactional `start --replace`.
- [ ] **C6** Implement durable 30-minute-default Pomodoro start and early-stop application services, including one-session duration override.
- [ ] **C7** Implement foreground countdown, desktop-notification attempt, terminal fallback, and non-fatal notification failure behavior.
- [ ] **C8** Reconcile overdue Pomodoros at the stored scheduled end before active-entry operations.
- [ ] **C9** Implement manual completed-entry creation.
- [ ] **C10** Implement entry correction/edit behavior.
- [ ] **C11** Define JSON resource envelopes and command error behavior.
- [ ] **C12** Add non-interactive command integration tests with isolated data directories.

**Depends on:** B1 through B6
**Acceptance references:** AT-13 through AT-22 and AT-43 through AT-49.

## Milestone D — History, reports, and durable exits

- [ ] **D1** Implement entry list filters for date range, project, description text, and state.
- [ ] **D2** Implement timezone-aware local date boundaries, including DST regression fixtures.
- [ ] **D3** Implement completed-duration report totals.
- [ ] **D4** Implement CSV export from the shared filter/query path.
- [ ] **D5** Implement consistent SQLite backup.
- [ ] **D6** Document database, WAL sidecar files, backup, recovery, and export behavior.

**Depends on:** C1 through C12
**Acceptance references:** AT-23 through AT-31.

## Milestone E — Release hardening

- [ ] **E1** Add schema-upgrade fixtures covering every released migration path.
- [ ] **E2** Add lock-contention and unexpected-interruption recovery tests.
- [ ] **E3** Add platform release matrix and release smoke tests.
- [ ] **E4** Add shell completion generation after command interfaces stabilise.
- [ ] **E5** Publish command reference and agent integration examples.

**Depends on:** D1 through D6
**Acceptance references:** AT-32 through AT-39.

## Optional milestone F — TUI

- [ ] **F1** Run a small TUI-library spike and document selection criteria.
- [ ] **F2** Implement active timer and quick start/stop views.
- [ ] **F3** Implement project and history/filter views.
- [ ] **F4** Verify all TUI actions call the same application services as CLI commands.

**Depends on:** E1 through E5
**Acceptance references:** AT-40 through AT-42.

## Explicitly deferred

- [ ] **X1** Phoenix app import/export adapter.
- [ ] **X2** Remote sync and conflict policy.
- [ ] **X3** Authentication, accounts, or hosted service.
- [ ] **X4** Team/project sharing.

These are intentionally not part of the v1 critical path.
# Local-First TimeTracker CLI — Acceptance Test Plan

**Status:** Proposed
**Purpose:** Define observable behavior before implementation. These are acceptance scenarios, not test code.

## Test conventions

- Every automated scenario uses a new temporary data directory through `--data-dir`.
- Test fixtures use an explicit configured timezone, never the machine timezone by accident.
- JSON assertions parse stdout as JSON; they do not compare formatted terminal tables.
- Error assertions verify both non-zero exit status and absence of unintended database changes.
- Commands are non-interactive in every scenario.

## A. Installation, paths, and initialisation

| ID | Scenario | Given / When | Then |
|---|---|---|---|
| AT-01 | Single-binary operation | A supported release artifact is installed with no `sqlite3` or `goose` executable available. | `timetracker doctor` succeeds, applies its embedded migrations when needed, and identifies the linked local database implementation without launching a service or external migration command. |
| AT-02 | Default data directory | No data override is supplied. | The database is created in the documented platform user-data location. |
| AT-03 | Environment override | `TIMETRACKER_DATA_DIR` points at an empty temporary directory. | All database files are created only in that directory. |
| AT-04 | Flag precedence | Both the environment variable and `--data-dir` specify different directories. | The flag directory is used and the environment directory is untouched. |
| AT-05 | First-run idempotency | `timetracker init` is run twice against the same empty directory. | Both invocations succeed; one usable schema exists; no settings or tables are duplicated. |
| AT-06 | Default timezone | A database is initialised while a known host timezone is available. | `doctor --json` reports the persisted resolved timezone. |
| AT-07 | Secure local directory | Initialisation runs on a platform supporting POSIX permissions. | The data directory is not world-readable or world-writable. |

## B. Domain invariants and persistence

| ID | Scenario | Given / When | Then |
|---|---|---|---|
| AT-08 | Persistence | A project and completed entry are created; the command exits; a new command lists entries. | The entry and its project remain present. |
| AT-09 | Required description | A caller starts or adds an entry with empty or whitespace-only description. | The command fails; no entry is created. |
| AT-10 | Valid time range | A caller adds or edits an entry with a stop instant before its start instant. | The command fails; the previous record remains unchanged. |
| AT-11 | One active entry | Two start operations race against a database with no active entry. | Exactly one succeeds; exactly one active entry exists afterwards. |
| AT-12 | WAL coexistence | A read command overlaps an ordinary completed-entry write. | Both commands complete within the configured contention policy; the database remains consistent. |

## C. Projects

| ID | Scenario | Given / When | Then |
|---|---|---|---|
| AT-13 | Create project | `projects add` receives a name and color. | JSON output contains an ID, supplied name/color, `archived: false`, and UTC audit fields. |
| AT-14 | List projects | Active and archived projects exist. | `projects list` excludes archived projects by default; `projects list --all` includes both. |
| AT-15 | Archive project | A project with historical entries is archived. | It is unavailable for normal new-entry selection but historical entries retain their project identity. |
| AT-16 | Unknown project | A start/add/edit command references an unknown project ID or name. | The command fails with no entry mutation. |

## D. Active timer lifecycle

| ID | Scenario | Given / When | Then |
|---|---|---|---|
| AT-17 | Start | No entry is active and a caller starts a description. | One entry is persisted with a non-empty ID, UTC `started_at`, `stopped_at: null`, and the exact description. |
| AT-18 | Status | One entry is active. | `status --json` returns that entry and no human-readable text on stdout. |
| AT-19 | Safe conflict | One entry is active and caller runs a normal second `start`. | It fails clearly, preserves the active entry, and creates no second entry. |
| AT-20 | Explicit replace | One entry is active and caller runs `start --replace` with another description. | The old entry is stopped and one new active entry is created as one durable transaction. |
| AT-21 | Stop | One entry is active and caller runs `stop`. | The same entry receives a UTC stop instant at or after its start and `status` reports no active entry. |
| AT-22 | Stop without active work | No entry is active and caller runs `stop`. | It fails clearly and does not create an entry. |

## E. Manual entries and corrections

| ID | Scenario | Given / When | Then |
|---|---|---|---|
| AT-23 | Manual completed entry | A caller provides description, valid start, valid stop, optional project, and pomodoro marker. | A completed entry persists with all supplied facts. |
| AT-24 | Edit metadata | A completed entry is edited to change description and project. | The same ID remains; updated values persist; timing is unchanged. |
| AT-25 | Edit timing | A completed entry is edited with a new valid start/stop range. | The entry’s subsequent report contribution uses the corrected duration. |
| AT-26 | Unknown entry | A stop or edit operation names an unknown ID. | It fails with no unrelated mutation. |

## F. Filters, report totals, and timezone semantics

| ID | Scenario | Given / When | Then |
|---|---|---|---|
| AT-27 | Description filter | Entries have distinct descriptions; a list/report query specifies a text term. | Only matching descriptions are selected according to documented case behavior. |
| AT-28 | Project filter | Entries belong to two projects and a filter selects one. | Only that project’s entries appear in list, report, and CSV output. |
| AT-29 | Inclusive date range | The configured timezone is explicit and entries occur at the local start/end boundaries of a selected day. | Both boundary entries are included. |
| AT-30 | DST boundary | Fixtures cross a daylight-saving transition in a non-UTC configured timezone. | Local calendar filtering selects entries by local date correctly; elapsed duration remains based on UTC instants. |
| AT-31 | Report totals | Two completed fixture entries and one active entry are selected. | The completed total equals the sum of the completed fixture durations; the active entry is listed but excluded from that total. |

## G. CSV, backup, and diagnosis

| ID | Scenario | Given / When | Then |
|---|---|---|---|
| AT-32 | Filter parity | Identical filters are used with `entries list`, `report`, and `export csv`. | The same set of entry IDs is selected by all three operations. |
| AT-33 | CSV validity | Descriptions and project names contain commas, quotes, and line breaks. | Export parses with a standards-compliant CSV parser and round-trips the field values. |
| AT-34 | CSV stdout purity | `export csv --output -` is run. | Stdout contains only CSV; diagnostics are absent or go to stderr. |
| AT-35 | Consistent backup | A database has entries while WAL mode is active; `backup` is run. | The backup opens independently and contains a consistent snapshot of expected records. |
| AT-36 | Doctor diagnostics | The data directory is unreadable, the database is malformed, or a migration cannot complete. | `doctor` exits non-zero with an actionable diagnostic and does not overwrite existing data. |

## H. Release and upgrade quality

| ID | Scenario | Given / When | Then |
|---|---|---|---|
| AT-37 | Migration upgrade | A fixture database at every previously released schema version is opened by the current executable with no external migration tool installed. | Its embedded Goose migrations apply forward, preserve entries/projects/settings, and pass `doctor`. |
| AT-38 | Interrupted command recovery | A process is terminated during an ordinary mutation in a controlled test. | On the next open, SQLite recovers to an all-or-nothing durable state with no invariant violation. |
| AT-39 | Release smoke test | Each supported-platform release artifact uses an isolated data directory to initialise, add a project, start/stop, report, export, back up, and diagnose. | The complete flow succeeds without network access or a separate database process. |

## I. Pomodoro lifecycle and notification

| ID | Scenario | Given / When | Then |
|---|---|---|---|
| AT-43 | Default duration | No active entry exists and `pomodoro start` is invoked without `--minutes`. | It creates one active Pomodoro with a planned duration of 30 minutes and scheduled end equal to start plus that duration. |
| AT-44 | Duration override | No active entry exists and `pomodoro start --minutes <N>` is invoked with a positive duration. | The persisted duration and scheduled end use exactly `<N>` minutes. |
| AT-45 | Foreground completion | A Pomodoro foreground runner reaches its scheduled end. | It completes the entry at the stored scheduled end, reports completion, and attempts one notification. |
| AT-46 | Early stop | A Pomodoro is active and `pomodoro stop` is called before the scheduled end. | The entry is completed at the explicit or current stop instant and the foreground runner exits without an expiry notification. |
| AT-47 | Safe conflict and replace | A Pomodoro is active and a caller starts another timer. | Ordinary start fails safely; only explicit `--replace` can atomically close the original and create the replacement. |
| AT-48 | Interrupted-process recovery | A Pomodoro process exits before its deadline; a later command runs after the deadline. | The later command reconciles the entry at its stored scheduled end, does not extend its duration, and does not replay a missed notification. |
| AT-49 | Notification failure | The notification adapter reports failure at a Pomodoro deadline. | The entry still completes successfully; stderr carries an actionable warning and JSON output remains valid when requested. |

## J. Optional TUI parity

| ID | Scenario | Given / When | Then |
|---|---|---|---|
| AT-40 | TUI start/stop parity | A user starts and stops equivalent work through the TUI and through CLI commands in isolated databases. | Persisted entries have the same domain facts and satisfy the same constraints. |
| AT-41 | TUI conflict safety | An entry is already active before a TUI start action. | The TUI surfaces the same safe conflict as `start`; it does not silently replace the entry. |
| AT-42 | TUI-free operation | The TUI dependencies or terminal capabilities are unavailable. | All version 1 CLI workflows remain functional and unchanged. |

## Definition of done for v1

Version 1 is acceptable when AT-01 through AT-39 and AT-43 through AT-49 are automated where practical, remaining platform-specific checks have recorded evidence, and all user-visible behavior matches `spec.md`. AT-40 through AT-42 apply only if the optional TUI milestone is accepted.

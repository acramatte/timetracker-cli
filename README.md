# timetracker

A local-first time tracking CLI. One portable binary, one private SQLite
database — no account, no server, no browser.

Humans and agents run the same commands; `--json` and CSV output are
presentation modes for automation, not a separate interface.

## Status

Phases 0 through 3 are implemented on stacked branches: local persistence,
tracking and Pomodoro commands, projects, history filters, reports, CSV export,
and SQLite backup. The optional TUI and release-hardening phases remain.

- Product specification: [specs/001-local-first-cli/spec.md](specs/001-local-first-cli/spec.md)
- Architecture: [specs/001-local-first-cli/architecture.md](specs/001-local-first-cli/architecture.md)
- Delivery plan: [specs/001-local-first-cli/plan.md](specs/001-local-first-cli/plan.md)
- Decision records: [docs/decisions/](docs/decisions/)

## Development

Requires Go 1.27+.

```bash
make fmt      # gofmt
make vet      # go vet
make test     # go test ./...
make build    # ./bin/timetracker
make check    # fmt + vet + test
```

The SQLite driver is pure Go (`modernc.org/sqlite`, ADR 0001); no CGO
toolchain or SQLite installation is required.

## Data locations

| Platform | Data directory |
|---|---|
| Linux | `$XDG_DATA_HOME/timetracker` (default `~/.local/share/timetracker`) |
| macOS | `~/Library/Application Support/timetracker` |
| Windows | `%LOCALAPPDATA%\timetracker` |

Overrides: `--data-dir <dir>` wins over `TIMETRACKER_DATA_DIR`, which wins
over the platform default.

## History and backup

Calendar filters use the timezone persisted during `init`; `--from` and `--to`
are inclusive local dates. The `--to` date is converted to the exclusive start
of the following local day, including daylight-saving transitions.

```bash
timetracker entries list --from 2026-09-01 --to 2026-09-30
timetracker --json report --project <project-id>
timetracker export csv --from 2026-09-01 --output ./time.csv
timetracker backup ./backups/timetracker-2026-09-30.db
```

`export csv` writes only CSV to stdout when `--output -` is used. A file path
writes the CSV to that file and sends the confirmation to stderr. `backup`
uses SQLite's consistent `VACUUM INTO` snapshot and requires a destination
that does not already exist. SQLite may also maintain `-wal` and `-shm`
sidecars beside the live database; back up with the `backup` command rather
than copying the live database or its sidecars manually. Recovery consists of
restoring a backup to the configured data directory and letting the executable
apply any newer embedded migrations on its next open.

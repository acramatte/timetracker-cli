# timetracker

A local-first time tracking CLI. One portable binary, one private SQLite
database — no account, no server, no browser.

Humans and agents run the same commands; `--json` and CSV output are
presentation modes for automation, not a separate interface.

## Status

Phase 0 (foundation) — repository scaffolding, platform path resolution, and
the embedded-migration SQLite boundary. No tracking commands exist yet.

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

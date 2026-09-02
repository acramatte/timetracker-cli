package main

import (
	"fmt"
	"strings"
)

const topLevelUsage = `Usage:
  timetracker [global flags] <command> [command flags] [arguments]

Global flags:
  --data-dir <dir>  override the data directory
  --json            emit machine-readable JSON on stdout
  -h, --help        show this help

Commands:
  init                         initialise the database
  status                       show the active entry
  start <description>          start tracking work
  stop                         stop the active entry
  pomodoro start <description> start a timed Pomodoro
  pomodoro stop               stop a Pomodoro early
  projects add <name>         create a project
  projects list               list projects
  projects archive <id>       archive a project
  entries add <description>   add a completed entry
  entries edit <id>           edit an entry
  entries list                list entries
  report                      summarise completed time
  export csv                  export entries as CSV
  backup <path>               create a consistent database backup
  doctor                      diagnose the local database
  help [command]              show command help

Run 'timetracker help <command>' for command-specific flags.
`

var commandHelp = map[string]string{
	"init": `Usage: timetracker init

Initialise the local database and persist the host timezone.
`,
	"status": `Usage: timetracker status [--json]

Show the currently active entry, or report that no entry is active.
`,
	"start": `Usage: timetracker start [flags] <description>

Flags:
  --project <id>  assign the entry to a project
  --replace       stop the active entry and start a new one
`,
	"stop": `Usage: timetracker stop [flags]

Flags:
  --entry <id>       entry to stop (default: the active entry)
  --at <RFC3339>     explicit stop instant
`,
	"pomodoro": `Usage: timetracker pomodoro <start|stop> ...

Run 'timetracker help pomodoro start' or 'timetracker help pomodoro stop'.
`,
	"pomodoro start": `Usage: timetracker pomodoro start [flags] <description>

Flags:
  --project <id>  assign the entry to a project
  --minutes <N>   override the default 30-minute duration
`,
	"pomodoro stop": `Usage: timetracker pomodoro stop [flags]

Flags:
  --entry <id>  entry to stop (default: the active entry)
`,
	"projects": `Usage: timetracker projects <add|list|archive> ...

Run 'timetracker help projects add', 'list', or 'archive'.
`,
	"projects add": `Usage: timetracker projects add [flags] <name>

Flags:
  --color <#RRGGBB>  display color
`,
	"projects list": `Usage: timetracker projects list [--all]

Flags:
  --all  include archived projects
`,
	"projects archive": `Usage: timetracker projects archive <project-id>
`,
	"entries": `Usage: timetracker entries <add|edit|list> ...

Subcommands:
  add <description>  add a completed entry
  edit <entry-id>    edit an entry
  list               list entries

Common list flags:
  --from <YYYY-MM-DD>  first local calendar date
  --to <YYYY-MM-DD>    last local calendar date
  --project <id>       filter by project
  --query <text>       case-insensitive description text
  --status <state>     active, completed, or all

Run 'timetracker help entries add' or 'timetracker help entries edit' for mutation flags.
`,
	"entries add": `Usage: timetracker entries add [flags] <description>

Flags:
  --start <RFC3339>  start instant (required)
  --stop <RFC3339>   stop instant (required)
  --project <id>     assign the entry to a project
  --pomodoro         mark the entry as a Pomodoro
`,
	"entries edit": `Usage: timetracker entries edit [flags] <entry-id>

Flags:
  --description <text>  new description
  --project <id>        new project ID (empty clears it)
  --start <RFC3339>     new start instant
  --stop <RFC3339>      new stop instant
`,
	"entries list": `Usage: timetracker entries list [flags]

Flags:
  --from <YYYY-MM-DD>  first local calendar date
  --to <YYYY-MM-DD>    last local calendar date
  --project <id>       filter by project
  --query <text>       case-insensitive description text
  --status <state>     active, completed, or all
`,
	"report": `Usage: timetracker report [flags]

Flags:
  --from <YYYY-MM-DD>  first local calendar date
  --to <YYYY-MM-DD>    last local calendar date
  --project <id>       filter by project
  --query <text>       case-insensitive description text
  --status <state>     active, completed, or all
`,
	"export": `Usage: timetracker export csv [flags]

Flags:
  --from <YYYY-MM-DD>  first local calendar date
  --to <YYYY-MM-DD>    last local calendar date
  --project <id>       filter by project
  --query <text>       case-insensitive description text
  --status <state>     active, completed, or all
  --output <path>      output path, or - for stdout (default -)
`,
	"export csv": `Usage: timetracker export csv [flags]

Flags:
  --from <YYYY-MM-DD>  first local calendar date
  --to <YYYY-MM-DD>    last local calendar date
  --project <id>       filter by project
  --query <text>       case-insensitive description text
  --status <state>     active, completed, or all
  --output <path>      output path, or - for stdout (default -)
`,
	"backup": `Usage: timetracker backup [flags] <path>

Create a consistent SQLite snapshot. The destination must not already exist.

Flags:
  --output <path>  backup database path (alternative to the positional path)
`,
	"doctor": `Usage: timetracker doctor

Report the data directory, schema version, timezone, and database health.
`,
}

func helpText(topic string) (string, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return topLevelUsage, nil
	}
	if text, ok := commandHelp[topic]; ok {
		return text, nil
	}
	return "", fmt.Errorf("unknown help topic %q (try: timetracker help)", topic)
}

func printHelp(topic string) error {
	text, err := helpText(topic)
	if err != nil {
		return err
	}
	fmt.Print(text)
	return nil
}

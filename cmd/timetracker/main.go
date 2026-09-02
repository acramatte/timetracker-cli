// Command timetracker is a local-first time tracking CLI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/acramatte/timetracker-cli/internal/app"
	"github.com/acramatte/timetracker-cli/internal/platform"
	"github.com/acramatte/timetracker-cli/internal/store"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "timetracker:", err)
		os.Exit(1)
	}
}

// run parses global flags and dispatches the subcommand. Output contract
// (spec §6.2): results to stdout, diagnostics to stderr, JSON in --json mode.
func run(args []string) error {
	fs := flag.NewFlagSet("timetracker", flag.ContinueOnError)
	dataDir := fs.String("data-dir", "", "override the data directory")
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON on stdout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return fmt.Errorf("no command given (try: init, status, start, stop, pomodoro, projects, entries, doctor)")
	}

	command, cmdArgs := rest[0], rest[1:]

	resolver := platform.NewResolver()
	if *dataDir != "" {
		resolver.SetDataDirOverride(*dataDir)
	}
	dataPath, err := resolver.DataDir()
	if err != nil {
		return err
	}
	dbPath := filepath.Join(dataPath, "timetracker.db")

	ctx := context.Background()
	db, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	st := &store.Store{DB: db}
	tracking := &app.TrackingService{Store: st}
	pomodoro := &app.PomodoroService{Store: st, Notifier: app.TerminalNotifier{Writer: os.Stderr}}
	projects := &app.ProjectsService{Store: st}
	entries := &app.EntriesService{Store: st}

	switch command {
	case "init":
		return cmdInit(ctx, st, dataPath)
	case "status":
		return cmdStatus(ctx, tracking, *jsonOut)
	case "start":
		return cmdStart(ctx, tracking, cmdArgs, *jsonOut)
	case "stop":
		return cmdStop(ctx, tracking, cmdArgs, *jsonOut)
	case "pomodoro":
		return cmdPomodoro(ctx, pomodoro, cmdArgs, *jsonOut)
	case "projects":
		return cmdProjects(ctx, projects, cmdArgs, *jsonOut)
	case "entries":
		return cmdEntries(ctx, entries, cmdArgs, *jsonOut)
	case "report":
		return cmdReport(ctx, st, cmdArgs, *jsonOut)
	case "export":
		return cmdExport(ctx, st, cmdArgs)
	case "backup":
		return cmdBackup(ctx, st, cmdArgs)
	case "doctor":
		return cmdDoctor(ctx, st, dataPath)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

// exitError carries a mapped exit code and stderr message through run().
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

func fail(err error) error {
	code, msg := app.MapError(err)
	return &exitError{code: code, msg: msg}
}

// printJSON writes the JSON envelope to stdout when --json; otherwise a
// human-readable line.
func printJSON(jsonMode bool, payload string, human string) {
	if jsonMode {
		fmt.Println(payload)
		return
	}
	fmt.Println(human)
}

var jsonMode = false

func cmdInit(ctx context.Context, s *store.Store, dataPath string) error {
	tz := "Etc/UTC"
	if local := time.Local.String(); local != "" && local != "Local" {
		tz = local
	}
	if err := s.Initialise(ctx, tz); err != nil {
		return err
	}
	fmt.Printf("initialised %s (timezone: %s)\n", dataPath, tz)
	return nil
}

func cmdStatus(ctx context.Context, t *app.TrackingService, jsonMode bool) error {
	e, err := t.Status(ctx)
	if err != nil {
		return err
	}
	if e == nil {
		if jsonMode {
			fmt.Println(`{"active": null}`)
		} else {
			fmt.Println("no active entry")
		}
		return nil
	}
	payload, err := app.MarshalActive(*e, nil)
	if err != nil {
		return err
	}
	printJSON(jsonMode, payload, fmt.Sprintf("tracking %q since %s", e.Description, e.StartedAt.Format(time.RFC3339)))
	return nil
}

func cmdStart(ctx context.Context, t *app.TrackingService, args []string, jsonMode bool) error {
	fs := flag.NewFlagSet("start", flag.ContinueOnError)
	project := fs.String("project", "", "project ID")
	replace := fs.Bool("replace", false, "stop the active entry and start a new one")
	if err := fs.Parse(args); err != nil {
		return err
	}
	desc := strings.Join(fs.Args(), " ")
	if desc == "" {
		return fail(fmt.Errorf("%w: description is required", app.ErrValidation))
	}

	opts := app.StartOptions{Description: desc, ProjectID: *project}

	if *replace {
		stopped, started, err := t.Replace(ctx, opts)
		if err != nil {
			return fail(err)
		}
		payload, err := app.MarshalEntryEnvelope(started)
		if err != nil {
			return err
		}
		printJSON(jsonMode, payload, fmt.Sprintf("replaced %s with %s", stopped.ID, started.ID))
		return nil
	}

	e, err := t.Start(ctx, opts)
	if err != nil {
		return fail(err)
	}
	payload, err := app.MarshalEntryEnvelope(e)
	if err != nil {
		return err
	}
	printJSON(jsonMode, payload, fmt.Sprintf("started %s", e.ID))
	return nil
}

func cmdStop(ctx context.Context, t *app.TrackingService, args []string, jsonMode bool) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	entry := fs.String("entry", "", "entry ID to stop (default: the active entry)")
	atStr := fs.String("at", "", "explicit stop instant (RFC3339)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var at *time.Time
	if *atStr != "" {
		parsed, err := time.Parse(time.RFC3339, *atStr)
		if err != nil {
			return fail(fmt.Errorf("%w: --at must be RFC3339", app.ErrValidation))
		}
		at = &parsed
	}

	e, err := t.Stop(ctx, *entry, at)
	if err != nil {
		return fail(err)
	}
	payload, err := app.MarshalEntryEnvelope(e)
	if err != nil {
		return err
	}
	printJSON(jsonMode, payload, fmt.Sprintf("stopped %s", e.ID))
	return nil
}

func cmdPomodoro(ctx context.Context, p *app.PomodoroService, args []string, jsonMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("pomodoro requires a subcommand: start|stop")
	}
	sub, subArgs := args[0], args[1:]

	switch sub {
	case "start":
		fs := flag.NewFlagSet("pomodoro start", flag.ContinueOnError)
		project := fs.String("project", "", "project ID")
		minutes := fs.Int("minutes", 0, "override duration in minutes (default 30)")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		desc := strings.Join(fs.Args(), " ")
		if desc == "" {
			return fail(fmt.Errorf("%w: description is required", app.ErrValidation))
		}

		e, err := p.Start(ctx, app.PomodoroStartOptions{
			Description: desc, ProjectID: *project, Minutes: *minutes,
		})
		if err != nil {
			return fail(err)
		}
		if !jsonMode {
			fmt.Printf("pomodoro started %s\n", e.ID)
			p.Progress = func(remaining time.Duration) {
				fmt.Printf("\rremaining %02d:%02d", int(remaining/time.Minute), int(remaining/time.Second)%60)
				if remaining <= 0 {
					fmt.Println()
				}
			}
		}
		completed, err := p.RunDeadline(ctx, e)
		if err != nil {
			return fail(err)
		}
		payload, err := app.MarshalEntryEnvelope(completed)
		if err != nil {
			return err
		}
		printJSON(jsonMode, payload, fmt.Sprintf("pomodoro complete %s", completed.ID))
		return nil

	case "stop":
		fs := flag.NewFlagSet("pomodoro stop", flag.ContinueOnError)
		entry := fs.String("entry", "", "entry ID (default: the active entry)")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		e, err := p.StopEarly(ctx, *entry, nil)
		if err != nil {
			return fail(err)
		}
		payload, err := app.MarshalEntryEnvelope(e)
		if err != nil {
			return err
		}
		printJSON(jsonMode, payload, fmt.Sprintf("pomodoro stopped early %s", e.ID))
		return nil

	default:
		return fmt.Errorf("unknown pomodoro subcommand %q", sub)
	}
}

func cmdProjects(ctx context.Context, p *app.ProjectsService, args []string, jsonMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("projects requires a subcommand: add|list|archive")
	}
	sub, subArgs := args[0], args[1:]

	switch sub {
	case "add":
		fs := flag.NewFlagSet("projects add", flag.ContinueOnError)
		color := fs.String("color", "", "display color (#RRGGBB)")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		name := strings.Join(fs.Args(), " ")
		proj, err := p.Add(ctx, app.AddOptions{Name: name, Color: *color})
		if err != nil {
			return fail(err)
		}
		fmt.Printf("created project %s\n", proj.ID)
		return nil

	case "list":
		fs := flag.NewFlagSet("projects list", flag.ContinueOnError)
		all := fs.Bool("all", false, "include archived")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		projects, err := p.List(ctx, *all)
		if err != nil {
			return err
		}
		for _, pr := range projects {
			archived := ""
			if pr.Archived {
				archived = " [archived]"
			}
			fmt.Printf("%s  %s%s\n", pr.ID, pr.Name, archived)
		}
		return nil

	case "archive":
		if len(subArgs) != 1 {
			return fmt.Errorf("archive requires exactly one project ID")
		}
		if err := p.Archive(ctx, subArgs[0]); err != nil {
			return fail(err)
		}
		fmt.Printf("archived %s\n", subArgs[0])
		return nil

	default:
		return fmt.Errorf("unknown projects subcommand %q", sub)
	}
}

func cmdEntries(ctx context.Context, e *app.EntriesService, args []string, jsonMode bool) error {
	if len(args) == 0 {
		return fmt.Errorf("entries requires a subcommand: add|edit|list")
	}
	sub, subArgs := args[0], args[1:]

	switch sub {
	case "list":
		return cmdEntriesList(ctx, e.Store, subArgs, jsonMode)
	case "add":
		fs := flag.NewFlagSet("entries add", flag.ContinueOnError)
		start := fs.String("start", "", "start instant (RFC3339)")
		stop := fs.String("stop", "", "stop instant (RFC3339)")
		project := fs.String("project", "", "project ID")
		pomodoro := fs.Bool("pomodoro", false, "mark as pomodoro")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		desc := strings.Join(fs.Args(), " ")
		startAt, err := time.Parse(time.RFC3339, *start)
		if err != nil {
			return fail(fmt.Errorf("%w: --start must be RFC3339", app.ErrValidation))
		}
		stopAt, err := time.Parse(time.RFC3339, *stop)
		if err != nil {
			return fail(fmt.Errorf("%w: --stop must be RFC3339", app.ErrValidation))
		}

		entry, err := e.Add(ctx, app.AddEntryOptions{
			Description: desc, Start: startAt, Stop: stopAt,
			ProjectID: *project, Pomodoro: *pomodoro,
		})
		if err != nil {
			return fail(err)
		}
		payload, err := app.MarshalEntryEnvelope(entry)
		if err != nil {
			return err
		}
		printJSON(jsonMode, payload, fmt.Sprintf("added entry %s", entry.ID))
		return nil

	case "edit":
		fs := flag.NewFlagSet("entries edit", flag.ContinueOnError)
		desc := fs.String("description", "", "new description")
		project := fs.String("project", "", "new project ID (empty string clears)")
		start := fs.String("start", "", "new start instant (RFC3339)")
		stop := fs.String("stop", "", "new stop instant (RFC3339)")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}
		if len(fs.Args()) != 1 {
			return fmt.Errorf("edit requires exactly one entry ID")
		}

		opts := app.EditOptions{Description: *desc}
		if *project != "" {
			opts.ProjectID = project
		}
		if *start != "" {
			parsed, err := time.Parse(time.RFC3339, *start)
			if err != nil {
				return fail(fmt.Errorf("%w: --start must be RFC3339", app.ErrValidation))
			}
			opts.Start = &parsed
		}
		if *stop != "" {
			parsed, err := time.Parse(time.RFC3339, *stop)
			if err != nil {
				return fail(fmt.Errorf("%w: --stop must be RFC3339", app.ErrValidation))
			}
			opts.Stop = &parsed
		}

		entry, err := e.Edit(ctx, fs.Arg(0), opts)
		if err != nil {
			return fail(err)
		}
		payload, err := app.MarshalEntryEnvelope(entry)
		if err != nil {
			return err
		}
		printJSON(jsonMode, payload, fmt.Sprintf("edited entry %s", entry.ID))
		return nil

	default:
		return fmt.Errorf("unknown entries subcommand %q", sub)
	}
}

func entryFilters(fs *flag.FlagSet) (from, to, project, query, status *string) {
	from = fs.String("from", "", "first local calendar date (YYYY-MM-DD)")
	to = fs.String("to", "", "last local calendar date (YYYY-MM-DD)")
	project = fs.String("project", "", "project ID")
	query = fs.String("query", "", "case-insensitive description text")
	status = fs.String("status", "", "entry state: active, completed, or all")
	return
}

func filterValues(from, to, project, query, status *string) app.EntryFilters {
	return app.EntryFilters{
		From: *from, To: *to, ProjectID: *project, Query: *query, Status: *status,
	}
}

func cmdEntriesList(ctx context.Context, s *store.Store, args []string, jsonMode bool) error {
	fs := flag.NewFlagSet("entries list", flag.ContinueOnError)
	from, to, project, query, status := entryFilters(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	entries, err := app.List(ctx, s, filterValues(from, to, project, query, status))
	if err != nil {
		return fail(err)
	}
	if jsonMode {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"entries": entries})
	}
	for _, result := range entries {
		entry := result.Entry
		end := "active"
		if entry.StoppedAt != nil {
			end = entry.StoppedAt.UTC().Format(time.RFC3339)
		}
		fmt.Printf("%s  %s  %s  %s  %s\n", entry.ID, entry.StartedAt.UTC().Format(time.RFC3339), end, result.ProjectName, entry.Description)
	}
	return nil
}

func cmdReport(ctx context.Context, s *store.Store, args []string, jsonMode bool) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	from, to, project, query, status := entryFilters(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	report, err := app.Report(ctx, s, filterValues(from, to, project, query, status))
	if err != nil {
		return fail(err)
	}
	payload := map[string]any{
		"count":                        report.Count,
		"completed_duration_seconds":   report.CompletedDuration,
		"completed_duration_formatted": formatReportDuration(report.CompletedDuration),
	}
	if jsonMode {
		return json.NewEncoder(os.Stdout).Encode(payload)
	}
	fmt.Printf("%d entries  %s completed\n", report.Count, payload["completed_duration_formatted"])
	return nil
}

func formatReportDuration(seconds int64) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	remaining := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, remaining)
}

func cmdExport(ctx context.Context, s *store.Store, args []string) error {
	if len(args) == 0 || args[0] != "csv" {
		return fmt.Errorf("export requires format: csv")
	}
	fs := flag.NewFlagSet("export csv", flag.ContinueOnError)
	from, to, project, query, status := entryFilters(fs)
	output := fs.String("output", "-", "output path, or - for stdout")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var out *os.File
	if *output == "-" {
		if err := app.ExportCSV(ctx, s, filterValues(from, to, project, query, status), os.Stdout); err != nil {
			return fail(err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0700); err != nil {
		return fail(fmt.Errorf("create export directory: %w", err))
	}
	var err error
	out, err = os.Create(*output)
	if err != nil {
		return fail(fmt.Errorf("create export file: %w", err))
	}
	defer out.Close()
	if err := app.ExportCSV(ctx, s, filterValues(from, to, project, query, status), out); err != nil {
		return fail(err)
	}
	fmt.Fprintf(os.Stderr, "exported CSV to %s\n", *output)
	return nil
}

func cmdBackup(ctx context.Context, s *store.Store, args []string) error {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	output := fs.String("output", "", "backup database path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	path := *output
	if path == "" && len(fs.Args()) == 1 {
		path = fs.Arg(0)
	}
	if path == "" {
		return fmt.Errorf("backup requires an output path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fail(fmt.Errorf("create backup directory: %w", err))
	}
	if err := s.Backup(ctx, path); err != nil {
		return fail(err)
	}
	fmt.Printf("backed up database to %s\n", path)
	return nil
}

func cmdDoctor(ctx context.Context, s *store.Store, dataPath string) error {
	report := store.Doctor(ctx, s)
	out := map[string]any{
		"data_dir":       dataPath,
		"schema_version": report.SchemaVersion,
		"timezone":       report.Timezone,
		"tables":         report.TablesPresent,
	}
	if report.Err != nil {
		out["error"] = report.Err.Error()
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	if report.Err != nil {
		os.Exit(app.ExitStorage)
	}
	return nil
}

// Package cli wires the timetracker command tree on Cobra.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/acramatte/timetracker-cli/internal/app"
	"github.com/acramatte/timetracker-cli/internal/platform"
	"github.com/acramatte/timetracker-cli/internal/store"
	"github.com/spf13/cobra"
)

// session holds the resolved data path and services shared by all commands.
// Commands that need the database (everything except help) resolve it lazily
// via services(), so --help works before any data directory exists.
type session struct {
	dataPath       string
	store          *store.Store
	tracking       *app.TrackingService
	pomodoro       *app.PomodoroService
	projects       *app.ProjectsService
	entries        *app.EntriesService
	notifierWriter io.Writer
}

type commandErrorWriter struct {
	command *cobra.Command
}

func (w commandErrorWriter) Write(p []byte) (int, error) {
	return w.command.ErrOrStderr().Write(p)
}

// services opens the database once per invocation and wires the services.
// The --data-dir flag is applied here via the resolver, so every command
// (not only init/doctor) honours the same precedence contract.
func (s *session) services(ctx context.Context) error {
	if s.store != nil {
		return nil
	}
	resolver := platform.NewResolver()
	resolver.SetDataDirOverride(s.dataPath)
	dir, err := resolver.DataDir()
	if err != nil {
		return err
	}
	s.dataPath = dir
	db, err := store.Open(ctx, filepath.Join(s.dataPath, "timetracker.db"))
	if err != nil {
		return err
	}
	st := &store.Store{DB: db}
	// Initialise is idempotent and never overwrites existing settings, so
	// calling it on every open guarantees timezone and Pomodoro defaults
	// exist without requiring an explicit init command.
	tz := "Etc/UTC"
	if local := time.Local.String(); local != "" && local != "Local" {
		tz = local
	}
	if err := st.Initialise(ctx, tz); err != nil {
		db.Close()
		return err
	}
	s.store = st
	s.tracking = &app.TrackingService{Store: st}
	notifierWriter := s.notifierWriter
	if notifierWriter == nil {
		notifierWriter = os.Stderr
	}
	s.pomodoro = &app.PomodoroService{Store: st, Notifier: app.TerminalNotifier{Writer: notifierWriter}}
	s.projects = &app.ProjectsService{Store: st}
	s.entries = &app.EntriesService{Store: st}
	return nil
}

// NewRootCommand builds the timetracker command tree. SilenceErrors and
// SilenceUsage keep output formatting in main's single error point.
func NewRootCommand() *cobra.Command {
	var jsonOut bool

	sess := &session{}
	root := &cobra.Command{
		Use:           "timetracker",
		Short:         "Local-first time tracking CLI",
		Long:          "A local-first time tracking CLI. One portable binary, one private SQLite database — no account, no server, no browser.",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Global flags may be placed before the command (README contract).
		TraverseChildren: true,
		Args:             cobra.ArbitraryArgs,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			flag := cmd.Root().PersistentFlags().Lookup("data-dir")
			if flag != nil && flag.Changed && sess.dataPath == "" {
				return usagef("--data-dir must not be empty")
			}
			return nil
		},
		// No bare invocation prints help; unknown first args are rejected.
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return usagef("unknown command %q for %q", args[0], cmd.CommandPath())
		},
	}
	sess.notifierWriter = commandErrorWriter{command: root}
	// The flag writes straight into the session so services() applies the
	// documented flag > env > platform-default precedence on first use.
	root.PersistentFlags().StringVar(&sess.dataPath, "data-dir", "", "override the data directory")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "emit machine-readable JSON on stdout")

	root.AddCommand(newInitCommand(sess),
		newStatusCommand(sess, &jsonOut),
		newStartCommand(sess, &jsonOut),
		newStopCommand(sess, &jsonOut),
		newPomodoroCommand(sess, &jsonOut),
		newProjectsCommand(sess, &jsonOut),
		newEntriesCommand(sess, &jsonOut),
		newReportCommand(sess, &jsonOut),
		newExportCommand(sess),
		newBackupCommand(sess),
		newDoctorCommand(sess))
	root.AddCommand(newCompletionCommand(root))
	return root
}

// newCompletionCommand exposes shell completion generation.
func newCompletionCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate a completion script for the given shell.

Install bash completion with:
  timetracker completion bash > /etc/bash_completion.d/timetracker

Install zsh completion with:
  timetracker completion zsh > "${fpath[1]}/_timetracker"`,
		Args:              cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgsFunction: cobra.NoFileCompletions,
		ValidArgs:         []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			case "powershell":
				return root.GenPowerShellCompletionWithDesc(out)
			}
			return nil
		},
	}
}

func newInitCommand(sess *session) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialise the local database",
		Long:  "Initialise the local database and persist the host timezone.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			tz, err := sess.store.GetSetting(ctx, store.SettingTimezone)
			if err != nil {
				return fail(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "initialised %s (timezone: %s)\n", sess.dataPath, tz)
			return nil
		},
	}
}

func newStatusCommand(sess *session, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active entry",
		Long:  "Show the currently active entry, or report that no entry is active.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			e, err := sess.tracking.Status(ctx)
			if err != nil {
				return fail(err)
			}
			if e == nil {
				w := cmd.OutOrStdout()
				if *jsonOut {
					fmt.Fprintln(w, `{"active": null}`)
				} else {
					fmt.Fprintln(w, "no active entry")
				}
				return nil
			}
			payload, err := app.MarshalActive(e)
			if err != nil {
				return err
			}
			printJSONWith(cmd, *jsonOut, payload, fmt.Sprintf("tracking %q since %s", e.Description, e.StartedAt.Format(time.RFC3339)))
			return nil
		},
	}
}

// printJSONWith writes the JSON envelope in --json mode; otherwise the
// human-readable line. Output goes through the command's writer so tests
// and callers can capture it.
func printJSONWith(cmd *cobra.Command, jsonMode bool, payload, human string) {
	w := cmd.OutOrStdout()
	if jsonMode {
		fmt.Fprintln(w, payload)
		return
	}
	fmt.Fprintln(w, human)
}

func newStartCommand(sess *session, jsonOut *bool) *cobra.Command {
	var project string
	var replace bool
	cmd := &cobra.Command{
		Use:   "start [flags] <description...>",
		Short: "Start tracking work",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			desc := strings.Join(args, " ")
			opts := app.StartOptions{Description: desc, ProjectID: project}
			if replace {
				stopped, started, err := sess.tracking.Replace(ctx, opts)
				if err != nil {
					return fail(err)
				}
				payload, err := app.MarshalEntryEnvelope(started)
				if err != nil {
					return err
				}
				printJSONWith(cmd, *jsonOut, payload, fmt.Sprintf("replaced %s with %s", stopped.ID, started.ID))
				return nil
			}
			e, err := sess.tracking.Start(ctx, opts)
			if err != nil {
				return fail(err)
			}
			payload, err := app.MarshalEntryEnvelope(e)
			if err != nil {
				return err
			}
			printJSONWith(cmd, *jsonOut, payload, fmt.Sprintf("started %s", e.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID")
	cmd.Flags().BoolVar(&replace, "replace", false, "stop the active entry and start a new one")
	return cmd
}

func newStopCommand(sess *session, jsonOut *bool) *cobra.Command {
	var entry string
	var atStr string
	cmd := &cobra.Command{
		Use:   "stop [flags]",
		Short: "Stop the active entry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			var at *time.Time
			if atStr != "" {
				parsed, err := time.Parse(time.RFC3339, atStr)
				if err != nil {
					return fail(fmt.Errorf("%w: --at must be RFC3339", app.ErrValidation))
				}
				at = &parsed
			}
			e, err := sess.tracking.Stop(ctx, entry, at)
			if err != nil {
				return fail(err)
			}
			payload, err := app.MarshalEntryEnvelope(e)
			if err != nil {
				return err
			}
			printJSONWith(cmd, *jsonOut, payload, fmt.Sprintf("stopped %s", e.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&entry, "entry", "", "entry ID to stop (default: the active entry)")
	cmd.Flags().StringVar(&atStr, "at", "", "explicit stop instant (RFC3339)")
	return cmd
}

func newPomodoroCommand(sess *session, jsonOut *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pomodoro",
		Short: "Timed Pomodoro sessions",
		Long:  "Start a timed Pomodoro or stop one early. Use 'timetracker pomodoro start' or 'timetracker pomodoro stop'.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newPomodoroStartCommand(sess, jsonOut), newPomodoroStopCommand(sess, jsonOut))
	return cmd
}

func newPomodoroStartCommand(sess *session, jsonOut *bool) *cobra.Command {
	var project string
	var minutes int
	cmd := &cobra.Command{
		Use:   "start [flags] <description...>",
		Short: "Start a timed Pomodoro",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			desc := strings.Join(args, " ")
			e, err := sess.pomodoro.Start(ctx, app.PomodoroStartOptions{
				Description: desc, ProjectID: project, Minutes: minutes,
			})
			if err != nil {
				return fail(err)
			}
			if !*jsonOut {
				fmt.Fprintf(cmd.OutOrStdout(), "pomodoro started %s\n", e.ID)
			}
			var progress func(remaining time.Duration)
			if !*jsonOut {
				progress = func(remaining time.Duration) {
					fmt.Fprintf(cmd.OutOrStdout(), "\rremaining %02d:%02d", int(remaining/time.Minute), int(remaining/time.Second)%60)
					if remaining <= 0 {
						fmt.Fprintln(cmd.OutOrStdout())
					}
				}
			}
			completed, err := sess.pomodoro.RunDeadline(ctx, e, progress)
			if err != nil {
				return fail(err)
			}
			payload, err := app.MarshalEntryEnvelope(completed)
			if err != nil {
				return err
			}
			printJSONWith(cmd, *jsonOut, payload, fmt.Sprintf("pomodoro complete %s", completed.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project ID")
	cmd.Flags().IntVar(&minutes, "minutes", 0, "override duration in minutes (default 30)")
	return cmd
}

func newPomodoroStopCommand(sess *session, jsonOut *bool) *cobra.Command {
	var entry string
	cmd := &cobra.Command{
		Use:   "stop [flags]",
		Short: "Stop a Pomodoro early",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			e, err := sess.pomodoro.StopEarly(ctx, entry, nil)
			if err != nil {
				return fail(err)
			}
			payload, err := app.MarshalEntryEnvelope(e)
			if err != nil {
				return err
			}
			printJSONWith(cmd, *jsonOut, payload, fmt.Sprintf("pomodoro stopped early %s", e.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&entry, "entry", "", "entry ID (default: the active entry)")
	return cmd
}

func newProjectsCommand(sess *session, jsonOut *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "projects",
		Short: "Manage projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newProjectsAddCommand(sess, jsonOut), newProjectsListCommand(sess, jsonOut), newProjectsArchiveCommand(sess, jsonOut))
	return cmd
}

func newProjectsAddCommand(sess *session, jsonOut *bool) *cobra.Command {
	var color string
	cmd := &cobra.Command{
		Use:   "add [flags] <name...>",
		Short: "Create a project",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			proj, err := sess.projects.Add(ctx, app.AddOptions{Name: strings.Join(args, " "), Color: color})
			if err != nil {
				return fail(err)
			}
			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(app.ProjectEnvelope{Project: app.ToProjectDTO(proj)})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created project %s\n", proj.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&color, "color", "", "display color (#RRGGBB)")
	return cmd
}

func newProjectsListCommand(sess *session, jsonOut *bool) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			projects, err := sess.projects.List(ctx, all)
			if err != nil {
				return fail(err)
			}
			if *jsonOut {
				dtos := make([]app.ProjectDTO, len(projects))
				for i, project := range projects {
					dtos[i] = app.ToProjectDTO(project)
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(app.ProjectsEnvelope{Projects: dtos})
			}
			for _, pr := range projects {
				archived := ""
				if pr.Archived {
					archived = " [archived]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s%s\n", pr.ID, pr.Name, archived)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "include archived projects")
	return cmd
}

func newProjectsArchiveCommand(sess *session, jsonOut *bool) *cobra.Command {
	return &cobra.Command{
		Use:   "archive <project-id>",
		Short: "Archive a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			project, err := sess.projects.Archive(ctx, args[0])
			if err != nil {
				return fail(err)
			}
			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(app.ProjectEnvelope{Project: app.ToProjectDTO(project)})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "archived %s\n", project.ID)
			return nil
		},
	}
}

func newEntriesCommand(sess *session, jsonOut *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "entries",
		Short: "Manage time entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newEntriesAddCommand(sess, jsonOut), newEntriesEditCommand(sess, jsonOut), newEntriesListCommand(sess, jsonOut))
	return cmd
}

func newEntriesAddCommand(sess *session, jsonOut *bool) *cobra.Command {
	var start, stop, project string
	var pomodoro bool
	cmd := &cobra.Command{
		Use:   "add [flags] <description...>",
		Short: "Add a completed entry",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			desc := strings.Join(args, " ")
			startAt, err := time.Parse(time.RFC3339, start)
			if err != nil {
				return fail(fmt.Errorf("%w: --start must be RFC3339", app.ErrValidation))
			}
			stopAt, err := time.Parse(time.RFC3339, stop)
			if err != nil {
				return fail(fmt.Errorf("%w: --stop must be RFC3339", app.ErrValidation))
			}
			entry, err := sess.entries.Add(ctx, app.AddEntryOptions{
				Description: desc, Start: startAt, Stop: stopAt, ProjectID: project, Pomodoro: pomodoro,
			})
			if err != nil {
				return fail(err)
			}
			payload, err := app.MarshalEntryEnvelope(entry)
			if err != nil {
				return err
			}
			printJSONWith(cmd, *jsonOut, payload, fmt.Sprintf("added entry %s", entry.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&start, "start", "", "start instant (RFC3339, required)")
	cmd.Flags().StringVar(&stop, "stop", "", "stop instant (RFC3339, required)")
	cmd.Flags().StringVar(&project, "project", "", "assign the entry to a project")
	cmd.Flags().BoolVar(&pomodoro, "pomodoro", false, "mark the entry as a Pomodoro")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("stop")
	return cmd
}

func newEntriesEditCommand(sess *session, jsonOut *bool) *cobra.Command {
	var desc, project, start, stop string
	cmd := &cobra.Command{
		Use:   "edit [flags] <entry-id>",
		Short: "Edit an entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			opts := app.EditOptions{Description: desc}
			if project != "" {
				opts.ProjectID = &project
			}
			if start != "" {
				parsed, err := time.Parse(time.RFC3339, start)
				if err != nil {
					return fail(fmt.Errorf("%w: --start must be RFC3339", app.ErrValidation))
				}
				opts.Start = &parsed
			}
			if stop != "" {
				parsed, err := time.Parse(time.RFC3339, stop)
				if err != nil {
					return fail(fmt.Errorf("%w: --stop must be RFC3339", app.ErrValidation))
				}
				opts.Stop = &parsed
			}
			entry, err := sess.entries.Edit(ctx, args[0], opts)
			if err != nil {
				return fail(err)
			}
			payload, err := app.MarshalEntryEnvelope(entry)
			if err != nil {
				return err
			}
			printJSONWith(cmd, *jsonOut, payload, fmt.Sprintf("edited entry %s", entry.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&desc, "description", "", "new description")
	cmd.Flags().StringVar(&project, "project", "", "new project ID (empty string clears)")
	cmd.Flags().StringVar(&start, "start", "", "new start instant (RFC3339)")
	cmd.Flags().StringVar(&stop, "stop", "", "new stop instant (RFC3339)")
	return cmd
}

func newEntriesListCommand(sess *session, jsonOut *bool) *cobra.Command {
	var filters entryFilterFlags
	cmd := &cobra.Command{
		Use:   "list [flags]",
		Short: "List entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			entries, err := app.List(ctx, sess.store, filters.values())
			if err != nil {
				return fail(err)
			}
			if *jsonOut {
				dtos := make([]app.EntryDTO, len(entries))
				for i, result := range entries {
					dtos[i] = app.ToDTO(result.Entry)
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"entries": dtos})
			}
			for _, result := range entries {
				entry := result.Entry
				end := "active"
				if entry.StoppedAt != nil {
					end = entry.StoppedAt.UTC().Format(time.RFC3339)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s  %s  %s\n", entry.ID, entry.StartedAt.UTC().Format(time.RFC3339), end, result.ProjectName, entry.Description)
			}
			return nil
		},
	}
	filters.register(cmd)
	return cmd
}

func newReportCommand(sess *session, jsonOut *bool) *cobra.Command {
	var filters entryFilterFlags
	cmd := &cobra.Command{
		Use:   "report [flags]",
		Short: "Summarise completed time",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			report, err := app.Report(ctx, sess.store, filters.values())
			if err != nil {
				return fail(err)
			}
			formatted := app.FormatDuration(report.CompletedDuration)
			payload := app.ReportEnvelope{
				Count:                      report.Count,
				CompletedDurationSeconds:   report.CompletedDuration,
				CompletedDurationFormatted: formatted,
			}
			if *jsonOut {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d entries  %s completed\n", report.Count, formatted)
			return nil
		},
	}
	filters.register(cmd)
	return cmd
}

func newExportCommand(sess *session) *cobra.Command {
	var filters entryFilterFlags
	var output string
	cmd := &cobra.Command{
		Use:   "export csv [flags]",
		Short: "Export entries as CSV",
		Long:  "Export entries as CSV. Use --output - (default) for stdout, or a file path to write the file (confirmation goes to stderr).",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && args[0] == "csv" {
				return nil // tolerate the documented `export csv` form
			}
			if len(args) == 0 {
				return nil
			}
			return usagef("unknown format %q: export requires csv", args[0])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			if output == "-" {
				if err := app.ExportCSV(ctx, sess.store, filters.values(), cmd.OutOrStdout()); err != nil {
					return fail(err)
				}
				return nil
			}
			if output == "" {
				return usagef("export requires an output path or - for stdout")
			}
			if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
				return fail(fmt.Errorf("create export directory: %w", err))
			}
			out, err := os.Create(output)
			if err != nil {
				return fail(fmt.Errorf("create export file: %w", err))
			}
			defer out.Close()
			if err := app.ExportCSV(ctx, sess.store, filters.values(), out); err != nil {
				return fail(err)
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "exported CSV to %s\n", output)
			return nil
		},
	}
	filters.register(cmd)
	cmd.Flags().StringVar(&output, "output", "-", "output path, or - for stdout")
	return cmd
}

func newBackupCommand(sess *session) *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "backup [flags] <path>",
		Short: "Create a consistent database backup",
		Long:  "Create a consistent SQLite snapshot. The destination must not already exist.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			path := output
			if path == "" && len(args) == 1 {
				path = args[0]
			}
			if path == "" {
				return usagef("backup requires an output path")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				return fail(fmt.Errorf("create backup directory: %w", err))
			}
			if err := sess.store.Backup(ctx, path); err != nil {
				return fail(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "backed up database to %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&output, "output", "", "backup database path (alternative to the positional path)")
	return cmd
}

func newDoctorCommand(sess *session) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the local database",
		Long:  "Report the data directory, schema version, timezone, and database health.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if err := sess.services(ctx); err != nil {
				return fail(err)
			}
			report := store.Doctor(ctx, sess.store)
			out := map[string]any{
				"data_dir":       sess.dataPath,
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
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			if report.Err != nil {
				return &ExitError{Code: app.ExitStorage, msg: report.Err.Error()}
			}
			return nil
		},
	}
}

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// execCLI runs one command invocation in-process against a fresh temporary
// data directory, then returns the captured stdout/stderr and the error.
// The --data-dir flag exercises the same flag > env > default precedence
// path every real invocation uses.
func execCLI(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	args = append([]string{"--data-dir", dir}, args...)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func TestTableDataDirIsHonoured(t *testing.T) {
	// The --data-dir flag must reach the resolver before any command opens
	// the store (regression: services() built a fresh resolver and silently
	// wrote to the platform default directory instead).
	dir := t.TempDir()

	// init reports the resolved path and creates the database there.
	out, stderr, err := execCLI(t, dir, "init")
	if err != nil {
		t.Fatalf("init: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}
	if !strings.Contains(out, dir) {
		t.Errorf("init reported %q, want the --data-dir path %q", out, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "timetracker.db")); err != nil {
		t.Fatalf("init did not create the database under --data-dir: %v", err)
	}

	// doctor agrees with the resolved path.
	out, stderr, err = execCLI(t, dir, "doctor")
	if err != nil {
		t.Fatalf("doctor: %v\nstderr: %s", err, stderr)
	}
	var report struct {
		DataDir string `json:"data_dir"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("parse doctor output %q: %v", out, err)
	}
	if report.DataDir != dir {
		t.Errorf("doctor data_dir = %q, want %q", report.DataDir, dir)
	}

	// A data-mutating command lands in the same directory.
	out, stderr, err = execCLI(t, dir, "start", "table test")
	if err != nil {
		t.Fatalf("start: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "timetracker.db")); err != nil {
		t.Fatalf("start did not use --data-dir: %v", err)
	}
	if strings.Contains(out, "timetracker.db") && !strings.Contains(out, "started e") {
		t.Errorf("unexpected start output %q", out)
	}
}

func TestTableEntryJSONUsesCanonicalShape(t *testing.T) {
	dir := t.TempDir()
	if _, stderr, err := execCLI(t, dir, "init"); err != nil {
		t.Fatalf("init: %v (stderr %s)", err, stderr)
	}

	out, stderr, err := execCLI(t, dir, "--json", "start", "json shape test")
	if err != nil {
		t.Fatalf("start --json: %v (stderr %s)", err, stderr)
	}
	var envelope struct {
		Entry json.RawMessage `json:"entry"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("start JSON %q: %v", out, err)
	}

	// Canonical DTO keys, not the raw store shape.
	for _, key := range []string{"id", "description", "started_at", "pomodoro"} {
		if !bytes.Contains(envelope.Entry, []byte(`"`+key+`"`)) {
			t.Errorf("entry envelope missing %q: %s", key, envelope.Entry)
		}
	}

	// entries list --json emits the same DTO shape, RFC 3339 timestamps.
	out, stderr, err = execCLI(t, dir, "--json", "entries", "list")
	if err != nil {
		t.Fatalf("entries list: %v (stderr %s)", err, stderr)
	}
	var payload struct {
		Entries []struct {
			ID          string  `json:"id"`
			StartedAt   string  `json:"started_at"`
			StoppedAt   *string `json:"stopped_at"`
			Description string  `json:"description"`
		} `json:"entries"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("parse list JSON %q: %v", out, err)
	}
	if len(payload.Entries) != 1 {
		t.Fatalf("entries list returned %d entries, want 1", len(payload.Entries))
	}
	e := payload.Entries[0]
	if e.Description != "json shape test" {
		t.Errorf("description = %q", e.Description)
	}
	if _, err := time.Parse(time.RFC3339, e.StartedAt); err != nil {
		t.Errorf("started_at %q is not RFC 3339: %v", e.StartedAt, err)
	}

	// stop before reporting so the completed total is deterministic.
	out, stderr, err = execCLI(t, dir, "--json", "stop")
	if err != nil {
		t.Fatalf("stop: %v (stderr %s)", err, stderr)
	}
	var stopped struct {
		Entry struct {
			ID string `json:"id"`
		} `json:"entry"`
	}
	if err := json.Unmarshal([]byte(out), &stopped); err != nil {
		t.Fatalf("parse stop JSON %q: %v", out, err)
	}
	if stopped.Entry.ID == "" {
		t.Errorf("stop envelope missing entry id: %s", out)
	}

	// report --json keeps stable keys.
	out, stderr, err = execCLI(t, dir, "--json", "report")
	if err != nil {
		t.Fatalf("report: %v (stderr %s)", err, stderr)
	}
	var reportPayload struct {
		Count                      int    `json:"count"`
		CompletedDurationSeconds   int64  `json:"completed_duration_seconds"`
		CompletedDurationFormatted string `json:"completed_duration_formatted"`
	}
	if err := json.Unmarshal([]byte(out), &reportPayload); err != nil {
		t.Fatalf("parse report JSON %q: %v", out, err)
	}
	if reportPayload.Count != 1 {
		t.Errorf("report count = %d, want 1", reportPayload.Count)
	}
	// Same-second start/stop truncates to zero duration (RFC 3339 has no
	// sub-second precision); only the shape is asserted.
	if reportPayload.CompletedDurationSeconds < 0 {
		t.Errorf("completed duration = %d, want >= 0", reportPayload.CompletedDurationSeconds)
	}
}

func TestTableExitCodes(t *testing.T) {
	dir := t.TempDir()
	if _, stderr, err := execCLI(t, dir, "init"); err != nil {
		t.Fatalf("init: %v (stderr %s)", err, stderr)
	}

	// start twice: conflict (3)
	if out, stderr, err := execCLI(t, dir, "start", "first"); err != nil {
		t.Fatalf("first start: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}
	if _, _, err := execCLI(t, dir, "start", "second"); err == nil {
		t.Fatal("second start accepted")
	} else if got := ExitCode(err); got != 3 {
		t.Errorf("second start exit = %d, want 3 (conflict)", got)
	}

	// stop unknown entry: not found (4)
	if _, _, err := execCLI(t, dir, "stop", "--entry", "e-missing"); err == nil {
		t.Fatal("stop unknown entry accepted")
	} else if got := ExitCode(err); got != 4 {
		t.Errorf("stop unknown exit = %d, want 4", got)
	}

	// stop the active entry, then start again for the --at case.
	if _, _, err := execCLI(t, dir, "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, _, err := execCLI(t, dir, "start", "range test"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, _, err := execCLI(t, dir, "stop", "--at", "2000-01-01T00:00:00Z"); err == nil {
		t.Fatal("backdated --at accepted")
	} else if got := ExitCode(err); got != 2 {
		t.Errorf("backdated --at exit = %d, want 2", got)
	}
}

func TestTableStatusJSONNullWhenIdle(t *testing.T) {
	dir := t.TempDir()
	out, stderr, err := execCLI(t, dir, "--json", "status")
	if err != nil {
		t.Fatalf("status on fresh dir: %v (stderr %s)", err, stderr)
	}
	if got := bytes.TrimSpace([]byte(out)); string(got) != `{"active": null}` {
		t.Errorf("status JSON = %q, want {\"active\": null}", got)
	}
}

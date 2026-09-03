//go:build acceptance

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCLIHelp(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "timetracker")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = "."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}

	checks := map[string][]string{
		"--help":       {"Usage:", "Available Commands:", "--data-dir", "--json"},
		"help entries": {"Usage:", "timetracker entries", "list"},
		"help report":  {"Usage:", "timetracker report", "--status"},
		"help export":  {"Usage:", "timetracker export csv", "--output"},
		"help backup":  {"Usage:", "timetracker backup", "--output"},
	}
	for invocation, want := range checks {
		result := runCLI(t, binary, strings.Fields(invocation)...)
		for _, fragment := range want {
			if !strings.Contains(result.stdout, fragment) {
				t.Errorf("%s output does not contain %q:\n%s", invocation, fragment, result.stdout)
			}
		}
		if result.stderr != "" {
			t.Errorf("%s wrote diagnostics to stderr: %q", invocation, result.stderr)
		}
	}
}

func TestPomodoroCommandCompletesAtDeadline(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "timetracker")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = "."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}

	dataDir := t.TempDir()
	runCLI(t, binary, "--data-dir", dataDir, "init")

	startedAt := time.Now()
	result := runCLI(t, binary,
		"--data-dir", dataDir,
		"--json", "pomodoro", "start", "--minutes", "1", "real CLI acceptance",
	)
	if elapsed := time.Since(startedAt); elapsed < 50*time.Second {
		t.Fatalf("pomodoro command returned too early after %s; foreground runner was likely skipped", elapsed)
	}

	var payload struct {
		Entry struct {
			Description string  `json:"description"`
			StoppedAt   *string `json:"stopped_at"`
		} `json:"entry"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &payload); err != nil {
		t.Fatalf("parse completed JSON %q: %v", result.stdout, err)
	}
	if payload.Entry.Description != "real CLI acceptance" {
		t.Fatalf("completed description = %q", payload.Entry.Description)
	}
	if payload.Entry.StoppedAt == nil {
		t.Fatal("completed entry has no stopped_at")
	}

	status := runCLI(t, binary, "--data-dir", dataDir, "--json", "status")
	if strings.TrimSpace(status.stdout) != `{"active": null}` {
		t.Fatalf("status after deadline = %q, want no active entry", status.stdout)
	}
	if strings.Contains(result.stdout, "remaining") {
		t.Fatal("JSON output must not contain countdown frames")
	}
}

type cliResult struct {
	stdout string
	stderr string
}

func runCLI(t *testing.T, binary string, args ...string) cliResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run %s %v: %v\nstdout: %s\nstderr: %s", binary, args, err, stdout.String(), stderr.String())
	}
	return cliResult{stdout: stdout.String(), stderr: stderr.String()}
}

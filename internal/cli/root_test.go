package cli

import (
	"bytes"
	"strings"
	"testing"
)

// runRoot executes the command tree in-process and captures stdout/stderr.
func runRoot(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := NewRootCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

func TestRootPrintsHelpWhenBare(t *testing.T) {
	out, _, err := runRoot(t)
	if err != nil {
		t.Fatalf("bare invocation: %v", err)
	}
	for _, want := range []string{"Usage:", "Available Commands:", "start", "doctor"} {
		if !strings.Contains(out, want) {
			t.Errorf("bare help missing %q", want)
		}
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	_, _, err := runRoot(t, "definitely-not-a-command")
	if err == nil {
		t.Fatal("unknown command accepted")
	}
	if got := ExitCode(err); got != 64 {
		t.Errorf("unknown command exit = %d, want 64", got)
	}
}

func TestCompletionUsesConfiguredWriter(t *testing.T) {
	out, _, err := runRoot(t, "completion", "bash")
	if err != nil {
		t.Fatalf("completion bash: %v", err)
	}
	if !strings.Contains(out, "bash completion") {
		t.Errorf("captured completion output missing marker")
	}
}

func TestCommandErrorWriterUsesConfiguredStderr(t *testing.T) {
	root := NewRootCommand()
	var stderr bytes.Buffer
	root.SetErr(&stderr)

	writer := commandErrorWriter{command: root}
	if _, err := writer.Write([]byte("notification")); err != nil {
		t.Fatalf("write notification: %v", err)
	}
	if got := stderr.String(); got != "notification" {
		t.Errorf("captured stderr = %q, want notification", got)
	}
}

func TestExitCodeCategories(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Errorf("nil error exit = %d, want 0", got)
	}
	if got := ExitCode(errGeneric); got != 1 {
		t.Errorf("generic error exit = %d, want 1", got)
	}
	if got := ExitCode(&ExitError{Code: 3, msg: "conflict"}); got != 3 {
		t.Errorf("ExitError exit = %d, want 3", got)
	}
}

var errGeneric = &genericError{}

type genericError struct{}

func (e *genericError) Error() string { return "generic" }

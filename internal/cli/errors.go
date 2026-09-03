// Package cli wires the timetracker command tree on Cobra.
package cli

import (
	"errors"
	"fmt"

	"github.com/acramatte/timetracker-cli/internal/app"
)

// UsageError marks argument/flag misuse. Spec §6.3 exit 64.
type UsageError struct{ err error }

func (e *UsageError) Error() string { return e.err.Error() }

// ExitError carries a mapped exit code and stderr message through Execute().
type ExitError struct {
	Code int
	msg  string
}

func (e *ExitError) Error() string { return e.msg }

// fail translates a domain error into its documented exit code and message.
func fail(err error) error {
	code, msg := app.MapError(err)
	return &ExitError{Code: code, msg: msg}
}

// usagef builds a UsageError for argument-count and value misuse.
func usagef(format string, args ...any) error {
	return &UsageError{err: fmt.Errorf(format, args...)}
}

// ExitCode maps an error returned by Execute() to the process exit code.
func ExitCode(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	var usageErr *UsageError
	if errors.As(err, &usageErr) {
		return app.ExitUsage
	}
	if err != nil {
		return 1
	}
	return app.ExitOK
}

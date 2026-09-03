// Package app implements the CLI use cases (Phase 2, tasks C1–C11).
// Command handlers depend on this package, never on database/sql or
// internal/store directly — mirroring the repository boundary in ADR 0002.
package app

import (
	"errors"
	"fmt"

	"github.com/acramatte/timetracker-cli/internal/store"
)

// ErrValidation covers user-input failures (empty description, bad time
// range, bad flag values). Spec §6.3: distinct error categories per exit
// behavior.
var ErrValidation = errors.New("validation error")

// Exit codes per spec §6.3. Documented, stable categories.
const (
	ExitOK         = 0
	ExitValidation = 2
	ExitConflict   = 3
	ExitNotFound   = 4
	ExitStorage    = 5
	ExitUsage      = 64
)

// MapError translates a domain error to its exit code and message.
// Unknown errors are storage errors (exit 5) with a wrapped message.
func MapError(err error) (int, string) {
	switch {
	case err == nil:
		return ExitOK, ""
	case errors.Is(err, ErrValidation):
		return ExitValidation, err.Error()
	case errors.Is(err, store.ErrActiveEntryExists):
		return ExitConflict, err.Error()
	case errors.Is(err, store.ErrNoActiveEntry):
		return ExitValidation, err.Error()
	case errors.Is(err, store.ErrNotFound):
		return ExitNotFound, err.Error()
	default:
		return ExitStorage, fmt.Sprintf("storage error: %v", err)
	}
}

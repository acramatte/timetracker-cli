package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/acramatte/timetracker-cli/internal/store"
)

// IDPrefix keeps generated identifiers readable and domain-tagged.
const idPrefix = "e"

// newID generates a 128-bit random hex identifier (ADR-spec §7.2:
// application-generated, globally portable string IDs).
func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return idPrefix + hex.EncodeToString(buf), nil
}

// normalizeDescription trims and validates a description per spec §9.3.
func normalizeDescription(desc string) (string, error) {
	trimmed := strings.TrimSpace(desc)
	if trimmed == "" {
		return "", fmt.Errorf("%w: description is required", ErrValidation)
	}
	return trimmed, nil
}

// resolveProject validates an optional project reference (ID for now;
// name lookup arrives with project commands' display layer). Returns nil
// when no project was supplied.
func resolveProject(ctx context.Context, s *store.Store, projectID string) (*string, error) {
	if projectID == "" {
		return nil, nil
	}
	exists, err := s.ProjectExists(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("%w: project %q does not exist", store.ErrNotFound, projectID)
	}
	id := projectID
	return &id, nil
}

// validateRange enforces spec §9.2: a completed entry must not stop before
// it starts.
func validateRange(start, stop time.Time) error {
	if stop.Before(start) {
		return fmt.Errorf("%w: stop time %s is before start time %s",
			ErrValidation, stop.Format(time.RFC3339), start.Format(time.RFC3339))
	}
	return nil
}

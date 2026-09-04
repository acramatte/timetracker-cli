package app

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/acramatte/timetracker-cli/internal/store"
)

// ProjectDTO is the stable JSON resource shape for a project.
type ProjectDTO struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Color     *string `json:"color"`
	Archived  bool    `json:"archived"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// ProjectEnvelope is the project mutation response.
type ProjectEnvelope struct {
	Project ProjectDTO `json:"project"`
}

// ProjectsEnvelope is the project list response.
type ProjectsEnvelope struct {
	Projects []ProjectDTO `json:"projects"`
}

// ToProjectDTO maps a project to its wire shape.
func ToProjectDTO(project store.Project) ProjectDTO {
	return ProjectDTO{
		ID:        project.ID,
		Name:      project.Name,
		Color:     project.Color,
		Archived:  project.Archived,
		CreatedAt: project.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt: project.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// EntryDTO is the JSON resource shape for a time entry (spec §6.2: stable
// JSON envelopes). Field names follow the command contract examples.
type EntryDTO struct {
	ID                      string  `json:"id"`
	Description             string  `json:"description"`
	StartedAt               string  `json:"started_at"`
	StoppedAt               *string `json:"stopped_at"`
	ProjectID               *string `json:"project_id,omitempty"`
	Pomodoro                bool    `json:"pomodoro"`
	PomodoroDurationSeconds *int64  `json:"pomodoro_duration_seconds,omitempty"`
	PomodoroEndsAt          *string `json:"pomodoro_ends_at,omitempty"`
	DurationSeconds         *int64  `json:"duration_seconds,omitempty"`
}

// EntryEnvelope is the mutation response: {"entry": {...}}.
type EntryEnvelope struct {
	Entry EntryDTO `json:"entry"`
}

// ActiveEnvelope is the status response: {"active": {...}|null}.
type ActiveEnvelope struct {
	Active *EntryDTO `json:"active"`
}

// ToDTO maps a store entry to its wire shape. Times render as RFC 3339 UTC.
func ToDTO(e store.TimeEntry) EntryDTO {
	dto := EntryDTO{
		ID:                      e.ID,
		Description:             e.Description,
		StartedAt:               e.StartedAt.UTC().Format(time.RFC3339),
		ProjectID:               e.ProjectID,
		Pomodoro:                e.Pomodoro,
		PomodoroDurationSeconds: e.PomodoroDurationSeconds,
	}
	if e.StoppedAt != nil {
		s := e.StoppedAt.UTC().Format(time.RFC3339)
		dto.StoppedAt = &s
		d := int64(e.StoppedAt.Sub(e.StartedAt).Seconds())
		dto.DurationSeconds = &d
	}
	if e.PomodoroEndsAt != nil {
		s := e.PomodoroEndsAt.UTC().Format(time.RFC3339)
		dto.PomodoroEndsAt = &s
	}
	return dto
}

// MarshalEntryEnvelope encodes the mutation response as one JSON document.
func MarshalEntryEnvelope(e store.TimeEntry) (string, error) {
	b, err := json.Marshal(EntryEnvelope{Entry: ToDTO(e)})
	if err != nil {
		return "", fmt.Errorf("encode entry: %w", err)
	}
	return string(b), nil
}

// MarshalActive encodes the status response: a literal null active field
// when no entry is active.
func MarshalActive(e *store.TimeEntry) (string, error) {
	envelope := ActiveEnvelope{}
	if e != nil {
		dto := ToDTO(*e)
		envelope.Active = &dto
	}
	b, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("encode status: %w", err)
	}
	return string(b), nil
}

package app

import (
	"context"
	"fmt"
	"time"

	"github.com/acramatte/timetracker-cli/internal/store"
)

// ProjectsService implements project use cases (task C1).
type ProjectsService struct {
	Store *store.Store
	// Now is injectable for tests.
	Now func() time.Time
}

func (p *ProjectsService) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// AddProjectOptions carries validated project inputs.
type AddOptions struct {
	Name  string
	Color string
}

// Add creates a project.
func (p *ProjectsService) Add(ctx context.Context, opts AddOptions) (store.Project, error) {
	name, err := normalizeDescription(opts.Name) // same non-blank rule as descriptions
	if err != nil {
		return store.Project{}, fmt.Errorf("%w: project name is required", ErrValidation)
	}

	id, err := newID()
	if err != nil {
		return store.Project{}, err
	}

	now := p.now().UTC().Truncate(time.Second)
	var color *string
	if opts.Color != "" {
		color = &opts.Color
	}

	return p.Store.CreateProject(ctx, store.Project{
		ID: id, Name: name, Color: color, CreatedAt: now, UpdatedAt: now,
	})
}

// List returns projects, optionally including archived.
func (p *ProjectsService) List(ctx context.Context, includeArchived bool) ([]store.Project, error) {
	return p.Store.ListProjects(ctx, includeArchived)
}

// Archive marks a project archived (spec §9.5: history untouched).
func (p *ProjectsService) Archive(ctx context.Context, id string) error {
	return p.Store.ArchiveProject(ctx, id)
}

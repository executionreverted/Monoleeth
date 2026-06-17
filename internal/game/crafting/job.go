package crafting

import (
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/foundation"
)

// NewCraftJob returns a running craft job with the recipe source version and
// server-calculated completion timestamp stored on durable state.
func NewCraftJob(
	jobID CraftJobID,
	playerID foundation.PlayerID,
	recipe RecipeDefinition,
	location CraftLocation,
	startedAt time.Time,
) (CraftJob, error) {
	if err := recipe.Validate(); err != nil {
		return CraftJob{}, err
	}
	if err := recipe.ValidateLocationRequirement(location); err != nil {
		return CraftJob{}, err
	}

	job := CraftJob{
		JobID:        jobID,
		PlayerID:     playerID,
		RecipeSource: recipe.Source,
		Location:     location,
		State:        CraftJobStateRunning,
		StartedAt:    startedAt,
		CompletesAt:  startedAt.Add(recipe.CraftDuration),
	}
	if err := job.Validate(); err != nil {
		return CraftJob{}, err
	}
	return job, nil
}

// Validate reports whether id is non-blank.
func (id CraftJobID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrEmptyCraftJobID
	}
	return nil
}

// Validate reports whether state is supported by this model slice.
func (state CraftJobState) Validate() error {
	switch state {
	case CraftJobStateRunning, CraftJobStateCompleted:
		return nil
	default:
		return fmt.Errorf("craft job state %q: %w", state, ErrInvalidCraftJobState)
	}
}

// Validate reports whether job has valid durable identifiers, recipe source,
// location, lifecycle state, and timestamps.
func (job CraftJob) Validate() error {
	if err := job.JobID.Validate(); err != nil {
		return err
	}
	if err := job.PlayerID.Validate(); err != nil {
		return err
	}
	if err := job.RecipeSource.Validate(); err != nil {
		return err
	}
	if err := job.Location.Validate(); err != nil {
		return err
	}
	if err := job.State.Validate(); err != nil {
		return err
	}
	if job.StartedAt.IsZero() || job.CompletesAt.IsZero() {
		return ErrZeroCraftJobTime
	}
	if !job.CompletesAt.After(job.StartedAt) {
		return fmt.Errorf("completes_at %s started_at %s: %w", job.CompletesAt, job.StartedAt, ErrInvalidCraftJobTime)
	}
	if job.State == CraftJobStateCompleted {
		if job.CompletedAt == nil || job.CompletedAt.IsZero() {
			return fmt.Errorf("completed_at: %w", ErrZeroCraftJobTime)
		}
		if job.CompletedAt.Before(job.CompletesAt) {
			return fmt.Errorf("completed_at %s before completes_at %s: %w", *job.CompletedAt, job.CompletesAt, ErrInvalidCraftJobTime)
		}
	}
	return nil
}

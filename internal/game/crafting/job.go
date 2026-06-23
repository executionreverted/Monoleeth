package crafting

import (
	"fmt"
	"strings"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

// NewCraftJob returns a running craft job with the recipe source version and
// server-calculated completion timestamp stored on durable state.
func NewCraftJob(
	jobID CraftJobID,
	playerID foundation.PlayerID,
	recipe RecipeDefinition,
	reservationID economy.ReservationID,
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
		JobID:         jobID,
		PlayerID:      playerID,
		RecipeSource:  recipe.Source,
		ReservationID: reservationID,
		Location:      location,
		State:         CraftJobStateRunning,
		StartedAt:     startedAt,
		CompletesAt:   startedAt.Add(recipe.CraftDuration),
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
	case CraftJobStateRunning, CraftJobStateCompleted, CraftJobStateCancelled:
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
	if err := job.ReservationID.Validate(); err != nil {
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
		for label, value := range map[string]*time.Time{
			"reservation_committed_at": job.ReservationCommittedAt,
			"output_granted_at":        job.OutputGrantedAt,
			"xp_granted_at":            job.XPGrantedAt,
		} {
			if value == nil || value.IsZero() {
				return fmt.Errorf("%s: %w", label, ErrZeroCraftJobTime)
			}
			if value.Before(job.CompletesAt) {
				return fmt.Errorf("%s %s before completes_at %s: %w", label, *value, job.CompletesAt, ErrInvalidCraftJobTime)
			}
		}
	}
	if job.State == CraftJobStateCancelled {
		if job.CancelledAt == nil || job.CancelledAt.IsZero() {
			return fmt.Errorf("cancelled_at: %w", ErrZeroCraftJobTime)
		}
		if job.CancelledAt.Before(job.StartedAt) {
			return fmt.Errorf("cancelled_at %s before started_at %s: %w", *job.CancelledAt, job.StartedAt, ErrInvalidCraftJobTime)
		}
	}
	return nil
}

func cloneCraftJob(job CraftJob) CraftJob {
	cloned := job
	if job.ReservationCommittedAt != nil {
		reservationCommittedAt := *job.ReservationCommittedAt
		cloned.ReservationCommittedAt = &reservationCommittedAt
	}
	if job.OutputGrantedAt != nil {
		outputGrantedAt := *job.OutputGrantedAt
		cloned.OutputGrantedAt = &outputGrantedAt
	}
	if job.XPGrantedAt != nil {
		xpGrantedAt := *job.XPGrantedAt
		cloned.XPGrantedAt = &xpGrantedAt
	}
	if job.CompletedAt != nil {
		completedAt := *job.CompletedAt
		cloned.CompletedAt = &completedAt
	}
	if job.CancelledAt != nil {
		cancelledAt := *job.CancelledAt
		cloned.CancelledAt = &cancelledAt
	}
	return cloned
}

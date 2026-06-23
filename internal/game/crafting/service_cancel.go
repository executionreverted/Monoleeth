package crafting

import (
	"fmt"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

// CancelCraft releases a running job's reserved materials, refunds its craft
// fee, and removes it from the active job list once.
func (service *CraftingService) CancelCraft(input CancelCraftInput) (CancelCraftResult, error) {
	if service == nil {
		return CancelCraftResult{}, ErrMissingRecipeCatalog
	}
	if err := input.validate(); err != nil {
		return CancelCraftResult{}, err
	}

	service.mu.Lock()
	if previous, ok := service.cancellations[input.JobID]; ok {
		service.mu.Unlock()
		if previous.Job.PlayerID != input.PlayerID {
			return CancelCraftResult{}, fmt.Errorf("craft job %q player %q want %q: %w", input.JobID, input.PlayerID, previous.Job.PlayerID, ErrCraftJobPlayerMismatch)
		}
		result := cloneCancelCraftResult(previous)
		result.Duplicate = true
		return result, nil
	}
	if inFlight, ok := service.canceling[input.JobID]; ok {
		service.mu.Unlock()
		<-inFlight.done
		if inFlight.err != nil {
			return CancelCraftResult{}, inFlight.err
		}
		result := cloneCancelCraftResult(inFlight.result)
		result.Duplicate = true
		return result, nil
	}
	job, ok := service.jobs[input.JobID]
	if !ok {
		service.mu.Unlock()
		return CancelCraftResult{}, fmt.Errorf("craft job %q: %w", input.JobID, ErrCraftJobNotFound)
	}
	job = cloneCraftJob(job)
	if job.PlayerID != input.PlayerID {
		service.mu.Unlock()
		return CancelCraftResult{}, fmt.Errorf("craft job %q player %q want %q: %w", input.JobID, input.PlayerID, job.PlayerID, ErrCraftJobPlayerMismatch)
	}
	if job.State == CraftJobStateCompleted {
		service.mu.Unlock()
		return CancelCraftResult{}, fmt.Errorf("craft job %q state %q: %w", input.JobID, job.State, ErrInvalidCraftJobState)
	}
	if job.State == CraftJobStateCancelled {
		service.mu.Unlock()
		return CancelCraftResult{}, fmt.Errorf("craft job %q: %w", input.JobID, ErrCraftJobCancelled)
	}
	inFlight := &cancelInFlight{done: make(chan struct{})}
	service.canceling[input.JobID] = inFlight
	service.mu.Unlock()

	failCancel := func(err error) (CancelCraftResult, error) {
		service.mu.Lock()
		service.finishCancelInFlightLocked(input.JobID, CancelCraftResult{}, err)
		service.mu.Unlock()
		return CancelCraftResult{}, err
	}

	recipe, err := service.recipeForJob(job)
	if err != nil {
		return failCancel(err)
	}
	referenceKey, err := foundation.CraftCancelIdempotencyKey(job.JobID.String())
	if err != nil {
		return failCancel(err)
	}
	release, err := service.reservations.ReleaseReservation(job.ReservationID)
	if err != nil {
		return failCancel(err)
	}

	var refund *economy.CreditWalletResult
	if recipe.RequiredCredits.Int64() > 0 {
		credit, err := service.wallet.CreditWallet(economy.CreditWalletInput{
			PlayerID:     job.PlayerID,
			Currency:     economy.CurrencyBucketCredits,
			Amount:       recipe.RequiredCredits.Int64(),
			Reason:       craftCancelReason,
			ReferenceKey: referenceKey,
		})
		if err != nil {
			return failCancel(err)
		}
		cloned := credit
		refund = &cloned
	}

	cancelledAt := service.clock.Now()
	job.State = CraftJobStateCancelled
	job.CancelledAt = &cancelledAt
	if err := job.Validate(); err != nil {
		return failCancel(err)
	}
	result := CancelCraftResult{
		Job:                job,
		Recipe:             recipe,
		ReservationRelease: release,
		WalletRefund:       refund,
		ReferenceKey:       referenceKey,
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	service.jobs[input.JobID] = cloneCraftJob(job)
	service.cancellations[input.JobID] = cloneCancelCraftResult(result)
	service.finishCancelInFlightLocked(input.JobID, result, nil)
	emitter = service.emitter
	if emitter != nil {
		emitted = append(emitted, service.newEventLocked(EventCraftJobCancelled, jobCancelledEvent(result), cancelledAt))
	}
	service.mu.Unlock()

	emitEvents(emitter, emitted)
	return cloneCancelCraftResult(result), nil
}

func (service *CraftingService) finishCancelInFlightLocked(jobID CraftJobID, result CancelCraftResult, err error) {
	inFlight, ok := service.canceling[jobID]
	if !ok {
		return
	}
	if err == nil {
		inFlight.result = cloneCancelCraftResult(result)
	}
	inFlight.err = err
	delete(service.canceling, jobID)
	close(inFlight.done)
}

func jobCancelledEvent(result CancelCraftResult) JobCancelledEvent {
	cancelledAt := result.Job.CancelledAt
	event := JobCancelledEvent{
		JobID:    result.Job.JobID,
		PlayerID: result.Job.PlayerID,
		RecipeID: result.Recipe.RecipeID,
	}
	if cancelledAt != nil {
		event.CancelledAt = *cancelledAt
	}
	return event
}

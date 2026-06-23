package crafting

import (
	"encoding/json"
	"errors"
	"testing"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestCancelCraftReleasesReservedMaterialsOnceWithoutFeeRefund(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	recipe := fixture.startReadyRecipe(t, RecipeIDRefinedAlloy, "cancel-release")
	started := fixture.mustStartCraft(t, recipe.RecipeID)
	cancelKey := mustCraftCancelKey(t, started.Job.JobID)

	first, err := fixture.service.CancelCraft(CancelCraftInput{
		PlayerID:     fixture.playerID,
		JobID:        started.Job.JobID,
		ReferenceKey: cancelKey,
	})
	if err != nil {
		t.Fatalf("CancelCraft first: %v", err)
	}
	second, err := fixture.service.CancelCraft(CancelCraftInput{
		PlayerID:     fixture.playerID,
		JobID:        started.Job.JobID,
		ReferenceKey: cancelKey,
	})
	if err != nil {
		t.Fatalf("CancelCraft duplicate: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first CancelCraft Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate CancelCraft Duplicate = false, want true")
	}
	if first.Job.State != CraftJobStateCancelled || second.Job.State != CraftJobStateCancelled {
		t.Fatalf("cancelled job states = %q/%q, want cancelled", first.Job.State, second.Job.State)
	}
	if first.Job.CancelledAt == nil || first.Job.CancelledAt.IsZero() {
		t.Fatalf("cancelled_at = %+v, want server timestamp", first.Job.CancelledAt)
	}
	if first.ReservationRelease.Duplicate {
		t.Fatal("first reservation release Duplicate = true, want false")
	}
	if len(first.ReservationRelease.Moves) == 0 || len(second.ReservationRelease.Moves) != len(first.ReservationRelease.Moves) {
		t.Fatalf("duplicate reservation release moves = %d/%d, want cached canonical release", len(second.ReservationRelease.Moves), len(first.ReservationRelease.Moves))
	}
	if got := stackQuantityAt(fixture, economy.LocationKindAccountInventory, "iron_ore"); got != 20 {
		t.Fatalf("account iron_ore after cancel = %d, want released 20", got)
	}
	if got := stackQuantityAt(fixture, economy.LocationKindCraftingReserved, "iron_ore"); got != 0 {
		t.Fatalf("reserved iron_ore after cancel = %d, want 0", got)
	}
	if got := fixture.wallet.Balance(fixture.playerID, economy.CurrencyBucketCredits); got != 0 {
		t.Fatalf("wallet credits after cancel = %d, want no fee refund", got)
	}
}

func TestCancelCraftRejectsCompletedWrongOwnerAndReferenceMismatch(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	recipe := fixture.startReadyRecipe(t, RecipeIDRefinedAlloy, "cancel-reject")
	started := fixture.mustStartCraft(t, recipe.RecipeID)

	if _, err := fixture.service.CancelCraft(CancelCraftInput{
		PlayerID:     "player-other",
		JobID:        started.Job.JobID,
		ReferenceKey: mustCraftCancelKey(t, started.Job.JobID),
	}); !errors.Is(err, ErrCraftJobPlayerMismatch) {
		t.Fatalf("wrong owner CancelCraft error = %v, want ErrCraftJobPlayerMismatch", err)
	}

	cancelKey := mustCraftCancelKey(t, started.Job.JobID)
	if _, err := fixture.service.CancelCraft(CancelCraftInput{
		PlayerID:     fixture.playerID,
		JobID:        started.Job.JobID,
		ReferenceKey: cancelKey,
	}); err != nil {
		t.Fatalf("CancelCraft first: %v", err)
	}
	if _, err := fixture.service.CancelCraft(CancelCraftInput{
		PlayerID:     fixture.playerID,
		JobID:        started.Job.JobID,
		ReferenceKey: mustCraftCancelKey(t, "craft-job-other"),
	}); !errors.Is(err, ErrCraftCancelReferenceMismatch) {
		t.Fatalf("mismatched duplicate CancelCraft error = %v, want ErrCraftCancelReferenceMismatch", err)
	}
	if _, err := fixture.service.CompleteCraft(CompleteCraftInput{
		PlayerID: fixture.playerID,
		JobID:    started.Job.JobID,
	}); !errors.Is(err, ErrInvalidCraftJobState) {
		t.Fatalf("CompleteCraft after cancel error = %v, want ErrInvalidCraftJobState", err)
	}

	completedFixture := newCraftingServiceFixture(t)
	completedRecipe := completedFixture.startReadyRecipe(t, RecipeIDRefinedAlloy, "cancel-completed")
	completed := completedFixture.mustStartCraft(t, completedRecipe.RecipeID)
	completedFixture.clock.Advance(completedRecipe.CraftDuration)
	if _, err := completedFixture.service.CompleteCraft(CompleteCraftInput{
		PlayerID: completedFixture.playerID,
		JobID:    completed.Job.JobID,
	}); err != nil {
		t.Fatalf("CompleteCraft before cancel rejection: %v", err)
	}
	if _, err := completedFixture.service.CancelCraft(CancelCraftInput{
		PlayerID:     completedFixture.playerID,
		JobID:        completed.Job.JobID,
		ReferenceKey: mustCraftCancelKey(t, completed.Job.JobID),
	}); !errors.Is(err, ErrInvalidCraftJobState) {
		t.Fatalf("CancelCraft completed error = %v, want ErrInvalidCraftJobState", err)
	}
}

func TestCancelCraftEmitsJobCancelledEventOnce(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	recorder := testutil.NewEventRecorder()
	fixture.service.SetEventEmitter(recorder)
	recipe := fixture.startReadyRecipe(t, RecipeIDRefinedAlloy, "cancel-event")
	started := fixture.mustStartCraft(t, recipe.RecipeID)

	first, err := fixture.service.CancelCraft(CancelCraftInput{
		PlayerID:     fixture.playerID,
		JobID:        started.Job.JobID,
		ReferenceKey: mustCraftCancelKey(t, started.Job.JobID),
	})
	if err != nil {
		t.Fatalf("CancelCraft first: %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, EventCraftJobCancelled)
	var payload JobCancelledEvent
	if err := json.Unmarshal(recorder.Events()[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal craft cancelled payload: %v", err)
	}
	if payload.JobID != started.Job.JobID ||
		payload.PlayerID != fixture.playerID ||
		payload.RecipeID != recipe.RecipeID {
		t.Fatalf("craft cancelled payload = %+v, want cancelled job", payload)
	}
	if !payload.CancelledAt.Equal(*first.Job.CancelledAt) {
		t.Fatalf("payload cancelled_at = %s, want %s", payload.CancelledAt, *first.Job.CancelledAt)
	}

	recorder.Reset()
	if _, err := fixture.service.CancelCraft(CancelCraftInput{
		PlayerID:     fixture.playerID,
		JobID:        started.Job.JobID,
		ReferenceKey: mustCraftCancelKey(t, started.Job.JobID),
	}); err != nil {
		t.Fatalf("duplicate CancelCraft: %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder)
}

func stackQuantityAt(fixture *craftingServiceFixture, kind economy.LocationKind, itemID foundation.ItemID) int64 {
	var total int64
	for _, stack := range fixture.inventory.StackableItems() {
		if stack.OwnerPlayerID == fixture.playerID && stack.ItemID == itemID && stack.Location.Kind == kind {
			total += stack.Quantity.Int64()
		}
	}
	return total
}

func mustCraftCancelKey(t *testing.T, jobID CraftJobID) foundation.IdempotencyKey {
	t.Helper()
	key, err := CraftCancelReferenceKey(jobID)
	if err != nil {
		t.Fatalf("CraftCancelReferenceKey(%q): %v", jobID, err)
	}
	return key
}

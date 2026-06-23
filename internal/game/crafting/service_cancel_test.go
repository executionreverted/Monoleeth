package crafting

import (
	"encoding/json"
	"errors"
	"testing"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/testutil"
)

func TestCancelCraftReleasesReservationRefundsFeeAndIsIdempotent(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	recipe := fixture.startReadyRecipe(t, RecipeIDRefinedAlloy, "cancel")
	started := fixture.mustStartCraft(t, recipe.RecipeID)

	first, err := fixture.service.CancelCraft(CancelCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("first CancelCraft: %v", err)
	}
	second, err := fixture.service.CancelCraft(CancelCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("duplicate CancelCraft: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first CancelCraft Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate CancelCraft Duplicate = false, want true")
	}
	if first.Job.State != CraftJobStateCancelled || first.Job.CancelledAt == nil {
		t.Fatalf("cancelled job = %+v, want cancelled timestamp", first.Job)
	}
	if first.ReservationRelease.Reservation.State != economy.ReservationStateReleased {
		t.Fatalf("reservation state = %q, want released", first.ReservationRelease.Reservation.State)
	}
	if first.WalletRefund == nil || first.WalletRefund.Duplicate {
		t.Fatalf("wallet refund = %#v, want first refund", first.WalletRefund)
	}
	if second.WalletRefund == nil || second.WalletRefund.LedgerEntry.LedgerID != first.WalletRefund.LedgerEntry.LedgerID {
		t.Fatalf("duplicate wallet refund = %#v, want cached first refund ledger", second.WalletRefund)
	}
	for _, input := range recipe.Inputs {
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, fixture.sourceLocation); got != input.Quantity.Int64() {
			t.Fatalf("source quantity for %q after cancel = %d, want %d", input.ItemID, got, input.Quantity.Int64())
		}
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, fixture.reservedLocation(started.Job.JobID)); got != 0 {
			t.Fatalf("reserved quantity for %q after cancel = %d, want 0", input.ItemID, got)
		}
	}
	if got := fixture.wallet.Balance(fixture.playerID, economy.CurrencyBucketCredits); got != recipe.RequiredCredits.Int64() {
		t.Fatalf("wallet balance after cancel = %d, want refunded %d", got, recipe.RequiredCredits.Int64())
	}
	if _, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID}); !errors.Is(err, ErrCraftJobCancelled) {
		t.Fatalf("CompleteCraft after cancel error = %v, want ErrCraftJobCancelled", err)
	}
}

func TestCancelCraftEmitsJobCancelledEventOnce(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	recorder := testutil.NewEventRecorder()
	fixture.service.SetEventEmitter(recorder)
	recipe := fixture.startReadyRecipe(t, RecipeIDRefinedAlloy, "cancel-event")
	started := fixture.mustStartCraft(t, recipe.RecipeID)

	first, err := fixture.service.CancelCraft(CancelCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("first CancelCraft: %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, EventCraftJobCancelled)
	var payload JobCancelledEvent
	if err := json.Unmarshal(recorder.Events()[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal craft cancelled payload: %v", err)
	}
	if payload.JobID != started.Job.JobID || payload.PlayerID != fixture.playerID || payload.RecipeID != recipe.RecipeID {
		t.Fatalf("craft cancelled payload = %+v, want job/player/recipe", payload)
	}
	if first.Job.CancelledAt == nil || !payload.CancelledAt.Equal(*first.Job.CancelledAt) {
		t.Fatalf("payload cancelled_at = %s, want %v", payload.CancelledAt, first.Job.CancelledAt)
	}

	recorder.Reset()
	if _, err := fixture.service.CancelCraft(CancelCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID}); err != nil {
		t.Fatalf("duplicate CancelCraft: %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder)
}

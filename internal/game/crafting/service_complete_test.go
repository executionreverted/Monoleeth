package crafting

import (
	"encoding/json"
	"errors"
	"testing"

	"gameproject/internal/game/progression"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/testutil"
)

func TestCompleteCraftBeforeTimeFailsWithoutOutput(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	recipe := fixture.startReadyRecipe(t, RecipeIDRefinedAlloy, "complete-early")
	started := fixture.mustStartCraft(t, recipe.RecipeID)

	_, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if !errors.Is(err, ErrCraftNotReady) {
		t.Fatalf("CompleteCraft error = %v, want ErrCraftNotReady", err)
	}

	if got := fixture.inventory.TotalItemQuantity(fixture.playerID, recipe.Output.ItemID, fixture.sourceLocation); got != 0 {
		t.Fatalf("output quantity = %d, want 0", got)
	}
	if got := countCraftXPRecords(fixture.progressionStore, fixture.playerID); got != 0 {
		t.Fatalf("craft XP records = %d, want 0", got)
	}
	job, ok := fixture.service.Job(started.Job.JobID)
	if !ok {
		t.Fatal("job missing after early complete")
	}
	if job.State != CraftJobStateRunning {
		t.Fatalf("job state = %q, want %q", job.State, CraftJobStateRunning)
	}
}

func TestCompleteCraftAfterTimeCreatesItemOnceForDuplicateCompletion(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	recipe := fixture.startReadyRecipe(t, RecipeIDLaserAlphaT1, "complete-item")
	started := fixture.mustStartCraft(t, recipe.RecipeID)
	fixture.clock.Advance(recipe.CraftDuration)

	first, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("first CompleteCraft: %v", err)
	}
	second, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("duplicate CompleteCraft: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first CompleteCraft Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate CompleteCraft Duplicate = false, want true")
	}
	if first.ItemOutput == nil {
		t.Fatal("first ItemOutput is nil, want item output")
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.playerID, recipe.Output.ItemID, fixture.sourceLocation); got != recipe.Output.Quantity.Int64() {
		t.Fatalf("output quantity = %d, want %d", got, recipe.Output.Quantity.Int64())
	}
	if got := countCraftXPRecords(fixture.progressionStore, fixture.playerID); got != 1 {
		t.Fatalf("craft XP records = %d, want 1", got)
	}
	job, ok := fixture.service.Job(started.Job.JobID)
	if !ok {
		t.Fatal("job missing after complete")
	}
	if job.State != CraftJobStateCompleted {
		t.Fatalf("job state = %q, want %q", job.State, CraftJobStateCompleted)
	}
}

func TestCompleteCraftEmitsJobCompletedEventOnce(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	recorder := testutil.NewEventRecorder()
	fixture.service.SetEventEmitter(recorder)
	recipe := fixture.startReadyRecipe(t, RecipeIDLaserAlphaT1, "complete-event")
	started := fixture.mustStartCraft(t, recipe.RecipeID)
	fixture.clock.Advance(recipe.CraftDuration)

	first, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("first CompleteCraft: %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder, EventCraftJobCompleted)
	var payload JobCompletedEvent
	if err := json.Unmarshal(recorder.Events()[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal craft completed payload: %v", err)
	}
	if payload.JobID != started.Job.JobID ||
		payload.PlayerID != fixture.playerID ||
		payload.RecipeID != recipe.RecipeID ||
		payload.ItemID != recipe.Output.ItemID ||
		payload.Quantity != recipe.Output.Quantity.Int64() ||
		payload.OutputKind != RecipeOutputKindItem {
		t.Fatalf("craft completed payload = %+v, want completed item craft", payload)
	}
	if !payload.CompletedAt.Equal(*first.Job.CompletedAt) {
		t.Fatalf("payload completed_at = %s, want %s", payload.CompletedAt, *first.Job.CompletedAt)
	}

	recorder.Reset()
	if _, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID}); err != nil {
		t.Fatalf("duplicate CompleteCraft: %v", err)
	}
	testutil.AssertRecordedEventTypes(t, recorder)
}

func TestCompleteCraftTracksLowTierCraftXPOnceForBalancing(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	recipe := fixture.startReadyRecipe(t, RecipeIDRefinedAlloy, "track-low-tier-xp")
	started := fixture.mustStartCraft(t, recipe.RecipeID)
	fixture.clock.Advance(recipe.CraftDuration)

	first, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("first CompleteCraft: %v", err)
	}
	if _, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID}); err != nil {
		t.Fatalf("duplicate CompleteCraft: %v", err)
	}

	observations := waitForCraftXPObservations(t, fixture.xpTracker, 1)
	if got := len(observations); got != 1 {
		t.Fatalf("craft XP observations len = %d, want 1", got)
	}
	observation := observations[0]
	if observation.PlayerID != fixture.playerID {
		t.Fatalf("observation player = %q, want %q", observation.PlayerID, fixture.playerID)
	}
	if observation.JobID != started.Job.JobID {
		t.Fatalf("observation job = %q, want %q", observation.JobID, started.Job.JobID)
	}
	if observation.RecipeID != recipe.RecipeID {
		t.Fatalf("observation recipe = %q, want %q", observation.RecipeID, recipe.RecipeID)
	}
	if observation.RecipeSource != recipe.Source {
		t.Fatalf("observation recipe source = %#v, want %#v", observation.RecipeSource, recipe.Source)
	}
	if observation.Category != recipe.Category {
		t.Fatalf("observation category = %q, want %q", observation.Category, recipe.Category)
	}
	if observation.OutputKind != recipe.Output.Kind || observation.OutputItemID != recipe.Output.ItemID {
		t.Fatalf("observation output = %q/%q, want %q/%q", observation.OutputKind, observation.OutputItemID, recipe.Output.Kind, recipe.Output.ItemID)
	}
	if observation.RequiredRank != recipe.RequiredRank {
		t.Fatalf("observation required rank = %d, want %d", observation.RequiredRank, recipe.RequiredRank)
	}
	if observation.RequiredCredits != recipe.RequiredCredits.Int64() {
		t.Fatalf("observation required credits = %d, want %d", observation.RequiredCredits, recipe.RequiredCredits.Int64())
	}
	if observation.CraftDuration != recipe.CraftDuration {
		t.Fatalf("observation duration = %s, want %s", observation.CraftDuration, recipe.CraftDuration)
	}
	if observation.InputCount != len(recipe.Inputs) {
		t.Fatalf("observation input count = %d, want %d", observation.InputCount, len(recipe.Inputs))
	}
	if observation.InputQuantityTotal != 25 {
		t.Fatalf("observation input quantity total = %d, want 25", observation.InputQuantityTotal)
	}
	if observation.MainXP != craftXPMainAmount {
		t.Fatalf("observation main xp = %d, want %d", observation.MainXP, craftXPMainAmount)
	}
	if len(observation.RoleXP) != 1 || observation.RoleXP[0].Role != progression.RoleTypeCrafting || observation.RoleXP[0].Amount != craftXPRoleAmount {
		t.Fatalf("observation role xp = %#v, want crafting %d", observation.RoleXP, craftXPRoleAmount)
	}
	if !observation.LowTier {
		t.Fatal("observation LowTier = false, want true")
	}
	if observation.XPSourceID != progression.XPSourceID(started.Job.JobID.String()) {
		t.Fatalf("observation xp source id = %q, want %q", observation.XPSourceID, started.Job.JobID)
	}
	if observation.ReferenceKey != first.ReferenceKey {
		t.Fatalf("observation reference = %q, want %q", observation.ReferenceKey, first.ReferenceKey)
	}
	if first.Job.XPGrantedAt == nil {
		t.Fatal("first job XPGrantedAt is nil, want timestamp")
	}
	if !observation.GrantedAt.Equal(*first.Job.XPGrantedAt) {
		t.Fatalf("observation granted at = %s, want %s", observation.GrantedAt, *first.Job.XPGrantedAt)
	}
}

func TestCompleteCraftXPTrackerCannotBlockCompletionOrDuplicateRetry(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	tracker := newBlockingCraftXPTracker()
	defer tracker.Release()
	fixture.service.xpTracker = tracker
	recipe := fixture.startReadyRecipe(t, RecipeIDRefinedAlloy, "blocking-xp-tracker")
	started := fixture.mustStartCraft(t, recipe.RecipeID)
	fixture.clock.Advance(recipe.CraftDuration)

	first, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("first CompleteCraft: %v", err)
	}
	if first.Duplicate {
		t.Fatal("first CompleteCraft Duplicate = true, want false")
	}
	waitForSignal(t, tracker.entered, "blocking craft XP tracker")

	duplicate, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("duplicate CompleteCraft while tracker blocked: %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate CompleteCraft Duplicate = false, want true")
	}
	if got := countCraftXPRecords(fixture.progressionStore, fixture.playerID); got != 1 {
		t.Fatalf("craft XP records = %d, want 1", got)
	}

	tracker.Release()
	waitForSignal(t, tracker.done, "blocking craft XP tracker release")
}

func TestCompleteCraftXPTrackerPanicDoesNotBreakCompletionCache(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	tracker := newPanickingCraftXPTracker()
	fixture.service.xpTracker = tracker
	recipe := fixture.startReadyRecipe(t, RecipeIDRefinedAlloy, "panicking-xp-tracker")
	started := fixture.mustStartCraft(t, recipe.RecipeID)
	fixture.clock.Advance(recipe.CraftDuration)

	first, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("first CompleteCraft: %v", err)
	}
	waitForSignal(t, tracker.called, "panicking craft XP tracker")
	second, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("duplicate CompleteCraft after tracker panic: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first CompleteCraft Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate CompleteCraft Duplicate = false, want true")
	}
	if got := countCraftXPRecords(fixture.progressionStore, fixture.playerID); got != 1 {
		t.Fatalf("craft XP records = %d, want 1", got)
	}
}

func TestConcurrentCompleteCraftItemOutputUsesOneCanonicalResult(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	recipe := fixture.startReadyRecipe(t, RecipeIDLaserAlphaT1, "concurrent-item")
	started := fixture.mustStartCraft(t, recipe.RecipeID)
	fixture.clock.Advance(recipe.CraftDuration)

	blockingInventory := newBlockingInventoryService(fixture.inventory)
	fixture.service.inventory = blockingInventory

	results := completeTwiceWhileFirstOutputIsBlocked(t, fixture.service, fixture.playerID, started.Job.JobID, blockingInventory.entered, blockingInventory.release)

	assertOneCanonicalCompleteResult(t, results)
	if got := blockingInventory.calls.Load(); got != 1 {
		t.Fatalf("AddItem calls = %d, want 1", got)
	}
	if got := fixture.inventory.TotalItemQuantity(fixture.playerID, recipe.Output.ItemID, fixture.sourceLocation); got != recipe.Output.Quantity.Int64() {
		t.Fatalf("output quantity = %d, want %d", got, recipe.Output.Quantity.Int64())
	}
	if got := countCraftXPRecords(fixture.progressionStore, fixture.playerID); got != 1 {
		t.Fatalf("craft XP records = %d, want 1", got)
	}
	canonical := canonicalCompleteResult(t, results)
	if canonical.ReservationCommit.Duplicate {
		t.Fatal("canonical ReservationCommit Duplicate = true, want false")
	}
	if canonical.ItemOutput == nil {
		t.Fatal("canonical ItemOutput is nil, want item output")
	}
	if canonical.ItemOutput.Duplicate {
		t.Fatal("canonical ItemOutput Duplicate = true, want false")
	}
	if canonical.XPGrant.Duplicate {
		t.Fatal("canonical XPGrant Duplicate = true, want false")
	}
}

func TestCompleteCraftShipUnlockRecipeIsIdempotent(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedRank2(t)
	fixture.seedCraftingRole(t, 2)
	recipe := fixture.recipe(t, RecipeIDScoutT1)
	fixture.seedRecipeInputs(t, recipe, "ship-unlock")
	fixture.seedCredits(t, recipe.RequiredCredits.Int64(), "ship-unlock-credits")
	started := fixture.mustStartCraft(t, recipe.RecipeID)
	fixture.clock.Advance(recipe.CraftDuration)

	first, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("first CompleteCraft: %v", err)
	}
	second, err := fixture.service.CompleteCraft(CompleteCraftInput{PlayerID: fixture.playerID, JobID: started.Job.JobID})
	if err != nil {
		t.Fatalf("duplicate CompleteCraft: %v", err)
	}

	if first.ShipUnlock == nil || !first.ShipUnlock.Unlocked {
		t.Fatalf("first ship unlock = %#v, want unlocked result", first.ShipUnlock)
	}
	if !second.Duplicate {
		t.Fatal("duplicate CompleteCraft Duplicate = false, want true")
	}
	hangar, err := fixture.ships.GetHangar(fixture.playerID)
	if err != nil {
		t.Fatalf("GetHangar: %v", err)
	}
	if got := countShip(hangar, ships.ShipIDScoutT1); got != 1 {
		t.Fatalf("scout unlock count = %d, want 1", got)
	}
	if got := countCraftXPRecords(fixture.progressionStore, fixture.playerID); got != 1 {
		t.Fatalf("craft XP records = %d, want 1", got)
	}
}

func TestConcurrentCompleteCraftShipUnlockUsesOneCanonicalResult(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedRank2(t)
	fixture.seedCraftingRole(t, 2)
	recipe := fixture.recipe(t, RecipeIDScoutT1)
	fixture.seedRecipeInputs(t, recipe, "concurrent-ship-unlock")
	fixture.seedCredits(t, recipe.RequiredCredits.Int64(), "concurrent-ship-unlock-credits")
	started := fixture.mustStartCraft(t, recipe.RecipeID)
	fixture.clock.Advance(recipe.CraftDuration)

	blockingShips := newBlockingShipService(fixture.ships)
	fixture.service.ships = blockingShips

	results := completeTwiceWhileFirstOutputIsBlocked(t, fixture.service, fixture.playerID, started.Job.JobID, blockingShips.entered, blockingShips.release)

	assertOneCanonicalCompleteResult(t, results)
	if got := blockingShips.calls.Load(); got != 1 {
		t.Fatalf("UnlockShip calls = %d, want 1", got)
	}
	if got := countShip(mustHangar(t, fixture.ships, fixture.playerID), ships.ShipIDScoutT1); got != 1 {
		t.Fatalf("scout unlock count = %d, want 1", got)
	}
	if got := countCraftXPRecords(fixture.progressionStore, fixture.playerID); got != 1 {
		t.Fatalf("craft XP records = %d, want 1", got)
	}
	canonical := canonicalCompleteResult(t, results)
	if canonical.ReservationCommit.Duplicate {
		t.Fatal("canonical ReservationCommit Duplicate = true, want false")
	}
	if canonical.ShipUnlock == nil {
		t.Fatal("canonical ShipUnlock is nil, want ship unlock")
	}
	if canonical.ShipUnlock.Duplicate {
		t.Fatal("canonical ShipUnlock Duplicate = true, want false")
	}
	if canonical.XPGrant.Duplicate {
		t.Fatal("canonical XPGrant Duplicate = true, want false")
	}
}

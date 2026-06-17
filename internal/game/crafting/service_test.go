package crafting

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/testutil"
)

func TestStartCraftMissingMaterialFailsWithoutWalletDebitOrJob(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedCraftingRole(t, 1)
	fixture.seedCredits(t, 1_000, "missing-material-credits")

	_, err := fixture.service.StartCraft(fixture.startInput(RecipeIDRefinedAlloy))
	if !errors.Is(err, economy.ErrItemNotOwned) {
		t.Fatalf("StartCraft error = %v, want ErrItemNotOwned", err)
	}

	if got := fixture.wallet.Balance(fixture.playerID, economy.CurrencyBucketCredits); got != 1_000 {
		t.Fatalf("wallet balance = %d, want 1000", got)
	}
	if got := len(fixture.service.Jobs()); got != 0 {
		t.Fatalf("jobs len = %d, want 0", got)
	}
}

func TestStartCraftMissingCreditsReleasesReservationAndDoesNotCreateJob(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedCraftingRole(t, 1)
	recipe := fixture.recipe(t, RecipeIDRefinedAlloy)
	fixture.seedRecipeInputs(t, recipe, "missing-credits")

	_, err := fixture.service.StartCraft(fixture.startInput(recipe.RecipeID))
	if !errors.Is(err, economy.ErrInsufficientWalletFunds) {
		t.Fatalf("StartCraft error = %v, want ErrInsufficientWalletFunds", err)
	}

	for _, input := range recipe.Inputs {
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, fixture.sourceLocation); got != input.Quantity.Int64() {
			t.Fatalf("source quantity for %q = %d, want %d", input.ItemID, got, input.Quantity.Int64())
		}
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, fixture.reservedLocation("craft-job-1")); got != 0 {
			t.Fatalf("reserved quantity for %q = %d, want 0", input.ItemID, got)
		}
	}
	if got := fixture.wallet.Balance(fixture.playerID, economy.CurrencyBucketCredits); got != 0 {
		t.Fatalf("wallet balance = %d, want 0", got)
	}
	if got := len(fixture.service.Jobs()); got != 0 {
		t.Fatalf("jobs len = %d, want 0", got)
	}
}

func TestStartCraftRejectsRankRoleAndLocationGatesBeforeEconomyMutation(t *testing.T) {
	tests := []struct {
		name      string
		recipeID  catalog.DefinitionID
		prepare   func(*testing.T, *craftingServiceFixture)
		location  CraftLocation
		wantError error
	}{
		{
			name:     "rank too low",
			recipeID: RecipeIDScoutT1,
			prepare: func(t *testing.T, fixture *craftingServiceFixture) {
				fixture.seedCraftingRole(t, 2)
			},
			location:  stationCraftLocation(),
			wantError: ErrRankRequirementNotMet,
		},
		{
			name:      "role too low",
			recipeID:  RecipeIDScoutT1,
			prepare:   func(t *testing.T, fixture *craftingServiceFixture) { fixture.seedRank2(t) },
			location:  stationCraftLocation(),
			wantError: ErrRoleRequirementNotMet,
		},
		{
			name:     "wrong location",
			recipeID: RecipeIDRefinedAlloy,
			prepare: func(t *testing.T, fixture *craftingServiceFixture) {
				fixture.seedCraftingRole(t, 1)
			},
			location:  CraftLocation{Type: CraftLocationOwnedPlanet, ID: "planet-1"},
			wantError: ErrLocationRequirementNotMet,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCraftingServiceFixture(t)
			tc.prepare(t, fixture)

			_, err := fixture.service.StartCraft(StartCraftInput{
				PlayerID: fixture.playerID,
				RecipeID: tc.recipeID,
				Location: tc.location,
			})
			if !errors.Is(err, tc.wantError) {
				t.Fatalf("StartCraft error = %v, want %v", err, tc.wantError)
			}
			if got := len(fixture.service.Jobs()); got != 0 {
				t.Fatalf("jobs len = %d, want 0", got)
			}
			if got := fixture.wallet.Balance(fixture.playerID, economy.CurrencyBucketCredits); got != 0 {
				t.Fatalf("wallet balance = %d, want 0", got)
			}
		})
	}
}

func TestStartCraftReservesMaterialsAndDebitsFee(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedCraftingRole(t, 1)
	recipe := fixture.recipe(t, RecipeIDRefinedAlloy)
	fixture.seedRecipeInputs(t, recipe, "start-success")
	fixture.seedCredits(t, 1_000, "start-success-credits")

	result, err := fixture.service.StartCraft(fixture.startInput(recipe.RecipeID))
	if err != nil {
		t.Fatalf("StartCraft: %v", err)
	}

	if result.Job.State != CraftJobStateRunning {
		t.Fatalf("job state = %q, want %q", result.Job.State, CraftJobStateRunning)
	}
	if !result.Job.CompletesAt.Equal(fixture.clock.Now().Add(recipe.CraftDuration)) {
		t.Fatalf("job completes_at = %s, want %s", result.Job.CompletesAt, fixture.clock.Now().Add(recipe.CraftDuration))
	}
	for _, input := range recipe.Inputs {
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, fixture.sourceLocation); got != 0 {
			t.Fatalf("source quantity for %q = %d, want 0", input.ItemID, got)
		}
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, fixture.reservedLocation(result.Job.JobID)); got != input.Quantity.Int64() {
			t.Fatalf("reserved quantity for %q = %d, want %d", input.ItemID, got, input.Quantity.Int64())
		}
	}
	if got, want := fixture.wallet.Balance(fixture.playerID, economy.CurrencyBucketCredits), int64(1_000)-recipe.RequiredCredits.Int64(); got != want {
		t.Fatalf("wallet balance = %d, want %d", got, want)
	}
	if got := len(fixture.service.Jobs()); got != 1 {
		t.Fatalf("jobs len = %d, want 1", got)
	}
}

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

type craftingServiceFixture struct {
	clock            *testutil.FakeClock
	playerID         foundation.PlayerID
	location         CraftLocation
	sourceLocation   economy.ItemLocation
	itemDefinitions  ItemDefinitionMap
	inventory        *economy.InventoryService
	reservations     *economy.ReservationService
	wallet           *economy.WalletService
	progression      *progression.ProgressionService
	progressionStore *progression.InMemoryProgressionStore
	ships            *ships.ShipService
	service          *CraftingService
}

func newCraftingServiceFixture(t *testing.T) *craftingServiceFixture {
	t.Helper()

	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 15, 0, 0, 0, time.UTC))
	playerID := foundation.PlayerID("player-1")
	sourceLocation, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		t.Fatalf("NewItemLocation: %v", err)
	}

	inventory := economy.NewInventoryService(clock)
	reservations := economy.NewReservationService(inventory)
	wallet := economy.NewWalletService(clock)
	progressionStore := progression.NewInMemoryProgressionStore()
	progressionService := progression.NewProgressionService(clock, progressionStore)
	shipCatalog, err := ships.MVPShipCatalog()
	if err != nil {
		t.Fatalf("MVPShipCatalog: %v", err)
	}
	shipService, err := ships.NewShipService(
		shipCatalog,
		nil,
		ships.StaticPlayerRankProvider{playerID: 2},
		ships.BaseShipCargoCapacityProvider{},
		clock,
	)
	if err != nil {
		t.Fatalf("NewShipService: %v", err)
	}

	service, err := NewCraftingService(CraftingServiceConfig{
		Clock:           clock,
		Recipes:         MustMVPRecipeCatalog(),
		ItemDefinitions: testCraftItemDefinitions(t),
		Reservations:    reservations,
		Inventory:       inventory,
		Wallet:          wallet,
		Progression:     progressionService,
		Ships:           shipService,
	})
	if err != nil {
		t.Fatalf("NewCraftingService: %v", err)
	}

	return &craftingServiceFixture{
		clock:            clock,
		playerID:         playerID,
		location:         stationCraftLocation(),
		sourceLocation:   sourceLocation,
		itemDefinitions:  testCraftItemDefinitions(t),
		inventory:        inventory,
		reservations:     reservations,
		wallet:           wallet,
		progression:      progressionService,
		progressionStore: progressionStore,
		ships:            shipService,
		service:          service,
	}
}

func (fixture *craftingServiceFixture) startInput(recipeID catalog.DefinitionID) StartCraftInput {
	return StartCraftInput{
		PlayerID: fixture.playerID,
		RecipeID: recipeID,
		Location: fixture.location,
	}
}

func (fixture *craftingServiceFixture) recipe(t *testing.T, recipeID catalog.DefinitionID) RecipeDefinition {
	t.Helper()

	recipe, err := fixture.service.recipes.MustGet(recipeID)
	if err != nil {
		t.Fatalf("MustGet(%q): %v", recipeID, err)
	}
	return recipe
}

func (fixture *craftingServiceFixture) startReadyRecipe(t *testing.T, recipeID catalog.DefinitionID, suffix string) RecipeDefinition {
	t.Helper()

	fixture.seedCraftingRole(t, 1)
	recipe := fixture.recipe(t, recipeID)
	fixture.seedRecipeInputs(t, recipe, suffix)
	fixture.seedCredits(t, recipe.RequiredCredits.Int64(), suffix+"-credits")
	return recipe
}

func (fixture *craftingServiceFixture) mustStartCraft(t *testing.T, recipeID catalog.DefinitionID) StartCraftResult {
	t.Helper()

	result, err := fixture.service.StartCraft(fixture.startInput(recipeID))
	if err != nil {
		t.Fatalf("StartCraft(%q): %v", recipeID, err)
	}
	return result
}

func (fixture *craftingServiceFixture) seedRecipeInputs(t *testing.T, recipe RecipeDefinition, suffix string) {
	t.Helper()

	for index, input := range recipe.Inputs {
		fixture.seedItem(t, input.ItemID, input.Quantity.Int64(), fmt.Sprintf("%s-%d", suffix, index))
	}
}

func (fixture *craftingServiceFixture) seedItem(t *testing.T, itemID foundation.ItemID, quantity int64, suffix string) {
	t.Helper()

	definition, ok := fixture.itemDefinitions.ItemDefinition(itemID)
	if !ok {
		t.Fatalf("missing item definition for %q", itemID)
	}
	reference, err := foundation.LootPickupIdempotencyKey("seed-" + suffix)
	if err != nil {
		t.Fatalf("LootPickupIdempotencyKey: %v", err)
	}
	if _, err := fixture.inventory.AddItem(economy.AddItemInput{
		PlayerID:       fixture.playerID,
		ItemDefinition: definition,
		Quantity:       quantity,
		Location:       fixture.sourceLocation,
		Reason:         economy.LedgerReason("test_seed"),
		ReferenceKey:   reference,
	}); err != nil {
		t.Fatalf("seed AddItem(%q): %v", itemID, err)
	}
}

func (fixture *craftingServiceFixture) seedCredits(t *testing.T, amount int64, suffix string) {
	t.Helper()

	reference, err := foundation.QuestRewardIdempotencyKey(foundation.QuestID("seed-" + suffix))
	if err != nil {
		t.Fatalf("QuestRewardIdempotencyKey: %v", err)
	}
	if _, err := fixture.wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     fixture.playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       amount,
		Reason:       economy.LedgerReason("test_seed"),
		ReferenceKey: reference,
	}); err != nil {
		t.Fatalf("seed CreditWallet: %v", err)
	}
}

func (fixture *craftingServiceFixture) seedCraftingRole(t *testing.T, level int) {
	t.Helper()

	xpByLevel := map[int]int64{1: 1, 2: 75}
	xp, ok := xpByLevel[level]
	if !ok {
		t.Fatalf("unsupported test crafting level %d", level)
	}
	_, err := fixture.progression.GrantXP(progression.GrantXPInput{
		PlayerID:       fixture.playerID,
		Amount:         0,
		SourceType:     progression.XPSourceTypeAdminAdjustment,
		SourceID:       progression.XPSourceID(fmt.Sprintf("seed-crafting-role-%d", level)),
		IdempotencyKey: progression.XPIdempotencyKey(fmt.Sprintf("seed-crafting-role-%d", level)),
		RoleXP: []progression.RoleXPGrant{
			{Role: progression.RoleTypeCrafting, Amount: xp},
		},
	})
	if err != nil {
		t.Fatalf("seed crafting role: %v", err)
	}
}

func (fixture *craftingServiceFixture) seedRank2(t *testing.T) {
	t.Helper()

	_, err := fixture.progression.GrantXP(progression.GrantXPInput{
		PlayerID:       fixture.playerID,
		Amount:         100,
		SourceType:     progression.XPSourceTypeAdminAdjustment,
		SourceID:       progression.XPSourceID("seed-main-rank-2"),
		IdempotencyKey: progression.XPIdempotencyKey("seed-main-rank-2"),
	})
	if err != nil {
		t.Fatalf("seed main xp: %v", err)
	}
	_, err = fixture.progression.TryRankUp(progression.TryRankUpInput{
		PlayerID:       fixture.playerID,
		TargetRank:     2,
		Reason:         "test_seed",
		IdempotencyKey: progression.XPIdempotencyKey("seed-rank-2"),
	})
	if err != nil {
		t.Fatalf("seed rank 2: %v", err)
	}
}

func (fixture *craftingServiceFixture) reservedLocation(jobID CraftJobID) economy.ItemLocation {
	location, err := economy.NewItemLocation(economy.LocationKindCraftingReserved, jobID.String())
	if err != nil {
		panic(err)
	}
	return location
}

func stationCraftLocation() CraftLocation {
	return CraftLocation{Type: CraftLocationStation, ID: "origin-station"}
}

func testCraftItemDefinitions(t *testing.T) ItemDefinitionMap {
	t.Helper()

	definitions := ItemDefinitionMap{
		"iron_ore":        testItemDefinition(t, "iron_ore", "Iron Ore", economy.ItemTypeStackable, 1, 100),
		"carbon_shards":   testItemDefinition(t, "carbon_shards", "Carbon Shards", economy.ItemTypeStackable, 1, 100),
		"refined_alloy":   testItemDefinition(t, "refined_alloy", "Refined Alloy", economy.ItemTypeStackable, 1, 100),
		"laser_lens":      testItemDefinition(t, "laser_lens", "Laser Lens", economy.ItemTypeStackable, 1, 100),
		"energy_cell":     testItemDefinition(t, "energy_cell", "Energy Cell", economy.ItemTypeStackable, 1, 100),
		"scanner_circuit": testItemDefinition(t, "scanner_circuit", "Scanner Circuit", economy.ItemTypeStackable, 1, 100),
		"warp_coil":       testItemDefinition(t, "warp_coil", "Warp Coil", economy.ItemTypeStackable, 1, 100),
		"laser_alpha_t1":  testItemDefinition(t, "laser_alpha_t1", "Laser Alpha T1", economy.ItemTypeInstance, 1, 1),
	}
	return definitions
}

func testItemDefinition(
	t *testing.T,
	itemID foundation.ItemID,
	name string,
	itemType economy.ItemType,
	weightUnits int64,
	maxStack int64,
) economy.ItemDefinition {
	t.Helper()

	source, err := catalog.NewVersionedDefinitionFromStrings(itemID.String(), "item_catalog_test_v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings(%q): %v", itemID, err)
	}
	weight, err := foundation.NewQuantity(weightUnits)
	if err != nil {
		t.Fatalf("NewQuantity(weight): %v", err)
	}
	stack, err := foundation.NewQuantity(maxStack)
	if err != nil {
		t.Fatalf("NewQuantity(maxStack): %v", err)
	}
	definition, err := economy.NewItemDefinition(
		source,
		itemID,
		name,
		itemType,
		economy.ItemRarityCommon,
		stack,
		weight,
		[]economy.TradeFlag{economy.TradeFlagTradeable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition(%q): %v", itemID, err)
	}
	return definition
}

func countCraftXPRecords(store *progression.InMemoryProgressionStore, playerID foundation.PlayerID) int {
	count := 0
	for _, record := range store.XPGrantRecords(playerID) {
		if record.SourceType == progression.XPSourceTypeCraft {
			count++
		}
	}
	return count
}

func countShip(snapshot ships.HangarSnapshot, shipID foundation.ShipID) int {
	count := 0
	for _, playerShip := range snapshot.Ships {
		if playerShip.ShipID == shipID {
			count++
		}
	}
	return count
}

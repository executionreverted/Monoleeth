package crafting

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/testutil"
)

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
	xpTracker        *InMemoryCraftXPTracker
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
	xpTracker := NewInMemoryCraftXPTracker()
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
		XPTracker:       xpTracker,
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
		xpTracker:        xpTracker,
		service:          service,
	}
}

func (fixture *craftingServiceFixture) startInput(recipeID catalog.DefinitionID) StartCraftInput {
	return fixture.startInputWithReference(recipeID, "start-"+recipeID.String())
}

func (fixture *craftingServiceFixture) startInputWithReference(recipeID catalog.DefinitionID, reference string) StartCraftInput {
	return StartCraftInput{
		PlayerID:     fixture.playerID,
		RecipeID:     recipeID,
		Location:     fixture.location,
		ReferenceKey: mustCraftStartKey(nil, reference),
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

func (fixture *craftingServiceFixture) replaceRecipeLocationRequirement(
	t *testing.T,
	locationType CraftLocationType,
) RecipeDefinition {
	t.Helper()

	recipe := fixture.recipe(t, RecipeIDRefinedAlloy)
	recipe.RecipeID = catalog.DefinitionID("test_" + locationType.String() + "_recipe")
	source, err := catalog.NewRecipeSource(recipe.RecipeID.String(), RecipeCatalogVersion.String())
	if err != nil {
		t.Fatalf("NewRecipeSource: %v", err)
	}
	recipe.Source = source
	recipe.RequiredLocationType = locationType
	recipes, err := NewRecipeCatalog([]RecipeDefinition{recipe})
	if err != nil {
		t.Fatalf("NewRecipeCatalog: %v", err)
	}
	fixture.service.recipes = recipes
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

	fixture.seedRecipeInputsFor(t, fixture.playerID, fixture.sourceLocation, recipe, suffix)
}

func (fixture *craftingServiceFixture) seedRecipeInputsFor(t *testing.T, playerID foundation.PlayerID, location economy.ItemLocation, recipe RecipeDefinition, suffix string) {
	t.Helper()

	for index, input := range recipe.Inputs {
		fixture.seedItemFor(t, playerID, location, input.ItemID, input.Quantity.Int64(), fmt.Sprintf("%s-%d", suffix, index))
	}
}

func (fixture *craftingServiceFixture) seedItem(t *testing.T, itemID foundation.ItemID, quantity int64, suffix string) {
	t.Helper()

	fixture.seedItemFor(t, fixture.playerID, fixture.sourceLocation, itemID, quantity, suffix)
}

func (fixture *craftingServiceFixture) seedItemFor(t *testing.T, playerID foundation.PlayerID, location economy.ItemLocation, itemID foundation.ItemID, quantity int64, suffix string) {
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
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       quantity,
		Location:       location,
		Reason:         economy.LedgerReason("test_seed"),
		ReferenceKey:   reference,
	}); err != nil {
		t.Fatalf("seed AddItem(%q): %v", itemID, err)
	}
}

func (fixture *craftingServiceFixture) seedCredits(t *testing.T, amount int64, suffix string) {
	t.Helper()

	fixture.seedCreditsFor(t, fixture.playerID, amount, suffix)
}

func (fixture *craftingServiceFixture) seedCreditsFor(t *testing.T, playerID foundation.PlayerID, amount int64, suffix string) {
	t.Helper()

	reference, err := foundation.QuestRewardIdempotencyKey(foundation.QuestID("seed-" + suffix))
	if err != nil {
		t.Fatalf("QuestRewardIdempotencyKey: %v", err)
	}
	if _, err := fixture.wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       amount,
		Reason:       economy.LedgerReason("test_seed"),
		ReferenceKey: reference,
	}); err != nil {
		t.Fatalf("seed CreditWallet: %v", err)
	}
}

func mustCraftStartKey(t *testing.T, reference string) foundation.IdempotencyKey {
	if t != nil {
		t.Helper()
	}
	key, err := foundation.CraftStartIdempotencyKey(reference)
	if err != nil {
		if t == nil {
			panic(err)
		}
		t.Fatalf("CraftStartIdempotencyKey(%q): %v", reference, err)
	}
	return key
}

type startCraftCallResult struct {
	result StartCraftResult
	err    error
}

type blockingStartReservationService struct {
	delegate     *economy.ReservationService
	entered      chan struct{}
	release      chan struct{}
	reserveCalls atomic.Int64
}

func newBlockingStartReservationService(delegate *economy.ReservationService) *blockingStartReservationService {
	return &blockingStartReservationService{
		delegate: delegate,
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (service *blockingStartReservationService) ReserveItems(input economy.ReserveItemsInput) (economy.ReserveItemsResult, error) {
	if service.reserveCalls.Add(1) == 1 {
		close(service.entered)
		<-service.release
	}
	return service.delegate.ReserveItems(input)
}

func (service *blockingStartReservationService) ReleaseReservation(reservationID economy.ReservationID) (economy.ReleaseReservationResult, error) {
	return service.delegate.ReleaseReservation(reservationID)
}

func (service *blockingStartReservationService) CommitReservation(reservationID economy.ReservationID) (economy.CommitReservationResult, error) {
	return service.delegate.CommitReservation(reservationID)
}

func receiveStartCraftCallResult(t *testing.T, ch <-chan startCraftCallResult) startCraftCallResult {
	t.Helper()

	select {
	case result := <-ch:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for StartCraft call")
		return startCraftCallResult{}
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
		Authority:      progression.XPGrantAuthorityAdminService,
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
		Authority:      progression.XPGrantAuthorityAdminService,
	})
	if err != nil {
		t.Fatalf("seed main xp: %v", err)
	}
	_, err = fixture.progression.GrantXP(progression.GrantXPInput{
		PlayerID:       fixture.playerID,
		Amount:         1,
		SourceType:     progression.XPSourceTypeQuest,
		SourceID:       progression.XPSourceID("seed-rank-2-quest"),
		IdempotencyKey: progression.XPIdempotencyKey("quest_reward:seed-rank-2-quest"),
		Authority:      progression.XPGrantAuthorityQuestService,
	})
	if err != nil {
		t.Fatalf("seed quest milestone: %v", err)
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
		if record.SourceType == progression.XPSourceTypeCraft && record.Authority == progression.XPGrantAuthorityCraftingService {
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

func assertNoStartCraftEconomyMutation(
	t *testing.T,
	fixture *craftingServiceFixture,
	recipe RecipeDefinition,
	reservedJobID string,
	walletLedgerCount int,
	itemLedgerCount int,
) {
	t.Helper()

	for _, input := range recipe.Inputs {
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, fixture.sourceLocation); got != input.Quantity.Int64() {
			t.Fatalf("source quantity for %q = %d, want %d", input.ItemID, got, input.Quantity.Int64())
		}
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, fixture.reservedLocation(CraftJobID(reservedJobID))); got != 0 {
			t.Fatalf("reserved quantity for %q = %d, want 0", input.ItemID, got)
		}
	}
	if got := fixture.wallet.Balance(fixture.playerID, economy.CurrencyBucketCredits); got != recipe.RequiredCredits.Int64() {
		t.Fatalf("wallet balance = %d, want %d", got, recipe.RequiredCredits.Int64())
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != walletLedgerCount {
		t.Fatalf("wallet ledger entries = %d, want %d", got, walletLedgerCount)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != itemLedgerCount {
		t.Fatalf("item ledger entries = %d, want %d", got, itemLedgerCount)
	}
	if got := len(fixture.service.Jobs()); got != 0 {
		t.Fatalf("jobs len = %d, want 0", got)
	}
}

func completeTwiceWhileFirstOutputIsBlocked(
	t *testing.T,
	service *CraftingService,
	playerID foundation.PlayerID,
	jobID CraftJobID,
	entered <-chan struct{},
	release chan struct{},
) []CompleteCraftResult {
	t.Helper()

	results := make([]CompleteCraftResult, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[0], errs[0] = service.CompleteCraft(CompleteCraftInput{PlayerID: playerID, JobID: jobID})
	}()
	waitForSignal(t, entered, "first output mutation")

	wg.Add(1)
	go func() {
		defer wg.Done()
		results[1], errs[1] = service.CompleteCraft(CompleteCraftInput{PlayerID: playerID, JobID: jobID})
	}()
	close(release)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	waitForSignal(t, done, "concurrent completions")

	for index, err := range errs {
		if err != nil {
			t.Fatalf("CompleteCraft[%d]: %v", index, err)
		}
	}
	return results
}

func waitForSignal(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func waitForCraftXPObservations(t *testing.T, tracker *InMemoryCraftXPTracker, want int) []CraftXPObservation {
	t.Helper()

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		observations := tracker.Observations()
		if len(observations) >= want {
			return observations
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for %d craft XP observations; got %d", want, len(observations))
		case <-ticker.C:
		}
	}
}

func assertOneCanonicalCompleteResult(t *testing.T, results []CompleteCraftResult) {
	t.Helper()

	canonicalCount := 0
	duplicateCount := 0
	var canonical CompleteCraftResult
	for _, result := range results {
		if result.Duplicate {
			duplicateCount++
			continue
		}
		canonicalCount++
		canonical = result
	}
	if canonicalCount != 1 {
		t.Fatalf("canonical result count = %d, want 1; results = %#v", canonicalCount, results)
	}
	if duplicateCount != len(results)-1 {
		t.Fatalf("duplicate result count = %d, want %d", duplicateCount, len(results)-1)
	}
	for index, result := range results {
		if result.Job.JobID != canonical.Job.JobID {
			t.Fatalf("result[%d] job id = %q, want %q", index, result.Job.JobID, canonical.Job.JobID)
		}
		if result.ReferenceKey != canonical.ReferenceKey {
			t.Fatalf("result[%d] reference = %q, want %q", index, result.ReferenceKey, canonical.ReferenceKey)
		}
	}
}

func canonicalCompleteResult(t *testing.T, results []CompleteCraftResult) CompleteCraftResult {
	t.Helper()

	for _, result := range results {
		if !result.Duplicate {
			return result
		}
	}
	t.Fatal("canonical result not found")
	return CompleteCraftResult{}
}

func mustHangar(t *testing.T, service ShipService, playerID foundation.PlayerID) ships.HangarSnapshot {
	t.Helper()

	hangar, err := service.GetHangar(playerID)
	if err != nil {
		t.Fatalf("GetHangar: %v", err)
	}
	return hangar
}

type recordingCraftLocationAuthorizer struct {
	called bool
	input  CraftLocationAuthorizationInput
	err    error
}

func (authorizer *recordingCraftLocationAuthorizer) AuthorizeCraftLocation(input CraftLocationAuthorizationInput) error {
	authorizer.called = true
	authorizer.input = input
	return authorizer.err
}

type blockingCraftXPTracker struct {
	entered     chan struct{}
	release     chan struct{}
	done        chan struct{}
	enterOnce   sync.Once
	releaseOnce sync.Once
	doneOnce    sync.Once
}

func newBlockingCraftXPTracker() *blockingCraftXPTracker {
	return &blockingCraftXPTracker{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (tracker *blockingCraftXPTracker) TrackCraftXP(CraftXPObservation) {
	tracker.enterOnce.Do(func() { close(tracker.entered) })
	<-tracker.release
	tracker.doneOnce.Do(func() { close(tracker.done) })
}

func (tracker *blockingCraftXPTracker) Release() {
	tracker.releaseOnce.Do(func() { close(tracker.release) })
}

type panickingCraftXPTracker struct {
	called chan struct{}
	once   sync.Once
}

func newPanickingCraftXPTracker() *panickingCraftXPTracker {
	return &panickingCraftXPTracker{called: make(chan struct{})}
}

func (tracker *panickingCraftXPTracker) TrackCraftXP(CraftXPObservation) {
	tracker.once.Do(func() { close(tracker.called) })
	panic("test craft XP tracker panic")
}

type blockingInventoryService struct {
	delegate InventoryService
	entered  chan struct{}
	release  chan struct{}
	calls    atomic.Int64
}

func newBlockingInventoryService(delegate InventoryService) *blockingInventoryService {
	return &blockingInventoryService{
		delegate: delegate,
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (service *blockingInventoryService) AddItem(input economy.AddItemInput) (economy.AddItemResult, error) {
	if service.calls.Add(1) == 1 {
		close(service.entered)
		<-service.release
	}
	return service.delegate.AddItem(input)
}

type countingReservationService struct {
	delegate     ReservationService
	reserveCalls atomic.Int64
}

func newCountingReservationService(delegate ReservationService) *countingReservationService {
	return &countingReservationService{delegate: delegate}
}

func (service *countingReservationService) ReserveItems(input economy.ReserveItemsInput) (economy.ReserveItemsResult, error) {
	service.reserveCalls.Add(1)
	return service.delegate.ReserveItems(input)
}

func (service *countingReservationService) ReleaseReservation(reservationID economy.ReservationID) (economy.ReleaseReservationResult, error) {
	return service.delegate.ReleaseReservation(reservationID)
}

func (service *countingReservationService) CommitReservation(reservationID economy.ReservationID) (economy.CommitReservationResult, error) {
	return service.delegate.CommitReservation(reservationID)
}

type blockingShipService struct {
	delegate ShipService
	entered  chan struct{}
	release  chan struct{}
	calls    atomic.Int64
}

func newBlockingShipService(delegate ShipService) *blockingShipService {
	return &blockingShipService{
		delegate: delegate,
		entered:  make(chan struct{}),
		release:  make(chan struct{}),
	}
}

func (service *blockingShipService) UnlockShip(input ships.UnlockShipInput) (ships.UnlockShipResult, error) {
	if service.calls.Add(1) == 1 {
		close(service.entered)
		<-service.release
	}
	return service.delegate.UnlockShip(input)
}

func (service *blockingShipService) GetHangar(playerID foundation.PlayerID) (ships.HangarSnapshot, error) {
	return service.delegate.GetHangar(playerID)
}

package crafting

import (
	"errors"
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

var errTestCraftLocationRejected = errors.New("test craft location rejected")

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

func TestStartCraftRejectsMissingStartReference(t *testing.T) {
	tests := []struct {
		name      string
		reference foundation.IdempotencyKey
	}{
		{name: "missing"},
		{name: "blank", reference: foundation.IdempotencyKey(" ")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCraftingServiceFixture(t)
			input := fixture.startInput(RecipeIDRefinedAlloy)
			input.ReferenceKey = tc.reference

			_, err := fixture.service.StartCraft(input)
			if !errors.Is(err, foundation.ErrEmptyIdempotencyKey) {
				t.Fatalf("StartCraft error = %v, want ErrEmptyIdempotencyKey", err)
			}
			if got := len(fixture.service.Jobs()); got != 0 {
				t.Fatalf("jobs len = %d, want 0", got)
			}
			if got := len(fixture.inventory.ItemLedgerEntries()); got != 0 {
				t.Fatalf("item ledger entries len = %d, want 0", got)
			}
			if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 0 {
				t.Fatalf("currency ledger entries len = %d, want 0", got)
			}
		})
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

	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCraftingServiceFixture(t)
			tc.prepare(t, fixture)

			_, err := fixture.service.StartCraft(StartCraftInput{
				PlayerID:     fixture.playerID,
				RecipeID:     tc.recipeID,
				Location:     tc.location,
				ReferenceKey: mustCraftStartKey(t, fmt.Sprintf("gate-%d", index)),
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

func TestStartCraftDuplicateReferenceReturnsOriginalJobWithoutSecondMutation(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedCraftingRole(t, 1)
	recipe := fixture.recipe(t, RecipeIDRefinedAlloy)
	fixture.seedRecipeInputs(t, recipe, "duplicate-start")
	fixture.seedCredits(t, recipe.RequiredCredits.Int64(), "duplicate-start-credits")
	input := fixture.startInputWithReference(recipe.RecipeID, "duplicate-start")

	first, err := fixture.service.StartCraft(input)
	if err != nil {
		t.Fatalf("first StartCraft: %v", err)
	}
	itemLedgerAfterFirst := len(fixture.inventory.ItemLedgerEntries())
	walletLedgerAfterFirst := len(fixture.wallet.CurrencyLedgerEntries())
	second, err := fixture.service.StartCraft(input)
	if err != nil {
		t.Fatalf("duplicate StartCraft: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first StartCraft Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate StartCraft Duplicate = false, want true")
	}
	if second.Job.JobID != first.Job.JobID {
		t.Fatalf("duplicate job id = %q, want %q", second.Job.JobID, first.Job.JobID)
	}
	if second.Reservation.ReservationID != first.Reservation.ReservationID {
		t.Fatalf("duplicate reservation id = %q, want %q", second.Reservation.ReservationID, first.Reservation.ReservationID)
	}
	if second.WalletDebit.LedgerEntry.LedgerID != first.WalletDebit.LedgerEntry.LedgerID {
		t.Fatalf("duplicate wallet ledger id = %q, want %q", second.WalletDebit.LedgerEntry.LedgerID, first.WalletDebit.LedgerEntry.LedgerID)
	}
	if got := len(fixture.inventory.ItemLedgerEntries()); got != itemLedgerAfterFirst {
		t.Fatalf("item ledger entries len = %d, want %d", got, itemLedgerAfterFirst)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != walletLedgerAfterFirst {
		t.Fatalf("currency ledger entries len = %d, want %d", got, walletLedgerAfterFirst)
	}
	if got := len(fixture.service.Jobs()); got != 1 {
		t.Fatalf("jobs len = %d, want 1", got)
	}
	if got := fixture.wallet.Balance(fixture.playerID, economy.CurrencyBucketCredits); got != 0 {
		t.Fatalf("wallet balance = %d, want 0", got)
	}
}

func TestStartCraftConcurrentDuplicateReferenceWaitsForCanonicalResult(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedCraftingRole(t, 1)
	recipe := fixture.recipe(t, RecipeIDRefinedAlloy)
	fixture.seedRecipeInputs(t, recipe, "concurrent-duplicate-start")
	fixture.seedCredits(t, recipe.RequiredCredits.Int64(), "concurrent-duplicate-start-credits")
	input := fixture.startInputWithReference(recipe.RecipeID, "concurrent-duplicate-start")
	blockingReservations := newBlockingStartReservationService(fixture.reservations)
	fixture.service.reservations = blockingReservations

	firstCh := make(chan startCraftCallResult, 1)
	secondCh := make(chan startCraftCallResult, 1)
	go func() {
		result, err := fixture.service.StartCraft(input)
		firstCh <- startCraftCallResult{result: result, err: err}
	}()
	<-blockingReservations.entered
	go func() {
		result, err := fixture.service.StartCraft(input)
		secondCh <- startCraftCallResult{result: result, err: err}
	}()
	close(blockingReservations.release)

	first := receiveStartCraftCallResult(t, firstCh)
	second := receiveStartCraftCallResult(t, secondCh)
	if first.err != nil {
		t.Fatalf("first StartCraft error = %v", first.err)
	}
	if second.err != nil {
		t.Fatalf("second StartCraft error = %v", second.err)
	}
	if first.result.Duplicate {
		t.Fatal("first StartCraft Duplicate = true, want false")
	}
	if !second.result.Duplicate {
		t.Fatal("second StartCraft Duplicate = false, want true")
	}
	if second.result.Job.JobID != first.result.Job.JobID {
		t.Fatalf("second job id = %q, want %q", second.result.Job.JobID, first.result.Job.JobID)
	}
	if got := blockingReservations.reserveCalls.Load(); got != 1 {
		t.Fatalf("ReserveItems calls = %d, want 1", got)
	}
	if got := len(fixture.service.Jobs()); got != 1 {
		t.Fatalf("jobs len = %d, want 1", got)
	}
}

func TestStartCraftDuplicateReferenceMismatchRejectsBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(StartCraftInput) StartCraftInput
	}{
		{
			name: "recipe",
			mutate: func(input StartCraftInput) StartCraftInput {
				input.RecipeID = RecipeIDLaserAlphaT1
				return input
			},
		},
		{
			name: "location",
			mutate: func(input StartCraftInput) StartCraftInput {
				input.Location = CraftLocation{Type: CraftLocationStation, ID: "other-station"}
				return input
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCraftingServiceFixture(t)
			fixture.seedCraftingRole(t, 1)
			recipe := fixture.recipe(t, RecipeIDRefinedAlloy)
			fixture.seedRecipeInputs(t, recipe, "mismatch-"+tc.name)
			fixture.seedCredits(t, recipe.RequiredCredits.Int64(), "mismatch-"+tc.name+"-credits")
			input := fixture.startInputWithReference(recipe.RecipeID, "mismatch-"+tc.name)

			if _, err := fixture.service.StartCraft(input); err != nil {
				t.Fatalf("first StartCraft: %v", err)
			}
			itemLedgerBefore := len(fixture.inventory.ItemLedgerEntries())
			walletLedgerBefore := len(fixture.wallet.CurrencyLedgerEntries())
			walletBefore := fixture.wallet.Balance(fixture.playerID, economy.CurrencyBucketCredits)
			jobsBefore := len(fixture.service.Jobs())

			_, err := fixture.service.StartCraft(tc.mutate(input))
			if !errors.Is(err, ErrCraftStartReferenceMismatch) {
				t.Fatalf("mismatch StartCraft error = %v, want ErrCraftStartReferenceMismatch", err)
			}
			if got := len(fixture.inventory.ItemLedgerEntries()); got != itemLedgerBefore {
				t.Fatalf("item ledger entries len = %d, want %d", got, itemLedgerBefore)
			}
			if got := len(fixture.wallet.CurrencyLedgerEntries()); got != walletLedgerBefore {
				t.Fatalf("currency ledger entries len = %d, want %d", got, walletLedgerBefore)
			}
			if got := fixture.wallet.Balance(fixture.playerID, economy.CurrencyBucketCredits); got != walletBefore {
				t.Fatalf("wallet balance = %d, want %d", got, walletBefore)
			}
			if got := len(fixture.service.Jobs()); got != jobsBefore {
				t.Fatalf("jobs len = %d, want %d", got, jobsBefore)
			}
		})
	}
}

func TestStartCraftReferenceCacheIsScopedByPlayer(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedCraftingRole(t, 1)
	recipe := fixture.recipe(t, RecipeIDRefinedAlloy)
	fixture.seedRecipeInputs(t, recipe, "player-one-shared-reference")
	fixture.seedCredits(t, recipe.RequiredCredits.Int64(), "player-one-shared-reference-credits")
	reference := mustCraftStartKey(t, "shared-reference")

	first, err := fixture.service.StartCraft(StartCraftInput{
		PlayerID:     fixture.playerID,
		RecipeID:     recipe.RecipeID,
		Location:     fixture.location,
		ReferenceKey: reference,
	})
	if err != nil {
		t.Fatalf("player one StartCraft: %v", err)
	}

	playerTwo := foundation.PlayerID("player-2")
	playerTwoLocation, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerTwo.String())
	if err != nil {
		t.Fatalf("player two NewItemLocation: %v", err)
	}
	fixture.seedRecipeInputsFor(t, playerTwo, playerTwoLocation, recipe, "player-two-shared-reference")
	fixture.seedCreditsFor(t, playerTwo, recipe.RequiredCredits.Int64(), "player-two-shared-reference-credits")

	second, err := fixture.service.StartCraft(StartCraftInput{
		PlayerID:     playerTwo,
		RecipeID:     recipe.RecipeID,
		Location:     fixture.location,
		ReferenceKey: reference,
	})
	if err != nil {
		t.Fatalf("player two StartCraft: %v", err)
	}

	if second.Duplicate {
		t.Fatal("player two StartCraft Duplicate = true, want false")
	}
	if second.Job.PlayerID != playerTwo {
		t.Fatalf("player two job player = %q, want %q", second.Job.PlayerID, playerTwo)
	}
	if second.Job.JobID == first.Job.JobID {
		t.Fatalf("player two job id = player one job id %q, want distinct", second.Job.JobID)
	}
	if got := len(fixture.service.Jobs()); got != 2 {
		t.Fatalf("jobs len = %d, want 2", got)
	}
}

func TestStartCraftLocationAuthorizerRejectsBeforeEconomyMutation(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedCraftingRole(t, 1)
	recipe := fixture.recipe(t, RecipeIDRefinedAlloy)
	fixture.seedRecipeInputs(t, recipe, "location-authorizer")
	fixture.seedCredits(t, recipe.RequiredCredits.Int64(), "location-authorizer-credits")
	authorizer := &recordingCraftLocationAuthorizer{err: errTestCraftLocationRejected}
	fixture.service.locationAuth = authorizer
	countingReservations := newCountingReservationService(fixture.reservations)
	fixture.service.reservations = countingReservations

	walletLedgerCount := len(fixture.wallet.CurrencyLedgerEntries())
	itemLedgerCount := len(fixture.inventory.ItemLedgerEntries())

	_, err := fixture.service.StartCraft(fixture.startInput(recipe.RecipeID))
	if !errors.Is(err, errTestCraftLocationRejected) {
		t.Fatalf("StartCraft error = %v, want authorizer rejection", err)
	}
	if !authorizer.called {
		t.Fatal("location authorizer was not called")
	}
	if authorizer.input.PlayerID != fixture.playerID || authorizer.input.Recipe.RecipeID != recipe.RecipeID || authorizer.input.Location != fixture.location {
		t.Fatalf("authorizer input = %#v, want player recipe and location", authorizer.input)
	}

	assertNoStartCraftEconomyMutation(t, fixture, recipe, "craft-job-1", walletLedgerCount, itemLedgerCount)
	if got := countingReservations.reserveCalls.Load(); got != 0 {
		t.Fatalf("ReserveItems calls = %d, want 0", got)
	}
}

func TestStartCraftRequiresAuthorizerForPlanetLocationsBeforeEconomyMutation(t *testing.T) {
	tests := []struct {
		name         string
		locationType CraftLocationType
		location     CraftLocation
	}{
		{
			name:         "owned planet",
			locationType: CraftLocationOwnedPlanet,
			location:     CraftLocation{Type: CraftLocationOwnedPlanet, ID: "planet-1"},
		},
		{
			name:         "planet building",
			locationType: CraftLocationPlanetBuilding,
			location:     CraftLocation{Type: CraftLocationPlanetBuilding, ID: "forge-1", PlanetID: "planet-1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCraftingServiceFixture(t)
			fixture.seedCraftingRole(t, 1)
			recipe := fixture.replaceRecipeLocationRequirement(t, tc.locationType)
			fixture.seedRecipeInputs(t, recipe, "missing-location-authorizer-"+tc.name)
			fixture.seedCredits(t, recipe.RequiredCredits.Int64(), "missing-location-authorizer-"+tc.name+"-credits")
			countingReservations := newCountingReservationService(fixture.reservations)
			fixture.service.reservations = countingReservations

			walletLedgerCount := len(fixture.wallet.CurrencyLedgerEntries())
			itemLedgerCount := len(fixture.inventory.ItemLedgerEntries())

			input := StartCraftInput{
				PlayerID:     fixture.playerID,
				RecipeID:     recipe.RecipeID,
				Location:     tc.location,
				ReferenceKey: mustCraftStartKey(t, "missing-location-authorizer-"+tc.name),
			}
			_, err := fixture.service.StartCraft(input)
			if !errors.Is(err, ErrMissingLocationAuthorizer) {
				t.Fatalf("StartCraft error = %v, want ErrMissingLocationAuthorizer", err)
			}

			retryCh := make(chan startCraftCallResult, 1)
			go func() {
				result, err := fixture.service.StartCraft(input)
				retryCh <- startCraftCallResult{result: result, err: err}
			}()
			retry := receiveStartCraftCallResult(t, retryCh)
			if !errors.Is(retry.err, ErrMissingLocationAuthorizer) {
				t.Fatalf("retry StartCraft error = %v, want ErrMissingLocationAuthorizer", retry.err)
			}

			assertNoStartCraftEconomyMutation(t, fixture, recipe, "craft-job-1", walletLedgerCount, itemLedgerCount)
			if got := countingReservations.reserveCalls.Load(); got != 0 {
				t.Fatalf("ReserveItems calls = %d, want 0", got)
			}
		})
	}
}

func TestStartCraftPlanetBuildingWithAuthorizerUsesPlanetStorage(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedCraftingRole(t, 1)
	recipe := fixture.replaceRecipeLocationRequirement(t, CraftLocationPlanetBuilding)
	location := CraftLocation{Type: CraftLocationPlanetBuilding, ID: "forge-1", PlanetID: "planet-1"}
	planetStorage, err := economy.NewItemLocation(economy.LocationKindPlanetStorage, location.PlanetID.String())
	if err != nil {
		t.Fatalf("planet storage NewItemLocation: %v", err)
	}
	buildingStorage, err := economy.NewItemLocation(economy.LocationKindPlanetStorage, location.ID)
	if err != nil {
		t.Fatalf("building storage NewItemLocation: %v", err)
	}
	fixture.seedRecipeInputsFor(t, fixture.playerID, planetStorage, recipe, "planet-building-authorized")
	fixture.seedCredits(t, recipe.RequiredCredits.Int64(), "planet-building-authorized-credits")
	authorizer := &recordingCraftLocationAuthorizer{}
	fixture.service.locationAuth = authorizer

	result, err := fixture.service.StartCraft(StartCraftInput{
		PlayerID:     fixture.playerID,
		RecipeID:     recipe.RecipeID,
		Location:     location,
		ReferenceKey: mustCraftStartKey(t, "planet-building-authorized"),
	})
	if err != nil {
		t.Fatalf("StartCraft error = %v, want nil", err)
	}
	if !authorizer.called {
		t.Fatal("location authorizer was not called")
	}
	if result.SourceLocation != planetStorage {
		t.Fatalf("source location = %+v, want planet storage %+v", result.SourceLocation, planetStorage)
	}
	for _, input := range recipe.Inputs {
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, planetStorage); got != 0 {
			t.Fatalf("planet storage source quantity for %q = %d, want 0", input.ItemID, got)
		}
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, buildingStorage); got != 0 {
			t.Fatalf("building-id storage quantity for %q = %d, want 0", input.ItemID, got)
		}
		if got := fixture.inventory.TotalItemQuantity(fixture.playerID, input.ItemID, fixture.reservedLocation(result.Job.JobID)); got != input.Quantity.Int64() {
			t.Fatalf("reserved quantity for %q = %d, want %d", input.ItemID, got, input.Quantity.Int64())
		}
	}
}

func TestStartCraftRejectsAlreadyOwnedNonRepeatableShipBeforeEconomyMutation(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedRank2(t)
	fixture.seedCraftingRole(t, 2)
	recipe := fixture.recipe(t, RecipeIDScoutT1)
	fixture.seedRecipeInputs(t, recipe, "owned-ship")
	fixture.seedCredits(t, recipe.RequiredCredits.Int64(), "owned-ship-credits")
	if _, err := fixture.ships.UnlockShip(ships.UnlockShipInput{
		PlayerID:    fixture.playerID,
		ShipID:      ships.ShipIDScoutT1,
		Source:      "test",
		ReferenceID: "already-owned",
	}); err != nil {
		t.Fatalf("seed UnlockShip: %v", err)
	}
	countingReservations := newCountingReservationService(fixture.reservations)
	fixture.service.reservations = countingReservations

	walletLedgerCount := len(fixture.wallet.CurrencyLedgerEntries())
	itemLedgerCount := len(fixture.inventory.ItemLedgerEntries())

	_, err := fixture.service.StartCraft(fixture.startInput(recipe.RecipeID))
	if !errors.Is(err, ErrCraftOutputAlreadyOwned) {
		t.Fatalf("StartCraft error = %v, want ErrCraftOutputAlreadyOwned", err)
	}

	assertNoStartCraftEconomyMutation(t, fixture, recipe, "craft-job-1", walletLedgerCount, itemLedgerCount)
	if got := countingReservations.reserveCalls.Load(); got != 0 {
		t.Fatalf("ReserveItems calls = %d, want 0", got)
	}
	if got := countShip(mustHangar(t, fixture.ships, fixture.playerID), ships.ShipIDScoutT1); got != 1 {
		t.Fatalf("scout unlock count = %d, want 1", got)
	}
}

func TestStartCraftReservesMaterialsAndDebitsFee(t *testing.T) {
	fixture := newCraftingServiceFixture(t)
	fixture.seedCraftingRole(t, 1)
	recipe := fixture.recipe(t, RecipeIDRefinedAlloy)
	fixture.seedRecipeInputs(t, recipe, "start-success")
	fixture.seedCredits(t, 1_000, "start-success-credits")
	input := fixture.startInput(recipe.RecipeID)

	result, err := fixture.service.StartCraft(input)
	if err != nil {
		t.Fatalf("StartCraft: %v", err)
	}

	if result.Job.State != CraftJobStateRunning {
		t.Fatalf("job state = %q, want %q", result.Job.State, CraftJobStateRunning)
	}
	if result.Duplicate {
		t.Fatal("StartCraft Duplicate = true, want false")
	}
	if result.ReferenceKey != input.ReferenceKey {
		t.Fatalf("result reference = %q, want %q", result.ReferenceKey, input.ReferenceKey)
	}
	if result.Reservation.ReferenceKey != input.ReferenceKey {
		t.Fatalf("reservation reference = %q, want %q", result.Reservation.ReferenceKey, input.ReferenceKey)
	}
	if result.WalletDebit.LedgerEntry.ReferenceKey != input.ReferenceKey {
		t.Fatalf("wallet debit reference = %q, want %q", result.WalletDebit.LedgerEntry.ReferenceKey, input.ReferenceKey)
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

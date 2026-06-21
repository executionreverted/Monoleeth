package crafting

import (
	"errors"
	"fmt"
	"testing"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/ships"
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

package production

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestBuildingMutationBuildDebitsMaterialsRecordsLedgerAndEmitsEvents(t *testing.T) {
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	wallet := &fakeBuildingWallet{}
	service := newTestBuildingMutationService(t, store, MustMVPCatalog(), wallet, mapBuildingCosts{
		{operation: BuildingMutationBuild, definitionID: ProductionDefinitionIDAlloyFoundryL1}: {
			Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 20}},
			Wallet: &BuildingWalletCost{
				PlayerID: "player-1",
				Currency: economy.CurrencyBucketCredits,
				Amount:   100,
			},
		},
	})
	reference := mustBuildingBuildKey(t, "planet-1", "building-1")

	result, err := service.BuildPlanetBuilding(BuildPlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
		RequestedAt:  testTime(1),
		ReferenceKey: reference,
	})
	if err != nil {
		t.Fatalf("BuildPlanetBuilding() error = %v, want nil", err)
	}
	if result.Duplicate {
		t.Fatal("Duplicate = true, want false")
	}
	if result.Building.State != BuildingStateActive || result.Building.Level != 1 || result.Building.BuildingType != BuildingTypeAlloyFoundry {
		t.Fatalf("building = %+v, want active alloy foundry L1", result.Building)
	}
	assertBuildingMutationStorage(t, store, "iron_ore", 30)
	assertBuildingMaterialLedger(t, store, reference, "iron_ore", 20, 30)
	assertBuildingMutationEvents(t, store, EventPlanetStorageUpdated, EventPlanetBuildingUpdated)
	if got := len(store.OutboxRecords()); got != 2 {
		t.Fatalf("outbox records = %d, want 2", got)
	}
	if wallet.calls != 1 || wallet.last.ReferenceKey != reference || wallet.last.Amount != 100 {
		t.Fatalf("wallet calls/input = %d/%+v, want one debit amount 100 ref %q", wallet.calls, wallet.last, reference)
	}

	duplicate, err := service.BuildPlanetBuilding(BuildPlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
		RequestedAt:  testTime(2),
		ReferenceKey: reference,
	})
	if err != nil {
		t.Fatalf("duplicate BuildPlanetBuilding() error = %v, want nil", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate Duplicate = false, want true")
	}
	assertBuildingMutationStorage(t, store, "iron_ore", 30)
	if got := len(store.BuildingMaterialLedgerEntries()); got != 1 {
		t.Fatalf("material ledger rows after duplicate = %d, want 1", got)
	}
	if got := len(store.Events()); got != 2 {
		t.Fatalf("events after duplicate = %d, want 2", got)
	}
	if got := len(store.OutboxRecords()); got != 2 {
		t.Fatalf("outbox after duplicate = %d, want 2", got)
	}
	if wallet.calls != 1 {
		t.Fatalf("wallet calls after duplicate = %d, want 1", wallet.calls)
	}
	if duplicate.MaterialLedger[0].LedgerID != result.MaterialLedger[0].LedgerID {
		t.Fatalf("duplicate ledger id = %q, want original %q", duplicate.MaterialLedger[0].LedgerID, result.MaterialLedger[0].LedgerID)
	}
}

func TestBuildingMutationBuildDuplicateReferenceRejectsConflictingIntent(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(BuildPlanetBuildingInput) BuildPlanetBuildingInput
	}{
		{
			name: "building",
			mutate: func(input BuildPlanetBuildingInput) BuildPlanetBuildingInput {
				input.BuildingID = "building-2"
				return input
			},
		},
		{
			name: "definition",
			mutate: func(input BuildPlanetBuildingInput) BuildPlanetBuildingInput {
				input.DefinitionID = ProductionDefinitionIDIronExtractorL1
				return input
			},
		},
		{
			name: "building type and level",
			mutate: func(input BuildPlanetBuildingInput) BuildPlanetBuildingInput {
				input.DefinitionID = ""
				input.BuildingType = BuildingTypeIronExtractor
				input.Level = 1
				return input
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 50}})
			wallet := &fakeBuildingWallet{}
			service := newTestBuildingMutationService(t, store, MustMVPCatalog(), wallet, mapBuildingCosts{
				{operation: BuildingMutationBuild, definitionID: ProductionDefinitionIDAlloyFoundryL1}: {
					Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 20}},
					Wallet: &BuildingWalletCost{
						PlayerID: "player-1",
						Currency: economy.CurrencyBucketCredits,
						Amount:   100,
					},
				},
			})
			input := BuildPlanetBuildingInput{
				PlanetID:     "planet-1",
				BuildingID:   "building-1",
				DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
				RequestedAt:  testTime(1),
				ReferenceKey: mustBuildingBuildKey(t, "planet-1", "building-1"),
			}

			if _, err := service.BuildPlanetBuilding(input); err != nil {
				t.Fatalf("first BuildPlanetBuilding() error = %v, want nil", err)
			}
			storageBefore := buildingMutationStorageQuantity(t, store, "iron_ore")
			ledgerBefore := len(store.BuildingMaterialLedgerEntries())
			eventsBefore := len(store.Events())
			outboxBefore := len(store.OutboxRecords())
			referencesBefore := len(store.BuildingMutationReferences())
			walletCallsBefore := wallet.calls

			conflicting := tc.mutate(input)
			conflicting.RequestedAt = testTime(2)
			_, err := service.BuildPlanetBuilding(conflicting)
			if !errors.Is(err, ErrInvalidBuildingMutationReference) {
				t.Fatalf("conflicting BuildPlanetBuilding() error = %v, want ErrInvalidBuildingMutationReference", err)
			}
			assertBuildingMutationStorage(t, store, "iron_ore", storageBefore)
			if got := len(store.BuildingMaterialLedgerEntries()); got != ledgerBefore {
				t.Fatalf("material ledger rows after mismatch = %d, want %d", got, ledgerBefore)
			}
			if got := len(store.Events()); got != eventsBefore {
				t.Fatalf("events after mismatch = %d, want %d", got, eventsBefore)
			}
			if got := len(store.OutboxRecords()); got != outboxBefore {
				t.Fatalf("outbox after mismatch = %d, want %d", got, outboxBefore)
			}
			if got := len(store.BuildingMutationReferences()); got != referencesBefore {
				t.Fatalf("building references after mismatch = %d, want %d", got, referencesBefore)
			}
			if wallet.calls != walletCallsBefore {
				t.Fatalf("wallet calls after mismatch = %d, want %d", wallet.calls, walletCallsBefore)
			}
			if _, ok, lookupErr := store.Building("planet-1", "building-2"); lookupErr != nil || ok {
				t.Fatalf("conflicting building lookup ok = %v err = %v, want false nil", ok, lookupErr)
			}
		})
	}
}

func TestBuildingMutationBuildRejectsWrongReferenceKeyBeforeAdaptersOrMutation(t *testing.T) {
	tests := []struct {
		name      string
		reference foundation.IdempotencyKey
	}{
		{
			name:      "wrong domain",
			reference: mustQuestRewardKeyForBuildingMutationTest(t, "quest-1"),
		},
		{
			name:      "wrong planet",
			reference: mustBuildingBuildKey(t, "planet-2", "building-1"),
		},
		{
			name:      "wrong building",
			reference: mustBuildingBuildKey(t, "planet-1", "building-2"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 50}})
			wallet := &fakeBuildingWallet{}
			costs := &countingBuildingCosts{delegate: mapBuildingCosts{
				{operation: BuildingMutationBuild, definitionID: ProductionDefinitionIDAlloyFoundryL1}: {
					Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 20}},
					Wallet: &BuildingWalletCost{
						PlayerID: "player-1",
						Currency: economy.CurrencyBucketCredits,
						Amount:   100,
					},
				},
			}}
			service := newTestBuildingMutationService(t, store, MustMVPCatalog(), wallet, costs)

			_, err := service.BuildPlanetBuilding(BuildPlanetBuildingInput{
				PlanetID:     "planet-1",
				BuildingID:   "building-1",
				DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
				RequestedAt:  testTime(1),
				ReferenceKey: tc.reference,
			})
			if !errors.Is(err, ErrInvalidBuildingMutationReference) {
				t.Fatalf("BuildPlanetBuilding() error = %v, want ErrInvalidBuildingMutationReference", err)
			}
			assertBuildingMutationStorage(t, store, "iron_ore", 50)
			assertNoBuildingMutationSideEffects(t, store, wallet)
			if costs.calls != 0 {
				t.Fatalf("cost provider calls = %d, want 0", costs.calls)
			}
		})
	}
}

func TestBuildingMutationBuildRejectsInsufficientStorageWithoutMutation(t *testing.T) {
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 10}})
	wallet := &fakeBuildingWallet{}
	service := newTestBuildingMutationService(t, store, MustMVPCatalog(), wallet, mapBuildingCosts{
		{operation: BuildingMutationBuild, definitionID: ProductionDefinitionIDAlloyFoundryL1}: {
			Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 20}},
		},
	})

	_, err := service.BuildPlanetBuilding(BuildPlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
		RequestedAt:  testTime(1),
		ReferenceKey: mustBuildingBuildKey(t, "planet-1", "building-1"),
	})
	if !errors.Is(err, ErrInsufficientBuildingMaterials) {
		t.Fatalf("BuildPlanetBuilding() error = %v, want ErrInsufficientBuildingMaterials", err)
	}
	assertBuildingMutationStorage(t, store, "iron_ore", 10)
	assertNoBuildingMutationSideEffects(t, store, wallet)
}

func TestBuildingMutationBuildWalletFailureLeavesStoreUnchanged(t *testing.T) {
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	walletErr := errors.New("wallet unavailable")
	wallet := &fakeBuildingWallet{err: walletErr}
	service := newTestBuildingMutationService(t, store, MustMVPCatalog(), wallet, mapBuildingCosts{
		{operation: BuildingMutationBuild, definitionID: ProductionDefinitionIDAlloyFoundryL1}: {
			Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 20}},
			Wallet: &BuildingWalletCost{
				PlayerID: "player-1",
				Currency: economy.CurrencyBucketCredits,
				Amount:   100,
			},
		},
	})

	_, err := service.BuildPlanetBuilding(BuildPlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
		RequestedAt:  testTime(1),
		ReferenceKey: mustBuildingBuildKey(t, "planet-1", "building-1"),
	})
	if !errors.Is(err, walletErr) {
		t.Fatalf("BuildPlanetBuilding() error = %v, want wallet error", err)
	}
	assertBuildingMutationStorage(t, store, "iron_ore", 50)
	if _, ok, lookupErr := store.Building("planet-1", "building-1"); lookupErr != nil || ok {
		t.Fatalf("Building() ok = %v err = %v, want false nil", ok, lookupErr)
	}
	if got := len(store.BuildingMaterialLedgerEntries()); got != 0 {
		t.Fatalf("material ledger rows = %d, want 0", got)
	}
	if got := len(store.BuildingMutationReferences()); got != 0 {
		t.Fatalf("building references = %d, want 0", got)
	}
	if got := len(store.Events()); got != 0 {
		t.Fatalf("events = %d, want 0", got)
	}
	if got := len(store.OutboxRecords()); got != 0 {
		t.Fatalf("outbox records = %d, want 0", got)
	}
	if wallet.calls != 1 {
		t.Fatalf("wallet calls = %d, want 1 failed call", wallet.calls)
	}
}

func TestBuildingMutationBuildReturnsStaleIfStateChangesAfterWalletDebit(t *testing.T) {
	catalogRows := MustMVPCatalog()
	definition, err := catalogRows.MustGet(ProductionDefinitionIDAlloyFoundryL1)
	if err != nil {
		t.Fatalf("MustGet() error = %v, want nil", err)
	}
	racingBuilding, err := NewPlanetBuilding("building-1", "planet-1", definition, BuildingStateActive, testTime(1), testTime(1))
	if err != nil {
		t.Fatalf("NewPlanetBuilding() error = %v, want nil", err)
	}
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	wallet := &staleBuildInsertWallet{store: store, building: racingBuilding}
	service := newTestBuildingMutationService(t, store, catalogRows, wallet, mapBuildingCosts{
		{operation: BuildingMutationBuild, definitionID: ProductionDefinitionIDAlloyFoundryL1}: {
			Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 20}},
			Wallet: &BuildingWalletCost{
				PlayerID: "player-1",
				Currency: economy.CurrencyBucketCredits,
				Amount:   100,
			},
		},
	})

	_, err = service.BuildPlanetBuilding(BuildPlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
		RequestedAt:  testTime(1),
		ReferenceKey: mustBuildingBuildKey(t, "planet-1", "building-1"),
	})
	if !errors.Is(err, ErrStaleBuildingMutation) {
		t.Fatalf("BuildPlanetBuilding() error = %v, want ErrStaleBuildingMutation", err)
	}
	assertBuildingMutationStorage(t, store, "iron_ore", 50)
	if got := len(store.BuildingMaterialLedgerEntries()); got != 0 {
		t.Fatalf("material ledger rows = %d, want 0", got)
	}
	if got := len(store.BuildingMutationReferences()); got != 0 {
		t.Fatalf("building references = %d, want 0", got)
	}
	if got := len(store.Events()); got != 0 {
		t.Fatalf("events = %d, want 0", got)
	}
	if got := len(store.OutboxRecords()); got != 0 {
		t.Fatalf("outbox records = %d, want 0", got)
	}
	if wallet.calls != 1 {
		t.Fatalf("wallet calls = %d, want 1", wallet.calls)
	}
}

func TestBuildingMutationUpgradeUsesNextCatalogLevelAndDuplicateIsSafe(t *testing.T) {
	catalogRows := testBuildingMutationCatalogWithL2(t)
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 100}})
	addBuildingMutationBuilding(t, store, catalogRows, "building-1", ProductionDefinitionIDIronExtractorL1)
	service := newTestBuildingMutationService(t, store, catalogRows, nil, mapBuildingCosts{
		{operation: BuildingMutationUpgrade, definitionID: "iron_extractor_l2"}: {
			Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 30}},
		},
	})
	reference := mustBuildingUpgradeKey(t, "planet-1", "building-1", 2)

	result, err := service.UpgradePlanetBuilding(UpgradePlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		RequestedAt:  testTime(2),
		ReferenceKey: reference,
	})
	if err != nil {
		t.Fatalf("UpgradePlanetBuilding() error = %v, want nil", err)
	}
	if result.Building.Level != 2 || result.Building.Source.DefinitionID != catalog.DefinitionID("iron_extractor_l2") {
		t.Fatalf("upgraded building = %+v, want iron_extractor_l2", result.Building)
	}
	if !result.Building.CreatedAt.Equal(testTime(1)) || result.Building.BuildingID != "building-1" || result.Building.PlanetID != "planet-1" || result.Building.State != BuildingStateActive {
		t.Fatalf("upgraded identity/state = %+v, want preserved id/planet/created/active", result.Building)
	}
	assertBuildingMutationStorage(t, store, "iron_ore", 70)
	assertBuildingMaterialLedger(t, store, reference, "iron_ore", 30, 70)
	assertBuildingMutationEvents(t, store, EventPlanetStorageUpdated, EventPlanetBuildingUpdated)

	duplicate, err := service.UpgradePlanetBuilding(UpgradePlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		RequestedAt:  testTime(3),
		ReferenceKey: reference,
	})
	if err != nil {
		t.Fatalf("duplicate UpgradePlanetBuilding() error = %v, want nil", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate Duplicate = false, want true")
	}
	assertBuildingMutationStorage(t, store, "iron_ore", 70)
	if got := len(store.BuildingMaterialLedgerEntries()); got != 1 {
		t.Fatalf("material ledger rows after duplicate = %d, want 1", got)
	}
	if got := len(store.Events()); got != 2 {
		t.Fatalf("events after duplicate = %d, want 2", got)
	}
}

func TestBuildingMutationUpgradeDuplicateReferenceRejectsConflictingIntent(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(UpgradePlanetBuildingInput) UpgradePlanetBuildingInput
	}{
		{
			name: "definition",
			mutate: func(input UpgradePlanetBuildingInput) UpgradePlanetBuildingInput {
				input.DefinitionID = ProductionDefinitionIDAlloyFoundryL1
				return input
			},
		},
		{
			name: "next level",
			mutate: func(input UpgradePlanetBuildingInput) UpgradePlanetBuildingInput {
				input.NextLevel = 3
				return input
			},
		},
		{
			name: "building",
			mutate: func(input UpgradePlanetBuildingInput) UpgradePlanetBuildingInput {
				input.BuildingID = "building-2"
				return input
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			catalogRows := testBuildingMutationCatalogWithL2(t)
			store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 100}})
			addBuildingMutationBuilding(t, store, catalogRows, "building-1", ProductionDefinitionIDIronExtractorL1)
			service := newTestBuildingMutationService(t, store, catalogRows, nil, mapBuildingCosts{
				{operation: BuildingMutationUpgrade, definitionID: "iron_extractor_l2"}: {
					Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 30}},
				},
			})
			input := UpgradePlanetBuildingInput{
				PlanetID:     "planet-1",
				BuildingID:   "building-1",
				RequestedAt:  testTime(2),
				ReferenceKey: mustBuildingUpgradeKey(t, "planet-1", "building-1", 2),
			}

			if _, err := service.UpgradePlanetBuilding(input); err != nil {
				t.Fatalf("first UpgradePlanetBuilding() error = %v, want nil", err)
			}
			storageBefore := buildingMutationStorageQuantity(t, store, "iron_ore")
			ledgerBefore := len(store.BuildingMaterialLedgerEntries())
			eventsBefore := len(store.Events())
			outboxBefore := len(store.OutboxRecords())
			referencesBefore := len(store.BuildingMutationReferences())

			conflicting := tc.mutate(input)
			conflicting.RequestedAt = testTime(3)
			_, err := service.UpgradePlanetBuilding(conflicting)
			if !errors.Is(err, ErrInvalidBuildingMutationReference) {
				t.Fatalf("conflicting UpgradePlanetBuilding() error = %v, want ErrInvalidBuildingMutationReference", err)
			}
			assertBuildingMutationStorage(t, store, "iron_ore", storageBefore)
			if got := len(store.BuildingMaterialLedgerEntries()); got != ledgerBefore {
				t.Fatalf("material ledger rows after mismatch = %d, want %d", got, ledgerBefore)
			}
			if got := len(store.Events()); got != eventsBefore {
				t.Fatalf("events after mismatch = %d, want %d", got, eventsBefore)
			}
			if got := len(store.OutboxRecords()); got != outboxBefore {
				t.Fatalf("outbox after mismatch = %d, want %d", got, outboxBefore)
			}
			if got := len(store.BuildingMutationReferences()); got != referencesBefore {
				t.Fatalf("building references after mismatch = %d, want %d", got, referencesBefore)
			}
			building, ok, lookupErr := store.Building("planet-1", "building-1")
			if lookupErr != nil || !ok {
				t.Fatalf("Building() ok = %v err = %v, want true nil", ok, lookupErr)
			}
			if building.Level != 2 || building.Source.DefinitionID != catalog.DefinitionID("iron_extractor_l2") {
				t.Fatalf("building after mismatch = %+v, want iron_extractor_l2", building)
			}
		})
	}
}

func TestBuildingMutationUpgradeRejectsWrongLevelReferenceBeforeAdaptersOrMutation(t *testing.T) {
	catalogRows := testBuildingMutationCatalogWithL2(t)
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 100}})
	addBuildingMutationBuilding(t, store, catalogRows, "building-1", ProductionDefinitionIDIronExtractorL1)
	wallet := &fakeBuildingWallet{}
	costs := &countingBuildingCosts{delegate: mapBuildingCosts{
		{operation: BuildingMutationUpgrade, definitionID: "iron_extractor_l2"}: {
			Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 30}},
			Wallet: &BuildingWalletCost{
				PlayerID: "player-1",
				Currency: economy.CurrencyBucketCredits,
				Amount:   100,
			},
		},
	}}
	service := newTestBuildingMutationService(t, store, catalogRows, wallet, costs)

	_, err := service.UpgradePlanetBuilding(UpgradePlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		RequestedAt:  testTime(2),
		ReferenceKey: mustBuildingUpgradeKey(t, "planet-1", "building-1", 3),
	})
	if !errors.Is(err, ErrInvalidBuildingMutationReference) {
		t.Fatalf("UpgradePlanetBuilding() error = %v, want ErrInvalidBuildingMutationReference", err)
	}
	assertBuildingMutationStorage(t, store, "iron_ore", 100)
	if wallet.calls != 0 {
		t.Fatalf("wallet calls = %d, want 0", wallet.calls)
	}
	if costs.calls != 0 {
		t.Fatalf("cost provider calls = %d, want 0", costs.calls)
	}
	if got := len(store.BuildingMaterialLedgerEntries()); got != 0 {
		t.Fatalf("material ledger rows = %d, want 0", got)
	}
	building, ok, lookupErr := store.Building("planet-1", "building-1")
	if lookupErr != nil || !ok {
		t.Fatalf("Building() ok = %v err = %v, want true nil", ok, lookupErr)
	}
	if building.Level != 1 {
		t.Fatalf("building level = %d, want 1", building.Level)
	}
}

func TestBuildingMutationAdaptersCanReenterProductionReadsWithoutDeadlock(t *testing.T) {
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 50}})
	wallet := &reentrantBuildingWallet{store: store}
	costs := &reentrantBuildingCosts{
		store: store,
		delegate: mapBuildingCosts{
			{operation: BuildingMutationBuild, definitionID: ProductionDefinitionIDAlloyFoundryL1}: {
				Materials: []BuildingMaterialCost{{ItemID: "iron_ore", Quantity: 20}},
				Wallet: &BuildingWalletCost{
					PlayerID: "player-1",
					Currency: economy.CurrencyBucketCredits,
					Amount:   100,
				},
			},
		},
	}
	service := newTestBuildingMutationService(t, store, MustMVPCatalog(), wallet, costs)

	done := make(chan error, 1)
	go func() {
		_, err := service.BuildPlanetBuilding(BuildPlanetBuildingInput{
			PlanetID:     "planet-1",
			BuildingID:   "building-1",
			DefinitionID: ProductionDefinitionIDAlloyFoundryL1,
			RequestedAt:  testTime(1),
			ReferenceKey: mustBuildingBuildKey(t, "planet-1", "building-1"),
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("BuildPlanetBuilding() error = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("BuildPlanetBuilding() timed out; adapters likely re-entered while production store lock was held")
	}
	if costs.calls != 1 || costs.snapshots != 1 {
		t.Fatalf("cost calls/snapshots = %d/%d, want 1/1", costs.calls, costs.snapshots)
	}
	if wallet.calls != 1 || !wallet.readStorage {
		t.Fatalf("wallet calls/readStorage = %d/%v, want 1/true", wallet.calls, wallet.readStorage)
	}
	assertBuildingMutationStorage(t, store, "iron_ore", 30)
}

func TestBuildingMutationUpgradeUnknownNextLevelFailsWithoutMutation(t *testing.T) {
	store := newBuildingMutationStore(t, []StoredItem{{ItemID: "iron_ore", Quantity: 100}})
	catalogRows := MustMVPCatalog()
	addBuildingMutationBuilding(t, store, catalogRows, "building-1", ProductionDefinitionIDIronExtractorL1)
	service := newTestBuildingMutationService(t, store, catalogRows, nil, mapBuildingCosts{})

	_, err := service.UpgradePlanetBuilding(UpgradePlanetBuildingInput{
		PlanetID:     "planet-1",
		BuildingID:   "building-1",
		RequestedAt:  testTime(2),
		ReferenceKey: mustBuildingUpgradeKey(t, "planet-1", "building-1", 2),
	})
	if !errors.Is(err, ErrUnknownBuildingDefinition) {
		t.Fatalf("UpgradePlanetBuilding() error = %v, want ErrUnknownBuildingDefinition", err)
	}
	assertBuildingMutationStorage(t, store, "iron_ore", 100)
	if got := len(store.BuildingMaterialLedgerEntries()); got != 0 {
		t.Fatalf("material ledger rows = %d, want 0", got)
	}
	if got := len(store.Events()); got != 0 {
		t.Fatalf("events = %d, want 0", got)
	}
}

type buildingCostKey struct {
	operation    BuildingMutationKind
	definitionID catalog.DefinitionID
}

type mapBuildingCosts map[buildingCostKey]BuildingMutationCost

func (costs mapBuildingCosts) BuildingMutationCost(input BuildingMutationCostInput) (BuildingMutationCost, error) {
	return costs[buildingCostKey{operation: input.Operation, definitionID: input.Definition.DefinitionID}], nil
}

type countingBuildingCosts struct {
	store     *InMemoryStore
	delegate  mapBuildingCosts
	calls     int
	snapshots int
}

func (costs *countingBuildingCosts) BuildingMutationCost(input BuildingMutationCostInput) (BuildingMutationCost, error) {
	costs.calls++
	if costs.store != nil {
		costs.snapshots = len(costs.store.Snapshots())
	}
	return costs.delegate.BuildingMutationCost(input)
}

type reentrantBuildingCosts = countingBuildingCosts

type fakeBuildingWallet struct {
	err   error
	calls int
	last  economy.DebitWalletInput
}

func (wallet *fakeBuildingWallet) DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error) {
	wallet.calls++
	wallet.last = input
	if wallet.err != nil {
		return economy.DebitWalletResult{}, wallet.err
	}
	return economy.DebitWalletResult{
		Balance: economy.WalletBalance{
			PlayerID:  input.PlayerID,
			Currency:  input.Currency,
			Balance:   1_000 - input.Amount,
			UpdatedAt: testTime(1),
		},
		LedgerEntry: economy.CurrencyLedgerEntry{
			LedgerID:     "wallet-ledger-1",
			PlayerID:     input.PlayerID,
			Currency:     input.Currency,
			Action:       economy.LedgerActionDecrease,
			BalanceAfter: 1_000 - input.Amount,
			Reason:       input.Reason,
			ReferenceKey: input.ReferenceKey,
			CreatedAt:    testTime(1),
		},
	}, nil
}

type reentrantBuildingWallet struct {
	store       *InMemoryStore
	fake        fakeBuildingWallet
	calls       int
	readStorage bool
}

func (wallet *reentrantBuildingWallet) DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error) {
	_, ok, err := wallet.store.PlanetStorage("planet-1")
	if err != nil {
		return economy.DebitWalletResult{}, err
	}
	wallet.readStorage = ok
	result, err := wallet.fake.DebitWallet(input)
	wallet.calls = wallet.fake.calls
	return result, err
}

type staleBuildInsertWallet struct {
	store    *InMemoryStore
	building PlanetBuilding
	fake     fakeBuildingWallet
	calls    int
}

func (wallet *staleBuildInsertWallet) DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error) {
	if _, _, err := wallet.store.UpsertBuilding(wallet.building); err != nil {
		return economy.DebitWalletResult{}, err
	}
	result, err := wallet.fake.DebitWallet(input)
	wallet.calls = wallet.fake.calls
	return result, err
}

func newBuildingMutationStore(t *testing.T, items []StoredItem) *InMemoryStore {
	t.Helper()
	store := NewInMemoryStore()
	if _, err := store.InitializePlanetProduction(InitializePlanetProductionInput{
		PlanetID:              "planet-1",
		LastCalculatedAt:      testTime(0),
		StorageCapacityUnits:  1_000,
		EnergyCapacityPerHour: 100,
		UpdatedAt:             testTime(0),
	}); err != nil {
		t.Fatalf("InitializePlanetProduction() error = %v, want nil", err)
	}
	storage, ok, err := store.PlanetStorage("planet-1")
	if err != nil || !ok {
		t.Fatalf("PlanetStorage() ok = %v err = %v, want true nil", ok, err)
	}
	for _, item := range items {
		if _, err := storage.AddUpToCapacity(item.ItemID, item.Quantity, testTime(0)); err != nil {
			t.Fatalf("seed storage AddUpToCapacity(%q) error = %v, want nil", item.ItemID, err)
		}
	}
	if err := store.SavePlanetStorage(storage); err != nil {
		t.Fatalf("SavePlanetStorage() error = %v, want nil", err)
	}
	return store
}

func newTestBuildingMutationService(
	t *testing.T,
	store *InMemoryStore,
	catalogRows Catalog,
	wallet BuildingMutationWalletDebiter,
	costs BuildingMutationCostProvider,
) *BuildingMutationService {
	t.Helper()
	service, err := NewBuildingMutationService(BuildingMutationServiceConfig{
		Store:   store,
		Catalog: catalogRows,
		Costs:   costs,
		Wallet:  wallet,
	})
	if err != nil {
		t.Fatalf("NewBuildingMutationService() error = %v, want nil", err)
	}
	return service
}

func testBuildingMutationCatalogWithL2(t *testing.T) Catalog {
	t.Helper()
	definitions := append(MVPProductionDefinitions(), newMVPDefinition(
		"iron_extractor_l2",
		BuildingTypeIronExtractor,
		BuildingCategoryExtractor,
		2,
		nil,
		[]ItemRate{mustItemRate("iron_ore", 60)},
		8,
	))
	catalogRows, err := NewCatalog(definitions)
	if err != nil {
		t.Fatalf("NewCatalog(L2) error = %v, want nil", err)
	}
	return catalogRows
}

func addBuildingMutationBuilding(t *testing.T, store *InMemoryStore, catalogRows Catalog, buildingID BuildingID, definitionID catalog.DefinitionID) {
	t.Helper()
	definition, err := catalogRows.MustGet(definitionID)
	if err != nil {
		t.Fatalf("MustGet(%q) error = %v, want nil", definitionID, err)
	}
	building, err := NewPlanetBuilding(buildingID, "planet-1", definition, BuildingStateActive, testTime(1), testTime(1))
	if err != nil {
		t.Fatalf("NewPlanetBuilding() error = %v, want nil", err)
	}
	if _, _, err := store.UpsertBuilding(building); err != nil {
		t.Fatalf("UpsertBuilding() error = %v, want nil", err)
	}
}

func assertBuildingMutationStorage(t *testing.T, store *InMemoryStore, itemID foundation.ItemID, want int64) {
	t.Helper()
	got := buildingMutationStorageQuantity(t, store, itemID)
	if got != want {
		t.Fatalf("storage %q = %d, want %d", itemID, got, want)
	}
}

func buildingMutationStorageQuantity(t *testing.T, store *InMemoryStore, itemID foundation.ItemID) int64 {
	t.Helper()
	storage, ok, err := store.PlanetStorage("planet-1")
	if err != nil || !ok {
		t.Fatalf("PlanetStorage() ok = %v err = %v, want true nil", ok, err)
	}
	return storage.QuantityOf(itemID)
}

func assertBuildingMaterialLedger(t *testing.T, store *InMemoryStore, reference foundation.IdempotencyKey, itemID foundation.ItemID, quantity int64, balanceAfter int64) {
	t.Helper()
	entries := store.BuildingMaterialLedgerEntries()
	if len(entries) != 1 {
		t.Fatalf("material ledger rows = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.ReferenceKey != reference || entry.ItemID != itemID || entry.Quantity != quantity || entry.BalanceAfter != balanceAfter {
		t.Fatalf("material ledger = %+v, want ref %q item %q qty %d balance %d", entry, reference, itemID, quantity, balanceAfter)
	}
	if len(store.BuildingMutationReferences()) != 1 {
		t.Fatalf("building references = %d, want 1", len(store.BuildingMutationReferences()))
	}
}

func assertBuildingMutationEvents(t *testing.T, store *InMemoryStore, wantTypes ...EventType) {
	t.Helper()
	events := store.Events()
	if len(events) != len(wantTypes) {
		t.Fatalf("events = %d, want %d", len(events), len(wantTypes))
	}
	for index, wantType := range wantTypes {
		if events[index].Type != wantType.String() {
			t.Fatalf("event[%d] type = %q, want %q", index, events[index].Type, wantType)
		}
	}
}

func assertNoBuildingMutationSideEffects(t *testing.T, store *InMemoryStore, wallet *fakeBuildingWallet) {
	t.Helper()
	if _, ok, lookupErr := store.Building("planet-1", "building-1"); lookupErr != nil || ok {
		t.Fatalf("Building() ok = %v err = %v, want false nil", ok, lookupErr)
	}
	if got := len(store.BuildingMaterialLedgerEntries()); got != 0 {
		t.Fatalf("material ledger rows = %d, want 0", got)
	}
	if got := len(store.BuildingMutationReferences()); got != 0 {
		t.Fatalf("building references = %d, want 0", got)
	}
	if got := len(store.Events()); got != 0 {
		t.Fatalf("events = %d, want 0", got)
	}
	if got := len(store.OutboxRecords()); got != 0 {
		t.Fatalf("outbox records = %d, want 0", got)
	}
	if wallet.calls != 0 {
		t.Fatalf("wallet calls = %d, want 0", wallet.calls)
	}
}

func mustBuildingBuildKey(t *testing.T, planetID foundation.PlanetID, buildingID BuildingID) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.PlanetBuildingBuildIdempotencyKey(planetID, buildingID.String())
	if err != nil {
		t.Fatalf("PlanetBuildingBuildIdempotencyKey() error = %v, want nil", err)
	}
	return key
}

func mustBuildingUpgradeKey(t *testing.T, planetID foundation.PlanetID, buildingID BuildingID, nextLevel int) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.PlanetBuildingUpgradeIdempotencyKey(planetID, buildingID.String(), nextLevel)
	if err != nil {
		t.Fatalf("PlanetBuildingUpgradeIdempotencyKey() error = %v, want nil", err)
	}
	return key
}

func mustQuestRewardKeyForBuildingMutationTest(t *testing.T, questID foundation.QuestID) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.QuestRewardIdempotencyKey(questID)
	if err != nil {
		t.Fatalf("QuestRewardIdempotencyKey() error = %v, want nil", err)
	}
	return key
}

package production

import (
	"fmt"
	"sort"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	gameevents "gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

const (
	BuildingMutationBuild   BuildingMutationKind = "build"
	BuildingMutationUpgrade BuildingMutationKind = "upgrade"

	buildingBuildReason   economy.LedgerReason = "planet_building_build"
	buildingUpgradeReason economy.LedgerReason = "planet_building_upgrade"
)

// BuildingMutationKind identifies the server-owned building mutation path.
type BuildingMutationKind string

// BuildingMaterialCost records one planet-storage material requirement.
type BuildingMaterialCost struct {
	ItemID   foundation.ItemID `json:"item_id"`
	Quantity int64             `json:"quantity"`
}

// BuildingWalletCost records an optional wallet fee for a building mutation.
type BuildingWalletCost struct {
	PlayerID foundation.PlayerID    `json:"player_id"`
	Currency economy.CurrencyBucket `json:"currency"`
	Amount   int64                  `json:"amount"`
}

// BuildingMutationCost is returned from server-owned building cost config.
type BuildingMutationCost struct {
	Materials []BuildingMaterialCost `json:"materials,omitempty"`
	Wallet    *BuildingWalletCost    `json:"wallet,omitempty"`
}

// BuildingMutationCostInput is the context passed to server-owned cost config.
type BuildingMutationCostInput struct {
	Operation  BuildingMutationKind
	PlanetID   foundation.PlanetID
	BuildingID BuildingID
	Existing   *PlanetBuilding
	Definition BuildingProductionDefinition
}

// BuildingMutationCostProvider resolves costs from server-side config.
type BuildingMutationCostProvider interface {
	BuildingMutationCost(input BuildingMutationCostInput) (BuildingMutationCost, error)
}

// BuildingMutationWalletDebiter is the wallet boundary used for optional fees.
type BuildingMutationWalletDebiter interface {
	DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error)
}

// BuildingMutationServiceConfig wires building mutations to store, catalog, and economy boundaries.
type BuildingMutationServiceConfig struct {
	Store   *InMemoryStore
	Catalog Catalog
	Costs   BuildingMutationCostProvider
	Wallet  BuildingMutationWalletDebiter
}

// BuildingMutationService owns in-memory planet building build/upgrade mutations.
type BuildingMutationService struct {
	store   *InMemoryStore
	catalog Catalog
	costs   BuildingMutationCostProvider
	wallet  BuildingMutationWalletDebiter
}

// BuildPlanetBuildingInput describes a server-authoritative building create request.
type BuildPlanetBuildingInput struct {
	PlanetID     foundation.PlanetID       `json:"planet_id"`
	BuildingID   BuildingID                `json:"building_id"`
	DefinitionID catalog.DefinitionID      `json:"definition_id,omitempty"`
	BuildingType BuildingType              `json:"building_type,omitempty"`
	Level        int                       `json:"level,omitempty"`
	RequestedAt  time.Time                 `json:"requested_at"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_key"`
}

// UpgradePlanetBuildingInput describes a server-authoritative building upgrade request.
type UpgradePlanetBuildingInput struct {
	PlanetID     foundation.PlanetID       `json:"planet_id"`
	BuildingID   BuildingID                `json:"building_id"`
	DefinitionID catalog.DefinitionID      `json:"definition_id,omitempty"`
	NextLevel    int                       `json:"next_level,omitempty"`
	RequestedAt  time.Time                 `json:"requested_at"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_key"`
}

// BuildingMaterialLedgerEntry is the production-local ledger for planet-storage debits.
type BuildingMaterialLedgerEntry struct {
	LedgerID     string                    `json:"ledger_id"`
	Operation    BuildingMutationKind      `json:"operation"`
	PlanetID     foundation.PlanetID       `json:"planet_id"`
	BuildingID   BuildingID                `json:"building_id"`
	ItemID       foundation.ItemID         `json:"item_id"`
	Quantity     int64                     `json:"quantity"`
	BalanceAfter int64                     `json:"balance_after"`
	Reason       economy.LedgerReason      `json:"reason"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_key"`
	CreatedAt    time.Time                 `json:"created_at"`
}

// BuildingMutationResult reports the committed building mutation and audit evidence.
type BuildingMutationResult struct {
	Operation      BuildingMutationKind          `json:"operation"`
	Building       PlanetBuilding                `json:"building"`
	Storage        PlanetStorage                 `json:"storage"`
	Definition     BuildingProductionDefinition  `json:"definition"`
	MaterialLedger []BuildingMaterialLedgerEntry `json:"material_ledger,omitempty"`
	WalletDebit    *economy.DebitWalletResult    `json:"wallet_debit,omitempty"`
	Events         []gameevents.EventEnvelope    `json:"events,omitempty"`
	OutboxRecords  []ProductionOutboxRecord      `json:"-"`
	ReferenceKey   foundation.IdempotencyKey     `json:"reference_key"`
	Duplicate      bool                          `json:"duplicate"`
}

// BuildingMutationReferenceRecord caches one applied build/upgrade reference.
type BuildingMutationReferenceRecord struct {
	ReferenceKey foundation.IdempotencyKey
	Operation    BuildingMutationKind
	PlanetID     foundation.PlanetID
	BuildingID   BuildingID
	Result       BuildingMutationResult
	RecordedAt   time.Time
}

// NewBuildingMutationService returns a building mutation domain service.
func NewBuildingMutationService(config BuildingMutationServiceConfig) (*BuildingMutationService, error) {
	if config.Store == nil {
		return nil, ErrInvalidBuildingMutationConfig
	}
	if config.Costs == nil {
		return nil, ErrInvalidBuildingMutationConfig
	}
	if len(config.Catalog.Definitions()) == 0 {
		return nil, ErrInvalidBuildingMutationConfig
	}
	return &BuildingMutationService{
		store:   config.Store,
		catalog: config.Catalog,
		costs:   config.Costs,
		wallet:  config.Wallet,
	}, nil
}

// BuildPlanetBuilding creates one active planet building after debiting configured costs.
func (service *BuildingMutationService) BuildPlanetBuilding(input BuildPlanetBuildingInput) (BuildingMutationResult, error) {
	if service == nil {
		return BuildingMutationResult{}, ErrInvalidBuildingMutationConfig
	}
	if err := input.validate(); err != nil {
		return BuildingMutationResult{}, err
	}
	definition, err := resolveBuildDefinition(service.catalog, input.DefinitionID, input.BuildingType, input.Level)
	if err != nil {
		return BuildingMutationResult{}, err
	}
	building, err := NewPlanetBuilding(input.BuildingID, input.PlanetID, definition, BuildingStateActive, input.RequestedAt, input.RequestedAt)
	if err != nil {
		return BuildingMutationResult{}, err
	}
	if err := validateBuildMutationReferenceKey(input); err != nil {
		return BuildingMutationResult{}, err
	}

	if result, duplicate, err := service.store.lookupBuildMutationReplayOrConflict(input, definition); err != nil || duplicate {
		return result, err
	}

	cost, err := service.costs.BuildingMutationCost(BuildingMutationCostInput{
		Operation:  BuildingMutationBuild,
		PlanetID:   input.PlanetID,
		BuildingID: input.BuildingID,
		Definition: definition,
	})
	if err != nil {
		return BuildingMutationResult{}, err
	}
	if err := cost.Validate(); err != nil {
		return BuildingMutationResult{}, err
	}

	if result, duplicate, err := service.store.preflightBuildPlanetBuilding(input, building, definition, cost); err != nil || duplicate {
		return result, err
	}
	walletDebit, err := debitBuildingMutationWallet(BuildingMutationBuild, input.ReferenceKey, cost.Wallet, service.wallet)
	if err != nil {
		return BuildingMutationResult{}, err
	}
	return service.store.commitBuildPlanetBuilding(input, building, definition, cost, walletDebit)
}

// UpgradePlanetBuilding replaces one building with its next-level catalog definition.
func (service *BuildingMutationService) UpgradePlanetBuilding(input UpgradePlanetBuildingInput) (BuildingMutationResult, error) {
	if service == nil {
		return BuildingMutationResult{}, ErrInvalidBuildingMutationConfig
	}
	if err := input.validate(); err != nil {
		return BuildingMutationResult{}, err
	}

	existing, result, duplicate, err := service.store.lookupUpgradeMutationExistingOrReplay(input)
	if err != nil || duplicate {
		return result, err
	}
	nextDefinition, err := resolveUpgradeDefinition(service.catalog, existing, input.DefinitionID, input.NextLevel)
	if err != nil {
		return BuildingMutationResult{}, err
	}
	if err := validateUpgradeMutationReferenceKey(input, nextDefinition.Level); err != nil {
		return BuildingMutationResult{}, err
	}
	upgraded := existing
	upgraded.Source = nextDefinition.Source
	upgraded.BuildingType = nextDefinition.BuildingType
	upgraded.Level = nextDefinition.Level
	upgraded.UpdatedAt = input.RequestedAt.UTC()
	if err := upgraded.Validate(); err != nil {
		return BuildingMutationResult{}, err
	}

	cost, err := service.costs.BuildingMutationCost(BuildingMutationCostInput{
		Operation:  BuildingMutationUpgrade,
		PlanetID:   input.PlanetID,
		BuildingID: input.BuildingID,
		Existing:   &existing,
		Definition: nextDefinition,
	})
	if err != nil {
		return BuildingMutationResult{}, err
	}
	if err := cost.Validate(); err != nil {
		return BuildingMutationResult{}, err
	}

	if result, duplicate, err := service.store.preflightUpgradePlanetBuilding(input, existing, nextDefinition, cost); err != nil || duplicate {
		return result, err
	}
	walletDebit, err := debitBuildingMutationWallet(BuildingMutationUpgrade, input.ReferenceKey, cost.Wallet, service.wallet)
	if err != nil {
		return BuildingMutationResult{}, err
	}
	return service.store.commitUpgradePlanetBuilding(input, existing, upgraded, nextDefinition, cost, walletDebit)
}

// BuildingMutationReferences returns all recorded building mutation references in key order.
func (store *InMemoryStore) BuildingMutationReferences() []BuildingMutationReferenceRecord {
	store.mu.RLock()
	defer store.mu.RUnlock()

	if len(store.buildingReferences) == 0 {
		return nil
	}
	keys := make([]foundation.IdempotencyKey, 0, len(store.buildingReferences))
	for referenceKey := range store.buildingReferences {
		keys = append(keys, referenceKey)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})
	records := make([]BuildingMutationReferenceRecord, 0, len(keys))
	for _, referenceKey := range keys {
		records = append(records, cloneBuildingMutationReferenceRecord(store.buildingReferences[referenceKey]))
	}
	return records
}

// BuildingMutationReference returns one recorded building mutation reference.
func (store *InMemoryStore) BuildingMutationReference(
	referenceKey foundation.IdempotencyKey,
) (BuildingMutationReferenceRecord, bool, error) {
	if err := referenceKey.Validate(); err != nil {
		return BuildingMutationReferenceRecord{}, false, err
	}
	store.mu.RLock()
	defer store.mu.RUnlock()

	record, ok := store.buildingReferences[referenceKey]
	if !ok {
		return BuildingMutationReferenceRecord{}, false, nil
	}
	return cloneBuildingMutationReferenceRecord(record), true, nil
}

// BuildingMaterialLedgerEntries returns building material ledger rows in append order.
func (store *InMemoryStore) BuildingMaterialLedgerEntries() []BuildingMaterialLedgerEntry {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return cloneBuildingMaterialLedgerEntries(store.buildingMaterialLedger)
}

func (store *InMemoryStore) lookupBuildMutationReplayOrConflict(
	input BuildPlanetBuildingInput,
	definition BuildingProductionDefinition,
) (BuildingMutationResult, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if result, duplicate, err := store.replayBuildMutationLocked(input, definition); duplicate || err != nil {
		return result, duplicate, err
	}
	if _, ok := store.buildings[input.PlanetID][input.BuildingID]; ok {
		return BuildingMutationResult{}, false, fmt.Errorf("building %q: %w", input.BuildingID, ErrDuplicateBuilding)
	}
	return BuildingMutationResult{}, false, nil
}

func (store *InMemoryStore) lookupUpgradeMutationExistingOrReplay(
	input UpgradePlanetBuildingInput,
) (PlanetBuilding, BuildingMutationResult, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if result, duplicate, err := store.replayUpgradeMutationLocked(input); duplicate || err != nil {
		return PlanetBuilding{}, result, duplicate, err
	}
	existing, ok := store.buildings[input.PlanetID][input.BuildingID]
	if !ok {
		return PlanetBuilding{}, BuildingMutationResult{}, false, fmt.Errorf("building %q: %w", input.BuildingID, ErrBuildingNotFound)
	}
	return clonePlanetBuilding(existing), BuildingMutationResult{}, false, nil
}

func (store *InMemoryStore) preflightBuildPlanetBuilding(
	input BuildPlanetBuildingInput,
	building PlanetBuilding,
	definition BuildingProductionDefinition,
	cost BuildingMutationCost,
) (BuildingMutationResult, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if result, duplicate, err := store.replayBuildMutationLocked(input, definition); duplicate || err != nil {
		return result, duplicate, err
	}
	if _, ok := store.buildings[input.PlanetID][input.BuildingID]; ok {
		return BuildingMutationResult{}, false, fmt.Errorf("building %q: %w", input.BuildingID, ErrDuplicateBuilding)
	}
	if err := store.validateBuildingMutationResourcesLocked(BuildingMutationBuild, input.ReferenceKey, input.RequestedAt, building, cost); err != nil {
		return BuildingMutationResult{}, false, err
	}
	return BuildingMutationResult{}, false, nil
}

func (store *InMemoryStore) preflightUpgradePlanetBuilding(
	input UpgradePlanetBuildingInput,
	existing PlanetBuilding,
	definition BuildingProductionDefinition,
	cost BuildingMutationCost,
) (BuildingMutationResult, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if result, duplicate, err := store.replayUpgradeMutationLocked(input); duplicate || err != nil {
		return result, duplicate, err
	}
	current, ok := store.buildings[input.PlanetID][input.BuildingID]
	if !ok {
		return BuildingMutationResult{}, false, fmt.Errorf("building %q: %w", input.BuildingID, ErrBuildingNotFound)
	}
	if !samePlanetBuildingForMutation(current, existing) {
		return BuildingMutationResult{}, false, fmt.Errorf("building %q changed before upgrade preflight: %w", input.BuildingID, ErrStaleBuildingMutation)
	}
	if err := store.validateBuildingMutationResourcesLocked(BuildingMutationUpgrade, input.ReferenceKey, input.RequestedAt, existing, cost); err != nil {
		return BuildingMutationResult{}, false, err
	}
	return BuildingMutationResult{}, false, nil
}

func (store *InMemoryStore) commitBuildPlanetBuilding(
	input BuildPlanetBuildingInput,
	building PlanetBuilding,
	definition BuildingProductionDefinition,
	cost BuildingMutationCost,
	walletDebit *economy.DebitWalletResult,
) (BuildingMutationResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if result, duplicate, err := store.replayBuildMutationLocked(input, definition); duplicate || err != nil {
		return result, err
	}
	if _, ok := store.buildings[input.PlanetID][input.BuildingID]; ok {
		return BuildingMutationResult{}, fmt.Errorf("building %q changed before build commit: %w", input.BuildingID, ErrStaleBuildingMutation)
	}
	return store.applyBuildingMutationLocked(BuildingMutationBuild, input.ReferenceKey, input.RequestedAt, building, definition, cost, walletDebit)
}

func (store *InMemoryStore) commitUpgradePlanetBuilding(
	input UpgradePlanetBuildingInput,
	existing PlanetBuilding,
	upgraded PlanetBuilding,
	definition BuildingProductionDefinition,
	cost BuildingMutationCost,
	walletDebit *economy.DebitWalletResult,
) (BuildingMutationResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureMapsLocked()

	if result, duplicate, err := store.replayUpgradeMutationLocked(input); duplicate || err != nil {
		return result, err
	}
	current, ok := store.buildings[input.PlanetID][input.BuildingID]
	if !ok {
		return BuildingMutationResult{}, fmt.Errorf("building %q: %w", input.BuildingID, ErrBuildingNotFound)
	}
	if !samePlanetBuildingForMutation(current, existing) {
		return BuildingMutationResult{}, fmt.Errorf("building %q changed before upgrade commit: %w", input.BuildingID, ErrStaleBuildingMutation)
	}
	return store.applyBuildingMutationLocked(BuildingMutationUpgrade, input.ReferenceKey, input.RequestedAt, upgraded, definition, cost, walletDebit)
}

func (store *InMemoryStore) validateBuildingMutationResourcesLocked(
	operation BuildingMutationKind,
	referenceKey foundation.IdempotencyKey,
	requestedAt time.Time,
	building PlanetBuilding,
	cost BuildingMutationCost,
) error {
	if err := cost.Validate(); err != nil {
		return err
	}
	storage, ok := store.storage[building.PlanetID]
	if !ok {
		return fmt.Errorf("planet %q storage: %w", building.PlanetID, ErrProductionSnapshotIncomplete)
	}
	storage = clonePlanetStorage(storage)
	if err := validateBuildingMaterialsAvailable(storage, cost.Materials); err != nil {
		return err
	}
	_, _, _, err := store.previewBuildingMaterialDebitLocked(operation, building.PlanetID, building.BuildingID, storage, cost.Materials, requestedAt, referenceKey)
	return err
}

func (store *InMemoryStore) replayBuildMutationLocked(
	input BuildPlanetBuildingInput,
	definition BuildingProductionDefinition,
) (BuildingMutationResult, bool, error) {
	previous, ok := store.buildingReferences[input.ReferenceKey]
	if !ok {
		return BuildingMutationResult{}, false, nil
	}
	if err := validateBuildMutationReferenceReplay(previous, input, definition); err != nil {
		return BuildingMutationResult{}, false, err
	}
	result := cloneBuildingMutationResult(previous.Result)
	result.Duplicate = true
	result.OutboxRecords = nil
	return result, true, nil
}

func (store *InMemoryStore) replayUpgradeMutationLocked(input UpgradePlanetBuildingInput) (BuildingMutationResult, bool, error) {
	previous, ok := store.buildingReferences[input.ReferenceKey]
	if !ok {
		return BuildingMutationResult{}, false, nil
	}
	if err := validateUpgradeMutationReferenceReplay(previous, input); err != nil {
		return BuildingMutationResult{}, false, err
	}
	result := cloneBuildingMutationResult(previous.Result)
	result.Duplicate = true
	result.OutboxRecords = nil
	return result, true, nil
}

func debitBuildingMutationWallet(
	operation BuildingMutationKind,
	referenceKey foundation.IdempotencyKey,
	cost *BuildingWalletCost,
	wallet BuildingMutationWalletDebiter,
) (*economy.DebitWalletResult, error) {
	if cost == nil {
		return nil, nil
	}
	if wallet == nil {
		return nil, ErrInvalidBuildingMutationConfig
	}
	debit, err := wallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     cost.PlayerID,
		Currency:     cost.Currency,
		Amount:       cost.Amount,
		Reason:       buildingMutationReason(operation),
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return nil, err
	}
	return &debit, nil
}

func (store *InMemoryStore) applyBuildingMutationLocked(
	operation BuildingMutationKind,
	referenceKey foundation.IdempotencyKey,
	requestedAt time.Time,
	building PlanetBuilding,
	definition BuildingProductionDefinition,
	cost BuildingMutationCost,
	walletDebit *economy.DebitWalletResult,
) (BuildingMutationResult, error) {
	if err := cost.Validate(); err != nil {
		return BuildingMutationResult{}, err
	}
	outboxCountBefore := len(store.outbox)
	storage, ok := store.storage[building.PlanetID]
	if !ok {
		return BuildingMutationResult{}, fmt.Errorf("planet %q storage: %w", building.PlanetID, ErrProductionSnapshotIncomplete)
	}
	storage = clonePlanetStorage(storage)
	if err := validateBuildingMaterialsAvailable(storage, cost.Materials); err != nil {
		return BuildingMutationResult{}, err
	}
	materialLedger, afterStorage, nextLedgerSequence, err := store.previewBuildingMaterialDebitLocked(operation, building.PlanetID, building.BuildingID, storage, cost.Materials, requestedAt, referenceKey)
	if err != nil {
		return BuildingMutationResult{}, err
	}
	var storagePayload StorageUpdatedPayload
	if len(materialLedger) > 0 {
		storagePayload, err = NewStorageUpdatedPayload(afterStorage)
		if err != nil {
			return BuildingMutationResult{}, err
		}
	}
	buildingPayload, err := NewBuildingUpdatedPayload(building)
	if err != nil {
		return BuildingMutationResult{}, err
	}

	store.storage[building.PlanetID] = clonePlanetStorage(afterStorage)
	if store.buildings[building.PlanetID] == nil {
		store.buildings[building.PlanetID] = make(map[BuildingID]PlanetBuilding)
	}
	store.buildings[building.PlanetID][building.BuildingID] = clonePlanetBuilding(building)
	store.nextBuildingLedgerSequence = nextLedgerSequence
	store.buildingMaterialLedger = append(store.buildingMaterialLedger, cloneBuildingMaterialLedgerEntries(materialLedger)...)

	events := make([]gameevents.EventEnvelope, 0, 2)
	if len(materialLedger) > 0 {
		event, err := store.appendProductionEventWithOutboxEvidenceLocked(EventPlanetStorageUpdated, storagePayload, requestedAt, referenceKey, "")
		if err != nil {
			return BuildingMutationResult{}, err
		}
		events = append(events, event)
	}
	event, err := store.appendProductionEventWithOutboxEvidenceLocked(EventPlanetBuildingUpdated, buildingPayload, requestedAt, referenceKey, "")
	if err != nil {
		return BuildingMutationResult{}, err
	}
	events = append(events, event)

	result := BuildingMutationResult{
		Operation:      operation,
		Building:       clonePlanetBuilding(building),
		Storage:        clonePlanetStorage(afterStorage),
		Definition:     cloneDefinition(definition),
		MaterialLedger: cloneBuildingMaterialLedgerEntries(materialLedger),
		WalletDebit:    cloneDebitWalletResultPtr(walletDebit),
		Events:         cloneProductionEventEnvelopes(events),
		OutboxRecords:  cloneProductionOutboxRecords(store.outbox[outboxCountBefore:]),
		ReferenceKey:   referenceKey,
	}
	store.buildingReferences[referenceKey] = BuildingMutationReferenceRecord{
		ReferenceKey: referenceKey,
		Operation:    operation,
		PlanetID:     building.PlanetID,
		BuildingID:   building.BuildingID,
		Result:       cloneBuildingMutationResult(result),
		RecordedAt:   requestedAt.UTC(),
	}
	return cloneBuildingMutationResult(result), nil
}

func (store *InMemoryStore) previewBuildingMaterialDebitLocked(
	operation BuildingMutationKind,
	planetID foundation.PlanetID,
	buildingID BuildingID,
	storage PlanetStorage,
	materials []BuildingMaterialCost,
	requestedAt time.Time,
	referenceKey foundation.IdempotencyKey,
) ([]BuildingMaterialLedgerEntry, PlanetStorage, uint64, error) {
	after := clonePlanetStorage(storage)
	ledger := make([]BuildingMaterialLedgerEntry, 0, len(materials))
	nextSequence := store.nextBuildingLedgerSequence
	for _, material := range materials {
		removed, err := after.RemoveUpTo(material.ItemID, material.Quantity, requestedAt)
		if err != nil {
			return nil, PlanetStorage{}, 0, err
		}
		if removed != material.Quantity {
			return nil, PlanetStorage{}, 0, fmt.Errorf("item %q have %d need %d: %w", material.ItemID, removed, material.Quantity, ErrInsufficientBuildingMaterials)
		}
		nextSequence++
		ledger = append(ledger, BuildingMaterialLedgerEntry{
			LedgerID:     fmt.Sprintf("building-material-ledger-%d", nextSequence),
			Operation:    operation,
			PlanetID:     planetID,
			BuildingID:   buildingID,
			ItemID:       material.ItemID,
			Quantity:     material.Quantity,
			BalanceAfter: after.QuantityOf(material.ItemID),
			Reason:       buildingMutationReason(operation),
			ReferenceKey: referenceKey,
			CreatedAt:    requestedAt.UTC(),
		})
	}
	for _, entry := range ledger {
		if err := entry.Validate(); err != nil {
			return nil, PlanetStorage{}, 0, err
		}
	}
	return ledger, after, nextSequence, nil
}

func resolveBuildDefinition(catalogRows Catalog, definitionID catalog.DefinitionID, buildingType BuildingType, level int) (BuildingProductionDefinition, error) {
	var definition BuildingProductionDefinition
	var ok bool
	if !definitionID.IsZero() {
		if err := definitionID.Validate(); err != nil {
			return BuildingProductionDefinition{}, err
		}
		definition, ok = catalogRows.Get(definitionID)
		if !ok {
			return BuildingProductionDefinition{}, fmt.Errorf("building definition %q: %w", definitionID, ErrUnknownBuildingDefinition)
		}
		if buildingType != "" && definition.BuildingType != buildingType {
			return BuildingProductionDefinition{}, fmt.Errorf("building type %q definition %q has %q: %w", buildingType, definitionID, definition.BuildingType, ErrBuildingSourceMismatch)
		}
		if level > 0 && definition.Level != level {
			return BuildingProductionDefinition{}, fmt.Errorf("building level %d definition %q has %d: %w", level, definitionID, definition.Level, ErrBuildingSourceMismatch)
		}
		return definition, nil
	}
	if err := buildingType.Validate(); err != nil {
		return BuildingProductionDefinition{}, err
	}
	if level <= 0 {
		return BuildingProductionDefinition{}, fmt.Errorf("building level %d: %w", level, ErrInvalidBuildingLevel)
	}
	definition, ok = catalogRows.GetBuilding(buildingType, level)
	if !ok {
		return BuildingProductionDefinition{}, fmt.Errorf("building %q level %d: %w", buildingType, level, ErrUnknownBuildingDefinition)
	}
	return definition, nil
}

func resolveUpgradeDefinition(catalogRows Catalog, existing PlanetBuilding, definitionID catalog.DefinitionID, nextLevel int) (BuildingProductionDefinition, error) {
	targetLevel := existing.Level + 1
	if nextLevel > 0 {
		targetLevel = nextLevel
	}
	if targetLevel != existing.Level+1 {
		return BuildingProductionDefinition{}, fmt.Errorf("building %q target level %d from %d: %w", existing.BuildingID, targetLevel, existing.Level, ErrInvalidBuildingLevel)
	}
	if !definitionID.IsZero() {
		definition, ok := catalogRows.Get(definitionID)
		if !ok {
			return BuildingProductionDefinition{}, fmt.Errorf("building definition %q: %w", definitionID, ErrUnknownBuildingDefinition)
		}
		if definition.BuildingType != existing.BuildingType || definition.Level != targetLevel {
			return BuildingProductionDefinition{}, fmt.Errorf("building %q definition %q: %w", existing.BuildingID, definitionID, ErrBuildingSourceMismatch)
		}
		return definition, nil
	}
	definition, ok := catalogRows.GetBuilding(existing.BuildingType, targetLevel)
	if !ok {
		return BuildingProductionDefinition{}, fmt.Errorf("building %q next level %d: %w", existing.BuildingID, targetLevel, ErrUnknownBuildingDefinition)
	}
	return definition, nil
}

func validateBuildMutationReferenceReplay(record BuildingMutationReferenceRecord, input BuildPlanetBuildingInput, definition BuildingProductionDefinition) error {
	if record.Operation != BuildingMutationBuild || record.Result.Operation != BuildingMutationBuild {
		return buildingMutationReferenceMismatch(input.ReferenceKey, "operation")
	}
	if record.PlanetID != input.PlanetID || record.Result.Building.PlanetID != input.PlanetID {
		return buildingMutationReferenceMismatch(input.ReferenceKey, "planet")
	}
	if record.BuildingID != input.BuildingID || record.Result.Building.BuildingID != input.BuildingID {
		return buildingMutationReferenceMismatch(input.ReferenceKey, "building")
	}
	if record.Result.Definition.DefinitionID != definition.DefinitionID ||
		record.Result.Building.Source.DefinitionID != definition.DefinitionID ||
		record.Result.Building.BuildingType != definition.BuildingType ||
		record.Result.Building.Level != definition.Level {
		return buildingMutationReferenceMismatch(input.ReferenceKey, "definition")
	}
	return nil
}

func validateUpgradeMutationReferenceReplay(record BuildingMutationReferenceRecord, input UpgradePlanetBuildingInput) error {
	if record.Operation != BuildingMutationUpgrade || record.Result.Operation != BuildingMutationUpgrade {
		return buildingMutationReferenceMismatch(input.ReferenceKey, "operation")
	}
	if record.PlanetID != input.PlanetID || record.Result.Building.PlanetID != input.PlanetID {
		return buildingMutationReferenceMismatch(input.ReferenceKey, "planet")
	}
	if record.BuildingID != input.BuildingID || record.Result.Building.BuildingID != input.BuildingID {
		return buildingMutationReferenceMismatch(input.ReferenceKey, "building")
	}
	if !input.DefinitionID.IsZero() && input.DefinitionID != record.Result.Definition.DefinitionID {
		return buildingMutationReferenceMismatch(input.ReferenceKey, "definition")
	}
	if input.NextLevel > 0 && (input.NextLevel != record.Result.Definition.Level || input.NextLevel != record.Result.Building.Level) {
		return buildingMutationReferenceMismatch(input.ReferenceKey, "level")
	}
	return nil
}

func buildingMutationReferenceMismatch(referenceKey foundation.IdempotencyKey, field string) error {
	return fmt.Errorf("building mutation reference %q %s mismatch: %w", referenceKey, field, ErrInvalidBuildingMutationReference)
}

func validateBuildMutationReferenceKey(input BuildPlanetBuildingInput) error {
	if err := foundation.ValidatePlanetBuildingBuildIdempotencyKey(input.ReferenceKey, input.PlanetID, input.BuildingID.String()); err != nil {
		return buildingMutationReferenceInvalid(input.ReferenceKey, err)
	}
	return nil
}

func validateUpgradeMutationReferenceKey(input UpgradePlanetBuildingInput, resolvedNextLevel int) error {
	if err := foundation.ValidatePlanetBuildingUpgradeIdempotencyKey(input.ReferenceKey, input.PlanetID, input.BuildingID.String(), resolvedNextLevel); err != nil {
		return buildingMutationReferenceInvalid(input.ReferenceKey, err)
	}
	return nil
}

func buildingMutationReferenceInvalid(referenceKey foundation.IdempotencyKey, err error) error {
	return fmt.Errorf("building mutation reference %q: %v: %w", referenceKey, err, ErrInvalidBuildingMutationReference)
}

func samePlanetBuildingForMutation(current PlanetBuilding, expected PlanetBuilding) bool {
	return current.BuildingID == expected.BuildingID &&
		current.PlanetID == expected.PlanetID &&
		current.Source == expected.Source &&
		current.BuildingType == expected.BuildingType &&
		current.Level == expected.Level &&
		current.State == expected.State &&
		current.DisabledReason == expected.DisabledReason &&
		current.CreatedAt.Equal(expected.CreatedAt) &&
		current.UpdatedAt.Equal(expected.UpdatedAt)
}

func validateBuildingMaterialsAvailable(storage PlanetStorage, materials []BuildingMaterialCost) error {
	for _, material := range materials {
		if have := storage.QuantityOf(material.ItemID); have < material.Quantity {
			return fmt.Errorf("item %q have %d need %d: %w", material.ItemID, have, material.Quantity, ErrInsufficientBuildingMaterials)
		}
	}
	return nil
}

func (input BuildPlanetBuildingInput) validate() error {
	if err := input.PlanetID.Validate(); err != nil {
		return err
	}
	if err := input.BuildingID.Validate(); err != nil {
		return err
	}
	if !input.DefinitionID.IsZero() {
		if err := input.DefinitionID.Validate(); err != nil {
			return err
		}
	} else if err := input.BuildingType.Validate(); err != nil {
		return err
	}
	if input.Level < 0 {
		return fmt.Errorf("building level %d: %w", input.Level, ErrInvalidBuildingLevel)
	}
	if input.DefinitionID.IsZero() && input.Level == 0 {
		return fmt.Errorf("building level %d: %w", input.Level, ErrInvalidBuildingLevel)
	}
	if input.RequestedAt.IsZero() {
		return fmt.Errorf("requested_at: %w", ErrZeroProductionTimestamp)
	}
	return input.ReferenceKey.Validate()
}

func (input UpgradePlanetBuildingInput) validate() error {
	if err := input.PlanetID.Validate(); err != nil {
		return err
	}
	if err := input.BuildingID.Validate(); err != nil {
		return err
	}
	if !input.DefinitionID.IsZero() {
		if err := input.DefinitionID.Validate(); err != nil {
			return err
		}
	}
	if input.NextLevel < 0 {
		return fmt.Errorf("next level %d: %w", input.NextLevel, ErrInvalidBuildingLevel)
	}
	if input.RequestedAt.IsZero() {
		return fmt.Errorf("requested_at: %w", ErrZeroProductionTimestamp)
	}
	return input.ReferenceKey.Validate()
}

// Validate reports whether kind is a supported building mutation operation.
func (kind BuildingMutationKind) Validate() error {
	switch kind {
	case BuildingMutationBuild, BuildingMutationUpgrade:
		return nil
	default:
		return fmt.Errorf("building mutation operation %q: %w", kind, ErrInvalidBuildingMutationCost)
	}
}

// Validate reports whether material cost names a positive item quantity.
func (cost BuildingMaterialCost) Validate() error {
	if err := cost.ItemID.Validate(); err != nil {
		return err
	}
	return validatePositiveBoundedAmount("building material quantity", cost.Quantity, ErrInvalidBuildingMutationCost)
}

// Validate reports whether optional wallet cost is complete.
func (cost BuildingWalletCost) Validate() error {
	if err := cost.PlayerID.Validate(); err != nil {
		return err
	}
	if err := cost.Currency.Validate(); err != nil {
		return err
	}
	return validatePositiveBoundedAmount("building wallet amount", cost.Amount, ErrInvalidBuildingMutationCost)
}

// Validate reports whether configured costs are complete and non-duplicated.
func (cost BuildingMutationCost) Validate() error {
	seen := make(map[foundation.ItemID]struct{}, len(cost.Materials))
	for _, material := range cost.Materials {
		if err := material.Validate(); err != nil {
			return err
		}
		if _, ok := seen[material.ItemID]; ok {
			return fmt.Errorf("building material item %q: %w", material.ItemID, ErrDuplicateBuildingInput)
		}
		seen[material.ItemID] = struct{}{}
	}
	if cost.Wallet != nil {
		return cost.Wallet.Validate()
	}
	return nil
}

// Validate reports whether a production-local material ledger row is complete.
func (entry BuildingMaterialLedgerEntry) Validate() error {
	if entry.LedgerID == "" {
		return economy.ErrEmptyLedgerID
	}
	if err := entry.Operation.Validate(); err != nil {
		return err
	}
	if err := entry.PlanetID.Validate(); err != nil {
		return err
	}
	if err := entry.BuildingID.Validate(); err != nil {
		return err
	}
	if err := entry.ItemID.Validate(); err != nil {
		return err
	}
	if err := validatePositiveBoundedAmount("building material ledger quantity", entry.Quantity, ErrInvalidBuildingMutationCost); err != nil {
		return err
	}
	if entry.BalanceAfter < 0 {
		return fmt.Errorf("balance after %d: %w", entry.BalanceAfter, economy.ErrNegativeBalance)
	}
	if err := entry.Reason.Validate(); err != nil {
		return err
	}
	if err := entry.ReferenceKey.Validate(); err != nil {
		return err
	}
	if entry.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrZeroProductionTimestamp)
	}
	return nil
}

func buildingMutationReason(operation BuildingMutationKind) economy.LedgerReason {
	if operation == BuildingMutationUpgrade {
		return buildingUpgradeReason
	}
	return buildingBuildReason
}

func cloneBuildingMutationReferenceRecord(record BuildingMutationReferenceRecord) BuildingMutationReferenceRecord {
	record.Result = cloneBuildingMutationResult(record.Result)
	record.RecordedAt = record.RecordedAt.UTC()
	return record
}

func cloneBuildingMutationResult(result BuildingMutationResult) BuildingMutationResult {
	result.Building = clonePlanetBuilding(result.Building)
	result.Storage = clonePlanetStorage(result.Storage)
	result.Definition = cloneDefinition(result.Definition)
	result.MaterialLedger = cloneBuildingMaterialLedgerEntries(result.MaterialLedger)
	result.WalletDebit = cloneDebitWalletResultPtr(result.WalletDebit)
	result.Events = cloneProductionEventEnvelopes(result.Events)
	result.OutboxRecords = cloneProductionOutboxRecords(result.OutboxRecords)
	return result
}

func cloneBuildingMaterialLedgerEntries(entries []BuildingMaterialLedgerEntry) []BuildingMaterialLedgerEntry {
	if len(entries) == 0 {
		return nil
	}
	cloned := append([]BuildingMaterialLedgerEntry(nil), entries...)
	for index := range cloned {
		cloned[index].CreatedAt = cloned[index].CreatedAt.UTC()
	}
	return cloned
}

func cloneDebitWalletResultPtr(result *economy.DebitWalletResult) *economy.DebitWalletResult {
	if result == nil {
		return nil
	}
	cloned := *result
	return &cloned
}

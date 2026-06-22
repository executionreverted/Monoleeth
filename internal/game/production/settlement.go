package production

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

const DefaultMaxOfflineSettlementDuration = 72 * time.Hour

type SettlementSkipReason string

const (
	SettlementSkipReasonEnergyInsufficient SettlementSkipReason = "energy_insufficient"
)

type SettlementItemDelta struct {
	ItemID   foundation.ItemID `json:"item_id"`
	Quantity int64             `json:"quantity"`
}

type SettlementSkippedBuilding struct {
	BuildingID            BuildingID           `json:"building_id"`
	Reason                SettlementSkipReason `json:"reason"`
	EnergyUsedPerHour     int64                `json:"energy_used_per_hour"`
	EnergyCostPerHour     int64                `json:"energy_cost_per_hour"`
	EnergyCapacityPerHour int64                `json:"energy_capacity_per_hour"`
}

type PlanetProductionBuildingResult struct {
	BuildingID        BuildingID            `json:"building_id"`
	DefinitionID      catalog.DefinitionID  `json:"definition_id"`
	BuildingType      BuildingType          `json:"building_type"`
	Category          BuildingCategory      `json:"category"`
	Level             int                   `json:"level"`
	ElapsedApplied    time.Duration         `json:"elapsed_applied"`
	EnergyCostPerHour int64                 `json:"energy_cost_per_hour"`
	ProducedItems     []SettlementItemDelta `json:"produced_items,omitempty"`
	ConsumedInputs    []SettlementItemDelta `json:"consumed_inputs,omitempty"`
	Skipped           bool                  `json:"skipped"`
	SkipReason        SettlementSkipReason  `json:"skip_reason,omitempty"`
	InputShortage     bool                  `json:"input_shortage"`
	StorageFull       bool                  `json:"storage_full"`
}

type PlanetProductionSettlementResult struct {
	PlanetID                     foundation.PlanetID              `json:"planet_id"`
	SettledAt                    time.Time                        `json:"settled_at"`
	ReferenceKey                 foundation.IdempotencyKey        `json:"reference_key,omitempty"`
	SettlementWindow             string                           `json:"settlement_window,omitempty"`
	MaxOfflineSettlementDuration time.Duration                    `json:"max_offline_settlement_duration"`
	ElapsedRequested             time.Duration                    `json:"elapsed_requested"`
	ElapsedApplied               time.Duration                    `json:"elapsed_applied"`
	Before                       PlanetProductionSnapshot         `json:"before"`
	After                        PlanetProductionSnapshot         `json:"after"`
	BuildingResults              []PlanetProductionBuildingResult `json:"building_results,omitempty"`
	ProducedItems                []SettlementItemDelta            `json:"produced_items,omitempty"`
	ConsumedInputs               []SettlementItemDelta            `json:"consumed_inputs,omitempty"`
	SkippedBuildings             []SettlementSkippedBuilding      `json:"skipped_buildings,omitempty"`
	ProductionEnabled            bool                             `json:"production_enabled"`
	NoOp                         bool                             `json:"no_op"`
	StorageFull                  bool                             `json:"storage_full"`
	EnergyInsufficient           bool                             `json:"energy_insufficient"`
}

type settlementBuilding struct {
	building   PlanetBuilding
	definition BuildingProductionDefinition
}

// SettlePlanetProduction applies server-timed offline production for one planet.
func (store *InMemoryStore) SettlePlanetProduction(planetID foundation.PlanetID, now time.Time) (PlanetProductionSettlementResult, error) {
	if err := planetID.Validate(); err != nil {
		return PlanetProductionSettlementResult{}, err
	}
	if now.IsZero() {
		return PlanetProductionSettlementResult{}, fmt.Errorf("now: %w", ErrZeroProductionTimestamp)
	}
	catalogRows, err := MVPCatalog()
	if err != nil {
		return PlanetProductionSettlementResult{}, err
	}

	now = now.UTC()
	store.mu.Lock()
	defer store.mu.Unlock()

	return store.settlePlanetProductionLocked(planetID, now, catalogRows, false)
}

// SettlePlanetProductionIfWholeOutputAvailable applies production only when the
// current locked snapshot can produce at least one whole output unit. It is
// intended for query-time reconciliation so frequent polls cannot advance the
// production cursor for fractional zero-output windows.
func (store *InMemoryStore) SettlePlanetProductionIfWholeOutputAvailable(planetID foundation.PlanetID, now time.Time) (PlanetProductionSettlementResult, error) {
	if err := planetID.Validate(); err != nil {
		return PlanetProductionSettlementResult{}, err
	}
	if now.IsZero() {
		return PlanetProductionSettlementResult{}, fmt.Errorf("now: %w", ErrZeroProductionTimestamp)
	}
	catalogRows, err := MVPCatalog()
	if err != nil {
		return PlanetProductionSettlementResult{}, err
	}

	now = now.UTC()
	store.mu.Lock()
	defer store.mu.Unlock()

	return store.settlePlanetProductionLocked(planetID, now, catalogRows, true)
}

func (store *InMemoryStore) settlePlanetProductionLocked(
	planetID foundation.PlanetID,
	now time.Time,
	catalogRows Catalog,
	requireWholeOutput bool,
) (PlanetProductionSettlementResult, error) {
	store.ensureMapsLocked()

	before, ok := store.snapshotLocked(planetID)
	if !ok {
		return PlanetProductionSettlementResult{}, fmt.Errorf("planet %q: %w", planetID, ErrProductionSnapshotIncomplete)
	}
	if err := before.Validate(); err != nil {
		return PlanetProductionSettlementResult{}, err
	}

	state := cloneProductionState(before.State)
	storage := clonePlanetStorage(before.Storage)
	result := newSettlementResult(planetID, now, before)
	result.ProductionEnabled = state.ProductionEnabled
	result.ElapsedRequested = now.Sub(state.LastCalculatedAt)
	if result.ElapsedRequested <= 0 {
		result.NoOp = true
		result.After = before.Clone()
		return result, nil
	}
	elapsedApplied := minDuration(result.ElapsedRequested, DefaultMaxOfflineSettlementDuration)

	if !state.ProductionEnabled {
		if requireWholeOutput {
			result.NoOp = true
			result.After = before.Clone()
			return result, nil
		}
		result.ElapsedApplied = elapsedApplied
		if err := attachProductionSettlementEvidence(&result, state.LastCalculatedAt, state.LastCalculatedAt.Add(result.ElapsedApplied)); err != nil {
			return PlanetProductionSettlementResult{}, err
		}
		if store.hasSettlementReferenceLocked(result.ReferenceKey) {
			result.NoOp = true
			result.ElapsedApplied = 0
			result.After = before.Clone()
			return result, nil
		}
		state.LastCalculatedAt = now
		state.UpdatedAt = now
		store.states[planetID] = cloneProductionState(state)
		result.After, _ = store.snapshotLocked(planetID)
		store.recordSettlementReferenceLocked(productionSettlementReferenceRecord(result))
		if err := store.appendProductionSettlementEventsLocked(result); err != nil {
			return PlanetProductionSettlementResult{}, err
		}
		return result, nil
	}

	active, err := settlementBuildings(before.Buildings, state.BuildingPriority, catalogRows)
	if err != nil {
		return PlanetProductionSettlementResult{}, err
	}
	if requireWholeOutput && !settlementBuildingsCanProduceWholeOutput(active, elapsedApplied) {
		result.NoOp = true
		result.After = before.Clone()
		return result, nil
	}
	result.ElapsedApplied = elapsedApplied
	if err := attachProductionSettlementEvidence(&result, state.LastCalculatedAt, state.LastCalculatedAt.Add(result.ElapsedApplied)); err != nil {
		return PlanetProductionSettlementResult{}, err
	}
	if store.hasSettlementReferenceLocked(result.ReferenceKey) {
		result.NoOp = true
		result.ElapsedApplied = 0
		result.After = before.Clone()
		return result, nil
	}

	energyUsedPerHour := state.EnergyReservedPerHour
	for _, activeBuilding := range active {
		buildingResult := newBuildingResult(activeBuilding, result.ElapsedApplied)
		if energyUsedPerHour+activeBuilding.definition.EnergyCostPerHour > state.EnergyCapacityPerHour {
			buildingResult.Skipped = true
			buildingResult.SkipReason = SettlementSkipReasonEnergyInsufficient
			result.EnergyInsufficient = true
			result.SkippedBuildings = append(result.SkippedBuildings, SettlementSkippedBuilding{
				BuildingID:            activeBuilding.building.BuildingID,
				Reason:                SettlementSkipReasonEnergyInsufficient,
				EnergyUsedPerHour:     energyUsedPerHour,
				EnergyCostPerHour:     activeBuilding.definition.EnergyCostPerHour,
				EnergyCapacityPerHour: state.EnergyCapacityPerHour,
			})
			result.BuildingResults = append(result.BuildingResults, buildingResult)
			continue
		}
		energyUsedPerHour += activeBuilding.definition.EnergyCostPerHour

		if err := settleBuildingProduction(&storage, activeBuilding.definition, result.ElapsedApplied, now, &buildingResult); err != nil {
			return PlanetProductionSettlementResult{}, err
		}
		if buildingResult.StorageFull {
			result.StorageFull = true
		}
		addDeltas(&result.ProducedItems, buildingResult.ProducedItems)
		addDeltas(&result.ConsumedInputs, buildingResult.ConsumedInputs)
		result.BuildingResults = append(result.BuildingResults, buildingResult)
	}

	state.LastCalculatedAt = now
	state.UpdatedAt = now
	store.states[planetID] = cloneProductionState(state)
	store.storage[planetID] = clonePlanetStorage(storage)
	result.After, _ = store.snapshotLocked(planetID)
	sortSettlementItemDeltas(result.ProducedItems)
	sortSettlementItemDeltas(result.ConsumedInputs)
	store.recordSettlementReferenceLocked(productionSettlementReferenceRecord(result))
	if err := store.appendProductionSettlementEventsLocked(result); err != nil {
		return PlanetProductionSettlementResult{}, err
	}
	return result, nil
}

func settlementBuildingsCanProduceWholeOutput(active []settlementBuilding, elapsed time.Duration) bool {
	for _, activeBuilding := range active {
		for _, output := range activeBuilding.definition.Outputs {
			if wholeUnitsForElapsed(output.AmountPerHour, elapsed) > 0 {
				return true
			}
		}
	}
	return false
}

func (store *InMemoryStore) appendProductionSettlementEventsLocked(result PlanetProductionSettlementResult) error {
	if result.NoOp {
		return nil
	}
	for _, buildingResult := range result.BuildingResults {
		if len(buildingResult.ProducedItems) == 0 && len(buildingResult.ConsumedInputs) == 0 {
			continue
		}
		payload, err := NewBuildingProducedPayload(result.PlanetID, result.SettledAt, buildingResult)
		if err != nil {
			return err
		}
		if _, err := store.appendProductionEventLocked(EventType(EventPlanetBuildingProduced), payload, result.SettledAt); err != nil {
			return err
		}
	}

	payload, err := NewProductionSettlementPayload(result)
	if err != nil {
		return err
	}
	if result.StorageFull {
		if _, err := store.appendProductionEventLocked(EventType(EventPlanetStorageFull), payload, result.SettledAt); err != nil {
			return err
		}
	}
	if result.EnergyInsufficient {
		if _, err := store.appendProductionEventLocked(EventType(EventPlanetEnergyInsufficient), payload, result.SettledAt); err != nil {
			return err
		}
	}
	if _, err := store.appendProductionEventLocked(EventType(EventPlanetProductionSettled), payload, result.SettledAt); err != nil {
		return err
	}
	_, err = store.appendProductionEventLocked(EventType(EventOfflineSettlementCompleted), payload, result.SettledAt)
	return err
}

func newSettlementResult(planetID foundation.PlanetID, now time.Time, before PlanetProductionSnapshot) PlanetProductionSettlementResult {
	return PlanetProductionSettlementResult{
		PlanetID:                     planetID,
		SettledAt:                    now,
		MaxOfflineSettlementDuration: DefaultMaxOfflineSettlementDuration,
		Before:                       before.Clone(),
		After:                        before.Clone(),
	}
}

func attachProductionSettlementEvidence(result *PlanetProductionSettlementResult, start time.Time, end time.Time) error {
	window := settlementWindow(start, end)
	reference, err := foundation.OfflineSettlementIdempotencyKey(result.PlanetID, window)
	if err != nil {
		return err
	}
	result.SettlementWindow = window
	result.ReferenceKey = reference
	return nil
}

func attachRouteSettlementEvidence(result *RouteSettlementResult, start time.Time, end time.Time) error {
	window := settlementWindow(start, end)
	reference, err := foundation.RouteSettlementIdempotencyKey(result.RouteID, window)
	if err != nil {
		return err
	}
	result.SettlementWindow = window
	result.ReferenceKey = reference
	return nil
}

func productionSettlementReferenceRecord(result PlanetProductionSettlementResult) SettlementReferenceRecord {
	return SettlementReferenceRecord{
		ReferenceKey:     result.ReferenceKey,
		SettlementWindow: result.SettlementWindow,
		Kind:             SettlementKindProduction,
		PlanetID:         result.PlanetID,
		AppliedAt:        result.SettledAt.UTC(),
		RecordedAt:       result.SettledAt.UTC(),
	}
}

func settlementWindow(start time.Time, end time.Time) string {
	return fmt.Sprintf("%d-%d", start.UTC().UnixMilli(), end.UTC().UnixMilli())
}

func settlementBuildings(
	buildings []PlanetBuilding,
	priority []BuildingID,
	catalogRows Catalog,
) ([]settlementBuilding, error) {
	active := make([]settlementBuilding, 0, len(buildings))
	for _, building := range buildings {
		if building.State != BuildingStateActive {
			continue
		}
		if err := building.Validate(); err != nil {
			return nil, err
		}
		definition, ok := catalogRows.GetBuilding(building.BuildingType, building.Level)
		if !ok {
			return nil, fmt.Errorf("building %q type %q level %d: %w", building.BuildingID, building.BuildingType, building.Level, ErrUnknownBuildingDefinition)
		}
		if definition.Source != building.Source {
			return nil, fmt.Errorf("building %q source %q production %q: %w", building.BuildingID, building.Source.DefinitionID, definition.DefinitionID, ErrBuildingSourceMismatch)
		}
		active = append(active, settlementBuilding{
			building:   clonePlanetBuilding(building),
			definition: cloneDefinition(definition),
		})
	}
	orderSettlementBuildings(active, priority)
	return active, nil
}

func orderSettlementBuildings(buildings []settlementBuilding, priority []BuildingID) {
	priorityRank := make(map[BuildingID]int, len(priority))
	for i, buildingID := range priority {
		priorityRank[buildingID] = i
	}
	sort.Slice(buildings, func(i, j int) bool {
		leftRank, leftPriority := priorityRank[buildings[i].building.BuildingID]
		rightRank, rightPriority := priorityRank[buildings[j].building.BuildingID]
		switch {
		case leftPriority && rightPriority:
			return leftRank < rightRank
		case leftPriority:
			return true
		case rightPriority:
			return false
		default:
			return buildings[i].building.BuildingID < buildings[j].building.BuildingID
		}
	})
}

func newBuildingResult(activeBuilding settlementBuilding, elapsed time.Duration) PlanetProductionBuildingResult {
	return PlanetProductionBuildingResult{
		BuildingID:        activeBuilding.building.BuildingID,
		DefinitionID:      activeBuilding.definition.DefinitionID,
		BuildingType:      activeBuilding.definition.BuildingType,
		Category:          activeBuilding.definition.Category,
		Level:             activeBuilding.definition.Level,
		ElapsedApplied:    elapsed,
		EnergyCostPerHour: activeBuilding.definition.EnergyCostPerHour,
	}
}

func settleBuildingProduction(
	storage *PlanetStorage,
	definition BuildingProductionDefinition,
	elapsed time.Duration,
	now time.Time,
	result *PlanetProductionBuildingResult,
) error {
	outputs := plannedItemDeltas(definition.Outputs, elapsed)
	if len(outputs) == 0 {
		return nil
	}

	switch definition.Category {
	case BuildingCategoryExtractor:
		return settleExtractorProduction(storage, outputs, now, result)
	case BuildingCategoryRefinery:
		return settleRefineryProduction(storage, definition, outputs, elapsed, now, result)
	default:
		return fmt.Errorf("building category %q: %w", definition.Category, ErrInvalidBuildingCategory)
	}
}

func settleExtractorProduction(
	storage *PlanetStorage,
	outputs []SettlementItemDelta,
	now time.Time,
	result *PlanetProductionBuildingResult,
) error {
	for _, output := range outputs {
		added, err := storage.AddUpToCapacity(output.ItemID, output.Quantity, now)
		if err != nil {
			return err
		}
		if added < output.Quantity {
			result.StorageFull = true
		}
		addDelta(&result.ProducedItems, output.ItemID, added)
	}
	sortSettlementItemDeltas(result.ProducedItems)
	return nil
}

func settleRefineryProduction(
	storage *PlanetStorage,
	definition BuildingProductionDefinition,
	outputs []SettlementItemDelta,
	elapsed time.Duration,
	now time.Time,
	result *PlanetProductionBuildingResult,
) error {
	inputs := plannedItemDeltas(definition.Inputs, elapsed)
	if len(inputs) == 0 {
		return nil
	}

	plannedOutputUnits := totalDeltaQuantity(outputs)
	productionNumerator, productionDenominator := int64(1), int64(1)
	for _, input := range inputs {
		available := storage.QuantityOf(input.ItemID)
		if available < input.Quantity {
			result.InputShortage = true
			productionNumerator, productionDenominator = minRatio(productionNumerator, productionDenominator, available, input.Quantity)
		}
	}
	if productionNumerator == 0 {
		return nil
	}

	outputs = scaleDeltas(outputs, productionNumerator, productionDenominator)
	if len(outputs) == 0 {
		return nil
	}
	inputs = inputsForOutputs(inputs, outputs, plannedOutputUnits)
	effectiveFreeUnits := storage.FreeUnits() + totalDeltaQuantity(inputs)
	actualOutputUnits := totalDeltaQuantity(outputs)
	if actualOutputUnits > effectiveFreeUnits {
		result.StorageFull = true
		outputs = scaleDeltas(outputs, effectiveFreeUnits, actualOutputUnits)
		inputs = inputsForOutputs(plannedItemDeltas(definition.Inputs, elapsed), outputs, plannedOutputUnits)
		if len(outputs) == 0 {
			return nil
		}
	}

	for _, input := range inputs {
		removed, err := storage.RemoveUpTo(input.ItemID, input.Quantity, now)
		if err != nil {
			return err
		}
		if removed != input.Quantity {
			return fmt.Errorf("input %q removed %d want %d: %w", input.ItemID, removed, input.Quantity, ErrInvalidProductionState)
		}
		addDelta(&result.ConsumedInputs, input.ItemID, removed)
	}
	for _, output := range outputs {
		added, err := storage.AddUpToCapacity(output.ItemID, output.Quantity, now)
		if err != nil {
			return err
		}
		if added != output.Quantity {
			return fmt.Errorf("output %q added %d want %d: %w", output.ItemID, added, output.Quantity, ErrInvalidProductionState)
		}
		addDelta(&result.ProducedItems, output.ItemID, added)
	}
	sortSettlementItemDeltas(result.ConsumedInputs)
	sortSettlementItemDeltas(result.ProducedItems)
	return nil
}

func plannedItemDeltas(rates []ItemRate, elapsed time.Duration) []SettlementItemDelta {
	deltas := make([]SettlementItemDelta, 0, len(rates))
	for _, rate := range rates {
		quantity := wholeUnitsForElapsed(rate.AmountPerHour, elapsed)
		if quantity > 0 {
			deltas = append(deltas, SettlementItemDelta{ItemID: rate.ItemID, Quantity: quantity})
		}
	}
	return deltas
}

func wholeUnitsForElapsed(amountPerHour int64, elapsed time.Duration) int64 {
	if amountPerHour <= 0 || elapsed <= 0 {
		return 0
	}
	quantity := math.Floor(elapsed.Hours() * float64(amountPerHour))
	if quantity < 1 {
		return 0
	}
	if quantity > float64(foundation.MaxAmount) {
		return foundation.MaxAmount
	}
	return int64(quantity)
}

func inputsForOutputs(inputs []SettlementItemDelta, outputs []SettlementItemDelta, plannedOutputUnits int64) []SettlementItemDelta {
	actualOutputUnits := totalDeltaQuantity(outputs)
	if actualOutputUnits == 0 || plannedOutputUnits == 0 {
		return nil
	}
	scaled := make([]SettlementItemDelta, 0, len(inputs))
	for _, input := range inputs {
		quantity := mulDivCeil(input.Quantity, actualOutputUnits, plannedOutputUnits)
		if quantity > 0 {
			scaled = append(scaled, SettlementItemDelta{ItemID: input.ItemID, Quantity: quantity})
		}
	}
	return scaled
}

func scaleDeltas(deltas []SettlementItemDelta, numerator int64, denominator int64) []SettlementItemDelta {
	if numerator <= 0 || denominator <= 0 {
		return nil
	}
	scaled := make([]SettlementItemDelta, 0, len(deltas))
	for _, delta := range deltas {
		quantity := mulDivFloor(delta.Quantity, numerator, denominator)
		if quantity > 0 {
			scaled = append(scaled, SettlementItemDelta{ItemID: delta.ItemID, Quantity: quantity})
		}
	}
	return scaled
}

func minRatio(currentNumerator, currentDenominator, candidateNumerator, candidateDenominator int64) (int64, int64) {
	if candidateNumerator < 0 {
		candidateNumerator = 0
	}
	if candidateDenominator <= 0 {
		return currentNumerator, currentDenominator
	}
	if ratioLess(candidateNumerator, candidateDenominator, currentNumerator, currentDenominator) {
		return candidateNumerator, candidateDenominator
	}
	return currentNumerator, currentDenominator
}

func totalDeltaQuantity(deltas []SettlementItemDelta) int64 {
	var total int64
	for _, delta := range deltas {
		total += delta.Quantity
	}
	return total
}

func addDeltas(existing *[]SettlementItemDelta, deltas []SettlementItemDelta) {
	for _, delta := range deltas {
		addDelta(existing, delta.ItemID, delta.Quantity)
	}
}

func addDelta(existing *[]SettlementItemDelta, itemID foundation.ItemID, quantity int64) {
	if quantity <= 0 {
		return
	}
	for i := range *existing {
		if (*existing)[i].ItemID == itemID {
			(*existing)[i].Quantity += quantity
			return
		}
	}
	*existing = append(*existing, SettlementItemDelta{ItemID: itemID, Quantity: quantity})
}

func sortSettlementItemDeltas(deltas []SettlementItemDelta) {
	sort.Slice(deltas, func(i, j int) bool {
		return deltas[i].ItemID < deltas[j].ItemID
	})
}

func mulDivFloor(a, b, denominator int64) int64 {
	if a <= 0 || b <= 0 || denominator <= 0 {
		return 0
	}
	product := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	product.Quo(product, big.NewInt(denominator))
	if product.Cmp(big.NewInt(foundation.MaxAmount)) > 0 {
		return foundation.MaxAmount
	}
	return product.Int64()
}

func mulDivCeil(a, b, denominator int64) int64 {
	if a <= 0 || b <= 0 || denominator <= 0 {
		return 0
	}
	product := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	divisor := big.NewInt(denominator)
	quotient, remainder := new(big.Int).QuoRem(product, divisor, new(big.Int))
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if quotient.Cmp(big.NewInt(foundation.MaxAmount)) > 0 {
		return foundation.MaxAmount
	}
	return quotient.Int64()
}

func ratioLess(leftNumerator, leftDenominator, rightNumerator, rightDenominator int64) bool {
	left := new(big.Int).Mul(big.NewInt(leftNumerator), big.NewInt(rightDenominator))
	right := new(big.Int).Mul(big.NewInt(rightNumerator), big.NewInt(leftDenominator))
	return left.Cmp(right) < 0
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

package death

import (
	"errors"
	"fmt"
	"sync"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/world"
)

const (
	deathItemCatalogVersion = catalog.Version("death_service_v1")

	// LedgerReasonDeathCargoDrop records cargo removed from a dead player's ship
	// before that cargo is materialized as player-death loot.
	LedgerReasonDeathCargoDrop = economy.LedgerReason("death_cargo_drop")
)

var (
	ErrNilInventoryService         = errors.New("nil inventory service")
	ErrNilLootService              = errors.New("nil loot service")
	ErrNilShipService              = errors.New("nil ship service")
	ErrCargoDropPolicyZoneMismatch = errors.New("cargo drop policy zone mismatch")
	ErrDeathCargoOwnerMismatch     = errors.New("death cargo owner mismatch")
	ErrDeathCargoLocationMismatch  = errors.New("death cargo location mismatch")
)

// InventoryRemover is the inventory boundary DeathService needs for cargo loss.
type InventoryRemover interface {
	RemoveItem(economy.RemoveItemInput) (economy.RemoveItemResult, error)
}

// PlayerDeathDropCreator is the loot boundary DeathService needs for concrete
// world drops after cargo loss has been selected and removed.
type PlayerDeathDropCreator interface {
	CreateDropsForPlayerDeath(loot.CreatePlayerDeathDropsInput) (loot.CreateDropsResult, error)
}

// ActiveShipDisabler is the ship boundary DeathService needs after lethal damage.
type ActiveShipDisabler interface {
	GetHangar(foundation.PlayerID) (ships.HangarSnapshot, error)
	DisableActiveShipForDeath(ships.DisableActiveShipForDeathInput) (ships.DisableActiveShipForDeathResult, error)
}

// ModuleDurabilityHook lets the module package apply its own death durability
// policy to the selected equipped module item instances.
type ModuleDurabilityHook interface {
	ApplyDeathDurability(ModuleDurabilityInput) (ModuleDurabilityResult, error)
}

// Config describes DeathService dependencies.
type Config struct {
	Clock foundation.Clock
	RNG   foundation.RNG

	Inventory InventoryRemover
	Loot      PlayerDeathDropCreator
	Ships     ActiveShipDisabler

	ModuleDurability ModuleDurabilityHook
}

// DeathService orchestrates the in-memory Phase 06 death transaction slice.
type DeathService struct {
	mu    sync.Mutex
	clock foundation.Clock
	rng   foundation.RNG

	inventory        InventoryRemover
	loot             PlayerDeathDropCreator
	ships            ActiveShipDisabler
	moduleDurability ModuleDurabilityHook

	attempts  map[LethalEventKey]processDeathAttempt
	processed map[LethalEventKey]ProcessDeathResult
}

type processDeathAttempt struct {
	selection         CargoDropSelection
	shipDisabled      bool
	shipDisableResult ships.DisableActiveShipForDeathResult
}

// ProcessDeathInput is one authoritative lethal event to process once.
type ProcessDeathInput struct {
	LethalEventID     foundation.EventID  `json:"lethal_event_id"`
	PlayerID          foundation.PlayerID `json:"player_id"`
	WorldID           world.WorldID       `json:"world_id"`
	ZoneID            world.ZoneID        `json:"zone_id"`
	Position          world.Vec2          `json:"position"`
	KillerEntityID    world.EntityID      `json:"killer_entity_id,omitempty"`
	Reason            DeathReason         `json:"death_reason"`
	CargoDropPolicy   ZoneCargoDropPolicy `json:"cargo_drop_policy"`
	Cargo             []CargoStack        `json:"cargo"`
	RespawnLocationID RespawnLocationID   `json:"respawn_location_id"`

	// DropOwnerPlayerID is optional. PvP rules can set it to the killer; PvE can
	// leave it empty so the drop becomes public after the loot service windows.
	DropOwnerPlayerID foundation.PlayerID `json:"drop_owner_player_id,omitempty"`

	// EquippedItemIDs are the module item instances selected by the caller for
	// death durability handling. DeathService does not roll random damage here.
	EquippedItemIDs []foundation.ItemID `json:"equipped_item_ids,omitempty"`
}

// ProcessDeathResult reports all service-owned side effects from ProcessDeath.
type ProcessDeathResult struct {
	Record                 DeathRecord                           `json:"record"`
	CargoSelection         CargoDropSelection                    `json:"cargo_selection"`
	CargoDrops             []CargoDrop                           `json:"cargo_drops"`
	CargoRemovalResults    []economy.RemoveItemResult            `json:"cargo_removal_results"`
	LootDrops              []loot.Drop                           `json:"loot_drops"`
	ScheduledTasks         []loot.ScheduledDropTask              `json:"scheduled_tasks"`
	ShipDisableResult      ships.DisableActiveShipForDeathResult `json:"ship_disable_result"`
	ModuleDurabilityResult *ModuleDurabilityResult               `json:"module_durability_result,omitempty"`
	Duplicate              bool                                  `json:"duplicate"`
}

// ModuleDurabilityInput is the death-domain handoff to module durability.
type ModuleDurabilityInput struct {
	DeathID         foundation.EventID  `json:"death_id"`
	LethalEventKey  LethalEventKey      `json:"lethal_event_key"`
	PlayerID        foundation.PlayerID `json:"player_id"`
	ShipID          foundation.ShipID   `json:"ship_id"`
	EquippedItemIDs []foundation.ItemID `json:"equipped_item_ids"`
}

// ModuleDurabilityResult reports the optional module durability hook result.
type ModuleDurabilityResult struct {
	SelectedItemIDs []foundation.ItemID `json:"selected_item_ids"`
	Duplicate       bool                `json:"duplicate"`
}

// NewDeathService returns an in-memory death orchestrator.
func NewDeathService(config Config) (*DeathService, error) {
	if config.Clock == nil {
		config.Clock = foundation.RealClock{}
	}
	if config.RNG == nil {
		return nil, ErrNilRNG
	}
	if config.Inventory == nil {
		return nil, ErrNilInventoryService
	}
	if config.Loot == nil {
		return nil, ErrNilLootService
	}
	if config.Ships == nil {
		return nil, ErrNilShipService
	}
	return &DeathService{
		clock:            config.Clock,
		rng:              config.RNG,
		inventory:        config.Inventory,
		loot:             config.Loot,
		ships:            config.Ships,
		moduleDurability: config.ModuleDurability,
		attempts:         make(map[LethalEventKey]processDeathAttempt),
		processed:        make(map[LethalEventKey]ProcessDeathResult),
	}, nil
}

// SetModuleDurabilityHook configures the optional durability hook.
func (service *DeathService) SetModuleDurabilityHook(hook ModuleDurabilityHook) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.moduleDurability = hook
}

// ProcessDeath processes one lethal event once.
func (service *DeathService) ProcessDeath(input ProcessDeathInput) (ProcessDeathResult, error) {
	if err := input.validate(); err != nil {
		return ProcessDeathResult{}, err
	}
	lethalKey, err := NewLethalEventKey(input.LethalEventID)
	if err != nil {
		return ProcessDeathResult{}, err
	}
	deathID, err := deathIDForLethalKey(lethalKey)
	if err != nil {
		return ProcessDeathResult{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	if existing, ok := service.processed[lethalKey]; ok {
		return duplicateProcessDeathResult(existing), nil
	}

	attempt, ok := service.attempts[lethalKey]
	if !ok {
		activeShipID, activeShipDisabled, err := service.activeShipForDeath(input.PlayerID)
		if err != nil {
			return ProcessDeathResult{}, err
		}
		if activeShipDisabled {
			shipDisable, err := service.ships.DisableActiveShipForDeath(ships.DisableActiveShipForDeathInput{
				PlayerID: input.PlayerID,
			})
			if err != nil {
				return ProcessDeathResult{}, err
			}
			result := ProcessDeathResult{
				ShipDisableResult: shipDisable,
				Duplicate:         true,
			}
			service.processed[lethalKey] = cloneProcessDeathResult(result)
			return cloneProcessDeathResult(result), nil
		}
		if err := validateDeathCargoStacks(input.PlayerID, activeShipID, input.Cargo); err != nil {
			return ProcessDeathResult{}, err
		}
	}

	shipDisable, err := service.ships.DisableActiveShipForDeath(ships.DisableActiveShipForDeathInput{
		PlayerID: input.PlayerID,
	})
	if err != nil {
		return ProcessDeathResult{}, err
	}
	if shipDisable.Duplicate && !ok {
		result := ProcessDeathResult{
			ShipDisableResult: shipDisable,
			Duplicate:         true,
		}
		service.processed[lethalKey] = cloneProcessDeathResult(result)
		return cloneProcessDeathResult(result), nil
	}

	if !ok {
		if err := validateDeathCargoStacks(input.PlayerID, shipDisable.ActiveShip.ShipID, input.Cargo); err != nil {
			return ProcessDeathResult{}, err
		}
		selection, err := SelectCargoDrops(SelectCargoDropsInput{
			Policy: input.CargoDropPolicy,
			Cargo:  input.Cargo,
			RNG:    service.rng,
		})
		if err != nil {
			return ProcessDeathResult{}, err
		}
		if err := validateDeathCargoDrops(input.PlayerID, shipDisable.ActiveShip.ShipID, selection.Drops); err != nil {
			return ProcessDeathResult{}, err
		}
		attempt = processDeathAttempt{
			selection:         cloneCargoDropSelection(selection),
			shipDisabled:      shipDisable.Disabled,
			shipDisableResult: shipDisable,
		}
		service.attempts[lethalKey] = attempt
	}
	selection := cloneCargoDropSelection(attempt.selection)
	if err := validateDeathCargoDrops(input.PlayerID, shipDisable.ActiveShip.ShipID, selection.Drops); err != nil {
		return ProcessDeathResult{}, err
	}
	if attempt.shipDisabled && shipDisable.Duplicate {
		shipDisable = attempt.shipDisableResult
	}

	removalResults := make([]economy.RemoveItemResult, 0, len(selection.Drops))
	for _, drop := range selection.Drops {
		removed, err := service.inventory.RemoveItem(removeInputForDrop(input.PlayerID, deathID, drop))
		if err != nil {
			return ProcessDeathResult{}, err
		}
		removalResults = append(removalResults, removed)
	}

	var lootResult loot.CreateDropsResult
	if len(selection.Drops) > 0 {
		lootInput, err := createLootInputForDeath(input, deathID, selection.Drops)
		if err != nil {
			return ProcessDeathResult{}, err
		}
		lootResult, err = service.loot.CreateDropsForPlayerDeath(lootInput)
		if err != nil {
			return ProcessDeathResult{}, err
		}
	}

	var durabilityResult *ModuleDurabilityResult
	if service.moduleDurability != nil && len(input.EquippedItemIDs) > 0 {
		hookResult, err := service.moduleDurability.ApplyDeathDurability(ModuleDurabilityInput{
			DeathID:         deathID,
			LethalEventKey:  lethalKey,
			PlayerID:        input.PlayerID,
			ShipID:          shipDisable.ActiveShip.ShipID,
			EquippedItemIDs: append([]foundation.ItemID(nil), input.EquippedItemIDs...),
		})
		if err != nil {
			return ProcessDeathResult{}, err
		}
		durabilityResult = &hookResult
	}

	record := DeathRecord{
		DeathID:           deathID,
		LethalEventKey:    lethalKey,
		PlayerID:          input.PlayerID,
		WorldID:           input.WorldID,
		ZoneID:            input.ZoneID,
		Position:          input.Position,
		KillerEntityID:    input.KillerEntityID,
		Reason:            input.Reason,
		CargoDropPercent:  selection.DropPercent,
		ActiveShipID:      shipDisable.ActiveShip.ShipID,
		RespawnLocationID: input.RespawnLocationID,
		CreatedAt:         service.clock.Now(),
	}
	if err := record.Validate(); err != nil {
		return ProcessDeathResult{}, err
	}

	result := ProcessDeathResult{
		Record:                 record,
		CargoSelection:         selection,
		CargoDrops:             append([]CargoDrop(nil), selection.Drops...),
		CargoRemovalResults:    removalResults,
		LootDrops:              append([]loot.Drop(nil), lootResult.Drops...),
		ScheduledTasks:         append([]loot.ScheduledDropTask(nil), lootResult.ScheduledTasks...),
		ShipDisableResult:      shipDisable,
		ModuleDurabilityResult: durabilityResult,
	}
	service.processed[lethalKey] = cloneProcessDeathResult(result)
	delete(service.attempts, lethalKey)
	return cloneProcessDeathResult(result), nil
}

func (input ProcessDeathInput) validate() error {
	if err := input.LethalEventID.Validate(); err != nil {
		return err
	}
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.WorldID.Validate(); err != nil {
		return err
	}
	if err := input.ZoneID.Validate(); err != nil {
		return err
	}
	if err := input.Position.Validate(); err != nil {
		return err
	}
	if !input.KillerEntityID.IsZero() {
		if err := input.KillerEntityID.Validate(); err != nil {
			return err
		}
	}
	if err := input.Reason.Validate(); err != nil {
		return err
	}
	if err := input.CargoDropPolicy.Validate(); err != nil {
		return err
	}
	if input.CargoDropPolicy.ZoneID != input.ZoneID {
		return fmt.Errorf("cargo drop policy zone %q does not match death zone %q: %w", input.CargoDropPolicy.ZoneID, input.ZoneID, ErrCargoDropPolicyZoneMismatch)
	}
	if err := input.RespawnLocationID.Validate(); err != nil {
		return err
	}
	if !input.DropOwnerPlayerID.IsZero() {
		if err := input.DropOwnerPlayerID.Validate(); err != nil {
			return err
		}
	}
	for _, itemID := range input.EquippedItemIDs {
		if err := itemID.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (service *DeathService) activeShipForDeath(playerID foundation.PlayerID) (foundation.ShipID, bool, error) {
	snapshot, err := service.ships.GetHangar(playerID)
	if err != nil {
		return "", false, err
	}
	if !snapshot.HasActiveShip {
		return "", false, ships.ErrNoActiveShip
	}
	activeShipID := snapshot.ActiveShip.ShipID
	for _, playerShip := range snapshot.Ships {
		if playerShip.ShipID == activeShipID {
			return activeShipID, playerShip.State == ships.ShipStateDisabled, nil
		}
	}
	return "", false, fmt.Errorf("active ship %q: %w", activeShipID, ships.ErrShipNotUnlocked)
}

func validateDeathCargoStacks(playerID foundation.PlayerID, activeShipID foundation.ShipID, cargo []CargoStack) error {
	if err := activeShipID.Validate(); err != nil {
		return err
	}
	for index, stack := range cargo {
		if err := stack.Validate(); err != nil {
			return fmt.Errorf("cargo[%d]: %w", index, err)
		}
		if stack.OwnerPlayerID != playerID {
			return fmt.Errorf("cargo[%d] owner %q does not match player %q: %w", index, stack.OwnerPlayerID, playerID, ErrDeathCargoOwnerMismatch)
		}
		if err := validateDeathCargoLocation(fmt.Sprintf("cargo[%d]", index), activeShipID, stack.Location); err != nil {
			return err
		}
	}
	return nil
}

func validateDeathCargoDrops(playerID foundation.PlayerID, activeShipID foundation.ShipID, drops []CargoDrop) error {
	if err := activeShipID.Validate(); err != nil {
		return err
	}
	for index, drop := range drops {
		if drop.OwnerPlayerID != playerID {
			return fmt.Errorf("cargo_drop[%d] owner %q does not match player %q: %w", index, drop.OwnerPlayerID, playerID, ErrDeathCargoOwnerMismatch)
		}
		if err := validateDeathCargoLocation(fmt.Sprintf("cargo_drop[%d]", index), activeShipID, drop.SourceLocation); err != nil {
			return err
		}
	}
	return nil
}

func validateDeathCargoLocation(label string, activeShipID foundation.ShipID, location economy.ItemLocation) error {
	if location.Kind != economy.LocationKindShipCargo {
		return fmt.Errorf("%s source location %q is not ship cargo for active ship %q: %w", label, location.Kind, activeShipID, ErrDeathCargoLocationMismatch)
	}
	if location.ID.String() != activeShipID.String() {
		return fmt.Errorf("%s source location %q does not match active ship %q: %w", label, location.ID, activeShipID, ErrDeathCargoLocationMismatch)
	}
	return nil
}

func removeInputForDrop(playerID foundation.PlayerID, deathID foundation.EventID, drop CargoDrop) economy.RemoveItemInput {
	definition, err := economyDefinitionForDrop(drop)
	if err != nil {
		// Input was already validated by SelectCargoDrops. Returning a zero
		// definition here makes RemoveItem surface the validation error.
		definition = economy.ItemDefinition{}
	}
	referenceKey, err := inventoryRemoveReferenceKey(deathID, drop)
	if err != nil {
		referenceKey = ""
	}
	return economy.RemoveItemInput{
		PlayerID: playerID,
		ItemRef: economy.RemoveItemRef{
			Definition:     definition,
			ItemInstanceID: itemInstanceIDForRemove(drop),
		},
		SourceLocation: drop.SourceLocation,
		Quantity:       drop.Quantity,
		Reason:         LedgerReasonDeathCargoDrop,
		ReferenceKey:   referenceKey,
	}
}

func itemInstanceIDForRemove(drop CargoDrop) foundation.ItemID {
	if drop.Type == economy.ItemTypeInstance {
		return drop.ItemInstanceID
	}
	return ""
}

func createLootInputForDeath(input ProcessDeathInput, deathID foundation.EventID, drops []CargoDrop) (loot.CreatePlayerDeathDropsInput, error) {
	items := make([]loot.DropItem, 0, len(drops))
	for _, drop := range drops {
		definition, err := economyDefinitionForDrop(drop)
		if err != nil {
			return loot.CreatePlayerDeathDropsInput{}, err
		}
		items = append(items, loot.DropItem{
			ItemDefinition: definition,
			Quantity:       drop.Quantity,
		})
	}
	return loot.CreatePlayerDeathDropsInput{
		SourceID:      world.EntityID(deathID),
		DeadPlayerID:  input.PlayerID,
		OwnerPlayerID: input.DropOwnerPlayerID,
		WorldID:       input.WorldID,
		ZoneID:        input.ZoneID,
		Position:      input.Position,
		Items:         items,
	}, nil
}

func economyDefinitionForDrop(drop CargoDrop) (economy.ItemDefinition, error) {
	if !drop.EconomyDefinition.Source.IsZero() {
		if err := drop.EconomyDefinition.Validate(); err != nil {
			return economy.ItemDefinition{}, err
		}
		return drop.EconomyDefinition, nil
	}

	definition := drop.Definition
	if definition.ItemID.IsZero() {
		definition.ItemID = drop.ItemID
	}
	if definition.Type == "" {
		definition.Type = drop.Type
	}
	return economyDefinitionFromCargoDefinition(definition)
}

func economyDefinitionFromCargoDefinition(definition CargoItemDefinition) (economy.ItemDefinition, error) {
	if err := definition.Validate(); err != nil {
		return economy.ItemDefinition{}, err
	}
	source := definition.Source
	if source.IsZero() {
		var err error
		source, err = catalog.NewVersionedDefinitionFromStrings(definition.ItemID.String(), deathItemCatalogVersion.String())
		if err != nil {
			return economy.ItemDefinition{}, err
		}
	}
	maxStackAmount := foundation.MaxAmount
	if definition.Type == economy.ItemTypeInstance {
		maxStackAmount = 1
	}
	maxStack, err := foundation.NewQuantity(maxStackAmount)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weightUnits, err := foundation.NewQuantity(cargoWeightUnits(definition))
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return economy.NewItemDefinition(
		source,
		definition.ItemID,
		definition.ItemID.String(),
		definition.Type,
		economy.ItemRarityCommon,
		maxStack,
		weightUnits,
		definition.TradeFlags,
		definition.BindRules,
		nil,
	)
}

func cargoWeightUnits(definition CargoItemDefinition) int64 {
	if definition.CargoUnitsPerItem > 0 {
		return definition.CargoUnitsPerItem
	}
	return 1
}

func deathIDForLethalKey(key LethalEventKey) (foundation.EventID, error) {
	eventID, err := key.EventID()
	if err != nil {
		return "", err
	}
	deathID := foundation.EventID("death-" + eventID.String())
	if err := deathID.Validate(); err != nil {
		return "", err
	}
	return deathID, nil
}

func inventoryRemoveReferenceKey(deathID foundation.EventID, drop CargoDrop) (foundation.IdempotencyKey, error) {
	return foundation.DeathCargoDropIdempotencyKey(deathID, drop.SourceStackID)
}

func duplicateProcessDeathResult(result ProcessDeathResult) ProcessDeathResult {
	duplicate := cloneProcessDeathResult(result)
	duplicate.Duplicate = true
	for index := range duplicate.CargoRemovalResults {
		duplicate.CargoRemovalResults[index].Duplicate = true
	}
	duplicate.ShipDisableResult.Disabled = false
	duplicate.ShipDisableResult.Duplicate = true
	if duplicate.ModuleDurabilityResult != nil {
		duplicate.ModuleDurabilityResult.Duplicate = true
	}
	return duplicate
}

func cloneProcessDeathResult(result ProcessDeathResult) ProcessDeathResult {
	clone := result
	clone.CargoSelection = cloneCargoDropSelection(result.CargoSelection)
	clone.CargoDrops = append([]CargoDrop(nil), result.CargoDrops...)
	clone.CargoRemovalResults = cloneRemoveItemResults(result.CargoRemovalResults)
	clone.LootDrops = append([]loot.Drop(nil), result.LootDrops...)
	clone.ScheduledTasks = append([]loot.ScheduledDropTask(nil), result.ScheduledTasks...)
	if result.ModuleDurabilityResult != nil {
		moduleDurability := *result.ModuleDurabilityResult
		moduleDurability.SelectedItemIDs = append([]foundation.ItemID(nil), result.ModuleDurabilityResult.SelectedItemIDs...)
		clone.ModuleDurabilityResult = &moduleDurability
	}
	return clone
}

func cloneCargoDropSelection(selection CargoDropSelection) CargoDropSelection {
	clone := selection
	clone.Drops = append([]CargoDrop(nil), selection.Drops...)
	clone.Preserved = append([]CargoStack(nil), selection.Preserved...)
	return clone
}

func cloneRemoveItemResults(results []economy.RemoveItemResult) []economy.RemoveItemResult {
	cloned := make([]economy.RemoveItemResult, len(results))
	for index, result := range results {
		cloned[index] = result
		cloned[index].StackableItems = append([]economy.StackableItem(nil), result.StackableItems...)
		cloned[index].InstanceItems = append([]economy.InstanceItem(nil), result.InstanceItems...)
		cloned[index].LedgerEntries = append([]economy.ItemLedgerEntry(nil), result.LedgerEntries...)
	}
	return cloned
}

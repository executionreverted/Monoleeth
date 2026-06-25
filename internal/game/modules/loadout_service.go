package modules

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

// LoadoutRepository stores saved loadout definitions.
type LoadoutRepository interface {
	SaveLoadout(loadout Loadout) error
	Loadout(playerID foundation.PlayerID, loadoutID LoadoutID) (Loadout, error)
}

// ActiveShipReader returns the server-owned active ship pointer.
type ActiveShipReader interface {
	ActiveShipID(playerID foundation.PlayerID) (foundation.ShipID, error)
}

// ModuleItemReader reads authoritative module item snapshots.
type ModuleItemReader interface {
	ModuleItem(itemInstanceID foundation.ItemID) (economy.InstanceItem, error)
}

// EquippedModuleReader reads server-owned equipped module state.
type EquippedModuleReader interface {
	EquippedModules(playerID foundation.PlayerID, shipID foundation.ShipID) ([]EquippedModule, error)
	EquippedModuleByItem(itemInstanceID foundation.ItemID) (EquippedModule, bool, error)
}

// ModuleItemMutator records equipped-module and durability transitions.
type ModuleItemMutator interface {
	ReplaceEquippedModules(input ReplaceEquippedModulesInput) error
	MarkEquippedModuleBroken(playerID foundation.PlayerID, shipID foundation.ShipID, itemInstanceID foundation.ItemID) (EquippedModule, bool, error)
}

// LoadoutStore keeps the original constructor boundary while composing the
// smaller repository interfaces expected from durable adapters.
type LoadoutStore interface {
	LoadoutRepository
	ActiveShipReader
	ModuleItemReader
	EquippedModuleReader
	ModuleItemMutator
}

// LoadoutService validates saved loadouts and applies them to the active ship.
type LoadoutService struct {
	catalog       Catalog
	loadouts      LoadoutRepository
	activeShips   ActiveShipReader
	moduleItems   ModuleItemReader
	equipped      EquippedModuleReader
	moduleMutator ModuleItemMutator
	ships         ShipSlotLayoutProvider
	progression   PilotProgressionProvider
	clock         foundation.Clock
}

// NewLoadoutService returns a loadout service over explicit storage.
func NewLoadoutService(
	moduleCatalog Catalog,
	store LoadoutStore,
	ships ShipSlotLayoutProvider,
	progression PilotProgressionProvider,
	clock foundation.Clock,
) (LoadoutService, error) {
	if store == nil {
		return LoadoutService{}, ErrNilLoadoutStore
	}
	return NewLoadoutServiceWithRepositories(moduleCatalog, store, store, store, store, store, ships, progression, clock)
}

// NewLoadoutServiceWithRepositories returns a loadout service over split
// storage interfaces. It lets future durable adapters provide independent
// repositories without forcing one monolithic store type.
func NewLoadoutServiceWithRepositories(
	moduleCatalog Catalog,
	loadouts LoadoutRepository,
	activeShips ActiveShipReader,
	moduleItems ModuleItemReader,
	equipped EquippedModuleReader,
	moduleMutator ModuleItemMutator,
	ships ShipSlotLayoutProvider,
	progression PilotProgressionProvider,
	clock foundation.Clock,
) (LoadoutService, error) {
	if loadouts == nil || activeShips == nil || moduleItems == nil || equipped == nil || moduleMutator == nil {
		return LoadoutService{}, ErrNilLoadoutStore
	}
	if ships == nil {
		return LoadoutService{}, ErrNilShipSlotLayoutProvider
	}
	if progression == nil {
		return LoadoutService{}, ErrNilPilotProgressionProvider
	}
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return LoadoutService{
		catalog:       moduleCatalog,
		loadouts:      loadouts,
		activeShips:   activeShips,
		moduleItems:   moduleItems,
		equipped:      equipped,
		moduleMutator: moduleMutator,
		ships:         ships,
		progression:   progression,
		clock:         clock,
	}, nil
}

// SaveLoadout validates and stores a loadout without mutating equipped modules.
func (service LoadoutService) SaveLoadout(input SaveLoadoutInput) (Loadout, error) {
	if err := input.LoadoutID.Validate(); err != nil {
		return Loadout{}, err
	}
	if err := input.PlayerID.Validate(); err != nil {
		return Loadout{}, err
	}
	if err := input.ShipID.Validate(); err != nil {
		return Loadout{}, err
	}
	shipSlots, err := service.ships.SlotLayoutForShip(input.ShipID)
	if err != nil {
		return Loadout{}, err
	}
	progression, err := service.progression.ProgressionForPlayer(input.PlayerID)
	if err != nil {
		return Loadout{}, err
	}
	if _, err := service.ValidateModuleAssignments(input.validationContext(shipSlots, progression), input.SlotAssignments); err != nil {
		return Loadout{}, err
	}

	now := service.clock.Now()
	createdAt := now
	if existing, err := service.loadouts.Loadout(input.PlayerID, input.LoadoutID); err == nil {
		createdAt = existing.CreatedAt
	} else if !errors.Is(err, ErrUnknownLoadout) {
		return Loadout{}, err
	}

	loadout := Loadout{
		LoadoutID:       input.LoadoutID,
		PlayerID:        input.PlayerID,
		ShipID:          input.ShipID,
		Name:            input.Name,
		SlotAssignments: input.SlotAssignments.Clone(),
		CreatedAt:       createdAt,
		UpdatedAt:       now,
	}
	if err := loadout.Validate(); err != nil {
		return Loadout{}, err
	}
	if err := service.loadouts.SaveLoadout(loadout); err != nil {
		return Loadout{}, err
	}
	return cloneLoadout(loadout), nil
}

// ApplyLoadout validates the saved loadout against current item state and
// replaces equipped modules on the active ship.
func (service LoadoutService) ApplyLoadout(input ApplyLoadoutInput) (ApplyLoadoutResult, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return ApplyLoadoutResult{}, err
	}
	if err := input.LoadoutID.Validate(); err != nil {
		return ApplyLoadoutResult{}, err
	}

	loadout, err := service.loadouts.Loadout(input.PlayerID, input.LoadoutID)
	if err != nil {
		return ApplyLoadoutResult{}, err
	}
	activeShipID, err := service.activeShips.ActiveShipID(input.PlayerID)
	if err != nil {
		return ApplyLoadoutResult{}, err
	}
	if activeShipID != loadout.ShipID {
		return ApplyLoadoutResult{}, fmt.Errorf("loadout ship %q active ship %q: %w", loadout.ShipID, activeShipID, ErrLoadoutShipMismatch)
	}
	shipSlots, err := service.ships.SlotLayoutForShip(activeShipID)
	if err != nil {
		return ApplyLoadoutResult{}, err
	}
	progression, err := service.progression.ProgressionForPlayer(input.PlayerID)
	if err != nil {
		return ApplyLoadoutResult{}, err
	}

	if _, err := service.ValidateModuleAssignments(input.validationContext(activeShipID, shipSlots, progression), loadout.SlotAssignments); err != nil {
		return ApplyLoadoutResult{}, err
	}

	current, err := service.equipped.EquippedModules(input.PlayerID, activeShipID)
	if err != nil {
		return ApplyLoadoutResult{}, err
	}
	now := service.clock.Now()
	target := buildTargetEquipped(input.PlayerID, activeShipID, loadout.SlotAssignments, current, now)
	equipped, unequipped := diffEquippedModules(current, target)
	if err := service.moduleMutator.ReplaceEquippedModules(ReplaceEquippedModulesInput{
		PlayerID:  input.PlayerID,
		ShipID:    activeShipID,
		Equipped:  target,
		RequestID: input.RequestID,
	}); err != nil {
		return ApplyLoadoutResult{}, err
	}

	result := ApplyLoadoutResult{
		Loadout:    cloneLoadout(loadout),
		Current:    cloneEquippedModules(target),
		Equipped:   cloneEquippedModules(equipped),
		Unequipped: cloneEquippedModules(unequipped),
	}
	if len(equipped) == 0 && len(unequipped) == 0 {
		result.Noop = true
		return result, nil
	}
	result.StatInvalidations = buildStatInvalidationSignals(input.PlayerID, activeShipID, input.LoadoutID, equipped, unequipped, now)
	return result, nil
}

// EquippedItemIDs returns the server-owned equipped module item instances for a
// player ship. Death processing uses this method to avoid trusting client-owned
// loadout payloads.
func (service LoadoutService) EquippedItemIDs(playerID foundation.PlayerID, shipID foundation.ShipID) ([]foundation.ItemID, error) {
	if err := playerID.Validate(); err != nil {
		return nil, err
	}
	if err := shipID.Validate(); err != nil {
		return nil, err
	}
	equipped, err := service.equipped.EquippedModules(playerID, shipID)
	if err != nil {
		return nil, err
	}
	itemIDs := make([]foundation.ItemID, 0, len(equipped))
	for _, module := range equipped {
		if err := module.Validate(); err != nil {
			return nil, err
		}
		itemIDs = append(itemIDs, module.ItemInstanceID)
	}
	return itemIDs, nil
}

// BreakEquippedModule records an equipped module crossing to broken durability
// and returns the stat invalidation caused by that state change.
func (service LoadoutService) BreakEquippedModule(input BreakEquippedModuleInput) (BreakEquippedModuleResult, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return BreakEquippedModuleResult{}, err
	}
	if err := input.ShipID.Validate(); err != nil {
		return BreakEquippedModuleResult{}, err
	}
	if err := input.ItemInstanceID.Validate(); err != nil {
		return BreakEquippedModuleResult{}, err
	}

	item, err := service.moduleItems.ModuleItem(input.ItemInstanceID)
	if err != nil {
		return BreakEquippedModuleResult{}, err
	}
	if err := item.Validate(); err != nil {
		return BreakEquippedModuleResult{}, err
	}
	if item.ItemInstanceID != input.ItemInstanceID {
		return BreakEquippedModuleResult{}, fmt.Errorf("lookup %q returned %q: %w", input.ItemInstanceID, item.ItemInstanceID, ErrModuleItemInstanceMismatch)
	}
	if item.OwnerPlayerID != input.PlayerID {
		return BreakEquippedModuleResult{}, fmt.Errorf("item %q owner %q player %q: %w", item.ItemInstanceID, item.OwnerPlayerID, input.PlayerID, ErrModuleItemNotOwned)
	}
	if _, ok := service.catalog.Lookup(item.ItemID); !ok {
		return BreakEquippedModuleResult{}, fmt.Errorf("item %q definition %q: %w", item.ItemInstanceID, item.ItemID, ErrUnknownModuleDefinition)
	}

	broken, changed, err := service.moduleMutator.MarkEquippedModuleBroken(input.PlayerID, input.ShipID, input.ItemInstanceID)
	if err != nil {
		return BreakEquippedModuleResult{}, err
	}

	result := BreakEquippedModuleResult{Broken: broken}
	if changed {
		result.StatInvalidations = []StatInvalidationSignal{
			buildModuleBreakStatInvalidationSignal(input.PlayerID, input.ShipID, input.ItemInstanceID, service.clock.Now()),
		}
	}
	return result, nil
}

// ValidateModuleAssignments validates ownership, item location, slot
// compatibility, player requirements, durability, and duplicate item use.
func (service LoadoutService) ValidateModuleAssignments(
	ctx LoadoutValidationContext,
	assignments SlotAssignments,
) ([]ValidatedModuleAssignment, error) {
	if err := ctx.validate(); err != nil {
		return nil, err
	}
	if err := assignments.Validate(); err != nil {
		return nil, err
	}

	validated := make([]ValidatedModuleAssignment, 0, len(assignments))
	for _, assignment := range sortedSlotAssignments(assignments) {
		slotType, err := assignment.slotID.SlotType()
		if err != nil {
			return nil, err
		}
		if err := ctx.ShipSlots.ValidateSlot(assignment.slotID); err != nil {
			return nil, err
		}
		item, err := service.moduleItems.ModuleItem(assignment.itemInstanceID)
		if err != nil {
			return nil, err
		}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		if item.ItemInstanceID != assignment.itemInstanceID {
			return nil, fmt.Errorf("lookup %q returned %q: %w", assignment.itemInstanceID, item.ItemInstanceID, ErrModuleItemInstanceMismatch)
		}
		if item.OwnerPlayerID != ctx.PlayerID {
			return nil, fmt.Errorf("item %q owner %q player %q: %w", item.ItemInstanceID, item.OwnerPlayerID, ctx.PlayerID, ErrModuleItemNotOwned)
		}

		definition, ok := service.catalog.Lookup(item.ItemID)
		if !ok {
			return nil, fmt.Errorf("item %q definition %q: %w", item.ItemInstanceID, item.ItemID, ErrUnknownModuleDefinition)
		}
		if !moduleCompatibleWithSlot(definition, slotType) {
			return nil, fmt.Errorf("slot %q type %q item %q type %q: %w", assignment.slotID, slotType, item.ItemID, definition.SlotType, ErrWrongModuleSlotType)
		}
		if ctx.PlayerRank < definition.RequiredRank {
			return nil, fmt.Errorf("player rank %d required %d for %q: %w", ctx.PlayerRank, definition.RequiredRank, item.ItemID, ErrPlayerRankTooLow)
		}
		if err := validateRoleLevels(ctx.RoleLevels, definition); err != nil {
			return nil, err
		}
		if item.DurabilityCurrent <= 0 {
			return nil, fmt.Errorf("item %q durability %d: %w", item.ItemInstanceID, item.DurabilityCurrent, ErrModuleBroken)
		}

		equipped, isEquipped, err := service.equipped.EquippedModuleByItem(item.ItemInstanceID)
		if err != nil {
			return nil, err
		}
		equippedOnTargetShip := isEquipped && equipped.PlayerID == ctx.PlayerID && equipped.ShipID == ctx.ShipID
		if isEquipped && !equippedOnTargetShip {
			return nil, fmt.Errorf("item %q equipped by player %q ship %q: %w", item.ItemInstanceID, equipped.PlayerID, equipped.ShipID, ErrModuleItemAlreadyEquipped)
		}
		if err := validateModuleEquipLocation(item.Location, equippedOnTargetShip); err != nil {
			return nil, err
		}

		validated = append(validated, ValidatedModuleAssignment{
			SlotID:         assignment.slotID,
			ItemInstanceID: assignment.itemInstanceID,
			Definition:     definition,
		})
	}
	return validated, nil
}

func validateRoleLevels(roleLevels map[PilotRole]int, definition ModuleDefinition) error {
	for role, level := range roleLevels {
		if err := role.Validate(); err != nil {
			return err
		}
		if level < 0 {
			return fmt.Errorf("role %q level %d: %w", role, level, ErrInvalidPlayerRoleLevel)
		}
	}
	for _, requirement := range definition.RequiredRoleLevels {
		level := 1
		if storedLevel, ok := roleLevels[requirement.Role]; ok {
			level = storedLevel
		}
		if level < requirement.Level {
			return fmt.Errorf("role %q level %d required %d for %q: %w", requirement.Role, level, requirement.Level, definition.ItemID, ErrPlayerRoleLevelTooLow)
		}
	}
	return nil
}

func validateModuleEquipLocation(location economy.ItemLocation, equippedOnTargetShip bool) error {
	if err := location.Validate(); err != nil {
		return err
	}
	if location.Kind == economy.LocationKindShipEquipped && equippedOnTargetShip {
		return nil
	}
	if economy.IsBlockedPlayerTradeOrEquipLocationKind(location.Kind) {
		return fmt.Errorf("location kind %q: %w", location.Kind, ErrBlockedModuleItemLocation)
	}
	if location.Kind == economy.LocationKindAccountInventory {
		return nil
	}
	if equippedOnTargetShip {
		return nil
	}
	return fmt.Errorf("location kind %q: %w", location.Kind, ErrInvalidModuleItemLocation)
}

func moduleCompatibleWithSlot(definition ModuleDefinition, slotType ModuleSlotType) bool {
	for _, compatible := range definition.CompatibleSlotTypes {
		if compatible == slotType {
			return true
		}
	}
	return false
}

func buildTargetEquipped(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	assignments SlotAssignments,
	current []EquippedModule,
	equippedAt time.Time,
) []EquippedModule {
	currentBySlot := make(map[ModuleSlotID]EquippedModule, len(current))
	for _, equipped := range current {
		currentBySlot[equipped.SlotID] = equipped
	}

	target := make([]EquippedModule, 0, len(assignments))
	for _, assignment := range sortedSlotAssignments(assignments) {
		if existing, ok := currentBySlot[assignment.slotID]; ok && existing.ItemInstanceID == assignment.itemInstanceID {
			target = append(target, existing)
			continue
		}
		target = append(target, EquippedModule{
			PlayerID:       playerID,
			ShipID:         shipID,
			SlotID:         assignment.slotID,
			ItemInstanceID: assignment.itemInstanceID,
			EquippedAt:     equippedAt,
		})
	}
	return target
}

func diffEquippedModules(current []EquippedModule, target []EquippedModule) ([]EquippedModule, []EquippedModule) {
	currentBySlot := make(map[ModuleSlotID]EquippedModule, len(current))
	targetBySlot := make(map[ModuleSlotID]EquippedModule, len(target))
	for _, equipped := range current {
		currentBySlot[equipped.SlotID] = equipped
	}
	for _, equipped := range target {
		targetBySlot[equipped.SlotID] = equipped
	}

	equipped := make([]EquippedModule, 0)
	unequipped := make([]EquippedModule, 0)
	for slotID, currentModule := range currentBySlot {
		targetModule, ok := targetBySlot[slotID]
		if !ok || targetModule.ItemInstanceID != currentModule.ItemInstanceID {
			unequipped = append(unequipped, currentModule)
		}
	}
	for slotID, targetModule := range targetBySlot {
		currentModule, ok := currentBySlot[slotID]
		if !ok || currentModule.ItemInstanceID != targetModule.ItemInstanceID {
			equipped = append(equipped, targetModule)
		}
	}
	sortEquippedModules(equipped)
	sortEquippedModules(unequipped)
	return equipped, unequipped
}

func buildStatInvalidationSignals(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	loadoutID LoadoutID,
	equipped []EquippedModule,
	unequipped []EquippedModule,
	createdAt time.Time,
) []StatInvalidationSignal {
	signals := make([]StatInvalidationSignal, 0, len(equipped)+len(unequipped)+1)
	for _, module := range unequipped {
		signals = append(signals, StatInvalidationSignal{
			PlayerID:  playerID,
			ShipID:    shipID,
			Reason:    StatInvalidationReasonModuleUnequipped,
			SourceID:  module.ItemInstanceID.String(),
			CreatedAt: createdAt,
		})
	}
	for _, module := range equipped {
		signals = append(signals, StatInvalidationSignal{
			PlayerID:  playerID,
			ShipID:    shipID,
			Reason:    StatInvalidationReasonModuleEquipped,
			SourceID:  module.ItemInstanceID.String(),
			CreatedAt: createdAt,
		})
	}
	signals = append(signals, StatInvalidationSignal{
		PlayerID:  playerID,
		ShipID:    shipID,
		Reason:    StatInvalidationReasonLoadoutApplied,
		SourceID:  loadoutID.String(),
		CreatedAt: createdAt,
	})
	return signals
}

func buildModuleBreakStatInvalidationSignal(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	itemInstanceID foundation.ItemID,
	createdAt time.Time,
) StatInvalidationSignal {
	return StatInvalidationSignal{
		PlayerID:  playerID,
		ShipID:    shipID,
		Reason:    StatInvalidationReasonModuleDurabilityBroken,
		SourceID:  itemInstanceID.String(),
		CreatedAt: createdAt,
	}
}

func sortEquippedModules(equipped []EquippedModule) {
	sort.Slice(equipped, func(i, j int) bool {
		return equipped[i].SlotID.String() < equipped[j].SlotID.String()
	})
}

func cloneEquippedModules(equipped []EquippedModule) []EquippedModule {
	if len(equipped) == 0 {
		return nil
	}
	cloned := append([]EquippedModule(nil), equipped...)
	sortEquippedModules(cloned)
	return cloned
}

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/world/worker"
)

type inventorySnapshotPayload struct {
	Stackable []inventoryStackPayload    `json:"stackable"`
	Instances []inventoryInstancePayload `json:"instances"`
	Counts    inventoryCountsPayload     `json:"counts"`
}

type inventoryStackPayload struct {
	ItemID      string `json:"item_id"`
	DisplayName string `json:"display_name,omitempty"`
	Quantity    int64  `json:"quantity"`
	Location    string `json:"location"`
}

type inventoryInstancePayload struct {
	ItemInstanceID    string `json:"item_instance_id"`
	ItemID            string `json:"item_id"`
	DisplayName       string `json:"display_name,omitempty"`
	Location          string `json:"location"`
	Rarity            string `json:"rarity,omitempty"`
	ItemType          string `json:"item_type,omitempty"`
	ModuleSlotType    string `json:"module_slot_type,omitempty"`
	ModuleCategory    string `json:"module_category,omitempty"`
	DurabilityCurrent int64  `json:"durability_current,omitempty"`
	DurabilityMax     int64  `json:"durability_max,omitempty"`
	BoundState        string `json:"bound_state,omitempty"`
}

type inventoryCountsPayload struct {
	CargoStacks       int `json:"cargo_stacks"`
	StorageStacks     int `json:"storage_stacks"`
	EquippedInstances int `json:"equipped_instances"`
}

type hangarSnapshotPayload struct {
	ActiveShipID string              `json:"active_ship_id"`
	Ships        []hangarShipPayload `json:"ships"`
}

type hangarShipPayload struct {
	ShipID          string `json:"ship_id"`
	DisplayName     string `json:"display_name"`
	State           string `json:"state"`
	Role            string `json:"role,omitempty"`
	Tier            int    `json:"tier,omitempty"`
	RankRequirement int    `json:"rank_requirement,omitempty"`
	Hull            int    `json:"hull"`
	MaxHull         int    `json:"max_hull"`
	Shield          int    `json:"shield"`
	MaxShield       int    `json:"max_shield"`
	Capacitor       int    `json:"capacitor,omitempty"`
	MaxCapacitor    int    `json:"max_capacitor,omitempty"`
	Speed           int64  `json:"speed,omitempty"`
	Radar           int64  `json:"radar,omitempty"`
	CargoCapacity   int64  `json:"cargo_capacity,omitempty"`
	SlotOffensive   int    `json:"slot_offensive,omitempty"`
	SlotDefensive   int    `json:"slot_defensive,omitempty"`
	SlotUtility     int    `json:"slot_utility,omitempty"`
	Disabled        bool   `json:"disabled"`
	Active          bool   `json:"active"`
	LockedReason    string `json:"locked_reason,omitempty"`
}

type hangarActivateShipPayload struct {
	ShipID string `json:"ship_id"`
}

type loadoutSnapshotPayload struct {
	ActiveShipID string               `json:"active_ship_id"`
	Slots        []loadoutSlotPayload `json:"slots"`
}

type loadoutSlotPayload struct {
	SlotID         string `json:"slot_id"`
	SlotType       string `json:"slot_type"`
	ModuleItemID   string `json:"module_item_id,omitempty"`
	ItemInstanceID string `json:"item_instance_id,omitempty"`
	ModuleID       string `json:"module_id,omitempty"`
	DisplayName    string `json:"display_name,omitempty"`
	ModuleState    string `json:"module_state,omitempty"`
	Durability     int64  `json:"durability,omitempty"`
	DurabilityMax  int64  `json:"durability_max,omitempty"`
}

type loadoutEquipModulePayload struct {
	SlotID         string `json:"slot_id"`
	ItemInstanceID string `json:"item_instance_id"`
}

type loadoutUnequipModulePayload struct {
	SlotID string `json:"slot_id"`
}

type craftingSnapshotPayload struct {
	Recipes    []craftingRecipePayload `json:"recipes"`
	ActiveJobs []craftingJobPayload    `json:"active_jobs"`
}

type craftingRecipePayload struct {
	RecipeID             string                    `json:"recipe_id"`
	Category             string                    `json:"category"`
	Output               craftingOutputPayload     `json:"output"`
	Inputs               []craftingInputPayload    `json:"inputs"`
	RequiredCredits      int64                     `json:"required_credits"`
	RequiredRank         int                       `json:"required_rank"`
	RequiredRoleLevels   []craftingRoleRequirement `json:"required_role_levels,omitempty"`
	RequiredLocationType string                    `json:"required_location_type"`
	CraftDurationMS      int64                     `json:"craft_duration_ms"`
	Repeatable           bool                      `json:"repeatable"`
}

type craftingInputPayload struct {
	ItemID   string `json:"item_id"`
	Quantity int64  `json:"quantity"`
}

type craftingOutputPayload struct {
	Kind      string `json:"kind"`
	ItemID    string `json:"item_id,omitempty"`
	ShipID    string `json:"ship_id,omitempty"`
	Quantity  int64  `json:"quantity"`
	Tradeable bool   `json:"tradeable"`
}

type craftingRoleRequirement struct {
	Role  string `json:"role"`
	Level int    `json:"level"`
}

type craftingJobPayload struct {
	JobID       string `json:"job_id"`
	RecipeID    string `json:"recipe_id"`
	State       string `json:"state"`
	StartedAt   int64  `json:"started_at"`
	CompletesAt int64  `json:"completes_at"`
}

func (runtime *Runtime) handleProgressionSnapshot(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	snapshot, err := runtime.Progression.GetProgressionSnapshot(ctx.PlayerID)
	if err != nil {
		return nil, err
	}
	return marshalPayload(map[string]any{
		"progression": progressionPayload(snapshot),
	})
}

func (runtime *Runtime) handleInventorySnapshot(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if _, ok := runtime.players[ctx.PlayerID]; !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	return marshalPayload(map[string]any{
		"inventory": runtime.inventorySnapshotLocked(ctx.PlayerID),
		"cargo":     runtime.cargoSnapshotFromInventoryLocked(ctx.PlayerID),
	})
}

func (runtime *Runtime) handleHangarSnapshot(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if _, ok := runtime.players[ctx.PlayerID]; !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	hangar, err := runtime.hangarSnapshotLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForHangar(err)
	}
	return marshalPayload(map[string]any{
		"hangar": hangar,
	})
}

func (runtime *Runtime) handleHangarActivateShip(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload hangarActivateShipPayload
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	shipID := foundation.ShipID(payload.ShipID)

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if _, ok := runtime.players[ctx.PlayerID]; !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	result, err := runtime.Hangar.SetActiveShip(ships.SetActiveShipInput{
		PlayerID: ctx.PlayerID,
		ShipID:   shipID,
		Context:  runtime.shipSwapContextLocked(ctx.PlayerID),
	})
	if err != nil {
		return nil, domainErrorForHangar(err)
	}
	if result.ActiveShip.ShipID != "" {
		if err := runtime.applyActiveShipLocked(ctx.PlayerID, result.ActiveShip.ShipID); err != nil {
			return nil, domainErrorForHangar(err)
		}
	}
	return runtime.hangarMutationResponseLocked(authSessionID(ctx.SessionID), ctx.PlayerID)
}

func (runtime *Runtime) handleLoadoutSnapshot(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if _, ok := runtime.players[ctx.PlayerID]; !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	loadout, err := runtime.loadoutSnapshotLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForLoadout(err)
	}
	return marshalPayload(map[string]any{
		"loadout": loadout,
	})
}

func (runtime *Runtime) handleLoadoutEquipModule(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload loadoutEquipModulePayload
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	slotID := modules.ModuleSlotID(payload.SlotID)
	itemInstanceID := foundation.ItemID(payload.ItemInstanceID)

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if _, ok := runtime.players[ctx.PlayerID]; !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	if err := runtime.equipModuleLocked(ctx.PlayerID, slotID, itemInstanceID, request.RequestID); err != nil {
		return nil, domainErrorForLoadout(err)
	}
	return runtime.loadoutMutationResponseLocked(authSessionID(ctx.SessionID), ctx.PlayerID)
}

func (runtime *Runtime) handleLoadoutUnequipModule(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload loadoutUnequipModulePayload
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	slotID := modules.ModuleSlotID(payload.SlotID)

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if _, ok := runtime.players[ctx.PlayerID]; !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	if err := runtime.unequipModuleLocked(ctx.PlayerID, slotID, request.RequestID); err != nil {
		return nil, domainErrorForLoadout(err)
	}
	return runtime.loadoutMutationResponseLocked(authSessionID(ctx.SessionID), ctx.PlayerID)
}

func (runtime *Runtime) handleStatsSnapshot(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	state, ok := runtime.players[ctx.PlayerID]
	if !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	return marshalPayload(map[string]any{
		"stats": state.Stats,
	})
}

func (runtime *Runtime) handleCraftingRecipes(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if err := ctx.PlayerID.Validate(); err != nil {
		return nil, err
	}
	return marshalPayload(map[string]any{
		"crafting": runtime.craftingSnapshot(),
	})
}

func (runtime *Runtime) inventorySnapshotLocked(playerID foundation.PlayerID) inventorySnapshotPayload {
	activeCargo := runtime.activeCargoLocationLocked(playerID)
	stackable := make([]inventoryStackPayload, 0)
	instances := make([]inventoryInstancePayload, 0)
	counts := inventoryCountsPayload{}

	for _, item := range runtime.Inventory.StackableItems() {
		if item.OwnerPlayerID != playerID {
			continue
		}
		location := publicInventoryLocation(item.Location)
		if item.Location == activeCargo {
			counts.CargoStacks++
		} else if isStorageLocation(item.Location.Kind) {
			counts.StorageStacks++
		}
		stackable = append(stackable, inventoryStackPayload{
			ItemID:      item.ItemID.String(),
			DisplayName: runtime.itemDisplayName(item.ItemID),
			Quantity:    item.Quantity.Int64(),
			Location:    location,
		})
	}

	for _, item := range runtime.Inventory.InstanceItems() {
		if item.OwnerPlayerID != playerID {
			continue
		}
		location := publicInventoryLocation(item.Location)
		if item.Location.Kind == economy.LocationKindShipEquipped {
			counts.EquippedInstances++
		}
		definition, definitionOK := runtime.itemCatalog[item.ItemID]
		moduleDefinition, moduleOK := runtime.ModuleCatalog.Lookup(item.ItemID)
		displayName := ""
		rarity := ""
		itemType := ""
		if definitionOK {
			displayName = definition.Name
			rarity = definition.Rarity.String()
			itemType = definition.Type.String()
		}
		moduleSlotType := ""
		moduleCategory := ""
		durabilityMax := int64(0)
		if moduleOK {
			moduleSlotType = moduleDefinition.SlotType.String()
			moduleCategory = moduleDefinition.Category.String()
			durabilityMax = moduleDefinition.Durability.Max
		}
		instances = append(instances, inventoryInstancePayload{
			ItemInstanceID:    item.ItemInstanceID.String(),
			ItemID:            item.ItemID.String(),
			DisplayName:       displayName,
			Location:          location,
			Rarity:            rarity,
			ItemType:          itemType,
			ModuleSlotType:    moduleSlotType,
			ModuleCategory:    moduleCategory,
			DurabilityCurrent: item.DurabilityCurrent,
			DurabilityMax:     durabilityMax,
			BoundState:        item.BoundState.String(),
		})
	}

	sort.Slice(stackable, func(i, j int) bool {
		if stackable[i].Location == stackable[j].Location {
			return stackable[i].ItemID < stackable[j].ItemID
		}
		return stackable[i].Location < stackable[j].Location
	})
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].Location == instances[j].Location {
			return instances[i].ItemInstanceID < instances[j].ItemInstanceID
		}
		return instances[i].Location < instances[j].Location
	})

	return inventorySnapshotPayload{
		Stackable: stackable,
		Instances: instances,
		Counts:    counts,
	}
}

func (runtime *Runtime) hangarSnapshotLocked(playerID foundation.PlayerID) (hangarSnapshotPayload, error) {
	state := runtime.players[playerID]
	hangar, err := runtime.Hangar.GetHangar(playerID)
	if err != nil {
		return hangarSnapshotPayload{}, err
	}
	shipsPayload := make([]hangarShipPayload, 0, len(hangar.Ships))
	for _, playerShip := range hangar.Ships {
		definition, err := runtime.ShipCatalog.MustGet(playerShip.ShipID)
		if err != nil {
			return hangarSnapshotPayload{}, err
		}
		active := playerShip.ShipID.String() == state.Ship.ActiveShipID
		payload := hangarShipPayload{
			ShipID:          playerShip.ShipID.String(),
			DisplayName:     definition.Name,
			State:           playerShip.State.String(),
			Role:            definition.Role.String(),
			Tier:            definition.Tier,
			RankRequirement: definition.RankRequirement,
			Hull:            int(definition.BaseStats.HP),
			MaxHull:         int(definition.BaseStats.HP),
			Shield:          int(definition.BaseStats.Shield),
			MaxShield:       int(definition.BaseStats.Shield),
			Capacitor:       int(definition.BaseStats.Energy),
			MaxCapacitor:    int(definition.BaseStats.Energy),
			Speed:           definition.BaseStats.Speed,
			Radar:           definition.BaseStats.Radar,
			CargoCapacity:   definition.BaseStats.CargoCapacity,
			SlotOffensive:   definition.Slots.Offensive,
			SlotDefensive:   definition.Slots.Defensive,
			SlotUtility:     definition.Slots.Utility,
			Disabled:        playerShip.State == ships.ShipStateDisabled,
			Active:          active,
		}
		if active {
			payload.DisplayName = state.Ship.DisplayName
			payload.State = state.Ship.RepairState
			payload.Hull = state.Ship.Hull
			payload.MaxHull = state.Ship.MaxHull
			payload.Shield = state.Ship.Shield
			payload.MaxShield = state.Ship.MaxShield
			payload.Capacitor = state.Ship.Capacitor
			payload.MaxCapacitor = state.Ship.MaxCapacitor
			payload.Speed = int64(state.Stats.Speed)
			payload.Radar = int64(state.Stats.RadarRange)
			payload.CargoCapacity = state.Cargo.Capacity
			payload.Disabled = state.Ship.Disabled
			if payload.State == "" {
				payload.State = ships.ShipStateActive.String()
			}
		}
		payload.LockedReason = hangarShipLockedReason(playerShip, definition, active)
		shipsPayload = append(shipsPayload, payload)
	}
	activeShipID := state.Ship.ActiveShipID
	if hangar.HasActiveShip {
		activeShipID = hangar.ActiveShip.ShipID.String()
	}
	return hangarSnapshotPayload{
		ActiveShipID: activeShipID,
		Ships:        shipsPayload,
	}, nil
}

func hangarShipLockedReason(playerShip ships.PlayerShipState, definition ships.ShipDefinition, active bool) string {
	switch {
	case active:
		return "active"
	case playerShip.State == ships.ShipStateDisabled:
		if playerShip.DisabledReason != "" {
			return playerShip.DisabledReason
		}
		return "disabled"
	case playerShip.State == ships.ShipStateRepairing:
		return "repairing"
	case playerShip.State == ships.ShipStateLocked:
		return "locked"
	case definition.RankRequirement > 1:
		return fmt.Sprintf("requires rank %d", definition.RankRequirement)
	default:
		return ""
	}
}

func (runtime *Runtime) loadoutSnapshotLocked(playerID foundation.PlayerID) (loadoutSnapshotPayload, error) {
	state := runtime.players[playerID]
	shipID := foundation.ShipID(state.Ship.ActiveShipID)
	layout, err := (runtimeShipSlotLayoutProvider{}).SlotLayoutForShip(shipID)
	if err != nil {
		return loadoutSnapshotPayload{}, err
	}
	equipped, err := runtime.LoadoutStore.EquippedModules(playerID, shipID)
	if err != nil {
		return loadoutSnapshotPayload{}, err
	}
	equippedBySlot := make(map[modules.ModuleSlotID]modules.EquippedModule, len(equipped))
	for _, module := range equipped {
		equippedBySlot[module.SlotID] = module
	}

	slots := make([]loadoutSlotPayload, 0, layout.Offensive+layout.Defensive+layout.Utility)
	for _, slot := range slotPayloadDefinitions(layout) {
		payload := loadoutSlotPayload{
			SlotID:   slot.SlotID.String(),
			SlotType: slot.Type.String(),
		}
		if equippedModule, ok := equippedBySlot[slot.SlotID]; ok {
			item, err := runtime.LoadoutStore.ModuleItem(equippedModule.ItemInstanceID)
			if err != nil {
				return loadoutSnapshotPayload{}, err
			}
			moduleDefinition, ok := runtime.ModuleCatalog.Lookup(item.ItemID)
			if !ok {
				return loadoutSnapshotPayload{}, fmt.Errorf("module item %q: %w", item.ItemID, modules.ErrUnknownModuleDefinition)
			}
			payload.ModuleItemID = item.ItemID.String()
			payload.ItemInstanceID = item.ItemInstanceID.String()
			payload.ModuleID = moduleDefinition.ItemID.String()
			payload.DisplayName = moduleDefinition.Name
			payload.ModuleState = "online"
			if item.DurabilityCurrent <= 0 {
				payload.ModuleState = "broken"
			}
			payload.Durability = item.DurabilityCurrent
			payload.DurabilityMax = moduleDefinition.Durability.Max
		}
		slots = append(slots, payload)
	}
	return loadoutSnapshotPayload{
		ActiveShipID: state.Ship.ActiveShipID,
		Slots:        slots,
	}, nil
}

func (runtime *Runtime) craftingSnapshot() craftingSnapshotPayload {
	definitions := runtime.Recipes.Definitions()
	recipes := make([]craftingRecipePayload, 0, len(definitions))
	for _, definition := range definitions {
		recipes = append(recipes, craftingRecipe(definition))
	}
	return craftingSnapshotPayload{
		Recipes:    recipes,
		ActiveJobs: []craftingJobPayload{},
	}
}

func (runtime *Runtime) equipModuleLocked(playerID foundation.PlayerID, slotID modules.ModuleSlotID, itemInstanceID foundation.ItemID, requestID foundation.RequestID) error {
	if err := slotID.Validate(); err != nil {
		return err
	}
	if err := itemInstanceID.Validate(); err != nil {
		return err
	}
	if err := requestID.Validate(); err != nil {
		return err
	}
	if err := runtime.syncLoadoutModuleItemsLocked(playerID); err != nil {
		return err
	}
	shipID := foundation.ShipID(runtime.players[playerID].Ship.ActiveShipID)
	current, err := runtime.LoadoutStore.EquippedModules(playerID, shipID)
	if err != nil {
		return err
	}
	assignments := make(modules.SlotAssignments, len(current)+1)
	for _, equipped := range current {
		if equipped.SlotID == slotID || equipped.ItemInstanceID == itemInstanceID {
			continue
		}
		assignments[equipped.SlotID] = equipped.ItemInstanceID
	}
	assignments[slotID] = itemInstanceID

	validationContext, err := runtime.loadoutValidationContextLocked(playerID, shipID)
	if err != nil {
		return err
	}
	if _, err := runtime.Loadout.ValidateModuleAssignments(validationContext, assignments); err != nil {
		return err
	}
	return runtime.LoadoutStore.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID:  playerID,
		ShipID:    shipID,
		Equipped:  runtimeTargetEquippedModules(playerID, shipID, assignments, current, runtime.clock.Now()),
		RequestID: requestID,
	})
}

func (runtime *Runtime) unequipModuleLocked(playerID foundation.PlayerID, slotID modules.ModuleSlotID, requestID foundation.RequestID) error {
	if err := slotID.Validate(); err != nil {
		return err
	}
	if err := requestID.Validate(); err != nil {
		return err
	}
	if err := runtime.syncLoadoutModuleItemsLocked(playerID); err != nil {
		return err
	}
	shipID := foundation.ShipID(runtime.players[playerID].Ship.ActiveShipID)
	current, err := runtime.LoadoutStore.EquippedModules(playerID, shipID)
	if err != nil {
		return err
	}
	validationContext, err := runtime.loadoutValidationContextLocked(playerID, shipID)
	if err != nil {
		return err
	}
	if err := validationContext.ShipSlots.ValidateSlot(slotID); err != nil {
		return err
	}
	assignments := make(modules.SlotAssignments, len(current))
	for _, equipped := range current {
		if equipped.SlotID == slotID {
			continue
		}
		assignments[equipped.SlotID] = equipped.ItemInstanceID
	}
	if _, err := runtime.Loadout.ValidateModuleAssignments(validationContext, assignments); err != nil {
		return err
	}
	return runtime.LoadoutStore.ReplaceEquippedModules(modules.ReplaceEquippedModulesInput{
		PlayerID:  playerID,
		ShipID:    shipID,
		Equipped:  runtimeTargetEquippedModules(playerID, shipID, assignments, current, runtime.clock.Now()),
		RequestID: requestID,
	})
}

func (runtime *Runtime) loadoutMutationResponseLocked(sessionID auth.SessionID, playerID foundation.PlayerID) (json.RawMessage, error) {
	loadout, err := runtime.loadoutSnapshotLocked(playerID)
	if err != nil {
		return nil, domainErrorForLoadout(err)
	}
	inventory := runtime.inventorySnapshotLocked(playerID)
	stats := runtime.players[playerID].Stats
	runtime.queueEventLocked(sessionID, realtime.EventInventorySnapshot, inventory)
	runtime.queueEventLocked(sessionID, realtime.EventLoadoutSnapshot, loadout)
	runtime.queueEventLocked(sessionID, realtime.EventStatsUpdated, stats)
	return marshalPayload(map[string]any{
		"inventory": inventory,
		"loadout":   loadout,
		"stats":     stats,
	})
}

func (runtime *Runtime) hangarMutationResponseLocked(sessionID auth.SessionID, playerID foundation.PlayerID) (json.RawMessage, error) {
	hangar, err := runtime.hangarSnapshotLocked(playerID)
	if err != nil {
		return nil, domainErrorForHangar(err)
	}
	loadout, err := runtime.loadoutSnapshotLocked(playerID)
	if err != nil {
		return nil, domainErrorForLoadout(err)
	}
	state := runtime.players[playerID]
	cargo := runtime.cargoSnapshotFromInventoryLocked(playerID)
	runtime.queueEventLocked(sessionID, realtime.EventHangarSnapshot, hangar)
	runtime.queueEventLocked(sessionID, realtime.EventShipSnapshot, state.Ship)
	runtime.queueEventLocked(sessionID, realtime.EventStatsUpdated, state.Stats)
	runtime.queueEventLocked(sessionID, realtime.EventCargoSnapshot, cargo)
	runtime.queueEventLocked(sessionID, realtime.EventLoadoutSnapshot, loadout)
	return marshalPayload(map[string]any{
		"hangar":  hangar,
		"ship":    state.Ship,
		"stats":   state.Stats,
		"cargo":   cargo,
		"loadout": loadout,
	})
}

func (runtime *Runtime) syncLoadoutModuleItemsLocked(playerID foundation.PlayerID) error {
	for _, item := range runtime.Inventory.InstanceItems() {
		if item.OwnerPlayerID != playerID {
			continue
		}
		if _, ok := runtime.ModuleCatalog.Lookup(item.ItemID); !ok {
			continue
		}
		if err := runtime.LoadoutStore.PutModuleItem(item); err != nil {
			return err
		}
	}
	return nil
}

func (runtime *Runtime) loadoutValidationContextLocked(playerID foundation.PlayerID, shipID foundation.ShipID) (modules.LoadoutValidationContext, error) {
	shipSlots, err := (runtimeShipSlotLayoutProvider{}).SlotLayoutForShip(shipID)
	if err != nil {
		return modules.LoadoutValidationContext{}, err
	}
	progressionSnapshot, err := (runtimeLoadoutProgressionProvider{progression: runtime.Progression}).ProgressionForPlayer(playerID)
	if err != nil {
		return modules.LoadoutValidationContext{}, err
	}
	return modules.LoadoutValidationContext{
		PlayerID:   playerID,
		ShipID:     shipID,
		ShipSlots:  shipSlots,
		PlayerRank: progressionSnapshot.Rank,
		RoleLevels: progressionSnapshot.RoleLevels,
	}, nil
}

func runtimeTargetEquippedModules(
	playerID foundation.PlayerID,
	shipID foundation.ShipID,
	assignments modules.SlotAssignments,
	current []modules.EquippedModule,
	equippedAt time.Time,
) []modules.EquippedModule {
	currentBySlot := make(map[modules.ModuleSlotID]modules.EquippedModule, len(current))
	for _, equipped := range current {
		currentBySlot[equipped.SlotID] = equipped
	}
	target := make([]modules.EquippedModule, 0, len(assignments))
	for slotID, itemInstanceID := range assignments {
		if existing, ok := currentBySlot[slotID]; ok && existing.ItemInstanceID == itemInstanceID {
			target = append(target, existing)
			continue
		}
		target = append(target, modules.EquippedModule{
			PlayerID:       playerID,
			ShipID:         shipID,
			SlotID:         slotID,
			ItemInstanceID: itemInstanceID,
			EquippedAt:     equippedAt,
		})
	}
	sort.Slice(target, func(i, j int) bool {
		return target[i].SlotID.String() < target[j].SlotID.String()
	})
	return target
}

func craftingRecipe(definition crafting.RecipeDefinition) craftingRecipePayload {
	inputs := make([]craftingInputPayload, 0, len(definition.Inputs))
	for _, input := range definition.Inputs {
		inputs = append(inputs, craftingInputPayload{
			ItemID:   input.ItemID.String(),
			Quantity: input.Quantity.Int64(),
		})
	}
	requirements := make([]craftingRoleRequirement, 0, len(definition.RequiredRoleLevels))
	for _, requirement := range definition.RequiredRoleLevels {
		requirements = append(requirements, craftingRoleRequirement{
			Role:  requirement.Role.String(),
			Level: requirement.Level,
		})
	}
	return craftingRecipePayload{
		RecipeID:             definition.RecipeID.String(),
		Category:             definition.Category.String(),
		Output:               craftingOutput(definition.Output),
		Inputs:               inputs,
		RequiredCredits:      definition.RequiredCredits.Int64(),
		RequiredRank:         definition.RequiredRank,
		RequiredRoleLevels:   requirements,
		RequiredLocationType: definition.RequiredLocationType.String(),
		CraftDurationMS:      definition.CraftDuration.Milliseconds(),
		Repeatable:           definition.Repeatable,
	}
}

func craftingOutput(output crafting.RecipeOutput) craftingOutputPayload {
	return craftingOutputPayload{
		Kind:      output.Kind.String(),
		ItemID:    output.ItemID.String(),
		ShipID:    output.ShipID.String(),
		Quantity:  output.Quantity.Int64(),
		Tradeable: output.Tradeable,
	}
}

func slotPayloadDefinitions(layout modules.ShipSlotLayout) []modules.SlotDefinition {
	slots := make([]modules.SlotDefinition, 0, layout.Offensive+layout.Defensive+layout.Utility)
	appendSlots := func(slotType modules.ModuleSlotType, count int) {
		for index := 1; index <= count; index++ {
			slots = append(slots, modules.SlotDefinition{
				SlotID: modules.ModuleSlotID(fmt.Sprintf("%s_%d", slotType.String(), index)),
				Type:   slotType,
			})
		}
	}
	appendSlots(modules.ModuleSlotTypeOffensive, layout.Offensive)
	appendSlots(modules.ModuleSlotTypeDefensive, layout.Defensive)
	appendSlots(modules.ModuleSlotTypeUtility, layout.Utility)
	return slots
}

func (runtime *Runtime) itemDisplayName(itemID foundation.ItemID) string {
	definition, ok := runtime.itemCatalog[itemID]
	if !ok {
		return ""
	}
	return definition.Name
}

func publicInventoryLocation(location economy.ItemLocation) string {
	return location.Kind.String()
}

func isStorageLocation(kind economy.LocationKind) bool {
	switch kind {
	case economy.LocationKindAccountInventory, economy.LocationKindStationStorage, economy.LocationKindPlanetStorage:
		return true
	default:
		return false
	}
}

func domainErrorForLoadout(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, modules.ErrUnknownModuleItem), errors.Is(err, modules.ErrUnknownModuleDefinition):
		return foundation.NewDomainError(foundation.CodeNotFound, "Module item was not found.", foundation.WithCause(err))
	case errors.Is(err, modules.ErrModuleItemNotOwned), errors.Is(err, economy.ErrItemNotOwned):
		return foundation.NewDomainError(foundation.CodeForbidden, "Module item is not owned by this player.", foundation.WithCause(err))
	case errors.Is(err, modules.ErrModuleItemAlreadyEquipped):
		return foundation.NewDomainError(foundation.CodeForbidden, "Module item is already equipped elsewhere.", foundation.WithCause(err))
	case errors.Is(err, modules.ErrModuleBroken):
		return foundation.NewDomainError(foundation.CodeForbidden, "Module item is broken.", foundation.WithCause(err))
	case errors.Is(err, modules.ErrPlayerRankTooLow), errors.Is(err, modules.ErrPlayerRoleLevelTooLow):
		return foundation.NewDomainError(foundation.CodeForbidden, "Pilot requirements are not met.", foundation.WithCause(err))
	case errors.Is(err, modules.ErrWrongModuleSlotType),
		errors.Is(err, modules.ErrModuleSlotUnavailable),
		errors.Is(err, modules.ErrInvalidModuleSlotID),
		errors.Is(err, modules.ErrInvalidModuleSlotType),
		errors.Is(err, modules.ErrInvalidModuleItemLocation),
		errors.Is(err, modules.ErrBlockedModuleItemLocation),
		errors.Is(err, modules.ErrDuplicateModuleAssignment),
		errors.Is(err, economy.ErrMoveItemSameSourceAndTarget),
		errors.Is(err, foundation.ErrEmptyID),
		errors.Is(err, foundation.ErrInvalidID):
		return foundation.NewDomainError(foundation.CodeInvalidPayload, "Loadout request is not valid.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Loadout command failed.", foundation.WithCause(err))
	}
}

func domainErrorForHangar(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, ships.ErrShipNotUnlocked), errors.Is(err, ships.ErrUnknownShipDefinition):
		return foundation.NewDomainError(foundation.CodeNotFound, "Ship was not found in this hangar.", foundation.WithCause(err))
	case errors.Is(err, ships.ErrCannotSwapInCombat),
		errors.Is(err, ships.ErrNotInHangarArea),
		errors.Is(err, ships.ErrShipUnavailable):
		return foundation.NewDomainError(foundation.CodeForbidden, "Ship cannot be activated here.", foundation.WithCause(err))
	case errors.Is(err, ships.ErrShipDisabled):
		return foundation.NewDomainError(foundation.CodeShipDisabled, "Ship is disabled.", foundation.WithCause(err))
	case errors.Is(err, ships.ErrCargoExceedsTargetCapacity):
		return foundation.NewDomainError(foundation.CodeNotEnoughCargo, "Cargo exceeds target ship capacity.", foundation.WithCause(err))
	case errors.Is(err, ships.ErrShipRankRequirementNotMet):
		return foundation.NewDomainError(foundation.CodeRankTooLow, "Ship rank requirement is not met.", foundation.WithCause(err))
	case errors.Is(err, foundation.ErrEmptyID),
		errors.Is(err, foundation.ErrInvalidID),
		errors.Is(err, ships.ErrInvalidCurrentCargoAmount),
		errors.Is(err, ships.ErrInvalidTargetCargoCapacity):
		return foundation.NewDomainError(foundation.CodeInvalidPayload, "Hangar request is not valid.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Hangar command failed.", foundation.WithCause(err))
	}
}

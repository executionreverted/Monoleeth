package server

import (
	"encoding/json"
	"sort"

	"gameproject/internal/game/crafting"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
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
	DurabilityCurrent int64  `json:"durability_current,omitempty"`
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
	ShipID      string `json:"ship_id"`
	DisplayName string `json:"display_name"`
	State       string `json:"state"`
	Hull        int    `json:"hull"`
	MaxHull     int    `json:"max_hull"`
	Shield      int    `json:"shield"`
	MaxShield   int    `json:"max_shield"`
	Disabled    bool   `json:"disabled"`
}

type loadoutSnapshotPayload struct {
	ActiveShipID string               `json:"active_ship_id"`
	Slots        []loadoutSlotPayload `json:"slots"`
}

type loadoutSlotPayload struct {
	SlotID        string `json:"slot_id"`
	SlotType      string `json:"slot_type"`
	ModuleItemID  string `json:"module_item_id,omitempty"`
	ModuleID      string `json:"module_id,omitempty"`
	ModuleState   string `json:"module_state,omitempty"`
	Durability    int64  `json:"durability,omitempty"`
	DurabilityMax int64  `json:"durability_max,omitempty"`
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
	return marshalPayload(map[string]any{
		"hangar": runtime.hangarSnapshotLocked(ctx.PlayerID),
	})
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
	return marshalPayload(map[string]any{
		"loadout": runtime.loadoutSnapshotLocked(ctx.PlayerID),
	})
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
		instances = append(instances, inventoryInstancePayload{
			ItemInstanceID:    item.ItemInstanceID.String(),
			ItemID:            item.ItemID.String(),
			DisplayName:       runtime.itemDisplayName(item.ItemID),
			Location:          location,
			DurabilityCurrent: item.DurabilityCurrent,
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

func (runtime *Runtime) hangarSnapshotLocked(playerID foundation.PlayerID) hangarSnapshotPayload {
	state := runtime.players[playerID]
	ship := hangarShipPayload{
		ShipID:      state.Ship.ActiveShipID,
		DisplayName: state.Ship.DisplayName,
		State:       state.Ship.RepairState,
		Hull:        state.Ship.Hull,
		MaxHull:     state.Ship.MaxHull,
		Shield:      state.Ship.Shield,
		MaxShield:   state.Ship.MaxShield,
		Disabled:    state.Ship.Disabled,
	}
	if ship.State == "" {
		ship.State = "ready"
	}
	return hangarSnapshotPayload{
		ActiveShipID: state.Ship.ActiveShipID,
		Ships:        []hangarShipPayload{ship},
	}
}

func (runtime *Runtime) loadoutSnapshotLocked(playerID foundation.PlayerID) loadoutSnapshotPayload {
	state := runtime.players[playerID]
	return loadoutSnapshotPayload{
		ActiveShipID: state.Ship.ActiveShipID,
		Slots: []loadoutSlotPayload{
			{SlotID: "offensive_1", SlotType: "offensive"},
			{SlotID: "defensive_1", SlotType: "defensive"},
			{
				SlotID:        "utility_1",
				SlotType:      "utility",
				ModuleItemID:  starterScannerItemID,
				ModuleID:      starterScannerModuleID,
				ModuleState:   "online",
				Durability:    100,
				DurabilityMax: 100,
			},
		},
	}
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

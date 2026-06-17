package modules

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gameproject/internal/game/foundation"
)

var (
	ErrEmptyLoadoutID             = errors.New("empty loadout id")
	ErrEmptyLoadoutName           = errors.New("empty loadout name")
	ErrZeroLoadoutTimestamp       = errors.New("zero loadout timestamp")
	ErrUnknownLoadout             = errors.New("unknown loadout")
	ErrLoadoutOwnerMismatch       = errors.New("loadout owner mismatch")
	ErrLoadoutShipMismatch        = errors.New("loadout ship mismatch")
	ErrDuplicateModuleAssignment  = errors.New("duplicate module assignment")
	ErrUnknownModuleItem          = errors.New("unknown module item")
	ErrModuleItemNotOwned         = errors.New("module item not owned")
	ErrModuleItemInstanceMismatch = errors.New("module item instance mismatch")
	ErrModuleItemAlreadyEquipped  = errors.New("module item already equipped")
	ErrInvalidModuleItemLocation  = errors.New("invalid module item location")
	ErrBlockedModuleItemLocation  = errors.New("blocked module item location")
	ErrUnknownModuleDefinition    = errors.New("unknown module definition")
	ErrWrongModuleSlotType        = errors.New("wrong module slot type")
	ErrPlayerRankTooLow           = errors.New("player rank too low")
	ErrPlayerRoleLevelTooLow      = errors.New("player role level too low")
	ErrInvalidPlayerRoleLevel     = errors.New("invalid player role level")
	ErrModuleBroken               = errors.New("module broken")
	ErrActiveShipNotFound         = errors.New("active ship not found")
	ErrNilLoadoutStore            = errors.New("nil loadout store")
)

// LoadoutID identifies a saved module assignment set.
type LoadoutID string

// SlotAssignments maps concrete ship slots to item instance ids.
type SlotAssignments map[ModuleSlotID]foundation.ItemID

// Loadout records one saved set of desired module assignments for a ship.
type Loadout struct {
	LoadoutID       LoadoutID           `json:"loadout_id"`
	PlayerID        foundation.PlayerID `json:"player_id"`
	ShipID          foundation.ShipID   `json:"ship_id"`
	Name            string              `json:"name"`
	SlotAssignments SlotAssignments     `json:"slot_assignments_json"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// LoadoutValidationContext contains the explicit player state needed to
// validate module assignment requirements without depending on progression.
type LoadoutValidationContext struct {
	PlayerID   foundation.PlayerID `json:"player_id"`
	ShipID     foundation.ShipID   `json:"ship_id"`
	PlayerRank int                 `json:"player_rank"`
	RoleLevels map[PilotRole]int   `json:"role_levels,omitempty"`
}

// SaveLoadoutInput records the caller-owned data needed to save a loadout.
type SaveLoadoutInput struct {
	LoadoutID       LoadoutID           `json:"loadout_id"`
	PlayerID        foundation.PlayerID `json:"player_id"`
	ShipID          foundation.ShipID   `json:"ship_id"`
	Name            string              `json:"name"`
	SlotAssignments SlotAssignments     `json:"slot_assignments_json"`
	PlayerRank      int                 `json:"player_rank"`
	RoleLevels      map[PilotRole]int   `json:"role_levels,omitempty"`
}

// ApplyLoadoutInput records the caller-owned data needed to apply a loadout.
type ApplyLoadoutInput struct {
	PlayerID   foundation.PlayerID `json:"player_id"`
	LoadoutID  LoadoutID           `json:"loadout_id"`
	PlayerRank int                 `json:"player_rank"`
	RoleLevels map[PilotRole]int   `json:"role_levels,omitempty"`
}

// ValidatedModuleAssignment records one assignment after catalog, item, and
// player requirement checks have passed.
type ValidatedModuleAssignment struct {
	SlotID         ModuleSlotID      `json:"slot_id"`
	ItemInstanceID foundation.ItemID `json:"item_instance_id"`
	Definition     ModuleDefinition  `json:"definition"`
}

// StatInvalidationReason names a module/loadout mutation that makes effective
// stats stale. The service returns these signals instead of wiring an event bus.
type StatInvalidationReason string

const (
	StatInvalidationReasonModuleEquipped   StatInvalidationReason = "module.equipped"
	StatInvalidationReasonModuleUnequipped StatInvalidationReason = "module.unequipped"
	StatInvalidationReasonLoadoutApplied   StatInvalidationReason = "ship.loadout_applied"
)

// StatInvalidationSignal is a local handoff value for the eventual stat cache
// invalidator.
type StatInvalidationSignal struct {
	PlayerID  foundation.PlayerID    `json:"player_id"`
	ShipID    foundation.ShipID      `json:"ship_id"`
	Reason    StatInvalidationReason `json:"reason"`
	SourceID  string                 `json:"source_id,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
}

// ApplyLoadoutResult reports the equipped-state changes and invalidation
// signals produced by a successful ApplyLoadout call.
type ApplyLoadoutResult struct {
	Loadout           Loadout                  `json:"loadout"`
	Current           []EquippedModule         `json:"current"`
	Equipped          []EquippedModule         `json:"equipped"`
	Unequipped        []EquippedModule         `json:"unequipped"`
	StatInvalidations []StatInvalidationSignal `json:"stat_invalidations"`
}

// String returns the stable loadout id representation.
func (id LoadoutID) String() string {
	return string(id)
}

// Validate reports whether id is non-blank.
func (id LoadoutID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrEmptyLoadoutID
	}
	return nil
}

// Clone returns an owned copy of assignments.
func (assignments SlotAssignments) Clone() SlotAssignments {
	if len(assignments) == 0 {
		return nil
	}
	cloned := make(SlotAssignments, len(assignments))
	for slotID, itemInstanceID := range assignments {
		cloned[slotID] = itemInstanceID
	}
	return cloned
}

// Validate reports whether assignments use known slots and unique item
// instances.
func (assignments SlotAssignments) Validate() error {
	seenItems := make(map[foundation.ItemID]ModuleSlotID, len(assignments))
	for slotID, itemInstanceID := range assignments {
		if err := slotID.Validate(); err != nil {
			return err
		}
		if err := itemInstanceID.Validate(); err != nil {
			return err
		}
		if firstSlot, ok := seenItems[itemInstanceID]; ok {
			return fmt.Errorf("item %q in slots %q and %q: %w", itemInstanceID, firstSlot, slotID, ErrDuplicateModuleAssignment)
		}
		seenItems[itemInstanceID] = slotID
	}
	return nil
}

// Validate reports whether loadout has durable identity and assignment data.
func (loadout Loadout) Validate() error {
	if err := loadout.LoadoutID.Validate(); err != nil {
		return err
	}
	if err := loadout.PlayerID.Validate(); err != nil {
		return err
	}
	if err := loadout.ShipID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(loadout.Name) == "" {
		return ErrEmptyLoadoutName
	}
	if err := loadout.SlotAssignments.Validate(); err != nil {
		return err
	}
	if loadout.CreatedAt.IsZero() || loadout.UpdatedAt.IsZero() {
		return ErrZeroLoadoutTimestamp
	}
	return nil
}

func (ctx LoadoutValidationContext) validate() error {
	if err := ctx.PlayerID.Validate(); err != nil {
		return err
	}
	if err := ctx.ShipID.Validate(); err != nil {
		return err
	}
	for role, level := range ctx.RoleLevels {
		if err := role.Validate(); err != nil {
			return err
		}
		if level < 0 {
			return fmt.Errorf("role %q level %d: %w", role, level, ErrInvalidPlayerRoleLevel)
		}
	}
	return nil
}

func (input SaveLoadoutInput) validationContext() LoadoutValidationContext {
	return LoadoutValidationContext{
		PlayerID:   input.PlayerID,
		ShipID:     input.ShipID,
		PlayerRank: input.PlayerRank,
		RoleLevels: cloneRoleLevels(input.RoleLevels),
	}
}

func (input ApplyLoadoutInput) validationContext(shipID foundation.ShipID) LoadoutValidationContext {
	return LoadoutValidationContext{
		PlayerID:   input.PlayerID,
		ShipID:     shipID,
		PlayerRank: input.PlayerRank,
		RoleLevels: cloneRoleLevels(input.RoleLevels),
	}
}

func sortedSlotAssignments(assignments SlotAssignments) []slotAssignment {
	sorted := make([]slotAssignment, 0, len(assignments))
	for slotID, itemInstanceID := range assignments {
		sorted = append(sorted, slotAssignment{
			slotID:         slotID,
			itemInstanceID: itemInstanceID,
		})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].slotID.String() < sorted[j].slotID.String()
	})
	return sorted
}

type slotAssignment struct {
	slotID         ModuleSlotID
	itemInstanceID foundation.ItemID
}

func cloneLoadout(loadout Loadout) Loadout {
	loadout.SlotAssignments = loadout.SlotAssignments.Clone()
	return loadout
}

func cloneRoleLevels(roleLevels map[PilotRole]int) map[PilotRole]int {
	if len(roleLevels) == 0 {
		return nil
	}
	cloned := make(map[PilotRole]int, len(roleLevels))
	for role, level := range roleLevels {
		cloned[role] = level
	}
	return cloned
}

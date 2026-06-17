package modules

import (
	"errors"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

var (
	ErrEmptyModuleName          = errors.New("empty module name")
	ErrInvalidModuleSlotType    = errors.New("invalid module slot type")
	ErrInvalidModuleSlotID      = errors.New("invalid module slot id")
	ErrInvalidModuleCategory    = errors.New("invalid module category")
	ErrInvalidModuleTier        = errors.New("invalid module tier")
	ErrInvalidRequiredRank      = errors.New("invalid required rank")
	ErrInvalidRequiredRoleLevel = errors.New("invalid required role level")
	ErrInvalidPilotRole         = errors.New("invalid pilot role")
	ErrInvalidStatKey           = errors.New("invalid stat key")
	ErrInvalidStatModifierKind  = errors.New("invalid stat modifier kind")
	ErrInvalidStatModifierValue = errors.New("invalid stat modifier value")
	ErrInvalidEnergyValue       = errors.New("invalid energy value")
	ErrInvalidCooldownKey       = errors.New("invalid cooldown key")
	ErrInvalidCooldownDuration  = errors.New("invalid cooldown duration")
	ErrInvalidDurabilityMax     = errors.New("invalid durability max")
	ErrDuplicateRoleRequirement = errors.New("duplicate role requirement")
	ErrDuplicateStatModifier    = errors.New("duplicate stat modifier")
	ErrDuplicateCooldown        = errors.New("duplicate cooldown")
	ErrModuleSourceMismatch     = errors.New("module source definition mismatch")
	ErrSlotCategoryMismatch     = errors.New("module category and slot type mismatch")
	ErrZeroEquippedAt           = errors.New("zero equipped timestamp")
)

const maxCatalogNumericValue int64 = foundation.MaxAmount

// ModuleSlotType identifies the class of ship slot a module can occupy.
type ModuleSlotType string

const (
	ModuleSlotTypeOffensive ModuleSlotType = "offensive"
	ModuleSlotTypeDefensive ModuleSlotType = "defensive"
	ModuleSlotTypeUtility   ModuleSlotType = "utility"
)

// ModuleSlotID identifies a concrete slot on a ship layout.
type ModuleSlotID string

const (
	ModuleSlotOffensive1 ModuleSlotID = "offensive_1"
	ModuleSlotOffensive2 ModuleSlotID = "offensive_2"
	ModuleSlotOffensive3 ModuleSlotID = "offensive_3"
	ModuleSlotOffensive4 ModuleSlotID = "offensive_4"
	ModuleSlotDefensive1 ModuleSlotID = "defensive_1"
	ModuleSlotDefensive2 ModuleSlotID = "defensive_2"
	ModuleSlotDefensive3 ModuleSlotID = "defensive_3"
	ModuleSlotUtility1   ModuleSlotID = "utility_1"
	ModuleSlotUtility2   ModuleSlotID = "utility_2"
	ModuleSlotUtility3   ModuleSlotID = "utility_3"
	ModuleSlotUtility4   ModuleSlotID = "utility_4"
)

// ModuleCategory identifies the gameplay category used by module catalogs.
type ModuleCategory string

const (
	ModuleCategoryOffensive ModuleCategory = "offensive"
	ModuleCategoryDefensive ModuleCategory = "defensive"
	ModuleCategoryUtility   ModuleCategory = "utility"
)

// PilotRole identifies the role level gate a module may require.
type PilotRole string

const (
	PilotRoleCombat       PilotRole = "combat"
	PilotRoleScout        PilotRole = "scout"
	PilotRoleCrafting     PilotRole = "crafting"
	PilotRoleConstruction PilotRole = "construction"
)

// StatKey identifies a server-side stat that a module can modify.
type StatKey string

const (
	StatWeaponDamage  StatKey = "weapon_damage"
	StatWeaponRange   StatKey = "weapon_range"
	StatAccuracy      StatKey = "accuracy"
	StatShieldMax     StatKey = "shield_max"
	StatShieldRegen   StatKey = "shield_regen"
	StatScanPower     StatKey = "scan_power"
	StatScanRadius    StatKey = "scan_radius"
	StatRadarRange    StatKey = "radar_range"
	StatCargoCapacity StatKey = "cargo_capacity"
)

// StatModifierKind identifies how a stat modifier is applied during aggregation.
type StatModifierKind string

const (
	StatModifierFlat    StatModifierKind = "flat"
	StatModifierPercent StatModifierKind = "percent"
)

// CooldownKey identifies a server-timed cooldown affected by a module.
type CooldownKey string

const (
	CooldownBasicAttack CooldownKey = "basic_attack"
	CooldownScanPulse   CooldownKey = "scan_pulse"
	CooldownRadarSweep  CooldownKey = "radar_sweep"
)

// SlotDefinition records the compatibility metadata for a concrete module slot.
type SlotDefinition struct {
	SlotID ModuleSlotID   `json:"slot_id"`
	Type   ModuleSlotType `json:"slot_type"`
}

// RoleRequirement records the rank-adjacent role level gate for a module.
type RoleRequirement struct {
	Role  PilotRole `json:"role"`
	Level int       `json:"level"`
}

// StatModifier records one catalog-defined stat contribution.
//
// Value uses whole stat units for flat modifiers and basis points for percent
// modifiers. Percent values must be greater than -10000 to avoid reducing a stat
// below zero before the later aggregation clamp runs.
type StatModifier struct {
	Stat  StatKey          `json:"stat"`
	Kind  StatModifierKind `json:"kind"`
	Value int64            `json:"value"`
}

// EnergyProfile records server-validated energy costs for active module use.
type EnergyProfile struct {
	ActivationCost int64 `json:"activation_cost,omitempty"`
	Upkeep         int64 `json:"upkeep,omitempty"`
}

// Cooldown records one server-timed cooldown duration in milliseconds.
type Cooldown struct {
	Key        CooldownKey `json:"key"`
	DurationMS int64       `json:"duration_ms"`
}

// DurabilityProfile records durability rules for a module definition.
type DurabilityProfile struct {
	Max int64 `json:"max"`
}

// ModuleDefinition records static catalog metadata for one equippable module.
type ModuleDefinition struct {
	Source               catalog.VersionedDefinition `json:"source"`
	ItemID               foundation.ItemID           `json:"item_id"`
	Name                 string                      `json:"name"`
	Category             ModuleCategory              `json:"module_category"`
	SlotType             ModuleSlotType              `json:"slot_type"`
	Tier                 int                         `json:"tier"`
	Rarity               economy.ItemRarity          `json:"rarity"`
	RequiredRank         int                         `json:"required_rank"`
	RequiredRoleLevels   []RoleRequirement           `json:"required_role_levels,omitempty"`
	StatModifiers        []StatModifier              `json:"stat_modifiers,omitempty"`
	Energy               EnergyProfile               `json:"energy,omitempty"`
	Cooldowns            []Cooldown                  `json:"cooldowns,omitempty"`
	Durability           DurabilityProfile           `json:"durability"`
	TradeFlags           []economy.TradeFlag         `json:"trade_flags,omitempty"`
	BindRules            []economy.BindRule          `json:"bind_rules,omitempty"`
	CompatibleSlotTypes  []ModuleSlotType            `json:"compatible_slot_types"`
	CompatibleCategories []ModuleCategory            `json:"compatible_categories"`
}

// EquippedModule records durable state for one item instance installed in a slot.
type EquippedModule struct {
	PlayerID       foundation.PlayerID `json:"player_id"`
	ShipID         foundation.ShipID   `json:"ship_id"`
	SlotID         ModuleSlotID        `json:"slot_id"`
	ItemInstanceID foundation.ItemID   `json:"item_instance_id"`
	EquippedAt     time.Time           `json:"equipped_at"`
}

// SupportedSlotTypes returns all MVP-supported module slot types.
func SupportedSlotTypes() []ModuleSlotType {
	return []ModuleSlotType{
		ModuleSlotTypeOffensive,
		ModuleSlotTypeDefensive,
		ModuleSlotTypeUtility,
	}
}

// DefaultSlotDefinitions returns the slot ids named by the module spec.
func DefaultSlotDefinitions() []SlotDefinition {
	return []SlotDefinition{
		{SlotID: ModuleSlotOffensive1, Type: ModuleSlotTypeOffensive},
		{SlotID: ModuleSlotOffensive2, Type: ModuleSlotTypeOffensive},
		{SlotID: ModuleSlotOffensive3, Type: ModuleSlotTypeOffensive},
		{SlotID: ModuleSlotOffensive4, Type: ModuleSlotTypeOffensive},
		{SlotID: ModuleSlotDefensive1, Type: ModuleSlotTypeDefensive},
		{SlotID: ModuleSlotDefensive2, Type: ModuleSlotTypeDefensive},
		{SlotID: ModuleSlotDefensive3, Type: ModuleSlotTypeDefensive},
		{SlotID: ModuleSlotUtility1, Type: ModuleSlotTypeUtility},
		{SlotID: ModuleSlotUtility2, Type: ModuleSlotTypeUtility},
		{SlotID: ModuleSlotUtility3, Type: ModuleSlotTypeUtility},
		{SlotID: ModuleSlotUtility4, Type: ModuleSlotTypeUtility},
	}
}

// SupportedCategories returns all MVP-supported module categories.
func SupportedCategories() []ModuleCategory {
	return []ModuleCategory{
		ModuleCategoryOffensive,
		ModuleCategoryDefensive,
		ModuleCategoryUtility,
	}
}

// SupportedPilotRoles returns the role gates understood by module definitions.
func SupportedPilotRoles() []PilotRole {
	return []PilotRole{
		PilotRoleCombat,
		PilotRoleScout,
		PilotRoleCrafting,
		PilotRoleConstruction,
	}
}

// String returns the stable wire representation.
func (slotType ModuleSlotType) String() string { return string(slotType) }

// String returns the stable wire representation.
func (slotID ModuleSlotID) String() string { return string(slotID) }

// String returns the stable wire representation.
func (category ModuleCategory) String() string { return string(category) }

// String returns the stable wire representation.
func (role PilotRole) String() string { return string(role) }

// String returns the stable wire representation.
func (stat StatKey) String() string { return string(stat) }

// String returns the stable wire representation.
func (kind StatModifierKind) String() string { return string(kind) }

// String returns the stable wire representation.
func (key CooldownKey) String() string { return string(key) }

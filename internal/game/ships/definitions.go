package ships

import (
	"errors"
	"fmt"
	"strings"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

// ShipCatalogVersion identifies the first static ship catalog slice.
const ShipCatalogVersion catalog.Version = "ship_catalog_v1"

// MVP ship ids.
const (
	ShipIDStarter   foundation.ShipID = "starter"
	ShipIDFighterT1 foundation.ShipID = "fighter_t1"
	ShipIDScoutT1   foundation.ShipID = "scout_t1"
	ShipIDHaulerT1  foundation.ShipID = "hauler_t1"
)

var (
	ErrEmptyShipCatalog            = errors.New("empty ship catalog")
	ErrDuplicateShipDefinition     = errors.New("duplicate ship definition")
	ErrUnknownShipDefinition       = errors.New("unknown ship definition")
	ErrShipSourceMismatch          = errors.New("ship source definition mismatch")
	ErrEmptyShipName               = errors.New("empty ship name")
	ErrInvalidShipTier             = errors.New("invalid ship tier")
	ErrInvalidShipRole             = errors.New("invalid ship role")
	ErrInvalidSlotType             = errors.New("invalid ship slot type")
	ErrInvalidSlotCount            = errors.New("invalid ship slot count")
	ErrEmptySlotLayout             = errors.New("empty ship slot layout")
	ErrInvalidRankRequirement      = errors.New("invalid ship rank requirement")
	ErrInvalidShipBaseStat         = errors.New("invalid ship base stat")
	ErrInvalidCargoCapacity        = errors.New("invalid ship cargo capacity")
	ErrInvalidRepairCostMultiplier = errors.New("invalid ship repair cost multiplier")
	ErrNegativeShipPrice           = errors.New("negative ship price")
)

// ShipRole identifies the intended chassis role. Stat aggregation decides final
// effective power after modules and passives are applied.
type ShipRole string

const (
	ShipRoleFighter    ShipRole = "fighter"
	ShipRoleScout      ShipRole = "scout"
	ShipRoleHauler     ShipRole = "hauler"
	ShipRoleSupport    ShipRole = "support"
	ShipRoleIndustrial ShipRole = "industrial"
)

// SlotType identifies one loadout slot family.
type SlotType string

const (
	SlotTypeOffensive SlotType = "offensive"
	SlotTypeDefensive SlotType = "defensive"
	SlotTypeUtility   SlotType = "utility"
)

// SlotLayout records module slot counts for one ship definition.
type SlotLayout struct {
	Offensive int `json:"slot_offensive"`
	Defensive int `json:"slot_defensive"`
	Utility   int `json:"slot_utility"`
}

// ShipBaseStats records raw ship stats before modules, passives, buffs, or
// clamps. Later stat aggregation must treat these values as server truth.
type ShipBaseStats struct {
	HP            int64 `json:"base_hp"`
	Shield        int64 `json:"base_shield"`
	Energy        int64 `json:"base_energy"`
	EnergyRegen   int64 `json:"base_energy_regen"`
	Speed         int64 `json:"base_speed"`
	CargoCapacity int64 `json:"base_cargo"`
	Radar         int64 `json:"base_radar"`
	Signature     int64 `json:"base_signature"`
}

// ShipDefinition records one static ship catalog row.
type ShipDefinition struct {
	Source                  catalog.VersionedDefinition `json:"source"`
	ShipID                  foundation.ShipID           `json:"ship_id"`
	Name                    string                      `json:"name"`
	Tier                    int                         `json:"tier"`
	Role                    ShipRole                    `json:"role_tag"`
	RankRequirement         int                         `json:"rank_requirement"`
	CraftRecipeID           catalog.DefinitionID        `json:"craft_recipe_id,omitempty"`
	CreditPrice             int64                       `json:"credit_price"`
	PremiumPrice            int64                       `json:"premium_price"`
	AuctionBuyNowPrice      int64                       `json:"auction_buy_now_price"`
	BaseStats               ShipBaseStats               `json:"base_stats"`
	RepairCostMultiplierBps int64                       `json:"repair_cost_multiplier_bps"`
	Slots                   SlotLayout                  `json:"slot_layout"`
	PassiveBonusID          catalog.DefinitionID        `json:"passive_bonus_id,omitempty"`
}

// Catalog stores validated ship definitions by id while preserving row order.
type Catalog struct {
	definitions map[foundation.ShipID]ShipDefinition
	order       []foundation.ShipID
}

// NewShipDefinition validates and returns a ship definition with no optional
// recipe, price, or passive fields set.
func NewShipDefinition(
	source catalog.VersionedDefinition,
	shipID foundation.ShipID,
	name string,
	tier int,
	role ShipRole,
	rankRequirement int,
	baseStats ShipBaseStats,
	slots SlotLayout,
) (ShipDefinition, error) {
	definition := ShipDefinition{
		Source:                  source,
		ShipID:                  shipID,
		Name:                    name,
		Tier:                    tier,
		Role:                    role,
		RankRequirement:         rankRequirement,
		BaseStats:               baseStats,
		RepairCostMultiplierBps: 10_000,
		Slots:                   slots,
	}
	if err := definition.Validate(); err != nil {
		return ShipDefinition{}, err
	}
	return definition, nil
}

// NewSlotLayout validates and returns a ship slot layout.
func NewSlotLayout(offensive int, defensive int, utility int) (SlotLayout, error) {
	layout := SlotLayout{
		Offensive: offensive,
		Defensive: defensive,
		Utility:   utility,
	}
	if err := layout.Validate(); err != nil {
		return SlotLayout{}, err
	}
	return layout, nil
}

// NewCatalog validates and indexes ship definitions.
func NewCatalog(definitions []ShipDefinition) (Catalog, error) {
	if len(definitions) == 0 {
		return Catalog{}, ErrEmptyShipCatalog
	}

	catalogRows := Catalog{
		definitions: make(map[foundation.ShipID]ShipDefinition, len(definitions)),
		order:       make([]foundation.ShipID, 0, len(definitions)),
	}
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return Catalog{}, err
		}
		if _, exists := catalogRows.definitions[definition.ShipID]; exists {
			return Catalog{}, fmt.Errorf("ship %q: %w", definition.ShipID, ErrDuplicateShipDefinition)
		}
		catalogRows.definitions[definition.ShipID] = definition
		catalogRows.order = append(catalogRows.order, definition.ShipID)
	}
	return catalogRows, nil
}

// SupportedShipRoles returns the roadmap-supported ship roles.
func SupportedShipRoles() []ShipRole {
	return []ShipRole{
		ShipRoleFighter,
		ShipRoleScout,
		ShipRoleHauler,
		ShipRoleSupport,
		ShipRoleIndustrial,
	}
}

// SupportedSlotTypes returns the module slot families known by ships.
func SupportedSlotTypes() []SlotType {
	return []SlotType{
		SlotTypeOffensive,
		SlotTypeDefensive,
		SlotTypeUtility,
	}
}

// String returns the stable role representation.
func (role ShipRole) String() string {
	return string(role)
}

// Validate reports whether role is supported.
func (role ShipRole) Validate() error {
	switch role {
	case ShipRoleFighter,
		ShipRoleScout,
		ShipRoleHauler,
		ShipRoleSupport,
		ShipRoleIndustrial:
		return nil
	default:
		return fmt.Errorf("ship role %q: %w", role, ErrInvalidShipRole)
	}
}

// String returns the stable slot type representation.
func (slotType SlotType) String() string {
	return string(slotType)
}

// Validate reports whether slotType is supported.
func (slotType SlotType) Validate() error {
	switch slotType {
	case SlotTypeOffensive, SlotTypeDefensive, SlotTypeUtility:
		return nil
	default:
		return fmt.Errorf("ship slot type %q: %w", slotType, ErrInvalidSlotType)
	}
}

// Validate reports whether layout contains non-negative counts and at least one
// total slot.
func (layout SlotLayout) Validate() error {
	if layout.Offensive < 0 {
		return fmt.Errorf("offensive slots %d: %w", layout.Offensive, ErrInvalidSlotCount)
	}
	if layout.Defensive < 0 {
		return fmt.Errorf("defensive slots %d: %w", layout.Defensive, ErrInvalidSlotCount)
	}
	if layout.Utility < 0 {
		return fmt.Errorf("utility slots %d: %w", layout.Utility, ErrInvalidSlotCount)
	}
	if layout.Total() == 0 {
		return ErrEmptySlotLayout
	}
	return nil
}

// Count returns the number of slots for a supported slot type.
func (layout SlotLayout) Count(slotType SlotType) (int, error) {
	switch slotType {
	case SlotTypeOffensive:
		return layout.Offensive, nil
	case SlotTypeDefensive:
		return layout.Defensive, nil
	case SlotTypeUtility:
		return layout.Utility, nil
	default:
		return 0, fmt.Errorf("ship slot type %q: %w", slotType, ErrInvalidSlotType)
	}
}

// Total returns all slots on this layout.
func (layout SlotLayout) Total() int {
	return layout.Offensive + layout.Defensive + layout.Utility
}

// Validate reports whether all raw base stats are positive.
func (stats ShipBaseStats) Validate() error {
	if err := validatePositiveBaseStat("base_hp", stats.HP); err != nil {
		return err
	}
	if err := validatePositiveBaseStat("base_shield", stats.Shield); err != nil {
		return err
	}
	if err := validatePositiveBaseStat("base_energy", stats.Energy); err != nil {
		return err
	}
	if err := validatePositiveBaseStat("base_energy_regen", stats.EnergyRegen); err != nil {
		return err
	}
	if err := validatePositiveBaseStat("base_speed", stats.Speed); err != nil {
		return err
	}
	if stats.CargoCapacity <= 0 {
		return fmt.Errorf("base_cargo %d: %w", stats.CargoCapacity, ErrInvalidCargoCapacity)
	}
	if stats.CargoCapacity > foundation.MaxAmount {
		return fmt.Errorf("base_cargo %d exceeds max %d: %w", stats.CargoCapacity, foundation.MaxAmount, ErrInvalidCargoCapacity)
	}
	if err := validatePositiveBaseStat("base_radar", stats.Radar); err != nil {
		return err
	}
	if err := validatePositiveBaseStat("base_signature", stats.Signature); err != nil {
		return err
	}
	return nil
}

// Validate reports whether definition is usable as a static catalog row.
func (definition ShipDefinition) Validate() error {
	if err := definition.Source.Validate(); err != nil {
		return err
	}
	if err := definition.ShipID.Validate(); err != nil {
		return err
	}
	if err := validateShipSource(definition.Source, definition.ShipID); err != nil {
		return err
	}
	if strings.TrimSpace(definition.Name) == "" {
		return ErrEmptyShipName
	}
	if definition.Tier <= 0 {
		return fmt.Errorf("tier %d: %w", definition.Tier, ErrInvalidShipTier)
	}
	if err := definition.Role.Validate(); err != nil {
		return err
	}
	if definition.RankRequirement <= 0 {
		return fmt.Errorf("rank requirement %d: %w", definition.RankRequirement, ErrInvalidRankRequirement)
	}
	if definition.CreditPrice < 0 {
		return fmt.Errorf("credit price %d: %w", definition.CreditPrice, ErrNegativeShipPrice)
	}
	if definition.PremiumPrice < 0 {
		return fmt.Errorf("premium price %d: %w", definition.PremiumPrice, ErrNegativeShipPrice)
	}
	if definition.AuctionBuyNowPrice < 0 {
		return fmt.Errorf("auction buy-now price %d: %w", definition.AuctionBuyNowPrice, ErrNegativeShipPrice)
	}
	if err := definition.BaseStats.Validate(); err != nil {
		return err
	}
	if definition.RepairCostMultiplierBps <= 0 {
		return fmt.Errorf("repair cost multiplier bps %d: %w", definition.RepairCostMultiplierBps, ErrInvalidRepairCostMultiplier)
	}
	if err := definition.Slots.Validate(); err != nil {
		return err
	}
	if err := validateOptionalDefinitionID("craft recipe", definition.CraftRecipeID); err != nil {
		return err
	}
	if err := validateOptionalDefinitionID("passive bonus", definition.PassiveBonusID); err != nil {
		return err
	}
	return nil
}

// Get returns one ship definition by id.
func (catalogRows Catalog) Get(shipID foundation.ShipID) (ShipDefinition, bool) {
	definition, ok := catalogRows.definitions[shipID]
	return definition, ok
}

// MustGet returns one ship definition by id or an unknown-definition error.
func (catalogRows Catalog) MustGet(shipID foundation.ShipID) (ShipDefinition, error) {
	definition, ok := catalogRows.Get(shipID)
	if !ok {
		return ShipDefinition{}, fmt.Errorf("ship %q: %w", shipID, ErrUnknownShipDefinition)
	}
	return definition, nil
}

// All returns a copy of definitions in catalog row order.
func (catalogRows Catalog) All() []ShipDefinition {
	definitions := make([]ShipDefinition, 0, len(catalogRows.order))
	for _, shipID := range catalogRows.order {
		definitions = append(definitions, catalogRows.definitions[shipID])
	}
	return definitions
}

func validateShipSource(source catalog.VersionedDefinition, shipID foundation.ShipID) error {
	if source.DefinitionID.String() != shipID.String() {
		return fmt.Errorf("source %q ship %q: %w", source.DefinitionID, shipID, ErrShipSourceMismatch)
	}
	return nil
}

func validateOptionalDefinitionID(kind string, id catalog.DefinitionID) error {
	if id.IsZero() {
		return nil
	}
	if err := id.Validate(); err != nil {
		return fmt.Errorf("%s id: %w", kind, err)
	}
	return nil
}

func validatePositiveBaseStat(name string, value int64) error {
	if value <= 0 {
		return fmt.Errorf("%s %d: %w", name, value, ErrInvalidShipBaseStat)
	}
	if value > foundation.MaxAmount {
		return fmt.Errorf("%s %d exceeds max %d: %w", name, value, foundation.MaxAmount, ErrInvalidShipBaseStat)
	}
	return nil
}

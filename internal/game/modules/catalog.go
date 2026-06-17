package modules

import (
	"errors"
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

var ErrDuplicateModuleDefinition = errors.New("duplicate module definition")

const ModuleCatalogVersion catalog.Version = "module_catalog_mvp_v1"

// Catalog indexes static module definitions by item id.
type Catalog struct {
	definitions []ModuleDefinition
	byItemID    map[foundation.ItemID]ModuleDefinition
}

// NewCatalog validates and indexes module definitions.
func NewCatalog(definitions []ModuleDefinition) (Catalog, error) {
	byItemID := make(map[foundation.ItemID]ModuleDefinition, len(definitions))
	cloned := make([]ModuleDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return Catalog{}, err
		}
		if _, ok := byItemID[definition.ItemID]; ok {
			return Catalog{}, fmt.Errorf("module item %q: %w", definition.ItemID, ErrDuplicateModuleDefinition)
		}
		clonedDefinition := cloneModuleDefinition(definition)
		byItemID[clonedDefinition.ItemID] = clonedDefinition
		cloned = append(cloned, clonedDefinition)
	}
	return Catalog{
		definitions: cloned,
		byItemID:    byItemID,
	}, nil
}

// MustMVPCatalog returns the validated MVP module catalog or panics if the
// checked-in catalog data is invalid.
func MustMVPCatalog() Catalog {
	catalog, err := NewCatalog(MVPModuleDefinitions())
	if err != nil {
		panic(err)
	}
	return catalog
}

// Definitions returns all definitions in deterministic catalog order.
func (catalog Catalog) Definitions() []ModuleDefinition {
	definitions := make([]ModuleDefinition, 0, len(catalog.definitions))
	for _, definition := range catalog.definitions {
		definitions = append(definitions, cloneModuleDefinition(definition))
	}
	return definitions
}

// Lookup returns the module definition for itemID.
func (catalog Catalog) Lookup(itemID foundation.ItemID) (ModuleDefinition, bool) {
	definition, ok := catalog.byItemID[itemID]
	if !ok {
		return ModuleDefinition{}, false
	}
	return cloneModuleDefinition(definition), true
}

// MVPModuleDefinitions returns the five module rows needed by Phase 03 MVP.
func MVPModuleDefinitions() []ModuleDefinition {
	return []ModuleDefinition{
		newMVPModuleDefinition(
			"laser_alpha_t1",
			"Laser Gun Alpha I",
			ModuleCategoryOffensive,
			ModuleSlotTypeOffensive,
			economy.ItemRarityCommon,
			[]RoleRequirement{{Role: PilotRoleCombat, Level: 1}},
			[]StatModifier{
				{Stat: StatWeaponDamage, Kind: StatModifierFlat, Value: 12},
				{Stat: StatWeaponRange, Kind: StatModifierFlat, Value: 650},
				{Stat: StatAccuracy, Kind: StatModifierFlat, Value: 8_200},
			},
			EnergyProfile{ActivationCost: 8},
			[]Cooldown{{Key: CooldownBasicAttack, DurationMS: 1_200}},
		),
		newMVPModuleDefinition(
			"shield_generator_t1",
			"Shield Generator I",
			ModuleCategoryDefensive,
			ModuleSlotTypeDefensive,
			economy.ItemRarityCommon,
			[]RoleRequirement{{Role: PilotRoleCombat, Level: 1}},
			[]StatModifier{
				{Stat: StatShieldMax, Kind: StatModifierFlat, Value: 80},
				{Stat: StatShieldRegen, Kind: StatModifierFlat, Value: 3},
			},
			EnergyProfile{Upkeep: 2},
			nil,
		),
		newMVPModuleDefinition(
			"scanner_t1",
			"Scanner I",
			ModuleCategoryUtility,
			ModuleSlotTypeUtility,
			economy.ItemRarityCommon,
			[]RoleRequirement{{Role: PilotRoleScout, Level: 1}},
			[]StatModifier{
				{Stat: StatScanPower, Kind: StatModifierFlat, Value: 10},
				{Stat: StatScanRadius, Kind: StatModifierFlat, Value: 450},
			},
			EnergyProfile{ActivationCost: 6},
			[]Cooldown{{Key: CooldownScanPulse, DurationMS: 3_000}},
		),
		newMVPModuleDefinition(
			"radar_t1",
			"Radar I",
			ModuleCategoryUtility,
			ModuleSlotTypeUtility,
			economy.ItemRarityCommon,
			[]RoleRequirement{{Role: PilotRoleScout, Level: 1}},
			[]StatModifier{
				{Stat: StatRadarRange, Kind: StatModifierFlat, Value: 1_200},
			},
			EnergyProfile{Upkeep: 1},
			[]Cooldown{{Key: CooldownRadarSweep, DurationMS: 1_000}},
		),
		newMVPModuleDefinition(
			"cargo_expander_t1",
			"Cargo Expander I",
			ModuleCategoryUtility,
			ModuleSlotTypeUtility,
			economy.ItemRarityCommon,
			[]RoleRequirement{{Role: PilotRoleConstruction, Level: 1}},
			[]StatModifier{
				{Stat: StatCargoCapacity, Kind: StatModifierFlat, Value: 40},
			},
			EnergyProfile{},
			nil,
		),
	}
}

func newMVPModuleDefinition(
	itemID foundation.ItemID,
	name string,
	category ModuleCategory,
	slotType ModuleSlotType,
	rarity economy.ItemRarity,
	roleRequirements []RoleRequirement,
	statModifiers []StatModifier,
	energy EnergyProfile,
	cooldowns []Cooldown,
) ModuleDefinition {
	return ModuleDefinition{
		Source: catalog.VersionedDefinition{
			DefinitionID: catalog.DefinitionID(itemID),
			Version:      ModuleCatalogVersion,
		},
		ItemID:               itemID,
		Name:                 name,
		Category:             category,
		SlotType:             slotType,
		Tier:                 1,
		Rarity:               rarity,
		RequiredRank:         1,
		RequiredRoleLevels:   append([]RoleRequirement(nil), roleRequirements...),
		StatModifiers:        append([]StatModifier(nil), statModifiers...),
		Energy:               energy,
		Cooldowns:            append([]Cooldown(nil), cooldowns...),
		Durability:           DurabilityProfile{Max: 100},
		TradeFlags:           []economy.TradeFlag{economy.TradeFlagTradeable, economy.TradeFlagMarketTradeable, economy.TradeFlagDestroyable},
		BindRules:            []economy.BindRule{economy.BindRuleOnEquip},
		CompatibleSlotTypes:  []ModuleSlotType{slotType},
		CompatibleCategories: []ModuleCategory{category},
	}
}

func cloneModuleDefinition(definition ModuleDefinition) ModuleDefinition {
	definition.RequiredRoleLevels = append([]RoleRequirement(nil), definition.RequiredRoleLevels...)
	definition.StatModifiers = append([]StatModifier(nil), definition.StatModifiers...)
	definition.Cooldowns = append([]Cooldown(nil), definition.Cooldowns...)
	definition.TradeFlags = append([]economy.TradeFlag(nil), definition.TradeFlags...)
	definition.BindRules = append([]economy.BindRule(nil), definition.BindRules...)
	definition.CompatibleSlotTypes = append([]ModuleSlotType(nil), definition.CompatibleSlotTypes...)
	definition.CompatibleCategories = append([]ModuleCategory(nil), definition.CompatibleCategories...)
	return definition
}

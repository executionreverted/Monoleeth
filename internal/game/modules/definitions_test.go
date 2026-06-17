package modules

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestMVPCatalogContainsExpectedModuleRows(t *testing.T) {
	moduleCatalog := MustMVPCatalog()
	definitions := moduleCatalog.Definitions()

	if got, want := len(definitions), 5; got != want {
		t.Fatalf("MVP definitions count = %d, want %d", got, want)
	}

	want := []struct {
		itemID   foundation.ItemID
		category ModuleCategory
		slotType ModuleSlotType
		stat     StatKey
	}{
		{itemID: "laser_alpha_t1", category: ModuleCategoryOffensive, slotType: ModuleSlotTypeOffensive, stat: StatWeaponDamage},
		{itemID: "shield_generator_t1", category: ModuleCategoryDefensive, slotType: ModuleSlotTypeDefensive, stat: StatShieldMax},
		{itemID: "scanner_t1", category: ModuleCategoryUtility, slotType: ModuleSlotTypeUtility, stat: StatScanPower},
		{itemID: "radar_t1", category: ModuleCategoryUtility, slotType: ModuleSlotTypeUtility, stat: StatRadarRange},
		{itemID: "cargo_expander_t1", category: ModuleCategoryUtility, slotType: ModuleSlotTypeUtility, stat: StatCargoCapacity},
	}

	for _, expected := range want {
		t.Run(expected.itemID.String(), func(t *testing.T) {
			definition, ok := moduleCatalog.Lookup(expected.itemID)
			if !ok {
				t.Fatalf("Lookup(%q) = false, want true", expected.itemID)
			}
			if definition.Category != expected.category {
				t.Fatalf("Category = %q, want %q", definition.Category, expected.category)
			}
			if definition.SlotType != expected.slotType {
				t.Fatalf("SlotType = %q, want %q", definition.SlotType, expected.slotType)
			}
			if definition.RequiredRank != 1 {
				t.Fatalf("RequiredRank = %d, want 1", definition.RequiredRank)
			}
			if len(definition.RequiredRoleLevels) != 1 {
				t.Fatalf("RequiredRoleLevels len = %d, want 1", len(definition.RequiredRoleLevels))
			}
			if definition.Durability.Max != 100 {
				t.Fatalf("Durability.Max = %d, want 100", definition.Durability.Max)
			}
			if !hasStat(definition.StatModifiers, expected.stat) {
				t.Fatalf("StatModifiers missing %q", expected.stat)
			}
			if err := economy.ValidateMarketListingTradeFlags(definition.TradeFlags); err != nil {
				t.Fatalf("market trade flags error = %v, want nil", err)
			}
		})
	}
}

func TestModuleDefinitionValidatesCompatibilityRequirementsDurabilityAndSource(t *testing.T) {
	valid := validModuleDefinition()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid definition Validate() = %v, want nil", err)
	}

	cases := []struct {
		name    string
		mutate  func(*ModuleDefinition)
		wantErr error
	}{
		{
			name: "blank item id",
			mutate: func(definition *ModuleDefinition) {
				definition.ItemID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "source mismatch",
			mutate: func(definition *ModuleDefinition) {
				definition.Source.DefinitionID = "other_module"
			},
			wantErr: ErrModuleSourceMismatch,
		},
		{
			name: "wrong category slot",
			mutate: func(definition *ModuleDefinition) {
				definition.SlotType = ModuleSlotTypeUtility
				definition.CompatibleSlotTypes = []ModuleSlotType{ModuleSlotTypeUtility}
			},
			wantErr: ErrSlotCategoryMismatch,
		},
		{
			name: "rank below one",
			mutate: func(definition *ModuleDefinition) {
				definition.RequiredRank = 0
			},
			wantErr: ErrInvalidRequiredRank,
		},
		{
			name: "invalid role requirement",
			mutate: func(definition *ModuleDefinition) {
				definition.RequiredRoleLevels = []RoleRequirement{{Role: PilotRoleCombat, Level: 0}}
			},
			wantErr: ErrInvalidRequiredRoleLevel,
		},
		{
			name: "duplicate role requirement",
			mutate: func(definition *ModuleDefinition) {
				definition.RequiredRoleLevels = []RoleRequirement{
					{Role: PilotRoleCombat, Level: 1},
					{Role: PilotRoleCombat, Level: 2},
				}
			},
			wantErr: ErrDuplicateRoleRequirement,
		},
		{
			name: "zero durability max",
			mutate: func(definition *ModuleDefinition) {
				definition.Durability.Max = 0
			},
			wantErr: ErrInvalidDurabilityMax,
		},
		{
			name: "primary slot missing from compatibility",
			mutate: func(definition *ModuleDefinition) {
				definition.CompatibleSlotTypes = []ModuleSlotType{ModuleSlotTypeUtility}
			},
			wantErr: ErrSlotCategoryMismatch,
		},
		{
			name: "primary category missing from compatibility",
			mutate: func(definition *ModuleDefinition) {
				definition.CompatibleCategories = []ModuleCategory{ModuleCategoryUtility}
			},
			wantErr: ErrSlotCategoryMismatch,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			definition := validModuleDefinition()
			tc.mutate(&definition)
			if err := definition.Validate(); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestModuleDefinitionValidatesStatEnergyCooldownAndTradeMetadata(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*ModuleDefinition)
		wantErr error
	}{
		{
			name: "invalid stat",
			mutate: func(definition *ModuleDefinition) {
				definition.StatModifiers = []StatModifier{{Stat: "speed_hack", Kind: StatModifierFlat, Value: 1}}
			},
			wantErr: ErrInvalidStatKey,
		},
		{
			name: "zero stat value",
			mutate: func(definition *ModuleDefinition) {
				definition.StatModifiers = []StatModifier{{Stat: StatWeaponDamage, Kind: StatModifierFlat, Value: 0}}
			},
			wantErr: ErrInvalidStatModifierValue,
		},
		{
			name: "invalid percent value",
			mutate: func(definition *ModuleDefinition) {
				definition.StatModifiers = []StatModifier{{Stat: StatWeaponDamage, Kind: StatModifierPercent, Value: -10_000}}
			},
			wantErr: ErrInvalidStatModifierValue,
		},
		{
			name: "duplicate stat modifier",
			mutate: func(definition *ModuleDefinition) {
				definition.StatModifiers = []StatModifier{
					{Stat: StatWeaponDamage, Kind: StatModifierFlat, Value: 1},
					{Stat: StatWeaponDamage, Kind: StatModifierFlat, Value: 2},
				}
			},
			wantErr: ErrDuplicateStatModifier,
		},
		{
			name: "negative energy",
			mutate: func(definition *ModuleDefinition) {
				definition.Energy.ActivationCost = -1
			},
			wantErr: ErrInvalidEnergyValue,
		},
		{
			name: "invalid cooldown key",
			mutate: func(definition *ModuleDefinition) {
				definition.Cooldowns = []Cooldown{{Key: "bad_key", DurationMS: 1}}
			},
			wantErr: ErrInvalidCooldownKey,
		},
		{
			name: "duplicate cooldown",
			mutate: func(definition *ModuleDefinition) {
				definition.Cooldowns = []Cooldown{
					{Key: CooldownBasicAttack, DurationMS: 1},
					{Key: CooldownBasicAttack, DurationMS: 2},
				}
			},
			wantErr: ErrDuplicateCooldown,
		},
		{
			name: "invalid trade flag",
			mutate: func(definition *ModuleDefinition) {
				definition.TradeFlags = []economy.TradeFlag{"bad_flag"}
			},
			wantErr: economy.ErrInvalidTradeFlag,
		},
		{
			name: "invalid bind rule",
			mutate: func(definition *ModuleDefinition) {
				definition.BindRules = []economy.BindRule{"bad_rule"}
			},
			wantErr: economy.ErrInvalidBindRule,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			definition := validModuleDefinition()
			tc.mutate(&definition)
			if err := definition.Validate(); !errors.Is(err, tc.wantErr) {
				t.Fatalf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestCatalogRejectsDuplicateItemIDsAndClonesDefinitions(t *testing.T) {
	first := validModuleDefinition()
	second := validModuleDefinition()

	if _, err := NewCatalog([]ModuleDefinition{first, second}); !errors.Is(err, ErrDuplicateModuleDefinition) {
		t.Fatalf("duplicate NewCatalog error = %v, want ErrDuplicateModuleDefinition", err)
	}

	moduleCatalog := MustMVPCatalog()
	definitions := moduleCatalog.Definitions()
	definitions[0].Name = "mutated"

	definition, ok := moduleCatalog.Lookup("laser_alpha_t1")
	if !ok {
		t.Fatal("Lookup(laser_alpha_t1) = false, want true")
	}
	if definition.Name == "mutated" {
		t.Fatal("catalog definition mutated through returned slice")
	}
}

func TestEquippedModuleStateValidation(t *testing.T) {
	equipped := EquippedModule{
		PlayerID:       "player-1",
		ShipID:         "ship-1",
		SlotID:         ModuleSlotOffensive1,
		ItemInstanceID: "laser-instance-1",
		EquippedAt:     time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC),
	}
	if err := equipped.Validate(); err != nil {
		t.Fatalf("valid equipped module Validate() = %v, want nil", err)
	}

	equipped.SlotID = "utility_9"
	if err := equipped.Validate(); !errors.Is(err, ErrInvalidModuleSlotID) {
		t.Fatalf("invalid slot id error = %v, want ErrInvalidModuleSlotID", err)
	}

	equipped.SlotID = ModuleSlotOffensive1
	equipped.EquippedAt = time.Time{}
	if err := equipped.Validate(); !errors.Is(err, ErrZeroEquippedAt) {
		t.Fatalf("zero equipped at error = %v, want ErrZeroEquippedAt", err)
	}
}

func TestSlotCategoryAndJSONBehaviorIsStable(t *testing.T) {
	slotType, err := ModuleSlotOffensive2.SlotType()
	if err != nil {
		t.Fatalf("SlotType() error = %v, want nil", err)
	}
	if slotType != ModuleSlotTypeOffensive {
		t.Fatalf("SlotType() = %q, want %q", slotType, ModuleSlotTypeOffensive)
	}
	if got := ModuleCategoryUtility.String(); got != "utility" {
		t.Fatalf("ModuleCategoryUtility.String() = %q, want utility", got)
	}

	definition := validModuleDefinition()
	payload, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("json marshal definition: %v", err)
	}
	want := `{"source":{"definition_id":"laser_alpha_t1","catalog_version":"module_catalog_mvp_v1"},"item_id":"laser_alpha_t1","name":"Laser Gun Alpha I","module_category":"offensive","slot_type":"offensive","tier":1,"rarity":"common","required_rank":1,"required_role_levels":[{"role":"combat","level":1}],"stat_modifiers":[{"stat":"weapon_damage","kind":"flat","value":12}],"energy":{"activation_cost":8},"cooldowns":[{"key":"basic_attack","duration_ms":1200}],"durability":{"max":100},"trade_flags":["tradeable","market_tradeable"],"bind_rules":["bind_on_equip"],"compatible_slot_types":["offensive"],"compatible_categories":["offensive"]}`
	if got := string(payload); got != want {
		t.Fatalf("definition JSON = %s, want %s", got, want)
	}
}

func validModuleDefinition() ModuleDefinition {
	return ModuleDefinition{
		Source: catalog.VersionedDefinition{
			DefinitionID: "laser_alpha_t1",
			Version:      ModuleCatalogVersion,
		},
		ItemID:             "laser_alpha_t1",
		Name:               "Laser Gun Alpha I",
		Category:           ModuleCategoryOffensive,
		SlotType:           ModuleSlotTypeOffensive,
		Tier:               1,
		Rarity:             economy.ItemRarityCommon,
		RequiredRank:       1,
		RequiredRoleLevels: []RoleRequirement{{Role: PilotRoleCombat, Level: 1}},
		StatModifiers: []StatModifier{
			{Stat: StatWeaponDamage, Kind: StatModifierFlat, Value: 12},
		},
		Energy:               EnergyProfile{ActivationCost: 8},
		Cooldowns:            []Cooldown{{Key: CooldownBasicAttack, DurationMS: 1_200}},
		Durability:           DurabilityProfile{Max: 100},
		TradeFlags:           []economy.TradeFlag{economy.TradeFlagTradeable, economy.TradeFlagMarketTradeable},
		BindRules:            []economy.BindRule{economy.BindRuleOnEquip},
		CompatibleSlotTypes:  []ModuleSlotType{ModuleSlotTypeOffensive},
		CompatibleCategories: []ModuleCategory{ModuleCategoryOffensive},
	}
}

func hasStat(modifiers []StatModifier, stat StatKey) bool {
	for _, modifier := range modifiers {
		if modifier.Stat == stat {
			return true
		}
	}
	return false
}

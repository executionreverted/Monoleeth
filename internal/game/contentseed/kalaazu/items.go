package kalaazu

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/ships"
)

const (
	kalaazuItemCatalogVersion catalog.Version = "kalaazu_item_seed_v1"
	defaultStackMax                           = 1_000_000
	defaultItemWeight                         = 1
)

func BuildStarterItemRows(filesystem fs.FS) ([]content.SnapshotRow, error) {
	itemRows, err := LoadDumpRows(filesystem, "testdata/items.sql")
	if err != nil {
		return nil, err
	}
	return mapStarterItemRows(itemRows)
}

func mapStarterItemRows(itemRows []DumpRow) ([]content.SnapshotRow, error) {
	sources, err := mappedItemSources(itemRows)
	if err != nil {
		return nil, err
	}
	rows := make([]content.SnapshotRow, 0, len(sources)+23)
	for _, source := range sources {
		definition, err := itemDefinitionWithID(source.Source, source.ItemID)
		if err != nil {
			return nil, err
		}
		row, err := snapshotRow(definition.ItemID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	compatibilityRows, err := starterCompatibilityItemRows(sources)
	if err != nil {
		return nil, err
	}
	rows = append(rows, compatibilityRows...)
	defaultRows, err := defaultProjectionItemRows(sources)
	if err != nil {
		return nil, err
	}
	rows = append(rows, defaultRows...)
	return rows, nil
}

type mappedKalaazuItemSource struct {
	Source kalaazuItemSource
	ItemID foundation.ItemID
}

func mappedItemSources(itemRows []DumpRow) ([]mappedKalaazuItemSource, error) {
	sources := make([]kalaazuItemSource, 0, len(itemRows))
	for _, row := range itemRows {
		source, err := decodeKalaazuItem(row)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].KalaazuID < sources[j].KalaazuID })

	seen := make(map[foundation.ItemID]struct{}, len(sources))
	mapped := make([]mappedKalaazuItemSource, 0, len(sources))
	for _, source := range sources {
		itemID := uniqueItemID(source, seen)
		seen[itemID] = struct{}{}
		mapped = append(mapped, mappedKalaazuItemSource{Source: source, ItemID: itemID})
	}
	return mapped, nil
}

func itemDefinition(source kalaazuItemSource) (economy.ItemDefinition, error) {
	return itemDefinitionWithID(source, foundation.ItemID(source.LootID))
}

func itemDefinitionWithID(source kalaazuItemSource, itemID foundation.ItemID) (economy.ItemDefinition, error) {
	sourceRow, err := catalog.NewVersionedDefinitionFromStrings(itemID.String(), kalaazuItemCatalogVersion.String())
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	maxStack := int64(defaultStackMax)
	itemType := economy.ItemTypeStackable
	if isKalaazuInstanceItem(source) {
		itemType = economy.ItemTypeInstance
		maxStack = 1
	}
	maxStackQuantity, err := foundation.NewQuantity(maxStack)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weightQuantity, err := foundation.NewQuantity(defaultItemWeight)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	definition, err := economy.NewItemDefinition(
		sourceRow,
		itemID,
		source.Name,
		itemType,
		itemRarity(source),
		maxStackQuantity,
		weightQuantity,
		itemTradeFlags(source),
		nil,
		nil,
	)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return definition, nil
}

func moduleItemDefinitionWithID(source kalaazuItemSource, itemID foundation.ItemID) (economy.ItemDefinition, error) {
	sourceRow, err := catalog.NewVersionedDefinitionFromStrings(itemID.String(), kalaazuItemCatalogVersion.String())
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	maxStackQuantity, err := foundation.NewQuantity(1)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weightQuantity, err := foundation.NewQuantity(6)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	definition, err := economy.NewItemDefinition(
		sourceRow,
		itemID,
		source.Name,
		economy.ItemTypeInstance,
		itemRarity(source),
		maxStackQuantity,
		weightQuantity,
		[]economy.TradeFlag{economy.TradeFlagTradeable, economy.TradeFlagDestroyable},
		[]economy.BindRule{economy.BindRuleOnEquip},
		nil,
	)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return definition, nil
}

func materialItemDefinitionWithID(source kalaazuItemSource, itemID foundation.ItemID) (economy.ItemDefinition, error) {
	sourceRow, err := catalog.NewVersionedDefinitionFromStrings(itemID.String(), kalaazuItemCatalogVersion.String())
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	maxStackQuantity, err := foundation.NewQuantity(defaultStackMax)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weightQuantity, err := foundation.NewQuantity(defaultItemWeight)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	definition, err := economy.NewItemDefinition(
		sourceRow,
		itemID,
		source.Name,
		economy.ItemTypeStackable,
		itemRarity(source),
		maxStackQuantity,
		weightQuantity,
		[]economy.TradeFlag{economy.TradeFlagDroppable, economy.TradeFlagMarketTradeable},
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	return definition, nil
}

func defaultProjectionItemRows(sources []mappedKalaazuItemSource) ([]content.SnapshotRow, error) {
	byItemID := make(map[foundation.ItemID]kalaazuItemSource, len(sources))
	for _, source := range sources {
		byItemID[source.ItemID] = source.Source
	}
	projections := []struct {
		sourceID foundation.ItemID
		targetID foundation.ItemID
		name     string
		itemType economy.ItemType
		rarity   economy.ItemRarity
		maxStack int64
		flags    []economy.TradeFlag
		bind     []economy.BindRule
	}{
		{
			sourceID: "equipment_weapon_laser_lf_1",
			targetID: "laser_lens",
			name:     "Laser Lens",
			itemType: economy.ItemTypeStackable,
			rarity:   economy.ItemRarityCommon,
			maxStack: 999,
			flags:    []economy.TradeFlag{economy.TradeFlagDroppable, economy.TradeFlagMarketTradeable},
			bind:     []economy.BindRule{economy.BindRuleNone},
		},
		{
			sourceID: "ammunition_laser_lcb_10",
			targetID: "energy_cell",
			name:     "Energy Cell",
			itemType: economy.ItemTypeStackable,
			rarity:   economy.ItemRarityCommon,
			maxStack: 999,
			flags:    []economy.TradeFlag{economy.TradeFlagDroppable, economy.TradeFlagMarketTradeable},
			bind:     []economy.BindRule{economy.BindRuleNone},
		},
		{
			sourceID: "equipment_petgear_g_rl1",
			targetID: "scanner_circuit",
			name:     "Scanner Circuit",
			itemType: economy.ItemTypeStackable,
			rarity:   economy.ItemRarityCommon,
			maxStack: 999,
			flags:    []economy.TradeFlag{economy.TradeFlagDroppable, economy.TradeFlagMarketTradeable},
			bind:     []economy.BindRule{economy.BindRuleNone},
		},
		{
			sourceID: "voucher_jump_vouhcer",
			targetID: "warp_coil",
			name:     "Warp Coil",
			itemType: economy.ItemTypeStackable,
			rarity:   economy.ItemRarityCommon,
			maxStack: 999,
			flags:    []economy.TradeFlag{economy.TradeFlagDroppable, economy.TradeFlagMarketTradeable},
			bind:     []economy.BindRule{economy.BindRuleNone},
		},
		{
			sourceID: "resource_ore_palladium",
			targetID: "helium_dust",
			name:     "Helium Dust",
			itemType: economy.ItemTypeStackable,
			rarity:   economy.ItemRarityUncommon,
			maxStack: 999,
			flags:    []economy.TradeFlag{economy.TradeFlagDroppable, economy.TradeFlagMarketTradeable},
			bind:     []economy.BindRule{economy.BindRuleNone},
		},
		{
			sourceID: "voucher_jump_vouhcer",
			targetID: "planet_coordinate_scroll",
			name:     "Planet Coordinate Scroll",
			itemType: economy.ItemTypeInstance,
			rarity:   economy.ItemRarityRare,
			maxStack: 1,
			flags:    []economy.TradeFlag{economy.TradeFlagTradeable, economy.TradeFlagMarketTradeable},
			bind:     []economy.BindRule{economy.BindRuleNone},
		},
		{
			sourceID: "deal_extra_energy",
			targetID: "x_core",
			name:     "X Core",
			itemType: economy.ItemTypeStackable,
			rarity:   economy.ItemRarityRare,
			maxStack: 99,
			flags:    []economy.TradeFlag{economy.TradeFlagTradeable},
			bind:     []economy.BindRule{economy.BindRuleNone},
		},
	}
	rows := make([]content.SnapshotRow, 0, len(projections))
	for _, projection := range projections {
		source, ok := byItemID[projection.sourceID]
		if !ok {
			return nil, fmt.Errorf("default item projection source %q missing", projection.sourceID)
		}
		definition, err := defaultProjectionItemDefinitionWithID(source, projection.targetID, projection.name, projection.itemType, projection.rarity, projection.maxStack, projection.flags, projection.bind)
		if err != nil {
			return nil, err
		}
		row, err := snapshotRow(definition.ItemID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func defaultProjectionItemDefinitionWithID(source kalaazuItemSource, itemID foundation.ItemID, name string, itemType economy.ItemType, rarity economy.ItemRarity, maxStack int64, flags []economy.TradeFlag, bind []economy.BindRule) (economy.ItemDefinition, error) {
	sourceRow, err := catalog.NewVersionedDefinitionFromStrings(itemID.String(), kalaazuItemCatalogVersion.String())
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	maxStackQuantity, err := foundation.NewQuantity(maxStack)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	weightUnits := int64(defaultItemWeight)
	if itemType == economy.ItemTypeInstance {
		weightUnits = 1
	}
	weightQuantity, err := foundation.NewQuantity(weightUnits)
	if err != nil {
		return economy.ItemDefinition{}, err
	}
	if name == "" {
		name = source.Name
	}
	return economy.NewItemDefinition(
		sourceRow,
		itemID,
		name,
		itemType,
		rarity,
		maxStackQuantity,
		weightQuantity,
		flags,
		bind,
		nil,
	)
}

func starterCompatibilityItemRows(sources []mappedKalaazuItemSource) ([]content.SnapshotRow, error) {
	byItemID := make(map[foundation.ItemID]kalaazuItemSource, len(sources))
	for _, source := range sources {
		byItemID[source.ItemID] = source.Source
	}
	compatibility := []struct {
		sourceID foundation.ItemID
		targetID foundation.ItemID
	}{
		{sourceID: "equipment_weapon_laser_lf_1", targetID: "laser_alpha_t1"},
		{sourceID: "equipment_generator_shield_sg3n_a01", targetID: "shield_generator_t1"},
		{sourceID: "equipment_petgear_g_rl1", targetID: "scanner_t1"},
		{sourceID: "equipment_aiprotocol_ai_r1", targetID: "radar_t1"},
		{sourceID: "equipment_extra_cpu_g3x_crgo_x", targetID: "cargo_expander_t1"},
		{sourceID: "resource_ore_prometium", targetID: "prometium"},
		{sourceID: "resource_ore_prometium", targetID: "raw_ore"},
		{sourceID: "resource_ore_endurium", targetID: "endurium"},
		{sourceID: "resource_ore_endurium", targetID: "iron_ore"},
		{sourceID: "resource_ore_terbium", targetID: "terbium"},
		{sourceID: "resource_ore_prometid", targetID: "prometid"},
		{sourceID: "resource_ore_prometid", targetID: "refined_alloy"},
		{sourceID: "resource_ore_duranium", targetID: "duranium"},
		{sourceID: "resource_ore_xenomit", targetID: "xenomit"},
		{sourceID: "resource_ore_xenomit", targetID: "carbon_shards"},
		{sourceID: "resource_ore_promerium", targetID: "promerium"},
	}
	rows := make([]content.SnapshotRow, 0, len(compatibility))
	for _, projection := range compatibility {
		source, ok := byItemID[projection.sourceID]
		if !ok {
			return nil, fmt.Errorf("starter compatibility item source %q missing", projection.sourceID)
		}
		definition, err := itemDefinitionWithID(source, projection.targetID)
		switch source.Type {
		case 14, 15, 16, 19, 21, 22:
			definition, err = moduleItemDefinitionWithID(source, projection.targetID)
		case 26:
			definition, err = materialItemDefinitionWithID(source, projection.targetID)
		}
		if err != nil {
			return nil, err
		}
		row, err := snapshotRow(definition.ItemID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func uniqueItemID(source kalaazuItemSource, seen map[foundation.ItemID]struct{}) foundation.ItemID {
	base := foundation.ItemID(source.LootID)
	if _, exists := seen[base]; !exists {
		return base
	}
	for suffix := 1; ; suffix++ {
		candidate := foundation.ItemID(fmt.Sprintf("%s_kalaazu_%d", base, source.KalaazuID))
		if suffix > 1 {
			candidate = foundation.ItemID(fmt.Sprintf("%s_kalaazu_%d_%d", base, source.KalaazuID, suffix))
		}
		if _, exists := seen[candidate]; !exists {
			return candidate
		}
	}
}

func isKalaazuInstanceItem(source kalaazuItemSource) bool {
	return source.Category == 1 || source.Type == 30
}

func itemRarity(source kalaazuItemSource) economy.ItemRarity {
	switch {
	case source.IsEvent:
		return economy.ItemRarityEvent
	case source.IsElite:
		return economy.ItemRarityRare
	case source.Price > 0:
		return economy.ItemRarityUncommon
	default:
		return economy.ItemRarityCommon
	}
}

func itemTradeFlags(source kalaazuItemSource) []economy.TradeFlag {
	if !source.IsBuyable {
		return nil
	}
	return []economy.TradeFlag{economy.TradeFlagTradeable}
}

func BuildStarterModuleRows(filesystem fs.FS) ([]content.SnapshotRow, error) {
	itemRows, err := LoadDumpRows(filesystem, "testdata/items.sql")
	if err != nil {
		return nil, err
	}
	return mapStarterModuleRows(itemRows)
}

func mapStarterModuleRows(itemRows []DumpRow) ([]content.SnapshotRow, error) {
	sources, err := mappedItemSources(itemRows)
	if err != nil {
		return nil, err
	}
	rows := make([]content.SnapshotRow, 0)
	definitions := make(map[foundation.ItemID]modules.ModuleDefinition)
	sourceByItemID := make(map[foundation.ItemID]kalaazuItemSource, len(sources))
	for _, source := range sources {
		sourceByItemID[source.ItemID] = source.Source
		definition, ok, err := moduleDefinition(source)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		definitions[definition.ItemID] = definition
		row, err := snapshotRow(definition.ItemID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	compatibilityRows, err := starterCompatibilityModuleRows(definitions, sourceByItemID)
	if err != nil {
		return nil, err
	}
	rows = append(rows, compatibilityRows...)
	return rows, nil
}

func moduleDefinition(source mappedKalaazuItemSource) (modules.ModuleDefinition, bool, error) {
	var category modules.ModuleCategory
	var slotType modules.ModuleSlotType
	var statModifiers []modules.StatModifier
	switch source.Source.Type {
	case 16:
		category = modules.ModuleCategoryOffensive
		slotType = modules.ModuleSlotTypeOffensive
		statModifiers = []modules.StatModifier{{Stat: modules.StatWeaponDamage, Kind: modules.StatModifierFlat, Value: int64(maxInt(1, source.Source.Bonus))}}
	case 14:
		category = modules.ModuleCategoryDefensive
		slotType = modules.ModuleSlotTypeDefensive
		statModifiers = []modules.StatModifier{{Stat: modules.StatShieldMax, Kind: modules.StatModifierFlat, Value: int64(shieldValue(source.Source))}}
	case 15:
		category = modules.ModuleCategoryDefensive
		slotType = modules.ModuleSlotTypeDefensive
		statModifiers = []modules.StatModifier{{Stat: modules.StatSpeed, Kind: modules.StatModifierFlat, Value: int64(maxInt(1, source.Source.Bonus))}}
	default:
		return modules.ModuleDefinition{}, false, nil
	}
	module := modules.ModuleDefinition{
		Source: catalog.VersionedDefinition{
			DefinitionID: catalog.DefinitionID(source.ItemID.String()),
			Version:      modules.ModuleCatalogVersion,
		},
		ItemID:               source.ItemID,
		Name:                 source.Source.Name,
		Category:             category,
		SlotType:             slotType,
		Tier:                 moduleTier(source.Source),
		Rarity:               itemRarity(source.Source),
		RequiredRank:         1,
		StatModifiers:        statModifiers,
		Durability:           modules.DurabilityProfile{Max: 100},
		TradeFlags:           []economy.TradeFlag{economy.TradeFlagTradeable, economy.TradeFlagDestroyable},
		BindRules:            []economy.BindRule{economy.BindRuleOnEquip},
		CompatibleSlotTypes:  []modules.ModuleSlotType{slotType},
		CompatibleCategories: []modules.ModuleCategory{category},
	}
	if err := module.Validate(); err != nil {
		return modules.ModuleDefinition{}, false, err
	}
	return module, true, nil
}

func starterCompatibilityModuleRows(definitions map[foundation.ItemID]modules.ModuleDefinition, sources map[foundation.ItemID]kalaazuItemSource) ([]content.SnapshotRow, error) {
	compatibility := []struct {
		sourceID foundation.ItemID
		targetID foundation.ItemID
		mutate   func(*modules.ModuleDefinition)
	}{
		{
			sourceID: "equipment_weapon_laser_lf_1",
			targetID: "laser_alpha_t1",
			mutate: func(definition *modules.ModuleDefinition) {
				definition.StatModifiers = append(definition.StatModifiers,
					modules.StatModifier{Stat: modules.StatWeaponRange, Kind: modules.StatModifierFlat, Value: 650},
					modules.StatModifier{Stat: modules.StatAccuracy, Kind: modules.StatModifierFlat, Value: 8_200},
				)
				definition.Energy = modules.EnergyProfile{ActivationCost: 8}
				definition.Cooldowns = []modules.Cooldown{{Key: modules.CooldownBasicAttack, DurationMS: 1_200}}
			},
		},
		{
			sourceID: "equipment_generator_shield_sg3n_a01",
			targetID: "shield_generator_t1",
			mutate: func(definition *modules.ModuleDefinition) {
				definition.StatModifiers = append(definition.StatModifiers,
					modules.StatModifier{Stat: modules.StatShieldRegen, Kind: modules.StatModifierFlat, Value: 4},
				)
				definition.Energy = modules.EnergyProfile{Upkeep: 2}
			},
		},
		{
			sourceID: "equipment_petgear_g_rl1",
			targetID: "scanner_t1",
			mutate: func(definition *modules.ModuleDefinition) {
				definition.StatModifiers = []modules.StatModifier{
					{Stat: modules.StatScanPower, Kind: modules.StatModifierFlat, Value: 10},
					{Stat: modules.StatScanRadius, Kind: modules.StatModifierFlat, Value: 2_000},
				}
				definition.Energy = modules.EnergyProfile{ActivationCost: 6}
				definition.Cooldowns = []modules.Cooldown{{Key: modules.CooldownScanPulse, DurationMS: 3_000}}
				definition.RequiredRoleLevels = []modules.RoleRequirement{{Role: modules.PilotRoleScout, Level: 1}}
			},
		},
		{
			sourceID: "equipment_aiprotocol_ai_r1",
			targetID: "radar_t1",
			mutate: func(definition *modules.ModuleDefinition) {
				definition.StatModifiers = []modules.StatModifier{
					{Stat: modules.StatRadarRange, Kind: modules.StatModifierPercent, Value: 200},
				}
				definition.Energy = modules.EnergyProfile{Upkeep: 1}
				definition.Cooldowns = []modules.Cooldown{{Key: modules.CooldownRadarSweep, DurationMS: 1_000}}
				definition.RequiredRoleLevels = []modules.RoleRequirement{{Role: modules.PilotRoleScout, Level: 1}}
			},
		},
		{
			sourceID: "equipment_extra_cpu_g3x_crgo_x",
			targetID: "cargo_expander_t1",
			mutate: func(definition *modules.ModuleDefinition) {
				definition.StatModifiers = []modules.StatModifier{
					{Stat: modules.StatCargoCapacity, Kind: modules.StatModifierPercent, Value: 10_000},
				}
				definition.RequiredRoleLevels = []modules.RoleRequirement{{Role: modules.PilotRoleConstruction, Level: 1}}
			},
		},
	}
	rows := make([]content.SnapshotRow, 0, len(compatibility))
	for _, projection := range compatibility {
		definition, ok := definitions[projection.sourceID]
		if !ok {
			source, sourceOK := sources[projection.sourceID]
			if !sourceOK {
				return nil, fmt.Errorf("starter compatibility module source %q missing", projection.sourceID)
			}
			var err error
			definition, err = utilityModuleDefinitionWithID(source, projection.targetID)
			if err != nil {
				return nil, err
			}
		} else {
			definition.Source = catalog.VersionedDefinition{
				DefinitionID: catalog.DefinitionID(projection.targetID.String()),
				Version:      modules.ModuleCatalogVersion,
			}
			definition.ItemID = projection.targetID
		}
		if definition.ItemID != projection.targetID {
			return nil, fmt.Errorf("starter compatibility module source %q produced item %q, want %q", projection.sourceID, definition.ItemID, projection.targetID)
		}
		projection.mutate(&definition)
		if err := definition.Validate(); err != nil {
			return nil, err
		}
		row, err := snapshotRow(definition.ItemID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func utilityModuleDefinitionWithID(source kalaazuItemSource, itemID foundation.ItemID) (modules.ModuleDefinition, error) {
	definition := modules.ModuleDefinition{
		Source: catalog.VersionedDefinition{
			DefinitionID: catalog.DefinitionID(itemID.String()),
			Version:      modules.ModuleCatalogVersion,
		},
		ItemID:               itemID,
		Name:                 source.Name,
		Category:             modules.ModuleCategoryUtility,
		SlotType:             modules.ModuleSlotTypeUtility,
		Tier:                 moduleTier(source),
		Rarity:               itemRarity(source),
		RequiredRank:         1,
		Durability:           modules.DurabilityProfile{Max: 100},
		TradeFlags:           []economy.TradeFlag{economy.TradeFlagTradeable, economy.TradeFlagMarketTradeable, economy.TradeFlagDestroyable},
		BindRules:            []economy.BindRule{economy.BindRuleOnEquip},
		CompatibleSlotTypes:  []modules.ModuleSlotType{modules.ModuleSlotTypeUtility},
		CompatibleCategories: []modules.ModuleCategory{modules.ModuleCategoryUtility},
	}
	if err := definition.Validate(); err != nil {
		return modules.ModuleDefinition{}, err
	}
	return definition, nil
}

func moduleTier(source kalaazuItemSource) int {
	switch {
	case source.Bonus >= 150:
		return 4
	case source.Bonus >= 100:
		return 3
	case source.Bonus >= 50:
		return 2
	default:
		return 1
	}
}

func shieldValue(source kalaazuItemSource) int {
	switch {
	case strings.Contains(strings.ToLower(source.Name), "b02"):
		return 10000
	case strings.Contains(strings.ToLower(source.Name), "a03"):
		return 5000
	case strings.Contains(strings.ToLower(source.Name), "b01"):
		return 4000
	case strings.Contains(strings.ToLower(source.Name), "a02"):
		return 2000
	default:
		return 1000
	}
}

func BuildStarterShopRows(filesystem fs.FS) ([]content.SnapshotRow, error) {
	itemRows, err := LoadDumpRows(filesystem, "testdata/items.sql")
	if err != nil {
		return nil, err
	}
	shipRows, err := BuildStarterShipRows(filesystem)
	if err != nil {
		return nil, err
	}
	moduleRows, err := BuildStarterModuleRows(filesystem)
	if err != nil {
		return nil, err
	}
	return mapStarterShopRows(itemRows, shipRows, moduleRows)
}

func mapStarterShopRows(itemRows []DumpRow, shipRows []content.SnapshotRow, moduleRows []content.SnapshotRow) ([]content.SnapshotRow, error) {
	sources, err := mappedItemSources(itemRows)
	if err != nil {
		return nil, err
	}
	shipIDs, err := snapshotRowIDs[ships.ShipDefinition](content.ContentTypeShip, shipRows)
	if err != nil {
		return nil, err
	}
	moduleIDs, err := snapshotRowIDs[modules.ModuleDefinition](content.ContentTypeModule, moduleRows)
	if err != nil {
		return nil, err
	}
	rows := make([]content.SnapshotRow, 0)
	for _, source := range sources {
		if !source.Source.IsBuyable {
			continue
		}
		product := shopProductForSource(source, shipIDs, moduleIDs)
		row, err := snapshotRow(string(product.ProductID), product)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func snapshotRowIDs[T interface {
	ships.ShipDefinition | modules.ModuleDefinition
}](contentType content.ContentType, rows []content.SnapshotRow) (map[foundation.ItemID]struct{}, error) {
	ids := make(map[foundation.ItemID]struct{}, len(rows))
	for _, row := range rows {
		var decoded T
		if err := json.Unmarshal(row.DataJSON, &decoded); err != nil {
			return nil, fmt.Errorf("%s row %q: %w", contentType, row.ContentID, err)
		}
		switch value := any(decoded).(type) {
		case ships.ShipDefinition:
			ids[foundation.ItemID(value.ShipID.String())] = struct{}{}
		case modules.ModuleDefinition:
			ids[value.ItemID] = struct{}{}
		}
	}
	return ids, nil
}

func shopProductForSource(source mappedKalaazuItemSource, shipIDs map[foundation.ItemID]struct{}, moduleIDs map[foundation.ItemID]struct{}) catalog.ShopProductDefinition {
	productType := catalog.ShopProductTypeItem
	grantKind := catalog.GrantTargetKindItem
	category := shopCategoryForItem(source.Source)
	if _, ok := shipIDs[source.ItemID]; ok {
		productType = catalog.ShopProductTypeShip
		grantKind = catalog.GrantTargetKindShip
		category = content.ShopCategoryShips
	} else if _, ok := moduleIDs[source.ItemID]; ok {
		productType = catalog.ShopProductTypeModule
		grantKind = catalog.GrantTargetKindModule
		category = shopCategoryForModule(source.Source)
	}
	return catalog.ShopProductDefinition{
		ProductID:   catalog.ShopProductID("product_" + source.ItemID.String()),
		ProductType: productType,
		Display: catalog.DisplayMetadata{
			DisplayName: source.Source.Name,
			Description: "Kalaazu-derived default shop row.",
			Category:    category,
			Subcategory: shopSubcategory(source.Source),
			ArtKey:      "item." + source.ItemID.String(),
			Rarity:      itemRarity(source.Source).String(),
			Tier:        moduleTier(source.Source),
			SortOrder:   source.Source.KalaazuID,
		},
		GrantTarget: catalog.GrantTarget{Kind: grantKind, RefID: source.ItemID.String(), Quantity: 1},
		Price: catalog.PricePolicy{
			Currency: shopCurrency(source.Source),
			Amount:   source.Source.Price,
			Fixed:    true,
		},
		Stock:        catalog.StockPolicy{Kind: catalog.StockPolicyUnlimited},
		Availability: catalog.AvailabilityRule{Available: true, RequiredRank: 1},
	}
}

func shopCategoryForModule(source kalaazuItemSource) string {
	switch source.Type {
	case 16:
		return content.ShopCategoryWeapons
	case 14, 15:
		return content.ShopCategoryShieldGenerators
	default:
		return content.ShopCategoryExtrasModules
	}
}

func shopCategoryForItem(source kalaazuItemSource) string {
	switch source.Category {
	case 1:
		return content.ShopCategoryShips
	case 2:
		return content.ShopCategoryAmmo
	case 4:
		return content.ShopCategoryExtrasModules
	default:
		return content.ShopCategoryResources
	}
}

func shopSubcategory(source kalaazuItemSource) string {
	switch source.Type {
	case 14:
		return "Shield"
	case 15:
		return "Speed"
	case 16:
		return "Laser"
	case 30:
		return "Ship"
	default:
		return "Item"
	}
}

func shopCurrency(source kalaazuItemSource) catalog.PriceCurrency {
	if source.IsElite {
		return catalog.PriceCurrencyPremium
	}
	return catalog.PriceCurrencyCredits
}

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
	rows := make([]content.SnapshotRow, 0, len(sources))
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
	for _, source := range sources {
		definition, ok, err := moduleDefinition(source)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		row, err := snapshotRow(definition.ItemID.String(), definition)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
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

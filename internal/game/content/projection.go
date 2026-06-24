package content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
)

// PlayerContentProjection is the explicit allowlist sent to normal players.
// It intentionally omits loot, spawn, enemy pool, audit, and procedural fields.
type PlayerContentProjection struct {
	Version      string                        `json:"version"`
	Categories   []PlayerContentCategory       `json:"categories,omitempty"`
	Items        []PlayerItemProjection        `json:"items,omitempty"`
	Modules      []PlayerModuleProjection      `json:"modules,omitempty"`
	ShopProducts []PlayerShopProductProjection `json:"shop_products,omitempty"`
}

type PlayerContentCategory struct {
	ID          string `json:"category_id"`
	DisplayName string `json:"display_name"`
	SortOrder   int    `json:"sort_order,omitempty"`
}

type PlayerDisplayMetadata struct {
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Subcategory string `json:"subcategory,omitempty"`
	ArtKey      string `json:"art_key,omitempty"`
	Rarity      string `json:"rarity,omitempty"`
	Tier        int    `json:"tier,omitempty"`
	SortOrder   int    `json:"sort_order,omitempty"`
}

type PlayerItemProjection struct {
	ItemID      string                `json:"item_id"`
	Display     PlayerDisplayMetadata `json:"display"`
	ItemType    string                `json:"item_type,omitempty"`
	Rarity      string                `json:"rarity,omitempty"`
	MaxStack    int64                 `json:"max_stack,omitempty"`
	WeightUnits int64                 `json:"weight_units,omitempty"`
	TradeFlags  []string              `json:"trade_flags,omitempty"`
	BindRules   []string              `json:"bind_rules,omitempty"`
}

type PlayerModuleProjection struct {
	ItemID               string                  `json:"item_id"`
	Display              PlayerDisplayMetadata   `json:"display"`
	Name                 string                  `json:"name,omitempty"`
	Category             string                  `json:"module_category,omitempty"`
	SlotType             string                  `json:"slot_type,omitempty"`
	Tier                 int                     `json:"tier,omitempty"`
	Rarity               string                  `json:"rarity,omitempty"`
	RequiredRank         int                     `json:"required_rank,omitempty"`
	RequiredRoleLevels   []PlayerRoleRequirement `json:"required_role_levels,omitempty"`
	StatModifiers        []PlayerStatModifier    `json:"stat_modifiers,omitempty"`
	Energy               PlayerEnergyProfile     `json:"energy,omitempty"`
	Cooldowns            []PlayerCooldown        `json:"cooldowns,omitempty"`
	DurabilityMax        int64                   `json:"durability_max,omitempty"`
	TradeFlags           []string                `json:"trade_flags,omitempty"`
	BindRules            []string                `json:"bind_rules,omitempty"`
	CompatibleSlotTypes  []string                `json:"compatible_slot_types,omitempty"`
	CompatibleCategories []string                `json:"compatible_categories,omitempty"`
}

type PlayerRoleRequirement struct {
	Role  string `json:"role"`
	Level int    `json:"level"`
}

type PlayerStatModifier struct {
	Stat  string `json:"stat"`
	Kind  string `json:"kind"`
	Value int64  `json:"value"`
}

type PlayerEnergyProfile struct {
	ActivationCost int64 `json:"activation_cost,omitempty"`
	Upkeep         int64 `json:"upkeep,omitempty"`
}

type PlayerCooldown struct {
	Key        string `json:"key"`
	DurationMS int64  `json:"duration_ms"`
}

type PlayerShopProductProjection struct {
	ProductID    string                 `json:"product_id"`
	ProductType  string                 `json:"product_type,omitempty"`
	Display      PlayerDisplayMetadata  `json:"display"`
	GrantTarget  PlayerGrantTarget      `json:"grant_target"`
	Price        PlayerPricePolicy      `json:"price_policy"`
	Stock        PlayerStockPolicy      `json:"stock_policy"`
	Availability PlayerAvailabilityRule `json:"availability"`
}

type PlayerGrantTarget struct {
	Kind     string `json:"kind,omitempty"`
	RefID    string `json:"ref_id,omitempty"`
	Quantity int64  `json:"quantity,omitempty"`
}

type PlayerPricePolicy struct {
	Currency string `json:"currency_type,omitempty"`
	Amount   int64  `json:"amount,omitempty"`
	Fixed    bool   `json:"fixed,omitempty"`
}

type PlayerStockPolicy struct {
	Kind      string `json:"kind,omitempty"`
	Remaining int64  `json:"remaining,omitempty"`
	Total     int64  `json:"total,omitempty"`
}

type PlayerAvailabilityRule struct {
	Available    bool   `json:"available"`
	LockedReason string `json:"locked_reason,omitempty"`
	RequiredRank int    `json:"required_rank,omitempty"`
}

// ProjectSnapshotForPlayers returns enabled, player-visible content from a
// published CMS snapshot using explicit DTO allowlists.
func ProjectSnapshotForPlayers(snapshot Snapshot) (PlayerContentProjection, error) {
	if err := snapshot.Validate(); err != nil {
		return PlayerContentProjection{}, err
	}
	projection := PlayerContentProjection{Version: snapshot.Version}
	for _, row := range snapshot.Items {
		if !row.Enabled {
			continue
		}
		item, err := projectSnapshotItem(row)
		if err != nil {
			return PlayerContentProjection{}, err
		}
		projection.Items = append(projection.Items, item)
	}
	for _, row := range snapshot.Modules {
		if !row.Enabled {
			continue
		}
		module, err := projectSnapshotModule(row)
		if err != nil {
			return PlayerContentProjection{}, err
		}
		projection.Modules = append(projection.Modules, module)
	}
	for _, row := range snapshot.ShopProducts {
		if !row.Enabled {
			continue
		}
		product, err := projectSnapshotShopProduct(row)
		if err != nil {
			return PlayerContentProjection{}, err
		}
		projection.ShopProducts = append(projection.ShopProducts, product)
	}
	projection.Categories = categoriesFromShopProducts(projection.ShopProducts)
	return projection, nil
}

// ProjectGameplayContentForPlayers returns the static runtime content visible
// to players without serializing server-only catalogs.
func ProjectGameplayContentForPlayers(bundle GameplayContent) (PlayerContentProjection, error) {
	if err := bundle.Validate(); err != nil {
		return PlayerContentProjection{}, err
	}
	projection := PlayerContentProjection{Version: bundle.Shop.Version.String()}
	for _, category := range bundle.Shop.SortedCategories() {
		projection.Categories = append(projection.Categories, projectCategory(category))
	}

	displayByGrant := shopDisplayByGrantTarget(bundle.Shop.SortedShopProducts())
	itemIDs := make([]foundation.ItemID, 0, len(bundle.Items))
	for itemID := range bundle.Items {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Slice(itemIDs, func(i, j int) bool { return itemIDs[i] < itemIDs[j] })
	for _, itemID := range itemIDs {
		projection.Items = append(projection.Items, projectItemDefinition(bundle.Items[itemID], displayByGrant))
	}
	for _, definition := range bundle.Modules.Definitions() {
		projection.Modules = append(projection.Modules, projectModuleDefinition(definition, displayByGrant))
	}
	for _, product := range bundle.Shop.SortedShopProducts() {
		projection.ShopProducts = append(projection.ShopProducts, projectShopProduct(product))
	}
	return projection, nil
}

type snapshotDisplayMetadata struct {
	DisplayName string `json:"display_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
	ArtKey      string `json:"art_key"`
	Rarity      string `json:"rarity"`
	Tier        int    `json:"tier"`
	SortOrder   int    `json:"sort_order"`
}

func (display snapshotDisplayMetadata) project(fallbackName string) PlayerDisplayMetadata {
	displayName := display.DisplayName
	if displayName == "" {
		displayName = display.Name
	}
	if displayName == "" {
		displayName = fallbackName
	}
	return PlayerDisplayMetadata{
		DisplayName: displayName,
		Description: display.Description,
		Category:    display.Category,
		Subcategory: display.Subcategory,
		ArtKey:      display.ArtKey,
		Rarity:      display.Rarity,
		Tier:        display.Tier,
		SortOrder:   display.SortOrder,
	}
}

func (display snapshotDisplayMetadata) isZero() bool {
	return display.DisplayName == "" &&
		display.Name == "" &&
		display.Description == "" &&
		display.Category == "" &&
		display.Subcategory == "" &&
		display.ArtKey == "" &&
		display.Rarity == "" &&
		display.Tier == 0 &&
		display.SortOrder == 0
}

type snapshotItemData struct {
	Name        string   `json:"name"`
	ItemType    string   `json:"item_type"`
	Rarity      string   `json:"rarity"`
	MaxStack    int64    `json:"max_stack"`
	WeightUnits int64    `json:"weight_units"`
	TradeFlags  []string `json:"trade_flags"`
	BindRules   []string `json:"bind_rules"`
}

func projectSnapshotItem(row SnapshotRow) (PlayerItemProjection, error) {
	var data snapshotItemData
	if err := decodeProjectionObject(row.DataJSON, &data); err != nil {
		return PlayerItemProjection{}, fmt.Errorf("item %q data: %w", row.ContentID, err)
	}
	display, err := projectSnapshotDisplay(row.DisplayJSON, firstNonEmpty(data.Name, string(row.ContentID)))
	if err != nil {
		return PlayerItemProjection{}, fmt.Errorf("item %q display: %w", row.ContentID, err)
	}
	if display.Rarity == "" {
		display.Rarity = data.Rarity
	}
	return PlayerItemProjection{
		ItemID:      string(row.ContentID),
		Display:     display,
		ItemType:    data.ItemType,
		Rarity:      data.Rarity,
		MaxStack:    data.MaxStack,
		WeightUnits: data.WeightUnits,
		TradeFlags:  append([]string(nil), data.TradeFlags...),
		BindRules:   append([]string(nil), data.BindRules...),
	}, nil
}

type snapshotModuleData struct {
	Name               string                  `json:"name"`
	Category           string                  `json:"module_category"`
	SlotType           string                  `json:"slot_type"`
	Tier               int                     `json:"tier"`
	Rarity             string                  `json:"rarity"`
	RequiredRank       int                     `json:"required_rank"`
	RequiredRoleLevels []PlayerRoleRequirement `json:"required_role_levels"`
	StatModifiers      []PlayerStatModifier    `json:"stat_modifiers"`
	Energy             PlayerEnergyProfile     `json:"energy"`
	Cooldowns          []PlayerCooldown        `json:"cooldowns"`
	Durability         struct {
		Max int64 `json:"max"`
	} `json:"durability"`
	DurabilityMax        int64    `json:"durability_max"`
	TradeFlags           []string `json:"trade_flags"`
	BindRules            []string `json:"bind_rules"`
	CompatibleSlotTypes  []string `json:"compatible_slot_types"`
	CompatibleCategories []string `json:"compatible_categories"`
}

func projectSnapshotModule(row SnapshotRow) (PlayerModuleProjection, error) {
	var data snapshotModuleData
	if err := decodeProjectionObject(row.DataJSON, &data); err != nil {
		return PlayerModuleProjection{}, fmt.Errorf("module %q data: %w", row.ContentID, err)
	}
	display, err := projectSnapshotDisplay(row.DisplayJSON, firstNonEmpty(data.Name, string(row.ContentID)))
	if err != nil {
		return PlayerModuleProjection{}, fmt.Errorf("module %q display: %w", row.ContentID, err)
	}
	if display.Rarity == "" {
		display.Rarity = data.Rarity
	}
	if display.Tier == 0 {
		display.Tier = data.Tier
	}
	durabilityMax := data.DurabilityMax
	if durabilityMax == 0 {
		durabilityMax = data.Durability.Max
	}
	return PlayerModuleProjection{
		ItemID:               string(row.ContentID),
		Display:              display,
		Name:                 data.Name,
		Category:             data.Category,
		SlotType:             data.SlotType,
		Tier:                 data.Tier,
		Rarity:               data.Rarity,
		RequiredRank:         data.RequiredRank,
		RequiredRoleLevels:   append([]PlayerRoleRequirement(nil), data.RequiredRoleLevels...),
		StatModifiers:        append([]PlayerStatModifier(nil), data.StatModifiers...),
		Energy:               data.Energy,
		Cooldowns:            append([]PlayerCooldown(nil), data.Cooldowns...),
		DurabilityMax:        durabilityMax,
		TradeFlags:           append([]string(nil), data.TradeFlags...),
		BindRules:            append([]string(nil), data.BindRules...),
		CompatibleSlotTypes:  append([]string(nil), data.CompatibleSlotTypes...),
		CompatibleCategories: append([]string(nil), data.CompatibleCategories...),
	}, nil
}

type snapshotShopProductData struct {
	ProductType  string                  `json:"product_type"`
	Display      snapshotDisplayMetadata `json:"display"`
	GrantTarget  PlayerGrantTarget       `json:"grant_target"`
	Price        PlayerPricePolicy       `json:"price_policy"`
	Stock        PlayerStockPolicy       `json:"stock_policy"`
	Availability PlayerAvailabilityRule  `json:"availability"`
}

func projectSnapshotShopProduct(row SnapshotRow) (PlayerShopProductProjection, error) {
	var data snapshotShopProductData
	if err := decodeProjectionObject(row.DataJSON, &data); err != nil {
		return PlayerShopProductProjection{}, fmt.Errorf("shop product %q data: %w", row.ContentID, err)
	}
	var display PlayerDisplayMetadata
	if data.Display.isZero() {
		var err error
		display, err = projectSnapshotDisplay(row.DisplayJSON, string(row.ContentID))
		if err != nil {
			return PlayerShopProductProjection{}, fmt.Errorf("shop product %q display: %w", row.ContentID, err)
		}
	} else {
		display = data.Display.project(string(row.ContentID))
	}
	return PlayerShopProductProjection{
		ProductID:    string(row.ContentID),
		ProductType:  data.ProductType,
		Display:      display,
		GrantTarget:  data.GrantTarget,
		Price:        data.Price,
		Stock:        data.Stock,
		Availability: data.Availability,
	}, nil
}

func projectSnapshotDisplay(raw json.RawMessage, fallbackName string) (PlayerDisplayMetadata, error) {
	var display snapshotDisplayMetadata
	if err := decodeOptionalProjectionObject(raw, &display); err != nil {
		return PlayerDisplayMetadata{}, err
	}
	return display.project(fallbackName), nil
}

func decodeProjectionObject(raw json.RawMessage, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ErrInvalidContentJSON
	}
	return json.Unmarshal(raw, target)
}

func decodeOptionalProjectionObject(raw json.RawMessage, target any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	return json.Unmarshal(raw, target)
}

func projectCategory(category catalog.ContentCategory) PlayerContentCategory {
	return PlayerContentCategory{
		ID:          category.ID,
		DisplayName: category.DisplayName,
		SortOrder:   category.SortOrder,
	}
}

func projectItemDefinition(definition economy.ItemDefinition, displays map[string]PlayerDisplayMetadata) PlayerItemProjection {
	display := displays[grantDisplayKey(catalog.GrantTargetKindItem, definition.ItemID.String())]
	if isZeroDisplay(display) {
		display = PlayerDisplayMetadata{
			DisplayName: definition.Name,
			Rarity:      definition.Rarity.String(),
		}
	}
	if display.Rarity == "" {
		display.Rarity = definition.Rarity.String()
	}
	return PlayerItemProjection{
		ItemID:      definition.ItemID.String(),
		Display:     display,
		ItemType:    definition.Type.String(),
		Rarity:      definition.Rarity.String(),
		MaxStack:    definition.MaxStack.Int64(),
		WeightUnits: definition.WeightUnits.Int64(),
		TradeFlags:  projectTradeFlags(definition.TradeFlags),
		BindRules:   projectBindRules(definition.BindRules),
	}
}

func projectModuleDefinition(definition modules.ModuleDefinition, displays map[string]PlayerDisplayMetadata) PlayerModuleProjection {
	display := displays[grantDisplayKey(catalog.GrantTargetKindModule, definition.ItemID.String())]
	if isZeroDisplay(display) {
		display = PlayerDisplayMetadata{
			DisplayName: definition.Name,
			Rarity:      definition.Rarity.String(),
			Tier:        definition.Tier,
		}
	}
	if display.Rarity == "" {
		display.Rarity = definition.Rarity.String()
	}
	if display.Tier == 0 {
		display.Tier = definition.Tier
	}
	return PlayerModuleProjection{
		ItemID:               definition.ItemID.String(),
		Display:              display,
		Name:                 definition.Name,
		Category:             definition.Category.String(),
		SlotType:             definition.SlotType.String(),
		Tier:                 definition.Tier,
		Rarity:               definition.Rarity.String(),
		RequiredRank:         definition.RequiredRank,
		RequiredRoleLevels:   projectRoleRequirements(definition.RequiredRoleLevels),
		StatModifiers:        projectStatModifiers(definition.StatModifiers),
		Energy:               projectEnergyProfile(definition.Energy),
		Cooldowns:            projectCooldowns(definition.Cooldowns),
		DurabilityMax:        definition.Durability.Max,
		TradeFlags:           projectTradeFlags(definition.TradeFlags),
		BindRules:            projectBindRules(definition.BindRules),
		CompatibleSlotTypes:  projectSlotTypes(definition.CompatibleSlotTypes),
		CompatibleCategories: projectModuleCategories(definition.CompatibleCategories),
	}
}

func projectShopProduct(product catalog.ShopProductDefinition) PlayerShopProductProjection {
	return PlayerShopProductProjection{
		ProductID:    string(product.ProductID),
		ProductType:  string(product.ProductType),
		Display:      projectDisplayMetadata(product.Display),
		GrantTarget:  projectGrantTarget(product.GrantTarget),
		Price:        projectPricePolicy(product.Price),
		Stock:        projectStockPolicy(product.Stock),
		Availability: projectAvailability(product.Availability),
	}
}

func projectDisplayMetadata(display catalog.DisplayMetadata) PlayerDisplayMetadata {
	return PlayerDisplayMetadata{
		DisplayName: display.DisplayName,
		Description: display.Description,
		Category:    display.Category,
		Subcategory: display.Subcategory,
		ArtKey:      display.ArtKey,
		Rarity:      display.Rarity,
		Tier:        display.Tier,
		SortOrder:   display.SortOrder,
	}
}

func projectGrantTarget(target catalog.GrantTarget) PlayerGrantTarget {
	return PlayerGrantTarget{
		Kind:     string(target.Kind),
		RefID:    target.RefID,
		Quantity: target.Quantity,
	}
}

func projectPricePolicy(price catalog.PricePolicy) PlayerPricePolicy {
	return PlayerPricePolicy{
		Currency: string(price.Currency),
		Amount:   price.Amount,
		Fixed:    price.Fixed,
	}
}

func projectStockPolicy(stock catalog.StockPolicy) PlayerStockPolicy {
	return PlayerStockPolicy{
		Kind:      string(stock.Kind),
		Remaining: stock.Remaining,
		Total:     stock.Total,
	}
}

func projectAvailability(availability catalog.AvailabilityRule) PlayerAvailabilityRule {
	return PlayerAvailabilityRule{
		Available:    availability.Available,
		LockedReason: availability.LockedReason,
		RequiredRank: availability.RequiredRank,
	}
}

func projectRoleRequirements(requirements []modules.RoleRequirement) []PlayerRoleRequirement {
	out := make([]PlayerRoleRequirement, 0, len(requirements))
	for _, requirement := range requirements {
		out = append(out, PlayerRoleRequirement{
			Role:  requirement.Role.String(),
			Level: requirement.Level,
		})
	}
	return out
}

func projectStatModifiers(modifiers []modules.StatModifier) []PlayerStatModifier {
	out := make([]PlayerStatModifier, 0, len(modifiers))
	for _, modifier := range modifiers {
		out = append(out, PlayerStatModifier{
			Stat:  modifier.Stat.String(),
			Kind:  modifier.Kind.String(),
			Value: modifier.Value,
		})
	}
	return out
}

func projectEnergyProfile(energy modules.EnergyProfile) PlayerEnergyProfile {
	return PlayerEnergyProfile{
		ActivationCost: energy.ActivationCost,
		Upkeep:         energy.Upkeep,
	}
}

func projectCooldowns(cooldowns []modules.Cooldown) []PlayerCooldown {
	out := make([]PlayerCooldown, 0, len(cooldowns))
	for _, cooldown := range cooldowns {
		out = append(out, PlayerCooldown{
			Key:        cooldown.Key.String(),
			DurationMS: cooldown.DurationMS,
		})
	}
	return out
}

func projectSlotTypes(slotTypes []modules.ModuleSlotType) []string {
	out := make([]string, 0, len(slotTypes))
	for _, slotType := range slotTypes {
		out = append(out, slotType.String())
	}
	return out
}

func projectModuleCategories(categories []modules.ModuleCategory) []string {
	out := make([]string, 0, len(categories))
	for _, category := range categories {
		out = append(out, category.String())
	}
	return out
}

func projectTradeFlags(flags []economy.TradeFlag) []string {
	out := make([]string, 0, len(flags))
	for _, flag := range flags {
		out = append(out, flag.String())
	}
	return out
}

func projectBindRules(rules []economy.BindRule) []string {
	out := make([]string, 0, len(rules))
	for _, rule := range rules {
		out = append(out, rule.String())
	}
	return out
}

func shopDisplayByGrantTarget(products []catalog.ShopProductDefinition) map[string]PlayerDisplayMetadata {
	displays := make(map[string]PlayerDisplayMetadata, len(products))
	for _, product := range products {
		displays[grantDisplayKey(product.GrantTarget.Kind, product.GrantTarget.RefID)] = projectDisplayMetadata(product.Display)
	}
	return displays
}

func grantDisplayKey(kind catalog.GrantTargetKind, refID string) string {
	return string(kind) + ":" + refID
}

func categoriesFromShopProducts(products []PlayerShopProductProjection) []PlayerContentCategory {
	byID := map[string]PlayerContentCategory{}
	for _, product := range products {
		if product.Display.Category == "" {
			continue
		}
		category, ok := byID[product.Display.Category]
		if !ok {
			category = PlayerContentCategory{
				ID:          product.Display.Category,
				DisplayName: product.Display.Category,
				SortOrder:   product.Display.SortOrder,
			}
		}
		if product.Display.SortOrder != 0 && (category.SortOrder == 0 || product.Display.SortOrder < category.SortOrder) {
			category.SortOrder = product.Display.SortOrder
		}
		byID[product.Display.Category] = category
	}
	out := make([]PlayerContentCategory, 0, len(byID))
	for _, category := range byID {
		out = append(out, category)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder == out[j].SortOrder {
			return out[i].ID < out[j].ID
		}
		return out[i].SortOrder < out[j].SortOrder
	})
	return out
}

func isZeroDisplay(display PlayerDisplayMetadata) bool {
	return display.DisplayName == "" &&
		display.Description == "" &&
		display.Category == "" &&
		display.Subcategory == "" &&
		display.ArtKey == "" &&
		display.Rarity == "" &&
		display.Tier == 0 &&
		display.SortOrder == 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

package economy

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

var (
	ErrEmptyItemName       = errors.New("empty item name")
	ErrInvalidItemType     = errors.New("invalid item type")
	ErrInvalidItemRarity   = errors.New("invalid item rarity")
	ErrInvalidTradeFlag    = errors.New("invalid trade flag")
	ErrInvalidBindRule     = errors.New("invalid bind rule")
	ErrInvalidMaxStack     = errors.New("invalid item max stack")
	ErrInvalidMetadataJSON = errors.New("invalid item metadata json")
	ErrItemSourceMismatch  = errors.New("item source definition mismatch")
)

// ItemType identifies whether item state is modeled as a stack or instance.
type ItemType string

const (
	ItemTypeStackable ItemType = "stackable"
	ItemTypeInstance  ItemType = "instance"
)

// ItemRarity identifies the item rarity bucket used by catalogs and economy UI.
type ItemRarity string

const (
	ItemRarityCommon    ItemRarity = "common"
	ItemRarityUncommon  ItemRarity = "uncommon"
	ItemRarityRare      ItemRarity = "rare"
	ItemRarityEpic      ItemRarity = "epic"
	ItemRarityLegendary ItemRarity = "legendary"
	ItemRarityEvent     ItemRarity = "event"
)

// TradeFlag declares where an item may move in later economy services.
type TradeFlag string

const (
	TradeFlagTradeable        TradeFlag = "tradeable"
	TradeFlagMarketTradeable  TradeFlag = "market_tradeable"
	TradeFlagAuctionTradeable TradeFlag = "auction_tradeable"
	TradeFlagDroppable        TradeFlag = "droppable"
	TradeFlagDestroyable      TradeFlag = "destroyable"
)

// BindRule declares when an item becomes account or player bound.
type BindRule string

const (
	BindRuleNone         BindRule = "none"
	BindRuleOnPickup     BindRule = "bind_on_pickup"
	BindRuleOnEquip      BindRule = "bind_on_equip"
	BindRuleAccountBound BindRule = "account_bound"
	BindRuleSoulbound    BindRule = "soulbound"
)

// ItemDefinition records catalog fields shared by stackable and instance items.
type ItemDefinition struct {
	Source         catalog.VersionedDefinition `json:"source"`
	ItemID         foundation.ItemID           `json:"item_id"`
	Name           string                      `json:"name"`
	Type           ItemType                    `json:"item_type"`
	Rarity         ItemRarity                  `json:"rarity"`
	MaxStack       foundation.Quantity         `json:"max_stack"`
	WeightUnits    foundation.Quantity         `json:"weight_units"`
	TradeFlags     []TradeFlag                 `json:"trade_flags,omitempty"`
	BindRules      []BindRule                  `json:"bind_rules,omitempty"`
	MetadataSchema json.RawMessage             `json:"metadata_schema,omitempty"`
}

// NewItemDefinition validates and returns an item definition.
func NewItemDefinition(
	source catalog.VersionedDefinition,
	itemID foundation.ItemID,
	name string,
	itemType ItemType,
	rarity ItemRarity,
	maxStack foundation.Quantity,
	weightUnits foundation.Quantity,
	tradeFlags []TradeFlag,
	bindRules []BindRule,
	metadataSchema json.RawMessage,
) (ItemDefinition, error) {
	definition := ItemDefinition{
		Source:         source,
		ItemID:         itemID,
		Name:           name,
		Type:           itemType,
		Rarity:         rarity,
		MaxStack:       maxStack,
		WeightUnits:    weightUnits,
		TradeFlags:     append([]TradeFlag(nil), tradeFlags...),
		BindRules:      append([]BindRule(nil), bindRules...),
		MetadataSchema: cloneRawJSON(metadataSchema),
	}
	if err := definition.Validate(); err != nil {
		return ItemDefinition{}, err
	}
	return definition, nil
}

// String returns the stable item type representation.
func (itemType ItemType) String() string {
	return string(itemType)
}

// Validate reports whether itemType is supported.
func (itemType ItemType) Validate() error {
	switch itemType {
	case ItemTypeStackable, ItemTypeInstance:
		return nil
	default:
		return fmt.Errorf("item type %q: %w", itemType, ErrInvalidItemType)
	}
}

// String returns the stable rarity representation.
func (rarity ItemRarity) String() string {
	return string(rarity)
}

// Validate reports whether rarity is supported.
func (rarity ItemRarity) Validate() error {
	switch rarity {
	case ItemRarityCommon,
		ItemRarityUncommon,
		ItemRarityRare,
		ItemRarityEpic,
		ItemRarityLegendary,
		ItemRarityEvent:
		return nil
	default:
		return fmt.Errorf("item rarity %q: %w", rarity, ErrInvalidItemRarity)
	}
}

// String returns the stable trade flag representation.
func (flag TradeFlag) String() string {
	return string(flag)
}

// Validate reports whether flag is supported.
func (flag TradeFlag) Validate() error {
	switch flag {
	case TradeFlagTradeable,
		TradeFlagMarketTradeable,
		TradeFlagAuctionTradeable,
		TradeFlagDroppable,
		TradeFlagDestroyable:
		return nil
	default:
		return fmt.Errorf("trade flag %q: %w", flag, ErrInvalidTradeFlag)
	}
}

// String returns the stable bind rule representation.
func (rule BindRule) String() string {
	return string(rule)
}

// Validate reports whether rule is supported.
func (rule BindRule) Validate() error {
	switch rule {
	case BindRuleNone,
		BindRuleOnPickup,
		BindRuleOnEquip,
		BindRuleAccountBound,
		BindRuleSoulbound:
		return nil
	default:
		return fmt.Errorf("bind rule %q: %w", rule, ErrInvalidBindRule)
	}
}

// Validate reports whether definition has valid catalog and item fields.
func (definition ItemDefinition) Validate() error {
	if err := definition.Source.Validate(); err != nil {
		return err
	}
	if err := definition.ItemID.Validate(); err != nil {
		return err
	}
	if err := validateItemSource(definition.Source, definition.ItemID); err != nil {
		return err
	}
	if strings.TrimSpace(definition.Name) == "" {
		return ErrEmptyItemName
	}
	if err := definition.Type.Validate(); err != nil {
		return err
	}
	if err := definition.Rarity.Validate(); err != nil {
		return err
	}
	if err := definition.MaxStack.Validate(); err != nil {
		return err
	}
	if definition.Type == ItemTypeInstance && definition.MaxStack.Int64() != 1 {
		return fmt.Errorf("instance item max stack %d: %w", definition.MaxStack.Int64(), ErrInvalidMaxStack)
	}
	if err := definition.WeightUnits.Validate(); err != nil {
		return err
	}
	for _, flag := range definition.TradeFlags {
		if err := flag.Validate(); err != nil {
			return err
		}
	}
	for _, rule := range definition.BindRules {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
	if err := validateRawJSON("metadata schema", definition.MetadataSchema); err != nil {
		return err
	}
	return nil
}

func validateItemSource(source catalog.VersionedDefinition, itemID foundation.ItemID) error {
	if source.DefinitionID.String() != itemID.String() {
		return fmt.Errorf("source %q item %q: %w", source.DefinitionID, itemID, ErrItemSourceMismatch)
	}
	return nil
}

func validateRawJSON(name string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	if !json.Valid(raw) {
		return fmt.Errorf("%s: %w", name, ErrInvalidMetadataJSON)
	}
	return nil
}

func cloneRawJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

package economy

import (
	"encoding/json"
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestItemDefinitionAcceptsCatalogFields(t *testing.T) {
	definition := validStackableDefinition(t)

	if err := definition.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
	if got := definition.Type.String(); got != "stackable" {
		t.Fatalf("Type.String() = %q, want stackable", got)
	}
	if got := definition.Rarity.String(); got != "common" {
		t.Fatalf("Rarity.String() = %q, want common", got)
	}
}

func TestItemDefinitionRejectsBlankInvalidAndMismatchedFields(t *testing.T) {
	source := validItemSource(t, "iron_ore")
	maxStack := validQuantity(t, 100)
	weight := validQuantity(t, 1)

	if _, err := NewItemDefinition(source, "", "Iron Ore", ItemTypeStackable, ItemRarityCommon, maxStack, weight, nil, nil, nil); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank item id error = %v, want foundation.ErrEmptyID", err)
	}
	if _, err := NewItemDefinition(source, "iron_ore", " ", ItemTypeStackable, ItemRarityCommon, maxStack, weight, nil, nil, nil); !errors.Is(err, ErrEmptyItemName) {
		t.Fatalf("blank name error = %v, want ErrEmptyItemName", err)
	}
	if _, err := NewItemDefinition(source, "iron_ore", "Iron Ore", ItemType("currency"), ItemRarityCommon, maxStack, weight, nil, nil, nil); !errors.Is(err, ErrInvalidItemType) {
		t.Fatalf("invalid type error = %v, want ErrInvalidItemType", err)
	}
	if _, err := NewItemDefinition(source, "iron_ore", "Iron Ore", ItemTypeStackable, ItemRarity("mythic"), maxStack, weight, nil, nil, nil); !errors.Is(err, ErrInvalidItemRarity) {
		t.Fatalf("invalid rarity error = %v, want ErrInvalidItemRarity", err)
	}
	if _, err := NewItemDefinition(source, "copper_ore", "Copper Ore", ItemTypeStackable, ItemRarityCommon, maxStack, weight, nil, nil, nil); !errors.Is(err, ErrItemSourceMismatch) {
		t.Fatalf("source mismatch error = %v, want ErrItemSourceMismatch", err)
	}
	if _, err := NewItemDefinition(source, "iron_ore", "Iron Ore", ItemTypeStackable, ItemRarityCommon, foundation.Quantity{}, weight, nil, nil, nil); !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("zero max stack error = %v, want foundation.ErrNonPositiveAmount", err)
	}
}

func TestInstanceDefinitionRejectsMaxStackAboveOne(t *testing.T) {
	source := validItemSource(t, "laser_module")
	maxStack := validQuantity(t, 2)
	weight := validQuantity(t, 3)

	_, err := NewItemDefinition(source, "laser_module", "Laser Module", ItemTypeInstance, ItemRarityRare, maxStack, weight, nil, nil, nil)
	if !errors.Is(err, ErrInvalidMaxStack) {
		t.Fatalf("instance max stack error = %v, want ErrInvalidMaxStack", err)
	}
}

func TestItemDefinitionRejectsInvalidFlagsRulesAndMetadata(t *testing.T) {
	source := validItemSource(t, "iron_ore")
	maxStack := validQuantity(t, 100)
	weight := validQuantity(t, 1)

	if _, err := NewItemDefinition(source, "iron_ore", "Iron Ore", ItemTypeStackable, ItemRarityCommon, maxStack, weight, []TradeFlag{"bad_flag"}, nil, nil); !errors.Is(err, ErrInvalidTradeFlag) {
		t.Fatalf("invalid trade flag error = %v, want ErrInvalidTradeFlag", err)
	}
	if _, err := NewItemDefinition(source, "iron_ore", "Iron Ore", ItemTypeStackable, ItemRarityCommon, maxStack, weight, nil, []BindRule{"bad_rule"}, nil); !errors.Is(err, ErrInvalidBindRule) {
		t.Fatalf("invalid bind rule error = %v, want ErrInvalidBindRule", err)
	}
	if _, err := NewItemDefinition(source, "iron_ore", "Iron Ore", ItemTypeStackable, ItemRarityCommon, maxStack, weight, nil, nil, json.RawMessage(`{"type":`)); !errors.Is(err, ErrInvalidMetadataJSON) {
		t.Fatalf("invalid metadata schema error = %v, want ErrInvalidMetadataJSON", err)
	}
}

func TestItemDefinitionJSONBehaviorIsStable(t *testing.T) {
	definition := validStackableDefinition(t)

	payload, err := json.Marshal(definition)
	if err != nil {
		t.Fatalf("json marshal definition: %v", err)
	}
	want := `{"source":{"definition_id":"iron_ore","catalog_version":"item_catalog_v1"},"item_id":"iron_ore","name":"Iron Ore","item_type":"stackable","rarity":"common","max_stack":100,"weight_units":1,"trade_flags":["tradeable","market_tradeable"],"bind_rules":["none"],"metadata_schema":{"type":"object"}}`
	if got := string(payload); got != want {
		t.Fatalf("definition JSON = %s, want %s", got, want)
	}
}

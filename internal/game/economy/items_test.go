package economy

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

func TestStackableItemRejectsZeroAndNegativeQuantity(t *testing.T) {
	source := validItemSource(t, "iron_ore")
	location := validLocation(t)

	item := StackableItem{
		Source:         source,
		ItemInstanceID: "stack-1",
		ItemID:         "iron_ore",
		OwnerPlayerID:  "player-1",
		Location:       location,
		Quantity:       foundation.Quantity{},
	}
	if err := item.Validate(); !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("zero quantity Validate() = %v, want foundation.ErrNonPositiveAmount", err)
	}

	if _, err := foundation.NewQuantity(-1); !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("negative quantity constructor error = %v, want foundation.ErrNonPositiveAmount", err)
	}
}

func TestInstanceItemQuantityCannotExceedOne(t *testing.T) {
	source := validItemSource(t, "laser_module")
	location := validLocation(t)
	quantity := validQuantity(t, 2)

	_, err := NewInstanceItem(source, "instance-1", "laser_module", "player-1", location, quantity)
	if !errors.Is(err, ErrInvalidInstanceQuantity) {
		t.Fatalf("quantity above one error = %v, want ErrInvalidInstanceQuantity", err)
	}
}

func TestItemModelsRejectBlankIDsAndInvalidLocations(t *testing.T) {
	source := validItemSource(t, "iron_ore")
	location := validLocation(t)
	quantity := validQuantity(t, 1)

	if _, err := NewStackableItem(source, "", "iron_ore", "player-1", location, quantity); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank stack item instance id error = %v, want foundation.ErrEmptyID", err)
	}
	if _, err := NewStackableItem(source, "stack-1", "", "player-1", location, quantity); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank stack item id error = %v, want foundation.ErrEmptyID", err)
	}
	if _, err := NewStackableItem(source, "stack-1", "iron_ore", "", location, quantity); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank owner id error = %v, want foundation.ErrEmptyID", err)
	}
	if _, err := NewStackableItem(source, "stack-1", "iron_ore", "player-1", ItemLocation{}, quantity); !errors.Is(err, ErrInvalidLocationKind) {
		t.Fatalf("invalid stack location error = %v, want ErrInvalidLocationKind", err)
	}
	if _, err := NewInstanceItem(source, "instance-1", "laser_module", "player-1", location, quantity); !errors.Is(err, ErrItemSourceMismatch) {
		t.Fatalf("source mismatch error = %v, want ErrItemSourceMismatch", err)
	}
}

func TestInstanceItemRejectsBlankBoundStateAndInvalidMetadata(t *testing.T) {
	source := validItemSource(t, "laser_module")
	location := validLocation(t)
	quantity := validQuantity(t, 1)

	item := InstanceItem{
		Source:         source,
		ItemInstanceID: "instance-1",
		ItemID:         "laser_module",
		OwnerPlayerID:  "player-1",
		Location:       location,
		Quantity:       quantity,
		BoundState:     BoundState(""),
	}
	if err := item.Validate(); !errors.Is(err, ErrInvalidBoundState) {
		t.Fatalf("blank bound state error = %v, want ErrInvalidBoundState", err)
	}

	item.BoundState = BoundStateUnbound
	item.MetadataJSON = json.RawMessage(`{"durability":`)
	if err := item.Validate(); !errors.Is(err, ErrInvalidMetadataJSON) {
		t.Fatalf("invalid metadata error = %v, want ErrInvalidMetadataJSON", err)
	}
}

func TestItemModelJSONAndStringBehaviorIsStable(t *testing.T) {
	source := validItemSource(t, "iron_ore")
	location := validLocation(t)
	quantity := validQuantity(t, 25)
	createdAt := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 17, 12, 30, 0, 0, time.UTC)

	item, err := NewStackableItem(source, "stack-1", "iron_ore", "player-1", location, quantity)
	if err != nil {
		t.Fatalf("NewStackableItem valid value: %v", err)
	}
	item.MetadataJSON = json.RawMessage(`{"quality":"standard"}`)
	item.CreatedAt = createdAt
	item.UpdatedAt = updatedAt

	payload, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json marshal stackable item: %v", err)
	}
	want := `{"source":{"definition_id":"iron_ore","catalog_version":"item_catalog_v1"},"item_instance_id":"stack-1","item_id":"iron_ore","owner_player_id":"player-1","location":{"location_type":"account_inventory","location_id":"player-1"},"quantity":25,"metadata_json":{"quality":"standard"},"created_at":"2026-06-17T12:00:00Z","updated_at":"2026-06-17T12:30:00Z"}`
	if got := string(payload); got != want {
		t.Fatalf("stackable item JSON = %s, want %s", got, want)
	}

	if got := BoundStateUnbound.String(); got != "unbound" {
		t.Fatalf("BoundState.String() = %q, want unbound", got)
	}
}

func validStackableDefinition(t *testing.T) ItemDefinition {
	t.Helper()

	source := validItemSource(t, "iron_ore")
	maxStack := validQuantity(t, 100)
	weight := validQuantity(t, 1)
	definition, err := NewItemDefinition(
		source,
		"iron_ore",
		"Iron Ore",
		ItemTypeStackable,
		ItemRarityCommon,
		maxStack,
		weight,
		[]TradeFlag{TradeFlagTradeable, TradeFlagMarketTradeable},
		[]BindRule{BindRuleNone},
		json.RawMessage(`{"type":"object"}`),
	)
	if err != nil {
		t.Fatalf("NewItemDefinition valid value: %v", err)
	}
	return definition
}

func validItemSource(t *testing.T, itemID string) catalog.VersionedDefinition {
	t.Helper()

	source, err := catalog.NewVersionedDefinitionFromStrings(itemID, "item_catalog_v1")
	if err != nil {
		t.Fatalf("NewVersionedDefinitionFromStrings valid value: %v", err)
	}
	return source
}

func validQuantity(t *testing.T, amount int64) foundation.Quantity {
	t.Helper()

	quantity, err := foundation.NewQuantity(amount)
	if err != nil {
		t.Fatalf("NewQuantity(%d): %v", amount, err)
	}
	return quantity
}

func validLocation(t *testing.T) ItemLocation {
	t.Helper()

	location, err := NewItemLocation(LocationKindAccountInventory, "player-1")
	if err != nil {
		t.Fatalf("NewItemLocation valid value: %v", err)
	}
	return location
}

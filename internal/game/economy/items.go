package economy

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

// Item state validation errors.
var (
	ErrInvalidInstanceQuantity = errors.New("invalid instance item quantity")
	ErrInvalidBoundState       = errors.New("invalid item bound state")
)

// BoundState records the current binding state of an item instance.
type BoundState string

const (
	BoundStateUnbound      BoundState = "unbound"
	BoundStateAccountBound BoundState = "account_bound"
	BoundStateSoulbound    BoundState = "soulbound"
)

// StackableItem models one stack row for stackable materials, consumables, or fragments.
type StackableItem struct {
	Source         catalog.VersionedDefinition `json:"source"`
	ItemInstanceID foundation.ItemID           `json:"item_instance_id"`
	ItemID         foundation.ItemID           `json:"item_id"`
	OwnerPlayerID  foundation.PlayerID         `json:"owner_player_id"`
	Location       ItemLocation                `json:"location"`
	Quantity       foundation.Quantity         `json:"quantity"`
	MetadataJSON   json.RawMessage             `json:"metadata_json,omitempty"`
	CreatedAt      time.Time                   `json:"created_at"`
	UpdatedAt      time.Time                   `json:"updated_at"`
}

// InstanceItem models one unique item such as a module or coordinate scroll.
type InstanceItem struct {
	Source            catalog.VersionedDefinition `json:"source"`
	ItemInstanceID    foundation.ItemID           `json:"item_instance_id"`
	ItemID            foundation.ItemID           `json:"item_id"`
	OwnerPlayerID     foundation.PlayerID         `json:"owner_player_id"`
	Location          ItemLocation                `json:"location"`
	Quantity          foundation.Quantity         `json:"quantity"`
	DurabilityCurrent int64                       `json:"durability_current,omitempty"`
	BoundState        BoundState                  `json:"bound_state"`
	MetadataJSON      json.RawMessage             `json:"metadata_json,omitempty"`
	CreatedAt         time.Time                   `json:"created_at"`
	UpdatedAt         time.Time                   `json:"updated_at"`
}

// NewStackableItem validates and returns a stackable item state model.
func NewStackableItem(
	source catalog.VersionedDefinition,
	itemInstanceID foundation.ItemID,
	itemID foundation.ItemID,
	ownerPlayerID foundation.PlayerID,
	location ItemLocation,
	quantity foundation.Quantity,
) (StackableItem, error) {
	item := StackableItem{
		Source:         source,
		ItemInstanceID: itemInstanceID,
		ItemID:         itemID,
		OwnerPlayerID:  ownerPlayerID,
		Location:       location,
		Quantity:       quantity,
	}
	if err := item.Validate(); err != nil {
		return StackableItem{}, err
	}
	return item, nil
}

// NewInstanceItem validates and returns a unique item state model.
func NewInstanceItem(
	source catalog.VersionedDefinition,
	itemInstanceID foundation.ItemID,
	itemID foundation.ItemID,
	ownerPlayerID foundation.PlayerID,
	location ItemLocation,
	quantity foundation.Quantity,
) (InstanceItem, error) {
	item := InstanceItem{
		Source:         source,
		ItemInstanceID: itemInstanceID,
		ItemID:         itemID,
		OwnerPlayerID:  ownerPlayerID,
		Location:       location,
		Quantity:       quantity,
		BoundState:     BoundStateUnbound,
	}
	if err := item.Validate(); err != nil {
		return InstanceItem{}, err
	}
	return item, nil
}

// String returns the stable bound state representation.
func (state BoundState) String() string {
	return string(state)
}

// Validate reports whether state is supported.
func (state BoundState) Validate() error {
	switch state {
	case BoundStateUnbound, BoundStateAccountBound, BoundStateSoulbound:
		return nil
	default:
		return fmt.Errorf("bound state %q: %w", state, ErrInvalidBoundState)
	}
}

// Validate reports whether item has valid ids, location, source, and quantity.
func (item StackableItem) Validate() error {
	if err := validateItemIdentity(item.Source, item.ItemInstanceID, item.ItemID, item.OwnerPlayerID, item.Location); err != nil {
		return err
	}
	if err := item.Quantity.Validate(); err != nil {
		return err
	}
	if err := validateRawJSON("metadata json", item.MetadataJSON); err != nil {
		return err
	}
	return nil
}

// Validate reports whether item has valid ids, location, source, and quantity.
func (item InstanceItem) Validate() error {
	if err := validateItemIdentity(item.Source, item.ItemInstanceID, item.ItemID, item.OwnerPlayerID, item.Location); err != nil {
		return err
	}
	if err := item.Quantity.Validate(); err != nil {
		return err
	}
	if item.Quantity.Int64() != 1 {
		return fmt.Errorf("instance quantity %d: %w", item.Quantity.Int64(), ErrInvalidInstanceQuantity)
	}
	if err := item.BoundState.Validate(); err != nil {
		return err
	}
	if err := validateRawJSON("metadata json", item.MetadataJSON); err != nil {
		return err
	}
	return nil
}

func validateItemIdentity(
	source catalog.VersionedDefinition,
	itemInstanceID foundation.ItemID,
	itemID foundation.ItemID,
	ownerPlayerID foundation.PlayerID,
	location ItemLocation,
) error {
	if err := source.Validate(); err != nil {
		return err
	}
	if err := itemInstanceID.Validate(); err != nil {
		return err
	}
	if err := itemID.Validate(); err != nil {
		return err
	}
	if err := validateItemSource(source, itemID); err != nil {
		return err
	}
	if err := ownerPlayerID.Validate(); err != nil {
		return err
	}
	if err := location.Validate(); err != nil {
		return err
	}
	return nil
}

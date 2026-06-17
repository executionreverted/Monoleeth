package economy

import (
	"errors"
	"fmt"
	"strings"
)

// Supported item location validation errors.
var (
	ErrInvalidLocationKind = errors.New("invalid item location kind")
	ErrEmptyLocationID     = errors.New("empty item location id")
)

// LocationKind identifies a supported item storage location.
type LocationKind string

const (
	LocationKindAccountInventory LocationKind = "account_inventory"
	LocationKindShipCargo        LocationKind = "ship_cargo"
	LocationKindPlanetStorage    LocationKind = "planet_storage"
	LocationKindStationStorage   LocationKind = "station_storage"
	LocationKindMarketEscrow     LocationKind = "market_escrow"
	LocationKindAuctionEscrow    LocationKind = "auction_escrow"
	LocationKindCraftingReserved LocationKind = "crafting_reserved"
	LocationKindSystemSink       LocationKind = "system_sink"
	LocationKindWorldDrop        LocationKind = "world_drop"
)

// LocationID identifies the owner/container for a location kind.
type LocationID string

// ItemLocation records where an item stack or instance currently resides.
type ItemLocation struct {
	Kind LocationKind `json:"location_type"`
	ID   LocationID   `json:"location_id"`
}

// SupportedLocationKinds returns all roadmap-supported location kinds.
func SupportedLocationKinds() []LocationKind {
	return []LocationKind{
		LocationKindAccountInventory,
		LocationKindShipCargo,
		LocationKindPlanetStorage,
		LocationKindStationStorage,
		LocationKindMarketEscrow,
		LocationKindAuctionEscrow,
		LocationKindCraftingReserved,
		LocationKindSystemSink,
		LocationKindWorldDrop,
	}
}

// NewItemLocation validates and returns an item location.
func NewItemLocation(kind LocationKind, id string) (ItemLocation, error) {
	location := ItemLocation{
		Kind: kind,
		ID:   LocationID(id),
	}
	if err := location.Validate(); err != nil {
		return ItemLocation{}, err
	}
	return location, nil
}

// String returns the wire representation of the location kind.
func (kind LocationKind) String() string {
	return string(kind)
}

// Validate reports whether kind is one of the supported roadmap locations.
func (kind LocationKind) Validate() error {
	switch kind {
	case LocationKindAccountInventory,
		LocationKindShipCargo,
		LocationKindPlanetStorage,
		LocationKindStationStorage,
		LocationKindMarketEscrow,
		LocationKindAuctionEscrow,
		LocationKindCraftingReserved,
		LocationKindSystemSink,
		LocationKindWorldDrop:
		return nil
	default:
		return fmt.Errorf("item location kind %q: %w", kind, ErrInvalidLocationKind)
	}
}

// IsZero reports whether kind is the zero value.
func (kind LocationKind) IsZero() bool {
	return kind == ""
}

// String returns the stable location id representation.
func (id LocationID) String() string {
	return string(id)
}

// Validate reports whether id is non-blank.
func (id LocationID) Validate() error {
	if strings.TrimSpace(string(id)) == "" {
		return ErrEmptyLocationID
	}
	return nil
}

// IsZero reports whether id is the zero value.
func (id LocationID) IsZero() bool {
	return id == ""
}

// String returns a stable human-readable location reference.
func (location ItemLocation) String() string {
	if location.ID.IsZero() {
		return location.Kind.String()
	}
	return location.Kind.String() + ":" + location.ID.String()
}

// Validate reports whether location has a supported kind and non-blank id.
func (location ItemLocation) Validate() error {
	if err := location.Kind.Validate(); err != nil {
		return err
	}
	if err := location.ID.Validate(); err != nil {
		return err
	}
	return nil
}

// IsZero reports whether location is the zero value.
func (location ItemLocation) IsZero() bool {
	return location.Kind.IsZero() && location.ID.IsZero()
}

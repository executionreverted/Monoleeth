package foundation

import (
	"errors"
	"fmt"
	"strings"
)

// ErrEmptyID reports a missing or blank gameplay identifier.
var ErrEmptyID = errors.New("empty gameplay id")

// AccountID identifies an authenticated account.
type AccountID string

// PlayerID identifies a player character.
type PlayerID string

// WorldID identifies a persistent game world.
type WorldID string

// ZoneID identifies an authoritative world or map zone.
type ZoneID string

// EntityID identifies a live world entity.
type EntityID string

// ItemID identifies an item definition or instance where context defines scope.
type ItemID string

// ShipID identifies a ship chassis or ship instance where context defines scope.
type ShipID string

// ModuleID identifies an equippable module definition or instance where context defines scope.
type ModuleID string

// QuestID identifies a quest definition or player quest where context defines scope.
type QuestID string

// PlanetID identifies a persistent planet.
type PlanetID string

// RouteID identifies an automation or logistics route.
type RouteID string

// ListingID identifies a market listing.
type ListingID string

// AuctionID identifies an auction.
type AuctionID string

// EventID identifies a domain or realtime event.
type EventID string

// RequestID identifies a client request for retry safety.
type RequestID string

// ParseAccountID validates value and returns an AccountID.
func ParseAccountID(value string) (AccountID, error) {
	if err := validateID("account", value); err != nil {
		return "", err
	}
	return AccountID(value), nil
}

// ParsePlayerID validates value and returns a PlayerID.
func ParsePlayerID(value string) (PlayerID, error) {
	if err := validateID("player", value); err != nil {
		return "", err
	}
	return PlayerID(value), nil
}

// ParseWorldID validates value and returns a WorldID.
func ParseWorldID(value string) (WorldID, error) {
	if err := validateID("world", value); err != nil {
		return "", err
	}
	return WorldID(value), nil
}

// ParseZoneID validates value and returns a ZoneID.
func ParseZoneID(value string) (ZoneID, error) {
	if err := validateID("zone", value); err != nil {
		return "", err
	}
	return ZoneID(value), nil
}

// ParseEntityID validates value and returns an EntityID.
func ParseEntityID(value string) (EntityID, error) {
	if err := validateID("entity", value); err != nil {
		return "", err
	}
	return EntityID(value), nil
}

// ParseItemID validates value and returns an ItemID.
func ParseItemID(value string) (ItemID, error) {
	if err := validateID("item", value); err != nil {
		return "", err
	}
	return ItemID(value), nil
}

// ParseShipID validates value and returns a ShipID.
func ParseShipID(value string) (ShipID, error) {
	if err := validateID("ship", value); err != nil {
		return "", err
	}
	return ShipID(value), nil
}

// ParseModuleID validates value and returns a ModuleID.
func ParseModuleID(value string) (ModuleID, error) {
	if err := validateID("module", value); err != nil {
		return "", err
	}
	return ModuleID(value), nil
}

// ParseQuestID validates value and returns a QuestID.
func ParseQuestID(value string) (QuestID, error) {
	if err := validateID("quest", value); err != nil {
		return "", err
	}
	return QuestID(value), nil
}

// ParsePlanetID validates value and returns a PlanetID.
func ParsePlanetID(value string) (PlanetID, error) {
	if err := validateID("planet", value); err != nil {
		return "", err
	}
	return PlanetID(value), nil
}

// ParseRouteID validates value and returns a RouteID.
func ParseRouteID(value string) (RouteID, error) {
	if err := validateID("route", value); err != nil {
		return "", err
	}
	return RouteID(value), nil
}

// ParseListingID validates value and returns a ListingID.
func ParseListingID(value string) (ListingID, error) {
	if err := validateID("listing", value); err != nil {
		return "", err
	}
	return ListingID(value), nil
}

// ParseAuctionID validates value and returns an AuctionID.
func ParseAuctionID(value string) (AuctionID, error) {
	if err := validateID("auction", value); err != nil {
		return "", err
	}
	return AuctionID(value), nil
}

// ParseEventID validates value and returns an EventID.
func ParseEventID(value string) (EventID, error) {
	if err := validateID("event", value); err != nil {
		return "", err
	}
	return EventID(value), nil
}

// ParseRequestID validates value and returns a RequestID.
func ParseRequestID(value string) (RequestID, error) {
	if err := validateID("request", value); err != nil {
		return "", err
	}
	return RequestID(value), nil
}

func (id AccountID) String() string { return string(id) }
func (id PlayerID) String() string  { return string(id) }
func (id WorldID) String() string   { return string(id) }
func (id ZoneID) String() string    { return string(id) }
func (id EntityID) String() string  { return string(id) }
func (id ItemID) String() string    { return string(id) }
func (id ShipID) String() string    { return string(id) }
func (id ModuleID) String() string  { return string(id) }
func (id QuestID) String() string   { return string(id) }
func (id PlanetID) String() string  { return string(id) }
func (id RouteID) String() string   { return string(id) }
func (id ListingID) String() string { return string(id) }
func (id AuctionID) String() string { return string(id) }
func (id EventID) String() string   { return string(id) }
func (id RequestID) String() string { return string(id) }

// Validate reports whether id is non-blank.
func (id AccountID) Validate() error { return validateID("account", string(id)) }

// Validate reports whether id is non-blank.
func (id PlayerID) Validate() error { return validateID("player", string(id)) }

// Validate reports whether id is non-blank.
func (id WorldID) Validate() error { return validateID("world", string(id)) }

// Validate reports whether id is non-blank.
func (id ZoneID) Validate() error { return validateID("zone", string(id)) }

// Validate reports whether id is non-blank.
func (id EntityID) Validate() error { return validateID("entity", string(id)) }

// Validate reports whether id is non-blank.
func (id ItemID) Validate() error { return validateID("item", string(id)) }

// Validate reports whether id is non-blank.
func (id ShipID) Validate() error { return validateID("ship", string(id)) }

// Validate reports whether id is non-blank.
func (id ModuleID) Validate() error { return validateID("module", string(id)) }

// Validate reports whether id is non-blank.
func (id QuestID) Validate() error { return validateID("quest", string(id)) }

// Validate reports whether id is non-blank.
func (id PlanetID) Validate() error { return validateID("planet", string(id)) }

// Validate reports whether id is non-blank.
func (id RouteID) Validate() error { return validateID("route", string(id)) }

// Validate reports whether id is non-blank.
func (id ListingID) Validate() error { return validateID("listing", string(id)) }

// Validate reports whether id is non-blank.
func (id AuctionID) Validate() error { return validateID("auction", string(id)) }

// Validate reports whether id is non-blank.
func (id EventID) Validate() error { return validateID("event", string(id)) }

// Validate reports whether id is non-blank.
func (id RequestID) Validate() error { return validateID("request", string(id)) }

func (id AccountID) IsZero() bool { return id == "" }
func (id PlayerID) IsZero() bool  { return id == "" }
func (id WorldID) IsZero() bool   { return id == "" }
func (id ZoneID) IsZero() bool    { return id == "" }
func (id EntityID) IsZero() bool  { return id == "" }
func (id ItemID) IsZero() bool    { return id == "" }
func (id ShipID) IsZero() bool    { return id == "" }
func (id ModuleID) IsZero() bool  { return id == "" }
func (id QuestID) IsZero() bool   { return id == "" }
func (id PlanetID) IsZero() bool  { return id == "" }
func (id RouteID) IsZero() bool   { return id == "" }
func (id ListingID) IsZero() bool { return id == "" }
func (id AuctionID) IsZero() bool { return id == "" }
func (id EventID) IsZero() bool   { return id == "" }
func (id RequestID) IsZero() bool { return id == "" }

func validateID(kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s id: %w", kind, ErrEmptyID)
	}
	return nil
}

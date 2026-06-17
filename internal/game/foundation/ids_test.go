package foundation

import (
	"errors"
	"testing"
)

type gameplayID interface {
	String() string
	Validate() error
	IsZero() bool
}

type idCase struct {
	name  string
	valid string
	parse func(string) (gameplayID, error)
	zero  func() gameplayID
}

func TestGameplayIDsAcceptValidValuesAndConvertToString(t *testing.T) {
	for _, tc := range gameplayIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			id, err := tc.parse(tc.valid)
			if err != nil {
				t.Fatalf("parse valid id: %v", err)
			}

			if got := id.String(); got != tc.valid {
				t.Fatalf("String() = %q, want %q", got, tc.valid)
			}
			if err := id.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if id.IsZero() {
				t.Fatal("IsZero() = true, want false")
			}
		})
	}
}

func TestGameplayIDsRejectEmptyValues(t *testing.T) {
	emptyValues := []string{"", " ", "\t"}

	for _, tc := range gameplayIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, value := range emptyValues {
				_, err := tc.parse(value)
				if !errors.Is(err, ErrEmptyID) {
					t.Fatalf("parse %q error = %v, want ErrEmptyID", value, err)
				}
			}
		})
	}
}

func TestGameplayIDsRejectNonCanonicalValues(t *testing.T) {
	invalidValues := []string{" player-1", "player-1 ", "player:1", "player\n1"}

	for _, tc := range gameplayIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			for _, value := range invalidValues {
				_, err := tc.parse(value)
				if !errors.Is(err, ErrInvalidID) {
					t.Fatalf("parse %q error = %v, want ErrInvalidID", value, err)
				}
			}
		})
	}
}

func TestGameplayIDZeroValuesAreInvalid(t *testing.T) {
	for _, tc := range gameplayIDCases() {
		t.Run(tc.name, func(t *testing.T) {
			id := tc.zero()

			if !id.IsZero() {
				t.Fatal("IsZero() = false, want true")
			}
			if err := id.Validate(); !errors.Is(err, ErrEmptyID) {
				t.Fatalf("Validate() = %v, want ErrEmptyID", err)
			}
			if got := id.String(); got != "" {
				t.Fatalf("String() = %q, want empty string", got)
			}
		})
	}
}

func gameplayIDCases() []idCase {
	return []idCase{
		{
			name:  "AccountID",
			valid: "account-123",
			parse: func(value string) (gameplayID, error) { return ParseAccountID(value) },
			zero:  func() gameplayID { return AccountID("") },
		},
		{
			name:  "PlayerID",
			valid: "player-123",
			parse: func(value string) (gameplayID, error) { return ParsePlayerID(value) },
			zero:  func() gameplayID { return PlayerID("") },
		},
		{
			name:  "WorldID",
			valid: "world-123",
			parse: func(value string) (gameplayID, error) { return ParseWorldID(value) },
			zero:  func() gameplayID { return WorldID("") },
		},
		{
			name:  "ZoneID",
			valid: "zone-123",
			parse: func(value string) (gameplayID, error) { return ParseZoneID(value) },
			zero:  func() gameplayID { return ZoneID("") },
		},
		{
			name:  "EntityID",
			valid: "entity-123",
			parse: func(value string) (gameplayID, error) { return ParseEntityID(value) },
			zero:  func() gameplayID { return EntityID("") },
		},
		{
			name:  "ItemID",
			valid: "item-123",
			parse: func(value string) (gameplayID, error) { return ParseItemID(value) },
			zero:  func() gameplayID { return ItemID("") },
		},
		{
			name:  "ShipID",
			valid: "ship-123",
			parse: func(value string) (gameplayID, error) { return ParseShipID(value) },
			zero:  func() gameplayID { return ShipID("") },
		},
		{
			name:  "ModuleID",
			valid: "module-123",
			parse: func(value string) (gameplayID, error) { return ParseModuleID(value) },
			zero:  func() gameplayID { return ModuleID("") },
		},
		{
			name:  "QuestID",
			valid: "quest-123",
			parse: func(value string) (gameplayID, error) { return ParseQuestID(value) },
			zero:  func() gameplayID { return QuestID("") },
		},
		{
			name:  "PlanetID",
			valid: "planet-123",
			parse: func(value string) (gameplayID, error) { return ParsePlanetID(value) },
			zero:  func() gameplayID { return PlanetID("") },
		},
		{
			name:  "RouteID",
			valid: "route-123",
			parse: func(value string) (gameplayID, error) { return ParseRouteID(value) },
			zero:  func() gameplayID { return RouteID("") },
		},
		{
			name:  "ListingID",
			valid: "listing-123",
			parse: func(value string) (gameplayID, error) { return ParseListingID(value) },
			zero:  func() gameplayID { return ListingID("") },
		},
		{
			name:  "AuctionID",
			valid: "auction-123",
			parse: func(value string) (gameplayID, error) { return ParseAuctionID(value) },
			zero:  func() gameplayID { return AuctionID("") },
		},
		{
			name:  "EventID",
			valid: "event-123",
			parse: func(value string) (gameplayID, error) { return ParseEventID(value) },
			zero:  func() gameplayID { return EventID("") },
		},
		{
			name:  "RequestID",
			valid: "request-123",
			parse: func(value string) (gameplayID, error) { return ParseRequestID(value) },
			zero:  func() gameplayID { return RequestID("") },
		},
	}
}

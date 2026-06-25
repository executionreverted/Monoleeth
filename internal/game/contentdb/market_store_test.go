package contentdb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
)

func TestMarketListingScanPreservesEscrowSnapshot(t *testing.T) {
	definition := contentdbMarketListingDefinitionForTest(t)
	definitionJSON, err := json.Marshal(snapshotMarketItemDefinition(definition))
	if err != nil {
		t.Fatalf("Marshal(definition) error = %v, want nil", err)
	}
	expiresAt := time.Date(2026, 6, 25, 21, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 6, 25, 20, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 25, 20, 1, 0, 0, time.UTC)

	listing, err := scanMarketListing(fakeMarketListingScanner{values: []any{
		"listing-unit-market",
		"player-unit-market",
		definitionJSON,
		"",
		definition.ItemID.String(),
		int64(12),
		int64(7),
		int64(42),
		economy.CurrencyBucketCredits.String(),
		market.ListingStatusActive.String(),
		economy.LocationKindAccountInventory.String(),
		"account",
		economy.LocationKindMarketEscrow.String(),
		"listing-unit-market",
		createdAt,
		updatedAt,
		sql.NullTime{Time: expiresAt, Valid: true},
		sql.NullTime{},
		"",
	}})
	if err != nil {
		t.Fatalf("scanMarketListing() error = %v, want nil", err)
	}

	if listing.EscrowLocation.Kind != economy.LocationKindMarketEscrow ||
		listing.EscrowLocation.ID != economy.LocationID("listing-unit-market") ||
		listing.SourceReturnLocation.Kind != economy.LocationKindAccountInventory ||
		listing.RemainingQuantity != 7 ||
		listing.Status != market.ListingStatusActive ||
		listing.ItemDefinition.ItemID != definition.ItemID {
		t.Fatalf("listing = %+v, want escrow/source/status/quantity/item snapshot preserved", listing)
	}
}

type fakeMarketListingScanner struct {
	values []any
}

func (scanner fakeMarketListingScanner) Scan(dest ...any) error {
	if len(dest) != len(scanner.values) {
		return fmt.Errorf("dest len %d values len %d", len(dest), len(scanner.values))
	}
	for index, value := range scanner.values {
		switch target := dest[index].(type) {
		case *string:
			typed, ok := value.(string)
			if !ok {
				return fmt.Errorf("value %d has type %T, want string", index, value)
			}
			*target = typed
		case *[]byte:
			typed, ok := value.([]byte)
			if !ok {
				return fmt.Errorf("value %d has type %T, want []byte", index, value)
			}
			*target = append((*target)[:0], typed...)
		case *int64:
			typed, ok := value.(int64)
			if !ok {
				return fmt.Errorf("value %d has type %T, want int64", index, value)
			}
			*target = typed
		case *time.Time:
			typed, ok := value.(time.Time)
			if !ok {
				return fmt.Errorf("value %d has type %T, want time.Time", index, value)
			}
			*target = typed
		case *sql.NullTime:
			typed, ok := value.(sql.NullTime)
			if !ok {
				return fmt.Errorf("value %d has type %T, want sql.NullTime", index, value)
			}
			*target = typed
		default:
			return fmt.Errorf("dest %d has unsupported type %T", index, target)
		}
	}
	return nil
}

func contentdbMarketListingDefinitionForTest(t *testing.T) economy.ItemDefinition {
	t.Helper()
	maxStack, err := foundation.NewQuantity(100)
	if err != nil {
		t.Fatalf("NewQuantity(100) error = %v, want nil", err)
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity(1) error = %v, want nil", err)
	}
	definition, err := economy.NewItemDefinition(
		catalog.VersionedDefinition{DefinitionID: catalog.DefinitionID("raw_ore"), Version: catalog.Version("test-v1")},
		foundation.ItemID("raw_ore"),
		"Raw Ore",
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		maxStack,
		weight,
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewItemDefinition() error = %v, want nil", err)
	}
	return definition
}

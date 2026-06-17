package economy

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestSupportedLocationKindsValidateAndStringify(t *testing.T) {
	want := []LocationKind{
		LocationKindAccountInventory,
		LocationKindShipCargo,
		LocationKindShipEquipped,
		LocationKindPlanetStorage,
		LocationKindStationStorage,
		LocationKindMarketEscrow,
		LocationKindAuctionEscrow,
		LocationKindCraftingReserved,
		LocationKindSystemSink,
		LocationKindWorldDrop,
	}

	got := SupportedLocationKinds()
	if len(got) != len(want) {
		t.Fatalf("SupportedLocationKinds len = %d, want %d", len(got), len(want))
	}
	for i, kind := range want {
		if got[i] != kind {
			t.Fatalf("SupportedLocationKinds[%d] = %q, want %q", i, got[i], kind)
		}
		if err := kind.Validate(); err != nil {
			t.Fatalf("%q Validate() = %v, want nil", kind, err)
		}
		if kind.String() != string(kind) {
			t.Fatalf("%q String() = %q, want %q", kind, kind.String(), string(kind))
		}
	}
}

func TestItemLocationRejectsInvalidKindAndBlankID(t *testing.T) {
	if _, err := NewItemLocation(LocationKind("bad_location"), "container-1"); !errors.Is(err, ErrInvalidLocationKind) {
		t.Fatalf("invalid kind error = %v, want ErrInvalidLocationKind", err)
	}
	if _, err := NewItemLocation(LocationKindShipCargo, " "); !errors.Is(err, ErrEmptyLocationID) {
		t.Fatalf("blank id error = %v, want ErrEmptyLocationID", err)
	}
	if err := (ItemLocation{}).Validate(); !errors.Is(err, ErrInvalidLocationKind) {
		t.Fatalf("zero location Validate() = %v, want ErrInvalidLocationKind", err)
	}
}

func TestItemLocationJSONAndStringBehaviorIsStable(t *testing.T) {
	location, err := NewItemLocation(LocationKindShipCargo, "ship-123")
	if err != nil {
		t.Fatalf("NewItemLocation valid value: %v", err)
	}

	if got := location.String(); got != "ship_cargo:ship-123" {
		t.Fatalf("String() = %q, want ship_cargo:ship-123", got)
	}

	payload, err := json.Marshal(location)
	if err != nil {
		t.Fatalf("json marshal location: %v", err)
	}
	want := `{"location_type":"ship_cargo","location_id":"ship-123"}`
	if got := string(payload); got != want {
		t.Fatalf("location JSON = %s, want %s", got, want)
	}
}

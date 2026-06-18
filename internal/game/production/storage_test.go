package production

import (
	"errors"
	"testing"

	"gameproject/internal/game/foundation"
)

func TestPlanetStorageCapacityClampAndRemoval(t *testing.T) {
	storage, err := NewPlanetStorage("planet-1", 10, []StoredItem{
		{ItemID: "crystal_fragment", Quantity: 4},
	}, testTime(0))
	if err != nil {
		t.Fatalf("NewPlanetStorage() error = %v, want nil", err)
	}

	added, err := storage.AddUpToCapacity("iron_ore", 9, testTime(1))
	if err != nil {
		t.Fatalf("AddUpToCapacity() error = %v, want nil", err)
	}
	if added != 6 {
		t.Fatalf("AddUpToCapacity() added = %d, want 6", added)
	}
	if got := storage.UsedUnits(); got != 10 {
		t.Fatalf("UsedUnits() = %d, want 10", got)
	}
	if got := storage.FreeUnits(); got != 0 {
		t.Fatalf("FreeUnits() = %d, want 0", got)
	}
	if got := storage.QuantityOf("iron_ore"); got != 6 {
		t.Fatalf("QuantityOf(iron_ore) = %d, want 6", got)
	}

	removed, err := storage.RemoveUpTo("crystal_fragment", 10, testTime(2))
	if err != nil {
		t.Fatalf("RemoveUpTo() error = %v, want nil", err)
	}
	if removed != 4 {
		t.Fatalf("RemoveUpTo() removed = %d, want 4", removed)
	}
	if got := storage.QuantityOf("crystal_fragment"); got != 0 {
		t.Fatalf("QuantityOf(crystal_fragment) = %d, want 0", got)
	}
}

func TestPlanetStorageValidationRejectsDuplicateAndOverCapacity(t *testing.T) {
	_, err := NewPlanetStorage("planet-1", 10, []StoredItem{
		{ItemID: "iron_ore", Quantity: 1},
		{ItemID: "iron_ore", Quantity: 1},
	}, testTime(0))
	if !errors.Is(err, ErrDuplicateStorageItem) {
		t.Fatalf("duplicate storage error = %v, want ErrDuplicateStorageItem", err)
	}

	_, err = NewPlanetStorage("planet-1", 2, []StoredItem{
		{ItemID: "iron_ore", Quantity: 3},
	}, testTime(0))
	if !errors.Is(err, ErrStorageOverCapacity) {
		t.Fatalf("over capacity error = %v, want ErrStorageOverCapacity", err)
	}
}

func TestPlanetStorageSnapshotIsDetachedAndSorted(t *testing.T) {
	storage, err := NewPlanetStorage("planet-1", 10, []StoredItem{
		{ItemID: "void_salt", Quantity: 1},
		{ItemID: "iron_ore", Quantity: 2},
	}, testTime(0))
	if err != nil {
		t.Fatalf("NewPlanetStorage() error = %v, want nil", err)
	}

	snapshot, err := storage.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot() error = %v, want nil", err)
	}
	if snapshot.Items[0].ItemID != foundation.ItemID("iron_ore") {
		t.Fatalf("snapshot first item = %q, want iron_ore", snapshot.Items[0].ItemID)
	}
	snapshot.Items[0].Quantity = 99
	if got := storage.QuantityOf("iron_ore"); got != 2 {
		t.Fatalf("mutating snapshot changed storage quantity = %d, want 2", got)
	}
}

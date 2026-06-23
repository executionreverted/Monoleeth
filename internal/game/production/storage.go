package production

import (
	"fmt"
	"sort"
	"time"

	"gameproject/internal/game/foundation"
)

// StoredItem records one stack of material in planet-local storage.
type StoredItem struct {
	ItemID   foundation.ItemID `json:"item_id"`
	Quantity int64             `json:"quantity"`
}

// PlanetStorage is the server-owned aggregate for one planet's local storage.
// MVP capacity is whole-unit based: one item quantity consumes one storage unit.
type PlanetStorage struct {
	PlanetID      foundation.PlanetID `json:"planet_id"`
	CapacityUnits int64               `json:"capacity_units"`
	Items         []StoredItem        `json:"items,omitempty"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// NewPlanetStorage validates and returns a storage aggregate.
func NewPlanetStorage(
	planetID foundation.PlanetID,
	capacityUnits int64,
	items []StoredItem,
	updatedAt time.Time,
) (PlanetStorage, error) {
	storage := PlanetStorage{
		PlanetID:      planetID,
		CapacityUnits: capacityUnits,
		Items:         cloneStoredItems(items),
		UpdatedAt:     updatedAt,
	}
	if err := storage.Validate(); err != nil {
		return PlanetStorage{}, err
	}
	return clonePlanetStorage(storage), nil
}

// Validate reports whether storage has valid identity, capacity, and item rows.
func (storage PlanetStorage) Validate() error {
	if err := storage.PlanetID.Validate(); err != nil {
		return err
	}
	if err := validatePositiveBoundedAmount("storage capacity", storage.CapacityUnits, ErrInvalidStorageCapacity); err != nil {
		return err
	}
	if storage.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at: %w", ErrZeroProductionTimestamp)
	}
	seen := make(map[foundation.ItemID]struct{}, len(storage.Items))
	for _, item := range storage.Items {
		if err := item.Validate(); err != nil {
			return err
		}
		if _, ok := seen[item.ItemID]; ok {
			return fmt.Errorf("item %q: %w", item.ItemID, ErrDuplicateStorageItem)
		}
		seen[item.ItemID] = struct{}{}
	}
	if used := storage.UsedUnits(); used > storage.CapacityUnits {
		return fmt.Errorf("used %d capacity %d: %w", used, storage.CapacityUnits, ErrStorageOverCapacity)
	}
	return nil
}

// Validate reports whether the storage row names an item and positive quantity.
func (item StoredItem) Validate() error {
	if err := item.ItemID.Validate(); err != nil {
		return err
	}
	return validatePositiveBoundedAmount("stored quantity", item.Quantity, foundation.ErrNonPositiveAmount)
}

// UsedUnits returns the whole-unit storage currently occupied.
func (storage PlanetStorage) UsedUnits() int64 {
	var used int64
	for _, item := range storage.Items {
		used += item.Quantity
	}
	return used
}

// FreeUnits returns the whole-unit storage available for new output.
func (storage PlanetStorage) FreeUnits() int64 {
	free := storage.CapacityUnits - storage.UsedUnits()
	if free < 0 {
		return 0
	}
	return free
}

// QuantityOf returns the stored quantity for itemID.
func (storage PlanetStorage) QuantityOf(itemID foundation.ItemID) int64 {
	for _, item := range storage.Items {
		if item.ItemID == itemID {
			return item.Quantity
		}
	}
	return 0
}

// AddUpToCapacity adds as much of quantity as fits and returns the amount added.
func (storage *PlanetStorage) AddUpToCapacity(itemID foundation.ItemID, quantity int64, updatedAt time.Time) (int64, error) {
	if err := itemID.Validate(); err != nil {
		return 0, err
	}
	if err := foundation.ValidatePositiveAmount(quantity); err != nil {
		return 0, err
	}
	if updatedAt.IsZero() {
		return 0, fmt.Errorf("updated_at: %w", ErrZeroProductionTimestamp)
	}
	if err := storage.Validate(); err != nil {
		return 0, err
	}

	added := minInt64(quantity, storage.FreeUnits())
	if added == 0 {
		return 0, nil
	}
	for i := range storage.Items {
		if storage.Items[i].ItemID == itemID {
			storage.Items[i].Quantity += added
			storage.UpdatedAt = updatedAt.UTC()
			return added, storage.Validate()
		}
	}
	storage.Items = append(storage.Items, StoredItem{ItemID: itemID, Quantity: added})
	sortStoredItems(storage.Items)
	storage.UpdatedAt = updatedAt.UTC()
	return added, storage.Validate()
}

// RemoveUpTo removes as much of quantity as exists and returns the amount removed.
func (storage *PlanetStorage) RemoveUpTo(itemID foundation.ItemID, quantity int64, updatedAt time.Time) (int64, error) {
	if err := itemID.Validate(); err != nil {
		return 0, err
	}
	if err := foundation.ValidatePositiveAmount(quantity); err != nil {
		return 0, err
	}
	if updatedAt.IsZero() {
		return 0, fmt.Errorf("updated_at: %w", ErrZeroProductionTimestamp)
	}
	if err := storage.Validate(); err != nil {
		return 0, err
	}

	for i := range storage.Items {
		if storage.Items[i].ItemID != itemID {
			continue
		}
		removed := minInt64(quantity, storage.Items[i].Quantity)
		storage.Items[i].Quantity -= removed
		if storage.Items[i].Quantity == 0 {
			storage.Items = append(storage.Items[:i], storage.Items[i+1:]...)
		}
		storage.UpdatedAt = updatedAt.UTC()
		return removed, storage.Validate()
	}
	return 0, nil
}

// Clone returns a deep copy of storage.
func (storage PlanetStorage) Clone() PlanetStorage {
	return clonePlanetStorage(storage)
}

// Snapshot returns a validated detached copy suitable for later API shaping.
func (storage PlanetStorage) Snapshot() (PlanetStorage, error) {
	if err := storage.Validate(); err != nil {
		return PlanetStorage{}, err
	}
	return clonePlanetStorage(storage), nil
}

func clonePlanetStorage(storage PlanetStorage) PlanetStorage {
	storage.Items = cloneStoredItems(storage.Items)
	storage.UpdatedAt = storage.UpdatedAt.UTC()
	return storage
}

func clonePlanetStorageRows(rows []PlanetStorage) []PlanetStorage {
	if len(rows) == 0 {
		return nil
	}
	cloned := make([]PlanetStorage, 0, len(rows))
	for _, row := range rows {
		cloned = append(cloned, clonePlanetStorage(row))
	}
	sort.Slice(cloned, func(i, j int) bool {
		return cloned[i].PlanetID < cloned[j].PlanetID
	})
	return cloned
}

func cloneStoredItems(items []StoredItem) []StoredItem {
	if len(items) == 0 {
		return nil
	}
	cloned := append([]StoredItem(nil), items...)
	sortStoredItems(cloned)
	return cloned
}

func sortStoredItems(items []StoredItem) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].ItemID < items[j].ItemID
	})
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

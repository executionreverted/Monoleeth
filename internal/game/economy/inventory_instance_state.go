package economy

import (
	"fmt"

	"gameproject/internal/game/foundation"
)

// SystemSetInstanceDurability initializes or repairs server-owned instance
// durability state for trusted runtime flows.
func (service *InventoryService) SystemSetInstanceDurability(playerID foundation.PlayerID, itemInstanceID foundation.ItemID, durability int64) (InstanceItem, error) {
	if err := playerID.Validate(); err != nil {
		return InstanceItem{}, err
	}
	if err := itemInstanceID.Validate(); err != nil {
		return InstanceItem{}, err
	}
	if err := foundation.ValidatePositiveAmount(durability); err != nil {
		return InstanceItem{}, fmt.Errorf("durability %d: %w", durability, err)
	}

	service.mu.Lock()
	defer service.mu.Unlock()

	for index, item := range service.instanceItems {
		if item.ItemInstanceID != itemInstanceID {
			continue
		}
		if item.OwnerPlayerID != playerID {
			return InstanceItem{}, ErrItemNotOwned
		}
		item.DurabilityCurrent = durability
		item.UpdatedAt = service.clock.Now()
		if err := service.persistInstanceItemLocked(item); err != nil {
			return InstanceItem{}, err
		}
		service.instanceItems[index] = item
		return item, nil
	}
	return InstanceItem{}, ErrItemNotOwned
}

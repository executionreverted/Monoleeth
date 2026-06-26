package server

import (
	"context"
	"errors"

	"gameproject/internal/game/admin"
	"gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
)

// ActiveEquippedModules enumerates the module definitions currently equipped
// across connected players' active ships. It is the runtime-backed
// active-reference reader consulted by publish safety so a content change to a
// module a pilot holds live is blocked until the loadout is quiesced.
func (runtime *Runtime) ActiveEquippedModules(ctx context.Context) ([]admin.EquippedModuleReference, error) {
	if runtime == nil || runtime.LoadoutStore == nil {
		return nil, nil
	}
	playerIDs := runtime.knownPlayerIDs()
	refs := make([]admin.EquippedModuleReference, 0)
	for _, playerID := range playerIDs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		shipID, err := runtime.LoadoutStore.ActiveShipID(playerID)
		if err != nil {
			if errors.Is(err, modules.ErrActiveShipNotFound) {
				continue
			}
			return nil, err
		}
		equipped, err := runtime.LoadoutStore.EquippedModules(playerID, shipID)
		if err != nil {
			return nil, err
		}
		for _, equippedModule := range equipped {
			item, err := runtime.LoadoutStore.ModuleItem(equippedModule.ItemInstanceID)
			if err != nil {
				if errors.Is(err, modules.ErrUnknownModuleItem) {
					continue
				}
				return nil, err
			}
			refs = append(refs, admin.EquippedModuleReference{
				ModuleID: content.ContentID(string(item.ItemID)),
				PlayerID: string(playerID),
				ShipID:   string(shipID),
			})
		}
	}
	return refs, nil
}

func (runtime *Runtime) knownPlayerIDs() []foundation.PlayerID {
	if runtime == nil {
		return nil
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	playerIDs := make([]foundation.PlayerID, 0, len(runtime.players))
	for playerID := range runtime.players {
		playerIDs = append(playerIDs, playerID)
	}
	return playerIDs
}

package server

import (
	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

const (
	trainingDroneSalvageLootTableID = gamecontent.TrainingDroneSalvageLootTableID
	borderRaiderSalvageLootTableID  = gamecontent.BorderRaiderSalvageLootTableID
	coordinateScrollItemID          = gamecontent.CoordinateScrollItemID
)

func runtimeStackableDefinition(itemID foundation.ItemID, name string) (economy.ItemDefinition, error) {
	return gamecontent.StackableItemDefinition(itemID, name)
}

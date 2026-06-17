package quests

import "gameproject/internal/game/foundation"

// ConsumeCombatNPCKilled consumes a validated combat.npc_killed server event
// and progresses matching active kill quests for the event player.
func (service *QuestService) ConsumeCombatNPCKilled(input CombatNPCKilledInput) ([]PlayerQuest, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	return service.consumeProgressEvent(input.EventID, input.PlayerID, func(objective Objective) (int64, bool) {
		if objective.Kind != ObjectiveKindKill || objective.Kill == nil {
			return 0, false
		}
		if objective.Kill.TargetNPCType != input.NPCType {
			return 0, false
		}
		return 1, true
	})
}

// ConsumeLootPickedUp consumes a validated loot.picked_up server event and
// progresses matching active collect quests for the event player.
func (service *QuestService) ConsumeLootPickedUp(input LootPickedUpInput) ([]PlayerQuest, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	return service.consumeProgressEvent(input.EventID, input.PlayerID, func(objective Objective) (int64, bool) {
		if objective.Kind != ObjectiveKindCollect || objective.Collect == nil {
			return 0, false
		}
		if objective.Collect.ItemID != input.ItemID {
			return 0, false
		}
		return input.Quantity.Int64(), true
	})
}

// ConsumeCraftJobCompleted consumes a validated craft.job_completed server
// event and progresses matching active craft quests for the event player.
func (service *QuestService) ConsumeCraftJobCompleted(input CraftJobCompletedInput) ([]PlayerQuest, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	return service.consumeProgressEvent(input.EventID, input.PlayerID, func(objective Objective) (int64, bool) {
		if objective.Kind != ObjectiveKindCraft || objective.Craft == nil {
			return 0, false
		}
		if !objective.Craft.RecipeID.IsZero() && objective.Craft.RecipeID != input.RecipeID {
			return 0, false
		}
		if !objective.Craft.ItemID.IsZero() && objective.Craft.ItemID != input.ItemID {
			return 0, false
		}
		return input.Quantity.Int64(), true
	})
}

// ConsumeScanCompleted validates the server scanner event shape and no-ops
// until the authoritative scanner provider is available.
func (service *QuestService) ConsumeScanCompleted(input ScanCompletedInput) ([]PlayerQuest, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	return nil, nil
}

// ConsumeBuildingCompleted validates the server building event shape and no-ops
// until authoritative building completion exists.
func (service *QuestService) ConsumeBuildingCompleted(input BuildingCompletedInput) ([]PlayerQuest, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	return nil, nil
}

// ConsumeDeliveryCompleted validates the server delivery event shape and no-ops
// until authoritative delivery settlement exists.
func (service *QuestService) ConsumeDeliveryCompleted(input DeliveryCompletedInput) ([]PlayerQuest, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	return nil, nil
}

func (service *QuestService) consumeProgressEvent(
	eventID foundation.EventID,
	playerID foundation.PlayerID,
	matcher objectiveProgressMatcher,
) ([]PlayerQuest, error) {
	return service.store.ApplyProgressEvent(eventID, playerID, service.clock.Now().UTC(), matcher)
}

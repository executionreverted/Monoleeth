package quests

import (
	"encoding/json"
	"fmt"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
)

// ConsumeDomainEvent routes one server-owned domain event into the matching
// quest progress consumer. Unknown event types are ignored.
func (service *QuestService) ConsumeDomainEvent(envelope events.EventEnvelope) ([]PlayerQuest, error) {
	switch envelope.Type {
	case combat.EventNPCKilled:
		payload, err := decodeDomainEventPayload[combat.NPCKilledEvent](envelope)
		if err != nil {
			return nil, err
		}
		eventKey, err := combatQuestProgressEventKey(payload)
		if err != nil {
			return nil, err
		}
		return service.ConsumeCombatNPCKilled(CombatNPCKilledInput{
			EventID:          envelope.EventID,
			ProgressEventKey: eventKey,
			PlayerID:         payload.OwnerPlayerID,
			NPCType:          payload.NPCType,
		})
	case loot.EventLootPickedUp:
		payload, err := decodeDomainEventPayload[loot.PickedUpPayload](envelope)
		if err != nil {
			return nil, err
		}
		eventKey, err := lootQuestProgressEventKey(payload)
		if err != nil {
			return nil, err
		}
		quantity, err := foundation.NewQuantity(payload.Quantity)
		if err != nil {
			return nil, fmt.Errorf("loot quantity: %w", err)
		}
		return service.ConsumeLootPickedUp(LootPickedUpInput{
			EventID:          envelope.EventID,
			ProgressEventKey: eventKey,
			PlayerID:         payload.PlayerID,
			ItemID:           payload.ItemID,
			Quantity:         quantity,
		})
	case crafting.EventCraftJobCompleted:
		payload, err := decodeDomainEventPayload[crafting.JobCompletedEvent](envelope)
		if err != nil {
			return nil, err
		}
		eventKey, err := craftQuestProgressEventKey(payload)
		if err != nil {
			return nil, err
		}
		quantity, err := foundation.NewQuantity(payload.Quantity)
		if err != nil {
			return nil, fmt.Errorf("craft quantity: %w", err)
		}
		return service.ConsumeCraftJobCompleted(CraftJobCompletedInput{
			EventID:          envelope.EventID,
			ProgressEventKey: eventKey,
			PlayerID:         payload.PlayerID,
			RecipeID:         payload.RecipeID,
			ItemID:           payload.ItemID,
			Quantity:         quantity,
		})
	default:
		return nil, nil
	}
}

func combatQuestProgressEventKey(payload combat.NPCKilledEvent) (QuestProgressEventKey, error) {
	if err := payload.NPCEntityID.Validate(); err != nil {
		return "", fmt.Errorf("combat npc_entity_id: %w", err)
	}
	return QuestProgressEventKey(fmt.Sprintf("%s:%s", combat.EventNPCKilled, payload.NPCEntityID)), nil
}

func lootQuestProgressEventKey(payload loot.PickedUpPayload) (QuestProgressEventKey, error) {
	if err := payload.DropID.Validate(); err != nil {
		return "", fmt.Errorf("loot drop_id: %w", err)
	}
	return QuestProgressEventKey(fmt.Sprintf("%s:%s", loot.EventLootPickedUp, payload.DropID)), nil
}

func craftQuestProgressEventKey(payload crafting.JobCompletedEvent) (QuestProgressEventKey, error) {
	if err := payload.JobID.Validate(); err != nil {
		return "", fmt.Errorf("craft job_id: %w", err)
	}
	return QuestProgressEventKey(fmt.Sprintf("%s:%s", crafting.EventCraftJobCompleted, payload.JobID)), nil
}

func decodeDomainEventPayload[T any](envelope events.EventEnvelope) (T, error) {
	var payload T
	if err := envelope.EventID.Validate(); err != nil {
		return payload, fmt.Errorf("event_id: %w", err)
	}
	if len(envelope.Payload) == 0 {
		return payload, fmt.Errorf("event %q payload: %w", envelope.Type, ErrInvalidQuestEvent)
	}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return payload, fmt.Errorf("event %q payload: %w", envelope.Type, err)
	}
	return payload, nil
}

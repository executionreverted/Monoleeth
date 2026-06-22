package worker

import (
	"fmt"

	"gameproject/internal/game/foundation"
)

func (worker *Worker) setPlayerAggroEligibility(playerID foundation.PlayerID, eligible bool) error {
	if err := playerID.Validate(); err != nil {
		return err
	}
	if _, ok := worker.playerEntities[playerID]; !ok {
		return fmt.Errorf("player %q: %w", playerID, ErrUnknownPlayer)
	}
	if eligible {
		delete(worker.playerAggroIneligible, playerID)
		return nil
	}
	worker.playerAggroIneligible[playerID] = true
	worker.clearEnemyAggroTargetsForPlayer(playerID)
	return nil
}

func (worker *Worker) clearEnemyAggroTargetsForPlayer(playerID foundation.PlayerID) {
	entityID, ok := worker.playerEntities[playerID]
	if !ok || worker.enemySpawner == nil {
		return
	}
	for index := range worker.enemySpawner.rows {
		record := worker.enemySpawner.rows[index]
		if record.AggroTargetEntityID != entityID {
			continue
		}
		clearEnemyTargetMemory(&record)
		worker.enemySpawner.rows[index] = record
		if entity, ok := worker.entities[record.EntityID]; ok {
			worker.stopEnemyMovement(entity)
		}
	}
}

package server

import (
	"fmt"
	"math"
	"time"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/visibility"
	"gameproject/internal/game/world/worker"
)

func (runtime *Runtime) upsertNPCCombatActorProjectionLocked(instance *mapInstance, entity world.Entity) (combat.ActorState, error) {
	actor, err := runtime.projectNPCCombatActorLocked(instance, entity)
	if err != nil {
		return combat.ActorState{}, err
	}
	if err := runtime.Combat.UpsertActor(actor); err != nil {
		return combat.ActorState{}, err
	}
	return actor, nil
}

func (runtime *Runtime) projectNPCCombatActorLocked(instance *mapInstance, entity world.Entity) (combat.ActorState, error) {
	if instance == nil || instance.Worker == nil {
		return combat.ActorState{}, worker.ErrUnknownEntity
	}
	if entity.Type != world.EntityTypeNPC {
		return combat.ActorState{}, foundation.NewDomainError(foundation.CodeInvalidPayload, "Target is not a hostile entity.")
	}
	record, template, err := runtime.npcSpawnRecordAndTemplateLocked(instance, entity.ID)
	if err != nil {
		return combat.ActorState{}, err
	}
	existing, hasExisting := runtime.Combat.Actor(entity.ID)
	hidden := instance.HiddenEntities[entity.ID]
	if hasExisting && shouldResetNPCCombatStateForSpawn(record, existing) {
		delete(instance.HiddenEntities, entity.ID)
		hidden = false
	}
	signature, stealthScore, jammerStrength := npcTemplateVisibilityInputs(template, hidden)
	actor := combat.ActorState{
		EntityID:       entity.ID,
		Type:           world.EntityTypeNPC,
		NPCType:        record.NPCType,
		WorldID:        entity.WorldID,
		ZoneID:         entity.ZoneID,
		Position:       entity.Position,
		Signature:      signature,
		StealthScore:   stealthScore,
		JammerStrength: jammerStrength,
		Hidden:         hidden,
		Stats:          npcStatSnapshot(template, runtime.clock.Now()),
		HP:             template.HPMax,
		Shield:         template.ShieldMax,
		Energy:         template.EnergyMax,
		Cooldowns:      combat.CooldownState{},
		Contributions:  make(map[foundation.PlayerID]float64),
	}
	if hasExisting && shouldPreserveNPCCombatState(record, existing) {
		actor.HP = existing.HP
		actor.Shield = existing.Shield
		actor.Energy = existing.Energy
		actor.Dead = existing.Dead
		actor.DiedAt = existing.DiedAt
		actor.Cooldowns = existing.Cooldowns
		actor.Contributions = existing.Contributions
	}
	return actor, nil
}

func (runtime *Runtime) syncAliveNPCCombatActorProjectionsLocked(instance *mapInstance) error {
	if instance == nil || instance.Worker == nil {
		return nil
	}
	for _, record := range instance.Worker.EnemySpawnSnapshot().Records {
		if !record.Alive {
			continue
		}
		entity, ok := instance.Worker.Entity(record.EntityID)
		if !ok {
			continue
		}
		if _, err := runtime.upsertNPCCombatActorProjectionLocked(instance, entity); err != nil {
			return err
		}
	}
	return nil
}

func shouldResetNPCCombatStateForSpawn(record worker.EnemySpawnRecord, existing combat.ActorState) bool {
	if !record.Alive {
		return false
	}
	if existing.Dead || existing.HP <= 0 {
		return true
	}
	return existing.DiedAt != nil && !record.SpawnedAt.IsZero() && !record.SpawnedAt.Before(*existing.DiedAt)
}

func shouldPreserveNPCCombatState(record worker.EnemySpawnRecord, existing combat.ActorState) bool {
	return !shouldResetNPCCombatStateForSpawn(record, existing)
}

func (runtime *Runtime) npcSpawnRecordAndTemplateLocked(instance *mapInstance, entityID world.EntityID) (worker.EnemySpawnRecord, worldmaps.NPCStatTemplate, error) {
	if instance == nil || instance.Worker == nil {
		return worker.EnemySpawnRecord{}, worldmaps.NPCStatTemplate{}, worker.ErrUnknownEntity
	}
	record, ok := instance.Worker.EnemySpawnRecord(entityID)
	if !ok || !record.Alive {
		return worker.EnemySpawnRecord{}, worldmaps.NPCStatTemplate{}, worker.ErrUnknownEntity
	}
	for _, template := range instance.Definition.NPCStatTemplates {
		if template.StatTemplateID != record.StatTemplateID {
			continue
		}
		if template.NPCType != record.NPCType {
			return worker.EnemySpawnRecord{}, worldmaps.NPCStatTemplate{}, fmt.Errorf("npc stat template %q npc type %q: %w", record.StatTemplateID, template.NPCType, worker.ErrUnknownEntity)
		}
		return record, template, nil
	}
	return worker.EnemySpawnRecord{}, worldmaps.NPCStatTemplate{}, fmt.Errorf("npc stat template %q: %w", record.StatTemplateID, worker.ErrUnknownEntity)
}

func npcStatSnapshot(template worldmaps.NPCStatTemplate, nowTime time.Time) stats.StatSnapshot {
	return stats.NewStatSnapshot("", foundation.ShipID(template.StatTemplateID), 1, stats.EffectiveStats{
		Core: stats.CoreStats{
			HPMax:     template.HPMax,
			ShieldMax: template.ShieldMax,
			EnergyMax: template.EnergyMax,
			Speed:     template.Speed,
		},
		Combat: stats.CombatStats{
			WeaponDamage:   template.WeaponDamage,
			WeaponRange:    template.WeaponRange,
			WeaponCooldown: template.WeaponCooldown.Seconds(),
			Accuracy:       template.Accuracy,
		},
		Exploration: stats.ExplorationStats{
			RadarRange:      template.WeaponRange,
			SignatureRadius: template.RadarSignature,
		},
	}, nowTime)
}

func npcTemplateSignature(template worldmaps.NPCStatTemplate) visibility.EntitySignature {
	if template.RadarSignature > 0 && !math.IsNaN(template.RadarSignature) && !math.IsInf(template.RadarSignature, 0) {
		return visibility.EntitySignature(template.RadarSignature)
	}
	return visibility.SignatureForEntityType(world.EntityTypeNPC)
}

func npcTemplateVisibilityInputs(template worldmaps.NPCStatTemplate, hidden bool) (visibility.EntitySignature, float64, float64) {
	signature := npcTemplateSignature(template)
	stealthScore := 0.0
	if hidden {
		stealthScore = stealthScoreForHiddenEntity(world.EntityTypeNPC, signature)
	}
	return signature, stealthScore, 0
}

func (runtime *Runtime) npcVisibilityInputsLocked(instance *mapInstance, entity world.Entity, hidden bool) (visibility.EntitySignature, float64, float64, bool) {
	_, template, err := runtime.npcSpawnRecordAndTemplateLocked(instance, entity.ID)
	if err != nil {
		return 0, 0, 0, false
	}
	signature, stealthScore, jammerStrength := npcTemplateVisibilityInputs(template, hidden)
	return signature, stealthScore, jammerStrength, true
}

func (runtime *Runtime) publicNPCMetadataLocked(instance *mapInstance, entity world.Entity) ([]aoi.StatusFlag, *aoi.EntityDisplay, *aoi.EntityCombatStatus) {
	flags := []aoi.StatusFlag{"hostile"}
	display := &aoi.EntityDisplay{Label: "NPC", Disposition: "hostile"}
	record, template, err := runtime.npcSpawnRecordAndTemplateLocked(instance, entity.ID)
	if err != nil {
		return flags, display, nil
	}
	display.Label = displayLabelForNPCTemplate(record.NPCType, template)
	combatStatus := runtime.entityCombatStatusLocked(entity.ID)
	actor, err := runtime.upsertNPCCombatActorProjectionLocked(instance, entity)
	if err == nil {
		combatStatus = combatStatusFromActor(actor)
	}
	if combatStatus != nil && combatStatus.HP < combatStatus.MaxHP {
		flags = append(flags, "damaged")
	}
	return flags, display, combatStatus
}

func displayLabelForNPCTemplate(npcType string, template worldmaps.NPCStatTemplate) string {
	switch template.LabelKey {
	case "npc.lordakia":
		return "Lordakia"
	case "npc.mordon":
		return "Mordon"
	case "npc.streuner":
		return "Streuner"
	case "npc.saimon":
		return "Saimon"
	}
	switch npcType {
	case "":
		return "NPC"
	default:
		return npcType
	}
}

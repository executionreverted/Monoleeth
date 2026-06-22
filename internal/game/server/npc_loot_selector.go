package server

import (
	"errors"
	"strings"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

var (
	errNPCLootMapUnavailable     = errors.New("npc loot map unavailable")
	errNPCLootSpawnUnavailable   = errors.New("npc loot spawn unavailable")
	errNPCLootProfileUnavailable = errors.New("npc loot profile unavailable")
	errNPCLootProfileMismatch    = errors.New("npc loot profile mismatch")
	errNPCLootTableUnavailable   = errors.New("npc loot table unavailable")
)

func (runtime *Runtime) selectNPCKillLootTableLocked(playerID foundation.PlayerID, event combat.NPCKilledEvent) (loot.LootTable, error) {
	instance, _, err := runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return loot.LootTable{}, err
	}
	return runtime.selectNPCKillLootTableForInstanceLocked(instance, event)
}

func (runtime *Runtime) selectNPCKillLootTableForInstanceLocked(instance *mapInstance, event combat.NPCKilledEvent) (loot.LootTable, error) {
	if runtime == nil || instance == nil || instance.Worker == nil {
		return loot.LootTable{}, errNPCLootMapUnavailable
	}
	if event.WorldID != instance.Definition.WorldID || event.ZoneID != instance.Definition.ZoneID {
		return loot.LootTable{}, errNPCLootMapUnavailable
	}

	record, ok := instance.Worker.EnemySpawnRecord(event.NPCEntityID)
	if !ok {
		return loot.LootTable{}, errNPCLootSpawnUnavailable
	}
	if !npcLootRecordMatchesKill(record, event) {
		return loot.LootTable{}, errNPCLootProfileMismatch
	}

	profile, ok := npcDropProfileByID(instance.Definition, record.DropProfileID)
	if !ok {
		return loot.LootTable{}, errNPCLootProfileUnavailable
	}
	if !npcDropProfileCompatible(instance.Definition, record, event, profile) {
		return loot.LootTable{}, errNPCLootProfileMismatch
	}

	tableID := strings.TrimSpace(profile.LootTableID)
	table, ok := runtime.lootTables[tableID]
	if !ok || table.Source.DefinitionID.String() != tableID {
		return loot.LootTable{}, errNPCLootTableUnavailable
	}
	return table, nil
}

func npcLootRecordMatchesKill(record worker.EnemySpawnRecord, event combat.NPCKilledEvent) bool {
	return strings.TrimSpace(record.NPCType) != "" &&
		record.NPCType == event.NPCType &&
		record.EntityID == event.NPCEntityID
}

func npcDropProfileByID(definition worldmaps.MapDefinition, profileID worldmaps.NPCDropProfileID) (worldmaps.NPCDropProfile, bool) {
	for _, profile := range definition.NPCDropProfiles {
		if profile.DropProfileID == profileID {
			return profile, true
		}
	}
	return worldmaps.NPCDropProfile{}, false
}

func npcDropProfileCompatible(
	definition worldmaps.MapDefinition,
	record worker.EnemySpawnRecord,
	event combat.NPCKilledEvent,
	profile worldmaps.NPCDropProfile,
) bool {
	if profile.NPCType != record.NPCType || profile.NPCType != event.NPCType {
		return false
	}
	if record.Level < profile.MinLevel || record.Level > profile.MaxLevel {
		return false
	}
	return profile.RiskBand == definition.RiskBand
}

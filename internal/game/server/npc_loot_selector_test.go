package server

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/observability"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

func TestNPCLootSelectorUsesSpawnRecordDropProfileLootTable(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSession(t, gameServer, "selector-table@example.com", "Selector Table")

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		t.Fatalf("starter map instance: %v", err)
	}
	record, ok := starter.Worker.EnemySpawnRecord("entity_training_npc")
	if !ok {
		t.Fatalf("starter spawner missing entity_training_npc; snapshot=%+v", starter.Worker.EnemySpawnSnapshot())
	}

	selectorTableID := "selector_test_salvage"
	selectorItemID := foundation.ItemID("selector_ore")
	selectorTable := testRuntimeLootTable(t, selectorTableID, selectorItemID, "Selector Ore", 7)
	gameServer.runtime.lootTables[selectorTableID] = selectorTable
	gameServer.runtime.itemCatalog[selectorItemID] = selectorTable.Rows[0].ItemDefinition
	starter.Definition.NPCDropProfiles[0].LootTableID = selectorTableID

	event := testNPCKilledEventForRecord(resolved.PlayerID, starter, record)
	if err := gameServer.runtime.submitWorkerCommandAndRecordMetricsLocked(starter, worker.MarkEnemyKilledCommand{
		Definition:  starter.Definition,
		NPCEntityID: event.NPCEntityID,
		KilledAt:    event.KilledAt,
	}); err != nil {
		t.Fatalf("MarkEnemyKilledCommand() error = %v, want nil", err)
	}

	selected, err := gameServer.runtime.selectNPCKillLootTableLocked(resolved.PlayerID, event)
	if err != nil {
		t.Fatalf("selectNPCKillLootTableLocked() error = %v, want nil", err)
	}
	if got := selected.Source.DefinitionID.String(); got != selectorTableID {
		t.Fatalf("selected loot table id = %q, want %q", got, selectorTableID)
	}
	requireMetricCounter(t, gameServer.runtime.Metrics.Snapshot(), observability.MetricNPCLootSelectorDecisions, 1, []observability.Label{
		{Name: "map_key", Value: "1-1"},
		{Name: "npc_type", Value: "training_drone"},
		{Name: "reason", Value: "selected"},
		{Name: "result", Value: "accepted"},
		{Name: "risk_band", Value: "low"},
		{Name: "stage", Value: "loot_table"},
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "map_1_1"},
	})
	created, err := gameServer.runtime.Loot.CreateDropsForNPCKill(event, selected)
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill() error = %v, want nil", err)
	}
	if len(created.Drops) != 1 ||
		created.Drops[0].ItemDefinition.ItemID != selectorItemID ||
		created.Drops[0].Quantity != 7 {
		t.Fatalf("created drops = %+v, want selector_ore x7", created.Drops)
	}

	rawPayload, err := json.Marshal(lootDropPayload(created.Drops[0], gameServer.runtime.clock.Now()))
	if err != nil {
		t.Fatalf("marshal loot payload: %v", err)
	}
	for _, forbidden := range []string{
		selectorTableID,
		trainingDroneSalvageLootTableID,
		"drop_profile",
		"loot_table",
	} {
		if strings.Contains(string(rawPayload), forbidden) {
			t.Fatalf("loot payload leaked %q in %s", forbidden, rawPayload)
		}
	}
}

func TestNPCLootSelectorUsesOuterRingSpawnRecordDropProfileLootTable(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "selector-map-two@example.com", "Selector Map Two", "map_1_2", "west_gate")

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	mapTwo, err := gameServer.runtime.mapInstanceLocked("map_1_2")
	if err != nil {
		t.Fatalf("map_1_2 instance: %v", err)
	}
	snapshot := mapTwo.Worker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 {
		t.Fatalf("map_1_2 spawn records = %+v, want one outer ring scout", snapshot.Records)
	}
	record := snapshot.Records[0]
	if record.NPCType != "outer_ring_scout_drone" ||
		record.DropProfileID != "outer_ring_scout_drone_salvage" ||
		record.Level != 1 ||
		!record.Alive {
		t.Fatalf("map_1_2 spawn record = %+v, want live outer ring scout drop profile", record)
	}
	profile, ok := npcDropProfileByID(mapTwo.Definition, record.DropProfileID)
	if !ok {
		t.Fatalf("map_1_2 drop profile %q missing", record.DropProfileID)
	}
	if profile.NPCType != "outer_ring_scout_drone" ||
		profile.RiskBand != "low" ||
		profile.LootTableID != trainingDroneSalvageLootTableID {
		t.Fatalf("map_1_2 drop profile = %+v, want explicit outer ring scout training salvage table", profile)
	}

	event := testNPCKilledEventForRecord(resolved.PlayerID, mapTwo, record)
	selected, err := gameServer.runtime.selectNPCKillLootTableLocked(resolved.PlayerID, event)
	if err != nil {
		t.Fatalf("selectNPCKillLootTableLocked(map_1_2) error = %v, want nil", err)
	}
	if got := selected.Source.DefinitionID.String(); got != trainingDroneSalvageLootTableID {
		t.Fatalf("selected loot table id = %q, want explicit profile table %q", got, trainingDroneSalvageLootTableID)
	}
	requireMetricCounter(t, gameServer.runtime.Metrics.Snapshot(), observability.MetricNPCLootSelectorDecisions, 1, []observability.Label{
		{Name: "map_key", Value: "1-2"},
		{Name: "npc_type", Value: "outer_ring_scout_drone"},
		{Name: "reason", Value: "selected"},
		{Name: "result", Value: "accepted"},
		{Name: "risk_band", Value: "low"},
		{Name: "stage", Value: "loot_table"},
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "map_1_2"},
	})

	created, err := gameServer.runtime.Loot.CreateDropsForNPCKill(event, selected)
	if err != nil {
		t.Fatalf("CreateDropsForNPCKill(map_1_2) error = %v, want nil", err)
	}
	if len(created.Drops) != 1 ||
		created.Drops[0].WorldID != mapTwo.Definition.WorldID ||
		created.Drops[0].ZoneID != mapTwo.Definition.ZoneID ||
		created.Drops[0].SourceID != record.EntityID ||
		created.Drops[0].ItemDefinition.ItemID != "raw_ore" ||
		created.Drops[0].Quantity != 3 {
		t.Fatalf("map_1_2 created drops = %+v, want raw_ore x3 in destination map", created.Drops)
	}
}

func TestNPCLootSelectorAcceptsSeededMapMatrixRows(t *testing.T) {
	for _, tc := range []struct {
		name             string
		mapID            worldmaps.MapID
		wantPublicMapKey string
		wantRiskBand     string
		wantNPCType      string
		wantLevel        int
		wantProfileID    worldmaps.NPCDropProfileID
		wantTableID      string
		wantItemID       foundation.ItemID
		wantQuantity     int64
	}{
		{
			name:             "starter training drone",
			mapID:            worldmaps.StarterMapID,
			wantPublicMapKey: "1-1",
			wantRiskBand:     "low",
			wantNPCType:      "training_drone",
			wantLevel:        1,
			wantProfileID:    "training_drone_salvage",
			wantTableID:      trainingDroneSalvageLootTableID,
			wantItemID:       "raw_ore",
			wantQuantity:     3,
		},
		{
			name:             "outer ring scout drone",
			mapID:            "map_1_2",
			wantPublicMapKey: "1-2",
			wantRiskBand:     "low",
			wantNPCType:      "outer_ring_scout_drone",
			wantLevel:        1,
			wantProfileID:    "outer_ring_scout_drone_salvage",
			wantTableID:      trainingDroneSalvageLootTableID,
			wantItemID:       "raw_ore",
			wantQuantity:     3,
		},
		{
			name:             "border skirmish raider drone",
			mapID:            "map_1_3",
			wantPublicMapKey: "1-3",
			wantRiskBand:     "medium",
			wantNPCType:      "border_raider_drone",
			wantLevel:        2,
			wantProfileID:    "border_raider_drone_salvage",
			wantTableID:      borderRaiderSalvageLootTableID,
			wantItemID:       "carbon_shards",
			wantQuantity:     2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gameServer, httpServer := newTestServer(t, false)
			defer httpServer.Close()
			resolved := createResolvedRuntimeSession(t, gameServer, "selector-matrix-accepted@example.com", "Selector Matrix Accepted")

			gameServer.runtime.mu.Lock()
			defer gameServer.runtime.mu.Unlock()

			instance, err := gameServer.runtime.mapInstanceLocked(tc.mapID)
			if err != nil {
				t.Fatalf("map instance %q: %v", tc.mapID, err)
			}
			if got := instance.Definition.PublicMapKey.String(); got != tc.wantPublicMapKey {
				t.Fatalf("public map key = %q, want %q", got, tc.wantPublicMapKey)
			}
			if got := instance.Definition.RiskBand; got != tc.wantRiskBand {
				t.Fatalf("risk band = %q, want %q", got, tc.wantRiskBand)
			}
			record := requireSpawnRecordByNPCType(t, instance, tc.wantNPCType)
			if record.NPCType != tc.wantNPCType ||
				record.Level != tc.wantLevel ||
				record.DropProfileID != tc.wantProfileID {
				t.Fatalf("spawn record = %+v, want npc_type=%q level=%d profile=%q", record, tc.wantNPCType, tc.wantLevel, tc.wantProfileID)
			}
			profile, ok := npcDropProfileByID(instance.Definition, record.DropProfileID)
			if !ok {
				t.Fatalf("drop profile %q missing", record.DropProfileID)
			}
			if profile.NPCType != tc.wantNPCType ||
				profile.MinLevel > record.Level ||
				profile.MaxLevel < record.Level ||
				profile.RiskBand != tc.wantRiskBand ||
				profile.LootTableID != tc.wantTableID {
				t.Fatalf("drop profile = %+v, want matrix npc_type=%q level=%d risk=%q table=%q", profile, tc.wantNPCType, record.Level, tc.wantRiskBand, tc.wantTableID)
			}

			event := testNPCKilledEventForRecord(resolved.PlayerID, instance, record)
			selected, err := gameServer.runtime.selectNPCKillLootTableForInstanceLocked(instance, event)
			if err != nil {
				t.Fatalf("selectNPCKillLootTableForInstanceLocked() error = %v, want nil", err)
			}
			if got := selected.Source.DefinitionID.String(); got != tc.wantTableID {
				t.Fatalf("selected loot table id = %q, want %q", got, tc.wantTableID)
			}
			created, err := gameServer.runtime.Loot.CreateDropsForNPCKill(event, selected)
			if err != nil {
				t.Fatalf("CreateDropsForNPCKill() error = %v, want nil", err)
			}
			if len(created.Drops) != 1 ||
				created.Drops[0].WorldID != instance.Definition.WorldID ||
				created.Drops[0].ZoneID != instance.Definition.ZoneID ||
				created.Drops[0].SourceID != record.EntityID ||
				created.Drops[0].ItemDefinition.ItemID != tc.wantItemID ||
				created.Drops[0].Quantity != tc.wantQuantity {
				t.Fatalf("created drops = %+v, want %s x%d in %s", created.Drops, tc.wantItemID, tc.wantQuantity, tc.wantPublicMapKey)
			}
		})
	}
}

func requireSpawnRecordByNPCType(t *testing.T, instance *mapInstance, npcType string) worker.EnemySpawnRecord {
	t.Helper()
	snapshot := instance.Worker.EnemySpawnSnapshot()
	for _, record := range snapshot.Records {
		if record.NPCType == npcType {
			return record
		}
	}
	t.Fatalf("spawn record with npc_type %q missing; snapshot=%+v", npcType, snapshot)
	return worker.EnemySpawnRecord{}
}

func TestNPCLootSelectorRejectsOuterRingMissingTableWithoutTrainingFallback(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	resolved := createResolvedRuntimeSessionOnMap(t, gameServer, "selector-map-two-missing@example.com", "Selector Map Two Missing", "map_1_2", "west_gate")

	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()

	mapTwo, err := gameServer.runtime.mapInstanceLocked("map_1_2")
	if err != nil {
		t.Fatalf("map_1_2 instance: %v", err)
	}
	snapshot := mapTwo.Worker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 1 {
		t.Fatalf("map_1_2 spawn records = %+v, want one outer ring scout", snapshot.Records)
	}
	record := snapshot.Records[0]
	if record.NPCType != "outer_ring_scout_drone" || record.DropProfileID != "outer_ring_scout_drone_salvage" {
		t.Fatalf("map_1_2 spawn record = %+v, want outer ring scout drop profile", record)
	}
	for index := range mapTwo.Definition.NPCDropProfiles {
		if mapTwo.Definition.NPCDropProfiles[index].DropProfileID == record.DropProfileID {
			mapTwo.Definition.NPCDropProfiles[index].LootTableID = "missing_outer_ring_salvage"
		}
	}

	event := testNPCKilledEventForRecord(resolved.PlayerID, mapTwo, record)
	_, err = gameServer.runtime.selectNPCKillLootTableLocked(resolved.PlayerID, event)
	if !errors.Is(err, errNPCLootTableUnavailable) {
		t.Fatalf("selectNPCKillLootTableLocked(map_1_2 missing table) error = %v, want %v", err, errNPCLootTableUnavailable)
	}
	requireMetricCounter(t, gameServer.runtime.Metrics.Snapshot(), observability.MetricNPCLootSelectorDecisions, 1, []observability.Label{
		{Name: "map_key", Value: "1-2"},
		{Name: "npc_type", Value: "outer_ring_scout_drone"},
		{Name: "reason", Value: "unavailable"},
		{Name: "result", Value: "rejected"},
		{Name: "risk_band", Value: "low"},
		{Name: "stage", Value: "loot_table"},
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "map_1_2"},
	})
	if drop, ok := gameServer.runtime.Loot.Drop("drop_1"); ok {
		t.Fatalf("selector failure created drop %+v; want no fallback drop", drop)
	}
}

func TestNPCLootSelectorRejectsMatrixMismatchesWithoutTrainingFallback(t *testing.T) {
	for _, tc := range []struct {
		name        string
		arrange     func(*mapInstance, *combat.NPCKilledEvent)
		want        error
		wantStage   string
		wantReason  string
		wantMapKey  string
		wantRisk    string
		wantWorldID string
		wantZoneID  string
		wantNPCType string
	}{
		{
			name: "kill event wrong map",
			arrange: func(_ *mapInstance, event *combat.NPCKilledEvent) {
				event.WorldID = "world-2"
				event.ZoneID = worldmaps.MapID("map_1_2").ZoneID()
			},
			want:        errNPCLootMapUnavailable,
			wantStage:   npcLootSelectorStageMap,
			wantReason:  npcLootSelectorReasonMismatch,
			wantMapKey:  "1-1",
			wantRisk:    "low",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_1",
			wantNPCType: "training_drone",
		},
		{
			name: "kill event npc type mismatches spawn record",
			arrange: func(_ *mapInstance, event *combat.NPCKilledEvent) {
				event.NPCType = "outer_ring_scout_drone"
			},
			want:        errNPCLootProfileMismatch,
			wantStage:   npcLootSelectorStageKillRecord,
			wantReason:  npcLootSelectorReasonMismatch,
			wantMapKey:  "1-1",
			wantRisk:    "low",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_1",
			wantNPCType: "outer_ring_scout_drone",
		},
		{
			name: "record level outside profile rank band",
			arrange: func(instance *mapInstance, _ *combat.NPCKilledEvent) {
				instance.Definition.NPCDropProfiles[0].MinLevel = 2
				instance.Definition.NPCDropProfiles[0].MaxLevel = 2
			},
			want:        errNPCLootProfileMismatch,
			wantStage:   npcLootSelectorStageDropProfile,
			wantReason:  npcLootSelectorReasonMismatch,
			wantMapKey:  "1-1",
			wantRisk:    "low",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_1",
			wantNPCType: "training_drone",
		},
		{
			name: "profile risk mismatches map risk",
			arrange: func(instance *mapInstance, _ *combat.NPCKilledEvent) {
				instance.Definition.NPCDropProfiles[0].RiskBand = "medium"
			},
			want:        errNPCLootProfileMismatch,
			wantStage:   npcLootSelectorStageDropProfile,
			wantReason:  npcLootSelectorReasonMismatch,
			wantMapKey:  "1-1",
			wantRisk:    "low",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_1",
			wantNPCType: "training_drone",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gameServer, httpServer := newTestServer(t, false)
			defer httpServer.Close()
			resolved := createResolvedRuntimeSession(t, gameServer, "selector-matrix-rejected@example.com", "Selector Matrix Rejected")

			gameServer.runtime.mu.Lock()
			defer gameServer.runtime.mu.Unlock()

			starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
			if err != nil {
				t.Fatalf("starter map instance: %v", err)
			}
			record, ok := starter.Worker.EnemySpawnRecord("entity_training_npc")
			if !ok {
				t.Fatalf("starter spawner missing entity_training_npc; snapshot=%+v", starter.Worker.EnemySpawnSnapshot())
			}
			event := testNPCKilledEventForRecord(resolved.PlayerID, starter, record)
			tc.arrange(starter, &event)

			_, err = gameServer.runtime.selectNPCKillLootTableForInstanceLocked(starter, event)
			if !errors.Is(err, tc.want) {
				t.Fatalf("selectNPCKillLootTableForInstanceLocked() error = %v, want %v", err, tc.want)
			}
			requireMetricCounter(t, gameServer.runtime.Metrics.Snapshot(), observability.MetricNPCLootSelectorDecisions, 1, []observability.Label{
				{Name: "map_key", Value: tc.wantMapKey},
				{Name: "npc_type", Value: tc.wantNPCType},
				{Name: "reason", Value: tc.wantReason},
				{Name: "result", Value: "rejected"},
				{Name: "risk_band", Value: tc.wantRisk},
				{Name: "stage", Value: tc.wantStage},
				{Name: "world_id", Value: tc.wantWorldID},
				{Name: "zone_id", Value: tc.wantZoneID},
			})
			if drop, ok := gameServer.runtime.Loot.Drop("drop_1"); ok {
				t.Fatalf("selector matrix mismatch created fallback drop %+v; want no fallback drop", drop)
			}
		})
	}
}

func TestNPCLootSelectorRejectsSeededMapMatrixMismatchesWithoutStarterFallback(t *testing.T) {
	for _, tc := range []struct {
		name        string
		mapID       worldmaps.MapID
		npcType     string
		wantMapKey  string
		wantRisk    string
		wantWorldID string
		wantZoneID  string
		arrange     func(*testing.T, *Runtime, *mapInstance, worker.EnemySpawnRecord)
		want        error
		wantStage   string
		wantReason  string
	}{
		{
			name:        "destination level band mismatch",
			mapID:       "map_1_2",
			npcType:     "outer_ring_scout_drone",
			wantMapKey:  "1-2",
			wantRisk:    "low",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_2",
			arrange: func(_ *testing.T, _ *Runtime, instance *mapInstance, record worker.EnemySpawnRecord) {
				for index := range instance.Definition.NPCDropProfiles {
					if instance.Definition.NPCDropProfiles[index].DropProfileID == record.DropProfileID {
						instance.Definition.NPCDropProfiles[index].MinLevel = record.Level + 1
						instance.Definition.NPCDropProfiles[index].MaxLevel = record.Level + 1
					}
				}
			},
			want:       errNPCLootProfileMismatch,
			wantStage:  npcLootSelectorStageDropProfile,
			wantReason: npcLootSelectorReasonMismatch,
		},
		{
			name:        "destination missing table",
			mapID:       "map_1_2",
			npcType:     "outer_ring_scout_drone",
			wantMapKey:  "1-2",
			wantRisk:    "low",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_2",
			arrange: func(_ *testing.T, _ *Runtime, instance *mapInstance, record worker.EnemySpawnRecord) {
				for index := range instance.Definition.NPCDropProfiles {
					if instance.Definition.NPCDropProfiles[index].DropProfileID == record.DropProfileID {
						instance.Definition.NPCDropProfiles[index].LootTableID = "missing_outer_ring_matrix_table"
					}
				}
			},
			want:       errNPCLootTableUnavailable,
			wantStage:  npcLootSelectorStageLootTable,
			wantReason: npcLootSelectorReasonUnavailable,
		},
		{
			name:        "destination table source mismatch",
			mapID:       "map_1_2",
			npcType:     "outer_ring_scout_drone",
			wantMapKey:  "1-2",
			wantRisk:    "low",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_2",
			arrange: func(t *testing.T, runtime *Runtime, instance *mapInstance, record worker.EnemySpawnRecord) {
				tableKey := "outer_ring_wrong_source_matrix_table"
				table := testRuntimeLootTable(t, "other_outer_ring_source", "outer_ring_wrong_source_item", "Outer Ring Wrong Source", 1)
				runtime.lootTables[tableKey] = table
				runtime.itemCatalog[table.Rows[0].ItemDefinition.ItemID] = table.Rows[0].ItemDefinition
				for index := range instance.Definition.NPCDropProfiles {
					if instance.Definition.NPCDropProfiles[index].DropProfileID == record.DropProfileID {
						instance.Definition.NPCDropProfiles[index].LootTableID = tableKey
					}
				}
			},
			want:       errNPCLootTableUnavailable,
			wantStage:  npcLootSelectorStageLootTable,
			wantReason: npcLootSelectorReasonUnavailable,
		},
		{
			name:        "destination risk band mismatch",
			mapID:       "map_1_2",
			npcType:     "outer_ring_scout_drone",
			wantMapKey:  "1-2",
			wantRisk:    "low",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_2",
			arrange: func(_ *testing.T, _ *Runtime, instance *mapInstance, record worker.EnemySpawnRecord) {
				for index := range instance.Definition.NPCDropProfiles {
					if instance.Definition.NPCDropProfiles[index].DropProfileID == record.DropProfileID {
						instance.Definition.NPCDropProfiles[index].RiskBand = "medium"
					}
				}
			},
			want:       errNPCLootProfileMismatch,
			wantStage:  npcLootSelectorStageDropProfile,
			wantReason: npcLootSelectorReasonMismatch,
		},
		{
			name:        "pvp risk band mismatch",
			mapID:       "map_1_3",
			npcType:     "border_raider_drone",
			wantMapKey:  "1-3",
			wantRisk:    "medium",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_3",
			arrange: func(_ *testing.T, _ *Runtime, instance *mapInstance, record worker.EnemySpawnRecord) {
				for index := range instance.Definition.NPCDropProfiles {
					if instance.Definition.NPCDropProfiles[index].DropProfileID == record.DropProfileID {
						instance.Definition.NPCDropProfiles[index].RiskBand = "low"
					}
				}
			},
			want:       errNPCLootProfileMismatch,
			wantStage:  npcLootSelectorStageDropProfile,
			wantReason: npcLootSelectorReasonMismatch,
		},
		{
			name:        "pvp level band mismatch",
			mapID:       "map_1_3",
			npcType:     "border_raider_drone",
			wantMapKey:  "1-3",
			wantRisk:    "medium",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_3",
			arrange: func(_ *testing.T, _ *Runtime, instance *mapInstance, record worker.EnemySpawnRecord) {
				for index := range instance.Definition.NPCDropProfiles {
					if instance.Definition.NPCDropProfiles[index].DropProfileID == record.DropProfileID {
						instance.Definition.NPCDropProfiles[index].MinLevel = record.Level + 1
						instance.Definition.NPCDropProfiles[index].MaxLevel = record.Level + 1
					}
				}
			},
			want:       errNPCLootProfileMismatch,
			wantStage:  npcLootSelectorStageDropProfile,
			wantReason: npcLootSelectorReasonMismatch,
		},
		{
			name:        "pvp missing table",
			mapID:       "map_1_3",
			npcType:     "border_raider_drone",
			wantMapKey:  "1-3",
			wantRisk:    "medium",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_3",
			arrange: func(_ *testing.T, _ *Runtime, instance *mapInstance, record worker.EnemySpawnRecord) {
				for index := range instance.Definition.NPCDropProfiles {
					if instance.Definition.NPCDropProfiles[index].DropProfileID == record.DropProfileID {
						instance.Definition.NPCDropProfiles[index].LootTableID = "missing_border_raider_matrix_table"
					}
				}
			},
			want:       errNPCLootTableUnavailable,
			wantStage:  npcLootSelectorStageLootTable,
			wantReason: npcLootSelectorReasonUnavailable,
		},
		{
			name:        "pvp table source mismatch",
			mapID:       "map_1_3",
			npcType:     "border_raider_drone",
			wantMapKey:  "1-3",
			wantRisk:    "medium",
			wantWorldID: "world-1",
			wantZoneID:  "map_1_3",
			arrange: func(t *testing.T, runtime *Runtime, instance *mapInstance, record worker.EnemySpawnRecord) {
				tableKey := "border_raider_wrong_source_matrix_table"
				table := testRuntimeLootTable(t, "other_border_raider_source", "border_raider_wrong_source_item", "Border Raider Wrong Source", 1)
				runtime.lootTables[tableKey] = table
				runtime.itemCatalog[table.Rows[0].ItemDefinition.ItemID] = table.Rows[0].ItemDefinition
				for index := range instance.Definition.NPCDropProfiles {
					if instance.Definition.NPCDropProfiles[index].DropProfileID == record.DropProfileID {
						instance.Definition.NPCDropProfiles[index].LootTableID = tableKey
					}
				}
			},
			want:       errNPCLootTableUnavailable,
			wantStage:  npcLootSelectorStageLootTable,
			wantReason: npcLootSelectorReasonUnavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gameServer, httpServer := newTestServer(t, false)
			defer httpServer.Close()
			resolved := createResolvedRuntimeSession(t, gameServer, "selector-seeded-matrix-rejected@example.com", "Selector Seeded Matrix Rejected")

			gameServer.runtime.mu.Lock()
			defer gameServer.runtime.mu.Unlock()

			instance, err := gameServer.runtime.mapInstanceLocked(tc.mapID)
			if err != nil {
				t.Fatalf("map instance %q: %v", tc.mapID, err)
			}
			record := requireSpawnRecordByNPCType(t, instance, tc.npcType)
			event := testNPCKilledEventForRecord(resolved.PlayerID, instance, record)
			tc.arrange(t, gameServer.runtime, instance, record)

			_, err = gameServer.runtime.selectNPCKillLootTableForInstanceLocked(instance, event)
			if !errors.Is(err, tc.want) {
				t.Fatalf("selectNPCKillLootTableForInstanceLocked() error = %v, want %v", err, tc.want)
			}
			requireMetricCounter(t, gameServer.runtime.Metrics.Snapshot(), observability.MetricNPCLootSelectorDecisions, 1, []observability.Label{
				{Name: "map_key", Value: tc.wantMapKey},
				{Name: "npc_type", Value: tc.npcType},
				{Name: "reason", Value: tc.wantReason},
				{Name: "result", Value: "rejected"},
				{Name: "risk_band", Value: tc.wantRisk},
				{Name: "stage", Value: tc.wantStage},
				{Name: "world_id", Value: tc.wantWorldID},
				{Name: "zone_id", Value: tc.wantZoneID},
			})
			if drop, ok := gameServer.runtime.Loot.Drop("drop_1"); ok {
				t.Fatalf("seeded matrix mismatch created fallback drop %+v; want no starter fallback drop", drop)
			}
		})
	}
}

func TestNPCLootSelectorRejectsMissingInputsWithoutTrainingFallback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arrange func(*Runtime, *mapInstance, *combat.NPCKilledEvent)
		want    error
	}{
		{
			name: "missing spawner record",
			arrange: func(_ *Runtime, _ *mapInstance, event *combat.NPCKilledEvent) {
				event.NPCEntityID = "entity_missing_npc"
				event.SourceID = event.NPCEntityID
			},
			want: errNPCLootSpawnUnavailable,
		},
		{
			name: "missing drop profile",
			arrange: func(_ *Runtime, instance *mapInstance, _ *combat.NPCKilledEvent) {
				instance.Definition.NPCDropProfiles = nil
			},
			want: errNPCLootProfileUnavailable,
		},
		{
			name: "missing loot table",
			arrange: func(_ *Runtime, instance *mapInstance, _ *combat.NPCKilledEvent) {
				instance.Definition.NPCDropProfiles[0].LootTableID = "missing_selector_table"
			},
			want: errNPCLootTableUnavailable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gameServer, httpServer := newTestServer(t, false)
			defer httpServer.Close()
			resolved := createResolvedRuntimeSession(t, gameServer, "selector-missing@example.com", "Selector Missing")

			gameServer.runtime.mu.Lock()
			defer gameServer.runtime.mu.Unlock()

			starter, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
			if err != nil {
				t.Fatalf("starter map instance: %v", err)
			}
			record, ok := starter.Worker.EnemySpawnRecord("entity_training_npc")
			if !ok {
				t.Fatalf("starter spawner missing entity_training_npc; snapshot=%+v", starter.Worker.EnemySpawnSnapshot())
			}
			event := testNPCKilledEventForRecord(resolved.PlayerID, starter, record)
			tc.arrange(gameServer.runtime, starter, &event)

			_, err = gameServer.runtime.selectNPCKillLootTableLocked(resolved.PlayerID, event)
			if !errors.Is(err, tc.want) {
				t.Fatalf("selectNPCKillLootTableLocked() error = %v, want %v", err, tc.want)
			}
			if tc.want == errNPCLootTableUnavailable {
				requireMetricCounter(t, gameServer.runtime.Metrics.Snapshot(), observability.MetricNPCLootSelectorDecisions, 1, []observability.Label{
					{Name: "map_key", Value: "1-1"},
					{Name: "npc_type", Value: "training_drone"},
					{Name: "reason", Value: "unavailable"},
					{Name: "result", Value: "rejected"},
					{Name: "risk_band", Value: "low"},
					{Name: "stage", Value: "loot_table"},
					{Name: "world_id", Value: "world-1"},
					{Name: "zone_id", Value: "map_1_1"},
				})
			}
			if drop, ok := gameServer.runtime.Loot.Drop("drop_1"); ok {
				t.Fatalf("selector failure created drop %+v; want no fallback drop", drop)
			}
		})
	}
}

func testNPCKilledEventForRecord(playerID foundation.PlayerID, instance *mapInstance, record worker.EnemySpawnRecord) combat.NPCKilledEvent {
	return combat.NPCKilledEvent{
		SourceID:      record.EntityID,
		NPCEntityID:   record.EntityID,
		NPCType:       record.NPCType,
		WorldID:       instance.Definition.WorldID,
		ZoneID:        instance.Definition.ZoneID,
		Position:      record.Position,
		OwnerPlayerID: playerID,
		KilledAt:      time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC),
	}
}

func testRuntimeLootTable(t *testing.T, tableID string, itemID foundation.ItemID, itemName string, quantity int64) loot.LootTable {
	t.Helper()
	source, err := catalog.NewLootTableSource(tableID, "v1")
	if err != nil {
		t.Fatalf("NewLootTableSource() error = %v", err)
	}
	itemDefinition, err := runtimeStackableDefinition(itemID, itemName)
	if err != nil {
		t.Fatalf("runtimeStackableDefinition() error = %v", err)
	}
	return loot.LootTable{
		Source: source,
		Rows: []loot.LootRow{{
			ItemDefinition: itemDefinition,
			MinQuantity:    quantity,
			MaxQuantity:    quantity,
			Chance:         1,
		}},
	}
}

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
	if err := commandErrorsFromSubmitAndTick(starter.Worker, worker.MarkEnemyKilledCommand{
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

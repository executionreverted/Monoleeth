package server

import (
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestPhase10SeededMapScannerMemoryAndDropMatrix(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	resolved := createResolvedRuntimeSession(
		t,
		gameServer,
		"phase10-scanner-drop-matrix@example.com",
		"Phase10 Matrix",
	)
	now := gameServer.runtime.clock.Now().UTC()

	cases := []struct {
		name             string
		mapID            worldmaps.MapID
		spawnID          worldmaps.SpawnID
		publicMapKey     string
		riskBand         string
		planetID         foundation.PlanetID
		coordinates      world.Vec2
		candidateKey     discovery.PlanetMaterializationKey
		npcType          string
		level            int
		dropProfileID    worldmaps.NPCDropProfileID
		lootTableID      string
		itemID           foundation.ItemID
		quantity         int64
		forbiddenPayload []string
	}{
		{
			name:          "starter map scanner memory and training drop",
			mapID:         worldmaps.StarterMapID,
			spawnID:       worldmaps.StarterSpawnID,
			publicMapKey:  "1-1",
			riskBand:      "low",
			planetID:      "phase10-public-1-1-planet",
			coordinates:   world.Vec2{X: 1400, Y: 1500},
			candidateKey:  "phase10-candidate-1-1",
			npcType:       "training_drone",
			level:         1,
			dropProfileID: "training_drone_salvage",
			lootTableID:   trainingDroneSalvageLootTableID,
			itemID:        "raw_ore",
			quantity:      3,
			forbiddenPayload: []string{
				"map_1_", "phase10-candidate", "drop_profile", "loot_table",
			},
		},
		{
			name:          "outer ring scanner memory and scout drop",
			mapID:         "map_1_2",
			spawnID:       "west_gate",
			publicMapKey:  "1-2",
			riskBand:      "low",
			planetID:      "phase10-public-1-2-planet",
			coordinates:   world.Vec2{X: 1600, Y: 5200},
			candidateKey:  "phase10-candidate-1-2",
			npcType:       "outer_ring_scout_drone",
			level:         1,
			dropProfileID: "outer_ring_scout_drone_salvage",
			lootTableID:   trainingDroneSalvageLootTableID,
			itemID:        "raw_ore",
			quantity:      3,
			forbiddenPayload: []string{
				"map_1_", "phase10-candidate", "drop_profile", "loot_table",
			},
		},
		{
			name:          "border skirmish scanner memory and raider drop",
			mapID:         "map_1_3",
			spawnID:       "west_gate",
			publicMapKey:  "1-3",
			riskBand:      "medium",
			planetID:      "phase10-public-1-3-planet",
			coordinates:   world.Vec2{X: 2200, Y: 6100},
			candidateKey:  "phase10-candidate-1-3",
			npcType:       "border_raider_drone",
			level:         2,
			dropProfileID: "border_raider_drone_salvage",
			lootTableID:   borderRaiderSalvageLootTableID,
			itemID:        "carbon_shards",
			quantity:      5,
			forbiddenPayload: []string{
				"map_1_", "phase10-candidate", "drop_profile", "loot_table",
			},
		},
	}

	for _, tc := range cases {
		if _, err := gameServer.runtime.Discovery.MaterializePlanet(discovery.MaterializePlanetInput{
			CandidateKey: tc.candidateKey,
			Planet: discovery.Planet{
				ID:           tc.planetID,
				WorldID:      gameServer.runtime.worldID,
				ZoneID:       tc.mapID.ZoneID(),
				Coordinates:  tc.coordinates,
				Biome:        discovery.PlanetBiomeOuterDrift,
				Type:         discovery.PlanetTypeIce,
				Rarity:       discovery.PlanetRarityUncommon,
				Level:        2,
				DiscoveredAt: now,
				DiscoveredBy: resolved.PlayerID,
			},
		}); err != nil {
			t.Fatalf("MaterializePlanet(%s) error = %v, want nil", tc.planetID, err)
		}
		if _, _, err := gameServer.runtime.Discovery.UpsertPlayerPlanetIntel(discovery.PlayerPlanetIntel{
			PlayerID:        resolved.PlayerID,
			PlanetID:        tc.planetID,
			WorldID:         gameServer.runtime.worldID,
			ZoneID:          tc.mapID.ZoneID(),
			Coordinates:     tc.coordinates,
			State:           discovery.IntelStateFresh,
			Confidence:      100,
			LastSeenAt:      now,
			SourceType:      discovery.IntelSourceAdmin,
			SourceReference: string(tc.candidateKey),
		}); err != nil {
			t.Fatalf("UpsertPlayerPlanetIntel(%s) error = %v, want nil", tc.planetID, err)
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setActiveMapForPhase10Matrix(t, gameServer, resolved.PlayerID, resolved, tc.mapID, tc.spawnID)

			known, err := gameServer.runtime.knownPlanetsPayload(resolved.PlayerID)
			if err != nil {
				t.Fatalf("knownPlanetsPayload(%s) error = %v, want nil", tc.mapID, err)
			}
			if len(known.Planets) != 1 ||
				known.Planets[0].PlanetID != tc.planetID.String() ||
				known.Planets[0].PublicMapKey != tc.publicMapKey {
				t.Fatalf("known planets on %s = %+v, want only %s on public map %s", tc.mapID, known, tc.planetID, tc.publicMapKey)
			}

			minimap, err := gameServer.runtime.currentMinimapPayload(resolved.PlayerID)
			if err != nil {
				t.Fatalf("currentMinimapPayload(%s) error = %v, want nil", tc.mapID, err)
			}
			if len(minimap.Remembered) != 1 ||
				minimap.Remembered[0].PlanetID != tc.planetID.String() ||
				minimap.Remembered[0].PublicMapKey != tc.publicMapKey {
				t.Fatalf("remembered minimap on %s = %+v, want only %s on public map %s", tc.mapID, minimap.Remembered, tc.planetID, tc.publicMapKey)
			}
			assertPhase10MapPayloadDoesNotLeak(t, tc.forbiddenPayload, known, minimap)

			gameServer.runtime.mu.Lock()
			defer gameServer.runtime.mu.Unlock()

			instance, err := gameServer.runtime.mapInstanceLocked(tc.mapID)
			if err != nil {
				t.Fatalf("mapInstanceLocked(%s) error = %v, want nil", tc.mapID, err)
			}
			if got := instance.Definition.PublicMapKey.String(); got != tc.publicMapKey {
				t.Fatalf("public map key = %q, want %q", got, tc.publicMapKey)
			}
			if got := instance.Definition.RiskBand; got != tc.riskBand {
				t.Fatalf("risk band = %q, want %q", got, tc.riskBand)
			}

			record := requireSpawnRecordByNPCType(t, instance, tc.npcType)
			if record.Level != tc.level || record.DropProfileID != tc.dropProfileID {
				t.Fatalf("spawn record = %+v, want level=%d drop_profile=%q", record, tc.level, tc.dropProfileID)
			}
			profile, ok := npcDropProfileByID(instance.Definition, record.DropProfileID)
			if !ok {
				t.Fatalf("drop profile %q missing", record.DropProfileID)
			}
			if profile.NPCType != tc.npcType ||
				profile.RiskBand != tc.riskBand ||
				profile.LootTableID != tc.lootTableID {
				t.Fatalf("drop profile = %+v, want npc=%q risk=%q table=%q", profile, tc.npcType, tc.riskBand, tc.lootTableID)
			}

			event := testNPCKilledEventForRecord(resolved.PlayerID, instance, record)
			selected, err := gameServer.runtime.selectNPCKillLootTableForInstanceLocked(instance, event)
			if err != nil {
				t.Fatalf("selectNPCKillLootTableForInstanceLocked(%s) error = %v, want nil", tc.mapID, err)
			}
			if got := selected.Source.DefinitionID.String(); got != tc.lootTableID {
				t.Fatalf("selected loot table id = %q, want %q", got, tc.lootTableID)
			}
			created, err := gameServer.runtime.Loot.CreateDropsForNPCKill(event, selected)
			if err != nil {
				t.Fatalf("CreateDropsForNPCKill(%s) error = %v, want nil", tc.mapID, err)
			}
			matrixDrop, ok := seededPhase10MatrixDrop(
				created.Drops,
				instance.Definition.WorldID,
				instance.Definition.ZoneID,
				record.EntityID,
				tc.itemID,
				tc.quantity,
			)
			if !ok {
				t.Fatalf("created drops = %+v, want %s x%d on public map %s", created.Drops, tc.itemID, tc.quantity, tc.publicMapKey)
			}
			assertPhase10MapPayloadDoesNotLeak(t, tc.forbiddenPayload, lootDropPayload(matrixDrop, gameServer.runtime.clock.Now()))
		})
	}
}

func seededPhase10MatrixDrop(
	drops []loot.Drop,
	worldID world.WorldID,
	zoneID foundation.ZoneID,
	sourceID world.EntityID,
	itemID foundation.ItemID,
	quantity int64,
) (loot.Drop, bool) {
	for _, drop := range drops {
		if drop.WorldID == worldID &&
			drop.ZoneID == zoneID &&
			drop.SourceID == sourceID &&
			drop.ItemDefinition.ItemID == itemID &&
			drop.Quantity == quantity {
			return drop, true
		}
	}
	return loot.Drop{}, false
}

func setActiveMapForPhase10Matrix(
	t *testing.T,
	gameServer *Server,
	playerID foundation.PlayerID,
	resolved auth.ResolvedSession,
	mapID worldmaps.MapID,
	spawnID worldmaps.SpawnID,
) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	if _, err := gameServer.runtime.mapRouter.SetActiveLocationFromSpawn(playerID, mapID, spawnID); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("SetActiveLocationFromSpawn(%q, %q) error = %v, want nil", mapID, spawnID, err)
	}
	gameServer.runtime.mu.Unlock()
	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("ensurePlayerSession(%q) error = %v, want nil", mapID, err)
	}
}

func assertPhase10MapPayloadDoesNotLeak(t *testing.T, forbidden []string, values ...any) {
	t.Helper()
	raw, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("marshal phase10 payloads: %v", err)
	}
	payload := string(raw)
	for _, value := range forbidden {
		if strings.Contains(payload, value) {
			t.Fatalf("payload leaked %q in %s", value, payload)
		}
	}
}

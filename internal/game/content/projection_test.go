package content

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

func TestProjectSnapshotForPlayersOmitsHiddenFieldsAndSentinels(t *testing.T) {
	const hiddenSentinel = "HIDDEN_PROJECTION_SENTINEL"
	snapshot := Snapshot{
		Version: "content_projection_v1",
		Items: []SnapshotRow{{
			ContentID:   "raw_ore",
			Enabled:     true,
			DisplayJSON: json.RawMessage(`{"display_name":"Raw Ore","category":"resources","audit_note":"HIDDEN_PROJECTION_SENTINEL"}`),
			DataJSON: json.RawMessage(`{
				"name":"Raw Ore",
				"item_type":"stackable",
				"rarity":"common",
				"max_stack":999,
				"weight_units":2,
				"loot_chance":"HIDDEN_PROJECTION_SENTINEL",
				"server_only":"HIDDEN_PROJECTION_SENTINEL"
			}`),
		}, {
			ContentID: "disabled_secret_item",
			Enabled:   false,
			DataJSON:  json.RawMessage(`{"name":"HIDDEN_PROJECTION_SENTINEL","item_type":"stackable"}`),
		}},
		Modules: []SnapshotRow{{
			ContentID:   "laser_alpha_t1",
			Enabled:     true,
			DisplayJSON: json.RawMessage(`{"display_name":"Prism Lance I","category":"weapons","art_key":"module.prism_lance_1"}`),
			DataJSON: json.RawMessage(`{
				"name":"Laser Alpha I",
				"module_category":"offensive",
				"slot_type":"offensive",
				"tier":1,
				"rarity":"common",
				"required_rank":1,
				"stat_modifiers":[{"stat":"weapon_damage","kind":"flat","value":12}],
				"energy":{"activation_cost":8},
				"cooldowns":[{"key":"basic_attack","duration_ms":1200}],
				"durability":{"max":100},
				"spawn_timer_ms":"HIDDEN_PROJECTION_SENTINEL",
				"procedural_seed":"HIDDEN_PROJECTION_SENTINEL"
			}`),
		}},
		ShopProducts: []SnapshotRow{{
			ContentID: "product_raw_ore",
			Enabled:   true,
			DataJSON: json.RawMessage(`{
				"product_type":"item",
				"display":{"display_name":"Ferrite Ore","description":"Starter ore.","category":"resources","art_key":"item.ferrite_ore","sort_order":10,"server_only":"HIDDEN_PROJECTION_SENTINEL"},
				"grant_target":{"kind":"item","ref_id":"raw_ore","quantity":10},
				"price_policy":{"currency_type":"credits","amount":40,"fixed":true},
				"stock_policy":{"kind":"unlimited"},
				"availability":{"available":true},
				"enemy_pool_caps":"HIDDEN_PROJECTION_SENTINEL"
			}`),
		}},
		LootTables: []SnapshotRow{{
			ContentID: "secret_loot_table",
			Enabled:   true,
			DataJSON:  json.RawMessage(`{"drop_chance":"HIDDEN_PROJECTION_SENTINEL"}`),
		}},
		SpawnAreas: []SnapshotRow{{
			ContentID: "secret_spawn_area",
			Enabled:   true,
			DataJSON:  json.RawMessage(`{"spawn_timer_ms":"HIDDEN_PROJECTION_SENTINEL"}`),
		}},
		EnemyPools: []SnapshotRow{{
			ContentID: "secret_enemy_pool",
			Enabled:   true,
			DataJSON:  json.RawMessage(`{"enemy_pool_caps":"HIDDEN_PROJECTION_SENTINEL"}`),
		}},
		NPCDropProfiles: []SnapshotRow{{
			ContentID: "secret_drop_profile",
			Enabled:   true,
			DataJSON:  json.RawMessage(`{"audit_note":"HIDDEN_PROJECTION_SENTINEL"}`),
		}},
	}

	projection, err := ProjectSnapshotForPlayers(snapshot)
	if err != nil {
		t.Fatalf("ProjectSnapshotForPlayers() error = %v, want nil", err)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("Marshal projection error = %v, want nil", err)
	}
	body := string(encoded)

	for _, want := range []string{`"version":"content_projection_v1"`, `"item_id":"raw_ore"`, `"display_name":"Raw Ore"`, `"stat":"weapon_damage"`, `"amount":40`} {
		if !strings.Contains(body, want) {
			t.Fatalf("projection missing %s: %s", want, body)
		}
	}
	for _, forbidden := range []string{
		hiddenSentinel,
		"disabled_secret_item",
		"loot_tables",
		"spawn_areas",
		"enemy_pools",
		"npc_drop_profiles",
		"loot_chance",
		"drop_chance",
		"spawn_timer_ms",
		"enemy_pool_caps",
		"audit_note",
		"procedural_seed",
		"server_only",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("projection leaked %q: %s", forbidden, body)
		}
	}
}

func TestProjectGameplayContentForPlayersOmitsServerOnlyCatalogs(t *testing.T) {
	const hiddenSentinel = "HIDDEN_RUNTIME_METADATA_SENTINEL"
	bundle, err := DefaultGameplayContent(world.WorldID("projection-world"))
	if err != nil {
		t.Fatalf("DefaultGameplayContent() error = %v, want nil", err)
	}
	rawOre := bundle.Items[foundation.ItemID("raw_ore")]
	rawOre.MetadataSchema = json.RawMessage(`{"server_only":"HIDDEN_RUNTIME_METADATA_SENTINEL"}`)
	bundle.Items[rawOre.ItemID] = rawOre

	projection, err := ProjectGameplayContentForPlayers(bundle)
	if err != nil {
		t.Fatalf("ProjectGameplayContentForPlayers() error = %v, want nil", err)
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatalf("Marshal projection error = %v, want nil", err)
	}
	body := string(encoded)

	for _, want := range []string{`"items"`, `"modules"`, `"shop_products"`, `"categories"`, `"item_id":"raw_ore"`, `"product_id":"product_ferrite_ore"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("projection missing %s: %s", want, body)
		}
	}
	for _, forbidden := range []string{
		hiddenSentinel,
		"metadata_schema",
		"loot_tables",
		"spawn_areas",
		"enemy_pools",
		"drop_chance",
		"chance",
		"procedural_seed",
		"server_only",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("projection leaked %q: %s", forbidden, body)
		}
	}
}

func TestProjectGameplayContentForPlayersMatchesRepositoryContent(t *testing.T) {
	bundle, err := LoadPublishedContent(context.Background(), NewStaticRepository(), world.WorldID("projection-world"))
	if err != nil {
		t.Fatalf("LoadPublishedContent() error = %v, want nil", err)
	}

	projection, err := ProjectGameplayContentForPlayers(bundle)
	if err != nil {
		t.Fatalf("ProjectGameplayContentForPlayers() error = %v, want nil", err)
	}

	if projection.Version == "" {
		t.Fatal("projection version empty, want content version")
	}
	if len(projection.Items) == 0 || len(projection.Modules) == 0 || len(projection.ShopProducts) == 0 {
		t.Fatalf("projection missing visible content: items=%d modules=%d products=%d", len(projection.Items), len(projection.Modules), len(projection.ShopProducts))
	}
}

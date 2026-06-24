package server

import (
	"encoding/json"
	"strings"
	"testing"

	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/modules"
)

func TestContentCatalogReturnsSafePlayerProjection(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	conn := dialWebSocket(t, httpServer, registerPilotWithIdentity(t, httpServer, "content-catalog@example.com", "Catalog Pilot"))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-content-catalog","op":"content.catalog","payload":{},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("content.catalog response = %+v, want success", response)
	}
	assertNoPhase09Leak(t, "content catalog", response.Payload)
	assertNoContentCatalogLeak(t, response.Payload)

	var payload contentCatalogResponsePayload
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode content catalog: %v", err)
	}
	catalog := payload.ContentCatalog
	if catalog.Version == "" || catalog.Version != gameServer.runtime.contentCatalogVersion {
		t.Fatalf("content catalog version = %q runtime=%q, want runtime version", catalog.Version, gameServer.runtime.contentCatalogVersion)
	}
	if len(catalog.Categories) == 0 || len(catalog.Items) == 0 || len(catalog.Modules) == 0 || len(catalog.ShopProducts) == 0 {
		t.Fatalf("content catalog missing slices: categories=%d items=%d modules=%d shop_products=%d", len(catalog.Categories), len(catalog.Items), len(catalog.Modules), len(catalog.ShopProducts))
	}
	if !contentCatalogHasItem(catalog.Items, "raw_ore") {
		t.Fatalf("content catalog items missing raw_ore: %+v", catalog.Items)
	}
	if !contentCatalogHasModule(catalog.Modules, "laser_alpha_t1") {
		t.Fatalf("content catalog modules missing laser_alpha_t1: %+v", catalog.Modules)
	}
	if !contentCatalogHasShopProduct(catalog.ShopProducts, "product_ferrite_ore") {
		t.Fatalf("content catalog shop products missing product_ferrite_ore: %+v", catalog.ShopProducts)
	}

	writeText(t, conn, `{"request_id":"request-content-catalog-spoof","op":"content.catalog","payload":{"player_id":"spoof"},"client_seq":2,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("content.catalog spoof error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}
}

func TestNewRuntimeStoresPlayerContentCatalogProjection(t *testing.T) {
	bundle := runtimeTestBundleWithLaserDamage(t, 77)
	runtime, err := NewRuntime(RuntimeConfig{
		WorldID:           foundation.WorldID("world-1"),
		ContentRepository: &fakeRuntimeRepository{bundle: bundle},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v, want nil", err)
	}

	if runtime.contentCatalogVersion == "" || runtime.contentCatalogProjection.Version != runtime.contentCatalogVersion {
		t.Fatalf("runtime content catalog version = %q projection=%q, want cached version", runtime.contentCatalogVersion, runtime.contentCatalogProjection.Version)
	}
	got := contentCatalogModuleStat(runtime.contentCatalogProjection.Modules, "laser_alpha_t1", modules.StatWeaponDamage.String())
	if got != 77 {
		t.Fatalf("content catalog laser damage = %d, want repository value 77", got)
	}
}

func assertNoContentCatalogLeak(t *testing.T, payload json.RawMessage) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{
		"HIDDEN_RUNTIME_METADATA_SENTINEL",
		"phase07-static-seed",
		"training_drone_salvage",
		"border_raider_salvage",
		"starter_training_drone_pool",
		"scanner_1_1_origin_v1",
		"loot_tables",
		"loot_table",
		"spawn_areas",
		"spawn_timer_ms",
		"enemy_pools",
		"enemy_pool",
		"drop_profile",
		"npc_drop_profiles",
		"npc_event_spawns",
		"spawn_budget",
		"audit_log",
		"audit_note",
		"procedural_seed",
		"server_only",
		"data_json",
		"display_json",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("content catalog leaked %q in %s", forbidden, raw)
		}
	}
}

func contentCatalogHasItem(items []gamecontent.PlayerItemProjection, itemID string) bool {
	for _, item := range items {
		if item.ItemID == itemID {
			return true
		}
	}
	return false
}

func contentCatalogHasModule(modules []gamecontent.PlayerModuleProjection, itemID string) bool {
	for _, module := range modules {
		if module.ItemID == itemID {
			return true
		}
	}
	return false
}

func contentCatalogHasShopProduct(products []gamecontent.PlayerShopProductProjection, productID string) bool {
	for _, product := range products {
		if product.ProductID == productID {
			return true
		}
	}
	return false
}

func contentCatalogModuleStat(modules []gamecontent.PlayerModuleProjection, itemID string, stat string) int64 {
	for _, module := range modules {
		if module.ItemID != itemID {
			continue
		}
		for _, modifier := range module.StatModifiers {
			if modifier.Stat == stat {
				return modifier.Value
			}
		}
	}
	return 0
}

package contentseed

import (
	"testing"

	"gameproject/internal/game/content"
	"gameproject/internal/game/contentseed/kalaazu"
	"gameproject/internal/game/world"
)

func TestDefaultSnapshotLegacyBridgeReportCoversEveryNonKalaazuRow(t *testing.T) {
	snapshot, err := BuildDefaultSnapshot(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("BuildDefaultSnapshot() error = %v, want nil", err)
	}
	kalaazuRows, err := kalaazu.BuildDefaultRows(kalaazu.DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildDefaultRows() error = %v, want nil", err)
	}
	kalaazuIDs := kalaazuDefaultRowIDs(kalaazuRows)
	report, err := DefaultSnapshotLegacyBridgeReport(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultSnapshotLegacyBridgeReport() error = %v, want nil", err)
	}
	reportIDs := make(map[content.ContentType]map[content.ContentID]string)
	for _, row := range report {
		if row.Reason == "" {
			t.Fatalf("bridge row %+v has empty reason", row)
		}
		if _, ok := reportIDs[row.ContentType]; !ok {
			reportIDs[row.ContentType] = make(map[content.ContentID]string)
		}
		reportIDs[row.ContentType][row.ContentID] = row.Reason
	}

	for _, group := range snapshot.Groups() {
		for _, row := range group.Rows {
			_, fromKalaazu := kalaazuIDs[group.Type][row.ContentID]
			_, documented := reportIDs[group.Type][row.ContentID]
			switch {
			case fromKalaazu && documented:
				t.Fatalf("Kalaazu row %s/%s also documented as legacy bridge", group.Type, row.ContentID)
			case !fromKalaazu && !documented:
				t.Fatalf("non-Kalaazu row %s/%s missing bridge report entry", group.Type, row.ContentID)
			}
		}
	}

	for _, row := range report {
		switch row.ContentType {
		case content.ContentTypeMap,
			content.ContentTypeMapPortal,
			content.ContentTypeModule,
			content.ContentTypeShopProduct,
			content.ContentTypeShip,
			content.ContentTypeScannerConfig,
			content.ContentTypeNPCTemplate,
			content.ContentTypeSpawnArea,
			content.ContentTypeEnemyPool,
			content.ContentTypeNPCDropProfile,
			content.ContentTypeNPCAggroProfile,
			content.ContentTypeNPCLeashProfile:
			t.Fatalf("bridge report contains %s/%s, want map/NPC/shop/ship rows fully Kalaazu-derived", row.ContentType, row.ContentID)
		}
	}
}

func TestDefaultSnapshotLegacyBridgeReportNamesExpectedTemporaryRows(t *testing.T) {
	report, err := DefaultSnapshotLegacyBridgeReport(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultSnapshotLegacyBridgeReport() error = %v, want nil", err)
	}
	for _, want := range []struct {
		contentType content.ContentType
		contentID   content.ContentID
	}{
		{content.ContentTypeCraftRecipe, "laser_alpha_t1"},
		{content.ContentTypeCombatRules, "combat_rules"},
	} {
		if !legacyBridgeReportHasRow(report, want.contentType, want.contentID) {
			t.Fatalf("bridge report missing expected temporary row %s/%s", want.contentType, want.contentID)
		}
	}
}

func TestDefaultSnapshotLegacyBridgeReportHasNoLootTableBridgeRows(t *testing.T) {
	report, err := DefaultSnapshotLegacyBridgeReport(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultSnapshotLegacyBridgeReport() error = %v, want nil", err)
	}
	for _, row := range report {
		if row.ContentType == content.ContentTypeLootTable {
			t.Fatalf("bridge report contains loot table row %+v, want loot tables seeded from Kalaazu default rows", row)
		}
	}
}

func TestDefaultSnapshotLegacyBridgeReportDoesNotBridgeProjectedStarterModules(t *testing.T) {
	report, err := DefaultSnapshotLegacyBridgeReport(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultSnapshotLegacyBridgeReport() error = %v, want nil", err)
	}
	for _, contentID := range []content.ContentID{
		"laser_alpha_t1",
		"shield_generator_t1",
		"scanner_t1",
		"radar_t1",
		"cargo_expander_t1",
	} {
		if legacyBridgeReportHasRow(report, content.ContentTypeModule, contentID) {
			t.Fatalf("bridge report contains module/%s, want Kalaazu compatibility projection", contentID)
		}
	}
}

func TestDefaultSnapshotLegacyBridgeReportDoesNotBridgeProjectedStarterModuleItems(t *testing.T) {
	report, err := DefaultSnapshotLegacyBridgeReport(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultSnapshotLegacyBridgeReport() error = %v, want nil", err)
	}
	for _, contentID := range []content.ContentID{
		"laser_alpha_t1",
		"shield_generator_t1",
		"scanner_t1",
		"radar_t1",
		"cargo_expander_t1",
	} {
		if legacyBridgeReportHasRow(report, content.ContentTypeItem, contentID) {
			t.Fatalf("bridge report contains item/%s, want Kalaazu compatibility projection", contentID)
		}
	}
}

func TestDefaultSnapshotLegacyBridgeReportDoesNotBridgeStarterConfig(t *testing.T) {
	report, err := DefaultSnapshotLegacyBridgeReport(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultSnapshotLegacyBridgeReport() error = %v, want nil", err)
	}
	if legacyBridgeReportHasRow(report, content.ContentTypeStarterConfig, "starter_config") {
		t.Fatalf("bridge report contains starter_config/starter_config, want Kalaazu starter config projection")
	}
}

func TestDefaultSnapshotLegacyBridgeReportDoesNotBridgeScannerConfig(t *testing.T) {
	report, err := DefaultSnapshotLegacyBridgeReport(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultSnapshotLegacyBridgeReport() error = %v, want nil", err)
	}
	if legacyBridgeReportHasRow(report, content.ContentTypeScannerConfig, "scanner_config") {
		t.Fatalf("bridge report contains scanner_config/scanner_config, want Kalaazu scanner config projection")
	}
}

func TestDefaultSnapshotLegacyBridgeReportDoesNotBridgeProjectedMaterialItems(t *testing.T) {
	report, err := DefaultSnapshotLegacyBridgeReport(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultSnapshotLegacyBridgeReport() error = %v, want nil", err)
	}
	for _, contentID := range []content.ContentID{
		"prometium",
		"raw_ore",
		"endurium",
		"iron_ore",
		"terbium",
		"prometid",
		"refined_alloy",
		"duranium",
		"xenomit",
		"carbon_shards",
		"promerium",
	} {
		if legacyBridgeReportHasRow(report, content.ContentTypeItem, contentID) {
			t.Fatalf("bridge report contains item/%s, want Kalaazu material projection", contentID)
		}
	}
}

func legacyBridgeReportHasRow(report []LegacyBridgeRow, contentType content.ContentType, contentID content.ContentID) bool {
	for _, row := range report {
		if row.ContentType == contentType && row.ContentID == contentID {
			return true
		}
	}
	return false
}

package kalaazu

import (
	"testing"

	"gameproject/internal/game/content"
)

func TestBuildDefaultRowsReturnsImportReportAndAllSupportedGroups(t *testing.T) {
	rows, err := BuildDefaultRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildDefaultRows() error = %v, want nil", err)
	}
	if got, want := len(rows.ItemRows), rows.Report.SourceRows["items"]+23; got != want {
		t.Fatalf("item imported/source rows = %d/%d, want source rows plus starter compatibility rows", got, rows.Report.SourceRows["items"])
	}
	if rows.Report.SourceRows["maps"] == 0 ||
		rows.Report.SourceRows["maps_npcs"] == 0 ||
		rows.Report.SourceRows["npcs"] == 0 ||
		rows.Report.SourceRows["ships"] == 0 ||
		rows.Report.SourceRows["maps_portals"] == 0 {
		t.Fatalf("source report = %+v, want all dump tables counted", rows.Report.SourceRows)
	}
	if rows.Report.ImportedRows[content.ContentTypeMap] != 3 ||
		rows.Report.ImportedRows[content.ContentTypeMapPortal] == 0 ||
		rows.Report.ImportedRows[content.ContentTypeModule] != 27 ||
		rows.Report.ImportedRows[content.ContentTypeShip] != 17 ||
		rows.Report.ImportedRows[content.ContentTypeCraftRecipe] != 3 ||
		rows.Report.ImportedRows[content.ContentTypeProductionBuilding] != 3 ||
		rows.Report.ImportedRows[content.ContentTypeProductionRules] != 1 ||
		rows.Report.ImportedRows[content.ContentTypeCombatRules] != 1 ||
		rows.Report.ImportedRows[content.ContentTypeQuestTemplate] != 14 ||
		rows.Report.ImportedRows[content.ContentTypeQuestRewardTable] != 14 ||
		rows.Report.ImportedRows[content.ContentTypeEnemyPool] != 11 ||
		rows.Report.ImportedRows[content.ContentTypeScannerConfig] != 1 ||
		rows.Report.ImportedRows[content.ContentTypeStarterConfig] != 1 ||
		rows.Report.ImportedRows[content.ContentTypeRoutePolicy] != 1 {
		t.Fatalf("imported report = %+v, want starter maps/modules/ships/enemy pools", rows.Report.ImportedRows)
	}
	if len(rows.Report.UnsupportedItems) == 0 {
		t.Fatalf("unsupported item report empty, want unsupported equipment categories counted")
	}
	if _, ok := rows.Report.UnsupportedItems["category_4_type_17"]; ok {
		t.Fatalf("unsupported item report = %+v, want rocket launchers mapped as offensive modules", rows.Report.UnsupportedItems)
	}
	if _, ok := rows.Report.UnsupportedItems["category_4_type_20"]; ok {
		t.Fatalf("unsupported item report = %+v, want repair bots mapped as utility modules", rows.Report.UnsupportedItems)
	}
	if got, want := rows.Report.UnsupportedItems["category_4_type_18"], 1; got != want {
		t.Fatalf("unsupported category_4_type_18 = %d, want %d trade drone row still tracked", got, want)
	}
	if got, want := rows.Report.UnsupportedItems["category_4_type_19"], 36; got != want {
		t.Fatalf("unsupported category_4_type_19 = %d, want %d special CPU rows still tracked", got, want)
	}
}

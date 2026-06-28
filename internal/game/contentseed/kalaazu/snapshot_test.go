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
		rows.Report.ImportedRows[content.ContentTypeModule] != 21 ||
		rows.Report.ImportedRows[content.ContentTypeShip] != 17 ||
		rows.Report.ImportedRows[content.ContentTypeCraftRecipe] != 3 ||
		rows.Report.ImportedRows[content.ContentTypeProductionBuilding] != 3 ||
		rows.Report.ImportedRows[content.ContentTypeProductionRules] != 1 ||
		rows.Report.ImportedRows[content.ContentTypeCombatRules] != 1 ||
		rows.Report.ImportedRows[content.ContentTypeEnemyPool] != 11 ||
		rows.Report.ImportedRows[content.ContentTypeScannerConfig] != 1 ||
		rows.Report.ImportedRows[content.ContentTypeStarterConfig] != 1 ||
		rows.Report.ImportedRows[content.ContentTypeRoutePolicy] != 1 {
		t.Fatalf("imported report = %+v, want starter maps/modules/ships/enemy pools", rows.Report.ImportedRows)
	}
	if len(rows.Report.UnsupportedItems) == 0 {
		t.Fatalf("unsupported item report empty, want unsupported equipment categories counted")
	}
}

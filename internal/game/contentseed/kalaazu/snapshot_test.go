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
	if got, want := rows.Report.SourceRows["items"], len(rows.ItemRows); got != want {
		t.Fatalf("item source/imported rows = %d/%d, want all items imported as definitions", got, want)
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
		rows.Report.ImportedRows[content.ContentTypeModule] != 18 ||
		rows.Report.ImportedRows[content.ContentTypeShip] != 17 ||
		rows.Report.ImportedRows[content.ContentTypeEnemyPool] != 11 {
		t.Fatalf("imported report = %+v, want starter maps/modules/ships/enemy pools", rows.Report.ImportedRows)
	}
	if len(rows.Report.UnsupportedItems) == 0 {
		t.Fatalf("unsupported item report empty, want unsupported equipment categories counted")
	}
}

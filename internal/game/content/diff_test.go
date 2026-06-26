package content

import (
	"encoding/json"
	"testing"
)

func snapshotRow(id string, enabled bool, data string) SnapshotRow {
	return SnapshotRow{
		ContentID: ContentID(id),
		Enabled:   enabled,
		DataJSON:  json.RawMessage(data),
	}
}

func TestDiffSnapshotsClassifiesAddedRemovedModified(t *testing.T) {
	base := Snapshot{
		Version: "base",
		Items: []SnapshotRow{
			snapshotRow("item_unchanged", true, `{"stackable":true}`),
			snapshotRow("item_modified", true, `{"stackable":true}`),
			snapshotRow("item_removed", true, `{"stackable":true}`),
		},
	}
	target := Snapshot{
		Version: "target",
		Items: []SnapshotRow{
			snapshotRow("item_unchanged", true, `{"stackable":true}`),
			snapshotRow("item_modified", true, `{"stackable":false}`),
			snapshotRow("item_added", true, `{"stackable":true}`),
		},
	}

	entries := DiffSnapshots(base, target)
	byID := make(map[string]DiffEntry, len(entries))
	for _, entry := range entries {
		byID[string(entry.ContentID)] = entry
	}

	if len(entries) != 3 {
		t.Fatalf("diff entries = %d, want 3 (added/removed/modified)", len(entries))
	}
	if added, ok := byID["item_added"]; !ok || added.Change != DiffChangeAdded || added.OldValueJSON != nil || added.NewValueJSON == nil {
		t.Fatalf("added entry = %+v, want add with new value only", added)
	}
	if removed, ok := byID["item_removed"]; !ok || removed.Change != DiffChangeRemoved || removed.NewValueJSON != nil || removed.OldValueJSON == nil {
		t.Fatalf("removed entry = %+v, want remove with old value only", removed)
	}
	if modified, ok := byID["item_modified"]; !ok || modified.Change != DiffChangeModified || modified.OldValueJSON == nil || modified.NewValueJSON == nil {
		t.Fatalf("modified entry = %+v, want modified with old and new", modified)
	}
	if _, ok := byID["item_unchanged"]; ok {
		t.Fatalf("unchanged row should not appear in diff")
	}
}

func TestDiffSnapshotsDetectsEnabledToggleAsModified(t *testing.T) {
	base := Snapshot{Version: "base", Items: []SnapshotRow{snapshotRow("item_toggle", true, `{"stackable":true}`)}}
	target := Snapshot{Version: "target", Items: []SnapshotRow{snapshotRow("item_toggle", false, `{"stackable":true}`)}}

	entries := DiffSnapshots(base, target)
	if len(entries) != 1 || entries[0].Change != DiffChangeModified {
		t.Fatalf("entries = %+v, want single modified for enabled toggle", entries)
	}
}

func TestDiffSnapshotsIgnoresWhitespaceOnlyJSONDifference(t *testing.T) {
	base := Snapshot{Version: "base", Items: []SnapshotRow{snapshotRow("item_ws", true, `{"stackable":true}`)}}
	target := Snapshot{Version: "target", Items: []SnapshotRow{snapshotRow("item_ws", true, "{\n  \"stackable\": true\n}")}}

	if entries := DiffSnapshots(base, target); len(entries) != 0 {
		t.Fatalf("entries = %+v, want no diff for whitespace-only JSON", entries)
	}
}

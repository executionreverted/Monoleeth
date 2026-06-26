package content

import (
	"bytes"
	"encoding/json"
)

// Diff change classifications recorded per changed content id.
const (
	DiffChangeAdded    = "added"
	DiffChangeRemoved  = "removed"
	DiffChangeModified = "modified"
)

// DiffInput selects the two snapshots to compare. Base/Target accept either a
// content version id, or the reserved selectors DiffSelectorCurrent / DiffSelectorDraft.
type DiffInput struct {
	BaseVersionID   string
	TargetVersionID string
}

// Reserved diff snapshot selectors.
const (
	DiffSelectorCurrent = "current"
	DiffSelectorDraft   = "draft"
)

// DiffEntry is a single content row change between two snapshots. OldValueJSON
// is empty for added rows; NewValueJSON is empty for removed rows. Payloads are
// already client/admin-safe (secrets scrubbed by the service layer).
type DiffEntry struct {
	ContentType  ContentType     `json:"content_type"`
	ContentID    ContentID       `json:"content_id"`
	Change       string          `json:"change"`
	OldValueJSON json.RawMessage `json:"old_value_json,omitempty"`
	NewValueJSON json.RawMessage `json:"new_value_json,omitempty"`
}

// DiffResult is the admin content diff response.
type DiffResult struct {
	BaseVersionID   string      `json:"base_version_id"`
	TargetVersionID string      `json:"target_version_id"`
	Entries         []DiffEntry `json:"entries"`
	Total           int         `json:"total"`
}

// DiffSnapshots computes the row-level changes from base to target. It is a pure
// function over snapshot groups so callers can diff any two resolved snapshots
// (published-vs-draft, version-vs-version). Row equivalence matches the publish
// safety check: enabled flag plus compacted display/data JSON.
func DiffSnapshots(base Snapshot, target Snapshot) []DiffEntry {
	var entries []DiffEntry
	for _, group := range base.Groups() {
		targetGroup := snapshotGroup(target, group.Type)
		entries = appendDiffEntries(entries, group.Type, group.Rows, targetGroup)
	}
	for _, group := range target.Groups() {
		if hasSnapshotGroup(base, group.Type) {
			continue
		}
		baseGroup := snapshotGroup(base, group.Type)
		entries = appendDiffEntries(entries, group.Type, baseGroup, group.Rows)
	}
	return entries
}

func snapshotGroup(snapshot Snapshot, contentType ContentType) []SnapshotRow {
	for _, group := range snapshot.Groups() {
		if group.Type == contentType {
			return group.Rows
		}
	}
	return nil
}

func hasSnapshotGroup(snapshot Snapshot, contentType ContentType) bool {
	for _, group := range snapshot.Groups() {
		if group.Type == contentType {
			return true
		}
	}
	return false
}

func appendDiffEntries(entries []DiffEntry, contentType ContentType, base []SnapshotRow, target []SnapshotRow) []DiffEntry {
	baseByID := snapshotRowsByID(contentType, base)
	targetByID := snapshotRowsByID(contentType, target)
	seen := make(map[ContentID]struct{}, len(baseByID)+len(targetByID))
	for id := range baseByID {
		seen[id] = struct{}{}
	}
	for id := range targetByID {
		seen[id] = struct{}{}
	}
	for id := range seen {
		baseRow, baseOK := baseByID[id]
		targetRow, targetOK := targetByID[id]
		switch {
		case baseOK && !targetOK:
			entries = append(entries, DiffEntry{
				ContentType:  contentType,
				ContentID:    id,
				Change:       DiffChangeRemoved,
				OldValueJSON: diffRowJSON(baseRow),
			})
		case !baseOK && targetOK:
			entries = append(entries, DiffEntry{
				ContentType:  contentType,
				ContentID:    id,
				Change:       DiffChangeAdded,
				NewValueJSON: diffRowJSON(targetRow),
			})
		case !diffRowsEquivalent(baseRow, targetRow):
			entries = append(entries, DiffEntry{
				ContentType:  contentType,
				ContentID:    id,
				Change:       DiffChangeModified,
				OldValueJSON: diffRowJSON(baseRow),
				NewValueJSON: diffRowJSON(targetRow),
			})
		}
	}
	return entries
}

func snapshotRowsByID(_ ContentType, rows []SnapshotRow) map[ContentID]SnapshotRow {
	out := make(map[ContentID]SnapshotRow, len(rows))
	for _, row := range rows {
		out[row.ContentID] = row
	}
	return out
}

func diffRowJSON(row SnapshotRow) json.RawMessage {
	encoded, err := json.Marshal(row)
	if err != nil {
		return nil
	}
	return encoded
}

// diffRowsEquivalent mirrors the publish-safety row equivalence check: enabled
// flag plus compacted display/data JSON. Kept local to the content package so
// DiffSnapshots stays a pure function without importing the admin layer.
func diffRowsEquivalent(left SnapshotRow, right SnapshotRow) bool {
	if left.Enabled != right.Enabled {
		return false
	}
	leftData, leftDataOK := compactDiffJSON(left.DataJSON)
	rightData, rightDataOK := compactDiffJSON(right.DataJSON)
	leftDisplay, leftDisplayOK := compactDiffJSON(left.DisplayJSON)
	rightDisplay, rightDisplayOK := compactDiffJSON(right.DisplayJSON)
	return leftDataOK && rightDataOK && leftDisplayOK && rightDisplayOK &&
		bytes.Equal(leftData, rightData) && bytes.Equal(leftDisplay, rightDisplay)
}

func compactDiffJSON(raw json.RawMessage) ([]byte, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, true
	}
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		return nil, false
	}
	return buffer.Bytes(), true
}

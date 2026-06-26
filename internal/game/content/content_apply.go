package content

import (
	"bytes"
	"encoding/json"
)

// ContentApplyClass describes whether a content change can reach the live
// runtime without a process restart.
type ContentApplyClass string

const (
	// ApplyClassSafeReload means every changed content type feeds the
	// player-facing catalog projection, which the runtime can recompute and
	// swap atomically without touching boot-wired authoritative services.
	ApplyClassSafeReload ContentApplyClass = "safe_reload"
	// ApplyClassRestartRequired means at least one changed content type is
	// wired into authoritative runtime services at boot (combat, economy,
	// world). It cannot be reflected until a restart; the runtime must report
	// pending_restart honestly instead of silently drifting.
	ApplyClassRestartRequired ContentApplyClass = "restart_required"
)

// safeReloadContentTypes are the snapshot groups that only feed the player
// content projection. They can be live-reloaded because the projection is
// presentational; server-authoritative gameplay truth stays boot-wired.
var safeReloadContentTypes = map[ContentType]bool{
	ContentTypeItem:        true,
	ContentTypeModule:      true,
	ContentTypeShopProduct: true,
}

// RuntimeApplyPlan is the domain decision produced by comparing the currently
// live content snapshot against the one about to be published. It tells the
// runtime layer whether it may hot-swap the player projection or must defer to
// a restart.
type RuntimeApplyPlan struct {
	Class               ContentApplyClass
	ChangedContentTypes []ContentType
}

// SafeReload reports whether the plan allows a live runtime apply.
func (plan RuntimeApplyPlan) SafeReload() bool {
	return plan.Class == ApplyClassSafeReload
}

// PlanRuntimeApply compares the currently live snapshot against the next
// snapshot and returns the runtime apply plan. A change is safe to live-reload
// only when every changed content type is projection-only; any boot-wired
// (restart-required) type forces a restart.
func PlanRuntimeApply(current Snapshot, next Snapshot) RuntimeApplyPlan {
	changed := changedSnapshotContentTypes(current, next)
	class := ApplyClassSafeReload
	for _, contentType := range changed {
		if !safeReloadContentTypes[contentType] {
			class = ApplyClassRestartRequired
			break
		}
	}
	return RuntimeApplyPlan{Class: class, ChangedContentTypes: changed}
}

func changedSnapshotContentTypes(current Snapshot, next Snapshot) []ContentType {
	var changed []ContentType
	for _, group := range current.Groups() {
		if !snapshotRowsEqualByID(group.Rows, rowsForContentType(next, group.Type)) {
			changed = append(changed, group.Type)
		}
	}
	return changed
}

func rowsForContentType(snapshot Snapshot, contentType ContentType) []SnapshotRow {
	for _, group := range snapshot.Groups() {
		if group.Type == contentType {
			return group.Rows
		}
	}
	return nil
}

func snapshotRowsEqualByID(left []SnapshotRow, right []SnapshotRow) bool {
	leftByID := snapshotRowMap(left)
	rightByID := snapshotRowMap(right)
	if len(leftByID) != len(rightByID) {
		return false
	}
	for id, leftRow := range leftByID {
		rightRow, ok := rightByID[id]
		if !ok || !snapshotRowEquivalent(leftRow, rightRow) {
			return false
		}
	}
	return true
}

func snapshotRowMap(rows []SnapshotRow) map[ContentID]SnapshotRow {
	out := make(map[ContentID]SnapshotRow, len(rows))
	for _, row := range rows {
		out[row.ContentID] = row
	}
	return out
}

func snapshotRowEquivalent(left SnapshotRow, right SnapshotRow) bool {
	if left.Enabled != right.Enabled {
		return false
	}
	leftData, leftOK := compactApplyJSON(left.DataJSON)
	rightData, rightOK := compactApplyJSON(right.DataJSON)
	leftDisplay, leftDisplayOK := compactApplyJSON(left.DisplayJSON)
	rightDisplay, rightDisplayOK := compactApplyJSON(right.DisplayJSON)
	return leftOK && rightOK && leftDisplayOK && rightDisplayOK &&
		equalApplyJSON(leftData, rightData) && equalApplyJSON(leftDisplay, rightDisplay)
}

func compactApplyJSON(raw json.RawMessage) ([]byte, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, true
	}
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, raw); err != nil {
		return nil, false
	}
	return buffer.Bytes(), true
}

func equalApplyJSON(left []byte, right []byte) bool {
	return bytes.Equal(left, right)
}

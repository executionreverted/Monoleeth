package stats

import (
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

func TestStatSnapshotTracksVersionAndInvalidation(t *testing.T) {
	createdAt := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	invalidatedAt := createdAt.Add(time.Minute)
	snapshot := NewStatSnapshot(
		foundation.PlayerID("player_1"),
		foundation.ShipID("ship_1"),
		SnapshotVersion(7),
		EffectiveStats{Core: CoreStats{CargoCapacity: 100}},
		createdAt,
	)

	if snapshot.IsInvalidated() {
		t.Fatal("new snapshot IsInvalidated() = true, want false")
	}

	invalidated := snapshot.Invalidate(invalidatedAt)
	if !invalidated.IsInvalidated() {
		t.Fatal("invalidated snapshot IsInvalidated() = false, want true")
	}
	if invalidated.InvalidatedAt == nil || !invalidated.InvalidatedAt.Equal(invalidatedAt) {
		t.Fatalf("InvalidatedAt = %v, want %v", invalidated.InvalidatedAt, invalidatedAt)
	}
	if snapshot.IsInvalidated() {
		t.Fatal("original snapshot mutated after Invalidate")
	}
	if invalidated.Version != SnapshotVersion(7) {
		t.Fatalf("Version = %d, want 7", invalidated.Version)
	}
}

func TestInvalidationStateRecordsReasonAndRecalculation(t *testing.T) {
	recalculatedAt := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	invalidatedAt := recalculatedAt.Add(time.Minute)
	nextRecalculatedAt := recalculatedAt.Add(2 * time.Minute)
	state := NewInvalidationState(SnapshotVersion(3), recalculatedAt)

	state = state.Invalidate(InvalidationReasonModuleEquipped, invalidatedAt)
	if !state.Invalidated {
		t.Fatal("Invalidated = false, want true")
	}
	if state.InvalidatedVersion != SnapshotVersion(3) {
		t.Fatalf("InvalidatedVersion = %d, want 3", state.InvalidatedVersion)
	}
	if state.Reason != InvalidationReasonModuleEquipped {
		t.Fatalf("Reason = %q, want %q", state.Reason, InvalidationReasonModuleEquipped)
	}
	if state.InvalidatedAt == nil || !state.InvalidatedAt.Equal(invalidatedAt) {
		t.Fatalf("InvalidatedAt = %v, want %v", state.InvalidatedAt, invalidatedAt)
	}

	state = state.MarkRecalculated(state.CurrentVersion.Next(), nextRecalculatedAt)
	if state.Invalidated {
		t.Fatal("Invalidated = true, want false")
	}
	if state.CurrentVersion != SnapshotVersion(4) {
		t.Fatalf("CurrentVersion = %d, want 4", state.CurrentVersion)
	}
	if state.InvalidatedVersion != 0 {
		t.Fatalf("InvalidatedVersion = %d, want 0", state.InvalidatedVersion)
	}
	if state.Reason != "" {
		t.Fatalf("Reason = %q, want empty", state.Reason)
	}
	if state.InvalidatedAt != nil {
		t.Fatalf("InvalidatedAt = %v, want nil", state.InvalidatedAt)
	}
	if state.LastRecalculatedAt == nil || !state.LastRecalculatedAt.Equal(nextRecalculatedAt) {
		t.Fatalf("LastRecalculatedAt = %v, want %v", state.LastRecalculatedAt, nextRecalculatedAt)
	}
}

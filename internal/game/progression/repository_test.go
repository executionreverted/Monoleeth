package progression

import (
	"context"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestProgressionRepositoryBackedStoreReloadsGrantedXP(t *testing.T) {
	repository := &fakeProgressionRepository{}
	clock := testutil.NewFakeClock(time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC))
	service := NewProgressionService(clock, mustProgressionStoreWithRepository(t, repository))

	if _, err := service.GrantXP(GrantXPInput{
		PlayerID:       foundation.PlayerID("player-progression-repo"),
		Amount:         125,
		SourceType:     XPSourceTypeAdminAdjustment,
		SourceID:       XPSourceID("repo-reload"),
		IdempotencyKey: XPIdempotencyKey("admin_xp:repo-reload"),
		Authority:      XPGrantAuthorityAdminService,
	}); err != nil {
		t.Fatalf("GrantXP() error = %v, want nil", err)
	}

	reloaded := NewProgressionService(clock, mustProgressionStoreWithRepository(t, repository))
	snapshot, err := reloaded.GetProgressionSnapshot(foundation.PlayerID("player-progression-repo"))
	if err != nil {
		t.Fatalf("GetProgressionSnapshot(reloaded) error = %v, want nil", err)
	}
	if snapshot.Player.MainXP != 125 {
		t.Fatalf("MainXP(reloaded) = %d, want 125", snapshot.Player.MainXP)
	}
}

func mustProgressionStoreWithRepository(t *testing.T, repository Repository) *InMemoryProgressionStore {
	t.Helper()
	store, err := NewInMemoryProgressionStoreWithRepository(context.Background(), repository)
	if err != nil {
		t.Fatalf("NewInMemoryProgressionStoreWithRepository() error = %v, want nil", err)
	}
	return store
}

type fakeProgressionRepository struct {
	snapshots []ProgressionSnapshot
}

func (repository *fakeProgressionRepository) LoadProgressionSnapshots(context.Context) ([]ProgressionSnapshot, error) {
	out := make([]ProgressionSnapshot, len(repository.snapshots))
	for index, snapshot := range repository.snapshots {
		out[index] = snapshot.Clone()
	}
	return out, nil
}

func (repository *fakeProgressionRepository) SaveProgressionSnapshot(_ context.Context, snapshot ProgressionSnapshot) error {
	for index, existing := range repository.snapshots {
		if existing.Player.PlayerID == snapshot.Player.PlayerID {
			repository.snapshots[index] = snapshot.Clone()
			return nil
		}
	}
	repository.snapshots = append(repository.snapshots, snapshot.Clone())
	return nil
}

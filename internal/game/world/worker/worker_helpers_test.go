package worker

import (
	"math"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
)

func newTestWorker(t *testing.T, tickDelta time.Duration) *Worker {
	t.Helper()

	zoneWorker, err := NewWorker(Config{
		WorldID:   "world-1",
		ZoneID:    "zone-1",
		TickDelta: tickDelta,
		Clock:     testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)),
	})
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}
	return zoneWorker
}

func spawnPlayer(t *testing.T, zoneWorker *Worker, playerID foundation.PlayerID, entityID world.EntityID, position world.Vec2, speed float64) {
	t.Helper()

	if err := zoneWorker.Submit(SpawnPlayerCommand{
		PlayerID: playerID,
		EntityID: entityID,
		Position: position,
		Speed:    speed,
	}); err != nil {
		t.Fatalf("Submit(spawn) error = %v", err)
	}
	assertNoCommandErrors(t, zoneWorker.Tick())
}

func mustInsertWorkerEntity(t *testing.T, zoneWorker *Worker, entityID world.EntityID, entityType world.EntityType, position world.Vec2) {
	t.Helper()

	entity, err := world.NewEntity(zoneWorker.WorldID(), zoneWorker.ZoneID(), entityID, entityType, position)
	if err != nil {
		t.Fatalf("NewEntity(%q) = %v, want nil", entityID, err)
	}
	if err := zoneWorker.InsertEntity(entity, 0); err != nil {
		t.Fatalf("InsertEntity(%q) = %v, want nil", entityID, err)
	}
}

func workerSnapshotEntityIDs(snapshot Snapshot) []world.EntityID {
	ids := make([]world.EntityID, 0, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		ids = append(ids, entity.ID)
	}
	return ids
}

func hasWorkerSnapshotEntityID(snapshot Snapshot, entityID world.EntityID) bool {
	for _, entity := range snapshot.Entities {
		if entity.ID == entityID {
			return true
		}
	}
	return false
}

func tickSubmitted(t *testing.T, zoneWorker *Worker, command Command) TickResult {
	t.Helper()

	if err := zoneWorker.Submit(command); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	return zoneWorker.Tick()
}

func mustMovementIntent(t *testing.T, target world.Vec2) world.MovementIntent {
	t.Helper()

	intent, err := world.NewMovementIntent(target)
	if err != nil {
		t.Fatalf("NewMovementIntent() error = %v", err)
	}
	return intent
}

func assertNoCommandErrors(t *testing.T, result TickResult) {
	t.Helper()

	if len(result.CommandErrors) != 0 {
		t.Fatalf("command errors = %+v, want none", result.CommandErrors)
	}
}

func assertVecNear(t *testing.T, got world.Vec2, want world.Vec2) {
	t.Helper()

	const tolerance = 1e-9
	if math.Abs(got.X-want.X) > tolerance || math.Abs(got.Y-want.Y) > tolerance {
		t.Fatalf("position = %+v, want %+v", got, want)
	}
}

func exportedFieldNames(structType reflect.Type) []string {
	fields := make([]string, 0, structType.NumField())
	for index := 0; index < structType.NumField(); index++ {
		field := structType.Field(index)
		if field.IsExported() {
			fields = append(fields, field.Name)
		}
	}
	return fields
}

func taskIDs(tasks []ScheduledTask) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

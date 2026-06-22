package worker

import (
	"errors"
	"testing"
)

func TestEnemySpawnerInitializeRejectsInternalMapZoneMismatchBeforeMutation(t *testing.T) {
	definition := testEnemyMapDefinition()
	definition.InternalMapID = "zone-2"
	zoneWorker := newWorkerForMapDefinition(t, definition)

	result := tickSubmitted(t, zoneWorker, InitializeEnemyPoolsCommand{Definition: definition})

	assertEnemyPoolOwnershipCommandError(t, result)
	assertNoEnemySpawnerRowsOrEntities(t, zoneWorker)
}

func assertEnemyPoolOwnershipCommandError(t *testing.T, result TickResult) {
	t.Helper()

	if len(result.CommandErrors) != 1 || !errors.Is(result.CommandErrors[0].Err, ErrInvalidWorkerConfig) {
		t.Fatalf("command errors = %+v, want one wrapping %v", result.CommandErrors, ErrInvalidWorkerConfig)
	}
}

func assertNoEnemySpawnerRowsOrEntities(t *testing.T, zoneWorker *Worker) {
	t.Helper()

	snapshot := zoneWorker.EnemySpawnSnapshot()
	if len(snapshot.Records) != 0 || snapshot.MapAliveCount != 0 || len(zoneWorker.Snapshot().Entities) != 0 {
		t.Fatalf("snapshot=%+v entities=%+v, want no spawner rows or entities", snapshot, zoneWorker.Snapshot().Entities)
	}
}

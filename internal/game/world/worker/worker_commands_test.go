package worker

import (
	"testing"
	"time"

	"gameproject/internal/game/world"
)

func TestCommandDrainOrderIsDeterministic(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)
	spawnPlayer(t, zoneWorker, "player-1", "entity-player-1", world.Vec2{}, 10)

	commands := []Command{
		MoveToCommand{PlayerID: "player-1", Intent: mustMovementIntent(t, world.Vec2{X: 100, Y: 0})},
		StopCommand{PlayerID: "player-1"},
		MoveToCommand{PlayerID: "player-1", Intent: mustMovementIntent(t, world.Vec2{X: 0, Y: 100})},
	}
	for _, command := range commands {
		if err := zoneWorker.Submit(command); err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
	}

	result := zoneWorker.Tick()
	assertNoCommandErrors(t, result)
	if result.DrainedCommands != len(commands) {
		t.Fatalf("DrainedCommands = %d, want %d", result.DrainedCommands, len(commands))
	}

	entity, ok := zoneWorker.PlayerEntity("player-1")
	if !ok {
		t.Fatal("PlayerEntity() ok = false, want true")
	}
	assertVecNear(t, entity.Position, world.Vec2{X: 0, Y: 10})
	if entity.Movement.Target != (world.Vec2{X: 0, Y: 100}) {
		t.Fatalf("movement target = %+v, want last command target", entity.Movement.Target)
	}
}

func TestWorkerEntityInsertUpdateRemove(t *testing.T) {
	zoneWorker := newTestWorker(t, time.Second)

	entity, err := world.NewEntity("world-1", "zone-1", "entity-npc-1", world.EntityTypeNPCPlaceholder, world.Vec2{X: 1, Y: 2})
	if err != nil {
		t.Fatalf("NewEntity() error = %v", err)
	}
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, InsertEntityCommand{Entity: entity}))

	updated := entity
	updated.Position = world.Vec2{X: 3, Y: 4}
	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, UpdateEntityCommand{Entity: updated}))

	got, ok := zoneWorker.Entity("entity-npc-1")
	if !ok {
		t.Fatal("Entity() ok = false, want true")
	}
	if got.Position != updated.Position {
		t.Fatalf("updated position = %+v, want %+v", got.Position, updated.Position)
	}

	assertNoCommandErrors(t, tickSubmitted(t, zoneWorker, RemoveEntityCommand{EntityID: "entity-npc-1"}))
	if _, ok := zoneWorker.Entity("entity-npc-1"); ok {
		t.Fatal("Entity() ok = true after remove, want false")
	}
}

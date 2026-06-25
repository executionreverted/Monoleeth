package worker

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
)

func TestWorkerConcurrentMoveTickReadRace(t *testing.T) {
	zoneWorker := newTestWorker(t, 10*time.Millisecond)
	if err := zoneWorker.Submit(SpawnPlayerCommand{
		PlayerID:  "player-1",
		EntityID:  "entity-player-1",
		Position:  world.Vec2{},
		Speed:     120,
		SessionID: "session-1",
	}); err != nil {
		t.Fatalf("Submit(spawn) error = %v", err)
	}
	assertNoCommandErrors(t, zoneWorker.Tick())

	intentRight := mustMovementIntent(t, world.Vec2{X: 1000, Y: 0})
	intentLeft := mustMovementIntent(t, world.Vec2{X: -1000, Y: 0})
	start := make(chan struct{})
	errs := make(chan error, 3)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for index := 0; index < 250; index++ {
			intent := intentRight
			if index%2 == 1 {
				intent = intentLeft
			}
			if err := zoneWorker.Submit(MoveToCommand{PlayerID: "player-1", Intent: intent}); err != nil {
				errs <- fmt.Errorf("submit move %d: %w", index, err)
				return
			}
			runtime.Gosched()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for index := 0; index < 250; index++ {
			result := zoneWorker.Tick()
			if len(result.CommandErrors) != 0 {
				errs <- fmt.Errorf("tick %d command errors: %+v", index, result.CommandErrors)
				return
			}
			runtime.Gosched()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for index := 0; index < 250; index++ {
			if _, ok := zoneWorker.PlayerEntity("player-1"); !ok {
				errs <- fmt.Errorf("read %d: missing player entity", index)
				return
			}
			if _, ok := zoneWorker.Entity("entity-player-1"); !ok {
				errs <- fmt.Errorf("read %d: missing entity", index)
				return
			}
			if _, ok := zoneWorker.EntitySpeed("entity-player-1"); !ok {
				errs <- fmt.Errorf("read %d: missing entity speed", index)
				return
			}
			if playerID, ok := zoneWorker.AttachedPlayer("session-1"); !ok || playerID != "player-1" {
				errs <- fmt.Errorf("read %d: attached player = %q ok=%t", index, playerID, ok)
				return
			}
			if sessions := zoneWorker.PlayerSessions("player-1"); len(sessions) != 1 || sessions[0] != "session-1" {
				errs <- fmt.Errorf("read %d: sessions = %v", index, sessions)
				return
			}
			if snapshot := zoneWorker.Snapshot(); len(snapshot.Entities) == 0 {
				errs <- fmt.Errorf("read %d: empty snapshot", index)
				return
			}
			if snapshot, err := zoneWorker.EntitiesWithinWindow(world.Vec2{}, 2000); err != nil || len(snapshot.Entities) == 0 {
				errs <- fmt.Errorf("read %d: window snapshot len=%d err=%v", index, len(snapshot.Entities), err)
				return
			}
			if snapshot, err := zoneWorker.EntitiesWithinRadius(world.Vec2{}, 2000); err != nil || len(snapshot.Entities) == 0 {
				errs <- fmt.Errorf("read %d: radius snapshot len=%d err=%v", index, len(snapshot.Entities), err)
				return
			}
			runtime.Gosched()
		}
	}()

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkerConcurrentCommandTickSameAndDifferentMapsRace(t *testing.T) {
	t.Run("same map", func(t *testing.T) {
		zoneWorker := newTestWorker(t, 10*time.Millisecond)
		seedRaceGateWorker(t, zoneWorker, "player-same", "entity-player-same", "session-same")

		runConcurrentCommandTickRaceGate(t, []raceGateWorker{{
			worker:   zoneWorker,
			playerID: "player-same",
			entityID: "entity-player-same",
		}})
	})

	t.Run("different maps", func(t *testing.T) {
		mapA := newTestWorker(t, 10*time.Millisecond)
		mapB := newTestWorkerForZone(t, "zone-2", 10*time.Millisecond)
		seedRaceGateWorker(t, mapA, "player-map-a", "entity-player-map-a", "session-map-a")
		seedRaceGateWorker(t, mapB, "player-map-b", "entity-player-map-b", "session-map-b")

		runConcurrentCommandTickRaceGate(t, []raceGateWorker{
			{worker: mapA, playerID: "player-map-a", entityID: "entity-player-map-a"},
			{worker: mapB, playerID: "player-map-b", entityID: "entity-player-map-b"},
		})
	})
}

type raceGateWorker struct {
	worker   *Worker
	playerID foundation.PlayerID
	entityID world.EntityID
}

func runConcurrentCommandTickRaceGate(t *testing.T, workers []raceGateWorker) {
	t.Helper()

	const iterations = 250
	start := make(chan struct{})
	errs := make(chan error, len(workers)*2)
	var wg sync.WaitGroup

	for _, gate := range workers {
		gate := gate
		intentRight := mustMovementIntent(t, world.Vec2{X: 1000, Y: 0})
		intentLeft := mustMovementIntent(t, world.Vec2{X: -1000, Y: 0})

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for index := 0; index < iterations; index++ {
				intent := intentRight
				if index%2 == 1 {
					intent = intentLeft
				}
				if err := gate.worker.Submit(MoveToCommand{PlayerID: gate.playerID, Intent: intent}); err != nil {
					errs <- fmt.Errorf("%s submit move %d: %w", gate.playerID, index, err)
					return
				}
				runtime.Gosched()
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for index := 0; index < iterations; index++ {
				result := gate.worker.Tick()
				if len(result.CommandErrors) != 0 {
					errs <- fmt.Errorf("%s tick %d command errors: %+v", gate.playerID, index, result.CommandErrors)
					return
				}
				if _, ok := gate.worker.PlayerEntity(gate.playerID); !ok {
					errs <- fmt.Errorf("%s tick %d missing player entity", gate.playerID, index)
					return
				}
				if _, ok := gate.worker.Entity(gate.entityID); !ok {
					errs <- fmt.Errorf("%s tick %d missing entity %s", gate.playerID, index, gate.entityID)
					return
				}
				runtime.Gosched()
			}
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func seedRaceGateWorker(t *testing.T, zoneWorker *Worker, playerID foundation.PlayerID, entityID world.EntityID, sessionID realtime.SessionID) {
	t.Helper()

	if err := zoneWorker.Submit(SpawnPlayerCommand{
		PlayerID:  playerID,
		EntityID:  entityID,
		Position:  world.Vec2{},
		Speed:     120,
		SessionID: sessionID,
	}); err != nil {
		t.Fatalf("Submit(spawn %s) error = %v", playerID, err)
	}
	assertNoCommandErrors(t, zoneWorker.Tick())
}

func newTestWorkerForZone(t *testing.T, zoneID world.ZoneID, tickDelta time.Duration) *Worker {
	t.Helper()

	zoneWorker, err := NewWorker(Config{
		WorldID:   "world-1",
		ZoneID:    zoneID,
		TickDelta: tickDelta,
	})
	if err != nil {
		t.Fatalf("NewWorker(%q) error = %v", zoneID, err)
	}
	return zoneWorker
}

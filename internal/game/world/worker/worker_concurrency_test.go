package worker

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

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

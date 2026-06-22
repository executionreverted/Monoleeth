package worker

import (
	"fmt"
	"sync"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
)

// Command is a single intent or server-side mutation for the owning worker.
type Command interface {
	apply(worker *Worker) error
}

// Mailbox accepts commands from the in-process harness and drains them for a
// worker tick.
type Mailbox interface {
	Submit(command Command) error
	Drain() []Command
}

// FIFOCommandMailbox is a thread-safe in-memory command queue.
type FIFOCommandMailbox struct {
	mu       sync.Mutex
	commands []Command
}

// NewFIFOCommandMailbox returns an empty FIFO command mailbox.
func NewFIFOCommandMailbox() *FIFOCommandMailbox {
	return &FIFOCommandMailbox{
		commands: make([]Command, 0),
	}
}

// Submit appends command to the mailbox in arrival order.
func (mailbox *FIFOCommandMailbox) Submit(command Command) error {
	if command == nil {
		return ErrNilCommand
	}

	mailbox.mu.Lock()
	defer mailbox.mu.Unlock()

	mailbox.commands = append(mailbox.commands, command)
	return nil
}

// Drain removes and returns all currently queued commands in arrival order.
func (mailbox *FIFOCommandMailbox) Drain() []Command {
	mailbox.mu.Lock()
	defer mailbox.mu.Unlock()

	if len(mailbox.commands) == 0 {
		return nil
	}

	commands := append([]Command(nil), mailbox.commands...)
	clear(mailbox.commands)
	mailbox.commands = mailbox.commands[:0]
	return commands
}

// SpawnPlayerCommand inserts a player entity and records its server-owned speed.
type SpawnPlayerCommand struct {
	PlayerID  foundation.PlayerID
	EntityID  world.EntityID
	Position  world.Vec2
	Speed     float64
	SessionID realtime.SessionID
}

func (command SpawnPlayerCommand) apply(worker *Worker) error {
	entity, err := world.NewEntity(worker.worldID, worker.zoneID, command.EntityID, world.EntityTypePlayer, command.Position)
	if err != nil {
		return err
	}
	if err := worker.insertEntity(entity, command.Speed); err != nil {
		return err
	}
	if err := worker.attachPlayerEntity(command.PlayerID, command.EntityID); err != nil {
		worker.removeEntity(command.EntityID)
		return err
	}
	if command.SessionID != "" {
		if err := worker.attachSession(command.SessionID, command.PlayerID); err != nil {
			worker.detachPlayerEntity(command.PlayerID)
			worker.removeEntity(command.EntityID)
			return err
		}
	}
	return nil
}

// AttachSessionCommand attaches one realtime session to a server-known player.
type AttachSessionCommand struct {
	SessionID realtime.SessionID
	PlayerID  foundation.PlayerID
}

func (command AttachSessionCommand) apply(worker *Worker) error {
	return worker.attachSession(command.SessionID, command.PlayerID)
}

// DetachSessionCommand detaches one realtime session from its player.
type DetachSessionCommand struct {
	SessionID realtime.SessionID
}

func (command DetachSessionCommand) apply(worker *Worker) error {
	return worker.detachSession(command.SessionID)
}

// SettleAndDetachSessionCommand settles in-flight player movement to the
// worker-owned clock before detaching the realtime session.
type SettleAndDetachSessionCommand struct {
	SessionID realtime.SessionID
}

func (command SettleAndDetachSessionCommand) apply(worker *Worker) error {
	return worker.settleAndDetachSession(command.SessionID)
}

// InsertEntityCommand inserts a non-player or server-owned entity.
type InsertEntityCommand struct {
	Entity world.Entity
	Speed  float64
}

func (command InsertEntityCommand) apply(worker *Worker) error {
	return worker.insertEntity(command.Entity, command.Speed)
}

// RemoveEntityCommand removes an entity from the worker and spatial index.
type RemoveEntityCommand struct {
	EntityID world.EntityID
}

func (command RemoveEntityCommand) apply(worker *Worker) error {
	if !worker.removeEntity(command.EntityID) {
		return fmt.Errorf("entity %q: %w", command.EntityID, ErrUnknownEntity)
	}
	return nil
}

// UpdateEntityCommand replaces an existing entity with server-owned state.
type UpdateEntityCommand struct {
	Entity world.Entity
}

func (command UpdateEntityCommand) apply(worker *Worker) error {
	return worker.updateEntity(command.Entity)
}

// MoveToCommand changes a player's server-owned movement target.
type MoveToCommand struct {
	PlayerID foundation.PlayerID
	Intent   world.MovementIntent
}

func (command MoveToCommand) apply(worker *Worker) error {
	return worker.movePlayerTo(command.PlayerID, command.Intent)
}

// StopCommand clears a player's movement target.
type StopCommand struct {
	PlayerID foundation.PlayerID
}

func (command StopCommand) apply(worker *Worker) error {
	return worker.stopPlayer(command.PlayerID)
}

// SetPlayerSpeedCommand changes a player's authoritative movement speed.
type SetPlayerSpeedCommand struct {
	PlayerID foundation.PlayerID
	Speed    float64
}

func (command SetPlayerSpeedCommand) apply(worker *Worker) error {
	return worker.setPlayerSpeed(command.PlayerID, command.Speed)
}

// SetPlayerAggroEligibilityCommand changes server-owned NPC aggro target
// eligibility for a player. Hidden/stealthed players should be marked
// ineligible by runtime-owned state sync before aggro ticks.
type SetPlayerAggroEligibilityCommand struct {
	PlayerID foundation.PlayerID
	Eligible bool
}

func (command SetPlayerAggroEligibilityCommand) apply(worker *Worker) error {
	return worker.setPlayerAggroEligibility(command.PlayerID, command.Eligible)
}

// DebugSpawnNPCCommand inserts an NPC placeholder for local harness tests.
type DebugSpawnNPCCommand struct {
	EntityID world.EntityID
	Position world.Vec2
	Speed    float64
}

func (command DebugSpawnNPCCommand) apply(worker *Worker) error {
	entity, err := world.NewEntity(worker.worldID, worker.zoneID, command.EntityID, world.EntityTypeNPCPlaceholder, command.Position)
	if err != nil {
		return err
	}
	return worker.insertEntity(entity, command.Speed)
}

package worker

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/spatial"
)

const (
	// DefaultTickDelta is the Phase 04 starter simulation step: 20 Hz.
	DefaultTickDelta = 50 * time.Millisecond
	// DefaultSpatialCellSize is a conservative placeholder cell size for the
	// worker-owned spatial index.
	DefaultSpatialCellSize = 128
)

var (
	// ErrNilCommand reports a nil command submitted to the mailbox.
	ErrNilCommand = errors.New("nil worker command")
	// ErrInvalidWorkerConfig reports an invalid worker configuration.
	ErrInvalidWorkerConfig = errors.New("invalid worker config")
	// ErrEntityAlreadyExists reports duplicate entity insertion.
	ErrEntityAlreadyExists = errors.New("entity already exists")
	// ErrUnknownEntity reports a mutation for an entity the worker does not own.
	ErrUnknownEntity = errors.New("unknown entity")
	// ErrPlayerAlreadyExists reports duplicate player attachment.
	ErrPlayerAlreadyExists = errors.New("player already exists")
	// ErrUnknownPlayer reports a command for a player not owned by the worker.
	ErrUnknownPlayer = errors.New("unknown player")
	// ErrInvalidServerSpeed reports a negative, NaN, or infinite server speed.
	ErrInvalidServerSpeed = errors.New("invalid server speed")
	// ErrInvalidSessionID reports a blank or malformed realtime session id.
	ErrInvalidSessionID = errors.New("invalid session id")
)

// Config defines a single zone worker.
type Config struct {
	WorldID         world.WorldID
	ZoneID          world.ZoneID
	TickDelta       time.Duration
	SpatialCellSize float64
	Mailbox         Mailbox
	Clock           foundation.Clock
}

// Worker is a single-owner in-process simulation harness for one zone.
type Worker struct {
	worldID   world.WorldID
	zoneID    world.ZoneID
	tickDelta time.Duration
	mailbox   Mailbox
	clock     foundation.Clock
	index     *spatial.Index

	tick uint64

	entities     map[world.EntityID]world.Entity
	entitySpeeds map[world.EntityID]float64

	playerEntities map[foundation.PlayerID]world.EntityID
	entityPlayers  map[world.EntityID]foundation.PlayerID
	sessionPlayers map[realtime.SessionID]foundation.PlayerID
	playerSessions map[foundation.PlayerID]map[realtime.SessionID]struct{}

	scheduler delayedScheduler
}

// TickResult describes one deterministic worker tick.
type TickResult struct {
	Tick            uint64
	DrainedCommands int
	CommandErrors   []CommandError
	DueTasks        []ScheduledTask
}

// CommandError records a command failure without stopping the rest of the drain.
type CommandError struct {
	Index int
	Err   error
}

// ScheduledTask is a local delayed job owned by the worker.
type ScheduledTask struct {
	ID        string
	DueAt     time.Time
	Kind      string
	SubjectID string
}

// Snapshot is a deterministic copy of worker-owned state.
//
// Snapshot is internal server state and is not client-safe. Use the AOI and
// visibility packages to build filtered payloads for clients.
type Snapshot struct {
	WorldID  world.WorldID
	ZoneID   world.ZoneID
	Tick     uint64
	Entities []world.Entity
}

// NewWorker validates config and returns an empty single-zone worker.
func NewWorker(config Config) (*Worker, error) {
	if err := config.WorldID.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkerConfig, err)
	}
	if err := config.ZoneID.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkerConfig, err)
	}

	tickDelta := config.TickDelta
	if tickDelta == 0 {
		tickDelta = DefaultTickDelta
	}
	if tickDelta < 0 {
		return nil, fmt.Errorf("tick delta %s: %w", tickDelta, ErrInvalidWorkerConfig)
	}

	cellSize := config.SpatialCellSize
	if cellSize == 0 {
		cellSize = DefaultSpatialCellSize
	}
	index, err := spatial.NewIndex(cellSize)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWorkerConfig, err)
	}

	mailbox := config.Mailbox
	if mailbox == nil {
		mailbox = NewFIFOCommandMailbox()
	}

	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}

	return &Worker{
		worldID:        config.WorldID,
		zoneID:         config.ZoneID,
		tickDelta:      tickDelta,
		mailbox:        mailbox,
		clock:          clock,
		index:          index,
		entities:       make(map[world.EntityID]world.Entity),
		entitySpeeds:   make(map[world.EntityID]float64),
		playerEntities: make(map[foundation.PlayerID]world.EntityID),
		entityPlayers:  make(map[world.EntityID]foundation.PlayerID),
		sessionPlayers: make(map[realtime.SessionID]foundation.PlayerID),
		playerSessions: make(map[foundation.PlayerID]map[realtime.SessionID]struct{}),
		scheduler:      newDelayedScheduler(),
	}, nil
}

// WorldID returns the worker's authoritative world id.
func (worker *Worker) WorldID() world.WorldID {
	return worker.worldID
}

// ZoneID returns the worker's authoritative zone id.
func (worker *Worker) ZoneID() world.ZoneID {
	return worker.zoneID
}

// TickDelta returns the fixed simulation delta used by Tick.
func (worker *Worker) TickDelta() time.Duration {
	return worker.tickDelta
}

// Submit appends command to the worker mailbox.
func (worker *Worker) Submit(command Command) error {
	return worker.mailbox.Submit(command)
}

// Tick drains queued commands, advances movement, and drains due local tasks.
func (worker *Worker) Tick() TickResult {
	commands := worker.mailbox.Drain()
	result := TickResult{
		Tick:            worker.tick + 1,
		DrainedCommands: len(commands),
		CommandErrors:   make([]CommandError, 0),
	}

	for index, command := range commands {
		if command == nil {
			result.CommandErrors = append(result.CommandErrors, CommandError{Index: index, Err: ErrNilCommand})
			continue
		}
		if err := command.apply(worker); err != nil {
			result.CommandErrors = append(result.CommandErrors, CommandError{Index: index, Err: err})
		}
	}

	result.CommandErrors = append(result.CommandErrors, worker.advanceMovement()...)
	result.DueTasks = worker.scheduler.drainDue(worker.clock.Now())
	worker.tick++
	return result
}

// Run ticks the worker until ctx is canceled.
func (worker *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(worker.tickDelta)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			worker.Tick()
		}
	}
}

// Entity returns a copy of the server-owned entity state.
func (worker *Worker) Entity(entityID world.EntityID) (world.Entity, bool) {
	entity, ok := worker.entities[entityID]
	return entity, ok
}

// InsertEntity inserts server-owned entity state directly into the worker.
//
// Client-originated gameplay changes should enter through mailbox intent
// commands. This helper is for server harness setup and authoritative systems.
func (worker *Worker) InsertEntity(entity world.Entity, serverSpeed float64) error {
	return worker.insertEntity(entity, serverSpeed)
}

// UpdateEntity replaces server-owned entity state directly in the worker.
func (worker *Worker) UpdateEntity(entity world.Entity) error {
	return worker.updateEntity(entity)
}

// RemoveEntity removes server-owned entity state directly from the worker.
func (worker *Worker) RemoveEntity(entityID world.EntityID) bool {
	return worker.removeEntity(entityID)
}

// PlayerEntity returns the server-owned entity for playerID.
func (worker *Worker) PlayerEntity(playerID foundation.PlayerID) (world.Entity, bool) {
	entityID, ok := worker.playerEntities[playerID]
	if !ok {
		return world.Entity{}, false
	}
	return worker.Entity(entityID)
}

// AttachedPlayer returns the player currently associated with sessionID.
func (worker *Worker) AttachedPlayer(sessionID realtime.SessionID) (foundation.PlayerID, bool) {
	playerID, ok := worker.sessionPlayers[sessionID]
	return playerID, ok
}

// PlayerSessions returns attached session ids for playerID in deterministic order.
func (worker *Worker) PlayerSessions(playerID foundation.PlayerID) []realtime.SessionID {
	sessionsByPlayer := worker.playerSessions[playerID]
	if len(sessionsByPlayer) == 0 {
		return nil
	}

	sessions := make([]realtime.SessionID, 0, len(sessionsByPlayer))
	for sessionID := range sessionsByPlayer {
		sessions = append(sessions, sessionID)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i] < sessions[j]
	})
	return sessions
}

// Snapshot returns a deterministic copy of entities owned by the worker.
func (worker *Worker) Snapshot() Snapshot {
	entities := make([]world.Entity, 0, len(worker.entities))
	for _, entity := range worker.entities {
		entities = append(entities, entity)
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].ID < entities[j].ID
	})

	return Snapshot{
		WorldID:  worker.worldID,
		ZoneID:   worker.zoneID,
		Tick:     worker.tick,
		Entities: entities,
	}
}

// ScheduleTask adds a map-local delayed task and returns the accepted copy.
func (worker *Worker) ScheduleTask(task ScheduledTask) (ScheduledTask, error) {
	if strings.TrimSpace(task.ID) == "" || strings.TrimSpace(task.Kind) == "" {
		return ScheduledTask{}, fmt.Errorf("scheduled task: %w", ErrInvalidWorkerConfig)
	}
	if task.DueAt.IsZero() {
		return ScheduledTask{}, fmt.Errorf("scheduled task due time: %w", ErrInvalidWorkerConfig)
	}
	worker.scheduler.schedule(task)
	return task, nil
}

func (worker *Worker) insertEntity(entity world.Entity, serverSpeed float64) error {
	if err := worker.validateOwnedEntity(entity); err != nil {
		return err
	}
	if err := validateServerSpeed(serverSpeed); err != nil {
		return err
	}
	if _, exists := worker.entities[entity.ID]; exists {
		return fmt.Errorf("entity %q: %w", entity.ID, ErrEntityAlreadyExists)
	}

	worker.entities[entity.ID] = entity
	worker.entitySpeeds[entity.ID] = serverSpeed
	if err := worker.index.Insert(spatial.EntityID(entity.ID.String()), spatialPosition(entity.Position)); err != nil {
		delete(worker.entities, entity.ID)
		delete(worker.entitySpeeds, entity.ID)
		return err
	}
	return nil
}

func (worker *Worker) updateEntity(entity world.Entity) error {
	if err := worker.validateOwnedEntity(entity); err != nil {
		return err
	}
	if _, exists := worker.entities[entity.ID]; !exists {
		return fmt.Errorf("entity %q: %w", entity.ID, ErrUnknownEntity)
	}

	if err := worker.index.Update(spatial.EntityID(entity.ID.String()), spatialPosition(entity.Position)); err != nil {
		return err
	}
	worker.entities[entity.ID] = entity
	return nil
}

func (worker *Worker) removeEntity(entityID world.EntityID) bool {
	if _, exists := worker.entities[entityID]; !exists {
		return false
	}

	delete(worker.entities, entityID)
	delete(worker.entitySpeeds, entityID)
	worker.index.Remove(spatial.EntityID(entityID.String()))

	if playerID, ok := worker.entityPlayers[entityID]; ok {
		worker.detachPlayerEntity(playerID)
	}
	return true
}

func (worker *Worker) attachPlayerEntity(playerID foundation.PlayerID, entityID world.EntityID) error {
	if err := playerID.Validate(); err != nil {
		return err
	}
	entity, ok := worker.entities[entityID]
	if !ok {
		return fmt.Errorf("entity %q: %w", entityID, ErrUnknownEntity)
	}
	if entity.Type != world.EntityTypePlayer {
		return fmt.Errorf("entity %q type %q: %w", entityID, entity.Type, ErrUnknownPlayer)
	}
	if existing, exists := worker.playerEntities[playerID]; exists && existing != entityID {
		return fmt.Errorf("player %q: %w", playerID, ErrPlayerAlreadyExists)
	}
	if existingPlayer, exists := worker.entityPlayers[entityID]; exists && existingPlayer != playerID {
		return fmt.Errorf("entity %q already attached to player %q: %w", entityID, existingPlayer, ErrPlayerAlreadyExists)
	}

	worker.playerEntities[playerID] = entityID
	worker.entityPlayers[entityID] = playerID
	return nil
}

func (worker *Worker) detachPlayerEntity(playerID foundation.PlayerID) {
	entityID, ok := worker.playerEntities[playerID]
	if !ok {
		return
	}
	delete(worker.playerEntities, playerID)
	delete(worker.entityPlayers, entityID)

	for sessionID, attachedPlayerID := range worker.sessionPlayers {
		if attachedPlayerID == playerID {
			delete(worker.sessionPlayers, sessionID)
		}
	}
	delete(worker.playerSessions, playerID)
}

func (worker *Worker) attachSession(sessionID realtime.SessionID, playerID foundation.PlayerID) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	if err := playerID.Validate(); err != nil {
		return err
	}
	if _, ok := worker.playerEntities[playerID]; !ok {
		return fmt.Errorf("player %q: %w", playerID, ErrUnknownPlayer)
	}
	if existingPlayerID, ok := worker.sessionPlayers[sessionID]; ok && existingPlayerID != playerID {
		if sessions := worker.playerSessions[existingPlayerID]; sessions != nil {
			delete(sessions, sessionID)
			if len(sessions) == 0 {
				delete(worker.playerSessions, existingPlayerID)
			}
		}
	}

	worker.sessionPlayers[sessionID] = playerID
	if worker.playerSessions[playerID] == nil {
		worker.playerSessions[playerID] = make(map[realtime.SessionID]struct{})
	}
	worker.playerSessions[playerID][sessionID] = struct{}{}
	return nil
}

func (worker *Worker) detachSession(sessionID realtime.SessionID) error {
	if err := validateSessionID(sessionID); err != nil {
		return err
	}
	playerID, ok := worker.sessionPlayers[sessionID]
	if !ok {
		return nil
	}

	delete(worker.sessionPlayers, sessionID)
	if sessions := worker.playerSessions[playerID]; sessions != nil {
		delete(sessions, sessionID)
		if len(sessions) == 0 {
			delete(worker.playerSessions, playerID)
		}
	}
	return nil
}

func (worker *Worker) movePlayerTo(playerID foundation.PlayerID, intent world.MovementIntent) error {
	entity, err := worker.playerEntityForMutation(playerID)
	if err != nil {
		return err
	}
	if err := intent.Validate(); err != nil {
		return err
	}

	movement, err := world.NewMovementState(intent.Target)
	if err != nil {
		return err
	}
	entity.Movement = movement
	worker.entities[entity.ID] = entity
	return nil
}

func (worker *Worker) stopPlayer(playerID foundation.PlayerID) error {
	entity, err := worker.playerEntityForMutation(playerID)
	if err != nil {
		return err
	}
	entity.Movement = world.MovementState{}
	worker.entities[entity.ID] = entity
	return nil
}

func (worker *Worker) playerEntityForMutation(playerID foundation.PlayerID) (world.Entity, error) {
	if err := playerID.Validate(); err != nil {
		return world.Entity{}, err
	}
	entityID, ok := worker.playerEntities[playerID]
	if !ok {
		return world.Entity{}, fmt.Errorf("player %q: %w", playerID, ErrUnknownPlayer)
	}
	entity, ok := worker.entities[entityID]
	if !ok {
		return world.Entity{}, fmt.Errorf("entity %q: %w", entityID, ErrUnknownEntity)
	}
	return entity, nil
}

func (worker *Worker) advanceMovement() []CommandError {
	movementErrors := make([]CommandError, 0)
	entityIDs := make([]world.EntityID, 0, len(worker.entities))
	for entityID := range worker.entities {
		entityIDs = append(entityIDs, entityID)
	}
	sort.Slice(entityIDs, func(i, j int) bool {
		return entityIDs[i] < entityIDs[j]
	})

	for _, entityID := range entityIDs {
		entity := worker.entities[entityID]
		if !entity.Movement.Moving {
			continue
		}

		next, done := world.AdvanceMovement(entity.Position, entity.Movement.Target, worker.entitySpeeds[entityID], worker.tickDelta)
		entity.Position = next
		if done {
			entity.Movement = world.MovementState{}
		}
		if err := worker.index.Update(spatial.EntityID(entity.ID.String()), spatialPosition(entity.Position)); err != nil {
			movementErrors = append(movementErrors, CommandError{
				Index: -1,
				Err:   fmt.Errorf("advance movement for entity %q: %w", entity.ID, err),
			})
			continue
		}
		worker.entities[entityID] = entity
	}
	return movementErrors
}

func (worker *Worker) validateOwnedEntity(entity world.Entity) error {
	if err := entity.Validate(); err != nil {
		return err
	}
	if entity.WorldID != worker.worldID {
		return fmt.Errorf("entity %q world %q not owned by worker world %q: %w", entity.ID, entity.WorldID, worker.worldID, ErrInvalidWorkerConfig)
	}
	if entity.ZoneID != worker.zoneID {
		return fmt.Errorf("entity %q zone %q not owned by worker zone %q: %w", entity.ID, entity.ZoneID, worker.zoneID, ErrInvalidWorkerConfig)
	}
	return nil
}

func validateServerSpeed(serverSpeed float64) error {
	if serverSpeed < 0 || math.IsNaN(serverSpeed) || math.IsInf(serverSpeed, 0) {
		return fmt.Errorf("server speed %v: %w", serverSpeed, ErrInvalidServerSpeed)
	}
	return nil
}

func validateSessionID(sessionID realtime.SessionID) error {
	value := string(sessionID)
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || strings.Contains(value, ":") {
		return fmt.Errorf("session %q: %w", sessionID, ErrInvalidSessionID)
	}
	return nil
}

func spatialPosition(position world.Vec2) spatial.Position {
	return spatial.Position{
		X: position.X,
		Y: position.Y,
	}
}

type delayedScheduler struct {
	tasks []ScheduledTask
}

func newDelayedScheduler() delayedScheduler {
	return delayedScheduler{
		tasks: make([]ScheduledTask, 0),
	}
}

func (scheduler *delayedScheduler) schedule(task ScheduledTask) {
	scheduler.tasks = append(scheduler.tasks, task)
	sort.SliceStable(scheduler.tasks, func(i, j int) bool {
		if scheduler.tasks[i].DueAt.Equal(scheduler.tasks[j].DueAt) {
			return scheduler.tasks[i].ID < scheduler.tasks[j].ID
		}
		return scheduler.tasks[i].DueAt.Before(scheduler.tasks[j].DueAt)
	})
}

func (scheduler *delayedScheduler) drainDue(now time.Time) []ScheduledTask {
	dueCount := 0
	for dueCount < len(scheduler.tasks) && !scheduler.tasks[dueCount].DueAt.After(now) {
		dueCount++
	}
	if dueCount == 0 {
		return nil
	}

	due := append([]ScheduledTask(nil), scheduler.tasks[:dueCount]...)
	copy(scheduler.tasks, scheduler.tasks[dueCount:])
	clear(scheduler.tasks[len(scheduler.tasks)-dueCount:])
	scheduler.tasks = scheduler.tasks[:len(scheduler.tasks)-dueCount]
	return due
}

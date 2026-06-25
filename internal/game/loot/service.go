package loot

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"gameproject/internal/game/combat"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/visibility"
)

const (
	EventLootCreated          = "loot.created"
	EventLootPickedUp         = "loot.picked_up"
	EventLootOwnerLockExpired = "loot.owner_lock_expired"
	EventLootExpired          = "loot.expired"
)

// CargoAdder is the cargo boundary used by pickup.
type CargoAdder interface {
	AddItem(economy.CargoAddItemInput) (economy.AddItemResult, error)
}

type transactionCargoAdder interface {
	AddItemWithoutRepository(economy.CargoAddItemInput) (economy.AddItemResult, error)
	AddItemCommit(economy.CargoAddItemInput, economy.AddItemResult) economy.InventoryAddItemCommit
	SnapshotMutationState() economy.InventoryMutationSnapshot
	RestoreMutationState(economy.InventoryMutationSnapshot)
}

type LootPickupTransactionRepository interface {
	WithLootPickupTransaction(ctx context.Context, fn func(LootPickupTransaction) error) error
}

type LootPickupTransaction interface {
	SaveLootDropClaim(ctx context.Context, drop Drop) error
	CommitInventoryAddItem(ctx context.Context, commit economy.InventoryAddItemCommit) error
	InsertOutboxRow(ctx context.Context, row economy.OutboxRow) error
}

// XPGranter is the progression boundary used for loot XP.
type XPGranter interface {
	GrantXP(progression.GrantXPInput) (progression.GrantXPResult, error)
}

// EventEmitter is the optional post-mutation event hook.
type EventEmitter interface {
	Record(events.EventEnvelope)
}

// MetricRecorder is the optional write-only loot metrics boundary.
type MetricRecorder interface {
	RecordLootCreated(sourceType string, itemID foundation.ItemID, quantity int64) error
	RecordLootPicked(sourceType string, itemID foundation.ItemID, quantity int64) error
}

// Viewer aliases the Phase 04 visibility viewer for pickup validation.
type Viewer = visibility.Viewer

// Service is the in-memory Phase 05 loot service.
type Service struct {
	mu    sync.Mutex
	clock foundation.Clock
	rng   foundation.RNG

	cargo       CargoAdder
	progression XPGranter
	xpOutbox    economy.OutboxStore
	pickupTx    LootPickupTransactionRepository

	ownerLockDuration time.Duration
	publicDuration    time.Duration
	totalLifetime     time.Duration
	pickupRange       float64

	nextDropSequence int64
	drops            map[world.EntityID]Drop
	sourceDrops      map[sourceKey][]world.EntityID
	pendingClaims    map[world.EntityID]struct{}
	ownerLockEvents  map[world.EntityID]struct{}
	expiredEvents    map[world.EntityID]struct{}

	emitter           EventEmitter
	metrics           MetricRecorder
	nextEventSequence uint64
}

type sourceKey struct {
	sourceType DropSourceType
	sourceID   world.EntityID
}

// Config describes service dependencies and tuning.
type Config struct {
	Clock foundation.Clock
	RNG   foundation.RNG

	Cargo              CargoAdder
	Progression        XPGranter
	XPOutbox           economy.OutboxStore
	PickupTransactions LootPickupTransactionRepository

	OwnerLockDuration time.Duration
	PublicDuration    time.Duration
	TotalLifetime     time.Duration
	PickupRange       float64
}

// NewService returns an in-memory loot service.
func NewService(config Config) (*Service, error) {
	if config.Cargo == nil {
		return nil, ErrNilCargoService
	}
	if config.Clock == nil {
		config.Clock = foundation.RealClock{}
	}
	if config.OwnerLockDuration == 0 {
		config.OwnerLockDuration = DefaultOwnerLockDuration
	}
	if config.PublicDuration == 0 {
		config.PublicDuration = DefaultPublicDuration
	}
	if config.TotalLifetime == 0 {
		config.TotalLifetime = DefaultTotalLifetime
	}
	if config.PickupRange == 0 {
		config.PickupRange = DefaultPickupRange
	}
	if config.OwnerLockDuration < 0 ||
		config.PublicDuration < 0 ||
		config.TotalLifetime <= 0 ||
		config.OwnerLockDuration+config.PublicDuration > config.TotalLifetime ||
		config.PickupRange < 0 {
		return nil, ErrInvalidLootDurations
	}
	return &Service{
		clock:             config.Clock,
		rng:               config.RNG,
		cargo:             config.Cargo,
		progression:       config.Progression,
		xpOutbox:          config.XPOutbox,
		pickupTx:          config.PickupTransactions,
		ownerLockDuration: config.OwnerLockDuration,
		publicDuration:    config.PublicDuration,
		totalLifetime:     config.TotalLifetime,
		pickupRange:       config.PickupRange,
		drops:             make(map[world.EntityID]Drop),
		sourceDrops:       make(map[sourceKey][]world.EntityID),
		pendingClaims:     make(map[world.EntityID]struct{}),
		ownerLockEvents:   make(map[world.EntityID]struct{}),
		expiredEvents:     make(map[world.EntityID]struct{}),
	}, nil
}

// SetEventEmitter configures the optional post-mutation event hook.
func (service *Service) SetEventEmitter(emitter EventEmitter) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.emitter = emitter
}

// SetMetricRecorder configures the optional post-mutation metric hook.
func (service *Service) SetMetricRecorder(metrics MetricRecorder) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.metrics = metrics
}

// CreateDropsForNPCKill rolls and creates drops once for a combat kill event.
func (service *Service) CreateDropsForNPCKill(event combat.NPCKilledEvent, table LootTable) (CreateDropsResult, error) {
	if err := table.validate(); err != nil {
		return CreateDropsResult{}, err
	}
	if err := event.NPCEntityID.Validate(); err != nil {
		return CreateDropsResult{}, err
	}
	if err := event.OwnerPlayerID.Validate(); err != nil {
		return CreateDropsResult{}, err
	}
	if err := event.WorldID.Validate(); err != nil {
		return CreateDropsResult{}, err
	}
	if err := event.ZoneID.Validate(); err != nil {
		return CreateDropsResult{}, err
	}
	if err := event.Position.Validate(); err != nil {
		return CreateDropsResult{}, err
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	var metrics MetricRecorder
	var metricDrops []Drop
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		recordLootCreatedMetrics(metrics, metricDrops)
		emitEvents(emitter, emitted)
	}()

	key := sourceKey{sourceType: DropSourceNPCDeath, sourceID: event.NPCEntityID}
	if existingIDs, ok := service.sourceDrops[key]; ok {
		return service.createDropsResultForIDsLocked(existingIDs, true), nil
	}

	now := service.clock.Now()
	rows := service.rollRows(table)
	service.sourceDrops[key] = make([]world.EntityID, 0, len(rows))
	drops := make([]Drop, 0, len(rows))
	scheduledTasks := make([]ScheduledDropTask, 0, len(rows)*2)
	for _, row := range rows {
		drop := Drop{
			ID:             service.nextDropID(),
			WorldID:        event.WorldID,
			ZoneID:         event.ZoneID,
			Position:       event.Position,
			ItemDefinition: row.ItemDefinition,
			Quantity:       row.quantity,
			OwnerPlayerID:  event.OwnerPlayerID,
			OwnerLockUntil: now.Add(service.ownerLockDuration),
			PublicUntil:    now.Add(service.ownerLockDuration + service.publicDuration),
			ExpiresAt:      now.Add(service.totalLifetime),
			CreatedAt:      now,
			SourceType:     DropSourceNPCDeath,
			SourceID:       event.NPCEntityID,
		}
		service.drops[drop.ID] = cloneDrop(drop)
		service.sourceDrops[key] = append(service.sourceDrops[key], drop.ID)
		drops = append(drops, drop)
		scheduledTasks = append(scheduledTasks, scheduledDropTasks(drop)...)
	}

	emitter = service.emitter
	metrics = service.metrics
	metricDrops = cloneDrops(drops)
	if emitter != nil {
		for _, drop := range drops {
			emitted = append(emitted, service.newEventLocked(EventLootCreated, dropPayload(drop, now), now))
		}
	}
	return CreateDropsResult{Drops: cloneDrops(drops), ScheduledTasks: cloneScheduledDropTasks(scheduledTasks)}, nil
}

// CreateDropsForPlayerDeath creates concrete death drops once for a
// DeathService-owned source event. Player-death drops are not eligible for loot XP.
func (service *Service) CreateDropsForPlayerDeath(input CreatePlayerDeathDropsInput) (CreateDropsResult, error) {
	if err := input.validate(); err != nil {
		return CreateDropsResult{}, err
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	var metrics MetricRecorder
	var metricDrops []Drop
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		recordLootCreatedMetrics(metrics, metricDrops)
		emitEvents(emitter, emitted)
	}()

	key := sourceKey{sourceType: DropSourcePlayerDeath, sourceID: input.SourceID}
	if existingIDs, ok := service.sourceDrops[key]; ok {
		return service.createDropsResultForIDsLocked(existingIDs, true), nil
	}

	now := service.clock.Now()
	service.sourceDrops[key] = make([]world.EntityID, 0, len(input.Items))
	drops := make([]Drop, 0, len(input.Items))
	scheduledTasks := make([]ScheduledDropTask, 0, len(input.Items)*2)
	for _, item := range input.Items {
		drop := Drop{
			ID:             service.nextDropID(),
			WorldID:        input.WorldID,
			ZoneID:         input.ZoneID,
			Position:       input.Position,
			ItemDefinition: item.ItemDefinition,
			Quantity:       item.Quantity,
			OwnerPlayerID:  input.OwnerPlayerID,
			OwnerLockUntil: now.Add(service.ownerLockDuration),
			PublicUntil:    now.Add(service.ownerLockDuration + service.publicDuration),
			ExpiresAt:      now.Add(service.totalLifetime),
			CreatedAt:      now,
			SourceType:     DropSourcePlayerDeath,
			SourceID:       input.SourceID,
		}
		service.drops[drop.ID] = cloneDrop(drop)
		service.sourceDrops[key] = append(service.sourceDrops[key], drop.ID)
		drops = append(drops, drop)
		scheduledTasks = append(scheduledTasks, scheduledDropTasks(drop)...)
	}

	emitter = service.emitter
	metrics = service.metrics
	metricDrops = cloneDrops(drops)
	if emitter != nil {
		for _, drop := range drops {
			emitted = append(emitted, service.newEventLocked(EventLootCreated, dropPayload(drop, now), now))
		}
	}
	return CreateDropsResult{Drops: cloneDrops(drops), ScheduledTasks: cloneScheduledDropTasks(scheduledTasks)}, nil
}

// PickupDrop validates ownership, visibility, range, cargo capacity, and claims one drop.
func (service *Service) PickupDrop(input PickupInput) (PickupResult, error) {
	if err := input.PlayerID.Validate(); err != nil {
		return PickupResult{}, err
	}
	if err := input.DropID.Validate(); err != nil {
		return PickupResult{}, err
	}
	if err := input.ActiveCargo.Validate(); err != nil {
		return PickupResult{}, err
	}

	service.mu.Lock()
	now := service.clock.Now()
	drop, ok := service.drops[input.DropID]
	if !ok {
		service.mu.Unlock()
		return PickupResult{}, fmt.Errorf("drop %q: %w", input.DropID, ErrUnknownDrop)
	}
	if drop.ClaimedAt != nil {
		service.mu.Unlock()
		return PickupResult{}, ErrDropClaimed
	}
	if _, pending := service.pendingClaims[input.DropID]; pending {
		service.mu.Unlock()
		return PickupResult{}, ErrDropClaimed
	}
	if !now.Before(drop.ExpiresAt) {
		service.mu.Unlock()
		return PickupResult{}, ErrDropExpired
	}
	if now.Before(drop.OwnerLockUntil) && drop.OwnerPlayerID != input.PlayerID {
		service.mu.Unlock()
		return PickupResult{}, ErrDropOwnerLocked
	}
	if err := visibility.CanInteract(input.Viewer, visibilityEntityFromDrop(drop)); err != nil {
		service.mu.Unlock()
		return PickupResult{}, ErrPickupNotVisible
	}
	if input.Viewer.Position.Distance(drop.Position) > service.pickupRange {
		service.mu.Unlock()
		return PickupResult{}, ErrPickupOutOfRange
	}
	service.pendingClaims[input.DropID] = struct{}{}
	drop = cloneDrop(drop)
	service.mu.Unlock()

	if service.pickupTx != nil {
		return service.pickupDropWithTransaction(input, drop)
	}

	cargoResult, err := service.cargo.AddItem(economy.CargoAddItemInput{
		PlayerID:           input.PlayerID,
		ActiveCargo:        input.ActiveCargo,
		ItemDefinition:     drop.ItemDefinition,
		Quantity:           drop.Quantity,
		CargoCapacityUnits: input.CargoCapacityUnits,
		Reason:             LedgerReasonLootPickup,
		ReferenceKey:       foundation.IdempotencyKey("loot_pickup:" + drop.ID.String()),
	})
	if err != nil {
		service.clearPendingClaim(input.DropID)
		return PickupResult{}, err
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	var metrics MetricRecorder
	service.mu.Lock()
	now = service.clock.Now()
	current, ok := service.drops[input.DropID]
	if !ok {
		delete(service.pendingClaims, input.DropID)
		service.mu.Unlock()
		return PickupResult{}, fmt.Errorf("drop %q: %w", input.DropID, ErrUnknownDrop)
	}
	if current.ClaimedAt != nil {
		delete(service.pendingClaims, input.DropID)
		service.mu.Unlock()
		return PickupResult{}, ErrDropClaimed
	}
	drop = current
	drop.ClaimedBy = input.PlayerID
	claimedAt := now
	drop.ClaimedAt = &claimedAt
	service.drops[input.DropID] = cloneDrop(drop)
	delete(service.pendingClaims, input.DropID)
	emitter = service.emitter
	metrics = service.metrics
	if emitter != nil {
		emitted = append(emitted, service.newEventLocked(EventLootPickedUp, pickedUpPayload(drop, now), now))
	}
	service.mu.Unlock()
	recordLootPickedMetric(metrics, drop)
	emitEvents(emitter, emitted)

	var xpResult *progression.GrantXPResult
	var xpErr error
	reconciliation := newLootXPReconciliation(drop, input.PlayerID, service.clock.Now())
	if !drop.SourceType.eligibleForLootXP() {
		reconciliation.Status = LootXPReconciliationNotEligible
	} else if service.progression == nil {
		reconciliation.Status = LootXPReconciliationFailed
		reconciliation.Error = ErrNilProgressionHook.Error()
		xpErr = ErrNilProgressionHook
	} else {
		grant, err := service.grantLootXP(drop, reconciliation)
		if err != nil {
			xpErr = err
			reconciliation.Status = LootXPReconciliationFailed
			reconciliation.Error = err.Error()
		} else {
			xpResult = &grant
			grantedAt := service.clock.Now()
			reconciliation.GrantedAt = &grantedAt
			if grant.Duplicate {
				reconciliation.Status = LootXPReconciliationDuplicate
			} else {
				reconciliation.Status = LootXPReconciliationGranted
			}
		}
	}
	drop = service.recordXPReconciliation(input.DropID, reconciliation)

	return PickupResult{
		Drop:        cloneDrop(drop),
		CargoResult: cargoResult,
		XPResult:    xpResult,
		XPError:     xpErr,
	}, nil
}

func (service *Service) pickupDropWithTransaction(input PickupInput, drop Drop) (PickupResult, error) {
	cargoTx, ok := service.cargo.(transactionCargoAdder)
	if !ok {
		service.clearPendingClaim(input.DropID)
		return PickupResult{}, ErrNilCargoService
	}
	inventorySnapshot := cargoTx.SnapshotMutationState()
	cargoInput := economy.CargoAddItemInput{
		PlayerID:           input.PlayerID,
		ActiveCargo:        input.ActiveCargo,
		ItemDefinition:     drop.ItemDefinition,
		Quantity:           drop.Quantity,
		CargoCapacityUnits: input.CargoCapacityUnits,
		Reason:             LedgerReasonLootPickup,
		ReferenceKey:       foundation.IdempotencyKey("loot_pickup:" + drop.ID.String()),
	}
	cargoResult, err := cargoTx.AddItemWithoutRepository(cargoInput)
	if err != nil {
		service.clearPendingClaim(input.DropID)
		return PickupResult{}, err
	}

	var claimed Drop
	var reconciliation LootXPReconciliation
	var outboxRow economy.OutboxRow
	var hasOutboxRow bool
	err = service.pickupTx.WithLootPickupTransaction(context.Background(), func(tx LootPickupTransaction) error {
		now := service.clock.Now()
		claimed = cloneDrop(drop)
		claimed.ClaimedBy = input.PlayerID
		claimedAt := now
		claimed.ClaimedAt = &claimedAt
		reconciliation = newLootXPReconciliation(claimed, input.PlayerID, now)
		if !claimed.SourceType.eligibleForLootXP() {
			reconciliation.Status = LootXPReconciliationNotEligible
		} else {
			row, err := newLootXPOutboxRow(claimed, reconciliation, now)
			if err != nil {
				return err
			}
			outboxRow = row
			hasOutboxRow = true
		}
		if err := tx.SaveLootDropClaim(context.Background(), claimed); err != nil {
			return err
		}
		if err := tx.CommitInventoryAddItem(context.Background(), cargoTx.AddItemCommit(cargoInput, cargoResult)); err != nil {
			return err
		}
		if hasOutboxRow {
			if err := tx.InsertOutboxRow(context.Background(), outboxRow); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		cargoTx.RestoreMutationState(inventorySnapshot)
		service.clearPendingClaim(input.DropID)
		return PickupResult{}, err
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	var metrics MetricRecorder
	service.mu.Lock()
	now := service.clock.Now()
	service.drops[input.DropID] = cloneDrop(claimed)
	delete(service.pendingClaims, input.DropID)
	emitter = service.emitter
	metrics = service.metrics
	if emitter != nil {
		emitted = append(emitted, service.newEventLocked(EventLootPickedUp, pickedUpPayload(claimed, now), now))
	}
	service.mu.Unlock()
	recordLootPickedMetric(metrics, claimed)
	emitEvents(emitter, emitted)

	var xpResult *progression.GrantXPResult
	var xpErr error
	if hasOutboxRow {
		if service.progression == nil {
			reconciliation.Status = LootXPReconciliationFailed
			reconciliation.Error = ErrNilProgressionHook.Error()
			xpErr = ErrNilProgressionHook
		} else {
			grant, err := ReplayLootXPOutboxRow(context.Background(), outboxRow, service.progression)
			if err != nil {
				xpErr = err
				reconciliation.Status = LootXPReconciliationFailed
				reconciliation.Error = err.Error()
			} else {
				xpResult = &grant
				grantedAt := service.clock.Now()
				reconciliation.GrantedAt = &grantedAt
				if grant.Duplicate {
					reconciliation.Status = LootXPReconciliationDuplicate
				} else {
					reconciliation.Status = LootXPReconciliationGranted
				}
			}
		}
	}
	claimed = service.recordXPReconciliation(input.DropID, reconciliation)
	return PickupResult{
		Drop:        cloneDrop(claimed),
		CargoResult: cargoResult,
		XPResult:    xpResult,
		XPError:     xpErr,
	}, nil
}

func (service *Service) grantLootXP(drop Drop, reconciliation LootXPReconciliation) (progression.GrantXPResult, error) {
	if service.xpOutbox == nil {
		return service.progression.GrantXP(progression.GrantXPInput{
			PlayerID:       reconciliation.PlayerID,
			Amount:         defaultLootXPAmount,
			SourceType:     reconciliation.SourceType,
			SourceID:       reconciliation.SourceID,
			IdempotencyKey: reconciliation.IdempotencyKey,
			Authority:      progression.XPGrantAuthorityLootService,
		})
	}

	row, err := newLootXPOutboxRow(drop, reconciliation, service.clock.Now())
	if err != nil {
		return progression.GrantXPResult{}, err
	}
	if err := service.xpOutbox.InsertOutboxRow(context.Background(), row); err != nil {
		existing, ok, loadErr := service.xpOutbox.LoadOutboxRow(context.Background(), row.OutboxID)
		if loadErr != nil {
			return progression.GrantXPResult{}, loadErr
		}
		if !ok || !sameLootXPOutboxRow(existing, row) {
			return progression.GrantXPResult{}, err
		}
		row = existing
	}
	return ReplayLootXPOutboxRow(context.Background(), row, service.progression)
}

func (service *Service) recordXPReconciliation(dropID world.EntityID, reconciliation LootXPReconciliation) Drop {
	service.mu.Lock()
	defer service.mu.Unlock()

	drop, ok := service.drops[dropID]
	if !ok {
		return Drop{ID: dropID, XPReconciliation: &reconciliation}
	}
	drop.XPReconciliation = &reconciliation
	service.drops[dropID] = cloneDrop(drop)
	return cloneDrop(drop)
}

func recordLootCreatedMetrics(recorder MetricRecorder, drops []Drop) {
	if recorder == nil {
		return
	}
	for _, drop := range drops {
		_ = recorder.RecordLootCreated(drop.SourceType.String(), drop.ItemDefinition.ItemID, drop.Quantity)
	}
}

func recordLootPickedMetric(recorder MetricRecorder, drop Drop) {
	if recorder == nil {
		return
	}
	_ = recorder.RecordLootPicked(drop.SourceType.String(), drop.ItemDefinition.ItemID, drop.Quantity)
}

func newLootXPReconciliation(drop Drop, playerID foundation.PlayerID, attemptedAt time.Time) LootXPReconciliation {
	return LootXPReconciliation{
		PlayerID:       playerID,
		SourceType:     progression.XPSourceTypeLoot,
		SourceID:       progression.XPSourceID(drop.ID.String()),
		IdempotencyKey: progression.XPIdempotencyKey("loot_pickup:" + drop.ID.String()),
		AttemptedAt:    attemptedAt,
	}
}

func (service *Service) clearPendingClaim(dropID world.EntityID) {
	service.mu.Lock()
	defer service.mu.Unlock()

	delete(service.pendingClaims, dropID)
}

// VisibleDrops returns client-safe drops that pass AOI/fog visibility.
func (service *Service) VisibleDrops(viewer Viewer) []DropPayload {
	service.mu.Lock()
	defer service.mu.Unlock()

	now := service.clock.Now()
	payloads := make([]DropPayload, 0)
	for _, drop := range service.drops {
		state := drop.State(now)
		if state == DropStateClaimed || state == DropStateExpired {
			continue
		}
		if visibility.CanInteract(viewer, visibilityEntityFromDrop(drop)) != nil {
			continue
		}
		payloads = append(payloads, dropPayload(drop, now))
	}
	sort.Slice(payloads, func(i, j int) bool {
		return payloads[i].ID < payloads[j].ID
	})
	return payloads
}

// ExpireDrops emits owner-lock and expiry events derived from server time.
func (service *Service) ExpireDrops() []Drop {
	var emitted []events.EventEnvelope
	var expired []Drop
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()

	now := service.clock.Now()
	emitter = service.emitter
	for id, drop := range service.drops {
		if drop.ClaimedAt != nil {
			continue
		}
		if !now.Before(drop.OwnerLockUntil) {
			if service.markOwnerLockExpiredLocked(id) {
				if emitter != nil {
					emitted = append(emitted, service.newEventLocked(EventLootOwnerLockExpired, dropPayload(drop, now), now))
				}
			}
		}
		if !now.Before(drop.ExpiresAt) {
			if service.markExpiredLocked(id) {
				expired = append(expired, cloneDrop(drop))
				if emitter != nil {
					emitted = append(emitted, service.newEventLocked(EventLootExpired, dropPayload(drop, now), now))
				}
			}
		}
	}
	return expired
}

// HandleScheduledDropTask applies one due loot scheduler task. It is idempotent:
// already claimed, missing, early, or previously handled tasks are no-ops.
func (service *Service) HandleScheduledDropTask(task ScheduledDropTask) (ScheduledDropTaskResult, error) {
	if err := task.DropID.Validate(); err != nil {
		return ScheduledDropTaskResult{}, err
	}

	var emitted []events.EventEnvelope
	var emitter EventEmitter
	service.mu.Lock()
	defer func() {
		service.mu.Unlock()
		emitEvents(emitter, emitted)
	}()

	now := service.clock.Now()
	drop, ok := service.drops[task.DropID]
	if !ok {
		return ScheduledDropTaskResult{}, nil
	}
	if drop.ClaimedAt != nil {
		return ScheduledDropTaskResult{Drop: cloneDrop(drop)}, nil
	}

	emitter = service.emitter
	switch task.Kind {
	case ScheduledDropTaskOwnerLockExpired:
		if now.Before(drop.OwnerLockUntil) {
			return ScheduledDropTaskResult{Drop: cloneDrop(drop), RetryAt: drop.OwnerLockUntil}, nil
		}
		if !service.markOwnerLockExpiredLocked(task.DropID) {
			return ScheduledDropTaskResult{Drop: cloneDrop(drop)}, nil
		}
		if emitter != nil {
			emitted = append(emitted, service.newEventLocked(EventLootOwnerLockExpired, dropPayload(drop, now), now))
		}
		return ScheduledDropTaskResult{Drop: cloneDrop(drop), Handled: true}, nil
	case ScheduledDropTaskDespawn:
		if now.Before(drop.ExpiresAt) {
			return ScheduledDropTaskResult{Drop: cloneDrop(drop), RetryAt: drop.ExpiresAt}, nil
		}
		if !service.markExpiredLocked(task.DropID) {
			return ScheduledDropTaskResult{Drop: cloneDrop(drop)}, nil
		}
		if emitter != nil {
			emitted = append(emitted, service.newEventLocked(EventLootExpired, dropPayload(drop, now), now))
		}
		return ScheduledDropTaskResult{Drop: cloneDrop(drop), Handled: true}, nil
	default:
		return ScheduledDropTaskResult{}, fmt.Errorf("loot scheduled task kind %q: %w", task.Kind, ErrInvalidScheduledTask)
	}
}

// Drop returns a copy of one drop.
func (service *Service) Drop(dropID world.EntityID) (Drop, bool) {
	service.mu.Lock()
	defer service.mu.Unlock()

	drop, ok := service.drops[dropID]
	return cloneDrop(drop), ok
}

func (service *Service) markOwnerLockExpiredLocked(dropID world.EntityID) bool {
	if _, seen := service.ownerLockEvents[dropID]; seen {
		return false
	}
	service.ownerLockEvents[dropID] = struct{}{}
	return true
}

func (service *Service) markExpiredLocked(dropID world.EntityID) bool {
	if _, seen := service.expiredEvents[dropID]; seen {
		return false
	}
	service.expiredEvents[dropID] = struct{}{}
	return true
}

func (service *Service) createDropsResultForIDsLocked(ids []world.EntityID, duplicate bool) CreateDropsResult {
	drops := service.dropsForIDsLocked(ids)
	return CreateDropsResult{
		Drops:          drops,
		ScheduledTasks: scheduledDropTasksForDrops(drops),
		Duplicate:      duplicate,
	}
}

func scheduledDropTasksForDrops(drops []Drop) []ScheduledDropTask {
	tasks := make([]ScheduledDropTask, 0, len(drops)*2)
	for _, drop := range drops {
		tasks = append(tasks, scheduledDropTasks(drop)...)
	}
	return tasks
}

func scheduledDropTasks(drop Drop) []ScheduledDropTask {
	return []ScheduledDropTask{
		{
			ID:     fmt.Sprintf("%s:%s", drop.ID, ScheduledDropTaskOwnerLockExpired),
			Kind:   ScheduledDropTaskOwnerLockExpired,
			DropID: drop.ID,
			DueAt:  drop.OwnerLockUntil,
		},
		{
			ID:     fmt.Sprintf("%s:%s", drop.ID, ScheduledDropTaskDespawn),
			Kind:   ScheduledDropTaskDespawn,
			DropID: drop.ID,
			DueAt:  drop.ExpiresAt,
		},
	}
}

func cloneScheduledDropTasks(tasks []ScheduledDropTask) []ScheduledDropTask {
	if tasks == nil {
		return nil
	}
	cloned := make([]ScheduledDropTask, len(tasks))
	copy(cloned, tasks)
	return cloned
}

type rolledRow struct {
	ItemDefinition economy.ItemDefinition
	quantity       int64
}

func (service *Service) rollRows(table LootTable) []rolledRow {
	rows := make([]rolledRow, 0, len(table.Rows))
	for _, row := range table.Rows {
		if service.rng != nil && service.rng.Float64() > row.Chance {
			continue
		}
		quantity := row.MinQuantity
		if row.MaxQuantity > row.MinQuantity {
			if service.rng == nil {
				quantity = row.MaxQuantity
			} else {
				quantity += int64(service.rng.Intn(int(row.MaxQuantity-row.MinQuantity) + 1))
			}
		}
		rows = append(rows, rolledRow{ItemDefinition: row.ItemDefinition, quantity: quantity})
	}
	return rows
}

func (service *Service) dropsForIDsLocked(ids []world.EntityID) []Drop {
	drops := make([]Drop, 0, len(ids))
	for _, id := range ids {
		if drop, ok := service.drops[id]; ok {
			drops = append(drops, cloneDrop(drop))
		}
	}
	return drops
}

func (service *Service) nextDropID() world.EntityID {
	service.nextDropSequence++
	return world.EntityID(fmt.Sprintf("drop_%d", service.nextDropSequence))
}

func visibilityEntityFromDrop(drop Drop) visibility.Entity {
	return visibility.Entity{
		WorldID:   drop.WorldID,
		ZoneID:    drop.ZoneID,
		ID:        drop.ID,
		Position:  drop.Position,
		Signature: visibility.SignatureForEntityType(world.EntityTypeLoot),
	}
}

func dropPayload(drop Drop, now time.Time) DropPayload {
	return DropPayload{
		ID:        drop.ID,
		Position:  drop.Position,
		ItemID:    drop.ItemDefinition.ItemID,
		Quantity:  drop.Quantity,
		State:     drop.State(now),
		ExpiresAt: drop.ExpiresAt,
	}
}

func pickedUpPayload(drop Drop, now time.Time) PickedUpPayload {
	claimedAt := now
	if drop.ClaimedAt != nil {
		claimedAt = *drop.ClaimedAt
	}
	return PickedUpPayload{
		DropID:    drop.ID,
		PlayerID:  drop.ClaimedBy,
		Position:  drop.Position,
		ItemID:    drop.ItemDefinition.ItemID,
		Quantity:  drop.Quantity,
		State:     drop.State(now),
		ClaimedAt: claimedAt,
	}
}

func (service *Service) newEventLocked(eventType string, payload any, now time.Time) events.EventEnvelope {
	service.nextEventSequence++
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		rawPayload = json.RawMessage(`{}`)
	}
	return events.NewEventEnvelope(
		foundation.EventID(fmt.Sprintf("loot-%d", service.nextEventSequence)),
		eventType,
		rawPayload,
		now.UnixMilli(),
		service.nextEventSequence,
	)
}

func emitEvents(emitter EventEmitter, emitted []events.EventEnvelope) {
	if emitter == nil {
		return
	}
	for _, event := range emitted {
		emitter.Record(event)
	}
}

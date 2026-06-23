package server

import (
	"encoding/json"
	"fmt"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
)

type RuntimeDurableOutboxRealtimeInput struct {
	// Limit is applied independently to each durable outbox store.
	Limit int

	Now                  time.Time
	ReleaseExpiredLeases bool
	LeaseTimeout         time.Duration
}

// DrainDurableOutboxesToRealtime drains committed durable outbox rows into the
// runtime's existing client-safe event projections. It never forwards raw
// domain outbox payloads to clients.
func (runtime *Runtime) DrainDurableOutboxesToRealtime(
	input RuntimeDurableOutboxRealtimeInput,
) (RuntimeDurableOutboxDrainResult, error) {
	if runtime == nil {
		return RuntimeDurableOutboxDrainResult{}, errInvalidRuntimeDurableOutbox
	}
	return runtime.DrainDurableOutboxes(RuntimeDurableOutboxDrainInput{
		Limit:                   input.Limit,
		Now:                     input.Now,
		ReleaseExpiredLeases:    input.ReleaseExpiredLeases,
		LeaseTimeout:            input.LeaseTimeout,
		PublishClaim:            runtime.projectClaimDurableOutboxToRealtime,
		PublishSettlement:       runtime.projectSettlementDurableOutboxToRealtime,
		PublishBuildingMutation: runtime.projectBuildingMutationDurableOutboxToRealtime,
	})
}

func (runtime *Runtime) drainDurableOutboxesToRealtimeTick() {
	if runtime == nil {
		return
	}
	now := time.Now().UTC()
	if runtime.clock != nil {
		now = runtime.clock.Now().UTC()
	}
	_, err := runtime.DrainDurableOutboxesToRealtime(RuntimeDurableOutboxRealtimeInput{
		Limit:                100,
		Now:                  now,
		ReleaseExpiredLeases: true,
		LeaseTimeout:         30 * time.Second,
	})
	if err != nil {
		runtime.recordDurableOutboxRealtimeError(err)
	}
}

func (runtime *Runtime) drainQueuedRealtimeEvents() map[auth.SessionID][]realtime.EventEnvelope {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.drainQueuedEventsBySessionLocked()
}

func mergeRuntimeRealtimeEvents(
	target map[auth.SessionID][]realtime.EventEnvelope,
	source map[auth.SessionID][]realtime.EventEnvelope,
) {
	if len(source) == 0 {
		return
	}
	for sessionID, events := range source {
		if len(events) > 0 {
			target[sessionID] = append(target[sessionID], events...)
		}
	}
}

func (runtime *Runtime) recordDurableOutboxRealtimeError(err error) {
	if runtime == nil || runtime.Metrics == nil || err == nil {
		return
	}
	code, ok := foundation.CodeOf(err)
	if !ok {
		code = foundation.CodeInternal
	}
	_ = runtime.Metrics.AddCounter(observability.MetricErrorsByCode, observability.Labels{
		"code":  code.String(),
		"op":    "runtime_durable_outbox_realtime",
		"stage": "drain",
	}, 1)
}

func (runtime *Runtime) projectClaimDurableOutboxToRealtime(record discovery.ClaimOutboxRecord) error {
	event := record.Event
	if event.Type != discovery.ClaimEventPlanetClaimed {
		return nil
	}
	if !runtime.hasActivePlayerSession(event.PlayerID) {
		return nil
	}
	knownPlanets, err := runtime.knownPlanetsPayload(event.PlayerID)
	if err != nil {
		return err
	}
	detail, err := runtime.planetDetailPayload(event.PlayerID, event.PlanetID)
	if err != nil {
		return err
	}
	productionPayload, err := runtime.productionSummaryPayload(event.PlayerID, event.PlanetID)
	if err != nil {
		return err
	}
	claimPayload := planetClaimedPayload{
		Accepted:           true,
		Planet:             detail.knownPlanetPayload,
		ProductionIncluded: len(productionPayload.Planets) > 0,
	}
	if planet, ok, err := runtime.Discovery.Planet(event.PlanetID); err != nil {
		return err
	} else if ok && planet.OwnerChangedAt != nil {
		claimPayload.ClaimedAt = planet.OwnerChangedAt.UTC().UnixMilli()
	}

	runtime.mu.Lock()
	inventory := runtime.inventorySnapshotLocked(event.PlayerID)
	runtime.queueEventToPlayerSessionsLocked(event.PlayerID, realtime.EventPlanetClaimed, claimPayload)
	runtime.queueEventToPlayerSessionsLocked(event.PlayerID, realtime.EventKnownPlanets, knownPlanets)
	runtime.queueEventToPlayerSessionsLocked(event.PlayerID, realtime.EventPlanetDetail, detail)
	if len(productionPayload.Planets) > 0 {
		runtime.queueEventToPlayerSessionsLocked(event.PlayerID, realtime.EventProductionSummary, productionPayload)
	}
	runtime.queueEventToPlayerSessionsLocked(event.PlayerID, realtime.EventInventorySnapshot, inventory)
	runtime.mu.Unlock()
	return nil
}

func (runtime *Runtime) projectSettlementDurableOutboxToRealtime(record production.ProductionOutboxRecord) error {
	return runtime.projectProductionDurableOutboxToRealtime(record, false)
}

func (runtime *Runtime) projectBuildingMutationDurableOutboxToRealtime(record production.ProductionOutboxRecord) error {
	return runtime.projectProductionDurableOutboxToRealtime(record, true)
}

func (runtime *Runtime) projectProductionDurableOutboxToRealtime(
	record production.ProductionOutboxRecord,
	includeWallet bool,
) error {
	switch record.Event.Type {
	case production.EventPlanetProductionSettled, production.EventPlanetBuildingUpdated:
		planetID, ok, err := productionOutboxPlanetID(record)
		if err != nil || !ok {
			return err
		}
		return runtime.projectPlanetProductionSnapshotToRealtime(planetID, includeWallet)
	case production.EventRouteTransferSettled:
		payload, ok, err := productionOutboxRouteSettlementPayload(record)
		if err != nil || !ok {
			return err
		}
		return runtime.projectRouteSettlementToRealtime(payload)
	default:
		return nil
	}
}

func (runtime *Runtime) projectPlanetProductionSnapshotToRealtime(
	planetID foundation.PlanetID,
	includeWallet bool,
) error {
	planet, ok, err := runtime.Discovery.Planet(planetID)
	if err != nil {
		return err
	}
	if !ok || planet.OwnerPlayerID.IsZero() {
		return fmt.Errorf("production outbox planet %q owner: %w", planetID, discovery.ErrUnknownPlanet)
	}
	if !runtime.hasActivePlayerSession(planet.OwnerPlayerID) {
		return nil
	}
	productionPayload, err := runtime.productionSummaryPayload(planet.OwnerPlayerID, planetID)
	if err != nil {
		return err
	}
	storagePayload := storageSummaryPayloadFromProduction(productionPayload)
	if includeWallet {
		runtime.mu.Lock()
		wallet := runtime.walletSnapshotLocked(planet.OwnerPlayerID)
		runtime.updatePlayerWalletCacheLocked(planet.OwnerPlayerID, wallet)
		runtime.queueEventToPlayerSessionsLocked(planet.OwnerPlayerID, realtime.EventProductionSummary, productionPayload)
		runtime.queueEventToPlayerSessionsLocked(planet.OwnerPlayerID, realtime.EventPlanetStorage, storagePayload)
		runtime.queueEventToPlayerSessionsLocked(planet.OwnerPlayerID, realtime.EventWalletSnapshot, wallet)
		runtime.mu.Unlock()
		return nil
	}
	runtime.queueProductionSummarySettlementEvents(planet.OwnerPlayerID, productionPayload, storagePayload)
	return nil
}

func (runtime *Runtime) projectRouteSettlementToRealtime(payload production.RouteSettlementPayload) error {
	if !runtime.hasActivePlayerSession(payload.OwnerPlayerID) {
		return nil
	}
	route, ok, err := runtime.Production.AutomationRoute(payload.RouteID)
	if err != nil {
		return err
	}
	if !ok || route.OwnerPlayerID != payload.OwnerPlayerID {
		return fmt.Errorf("route settlement outbox route %q: %w", payload.RouteID, production.ErrRouteNotFound)
	}
	routePayload, err := runtime.routePayloadFromRoute(route)
	if err != nil {
		return err
	}
	routes, err := runtime.routeListPayload(payload.OwnerPlayerID)
	if err != nil {
		return err
	}
	productionPayload, err := runtime.productionSummaryPayload(payload.OwnerPlayerID, "")
	if err != nil {
		return err
	}
	storagePayload, err := runtime.storageSummaryPayload(payload.OwnerPlayerID, "")
	if err != nil {
		return err
	}
	settlementPayload := routeSettlementPayloadFromDomainPayload(payload)

	runtime.mu.Lock()
	runtime.queueEventToPlayerSessionsLocked(payload.OwnerPlayerID, realtime.EventRouteSettled, map[string]any{
		"route":      routePayload,
		"settlement": settlementPayload,
	})
	runtime.queueEventToPlayerSessionsLocked(payload.OwnerPlayerID, realtime.EventRouteUpdated, map[string]any{"route": routePayload})
	runtime.queueEventToPlayerSessionsLocked(payload.OwnerPlayerID, realtime.EventRouteSnapshot, map[string]any{"route": routePayload})
	runtime.queueEventToPlayerSessionsLocked(payload.OwnerPlayerID, realtime.EventRouteList, map[string]any{"routes": routes})
	runtime.queueEventToPlayerSessionsLocked(payload.OwnerPlayerID, realtime.EventProductionSummary, productionPayload)
	runtime.queueEventToPlayerSessionsLocked(payload.OwnerPlayerID, realtime.EventPlanetStorage, storagePayload)
	runtime.mu.Unlock()
	return nil
}

func (runtime *Runtime) hasActivePlayerSession(playerID foundation.PlayerID) bool {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	for _, sessionPlayerID := range runtime.sessions {
		if sessionPlayerID == playerID {
			return true
		}
	}
	return false
}

func productionOutboxPlanetID(record production.ProductionOutboxRecord) (foundation.PlanetID, bool, error) {
	switch record.Event.Type {
	case production.EventPlanetProductionSettled:
		var payload production.ProductionSettlementPayload
		if err := json.Unmarshal(record.Event.Payload, &payload); err != nil {
			return "", false, err
		}
		return payload.PlanetID, true, nil
	case production.EventPlanetBuildingUpdated:
		var payload production.BuildingUpdatedPayload
		if err := json.Unmarshal(record.Event.Payload, &payload); err != nil {
			return "", false, err
		}
		return payload.PlanetID, true, nil
	default:
		return "", false, nil
	}
}

func productionOutboxRouteSettlementPayload(
	record production.ProductionOutboxRecord,
) (production.RouteSettlementPayload, bool, error) {
	if record.Event.Type != production.EventRouteTransferSettled {
		return production.RouteSettlementPayload{}, false, nil
	}
	var payload production.RouteSettlementPayload
	if err := json.Unmarshal(record.Event.Payload, &payload); err != nil {
		return production.RouteSettlementPayload{}, false, err
	}
	return payload, true, nil
}

func routeSettlementPayloadFromDomainPayload(payload production.RouteSettlementPayload) routeSettlementPayload {
	return routeSettlementPayload{
		RouteID:          payload.RouteID.String(),
		ResourceItemID:   payload.ResourceItemID.String(),
		SettledAt:        payload.SettledAt.UTC().UnixMilli(),
		ElapsedAppliedMS: payload.ElapsedApplied.Milliseconds(),
		WantedAmount:     payload.WantedAmount,
		TakenAmount:      payload.TakenAmount,
		LostAmount:       payload.LostAmount,
		DeliveredAmount:  payload.DeliveredAmount,
		AddedAmount:      payload.AddedAmount,
		SourceEmpty:      payload.SourceEmpty,
		DestinationFull:  payload.DestinationFull,
		LossApplied:      payload.LossApplied,
		NoOp:             payload.NoOp,
	}
}

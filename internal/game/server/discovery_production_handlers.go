package server

import (
	"encoding/json"
	"errors"
	"sort"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	worldmaps "gameproject/internal/game/world/maps"
)

type scanPulsePayload struct {
	PulseReference string                               `json:"pulse_reference"`
	Status         string                               `json:"status"`
	ResolveAfter   int64                                `json:"resolve_after,omitempty"`
	Message        string                               `json:"message,omitempty"`
	Signal         *discovery.CandidateSignalProjection `json:"signal,omitempty"`
	PlanetID       string                               `json:"planet_id,omitempty"`
	XPGranted      bool                                 `json:"xp_granted,omitempty"`
	Duplicate      bool                                 `json:"duplicate,omitempty"`
}

type scanPulseMapGuard struct {
	PlayerID  foundation.PlayerID
	SessionID auth.SessionID
	MapID     worldmaps.MapID
	WorldID   foundation.WorldID
	ZoneID    foundation.ZoneID
	Epoch     uint64
}

type scanPulseInterleaveStage string

const (
	scanPulseInterleaveBeforeMutation scanPulseInterleaveStage = "before_mutation"
	scanPulseInterleaveBeforeQueue    scanPulseInterleaveStage = "before_queue"
)

// scanPulseInterleaveTestHook is nil in production. Same-package tests install
// it to force scan/transfer interleavings at deterministic guard boundaries.
var scanPulseInterleaveTestHook func(scanPulseInterleaveStage, *Runtime, scanPulseMapGuard)

type knownPlanetsPayload struct {
	Planets []knownPlanetPayload `json:"planets"`
	Counts  planetIntelCounts    `json:"counts"`
}

type planetIntelCounts struct {
	Known int `json:"known"`
	Stale int `json:"stale"`
	Owned int `json:"owned"`
}

type knownPlanetPayload struct {
	PlanetID     string `json:"planet_id"`
	PublicMapKey string `json:"public_map_key"`
	Biome        string `json:"biome"`
	PlanetType   string `json:"planet_type"`
	Rarity       string `json:"rarity"`
	Level        int    `json:"level"`
	IntelState   string `json:"intel_state"`
	Confidence   int    `json:"confidence"`
	LastSeenAt   int64  `json:"last_seen_at"`
	OwnerStatus  string `json:"owner_status"`
	DiscoveredAt int64  `json:"discovered_at"`
}

type planetDetailPayload struct {
	knownPlanetPayload
	Coordinates       publicVec2               `json:"coordinates"`
	Production        *planetProductionPayload `json:"production,omitempty"`
	Routes            []routePayload           `json:"routes,omitempty"`
	ProductionLocked  bool                     `json:"production_locked"`
	AvailableCommands []string                 `json:"available_commands,omitempty"`
}

type publicVec2 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type planetProductionCollectionPayload struct {
	Planets []planetProductionPayload `json:"planets"`
}

type planetProductionPayload struct {
	PlanetID              string                  `json:"planet_id"`
	PublicMapKey          string                  `json:"public_map_key"`
	ProductionEnabled     bool                    `json:"production_enabled"`
	LastCalculatedAt      int64                   `json:"last_calculated_at"`
	EnergyCapacityPerHour int64                   `json:"energy_capacity_per_hour"`
	EnergyReservedPerHour int64                   `json:"energy_reserved_per_hour"`
	Storage               planetStoragePayload    `json:"storage"`
	Buildings             []planetBuildingPayload `json:"buildings"`
}

type planetStorageCollectionPayload struct {
	Planets []planetStoragePayload `json:"planets"`
}

type planetStoragePayload struct {
	PlanetID      string              `json:"planet_id"`
	PublicMapKey  string              `json:"public_map_key"`
	UsedUnits     int64               `json:"used_units"`
	FreeUnits     int64               `json:"free_units"`
	CapacityUnits int64               `json:"capacity_units"`
	UpdatedAt     int64               `json:"updated_at"`
	Items         []planetStorageItem `json:"items"`
}

type planetStorageItem struct {
	ItemID   string `json:"item_id"`
	Quantity int64  `json:"quantity"`
}

type planetBuildingPayload struct {
	PlanetID     string `json:"planet_id"`
	PublicMapKey string `json:"public_map_key"`
	BuildingID   string `json:"building_id"`
	BuildingType string `json:"building_type"`
	Category     string `json:"category"`
	Level        int    `json:"level"`
	State        string `json:"state"`
	UpdatedAt    int64  `json:"updated_at"`
}

type routeListPayload struct {
	Routes []routePayload `json:"routes"`
}

type routePayload struct {
	RouteID           string                  `json:"route_id"`
	SourcePlanetID    string                  `json:"source_planet_id"`
	FromPublicMapKey  string                  `json:"from_public_map_key"`
	ToPublicMapKey    string                  `json:"to_public_map_key"`
	Destination       routeDestinationPayload `json:"destination"`
	ResourceItemID    string                  `json:"resource_item_id"`
	AmountPerHour     int64                   `json:"amount_per_hour"`
	EnergyCostPerHour int64                   `json:"energy_cost_per_hour"`
	Enabled           bool                    `json:"enabled"`
	Risk              routeRiskPayload        `json:"risk"`
	LastCalculatedAt  int64                   `json:"last_calculated_at"`
	UpdatedAt         int64                   `json:"updated_at"`
}

type routeDestinationPayload struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

type routeRiskPayload struct {
	LossChance     float64 `json:"loss_chance"`
	MinLossPercent float64 `json:"min_loss_percent"`
	MaxLossPercent float64 `json:"max_loss_percent"`
}

func (runtime *Runtime) handleScanPulse(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectEmptyIntentPayload(request.Payload, "position", "coordinates", "energy", "capacitor", "max_energy"); err != nil {
		return nil, err
	}
	sessionID := authSessionID(ctx.SessionID)
	runtime.mu.Lock()
	guard, err := runtime.beginScanPulseMapGuardLocked(sessionID, ctx.PlayerID)
	runtime.mu.Unlock()
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	defer runtime.finishScanPulseMapGuard(guard)

	notifyScanPulseInterleaveTestHook(scanPulseInterleaveBeforeMutation, runtime, guard)
	if err := runtime.validateScanPulseMapGuard(guard); err != nil {
		return nil, err
	}

	ref := discovery.ScanPulseReference("pulse_" + request.RequestID.String())
	start, err := runtime.Scanner.StartScanPulse(discovery.StartScanPulseInput{
		PlayerID:       ctx.PlayerID,
		WorldID:        guard.WorldID,
		ZoneID:         guard.ZoneID,
		ShipID:         starterShipID,
		PulseReference: ref,
	})
	if err != nil {
		return nil, domainErrorForDiscovery(err)
	}
	if err := runtime.validateScanPulseMapGuard(guard); err != nil {
		return nil, err
	}

	result, err := runtime.Scanner.ResolveScanPulse(discovery.ResolveScanPulseInput{
		PlayerID:       ctx.PlayerID,
		WorldID:        guard.WorldID,
		ZoneID:         guard.ZoneID,
		PulseReference: ref,
	})
	if errors.Is(err, discovery.ErrScanPulseNotReady) {
		payload := scanPulsePayloadFromStart(start)
		notifyScanPulseInterleaveTestHook(scanPulseInterleaveBeforeQueue, runtime, guard)
		if err := runtime.queueScanEvents(guard, payload, nil, nil); err != nil {
			return nil, err
		}
		return marshalPayload(map[string]any{"scan": payload})
	}
	if err != nil {
		return nil, domainErrorForDiscovery(err)
	}
	if err := runtime.validateScanPulseMapGuard(guard); err != nil {
		return nil, err
	}

	scan := scanPulsePayloadFromResult(result)
	if result.Status == discovery.ScanPulseStatusPlayerRevealed {
		notifyScanPulseInterleaveTestHook(scanPulseInterleaveBeforeQueue, runtime, guard)
		if err := runtime.queueScanEvents(guard, scan, nil, nil); err != nil {
			return nil, err
		}
		return marshalPayload(map[string]any{"scan": scan})
	}
	if err := runtime.validateScanPulseMapGuard(guard); err != nil {
		return nil, err
	}
	knownPlanets, err := runtime.knownPlanetsPayload(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForDiscovery(err)
	}
	minimap, err := runtime.currentMinimapPayload(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	progressionSnapshot, err := runtime.Progression.GetProgressionSnapshot(ctx.PlayerID)
	if err != nil {
		return nil, err
	}
	notifyScanPulseInterleaveTestHook(scanPulseInterleaveBeforeQueue, runtime, guard)
	if err := runtime.queueScanEvents(guard, scan, &knownPlanets, &minimap); err != nil {
		return nil, err
	}
	return marshalPayload(map[string]any{
		"scan":          scan,
		"known_planets": knownPlanets,
		"progression":   progressionPayload(progressionSnapshot),
	})
}

func (runtime *Runtime) handleKnownPlanets(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	payload, err := runtime.knownPlanetsPayload(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForDiscovery(err)
	}
	minimap, err := runtime.currentMinimapPayload(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	return marshalPayload(map[string]any{"known_planets": payload, "minimap": minimap})
}

func (runtime *Runtime) handlePlanetDetail(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		PlanetID string `json:"planet_id"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	planetID, err := foundation.ParsePlanetID(payload.PlanetID)
	if err != nil {
		return nil, invalidPayload("Planet is invalid.", err)
	}
	detail, err := runtime.planetDetailPayload(ctx.PlayerID, planetID)
	if err != nil {
		return nil, domainErrorForDiscovery(err)
	}
	return marshalPayload(map[string]any{"planet_detail": detail})
}

func (runtime *Runtime) handleProductionSummary(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	planetID, err := optionalPlanetID(request.Payload)
	if err != nil {
		return nil, invalidPayload("Planet is invalid.", err)
	}
	now := runtime.clock.Now().UTC()
	payload, storagePayload, changed, err := runtime.settleOwnedProductionForSummary(ctx.PlayerID, planetID, now)
	if err != nil {
		return nil, domainErrorForDiscovery(err)
	}
	if changed {
		runtime.queueProductionSummarySettlementEvents(ctx.PlayerID, payload, storagePayload)
	}
	return marshalPayload(map[string]any{"production": payload})
}

func (runtime *Runtime) handlePlanetStorage(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	planetID, err := optionalPlanetID(request.Payload)
	if err != nil {
		return nil, invalidPayload("Planet is invalid.", err)
	}
	now := runtime.clock.Now().UTC()
	productionPayload, payload, changed, err := runtime.settleOwnedProductionForSummary(ctx.PlayerID, planetID, now)
	if err != nil {
		return nil, domainErrorForDiscovery(err)
	}
	if changed {
		runtime.queueProductionSummarySettlementEvents(ctx.PlayerID, productionPayload, payload)
	}
	return marshalPayload(map[string]any{"planet_storage": payload})
}

func (runtime *Runtime) handleRouteList(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	payload, err := runtime.routeListPayload(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	return marshalPayload(map[string]any{"routes": payload})
}

func (runtime *Runtime) handleRouteSnapshot(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		RouteID string `json:"route_id"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	routeID, err := foundation.ParseRouteID(payload.RouteID)
	if err != nil {
		return nil, invalidPayload("Route is invalid.", err)
	}
	route, ok, err := runtime.Production.AutomationRoute(routeID)
	if err != nil {
		return nil, err
	}
	if !ok || route.OwnerPlayerID != ctx.PlayerID {
		return nil, foundation.NewDomainError(foundation.CodeNotFound, "Route was not found.")
	}
	routePayload, err := runtime.routePayloadFromRoute(route)
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	return marshalPayload(map[string]any{"route": routePayload})
}

func (runtime *Runtime) beginScanPulseMapGuardLocked(sessionID auth.SessionID, playerID foundation.PlayerID) (scanPulseMapGuard, error) {
	if err := runtime.validateNoActiveTransferLocked(playerID); err != nil {
		return scanPulseMapGuard{}, err
	}
	if _, active := runtime.activeScanPulses[playerID]; active {
		return scanPulseMapGuard{}, foundation.NewDomainError(foundation.CodeForbidden, "Scan pulse is already active.", foundation.WithCause(errScanPulseActive))
	}
	location, err := runtime.mapRouter.ActiveLocation(playerID)
	if err != nil {
		return scanPulseMapGuard{}, err
	}
	epoch := runtime.sessionMapEpochLocked(sessionID)
	if epoch == 0 || runtime.sessionLocations[sessionID] != location.InternalMapID {
		return scanPulseMapGuard{}, foundation.NewDomainError(foundation.CodeForbidden, "Map subscription is not active.", foundation.WithCause(errMapEpochChanged))
	}
	guard := scanPulseMapGuard{
		PlayerID:  playerID,
		SessionID: sessionID,
		MapID:     location.InternalMapID,
		WorldID:   location.WorldID,
		ZoneID:    location.ZoneID,
		Epoch:     epoch,
	}
	runtime.activeScanPulses[playerID] = guard
	return guard, nil
}

func (runtime *Runtime) finishScanPulseMapGuard(guard scanPulseMapGuard) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if active, ok := runtime.activeScanPulses[guard.PlayerID]; ok && active == guard {
		delete(runtime.activeScanPulses, guard.PlayerID)
	}
}

func (runtime *Runtime) validateScanPulseMapGuard(guard scanPulseMapGuard) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.validateScanPulseMapGuardLocked(guard)
}

func (runtime *Runtime) validateScanPulseMapGuardLocked(guard scanPulseMapGuard) error {
	if err := runtime.validateNoActiveTransferLocked(guard.PlayerID); err != nil {
		return err
	}
	active, ok := runtime.activeScanPulses[guard.PlayerID]
	if !ok || active != guard {
		return foundation.NewDomainError(foundation.CodeForbidden, "Scan pulse map guard changed.", foundation.WithCause(errScanPulseActive))
	}
	location, err := runtime.mapRouter.ActiveLocation(guard.PlayerID)
	if err != nil {
		return err
	}
	if location.InternalMapID != guard.MapID || location.WorldID != guard.WorldID || location.ZoneID != guard.ZoneID {
		return foundation.NewDomainError(foundation.CodeForbidden, "Map subscription changed during scan.", foundation.WithCause(errMapEpochChanged))
	}
	if runtime.sessionLocations[guard.SessionID] != guard.MapID || runtime.sessionMapEpochLocked(guard.SessionID) != guard.Epoch {
		return foundation.NewDomainError(foundation.CodeForbidden, "Map subscription changed during scan.", foundation.WithCause(errMapEpochChanged))
	}
	return nil
}

func notifyScanPulseInterleaveTestHook(stage scanPulseInterleaveStage, runtime *Runtime, guard scanPulseMapGuard) {
	if scanPulseInterleaveTestHook != nil {
		scanPulseInterleaveTestHook(stage, runtime, guard)
	}
}

func (runtime *Runtime) queueScanEvents(guard scanPulseMapGuard, scan scanPulsePayload, knownPlanets *knownPlanetsPayload, minimap *minimapPayload) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if err := runtime.validateScanPulseMapGuardLocked(guard); err != nil {
		return err
	}
	runtime.queueEventLocked(guard.SessionID, realtime.EventScanPulseStarted, map[string]any{
		"pulse_reference": scan.PulseReference,
		"status":          "started",
		"resolve_after":   scan.ResolveAfter,
	})
	runtime.queueEventLocked(guard.SessionID, realtime.EventScanPulseResolved, scan)
	if scan.PlanetID != "" {
		runtime.queueEventLocked(guard.SessionID, realtime.EventScanPlanetDiscovered, scan)
	}
	if knownPlanets != nil {
		payload := map[string]any{
			"planets": knownPlanets.Planets,
			"counts":  knownPlanets.Counts,
		}
		if minimap != nil {
			payload["minimap"] = *minimap
		}
		runtime.queueEventLocked(guard.SessionID, realtime.EventKnownPlanets, payload)
	}
	return nil
}

func (runtime *Runtime) knownPlanetsPayload(playerID foundation.PlayerID) (knownPlanetsPayload, error) {
	scope, err := runtime.knownPlanetMapScope(playerID)
	if err != nil {
		return knownPlanetsPayload{}, err
	}
	return runtime.knownPlanetsPayloadForScope(playerID, scope)
}

func (runtime *Runtime) knownPlanetsPayloadLocked(playerID foundation.PlayerID) (knownPlanetsPayload, error) {
	scope, err := runtime.knownPlanetMapScopeLocked(playerID)
	if err != nil {
		return knownPlanetsPayload{}, err
	}
	return runtime.knownPlanetsPayloadForScope(playerID, scope)
}

func (runtime *Runtime) knownPlanetsPayloadForScope(playerID foundation.PlayerID, scope knownPlanetMapScope) (knownPlanetsPayload, error) {
	intelRows, err := runtime.Discovery.PlayerPlanetIntelRecords(playerID)
	if err != nil {
		return knownPlanetsPayload{}, err
	}
	planets := make([]knownPlanetPayload, 0, len(intelRows))
	counts := planetIntelCounts{}
	for _, intel := range intelRows {
		planet, ok, err := runtime.Discovery.Planet(intel.PlanetID)
		if err != nil {
			return knownPlanetsPayload{}, err
		}
		if !ok {
			continue
		}
		if !intelAndPlanetMatchActiveMap(intel, planet, scope.worldID, scope.zoneID) {
			continue
		}
		summary := knownPlanetPayloadFromIntel(playerID, planet, intel, scope.publicMapKey)
		planets = append(planets, summary)
		counts.Known++
		if intel.State == discovery.IntelStateStale {
			counts.Stale++
		}
		if planet.OwnerPlayerID == playerID {
			counts.Owned++
		}
	}
	sort.Slice(planets, func(i, j int) bool {
		return planets[i].PlanetID < planets[j].PlanetID
	})
	return knownPlanetsPayload{Planets: planets, Counts: counts}, nil
}

func (runtime *Runtime) planetDetailPayload(playerID foundation.PlayerID, planetID foundation.PlanetID) (planetDetailPayload, error) {
	scope, err := runtime.knownPlanetMapScope(playerID)
	if err != nil {
		return planetDetailPayload{}, err
	}
	intel, ok, err := runtime.Discovery.PlayerPlanetIntel(playerID, planetID)
	if err != nil {
		return planetDetailPayload{}, err
	}
	if !ok {
		return planetDetailPayload{}, foundation.NewDomainError(foundation.CodeNotFound, "Planet was not found.")
	}
	planet, ok, err := runtime.Discovery.Planet(planetID)
	if err != nil {
		return planetDetailPayload{}, err
	}
	if !ok {
		return planetDetailPayload{}, foundation.NewDomainError(foundation.CodeNotFound, "Planet was not found.")
	}
	if !intelAndPlanetMatchActiveMap(intel, planet, scope.worldID, scope.zoneID) {
		return planetDetailPayload{}, foundation.NewDomainError(foundation.CodeNotFound, "Planet was not found.")
	}

	detail := planetDetailPayload{
		knownPlanetPayload: knownPlanetPayloadFromIntel(playerID, planet, intel, scope.publicMapKey),
		Coordinates: publicVec2{
			X: intel.Coordinates.X,
			Y: intel.Coordinates.Y,
		},
		ProductionLocked: planet.OwnerPlayerID != playerID,
	}
	if planet.OwnerPlayerID == playerID {
		if snapshot, ok, err := runtime.Production.Snapshot(planetID); err != nil {
			return planetDetailPayload{}, err
		} else if ok {
			publicMapKey, err := runtime.publicMapKeyForPlanet(planet)
			if err != nil {
				return planetDetailPayload{}, err
			}
			productionPayload := planetProductionPayloadFromSnapshot(snapshot, publicMapKey)
			detail.Production = &productionPayload
		}
		routes, err := runtime.routesForPlanet(playerID, planetID)
		if err != nil {
			return planetDetailPayload{}, err
		}
		detail.Routes = routes
		detail.AvailableCommands = []string{"planet.production_summary", "planet.storage_summary", "route.list"}
	}
	return detail, nil
}

func (runtime *Runtime) productionSummaryPayload(playerID foundation.PlayerID, planetID foundation.PlanetID) (planetProductionCollectionPayload, error) {
	scope, err := runtime.knownPlanetMapScope(playerID)
	if err != nil {
		return planetProductionCollectionPayload{}, err
	}
	return runtime.productionSummaryPayloadForScope(playerID, planetID, scope)
}

func (runtime *Runtime) productionSummaryPayloadForScope(playerID foundation.PlayerID, planetID foundation.PlanetID, scope knownPlanetMapScope) (planetProductionCollectionPayload, error) {
	snapshots := runtime.Production.Snapshots()
	planets := make([]planetProductionPayload, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !planetID.IsZero() && snapshot.State.PlanetID != planetID {
			continue
		}
		planet, ok, err := runtime.Discovery.Planet(snapshot.State.PlanetID)
		if err != nil {
			return planetProductionCollectionPayload{}, err
		}
		if !ok || planet.OwnerPlayerID != playerID {
			continue
		}
		if planet.WorldID != scope.worldID || planet.ZoneID != scope.zoneID {
			continue
		}
		publicMapKey, err := runtime.publicMapKeyForPlanet(planet)
		if err != nil {
			return planetProductionCollectionPayload{}, err
		}
		planets = append(planets, planetProductionPayloadFromSnapshot(snapshot, publicMapKey))
	}
	return planetProductionCollectionPayload{Planets: planets}, nil
}

func (runtime *Runtime) storageSummaryPayload(playerID foundation.PlayerID, planetID foundation.PlanetID) (planetStorageCollectionPayload, error) {
	productionPayload, err := runtime.productionSummaryPayload(playerID, planetID)
	if err != nil {
		return planetStorageCollectionPayload{}, err
	}
	return storageSummaryPayloadFromProduction(productionPayload), nil
}

func storageSummaryPayloadFromProduction(productionPayload planetProductionCollectionPayload) planetStorageCollectionPayload {
	storage := make([]planetStoragePayload, 0, len(productionPayload.Planets))
	for _, planet := range productionPayload.Planets {
		storage = append(storage, planet.Storage)
	}
	return planetStorageCollectionPayload{Planets: storage}
}

func (runtime *Runtime) settleOwnedProductionForSummary(
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
	now time.Time,
) (planetProductionCollectionPayload, planetStorageCollectionPayload, bool, error) {
	scope, err := runtime.knownPlanetMapScope(playerID)
	if err != nil {
		return planetProductionCollectionPayload{}, planetStorageCollectionPayload{}, false, err
	}
	planetIDs, err := runtime.ownedProductionPlanetIDsForScope(playerID, planetID, scope)
	if err != nil {
		return planetProductionCollectionPayload{}, planetStorageCollectionPayload{}, false, err
	}
	changed := false
	for _, candidateID := range planetIDs {
		result, err := runtime.applyProductionSummarySettlement(candidateID, now)
		if err != nil {
			return planetProductionCollectionPayload{}, planetStorageCollectionPayload{}, false, err
		}
		if !result.Settlement.NoOp {
			changed = true
		}
	}

	productionPayload, err := runtime.productionSummaryPayloadForScope(playerID, planetID, scope)
	if err != nil {
		return planetProductionCollectionPayload{}, planetStorageCollectionPayload{}, false, err
	}
	return productionPayload, storageSummaryPayloadFromProduction(productionPayload), changed, nil
}

func (runtime *Runtime) applyProductionSummarySettlement(
	planetID foundation.PlanetID,
	now time.Time,
) (production.ProductionSettlementTransactionResult, error) {
	result, err := runtime.Production.ApplyProductionSettlementTransaction(production.ProductionSettlementTransactionInput{
		PlanetID:           planetID,
		SettledAt:          now,
		RequireWholeOutput: true,
	})
	if err != nil {
		return production.ProductionSettlementTransactionResult{}, err
	}
	if _, err := result.ApplyDurableCommit(runtime.Settlements); err != nil {
		return production.ProductionSettlementTransactionResult{}, err
	}
	return result, nil
}

func (runtime *Runtime) ownedProductionPlanetIDsForScope(
	playerID foundation.PlayerID,
	planetID foundation.PlanetID,
	scope knownPlanetMapScope,
) ([]foundation.PlanetID, error) {
	snapshots := runtime.Production.Snapshots()
	planetIDs := make([]foundation.PlanetID, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if !planetID.IsZero() && snapshot.State.PlanetID != planetID {
			continue
		}
		planet, ok, err := runtime.Discovery.Planet(snapshot.State.PlanetID)
		if err != nil {
			return nil, err
		}
		if !ok || planet.OwnerPlayerID != playerID {
			continue
		}
		if planet.WorldID != scope.worldID || planet.ZoneID != scope.zoneID {
			continue
		}
		planetIDs = append(planetIDs, snapshot.State.PlanetID)
	}
	return planetIDs, nil
}

func (runtime *Runtime) queueProductionSummarySettlementEvents(
	playerID foundation.PlayerID,
	productionPayload planetProductionCollectionPayload,
	storagePayload planetStorageCollectionPayload,
) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventProductionSummary, productionPayload)
	runtime.queueEventToPlayerSessionsLocked(playerID, realtime.EventPlanetStorage, storagePayload)
}

func (runtime *Runtime) routeListPayload(playerID foundation.PlayerID) (routeListPayload, error) {
	routes := runtime.Production.AutomationRoutes()
	payload := make([]routePayload, 0, len(routes))
	for _, route := range routes {
		if route.OwnerPlayerID != playerID {
			continue
		}
		routePayload, err := runtime.routePayloadFromRoute(route)
		if err != nil {
			return routeListPayload{}, err
		}
		payload = append(payload, routePayload)
	}
	return routeListPayload{Routes: payload}, nil
}

func (runtime *Runtime) routesForPlanet(playerID foundation.PlayerID, planetID foundation.PlanetID) ([]routePayload, error) {
	routes := runtime.Production.AutomationRoutes()
	payload := make([]routePayload, 0)
	for _, route := range routes {
		if route.OwnerPlayerID != playerID || route.SourcePlanetID != planetID {
			continue
		}
		routePayload, err := runtime.routePayloadFromRoute(route)
		if err != nil {
			return nil, err
		}
		payload = append(payload, routePayload)
	}
	return payload, nil
}

type knownPlanetMapScope struct {
	worldID      foundation.WorldID
	zoneID       foundation.ZoneID
	publicMapKey string
}

func (runtime *Runtime) knownPlanetMapScope(playerID foundation.PlayerID) (knownPlanetMapScope, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	return runtime.knownPlanetMapScopeLocked(playerID)
}

func (runtime *Runtime) knownPlanetMapScopeLocked(playerID foundation.PlayerID) (knownPlanetMapScope, error) {
	location, err := runtime.mapRouter.ActiveLocation(playerID)
	if err != nil {
		return knownPlanetMapScope{}, err
	}
	projection, err := runtime.mapCatalog.ClientProjection(location.InternalMapID)
	if err != nil {
		return knownPlanetMapScope{}, err
	}
	return knownPlanetMapScope{
		worldID:      location.WorldID,
		zoneID:       location.ZoneID,
		publicMapKey: publicMapKeyFromProjection(projection),
	}, nil
}

func optionalPlanetID(payload json.RawMessage) (foundation.PlanetID, error) {
	var decoded struct {
		PlanetID string `json:"planet_id,omitempty"`
	}
	if err := decodeStrict(payload, &decoded); err != nil {
		return "", err
	}
	if decoded.PlanetID == "" {
		return "", nil
	}
	return foundation.ParsePlanetID(decoded.PlanetID)
}

func domainErrorForDiscovery(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, discovery.ErrScannerEnergyUnavailable):
		return foundation.NewDomainError(foundation.CodeNotEnoughEnergy, "Scanner energy is unavailable.", foundation.WithCause(err))
	case errors.Is(err, discovery.ErrScanCooldownActive):
		return foundation.NewDomainError(foundation.CodeCooldown, "Scanner is cooling down.", foundation.WithCause(err))
	case errors.Is(err, discovery.ErrScanMovementRestricted):
		return foundation.NewDomainError(foundation.CodeForbidden, "Scanner requires a stationary ship.", foundation.WithCause(err))
	case errors.Is(err, discovery.ErrScannerUnavailable):
		return foundation.NewDomainError(foundation.CodeForbidden, "Scanner is unavailable.", foundation.WithCause(err))
	case errors.Is(err, discovery.ErrScanPulseNotFound), errors.Is(err, discovery.ErrUnknownPlanet):
		return foundation.NewDomainError(foundation.CodeNotFound, "Discovery record was not found.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Discovery command failed.", foundation.WithCause(err))
	}
}

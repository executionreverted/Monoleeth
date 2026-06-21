package server

import (
	"encoding/json"
	"errors"
	"sort"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
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
	ID   string `json:"id"`
}

type routeRiskPayload struct {
	LossChance     float64 `json:"loss_chance"`
	MinLossPercent float64 `json:"min_loss_percent"`
	MaxLossPercent float64 `json:"max_loss_percent"`
}

func (runtime *Runtime) handleScanPulse(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	location, err := runtime.mapRouter.ActiveLocation(ctx.PlayerID)
	runtime.mu.Unlock()
	if err != nil {
		return nil, domainErrorForRuntime(err)
	}
	ref := discovery.ScanPulseReference("pulse_" + request.RequestID.String())
	start, err := runtime.Scanner.StartScanPulse(discovery.StartScanPulseInput{
		PlayerID:       ctx.PlayerID,
		WorldID:        location.WorldID,
		ZoneID:         location.ZoneID,
		ShipID:         starterShipID,
		PulseReference: ref,
	})
	if err != nil {
		return nil, domainErrorForDiscovery(err)
	}

	result, err := runtime.Scanner.ResolveScanPulse(discovery.ResolveScanPulseInput{
		PlayerID:       ctx.PlayerID,
		WorldID:        location.WorldID,
		ZoneID:         location.ZoneID,
		PulseReference: ref,
	})
	if errors.Is(err, discovery.ErrScanPulseNotReady) {
		payload := scanPulsePayloadFromStart(start)
		runtime.queueScanEvents(authSessionID(ctx.SessionID), ctx.PlayerID, payload, nil, nil)
		return marshalPayload(map[string]any{"scan": payload})
	}
	if err != nil {
		return nil, domainErrorForDiscovery(err)
	}

	scan := scanPulsePayloadFromResult(result)
	if result.Status == discovery.ScanPulseStatusPlayerRevealed {
		runtime.queueScanEvents(authSessionID(ctx.SessionID), ctx.PlayerID, scan, nil, nil)
		return marshalPayload(map[string]any{"scan": scan})
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
	runtime.queueScanEvents(authSessionID(ctx.SessionID), ctx.PlayerID, scan, &knownPlanets, &minimap)
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
	payload, err := runtime.productionSummaryPayload(ctx.PlayerID, planetID)
	if err != nil {
		return nil, domainErrorForDiscovery(err)
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
	payload, err := runtime.storageSummaryPayload(ctx.PlayerID, planetID)
	if err != nil {
		return nil, domainErrorForDiscovery(err)
	}
	return marshalPayload(map[string]any{"planet_storage": payload})
}

func (runtime *Runtime) handleRouteList(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	return marshalPayload(map[string]any{"routes": runtime.routeListPayload(ctx.PlayerID)})
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
	return marshalPayload(map[string]any{"route": routePayloadFromRoute(route)})
}

func (runtime *Runtime) queueScanEvents(sessionID auth.SessionID, playerID foundation.PlayerID, scan scanPulsePayload, knownPlanets *knownPlanetsPayload, minimap *minimapPayload) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.queueEventLocked(sessionID, realtime.EventScanPulseStarted, map[string]any{
		"pulse_reference": scan.PulseReference,
		"status":          "started",
		"resolve_after":   scan.ResolveAfter,
	})
	runtime.queueEventLocked(sessionID, realtime.EventScanPulseResolved, scan)
	if scan.PlanetID != "" {
		runtime.queueEventLocked(sessionID, realtime.EventScanPlanetDiscovered, scan)
	}
	if knownPlanets != nil {
		payload := map[string]any{
			"planets": knownPlanets.Planets,
			"counts":  knownPlanets.Counts,
		}
		if minimap != nil {
			payload["minimap"] = *minimap
		}
		runtime.queueEventLocked(sessionID, realtime.EventKnownPlanets, payload)
	}
	_ = playerID
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
			productionPayload := planetProductionPayloadFromSnapshot(snapshot)
			detail.Production = &productionPayload
		}
		detail.Routes = runtime.routesForPlanet(playerID, planetID)
		detail.AvailableCommands = []string{"planet.production_summary", "planet.storage_summary", "route.list"}
	}
	return detail, nil
}

func (runtime *Runtime) productionSummaryPayload(playerID foundation.PlayerID, planetID foundation.PlanetID) (planetProductionCollectionPayload, error) {
	scope, err := runtime.knownPlanetMapScope(playerID)
	if err != nil {
		return planetProductionCollectionPayload{}, err
	}
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
		planets = append(planets, planetProductionPayloadFromSnapshot(snapshot))
	}
	return planetProductionCollectionPayload{Planets: planets}, nil
}

func (runtime *Runtime) storageSummaryPayload(playerID foundation.PlayerID, planetID foundation.PlanetID) (planetStorageCollectionPayload, error) {
	productionPayload, err := runtime.productionSummaryPayload(playerID, planetID)
	if err != nil {
		return planetStorageCollectionPayload{}, err
	}
	storage := make([]planetStoragePayload, 0, len(productionPayload.Planets))
	for _, planet := range productionPayload.Planets {
		storage = append(storage, planet.Storage)
	}
	return planetStorageCollectionPayload{Planets: storage}, nil
}

func (runtime *Runtime) routeListPayload(playerID foundation.PlayerID) routeListPayload {
	routes := runtime.Production.AutomationRoutes()
	payload := make([]routePayload, 0, len(routes))
	for _, route := range routes {
		if route.OwnerPlayerID != playerID {
			continue
		}
		payload = append(payload, routePayloadFromRoute(route))
	}
	return routeListPayload{Routes: payload}
}

func (runtime *Runtime) routesForPlanet(playerID foundation.PlayerID, planetID foundation.PlanetID) []routePayload {
	routes := runtime.Production.AutomationRoutes()
	payload := make([]routePayload, 0)
	for _, route := range routes {
		if route.OwnerPlayerID != playerID || route.SourcePlanetID != planetID {
			continue
		}
		payload = append(payload, routePayloadFromRoute(route))
	}
	return payload
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

func knownPlanetPayloadFromIntel(playerID foundation.PlayerID, planet discovery.Planet, intel discovery.PlayerPlanetIntel, publicMapKey string) knownPlanetPayload {
	return knownPlanetPayload{
		PlanetID:     planet.ID.String(),
		PublicMapKey: publicMapKey,
		Biome:        string(planet.Biome),
		PlanetType:   string(planet.Type),
		Rarity:       string(planet.Rarity),
		Level:        planet.Level,
		IntelState:   string(intel.State),
		Confidence:   intel.Confidence,
		LastSeenAt:   intel.LastSeenAt.UTC().UnixMilli(),
		OwnerStatus:  publicPlanetOwnerStatus(playerID, planet),
		DiscoveredAt: planet.DiscoveredAt.UTC().UnixMilli(),
	}
}

func publicPlanetOwnerStatus(playerID foundation.PlayerID, planet discovery.Planet) string {
	if planet.OwnerPlayerID.IsZero() {
		return "unclaimed"
	}
	if planet.OwnerPlayerID == playerID {
		return "owned_by_you"
	}
	return "claimed"
}

func scanPulsePayloadFromStart(start discovery.StartScanPulseResult) scanPulsePayload {
	return scanPulsePayload{
		PulseReference: string(start.PulseReference),
		Status:         string(start.Status),
		ResolveAfter:   start.ResolveAfter.UTC().UnixMilli(),
	}
}

func scanPulsePayloadFromResult(result discovery.ResolveScanPulseResult) scanPulsePayload {
	return scanPulsePayload{
		PulseReference: string(result.PulseReference),
		Status:         string(result.Status),
		Message:        result.Message,
		Signal:         result.Signal,
		PlanetID:       result.PlanetID.String(),
		XPGranted:      result.XPGranted,
		Duplicate:      result.Duplicate,
	}
}

func planetProductionPayloadFromSnapshot(snapshot production.PlanetProductionSnapshot) planetProductionPayload {
	buildings := make([]planetBuildingPayload, 0, len(snapshot.Buildings))
	for _, building := range snapshot.Buildings {
		buildings = append(buildings, planetBuildingPayload{
			BuildingID:   building.BuildingID.String(),
			BuildingType: building.BuildingType.String(),
			Category:     productionDefinitionCategory(building),
			Level:        building.Level,
			State:        building.State.String(),
			UpdatedAt:    building.UpdatedAt.UTC().UnixMilli(),
		})
	}
	return planetProductionPayload{
		PlanetID:              snapshot.State.PlanetID.String(),
		ProductionEnabled:     snapshot.State.ProductionEnabled,
		LastCalculatedAt:      snapshot.State.LastCalculatedAt.UTC().UnixMilli(),
		EnergyCapacityPerHour: snapshot.State.EnergyCapacityPerHour,
		EnergyReservedPerHour: snapshot.State.EnergyReservedPerHour,
		Storage:               planetStoragePayloadFromStorage(snapshot.Storage),
		Buildings:             buildings,
	}
}

func planetStoragePayloadFromStorage(storage production.PlanetStorage) planetStoragePayload {
	items := make([]planetStorageItem, 0, len(storage.Items))
	for _, item := range storage.Items {
		items = append(items, planetStorageItem{
			ItemID:   item.ItemID.String(),
			Quantity: item.Quantity,
		})
	}
	return planetStoragePayload{
		PlanetID:      storage.PlanetID.String(),
		UsedUnits:     storage.UsedUnits(),
		FreeUnits:     storage.FreeUnits(),
		CapacityUnits: storage.CapacityUnits,
		UpdatedAt:     storage.UpdatedAt.UTC().UnixMilli(),
		Items:         items,
	}
}

func productionDefinitionCategory(building production.PlanetBuilding) string {
	switch building.BuildingType {
	case production.BuildingTypeIronExtractor:
		return production.BuildingCategoryExtractor.String()
	case production.BuildingTypeAlloyFoundry:
		return production.BuildingCategoryRefinery.String()
	default:
		return ""
	}
}

func routePayloadFromRoute(route production.AutomationRoute) routePayload {
	return routePayload{
		RouteID:           route.RouteID.String(),
		SourcePlanetID:    route.SourcePlanetID.String(),
		Destination:       routeDestinationPayload{Type: route.Destination.Type.String(), ID: route.Destination.ID.String()},
		ResourceItemID:    route.ResourceItemID.String(),
		AmountPerHour:     route.AmountPerHour,
		EnergyCostPerHour: route.EnergyCostPerHour,
		Enabled:           route.Enabled,
		Risk: routeRiskPayload{
			LossChance:     route.Risk.LossChance,
			MinLossPercent: route.Risk.MinLossPercent,
			MaxLossPercent: route.Risk.MaxLossPercent,
		},
		LastCalculatedAt: route.LastCalculatedAt.UTC().UnixMilli(),
		UpdatedAt:        route.UpdatedAt.UTC().UnixMilli(),
	}
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

package server

import (
	"encoding/json"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
)

func TestPlayableVerticalServerAuthoritativeLoop(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:     []string{testOrigin},
		DevMode:            true,
		E2EPlanetClaimSeed: true,
		E2ERouteSeed:       true,
		SessionTTL:         24 * time.Hour,
		TickDelta:          50 * time.Millisecond,
		PasswordHasher:     auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
		Clock:              clock,
	})
	if err != nil {
		t.Fatalf("New(playable vertical) error = %v, want nil", err)
	}
	resolved := createResolvedRuntimeSession(t, gameServer, "playable-vertical@example.com", "Playable Vertical")
	if got := inventoryStackQuantityForTest(gameServer, resolved.PlayerID, "x_core"); got != 1 {
		t.Fatalf("x_core quantity = %d, want one E2E claim core", got)
	}

	dropID := playableVerticalCombatLoot(t, gameServer, resolved)
	playableVerticalRouteSettle(t, gameServer, resolved, clock)
	playableVerticalPortalToDestination(t, gameServer, resolved)
	planetID := playableVerticalScanClaim(t, gameServer, resolved)

	if got := inventoryStackQuantityForTest(gameServer, resolved.PlayerID, "x_core"); got != 0 {
		t.Fatalf("x_core quantity after claim = %d, want consumed", got)
	}
	if dropID == "" || planetID == "" {
		t.Fatalf("vertical loop identifiers drop=%q planet=%q, want both", dropID, planetID)
	}
}

func playableVerticalCombatLoot(t *testing.T, gameServer *Server, resolved auth.ResolvedSession) string {
	t.Helper()

	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	combat := gatewayJSON(t, gameServer, resolved, "vertical-combat", realtime.OperationCombatUseSkill, map[string]any{
		"skill_id":  "basic_laser",
		"target_id": "entity_training_npc",
	}, 1)
	assertPayloadOmitsPlayerOwner(t, "vertical combat response", combat, resolved.PlayerID)
	var combatPayload struct {
		Accepted bool `json:"accepted"`
		Killed   bool `json:"killed"`
	}
	if err := json.Unmarshal(combat, &combatPayload); err != nil {
		t.Fatalf("decode vertical combat payload: %v", err)
	}
	if !combatPayload.Accepted || !combatPayload.Killed {
		t.Fatalf("vertical combat payload = %+v, want accepted kill", combatPayload)
	}
	events, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationCombatUseSkill, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post vertical combat events: %v", err)
	}
	var dropID string
	for _, event := range events {
		if event.Type != realtime.EventLootCreated {
			continue
		}
		var payload struct {
			DropID string `json:"drop_id"`
			ItemID string `json:"item_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("decode vertical loot.created: %v", err)
		}
		if payload.ItemID == "raw_ore" {
			dropID = payload.DropID
		}
	}
	if dropID == "" {
		t.Fatalf("vertical combat events = %+v, want raw_ore loot.created", events)
	}

	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, world.EntityID(dropID), world.Vec2{})
	pickup := gatewayJSON(t, gameServer, resolved, "vertical-loot", realtime.OperationLootPickup, map[string]any{
		"drop_id": dropID,
	}, 2)
	var pickupPayload struct {
		Accepted bool                 `json:"accepted"`
		Cargo    cargoSnapshotPayload `json:"cargo"`
	}
	if err := json.Unmarshal(pickup, &pickupPayload); err != nil {
		t.Fatalf("decode vertical pickup payload: %v", err)
	}
	if !pickupPayload.Accepted || !cargoPayloadHasItem(pickupPayload.Cargo, "raw_ore", 3) {
		t.Fatalf("vertical pickup payload = %+v, want raw_ore cargo", pickupPayload)
	}
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationLootPickup, resolved.PlayerID); err != nil {
		t.Fatalf("post vertical loot events: %v", err)
	}
	return dropID
}

func playableVerticalRouteSettle(t *testing.T, gameServer *Server, resolved auth.ResolvedSession, clock *testutil.FakeClock) {
	t.Helper()

	sourceID := e2eRoutePlanetID(resolved.PlayerID, "source")
	destinationID := e2eRoutePlanetID(resolved.PlayerID, "destination")
	create := gatewayJSON(t, gameServer, resolved, "vertical-route-create", realtime.OperationRouteCreate, map[string]any{
		"source_planet_id":      sourceID.String(),
		"destination_planet_id": destinationID.String(),
		"resource_item_id":      "refined_alloy",
		"amount_per_hour":       40,
	}, 3)
	assertPayloadOmitsInternalMapIdentity(t, "vertical route.create response", create)
	var createPayload struct {
		Route routePayload `json:"route"`
	}
	if err := json.Unmarshal(create, &createPayload); err != nil {
		t.Fatalf("decode vertical route.create payload: %v", err)
	}
	if createPayload.Route.RouteID == "" || createPayload.Route.SourcePlanetID != sourceID.String() {
		t.Fatalf("vertical route.create payload = %+v, want source route", createPayload.Route)
	}
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationRouteCreate, resolved.PlayerID); err != nil {
		t.Fatalf("post vertical route.create events: %v", err)
	}

	clock.Advance(time.Hour)
	settle := gatewayJSON(t, gameServer, resolved, "vertical-route-settle", realtime.OperationRouteSettle, map[string]any{
		"route_id": createPayload.Route.RouteID,
	}, 4)
	assertPayloadOmitsInternalMapIdentity(t, "vertical route.settle response", settle)
	var settlePayload struct {
		Settlement routeSettlementPayload `json:"settlement"`
	}
	if err := json.Unmarshal(settle, &settlePayload); err != nil {
		t.Fatalf("decode vertical route.settle payload: %v", err)
	}
	if settlePayload.Settlement.AddedAmount <= 0 || settlePayload.Settlement.ResourceItemID != "refined_alloy" {
		t.Fatalf("vertical route settlement = %+v, want refined_alloy transfer", settlePayload.Settlement)
	}
	assertStoredRouteStorageQuantity(t, gameServer, sourceID, "refined_alloy", 120)
	assertStoredRouteStorageQuantity(t, gameServer, destinationID, "refined_alloy", 40)
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationRouteSettle, resolved.PlayerID); err != nil {
		t.Fatalf("post vertical route.settle events: %v", err)
	}
}

func playableVerticalPortalToDestination(t *testing.T, gameServer *Server, resolved auth.ResolvedSession) {
	t.Helper()

	moveTestPlayerEntity(gameServer, resolved.PlayerID, world.Vec2{X: 9800, Y: 5000})
	portal := gatewayJSON(t, gameServer, resolved, "vertical-portal", realtime.OperationPortalEnter, map[string]any{
		"portal_id": "east_gate",
	}, 5)
	assertPayloadOmitsInternalMapIdentity(t, "vertical portal response", portal)
	var payload struct {
		Accepted       bool                 `json:"accepted"`
		ToPublicMapKey string               `json:"to_public_map_key"`
		Snapshot       worldSnapshotPayload `json:"snapshot"`
	}
	if err := json.Unmarshal(portal, &payload); err != nil {
		t.Fatalf("decode vertical portal payload: %v", err)
	}
	if !payload.Accepted || payload.ToPublicMapKey != "1-2" || payload.Snapshot.Map.PublicMapKey != "1-2" {
		t.Fatalf("vertical portal payload = %+v, want public 1-2", payload)
	}
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationPortalEnter, resolved.PlayerID); err != nil {
		t.Fatalf("post vertical portal events: %v", err)
	}
}

func playableVerticalScanClaim(t *testing.T, gameServer *Server, resolved auth.ResolvedSession) string {
	t.Helper()

	scan := gatewayJSON(t, gameServer, resolved, "vertical-scan", realtime.OperationScanPulse, map[string]any{}, 6)
	assertPayloadOmitsScannerNoFogTruth(t, "vertical scan response", scan)
	assertPayloadOmitsActiveMapInternalTruth(t, "vertical scan response", scan)
	var scanPayload struct {
		Scan scanPulsePayload `json:"scan"`
	}
	if err := json.Unmarshal(scan, &scanPayload); err != nil {
		t.Fatalf("decode vertical scan payload: %v", err)
	}
	if scanPayload.Scan.PlanetID == "" {
		t.Fatalf("vertical scan payload = %+v, want discovered planet", scanPayload.Scan)
	}
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationScanPulse, resolved.PlayerID); err != nil {
		t.Fatalf("post vertical scan events: %v", err)
	}

	detail := gatewayJSON(t, gameServer, resolved, "vertical-detail", realtime.OperationPlanetDetail, map[string]any{
		"planet_id": scanPayload.Scan.PlanetID,
	}, 7)
	var detailPayload struct {
		PlanetDetail planetDetailPayload `json:"planet_detail"`
	}
	if err := json.Unmarshal(detail, &detailPayload); err != nil {
		t.Fatalf("decode vertical detail payload: %v", err)
	}
	moveTestPlayerEntity(gameServer, resolved.PlayerID, world.Vec2{
		X: float64(detailPayload.PlanetDetail.Coordinates.X),
		Y: float64(detailPayload.PlanetDetail.Coordinates.Y),
	})

	claim := gatewayJSON(t, gameServer, resolved, "vertical-claim", realtime.OperationDiscoveryClaimPlanet, map[string]any{
		"planet_id": scanPayload.Scan.PlanetID,
	}, 8)
	assertPayloadOmitsPlayerOwner(t, "vertical claim response", claim, resolved.PlayerID)
	var claimPayload struct {
		PlanetDetail planetDetailPayload               `json:"planet_detail"`
		Inventory    inventorySnapshotPayload          `json:"inventory"`
		Production   planetProductionCollectionPayload `json:"production"`
	}
	if err := json.Unmarshal(claim, &claimPayload); err != nil {
		t.Fatalf("decode vertical claim payload: %v", err)
	}
	if claimPayload.PlanetDetail.OwnerStatus != "owned_by_you" || len(claimPayload.Production.Planets) == 0 {
		t.Fatalf("vertical claim payload = %+v production=%+v, want owned production", claimPayload.PlanetDetail, claimPayload.Production)
	}
	if inventorySnapshotHasStackQuantity(claimPayload.Inventory, "x_core", 1) {
		t.Fatalf("vertical claim inventory = %+v, want x_core consumed", claimPayload.Inventory)
	}
	if _, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationDiscoveryClaimPlanet, resolved.PlayerID); err != nil {
		t.Fatalf("post vertical claim events: %v", err)
	}
	return scanPayload.Scan.PlanetID
}

func gatewayJSON(
	t *testing.T,
	gameServer *Server,
	resolved auth.ResolvedSession,
	requestID string,
	op realtime.Operation,
	payload map[string]any,
	clientSeq uint64,
) json.RawMessage {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"request_id": requestID,
		"op":         op,
		"payload":    payload,
		"client_seq": clientSeq,
		"v":          1,
	})
	if err != nil {
		t.Fatalf("marshal gateway request %s: %v", requestID, err)
	}
	response := gameServer.runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), body)
	if response.HasError {
		t.Fatalf("%s response error = %+v, want success", requestID, response.Error)
	}
	return response.Response.Payload
}

func cargoPayloadHasItem(cargo cargoSnapshotPayload, itemID string, quantity int64) bool {
	for _, item := range cargo.Items {
		if item.ItemID == itemID && item.Quantity == quantity {
			return true
		}
	}
	return false
}

func inventorySnapshotHasStackQuantity(snapshot inventorySnapshotPayload, itemID string, quantity int64) bool {
	for _, item := range snapshot.Stackable {
		if item.ItemID == itemID && item.Quantity >= quantity {
			return true
		}
	}
	return false
}

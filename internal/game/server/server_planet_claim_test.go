package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestClaimPlanetSucceedsForKnownNearbyPlanetAndEmitsSafeOwnerEvents(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "claim-owner@example.com", "Claim Owner")
	ownerSecond, err := gameServer.runtime.Auth.Login(context.Background(), auth.LoginInput{
		Email:    owner.Email.String(),
		Password: "correct-password",
	})
	if err != nil {
		t.Fatalf("second login: %v", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(ownerSecond.Session); err != nil {
		t.Fatalf("ensure second owner session: %v", err)
	}
	other := createResolvedRuntimeSession(t, gameServer, "claim-viewer@example.com", "Claim Viewer")
	planetID := foundation.PlanetID("claim-success-planet")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)
	grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "claim-success-xcore")

	response := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-success", planetID)
	if response.HasError {
		t.Fatalf("claim response error = %+v, want success", response.Error)
	}
	var payload planetClaimResponsePayload
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode claim response: %v", err)
	}
	if !payload.Claim.Accepted || payload.Claim.Duplicate || payload.Claim.Planet.OwnerStatus != "owned_by_you" || !payload.Claim.ProductionIncluded {
		t.Fatalf("claim payload = %+v, want accepted owned planet with production", payload.Claim)
	}
	if payload.Claim.Planet.PublicMapKey != "1-1" || payload.PlanetDetail.Coordinates.X != 120 || len(payload.Production.Planets) != 1 {
		t.Fatalf("claim refreshed payload = %+v, want safe active-map detail and production", payload)
	}
	if stack := inventoryStackQuantityForTest(gameServer, owner.PlayerID, "x_core"); stack != 0 {
		t.Fatalf("x_core quantity = %d, want consumed", stack)
	}
	planet, ok, err := gameServer.runtime.Discovery.Planet(planetID)
	if err != nil || !ok {
		t.Fatalf("claimed planet lookup = ok %v err %v, want ok", ok, err)
	}
	if planet.OwnerPlayerID != owner.PlayerID {
		t.Fatalf("planet owner = %q, want %q", planet.OwnerPlayerID, owner.PlayerID)
	}
	if _, ok, err := gameServer.runtime.Production.Snapshot(planetID); err != nil || !ok {
		t.Fatalf("production snapshot = ok %v err %v, want initialized", ok, err)
	}

	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationDiscoveryClaimPlanet, owner.PlayerID)
	if err != nil {
		t.Fatalf("post claim events: %v", err)
	}
	for _, sessionID := range []auth.SessionID{owner.SessionID, ownerSecond.Session.SessionID} {
		events := eventsBySession[sessionID]
		claimed := requireEventTypeForTest(t, events, realtime.EventPlanetClaimed)
		requireEventTypeForTest(t, events, realtime.EventKnownPlanets)
		requireEventTypeForTest(t, events, realtime.EventPlanetDetail)
		requireEventTypeForTest(t, events, realtime.EventProductionSummary)
		requireEventTypeForTest(t, events, realtime.EventInventorySnapshot)
		assertClaimedEventSafeForTest(t, claimed, owner.PlayerID)
	}
	if events := eventsBySession[other.SessionID]; len(events) != 0 {
		t.Fatalf("unrelated player events = %+v, want none", events)
	}
}

func TestClaimPlanetDuplicateRetryDoesNotConsumeSecondXCore(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "claim-duplicate@example.com", "Claim Duplicate")
	planetID := foundation.PlanetID("claim-duplicate-planet")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)
	grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "claim-duplicate-xcore")

	first := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-duplicate-first", planetID)
	if first.HasError {
		t.Fatalf("first claim error = %+v, want success", first.Error)
	}
	claimReference, err := planetClaimReference(owner.PlayerID, planetID)
	if err != nil {
		t.Fatalf("planetClaimReference: %v", err)
	}
	references := gameServer.runtime.ClaimLifecycles.ClaimReferences()
	if len(references) != 1 || references[0] != claimReference {
		t.Fatalf("claim lifecycle references after first claim = %+v, want [%q]", references, claimReference)
	}
	plan, ok, err := gameServer.runtime.ClaimLifecycles.CommittedClaimDurableLifecyclePlan(claimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimDurableLifecyclePlan() = ok %v err %v, want ok nil", ok, err)
	}
	if !plan.HasProductionInit ||
		plan.Begin.Boundary.ClaimReference != claimReference ||
		plan.Commit.Boundary.ClaimReference != claimReference ||
		plan.ProductionInitialized.Initialization.ClaimReference != claimReference {
		t.Fatalf("claim lifecycle plan = %+v, want begin/production/commit evidence", plan)
	}
	initReferences := gameServer.runtime.ClaimProductionInitializations.ClaimReferences()
	if len(initReferences) != 1 || initReferences[0] != claimReference {
		t.Fatalf("claim production init references after first claim = %+v, want [%q]", initReferences, claimReference)
	}
	initPlan, ok, err := gameServer.runtime.ClaimProductionInitializations.CommittedClaimProductionInitializationDurablePlan(claimReference)
	if err != nil || !ok {
		t.Fatalf("CommittedClaimProductionInitializationDurablePlan() = ok %v err %v, want ok nil", ok, err)
	}
	if initPlan.Initialization.ClaimReference != claimReference || initPlan.Boundary.Status != discovery.ClaimBoundaryStatusComplete {
		t.Fatalf("claim production init durable plan = %+v, want complete boundary evidence", initPlan)
	}
	if events, err := gameServer.runtime.postCommandEvents(owner.SessionID, realtime.OperationDiscoveryClaimPlanet, owner.PlayerID); err != nil {
		t.Fatalf("post first claim events: %v", err)
	} else if len(events) == 0 {
		t.Fatal("first claim events missing")
	}

	second := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-duplicate-second", planetID)
	if second.HasError {
		t.Fatalf("duplicate claim error = %+v, want success", second.Error)
	}
	var payload planetClaimResponsePayload
	if err := json.Unmarshal(second.Response.Payload, &payload); err != nil {
		t.Fatalf("decode duplicate claim: %v", err)
	}
	if !payload.Claim.Duplicate || !payload.Claim.Accepted {
		t.Fatalf("duplicate claim payload = %+v, want duplicate accepted", payload.Claim)
	}
	if got := claimXCoreDecreaseLedgerCountForTest(gameServer, owner.PlayerID); got != 1 {
		t.Fatalf("x_core decrease ledger entries = %d, want one", got)
	}
	if references := gameServer.runtime.ClaimLifecycles.ClaimReferences(); len(references) != 1 || references[0] != claimReference {
		t.Fatalf("claim lifecycle references after duplicate = %+v, want stable [%q]", references, claimReference)
	}
	if references := gameServer.runtime.ClaimProductionInitializations.ClaimReferences(); len(references) != 1 || references[0] != claimReference {
		t.Fatalf("claim production init references after duplicate = %+v, want stable [%q]", references, claimReference)
	}
}

func TestClaimPlanetFailureDoesNotRecordDurableLifecycle(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "claim-failed-lifecycle@example.com", "Claim Failed Lifecycle")
	planetID := foundation.PlanetID("claim-failed-lifecycle-planet")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)

	response := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-failed-lifecycle", planetID)
	if !response.HasError {
		t.Fatalf("claim response error missing, want X Core failure")
	}
	if references := gameServer.runtime.ClaimLifecycles.ClaimReferences(); len(references) != 0 {
		t.Fatalf("claim lifecycle references after failed claim = %+v, want none", references)
	}
	if references := gameServer.runtime.ClaimProductionInitializations.ClaimReferences(); len(references) != 0 {
		t.Fatalf("claim production init references after failed claim = %+v, want none", references)
	}
}

func TestClaimPlanetMarksCoordinateScrollMarketListingsStale(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "claim-coordinate-market@example.com", "Claim Coordinate Market")
	buyer := createResolvedRuntimeSession(t, gameServer, "claim-coordinate-buyer@example.com", "Claim Coordinate Buyer")
	planetID := foundation.PlanetID("claim-coordinate-market-planet")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)

	create := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-claim-coordinate-create","op":"intel.coordinate_item.create","payload":{"planet_id":"`+planetID.String()+`"},"client_seq":1,"v":1}`),
	)
	if create.HasError {
		t.Fatalf("coordinate create response error = %+v, want success", create.Error)
	}
	var createPayload struct {
		CoordinateItem intelCoordinateItemPayload `json:"coordinate_item"`
	}
	if err := json.Unmarshal(create.Response.Payload, &createPayload); err != nil {
		t.Fatalf("decode coordinate create payload: %v", err)
	}

	list := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(owner.SessionID.String()),
		[]byte(`{"request_id":"request-claim-coordinate-listing","op":"market.create_listing","payload":{"item_id":"planet_coordinate_scroll","item_instance_id":"`+createPayload.CoordinateItem.ItemInstanceID+`","quantity":1,"unit_price":75},"client_seq":2,"v":1}`),
	)
	if list.HasError {
		t.Fatalf("market create listing response error = %+v, want success", list.Error)
	}
	var listPayload marketMutationPayload
	if err := json.Unmarshal(list.Response.Payload, &listPayload); err != nil {
		t.Fatalf("decode market create listing payload: %v", err)
	}
	listingID, err := foundation.ParseListingID(listPayload.Listing.ListingID)
	if err != nil {
		t.Fatalf("parse listing id: %v", err)
	}
	before, ok := gameServer.runtime.Market.Listing(listingID)
	if !ok || before.Status != market.ListingStatusActive {
		t.Fatalf("coordinate listing before claim = %+v ok=%v, want active", before, ok)
	}
	gameServer.runtime.beginPlanetClaimMarketGuard(planetID)
	guardedBuy := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(buyer.SessionID.String()),
		[]byte(`{"request_id":"request-claim-coordinate-buy-guarded","op":"market.buy","payload":{"listing_id":"`+listingID.String()+`","quantity":1},"client_seq":1,"v":1}`),
	)
	gameServer.runtime.endPlanetClaimMarketGuard(planetID)
	if !guardedBuy.HasError || guardedBuy.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("buy guarded coordinate listing response = %+v, want forbidden", guardedBuy)
	}
	if guardedListing, ok := gameServer.runtime.Market.Listing(listingID); !ok || guardedListing.Status != market.ListingStatusActive {
		t.Fatalf("coordinate listing after guarded buy = %+v ok=%v, want still active before claim", guardedListing, ok)
	}
	if _, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationMarketCreateListing, owner.PlayerID); err != nil {
		t.Fatalf("drain pre-claim market events: %v", err)
	}

	grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "claim-coordinate-market-xcore")
	claim := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-coordinate-market", planetID)
	if claim.HasError {
		t.Fatalf("claim response error = %+v, want success", claim.Error)
	}
	var claimPayload planetClaimResponsePayload
	if err := json.Unmarshal(claim.Response.Payload, &claimPayload); err != nil {
		t.Fatalf("decode claim payload: %v", err)
	}
	if claimPayload.Claim.StaleListingCount != 1 {
		t.Fatalf("stale listing count = %d, want one coordinate listing marked stale", claimPayload.Claim.StaleListingCount)
	}

	after, ok := gameServer.runtime.Market.Listing(listingID)
	if !ok {
		t.Fatalf("coordinate listing %s missing after claim", listingID)
	}
	if after.Status != market.ListingStatusStale || after.StaleReason != "planet_claimed" || after.StaleAt == nil {
		t.Fatalf("coordinate listing after claim = %+v, want stale planet_claimed", after)
	}
	staleRetry, err := (runtimeClaimListedIntelStaleMarker{
		market: gameServer.runtime.Market,
		intel:  gameServer.runtime.Intel,
	}).MarkClaimedPlanetListingsStale(discovery.ClaimListedIntelStaleInput{
		PlayerID:        owner.PlayerID,
		PlanetID:        planetID,
		ClaimReference:  "claim-coordinate-market-retry",
		Reason:          "planet_claimed",
		ClaimedAt:       gameServer.runtime.clock.Now().UTC(),
		SourceReference: "planet.claimed:claim-coordinate-market-retry",
	})
	if err != nil {
		t.Fatalf("stale marker retry error = %v, want nil", err)
	}
	if staleRetry.MarkedCount != 1 {
		t.Fatalf("stale marker retry count = %d, want stable count for already-stale coordinate listing", staleRetry.MarkedCount)
	}
	eventsBySession, err := gameServer.runtime.postCommandEventsBySession(owner.SessionID, realtime.OperationDiscoveryClaimPlanet, owner.PlayerID)
	if err != nil {
		t.Fatalf("post claim events: %v", err)
	}
	updated := requireEventTypeForTest(t, eventsBySession[buyer.SessionID], realtime.EventMarketListingUpdated)
	var updatedListing marketListingPayload
	if err := json.Unmarshal(updated.Payload, &updatedListing); err != nil {
		t.Fatalf("decode stale market listing update: %v", err)
	}
	if updatedListing.ListingID != listingID.String() || updatedListing.Status != market.ListingStatusStale.String() {
		t.Fatalf("buyer stale market event = %+v, want listing %s stale", updatedListing, listingID)
	}

	buy := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(buyer.SessionID.String()),
		[]byte(`{"request_id":"request-claim-coordinate-buy-stale","op":"market.buy","payload":{"listing_id":"`+listingID.String()+`","quantity":1},"client_seq":1,"v":1}`),
	)
	if !buy.HasError || buy.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("buy stale coordinate listing response = %+v, want forbidden", buy)
	}
}

func TestClaimPlanetSucceedsOnSeededDestinationMap(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSessionOnMap(t, gameServer, "claim-map-two@example.com", "Claim Map Two", "map_1_2", "west_gate")
	planetID := foundation.PlanetID("claim-map-two-planet")
	coordinates := world.Vec2{X: 520, Y: 5000}
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, "map_1_2", coordinates, 1)
	grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "claim-map-two-xcore")

	response := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-map-two", planetID)
	if response.HasError {
		t.Fatalf("claim response error = %+v, want success", response.Error)
	}
	assertPlanetClaimJSONSafeForTest(t, "map two claim response", response.Response.Payload, owner.PlayerID)
	var payload planetClaimResponsePayload
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode claim response: %v", err)
	}
	if !payload.Claim.Accepted || payload.Claim.Duplicate || !payload.Claim.ProductionIncluded {
		t.Fatalf("claim payload = %+v, want accepted non-duplicate with production", payload.Claim)
	}
	if payload.Claim.Planet.PublicMapKey != "1-2" || payload.Claim.Planet.OwnerStatus != "owned_by_you" {
		t.Fatalf("claim planet summary = %+v, want owned public map 1-2", payload.Claim.Planet)
	}
	if payload.PlanetDetail.PublicMapKey != "1-2" ||
		payload.PlanetDetail.Coordinates.X != coordinates.X ||
		payload.PlanetDetail.Coordinates.Y != coordinates.Y ||
		payload.PlanetDetail.Production == nil {
		t.Fatalf("planet detail = %+v, want safe map 1-2 coordinates and initialized production", payload.PlanetDetail)
	}
	if len(payload.Production.Planets) != 1 ||
		payload.Production.Planets[0].PlanetID != planetID.String() ||
		payload.Production.Planets[0].PublicMapKey != "1-2" ||
		!payload.Production.Planets[0].ProductionEnabled ||
		payload.Production.Planets[0].Storage.CapacityUnits == 0 {
		t.Fatalf("production payload = %+v, want initialized map 1-2 production", payload.Production)
	}
	if stack := inventoryStackQuantityForTest(gameServer, owner.PlayerID, "x_core"); stack != 0 {
		t.Fatalf("x_core quantity = %d, want consumed", stack)
	}
	if got := claimXCoreDecreaseLedgerCountForTest(gameServer, owner.PlayerID); got != 1 {
		t.Fatalf("x_core decrease ledger entries = %d, want one", got)
	}
	planet, ok, err := gameServer.runtime.Discovery.Planet(planetID)
	if err != nil || !ok {
		t.Fatalf("claimed planet lookup = ok %v err %v, want ok", ok, err)
	}
	if planet.OwnerPlayerID != owner.PlayerID {
		t.Fatalf("planet owner = %q, want %q", planet.OwnerPlayerID, owner.PlayerID)
	}
	if _, ok, err := gameServer.runtime.Production.Snapshot(planetID); err != nil || !ok {
		t.Fatalf("production snapshot = ok %v err %v, want initialized", ok, err)
	}

	events, err := gameServer.runtime.postCommandEvents(owner.SessionID, realtime.OperationDiscoveryClaimPlanet, owner.PlayerID)
	if err != nil {
		t.Fatalf("post claim events: %v", err)
	}
	for _, event := range events {
		assertPlanetClaimJSONSafeForTest(t, string(event.Type), event.Payload, owner.PlayerID)
	}
	assertClaimedEventSafeForTest(t, requireEventTypeForTest(t, events, realtime.EventPlanetClaimed), owner.PlayerID, "1-2")
}

func TestClaimPlanetSucceedsOnSeededPVPMap(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSessionOnMap(t, gameServer, "claim-map-three@example.com", "Claim Map Three", "map_1_3", "west_gate")
	planetID := foundation.PlanetID("claim-map-three-planet")
	coordinates := world.Vec2{X: 520, Y: 5000}
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, "map_1_3", coordinates, 1)
	grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "claim-map-three-xcore")

	response := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-map-three", planetID)
	if response.HasError {
		t.Fatalf("claim response error = %+v, want success", response.Error)
	}
	assertPlanetClaimJSONSafeForTest(t, "map three claim response", response.Response.Payload, owner.PlayerID)
	var payload planetClaimResponsePayload
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode claim response: %v", err)
	}
	if !payload.Claim.Accepted || payload.Claim.Duplicate || !payload.Claim.ProductionIncluded {
		t.Fatalf("claim payload = %+v, want accepted non-duplicate with production", payload.Claim)
	}
	if payload.Claim.Planet.PublicMapKey != "1-3" || payload.Claim.Planet.OwnerStatus != "owned_by_you" {
		t.Fatalf("claim planet summary = %+v, want owned public map 1-3", payload.Claim.Planet)
	}
	if payload.PlanetDetail.PublicMapKey != "1-3" ||
		payload.PlanetDetail.Coordinates.X != coordinates.X ||
		payload.PlanetDetail.Coordinates.Y != coordinates.Y ||
		payload.PlanetDetail.Production == nil {
		t.Fatalf("planet detail = %+v, want safe map 1-3 coordinates and initialized production", payload.PlanetDetail)
	}
	if len(payload.Production.Planets) != 1 ||
		payload.Production.Planets[0].PlanetID != planetID.String() ||
		payload.Production.Planets[0].PublicMapKey != "1-3" ||
		!payload.Production.Planets[0].ProductionEnabled ||
		payload.Production.Planets[0].Storage.CapacityUnits == 0 {
		t.Fatalf("production payload = %+v, want initialized map 1-3 production", payload.Production)
	}
	if stack := inventoryStackQuantityForTest(gameServer, owner.PlayerID, "x_core"); stack != 0 {
		t.Fatalf("x_core quantity = %d, want consumed", stack)
	}
	if got := claimXCoreDecreaseLedgerCountForTest(gameServer, owner.PlayerID); got != 1 {
		t.Fatalf("x_core decrease ledger entries = %d, want one", got)
	}
	planet, ok, err := gameServer.runtime.Discovery.Planet(planetID)
	if err != nil || !ok {
		t.Fatalf("claimed planet lookup = ok %v err %v, want ok", ok, err)
	}
	if planet.OwnerPlayerID != owner.PlayerID {
		t.Fatalf("planet owner = %q, want %q", planet.OwnerPlayerID, owner.PlayerID)
	}
	if _, ok, err := gameServer.runtime.Production.Snapshot(planetID); err != nil || !ok {
		t.Fatalf("production snapshot = ok %v err %v, want initialized", ok, err)
	}

	events, err := gameServer.runtime.postCommandEvents(owner.SessionID, realtime.OperationDiscoveryClaimPlanet, owner.PlayerID)
	if err != nil {
		t.Fatalf("post claim events: %v", err)
	}
	for _, event := range events {
		assertPlanetClaimJSONSafeForTest(t, string(event.Type), event.Payload, owner.PlayerID)
	}
	assertClaimedEventSafeForTest(t, requireEventTypeForTest(t, events, realtime.EventPlanetClaimed), owner.PlayerID, "1-3")
}

func TestClaimPlanetRejectsTrustedAndUnknownPayloadFieldsWithoutMutation(t *testing.T) {
	for _, field := range []string{"player_id", "map_id", "coordinates", "owner", "x_core", "production", "unexpected"} {
		t.Run(field, func(t *testing.T) {
			gameServer, _ := newTestServer(t, false)
			owner := createResolvedRuntimeSession(t, gameServer, "claim-field-"+field+"@example.com", "Claim Field")
			planetID := foundation.PlanetID("claim-field-" + strings.ReplaceAll(field, "_", "-"))
			seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)
			grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "claim-field-xcore-"+field)

			response := gameServer.runtime.Gateway.HandleRequest(
				realtime.SessionID(owner.SessionID.String()),
				[]byte(fmt.Sprintf(
					`{"request_id":"request-claim-field-%s","op":"discovery.claim_planet","payload":{"planet_id":%q,%q":"client-authored"},"client_seq":1,"v":1}`,
					strings.ReplaceAll(field, "_", "-"),
					planetID.String(),
					field,
				)),
			)
			if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
				t.Fatalf("claim with field %s response = %+v, want invalid payload", field, response)
			}
			assertClaimDidNotMutateForTest(t, gameServer, owner.PlayerID, planetID, 1)
			if events, err := gameServer.runtime.postCommandEvents(owner.SessionID, realtime.OperationDiscoveryClaimPlanet, owner.PlayerID); err != nil {
				t.Fatalf("post failed claim events: %v", err)
			} else if len(events) != 0 {
				t.Fatalf("failed claim events = %+v, want none", events)
			}
		})
	}
}

func TestClaimPlanetRejectsMissingXCoreWithoutOwnerProductionOrEvents(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "claim-missing-xcore@example.com", "Claim Missing")
	planetID := foundation.PlanetID("claim-missing-xcore-planet")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)

	response := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-missing-xcore", planetID)
	if !response.HasError || response.Error.Error.Code != foundation.CodeForbidden {
		t.Fatalf("missing x_core response = %+v, want forbidden", response)
	}
	assertClaimDidNotMutateForTest(t, gameServer, owner.PlayerID, planetID, 0)
}

func TestClaimPlanetRejectsCrossMapKnownPlanet(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "claim-cross-map@example.com", "Claim Cross")
	planetID := foundation.PlanetID("claim-cross-map-planet")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, "map_1_2", world.Vec2{X: 120, Y: 0}, 1)
	grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "claim-cross-map-xcore")

	response := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-cross-map", planetID)
	if !response.HasError || response.Error.Error.Code != foundation.CodeOutOfRange {
		t.Fatalf("cross-map claim response = %+v, want out of range", response)
	}
	assertClaimDidNotMutateForTest(t, gameServer, owner.PlayerID, planetID, 1)
}

func TestClaimPlanetRejectsSameMapOutOfRange(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "claim-out-range@example.com", "Claim Range")
	planetID := foundation.PlanetID("claim-out-range-planet")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: runtimePlanetClaimRange + 50, Y: 0}, 1)
	grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "claim-out-range-xcore")

	response := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-out-range", planetID)
	if !response.HasError || response.Error.Error.Code != foundation.CodeOutOfRange {
		t.Fatalf("out-of-range claim response = %+v, want out of range", response)
	}
	assertClaimDidNotMutateForTest(t, gameServer, owner.PlayerID, planetID, 1)
}

func TestClaimPlanetRejectsRankTooLow(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "claim-rank-low@example.com", "Claim Rank")
	planetID := foundation.PlanetID("claim-rank-low-planet")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 2)
	grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "claim-rank-low-xcore")

	response := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-rank-low", planetID)
	if !response.HasError || response.Error.Error.Code != foundation.CodeRankTooLow {
		t.Fatalf("rank-low claim response = %+v, want rank too low", response)
	}
	assertClaimDidNotMutateForTest(t, gameServer, owner.PlayerID, planetID, 1)
}

func claimPlanetForTest(t *testing.T, gameServer *Server, sessionID auth.SessionID, requestID string, planetID foundation.PlanetID) realtime.CachedResponse {
	t.Helper()
	return gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(sessionID.String()),
		[]byte(fmt.Sprintf(
			`{"request_id":%q,"op":"discovery.claim_planet","payload":{"planet_id":%q},"client_seq":1,"v":1}`,
			requestID,
			planetID.String(),
		)),
	)
}

func seedKnownClaimPlanetForTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID, planetID foundation.PlanetID, mapID worldmaps.MapID, coordinates world.Vec2, level int) {
	t.Helper()
	definition, ok := gameServer.runtime.mapCatalog.Get(mapID)
	if !ok {
		t.Fatalf("map %q missing", mapID)
	}
	now := gameServer.runtime.clock.Now().UTC()
	if _, err := gameServer.runtime.Discovery.MaterializePlanet(discovery.MaterializePlanetInput{
		CandidateKey: discovery.PlanetMaterializationKey("candidate-" + planetID.String()),
		Planet: discovery.Planet{
			ID:           planetID,
			WorldID:      definition.WorldID,
			ZoneID:       definition.ZoneID,
			Coordinates:  coordinates,
			Biome:        discovery.PlanetBiomeOuterDrift,
			Type:         discovery.PlanetTypeIce,
			Rarity:       discovery.PlanetRarityUncommon,
			Level:        level,
			DiscoveredAt: now,
			DiscoveredBy: playerID,
		},
	}); err != nil {
		t.Fatalf("MaterializePlanet(%s): %v", planetID, err)
	}
	if _, _, err := gameServer.runtime.Discovery.UpsertPlayerPlanetIntel(discovery.PlayerPlanetIntel{
		PlayerID:        playerID,
		PlanetID:        planetID,
		WorldID:         definition.WorldID,
		ZoneID:          definition.ZoneID,
		Coordinates:     coordinates,
		State:           discovery.IntelStateVerified,
		Confidence:      100,
		LastSeenAt:      now,
		SourceType:      discovery.IntelSourceScanSuccess,
		SourceReference: "scan-" + planetID.String(),
	}); err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel(%s): %v", planetID, err)
	}
}

func grantClaimXCoreForTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID, quantity int64, referenceSuffix string) {
	t.Helper()
	definition, ok := gameServer.runtime.itemCatalog["x_core"]
	if !ok {
		t.Fatal("runtime x_core definition missing")
	}
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		t.Fatalf("x_core location: %v", err)
	}
	addTestInventoryStack(t, gameServer, playerID, definition, quantity, location, referenceSuffix)
}

func assertClaimDidNotMutateForTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID, planetID foundation.PlanetID, wantXCore int64) {
	t.Helper()
	planet, ok, err := gameServer.runtime.Discovery.Planet(planetID)
	if err != nil || !ok {
		t.Fatalf("planet lookup = ok %v err %v, want ok", ok, err)
	}
	if !planet.OwnerPlayerID.IsZero() {
		t.Fatalf("planet owner = %q, want unowned", planet.OwnerPlayerID)
	}
	if _, ok, err := gameServer.runtime.Production.Snapshot(planetID); err != nil || ok {
		t.Fatalf("production snapshot = ok %v err %v, want absent", ok, err)
	}
	if got := inventoryStackQuantityForTest(gameServer, playerID, "x_core"); got != wantXCore {
		t.Fatalf("x_core quantity = %d, want unchanged %d", got, wantXCore)
	}
}

func inventoryStackQuantityForTest(gameServer *Server, playerID foundation.PlayerID, itemID foundation.ItemID) int64 {
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		return 0
	}
	var total int64
	for _, item := range gameServer.runtime.Inventory.StackableItems() {
		if item.OwnerPlayerID == playerID && item.ItemID == itemID && item.Location == location {
			total += item.Quantity.Int64()
		}
	}
	return total
}

func claimXCoreDecreaseLedgerCountForTest(gameServer *Server, playerID foundation.PlayerID) int {
	count := 0
	for _, entry := range gameServer.runtime.Inventory.ItemLedgerEntries() {
		if entry.PlayerID == playerID &&
			entry.ItemID == "x_core" &&
			entry.Action == economy.LedgerActionDecrease &&
			entry.Reason == economy.LedgerReason("planet_claim") {
			count++
		}
	}
	return count
}

func assertPlanetClaimJSONSafeForTest(t *testing.T, label string, payload json.RawMessage, playerID foundation.PlayerID) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{
		"world_id",
		"zone_id",
		"internal_map_id",
		"map_1_2",
		"map_1_3",
		"owner_player_id",
		"player_id",
		playerID.String(),
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}

func assertClaimedEventSafeForTest(t *testing.T, event realtime.EventEnvelope, playerID foundation.PlayerID, wantPublicMapKey ...string) {
	t.Helper()
	if event.Type != realtime.EventPlanetClaimed {
		t.Fatalf("event type = %s, want %s", event.Type, realtime.EventPlanetClaimed)
	}
	assertPlanetClaimJSONSafeForTest(t, "planet.claimed event", event.Payload, playerID)
	if strings.Contains(string(event.Payload), "coordinates") {
		t.Fatalf("planet.claimed event leaked coordinates in %s", event.Payload)
	}
	var payload planetClaimedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode planet.claimed event: %v", err)
	}
	publicMapKey := "1-1"
	if len(wantPublicMapKey) > 0 {
		publicMapKey = wantPublicMapKey[0]
	}
	if !payload.Accepted || payload.Planet.OwnerStatus != "owned_by_you" || payload.Planet.PublicMapKey != publicMapKey {
		t.Fatalf("planet.claimed payload = %+v, want safe owned summary on %s", payload, publicMapKey)
	}
}

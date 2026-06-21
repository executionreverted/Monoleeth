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

func assertClaimedEventSafeForTest(t *testing.T, event realtime.EventEnvelope, playerID foundation.PlayerID) {
	t.Helper()
	if event.Type != realtime.EventPlanetClaimed {
		t.Fatalf("event type = %s, want %s", event.Type, realtime.EventPlanetClaimed)
	}
	raw := string(event.Payload)
	for _, forbidden := range []string{
		"world_id",
		"zone_id",
		"internal_map_id",
		"coordinates",
		"owner_player_id",
		playerID.String(),
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("planet.claimed event leaked %q in %s", forbidden, raw)
		}
	}
	var payload planetClaimedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode planet.claimed event: %v", err)
	}
	if !payload.Accepted || payload.Planet.OwnerStatus != "owned_by_you" || payload.Planet.PublicMapKey != "1-1" {
		t.Fatalf("planet.claimed payload = %+v, want safe owned summary", payload)
	}
}

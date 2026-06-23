package server

import (
	"encoding/json"
	"errors"
	"testing"

	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestClaimPlanetGatewayRetryRepairsMissingProductionInitialization(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "claim-gateway-repair@example.com", "Claim Gateway Repair")
	planetID := foundation.PlanetID("claim-gateway-repair-planet")
	seedKnownClaimPlanetForTest(t, gameServer, owner.PlayerID, planetID, worldmaps.StarterMapID, world.Vec2{X: 120, Y: 0}, 1)
	grantClaimXCoreForTest(t, gameServer, owner.PlayerID, 1, "claim-gateway-repair-xcore")

	initErr := errors.New("production init unavailable")
	initializer := installRuntimeClaimServiceWithFlakyProductionInitializerForTest(t, gameServer, initErr)

	failed := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-gateway-repair-fail", planetID)
	if !failed.HasError || failed.Error.Error.Code != foundation.CodeInternal {
		t.Fatalf("first claim response = %+v, want internal production init failure", failed)
	}
	planet, ok, err := gameServer.runtime.Discovery.Planet(planetID)
	if err != nil || !ok {
		t.Fatalf("planet lookup after failed init = ok %v err %v, want ok nil", ok, err)
	}
	if planet.OwnerPlayerID != owner.PlayerID {
		t.Fatalf("planet owner after failed init = %q, want authoritative owner %q", planet.OwnerPlayerID, owner.PlayerID)
	}
	if _, ok, err := gameServer.runtime.Production.Snapshot(planetID); err != nil || ok {
		t.Fatalf("production snapshot after failed init = ok %v err %v, want missing", ok, err)
	}
	if got := claimXCoreDecreaseLedgerCountForTest(gameServer, owner.PlayerID); got != 1 {
		t.Fatalf("x_core debit after failed init = %d, want one", got)
	}
	if events, err := gameServer.runtime.postCommandEvents(owner.SessionID, realtime.OperationDiscoveryClaimPlanet, owner.PlayerID); err != nil {
		t.Fatalf("post failed claim events: %v", err)
	} else if len(events) != 0 {
		t.Fatalf("failed claim events = %+v, want none before repair", events)
	}

	initializer.err = nil
	repaired := claimPlanetForTest(t, gameServer, owner.SessionID, "request-claim-gateway-repair-retry", planetID)
	if repaired.HasError {
		t.Fatalf("retry claim response error = %+v, want repaired success", repaired.Error)
	}
	payload := decodePlanetClaimResponseForTest(t, repaired.Response.Payload)
	if !payload.Claim.Accepted || payload.Claim.Duplicate || payload.Claim.AlreadyOwned || !payload.Claim.ProductionIncluded {
		t.Fatalf("repaired claim payload = %+v, want original completion with production", payload.Claim)
	}
	if payload.PlanetDetail.Production == nil ||
		len(payload.Production.Planets) != 1 ||
		payload.Production.Planets[0].PlanetID != planetID.String() ||
		!payload.Production.Planets[0].ProductionEnabled ||
		payload.Production.Planets[0].Storage.CapacityUnits == 0 {
		t.Fatalf("repaired production payload = detail %+v summary %+v, want initialized production", payload.PlanetDetail, payload.Production)
	}
	if got := claimXCoreDecreaseLedgerCountForTest(gameServer, owner.PlayerID); got != 1 {
		t.Fatalf("x_core debit after repair = %d, want still one", got)
	}

	claimReference, err := planetClaimReference(owner.PlayerID, planetID)
	if err != nil {
		t.Fatalf("planetClaimReference: %v", err)
	}
	initPlan, ok, err := gameServer.runtime.ClaimProductionInitializations.CommittedClaimProductionInitializationDurablePlan(claimReference)
	if err != nil || !ok || initPlan.Boundary.Status != discovery.ClaimBoundaryStatusComplete {
		t.Fatalf("production init durable plan after repair = %+v ok %v err %v, want complete", initPlan, ok, err)
	}
	lifecycle, ok, err := gameServer.runtime.ClaimLifecycles.CommittedClaimDurableLifecyclePlan(claimReference)
	if err != nil || !ok || !lifecycle.HasProductionInit || lifecycle.Commit.Boundary.Status != discovery.ClaimBoundaryStatusComplete {
		t.Fatalf("claim lifecycle after repair = %+v ok %v err %v, want complete with production init", lifecycle, ok, err)
	}

	events, err := gameServer.runtime.postCommandEvents(owner.SessionID, realtime.OperationDiscoveryClaimPlanet, owner.PlayerID)
	if err != nil {
		t.Fatalf("post repaired claim events: %v", err)
	}
	requireEventTypeForTest(t, events, realtime.EventPlanetClaimed)
	requireEventTypeForTest(t, events, realtime.EventPlanetDetail)
	requireEventTypeForTest(t, events, realtime.EventProductionSummary)
	requireEventTypeForTest(t, events, realtime.EventInventorySnapshot)
}

type flakyClaimProductionInitializerForTest struct {
	inner *production.ClaimProductionInitializer
	err   error
}

func (initializer *flakyClaimProductionInitializerForTest) InitializeClaimProduction(input discovery.ClaimProductionInitializeInput) (discovery.ClaimProductionInitializeResult, error) {
	if initializer.err != nil {
		return discovery.ClaimProductionInitializeResult{}, initializer.err
	}
	return initializer.inner.InitializeClaimProduction(input)
}

func installRuntimeClaimServiceWithFlakyProductionInitializerForTest(
	t *testing.T,
	gameServer *Server,
	initErr error,
) *flakyClaimProductionInitializerForTest {
	t.Helper()
	xCoreDefinition, ok := gameServer.runtime.itemCatalog["x_core"]
	if !ok {
		t.Fatal("runtime x_core definition missing")
	}
	inner, err := production.NewClaimProductionInitializer(production.ClaimProductionInitializerConfig{
		Store: gameServer.runtime.Production,
		Defaults: production.ClaimProductionInitializationDefaults{
			StorageCapacityUnits:  runtimeClaimProductionStorageCapacity,
			EnergyCapacityPerHour: runtimeClaimProductionEnergyCapacity,
		},
	})
	if err != nil {
		t.Fatalf("NewClaimProductionInitializer() error = %v, want nil", err)
	}
	initializer := &flakyClaimProductionInitializerForTest{
		inner: inner,
		err:   initErr,
	}
	claimService, err := discovery.NewClaimService(discovery.ClaimServiceConfig{
		Store:                  gameServer.runtime.Discovery,
		Clock:                  gameServer.runtime.clock,
		Ranks:                  runtimeClaimRankProvider{progression: gameServer.runtime.Progression},
		Proximity:              runtimeClaimProximityProvider{runtime: gameServer.runtime},
		XCoreConsumer:          runtimeClaimXCoreConsumer{inventory: gameServer.runtime.Inventory},
		ProductionInitializer:  initializer,
		ListedIntelStaleMarker: runtimeClaimListedIntelStaleMarker{market: gameServer.runtime.Market, intel: gameServer.runtime.Intel},
		XCoreItemDefinition:    xCoreDefinition,
	})
	if err != nil {
		t.Fatalf("NewClaimService() error = %v, want nil", err)
	}
	gameServer.runtime.Claim = claimService
	return initializer
}

func decodePlanetClaimResponseForTest(t *testing.T, raw []byte) planetClaimResponsePayload {
	t.Helper()
	var payload planetClaimResponsePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode planet claim response: %v", err)
	}
	return payload
}

package production

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	gameevents "gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

func TestProductionEventPayloadsAndEnvelopeValidate(t *testing.T) {
	state, err := NewPlanetProductionState("planet-1", testTime(0), 25, testTime(0))
	if err != nil {
		t.Fatalf("NewPlanetProductionState() error = %v, want nil", err)
	}
	storage, err := NewPlanetStorage("planet-1", 100, []StoredItem{{ItemID: "iron_ore", Quantity: 10}}, testTime(1))
	if err != nil {
		t.Fatalf("NewPlanetStorage() error = %v, want nil", err)
	}
	initPayload, err := NewProductionStateInitializedPayload(PlanetProductionSnapshot{State: state, Storage: storage})
	if err != nil {
		t.Fatalf("NewProductionStateInitializedPayload() error = %v, want nil", err)
	}
	if initPayload.StorageCapacityUnits != 100 || initPayload.EnergyCapacityPerHour != 25 {
		t.Fatalf("init payload = %+v, want storage 100 energy 25", initPayload)
	}

	storagePayload, err := NewStorageUpdatedPayload(storage)
	if err != nil {
		t.Fatalf("NewStorageUpdatedPayload() error = %v, want nil", err)
	}
	if storagePayload.UsedUnits != 10 {
		t.Fatalf("storage payload used = %d, want 10", storagePayload.UsedUnits)
	}

	event, err := NewProductionEvent("event-1", EventType(EventPlanetStorageUpdated), storagePayload, testTime(2), 7)
	if err != nil {
		t.Fatalf("NewProductionEvent() error = %v, want nil", err)
	}
	if event.Type != EventPlanetStorageUpdated || event.Sequence != 7 || event.EventID != foundation.EventID("event-1") {
		t.Fatalf("event = %+v, want storage update seq 7 event-1", event)
	}
	var decoded StorageUpdatedPayload
	if err := json.Unmarshal(event.Payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal(payload) error = %v, want nil", err)
	}
	if decoded.UsedUnits != 10 || decoded.PlanetID != "planet-1" {
		t.Fatalf("decoded payload = %+v, want planet-1 used 10", decoded)
	}
}

func TestProductionEventRejectsUnknownType(t *testing.T) {
	_, err := NewProductionEvent("event-1", EventType("route.created"), map[string]string{"bad": "scope"}, testTime(0), 1)
	if !errors.Is(err, ErrInvalidProductionEvent) {
		t.Fatalf("unknown event type error = %v, want ErrInvalidProductionEvent", err)
	}
}

func TestProductionSettlementPayloadRejectsInvalidFacts(t *testing.T) {
	base := PlanetProductionSettlementResult{
		PlanetID:         "planet-1",
		SettledAt:        testTime(1),
		ReferenceKey:     mustOfflineSettlementKey(t, "planet-1", wantSettlementWindow(testTime(0), testTime(1))),
		SettlementWindow: wantSettlementWindow(testTime(0), testTime(1)),
		ElapsedApplied:   time.Hour,
		ProducedItems:    []SettlementItemDelta{{ItemID: "iron_ore", Quantity: 1}},
	}
	cases := map[string]PlanetProductionSettlementResult{
		"missing reference": func() PlanetProductionSettlementResult {
			result := base
			result.ReferenceKey = ""
			return result
		}(),
		"missing window": func() PlanetProductionSettlementResult {
			result := base
			result.SettlementWindow = ""
			return result
		}(),
		"window contains colon": func() PlanetProductionSettlementResult {
			result := base
			result.SettlementWindow = "from:to"
			return result
		}(),
		"reference mismatch": func() PlanetProductionSettlementResult {
			result := base
			result.ReferenceKey = mustOfflineSettlementKey(t, result.PlanetID, "other-window")
			return result
		}(),
		"negative elapsed": func() PlanetProductionSettlementResult {
			result := base
			result.ElapsedApplied = -time.Second
			return result
		}(),
		"bad produced item": func() PlanetProductionSettlementResult {
			result := base
			result.ProducedItems = []SettlementItemDelta{{ItemID: "", Quantity: 1}}
			return result
		}(),
		"non-positive consumed quantity": func() PlanetProductionSettlementResult {
			result := base
			result.ConsumedInputs = []SettlementItemDelta{{ItemID: "iron_ore", Quantity: 0}}
			return result
		}(),
		"bad skipped building": func() PlanetProductionSettlementResult {
			result := base
			result.SkippedBuildings = []SettlementSkippedBuilding{{
				BuildingID:            "building-1",
				Reason:                "unknown",
				EnergyCostPerHour:     1,
				EnergyCapacityPerHour: 1,
			}}
			return result
		}(),
		"negative skipped energy": func() PlanetProductionSettlementResult {
			result := base
			result.SkippedBuildings = []SettlementSkippedBuilding{{
				BuildingID:            "building-1",
				Reason:                SettlementSkipReasonEnergyInsufficient,
				EnergyUsedPerHour:     -1,
				EnergyCostPerHour:     1,
				EnergyCapacityPerHour: 1,
			}}
			return result
		}(),
		"energy skip not insufficient": func() PlanetProductionSettlementResult {
			result := base
			result.SkippedBuildings = []SettlementSkippedBuilding{{
				BuildingID:            "building-1",
				Reason:                SettlementSkipReasonEnergyInsufficient,
				EnergyUsedPerHour:     0,
				EnergyCostPerHour:     1,
				EnergyCapacityPerHour: 1,
			}}
			return result
		}(),
	}
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewProductionSettlementPayload(result); err == nil {
				t.Fatal("NewProductionSettlementPayload() error = nil, want invalid fact error")
			}
		})
	}
}

func TestBuildingProducedPayloadRejectsEmptyDeltas(t *testing.T) {
	result := PlanetProductionBuildingResult{
		BuildingID:        "building-1",
		DefinitionID:      ProductionDefinitionIDIronExtractorL1,
		BuildingType:      BuildingTypeIronExtractor,
		Category:          BuildingCategoryExtractor,
		Level:             1,
		ElapsedApplied:    time.Hour,
		EnergyCostPerHour: 1,
	}
	if _, err := NewBuildingProducedPayload("planet-1", testTime(1), result); !errors.Is(err, ErrInvalidProductionEvent) {
		t.Fatalf("NewBuildingProducedPayload() error = %v, want ErrInvalidProductionEvent", err)
	}
}

func TestRouteSettlementPayloadRejectsInvalidFacts(t *testing.T) {
	route := validSettlementRoute(testRouteNow())
	base := RouteSettlementResult{
		RouteID:          route.RouteID,
		SettledAt:        testRouteNow().Add(time.Hour),
		ReferenceKey:     mustRouteSettlementKey(t, route.RouteID, wantSettlementWindow(testRouteNow(), testRouteNow().Add(time.Hour))),
		SettlementWindow: wantSettlementWindow(testRouteNow(), testRouteNow().Add(time.Hour)),
		ElapsedApplied:   time.Hour,
		AfterRoute:       route,
		WantedAmount:     40,
		TakenAmount:      40,
		DeliveredAmount:  40,
		AddedAmount:      40,
	}
	cases := map[string]RouteSettlementResult{
		"missing reference": func() RouteSettlementResult {
			result := base
			result.ReferenceKey = ""
			return result
		}(),
		"missing window": func() RouteSettlementResult {
			result := base
			result.SettlementWindow = ""
			return result
		}(),
		"window contains colon": func() RouteSettlementResult {
			result := base
			result.SettlementWindow = "from:to"
			return result
		}(),
		"reference mismatch": func() RouteSettlementResult {
			result := base
			result.ReferenceKey = mustRouteSettlementKey(t, result.RouteID, "other-window")
			return result
		}(),
		"negative elapsed": func() RouteSettlementResult {
			result := base
			result.ElapsedApplied = -time.Second
			return result
		}(),
		"taken exceeds wanted": func() RouteSettlementResult {
			result := base
			result.TakenAmount = 41
			return result
		}(),
		"loss equation mismatch": func() RouteSettlementResult {
			result := base
			result.LostAmount = 1
			return result
		}(),
		"added exceeds delivered": func() RouteSettlementResult {
			result := base
			result.AddedAmount = 41
			return result
		}(),
		"destination full flag mismatch": func() RouteSettlementResult {
			result := base
			result.DestinationFull = true
			return result
		}(),
		"nan loss percent": func() RouteSettlementResult {
			result := base
			result.LossApplied = true
			result.LossPercent = math.NaN()
			return result
		}(),
		"loss percent without lost units": func() RouteSettlementResult {
			result := base
			result.LossApplied = true
			result.LossPercent = 0.50
			return result
		}(),
		"loss fields without loss flag": func() RouteSettlementResult {
			result := base
			result.TakenAmount = 40
			result.LostAmount = 10
			result.DeliveredAmount = 30
			result.AddedAmount = 30
			return result
		}(),
	}
	for name, result := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRouteSettlementPayload(result); !errors.Is(err, ErrInvalidProductionEvent) {
				t.Fatalf("NewRouteSettlementPayload() error = %v, want ErrInvalidProductionEvent", err)
			}
		})
	}
}

func TestSettlementPayloadsAllowNoOpWithoutEvidence(t *testing.T) {
	productionPayload, err := NewProductionSettlementPayload(PlanetProductionSettlementResult{
		PlanetID:  "planet-1",
		SettledAt: testTime(1),
		NoOp:      true,
	})
	if err != nil {
		t.Fatalf("NewProductionSettlementPayload(no-op) error = %v, want nil", err)
	}
	if productionPayload.ReferenceKey != "" || productionPayload.SettlementWindow != "" {
		t.Fatalf("production no-op evidence = %q/%q, want empty", productionPayload.ReferenceKey, productionPayload.SettlementWindow)
	}

	route := validSettlementRoute(testRouteNow())
	routePayload, err := NewRouteSettlementPayload(RouteSettlementResult{
		RouteID:    route.RouteID,
		SettledAt:  testRouteNow(),
		AfterRoute: route,
		NoOp:       true,
	})
	if err != nil {
		t.Fatalf("NewRouteSettlementPayload(no-op) error = %v, want nil", err)
	}
	if routePayload.ReferenceKey != "" || routePayload.SettlementWindow != "" {
		t.Fatalf("route no-op evidence = %q/%q, want empty", routePayload.ReferenceKey, routePayload.SettlementWindow)
	}
}

func assertProductionEventTypes(t *testing.T, events []gameevents.EventEnvelope, want ...string) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d; events = %+v", len(events), len(want), events)
	}
	for index, event := range events {
		if event.Type != want[index] {
			t.Fatalf("event[%d] type = %q, want %q; events = %+v", index, event.Type, want[index], events)
		}
		if event.Sequence != uint64(index+1) {
			t.Fatalf("event[%d] sequence = %d, want %d", index, event.Sequence, index+1)
		}
		if event.EventID.IsZero() {
			t.Fatalf("event[%d] EventID is empty", index)
		}
	}
}

func wantSettlementWindow(start time.Time, end time.Time) string {
	return fmt.Sprintf("%d-%d", start.UTC().UnixMilli(), end.UTC().UnixMilli())
}

func mustOfflineSettlementKey(t *testing.T, planetID foundation.PlanetID, window string) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.OfflineSettlementIdempotencyKey(planetID, window)
	if err != nil {
		t.Fatalf("OfflineSettlementIdempotencyKey(%q, %q) error = %v, want nil", planetID, window, err)
	}
	return key
}

func mustRouteSettlementKey(t *testing.T, routeID foundation.RouteID, window string) foundation.IdempotencyKey {
	t.Helper()
	key, err := foundation.RouteSettlementIdempotencyKey(routeID, window)
	if err != nil {
		t.Fatalf("RouteSettlementIdempotencyKey(%q, %q) error = %v, want nil", routeID, window, err)
	}
	return key
}

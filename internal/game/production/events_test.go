package production

import (
	"encoding/json"
	"errors"
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
		PlanetID:       "planet-1",
		SettledAt:      testTime(1),
		ElapsedApplied: time.Hour,
		ProducedItems:  []SettlementItemDelta{{ItemID: "iron_ore", Quantity: 1}},
	}
	cases := map[string]PlanetProductionSettlementResult{
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
		RouteID:         route.RouteID,
		SettledAt:       testRouteNow().Add(time.Hour),
		ElapsedApplied:  time.Hour,
		AfterRoute:      route,
		WantedAmount:    40,
		TakenAmount:     40,
		DeliveredAmount: 40,
		AddedAmount:     40,
	}
	cases := map[string]RouteSettlementResult{
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

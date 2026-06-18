package production

import (
	"encoding/json"
	"errors"
	"testing"

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

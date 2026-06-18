package production

import (
	"encoding/json"
	"fmt"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

const (
	EventPlanetProductionInitialized = "planet.production_initialized"
	EventPlanetStorageUpdated        = "planet.storage_updated"
	EventPlanetBuildingUpdated       = "planet.building_updated"
	EventPlanetBuildingStateChanged  = "planet.building_state_changed"
	EventPlanetProductionSettled     = "planet.production_settled"
	EventPlanetStorageFull           = "planet.storage_full"
	EventPlanetEnergyInsufficient    = "planet.energy_insufficient"
	EventPlanetBuildingProduced      = "planet.building_produced"
	EventOfflineSettlementCompleted  = "offline.settlement_completed"
)

// EventType names production-domain events used by this package and later settlement slices.
type EventType string

// ProductionStateInitializedPayload describes initial production state creation.
type ProductionStateInitializedPayload struct {
	PlanetID              foundation.PlanetID `json:"planet_id"`
	LastCalculatedAt      time.Time           `json:"last_calculated_at"`
	StorageCapacityUnits  int64               `json:"storage_capacity_units"`
	EnergyCapacityPerHour int64               `json:"energy_capacity_per_hour"`
}

// StorageUpdatedPayload describes a storage aggregate update.
type StorageUpdatedPayload struct {
	PlanetID      foundation.PlanetID `json:"planet_id"`
	CapacityUnits int64               `json:"capacity_units"`
	UsedUnits     int64               `json:"used_units"`
	UpdatedAt     time.Time           `json:"updated_at"`
}

// BuildingUpdatedPayload describes a building row update.
type BuildingUpdatedPayload struct {
	PlanetID     foundation.PlanetID         `json:"planet_id"`
	BuildingID   BuildingID                  `json:"building_id"`
	Source       catalog.VersionedDefinition `json:"source"`
	BuildingType BuildingType                `json:"building_type"`
	Level        int                         `json:"level"`
	State        BuildingState               `json:"state"`
	UpdatedAt    time.Time                   `json:"updated_at"`
}

// String returns the stable event type representation.
func (eventType EventType) String() string {
	return string(eventType)
}

// Validate reports whether eventType is a supported production event name.
func (eventType EventType) Validate() error {
	switch eventType.String() {
	case EventPlanetProductionInitialized,
		EventPlanetStorageUpdated,
		EventPlanetBuildingUpdated,
		EventPlanetBuildingStateChanged,
		EventPlanetProductionSettled,
		EventPlanetStorageFull,
		EventPlanetEnergyInsufficient,
		EventPlanetBuildingProduced,
		EventOfflineSettlementCompleted:
		return nil
	default:
		return fmt.Errorf("event_type %q: %w", eventType, ErrInvalidProductionEvent)
	}
}

// NewProductionEvent validates and returns a standard game event envelope.
func NewProductionEvent(eventID foundation.EventID, eventType EventType, payload any, occurredAt time.Time, sequence uint64) (events.EventEnvelope, error) {
	if err := eventID.Validate(); err != nil {
		return events.EventEnvelope{}, err
	}
	if err := eventType.Validate(); err != nil {
		return events.EventEnvelope{}, err
	}
	if occurredAt.IsZero() {
		return events.EventEnvelope{}, fmt.Errorf("occurred_at: %w", ErrZeroProductionTimestamp)
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return events.EventEnvelope{}, fmt.Errorf("payload: %w", ErrInvalidProductionEvent)
	}
	return events.NewEventEnvelope(eventID, eventType.String(), rawPayload, occurredAt.UTC().UnixMilli(), sequence), nil
}

// NewProductionStateInitializedPayload validates and returns an initialization payload.
func NewProductionStateInitializedPayload(snapshot PlanetProductionSnapshot) (ProductionStateInitializedPayload, error) {
	if err := snapshot.Validate(); err != nil {
		return ProductionStateInitializedPayload{}, err
	}
	return ProductionStateInitializedPayload{
		PlanetID:              snapshot.State.PlanetID,
		LastCalculatedAt:      snapshot.State.LastCalculatedAt.UTC(),
		StorageCapacityUnits:  snapshot.Storage.CapacityUnits,
		EnergyCapacityPerHour: snapshot.State.EnergyCapacityPerHour,
	}, nil
}

// NewStorageUpdatedPayload validates and returns a storage update payload.
func NewStorageUpdatedPayload(storage PlanetStorage) (StorageUpdatedPayload, error) {
	if err := storage.Validate(); err != nil {
		return StorageUpdatedPayload{}, err
	}
	return StorageUpdatedPayload{
		PlanetID:      storage.PlanetID,
		CapacityUnits: storage.CapacityUnits,
		UsedUnits:     storage.UsedUnits(),
		UpdatedAt:     storage.UpdatedAt.UTC(),
	}, nil
}

// NewBuildingUpdatedPayload validates and returns a building update payload.
func NewBuildingUpdatedPayload(building PlanetBuilding) (BuildingUpdatedPayload, error) {
	if err := building.Validate(); err != nil {
		return BuildingUpdatedPayload{}, err
	}
	return BuildingUpdatedPayload{
		PlanetID:     building.PlanetID,
		BuildingID:   building.BuildingID,
		Source:       building.Source,
		BuildingType: building.BuildingType,
		Level:        building.Level,
		State:        building.State,
		UpdatedAt:    building.UpdatedAt.UTC(),
	}, nil
}

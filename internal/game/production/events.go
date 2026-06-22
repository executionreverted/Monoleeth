package production

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
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
	EventRouteTransferSettled        = "route.transfer_settled"
	EventRouteTransferLost           = "route.transfer_lost"
	EventRouteDestinationFull        = "route.destination_full"
	EventRouteSourceEmpty            = "route.source_empty"
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

// ProductionSettlementPayload is a compact outbox-safe summary of one
// server-timed production settlement.
type ProductionSettlementPayload struct {
	PlanetID           foundation.PlanetID         `json:"planet_id"`
	SettledAt          time.Time                   `json:"settled_at"`
	ReferenceKey       foundation.IdempotencyKey   `json:"reference_key,omitempty"`
	SettlementWindow   string                      `json:"settlement_window,omitempty"`
	ElapsedApplied     time.Duration               `json:"elapsed_applied"`
	ProducedItems      []SettlementItemDelta       `json:"produced_items,omitempty"`
	ConsumedInputs     []SettlementItemDelta       `json:"consumed_inputs,omitempty"`
	SkippedBuildings   []SettlementSkippedBuilding `json:"skipped_buildings,omitempty"`
	StorageFull        bool                        `json:"storage_full,omitempty"`
	EnergyInsufficient bool                        `json:"energy_insufficient,omitempty"`
	NoOp               bool                        `json:"no_op,omitempty"`
}

// BuildingProducedPayload summarizes one active building's settlement result.
type BuildingProducedPayload struct {
	PlanetID       foundation.PlanetID   `json:"planet_id"`
	BuildingID     BuildingID            `json:"building_id"`
	DefinitionID   catalog.DefinitionID  `json:"definition_id"`
	BuildingType   BuildingType          `json:"building_type"`
	Category       BuildingCategory      `json:"category"`
	Level          int                   `json:"level"`
	SettledAt      time.Time             `json:"settled_at"`
	ElapsedApplied time.Duration         `json:"elapsed_applied"`
	ProducedItems  []SettlementItemDelta `json:"produced_items,omitempty"`
	ConsumedInputs []SettlementItemDelta `json:"consumed_inputs,omitempty"`
	InputShortage  bool                  `json:"input_shortage,omitempty"`
	StorageFull    bool                  `json:"storage_full,omitempty"`
}

// RouteSettlementPayload is a compact outbox-safe summary of one virtual route
// settlement.
type RouteSettlementPayload struct {
	RouteID          foundation.RouteID        `json:"route_id"`
	ReferenceKey     foundation.IdempotencyKey `json:"reference_key,omitempty"`
	SettlementWindow string                    `json:"settlement_window,omitempty"`
	OwnerPlayerID    foundation.PlayerID       `json:"owner_player_id"`
	SourcePlanetID   foundation.PlanetID       `json:"source_planet_id"`
	Destination      RouteDestination          `json:"destination"`
	ResourceItemID   foundation.ItemID         `json:"resource_item_id"`
	SettledAt        time.Time                 `json:"settled_at"`
	ElapsedApplied   time.Duration             `json:"elapsed_applied"`
	WantedAmount     int64                     `json:"wanted_amount"`
	TakenAmount      int64                     `json:"taken_amount"`
	LostAmount       int64                     `json:"lost_amount"`
	DeliveredAmount  int64                     `json:"delivered_amount"`
	AddedAmount      int64                     `json:"added_amount"`
	LossPercent      float64                   `json:"loss_percent,omitempty"`
	SourceEmpty      bool                      `json:"source_empty,omitempty"`
	DestinationFull  bool                      `json:"destination_full,omitempty"`
	LossApplied      bool                      `json:"loss_applied,omitempty"`
	NoOp             bool                      `json:"no_op,omitempty"`
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
		EventOfflineSettlementCompleted,
		EventRouteTransferSettled,
		EventRouteTransferLost,
		EventRouteDestinationFull,
		EventRouteSourceEmpty:
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

// NewProductionSettlementPayload validates and returns a compact settlement payload.
func NewProductionSettlementPayload(result PlanetProductionSettlementResult) (ProductionSettlementPayload, error) {
	if err := result.PlanetID.Validate(); err != nil {
		return ProductionSettlementPayload{}, err
	}
	if result.SettledAt.IsZero() {
		return ProductionSettlementPayload{}, fmt.Errorf("settled_at: %w", ErrZeroProductionTimestamp)
	}
	if result.ElapsedApplied < 0 {
		return ProductionSettlementPayload{}, fmt.Errorf("elapsed_applied %s: %w", result.ElapsedApplied, ErrInvalidProductionEvent)
	}
	if err := validateProductionSettlementEvidence(result); err != nil {
		return ProductionSettlementPayload{}, err
	}
	if err := validateSettlementItemDeltas("produced_items", result.ProducedItems); err != nil {
		return ProductionSettlementPayload{}, err
	}
	if err := validateSettlementItemDeltas("consumed_inputs", result.ConsumedInputs); err != nil {
		return ProductionSettlementPayload{}, err
	}
	if err := validateSettlementSkippedBuildings(result.SkippedBuildings); err != nil {
		return ProductionSettlementPayload{}, err
	}
	return ProductionSettlementPayload{
		PlanetID:           result.PlanetID,
		SettledAt:          result.SettledAt.UTC(),
		ReferenceKey:       result.ReferenceKey,
		SettlementWindow:   result.SettlementWindow,
		ElapsedApplied:     result.ElapsedApplied,
		ProducedItems:      cloneSettlementItemDeltas(result.ProducedItems),
		ConsumedInputs:     cloneSettlementItemDeltas(result.ConsumedInputs),
		SkippedBuildings:   cloneSettlementSkippedBuildings(result.SkippedBuildings),
		StorageFull:        result.StorageFull,
		EnergyInsufficient: result.EnergyInsufficient,
		NoOp:               result.NoOp,
	}, nil
}

// NewBuildingProducedPayload validates and returns one building production payload.
func NewBuildingProducedPayload(planetID foundation.PlanetID, settledAt time.Time, result PlanetProductionBuildingResult) (BuildingProducedPayload, error) {
	if err := planetID.Validate(); err != nil {
		return BuildingProducedPayload{}, err
	}
	if err := result.BuildingID.Validate(); err != nil {
		return BuildingProducedPayload{}, err
	}
	if err := result.DefinitionID.Validate(); err != nil {
		return BuildingProducedPayload{}, err
	}
	if err := result.BuildingType.Validate(); err != nil {
		return BuildingProducedPayload{}, err
	}
	if err := result.Category.Validate(); err != nil {
		return BuildingProducedPayload{}, err
	}
	if settledAt.IsZero() {
		return BuildingProducedPayload{}, fmt.Errorf("settled_at: %w", ErrZeroProductionTimestamp)
	}
	if result.Level <= 0 {
		return BuildingProducedPayload{}, fmt.Errorf("building level %d: %w", result.Level, ErrInvalidProductionEvent)
	}
	if result.ElapsedApplied < 0 {
		return BuildingProducedPayload{}, fmt.Errorf("elapsed_applied %s: %w", result.ElapsedApplied, ErrInvalidProductionEvent)
	}
	if len(result.ProducedItems) == 0 && len(result.ConsumedInputs) == 0 {
		return BuildingProducedPayload{}, fmt.Errorf("building %q empty output/input deltas: %w", result.BuildingID, ErrInvalidProductionEvent)
	}
	if err := validateSettlementItemDeltas("produced_items", result.ProducedItems); err != nil {
		return BuildingProducedPayload{}, err
	}
	if err := validateSettlementItemDeltas("consumed_inputs", result.ConsumedInputs); err != nil {
		return BuildingProducedPayload{}, err
	}
	return BuildingProducedPayload{
		PlanetID:       planetID,
		BuildingID:     result.BuildingID,
		DefinitionID:   result.DefinitionID,
		BuildingType:   result.BuildingType,
		Category:       result.Category,
		Level:          result.Level,
		SettledAt:      settledAt.UTC(),
		ElapsedApplied: result.ElapsedApplied,
		ProducedItems:  cloneSettlementItemDeltas(result.ProducedItems),
		ConsumedInputs: cloneSettlementItemDeltas(result.ConsumedInputs),
		InputShortage:  result.InputShortage,
		StorageFull:    result.StorageFull,
	}, nil
}

// NewRouteSettlementPayload validates and returns a compact route settlement payload.
func NewRouteSettlementPayload(result RouteSettlementResult) (RouteSettlementPayload, error) {
	if err := result.RouteID.Validate(); err != nil {
		return RouteSettlementPayload{}, err
	}
	if err := result.AfterRoute.OwnerPlayerID.Validate(); err != nil {
		return RouteSettlementPayload{}, err
	}
	if err := result.AfterRoute.SourcePlanetID.Validate(); err != nil {
		return RouteSettlementPayload{}, err
	}
	if err := result.AfterRoute.Destination.Validate(); err != nil {
		return RouteSettlementPayload{}, err
	}
	if err := result.AfterRoute.ResourceItemID.Validate(); err != nil {
		return RouteSettlementPayload{}, err
	}
	if result.SettledAt.IsZero() {
		return RouteSettlementPayload{}, fmt.Errorf("settled_at: %w", ErrZeroProductionTimestamp)
	}
	if result.ElapsedApplied < 0 {
		return RouteSettlementPayload{}, fmt.Errorf("elapsed_applied %s: %w", result.ElapsedApplied, ErrInvalidProductionEvent)
	}
	if err := validateRouteSettlementEvidence(result); err != nil {
		return RouteSettlementPayload{}, err
	}
	if err := validateRouteSettlementAmounts(result); err != nil {
		return RouteSettlementPayload{}, err
	}
	return RouteSettlementPayload{
		RouteID:          result.RouteID,
		ReferenceKey:     result.ReferenceKey,
		SettlementWindow: result.SettlementWindow,
		OwnerPlayerID:    result.AfterRoute.OwnerPlayerID,
		SourcePlanetID:   result.AfterRoute.SourcePlanetID,
		Destination:      result.AfterRoute.Destination,
		ResourceItemID:   result.AfterRoute.ResourceItemID,
		SettledAt:        result.SettledAt.UTC(),
		ElapsedApplied:   result.ElapsedApplied,
		WantedAmount:     result.WantedAmount,
		TakenAmount:      result.TakenAmount,
		LostAmount:       result.LostAmount,
		DeliveredAmount:  result.DeliveredAmount,
		AddedAmount:      result.AddedAmount,
		LossPercent:      result.LossPercent,
		SourceEmpty:      result.SourceEmpty,
		DestinationFull:  result.DestinationFull,
		LossApplied:      result.LossApplied,
		NoOp:             result.NoOp,
	}, nil
}

func validateProductionSettlementEvidence(result PlanetProductionSettlementResult) error {
	if result.NoOp && result.ReferenceKey.IsZero() && result.SettlementWindow == "" {
		return nil
	}
	if err := validateSettlementWindow(result.SettlementWindow); err != nil {
		return err
	}
	want, err := foundation.OfflineSettlementIdempotencyKey(result.PlanetID, result.SettlementWindow)
	if err != nil {
		return fmt.Errorf("settlement evidence: %v: %w", err, ErrInvalidProductionEvent)
	}
	return validateSettlementReference(result.ReferenceKey, want)
}

func validateRouteSettlementEvidence(result RouteSettlementResult) error {
	if result.NoOp && result.ReferenceKey.IsZero() && result.SettlementWindow == "" {
		return nil
	}
	if err := validateSettlementWindow(result.SettlementWindow); err != nil {
		return err
	}
	want, err := foundation.RouteSettlementIdempotencyKey(result.RouteID, result.SettlementWindow)
	if err != nil {
		return fmt.Errorf("settlement evidence: %v: %w", err, ErrInvalidProductionEvent)
	}
	return validateSettlementReference(result.ReferenceKey, want)
}

func validateSettlementWindow(window string) error {
	if strings.TrimSpace(window) == "" || strings.Contains(window, ":") {
		return fmt.Errorf("settlement_window %q: %w", window, ErrInvalidProductionEvent)
	}
	return nil
}

func validateSettlementReference(got foundation.IdempotencyKey, want foundation.IdempotencyKey) error {
	if err := got.Validate(); err != nil {
		return fmt.Errorf("reference_key %q: %v: %w", got, err, ErrInvalidProductionEvent)
	}
	if got != want {
		return fmt.Errorf("reference_key %q does not match settlement window %q: %w", got, want, ErrInvalidProductionEvent)
	}
	return nil
}

func validateSettlementItemDeltas(name string, deltas []SettlementItemDelta) error {
	for _, delta := range deltas {
		if err := delta.ItemID.Validate(); err != nil {
			return fmt.Errorf("%s item_id %q: %w", name, delta.ItemID, err)
		}
		if delta.Quantity <= 0 || delta.Quantity > foundation.MaxAmount {
			return fmt.Errorf("%s quantity %d: %w", name, delta.Quantity, ErrInvalidProductionEvent)
		}
	}
	return nil
}

func validateSettlementSkippedBuildings(skipped []SettlementSkippedBuilding) error {
	for _, building := range skipped {
		if err := building.BuildingID.Validate(); err != nil {
			return fmt.Errorf("skipped building_id %q: %w", building.BuildingID, err)
		}
		if building.Reason != SettlementSkipReasonEnergyInsufficient {
			return fmt.Errorf("skipped reason %q: %w", building.Reason, ErrInvalidProductionEvent)
		}
		for name, value := range map[string]int64{
			"energy_used_per_hour":     building.EnergyUsedPerHour,
			"energy_cost_per_hour":     building.EnergyCostPerHour,
			"energy_capacity_per_hour": building.EnergyCapacityPerHour,
		} {
			if value < 0 || value > foundation.MaxAmount {
				return fmt.Errorf("%s %d: %w", name, value, ErrInvalidProductionEvent)
			}
		}
		if building.EnergyUsedPerHour+building.EnergyCostPerHour <= building.EnergyCapacityPerHour {
			return fmt.Errorf("energy skip used %d cost %d capacity %d: %w", building.EnergyUsedPerHour, building.EnergyCostPerHour, building.EnergyCapacityPerHour, ErrInvalidProductionEvent)
		}
	}
	return nil
}

func validateRouteSettlementAmounts(result RouteSettlementResult) error {
	amounts := map[string]int64{
		"wanted_amount":    result.WantedAmount,
		"taken_amount":     result.TakenAmount,
		"lost_amount":      result.LostAmount,
		"delivered_amount": result.DeliveredAmount,
		"added_amount":     result.AddedAmount,
	}
	for name, amount := range amounts {
		if amount < 0 || amount > foundation.MaxAmount {
			return fmt.Errorf("%s %d: %w", name, amount, ErrInvalidProductionEvent)
		}
	}
	if result.TakenAmount > result.WantedAmount {
		return fmt.Errorf("taken_amount %d exceeds wanted_amount %d: %w", result.TakenAmount, result.WantedAmount, ErrInvalidProductionEvent)
	}
	if result.LostAmount+result.DeliveredAmount != result.TakenAmount {
		return fmt.Errorf("lost_amount %d + delivered_amount %d != taken_amount %d: %w", result.LostAmount, result.DeliveredAmount, result.TakenAmount, ErrInvalidProductionEvent)
	}
	if result.AddedAmount > result.DeliveredAmount {
		return fmt.Errorf("added_amount %d exceeds delivered_amount %d: %w", result.AddedAmount, result.DeliveredAmount, ErrInvalidProductionEvent)
	}
	if result.DestinationFull != (result.DeliveredAmount > result.AddedAmount) {
		return fmt.Errorf("destination_full %v conflicts with delivered %d added %d: %w", result.DestinationFull, result.DeliveredAmount, result.AddedAmount, ErrInvalidProductionEvent)
	}
	if math.IsNaN(result.LossPercent) || math.IsInf(result.LossPercent, 0) || result.LossPercent < 0 || result.LossPercent > 1 {
		return fmt.Errorf("loss_percent %.4f: %w", result.LossPercent, ErrInvalidProductionEvent)
	}
	if !result.LossApplied && (result.LostAmount != 0 || result.LossPercent != 0) {
		return fmt.Errorf("loss_applied false with lost_amount %d loss_percent %.4f: %w", result.LostAmount, result.LossPercent, ErrInvalidProductionEvent)
	}
	if result.LossApplied && result.LossPercent > 0 && result.LostAmount == 0 {
		return fmt.Errorf("loss_applied true with loss_percent %.4f and lost_amount 0: %w", result.LossPercent, ErrInvalidProductionEvent)
	}
	return nil
}

func cloneSettlementItemDeltas(deltas []SettlementItemDelta) []SettlementItemDelta {
	if len(deltas) == 0 {
		return nil
	}
	cloned := make([]SettlementItemDelta, len(deltas))
	copy(cloned, deltas)
	return cloned
}

func cloneSettlementSkippedBuildings(skipped []SettlementSkippedBuilding) []SettlementSkippedBuilding {
	if len(skipped) == 0 {
		return nil
	}
	cloned := make([]SettlementSkippedBuilding, len(skipped))
	copy(cloned, skipped)
	return cloned
}

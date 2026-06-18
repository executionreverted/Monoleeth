package production

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	"gameproject/internal/game/foundation"
)

const (
	// MaxRouteLossChance is the Phase 09 MVP cap for partial route-loss rolls.
	MaxRouteLossChance = 0.40
	maxRouteLossPct    = 1.0
)

// RouteDestinationType identifies the storage domain a route delivers into.
type RouteDestinationType string

const (
	RouteDestinationTypePlanet  RouteDestinationType = "planet"
	RouteDestinationTypeStorage RouteDestinationType = "storage"
	RouteDestinationTypeStation RouteDestinationType = "station"
)

// RouteDestinationID identifies a non-planet route destination such as a
// global storage endpoint or station. Planet destinations should still pass a
// planet-shaped id through RouteDestination for a uniform durable route row.
type RouteDestinationID string

// RouteDestination records the server-validated destination selected by intent.
type RouteDestination struct {
	Type RouteDestinationType `json:"type"`
	ID   RouteDestinationID   `json:"id"`
}

// RouteRisk records the partial-loss model calculated from server-owned facts.
type RouteRisk struct {
	LossChance     float64 `json:"loss_chance"`
	MinLossPercent float64 `json:"min_loss_percent"`
	MaxLossPercent float64 `json:"max_loss_percent"`
}

// AutomationRoute is the durable row for an MVP virtual automation route.
type AutomationRoute struct {
	RouteID           foundation.RouteID  `json:"route_id"`
	OwnerPlayerID     foundation.PlayerID `json:"owner_player_id"`
	SourcePlanetID    foundation.PlanetID `json:"source_planet_id"`
	Destination       RouteDestination    `json:"destination"`
	ResourceItemID    foundation.ItemID   `json:"resource_item_id"`
	AmountPerHour     int64               `json:"amount_per_hour"`
	EnergyCostPerHour int64               `json:"energy_cost_per_hour"`
	Risk              RouteRisk           `json:"route_risk"`
	Enabled           bool                `json:"enabled"`
	LastCalculatedAt  time.Time           `json:"last_calculated_at"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

// CreateRouteInput carries player intent only. Authorization, cost, distance,
// and risk are supplied by RouteCreatePolicyProvider.
type CreateRouteInput struct {
	RouteID        foundation.RouteID  `json:"route_id"`
	OwnerPlayerID  foundation.PlayerID `json:"owner_player_id"`
	SourcePlanetID foundation.PlanetID `json:"source_planet_id"`
	Destination    RouteDestination    `json:"destination"`
	ResourceItemID foundation.ItemID   `json:"resource_item_id"`
	AmountPerHour  int64               `json:"amount_per_hour"`
}

// UpdateRouteInput carries new route terms for an existing automation route.
// The source planet and owner identity are loaded from the durable route row.
type UpdateRouteInput struct {
	RouteID        foundation.RouteID  `json:"route_id"`
	OwnerPlayerID  foundation.PlayerID `json:"owner_player_id"`
	Destination    RouteDestination    `json:"destination"`
	ResourceItemID foundation.ItemID   `json:"resource_item_id"`
	AmountPerHour  int64               `json:"amount_per_hour"`
}

// CreateRouteResult returns the detached route row created by the server.
type CreateRouteResult struct {
	Route   AutomationRoute `json:"route"`
	Created bool            `json:"created"`
}

// RouteControlResult reports the route row after an enable/disable command.
type RouteControlResult struct {
	Route      AutomationRoute       `json:"route"`
	Settlement RouteSettlementResult `json:"settlement"`
	Changed    bool                  `json:"changed"`
}

// UpdateRouteResult reports the route row after replacing route terms.
type UpdateRouteResult struct {
	Route      AutomationRoute       `json:"route"`
	Settlement RouteSettlementResult `json:"settlement"`
	Updated    bool                  `json:"updated"`
}

// RouteCreatePolicyInput asks the policy boundary for server-owned route facts.
type RouteCreatePolicyInput struct {
	OwnerPlayerID  foundation.PlayerID
	SourcePlanetID foundation.PlanetID
	Destination    RouteDestination
	ResourceItemID foundation.ItemID
	AmountPerHour  int64
}

// RouteCreatePolicyProvider supplies facts owned by world/progression/catalog
// domains without coupling production routes to their internals.
type RouteCreatePolicyProvider interface {
	RouteCreatePolicy(input RouteCreatePolicyInput) (RouteCreatePolicy, error)
}

// RouteCreatePolicy is the server-owned fact bundle used to validate and build
// an automation route.
type RouteCreatePolicy struct {
	SourcePlanetOwned     bool
	DestinationAccessible bool
	ResourceRouteable     bool
	RequirementsMet       bool

	DistanceUnits    float64
	MaxDistanceUnits float64

	BaseLossChance            float64
	DistanceLossChancePerUnit float64
	SourceRegionRisk          float64
	DestinationRegionRisk     float64
	DeepSpaceRisk             float64
	PlayerLossReduction       float64
	RouteSecurityReduction    float64
	MinLossPercent            float64
	MaxLossPercent            float64

	EnergyCostPerHour int64
}

// NewPlanetRouteDestination returns a planet destination from a foundation id.
func NewPlanetRouteDestination(planetID foundation.PlanetID) (RouteDestination, error) {
	if err := planetID.Validate(); err != nil {
		return RouteDestination{}, err
	}
	return RouteDestination{Type: RouteDestinationTypePlanet, ID: RouteDestinationID(planetID.String())}, nil
}

// String returns the stable destination type representation.
func (destinationType RouteDestinationType) String() string { return string(destinationType) }

// String returns the stable destination id representation.
func (id RouteDestinationID) String() string { return string(id) }

// Validate reports whether destinationType is supported by the MVP route model.
func (destinationType RouteDestinationType) Validate() error {
	switch destinationType {
	case RouteDestinationTypePlanet, RouteDestinationTypeStorage, RouteDestinationTypeStation:
		return nil
	default:
		return fmt.Errorf("route destination type %q: %w", destinationType, ErrInvalidRouteDestinationType)
	}
}

// Validate reports whether id is non-blank and safe for durable route storage.
func (id RouteDestinationID) Validate() error {
	value := string(id)
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("route destination id: %w", ErrInvalidRouteDestinationID)
	}
	if value != strings.TrimSpace(value) || strings.Contains(value, ":") || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("route destination id %q: %w", value, ErrInvalidRouteDestinationID)
	}
	return nil
}

// Validate reports whether destination is structurally usable. Accessibility is
// still checked through RouteCreatePolicyProvider.
func (destination RouteDestination) Validate() error {
	if err := destination.Type.Validate(); err != nil {
		return err
	}
	if destination.Type == RouteDestinationTypePlanet {
		if err := foundation.PlanetID(destination.ID).Validate(); err != nil {
			return fmt.Errorf("route destination planet %q: %w", destination.ID, err)
		}
		return nil
	}
	return destination.ID.Validate()
}

// NewRouteRisk clamps chance/range values to MVP bounds and validates the
// resulting risk model.
func NewRouteRisk(lossChance, minLossPercent, maxLossPercent float64) (RouteRisk, error) {
	if err := validateFiniteRouteFloat("loss chance", lossChance, ErrInvalidRouteRisk); err != nil {
		return RouteRisk{}, err
	}
	if err := validateFiniteRouteFloat("min loss percent", minLossPercent, ErrInvalidRouteRisk); err != nil {
		return RouteRisk{}, err
	}
	if err := validateFiniteRouteFloat("max loss percent", maxLossPercent, ErrInvalidRouteRisk); err != nil {
		return RouteRisk{}, err
	}
	risk := RouteRisk{
		LossChance:     clampFloat64(lossChance, 0, MaxRouteLossChance),
		MinLossPercent: clampFloat64(minLossPercent, 0, maxRouteLossPct),
		MaxLossPercent: clampFloat64(maxLossPercent, 0, maxRouteLossPct),
	}
	if err := risk.Validate(); err != nil {
		return RouteRisk{}, err
	}
	return risk, nil
}

// Validate reports whether risk is normalized to MVP-safe bounds.
func (risk RouteRisk) Validate() error {
	if err := validateFiniteRouteFloat("loss chance", risk.LossChance, ErrInvalidRouteRisk); err != nil {
		return err
	}
	if risk.LossChance < 0 || risk.LossChance > MaxRouteLossChance {
		return fmt.Errorf("loss chance %.4f: %w", risk.LossChance, ErrInvalidRouteRisk)
	}
	if err := validateFiniteRouteFloat("min loss percent", risk.MinLossPercent, ErrInvalidRouteRisk); err != nil {
		return err
	}
	if err := validateFiniteRouteFloat("max loss percent", risk.MaxLossPercent, ErrInvalidRouteRisk); err != nil {
		return err
	}
	if risk.MinLossPercent < 0 || risk.MinLossPercent > maxRouteLossPct {
		return fmt.Errorf("min loss percent %.4f: %w", risk.MinLossPercent, ErrInvalidRouteRisk)
	}
	if risk.MaxLossPercent < 0 || risk.MaxLossPercent > maxRouteLossPct {
		return fmt.Errorf("max loss percent %.4f: %w", risk.MaxLossPercent, ErrInvalidRouteRisk)
	}
	if risk.MinLossPercent > risk.MaxLossPercent {
		return fmt.Errorf("loss range %.4f..%.4f: %w", risk.MinLossPercent, risk.MaxLossPercent, ErrInvalidRouteRisk)
	}
	return nil
}

// Validate reports whether route input is structurally valid intent.
func (input CreateRouteInput) Validate() error {
	if err := input.RouteID.Validate(); err != nil {
		return err
	}
	if err := input.OwnerPlayerID.Validate(); err != nil {
		return err
	}
	if err := input.SourcePlanetID.Validate(); err != nil {
		return err
	}
	if err := input.Destination.Validate(); err != nil {
		return err
	}
	if err := validateSupportedRouteSettlementDestination(input.Destination); err != nil {
		return err
	}
	if err := input.ResourceItemID.Validate(); err != nil {
		return err
	}
	if err := validatePositiveBoundedAmount("route amount per hour", input.AmountPerHour, ErrInvalidRouteRate); err != nil {
		return err
	}
	return nil
}

// Validate reports whether route update input is structurally valid intent.
func (input UpdateRouteInput) Validate() error {
	if err := input.RouteID.Validate(); err != nil {
		return err
	}
	if err := input.OwnerPlayerID.Validate(); err != nil {
		return err
	}
	if err := input.Destination.Validate(); err != nil {
		return err
	}
	if err := validateSupportedRouteSettlementDestination(input.Destination); err != nil {
		return err
	}
	if err := input.ResourceItemID.Validate(); err != nil {
		return err
	}
	if err := validatePositiveBoundedAmount("route amount per hour", input.AmountPerHour, ErrInvalidRouteRate); err != nil {
		return err
	}
	return nil
}

func (input UpdateRouteInput) policyInput(sourcePlanetID foundation.PlanetID) RouteCreatePolicyInput {
	return RouteCreatePolicyInput{
		OwnerPlayerID:  input.OwnerPlayerID,
		SourcePlanetID: sourcePlanetID,
		Destination:    input.Destination,
		ResourceItemID: input.ResourceItemID,
		AmountPerHour:  input.AmountPerHour,
	}
}

func validateSupportedRouteSettlementDestination(destination RouteDestination) error {
	if destination.Type != RouteDestinationTypePlanet {
		return fmt.Errorf("route destination %q: %w", destination.Type, ErrUnsupportedRouteDestination)
	}
	return nil
}

// Validate reports whether policy input can be sent to a provider.
func (input RouteCreatePolicyInput) Validate() error {
	return (CreateRouteInput{
		RouteID:        "route-policy-validation",
		OwnerPlayerID:  input.OwnerPlayerID,
		SourcePlanetID: input.SourcePlanetID,
		Destination:    input.Destination,
		ResourceItemID: input.ResourceItemID,
		AmountPerHour:  input.AmountPerHour,
	}).Validate()
}

// Validate reports whether policy permits route creation and contains usable
// server-owned cost, distance, and risk facts.
func (policy RouteCreatePolicy) Validate() error {
	switch {
	case !policy.SourcePlanetOwned:
		return ErrRouteSourceNotOwned
	case !policy.DestinationAccessible:
		return ErrRouteDestinationNotAccessible
	case !policy.ResourceRouteable:
		return ErrRouteResourceNotRouteable
	case !policy.RequirementsMet:
		return ErrRouteRequirementNotMet
	}
	if err := validateRouteDistance(policy.DistanceUnits, policy.MaxDistanceUnits); err != nil {
		return err
	}
	if err := validateNonNegativeBoundedAmount("route energy cost per hour", policy.EnergyCostPerHour, ErrInvalidRouteEnergyCost); err != nil {
		return err
	}
	if err := validateNonNegativeRiskComponent("base loss chance", policy.BaseLossChance); err != nil {
		return err
	}
	if err := validateNonNegativeRiskComponent("distance loss chance per unit", policy.DistanceLossChancePerUnit); err != nil {
		return err
	}
	if err := validateNonNegativeRiskComponent("source region risk", policy.SourceRegionRisk); err != nil {
		return err
	}
	if err := validateNonNegativeRiskComponent("destination region risk", policy.DestinationRegionRisk); err != nil {
		return err
	}
	if err := validateNonNegativeRiskComponent("deep space risk", policy.DeepSpaceRisk); err != nil {
		return err
	}
	if err := validateNonNegativeRiskComponent("player loss reduction", policy.PlayerLossReduction); err != nil {
		return err
	}
	if err := validateNonNegativeRiskComponent("route security reduction", policy.RouteSecurityReduction); err != nil {
		return err
	}
	if err := validateFiniteRouteFloat("min loss percent", policy.MinLossPercent, ErrInvalidRouteRisk); err != nil {
		return err
	}
	if err := validateFiniteRouteFloat("max loss percent", policy.MaxLossPercent, ErrInvalidRouteRisk); err != nil {
		return err
	}
	return nil
}

// CalculateRisk calculates and clamps route risk from server-owned policy facts.
func (policy RouteCreatePolicy) CalculateRisk() (RouteRisk, error) {
	if err := policy.Validate(); err != nil {
		return RouteRisk{}, err
	}
	lossChance := policy.BaseLossChance +
		(policy.DistanceUnits * policy.DistanceLossChancePerUnit) +
		policy.SourceRegionRisk +
		policy.DestinationRegionRisk +
		policy.DeepSpaceRisk -
		policy.PlayerLossReduction -
		policy.RouteSecurityReduction
	return NewRouteRisk(lossChance, policy.MinLossPercent, policy.MaxLossPercent)
}

// Validate reports whether route is a complete durable row.
func (route AutomationRoute) Validate() error {
	if err := route.RouteID.Validate(); err != nil {
		return err
	}
	if err := route.OwnerPlayerID.Validate(); err != nil {
		return err
	}
	if err := route.SourcePlanetID.Validate(); err != nil {
		return err
	}
	if err := route.Destination.Validate(); err != nil {
		return err
	}
	if err := route.ResourceItemID.Validate(); err != nil {
		return err
	}
	if err := validatePositiveBoundedAmount("route amount per hour", route.AmountPerHour, ErrInvalidRouteRate); err != nil {
		return err
	}
	if err := validateNonNegativeBoundedAmount("route energy cost per hour", route.EnergyCostPerHour, ErrInvalidRouteEnergyCost); err != nil {
		return err
	}
	if err := route.Risk.Validate(); err != nil {
		return err
	}
	if route.LastCalculatedAt.IsZero() {
		return fmt.Errorf("last_calculated_at: %w", ErrZeroProductionTimestamp)
	}
	if route.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrZeroProductionTimestamp)
	}
	if route.UpdatedAt.IsZero() {
		return fmt.Errorf("updated_at: %w", ErrZeroProductionTimestamp)
	}
	if route.LastCalculatedAt.Before(route.CreatedAt) {
		return fmt.Errorf("last_calculated_at before created_at: %w", ErrInvalidRouteCreateConfig)
	}
	if route.UpdatedAt.Before(route.CreatedAt) {
		return fmt.Errorf("updated_at before created_at: %w", ErrInvalidRouteCreateConfig)
	}
	return nil
}

// Clone returns a detached route copy.
func (route AutomationRoute) Clone() AutomationRoute {
	return cloneAutomationRoute(route)
}

func newAutomationRoute(input CreateRouteInput, policy RouteCreatePolicy, now time.Time) (AutomationRoute, error) {
	if err := input.Validate(); err != nil {
		return AutomationRoute{}, err
	}
	if now.IsZero() {
		return AutomationRoute{}, fmt.Errorf("now: %w", ErrZeroProductionTimestamp)
	}
	risk, err := policy.CalculateRisk()
	if err != nil {
		return AutomationRoute{}, err
	}
	now = now.UTC()
	route := AutomationRoute{
		RouteID:           input.RouteID,
		OwnerPlayerID:     input.OwnerPlayerID,
		SourcePlanetID:    input.SourcePlanetID,
		Destination:       input.Destination,
		ResourceItemID:    input.ResourceItemID,
		AmountPerHour:     input.AmountPerHour,
		EnergyCostPerHour: policy.EnergyCostPerHour,
		Risk:              risk,
		Enabled:           true,
		LastCalculatedAt:  now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := route.Validate(); err != nil {
		return AutomationRoute{}, err
	}
	return cloneAutomationRoute(route), nil
}

func cloneAutomationRoute(route AutomationRoute) AutomationRoute {
	route.LastCalculatedAt = route.LastCalculatedAt.UTC()
	route.CreatedAt = route.CreatedAt.UTC()
	route.UpdatedAt = route.UpdatedAt.UTC()
	return route
}

func validateRouteDistance(distanceUnits, maxDistanceUnits float64) error {
	if err := validateFiniteRouteFloat("route distance", distanceUnits, ErrInvalidRouteDistance); err != nil {
		return err
	}
	if err := validateFiniteRouteFloat("max route distance", maxDistanceUnits, ErrInvalidRouteDistance); err != nil {
		return err
	}
	if distanceUnits < 0 {
		return fmt.Errorf("route distance %.4f: %w", distanceUnits, ErrInvalidRouteDistance)
	}
	if maxDistanceUnits <= 0 {
		return fmt.Errorf("max route distance %.4f: %w", maxDistanceUnits, ErrInvalidRouteDistance)
	}
	if distanceUnits > maxDistanceUnits {
		return fmt.Errorf("route distance %.4f max %.4f: %w", distanceUnits, maxDistanceUnits, ErrRouteDistanceTooFar)
	}
	return nil
}

func validateNonNegativeRiskComponent(name string, value float64) error {
	if err := validateFiniteRouteFloat(name, value, ErrInvalidRouteRisk); err != nil {
		return err
	}
	if value < 0 {
		return fmt.Errorf("%s %.4f: %w", name, value, ErrInvalidRouteRisk)
	}
	return nil
}

func validateFiniteRouteFloat(name string, value float64, sentinel error) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("%s %.4f: %w", name, value, sentinel)
	}
	return nil
}

func clampFloat64(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

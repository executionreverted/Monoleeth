package crafting

import (
	"sync"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

const (
	lowTierCraftXPMaxRank     = 1
	lowTierCraftXPMaxDuration = 30 * time.Minute
)

// CraftXPObservation records one successful, non-duplicate craft XP grant with
// enough recipe metadata for later low-tier spam balancing.
type CraftXPObservation struct {
	PlayerID           foundation.PlayerID         `json:"player_id"`
	JobID              CraftJobID                  `json:"job_id"`
	RecipeID           catalog.DefinitionID        `json:"recipe_id"`
	RecipeSource       catalog.VersionedDefinition `json:"recipe_source"`
	Category           RecipeCategory              `json:"category"`
	OutputKind         RecipeOutputKind            `json:"output_kind"`
	OutputItemID       foundation.ItemID           `json:"output_item_id,omitempty"`
	OutputShipID       foundation.ShipID           `json:"output_ship_id,omitempty"`
	OutputQuantity     int64                       `json:"output_quantity"`
	LocationType       CraftLocationType           `json:"location_type"`
	RequiredRank       int                         `json:"required_rank"`
	RequiredCredits    int64                       `json:"required_credits"`
	CraftDuration      time.Duration               `json:"craft_duration"`
	InputCount         int                         `json:"input_count"`
	InputQuantityTotal int64                       `json:"input_quantity_total"`
	MainXP             int64                       `json:"main_xp"`
	RoleXP             []progression.RoleXPGrant   `json:"role_xp,omitempty"`
	LowTier            bool                        `json:"low_tier"`
	XPSourceID         progression.XPSourceID      `json:"xp_source_id"`
	ReferenceKey       foundation.IdempotencyKey   `json:"reference_id"`
	GrantedAt          time.Time                   `json:"granted_at"`
}

// CraftXPTracker is an observability/balancing hook. Implementations must not
// mutate gameplay truth; crafting treats tracking as best-effort telemetry.
type CraftXPTracker interface {
	TrackCraftXP(observation CraftXPObservation)
}

// InMemoryCraftXPTracker stores observations for tests and local balancing
// analysis until a durable metrics/event pipeline owns this data.
type InMemoryCraftXPTracker struct {
	mu           sync.Mutex
	observations []CraftXPObservation
}

// NewInMemoryCraftXPTracker returns a concurrency-safe local craft XP tracker.
func NewInMemoryCraftXPTracker() *InMemoryCraftXPTracker {
	return &InMemoryCraftXPTracker{}
}

// TrackCraftXP records a defensive copy of observation.
func (tracker *InMemoryCraftXPTracker) TrackCraftXP(observation CraftXPObservation) {
	if tracker == nil {
		return
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	tracker.observations = append(tracker.observations, cloneCraftXPObservation(observation))
}

// Observations returns observations in insertion order.
func (tracker *InMemoryCraftXPTracker) Observations() []CraftXPObservation {
	if tracker == nil {
		return nil
	}
	tracker.mu.Lock()
	defer tracker.mu.Unlock()

	observations := make([]CraftXPObservation, 0, len(tracker.observations))
	for _, observation := range tracker.observations {
		observations = append(observations, cloneCraftXPObservation(observation))
	}
	return observations
}

func isLowTierCraftXPRecipe(recipe RecipeDefinition) bool {
	return recipe.RequiredRank <= lowTierCraftXPMaxRank && recipe.CraftDuration <= lowTierCraftXPMaxDuration
}

func recipeInputQuantityTotal(recipe RecipeDefinition) int64 {
	var total int64
	for _, input := range recipe.Inputs {
		total += input.Quantity.Int64()
	}
	return total
}

func cloneCraftXPObservation(observation CraftXPObservation) CraftXPObservation {
	observation.RoleXP = append([]progression.RoleXPGrant(nil), observation.RoleXP...)
	return observation
}

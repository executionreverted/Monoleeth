package crafting

import (
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

// RecipeCategory groups recipes for catalog browsing and balance review.
type RecipeCategory string

const (
	RecipeCategoryProcessedMaterial RecipeCategory = "processed_material"
	RecipeCategoryModule            RecipeCategory = "module"
	RecipeCategoryShipUnlock        RecipeCategory = "ship_unlock"
)

// RecipeOutputKind identifies how a completed craft output is applied.
type RecipeOutputKind string

const (
	RecipeOutputKindItem       RecipeOutputKind = "item"
	RecipeOutputKindShipUnlock RecipeOutputKind = "ship_unlock"
)

// CraftLocationType identifies the server-owned location family for a craft.
type CraftLocationType string

const (
	CraftLocationStation             CraftLocationType = "station"
	CraftLocationOwnedPlanet         CraftLocationType = "owned_planet"
	CraftLocationPlanetBuilding      CraftLocationType = "planet_building"
	CraftLocationSpecialEventStation CraftLocationType = "special_event_station"
)

// CraftLocation is the validated location metadata stored on a craft job.
type CraftLocation struct {
	Type       CraftLocationType   `json:"location_type"`
	ID         string              `json:"location_id"`
	PlanetID   foundation.PlanetID `json:"planet_id,omitempty"`
	PlanetType string              `json:"planet_type,omitempty"`
}

// RecipeInput records one item stack consumed or reserved by a recipe.
type RecipeInput struct {
	ItemID   foundation.ItemID   `json:"item_id"`
	Quantity foundation.Quantity `json:"quantity"`
}

// RecipeOutput records the item or ship unlock produced by a recipe.
type RecipeOutput struct {
	Kind      RecipeOutputKind    `json:"kind"`
	ItemID    foundation.ItemID   `json:"item_id,omitempty"`
	ShipID    foundation.ShipID   `json:"ship_id,omitempty"`
	Quantity  foundation.Quantity `json:"quantity"`
	Tradeable bool                `json:"tradeable"`
}

// RoleRequirement records the role level gate for a recipe.
type RoleRequirement struct {
	Role  progression.RoleType `json:"role"`
	Level int                  `json:"level"`
}

// RecipeDefinition records one static crafting catalog row.
type RecipeDefinition struct {
	Source               catalog.VersionedDefinition `json:"source"`
	RecipeID             catalog.DefinitionID        `json:"recipe_id"`
	Category             RecipeCategory              `json:"category"`
	Output               RecipeOutput                `json:"output"`
	Inputs               []RecipeInput               `json:"inputs"`
	RequiredCredits      foundation.Money            `json:"required_credits"`
	RequiredRank         int                         `json:"required_rank"`
	RequiredRoleLevels   []RoleRequirement           `json:"required_role_levels,omitempty"`
	RequiredLocationType CraftLocationType           `json:"required_location_type"`
	CraftDuration        time.Duration               `json:"craft_duration"`
	Repeatable           bool                        `json:"repeatable"`
}

// CraftJobID identifies durable craft job state.
type CraftJobID string

// CraftJobState records the lifecycle state for a craft job.
type CraftJobState string

const (
	CraftJobStateRunning   CraftJobState = "running"
	CraftJobStateCompleted CraftJobState = "completed"
)

// CraftJob records durable state for a recipe that has been started.
type CraftJob struct {
	JobID                  CraftJobID                  `json:"job_id"`
	PlayerID               foundation.PlayerID         `json:"player_id"`
	RecipeSource           catalog.VersionedDefinition `json:"recipe_source"`
	ReservationID          economy.ReservationID       `json:"reservation_id"`
	Location               CraftLocation               `json:"location"`
	State                  CraftJobState               `json:"state"`
	StartedAt              time.Time                   `json:"started_at"`
	CompletesAt            time.Time                   `json:"completes_at"`
	ReservationCommittedAt *time.Time                  `json:"reservation_committed_at,omitempty"`
	OutputGrantedAt        *time.Time                  `json:"output_granted_at,omitempty"`
	XPGrantedAt            *time.Time                  `json:"xp_granted_at,omitempty"`
	CompletedAt            *time.Time                  `json:"completed_at,omitempty"`
}

// JobCompletedEvent is the internal post-commit craft completion payload
// consumed by quest, progression audit, and balancing processors.
type JobCompletedEvent struct {
	JobID       CraftJobID           `json:"job_id"`
	PlayerID    foundation.PlayerID  `json:"player_id"`
	RecipeID    catalog.DefinitionID `json:"recipe_id"`
	OutputKind  RecipeOutputKind     `json:"output_kind"`
	ItemID      foundation.ItemID    `json:"item_id,omitempty"`
	ShipID      foundation.ShipID    `json:"ship_id,omitempty"`
	Quantity    int64                `json:"quantity"`
	CompletedAt time.Time            `json:"completed_at"`
}

// String returns the stable category representation.
func (category RecipeCategory) String() string { return string(category) }

// String returns the stable output kind representation.
func (kind RecipeOutputKind) String() string { return string(kind) }

// String returns the stable location type representation.
func (locationType CraftLocationType) String() string { return string(locationType) }

// String returns the stable craft job id representation.
func (id CraftJobID) String() string { return string(id) }

// String returns the stable craft job state representation.
func (state CraftJobState) String() string { return string(state) }

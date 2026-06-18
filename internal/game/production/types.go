package production

import (
	"fmt"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
)

// BuildingCategory groups production buildings for validation and balance.
type BuildingCategory string

const (
	BuildingCategoryExtractor BuildingCategory = "extractor"
	BuildingCategoryRefinery  BuildingCategory = "refinery"
)

// BuildingType identifies a production-capable planet building type.
type BuildingType string

const (
	BuildingTypeIronExtractor BuildingType = "iron_extractor"
	BuildingTypeAlloyFoundry  BuildingType = "alloy_foundry"
)

// BuildingState identifies the production-relevant runtime state for a
// building. Active and disabled are the only catalog-compatible MVP states.
type BuildingState string

const (
	BuildingStateActive   BuildingState = "active"
	BuildingStateDisabled BuildingState = "disabled"
)

// ItemRate records one item consumed or produced per server hour.
type ItemRate struct {
	ItemID        foundation.ItemID `json:"item_id"`
	AmountPerHour int64             `json:"amount_per_hour"`
}

// BuildingProductionDefinition records one static production catalog row.
type BuildingProductionDefinition struct {
	Source            catalog.VersionedDefinition `json:"source"`
	DefinitionID      catalog.DefinitionID        `json:"definition_id"`
	BuildingType      BuildingType                `json:"building_type"`
	Category          BuildingCategory            `json:"category"`
	Level             int                         `json:"level"`
	Inputs            []ItemRate                  `json:"inputs,omitempty"`
	Outputs           []ItemRate                  `json:"outputs"`
	EnergyCostPerHour int64                       `json:"energy_cost_per_hour"`
}

// String returns the stable category representation.
func (category BuildingCategory) String() string { return string(category) }

// String returns the stable building type representation.
func (buildingType BuildingType) String() string { return string(buildingType) }

// String returns the stable building state representation.
func (state BuildingState) String() string { return string(state) }

// Validate reports whether category is supported by the MVP catalog.
func (category BuildingCategory) Validate() error {
	switch category {
	case BuildingCategoryExtractor, BuildingCategoryRefinery:
		return nil
	default:
		return fmt.Errorf("building category %q: %w", category, ErrInvalidBuildingCategory)
	}
}

// Validate reports whether buildingType is supported by the MVP catalog.
func (buildingType BuildingType) Validate() error {
	switch buildingType {
	case BuildingTypeIronExtractor, BuildingTypeAlloyFoundry:
		return nil
	default:
		return fmt.Errorf("building type %q: %w", buildingType, ErrInvalidBuildingType)
	}
}

// Validate reports whether state is compatible with production catalog checks.
func (state BuildingState) Validate() error {
	switch state {
	case BuildingStateActive, BuildingStateDisabled:
		return nil
	default:
		return fmt.Errorf("building state %q: %w", state, ErrInvalidBuildingState)
	}
}

// Produces reports whether a catalog-compatible building state should produce
// output. Disabled is valid state, but it never produces.
func (state BuildingState) Produces() (bool, error) {
	if err := state.Validate(); err != nil {
		return false, err
	}
	return state == BuildingStateActive, nil
}

// Validate reports whether rate names an item and a positive whole-unit rate.
func (rate ItemRate) Validate() error {
	if err := rate.ItemID.Validate(); err != nil {
		return err
	}
	if rate.AmountPerHour <= 0 || rate.AmountPerHour > foundation.MaxAmount {
		return fmt.Errorf("rate per hour %d: %w", rate.AmountPerHour, ErrInvalidProductionRate)
	}
	return nil
}

// Validate reports whether definition is a complete static production row.
func (definition BuildingProductionDefinition) Validate() error {
	if err := definition.Source.Validate(); err != nil {
		return err
	}
	if err := definition.DefinitionID.Validate(); err != nil {
		return err
	}
	if definition.Source.DefinitionID != definition.DefinitionID {
		return fmt.Errorf("source %q production %q: %w", definition.Source.DefinitionID, definition.DefinitionID, ErrProductionSourceMismatch)
	}
	if err := definition.BuildingType.Validate(); err != nil {
		return err
	}
	if err := definition.Category.Validate(); err != nil {
		return err
	}
	if definition.Level <= 0 {
		return fmt.Errorf("building level %d: %w", definition.Level, ErrInvalidBuildingLevel)
	}
	if err := validateItemRates("input", definition.Inputs, false); err != nil {
		return err
	}
	if err := validateItemRates("output", definition.Outputs, true); err != nil {
		return err
	}
	if err := validateCategoryIO(definition.Category, definition.Inputs); err != nil {
		return err
	}
	if definition.EnergyCostPerHour <= 0 || definition.EnergyCostPerHour > foundation.MaxAmount {
		return fmt.Errorf("energy cost per hour %d: %w", definition.EnergyCostPerHour, ErrInvalidEnergyCost)
	}
	return nil
}

// ValidateStateCompatibility checks that state can safely be used with this
// definition by later production settlement code.
func (definition BuildingProductionDefinition) ValidateStateCompatibility(state BuildingState) error {
	if err := definition.Validate(); err != nil {
		return err
	}
	return state.Validate()
}

// ProducesInState reports whether this definition should produce in state.
func (definition BuildingProductionDefinition) ProducesInState(state BuildingState) (bool, error) {
	if err := definition.ValidateStateCompatibility(state); err != nil {
		return false, err
	}
	return state.Produces()
}

type buildingLevelKey struct {
	buildingType BuildingType
	level        int
}

func validateCategoryIO(category BuildingCategory, inputs []ItemRate) error {
	switch category {
	case BuildingCategoryExtractor:
		if len(inputs) > 0 {
			return ErrUnexpectedExtractorInputs
		}
	case BuildingCategoryRefinery:
		if len(inputs) == 0 {
			return ErrEmptyRefineryInputs
		}
	}
	return nil
}

func validateItemRates(kind string, rates []ItemRate, requireNonEmpty bool) error {
	if len(rates) == 0 {
		if requireNonEmpty {
			return ErrEmptyProductionOutputs
		}
		return nil
	}
	seen := make(map[foundation.ItemID]struct{}, len(rates))
	for _, rate := range rates {
		if err := rate.Validate(); err != nil {
			return fmt.Errorf("%s item %q: %w", kind, rate.ItemID, err)
		}
		if _, ok := seen[rate.ItemID]; ok {
			if kind == "input" {
				return fmt.Errorf("%s item %q: %w", kind, rate.ItemID, ErrDuplicateProductionInput)
			}
			return fmt.Errorf("%s item %q: %w", kind, rate.ItemID, ErrDuplicateProductionOutput)
		}
		seen[rate.ItemID] = struct{}{}
	}
	return nil
}

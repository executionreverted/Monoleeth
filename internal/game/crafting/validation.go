package crafting

import (
	"fmt"
	"strings"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

// Validate reports whether category is supported by the MVP recipe model.
func (category RecipeCategory) Validate() error {
	switch category {
	case RecipeCategoryProcessedMaterial, RecipeCategoryModule, RecipeCategoryShipUnlock:
		return nil
	default:
		return fmt.Errorf("recipe category %q: %w", category, ErrInvalidRecipeCategory)
	}
}

// Validate reports whether kind is a supported recipe output kind.
func (kind RecipeOutputKind) Validate() error {
	switch kind {
	case RecipeOutputKindItem, RecipeOutputKindShipUnlock:
		return nil
	default:
		return fmt.Errorf("recipe output kind %q: %w", kind, ErrInvalidRecipeOutputKind)
	}
}

// Validate reports whether locationType is supported by the crafting spec.
func (locationType CraftLocationType) Validate() error {
	switch locationType {
	case CraftLocationStation,
		CraftLocationOwnedPlanet,
		CraftLocationPlanetBuilding,
		CraftLocationSpecialEventStation:
		return nil
	default:
		return fmt.Errorf("craft location type %q: %w", locationType, ErrInvalidCraftLocationType)
	}
}

// Validate reports whether location has a supported type and concrete id.
func (location CraftLocation) Validate() error {
	if err := location.Type.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(location.ID) == "" {
		return ErrEmptyCraftLocationID
	}
	if location.Type == CraftLocationPlanetBuilding {
		if err := location.PlanetID.Validate(); err != nil {
			return fmt.Errorf("planet_id: %w", err)
		}
	}
	return nil
}

// Validate reports whether input names an item and a positive quantity.
func (input RecipeInput) Validate() error {
	if err := input.ItemID.Validate(); err != nil {
		return err
	}
	if err := input.Quantity.Validate(); err != nil {
		return err
	}
	return nil
}

// Validate reports whether output names an item or ship unlock and a positive quantity.
func (output RecipeOutput) Validate() error {
	if err := output.Kind.Validate(); err != nil {
		return err
	}
	if err := output.Quantity.Validate(); err != nil {
		return err
	}
	switch output.Kind {
	case RecipeOutputKindItem:
		if err := output.ItemID.Validate(); err != nil {
			return err
		}
	case RecipeOutputKindShipUnlock:
		if err := output.ShipID.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate reports whether requirement names a supported role and level.
func (requirement RoleRequirement) Validate() error {
	if err := requirement.Role.Validate(); err != nil {
		return fmt.Errorf("required role %q: %w", requirement.Role, ErrInvalidRequiredRole)
	}
	if err := progression.ValidateRoleLevel(requirement.Level); err != nil {
		return fmt.Errorf("required role level %d: %w", requirement.Level, ErrInvalidRequiredRoleLevel)
	}
	return nil
}

// Validate reports whether definition is a complete static recipe row.
func (definition RecipeDefinition) Validate() error {
	if err := definition.Source.Validate(); err != nil {
		return err
	}
	if err := definition.RecipeID.Validate(); err != nil {
		return err
	}
	if definition.Source.DefinitionID != definition.RecipeID {
		return fmt.Errorf("source %q recipe %q: %w", definition.Source.DefinitionID, definition.RecipeID, ErrRecipeSourceMismatch)
	}
	if err := definition.Category.Validate(); err != nil {
		return err
	}
	if err := definition.Output.Validate(); err != nil {
		return err
	}
	if err := validateRecipeInputs(definition.Inputs); err != nil {
		return err
	}
	if err := definition.RequiredCredits.Validate(); err != nil {
		return fmt.Errorf("required credits: %w", err)
	}
	if err := validateRequiredRank(definition.RequiredRank); err != nil {
		return err
	}
	if err := validateRoleRequirements(definition.RequiredRoleLevels); err != nil {
		return err
	}
	if err := definition.RequiredLocationType.Validate(); err != nil {
		return err
	}
	if definition.CraftDuration <= 0 {
		return fmt.Errorf("craft duration %s: %w", definition.CraftDuration, ErrInvalidCraftDuration)
	}
	return nil
}

// ValidateRankRequirement checks whether playerRank meets this recipe gate.
func (definition RecipeDefinition) ValidateRankRequirement(playerRank int) error {
	if err := validateRequiredRank(definition.RequiredRank); err != nil {
		return err
	}
	if err := progression.ValidateRank(playerRank); err != nil {
		return fmt.Errorf("player rank %d: %w", playerRank, ErrRankRequirementNotMet)
	}
	if playerRank < definition.RequiredRank {
		return fmt.Errorf("player rank %d requires %d: %w", playerRank, definition.RequiredRank, ErrRankRequirementNotMet)
	}
	return nil
}

// ValidateRoleRequirements checks whether roleLevels meet every recipe role gate.
func (definition RecipeDefinition) ValidateRoleRequirements(roleLevels map[progression.RoleType]int) error {
	if err := validateRoleRequirements(definition.RequiredRoleLevels); err != nil {
		return err
	}
	for _, requirement := range definition.RequiredRoleLevels {
		level := roleLevels[requirement.Role]
		if level > 0 {
			if err := progression.ValidateRoleLevel(level); err != nil {
				return fmt.Errorf("role %q level %d: %w", requirement.Role, level, ErrRoleRequirementNotMet)
			}
		}
		if level < requirement.Level {
			return fmt.Errorf("role %q level %d requires %d: %w", requirement.Role, level, requirement.Level, ErrRoleRequirementNotMet)
		}
	}
	return nil
}

// ValidateLocationRequirement checks whether location meets this recipe gate.
func (definition RecipeDefinition) ValidateLocationRequirement(location CraftLocation) error {
	if err := definition.RequiredLocationType.Validate(); err != nil {
		return err
	}
	if err := location.Validate(); err != nil {
		return err
	}
	if location.Type != definition.RequiredLocationType {
		return fmt.Errorf("location type %q requires %q: %w", location.Type, definition.RequiredLocationType, ErrLocationRequirementNotMet)
	}
	return nil
}

// ValidateRequirements checks the non-inventory, non-wallet gates for this recipe.
func (definition RecipeDefinition) ValidateRequirements(
	playerRank int,
	roleLevels map[progression.RoleType]int,
	location CraftLocation,
) error {
	if err := definition.ValidateRankRequirement(playerRank); err != nil {
		return err
	}
	if err := definition.ValidateRoleRequirements(roleLevels); err != nil {
		return err
	}
	if err := definition.ValidateLocationRequirement(location); err != nil {
		return err
	}
	return nil
}

func validateRecipeInputs(inputs []RecipeInput) error {
	if len(inputs) == 0 {
		return ErrEmptyRecipeInputs
	}
	seen := make(map[foundation.ItemID]struct{}, len(inputs))
	for _, input := range inputs {
		if err := input.Validate(); err != nil {
			return err
		}
		if _, ok := seen[input.ItemID]; ok {
			return fmt.Errorf("input item %q: %w", input.ItemID, ErrDuplicateRecipeInput)
		}
		seen[input.ItemID] = struct{}{}
	}
	return nil
}

func validateRequiredRank(rank int) error {
	if err := progression.ValidateRank(rank); err != nil {
		return fmt.Errorf("required rank %d: %w", rank, ErrInvalidRequiredRank)
	}
	return nil
}

func validateRoleRequirements(requirements []RoleRequirement) error {
	seen := make(map[progression.RoleType]struct{}, len(requirements))
	for _, requirement := range requirements {
		if err := requirement.Validate(); err != nil {
			return err
		}
		if _, ok := seen[requirement.Role]; ok {
			return fmt.Errorf("role %q: %w", requirement.Role, ErrDuplicateRoleRequirement)
		}
		seen[requirement.Role] = struct{}{}
	}
	return nil
}

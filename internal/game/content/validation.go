package content

import (
	"errors"
	"fmt"
	"math"

	"gameproject/internal/game/crafting"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/production"
)

var (
	ErrInvalidContentBundle          = errors.New("invalid gameplay content bundle")
	ErrUnknownContentItem            = errors.New("unknown content item")
	ErrUnknownContentLoot            = errors.New("unknown content loot table")
	ErrUnknownContentShip            = errors.New("unknown content ship")
	ErrInvalidContentLootRow         = errors.New("invalid content loot row")
	ErrMissingContentCatalog         = errors.New("missing content catalog")
	ErrInvalidScannerContent         = errors.New("invalid scanner content")
	ErrInvalidStarterContent         = errors.New("invalid starter content")
	ErrInvalidRouteContent           = errors.New("invalid route content")
	ErrInvalidProductionRulesContent = errors.New("invalid production rules content")
)

// Validate proves the static gameplay bundle is internally consistent before
// runtime uses it.
func (bundle GameplayContent) Validate() error {
	if bundle.Maps == nil {
		return fmt.Errorf("maps: %w", ErrMissingContentCatalog)
	}
	if len(bundle.Items) == 0 {
		return fmt.Errorf("items: %w", ErrMissingContentCatalog)
	}
	if len(bundle.LootTables) == 0 {
		return fmt.Errorf("loot tables: %w", ErrMissingContentCatalog)
	}
	if err := validateItemDefinitions(bundle); err != nil {
		return err
	}
	if err := validateLootTables(bundle); err != nil {
		return err
	}
	if err := validateModuleItemReferences(bundle); err != nil {
		return err
	}
	if err := validateRecipeReferences(bundle); err != nil {
		return err
	}
	if err := validateProductionReferences(bundle); err != nil {
		return err
	}
	if err := validateMapLootReferences(bundle); err != nil {
		return err
	}
	if err := validateShopReferences(bundle); err != nil {
		return err
	}
	if err := bundle.Route.Validate(bundle); err != nil {
		return err
	}
	if err := bundle.Rules.Validate(bundle); err != nil {
		return err
	}
	if err := bundle.Scanner.Validate(bundle.Maps); err != nil {
		return err
	}
	if err := bundle.Starter.Validate(bundle); err != nil {
		return err
	}
	return nil
}

func validateShopReferences(bundle GameplayContent) error {
	return bundle.Shop.ValidateReferences(shopReferenceResolver(bundle.Items, bundle.Modules, bundle.Ships))
}

func validateKnownItem(bundle GameplayContent, kind string, itemID foundation.ItemID) error {
	if _, ok := bundle.Items[itemID]; !ok {
		return fmt.Errorf("%s item %q: %w", kind, itemID, ErrUnknownContentItem)
	}
	return nil
}

func validateItemDefinitions(bundle GameplayContent) error {
	for itemID, definition := range bundle.Items {
		if err := definition.Validate(); err != nil {
			return fmt.Errorf("item %q: %w", itemID, err)
		}
		if definition.ItemID != itemID {
			return fmt.Errorf("item key %q definition %q: %w", itemID, definition.ItemID, ErrInvalidContentBundle)
		}
	}
	return nil
}

func validateLootTables(bundle GameplayContent) error {
	for tableID, table := range bundle.LootTables {
		if err := table.Source.Validate(); err != nil {
			return fmt.Errorf("loot table %q: %w", tableID, err)
		}
		if table.Source.DefinitionID.String() != tableID {
			return fmt.Errorf("loot table %q source %q: %w", tableID, table.Source.DefinitionID, ErrInvalidContentBundle)
		}
		if len(table.Rows) == 0 {
			return fmt.Errorf("loot table %q rows: %w", tableID, ErrInvalidContentLootRow)
		}
		for index, row := range table.Rows {
			if err := validateLootRow(bundle, row); err != nil {
				return fmt.Errorf("loot table %q row %d: %w", tableID, index, err)
			}
		}
	}
	return nil
}

func validateLootRow(bundle GameplayContent, row loot.LootRow) error {
	if err := row.ItemDefinition.Validate(); err != nil {
		return err
	}
	itemID := row.ItemDefinition.ItemID
	definition, ok := bundle.Items[itemID]
	if !ok {
		return fmt.Errorf("item %q: %w", itemID, ErrUnknownContentItem)
	}
	if definition.Source != row.ItemDefinition.Source {
		return fmt.Errorf("item %q source: %w", itemID, ErrInvalidContentBundle)
	}
	if row.MinQuantity <= 0 || row.MaxQuantity < row.MinQuantity {
		return fmt.Errorf("quantity %d..%d: %w", row.MinQuantity, row.MaxQuantity, ErrInvalidContentLootRow)
	}
	if math.IsNaN(row.Chance) || math.IsInf(row.Chance, 0) || row.Chance < 0 || row.Chance > 1 {
		return fmt.Errorf("chance %v: %w", row.Chance, ErrInvalidContentLootRow)
	}
	return nil
}

func validateModuleItemReferences(bundle GameplayContent) error {
	for _, module := range bundle.Modules.Definitions() {
		if _, ok := bundle.Items[module.ItemID]; !ok {
			return fmt.Errorf("module %q item %q: %w", module.Source.DefinitionID, module.ItemID, ErrUnknownContentItem)
		}
	}
	return nil
}

func validateRecipeReferences(bundle GameplayContent) error {
	for _, recipe := range bundle.Recipes.Definitions() {
		for _, input := range recipe.Inputs {
			if _, ok := bundle.Items[input.ItemID]; !ok {
				return fmt.Errorf("recipe %q input %q: %w", recipe.RecipeID, input.ItemID, ErrUnknownContentItem)
			}
		}
		switch recipe.Output.Kind {
		case crafting.RecipeOutputKindItem:
			if _, ok := bundle.Items[recipe.Output.ItemID]; !ok {
				return fmt.Errorf("recipe %q output %q: %w", recipe.RecipeID, recipe.Output.ItemID, ErrUnknownContentItem)
			}
		case crafting.RecipeOutputKindShipUnlock:
			if _, ok := bundle.Ships.Get(recipe.Output.ShipID); !ok {
				return fmt.Errorf("recipe %q ship %q: %w", recipe.RecipeID, recipe.Output.ShipID, ErrUnknownContentShip)
			}
		}
	}
	return nil
}

func validateProductionReferences(bundle GameplayContent) error {
	for _, definition := range bundle.Production.Definitions() {
		if err := validateItemRates(bundle, "production input", definition.DefinitionID.String(), definition.Inputs); err != nil {
			return err
		}
		if err := validateItemRates(bundle, "production output", definition.DefinitionID.String(), definition.Outputs); err != nil {
			return err
		}
	}
	return nil
}

func validateItemRates(bundle GameplayContent, kind string, definitionID string, rates []production.ItemRate) error {
	for _, rate := range rates {
		if err := validateKnownItem(bundle, kind+" "+definitionID, rate.ItemID); err != nil {
			return err
		}
	}
	return nil
}

func validateMapLootReferences(bundle GameplayContent) error {
	for _, definition := range bundle.Maps.Definitions() {
		for _, profile := range definition.NPCDropProfiles {
			if _, ok := bundle.LootTables[profile.LootTableID]; !ok {
				return fmt.Errorf("map %q drop profile %q loot table %q: %w", definition.InternalMapID, profile.DropProfileID, profile.LootTableID, ErrUnknownContentLoot)
			}
		}
	}
	return nil
}

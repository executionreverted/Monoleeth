package crafting

import (
	"fmt"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

// RecipeCatalogVersion identifies the first static crafting catalog slice.
const RecipeCatalogVersion catalog.Version = "recipe_catalog_mvp_v1"

// MVP recipe ids.
const (
	RecipeIDRefinedAlloy catalog.DefinitionID = "refined_alloy_batch"
	RecipeIDLaserAlphaT1 catalog.DefinitionID = "laser_alpha_t1"
	RecipeIDScoutT1      catalog.DefinitionID = "scout_t1_unlock"
)

// RecipeCatalog indexes static recipe definitions by recipe id.
type RecipeCatalog struct {
	definitions []RecipeDefinition
	byRecipeID  map[catalog.DefinitionID]RecipeDefinition
}

// NewRecipeCatalog validates and indexes recipe definitions.
func NewRecipeCatalog(definitions []RecipeDefinition) (RecipeCatalog, error) {
	byRecipeID := make(map[catalog.DefinitionID]RecipeDefinition, len(definitions))
	cloned := make([]RecipeDefinition, 0, len(definitions))
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return RecipeCatalog{}, err
		}
		if _, ok := byRecipeID[definition.RecipeID]; ok {
			return RecipeCatalog{}, fmt.Errorf("recipe %q: %w", definition.RecipeID, ErrDuplicateRecipeDefinition)
		}
		clonedDefinition := cloneRecipeDefinition(definition)
		byRecipeID[clonedDefinition.RecipeID] = clonedDefinition
		cloned = append(cloned, clonedDefinition)
	}
	return RecipeCatalog{
		definitions: cloned,
		byRecipeID:  byRecipeID,
	}, nil
}

// MVPRecipeCatalog returns the validated MVP recipe catalog.
func MVPRecipeCatalog() (RecipeCatalog, error) {
	return NewRecipeCatalog(MVPRecipeDefinitions())
}

// MustMVPRecipeCatalog returns the validated MVP recipe catalog or panics if
// checked-in catalog data is invalid.
func MustMVPRecipeCatalog() RecipeCatalog {
	catalogRows, err := MVPRecipeCatalog()
	if err != nil {
		panic(err)
	}
	return catalogRows
}

// Definitions returns all definitions in deterministic catalog order.
func (recipeCatalog RecipeCatalog) Definitions() []RecipeDefinition {
	definitions := make([]RecipeDefinition, 0, len(recipeCatalog.definitions))
	for _, definition := range recipeCatalog.definitions {
		definitions = append(definitions, cloneRecipeDefinition(definition))
	}
	return definitions
}

// Get returns the recipe definition for recipeID.
func (recipeCatalog RecipeCatalog) Get(recipeID catalog.DefinitionID) (RecipeDefinition, bool) {
	definition, ok := recipeCatalog.byRecipeID[recipeID]
	if !ok {
		return RecipeDefinition{}, false
	}
	return cloneRecipeDefinition(definition), true
}

// MustGet returns one recipe definition by id or an unknown-definition error.
func (recipeCatalog RecipeCatalog) MustGet(recipeID catalog.DefinitionID) (RecipeDefinition, error) {
	definition, ok := recipeCatalog.Get(recipeID)
	if !ok {
		return RecipeDefinition{}, fmt.Errorf("recipe %q: %w", recipeID, ErrUnknownRecipeDefinition)
	}
	return definition, nil
}

// MVPRecipeDefinitions returns the initial recipe rows for Phase 06 Wave 1.
func MVPRecipeDefinitions() []RecipeDefinition {
	return []RecipeDefinition{
		newMVPItemRecipe(
			RecipeIDRefinedAlloy,
			RecipeCategoryProcessedMaterial,
			"refined_alloy",
			5,
			[]RecipeInput{
				mustRecipeInput("iron_ore", 20),
				mustRecipeInput("carbon_shards", 5),
			},
			100,
			1,
			[]RoleRequirement{{Role: progression.RoleTypeCrafting, Level: 1}},
			5*time.Minute,
			true,
		),
		newMVPItemRecipe(
			RecipeIDLaserAlphaT1,
			RecipeCategoryModule,
			"laser_alpha_t1",
			1,
			[]RecipeInput{
				mustRecipeInput("refined_alloy", 25),
				mustRecipeInput("laser_lens", 4),
				mustRecipeInput("energy_cell", 2),
			},
			750,
			1,
			[]RoleRequirement{{Role: progression.RoleTypeCrafting, Level: 1}},
			30*time.Minute,
			true,
		),
		newMVPShipUnlockRecipe(
			RecipeIDScoutT1,
			"scout_t1",
			[]RecipeInput{
				mustRecipeInput("refined_alloy", 100),
				mustRecipeInput("scanner_circuit", 15),
				mustRecipeInput("warp_coil", 5),
			},
			2_500,
			2,
			[]RoleRequirement{{Role: progression.RoleTypeCrafting, Level: 2}},
			2*time.Hour,
		),
	}
}

func newMVPItemRecipe(
	recipeID catalog.DefinitionID,
	category RecipeCategory,
	outputItemID foundation.ItemID,
	outputQuantity int64,
	inputs []RecipeInput,
	requiredCredits int64,
	requiredRank int,
	requiredRoleLevels []RoleRequirement,
	duration time.Duration,
	repeatable bool,
) RecipeDefinition {
	return RecipeDefinition{
		Source:               mustRecipeSource(recipeID),
		RecipeID:             recipeID,
		Category:             category,
		Output:               RecipeOutput{Kind: RecipeOutputKindItem, ItemID: outputItemID, Quantity: mustQuantity(outputQuantity), Tradeable: true},
		Inputs:               append([]RecipeInput(nil), inputs...),
		RequiredCredits:      mustMoney(requiredCredits),
		RequiredRank:         requiredRank,
		RequiredRoleLevels:   append([]RoleRequirement(nil), requiredRoleLevels...),
		RequiredLocationType: CraftLocationStation,
		CraftDuration:        duration,
		Repeatable:           repeatable,
	}
}

func newMVPShipUnlockRecipe(
	recipeID catalog.DefinitionID,
	outputShipID foundation.ShipID,
	inputs []RecipeInput,
	requiredCredits int64,
	requiredRank int,
	requiredRoleLevels []RoleRequirement,
	duration time.Duration,
) RecipeDefinition {
	return RecipeDefinition{
		Source:               mustRecipeSource(recipeID),
		RecipeID:             recipeID,
		Category:             RecipeCategoryShipUnlock,
		Output:               RecipeOutput{Kind: RecipeOutputKindShipUnlock, ShipID: outputShipID, Quantity: mustQuantity(1), Tradeable: false},
		Inputs:               append([]RecipeInput(nil), inputs...),
		RequiredCredits:      mustMoney(requiredCredits),
		RequiredRank:         requiredRank,
		RequiredRoleLevels:   append([]RoleRequirement(nil), requiredRoleLevels...),
		RequiredLocationType: CraftLocationStation,
		CraftDuration:        duration,
		Repeatable:           false,
	}
}

func mustRecipeInput(itemID foundation.ItemID, quantity int64) RecipeInput {
	return RecipeInput{
		ItemID:   itemID,
		Quantity: mustQuantity(quantity),
	}
}

func mustRecipeSource(recipeID catalog.DefinitionID) catalog.VersionedDefinition {
	source, err := catalog.NewRecipeSource(recipeID.String(), RecipeCatalogVersion.String())
	if err != nil {
		panic(err)
	}
	return source
}

func mustQuantity(amount int64) foundation.Quantity {
	quantity, err := foundation.NewQuantity(amount)
	if err != nil {
		panic(err)
	}
	return quantity
}

func mustMoney(amount int64) foundation.Money {
	money, err := foundation.NewMoney(amount)
	if err != nil {
		panic(err)
	}
	return money
}

func cloneRecipeDefinition(definition RecipeDefinition) RecipeDefinition {
	definition.Inputs = append([]RecipeInput(nil), definition.Inputs...)
	definition.RequiredRoleLevels = append([]RoleRequirement(nil), definition.RequiredRoleLevels...)
	return definition
}

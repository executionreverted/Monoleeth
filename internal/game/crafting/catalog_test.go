package crafting

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

func TestMVPRecipeCatalogValidates(t *testing.T) {
	recipeCatalog := MustMVPRecipeCatalog()
	definitions := recipeCatalog.Definitions()

	if got, want := len(definitions), 3; got != want {
		t.Fatalf("MVP definitions count = %d, want %d", got, want)
	}

	tests := []struct {
		recipeID catalog.DefinitionID
		category RecipeCategory
		kind     RecipeOutputKind
		rank     int
	}{
		{RecipeIDRefinedAlloy, RecipeCategoryProcessedMaterial, RecipeOutputKindItem, 1},
		{RecipeIDLaserAlphaT1, RecipeCategoryModule, RecipeOutputKindItem, 1},
		{RecipeIDScoutT1, RecipeCategoryShipUnlock, RecipeOutputKindShipUnlock, 2},
	}

	for _, test := range tests {
		t.Run(test.recipeID.String(), func(t *testing.T) {
			definition, err := recipeCatalog.MustGet(test.recipeID)
			if err != nil {
				t.Fatalf("MustGet(%q) error = %v, want nil", test.recipeID, err)
			}
			if err := definition.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if definition.Source.Version != RecipeCatalogVersion {
				t.Fatalf("Source.Version = %q, want %q", definition.Source.Version, RecipeCatalogVersion)
			}
			if definition.Category != test.category {
				t.Fatalf("Category = %q, want %q", definition.Category, test.category)
			}
			if definition.Output.Kind != test.kind {
				t.Fatalf("Output.Kind = %q, want %q", definition.Output.Kind, test.kind)
			}
			if definition.RequiredRank != test.rank {
				t.Fatalf("RequiredRank = %d, want %d", definition.RequiredRank, test.rank)
			}
			if definition.RequiredCredits.IsZero() {
				t.Fatal("RequiredCredits is zero, want positive")
			}
			if definition.Output.Quantity.IsZero() {
				t.Fatal("Output.Quantity is zero, want positive")
			}
			if len(definition.Inputs) == 0 {
				t.Fatal("Inputs empty, want at least one input")
			}
		})
	}
}

func TestRecipeDefinitionRejectsInvalidRows(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*RecipeDefinition)
		wantErr error
	}{
		{
			name: "source mismatch",
			mutate: func(definition *RecipeDefinition) {
				definition.Source.DefinitionID = "other_recipe"
			},
			wantErr: ErrRecipeSourceMismatch,
		},
		{
			name: "invalid category",
			mutate: func(definition *RecipeDefinition) {
				definition.Category = "alchemy"
			},
			wantErr: ErrInvalidRecipeCategory,
		},
		{
			name: "invalid output kind",
			mutate: func(definition *RecipeDefinition) {
				definition.Output.Kind = "credits"
			},
			wantErr: ErrInvalidRecipeOutputKind,
		},
		{
			name: "zero output quantity",
			mutate: func(definition *RecipeDefinition) {
				definition.Output.Quantity = foundation.Quantity{}
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "empty inputs",
			mutate: func(definition *RecipeDefinition) {
				definition.Inputs = nil
			},
			wantErr: ErrEmptyRecipeInputs,
		},
		{
			name: "zero input quantity",
			mutate: func(definition *RecipeDefinition) {
				definition.Inputs[0].Quantity = foundation.Quantity{}
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "duplicate input",
			mutate: func(definition *RecipeDefinition) {
				definition.Inputs = append(definition.Inputs, definition.Inputs[0])
			},
			wantErr: ErrDuplicateRecipeInput,
		},
		{
			name: "zero fee",
			mutate: func(definition *RecipeDefinition) {
				definition.RequiredCredits = foundation.Money{}
			},
			wantErr: foundation.ErrNonPositiveAmount,
		},
		{
			name: "invalid rank requirement",
			mutate: func(definition *RecipeDefinition) {
				definition.RequiredRank = 0
			},
			wantErr: ErrInvalidRequiredRank,
		},
		{
			name: "invalid role",
			mutate: func(definition *RecipeDefinition) {
				definition.RequiredRoleLevels = []RoleRequirement{{Role: "alchemy", Level: 1}}
			},
			wantErr: ErrInvalidRequiredRole,
		},
		{
			name: "invalid role level",
			mutate: func(definition *RecipeDefinition) {
				definition.RequiredRoleLevels = []RoleRequirement{{Role: progression.RoleTypeCrafting, Level: 0}}
			},
			wantErr: ErrInvalidRequiredRoleLevel,
		},
		{
			name: "duplicate role requirement",
			mutate: func(definition *RecipeDefinition) {
				definition.RequiredRoleLevels = []RoleRequirement{
					{Role: progression.RoleTypeCrafting, Level: 1},
					{Role: progression.RoleTypeCrafting, Level: 2},
				}
			},
			wantErr: ErrDuplicateRoleRequirement,
		},
		{
			name: "invalid location type",
			mutate: func(definition *RecipeDefinition) {
				definition.RequiredLocationType = "kitchen"
			},
			wantErr: ErrInvalidCraftLocationType,
		},
		{
			name: "zero duration",
			mutate: func(definition *RecipeDefinition) {
				definition.CraftDuration = 0
			},
			wantErr: ErrInvalidCraftDuration,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			definition := validRecipeDefinition()
			tc.mutate(&definition)
			if _, err := NewRecipeCatalog([]RecipeDefinition{definition}); !errors.Is(err, tc.wantErr) {
				t.Fatalf("NewRecipeCatalog() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestRecipeCatalogRejectsDuplicateAndUnknownRecipes(t *testing.T) {
	definition := validRecipeDefinition()

	if _, err := NewRecipeCatalog([]RecipeDefinition{definition, definition}); !errors.Is(err, ErrDuplicateRecipeDefinition) {
		t.Fatalf("duplicate recipe error = %v, want %v", err, ErrDuplicateRecipeDefinition)
	}

	recipeCatalog := MustMVPRecipeCatalog()
	if _, err := recipeCatalog.MustGet("missing_recipe"); !errors.Is(err, ErrUnknownRecipeDefinition) {
		t.Fatalf("unknown recipe error = %v, want %v", err, ErrUnknownRecipeDefinition)
	}
}

func TestCraftJobStoresRecipeVersionCompletesAtAndState(t *testing.T) {
	recipeCatalog := MustMVPRecipeCatalog()
	definition, err := recipeCatalog.MustGet(RecipeIDRefinedAlloy)
	if err != nil {
		t.Fatalf("MustGet(%q) error = %v, want nil", RecipeIDRefinedAlloy, err)
	}

	startedAt := time.Date(2026, 6, 17, 15, 0, 0, 0, time.UTC)
	location := CraftLocation{Type: CraftLocationStation, ID: "origin-station"}
	job, err := NewCraftJob("craft-job-1", "player-1", definition, location, startedAt)
	if err != nil {
		t.Fatalf("NewCraftJob() error = %v, want nil", err)
	}

	if job.RecipeSource.DefinitionID != definition.RecipeID {
		t.Fatalf("RecipeSource.DefinitionID = %q, want %q", job.RecipeSource.DefinitionID, definition.RecipeID)
	}
	if job.RecipeSource.Version != RecipeCatalogVersion {
		t.Fatalf("RecipeSource.Version = %q, want %q", job.RecipeSource.Version, RecipeCatalogVersion)
	}
	if !job.CompletesAt.Equal(startedAt.Add(definition.CraftDuration)) {
		t.Fatalf("CompletesAt = %s, want %s", job.CompletesAt, startedAt.Add(definition.CraftDuration))
	}
	if job.State != CraftJobStateRunning {
		t.Fatalf("State = %q, want %q", job.State, CraftJobStateRunning)
	}
	if err := job.Validate(); err != nil {
		t.Fatalf("job Validate() = %v, want nil", err)
	}
}

func TestRequirementHelpersFailDeterministically(t *testing.T) {
	definition := validRecipeDefinition()
	definition.RequiredRank = 3
	definition.RequiredRoleLevels = []RoleRequirement{
		{Role: progression.RoleTypeCombat, Level: 2},
		{Role: progression.RoleTypeCrafting, Level: 3},
	}

	wrongLocation := CraftLocation{Type: CraftLocationOwnedPlanet, ID: "planet-1"}
	if err := definition.ValidateLocationRequirement(wrongLocation); !errors.Is(err, ErrLocationRequirementNotMet) {
		t.Fatalf("wrong location error = %v, want %v", err, ErrLocationRequirementNotMet)
	}

	if err := definition.ValidateRankRequirement(2); !errors.Is(err, ErrRankRequirementNotMet) {
		t.Fatalf("rank requirement error = %v, want %v", err, ErrRankRequirementNotMet)
	}

	err := definition.ValidateRoleRequirements(map[progression.RoleType]int{
		progression.RoleTypeCrafting: 5,
	})
	if !errors.Is(err, ErrRoleRequirementNotMet) {
		t.Fatalf("role requirement error = %v, want %v", err, ErrRoleRequirementNotMet)
	}
	if !strings.Contains(err.Error(), `role "combat"`) {
		t.Fatalf("role requirement error = %q, want first missing role to be combat", err)
	}
}

func validRecipeDefinition() RecipeDefinition {
	return RecipeDefinition{
		Source:   mustRecipeSource("test_recipe"),
		RecipeID: "test_recipe",
		Category: RecipeCategoryProcessedMaterial,
		Output: RecipeOutput{
			Kind:      RecipeOutputKindItem,
			ItemID:    "refined_alloy",
			Quantity:  mustQuantity(1),
			Tradeable: true,
		},
		Inputs: []RecipeInput{
			mustRecipeInput("iron_ore", 5),
			mustRecipeInput("carbon_shards", 2),
		},
		RequiredCredits:      mustMoney(25),
		RequiredRank:         1,
		RequiredRoleLevels:   []RoleRequirement{{Role: progression.RoleTypeCrafting, Level: 1}},
		RequiredLocationType: CraftLocationStation,
		CraftDuration:        time.Minute,
		Repeatable:           true,
	}
}

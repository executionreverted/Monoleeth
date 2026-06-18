package production

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/world"
)

func TestCraftLocationAuthorizerAllowsOwnedPlanetAndActiveBuilding(t *testing.T) {
	planets := discovery.NewInMemoryStore()
	productionStore := NewInMemoryStore()
	authorizer := newTestCraftLocationAuthorizer(t, planets, productionStore)
	playerID := foundation.PlayerID("player-1")
	planetID := foundation.PlanetID("planet-owned")
	buildingID := BuildingID("building-1")

	materializeCraftAuthorizerPlanet(t, planets, planetID, playerID)
	initializeCraftAuthorizerProduction(t, productionStore, planetID)
	upsertCraftAuthorizerBuilding(t, productionStore, planetID, buildingID, BuildingStateActive)

	if err := authorizer.AuthorizeCraftLocation(crafting.CraftLocationAuthorizationInput{
		PlayerID: playerID,
		Recipe:   craftAuthorizerRecipe(t, crafting.CraftLocationOwnedPlanet),
		Location: crafting.CraftLocation{Type: crafting.CraftLocationOwnedPlanet, ID: planetID.String()},
	}); err != nil {
		t.Fatalf("AuthorizeCraftLocation(owned planet) error = %v, want nil", err)
	}

	if err := authorizer.AuthorizeCraftLocation(crafting.CraftLocationAuthorizationInput{
		PlayerID: playerID,
		Recipe:   craftAuthorizerRecipe(t, crafting.CraftLocationPlanetBuilding),
		Location: crafting.CraftLocation{Type: crafting.CraftLocationPlanetBuilding, ID: buildingID.String(), PlanetID: planetID},
	}); err != nil {
		t.Fatalf("AuthorizeCraftLocation(planet building) error = %v, want nil", err)
	}
}

func TestCraftLocationAuthorizerRejectsFakePlanetLocations(t *testing.T) {
	tests := []struct {
		name      string
		seed      func(*testing.T, *discovery.InMemoryStore, *InMemoryStore)
		location  crafting.CraftLocation
		wantError error
	}{
		{
			name:      "unknown planet",
			location:  crafting.CraftLocation{Type: crafting.CraftLocationOwnedPlanet, ID: "planet-missing"},
			wantError: ErrCraftPlanetNotOwned,
		},
		{
			name: "unowned planet",
			seed: func(t *testing.T, planets *discovery.InMemoryStore, productionStore *InMemoryStore) {
				materializeCraftAuthorizerPlanet(t, planets, "planet-unowned", "")
				initializeCraftAuthorizerProduction(t, productionStore, "planet-unowned")
			},
			location:  crafting.CraftLocation{Type: crafting.CraftLocationOwnedPlanet, ID: "planet-unowned"},
			wantError: ErrCraftPlanetNotOwned,
		},
		{
			name: "other-owned planet",
			seed: func(t *testing.T, planets *discovery.InMemoryStore, productionStore *InMemoryStore) {
				materializeCraftAuthorizerPlanet(t, planets, "planet-other", "player-2")
				initializeCraftAuthorizerProduction(t, productionStore, "planet-other")
			},
			location:  crafting.CraftLocation{Type: crafting.CraftLocationOwnedPlanet, ID: "planet-other"},
			wantError: ErrCraftPlanetNotOwned,
		},
		{
			name: "missing production rows",
			seed: func(t *testing.T, planets *discovery.InMemoryStore, productionStore *InMemoryStore) {
				materializeCraftAuthorizerPlanet(t, planets, "planet-no-production", "player-1")
			},
			location:  crafting.CraftLocation{Type: crafting.CraftLocationOwnedPlanet, ID: "planet-no-production"},
			wantError: ErrCraftPlanetProductionMissing,
		},
		{
			name: "missing building",
			seed: func(t *testing.T, planets *discovery.InMemoryStore, productionStore *InMemoryStore) {
				materializeCraftAuthorizerPlanet(t, planets, "planet-owned", "player-1")
				initializeCraftAuthorizerProduction(t, productionStore, "planet-owned")
			},
			location:  crafting.CraftLocation{Type: crafting.CraftLocationPlanetBuilding, ID: "building-missing", PlanetID: "planet-owned"},
			wantError: ErrCraftBuildingNotFound,
		},
		{
			name: "inactive building",
			seed: func(t *testing.T, planets *discovery.InMemoryStore, productionStore *InMemoryStore) {
				materializeCraftAuthorizerPlanet(t, planets, "planet-owned", "player-1")
				initializeCraftAuthorizerProduction(t, productionStore, "planet-owned")
				upsertCraftAuthorizerBuilding(t, productionStore, "planet-owned", "building-disabled", BuildingStateDisabled)
			},
			location:  crafting.CraftLocation{Type: crafting.CraftLocationPlanetBuilding, ID: "building-disabled", PlanetID: "planet-owned"},
			wantError: ErrCraftBuildingInactive,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			planets := discovery.NewInMemoryStore()
			productionStore := NewInMemoryStore()
			authorizer := newTestCraftLocationAuthorizer(t, planets, productionStore)
			if tc.seed != nil {
				tc.seed(t, planets, productionStore)
			}

			err := authorizer.AuthorizeCraftLocation(crafting.CraftLocationAuthorizationInput{
				PlayerID: "player-1",
				Recipe:   craftAuthorizerRecipe(t, tc.location.Type),
				Location: tc.location,
			})
			if !errors.Is(err, tc.wantError) {
				t.Fatalf("AuthorizeCraftLocation error = %v, want %v", err, tc.wantError)
			}
		})
	}
}

func newTestCraftLocationAuthorizer(
	t *testing.T,
	planets *discovery.InMemoryStore,
	productionStore *InMemoryStore,
) *CraftLocationAuthorizer {
	t.Helper()

	authorizer, err := NewCraftLocationAuthorizer(CraftLocationAuthorizerConfig{
		Planets:    planets,
		Production: productionStore,
	})
	if err != nil {
		t.Fatalf("NewCraftLocationAuthorizer() error = %v, want nil", err)
	}
	return authorizer
}

func craftAuthorizerRecipe(t *testing.T, locationType crafting.CraftLocationType) crafting.RecipeDefinition {
	t.Helper()

	recipeID := catalog.DefinitionID("craft_authorizer_" + locationType.String())
	source, err := catalog.NewRecipeSource(recipeID.String(), crafting.RecipeCatalogVersion.String())
	if err != nil {
		t.Fatalf("NewRecipeSource() error = %v, want nil", err)
	}
	quantity, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("NewQuantity() error = %v, want nil", err)
	}
	credits, err := foundation.NewMoney(1)
	if err != nil {
		t.Fatalf("NewMoney() error = %v, want nil", err)
	}
	recipe := crafting.RecipeDefinition{
		Source:               source,
		RecipeID:             recipeID,
		Category:             crafting.RecipeCategoryProcessedMaterial,
		Output:               crafting.RecipeOutput{Kind: crafting.RecipeOutputKindItem, ItemID: "refined_alloy", Quantity: quantity, Tradeable: true},
		Inputs:               []crafting.RecipeInput{{ItemID: "iron_ore", Quantity: quantity}},
		RequiredCredits:      credits,
		RequiredRank:         1,
		RequiredRoleLevels:   []crafting.RoleRequirement{{Role: progression.RoleTypeCrafting, Level: 1}},
		RequiredLocationType: locationType,
		CraftDuration:        time.Minute,
		Repeatable:           true,
	}
	if err := recipe.Validate(); err != nil {
		t.Fatalf("recipe Validate() error = %v, want nil", err)
	}
	return recipe
}

func materializeCraftAuthorizerPlanet(
	t *testing.T,
	store *discovery.InMemoryStore,
	planetID foundation.PlanetID,
	ownerID foundation.PlayerID,
) {
	t.Helper()

	discoveredAt := testTime(0)
	planet := discovery.Planet{
		ID:           planetID,
		WorldID:      "world-1",
		ZoneID:       "zone-1",
		Coordinates:  world.Vec2{X: 10, Y: 20},
		Biome:        discovery.PlanetBiomeOriginBelt,
		Type:         discovery.PlanetTypeTerrestrial,
		Rarity:       discovery.PlanetRarityCommon,
		Level:        1,
		DiscoveredAt: discoveredAt,
		DiscoveredBy: "player-scout",
	}
	if !ownerID.IsZero() {
		changedAt := testTime(1)
		planet.OwnerPlayerID = ownerID
		planet.OwnerChangedAt = &changedAt
	}
	_, err := store.MaterializePlanet(discovery.MaterializePlanetInput{
		CandidateKey: discovery.PlanetMaterializationKey("candidate-" + planetID.String()),
		Planet:       planet,
	})
	if err != nil {
		t.Fatalf("MaterializePlanet(%q) error = %v, want nil", planetID, err)
	}
}

func initializeCraftAuthorizerProduction(
	t *testing.T,
	store *InMemoryStore,
	planetID foundation.PlanetID,
) {
	t.Helper()

	_, err := store.InitializePlanetProduction(InitializePlanetProductionInput{
		PlanetID:              planetID,
		LastCalculatedAt:      testTime(1),
		StorageCapacityUnits:  100,
		EnergyCapacityPerHour: 25,
		UpdatedAt:             testTime(1),
	})
	if err != nil {
		t.Fatalf("InitializePlanetProduction(%q) error = %v, want nil", planetID, err)
	}
}

func upsertCraftAuthorizerBuilding(
	t *testing.T,
	store *InMemoryStore,
	planetID foundation.PlanetID,
	buildingID BuildingID,
	state BuildingState,
) {
	t.Helper()

	definition, err := MustMVPCatalog().MustGet(ProductionDefinitionIDAlloyFoundryL1)
	if err != nil {
		t.Fatalf("MustGet(%q) error = %v, want nil", ProductionDefinitionIDAlloyFoundryL1, err)
	}
	building, err := NewPlanetBuilding(buildingID, planetID, definition, state, testTime(1), testTime(2))
	if err != nil {
		t.Fatalf("NewPlanetBuilding() error = %v, want nil", err)
	}
	if _, _, err := store.UpsertBuilding(building); err != nil {
		t.Fatalf("UpsertBuilding() error = %v, want nil", err)
	}
}

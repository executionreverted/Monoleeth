package content

import (
	"context"
	"errors"
	"testing"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestDefaultGameplayContentValidates(t *testing.T) {
	bundle := validBundle(t)

	if err := bundle.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if _, ok := bundle.Items["raw_ore"]; !ok {
		t.Fatal("raw_ore missing from content items")
	}
	if _, ok := bundle.LootTables[TrainingDroneSalvageLootTableID]; !ok {
		t.Fatalf("%s missing from loot tables", TrainingDroneSalvageLootTableID)
	}
	if _, ok := bundle.Modules.Lookup("laser_alpha_t1"); !ok {
		t.Fatal("laser_alpha_t1 missing from module catalog")
	}
	if _, ok := bundle.Ships.Get("starter"); !ok {
		t.Fatal("starter ship missing from ship catalog")
	}
	if got := bundle.Scanner.CandidateOptions.ProfileVersion; got != DefaultScannerProfileVersion {
		t.Fatalf("scanner profile = %q, want %q", got, DefaultScannerProfileVersion)
	}
	if got, want := len(bundle.Scanner.MapProfiles), 3; got != want {
		t.Fatalf("scanner map profile count = %d, want %d", got, want)
	}
	if len(bundle.Shop.ShopProducts) == 0 || len(bundle.Shop.Categories) == 0 {
		t.Fatalf("shop content incomplete: categories=%d products=%d", len(bundle.Shop.Categories), len(bundle.Shop.ShopProducts))
	}
	if !bundle.Route.ResourceRouteable("refined_alloy") {
		t.Fatal("refined_alloy missing from routeable content")
	}
	if bundle.Rules.ClaimRange <= 0 || len(bundle.Rules.BuildingCosts) == 0 {
		t.Fatalf("production rules incomplete: %+v", bundle.Rules)
	}
	if bundle.Combat.BasicLaserSkillID == "" || bundle.Combat.LootPickupRange <= 0 {
		t.Fatalf("combat rules incomplete: %+v", bundle.Combat)
	}
}

func TestStaticRepositoryLoadsValidatedPublishedContent(t *testing.T) {
	bundle, err := LoadPublishedContent(context.Background(), NewStaticRepository(), world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("LoadPublishedContent() error = %v, want nil", err)
	}
	if bundle.Maps == nil || len(bundle.Items) == 0 || len(bundle.Starter.ModuleItemIDs) == 0 {
		t.Fatalf("published content incomplete: maps=%v items=%d starter=%+v", bundle.Maps != nil, len(bundle.Items), bundle.Starter)
	}
}

func TestLoadPublishedContentRejectsMissingRepository(t *testing.T) {
	_, err := LoadPublishedContent(context.Background(), nil, world.WorldID("world-1"))
	if !errors.Is(err, ErrMissingContentRepository) {
		t.Fatalf("LoadPublishedContent() error = %v, want %v", err, ErrMissingContentRepository)
	}
}

func TestGameplayContentRejectsShopUnknownItemReference(t *testing.T) {
	bundle := validBundle(t)
	bundle.Shop.ShopProducts = append([]catalog.ShopProductDefinition(nil), bundle.Shop.ShopProducts...)
	found := false
	for index, product := range bundle.Shop.ShopProducts {
		if product.GrantTarget.Kind == catalog.GrantTargetKindItem {
			product.GrantTarget.RefID = "missing_item"
			bundle.Shop.ShopProducts[index] = product
			found = true
			break
		}
	}
	if !found {
		t.Fatal("default shop item product missing")
	}

	err := bundle.Validate()
	if !errors.Is(err, catalog.ErrMissingContentReference) {
		t.Fatalf("Validate() error = %v, want %v", err, catalog.ErrMissingContentReference)
	}
}

func TestGameplayContentRejectsRouteUnknownItem(t *testing.T) {
	bundle := validBundle(t)
	bundle.Route.RouteableItemIDs[0] = "missing_item"

	err := bundle.Validate()
	if !errors.Is(err, ErrUnknownContentItem) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownContentItem)
	}
}

func TestGameplayContentRejectsRouteDuplicateItem(t *testing.T) {
	bundle := validBundle(t)
	bundle.Route.RouteableItemIDs = append(bundle.Route.RouteableItemIDs, bundle.Route.RouteableItemIDs[0])

	err := bundle.Validate()
	if !errors.Is(err, ErrInvalidRouteContent) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidRouteContent)
	}
}

func TestGameplayContentRejectsInvalidRouteEndpointCapacity(t *testing.T) {
	bundle := validBundle(t)
	bundle.Route.EndpointStorageCapacityUnits = 0

	err := bundle.Validate()
	if !errors.Is(err, ErrInvalidRouteContent) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidRouteContent)
	}
}

func TestGameplayContentRejectsProductionRuleUnknownMaterial(t *testing.T) {
	bundle := validBundle(t)
	bundle.Rules.BuildingCosts[1].Materials[0].ItemID = "missing_item"

	err := bundle.Validate()
	if !errors.Is(err, ErrUnknownContentItem) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownContentItem)
	}
}

func TestGameplayContentRejectsProductionRuleDuplicateCost(t *testing.T) {
	bundle := validBundle(t)
	bundle.Rules.BuildingCosts = append(bundle.Rules.BuildingCosts, bundle.Rules.BuildingCosts[0])

	err := bundle.Validate()
	if !errors.Is(err, ErrInvalidProductionRulesContent) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidProductionRulesContent)
	}
}

func TestGameplayContentRejectsInvalidProductionClaimDefaults(t *testing.T) {
	bundle := validBundle(t)
	bundle.Rules.ClaimStorageCapacityUnits = 0

	err := bundle.Validate()
	if !errors.Is(err, ErrInvalidProductionRulesContent) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidProductionRulesContent)
	}
}

func TestGameplayContentRejectsInvalidCombatRules(t *testing.T) {
	bundle := validBundle(t)
	bundle.Combat.BasicLaserCooldownMS = 0

	err := bundle.Validate()
	if !errors.Is(err, ErrInvalidCombatRulesContent) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidCombatRulesContent)
	}
}

func TestGameplayContentRejectsInvalidCombatDeathZone(t *testing.T) {
	bundle := validBundle(t)
	bundle.Combat.PVPDeathCargoDropByZone[""] = 0.5

	err := bundle.Validate()
	if !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("Validate() error = %v, want %v", err, foundation.ErrEmptyID)
	}
}

func TestGameplayContentRejectsLootRowUnknownItem(t *testing.T) {
	bundle := validBundle(t)
	delete(bundle.Items, "raw_ore")

	err := bundle.Validate()
	if !errors.Is(err, ErrUnknownContentItem) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownContentItem)
	}
}

func TestGameplayContentRejectsMapDropProfileUnknownLootTable(t *testing.T) {
	bundle := validBundle(t)
	definitions := bundle.Maps.Definitions()
	definitions[0].NPCDropProfiles[0].LootTableID = "missing_salvage"
	mapCatalog, err := worldmaps.NewCatalog(definitions, worldmaps.StarterMapID, worldmaps.StarterSpawnID)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v, want nil before cross-catalog validation", err)
	}
	bundle.Maps = mapCatalog

	err = bundle.Validate()
	if !errors.Is(err, ErrUnknownContentLoot) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownContentLoot)
	}
}

func TestGameplayContentRejectsRecipeUnknownInputItem(t *testing.T) {
	bundle := validBundle(t)
	definitions := bundle.Recipes.Definitions()
	definitions[0].Inputs[0].ItemID = "missing_ore"
	recipeCatalog, err := crafting.NewRecipeCatalog(definitions)
	if err != nil {
		t.Fatalf("NewRecipeCatalog() error = %v, want nil before cross-catalog validation", err)
	}
	bundle.Recipes = recipeCatalog

	err = bundle.Validate()
	if !errors.Is(err, ErrUnknownContentItem) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownContentItem)
	}
}

func TestGameplayContentRejectsRecipeUnknownShipUnlock(t *testing.T) {
	bundle := validBundle(t)
	definitions := bundle.Recipes.Definitions()
	for index := range definitions {
		if definitions[index].Output.Kind == crafting.RecipeOutputKindShipUnlock {
			definitions[index].Output.ShipID = "missing_ship"
			break
		}
	}
	recipeCatalog, err := crafting.NewRecipeCatalog(definitions)
	if err != nil {
		t.Fatalf("NewRecipeCatalog() error = %v, want nil before cross-catalog validation", err)
	}
	bundle.Recipes = recipeCatalog

	err = bundle.Validate()
	if !errors.Is(err, ErrUnknownContentShip) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownContentShip)
	}
}

func TestGameplayContentRejectsProductionUnknownOutputItem(t *testing.T) {
	bundle := validBundle(t)
	definitions := bundle.Production.Definitions()
	definitions[0].Outputs[0].ItemID = "missing_output"
	productionCatalog, err := production.NewCatalog(definitions)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v, want nil before cross-catalog validation", err)
	}
	bundle.Production = productionCatalog

	err = bundle.Validate()
	if !errors.Is(err, ErrUnknownContentItem) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownContentItem)
	}
}

func TestGameplayContentRuntimeItemsAndLootTablesAreCloned(t *testing.T) {
	bundle := validBundle(t)

	items, lootTables, err := bundle.RuntimeItemsAndLootTables()
	if err != nil {
		t.Fatalf("RuntimeItemsAndLootTables() error = %v, want nil", err)
	}
	delete(items, "raw_ore")
	table := lootTables[TrainingDroneSalvageLootTableID]
	table.Rows[0].MinQuantity = 99
	lootTables[TrainingDroneSalvageLootTableID] = table

	if _, ok := bundle.Items["raw_ore"]; !ok {
		t.Fatal("mutating returned items changed bundle")
	}
	if got := bundle.LootTables[TrainingDroneSalvageLootTableID].Rows[0].MinQuantity; got != 3 {
		t.Fatalf("bundle loot row min quantity = %d, want 3", got)
	}
}

func validBundle(t *testing.T) GameplayContent {
	t.Helper()
	bundle, err := DefaultGameplayContent(world.WorldID("world-1"))
	if err != nil {
		t.Fatalf("DefaultGameplayContent() error = %v", err)
	}
	return bundle
}

func TestGameplayContentRejectsLootTableSourceMismatch(t *testing.T) {
	bundle := validBundle(t)
	table := bundle.LootTables[TrainingDroneSalvageLootTableID]
	source, err := catalog.NewLootTableSource("other_table", "v1")
	if err != nil {
		t.Fatalf("NewLootTableSource() error = %v", err)
	}
	table.Source = source
	bundle.LootTables[TrainingDroneSalvageLootTableID] = table

	err = bundle.Validate()
	if !errors.Is(err, ErrInvalidContentBundle) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidContentBundle)
	}
}

func TestGameplayContentRejectsInvalidLootChance(t *testing.T) {
	bundle := validBundle(t)
	table := bundle.LootTables[TrainingDroneSalvageLootTableID]
	table.Rows[0].Chance = 1.1
	bundle.LootTables[TrainingDroneSalvageLootTableID] = table

	err := bundle.Validate()
	if !errors.Is(err, ErrInvalidContentLootRow) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidContentLootRow)
	}
}

func TestGameplayContentRejectsModuleWithoutItemDefinition(t *testing.T) {
	bundle := validBundle(t)
	delete(bundle.Items, foundation.ItemID("scanner_t1"))

	err := bundle.Validate()
	if !errors.Is(err, ErrUnknownContentItem) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownContentItem)
	}
}

func TestGameplayContentRejectsScannerOutOfBoundsProfile(t *testing.T) {
	bundle := validBundle(t)
	bundle.Scanner.CandidateOptions.MapBounds.MaxX = 9000

	err := bundle.Validate()
	if !errors.Is(err, discovery.ErrInvalidCandidateOptions) {
		t.Fatalf("Validate() error = %v, want %v", err, discovery.ErrInvalidCandidateOptions)
	}
}

func TestGameplayContentRejectsScannerInvalidDensity(t *testing.T) {
	bundle := validBundle(t)
	bundle.Scanner.CandidateOptions.Density = 1.5

	err := bundle.Validate()
	if !errors.Is(err, discovery.ErrInvalidCandidateOptions) {
		t.Fatalf("Validate() error = %v, want %v", err, discovery.ErrInvalidCandidateOptions)
	}
}

func TestGameplayContentRejectsScannerInvalidMapProfile(t *testing.T) {
	bundle := validBundle(t)
	bundle.Scanner.MapProfiles[0].Density = 1.5

	err := bundle.Validate()
	if !errors.Is(err, discovery.ErrInvalidCandidateOptions) {
		t.Fatalf("Validate() error = %v, want %v", err, discovery.ErrInvalidCandidateOptions)
	}
}

func TestGameplayContentRejectsScannerUnknownMapProfile(t *testing.T) {
	bundle := validBundle(t)
	bundle.Scanner.MapProfiles[0].MapID = "missing_map"

	err := bundle.Validate()
	if !errors.Is(err, ErrInvalidScannerContent) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidScannerContent)
	}
}

func TestGameplayContentRejectsScannerDuplicateMapProfile(t *testing.T) {
	bundle := validBundle(t)
	bundle.Scanner.MapProfiles[1].MapID = bundle.Scanner.MapProfiles[0].MapID

	err := bundle.Validate()
	if !errors.Is(err, ErrInvalidScannerContent) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidScannerContent)
	}
}

func TestGameplayContentRejectsScannerMissingSeed(t *testing.T) {
	bundle := validBundle(t)
	bundle.Scanner.StaticSeed = nil

	err := bundle.Validate()
	if !errors.Is(err, discovery.ErrInvalidWorldSeed) {
		t.Fatalf("Validate() error = %v, want %v", err, discovery.ErrInvalidWorldSeed)
	}
}

func TestGameplayContentRejectsStarterUnknownModuleItem(t *testing.T) {
	bundle := validBundle(t)
	bundle.Starter.ModuleItemIDs[0] = "missing_module"

	err := bundle.Validate()
	if !errors.Is(err, ErrUnknownContentItem) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownContentItem)
	}
}

func TestGameplayContentRejectsStarterDuplicateModuleItem(t *testing.T) {
	bundle := validBundle(t)
	bundle.Starter.ModuleItemIDs[1] = bundle.Starter.ModuleItemIDs[0]

	err := bundle.Validate()
	if !errors.Is(err, ErrInvalidStarterContent) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidStarterContent)
	}
}

func TestGameplayContentRejectsStarterUnknownWorldSeedPool(t *testing.T) {
	bundle := validBundle(t)
	bundle.Starter.WorldSeeds[0].EnemyPoolID = "missing_pool"

	err := bundle.Validate()
	if !errors.Is(err, ErrInvalidStarterContent) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrInvalidStarterContent)
	}
}

func TestGameplayContentRejectsStarterRouteSeedUnknownItem(t *testing.T) {
	bundle := validBundle(t)
	bundle.Starter.RouteSeed.SourceStoredItems[0].ItemID = "missing_item"

	err := bundle.Validate()
	if !errors.Is(err, ErrUnknownContentItem) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrUnknownContentItem)
	}
}

func TestScannerContentE2ENoPlanetOptionsDoNotMutateBundle(t *testing.T) {
	bundle := validBundle(t)

	options := bundle.Scanner.CandidateOptionsForRuntime(true)
	if len(options.AllowedBiomes) != 1 || options.AllowedBiomes[0] != discovery.Biome("e2e_no_planet") {
		t.Fatalf("e2e allowed biomes = %+v, want e2e_no_planet", options.AllowedBiomes)
	}
	if len(bundle.Scanner.CandidateOptions.AllowedBiomes) != 0 {
		t.Fatalf("bundle allowed biomes mutated: %+v", bundle.Scanner.CandidateOptions.AllowedBiomes)
	}
}

func TestScannerContentResolvesMapProfiles(t *testing.T) {
	bundle := validBundle(t)

	options, ok := bundle.Scanner.CandidateOptionsForZone("map_1_3")
	if !ok {
		t.Fatal("CandidateOptionsForZone(map_1_3) ok=false, want true")
	}
	if options.MapID != "map_1_3" || options.ProfileVersion != DefaultScannerProfileVersion {
		t.Fatalf("map_1_3 options = %+v, want map profile", options)
	}
	if options.LevelMin != 1 || options.LevelMax != 4 || options.SpawnBudget != 6 {
		t.Fatalf("map_1_3 profile options = %+v, want 1..4 level band and spawn budget 6", options)
	}

	fallback, ok := bundle.Scanner.CandidateOptionsForZone("map_9_9")
	if ok {
		t.Fatal("CandidateOptionsForZone(map_9_9) ok=true, want fallback false")
	}
	if fallback.MapID != "map_9_9" || fallback.ProfileVersion != DefaultScannerProfileVersion {
		t.Fatalf("fallback options = %+v, want default profile with requested map id", fallback)
	}
}

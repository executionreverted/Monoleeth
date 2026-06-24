package content

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/modules"
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

func TestDefaultGameplayContentStarterBalanceProfileIsCoherent(t *testing.T) {
	bundle := validBundle(t)

	if bundle.Starter.BalanceProfileID != DefaultStarterBalanceProfileID ||
		bundle.Starter.BalanceProfileNote != DefaultStarterBalanceProfileNote {
		t.Fatalf("starter balance profile = %q/%q, want default profile metadata", bundle.Starter.BalanceProfileID, bundle.Starter.BalanceProfileNote)
	}
	if bundle.Starter.ShipDisplayName != "Sparrow" {
		t.Fatalf("starter ship display = %q, want Sparrow", bundle.Starter.ShipDisplayName)
	}
	laser, ok := bundle.Modules.Lookup("laser_alpha_t1")
	if !ok {
		t.Fatal("laser_alpha_t1 missing")
	}
	if laser.Name != "Prism Lance I" ||
		moduleStatValue(t, laser, modules.StatWeaponDamage) != 12 ||
		moduleStatValue(t, laser, modules.StatWeaponRange) != 650 ||
		laser.Energy.ActivationCost != 8 ||
		len(laser.Cooldowns) != 1 ||
		laser.Cooldowns[0].DurationMS != 1200 {
		t.Fatalf("starter laser = %+v, want coherent Prism Lance I baseline", laser)
	}

	starterMap, ok := bundle.Maps.Get(worldmaps.StarterMapID)
	if !ok {
		t.Fatal("starter map missing")
	}
	warden := findContentNPCStatTemplate(t, starterMap, "training_drone")
	if warden.LabelKey != "npc.warden_drone" ||
		warden.HPMax != 34 ||
		warden.ShieldMax != 4 ||
		warden.WeaponDamage != 1 ||
		warden.WeaponRange != 120 {
		t.Fatalf("starter NPC = %+v, want renamed low-risk Warden tuning", warden)
	}
	raiderMap, ok := bundle.Maps.ByPublicKey("1-3")
	if !ok {
		t.Fatal("map 1-3 missing")
	}
	raider := findContentNPCStatTemplate(t, raiderMap, "border_raider_drone")
	if raider.LabelKey != "npc.raider_drone" ||
		raider.HPMax != 72 ||
		raider.ShieldMax != 22 ||
		raider.WeaponDamage != 7 ||
		raider.WeaponCooldown != 1800*time.Millisecond {
		t.Fatalf("raider NPC = %+v, want renamed medium-risk Raider tuning", raider)
	}

	trainingLoot := bundle.LootTables[TrainingDroneSalvageLootTableID]
	if !lootTableHasRow(trainingLoot, "raw_ore", 3, 3, 1) ||
		!lootTableHasRow(trainingLoot, "iron_ore", 2, 4, 0.7) ||
		!lootTableHasRow(trainingLoot, "carbon_shards", 1, 2, 0.35) {
		t.Fatalf("training loot rows = %+v, want smoke drop plus starter craft inputs", trainingLoot.Rows)
	}
	for _, productID := range []catalog.ShopProductID{
		"product_ferrite_ore",
		"product_iron_ore",
		"product_carbon_shards",
		"product_laser_lens",
		"product_energy_cell",
		"product_scanner_circuit",
		"product_warp_coil",
	} {
		if !shopHasProduct(bundle.Shop, productID) {
			t.Fatalf("shop missing %q", productID)
		}
	}
	laserRecipe, ok := bundle.Recipes.Get(crafting.RecipeIDLaserAlphaT1)
	if !ok {
		t.Fatal("laser recipe missing")
	}
	if laserRecipe.RequiredCredits.Int64() != 650 ||
		laserRecipe.CraftDuration != 20*time.Minute ||
		!recipeHasInput(laserRecipe, "refined_alloy", 18) ||
		!recipeHasInput(laserRecipe, "laser_lens", 3) ||
		!recipeHasInput(laserRecipe, "energy_cell", 2) {
		t.Fatalf("laser recipe = %+v, want starter balance inputs/fee/timing", laserRecipe)
	}
}

func TestDefaultGameplayContentRuntimeSeedNamesAvoidOriginalReferenceTerms(t *testing.T) {
	text := strings.ToLower(runtimeSeedSearchText(validBundle(t)))
	for _, forbidden := range forbiddenOriginalReferenceTerms() {
		if strings.Contains(text, forbidden) {
			t.Fatalf("runtime seed contains forbidden reference term %q in %q", forbidden, text)
		}
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

func moduleStatValue(t *testing.T, definition modules.ModuleDefinition, stat modules.StatKey) int64 {
	t.Helper()
	for _, modifier := range definition.StatModifiers {
		if modifier.Stat == stat {
			return modifier.Value
		}
	}
	t.Fatalf("module %q stat %q missing", definition.ItemID, stat)
	return 0
}

func findContentNPCStatTemplate(t *testing.T, definition worldmaps.MapDefinition, npcType string) worldmaps.NPCStatTemplate {
	t.Helper()
	for _, template := range definition.NPCStatTemplates {
		if template.NPCType == npcType {
			return template
		}
	}
	t.Fatalf("npc type %q missing from map %q", npcType, definition.InternalMapID)
	return worldmaps.NPCStatTemplate{}
}

func lootTableHasRow(table loot.LootTable, itemID foundation.ItemID, min int64, max int64, chance float64) bool {
	for _, row := range table.Rows {
		if row.ItemDefinition.ItemID == itemID &&
			row.MinQuantity == min &&
			row.MaxQuantity == max &&
			row.Chance == chance {
			return true
		}
	}
	return false
}

func shopHasProduct(registry catalog.ContentRegistry, productID catalog.ShopProductID) bool {
	for _, product := range registry.ShopProducts {
		if product.ProductID == productID {
			return true
		}
	}
	return false
}

func recipeHasInput(definition crafting.RecipeDefinition, itemID foundation.ItemID, quantity int64) bool {
	for _, input := range definition.Inputs {
		if input.ItemID == itemID && input.Quantity.Int64() == quantity {
			return true
		}
	}
	return false
}

func runtimeSeedSearchText(bundle GameplayContent) string {
	var builder strings.Builder
	builder.WriteString(bundle.Starter.BalanceProfileID)
	builder.WriteString(" ")
	builder.WriteString(bundle.Starter.ShipDisplayName)
	for itemID, definition := range bundle.Items {
		builder.WriteString(" ")
		builder.WriteString(itemID.String())
		builder.WriteString(" ")
		builder.WriteString(definition.Name)
	}
	for tableID, table := range bundle.LootTables {
		builder.WriteString(" ")
		builder.WriteString(tableID)
		for _, row := range table.Rows {
			builder.WriteString(" ")
			builder.WriteString(row.ItemDefinition.ItemID.String())
		}
	}
	for _, definition := range bundle.Modules.Definitions() {
		builder.WriteString(" ")
		builder.WriteString(definition.ItemID.String())
		builder.WriteString(" ")
		builder.WriteString(definition.Name)
	}
	for _, definition := range bundle.Ships.All() {
		builder.WriteString(" ")
		builder.WriteString(definition.ShipID.String())
		builder.WriteString(" ")
		builder.WriteString(definition.Name)
	}
	for _, definition := range bundle.Recipes.Definitions() {
		builder.WriteString(" ")
		builder.WriteString(definition.RecipeID.String())
		for _, input := range definition.Inputs {
			builder.WriteString(" ")
			builder.WriteString(input.ItemID.String())
		}
	}
	for _, product := range bundle.Shop.ShopProducts {
		builder.WriteString(" ")
		builder.WriteString(string(product.ProductID))
		builder.WriteString(" ")
		builder.WriteString(product.Display.DisplayName)
		builder.WriteString(" ")
		builder.WriteString(product.Display.ArtKey)
		builder.WriteString(" ")
		builder.WriteString(product.GrantTarget.RefID)
	}
	if bundle.Maps != nil {
		for _, definition := range bundle.Maps.Definitions() {
			builder.WriteString(" ")
			builder.WriteString(definition.InternalMapID.String())
			builder.WriteString(" ")
			builder.WriteString(definition.DisplayName)
			for _, pool := range definition.EnemyPools {
				builder.WriteString(" ")
				builder.WriteString(pool.EnemyPoolID.String())
				builder.WriteString(" ")
				builder.WriteString(pool.NPCType)
			}
			for _, template := range definition.NPCStatTemplates {
				builder.WriteString(" ")
				builder.WriteString(template.StatTemplateID.String())
				builder.WriteString(" ")
				builder.WriteString(template.NPCType)
				builder.WriteString(" ")
				builder.WriteString(template.LabelKey)
			}
			for _, profile := range definition.NPCDropProfiles {
				builder.WriteString(" ")
				builder.WriteString(profile.DropProfileID.String())
				builder.WriteString(" ")
				builder.WriteString(profile.NPCType)
				builder.WriteString(" ")
				builder.WriteString(profile.LootTableID)
			}
		}
	}
	return builder.String()
}

func forbiddenOriginalReferenceTerms() []string {
	return []string{
		"darkorbit",
		"dark orbit",
		"streuner",
		"lordakia",
		"mordon",
		"saimon",
		"devolarium",
		"sibelon",
		"kristallon",
		"cubikon",
		"protegit",
		"phoenix",
		"yamato",
		"nostromo",
		"leonov",
		"piranha",
		"goliath",
		"vengeance",
		"bigboy",
		"citadel",
		"aegis",
		"iris",
		"flax",
		"lf-1",
		"lf-2",
		"lf-3",
		"lf-4",
		"mp-1",
		"bo-1",
		"bo-2",
		"g3n",
	}
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

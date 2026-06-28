package kalaazu

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/modules"
)

func TestBuildStarterItemRowsMapsKalaazuItems(t *testing.T) {
	dumpRows, err := LoadDumpRows(DefaultSeedFS(), "testdata/items.sql")
	if err != nil {
		t.Fatalf("LoadDumpRows(items) error = %v, want nil", err)
	}
	rows, err := BuildStarterItemRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildStarterItemRows() error = %v, want nil", err)
	}
	if got, want := len(rows), len(dumpRows)+13; got != want {
		t.Fatalf("item rows = %d, want dump row count plus compatibility rows %d", got, want)
	}

	phoenix := requireItemDefinitionForTest(t, rows, "ship_phoenix")
	if phoenix.Name != "Phoenix" || phoenix.Type != economy.ItemTypeInstance || phoenix.MaxStack.Int64() != 1 {
		t.Fatalf("phoenix item = %+v, want instance ship item", phoenix)
	}

	ammo := requireItemDefinitionForTest(t, rows, "ammunition_laser_lcb_10")
	if ammo.Name != "LCB-10" || ammo.Type != economy.ItemTypeStackable || ammo.MaxStack.Int64() != defaultStackMax {
		t.Fatalf("lcb ammo item = %+v, want stackable ammo", ammo)
	}
	starterLaser := requireItemDefinitionForTest(t, rows, "laser_alpha_t1")
	if starterLaser.Name != "LF-1" || starterLaser.Type != economy.ItemTypeInstance || starterLaser.MaxStack.Int64() != 1 {
		t.Fatalf("starter laser item = %+v, want Kalaazu LF-1 instance item projected onto starter contract", starterLaser)
	}
	starterShield := requireItemDefinitionForTest(t, rows, "shield_generator_t1")
	if starterShield.Name != "SG3N-A01" || starterShield.Type != economy.ItemTypeInstance || starterShield.MaxStack.Int64() != 1 {
		t.Fatalf("starter shield item = %+v, want Kalaazu SG3N-A01 instance item projected onto starter contract", starterShield)
	}
	for _, want := range []struct {
		contentID content.ContentID
		name      string
	}{
		{contentID: "prometium", name: "Prometium"},
		{contentID: "raw_ore", name: "Prometium"},
		{contentID: "endurium", name: "Endurium"},
		{contentID: "iron_ore", name: "Endurium"},
		{contentID: "terbium", name: "Terbium"},
		{contentID: "prometid", name: "Prometid"},
		{contentID: "refined_alloy", name: "Prometid"},
		{contentID: "duranium", name: "Duranium"},
		{contentID: "xenomit", name: "Xenomit"},
		{contentID: "carbon_shards", name: "Xenomit"},
		{contentID: "promerium", name: "Promerium"},
	} {
		definition := requireItemDefinitionForTest(t, rows, want.contentID)
		if definition.Name != want.name || definition.Type != economy.ItemTypeStackable {
			t.Fatalf("compatibility item %s = %+v, want Kalaazu %s stackable projection", want.contentID, definition, want.name)
		}
		if err := economy.ValidateMarketListingTradeFlags(definition.TradeFlags); err != nil {
			t.Fatalf("compatibility item %s market flags = %v, want routeable material", want.contentID, definition.TradeFlags)
		}
		if err := economy.ValidateDroppableTradeFlags(definition.TradeFlags); err != nil {
			t.Fatalf("compatibility item %s drop flags = %v, want loot material", want.contentID, definition.TradeFlags)
		}
	}
}

func TestBuildStarterModuleRowsMapsKalaazuLasersShieldsAndSpeedGenerators(t *testing.T) {
	rows, err := BuildStarterModuleRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildStarterModuleRows() error = %v, want nil", err)
	}
	if got, want := len(rows), 18; got != want {
		t.Fatalf("module rows = %d, want %d laser/shield/speed rows plus starter compatibility rows", got, want)
	}

	lf1 := requireModuleDefinitionForTest(t, rows, "equipment_weapon_laser_lf_1")
	if lf1.SlotType != "offensive" || lf1.StatModifiers[0].Stat != "weapon_damage" || lf1.StatModifiers[0].Value != 40 {
		t.Fatalf("lf1 module = %+v, want Kalaazu laser damage module", lf1)
	}
	sg3n := requireModuleDefinitionForTest(t, rows, "equipment_generator_shield_sg3n_a01")
	if sg3n.SlotType != "defensive" || sg3n.StatModifiers[0].Stat != "shield_max" || sg3n.StatModifiers[0].Value != 1000 {
		t.Fatalf("sg3n module = %+v, want Kalaazu shield module", sg3n)
	}
	g3n := requireModuleDefinitionForTest(t, rows, "equipment_generator_speed_g3n_7900")
	if g3n.SlotType != "defensive" || g3n.StatModifiers[0].Stat != modules.StatSpeed || g3n.StatModifiers[0].Value != 10 {
		t.Fatalf("g3n module = %+v, want Kalaazu speed module", g3n)
	}
	starterLaser := requireModuleDefinitionForTest(t, rows, "laser_alpha_t1")
	if starterLaser.Name != "LF-1" ||
		moduleTestStatValue(starterLaser, modules.StatWeaponDamage) != 40 ||
		moduleTestStatValue(starterLaser, modules.StatWeaponRange) != 650 ||
		starterLaser.Energy.ActivationCost != 8 ||
		len(starterLaser.Cooldowns) != 1 ||
		starterLaser.Cooldowns[0].DurationMS != 1200 {
		t.Fatalf("starter compatibility laser = %+v, want Kalaazu LF-1 projected onto starter contract", starterLaser)
	}
	starterShield := requireModuleDefinitionForTest(t, rows, "shield_generator_t1")
	if starterShield.Name != "SG3N-A01" ||
		moduleTestStatValue(starterShield, modules.StatShieldMax) != 1000 ||
		moduleTestStatValue(starterShield, modules.StatShieldRegen) != 4 ||
		starterShield.Energy.Upkeep != 2 {
		t.Fatalf("starter compatibility shield = %+v, want Kalaazu SG3N-A01 projected onto starter contract", starterShield)
	}
}

func TestBuildStarterShopRowsMapsBuyableRows(t *testing.T) {
	rows, err := BuildStarterShopRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildStarterShopRows() error = %v, want nil", err)
	}
	if len(rows) == 0 {
		t.Fatal("shop rows empty, want buyable Kalaazu products")
	}

	shipProduct := requireShopProductForTest(t, rows, "product_ship_goliath")
	if shipProduct.ProductType != "ship" || shipProduct.GrantTarget.Kind != "ship" || shipProduct.GrantTarget.RefID != "ship_goliath" {
		t.Fatalf("goliath product = %+v, want ship product", shipProduct)
	}
	moduleProduct := requireShopProductForTest(t, rows, "product_equipment_weapon_laser_lf_1")
	if moduleProduct.ProductType != "module" || moduleProduct.GrantTarget.Kind != "module" || moduleProduct.GrantTarget.RefID != "equipment_weapon_laser_lf_1" {
		t.Fatalf("lf1 product = %+v, want module product", moduleProduct)
	}
	itemProduct := requireShopProductForTest(t, rows, "product_ammunition_laser_lcb_10")
	if itemProduct.ProductType != "item" || itemProduct.GrantTarget.Kind != "item" || itemProduct.GrantTarget.RefID != "ammunition_laser_lcb_10" {
		t.Fatalf("lcb product = %+v, want item product", itemProduct)
	}
}

func requireItemDefinitionForTest(t *testing.T, rows []content.SnapshotRow, contentID content.ContentID) economy.ItemDefinition {
	t.Helper()
	for _, row := range rows {
		if row.ContentID != contentID {
			continue
		}
		var definition economy.ItemDefinition
		if err := json.Unmarshal(row.DataJSON, &definition); err != nil {
			t.Fatalf("item row %q json error = %v", row.ContentID, err)
		}
		if err := definition.Validate(); err != nil {
			t.Fatalf("item row %q Validate() error = %v", row.ContentID, err)
		}
		return definition
	}
	t.Fatalf("item row %q missing", contentID)
	return economy.ItemDefinition{}
}

func requireModuleDefinitionForTest(t *testing.T, rows []content.SnapshotRow, contentID content.ContentID) modules.ModuleDefinition {
	t.Helper()
	for _, row := range rows {
		if row.ContentID != contentID {
			continue
		}
		var definition modules.ModuleDefinition
		if err := json.Unmarshal(row.DataJSON, &definition); err != nil {
			t.Fatalf("module row %q json error = %v", row.ContentID, err)
		}
		if err := definition.Validate(); err != nil {
			t.Fatalf("module row %q Validate() error = %v", row.ContentID, err)
		}
		return definition
	}
	t.Fatalf("module row %q missing", contentID)
	return modules.ModuleDefinition{}
}

func requireShopProductForTest(t *testing.T, rows []content.SnapshotRow, contentID content.ContentID) catalog.ShopProductDefinition {
	t.Helper()
	for _, row := range rows {
		if row.ContentID != contentID {
			continue
		}
		var product catalog.ShopProductDefinition
		if err := json.Unmarshal(row.DataJSON, &product); err != nil {
			t.Fatalf("shop row %q json error = %v", row.ContentID, err)
		}
		return product
	}
	t.Fatalf("shop row %q missing", contentID)
	return catalog.ShopProductDefinition{}
}

func moduleTestStatValue(definition modules.ModuleDefinition, stat modules.StatKey) int64 {
	for _, modifier := range definition.StatModifiers {
		if modifier.Stat == stat && modifier.Kind == modules.StatModifierFlat {
			return modifier.Value
		}
	}
	return 0
}

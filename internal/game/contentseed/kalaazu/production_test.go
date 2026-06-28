package kalaazu

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/catalog"
	"gameproject/internal/game/content"
	"gameproject/internal/game/production"
)

func TestBuildDefaultRowsMapsProductionBuildings(t *testing.T) {
	rows, err := BuildDefaultRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildDefaultRows() error = %v, want nil", err)
	}
	if len(rows.ProductionBuildingRows) != 3 {
		t.Fatalf("production building rows = %d, want 3", len(rows.ProductionBuildingRows))
	}

	extractor := requireProductionBuildingForTest(t, rows.ProductionBuildingRows, production.ProductionDefinitionIDIronExtractorL1)
	if extractor.BuildingType != production.BuildingTypeIronExtractor ||
		extractor.Category != production.BuildingCategoryExtractor ||
		extractor.Level != 1 ||
		len(extractor.Inputs) != 0 ||
		!productionDefinitionHasOutput(extractor, "iron_ore", 30) ||
		extractor.EnergyCostPerHour != 4 {
		t.Fatalf("iron extractor L1 = %+v, want default extractor over Kalaazu-projected ore", extractor)
	}

	foundry := requireProductionBuildingForTest(t, rows.ProductionBuildingRows, production.ProductionDefinitionIDAlloyFoundryL1)
	if foundry.BuildingType != production.BuildingTypeAlloyFoundry ||
		foundry.Category != production.BuildingCategoryRefinery ||
		!productionDefinitionHasInput(foundry, "iron_ore", 30) ||
		!productionDefinitionHasOutput(foundry, "refined_alloy", 10) ||
		foundry.EnergyCostPerHour != 5 {
		t.Fatalf("alloy foundry L1 = %+v, want default refinery over Kalaazu-projected resources", foundry)
	}

	extractorL2 := requireProductionBuildingForTest(t, rows.ProductionBuildingRows, production.ProductionDefinitionIDIronExtractorL2)
	if extractorL2.Level != 2 ||
		!productionDefinitionHasOutput(extractorL2, "iron_ore", 60) ||
		extractorL2.EnergyCostPerHour != 8 {
		t.Fatalf("iron extractor L2 = %+v, want default extractor upgrade", extractorL2)
	}
}

func TestBuildDefaultRowsMapsProductionRules(t *testing.T) {
	rows, err := BuildDefaultRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildDefaultRows() error = %v, want nil", err)
	}
	if len(rows.ProductionRuleRows) != 1 {
		t.Fatalf("production rule rows = %d, want 1", len(rows.ProductionRuleRows))
	}
	var rules content.ProductionRulesContent
	if err := json.Unmarshal(rows.ProductionRuleRows[0].DataJSON, &rules); err != nil {
		t.Fatalf("production rules json error = %v", err)
	}
	if rows.ProductionRuleRows[0].ContentID != "production_rules" ||
		rules.ClaimRange <= 0 ||
		rules.ClaimStorageCapacityUnits <= 0 ||
		rules.ClaimEnergyCapacityPerHour <= 0 ||
		!productionRulesHasCost(rules, production.BuildingMutationBuild, production.BuildingTypeIronExtractor, 1, 50) ||
		!productionRulesHasCost(rules, production.BuildingMutationBuild, production.BuildingTypeAlloyFoundry, 1, 75) ||
		!productionRulesHasCost(rules, production.BuildingMutationUpgrade, production.BuildingTypeIronExtractor, 2, 100) {
		t.Fatalf("production rules = %+v, want default rules over Kalaazu/default production rows", rules)
	}
}

func requireProductionBuildingForTest(t *testing.T, rows []content.SnapshotRow, definitionID catalog.DefinitionID) production.BuildingProductionDefinition {
	t.Helper()
	for _, row := range rows {
		if row.ContentID != content.ContentID(definitionID.String()) {
			continue
		}
		var definition production.BuildingProductionDefinition
		if err := json.Unmarshal(row.DataJSON, &definition); err != nil {
			t.Fatalf("production row %q json error = %v", row.ContentID, err)
		}
		if err := definition.Validate(); err != nil {
			t.Fatalf("production row %q Validate() error = %v", row.ContentID, err)
		}
		if definition.Source.DefinitionID != definition.DefinitionID {
			t.Fatalf("production row %q source id = %q, want definition id", row.ContentID, definition.Source.DefinitionID)
		}
		return definition
	}
	t.Fatalf("production row %q missing", definitionID)
	return production.BuildingProductionDefinition{}
}

func productionRulesHasCost(rules content.ProductionRulesContent, operation production.BuildingMutationKind, buildingType production.BuildingType, level int, credits int64) bool {
	for _, cost := range rules.BuildingCosts {
		if cost.Operation == operation && cost.BuildingType == buildingType && cost.Level == level && cost.Credits == credits {
			return true
		}
	}
	return false
}

func productionDefinitionHasInput(definition production.BuildingProductionDefinition, itemID content.ContentID, amountPerHour int64) bool {
	return productionDefinitionHasRate(definition.Inputs, itemID, amountPerHour)
}

func productionDefinitionHasOutput(definition production.BuildingProductionDefinition, itemID content.ContentID, amountPerHour int64) bool {
	return productionDefinitionHasRate(definition.Outputs, itemID, amountPerHour)
}

func productionDefinitionHasRate(rates []production.ItemRate, itemID content.ContentID, amountPerHour int64) bool {
	for _, rate := range rates {
		if content.ContentID(rate.ItemID.String()) == itemID && rate.AmountPerHour == amountPerHour {
			return true
		}
	}
	return false
}

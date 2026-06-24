package content

import (
	"fmt"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/production"
)

const (
	DefaultPlanetClaimRange               = 300.0
	DefaultClaimProductionStorageCapacity = 250
	DefaultClaimProductionEnergyCapacity  = 40

	DefaultBuildingBuildIronExtractorCredits   int64 = 50
	DefaultBuildingBuildAlloyFoundryIronOre    int64 = 30
	DefaultBuildingBuildAlloyFoundryCredits    int64 = 75
	DefaultBuildingUpgradeIronExtractorIronOre int64 = 20
	DefaultBuildingUpgradeIronExtractorCredits int64 = 100
)

type ProductionRulesContent struct {
	ClaimRange                 float64
	ClaimStorageCapacityUnits  int64
	ClaimEnergyCapacityPerHour int64
	BuildingCosts              []BuildingCostContent
}

type BuildingCostContent struct {
	Operation    production.BuildingMutationKind
	BuildingType production.BuildingType
	Level        int
	Materials    []production.BuildingMaterialCost
	Credits      int64
}

func DefaultProductionRulesContent() ProductionRulesContent {
	return ProductionRulesContent{
		ClaimRange:                 DefaultPlanetClaimRange,
		ClaimStorageCapacityUnits:  DefaultClaimProductionStorageCapacity,
		ClaimEnergyCapacityPerHour: DefaultClaimProductionEnergyCapacity,
		BuildingCosts: []BuildingCostContent{
			{
				Operation:    production.BuildingMutationBuild,
				BuildingType: production.BuildingTypeIronExtractor,
				Level:        1,
				Credits:      DefaultBuildingBuildIronExtractorCredits,
			},
			{
				Operation:    production.BuildingMutationBuild,
				BuildingType: production.BuildingTypeAlloyFoundry,
				Level:        1,
				Materials: []production.BuildingMaterialCost{{
					ItemID:   "iron_ore",
					Quantity: DefaultBuildingBuildAlloyFoundryIronOre,
				}},
				Credits: DefaultBuildingBuildAlloyFoundryCredits,
			},
			{
				Operation:    production.BuildingMutationUpgrade,
				BuildingType: production.BuildingTypeIronExtractor,
				Level:        2,
				Materials: []production.BuildingMaterialCost{{
					ItemID:   "iron_ore",
					Quantity: DefaultBuildingUpgradeIronExtractorIronOre,
				}},
				Credits: DefaultBuildingUpgradeIronExtractorCredits,
			},
		},
	}
}

func (content ProductionRulesContent) Validate(bundle GameplayContent) error {
	if content.ClaimRange <= 0 ||
		content.ClaimStorageCapacityUnits <= 0 ||
		content.ClaimEnergyCapacityPerHour <= 0 {
		return fmt.Errorf("claim production rules: %w", ErrInvalidProductionRulesContent)
	}
	if len(content.BuildingCosts) == 0 {
		return fmt.Errorf("building costs: %w", ErrInvalidProductionRulesContent)
	}
	seen := make(map[string]struct{}, len(content.BuildingCosts))
	for _, cost := range content.BuildingCosts {
		key, err := cost.validationKey()
		if err != nil {
			return err
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("building cost %s duplicate: %w", key, ErrInvalidProductionRulesContent)
		}
		seen[key] = struct{}{}
		for _, material := range cost.Materials {
			if material.Quantity <= 0 {
				return fmt.Errorf("building cost %s material %q quantity=%d: %w", key, material.ItemID, material.Quantity, ErrInvalidProductionRulesContent)
			}
			if err := validateKnownItem(bundle, "building cost", material.ItemID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (content ProductionRulesContent) BuildingMutationCost(
	playerID foundation.PlayerID,
	input production.BuildingMutationCostInput,
) (production.BuildingMutationCost, bool) {
	for _, cost := range content.BuildingCosts {
		if cost.Operation != input.Operation ||
			cost.BuildingType != input.Definition.BuildingType ||
			cost.Level != input.Definition.Level {
			continue
		}
		result := production.BuildingMutationCost{
			Materials: append([]production.BuildingMaterialCost(nil), cost.Materials...),
		}
		if cost.Credits > 0 {
			result.Wallet = &production.BuildingWalletCost{
				PlayerID: playerID,
				Currency: economy.CurrencyBucketCredits,
				Amount:   cost.Credits,
			}
		}
		return result, true
	}
	return production.BuildingMutationCost{}, false
}

func (cost BuildingCostContent) validationKey() (string, error) {
	switch cost.Operation {
	case production.BuildingMutationBuild, production.BuildingMutationUpgrade:
	default:
		return "", fmt.Errorf("building cost operation %q: %w", cost.Operation, ErrInvalidProductionRulesContent)
	}
	if err := cost.BuildingType.Validate(); err != nil {
		return "", err
	}
	if cost.Level <= 0 || cost.Credits < 0 {
		return "", fmt.Errorf("building cost level=%d credits=%d: %w", cost.Level, cost.Credits, ErrInvalidProductionRulesContent)
	}
	return fmt.Sprintf("%s:%s:%d", cost.Operation, cost.BuildingType, cost.Level), nil
}

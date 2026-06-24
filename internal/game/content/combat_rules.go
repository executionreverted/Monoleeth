package content

import (
	"fmt"
	"math"

	"gameproject/internal/game/combat"
	deathdomain "gameproject/internal/game/death"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	worldmaps "gameproject/internal/game/world/maps"
)

const (
	DefaultPlayerSpeed             = 180
	DefaultRadarRange              = 420
	DefaultLootPickupRange         = 120.0
	DefaultBasicLaserEnergyCost    = 10
	DefaultBasicLaserCooldownMS    = 350
	DefaultBasicLaserSkillID       = "basic_laser"
	DefaultTrainingNPCType         = "training_drone"
	DefaultRepairCurrency          = economy.CurrencyBucketCredits
	DefaultNonStarterShipRepairFee = 25
	DefaultNPCKillMainXP           = 20
	DefaultNPCKillCombatXP         = 20
	DefaultPVPDeathCargoDrop       = 0.50
	DefaultSeededPVPDeathCargoDrop = 1.00
)

type CombatRulesContent struct {
	PlayerSpeed              float64
	RadarRange               float64
	LootPickupRange          float64
	BasicLaserSkillID        string
	BasicLaserEnergyCost     int
	BasicLaserCooldownMS     int
	TrainingNPCType          string
	RepairCurrency           economy.CurrencyBucket
	NonStarterShipRepairFee  int64
	NPCKillMainXP            int64
	NPCKillCombatXP          int64
	PVPDeathCargoDropPercent float64
	PVPDeathCargoDropByZone  map[foundation.ZoneID]float64
}

func DefaultCombatRulesContent() CombatRulesContent {
	return CombatRulesContent{
		PlayerSpeed:              DefaultPlayerSpeed,
		RadarRange:               DefaultRadarRange,
		LootPickupRange:          DefaultLootPickupRange,
		BasicLaserSkillID:        DefaultBasicLaserSkillID,
		BasicLaserEnergyCost:     DefaultBasicLaserEnergyCost,
		BasicLaserCooldownMS:     DefaultBasicLaserCooldownMS,
		TrainingNPCType:          DefaultTrainingNPCType,
		RepairCurrency:           DefaultRepairCurrency,
		NonStarterShipRepairFee:  DefaultNonStarterShipRepairFee,
		NPCKillMainXP:            DefaultNPCKillMainXP,
		NPCKillCombatXP:          DefaultNPCKillCombatXP,
		PVPDeathCargoDropPercent: DefaultPVPDeathCargoDrop,
		PVPDeathCargoDropByZone: map[foundation.ZoneID]float64{
			worldmaps.MapID("map_1_3").ZoneID(): DefaultSeededPVPDeathCargoDrop,
		},
	}
}

func (content CombatRulesContent) Validate() error {
	if content.PlayerSpeed <= 0 ||
		content.RadarRange <= 0 ||
		content.LootPickupRange <= 0 ||
		content.BasicLaserEnergyCost <= 0 ||
		content.BasicLaserCooldownMS <= 0 ||
		content.NonStarterShipRepairFee < 0 ||
		content.NPCKillMainXP <= 0 ||
		content.NPCKillCombatXP <= 0 {
		return fmt.Errorf("numeric combat rule: %w", ErrInvalidCombatRulesContent)
	}
	if content.BasicLaserSkillID == "" || content.TrainingNPCType == "" {
		return fmt.Errorf("combat ids: %w", ErrInvalidCombatRulesContent)
	}
	if err := content.RepairCurrency.Validate(); err != nil {
		return fmt.Errorf("repair currency: %w", err)
	}
	if !validPercent(content.PVPDeathCargoDropPercent) {
		return fmt.Errorf("pvp death cargo drop %.4f: %w", content.PVPDeathCargoDropPercent, ErrInvalidCombatRulesContent)
	}
	for zoneID, percent := range content.PVPDeathCargoDropByZone {
		if err := zoneID.Validate(); err != nil {
			return fmt.Errorf("pvp death cargo zone: %w", err)
		}
		if !validPercent(percent) {
			return fmt.Errorf("pvp death cargo zone %q %.4f: %w", zoneID, percent, ErrInvalidCombatRulesContent)
		}
	}
	return nil
}

func (content CombatRulesContent) NPCKillXPReward() combat.NPCKillXPReward {
	return combat.NPCKillXPReward{
		MainXP: content.NPCKillMainXP,
		Role:   progression.RoleTypeCombat,
		RoleXP: content.NPCKillCombatXP,
	}
}

func (content CombatRulesContent) PVPDeathCargoDropPolicy(zoneID foundation.ZoneID) (deathdomain.ZoneCargoDropPolicy, error) {
	percent := content.PVPDeathCargoDropPercent
	if override, ok := content.PVPDeathCargoDropByZone[zoneID]; ok {
		percent = override
	}
	return deathdomain.NewZoneCargoDropPolicy(zoneID, percent, percent)
}

func validPercent(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

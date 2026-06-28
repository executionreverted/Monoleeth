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
	Ammo                     []CombatAmmoDefinition
}

type CombatAmmoFamily string

const (
	CombatAmmoFamilyLaser          CombatAmmoFamily = "laser"
	CombatAmmoFamilyRocket         CombatAmmoFamily = "rocket"
	CombatAmmoFamilyRocketLauncher CombatAmmoFamily = "rocket_launcher"
	CombatAmmoFamilyMine           CombatAmmoFamily = "mine"
)

type CombatAmmoDefinition struct {
	ItemID                foundation.ItemID
	Family                CombatAmmoFamily
	AmmoKey               string
	DamageMultiplier      float64
	FlatDamage            int64
	ShieldLeechMultiplier float64
	CooldownMS            int
	AccuracyModifier      int
	FallbackRank          int
	Selectable            bool
	Buyable               bool
	SlotbarOrder          int
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
		Ammo: DefaultCombatAmmoDefinitions(),
	}
}

func DefaultCombatAmmoDefinitions() []CombatAmmoDefinition {
	return []CombatAmmoDefinition{
		{ItemID: "ammunition_laser_lcb_10", Family: CombatAmmoFamilyLaser, AmmoKey: "lcb_10", DamageMultiplier: 1, FallbackRank: 1, Selectable: true, Buyable: true, SlotbarOrder: 1},
		{ItemID: "ammunition_laser_mcb_25", Family: CombatAmmoFamilyLaser, AmmoKey: "mcb_25", DamageMultiplier: 2, FallbackRank: 2, Selectable: true, Buyable: true, SlotbarOrder: 2},
		{ItemID: "ammunition_laser_mcb_50", Family: CombatAmmoFamilyLaser, AmmoKey: "mcb_50", DamageMultiplier: 3, FallbackRank: 3, Selectable: true, Buyable: true, SlotbarOrder: 3},
		{ItemID: "ammunition_laser_sab_50", Family: CombatAmmoFamilyLaser, AmmoKey: "sab_50", DamageMultiplier: 2, ShieldLeechMultiplier: 2, FallbackRank: 4, Selectable: true, Buyable: true, SlotbarOrder: 4},
		{ItemID: "ammunition_laser_ucb_100", Family: CombatAmmoFamilyLaser, AmmoKey: "ucb_100", DamageMultiplier: 4, FallbackRank: 5, Selectable: true, Buyable: true, SlotbarOrder: 5},
		{ItemID: "ammunition_laser_rsb_75", Family: CombatAmmoFamilyLaser, AmmoKey: "rsb_75", DamageMultiplier: 6, CooldownMS: 3000, FallbackRank: 6, Selectable: true, Buyable: true, SlotbarOrder: 6},
		{ItemID: "ammunition_rocket_r_310", Family: CombatAmmoFamilyRocket, AmmoKey: "r_310", FlatDamage: 1000, CooldownMS: 1000, Selectable: true, Buyable: true, SlotbarOrder: 0},
		{ItemID: "ammunition_rocket_plt_2026", Family: CombatAmmoFamilyRocket, AmmoKey: "plt_2026", FlatDamage: 2000, CooldownMS: 1000, Selectable: true, Buyable: true, SlotbarOrder: 2},
		{ItemID: "ammunition_rocket_plt_2021", Family: CombatAmmoFamilyRocket, AmmoKey: "plt_2021", FlatDamage: 4000, CooldownMS: 1000, Selectable: true, Buyable: true, SlotbarOrder: 1},
		{ItemID: "ammunition_rocket_plt_3030", Family: CombatAmmoFamilyRocket, AmmoKey: "plt_3030", FlatDamage: 6000, CooldownMS: 1000, AccuracyModifier: -1500, Selectable: true, Buyable: true, SlotbarOrder: 3},
		{ItemID: "ammunition_rocketlauncher_eco_10", Family: CombatAmmoFamilyRocketLauncher, AmmoKey: "eco_10", FlatDamage: 2000, CooldownMS: 1000, Selectable: true, Buyable: true, SlotbarOrder: 0},
		{ItemID: "ammunition_rocketlauncher_hstrm_01", Family: CombatAmmoFamilyRocketLauncher, AmmoKey: "hstrm_01", FlatDamage: 4000, CooldownMS: 1000, Selectable: true, Buyable: true, SlotbarOrder: 1},
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
	if err := validateCombatAmmoDefinitions(content.Ammo); err != nil {
		return err
	}
	return nil
}

func validateCombatAmmoDefinitions(definitions []CombatAmmoDefinition) error {
	if len(definitions) == 0 {
		return fmt.Errorf("combat ammo empty: %w", ErrInvalidCombatRulesContent)
	}
	seen := make(map[foundation.ItemID]struct{}, len(definitions))
	for _, definition := range definitions {
		if err := definition.ItemID.Validate(); err != nil {
			return fmt.Errorf("combat ammo item: %w", err)
		}
		if definition.AmmoKey == "" {
			return fmt.Errorf("combat ammo %q key: %w", definition.ItemID, ErrInvalidCombatRulesContent)
		}
		switch definition.Family {
		case CombatAmmoFamilyLaser:
			if definition.DamageMultiplier <= 0 && definition.ShieldLeechMultiplier <= 0 {
				return fmt.Errorf("laser ammo %q effect: %w", definition.ItemID, ErrInvalidCombatRulesContent)
			}
		case CombatAmmoFamilyRocket, CombatAmmoFamilyRocketLauncher, CombatAmmoFamilyMine:
			if definition.FlatDamage < 0 {
				return fmt.Errorf("ammo %q flat damage: %w", definition.ItemID, ErrInvalidCombatRulesContent)
			}
		default:
			return fmt.Errorf("combat ammo %q family %q: %w", definition.ItemID, definition.Family, ErrInvalidCombatRulesContent)
		}
		if definition.CooldownMS < 0 || definition.SlotbarOrder < 0 || definition.FallbackRank < 0 {
			return fmt.Errorf("combat ammo %q numeric rule: %w", definition.ItemID, ErrInvalidCombatRulesContent)
		}
		if _, ok := seen[definition.ItemID]; ok {
			return fmt.Errorf("combat ammo %q duplicate: %w", definition.ItemID, ErrInvalidCombatRulesContent)
		}
		seen[definition.ItemID] = struct{}{}
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

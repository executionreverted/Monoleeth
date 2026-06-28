package kalaazu

import (
	"fmt"
	"strings"

	"gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
)

func mapCombatRuleRows(sourceItemRows []DumpRow, itemRows []content.SnapshotRow, moduleRows []content.SnapshotRow, npcRows NPCRowsResult) ([]content.SnapshotRow, error) {
	for _, contentID := range []content.ContentID{
		"laser_alpha_t1",
		"shield_generator_t1",
	} {
		if !snapshotRowsContain(itemRows, contentID) {
			return nil, fmt.Errorf("combat rule item source %q missing", contentID)
		}
	}
	for _, contentID := range []content.ContentID{
		"laser_alpha_t1",
		"shield_generator_t1",
	} {
		if !snapshotRowsContain(moduleRows, contentID) {
			return nil, fmt.Errorf("combat rule module source %q missing", contentID)
		}
	}
	if len(npcRows.NPCTemplates) == 0 || len(npcRows.EnemyPools) == 0 {
		return nil, fmt.Errorf("combat rule npc source rows missing")
	}
	rules := content.DefaultCombatRulesContent()
	ammo, err := combatAmmoDefinitionsFromSources(sourceItemRows)
	if err != nil {
		return nil, err
	}
	rules.Ammo = ammo
	row, err := snapshotRow("combat_rules", rules)
	if err != nil {
		return nil, err
	}
	return []content.SnapshotRow{row}, nil
}

func combatAmmoDefinitionsFromSources(itemRows []DumpRow) ([]content.CombatAmmoDefinition, error) {
	definitions := make([]content.CombatAmmoDefinition, 0)
	for _, row := range itemRows {
		source, err := decodeKalaazuItem(row)
		if err != nil {
			return nil, err
		}
		definition, ok := combatAmmoDefinition(source)
		if !ok {
			continue
		}
		definitions = append(definitions, definition)
	}
	if len(definitions) == 0 {
		return nil, fmt.Errorf("combat ammo source rows missing: %w", ErrMalformedDumpSQL)
	}
	return definitions, nil
}

func combatAmmoDefinition(source kalaazuItemSource) (content.CombatAmmoDefinition, bool) {
	lootID := strings.ToLower(source.LootID)
	itemID := foundation.ItemID(source.LootID)
	base := content.CombatAmmoDefinition{
		ItemID:       itemID,
		AmmoKey:      ammoKey(lootID),
		CooldownMS:   sourceCooldownMS(source),
		Selectable:   true,
		Buyable:      source.IsBuyable,
		SlotbarOrder: maxInt(0, source.SlotbarOrder),
	}
	switch {
	case strings.HasPrefix(lootID, "ammunition_laser_"):
		base.Family = content.CombatAmmoFamilyLaser
		base.FallbackRank = maxInt(0, source.SlotbarOrder)
		base.DamageMultiplier = laserAmmoMultiplier(source)
		base.ShieldLeechMultiplier = laserAmmoShieldLeech(source)
	case strings.HasPrefix(lootID, "ammunition_rocketlauncher_"):
		base.Family = content.CombatAmmoFamilyRocketLauncher
		base.FlatDamage = rocketLauncherAmmoDamage(source)
	case strings.HasPrefix(lootID, "ammunition_rocket_"):
		base.Family = content.CombatAmmoFamilyRocket
		base.FlatDamage = rocketAmmoDamage(source)
		base.AccuracyModifier = rocketAmmoAccuracyModifier(source)
	default:
		return content.CombatAmmoDefinition{}, false
	}
	return base, true
}

func ammoKey(lootID string) string {
	const laserPrefix = "ammunition_laser_"
	const rocketPrefix = "ammunition_rocket_"
	const launcherPrefix = "ammunition_rocketlauncher_"
	switch {
	case strings.HasPrefix(lootID, laserPrefix):
		return strings.TrimPrefix(lootID, laserPrefix)
	case strings.HasPrefix(lootID, launcherPrefix):
		return strings.TrimPrefix(lootID, launcherPrefix)
	case strings.HasPrefix(lootID, rocketPrefix):
		return strings.TrimPrefix(lootID, rocketPrefix)
	default:
		return lootID
	}
}

func sourceCooldownMS(source kalaazuItemSource) int {
	switch {
	case strings.Contains(source.LootID, "rsb_75"):
		return 3000
	case strings.Contains(source.LootID, "rcb_140"):
		return 3000
	default:
		return 1000
	}
}

func laserAmmoMultiplier(source kalaazuItemSource) float64 {
	switch source.LootID {
	case "ammunition_laser_lcb_10":
		return 1
	case "ammunition_laser_mcb_25":
		return 2
	case "ammunition_laser_mcb_50":
		return 3
	case "ammunition_laser_ucb_100":
		return 4
	case "ammunition_laser_sab_50":
		return 2
	case "ammunition_laser_rsb_75":
		return 6
	case "ammunition_laser_cbo_100":
		return 3
	default:
		if source.Bonus > 0 {
			return float64(source.Bonus)
		}
		return 1
	}
}

func laserAmmoShieldLeech(source kalaazuItemSource) float64 {
	switch source.LootID {
	case "ammunition_laser_sab_50":
		return 2
	case "ammunition_laser_cbo_100":
		return 1
	default:
		return 0
	}
}

func rocketAmmoDamage(source kalaazuItemSource) int64 {
	switch source.LootID {
	case "ammunition_rocket_r_310":
		return 1000
	case "ammunition_rocket_plt_2026":
		return 2000
	case "ammunition_rocket_plt_2021":
		return 4000
	case "ammunition_rocket_plt_3030":
		return 6000
	case "ammunition_rocket_bdr_1211":
		return 7500
	default:
		return int64(maxInt(0, source.Bonus))
	}
}

func rocketAmmoAccuracyModifier(source kalaazuItemSource) int {
	if source.LootID == "ammunition_rocket_plt_3030" {
		return -1500
	}
	return 0
}

func rocketLauncherAmmoDamage(source kalaazuItemSource) int64 {
	switch source.LootID {
	case "ammunition_rocketlauncher_eco_10":
		return 2000
	case "ammunition_rocketlauncher_hstrm_01":
		return 4000
	case "ammunition_rocketlauncher_sar_01":
		return 2500
	case "ammunition_rocketlauncher_sar_02":
		return 3000
	case "ammunition_rocketlauncher_ubr_100":
		return 7500
	case "ammunition_rocketlauncher_cbr":
		return 3000
	default:
		return int64(maxInt(0, source.Bonus))
	}
}

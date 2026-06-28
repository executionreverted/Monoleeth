package kalaazu

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/content"
)

func TestBuildDefaultRowsMapsCombatRules(t *testing.T) {
	rows, err := BuildDefaultRows(DefaultSeedFS())
	if err != nil {
		t.Fatalf("BuildDefaultRows() error = %v, want nil", err)
	}
	if len(rows.CombatRuleRows) != 1 {
		t.Fatalf("combat rule rows = %d, want 1", len(rows.CombatRuleRows))
	}
	if rows.CombatRuleRows[0].ContentID != "combat_rules" {
		t.Fatalf("combat rules content id = %q, want combat_rules", rows.CombatRuleRows[0].ContentID)
	}
	var rules content.CombatRulesContent
	if err := json.Unmarshal(rows.CombatRuleRows[0].DataJSON, &rules); err != nil {
		t.Fatalf("combat rules json error = %v", err)
	}
	if err := rules.Validate(); err != nil {
		t.Fatalf("combat rules Validate() error = %v, want nil", err)
	}
	if rules.BasicLaserSkillID != "basic_laser" ||
		rules.BasicLaserEnergyCost != content.DefaultBasicLaserEnergyCost ||
		rules.BasicLaserCooldownMS != content.DefaultBasicLaserCooldownMS ||
		rules.TrainingNPCType != content.DefaultTrainingNPCType ||
		rules.LootPickupRange != content.DefaultLootPickupRange ||
		rules.NPCKillMainXP != content.DefaultNPCKillMainXP ||
		rules.NPCKillCombatXP != content.DefaultNPCKillCombatXP {
		t.Fatalf("combat rules = %+v, want default combat rules over Kalaazu/default combat rows", rules)
	}
	for _, want := range []struct {
		itemID     string
		family     content.CombatAmmoFamily
		multiplier float64
	}{
		{itemID: "ammunition_laser_lcb_10", family: content.CombatAmmoFamilyLaser, multiplier: 1},
		{itemID: "ammunition_laser_mcb_25", family: content.CombatAmmoFamilyLaser, multiplier: 2},
		{itemID: "ammunition_laser_mcb_50", family: content.CombatAmmoFamilyLaser, multiplier: 3},
		{itemID: "ammunition_laser_ucb_100", family: content.CombatAmmoFamilyLaser, multiplier: 4},
		{itemID: "ammunition_laser_sab_50", family: content.CombatAmmoFamilyLaser, multiplier: 2},
		{itemID: "ammunition_laser_rsb_75", family: content.CombatAmmoFamilyLaser, multiplier: 6},
		{itemID: "ammunition_rocket_plt_2021", family: content.CombatAmmoFamilyRocket, multiplier: 0},
		{itemID: "ammunition_rocketlauncher_hstrm_01", family: content.CombatAmmoFamilyRocketLauncher, multiplier: 0},
	} {
		ammo, ok := combatAmmoForTest(rules.Ammo, want.itemID)
		if !ok {
			t.Fatalf("ammo %s missing in combat rules", want.itemID)
		}
		if ammo.Family != want.family || ammo.DamageMultiplier != want.multiplier || !ammo.Selectable {
			t.Fatalf("ammo %s = %+v, want family %s multiplier %.1f selectable", want.itemID, ammo, want.family, want.multiplier)
		}
	}
}

func combatAmmoForTest(definitions []content.CombatAmmoDefinition, itemID string) (content.CombatAmmoDefinition, bool) {
	for _, definition := range definitions {
		if definition.ItemID.String() == itemID {
			return definition, true
		}
	}
	return content.CombatAmmoDefinition{}, false
}

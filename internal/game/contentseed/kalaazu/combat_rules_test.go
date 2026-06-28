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
}

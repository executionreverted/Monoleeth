package kalaazu

import (
	"fmt"

	"gameproject/internal/game/content"
)

func mapCombatRuleRows(itemRows []content.SnapshotRow, moduleRows []content.SnapshotRow, npcRows NPCRowsResult) ([]content.SnapshotRow, error) {
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
	row, err := snapshotRow("combat_rules", rules)
	if err != nil {
		return nil, err
	}
	return []content.SnapshotRow{row}, nil
}

package contentdb

import (
	"fmt"

	"gameproject/internal/game/content"
	worldmaps "gameproject/internal/game/world/maps"
)

const (
	serverRuleScannerConfigID   content.ContentID = "scanner_config"
	serverRuleStarterConfigID   content.ContentID = "starter_config"
	serverRuleRoutePolicyID     content.ContentID = "route_policy"
	serverRuleProductionRulesID content.ContentID = "production_rules"
	serverRuleCombatRulesID     content.ContentID = "combat_rules"
)

func mapScannerConfigRows(snapshot content.Snapshot, mapCatalog *worldmaps.Catalog) (content.ScannerContent, error) {
	var out content.ScannerContent
	if err := decodeSingletonRuleRow(content.ContentTypeScannerConfig, snapshot.ScannerConfigs, serverRuleScannerConfigID, &out); err != nil {
		return content.ScannerContent{}, err
	}
	if err := out.Validate(mapCatalog); err != nil {
		return content.ScannerContent{}, err
	}
	return out, nil
}

func mapStarterConfigRows(snapshot content.Snapshot, bundle content.GameplayContent) (content.StarterContent, error) {
	var out content.StarterContent
	if err := decodeSingletonRuleRow(content.ContentTypeStarterConfig, snapshot.StarterConfigs, serverRuleStarterConfigID, &out); err != nil {
		return content.StarterContent{}, err
	}
	if err := out.Validate(bundle); err != nil {
		return content.StarterContent{}, err
	}
	return out, nil
}

func mapRoutePolicyRows(snapshot content.Snapshot, bundle content.GameplayContent) (content.RouteContent, error) {
	var out content.RouteContent
	if err := decodeSingletonRuleRow(content.ContentTypeRoutePolicy, snapshot.RoutePolicies, serverRuleRoutePolicyID, &out); err != nil {
		return content.RouteContent{}, err
	}
	if err := out.Validate(bundle); err != nil {
		return content.RouteContent{}, err
	}
	return out, nil
}

func mapProductionRuleRows(snapshot content.Snapshot, bundle content.GameplayContent) (content.ProductionRulesContent, error) {
	var out content.ProductionRulesContent
	if err := decodeSingletonRuleRow(content.ContentTypeProductionRules, snapshot.ProductionRules, serverRuleProductionRulesID, &out); err != nil {
		return content.ProductionRulesContent{}, err
	}
	if err := out.Validate(bundle); err != nil {
		return content.ProductionRulesContent{}, err
	}
	return out, nil
}

func mapCombatRuleRows(snapshot content.Snapshot) (content.CombatRulesContent, error) {
	var out content.CombatRulesContent
	if err := decodeSingletonRuleRow(content.ContentTypeCombatRules, snapshot.CombatRules, serverRuleCombatRulesID, &out); err != nil {
		return content.CombatRulesContent{}, err
	}
	if err := out.Validate(); err != nil {
		return content.CombatRulesContent{}, err
	}
	return out, nil
}

func decodeSingletonRuleRow(contentType content.ContentType, rows []content.SnapshotRow, expectedID content.ContentID, out any) error {
	if len(rows) != 1 {
		return fmt.Errorf("%s rows=%d: %w", contentType, len(rows), content.ErrInvalidContentSnapshot)
	}
	row := rows[0]
	if !row.Enabled {
		return fmt.Errorf("%s %q disabled: %w", contentType, row.ContentID, content.ErrInvalidContentSnapshot)
	}
	if row.ContentID != expectedID {
		return fmt.Errorf("%s row %q: want %q: %w", contentType, row.ContentID, expectedID, ErrContentRowIDMismatch)
	}
	if err := decodeSnapshotRow(contentType, row, out); err != nil {
		return err
	}
	return nil
}

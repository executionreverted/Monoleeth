package content

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"gameproject/internal/game/foundation"
)

type ContentID string

type ContentType string

var ErrUnknownContentType = errors.New("unknown content type")

const (
	ContentTypeItem               ContentType = "item"
	ContentTypeModule             ContentType = "module"
	ContentTypeShip               ContentType = "ship"
	ContentTypeShopProduct        ContentType = "shop_product"
	ContentTypeNPCTemplate        ContentType = "npc_template"
	ContentTypeSpawnArea          ContentType = "spawn_area"
	ContentTypeEnemyPool          ContentType = "enemy_pool"
	ContentTypeNPCDropProfile     ContentType = "npc_drop_profile"
	ContentTypeNPCAggroProfile    ContentType = "npc_aggro_profile"
	ContentTypeNPCLeashProfile    ContentType = "npc_leash_profile"
	ContentTypeNPCEventSpawn      ContentType = "npc_event_spawn"
	ContentTypeLootTable          ContentType = "loot_table"
	ContentTypeCraftRecipe        ContentType = "craft_recipe"
	ContentTypeProductionBuilding ContentType = "production_building"
	ContentTypeQuestTemplate      ContentType = "quest_template"
	ContentTypeQuestRewardTable   ContentType = "quest_reward_table"
	ContentTypeScannerConfig      ContentType = "scanner_config"
	ContentTypeStarterConfig      ContentType = "starter_config"
	ContentTypeRoutePolicy        ContentType = "route_policy"
	ContentTypeProductionRules    ContentType = "production_rules"
	ContentTypeCombatRules        ContentType = "combat_rules"
)

func AllContentTypes() []ContentType {
	return []ContentType{
		ContentTypeItem,
		ContentTypeModule,
		ContentTypeShip,
		ContentTypeShopProduct,
		ContentTypeNPCTemplate,
		ContentTypeSpawnArea,
		ContentTypeEnemyPool,
		ContentTypeNPCDropProfile,
		ContentTypeNPCAggroProfile,
		ContentTypeNPCLeashProfile,
		ContentTypeNPCEventSpawn,
		ContentTypeLootTable,
		ContentTypeCraftRecipe,
		ContentTypeProductionBuilding,
		ContentTypeQuestTemplate,
		ContentTypeQuestRewardTable,
		ContentTypeScannerConfig,
		ContentTypeStarterConfig,
		ContentTypeRoutePolicy,
		ContentTypeProductionRules,
		ContentTypeCombatRules,
	}
}

func IsKnownContentType(contentType ContentType) bool {
	for _, known := range AllContentTypes() {
		if contentType == known {
			return true
		}
	}
	return false
}

func ValidateContentID(kind string, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s id: %w", kind, foundation.ErrEmptyID)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s id %q: %w", kind, value, foundation.ErrInvalidID)
	}
	if strings.Contains(value, ":") {
		return fmt.Errorf("%s id %q: %w", kind, value, foundation.ErrInvalidID)
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return fmt.Errorf("%s id %q: %w", kind, value, foundation.ErrInvalidID)
		}
	}
	return nil
}

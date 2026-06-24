package content

import (
	"fmt"
	"strings"
	"unicode"

	"gameproject/internal/game/foundation"
)

type ContentID string

type ContentType string

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
)

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

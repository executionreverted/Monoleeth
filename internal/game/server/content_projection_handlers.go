package server

import (
	"encoding/json"

	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/realtime"
)

type contentCatalogResponsePayload struct {
	ContentCatalog gamecontent.PlayerContentProjection `json:"content_catalog"`
}

func (runtime *Runtime) handleContentCatalog(_ realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectEmptyIntentPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	projection := clonePlayerContentProjection(runtime.contentCatalogProjection)
	if projection.Version == "" {
		projection.Version = runtime.contentCatalogVersion
	}
	runtime.mu.Unlock()
	return marshalPayload(contentCatalogResponsePayload{ContentCatalog: projection})
}

func clonePlayerContentProjection(projection gamecontent.PlayerContentProjection) gamecontent.PlayerContentProjection {
	cloned := projection
	cloned.Categories = append([]gamecontent.PlayerContentCategory(nil), projection.Categories...)
	cloned.Items = make([]gamecontent.PlayerItemProjection, len(projection.Items))
	for index, item := range projection.Items {
		cloned.Items[index] = item
		cloned.Items[index].TradeFlags = append([]string(nil), item.TradeFlags...)
		cloned.Items[index].BindRules = append([]string(nil), item.BindRules...)
	}
	cloned.Modules = make([]gamecontent.PlayerModuleProjection, len(projection.Modules))
	for index, module := range projection.Modules {
		cloned.Modules[index] = module
		cloned.Modules[index].RequiredRoleLevels = append([]gamecontent.PlayerRoleRequirement(nil), module.RequiredRoleLevels...)
		cloned.Modules[index].StatModifiers = append([]gamecontent.PlayerStatModifier(nil), module.StatModifiers...)
		cloned.Modules[index].Cooldowns = append([]gamecontent.PlayerCooldown(nil), module.Cooldowns...)
		cloned.Modules[index].TradeFlags = append([]string(nil), module.TradeFlags...)
		cloned.Modules[index].BindRules = append([]string(nil), module.BindRules...)
		cloned.Modules[index].CompatibleSlotTypes = append([]string(nil), module.CompatibleSlotTypes...)
		cloned.Modules[index].CompatibleCategories = append([]string(nil), module.CompatibleCategories...)
	}
	cloned.ShopProducts = append([]gamecontent.PlayerShopProductProjection(nil), projection.ShopProducts...)
	return cloned
}

package server

import (
	"encoding/json"
	"sort"

	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world/worker"
)

type combatSelectAmmoIntent struct {
	Family string            `json:"family"`
	ItemID foundation.ItemID `json:"item_id"`
}

func decodeCombatSelectAmmoIntent(raw json.RawMessage) (combatSelectAmmoIntent, error) {
	var intent combatSelectAmmoIntent
	if err := decodeStrict(raw, &intent); err != nil {
		return combatSelectAmmoIntent{}, err
	}
	if intent.Family == "" {
		return combatSelectAmmoIntent{}, invalidPayload("family is required.", nil)
	}
	if err := intent.ItemID.Validate(); err != nil {
		return combatSelectAmmoIntent{}, invalidPayload("item_id is required.", err)
	}
	return intent, nil
}

func (runtime *Runtime) handleCombatSelectAmmo(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayloadWithAdditional(request.Payload, "quantity", "multiplier", "damage_multiplier", "flat_damage", "hit_result", "fallback"); err != nil {
		return nil, err
	}
	intent, err := decodeCombatSelectAmmoIntent(request.Payload)
	if err != nil {
		return nil, err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if _, ok := runtime.players[ctx.PlayerID]; !ok {
		return nil, domainErrorForRuntime(worker.ErrUnknownPlayer)
	}
	family := gamecontent.CombatAmmoFamily(intent.Family)
	definition, ok := runtime.combatAmmoDefinitionLocked(family, intent.ItemID)
	if !ok || !definition.Selectable {
		return nil, foundation.NewDomainError(foundation.CodeNotFound, "Combat ammo was not found.")
	}
	itemDefinition, ok := runtime.itemCatalog[intent.ItemID]
	if !ok {
		return nil, foundation.NewDomainError(foundation.CodeNotFound, "Ammo item was not found.")
	}
	if itemDefinition.Type != economy.ItemTypeStackable {
		return nil, foundation.NewDomainError(foundation.CodeInvalidPayload, "Combat ammo must be stackable.")
	}
	quantity, err := runtime.combatAmmoQuantityLocked(ctx.PlayerID, intent.ItemID)
	if err != nil {
		return nil, domainErrorForEconomy(err)
	}
	if quantity <= 0 {
		return nil, foundation.NewDomainError(foundation.CodeNotEnoughAmmo, "Not enough ammo.")
	}

	runtime.setActiveCombatAmmoLocked(ctx.PlayerID, family, intent.ItemID)
	payload := runtime.combatAmmoStatePayloadLocked(ctx.PlayerID)
	sessionID := authSessionID(ctx.SessionID)
	runtime.queueEventLocked(sessionID, realtime.EventCombatStateSnapshot, runtime.combatEngagementPayloadLocked(ctx.PlayerID, runtime.clock.Now()))

	return marshalPayload(map[string]any{
		"accepted":    true,
		"family":      string(family),
		"item_id":     intent.ItemID.String(),
		"quantity":    quantity,
		"active_ammo": payload,
	})
}

func (runtime *Runtime) combatAmmoDefinitionLocked(family gamecontent.CombatAmmoFamily, itemID foundation.ItemID) (gamecontent.CombatAmmoDefinition, bool) {
	for _, definition := range runtime.combatRules.Ammo {
		if definition.Family == family && definition.ItemID == itemID {
			return definition, true
		}
	}
	return gamecontent.CombatAmmoDefinition{}, false
}

func (runtime *Runtime) setActiveCombatAmmoLocked(playerID foundation.PlayerID, family gamecontent.CombatAmmoFamily, itemID foundation.ItemID) {
	if runtime.activeCombatAmmo == nil {
		runtime.activeCombatAmmo = make(map[foundation.PlayerID]map[gamecontent.CombatAmmoFamily]foundation.ItemID)
	}
	if runtime.activeCombatAmmo[playerID] == nil {
		runtime.activeCombatAmmo[playerID] = make(map[gamecontent.CombatAmmoFamily]foundation.ItemID)
	}
	runtime.activeCombatAmmo[playerID][family] = itemID
}

func (runtime *Runtime) combatAmmoQuantityLocked(playerID foundation.PlayerID, itemID foundation.ItemID) (int64, error) {
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, playerID.String())
	if err != nil {
		return 0, err
	}
	return runtime.Inventory.TotalItemQuantity(playerID, itemID, location), nil
}

func (runtime *Runtime) combatAmmoStatePayloadLocked(playerID foundation.PlayerID) map[string]any {
	if runtime.activeCombatAmmo == nil || len(runtime.activeCombatAmmo[playerID]) == 0 {
		return map[string]any{}
	}
	families := make([]gamecontent.CombatAmmoFamily, 0, len(runtime.activeCombatAmmo[playerID]))
	for family := range runtime.activeCombatAmmo[playerID] {
		families = append(families, family)
	}
	sort.Slice(families, func(i, j int) bool {
		return families[i] < families[j]
	})
	payload := make(map[string]any, len(families))
	for _, family := range families {
		itemID := runtime.activeCombatAmmo[playerID][family]
		definition, ok := runtime.combatAmmoDefinitionLocked(family, itemID)
		if !ok {
			continue
		}
		quantity, _ := runtime.combatAmmoQuantityLocked(playerID, itemID)
		payload[string(family)] = map[string]any{
			"item_id":                 itemID.String(),
			"ammo_key":                definition.AmmoKey,
			"quantity":                quantity,
			"damage_multiplier":       definition.DamageMultiplier,
			"flat_damage":             definition.FlatDamage,
			"shield_leech_multiplier": definition.ShieldLeechMultiplier,
			"cooldown_ms":             definition.CooldownMS,
			"accuracy_modifier":       definition.AccuracyModifier,
			"fallback_rank":           definition.FallbackRank,
			"slotbar_order":           definition.SlotbarOrder,
		}
	}
	return payload
}

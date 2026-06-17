package death_test

import (
	"errors"
	"math"
	"testing"

	"gameproject/internal/game/death"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestLethalEventIdempotencyKeyValidationShape(t *testing.T) {
	key, err := death.NewLethalEventKey("combat-lethal-1")
	if err != nil {
		t.Fatalf("NewLethalEventKey() error = %v", err)
	}
	if got, want := key.String(), "player_death:combat-lethal-1"; got != want {
		t.Fatalf("key = %q, want %q", got, want)
	}

	duplicate, err := death.NewLethalEventKey("combat-lethal-1")
	if err != nil {
		t.Fatalf("duplicate NewLethalEventKey() error = %v", err)
	}
	if duplicate != key {
		t.Fatalf("duplicate key = %q, want same key %q", duplicate, key)
	}

	parsed, err := death.ParseLethalEventKey(key.String())
	if err != nil {
		t.Fatalf("ParseLethalEventKey() error = %v", err)
	}
	if parsed != key {
		t.Fatalf("parsed key = %q, want %q", parsed, key)
	}
	parsedEventID, err := parsed.EventID()
	if err != nil {
		t.Fatalf("EventID() error = %v", err)
	}
	if parsedEventID != foundation.EventID("combat-lethal-1") {
		t.Fatalf("EventID() = %q, want combat-lethal-1", parsedEventID)
	}

	if _, err := death.NewLethalEventKey(""); !errors.Is(err, foundation.ErrEmptyID) {
		t.Fatalf("blank event id error = %v, want foundation.ErrEmptyID", err)
	}
	if _, err := death.ParseLethalEventKey(""); !errors.Is(err, death.ErrEmptyLethalEventKey) {
		t.Fatalf("blank key error = %v, want ErrEmptyLethalEventKey", err)
	}
	if _, err := death.ParseLethalEventKey("loot_pickup:drop-1"); !errors.Is(err, death.ErrInvalidLethalEventKey) {
		t.Fatalf("wrong operation error = %v, want ErrInvalidLethalEventKey", err)
	}
	if _, err := death.ParseLethalEventKey("player_death:bad:event"); !errors.Is(err, death.ErrInvalidLethalEventKey) {
		t.Fatalf("malformed key error = %v, want ErrInvalidLethalEventKey", err)
	}
}

func TestZoneCargoDropPolicyRollsPercentInsideConfiguredRangeDeterministically(t *testing.T) {
	policy := cargoPolicy(t, 0.30, 0.70)
	percent, err := death.RollCargoDropPercent(policy, testutil.NewFakeRNG(nil, []float64{0.25}))
	if err != nil {
		t.Fatalf("RollCargoDropPercent() error = %v", err)
	}
	if percent < policy.MinPercent || percent > policy.MaxPercent {
		t.Fatalf("percent = %v outside policy range %v..%v", percent, policy.MinPercent, policy.MaxPercent)
	}
	if math.Abs(percent-0.40) > 0.000001 {
		t.Fatalf("percent = %v, want deterministic 0.40", percent)
	}
}

func TestSelectCargoDropsDropsStackablePartialQuantity(t *testing.T) {
	stack := stackableCargo(t, "iron-stack-1", "iron_ore", 10, economy.LocationKindShipCargo)

	result, err := death.SelectCargoDrops(death.SelectCargoDropsInput{
		Policy: cargoPolicy(t, 0.45, 0.45),
		Cargo:  []death.CargoStack{stack},
		RNG:    testutil.NewFakeRNG(nil, nil),
	})
	if err != nil {
		t.Fatalf("SelectCargoDrops() error = %v", err)
	}
	if got, want := result.TargetUnits, int64(4); got != want {
		t.Fatalf("target units = %d, want %d", got, want)
	}
	if got, want := len(result.Drops), 1; got != want {
		t.Fatalf("drops len = %d, want %d", got, want)
	}
	if got, want := result.Drops[0].Quantity, int64(4); got != want {
		t.Fatalf("drop quantity = %d, want %d", got, want)
	}
	if got, want := len(result.Preserved), 1; got != want {
		t.Fatalf("preserved len = %d, want %d", got, want)
	}
	if got, want := result.Preserved[0].Quantity, int64(6); got != want {
		t.Fatalf("preserved quantity = %d, want %d", got, want)
	}
}

func TestSelectCargoDropsDropsInstanceItemsWhole(t *testing.T) {
	first := instanceCargo(t, "laser-instance-1", "laser_alpha_t1", economy.LocationKindShipCargo)
	second := instanceCargo(t, "shield-instance-1", "shield_alpha_t1", economy.LocationKindShipCargo)
	third := instanceCargo(t, "scanner-instance-1", "scanner_alpha_t1", economy.LocationKindShipCargo)

	result, err := death.SelectCargoDrops(death.SelectCargoDropsInput{
		Policy: cargoPolicy(t, 0.67, 0.67),
		Cargo:  []death.CargoStack{first, second, third},
		RNG:    testutil.NewFakeRNG([]int{2, 1}, nil),
	})
	if err != nil {
		t.Fatalf("SelectCargoDrops() error = %v", err)
	}
	if got, want := result.TargetUnits, int64(2); got != want {
		t.Fatalf("target units = %d, want %d", got, want)
	}
	if got, want := len(result.Drops), 2; got != want {
		t.Fatalf("drops len = %d, want %d", got, want)
	}
	for _, drop := range result.Drops {
		if drop.Quantity != 1 {
			t.Fatalf("instance drop %+v quantity = %d, want 1", drop, drop.Quantity)
		}
	}
	if result.Drops[0].ItemInstanceID != first.ItemInstanceID || result.Drops[1].ItemInstanceID != second.ItemInstanceID {
		t.Fatalf("drops = %+v, want first two instances with deterministic no-op shuffle", result.Drops)
	}
	if got, want := len(result.Preserved), 1; got != want {
		t.Fatalf("preserved len = %d, want %d", got, want)
	}
	if result.Preserved[0].ItemInstanceID != third.ItemInstanceID {
		t.Fatalf("preserved = %+v, want third instance", result.Preserved)
	}
}

func TestSelectCargoDropsPreservesProtectedAndBlockedCargo(t *testing.T) {
	eligible := stackableCargo(t, "iron-stack-1", "iron_ore", 10, economy.LocationKindShipCargo)
	nonDroppable := stackableCargo(t, "quest-token-stack", "quest_token", 3, economy.LocationKindShipCargo, withoutDroppableFlag)
	soulbound := instanceCargo(t, "bound-module-1", "bound_module", economy.LocationKindShipCargo, withBoundState(economy.BoundStateSoulbound))
	soulboundRule := stackableCargo(t, "bound-fragment-stack", "bound_fragment", 5, economy.LocationKindShipCargo, withBindRules(economy.BindRuleSoulbound))
	questCritical := stackableCargo(t, "quest-critical-stack", "quest_ore", 7, economy.LocationKindShipCargo, withQuestCritical)
	systemCritical := stackableCargo(t, "system-critical-stack", "system_core", 1, economy.LocationKindShipCargo, withSystemCritical)
	reserved := stackableCargo(t, "reserved-stack", "reserved_ore", 8, economy.LocationKindCraftingReserved)
	marketEscrow := stackableCargo(t, "market-stack", "market_ore", 9, economy.LocationKindMarketEscrow)
	auctionEscrow := stackableCargo(t, "auction-stack", "auction_ore", 9, economy.LocationKindAuctionEscrow)
	systemSink := stackableCargo(t, "system-stack", "system_ore", 9, economy.LocationKindSystemSink)

	result, err := death.SelectCargoDrops(death.SelectCargoDropsInput{
		Policy: cargoPolicy(t, 1, 1),
		Cargo: []death.CargoStack{
			eligible,
			nonDroppable,
			soulbound,
			soulboundRule,
			questCritical,
			systemCritical,
			reserved,
			marketEscrow,
			auctionEscrow,
			systemSink,
		},
		RNG: testutil.NewFakeRNG(nil, nil),
	})
	if err != nil {
		t.Fatalf("SelectCargoDrops() error = %v", err)
	}
	if got, want := len(result.Drops), 1; got != want {
		t.Fatalf("drops len = %d, want %d", got, want)
	}
	if result.Drops[0].ItemInstanceID != eligible.ItemInstanceID || result.Drops[0].Quantity != eligible.Quantity {
		t.Fatalf("drop = %+v, want only eligible stack", result.Drops[0])
	}

	preserved := preservedByInstanceID(result.Preserved)
	for _, stack := range []death.CargoStack{
		nonDroppable,
		soulbound,
		soulboundRule,
		questCritical,
		systemCritical,
		reserved,
		marketEscrow,
		auctionEscrow,
		systemSink,
	} {
		got, ok := preserved[stack.ItemInstanceID]
		if !ok {
			t.Fatalf("protected stack %q was not preserved; preserved = %+v", stack.ItemInstanceID, result.Preserved)
		}
		if got.Quantity != stack.Quantity {
			t.Fatalf("protected stack %q quantity = %d, want %d", stack.ItemInstanceID, got.Quantity, stack.Quantity)
		}
	}
}

type cargoOption func(*death.CargoStack)

func stackableCargo(t *testing.T, instanceID, itemID string, quantity int64, locationKind economy.LocationKind, options ...cargoOption) death.CargoStack {
	t.Helper()
	stack := death.CargoStack{
		ItemInstanceID: foundation.ItemID(instanceID),
		Definition: death.CargoItemDefinition{
			ItemID:     foundation.ItemID(itemID),
			Type:       economy.ItemTypeStackable,
			TradeFlags: []economy.TradeFlag{economy.TradeFlagDroppable},
			BindRules:  []economy.BindRule{economy.BindRuleNone},
		},
		Location:   cargoLocation(t, locationKind, "ship-1"),
		Quantity:   quantity,
		BoundState: economy.BoundStateUnbound,
	}
	for _, option := range options {
		option(&stack)
	}
	return stack
}

func instanceCargo(t *testing.T, instanceID, itemID string, locationKind economy.LocationKind, options ...cargoOption) death.CargoStack {
	t.Helper()
	stack := death.CargoStack{
		ItemInstanceID: foundation.ItemID(instanceID),
		Definition: death.CargoItemDefinition{
			ItemID:     foundation.ItemID(itemID),
			Type:       economy.ItemTypeInstance,
			TradeFlags: []economy.TradeFlag{economy.TradeFlagDroppable},
			BindRules:  []economy.BindRule{economy.BindRuleNone},
		},
		Location:   cargoLocation(t, locationKind, "ship-1"),
		Quantity:   1,
		BoundState: economy.BoundStateUnbound,
	}
	for _, option := range options {
		option(&stack)
	}
	return stack
}

func withoutDroppableFlag(stack *death.CargoStack) {
	stack.Definition.TradeFlags = []economy.TradeFlag{economy.TradeFlagTradeable}
}

func withBoundState(state economy.BoundState) cargoOption {
	return func(stack *death.CargoStack) {
		stack.BoundState = state
	}
}

func withBindRules(rules ...economy.BindRule) cargoOption {
	return func(stack *death.CargoStack) {
		stack.Definition.BindRules = append([]economy.BindRule(nil), rules...)
	}
}

func withQuestCritical(stack *death.CargoStack) {
	stack.Definition.QuestCritical = true
}

func withSystemCritical(stack *death.CargoStack) {
	stack.Definition.SystemCritical = true
}

func cargoPolicy(t *testing.T, minPercent, maxPercent float64) death.ZoneCargoDropPolicy {
	t.Helper()
	policy, err := death.NewZoneCargoDropPolicy("zone-1", minPercent, maxPercent)
	if err != nil {
		t.Fatalf("NewZoneCargoDropPolicy(): %v", err)
	}
	return policy
}

func cargoLocation(t *testing.T, kind economy.LocationKind, id string) economy.ItemLocation {
	t.Helper()
	location, err := economy.NewItemLocation(kind, id)
	if err != nil {
		t.Fatalf("NewItemLocation(%q, %q): %v", kind, id, err)
	}
	return location
}

func preservedByInstanceID(stacks []death.CargoStack) map[foundation.ItemID]death.CargoStack {
	preserved := make(map[foundation.ItemID]death.CargoStack, len(stacks))
	for _, stack := range stacks {
		preserved[stack.ItemInstanceID] = stack
	}
	return preserved
}

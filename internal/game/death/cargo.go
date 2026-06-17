package death

import (
	"errors"
	"fmt"
	"math"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

var (
	ErrNilRNG                    = errors.New("nil rng")
	ErrInvalidCargoDropPolicy    = errors.New("invalid cargo drop policy")
	ErrInvalidCargoDropPercent   = errors.New("invalid cargo drop percent")
	ErrInvalidCargoDefinition    = errors.New("invalid cargo item definition")
	ErrInvalidCargoStack         = errors.New("invalid cargo stack")
	ErrInvalidCargoStackQuantity = errors.New("invalid cargo stack quantity")
)

// ZoneCargoDropPolicy defines the server-owned death cargo loss range for a
// zone. Percent fields use 0..1, where 0.5 means 50%.
type ZoneCargoDropPolicy struct {
	ZoneID     foundation.ZoneID `json:"zone_id"`
	MinPercent float64           `json:"min_percent"`
	MaxPercent float64           `json:"max_percent"`
}

// CargoItemDefinition is the item-definition subset the death selector needs.
type CargoItemDefinition struct {
	ItemID            foundation.ItemID   `json:"item_id"`
	Type              economy.ItemType    `json:"item_type"`
	TradeFlags        []economy.TradeFlag `json:"trade_flags,omitempty"`
	BindRules         []economy.BindRule  `json:"bind_rules,omitempty"`
	CargoUnitsPerItem int64               `json:"cargo_units_per_item,omitempty"`
	QuestCritical     bool                `json:"quest_critical,omitempty"`
	SystemCritical    bool                `json:"system_critical,omitempty"`
}

// CargoStack is one cargo row or unique instance supplied to the pure selector.
type CargoStack struct {
	StackID        foundation.ItemID    `json:"stack_id,omitempty"`
	ItemInstanceID foundation.ItemID    `json:"item_instance_id"`
	SourceStackID  foundation.ItemID    `json:"source_stack_id,omitempty"`
	Definition     CargoItemDefinition  `json:"definition"`
	OwnerPlayerID  foundation.PlayerID  `json:"owner_player_id,omitempty"`
	Location       economy.ItemLocation `json:"location"`
	Quantity       int64                `json:"quantity"`
	BoundState     economy.BoundState   `json:"bound_state"`
	Reason         CargoPreserveReason  `json:"preserve_reason,omitempty"`
}

// CargoDrop is one selected cargo loss line to be removed from cargo and later
// handed to the loot service by a death transaction.
type CargoDrop struct {
	SourceStackID  foundation.ItemID    `json:"source_stack_id"`
	ItemInstanceID foundation.ItemID    `json:"item_instance_id"`
	ItemID         foundation.ItemID    `json:"item_id"`
	Type           economy.ItemType     `json:"item_type"`
	SourceLocation economy.ItemLocation `json:"source_location"`
	Quantity       int64                `json:"quantity"`
}

// CargoPreserveReason explains why a cargo row remains after selection.
type CargoPreserveReason string

const (
	CargoPreserveNotSelected      CargoPreserveReason = "not_selected"
	CargoPreservePartialRemainder CargoPreserveReason = "partial_remainder"
	CargoPreserveNotDroppable     CargoPreserveReason = "not_droppable"
	CargoPreserveSoulbound        CargoPreserveReason = "soulbound"
	CargoPreserveQuestCritical    CargoPreserveReason = "quest_critical"
	CargoPreserveSystemCritical   CargoPreserveReason = "system_critical"
	CargoPreserveBlockedLocation  CargoPreserveReason = "blocked_location"
)

// SelectCargoDropsInput is the full pure-selector input.
type SelectCargoDropsInput struct {
	Policy ZoneCargoDropPolicy
	Cargo  []CargoStack
	RNG    foundation.RNG
}

// CargoDropSelection reports selected drops and the cargo remainder that should
// be preserved by later inventory orchestration.
type CargoDropSelection struct {
	DropPercent      float64      `json:"cargo_drop_percent"`
	EligibleUnits    int64        `json:"eligible_units"`
	TargetUnits      int64        `json:"target_units"`
	TargetCargoUnits int64        `json:"target_cargo_units"`
	DroppedUnits     int64        `json:"dropped_units"`
	Drops            []CargoDrop  `json:"drops"`
	Preserved        []CargoStack `json:"preserved"`
}

// NewZoneCargoDropPolicy validates and returns a zone cargo loss policy.
func NewZoneCargoDropPolicy(zoneID foundation.ZoneID, minPercent, maxPercent float64) (ZoneCargoDropPolicy, error) {
	policy := ZoneCargoDropPolicy{
		ZoneID:     zoneID,
		MinPercent: minPercent,
		MaxPercent: maxPercent,
	}
	if err := policy.Validate(); err != nil {
		return ZoneCargoDropPolicy{}, err
	}
	return policy, nil
}

// Validate reports whether the policy has a valid zone and bounded percent range.
func (policy ZoneCargoDropPolicy) Validate() error {
	if err := policy.ZoneID.Validate(); err != nil {
		return err
	}
	if !validDropPercent(policy.MinPercent) || !validDropPercent(policy.MaxPercent) || policy.MinPercent > policy.MaxPercent {
		return fmt.Errorf("cargo drop percent %.4f..%.4f: %w", policy.MinPercent, policy.MaxPercent, ErrInvalidCargoDropPolicy)
	}
	return nil
}

// RollCargoDropPercent returns a deterministic server-side cargo drop percent
// within policy bounds.
func RollCargoDropPercent(policy ZoneCargoDropPolicy, rng foundation.RNG) (float64, error) {
	if err := policy.Validate(); err != nil {
		return 0, err
	}
	if rng == nil {
		return 0, ErrNilRNG
	}
	if policy.MinPercent == policy.MaxPercent {
		return policy.MinPercent, nil
	}

	roll := rng.Float64()
	if math.IsNaN(roll) || math.IsInf(roll, 0) {
		return 0, fmt.Errorf("rng roll %v: %w", roll, ErrInvalidCargoDropPolicy)
	}
	percent := policy.MinPercent + roll*(policy.MaxPercent-policy.MinPercent)
	return clamp(percent, policy.MinPercent, policy.MaxPercent), nil
}

// Validate reports whether definition has the fields needed for eligibility.
func (definition CargoItemDefinition) Validate() error {
	if err := definition.ItemID.Validate(); err != nil {
		return err
	}
	if err := definition.Type.Validate(); err != nil {
		return err
	}
	if definition.CargoUnitsPerItem < 0 {
		return fmt.Errorf("cargo units per item %d: %w", definition.CargoUnitsPerItem, ErrInvalidCargoDefinition)
	}
	for _, flag := range definition.TradeFlags {
		if err := flag.Validate(); err != nil {
			return err
		}
	}
	for _, rule := range definition.BindRules {
		if err := rule.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate reports whether stack has a valid item identity, location, state,
// and quantity.
func (stack CargoStack) Validate() error {
	if err := stack.sourceStackID().Validate(); err != nil {
		return err
	}
	if !stack.StackID.IsZero() && !stack.ItemInstanceID.IsZero() && stack.StackID != stack.ItemInstanceID {
		return fmt.Errorf("stack id %q item instance id %q: %w", stack.StackID, stack.ItemInstanceID, ErrInvalidCargoStack)
	}
	if err := stack.Definition.Validate(); err != nil {
		return err
	}
	if !stack.OwnerPlayerID.IsZero() {
		if err := stack.OwnerPlayerID.Validate(); err != nil {
			return err
		}
	}
	if err := stack.Location.Validate(); err != nil {
		return err
	}
	if err := foundation.ValidatePositiveAmount(stack.Quantity); err != nil {
		return fmt.Errorf("cargo stack quantity %d: %w", stack.Quantity, err)
	}
	if stack.Definition.Type == economy.ItemTypeInstance && stack.Quantity != 1 {
		return fmt.Errorf("instance cargo quantity %d: %w", stack.Quantity, economy.ErrInvalidInstanceQuantity)
	}
	if err := stack.effectiveBoundState().Validate(); err != nil {
		return err
	}
	return nil
}

// DropEligible reports whether this cargo row may be selected for death loss.
func (stack CargoStack) DropEligible() bool {
	_, blocked := stack.blockedPreserveReason()
	return !blocked
}

// UnitCount returns the selector unit count for the cargo row.
func (stack CargoStack) UnitCount() int64 {
	if stack.Definition.Type == economy.ItemTypeInstance {
		return 1
	}
	return stack.Quantity * stack.cargoUnitsPerItem()
}

// SelectCargoDrops chooses cargo loss without mutating input cargo.
func SelectCargoDrops(input SelectCargoDropsInput) (CargoDropSelection, error) {
	return selectCargoDrops(input.Cargo, input.Policy, input.RNG)
}

func selectCargoDrops(cargo []CargoStack, policy ZoneCargoDropPolicy, rng foundation.RNG) (CargoDropSelection, error) {
	percent, err := RollCargoDropPercent(policy, rng)
	if err != nil {
		return CargoDropSelection{}, err
	}

	eligible := make([]indexedCargoStack, 0, len(cargo))
	preserveReasons := make(map[int]CargoPreserveReason)
	for index, stack := range cargo {
		if err := stack.Validate(); err != nil {
			return CargoDropSelection{}, fmt.Errorf("cargo[%d]: %w", index, err)
		}
		if reason, blocked := stack.blockedPreserveReason(); blocked {
			preserveReasons[index] = reason
			continue
		}
		eligible = append(eligible, indexedCargoStack{
			index: index,
			stack: stack,
		})
	}

	eligibleUnits := sumEligibleUnits(eligible)
	targetUnits := int64(float64(eligibleUnits) * percent)
	if targetUnits > eligibleUnits {
		targetUnits = eligibleUnits
	}

	droppedByIndex := make(map[int]int64)
	drops := make([]CargoDrop, 0)
	remaining := targetUnits
	for _, entry := range shuffleIndexedCargo(eligible, rng) {
		if remaining <= 0 {
			break
		}

		var dropQuantity int64
		var droppedUnits int64
		if entry.stack.Definition.Type == economy.ItemTypeInstance {
			dropQuantity = 1
			droppedUnits = 1
		} else {
			unitsPerItem := entry.stack.cargoUnitsPerItem()
			maxQuantity := remaining / unitsPerItem
			if maxQuantity <= 0 {
				continue
			}
			dropQuantity = entry.stack.Quantity
			if dropQuantity > maxQuantity {
				dropQuantity = maxQuantity
			}
			droppedUnits = dropQuantity * unitsPerItem
		}
		drops = append(drops, cargoDropFromStack(entry.stack, dropQuantity))
		droppedByIndex[entry.index] += droppedUnits
		remaining -= droppedUnits
	}

	preserved := preserveCargo(cargo, droppedByIndex, preserveReasons)
	droppedUnits := targetUnits - remaining
	return CargoDropSelection{
		DropPercent:      percent,
		EligibleUnits:    eligibleUnits,
		TargetUnits:      targetUnits,
		TargetCargoUnits: targetUnits,
		DroppedUnits:     droppedUnits,
		Drops:            drops,
		Preserved:        preserved,
	}, nil
}

// SelectCargoDropsFromInput adapts the structured input shape to the pure
// selector. It is kept for callers that prefer an explicit adapter name.
func SelectCargoDropsFromInput(input SelectCargoDropsInput) (CargoDropSelection, error) {
	return selectCargoDrops(input.Cargo, input.Policy, input.RNG)
}

type indexedCargoStack struct {
	index int
	stack CargoStack
}

func sumEligibleUnits(stacks []indexedCargoStack) int64 {
	var total int64
	for _, entry := range stacks {
		total += entry.stack.UnitCount()
	}
	return total
}

func shuffleIndexedCargo(stacks []indexedCargoStack, rng foundation.RNG) []indexedCargoStack {
	shuffled := append([]indexedCargoStack(nil), stacks...)
	for i := len(shuffled) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}
	return shuffled
}

func cargoDropFromStack(stack CargoStack, quantity int64) CargoDrop {
	sourceID := stack.sourceStackID()
	return CargoDrop{
		SourceStackID:  sourceID,
		ItemInstanceID: sourceID,
		ItemID:         stack.Definition.ItemID,
		Type:           stack.Definition.Type,
		SourceLocation: stack.Location,
		Quantity:       quantity,
	}
}

func preserveCargo(cargo []CargoStack, droppedByIndex map[int]int64, preserveReasons map[int]CargoPreserveReason) []CargoStack {
	preserved := make([]CargoStack, 0, len(cargo))
	for index, stack := range cargo {
		dropped := droppedByIndex[index]
		if dropped <= 0 {
			preserved = append(preserved, preservedStack(stack, preserveReasonForIndex(index, preserveReasons, CargoPreserveNotSelected)))
			continue
		}
		if stack.Definition.Type == economy.ItemTypeInstance {
			continue
		}
		droppedQuantity := dropped / stack.cargoUnitsPerItem()
		if dropped%stack.cargoUnitsPerItem() != 0 {
			droppedQuantity++
		}
		if remaining := stack.Quantity - droppedQuantity; remaining > 0 {
			clone := preservedStack(stack, CargoPreservePartialRemainder)
			clone.Quantity = remaining
			preserved = append(preserved, clone)
		}
	}
	return preserved
}

func preservedStack(stack CargoStack, reason CargoPreserveReason) CargoStack {
	clone := stack
	sourceID := stack.sourceStackID()
	clone.StackID = sourceID
	clone.ItemInstanceID = sourceID
	clone.SourceStackID = sourceID
	clone.Reason = reason
	if clone.BoundState == "" {
		clone.BoundState = economy.BoundStateUnbound
	}
	return clone
}

func preserveReasonForIndex(index int, reasons map[int]CargoPreserveReason, fallback CargoPreserveReason) CargoPreserveReason {
	if reason, ok := reasons[index]; ok {
		return reason
	}
	return fallback
}

func (stack CargoStack) blockedPreserveReason() (CargoPreserveReason, bool) {
	if stack.Location.Kind != economy.LocationKindShipCargo {
		return CargoPreserveBlockedLocation, true
	}
	if stack.Definition.QuestCritical {
		return CargoPreserveQuestCritical, true
	}
	if stack.Definition.SystemCritical {
		return CargoPreserveSystemCritical, true
	}
	if stack.effectiveBoundState() == economy.BoundStateSoulbound {
		return CargoPreserveSoulbound, true
	}
	for _, rule := range stack.Definition.BindRules {
		if rule == economy.BindRuleSoulbound {
			return CargoPreserveSoulbound, true
		}
	}
	if economy.ValidateDroppableTradeFlags(stack.Definition.TradeFlags) != nil {
		return CargoPreserveNotDroppable, true
	}
	return "", false
}

func (stack CargoStack) sourceStackID() foundation.ItemID {
	switch {
	case !stack.ItemInstanceID.IsZero():
		return stack.ItemInstanceID
	case !stack.StackID.IsZero():
		return stack.StackID
	default:
		return stack.SourceStackID
	}
}

func (stack CargoStack) effectiveBoundState() economy.BoundState {
	if stack.BoundState == "" {
		return economy.BoundStateUnbound
	}
	return stack.BoundState
}

func (stack CargoStack) cargoUnitsPerItem() int64 {
	if stack.Definition.CargoUnitsPerItem <= 0 {
		return 1
	}
	return stack.Definition.CargoUnitsPerItem
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func validateDropPercent(name string, percent float64) error {
	if !validDropPercent(percent) {
		return fmt.Errorf("%s %v: %w", name, percent, ErrInvalidCargoDropPercent)
	}
	return nil
}

func validDropPercent(percent float64) bool {
	return !math.IsNaN(percent) && !math.IsInf(percent, 0) && percent >= 0 && percent <= 1
}

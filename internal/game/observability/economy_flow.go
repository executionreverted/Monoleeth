package observability

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

// ValueFlowDirection classifies whether value entered or left the economy.
type ValueFlowDirection string

const (
	ValueFlowDirectionFaucet ValueFlowDirection = "faucet"
	ValueFlowDirectionSink   ValueFlowDirection = "sink"
)

// EconomyFlowValueKind distinguishes currency accounting from item accounting.
type EconomyFlowValueKind string

const (
	EconomyFlowValueKindCurrency EconomyFlowValueKind = "currency"
	EconomyFlowValueKindItem     EconomyFlowValueKind = "item"
)

// EconomyFlowEntry is one immutable observation of value creation or removal.
type EconomyFlowEntry struct {
	ValueKind   EconomyFlowValueKind      `json:"value_kind"`
	Currency    economy.CurrencyBucket    `json:"currency_type,omitempty"`
	ItemID      foundation.ItemID         `json:"item_id,omitempty"`
	Amount      int64                     `json:"amount"`
	Reason      economy.LedgerReason      `json:"reason"`
	ReferenceID foundation.IdempotencyKey `json:"reference_id"`
	Direction   ValueFlowDirection        `json:"direction"`
	Timestamp   time.Time                 `json:"timestamp"`
}

// CurrencyFlowSummary reports one currency faucet or sink total.
type CurrencyFlowSummary struct {
	Currency economy.CurrencyBucket `json:"currency_type"`
	Reason   economy.LedgerReason   `json:"reason"`
	Total    int64                  `json:"total"`
}

// ItemFlowSummary reports one item faucet or sink total.
type ItemFlowSummary struct {
	ItemID foundation.ItemID    `json:"item_id"`
	Reason economy.LedgerReason `json:"reason"`
	Total  int64                `json:"total"`
}

// EconomyFlowSnapshot is a deterministic clone of all value-flow summaries.
type EconomyFlowSnapshot struct {
	CurrencyFaucets []CurrencyFlowSummary `json:"currency_faucets,omitempty"`
	CurrencySinks   []CurrencyFlowSummary `json:"currency_sinks,omitempty"`
	ItemFaucets     []ItemFlowSummary     `json:"item_faucets,omitempty"`
	ItemSinks       []ItemFlowSummary     `json:"item_sinks,omitempty"`
}

type economyFlowReferenceKey struct {
	kind      EconomyFlowValueKind
	direction ValueFlowDirection
	reason    economy.LedgerReason
	reference foundation.IdempotencyKey
}

type economyFlowSummaryKey struct {
	kind      EconomyFlowValueKind
	direction ValueFlowDirection
	currency  economy.CurrencyBucket
	itemID    foundation.ItemID
	reason    economy.LedgerReason
}

// NewCurrencyFlowEntry returns a validated currency flow observation.
func NewCurrencyFlowEntry(
	currency economy.CurrencyBucket,
	amount int64,
	reason economy.LedgerReason,
	referenceID foundation.IdempotencyKey,
	direction ValueFlowDirection,
	timestamp time.Time,
) (EconomyFlowEntry, error) {
	entry := EconomyFlowEntry{
		ValueKind:   EconomyFlowValueKindCurrency,
		Currency:    currency,
		Amount:      amount,
		Reason:      reason,
		ReferenceID: referenceID,
		Direction:   direction,
		Timestamp:   timestamp,
	}
	if err := entry.Validate(); err != nil {
		return EconomyFlowEntry{}, err
	}
	return entry, nil
}

// NewItemFlowEntry returns a validated item flow observation.
func NewItemFlowEntry(
	itemID foundation.ItemID,
	amount int64,
	reason economy.LedgerReason,
	referenceID foundation.IdempotencyKey,
	direction ValueFlowDirection,
	timestamp time.Time,
) (EconomyFlowEntry, error) {
	entry := EconomyFlowEntry{
		ValueKind:   EconomyFlowValueKindItem,
		ItemID:      itemID,
		Amount:      amount,
		Reason:      reason,
		ReferenceID: referenceID,
		Direction:   direction,
		Timestamp:   timestamp,
	}
	if err := entry.Validate(); err != nil {
		return EconomyFlowEntry{}, err
	}
	return entry, nil
}

// String returns the stable wire representation.
func (direction ValueFlowDirection) String() string {
	return string(direction)
}

// Validate reports whether direction is supported.
func (direction ValueFlowDirection) Validate() error {
	switch direction {
	case ValueFlowDirectionFaucet, ValueFlowDirectionSink:
		return nil
	default:
		return fmt.Errorf("value flow direction %q: %w", direction, ErrInvalidValueFlowDirection)
	}
}

// String returns the stable wire representation.
func (kind EconomyFlowValueKind) String() string {
	return string(kind)
}

// Validate reports whether kind is supported.
func (kind EconomyFlowValueKind) Validate() error {
	switch kind {
	case EconomyFlowValueKindCurrency, EconomyFlowValueKindItem:
		return nil
	default:
		return fmt.Errorf("economy flow value kind %q: %w", kind, ErrInvalidEconomyFlowValueKind)
	}
}

// Validate reports whether entry is a complete, unambiguous value-flow observation.
func (entry EconomyFlowEntry) Validate() error {
	if err := entry.ValueKind.Validate(); err != nil {
		return err
	}
	if err := entry.Direction.Validate(); err != nil {
		return err
	}
	if err := foundation.ValidatePositiveAmount(entry.Amount); err != nil {
		return err
	}
	if entry.Reason.IsZero() {
		return ErrMissingEconomyFlowReason
	}
	if err := entry.Reason.Validate(); err != nil {
		return err
	}
	if entry.ReferenceID.IsZero() {
		return ErrMissingEconomyFlowReference
	}
	if err := entry.ReferenceID.Validate(); err != nil {
		return err
	}
	if entry.Timestamp.IsZero() {
		return ErrMissingEconomyFlowTimestamp
	}
	return entry.validateValueIdentity()
}

func (entry EconomyFlowEntry) validateValueIdentity() error {
	hasCurrency := !entry.Currency.IsZero()
	hasItem := !entry.ItemID.IsZero()
	if !hasCurrency && !hasItem {
		return ErrMissingEconomyFlowValueIdentity
	}
	if hasCurrency && hasItem {
		return ErrAmbiguousEconomyFlowValueIdentity
	}
	switch entry.ValueKind {
	case EconomyFlowValueKindCurrency:
		if !hasCurrency {
			return ErrMissingEconomyFlowValueIdentity
		}
		return entry.Currency.Validate()
	case EconomyFlowValueKindItem:
		if !hasItem {
			return ErrMissingEconomyFlowValueIdentity
		}
		return entry.ItemID.Validate()
	default:
		return fmt.Errorf("economy flow value kind %q: %w", entry.ValueKind, ErrInvalidEconomyFlowValueKind)
	}
}

// EconomyFlowAccumulator records duplicate-safe value-flow observations.
type EconomyFlowAccumulator struct {
	mu        sync.Mutex
	seen      map[economyFlowReferenceKey]struct{}
	summaries map[economyFlowSummaryKey]int64
}

// NewEconomyFlowAccumulator returns an empty value-flow accumulator.
func NewEconomyFlowAccumulator() *EconomyFlowAccumulator {
	return &EconomyFlowAccumulator{
		seen:      make(map[economyFlowReferenceKey]struct{}),
		summaries: make(map[economyFlowSummaryKey]int64),
	}
}

// Record validates entry and adds it to the duplicate-safe summaries.
func (accumulator *EconomyFlowAccumulator) Record(entry EconomyFlowEntry) error {
	if err := entry.Validate(); err != nil {
		return err
	}

	accumulator.mu.Lock()
	defer accumulator.mu.Unlock()
	accumulator.ensureMaps()

	referenceKey := entry.referenceKey()
	if _, exists := accumulator.seen[referenceKey]; exists {
		return fmt.Errorf("reference_id %q: %w", entry.ReferenceID, ErrDuplicateEconomyFlowReference)
	}

	accumulator.seen[referenceKey] = struct{}{}
	accumulator.summaries[entry.summaryKey()] += entry.Amount
	return nil
}

// Snapshot returns deterministic clones of all faucet and sink summaries.
func (accumulator *EconomyFlowAccumulator) Snapshot() EconomyFlowSnapshot {
	accumulator.mu.Lock()
	defer accumulator.mu.Unlock()

	snapshot := EconomyFlowSnapshot{
		CurrencyFaucets: make([]CurrencyFlowSummary, 0),
		CurrencySinks:   make([]CurrencyFlowSummary, 0),
		ItemFaucets:     make([]ItemFlowSummary, 0),
		ItemSinks:       make([]ItemFlowSummary, 0),
	}

	for key, total := range accumulator.summaries {
		switch {
		case key.kind == EconomyFlowValueKindCurrency && key.direction == ValueFlowDirectionFaucet:
			snapshot.CurrencyFaucets = append(snapshot.CurrencyFaucets, CurrencyFlowSummary{
				Currency: key.currency,
				Reason:   key.reason,
				Total:    total,
			})
		case key.kind == EconomyFlowValueKindCurrency && key.direction == ValueFlowDirectionSink:
			snapshot.CurrencySinks = append(snapshot.CurrencySinks, CurrencyFlowSummary{
				Currency: key.currency,
				Reason:   key.reason,
				Total:    total,
			})
		case key.kind == EconomyFlowValueKindItem && key.direction == ValueFlowDirectionFaucet:
			snapshot.ItemFaucets = append(snapshot.ItemFaucets, ItemFlowSummary{
				ItemID: key.itemID,
				Reason: key.reason,
				Total:  total,
			})
		case key.kind == EconomyFlowValueKindItem && key.direction == ValueFlowDirectionSink:
			snapshot.ItemSinks = append(snapshot.ItemSinks, ItemFlowSummary{
				ItemID: key.itemID,
				Reason: key.reason,
				Total:  total,
			})
		}
	}

	sort.Slice(snapshot.CurrencyFaucets, func(i, j int) bool {
		return lessCurrencyFlowSummary(snapshot.CurrencyFaucets[i], snapshot.CurrencyFaucets[j])
	})
	sort.Slice(snapshot.CurrencySinks, func(i, j int) bool {
		return lessCurrencyFlowSummary(snapshot.CurrencySinks[i], snapshot.CurrencySinks[j])
	})
	sort.Slice(snapshot.ItemFaucets, func(i, j int) bool {
		return lessItemFlowSummary(snapshot.ItemFaucets[i], snapshot.ItemFaucets[j])
	})
	sort.Slice(snapshot.ItemSinks, func(i, j int) bool {
		return lessItemFlowSummary(snapshot.ItemSinks[i], snapshot.ItemSinks[j])
	})

	return snapshot
}

func (accumulator *EconomyFlowAccumulator) ensureMaps() {
	if accumulator.seen == nil {
		accumulator.seen = make(map[economyFlowReferenceKey]struct{})
	}
	if accumulator.summaries == nil {
		accumulator.summaries = make(map[economyFlowSummaryKey]int64)
	}
}

func (entry EconomyFlowEntry) referenceKey() economyFlowReferenceKey {
	return economyFlowReferenceKey{
		kind:      entry.ValueKind,
		direction: entry.Direction,
		reason:    entry.Reason,
		reference: entry.ReferenceID,
	}
}

func (entry EconomyFlowEntry) summaryKey() economyFlowSummaryKey {
	return economyFlowSummaryKey{
		kind:      entry.ValueKind,
		direction: entry.Direction,
		currency:  entry.Currency,
		itemID:    entry.ItemID,
		reason:    entry.Reason,
	}
}

func lessCurrencyFlowSummary(left, right CurrencyFlowSummary) bool {
	if left.Currency != right.Currency {
		return left.Currency < right.Currency
	}
	return left.Reason < right.Reason
}

func lessItemFlowSummary(left, right ItemFlowSummary) bool {
	if left.ItemID != right.ItemID {
		return left.ItemID < right.ItemID
	}
	return left.Reason < right.Reason
}

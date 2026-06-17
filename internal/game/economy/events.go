package economy

import (
	"encoding/json"
	"fmt"
	"time"

	"gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
)

const (
	EventInventoryItemAdded   = "inventory.item_added"
	EventInventoryItemMoved   = "inventory.item_moved"
	EventInventoryItemRemoved = "inventory.item_removed"
	EventCargoUpdated         = "cargo.updated"
	EventWalletCredited       = "wallet.credited"
	EventWalletDebited        = "wallet.debited"
	EventLedgerEntryCreated   = "ledger.entry_created"
)

const (
	ledgerAssetTypeItem     = "item"
	ledgerAssetTypeCurrency = "currency"
)

// EventEmitter is the minimal hook economy services use after successful
// in-memory mutation. testutil.EventRecorder satisfies this interface.
type EventEmitter interface {
	Record(events.EventEnvelope)
}

type InventoryItemAddedPayload struct {
	PlayerID        foundation.PlayerID       `json:"player_id"`
	ItemID          foundation.ItemID         `json:"item_id"`
	ItemInstanceIDs []foundation.ItemID       `json:"item_instance_ids,omitempty"`
	Quantity        int64                     `json:"quantity"`
	Location        ItemLocation              `json:"location"`
	Reason          LedgerReason              `json:"reason"`
	ReferenceKey    foundation.IdempotencyKey `json:"reference_id"`
	LedgerID        LedgerID                  `json:"ledger_id"`
}

type InventoryItemMovedPayload struct {
	PlayerID       foundation.PlayerID       `json:"player_id"`
	ItemID         foundation.ItemID         `json:"item_id"`
	ItemInstanceID foundation.ItemID         `json:"item_instance_id,omitempty"`
	Quantity       int64                     `json:"quantity"`
	FromLocation   ItemLocation              `json:"from_location"`
	ToLocation     ItemLocation              `json:"to_location"`
	Reason         LedgerReason              `json:"reason"`
	ReferenceKey   foundation.IdempotencyKey `json:"reference_id"`
	LedgerIDs      []LedgerID                `json:"ledger_ids"`
}

type InventoryItemRemovedPayload struct {
	PlayerID       foundation.PlayerID       `json:"player_id"`
	ItemID         foundation.ItemID         `json:"item_id"`
	ItemInstanceID foundation.ItemID         `json:"item_instance_id,omitempty"`
	Quantity       int64                     `json:"quantity"`
	Location       ItemLocation              `json:"location"`
	Reason         LedgerReason              `json:"reason"`
	ReferenceKey   foundation.IdempotencyKey `json:"reference_id"`
	LedgerID       LedgerID                  `json:"ledger_id"`
}

type CargoUpdatedPayload struct {
	PlayerID     foundation.PlayerID       `json:"player_id"`
	ItemID       foundation.ItemID         `json:"item_id"`
	Quantity     int64                     `json:"quantity"`
	Location     ItemLocation              `json:"location"`
	Reason       LedgerReason              `json:"reason"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
	LedgerID     LedgerID                  `json:"ledger_id"`
}

type WalletMutationPayload struct {
	PlayerID     foundation.PlayerID       `json:"player_id"`
	Currency     CurrencyBucket            `json:"currency_type"`
	Amount       int64                     `json:"amount"`
	BalanceAfter int64                     `json:"balance_after"`
	Reason       LedgerReason              `json:"reason"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
	LedgerID     LedgerID                  `json:"ledger_id"`
}

type LedgerEntryCreatedPayload struct {
	LedgerID       LedgerID                  `json:"ledger_id"`
	AssetType      string                    `json:"asset_type"`
	PlayerID       foundation.PlayerID       `json:"player_id"`
	ItemID         foundation.ItemID         `json:"item_id,omitempty"`
	ItemInstanceID foundation.ItemID         `json:"item_instance_id,omitempty"`
	Currency       CurrencyBucket            `json:"currency_type,omitempty"`
	Quantity       int64                     `json:"quantity,omitempty"`
	Amount         int64                     `json:"amount,omitempty"`
	Action         LedgerAction              `json:"action"`
	BalanceAfter   int64                     `json:"balance_after"`
	Location       *ItemLocation             `json:"location,omitempty"`
	Reason         LedgerReason              `json:"reason"`
	ReferenceKey   foundation.IdempotencyKey `json:"reference_id"`
}

// SetEventEmitter configures the optional post-mutation event hook.
func (service *InventoryService) SetEventEmitter(emitter EventEmitter) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.emitter = emitter
}

// SetEventEmitter configures the optional post-mutation event hook.
func (service *WalletService) SetEventEmitter(emitter EventEmitter) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.emitter = emitter
}

// SetEventEmitter configures the optional post-mutation event hook.
func (service *CargoService) SetEventEmitter(emitter EventEmitter) {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.emitter = emitter
}

func emitEvents(emitter EventEmitter, emitted []events.EventEnvelope) {
	if emitter == nil {
		return
	}
	for _, event := range emitted {
		emitter.Record(event)
	}
}

func newEconomyEvent(prefix string, sequence uint64, eventType string, payload any, now time.Time) events.EventEnvelope {
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		rawPayload = json.RawMessage(`{}`)
	}
	return events.NewEventEnvelope(
		foundation.EventID(fmt.Sprintf("%s-%d", prefix, sequence)),
		eventType,
		rawPayload,
		now.UnixMilli(),
		sequence,
	)
}

func itemInstanceIDsFromAddResult(result AddItemResult) []foundation.ItemID {
	ids := make([]foundation.ItemID, 0, len(result.StackableItems)+len(result.InstanceItems))
	for _, item := range result.StackableItems {
		ids = append(ids, item.ItemInstanceID)
	}
	for _, item := range result.InstanceItems {
		ids = append(ids, item.ItemInstanceID)
	}
	return ids
}

func ledgerIDsFromItemEntries(entries []ItemLedgerEntry) []LedgerID {
	ids := make([]LedgerID, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.LedgerID)
	}
	return ids
}

func itemLedgerEntryCreatedPayload(entry ItemLedgerEntry) LedgerEntryCreatedPayload {
	location := entry.Location
	return LedgerEntryCreatedPayload{
		LedgerID:       entry.LedgerID,
		AssetType:      ledgerAssetTypeItem,
		PlayerID:       entry.PlayerID,
		ItemID:         entry.ItemID,
		ItemInstanceID: entry.ItemInstanceID,
		Quantity:       entry.Quantity.Int64(),
		Action:         entry.Action,
		BalanceAfter:   entry.BalanceAfter,
		Location:       &location,
		Reason:         entry.Reason,
		ReferenceKey:   entry.ReferenceKey,
	}
}

func currencyLedgerEntryCreatedPayload(entry CurrencyLedgerEntry) LedgerEntryCreatedPayload {
	return LedgerEntryCreatedPayload{
		LedgerID:     entry.LedgerID,
		AssetType:    ledgerAssetTypeCurrency,
		PlayerID:     entry.PlayerID,
		Currency:     entry.Currency,
		Amount:       entry.Amount.Int64(),
		Action:       entry.Action,
		BalanceAfter: entry.BalanceAfter,
		Reason:       entry.Reason,
		ReferenceKey: entry.ReferenceKey,
	}
}

func (service *InventoryService) newEventLocked(eventType string, payload any, now time.Time) events.EventEnvelope {
	service.nextEventSequence++
	return newEconomyEvent("inventory-event", service.nextEventSequence, eventType, payload, now)
}

func (service *WalletService) newEventLocked(eventType string, payload any, now time.Time) events.EventEnvelope {
	service.nextEventSequence++
	return newEconomyEvent("wallet-event", service.nextEventSequence, eventType, payload, now)
}

func (service *CargoService) newEventLocked(eventType string, payload any, now time.Time) events.EventEnvelope {
	service.nextEventSequence++
	return newEconomyEvent("cargo-event", service.nextEventSequence, eventType, payload, now)
}

func (service *InventoryService) addItemEventsLocked(input AddItemInput, result AddItemResult, now time.Time) []events.EventEnvelope {
	emitted := []events.EventEnvelope{
		service.newEventLocked(EventInventoryItemAdded, InventoryItemAddedPayload{
			PlayerID:        input.PlayerID,
			ItemID:          input.ItemDefinition.ItemID,
			ItemInstanceIDs: itemInstanceIDsFromAddResult(result),
			Quantity:        input.Quantity,
			Location:        input.Location,
			Reason:          input.Reason,
			ReferenceKey:    input.ReferenceKey,
			LedgerID:        result.LedgerEntry.LedgerID,
		}, now),
	}
	return append(emitted, service.itemLedgerEventsLocked([]ItemLedgerEntry{result.LedgerEntry}, now)...)
}

func (service *InventoryService) moveItemEventsLocked(input MoveItemInput, result MoveItemResult, now time.Time) []events.EventEnvelope {
	emitted := []events.EventEnvelope{
		service.newEventLocked(EventInventoryItemMoved, InventoryItemMovedPayload{
			PlayerID:       input.PlayerID,
			ItemID:         input.ItemRef.Definition.ItemID,
			ItemInstanceID: input.ItemRef.ItemInstanceID,
			Quantity:       input.Quantity,
			FromLocation:   input.FromLocation,
			ToLocation:     input.ToLocation,
			Reason:         input.Reason,
			ReferenceKey:   input.ReferenceKey,
			LedgerIDs:      ledgerIDsFromItemEntries(result.LedgerEntries),
		}, now),
	}
	return append(emitted, service.itemLedgerEventsLocked(result.LedgerEntries, now)...)
}

func (service *InventoryService) moveResultsEventsLocked(inputs []MoveItemInput, results []MoveItemResult, now time.Time) []events.EventEnvelope {
	emitted := make([]events.EventEnvelope, 0)
	for index, result := range results {
		if result.Duplicate {
			continue
		}
		emitted = append(emitted, service.moveItemEventsLocked(inputs[index], result, now)...)
	}
	return emitted
}

func (service *InventoryService) removeItemEventsLocked(input RemoveItemInput, result RemoveItemResult, now time.Time) []events.EventEnvelope {
	emitted := []events.EventEnvelope{
		service.newEventLocked(EventInventoryItemRemoved, InventoryItemRemovedPayload{
			PlayerID:       input.PlayerID,
			ItemID:         input.ItemRef.Definition.ItemID,
			ItemInstanceID: input.ItemRef.ItemInstanceID,
			Quantity:       input.Quantity,
			Location:       input.SourceLocation,
			Reason:         input.Reason,
			ReferenceKey:   input.ReferenceKey,
			LedgerID:       result.LedgerEntries[0].LedgerID,
		}, now),
	}
	return append(emitted, service.itemLedgerEventsLocked(result.LedgerEntries, now)...)
}

func (service *InventoryService) itemLedgerEventsLocked(entries []ItemLedgerEntry, now time.Time) []events.EventEnvelope {
	emitted := make([]events.EventEnvelope, 0, len(entries))
	for _, entry := range entries {
		emitted = append(emitted, service.newEventLocked(EventLedgerEntryCreated, itemLedgerEntryCreatedPayload(entry), now))
	}
	return emitted
}

func (service *WalletService) walletMutationEventLocked(eventType string, payload WalletMutationPayload, now time.Time) events.EventEnvelope {
	return service.newEventLocked(eventType, payload, now)
}

func (service *WalletService) currencyLedgerEventsLocked(entries []CurrencyLedgerEntry, now time.Time) []events.EventEnvelope {
	emitted := make([]events.EventEnvelope, 0, len(entries))
	for _, entry := range entries {
		emitted = append(emitted, service.newEventLocked(EventLedgerEntryCreated, currencyLedgerEntryCreatedPayload(entry), now))
	}
	return emitted
}

func (service *CargoService) cargoUpdatedEventLocked(input CargoAddItemInput, result AddItemResult, now time.Time) events.EventEnvelope {
	return service.newEventLocked(EventCargoUpdated, CargoUpdatedPayload{
		PlayerID:     input.PlayerID,
		ItemID:       input.ItemDefinition.ItemID,
		Quantity:     input.Quantity,
		Location:     input.ActiveCargo,
		Reason:       input.Reason,
		ReferenceKey: input.ReferenceKey,
		LedgerID:     result.LedgerEntry.LedgerID,
	}, now)
}

func (service *CargoService) cargoUpdatedMoveEventLocked(input CargoMoveItemInput, result MoveItemResult, now time.Time) events.EventEnvelope {
	var ledgerID LedgerID
	if len(result.LedgerEntries) > 0 {
		ledgerID = result.LedgerEntries[len(result.LedgerEntries)-1].LedgerID
	}
	return service.newEventLocked(EventCargoUpdated, CargoUpdatedPayload{
		PlayerID:     input.PlayerID,
		ItemID:       input.ItemRef.Definition.ItemID,
		Quantity:     input.Quantity,
		Location:     input.ActiveCargo,
		Reason:       input.Reason,
		ReferenceKey: input.ReferenceKey,
		LedgerID:     ledgerID,
	}, now)
}

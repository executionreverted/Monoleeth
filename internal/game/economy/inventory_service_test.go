package economy

import (
	"context"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestAddItemRejectsNegativeQuantity(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)
	input.Quantity = -1

	_, err := service.AddItem(input)
	if !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("AddItem negative quantity error = %v, want foundation.ErrNonPositiveAmount", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.Location); got != 0 {
		t.Fatalf("TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 0 {
		t.Fatalf("ledger entries len = %d, want 0", got)
	}
}

func TestAddItemRejectsZeroQuantity(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)
	input.Quantity = 0

	_, err := service.AddItem(input)
	if !errors.Is(err, foundation.ErrNonPositiveAmount) {
		t.Fatalf("AddItem zero quantity error = %v, want foundation.ErrNonPositiveAmount", err)
	}
	if got := service.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.Location); got != 0 {
		t.Fatalf("TotalItemQuantity() = %d, want 0", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 0 {
		t.Fatalf("ledger entries len = %d, want 0", got)
	}
}

func TestAddItemValidatesRequiredInputs(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*AddItemInput)
		wantErr error
	}{
		{
			name: "blank player",
			mutate: func(input *AddItemInput) {
				input.PlayerID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "blank item",
			mutate: func(input *AddItemInput) {
				input.ItemDefinition.ItemID = ""
			},
			wantErr: foundation.ErrEmptyID,
		},
		{
			name: "blank location",
			mutate: func(input *AddItemInput) {
				input.Location = ItemLocation{}
			},
			wantErr: ErrInvalidLocationKind,
		},
		{
			name: "blank reason",
			mutate: func(input *AddItemInput) {
				input.Reason = ""
			},
			wantErr: ErrEmptyLedgerReason,
		},
		{
			name: "blank reference",
			mutate: func(input *AddItemInput) {
				input.ReferenceKey = ""
			},
			wantErr: foundation.ErrEmptyIdempotencyKey,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestInventoryService()
			input := validAddItemInput(t)
			tc.mutate(&input)

			_, err := service.AddItem(input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("AddItem error = %v, want %v", err, tc.wantErr)
			}
			if got := len(service.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestAddItemRejectsGenericCargoAndEquippedTargets(t *testing.T) {
	cases := []ItemLocation{
		validShipCargoLocation(t),
		{Kind: LocationKindShipEquipped, ID: "ship-1"},
	}

	for _, location := range cases {
		t.Run(location.Kind.String(), func(t *testing.T) {
			service := newTestInventoryService()
			input := validAddItemInput(t)
			input.Location = location

			_, err := service.AddItem(input)
			if !errors.Is(err, ErrBlockedGenericMoveTarget) {
				t.Fatalf("AddItem error = %v, want ErrBlockedGenericMoveTarget", err)
			}
			if got := service.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.Location); got != 0 {
				t.Fatalf("TotalItemQuantity() = %d, want 0", got)
			}
			if got := len(service.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestAddItemDuplicateReferenceDoesNotDuplicateGrant(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)

	first, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("first AddItem: %v", err)
	}
	second, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("duplicate AddItem: %v", err)
	}

	if first.Duplicate {
		t.Fatal("first AddItem Duplicate = true, want false")
	}
	if !second.Duplicate {
		t.Fatal("duplicate AddItem Duplicate = false, want true")
	}
	if got := service.TotalItemQuantity(input.PlayerID, input.ItemDefinition.ItemID, input.Location); got != input.Quantity {
		t.Fatalf("TotalItemQuantity() = %d, want %d", got, input.Quantity)
	}
	if got := len(service.StackableItems()); got != 1 {
		t.Fatalf("stackable items len = %d, want 1", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
	if second.LedgerEntry.LedgerID != first.LedgerEntry.LedgerID {
		t.Fatalf("duplicate LedgerID = %q, want %q", second.LedgerEntry.LedgerID, first.LedgerEntry.LedgerID)
	}
}

func TestAddItemInstanceAcceptsServerAuthoredItemInstanceID(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)
	input.ItemDefinition = validInstanceDefinition(t)
	input.ItemInstanceID = "coordinate-scroll-instance-1"
	input.Quantity = 1

	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("AddItem explicit instance: %v", err)
	}
	if len(result.InstanceItems) != 1 || result.InstanceItems[0].ItemInstanceID != input.ItemInstanceID {
		t.Fatalf("instance items = %+v, want explicit id %s", result.InstanceItems, input.ItemInstanceID)
	}
	instances := service.InstanceItems()
	if len(instances) != 1 || instances[0].ItemInstanceID != input.ItemInstanceID {
		t.Fatalf("stored instances = %+v, want explicit id %s", instances, input.ItemInstanceID)
	}
	if result.LedgerEntry.ItemInstanceID != input.ItemInstanceID {
		t.Fatalf("ledger item instance = %q, want %q", result.LedgerEntry.ItemInstanceID, input.ItemInstanceID)
	}
}

func TestAddItemRejectsExplicitItemInstanceIDForStackableOrMultiInstanceGrant(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AddItemInput)
	}{
		{
			name: "stackable",
			mutate: func(input *AddItemInput) {
				input.ItemInstanceID = "explicit-stack"
			},
		},
		{
			name: "multi instance",
			mutate: func(input *AddItemInput) {
				input.ItemDefinition = validInstanceDefinition(t)
				input.ItemInstanceID = "explicit-instance"
				input.Quantity = 2
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			service := newTestInventoryService()
			input := validAddItemInput(t)
			tc.mutate(&input)

			_, err := service.AddItem(input)
			if !errors.Is(err, ErrInvalidInstanceQuantity) {
				t.Fatalf("AddItem error = %v, want ErrInvalidInstanceQuantity", err)
			}
			if got := len(service.ItemLedgerEntries()); got != 0 {
				t.Fatalf("ledger entries len = %d, want 0", got)
			}
		})
	}
}

func TestAddItemRejectsExplicitItemInstanceIDCollisionAcrossReferences(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)
	input.ItemDefinition = validInstanceDefinition(t)
	input.ItemInstanceID = "coordinate-scroll-instance-1"
	input.Quantity = 1

	if _, err := service.AddItem(input); err != nil {
		t.Fatalf("first AddItem: %v", err)
	}
	conflict := input
	conflict.ReferenceKey = "shop_purchase:player-1:request-conflict"
	_, err := service.AddItem(conflict)
	if !errors.Is(err, ErrItemInstanceAlreadyExists) {
		t.Fatalf("conflicting AddItem error = %v, want ErrItemInstanceAlreadyExists", err)
	}
	if got := len(service.InstanceItems()); got != 1 {
		t.Fatalf("instance items len = %d, want 1", got)
	}
	if got := len(service.ItemLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestAddItemWritesItemLedgerEntryWithReasonAndReference(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)

	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	entries := service.ItemLedgerEntries()
	if len(entries) != 1 {
		t.Fatalf("ledger entries len = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.LedgerID.IsZero() {
		t.Fatal("ledger id is zero")
	}
	if entry.PlayerID != input.PlayerID {
		t.Fatalf("ledger player = %q, want %q", entry.PlayerID, input.PlayerID)
	}
	if entry.ItemID != input.ItemDefinition.ItemID {
		t.Fatalf("ledger item = %q, want %q", entry.ItemID, input.ItemDefinition.ItemID)
	}
	if got := entry.Quantity.Int64(); got != input.Quantity {
		t.Fatalf("ledger quantity = %d, want %d", got, input.Quantity)
	}
	if entry.Action != LedgerActionIncrease {
		t.Fatalf("ledger action = %q, want %q", entry.Action, LedgerActionIncrease)
	}
	if entry.BalanceAfter != input.Quantity {
		t.Fatalf("ledger balance after = %d, want %d", entry.BalanceAfter, input.Quantity)
	}
	if entry.Location != input.Location {
		t.Fatalf("ledger location = %v, want %v", entry.Location, input.Location)
	}
	if entry.Reason != input.Reason {
		t.Fatalf("ledger reason = %q, want %q", entry.Reason, input.Reason)
	}
	if entry.ReferenceKey != input.ReferenceKey {
		t.Fatalf("ledger reference = %q, want %q", entry.ReferenceKey, input.ReferenceKey)
	}
	if entry.CreatedAt != testInventoryNow {
		t.Fatalf("ledger created at = %s, want %s", entry.CreatedAt, testInventoryNow)
	}
	if result.LedgerEntry != entry {
		t.Fatalf("result ledger entry = %#v, want %#v", result.LedgerEntry, entry)
	}
}

func TestAddItemSplitsStackableRowsByDefinitionMaxStack(t *testing.T) {
	service := newTestInventoryService()
	input := validAddItemInput(t)
	input.Quantity = 250

	result, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("AddItem: %v", err)
	}

	if got := len(result.StackableItems); got != 3 {
		t.Fatalf("stackable result len = %d, want 3", got)
	}
	wantQuantities := []int64{100, 100, 50}
	for index, item := range result.StackableItems {
		if got := item.Quantity.Int64(); got != wantQuantities[index] {
			t.Fatalf("stack %d quantity = %d, want %d", index, got, wantQuantities[index])
		}
	}
	if got := result.LedgerEntry.Quantity.Int64(); got != input.Quantity {
		t.Fatalf("ledger quantity = %d, want %d", got, input.Quantity)
	}
	if got := result.LedgerEntry.BalanceAfter; got != input.Quantity {
		t.Fatalf("ledger balance after = %d, want %d", got, input.Quantity)
	}
}

var testInventoryNow = time.Date(2026, 6, 17, 15, 0, 0, 0, time.UTC)

func TestNewInventoryServiceWithRepositoryLoadsPersistedStackableItems(t *testing.T) {
	stack := stackableItemForTest(t)
	repository := &fakeInventoryRepository{stackables: []StackableItem{stack}}

	service, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}

	if got := service.TotalItemQuantity(stack.OwnerPlayerID, stack.ItemID, stack.Location); got != stack.Quantity.Int64() {
		t.Fatalf("TotalItemQuantity() = %d, want loaded %d", got, stack.Quantity.Int64())
	}
}

func TestNewInventoryServiceWithRepositoryLoadsPersistedInstanceItems(t *testing.T) {
	instance := instanceItemForTest(t)
	repository := &fakeInventoryRepository{instances: []InstanceItem{instance}}

	service, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}

	instances := service.InstanceItems()
	if len(instances) != 1 || instances[0].ItemInstanceID != instance.ItemInstanceID {
		t.Fatalf("InstanceItems() = %+v, want loaded instance %+v", instances, instance)
	}
}

func TestAddItemPersistsStackableThroughRepository(t *testing.T) {
	repository := &fakeInventoryRepository{}
	service, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}

	if _, err := service.AddItem(validAddItemInput(t)); err != nil {
		t.Fatalf("AddItem() error = %v, want nil", err)
	}

	if len(repository.upserts) == 0 {
		t.Fatal("AddItem did not persist any stackable rows through repository")
	}
}

func TestAddItemPersistsInstanceThroughRepository(t *testing.T) {
	repository := &fakeInventoryRepository{}
	service, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}
	input := validAddItemInput(t)
	input.ItemDefinition = validInstanceDefinition(t)
	input.ItemInstanceID = "coordinate-scroll-instance-persisted"
	input.Quantity = 1

	if _, err := service.AddItem(input); err != nil {
		t.Fatalf("AddItem() error = %v, want nil", err)
	}

	if len(repository.instanceUpserts) != 1 || repository.instanceUpserts[0].ItemInstanceID != input.ItemInstanceID {
		t.Fatalf("instance upserts = %+v, want one persisted instance %s", repository.instanceUpserts, input.ItemInstanceID)
	}
}

func TestInventoryRepositoryAddItemReferenceSurvivesServiceReload(t *testing.T) {
	repository := &fakeInventoryRepository{}
	service, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}
	input := validAddItemInput(t)
	input.ItemDefinition = validInstanceDefinition(t)
	input.ItemInstanceID = "coordinate-scroll-instance-reload-ref"
	input.Quantity = 1

	first, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("first AddItem() error = %v, want nil", err)
	}
	reloaded, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository(reload) error = %v, want nil", err)
	}

	second, err := reloaded.AddItem(input)
	if err != nil {
		t.Fatalf("duplicate AddItem() after reload error = %v, want nil", err)
	}

	if !second.Duplicate {
		t.Fatal("duplicate AddItem after reload Duplicate = false, want true")
	}
	if second.LedgerEntry.LedgerID != first.LedgerEntry.LedgerID {
		t.Fatalf("duplicate ledger = %q, want %q", second.LedgerEntry.LedgerID, first.LedgerEntry.LedgerID)
	}
	if got := len(repository.instances); got != 1 {
		t.Fatalf("repository instances len = %d, want 1", got)
	}
	if got := len(repository.commits); got != 1 {
		t.Fatalf("repository commits len = %d, want 1", got)
	}
}

func TestInventoryRepositoryMoveItemReferenceSurvivesServiceReload(t *testing.T) {
	repository := &fakeInventoryRepository{}
	service, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}
	definition := validStackableDefinition(t)
	fromLocation := validLocation(t)
	toLocation := validStationStorageLocation(t)
	addStackableItems(t, service, definition, 75, fromLocation, "loot_pickup:move-reload-seed")

	input := validMoveItemInput(t)
	input.ItemRef.Definition = definition
	input.FromLocation = fromLocation
	input.ToLocation = toLocation
	input.Quantity = 30
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:move-reload")

	first, err := service.MoveItem(input)
	if err != nil {
		t.Fatalf("first MoveItem() error = %v, want nil", err)
	}
	reloaded, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository(reload) error = %v, want nil", err)
	}

	second, err := reloaded.MoveItem(input)
	if err != nil {
		t.Fatalf("duplicate MoveItem() after reload error = %v, want nil", err)
	}

	if !second.Duplicate {
		t.Fatal("duplicate MoveItem after reload Duplicate = false, want true")
	}
	if second.LedgerEntries[0].LedgerID != first.LedgerEntries[0].LedgerID ||
		second.LedgerEntries[1].LedgerID != first.LedgerEntries[1].LedgerID {
		t.Fatalf("duplicate ledgers = %+v, want %+v", second.LedgerEntries, first.LedgerEntries)
	}
	if got := reloaded.TotalItemQuantity(input.PlayerID, definition.ItemID, fromLocation); got != 45 {
		t.Fatalf("source TotalItemQuantity() after duplicate reload = %d, want 45", got)
	}
	if got := reloaded.TotalItemQuantity(input.PlayerID, definition.ItemID, toLocation); got != 30 {
		t.Fatalf("destination TotalItemQuantity() after duplicate reload = %d, want 30", got)
	}
	if got := len(repository.moveCommits); got != 1 {
		t.Fatalf("repository move commits len = %d, want 1", got)
	}
}

func TestInventoryRepositoryRemoveItemReferenceSurvivesServiceReload(t *testing.T) {
	repository := &fakeInventoryRepository{}
	service, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}
	definition := validStackableDefinition(t)
	sourceLocation := validLocation(t)
	addStackableItems(t, service, definition, 75, sourceLocation, "loot_pickup:remove-reload-seed")

	input := validRemoveItemInput(t)
	input.ItemRef.Definition = definition
	input.SourceLocation = sourceLocation
	input.Quantity = 30
	input.ReferenceKey = validReferenceKey(t, "loot_pickup:remove-reload")

	first, err := service.RemoveItem(input)
	if err != nil {
		t.Fatalf("first RemoveItem() error = %v, want nil", err)
	}
	reloaded, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository(reload) error = %v, want nil", err)
	}

	second, err := reloaded.RemoveItem(input)
	if err != nil {
		t.Fatalf("duplicate RemoveItem() after reload error = %v, want nil", err)
	}

	if !second.Duplicate {
		t.Fatal("duplicate RemoveItem after reload Duplicate = false, want true")
	}
	if second.LedgerEntries[0].LedgerID != first.LedgerEntries[0].LedgerID {
		t.Fatalf("duplicate ledger = %q, want %q", second.LedgerEntries[0].LedgerID, first.LedgerEntries[0].LedgerID)
	}
	if got := reloaded.TotalItemQuantity(input.PlayerID, definition.ItemID, sourceLocation); got != 45 {
		t.Fatalf("source TotalItemQuantity() after duplicate reload = %d, want 45", got)
	}
	if got := len(repository.removeCommits); got != 1 {
		t.Fatalf("repository remove commits len = %d, want 1", got)
	}
}

func TestInventoryRepositoryCountersAvoidGeneratedInstanceIDCollisionAfterReload(t *testing.T) {
	repository := &fakeInventoryRepository{}
	service, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}
	input := validAddItemInput(t)
	input.ItemDefinition = validInstanceDefinition(t)
	input.Quantity = 1

	first, err := service.AddItem(input)
	if err != nil {
		t.Fatalf("first AddItem() error = %v, want nil", err)
	}
	reloaded, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository(reload) error = %v, want nil", err)
	}
	next := input
	next.ReferenceKey = validReferenceKey(t, "loot_pickup:drop-after-reload")

	second, err := reloaded.AddItem(next)
	if err != nil {
		t.Fatalf("second AddItem() after reload error = %v, want nil", err)
	}

	firstID := first.InstanceItems[0].ItemInstanceID
	secondID := second.InstanceItems[0].ItemInstanceID
	if secondID == firstID {
		t.Fatalf("generated instance id after reload = %q, want non-colliding id", secondID)
	}
	if got := len(repository.instances); got != 2 {
		t.Fatalf("repository instances len = %d, want 2", got)
	}
}

func TestSystemSetInstanceDurabilityPersistsInstanceThroughRepository(t *testing.T) {
	instance := instanceItemForTest(t)
	repository := &fakeInventoryRepository{instances: []InstanceItem{instance}}
	service, err := NewInventoryServiceWithRepository(testutil.NewFakeClock(testInventoryNow), repository)
	if err != nil {
		t.Fatalf("NewInventoryServiceWithRepository() error = %v, want nil", err)
	}

	updated, err := service.SystemSetInstanceDurability(instance.OwnerPlayerID, instance.ItemInstanceID, 33)
	if err != nil {
		t.Fatalf("SystemSetInstanceDurability() error = %v, want nil", err)
	}

	if updated.DurabilityCurrent != 33 {
		t.Fatalf("DurabilityCurrent = %d, want 33", updated.DurabilityCurrent)
	}
	if len(repository.instanceUpserts) != 1 || repository.instanceUpserts[0].DurabilityCurrent != 33 {
		t.Fatalf("instance upserts = %+v, want durability 33", repository.instanceUpserts)
	}
}

func stackableItemForTest(t *testing.T) StackableItem {
	t.Helper()
	definition := validStackableDefinition(t)
	stack, err := NewStackableItem(
		definition.Source,
		foundation.ItemID("stack-instance-load-1"),
		definition.ItemID,
		foundation.PlayerID("player-1"),
		validLocation(t),
		mustQuantity(t, 7),
	)
	if err != nil {
		t.Fatalf("NewStackableItem() error = %v, want nil", err)
	}
	stack.CreatedAt = testInventoryNow
	stack.UpdatedAt = testInventoryNow
	return stack
}

func instanceItemForTest(t *testing.T) InstanceItem {
	t.Helper()
	definition := validInstanceDefinition(t)
	instance, err := NewInstanceItem(
		definition.Source,
		foundation.ItemID("coordinate-scroll-instance-load-1"),
		definition.ItemID,
		foundation.PlayerID("player-1"),
		validLocation(t),
		mustQuantity(t, 1),
	)
	if err != nil {
		t.Fatalf("NewInstanceItem() error = %v, want nil", err)
	}
	instance.DurabilityCurrent = 77
	instance.BoundState = BoundStateAccountBound
	instance.CreatedAt = testInventoryNow
	instance.UpdatedAt = testInventoryNow
	return instance
}

func mustQuantity(t *testing.T, amount int64) foundation.Quantity {
	t.Helper()
	quantity, err := foundation.NewQuantity(amount)
	if err != nil {
		t.Fatalf("NewQuantity(%d) error = %v, want nil", amount, err)
	}
	return quantity
}

type fakeInventoryRepository struct {
	stackables       []StackableItem
	instances        []InstanceItem
	ledgerEntries    []ItemLedgerEntry
	references       []AddItemReference
	moveReferences   []MoveItemReference
	removeReferences []RemoveItemReference
	counters         InventoryCounters
	commits          []InventoryAddItemCommit
	moveCommits      []InventoryMoveItemCommit
	removeCommits    []InventoryRemoveItemCommit
	upserts          []StackableItem
	instanceUpserts  []InstanceItem
}

func (repository *fakeInventoryRepository) LoadStackableItems(context.Context) ([]StackableItem, error) {
	return append([]StackableItem(nil), repository.stackables...), nil
}

func (repository *fakeInventoryRepository) LoadInstanceItems(context.Context) ([]InstanceItem, error) {
	return append([]InstanceItem(nil), repository.instances...), nil
}

func (repository *fakeInventoryRepository) LoadItemLedgerEntries(context.Context) ([]ItemLedgerEntry, error) {
	return append([]ItemLedgerEntry(nil), repository.ledgerEntries...), nil
}

func (repository *fakeInventoryRepository) LoadAddItemReferences(context.Context) ([]AddItemReference, error) {
	references := make([]AddItemReference, 0, len(repository.references))
	for _, reference := range repository.references {
		reference.Result = cloneAddItemResult(reference.Result)
		references = append(references, reference)
	}
	return references, nil
}

func (repository *fakeInventoryRepository) LoadMoveItemReferences(context.Context) ([]MoveItemReference, error) {
	references := make([]MoveItemReference, 0, len(repository.moveReferences))
	for _, reference := range repository.moveReferences {
		reference.Result = cloneMoveItemResult(reference.Result)
		references = append(references, reference)
	}
	return references, nil
}

func (repository *fakeInventoryRepository) LoadRemoveItemReferences(context.Context) ([]RemoveItemReference, error) {
	references := make([]RemoveItemReference, 0, len(repository.removeReferences))
	for _, reference := range repository.removeReferences {
		reference.Result = cloneRemoveItemResult(reference.Result)
		references = append(references, reference)
	}
	return references, nil
}

func (repository *fakeInventoryRepository) LoadInventoryCounters(context.Context) (InventoryCounters, error) {
	return repository.counters, nil
}

func (repository *fakeInventoryRepository) UpsertStackableItem(_ context.Context, item StackableItem) error {
	repository.upserts = append(repository.upserts, item)
	for index := range repository.stackables {
		if repository.stackables[index].ItemInstanceID == item.ItemInstanceID {
			repository.stackables[index] = item
			return nil
		}
	}
	repository.stackables = append(repository.stackables, item)
	return nil
}

func (repository *fakeInventoryRepository) UpsertInstanceItem(_ context.Context, item InstanceItem) error {
	repository.instanceUpserts = append(repository.instanceUpserts, item)
	for index := range repository.instances {
		if repository.instances[index].ItemInstanceID == item.ItemInstanceID {
			repository.instances[index] = item
			return nil
		}
	}
	repository.instances = append(repository.instances, item)
	return nil
}

func (repository *fakeInventoryRepository) CommitAddItem(ctx context.Context, commit InventoryAddItemCommit) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	repository.commits = append(repository.commits, cloneInventoryAddItemCommit(commit))
	for _, item := range commit.StackableItems {
		if err := repository.UpsertStackableItem(ctx, item); err != nil {
			return err
		}
	}
	for _, item := range commit.InstanceItems {
		if err := repository.UpsertInstanceItem(ctx, item); err != nil {
			return err
		}
	}
	repository.upsertItemLedgerEntry(commit.LedgerEntry)
	repository.upsertAddItemReference(commit.Reference)
	if commit.Counters.ItemSequence > repository.counters.ItemSequence {
		repository.counters.ItemSequence = commit.Counters.ItemSequence
	}
	if commit.Counters.LedgerSequence > repository.counters.LedgerSequence {
		repository.counters.LedgerSequence = commit.Counters.LedgerSequence
	}
	return nil
}

func (repository *fakeInventoryRepository) CommitMoveItem(ctx context.Context, commit InventoryMoveItemCommit) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	repository.moveCommits = append(repository.moveCommits, cloneInventoryMoveItemCommit(commit))
	for _, item := range commit.DeletedStackableItems {
		repository.deleteStackableItem(item)
	}
	for _, item := range commit.StackableItems {
		if err := repository.UpsertStackableItem(ctx, item); err != nil {
			return err
		}
	}
	for _, item := range commit.InstanceItems {
		if err := repository.UpsertInstanceItem(ctx, item); err != nil {
			return err
		}
	}
	for _, entry := range commit.LedgerEntries {
		repository.upsertItemLedgerEntry(entry)
	}
	repository.upsertMoveItemReference(commit.Reference)
	repository.advanceCounters(commit.Counters)
	return nil
}

func (repository *fakeInventoryRepository) CommitRemoveItem(ctx context.Context, commit InventoryRemoveItemCommit) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	repository.removeCommits = append(repository.removeCommits, cloneInventoryRemoveItemCommit(commit))
	for _, item := range commit.DeletedStackableItems {
		repository.deleteStackableItem(item)
	}
	for _, item := range commit.StackableItems {
		if err := repository.UpsertStackableItem(ctx, item); err != nil {
			return err
		}
	}
	for _, item := range commit.DeletedInstanceItems {
		repository.deleteInstanceItem(item)
	}
	for _, entry := range commit.LedgerEntries {
		repository.upsertItemLedgerEntry(entry)
	}
	repository.upsertRemoveItemReference(commit.Reference)
	repository.advanceCounters(commit.Counters)
	return nil
}

func (repository *fakeInventoryRepository) upsertItemLedgerEntry(entry ItemLedgerEntry) {
	for index := range repository.ledgerEntries {
		if repository.ledgerEntries[index].LedgerID == entry.LedgerID {
			repository.ledgerEntries[index] = entry
			return
		}
	}
	repository.ledgerEntries = append(repository.ledgerEntries, entry)
}

func (repository *fakeInventoryRepository) upsertAddItemReference(reference AddItemReference) {
	for index := range repository.references {
		if repository.references[index].PlayerID == reference.PlayerID && repository.references[index].ReferenceKey == reference.ReferenceKey {
			repository.references[index] = AddItemReference{
				PlayerID:     reference.PlayerID,
				ReferenceKey: reference.ReferenceKey,
				Result:       cloneAddItemResult(reference.Result),
			}
			return
		}
	}
	repository.references = append(repository.references, AddItemReference{
		PlayerID:     reference.PlayerID,
		ReferenceKey: reference.ReferenceKey,
		Result:       cloneAddItemResult(reference.Result),
	})
}

func (repository *fakeInventoryRepository) upsertMoveItemReference(reference MoveItemReference) {
	for index := range repository.moveReferences {
		if repository.moveReferences[index].PlayerID == reference.PlayerID && repository.moveReferences[index].ReferenceKey == reference.ReferenceKey {
			repository.moveReferences[index] = MoveItemReference{
				PlayerID:     reference.PlayerID,
				ReferenceKey: reference.ReferenceKey,
				Result:       cloneMoveItemResult(reference.Result),
			}
			return
		}
	}
	repository.moveReferences = append(repository.moveReferences, MoveItemReference{
		PlayerID:     reference.PlayerID,
		ReferenceKey: reference.ReferenceKey,
		Result:       cloneMoveItemResult(reference.Result),
	})
}

func (repository *fakeInventoryRepository) upsertRemoveItemReference(reference RemoveItemReference) {
	for index := range repository.removeReferences {
		if repository.removeReferences[index].PlayerID == reference.PlayerID && repository.removeReferences[index].ReferenceKey == reference.ReferenceKey {
			repository.removeReferences[index] = RemoveItemReference{
				PlayerID:     reference.PlayerID,
				ReferenceKey: reference.ReferenceKey,
				Result:       cloneRemoveItemResult(reference.Result),
			}
			return
		}
	}
	repository.removeReferences = append(repository.removeReferences, RemoveItemReference{
		PlayerID:     reference.PlayerID,
		ReferenceKey: reference.ReferenceKey,
		Result:       cloneRemoveItemResult(reference.Result),
	})
}

func (repository *fakeInventoryRepository) deleteStackableItem(item StackableItem) {
	filtered := repository.stackables[:0]
	for _, existing := range repository.stackables {
		if existing.OwnerPlayerID == item.OwnerPlayerID &&
			existing.Location == item.Location &&
			existing.ItemID == item.ItemID {
			continue
		}
		filtered = append(filtered, existing)
	}
	repository.stackables = filtered
}

func (repository *fakeInventoryRepository) deleteInstanceItem(item InstanceItem) {
	filtered := repository.instances[:0]
	for _, existing := range repository.instances {
		if existing.ItemInstanceID == item.ItemInstanceID {
			continue
		}
		filtered = append(filtered, existing)
	}
	repository.instances = filtered
}

func (repository *fakeInventoryRepository) advanceCounters(counters InventoryCounters) {
	if counters.ItemSequence > repository.counters.ItemSequence {
		repository.counters.ItemSequence = counters.ItemSequence
	}
	if counters.LedgerSequence > repository.counters.LedgerSequence {
		repository.counters.LedgerSequence = counters.LedgerSequence
	}
}

func cloneInventoryAddItemCommit(commit InventoryAddItemCommit) InventoryAddItemCommit {
	commit.StackableItems = append([]StackableItem(nil), commit.StackableItems...)
	commit.InstanceItems = append([]InstanceItem(nil), commit.InstanceItems...)
	commit.Reference.Result = cloneAddItemResult(commit.Reference.Result)
	return commit
}

func cloneInventoryMoveItemCommit(commit InventoryMoveItemCommit) InventoryMoveItemCommit {
	commit.StackableItems = append([]StackableItem(nil), commit.StackableItems...)
	commit.DeletedStackableItems = append([]StackableItem(nil), commit.DeletedStackableItems...)
	commit.InstanceItems = append([]InstanceItem(nil), commit.InstanceItems...)
	commit.LedgerEntries = append([]ItemLedgerEntry(nil), commit.LedgerEntries...)
	commit.Reference.Result = cloneMoveItemResult(commit.Reference.Result)
	return commit
}

func cloneInventoryRemoveItemCommit(commit InventoryRemoveItemCommit) InventoryRemoveItemCommit {
	commit.StackableItems = append([]StackableItem(nil), commit.StackableItems...)
	commit.DeletedStackableItems = append([]StackableItem(nil), commit.DeletedStackableItems...)
	commit.DeletedInstanceItems = append([]InstanceItem(nil), commit.DeletedInstanceItems...)
	commit.LedgerEntries = append([]ItemLedgerEntry(nil), commit.LedgerEntries...)
	commit.Reference.Result = cloneRemoveItemResult(commit.Reference.Result)
	return commit
}

func newTestInventoryService() *InventoryService {
	return NewInventoryService(testutil.NewFakeClock(testInventoryNow))
}

func validAddItemInput(t *testing.T) AddItemInput {
	t.Helper()

	return AddItemInput{
		PlayerID:       "player-1",
		ItemDefinition: validStackableDefinition(t),
		Quantity:       5,
		Location:       validLocation(t),
		Reason:         "loot_pickup",
		ReferenceKey:   validReferenceKey(t, "loot_pickup:drop-1"),
	}
}

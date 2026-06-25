package contentdb_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/market"
)

func TestPostgresMarketStorePersistsListingEscrowAcrossStoreReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	sellerID := foundation.PlayerID("player-postgres-market-seller")
	seedPostgresWalletPlayer(t, ctx, store, sellerID)
	marketStore, err := contentdb.NewMarketListingStore(store)
	if err != nil {
		t.Fatalf("NewMarketListingStore() error = %v, want nil", err)
	}
	listingID := foundation.ListingID("listing-postgres-market-escrow")
	expiresAt := time.Date(2026, 6, 25, 21, 0, 0, 0, time.UTC)
	listing := market.Listing{
		ListingID:         listingID,
		SellerPlayerID:    sellerID,
		ItemDefinition:    postgresStackableDefinitionForTest(t),
		OriginalQuantity:  12,
		RemainingQuantity: 7,
		UnitPrice:         42,
		Currency:          economy.CurrencyBucketCredits,
		Status:            market.ListingStatusActive,
		SourceReturnLocation: economy.ItemLocation{
			Kind: economy.LocationKindAccountInventory,
			ID:   economy.LocationID("account"),
		},
		EscrowLocation: economy.ItemLocation{
			Kind: economy.LocationKindMarketEscrow,
			ID:   economy.LocationID(listingID.String()),
		},
		CreatedAt: time.Date(2026, 6, 25, 20, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 25, 20, 1, 0, 0, time.UTC),
		ExpiresAt: &expiresAt,
	}
	listing.ItemID = listing.ItemDefinition.ItemID
	if err := marketStore.UpsertMarketListing(ctx, listing); err != nil {
		t.Fatalf("UpsertMarketListing() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewMarketListingStore(store)
	if err != nil {
		t.Fatalf("NewMarketListingStore(reopen) error = %v, want nil", err)
	}
	loaded, ok, err := reopened.LoadMarketListing(ctx, listingID)
	if err != nil {
		t.Fatalf("LoadMarketListing() error = %v, want nil", err)
	}
	if !ok {
		t.Fatalf("LoadMarketListing(%q) ok = false, want true", listingID)
	}
	if loaded.ListingID != listing.ListingID ||
		loaded.Status != market.ListingStatusActive ||
		loaded.RemainingQuantity != listing.RemainingQuantity ||
		loaded.SourceReturnLocation != listing.SourceReturnLocation ||
		loaded.EscrowLocation != listing.EscrowLocation ||
		loaded.ItemDefinition.ItemID != listing.ItemDefinition.ItemID {
		t.Fatalf("loaded listing = %+v, want persisted status/quantity/escrow/source/item from %+v", loaded, listing)
	}
}

func TestPostgresMarketStoreTransactionLocksListingForUpdate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	sellerID := foundation.PlayerID("player-postgres-market-lock-seller")
	seedPostgresWalletPlayer(t, ctx, store, sellerID)
	marketStore, err := contentdb.NewMarketStore(store)
	if err != nil {
		t.Fatalf("NewMarketStore() error = %v, want nil", err)
	}
	listingID := foundation.ListingID("listing-postgres-market-lock")
	listing := market.Listing{
		ListingID:         listingID,
		SellerPlayerID:    sellerID,
		ItemDefinition:    postgresStackableDefinitionForTest(t),
		OriginalQuantity:  9,
		RemainingQuantity: 9,
		UnitPrice:         25,
		Currency:          economy.CurrencyBucketCredits,
		Status:            market.ListingStatusActive,
		SourceReturnLocation: economy.ItemLocation{
			Kind: economy.LocationKindAccountInventory,
			ID:   economy.LocationID("account"),
		},
		EscrowLocation: economy.ItemLocation{
			Kind: economy.LocationKindMarketEscrow,
			ID:   economy.LocationID(listingID.String()),
		},
		CreatedAt: time.Date(2026, 6, 25, 21, 10, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 6, 25, 21, 10, 0, 0, time.UTC),
	}
	listing.ItemID = listing.ItemDefinition.ItemID
	if err := marketStore.SaveMarketListing(ctx, listing); err != nil {
		t.Fatalf("SaveMarketListing() error = %v, want nil", err)
	}

	if err := marketStore.WithTransaction(ctx, func(tx *contentdb.MarketListingTx) error {
		locked, ok, err := tx.LoadMarketListingForUpdate(ctx, listingID)
		if err != nil {
			return err
		}
		if !ok {
			t.Fatalf("LoadMarketListingForUpdate(%q) ok = false, want true", listingID)
		}
		locked.RemainingQuantity = 4
		locked.UpdatedAt = time.Date(2026, 6, 25, 21, 11, 0, 0, time.UTC)
		return tx.UpsertMarketListing(ctx, locked)
	}); err != nil {
		t.Fatalf("WithTransaction(LoadMarketListingForUpdate) error = %v, want nil", err)
	}

	loaded, ok, err := marketStore.LoadMarketListing(ctx, listingID)
	if err != nil {
		t.Fatalf("LoadMarketListing(after tx) error = %v, want nil", err)
	}
	if !ok || loaded.RemainingQuantity != 4 {
		t.Fatalf("listing after locked transaction = %+v ok %v, want remaining quantity 4", loaded, ok)
	}
}

func TestPostgresMarketStoreTransactionRollsBackSettlementRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	sellerID := foundation.PlayerID("player-postgres-market-tx-seller")
	buyerID := foundation.PlayerID("player-postgres-market-tx-buyer")
	seedPostgresWalletPlayer(t, ctx, store, sellerID)
	seedPostgresWalletPlayer(t, ctx, store, buyerID)
	marketStore, err := contentdb.NewMarketStore(store)
	if err != nil {
		t.Fatalf("NewMarketStore() error = %v, want nil", err)
	}
	walletStore, err := contentdb.NewWalletStore(store)
	if err != nil {
		t.Fatalf("NewWalletStore() error = %v, want nil", err)
	}
	inventoryStore, err := contentdb.NewInventoryStore(store)
	if err != nil {
		t.Fatalf("NewInventoryStore() error = %v, want nil", err)
	}

	now := time.Date(2026, 6, 25, 22, 0, 0, 0, time.UTC)
	listingID := foundation.ListingID("listing-postgres-market-tx-rollback")
	definition := postgresStackableDefinitionForTest(t)
	account := economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID("account")}
	escrow := economy.ItemLocation{Kind: economy.LocationKindMarketEscrow, ID: economy.LocationID(listingID.String())}
	listing := market.Listing{
		ListingID:            listingID,
		SellerPlayerID:       sellerID,
		ItemDefinition:       definition,
		ItemID:               definition.ItemID,
		OriginalQuantity:     5,
		RemainingQuantity:    5,
		UnitPrice:            20,
		Currency:             economy.CurrencyBucketCredits,
		Status:               market.ListingStatusActive,
		SourceReturnLocation: account,
		EscrowLocation:       escrow,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := marketStore.SaveMarketListing(ctx, listing); err != nil {
		t.Fatalf("SaveMarketListing() error = %v, want nil", err)
	}
	if err := walletStore.UpsertWalletBalance(ctx, economy.WalletBalance{PlayerID: buyerID, Currency: economy.CurrencyBucketCredits, Balance: 1000, UpdatedAt: now}); err != nil {
		t.Fatalf("UpsertWalletBalance(buyer) error = %v, want nil", err)
	}
	if err := walletStore.UpsertWalletBalance(ctx, economy.WalletBalance{PlayerID: sellerID, Currency: economy.CurrencyBucketCredits, Balance: 0, UpdatedAt: now}); err != nil {
		t.Fatalf("UpsertWalletBalance(seller) error = %v, want nil", err)
	}
	escrowItem := postgresMarketStackableItemForTest(t, definition, sellerID, foundation.ItemID("raw_ore-market-tx-escrow"), escrow, 5, now)
	if err := inventoryStore.UpsertStackableItem(ctx, escrowItem); err != nil {
		t.Fatalf("UpsertStackableItem(escrow) error = %v, want nil", err)
	}

	requestID := foundation.RequestID("request-postgres-market-tx")
	buyReference, err := foundation.MarketBuyIdempotencyKey(listingID, buyerID, requestID)
	if err != nil {
		t.Fatalf("MarketBuyIdempotencyKey() error = %v, want nil", err)
	}
	saleReference, err := foundation.MarketSaleIdempotencyKey(listingID, buyerID, requestID)
	if err != nil {
		t.Fatalf("MarketSaleIdempotencyKey() error = %v, want nil", err)
	}
	idempotencyRow := economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         buyReference,
		Operation:   "market_buy",
		PlayerID:    buyerID,
		RequestHash: "market-tx-request-hash",
		Status:      economy.IdempotencyStatusInProgress,
		ResultJSON:  []byte(`{}`),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	rollbackErr := errors.New("injected market transaction rollback")

	err = marketStore.WithTransaction(ctx, func(tx *contentdb.MarketListingTx) error {
		locked, ok, err := tx.LoadMarketListingForUpdate(ctx, listingID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("locked listing missing")
		}
		locked.RemainingQuantity = 2
		locked.UpdatedAt = now.Add(time.Minute)
		if err := tx.UpsertMarketListing(ctx, locked); err != nil {
			return err
		}
		if err := tx.CommitWalletMutation(ctx, postgresMarketWalletCommit(t, buyerID, 900, 100, economy.LedgerActionDecrease, buyReference, economy.LedgerID("currency-ledger-market-tx-buy"), now)); err != nil {
			return err
		}
		if err := tx.CommitWalletMutation(ctx, postgresMarketWalletCommit(t, sellerID, 100, 100, economy.LedgerActionIncrease, saleReference, economy.LedgerID("currency-ledger-market-tx-sale"), now)); err != nil {
			return err
		}
		moveCommit := postgresMarketInventoryMoveCommit(t, definition, sellerID, buyerID, listingID, escrowItem, buyReference, now)
		if err := tx.CommitInventoryMoveItem(ctx, moveCommit); err != nil {
			return err
		}
		claim, err := tx.ClaimIdempotencyKey(ctx, idempotencyRow)
		if err != nil {
			return err
		}
		if claim.Duplicate {
			return errors.New("idempotency claim duplicate before rollback")
		}
		completed := claim.Row.Clone()
		completed.Status = economy.IdempotencyStatusCompleted
		completed.ResultJSON = []byte(`{"listing_id":"listing-postgres-market-tx-rollback"}`)
		completed.UpdatedAt = now.Add(time.Minute)
		completed.CompletedAt = now.Add(time.Minute)
		if _, err := tx.CompleteIdempotencyKey(ctx, completed); err != nil {
			return err
		}
		if err := tx.InsertOutboxRow(ctx, economy.OutboxRow{
			OutboxID:         "outbox-postgres-market-tx-rollback",
			Topic:            "economy",
			EventType:        "market.buy_completed",
			AggregateType:    "market_listing",
			AggregateID:      listingID.String(),
			IdempotencyScope: economy.IdempotencyScopeEconomy,
			IdempotencyKey:   buyReference,
			PayloadJSON:      []byte(`{"listing_id":"listing-postgres-market-tx-rollback"}`),
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("WithTransaction() error = %v, want rollback sentinel", err)
	}

	loaded, ok, err := marketStore.LoadMarketListing(ctx, listingID)
	if err != nil {
		t.Fatalf("LoadMarketListing(after rollback) error = %v, want nil", err)
	}
	if !ok || loaded.RemainingQuantity != 5 {
		t.Fatalf("listing after rollback = %+v ok %v, want original remaining quantity 5", loaded, ok)
	}
	buyerBalance, err := walletStore.WalletBalance(ctx, buyerID, economy.CurrencyBucketCredits)
	if err != nil {
		t.Fatalf("WalletBalance(buyer after rollback) error = %v, want nil", err)
	}
	sellerBalance, err := walletStore.WalletBalance(ctx, sellerID, economy.CurrencyBucketCredits)
	if err != nil {
		t.Fatalf("WalletBalance(seller after rollback) error = %v, want nil", err)
	}
	if buyerBalance.Balance != 1000 || sellerBalance.Balance != 0 {
		t.Fatalf("wallet balances after rollback = buyer %d seller %d, want 1000/0", buyerBalance.Balance, sellerBalance.Balance)
	}
	ledgerEntries, err := walletStore.LoadCurrencyLedgerEntries(ctx)
	if err != nil {
		t.Fatalf("LoadCurrencyLedgerEntries(after rollback) error = %v, want nil", err)
	}
	if len(ledgerEntries) != 0 {
		t.Fatalf("wallet ledger entries after rollback = %+v, want none", ledgerEntries)
	}
	items, err := inventoryStore.LoadStackableItems(ctx)
	if err != nil {
		t.Fatalf("LoadStackableItems(after rollback) error = %v, want nil", err)
	}
	if len(items) != 1 || items[0].OwnerPlayerID != sellerID || items[0].Location != escrow || items[0].Quantity.Int64() != 5 {
		t.Fatalf("inventory after rollback = %+v, want original seller escrow stack", items)
	}
	if _, ok, err := store.LoadOutboxRow(ctx, "outbox-postgres-market-tx-rollback"); err != nil || ok {
		t.Fatalf("LoadOutboxRow(after rollback) = ok %v err %v, want false nil", ok, err)
	}
	claim, err := store.ClaimIdempotencyKey(ctx, idempotencyRow)
	if err != nil {
		t.Fatalf("ClaimIdempotencyKey(after rollback) error = %v, want nil", err)
	}
	if claim.Duplicate {
		t.Fatal("ClaimIdempotencyKey(after rollback) Duplicate = true, want false")
	}
}

func postgresMarketWalletCommit(
	t *testing.T,
	playerID foundation.PlayerID,
	balanceAfter int64,
	amountValue int64,
	action economy.LedgerAction,
	referenceKey foundation.IdempotencyKey,
	ledgerID economy.LedgerID,
	now time.Time,
) economy.WalletMutationCommit {
	t.Helper()
	amount, err := foundation.NewMoney(amountValue)
	if err != nil {
		t.Fatalf("NewMoney(%d) error = %v, want nil", amountValue, err)
	}
	entry, err := economy.NewCurrencyLedgerEntry(
		ledgerID,
		playerID,
		economy.CurrencyBucketCredits,
		amount,
		action,
		balanceAfter,
		economy.LedgerReason("market_buy_tx_test"),
		referenceKey,
	)
	if err != nil {
		t.Fatalf("NewCurrencyLedgerEntry() error = %v, want nil", err)
	}
	entry.CreatedAt = now
	operation := economy.WalletMutationOperationCredit
	if action == economy.LedgerActionDecrease {
		operation = economy.WalletMutationOperationDebit
	}
	return economy.WalletMutationCommit{
		Balances: []economy.WalletBalance{{
			PlayerID:  playerID,
			Currency:  economy.CurrencyBucketCredits,
			Balance:   balanceAfter,
			UpdatedAt: now,
		}},
		LedgerEntries: []economy.CurrencyLedgerEntry{entry},
		Reference: economy.WalletMutationReference{
			PlayerID:      playerID,
			Operation:     operation,
			ReferenceKey:  referenceKey,
			LedgerEntries: []economy.CurrencyLedgerEntry{entry},
		},
		Counters: economy.WalletCounters{LedgerSequence: 2},
	}
}

func postgresMarketInventoryMoveCommit(
	t *testing.T,
	definition economy.ItemDefinition,
	sellerID foundation.PlayerID,
	buyerID foundation.PlayerID,
	listingID foundation.ListingID,
	escrowItem economy.StackableItem,
	referenceKey foundation.IdempotencyKey,
	now time.Time,
) economy.InventoryMoveItemCommit {
	t.Helper()
	buyerLocation := economy.ItemLocation{Kind: economy.LocationKindAccountInventory, ID: economy.LocationID("account")}
	buyerItem := postgresMarketStackableItemForTest(t, definition, buyerID, foundation.ItemID("raw_ore-market-tx-buyer"), buyerLocation, 3, now)
	quantity, err := foundation.NewQuantity(3)
	if err != nil {
		t.Fatalf("NewQuantity(3) error = %v, want nil", err)
	}
	sourceEntry, err := economy.NewItemLedgerEntry(
		economy.LedgerID("item-ledger-market-tx-source"),
		sellerID,
		definition.ItemID,
		foundation.ItemID(""),
		quantity,
		economy.LedgerActionDecrease,
		2,
		economy.ItemLocation{Kind: economy.LocationKindMarketEscrow, ID: economy.LocationID(listingID.String())},
		economy.LedgerReason("market_buy_tx_test"),
		referenceKey,
	)
	if err != nil {
		t.Fatalf("NewItemLedgerEntry(source) error = %v, want nil", err)
	}
	sourceEntry.CreatedAt = now
	destinationEntry, err := economy.NewItemLedgerEntry(
		economy.LedgerID("item-ledger-market-tx-destination"),
		buyerID,
		definition.ItemID,
		foundation.ItemID(""),
		quantity,
		economy.LedgerActionIncrease,
		3,
		buyerLocation,
		economy.LedgerReason("market_buy_tx_test"),
		referenceKey,
	)
	if err != nil {
		t.Fatalf("NewItemLedgerEntry(destination) error = %v, want nil", err)
	}
	destinationEntry.CreatedAt = now
	result := economy.MoveItemResult{
		StackableItems:        []economy.StackableItem{buyerItem},
		DeletedStackableItems: []economy.StackableItem{escrowItem},
		LedgerEntries:         []economy.ItemLedgerEntry{sourceEntry, destinationEntry},
	}
	return economy.InventoryMoveItemCommit{
		StackableItems:        []economy.StackableItem{buyerItem},
		DeletedStackableItems: []economy.StackableItem{escrowItem},
		LedgerEntries:         []economy.ItemLedgerEntry{sourceEntry, destinationEntry},
		Reference: economy.MoveItemReference{
			PlayerID:     sellerID,
			ReferenceKey: referenceKey,
			Result:       result,
		},
		Counters: economy.InventoryCounters{ItemSequence: 2, LedgerSequence: 2},
	}
}

func postgresMarketStackableItemForTest(
	t *testing.T,
	definition economy.ItemDefinition,
	playerID foundation.PlayerID,
	itemInstanceID foundation.ItemID,
	location economy.ItemLocation,
	quantityValue int64,
	now time.Time,
) economy.StackableItem {
	t.Helper()
	quantity, err := foundation.NewQuantity(quantityValue)
	if err != nil {
		t.Fatalf("NewQuantity(%d) error = %v, want nil", quantityValue, err)
	}
	item, err := economy.NewStackableItem(
		definition.Source,
		itemInstanceID,
		definition.ItemID,
		playerID,
		location,
		quantity,
	)
	if err != nil {
		t.Fatalf("NewStackableItem() error = %v, want nil", err)
	}
	item.CreatedAt = now
	item.UpdatedAt = now
	return item
}

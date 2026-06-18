package premium

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestCreateEntitlementStoresPendingAndProviderReplayReturnsOriginal(t *testing.T) {
	service, _, _ := newTestPremiumService(t)

	input := validCurrencyPackCreateInput("entitlement-1", "player-1", "event-1", 500)
	result, err := service.CreateEntitlement(input)
	if err != nil {
		t.Fatalf("CreateEntitlement() error = %v", err)
	}
	if result.Duplicate {
		t.Fatal("CreateEntitlement() duplicate = true, want false")
	}
	if result.Entitlement.State != EntitlementStatePending {
		t.Fatalf("state = %q, want %q", result.Entitlement.State, EntitlementStatePending)
	}

	replay := validCurrencyPackCreateInput("entitlement-2", "player-2", "event-1", 900)
	duplicate, err := service.CreateEntitlement(replay)
	if err != nil {
		t.Fatalf("CreateEntitlement() replay error = %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("replay duplicate = false, want true")
	}
	if duplicate.Entitlement.ID != input.EntitlementID {
		t.Fatalf("replay entitlement id = %q, want original %q", duplicate.Entitlement.ID, input.EntitlementID)
	}
	if duplicate.Entitlement.PlayerID != input.PlayerID {
		t.Fatalf("replay player = %q, want original %q", duplicate.Entitlement.PlayerID, input.PlayerID)
	}
	if got := len(service.Entitlements()); got != 1 {
		t.Fatalf("entitlements len = %d, want 1", got)
	}
}

func TestClaimPremiumCurrencyPackCreditsPremiumPaidOnce(t *testing.T) {
	service, wallet, _ := newTestPremiumService(t)
	input := validCurrencyPackCreateInput("entitlement-1", "player-1", "event-1", 750)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v", err)
	}

	result, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-1",
	})
	if err != nil {
		t.Fatalf("ClaimEntitlement() error = %v", err)
	}
	if result.WalletCredit == nil {
		t.Fatal("WalletCredit = nil, want credit result")
	}
	if result.WalletCredit.Duplicate {
		t.Fatal("WalletCredit duplicate = true, want false")
	}
	if result.Entitlement.State != EntitlementStateClaimed {
		t.Fatalf("state = %q, want %q", result.Entitlement.State, EntitlementStateClaimed)
	}
	if got := wallet.Balance(input.PlayerID, economy.CurrencyBucketPremiumPaid); got != 750 {
		t.Fatalf("premium_paid balance = %d, want 750", got)
	}

	duplicate, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-1",
	})
	if err != nil {
		t.Fatalf("ClaimEntitlement() duplicate error = %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate claim result duplicate = false, want true")
	}
	if got := wallet.Balance(input.PlayerID, economy.CurrencyBucketPremiumPaid); got != 750 {
		t.Fatalf("premium_paid balance after duplicate = %d, want 750", got)
	}
	if got := len(wallet.CurrencyLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
	ledger := wallet.CurrencyLedgerEntries()[0]
	wantReference, err := foundation.PremiumWebhookIdempotencyKey("stripe.event-1")
	if err != nil {
		t.Fatalf("PremiumWebhookIdempotencyKey() error = %v", err)
	}
	if ledger.ReferenceKey != wantReference {
		t.Fatalf("ledger reference = %q, want %q", ledger.ReferenceKey, wantReference)
	}
	if ledger.Reason != LedgerReasonPremiumEntitlementClaim {
		t.Fatalf("ledger reason = %q, want %q", ledger.Reason, LedgerReasonPremiumEntitlementClaim)
	}
}

func TestClaimRejectsWrongPlayerAndDifferentRequestAfterClaim(t *testing.T) {
	service, wallet, _ := newTestPremiumService(t)
	input := validCurrencyPackCreateInput("entitlement-1", "player-1", "event-1", 300)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v", err)
	}

	_, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         "player-2",
		RequestReference: "claim-wrong-player",
	})
	if !errors.Is(err, ErrEntitlementWrongPlayer) {
		t.Fatalf("wrong player error = %v, want ErrEntitlementWrongPlayer", err)
	}
	if got := wallet.Balance(input.PlayerID, economy.CurrencyBucketPremiumPaid); got != 0 {
		t.Fatalf("premium_paid after wrong player = %d, want 0", got)
	}

	if _, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-1",
	}); err != nil {
		t.Fatalf("ClaimEntitlement() error = %v", err)
	}

	_, err = service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-2",
	})
	if !errors.Is(err, ErrEntitlementAlreadyClaimed) {
		t.Fatalf("second claim different request error = %v, want ErrEntitlementAlreadyClaimed", err)
	}
	if got := wallet.Balance(input.PlayerID, economy.CurrencyBucketPremiumPaid); got != 300 {
		t.Fatalf("premium_paid after rejected second claim = %d, want 300", got)
	}
	if got := len(wallet.CurrencyLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len = %d, want 1", got)
	}
}

func TestClaimRecordsLoadoutSlotAndWeeklyXCorePurchaseRightSkeletons(t *testing.T) {
	service, _, _ := newTestPremiumService(t)

	loadoutInput := validCreateInput("entitlement-loadout", "player-1", "event-loadout", EntitlementTypeLoadoutSlot, EntitlementGrantPayload{
		LoadoutSlotScope: "ship",
		LoadoutSlotCount: 1,
	})
	if _, err := service.CreateEntitlement(loadoutInput); err != nil {
		t.Fatalf("CreateEntitlement(loadout) error = %v", err)
	}
	if _, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    loadoutInput.EntitlementID,
		PlayerID:         loadoutInput.PlayerID,
		RequestReference: "claim-loadout",
	}); err != nil {
		t.Fatalf("ClaimEntitlement(loadout) error = %v", err)
	}

	xCoreRightInput := validCreateInput("entitlement-xcore-right", "player-1", "event-xcore-right", EntitlementTypeWeeklyXCorePurchaseRight, EntitlementGrantPayload{
		WorldID:   "world-1",
		PeriodKey: "2026-W25",
	})
	if _, err := service.CreateEntitlement(xCoreRightInput); err != nil {
		t.Fatalf("CreateEntitlement(xcore right) error = %v", err)
	}
	if _, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    xCoreRightInput.EntitlementID,
		PlayerID:         xCoreRightInput.PlayerID,
		RequestReference: "claim-xcore-right",
	}); err != nil {
		t.Fatalf("ClaimEntitlement(xcore right) error = %v", err)
	}

	loadoutGrants := service.LoadoutSlotGrants()
	if got := len(loadoutGrants); got != 1 {
		t.Fatalf("loadout grants len = %d, want 1", got)
	}
	if loadoutGrants[0].Scope != "ship" || loadoutGrants[0].Count != 1 {
		t.Fatalf("loadout grant = %+v, want scope ship count 1", loadoutGrants[0])
	}

	rightGrants := service.WeeklyXCorePurchaseRightGrants()
	if got := len(rightGrants); got != 1 {
		t.Fatalf("weekly x core right grants len = %d, want 1", got)
	}
	if rightGrants[0].WorldID != "world-1" || rightGrants[0].PeriodKey != "2026-W25" {
		t.Fatalf("weekly x core right grant = %+v, want world-1 2026-W25", rightGrants[0])
	}
}

func TestWeeklyXCorePurchaseEnforcesOnePerPlayerPerPeriod(t *testing.T) {
	service, _, _ := newTestPremiumService(t)
	if _, err := service.ConfigureWeeklyXCoreStock(ConfigureWeeklyXCoreStockInput{
		WorldID:    "world-1",
		PeriodKey:  "2026-W25",
		StockTotal: 2,
	}); err != nil {
		t.Fatalf("ConfigureWeeklyXCoreStock() error = %v", err)
	}

	result, err := service.PurchaseWeeklyXCore(PurchaseWeeklyXCoreInput{
		PlayerID:          "player-1",
		WorldID:           "world-1",
		PeriodKey:         "2026-W25",
		PurchaseReference: "purchase-1",
		PaymentCurrency:   economy.CurrencyBucketPremiumPaid,
	})
	if err != nil {
		t.Fatalf("PurchaseWeeklyXCore() error = %v", err)
	}
	if result.Stock.StockRemaining != 1 {
		t.Fatalf("stock remaining = %d, want 1", result.Stock.StockRemaining)
	}

	duplicate, err := service.PurchaseWeeklyXCore(PurchaseWeeklyXCoreInput{
		PlayerID:          "player-1",
		WorldID:           "world-1",
		PeriodKey:         "2026-W25",
		PurchaseReference: "purchase-1",
		PaymentCurrency:   economy.CurrencyBucketPremiumPaid,
	})
	if err != nil {
		t.Fatalf("duplicate PurchaseWeeklyXCore() error = %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate purchase duplicate = false, want true")
	}

	_, err = service.PurchaseWeeklyXCore(PurchaseWeeklyXCoreInput{
		PlayerID:          "player-1",
		WorldID:           "world-1",
		PeriodKey:         "2026-W25",
		PurchaseReference: "purchase-2",
		PaymentCurrency:   economy.CurrencyBucketPremiumPaid,
	})
	if !errors.Is(err, ErrWeeklyLimitReached) {
		t.Fatalf("second purchase error = %v, want ErrWeeklyLimitReached", err)
	}
	if got := len(service.WeeklyXCorePurchases()); got != 1 {
		t.Fatalf("weekly purchases len = %d, want 1", got)
	}
	stock := service.WeeklyXCoreStockRecords()[0]
	if stock.StockRemaining != 1 {
		t.Fatalf("stock remaining after rejected second purchase = %d, want 1", stock.StockRemaining)
	}
}

func TestWeeklyXCoreConcurrentPurchasesDoNotOversell(t *testing.T) {
	service, _, _ := newTestPremiumService(t)
	const stockTotal = 5
	if _, err := service.ConfigureWeeklyXCoreStock(ConfigureWeeklyXCoreStockInput{
		WorldID:    "world-1",
		PeriodKey:  "2026-W25",
		StockTotal: stockTotal,
	}); err != nil {
		t.Fatalf("ConfigureWeeklyXCoreStock() error = %v", err)
	}

	const attempts = 32
	var wg sync.WaitGroup
	errs := make(chan error, attempts)
	for index := 0; index < attempts; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := service.PurchaseWeeklyXCore(PurchaseWeeklyXCoreInput{
				PlayerID:          foundation.PlayerID(fmt.Sprintf("player-%02d", index)),
				WorldID:           "world-1",
				PeriodKey:         "2026-W25",
				PurchaseReference: fmt.Sprintf("purchase-%02d", index),
				PaymentCurrency:   economy.CurrencyBucketPremiumPaid,
			})
			errs <- err
		}(index)
	}
	wg.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		if !errors.Is(err, ErrWeeklyStockSoldOut) {
			t.Fatalf("concurrent purchase error = %v, want ErrWeeklyStockSoldOut or nil", err)
		}
	}
	if successes != stockTotal {
		t.Fatalf("successful purchases = %d, want %d", successes, stockTotal)
	}
	purchases := service.WeeklyXCorePurchases()
	if got := len(purchases); got != stockTotal {
		t.Fatalf("recorded purchases len = %d, want %d", got, stockTotal)
	}
	stock := service.WeeklyXCoreStockRecords()[0]
	if stock.StockRemaining != 0 {
		t.Fatalf("stock remaining = %d, want 0", stock.StockRemaining)
	}
}

func TestValidatePaidPremiumUseKeepsPremiumBucketsSeparate(t *testing.T) {
	if err := ValidatePaidPremiumUse(economy.CurrencyBucketPremiumPaid); err != nil {
		t.Fatalf("paid premium eligibility error = %v, want nil", err)
	}
	if err := ValidatePaidPremiumUse(economy.CurrencyBucketPremiumEarned); !errors.Is(err, economy.ErrEarnedPremiumNotEligible) {
		t.Fatalf("earned premium eligibility error = %v, want ErrEarnedPremiumNotEligible", err)
	}
	if err := ValidatePaidPremiumUse(economy.CurrencyBucketPremiumMarketAcquired); !errors.Is(err, economy.ErrMarketAcquiredPremiumNotEligible) {
		t.Fatalf("market acquired eligibility error = %v, want ErrMarketAcquiredPremiumNotEligible", err)
	}
}

func TestSuspiciousTradeLogRecordsFieldsAndSnapshotsAreImmutable(t *testing.T) {
	service, _, _ := newTestPremiumService(t)

	log, err := service.RecordSuspiciousTrade(RecordSuspiciousTradeInput{
		ActorPlayerID:  "player-1",
		CounterpartyID: "player-2",
		Currency:       economy.CurrencyBucketPremiumPaid,
		Amount:         999,
		Reason:         "price-outlier",
		Reference:      "trade-1",
	})
	if err != nil {
		t.Fatalf("RecordSuspiciousTrade() error = %v", err)
	}
	if log.ActorPlayerID != "player-1" ||
		log.CounterpartyID != "player-2" ||
		log.Currency != economy.CurrencyBucketPremiumPaid ||
		log.Amount != 999 ||
		log.Reason != "price-outlier" ||
		log.Reference != "trade-1" {
		t.Fatalf("log = %+v, fields did not match input", log)
	}

	snapshot := service.SuspiciousTradeLogs()
	if got := len(snapshot); got != 1 {
		t.Fatalf("snapshot len = %d, want 1", got)
	}
	snapshot[0].Amount = 1
	snapshot[0].Reason = "mutated"

	nextSnapshot := service.SuspiciousTradeLogs()
	if nextSnapshot[0].Amount != 999 || nextSnapshot[0].Reason != "price-outlier" {
		t.Fatalf("snapshot mutation leaked into service state: %+v", nextSnapshot[0])
	}
}

func newTestPremiumService(t *testing.T) (*PremiumEntitlementService, *economy.WalletService, *testutil.FakeClock) {
	t.Helper()

	clock := testutil.NewFakeClock(time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC))
	wallet := economy.NewWalletService(clock)
	service, err := NewPremiumEntitlementService(wallet, clock)
	if err != nil {
		t.Fatalf("NewPremiumEntitlementService() error = %v", err)
	}
	return service, wallet, clock
}

func validCurrencyPackCreateInput(entitlementID EntitlementID, playerID foundation.PlayerID, providerReference string, amount int64) CreateEntitlementInput {
	return validCreateInput(entitlementID, playerID, providerReference, EntitlementTypePremiumCurrencyPack, EntitlementGrantPayload{
		CurrencyBucket: economy.CurrencyBucketPremiumPaid,
		Amount:         amount,
	})
}

func validCreateInput(entitlementID EntitlementID, playerID foundation.PlayerID, providerReference string, entitlementType EntitlementType, payload EntitlementGrantPayload) CreateEntitlementInput {
	createdAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	return CreateEntitlementInput{
		EntitlementID:       entitlementID,
		PlayerID:            playerID,
		Type:                entitlementType,
		Provider:            ProviderReference{Source: "stripe", Reference: providerReference},
		Payload:             payload,
		ProviderConfirmedAt: createdAt.Add(-time.Second),
		CreatedAt:           createdAt,
	}
}

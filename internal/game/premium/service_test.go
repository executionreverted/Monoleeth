package premium

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
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

	replay := validCurrencyPackCreateInput("entitlement-2", "player-1", "event-1", 500)
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

func TestClaimReplayThroughIdempotencyStoreDoesNotCreditTwiceAfterServiceRebuild(t *testing.T) {
	store := newMemoryPremiumIdempotencyStore()
	service, wallet, clock := newTestPremiumServiceWithIdempotencyStore(t, store)
	input := validCurrencyPackCreateInput("entitlement-replay", "player-replay", "event-replay", 750)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v", err)
	}
	if _, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-replay",
	}); err != nil {
		t.Fatalf("ClaimEntitlement() error = %v", err)
	}

	rebuilt, err := NewPremiumEntitlementServiceWithConfig(PremiumEntitlementServiceConfig{
		Wallet:           wallet,
		Clock:            clock,
		IdempotencyStore: store,
	})
	if err != nil {
		t.Fatalf("NewPremiumEntitlementServiceWithConfig() error = %v", err)
	}
	replayed, err := rebuilt.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-replay",
	})
	if err != nil {
		t.Fatalf("replayed ClaimEntitlement() error = %v", err)
	}

	if !replayed.Duplicate {
		t.Fatal("replayed claim Duplicate = false, want true")
	}
	if got := wallet.Balance(input.PlayerID, economy.CurrencyBucketPremiumPaid); got != 750 {
		t.Fatalf("premium_paid balance after replay = %d, want 750", got)
	}
	if got := len(wallet.CurrencyLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len after replay = %d, want 1", got)
	}
}

func TestClaimDifferentRequestAfterDurableClaimConflictsBeforeGrant(t *testing.T) {
	store := newMemoryPremiumIdempotencyStore()
	service, wallet, _ := newTestPremiumServiceWithIdempotencyStore(t, store)
	input := validCurrencyPackCreateInput("entitlement-conflict", "player-conflict", "event-conflict", 300)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v", err)
	}
	if _, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-1",
	}); err != nil {
		t.Fatalf("ClaimEntitlement() error = %v", err)
	}

	_, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-2",
	})
	if !errors.Is(err, economy.ErrIdempotencyKeyConflict) {
		t.Fatalf("second claim error = %v, want ErrIdempotencyKeyConflict", err)
	}
	if got := wallet.Balance(input.PlayerID, economy.CurrencyBucketPremiumPaid); got != 300 {
		t.Fatalf("premium_paid balance after conflict = %d, want 300", got)
	}
	if got := len(wallet.CurrencyLedgerEntries()); got != 1 {
		t.Fatalf("ledger entries len after conflict = %d, want 1", got)
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

func TestProviderWebhookDuplicateKeyCannotCreateConflictingEntitlement(t *testing.T) {
	store := newMemoryPremiumIdempotencyStore()
	service, _, _ := newTestPremiumServiceWithIdempotencyStore(t, store)
	input := validCurrencyPackCreateInput("entitlement-provider", "player-provider", "event-provider", 500)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v", err)
	}
	duplicate, err := service.CreateEntitlement(input)
	if err != nil {
		t.Fatalf("duplicate CreateEntitlement() error = %v", err)
	}
	if !duplicate.Duplicate {
		t.Fatal("duplicate provider event Duplicate = false, want true")
	}

	conflicting := validCurrencyPackCreateInput("entitlement-provider-2", "player-provider-2", "event-provider", 900)
	_, err = service.CreateEntitlement(conflicting)
	if !errors.Is(err, economy.ErrIdempotencyKeyConflict) {
		t.Fatalf("conflicting CreateEntitlement() error = %v, want ErrIdempotencyKeyConflict", err)
	}
	if got := len(service.Entitlements()); got != 1 {
		t.Fatalf("entitlements len after conflicting webhook = %d, want 1", got)
	}
}

func TestCreateEntitlementTransactionRepositoryReplaysProviderReference(t *testing.T) {
	repository := newFakePremiumEntitlementTransactionRepository()
	service, _, _ := newTestPremiumServiceWithTransactionRepository(t, repository)
	input := validCurrencyPackCreateInput("entitlement-tx-provider", "player-tx-provider", "event-tx-provider", 500)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v, want nil", err)
	}

	replay := validCurrencyPackCreateInput("entitlement-tx-provider-replay", "player-tx-provider", "event-tx-provider", 500)
	duplicate, err := service.CreateEntitlement(replay)
	if err != nil {
		t.Fatalf("CreateEntitlement(replay) error = %v, want nil", err)
	}

	if !duplicate.Duplicate || duplicate.Entitlement.ID != input.EntitlementID {
		t.Fatalf("provider replay result = %+v, want duplicate original entitlement", duplicate)
	}
	if got := repository.entitlementCount(); got != 1 {
		t.Fatalf("repository entitlement count = %d, want 1", got)
	}
	referenceKey, err := premiumProviderIdempotencyKey(input.Provider)
	if err != nil {
		t.Fatalf("premiumProviderIdempotencyKey() error = %v, want nil", err)
	}
	if outbox, ok := repository.outboxRow(premiumProviderOutboxPrefix + referenceKey.String()); !ok || outbox.EventType != premiumEntitlementCreatedEvent {
		t.Fatalf("provider outbox row = %+v ok %v, want one created event", outbox, ok)
	}
}

func TestCreateEntitlementPostCommitIdempotencyCacheConflictKeepsCommittedState(t *testing.T) {
	repository := newFakePremiumEntitlementTransactionRepository()
	logger := observability.NewMemoryPremiumTransitionLogger()
	clock := testutil.NewFakeClock(time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC))
	wallet := economy.NewWalletService(clock)
	service, err := NewPremiumEntitlementServiceWithConfig(PremiumEntitlementServiceConfig{
		Wallet:                wallet,
		Clock:                 clock,
		EntitlementRepository: repository,
		TransitionLogger:      logger,
	})
	if err != nil {
		t.Fatalf("NewPremiumEntitlementServiceWithConfig() error = %v", err)
	}

	input := validCurrencyPackCreateInput("entitlement-post-commit", "player-post-commit", "event-post-commit", 500)
	referenceKey, err := premiumProviderIdempotencyKey(input.Provider)
	if err != nil {
		t.Fatalf("premiumProviderIdempotencyKey() error = %v", err)
	}
	requestHash, err := premiumProviderRequestHash(input)
	if err != nil {
		t.Fatalf("premiumProviderRequestHash() error = %v", err)
	}

	// Simulate a corrupt/foreign in-memory idempotency cache entry for the same
	// key but a mismatched request hash. The durable transaction has no such
	// row, so the durable claim succeeds and commits; the post-commit in-memory
	// record must then conflict without rolling back committed state.
	staleRow := economy.IdempotencyKeyRow{
		Scope:       economy.IdempotencyScopeEconomy,
		Key:         referenceKey,
		Operation:   premiumProviderOperation,
		PlayerID:    input.PlayerID,
		RequestHash: requestHash + ":stale-mismatch",
		Status:      economy.IdempotencyStatusCompleted,
		ResultJSON:  json.RawMessage(`{}`),
		CreatedAt:   clock.Now(),
		UpdatedAt:   clock.Now(),
		CompletedAt: clock.Now(),
	}
	service.mu.Lock()
	service.ensurePremiumIdempotencyMapsLocked()
	service.providerIdempotencyRows[referenceKey] = staleRow
	service.mu.Unlock()

	result, err := service.CreateEntitlement(input)
	if err != nil {
		t.Fatalf("CreateEntitlement() error = %v, want nil (commit is authoritative)", err)
	}
	if result.Duplicate {
		t.Fatalf("CreateEntitlement() result = duplicate, want fresh entitlement")
	}

	// Committed durable rows must be present.
	stored, ok := repository.entitlement(input.EntitlementID)
	if !ok || stored.State != EntitlementStatePending {
		t.Fatalf("stored entitlement = %+v ok %v, want committed pending row", stored, ok)
	}
	if got := repository.entitlementCount(); got != 1 {
		t.Fatalf("repository entitlement count = %d, want 1", got)
	}

	// The in-memory committed state must NOT have been rolled back.
	service.mu.Lock()
	entitlement, inMemory := service.entitlements[input.EntitlementID]
	service.mu.Unlock()
	if !inMemory {
		t.Fatalf("in-memory entitlement missing: post-commit cache conflict rolled back committed state")
	}
	if entitlement.State != EntitlementStatePending {
		t.Fatalf("in-memory entitlement state = %q, want pending", entitlement.State)
	}

	// The anomaly must surface in the transition log without failing the op.
	anomaly := requirePremiumTransitionEntry(
		t,
		logger.Snapshot(),
		observability.Operation(premiumPostCommitCacheAnomalyOperation),
		input.PlayerID,
	)
	if anomaly.Status != observability.CommandStatusError {
		t.Fatalf("anomaly transition status = %q, want error", anomaly.Status)
	}
}

func TestClaimEntitlementTransactionRepositoryCommitsCurrencyPackRows(t *testing.T) {
	repository := newFakePremiumEntitlementTransactionRepository()
	service, wallet, _ := newTestPremiumServiceWithTransactionRepository(t, repository)
	input := validCurrencyPackCreateInput("entitlement-tx-claim", "player-tx-claim", "event-tx-claim", 700)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v, want nil", err)
	}

	result, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-tx",
	})
	if err != nil {
		t.Fatalf("ClaimEntitlement() error = %v, want nil", err)
	}

	if result.WalletCredit == nil || result.Entitlement.State != EntitlementStateClaimed {
		t.Fatalf("claim result = %+v, want wallet credit and claimed entitlement", result)
	}
	if got := wallet.Balance(input.PlayerID, economy.CurrencyBucketPremiumPaid); got != 700 {
		t.Fatalf("premium_paid balance = %d, want 700", got)
	}
	stored, ok := repository.entitlement(input.EntitlementID)
	if !ok || stored.State != EntitlementStateClaimed || stored.ClaimRequestRef != "claim-tx" {
		t.Fatalf("stored entitlement = %+v ok %v, want claimed row", stored, ok)
	}
	if got := len(repository.walletCommitsSnapshot()); got != 1 {
		t.Fatalf("wallet commits len = %d, want 1", got)
	}
	referenceKey, err := premiumClaimIdempotencyKey(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-tx",
	})
	if err != nil {
		t.Fatalf("premiumClaimIdempotencyKey() error = %v, want nil", err)
	}
	row, ok := repository.idempotencyRow(economy.IdempotencyScopeEconomy, referenceKey)
	if !ok || row.Status != economy.IdempotencyStatusCompleted {
		t.Fatalf("claim idempotency row = %+v ok %v, want completed", row, ok)
	}
	outbox, ok := repository.outboxRow(premiumClaimOutboxPrefix + referenceKey.String())
	if !ok || outbox.EventType != premiumEntitlementClaimedEvent || outbox.AggregateID != input.EntitlementID.String() {
		t.Fatalf("claim outbox row = %+v ok %v, want claimed event", outbox, ok)
	}
}

func TestClaimEntitlementTransactionOutboxFailureRollsBackRowsAndWalletState(t *testing.T) {
	repository := newFakePremiumEntitlementTransactionRepository()
	service, wallet, _ := newTestPremiumServiceWithTransactionRepository(t, repository)
	input := validCurrencyPackCreateInput("entitlement-tx-rollback", "player-tx-rollback", "event-tx-rollback", 900)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v, want nil", err)
	}
	injectedErr := errors.New("injected premium outbox failure")
	repository.failOutboxWith = injectedErr
	claimInput := ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-rollback",
	}

	_, err := service.ClaimEntitlement(claimInput)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("ClaimEntitlement() error = %v, want injected outbox failure", err)
	}

	stored, ok := repository.entitlement(input.EntitlementID)
	if !ok || stored.State != EntitlementStatePending {
		t.Fatalf("stored entitlement after rollback = %+v ok %v, want pending", stored, ok)
	}
	if got := wallet.Balance(input.PlayerID, economy.CurrencyBucketPremiumPaid); got != 0 {
		t.Fatalf("premium_paid balance after rollback = %d, want 0", got)
	}
	if got := len(wallet.CurrencyLedgerEntries()); got != 0 {
		t.Fatalf("wallet ledger entries after rollback = %d, want 0", got)
	}
	if got := len(repository.walletCommitsSnapshot()); got != 0 {
		t.Fatalf("repository wallet commits after rollback = %d, want 0", got)
	}
	referenceKey, err := premiumClaimIdempotencyKey(claimInput)
	if err != nil {
		t.Fatalf("premiumClaimIdempotencyKey() error = %v, want nil", err)
	}
	if row, ok := repository.idempotencyRow(economy.IdempotencyScopeEconomy, referenceKey); ok {
		t.Fatalf("claim idempotency row after rollback = %+v, want none", row)
	}
	if outbox, ok := repository.outboxRow(premiumClaimOutboxPrefix + referenceKey.String()); ok {
		t.Fatalf("claim outbox after rollback = %+v, want none", outbox)
	}
	entitlements := service.Entitlements()
	if len(entitlements) != 1 || entitlements[0].State != EntitlementStatePending {
		t.Fatalf("service entitlements after rollback = %+v, want pending entitlement", entitlements)
	}
}

func TestPremiumProviderAndClaimTransitionLogsContainSafeFieldsNoSecrets(t *testing.T) {
	logger := observability.NewMemoryPremiumTransitionLogger()
	service, _, _ := newTestPremiumServiceWithTransitionLogger(t, logger)
	input := validCurrencyPackCreateInput("entitlement-transition", "player-transition", "event-transition", 250)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v", err)
	}
	if _, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-transition",
	}); err != nil {
		t.Fatalf("ClaimEntitlement() error = %v", err)
	}

	entries := logger.Snapshot()
	if got := len(entries); got != 2 {
		t.Fatalf("premium transition entries len = %d, want 2: %+v", got, entries)
	}
	provider := requirePremiumTransitionEntry(t, entries, observability.Operation(premiumProviderOperation), input.PlayerID)
	wantProviderKey, err := foundation.PremiumWebhookIdempotencyKey("stripe.event-transition")
	if err != nil {
		t.Fatalf("PremiumWebhookIdempotencyKey(provider) error = %v", err)
	}
	if provider.IdempotencyKey != wantProviderKey ||
		provider.Status != observability.CommandStatusOK ||
		provider.ErrorCode != "" ||
		provider.Duration < 0 ||
		provider.Timestamp.IsZero() {
		t.Fatalf("provider transition = %+v, want safe ok transition with idempotency", provider)
	}
	if len(provider.ProviderRefIDs) != 1 || provider.ProviderRefIDs[0] != "stripe.event-transition" {
		t.Fatalf("provider refs = %+v, want stripe.event-transition", provider.ProviderRefIDs)
	}

	claim := requirePremiumTransitionEntry(t, entries, observability.Operation(premiumClaimOperation), input.PlayerID)
	wantClaimKey, err := foundation.PremiumWebhookIdempotencyKey("claim.entitlement-transition.player-transition")
	if err != nil {
		t.Fatalf("PremiumWebhookIdempotencyKey(claim) error = %v", err)
	}
	if claim.RequestRef != "claim-transition" ||
		claim.IdempotencyKey != wantClaimKey ||
		claim.Status != observability.CommandStatusOK ||
		claim.ErrorCode != "" ||
		claim.Duration < 0 ||
		claim.Timestamp.IsZero() {
		t.Fatalf("claim transition = %+v, want safe ok transition with request/idempotency", claim)
	}
	if len(claim.ProviderRefIDs) != 1 || claim.ProviderRefIDs[0] != "stripe.event-transition" {
		t.Fatalf("claim provider refs = %+v, want stripe.event-transition", claim.ProviderRefIDs)
	}

	payload, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal premium transition entries: %v", err)
	}
	assertPremiumTransitionLogHasNoSecrets(t, string(payload))
}

func TestPremiumClaimTransitionLogsErrorCodeAndDropsUnsafeRefs(t *testing.T) {
	logger := observability.NewMemoryPremiumTransitionLogger()
	service, _, _ := newTestPremiumServiceWithTransitionLogger(t, logger)
	input := validCurrencyPackCreateInput("entitlement-risk", "player-owner", "event-token-secret-hash", 250)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v", err)
	}

	_, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         "player-attacker",
		RequestReference: "claim-token-secret-hash",
	})
	if !errors.Is(err, ErrEntitlementWrongPlayer) {
		t.Fatalf("ClaimEntitlement(wrong player) error = %v, want ErrEntitlementWrongPlayer", err)
	}

	entries := logger.Snapshot()
	claim := requirePremiumTransitionEntry(t, entries, observability.Operation(premiumClaimOperation), "player-attacker")
	if claim.Status != observability.CommandStatusError || claim.ErrorCode != foundation.CodeForbidden {
		t.Fatalf("claim transition status/code = %q/%q, want error/%s", claim.Status, claim.ErrorCode, foundation.CodeForbidden)
	}
	if claim.RequestRef != "" {
		t.Fatalf("unsafe request ref logged as %q, want omitted", claim.RequestRef)
	}
	wantClaimKey, err := foundation.PremiumWebhookIdempotencyKey("claim.entitlement-risk.player-attacker")
	if err != nil {
		t.Fatalf("PremiumWebhookIdempotencyKey(claim) error = %v", err)
	}
	if claim.IdempotencyKey != wantClaimKey {
		t.Fatalf("claim idempotency = %q, want %q", claim.IdempotencyKey, wantClaimKey)
	}

	payload, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal premium transition entries: %v", err)
	}
	assertPremiumTransitionLogHasNoSecrets(t, string(payload))
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

func TestValidatePremiumCurrencyListingRejectsFreePremium(t *testing.T) {
	if err := ValidatePremiumCurrencyListing(economy.CurrencyBucketPremiumPaid); err != nil {
		t.Fatalf("paid premium listing eligibility error = %v, want nil", err)
	}
	if err := ValidatePremiumCurrencyListing(economy.CurrencyBucketPremiumEarned); !errors.Is(err, economy.ErrEarnedPremiumNotEligible) {
		t.Fatalf("earned premium listing eligibility error = %v, want ErrEarnedPremiumNotEligible", err)
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

func TestApplyProviderRiskLockRevokesEntitlementAndReplaysByReference(t *testing.T) {
	service, _, _ := newTestPremiumService(t)
	input := validCurrencyPackCreateInput("entitlement-risk", "player-risk", "event-risk", 300)
	if _, err := service.CreateEntitlement(input); err != nil {
		t.Fatalf("CreateEntitlement() error = %v, want nil", err)
	}
	if _, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-risk",
	}); err != nil {
		t.Fatalf("ClaimEntitlement() error = %v, want nil", err)
	}

	first, err := service.ApplyProviderRiskLock(ApplyProviderRiskLockInput{
		Provider:  input.Provider,
		Reason:    "chargeback",
		Reference: "risk-event-1",
	})
	if err != nil {
		t.Fatalf("ApplyProviderRiskLock() error = %v, want nil", err)
	}
	second, err := service.ApplyProviderRiskLock(ApplyProviderRiskLockInput{
		Provider:  input.Provider,
		Reason:    "chargeback",
		Reference: "risk-event-1",
	})
	if err != nil {
		t.Fatalf("duplicate ApplyProviderRiskLock() error = %v, want nil", err)
	}

	if first.Entitlement.State != EntitlementStateRevoked {
		t.Fatalf("entitlement state = %q, want revoked", first.Entitlement.State)
	}
	if first.Lock.PreviousState != EntitlementStateClaimed || first.Lock.CurrentState != EntitlementStateRevoked {
		t.Fatalf("risk lock states = %q/%q, want claimed/revoked", first.Lock.PreviousState, first.Lock.CurrentState)
	}
	if !second.Duplicate {
		t.Fatal("duplicate risk lock Duplicate = false, want true")
	}
	if got := len(service.ProviderRiskLocks()); got != 1 {
		t.Fatalf("risk locks len = %d, want 1", got)
	}
	entitlements := service.Entitlements()
	if len(entitlements) != 1 || entitlements[0].State != EntitlementStateRevoked {
		t.Fatalf("entitlements = %+v, want one revoked entitlement", entitlements)
	}
	replayedClaim, err := service.ClaimEntitlement(ClaimEntitlementInput{
		EntitlementID:    input.EntitlementID,
		PlayerID:         input.PlayerID,
		RequestReference: "claim-risk",
	})
	if err != nil {
		t.Fatalf("replayed ClaimEntitlement() error = %v, want nil", err)
	}
	if !replayedClaim.Duplicate {
		t.Fatal("replayed claim Duplicate = false, want true")
	}
	if replayedClaim.Entitlement.State != EntitlementStateRevoked {
		t.Fatalf("replayed claim state = %q, want revoked", replayedClaim.Entitlement.State)
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

func newTestPremiumServiceWithIdempotencyStore(
	t *testing.T,
	store economy.IdempotencyStore,
) (*PremiumEntitlementService, *economy.WalletService, *testutil.FakeClock) {
	t.Helper()

	clock := testutil.NewFakeClock(time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC))
	wallet := economy.NewWalletService(clock)
	service, err := NewPremiumEntitlementServiceWithConfig(PremiumEntitlementServiceConfig{
		Wallet:           wallet,
		Clock:            clock,
		IdempotencyStore: store,
	})
	if err != nil {
		t.Fatalf("NewPremiumEntitlementServiceWithConfig() error = %v", err)
	}
	return service, wallet, clock
}

func newTestPremiumServiceWithTransitionLogger(
	t *testing.T,
	logger observability.PremiumTransitionLogger,
) (*PremiumEntitlementService, *economy.WalletService, *testutil.FakeClock) {
	t.Helper()

	clock := testutil.NewFakeClock(time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC))
	wallet := economy.NewWalletService(clock)
	service, err := NewPremiumEntitlementServiceWithConfig(PremiumEntitlementServiceConfig{
		Wallet:           wallet,
		Clock:            clock,
		TransitionLogger: logger,
	})
	if err != nil {
		t.Fatalf("NewPremiumEntitlementServiceWithConfig() error = %v", err)
	}
	return service, wallet, clock
}

func newTestPremiumServiceWithTransactionRepository(
	t *testing.T,
	repository *fakePremiumEntitlementTransactionRepository,
) (*PremiumEntitlementService, *economy.WalletService, *testutil.FakeClock) {
	t.Helper()

	clock := testutil.NewFakeClock(time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC))
	wallet := economy.NewWalletService(clock)
	service, err := NewPremiumEntitlementServiceWithConfig(PremiumEntitlementServiceConfig{
		Wallet:                wallet,
		Clock:                 clock,
		EntitlementRepository: repository,
	})
	if err != nil {
		t.Fatalf("NewPremiumEntitlementServiceWithConfig() error = %v", err)
	}
	return service, wallet, clock
}

func requirePremiumTransitionEntry(
	t *testing.T,
	entries []observability.PremiumTransitionLogEntry,
	operation observability.Operation,
	playerID foundation.PlayerID,
) observability.PremiumTransitionLogEntry {
	t.Helper()

	for _, entry := range entries {
		if entry.Operation == operation && entry.PlayerID == playerID {
			return entry
		}
	}
	t.Fatalf("premium transition entry %s/%s not found in %+v", operation, playerID, entries)
	return observability.PremiumTransitionLogEntry{}
}

func assertPremiumTransitionLogHasNoSecrets(t *testing.T, payload string) {
	t.Helper()

	for _, leaked := range []string{
		"password",
		"token",
		"secret",
		"cookie",
		"hash",
		"bearer",
		"credential",
	} {
		if strings.Contains(payload, leaked) {
			t.Fatalf("premium transition log leaked %q in %s", leaked, payload)
		}
	}
}

type memoryPremiumIdempotencyStore struct {
	mu   sync.Mutex
	rows map[string]economy.IdempotencyKeyRow
}

func newMemoryPremiumIdempotencyStore() *memoryPremiumIdempotencyStore {
	return &memoryPremiumIdempotencyStore{
		rows: make(map[string]economy.IdempotencyKeyRow),
	}
}

func (store *memoryPremiumIdempotencyStore) ClaimIdempotencyKey(
	ctx context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyClaimResult, error) {
	store.mu.Lock()
	defer store.mu.Unlock()

	key := memoryPremiumIdempotencyKey(row.Scope, row.Key)
	if existing, ok := store.rows[key]; ok {
		return economy.ResolveIdempotencyClaim(&existing, row)
	}
	result, err := economy.ResolveIdempotencyClaim(nil, row)
	if err != nil {
		return economy.IdempotencyClaimResult{}, err
	}
	store.rows[key] = result.Row.Clone()
	return result, nil
}

func (store *memoryPremiumIdempotencyStore) CompleteIdempotencyKey(
	ctx context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, error) {
	if err := row.Validate(); err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	key := memoryPremiumIdempotencyKey(row.Scope, row.Key)
	if existing, ok := store.rows[key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return economy.IdempotencyKeyRow{}, err
		}
	}
	store.rows[key] = row.Clone()
	return row.Clone(), nil
}

func memoryPremiumIdempotencyKey(scope string, key foundation.IdempotencyKey) string {
	return scope + "\x00" + key.String()
}

type fakePremiumEntitlementTransactionRepository struct {
	mu                 sync.Mutex
	entitlements       map[EntitlementID]Entitlement
	providerReferences map[providerReferenceKey]EntitlementID
	idempotencyRows    map[string]economy.IdempotencyKeyRow
	outboxRows         map[string]economy.OutboxRow
	walletCommits      []economy.WalletMutationCommit
	failOutboxWith     error
	transactionCount   int
	entitlementLocks   int
	providerLocks      int
}

func newFakePremiumEntitlementTransactionRepository() *fakePremiumEntitlementTransactionRepository {
	return &fakePremiumEntitlementTransactionRepository{
		entitlements:       make(map[EntitlementID]Entitlement),
		providerReferences: make(map[providerReferenceKey]EntitlementID),
		idempotencyRows:    make(map[string]economy.IdempotencyKeyRow),
		outboxRows:         make(map[string]economy.OutboxRow),
	}
}

func (repository *fakePremiumEntitlementTransactionRepository) SavePremiumEntitlement(_ context.Context, entitlement Entitlement) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	return repository.savePremiumEntitlementLocked(entitlement)
}

func (repository *fakePremiumEntitlementTransactionRepository) WithPremiumEntitlementTransaction(
	_ context.Context,
	fn func(PremiumEntitlementTransaction) error,
) error {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	repository.transactionCount++
	tx := &fakePremiumEntitlementTransaction{
		repository:         repository,
		entitlements:       cloneEntitlementMap(repository.entitlements),
		providerReferences: cloneProviderReferenceMap(repository.providerReferences),
		idempotencyRows:    clonePremiumStringIdempotencyRows(repository.idempotencyRows),
		outboxRows:         clonePremiumOutboxRows(repository.outboxRows),
		walletCommits:      append([]economy.WalletMutationCommit(nil), repository.walletCommits...),
		failOutboxWith:     repository.failOutboxWith,
	}
	if err := fn(tx); err != nil {
		return err
	}
	repository.entitlements = cloneEntitlementMap(tx.entitlements)
	repository.providerReferences = cloneProviderReferenceMap(tx.providerReferences)
	repository.idempotencyRows = clonePremiumStringIdempotencyRows(tx.idempotencyRows)
	repository.outboxRows = clonePremiumOutboxRows(tx.outboxRows)
	repository.walletCommits = append([]economy.WalletMutationCommit(nil), tx.walletCommits...)
	return nil
}

func (repository *fakePremiumEntitlementTransactionRepository) savePremiumEntitlementLocked(entitlement Entitlement) error {
	if err := entitlement.ValidateSnapshot(); err != nil {
		return err
	}
	key := providerKey(entitlement.Provider)
	if existingID, ok := repository.providerReferences[key]; ok && existingID != entitlement.ID {
		return economy.ErrIdempotencyKeyConflict
	}
	if existing, ok := repository.entitlements[entitlement.ID]; ok {
		delete(repository.providerReferences, providerKey(existing.Provider))
	}
	repository.entitlements[entitlement.ID] = entitlement
	repository.providerReferences[key] = entitlement.ID
	return nil
}

func (repository *fakePremiumEntitlementTransactionRepository) entitlement(entitlementID EntitlementID) (Entitlement, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	entitlement, ok := repository.entitlements[entitlementID]
	return entitlement, ok
}

func (repository *fakePremiumEntitlementTransactionRepository) entitlementCount() int {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	return len(repository.entitlements)
}

func (repository *fakePremiumEntitlementTransactionRepository) idempotencyRow(scope string, key foundation.IdempotencyKey) (economy.IdempotencyKeyRow, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	row, ok := repository.idempotencyRows[memoryPremiumIdempotencyKey(scope, key)]
	return row.Clone(), ok
}

func (repository *fakePremiumEntitlementTransactionRepository) outboxRow(outboxID string) (economy.OutboxRow, bool) {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	row, ok := repository.outboxRows[outboxID]
	return row.Clone(), ok
}

func (repository *fakePremiumEntitlementTransactionRepository) walletCommitsSnapshot() []economy.WalletMutationCommit {
	repository.mu.Lock()
	defer repository.mu.Unlock()

	return append([]economy.WalletMutationCommit(nil), repository.walletCommits...)
}

type fakePremiumEntitlementTransaction struct {
	repository         *fakePremiumEntitlementTransactionRepository
	entitlements       map[EntitlementID]Entitlement
	providerReferences map[providerReferenceKey]EntitlementID
	idempotencyRows    map[string]economy.IdempotencyKeyRow
	outboxRows         map[string]economy.OutboxRow
	walletCommits      []economy.WalletMutationCommit
	failOutboxWith     error
}

func (tx *fakePremiumEntitlementTransaction) LoadPremiumEntitlementForUpdate(_ context.Context, entitlementID EntitlementID) (Entitlement, bool, error) {
	tx.repository.entitlementLocks++
	entitlement, ok := tx.entitlements[entitlementID]
	return entitlement, ok, nil
}

func (tx *fakePremiumEntitlementTransaction) LoadPremiumEntitlementByProviderForUpdate(_ context.Context, provider ProviderReference) (Entitlement, bool, error) {
	tx.repository.providerLocks++
	entitlementID, ok := tx.providerReferences[providerKey(provider)]
	if !ok {
		return Entitlement{}, false, nil
	}
	entitlement, ok := tx.entitlements[entitlementID]
	return entitlement, ok, nil
}

func (tx *fakePremiumEntitlementTransaction) SavePremiumEntitlement(_ context.Context, entitlement Entitlement) error {
	if err := entitlement.ValidateSnapshot(); err != nil {
		return err
	}
	key := providerKey(entitlement.Provider)
	if existingID, ok := tx.providerReferences[key]; ok && existingID != entitlement.ID {
		return economy.ErrIdempotencyKeyConflict
	}
	if existing, ok := tx.entitlements[entitlement.ID]; ok {
		delete(tx.providerReferences, providerKey(existing.Provider))
	}
	tx.entitlements[entitlement.ID] = entitlement
	tx.providerReferences[key] = entitlement.ID
	return nil
}

func (tx *fakePremiumEntitlementTransaction) CommitWalletMutation(_ context.Context, commit economy.WalletMutationCommit) error {
	if err := commit.Validate(); err != nil {
		return err
	}
	tx.walletCommits = append(tx.walletCommits, commit)
	return nil
}

func (tx *fakePremiumEntitlementTransaction) ClaimIdempotencyKey(
	_ context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyClaimResult, error) {
	key := memoryPremiumIdempotencyKey(row.Scope, row.Key)
	if existing, ok := tx.idempotencyRows[key]; ok {
		return economy.ResolveIdempotencyClaim(&existing, row)
	}
	claim, err := economy.ResolveIdempotencyClaim(nil, row)
	if err != nil {
		return economy.IdempotencyClaimResult{}, err
	}
	tx.idempotencyRows[key] = claim.Row.Clone()
	return claim, nil
}

func (tx *fakePremiumEntitlementTransaction) CompleteIdempotencyKey(
	_ context.Context,
	row economy.IdempotencyKeyRow,
) (economy.IdempotencyKeyRow, error) {
	if err := row.Validate(); err != nil {
		return economy.IdempotencyKeyRow{}, err
	}
	key := memoryPremiumIdempotencyKey(row.Scope, row.Key)
	if existing, ok := tx.idempotencyRows[key]; ok {
		if _, err := economy.ResolveIdempotencyClaim(&existing, row); err != nil {
			return economy.IdempotencyKeyRow{}, err
		}
	}
	tx.idempotencyRows[key] = row.Clone()
	return row.Clone(), nil
}

func (tx *fakePremiumEntitlementTransaction) InsertOutboxRow(_ context.Context, row economy.OutboxRow) error {
	if tx.failOutboxWith != nil {
		return tx.failOutboxWith
	}
	inserted, err := economy.NewOutboxRow(row)
	if err != nil {
		return err
	}
	if _, exists := tx.outboxRows[inserted.OutboxID]; exists {
		return fmt.Errorf("outbox %q: %w", inserted.OutboxID, economy.ErrInvalidOutboxRow)
	}
	tx.outboxRows[inserted.OutboxID] = inserted.Clone()
	return nil
}

func clonePremiumStringIdempotencyRows(rows map[string]economy.IdempotencyKeyRow) map[string]economy.IdempotencyKeyRow {
	if rows == nil {
		return nil
	}
	cloned := make(map[string]economy.IdempotencyKeyRow, len(rows))
	for key, row := range rows {
		cloned[key] = row.Clone()
	}
	return cloned
}

func clonePremiumOutboxRows(rows map[string]economy.OutboxRow) map[string]economy.OutboxRow {
	if rows == nil {
		return nil
	}
	cloned := make(map[string]economy.OutboxRow, len(rows))
	for key, row := range rows {
		cloned[key] = row.Clone()
	}
	return cloned
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

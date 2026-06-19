package death_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"gameproject/internal/game/death"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/testutil"
)

const fighterRepairCost int64 = 250

func TestRepairServiceRejectsNonDisabledShipWithoutWalletLedger(t *testing.T) {
	fixture := newRepairServiceFixture(t)
	input := repairShipInput(t, ships.ShipIDFighterT1, "non-disabled")

	_, err := fixture.repair.RepairShip(input)
	if !errors.Is(err, ships.ErrShipNotDisabled) {
		t.Fatalf("RepairShip(non-disabled) error = %v, want ErrShipNotDisabled", err)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 0 {
		t.Fatalf("wallet ledger entries = %d, want 0", got)
	}
	assertRepairServiceShipState(t, fixture.ships, ships.ShipIDFighterT1, ships.ShipStateActive)
}

func TestRepairServiceRejectsInsufficientCreditsWithoutShipMutation(t *testing.T) {
	fixture := newRepairServiceFixture(t)
	disableRepairServiceFighterAndSwapToStarter(t, fixture.ships)
	input := repairShipInput(t, ships.ShipIDFighterT1, "insufficient")

	_, err := fixture.repair.RepairShip(input)
	if !errors.Is(err, economy.ErrInsufficientWalletFunds) {
		t.Fatalf("RepairShip(insufficient) error = %v, want ErrInsufficientWalletFunds", err)
	}
	if got := fixture.wallet.Balance("player-1", economy.CurrencyBucketCredits); got != 0 {
		t.Fatalf("wallet balance = %d, want 0", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 0 {
		t.Fatalf("wallet ledger entries = %d, want 0", got)
	}
	assertRepairServiceShipState(t, fixture.ships, ships.ShipIDFighterT1, ships.ShipStateDisabled)
}

func TestRepairServiceDebitsServerCalculatedCostAndRestoresShipAvailable(t *testing.T) {
	fixture := newRepairServiceFixture(t)
	disableRepairServiceFighterAndSwapToStarter(t, fixture.ships)
	seedRepairCredits(t, fixture.wallet, fighterRepairCost, "repair-success-seed")
	input := repairShipInput(t, ships.ShipIDFighterT1, "success")

	result, err := fixture.repair.RepairShip(input)
	if err != nil {
		t.Fatalf("RepairShip(success) error = %v", err)
	}
	if !result.Repaired || result.Duplicate {
		t.Fatalf("RepairShip result = %+v, want repaired non-duplicate", result)
	}
	if result.RepairCost != fighterRepairCost {
		t.Fatalf("RepairCost = %d, want %d", result.RepairCost, fighterRepairCost)
	}
	if result.Currency != economy.CurrencyBucketCredits {
		t.Fatalf("Currency = %q, want credits", result.Currency)
	}
	if result.WalletDebit.LedgerEntry.Reason != death.LedgerReasonShipRepair ||
		result.WalletDebit.LedgerEntry.ReferenceKey != input.ReferenceKey ||
		result.WalletDebit.LedgerEntry.Amount.Int64() != fighterRepairCost {
		t.Fatalf("WalletDebit ledger = %+v, want ship repair debit for %d", result.WalletDebit.LedgerEntry, fighterRepairCost)
	}
	if got := fixture.wallet.Balance("player-1", economy.CurrencyBucketCredits); got != 0 {
		t.Fatalf("wallet balance = %d, want 0", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 2 {
		t.Fatalf("wallet ledger entries = %d, want seed + debit", got)
	}
	assertRepairServiceShipState(t, fixture.ships, ships.ShipIDFighterT1, ships.ShipStateAvailable)
}

func TestRepairServiceDuplicateReferenceDoesNotDoubleCharge(t *testing.T) {
	fixture := newRepairServiceFixture(t)
	disableRepairServiceFighterAndSwapToStarter(t, fixture.ships)
	seedRepairCredits(t, fixture.wallet, 500, "repair-duplicate-seed")
	input := repairShipInput(t, ships.ShipIDFighterT1, "duplicate")

	first, err := fixture.repair.RepairShip(input)
	if err != nil {
		t.Fatalf("first RepairShip() error = %v", err)
	}
	second, err := fixture.repair.RepairShip(input)
	if err != nil {
		t.Fatalf("duplicate RepairShip() error = %v", err)
	}
	if first.Duplicate {
		t.Fatal("first Duplicate = true, want false")
	}
	if !second.Duplicate || !second.Repaired {
		t.Fatalf("duplicate result = %+v, want duplicate repaired result", second)
	}
	if got := fixture.wallet.Balance("player-1", economy.CurrencyBucketCredits); got != 250 {
		t.Fatalf("wallet balance = %d, want 250", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 2 {
		t.Fatalf("wallet ledger entries after duplicate = %d, want seed + one debit", got)
	}
	assertRepairServiceShipState(t, fixture.ships, ships.ShipIDFighterT1, ships.ShipStateAvailable)
}

func TestRepairServiceRestoreFailureAfterDebitRefundsAndCachesFailure(t *testing.T) {
	fixture := newRepairServiceFixture(t)
	disableRepairServiceFighterAndSwapToStarter(t, fixture.ships)
	seedRepairCredits(t, fixture.wallet, 500, "repair-restore-failure-seed")
	restoreErr := errors.New("temporary ship store outage")
	flakyShips := &failOnceRepairServiceShips{
		delegate: fixture.ships,
		err:      restoreErr,
	}
	repairService, err := death.NewRepairService(death.RepairConfig{
		ShipCatalog: fixture.catalog,
		Wallet:      fixture.wallet,
		Ships:       flakyShips,
	})
	if err != nil {
		t.Fatalf("death.NewRepairService(flaky) error = %v", err)
	}
	fixture.repair = repairService
	input := repairShipInput(t, ships.ShipIDFighterT1, "restore-failure")

	result, err := fixture.repair.RepairShip(input)
	if !errors.Is(err, death.ErrRepairPreviouslyCompensated) || !errors.Is(err, restoreErr) {
		t.Fatalf("RepairShip(restore failure) error = %v, want compensated restore error", err)
	}
	if !result.Compensated || result.WalletRefund == nil {
		t.Fatalf("RepairShip failure result = %+v, want compensation refund", result)
	}
	if got := fixture.wallet.Balance("player-1", economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("wallet balance after compensation = %d, want 500", got)
	}
	entries := fixture.wallet.CurrencyLedgerEntries()
	if got := len(entries); got != 3 {
		t.Fatalf("wallet ledger entries = %d, want seed + debit + refund", got)
	}
	if entries[1].Reason != death.LedgerReasonShipRepair ||
		entries[1].Action != economy.LedgerActionDecrease ||
		entries[1].ReferenceKey != input.ReferenceKey {
		t.Fatalf("debit ledger = %+v, want ship repair debit", entries[1])
	}
	if entries[2].Reason != death.LedgerReasonShipRepairRefund ||
		entries[2].Action != economy.LedgerActionIncrease ||
		entries[2].ReferenceKey != input.ReferenceKey {
		t.Fatalf("refund ledger = %+v, want ship repair refund", entries[2])
	}
	assertRepairServiceShipState(t, fixture.ships, ships.ShipIDFighterT1, ships.ShipStateDisabled)

	second, err := fixture.repair.RepairShip(input)
	if !errors.Is(err, death.ErrRepairPreviouslyCompensated) {
		t.Fatalf("duplicate compensated RepairShip() error = %v, want ErrRepairPreviouslyCompensated", err)
	}
	if !second.Duplicate || !second.Compensated {
		t.Fatalf("duplicate compensated result = %+v, want cached compensated duplicate", second)
	}
	if flakyShips.calls != 1 {
		t.Fatalf("ship repair calls = %d, want 1", flakyShips.calls)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 3 {
		t.Fatalf("wallet ledger entries after duplicate compensated retry = %d, want 3", got)
	}

	freshRepairService, err := death.NewRepairService(death.RepairConfig{
		ShipCatalog: fixture.catalog,
		Wallet:      fixture.wallet,
		Ships:       fixture.ships,
	})
	if err != nil {
		t.Fatalf("death.NewRepairService(fresh) error = %v", err)
	}
	third, err := freshRepairService.RepairShip(input)
	if !errors.Is(err, death.ErrRepairPreviouslyCompensated) {
		t.Fatalf("fresh service compensated RepairShip() error = %v, want ErrRepairPreviouslyCompensated", err)
	}
	if !third.Compensated || third.Repaired {
		t.Fatalf("fresh service compensated result = %+v, want compensated non-repair", third)
	}
	assertRepairServiceShipState(t, fixture.ships, ships.ShipIDFighterT1, ships.ShipStateDisabled)
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 3 {
		t.Fatalf("wallet ledger entries after fresh retry = %d, want 3", got)
	}
}

func TestRepairServiceUsesWalletLedgerReferenceLookupForRefundReplay(t *testing.T) {
	fixture := newRepairServiceFixture(t)
	disableRepairServiceFighterAndSwapToStarter(t, fixture.ships)
	seedRepairCredits(t, fixture.wallet, 500, "repair-reference-lookup-seed")
	restoreErr := errors.New("temporary ship store outage")
	flakyShips := &failOnceRepairServiceShips{
		delegate: fixture.ships,
		err:      restoreErr,
	}
	repairService, err := death.NewRepairService(death.RepairConfig{
		ShipCatalog: fixture.catalog,
		Wallet:      fixture.wallet,
		Ships:       flakyShips,
	})
	if err != nil {
		t.Fatalf("death.NewRepairService(flaky) error = %v", err)
	}
	input := repairShipInput(t, ships.ShipIDFighterT1, "reference-lookup")

	_, err = repairService.RepairShip(input)
	if !errors.Is(err, death.ErrRepairPreviouslyCompensated) || !errors.Is(err, restoreErr) {
		t.Fatalf("RepairShip(restore failure) error = %v, want compensated restore error", err)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 3 {
		t.Fatalf("wallet ledger entries after compensation = %d, want seed + debit + refund", got)
	}

	wallet := &lookupTrackingRepairWallet{delegate: fixture.wallet}
	freshRepairService, err := death.NewRepairService(death.RepairConfig{
		ShipCatalog: fixture.catalog,
		Wallet:      wallet,
		Ships:       fixture.ships,
	})
	if err != nil {
		t.Fatalf("death.NewRepairService(fresh) error = %v", err)
	}
	result, err := freshRepairService.RepairShip(input)
	if !errors.Is(err, death.ErrRepairPreviouslyCompensated) {
		t.Fatalf("fresh RepairShip(compensated) error = %v, want ErrRepairPreviouslyCompensated", err)
	}
	if !result.Compensated || result.Repaired {
		t.Fatalf("fresh compensated result = %+v, want compensated non-repair", result)
	}
	if wallet.lookupCalls != 1 {
		t.Fatalf("wallet lookup calls = %d, want 1", wallet.lookupCalls)
	}
	if wallet.scanCalls != 0 {
		t.Fatalf("wallet scan calls = %d, want 0", wallet.scanCalls)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 3 {
		t.Fatalf("wallet ledger entries after lookup replay = %d, want 3", got)
	}
}

func TestRepairServiceConcurrentDifferentPlayersDoNotBlockEachOther(t *testing.T) {
	fixture := newRepairServiceFixture(t)
	ensureRepairServiceActiveFighterForPlayer(t, fixture.ships, "player-2")
	disableRepairServiceFighterAndSwapToStarterForPlayer(t, fixture.ships, "player-1")
	disableRepairServiceFighterAndSwapToStarterForPlayer(t, fixture.ships, "player-2")
	seedRepairCreditsForPlayer(t, fixture.wallet, "player-1", fighterRepairCost, "repair-concurrent-player-1-seed")
	seedRepairCreditsForPlayer(t, fixture.wallet, "player-2", fighterRepairCost, "repair-concurrent-player-2-seed")

	blockingShips := newBlockingRepairServiceShips(fixture.ships, "player-1")
	repairService, err := death.NewRepairService(death.RepairConfig{
		ShipCatalog: fixture.catalog,
		Wallet:      fixture.wallet,
		Ships:       blockingShips,
	})
	if err != nil {
		t.Fatalf("death.NewRepairService(blocking) error = %v", err)
	}
	input1 := repairShipInputForPlayer(t, "player-1", ships.ShipIDFighterT1, "same-reference-across-players")
	input2 := repairShipInputForPlayer(t, "player-2", ships.ShipIDFighterT1, "same-reference-across-players")

	firstDone := make(chan repairCallResult, 1)
	go func() {
		result, err := repairService.RepairShip(input1)
		firstDone <- repairCallResult{result: result, err: err}
	}()
	blockingShips.waitEntered(t)

	secondDone := make(chan repairCallResult, 1)
	go func() {
		result, err := repairService.RepairShip(input2)
		secondDone <- repairCallResult{result: result, err: err}
	}()

	var second repairCallResult
	select {
	case second = <-secondDone:
	case <-time.After(2 * time.Second):
		blockingShips.release()
		<-firstDone
		t.Fatal("player-2 repair did not finish while player-1 repair was in flight")
	}
	if second.err != nil {
		blockingShips.release()
		<-firstDone
		t.Fatalf("player-2 RepairShip() error = %v", second.err)
	}
	if !second.result.Repaired || second.result.Duplicate || second.result.PlayerID != "player-2" {
		blockingShips.release()
		<-firstDone
		t.Fatalf("player-2 result = %+v, want independent non-duplicate repair", second.result)
	}

	blockingShips.release()
	first := <-firstDone
	if first.err != nil {
		t.Fatalf("player-1 RepairShip() error = %v", first.err)
	}
	if !first.result.Repaired || first.result.Duplicate || first.result.PlayerID != "player-1" {
		t.Fatalf("player-1 result = %+v, want non-duplicate repair", first.result)
	}
	if got := fixture.wallet.Balance("player-1", economy.CurrencyBucketCredits); got != 0 {
		t.Fatalf("player-1 wallet balance = %d, want 0", got)
	}
	if got := fixture.wallet.Balance("player-2", economy.CurrencyBucketCredits); got != 0 {
		t.Fatalf("player-2 wallet balance = %d, want 0", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 4 {
		t.Fatalf("wallet ledger entries = %d, want two seeds + two repair debits", got)
	}
	assertRepairServiceShipStateForPlayer(t, fixture.ships, "player-1", ships.ShipIDFighterT1, ships.ShipStateAvailable)
	assertRepairServiceShipStateForPlayer(t, fixture.ships, "player-2", ships.ShipIDFighterT1, ships.ShipStateAvailable)
}

func TestRepairServiceConcurrentDuplicateReferenceReturnsCachedResult(t *testing.T) {
	fixture := newRepairServiceFixture(t)
	disableRepairServiceFighterAndSwapToStarter(t, fixture.ships)
	seedRepairCredits(t, fixture.wallet, 500, "repair-concurrent-duplicate-seed")

	blockingShips := newBlockingRepairServiceShips(fixture.ships, "player-1")
	repairService, err := death.NewRepairService(death.RepairConfig{
		ShipCatalog: fixture.catalog,
		Wallet:      fixture.wallet,
		Ships:       blockingShips,
	})
	if err != nil {
		t.Fatalf("death.NewRepairService(blocking) error = %v", err)
	}
	input := repairShipInput(t, ships.ShipIDFighterT1, "concurrent-duplicate")

	firstDone := make(chan repairCallResult, 1)
	go func() {
		result, err := repairService.RepairShip(input)
		firstDone <- repairCallResult{result: result, err: err}
	}()
	blockingShips.waitEntered(t)

	secondDone := make(chan repairCallResult, 1)
	go func() {
		result, err := repairService.RepairShip(input)
		secondDone <- repairCallResult{result: result, err: err}
	}()

	blockingShips.release()
	first := <-firstDone
	second := <-secondDone
	if first.err != nil {
		t.Fatalf("first RepairShip() error = %v", first.err)
	}
	if second.err != nil {
		t.Fatalf("duplicate RepairShip() error = %v", second.err)
	}
	if !first.result.Repaired || first.result.Duplicate {
		t.Fatalf("first result = %+v, want repaired non-duplicate", first.result)
	}
	if !second.result.Repaired || !second.result.Duplicate {
		t.Fatalf("duplicate result = %+v, want cached repaired duplicate", second.result)
	}
	if second.result.WalletDebit.LedgerEntry.LedgerID != first.result.WalletDebit.LedgerEntry.LedgerID {
		t.Fatalf("duplicate debit ledger = %q, want cached ledger %q", second.result.WalletDebit.LedgerEntry.LedgerID, first.result.WalletDebit.LedgerEntry.LedgerID)
	}
	if got := blockingShips.callsForPlayer("player-1"); got != 1 {
		t.Fatalf("ship repair calls = %d, want 1", got)
	}
	if got := fixture.wallet.Balance("player-1", economy.CurrencyBucketCredits); got != 250 {
		t.Fatalf("wallet balance = %d, want 250", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 2 {
		t.Fatalf("wallet ledger entries = %d, want seed + one repair debit", got)
	}
	assertRepairServiceShipState(t, fixture.ships, ships.ShipIDFighterT1, ships.ShipStateAvailable)
}

func TestRepairServiceConcurrentDuplicateReferenceReturnsCachedCompensation(t *testing.T) {
	fixture := newRepairServiceFixture(t)
	disableRepairServiceFighterAndSwapToStarter(t, fixture.ships)
	seedRepairCredits(t, fixture.wallet, 500, "repair-concurrent-compensation-seed")
	restoreErr := errors.New("temporary ship store outage")
	flakyShips := &failOnceRepairServiceShips{
		delegate: fixture.ships,
		err:      restoreErr,
	}
	blockingShips := newBlockingRepairServiceShips(flakyShips, "player-1")
	repairService, err := death.NewRepairService(death.RepairConfig{
		ShipCatalog: fixture.catalog,
		Wallet:      fixture.wallet,
		Ships:       blockingShips,
	})
	if err != nil {
		t.Fatalf("death.NewRepairService(blocking flaky) error = %v", err)
	}
	input := repairShipInput(t, ships.ShipIDFighterT1, "concurrent-compensation")

	firstDone := make(chan repairCallResult, 1)
	go func() {
		result, err := repairService.RepairShip(input)
		firstDone <- repairCallResult{result: result, err: err}
	}()
	blockingShips.waitEntered(t)

	secondDone := make(chan repairCallResult, 1)
	go func() {
		result, err := repairService.RepairShip(input)
		secondDone <- repairCallResult{result: result, err: err}
	}()

	blockingShips.release()
	first := <-firstDone
	second := <-secondDone
	if !errors.Is(first.err, death.ErrRepairPreviouslyCompensated) || !errors.Is(first.err, restoreErr) {
		t.Fatalf("first RepairShip() error = %v, want compensated restore error", first.err)
	}
	if !errors.Is(second.err, death.ErrRepairPreviouslyCompensated) || !errors.Is(second.err, restoreErr) {
		t.Fatalf("duplicate RepairShip() error = %v, want cached compensated restore error", second.err)
	}
	if !first.result.Compensated || first.result.Duplicate || first.result.WalletRefund == nil {
		t.Fatalf("first result = %+v, want compensated non-duplicate refund", first.result)
	}
	if !second.result.Compensated || !second.result.Duplicate || second.result.WalletRefund == nil {
		t.Fatalf("duplicate result = %+v, want cached compensated duplicate refund", second.result)
	}
	if second.result.WalletRefund.LedgerEntry.LedgerID != first.result.WalletRefund.LedgerEntry.LedgerID {
		t.Fatalf("duplicate refund ledger = %q, want cached ledger %q", second.result.WalletRefund.LedgerEntry.LedgerID, first.result.WalletRefund.LedgerEntry.LedgerID)
	}
	if got := blockingShips.callsForPlayer("player-1"); got != 1 {
		t.Fatalf("ship repair calls = %d, want 1", got)
	}
	if got := fixture.wallet.Balance("player-1", economy.CurrencyBucketCredits); got != 500 {
		t.Fatalf("wallet balance after compensation = %d, want 500", got)
	}
	if got := len(fixture.wallet.CurrencyLedgerEntries()); got != 3 {
		t.Fatalf("wallet ledger entries = %d, want seed + repair debit + refund", got)
	}
	assertRepairServiceShipState(t, fixture.ships, ships.ShipIDFighterT1, ships.ShipStateDisabled)
}

type repairServiceFixture struct {
	clock   *testutil.FakeClock
	catalog ships.Catalog
	wallet  *economy.WalletService
	ships   *ships.HangarService
	repair  *death.RepairService
}

func newRepairServiceFixture(t *testing.T) repairServiceFixture {
	t.Helper()

	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 20, 0, 0, 0, time.UTC))
	shipCatalog, err := ships.MVPShipCatalog()
	if err != nil {
		t.Fatalf("ships.MVPShipCatalog() error = %v", err)
	}
	shipService, err := ships.NewHangarService(
		shipCatalog,
		ships.NewInMemoryHangarStore(),
		ships.StaticPlayerRankProvider{"player-1": 2, "player-2": 2},
		ships.BaseShipCargoCapacityProvider{},
		clock,
	)
	if err != nil {
		t.Fatalf("ships.NewHangarService() error = %v", err)
	}
	ensureRepairServiceActiveFighterForPlayer(t, shipService, "player-1")

	wallet := economy.NewWalletService(clock)
	repairService, err := death.NewRepairService(death.RepairConfig{
		ShipCatalog: shipCatalog,
		Wallet:      wallet,
		Ships:       shipService,
	})
	if err != nil {
		t.Fatalf("death.NewRepairService() error = %v", err)
	}
	return repairServiceFixture{
		clock:   clock,
		catalog: shipCatalog,
		wallet:  wallet,
		ships:   shipService,
		repair:  repairService,
	}
}

func ensureRepairServiceActiveFighterForPlayer(t *testing.T, service *ships.HangarService, playerID foundation.PlayerID) {
	t.Helper()
	if _, err := service.EnsureStarterShip(playerID); err != nil {
		t.Fatalf("EnsureStarterShip(%q) error = %v", playerID, err)
	}
	if _, err := service.UnlockShip(ships.UnlockShipInput{PlayerID: playerID, ShipID: ships.ShipIDFighterT1}); err != nil {
		t.Fatalf("UnlockShip(%q) error = %v", playerID, err)
	}
	if _, err := service.SetActiveShip(ships.SetActiveShipInput{
		PlayerID: playerID,
		ShipID:   ships.ShipIDFighterT1,
		Context: ships.ShipSwapContext{
			InSafeHangarArea: true,
		},
	}); err != nil {
		t.Fatalf("SetActiveShip(%q) error = %v", playerID, err)
	}
}

func disableRepairServiceFighterAndSwapToStarter(t *testing.T, service *ships.HangarService) {
	t.Helper()

	disableRepairServiceFighterAndSwapToStarterForPlayer(t, service, "player-1")
}

func disableRepairServiceFighterAndSwapToStarterForPlayer(t *testing.T, service *ships.HangarService, playerID foundation.PlayerID) {
	t.Helper()

	if _, err := service.DisableActiveShipForDeath(ships.DisableActiveShipForDeathInput{PlayerID: playerID}); err != nil {
		t.Fatalf("DisableActiveShipForDeath() error = %v", err)
	}
	if _, err := service.SetActiveShip(ships.SetActiveShipInput{
		PlayerID: playerID,
		ShipID:   ships.ShipIDStarter,
		Context: ships.ShipSwapContext{
			InSafeHangarArea: true,
		},
	}); err != nil {
		t.Fatalf("SetActiveShip(starter) error = %v", err)
	}
}

func seedRepairCredits(t *testing.T, wallet *economy.WalletService, amount int64, reference string) {
	t.Helper()

	seedRepairCreditsForPlayer(t, wallet, "player-1", amount, reference)
}

func seedRepairCreditsForPlayer(t *testing.T, wallet *economy.WalletService, playerID foundation.PlayerID, amount int64, reference string) {
	t.Helper()

	referenceKey, err := foundation.QuestRewardIdempotencyKey(foundation.QuestID(reference))
	if err != nil {
		t.Fatalf("QuestRewardIdempotencyKey(%q) error = %v", reference, err)
	}
	_, err = wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     playerID,
		Currency:     economy.CurrencyBucketCredits,
		Amount:       amount,
		Reason:       economy.LedgerReason("test_seed"),
		ReferenceKey: referenceKey,
	})
	if err != nil {
		t.Fatalf("CreditWallet(seed) error = %v", err)
	}
}

func repairShipInput(t *testing.T, shipID foundation.ShipID, reference string) death.RepairShipInput {
	t.Helper()

	return repairShipInputForPlayer(t, "player-1", shipID, reference)
}

func repairShipInputForPlayer(t *testing.T, playerID foundation.PlayerID, shipID foundation.ShipID, reference string) death.RepairShipInput {
	t.Helper()

	referenceKey, err := foundation.ShipRepairIdempotencyKey(shipID, reference)
	if err != nil {
		t.Fatalf("ShipRepairIdempotencyKey(%q, %q) error = %v", shipID, reference, err)
	}
	return death.RepairShipInput{
		PlayerID:     playerID,
		ShipID:       shipID,
		ReferenceKey: referenceKey,
	}
}

func assertRepairServiceShipState(t *testing.T, service *ships.HangarService, shipID foundation.ShipID, want ships.ShipState) {
	t.Helper()

	assertRepairServiceShipStateForPlayer(t, service, "player-1", shipID, want)
}

func assertRepairServiceShipStateForPlayer(t *testing.T, service *ships.HangarService, playerID foundation.PlayerID, shipID foundation.ShipID, want ships.ShipState) {
	t.Helper()

	hangar, err := service.GetHangar(playerID)
	if err != nil {
		t.Fatalf("GetHangar() error = %v", err)
	}
	for _, playerShip := range hangar.Ships {
		if playerShip.ShipID == shipID {
			if playerShip.State != want {
				t.Fatalf("ship %q state = %q, want %q", shipID, playerShip.State, want)
			}
			return
		}
	}
	t.Fatalf("ship %q missing from hangar %+v", shipID, hangar)
}

type repairCallResult struct {
	result death.RepairShipResult
	err    error
}

type failOnceRepairServiceShips struct {
	delegate death.ShipRepairer
	err      error
	mu       sync.Mutex
	calls    int
}

func (service *failOnceRepairServiceShips) GetHangar(playerID foundation.PlayerID) (ships.HangarSnapshot, error) {
	return service.delegate.GetHangar(playerID)
}

func (service *failOnceRepairServiceShips) RepairShip(input ships.RepairShipInput) (ships.RepairShipResult, error) {
	service.mu.Lock()
	service.calls++
	calls := service.calls
	service.mu.Unlock()
	if calls == 1 {
		return ships.RepairShipResult{}, service.err
	}
	return service.delegate.RepairShip(input)
}

type blockingRepairServiceShips struct {
	delegate    death.ShipRepairer
	blockPlayer foundation.PlayerID
	entered     chan struct{}
	releaseCh   chan struct{}

	mu          sync.Mutex
	calls       map[foundation.PlayerID]int
	enteredOnce sync.Once
	releaseOnce sync.Once
}

func newBlockingRepairServiceShips(delegate death.ShipRepairer, blockPlayer foundation.PlayerID) *blockingRepairServiceShips {
	return &blockingRepairServiceShips{
		delegate:    delegate,
		blockPlayer: blockPlayer,
		entered:     make(chan struct{}),
		releaseCh:   make(chan struct{}),
		calls:       make(map[foundation.PlayerID]int),
	}
}

func (service *blockingRepairServiceShips) GetHangar(playerID foundation.PlayerID) (ships.HangarSnapshot, error) {
	return service.delegate.GetHangar(playerID)
}

func (service *blockingRepairServiceShips) RepairShip(input ships.RepairShipInput) (ships.RepairShipResult, error) {
	callNumber := service.recordCall(input.PlayerID)
	if input.PlayerID == service.blockPlayer && callNumber == 1 {
		service.enteredOnce.Do(func() {
			close(service.entered)
		})
		<-service.releaseCh
	}
	return service.delegate.RepairShip(input)
}

func (service *blockingRepairServiceShips) recordCall(playerID foundation.PlayerID) int {
	service.mu.Lock()
	defer service.mu.Unlock()

	service.calls[playerID]++
	return service.calls[playerID]
}

func (service *blockingRepairServiceShips) callsForPlayer(playerID foundation.PlayerID) int {
	service.mu.Lock()
	defer service.mu.Unlock()

	return service.calls[playerID]
}

func (service *blockingRepairServiceShips) waitEntered(t *testing.T) {
	t.Helper()

	select {
	case <-service.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("blocked ship repair was not entered")
	}
}

func (service *blockingRepairServiceShips) release() {
	service.releaseOnce.Do(func() {
		close(service.releaseCh)
	})
}

type lookupTrackingRepairWallet struct {
	delegate    *economy.WalletService
	lookupCalls int
	scanCalls   int
}

func (wallet *lookupTrackingRepairWallet) DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error) {
	return wallet.delegate.DebitWallet(input)
}

func (wallet *lookupTrackingRepairWallet) CreditWallet(input economy.CreditWalletInput) (economy.CreditWalletResult, error) {
	return wallet.delegate.CreditWallet(input)
}

func (wallet *lookupTrackingRepairWallet) FindCurrencyLedgerEntry(lookup economy.CurrencyLedgerReferenceLookup) (economy.CurrencyLedgerEntry, bool) {
	wallet.lookupCalls++
	return wallet.delegate.FindCurrencyLedgerEntry(lookup)
}

func (wallet *lookupTrackingRepairWallet) CurrencyLedgerEntries() []economy.CurrencyLedgerEntry {
	wallet.scanCalls++
	return wallet.delegate.CurrencyLedgerEntries()
}

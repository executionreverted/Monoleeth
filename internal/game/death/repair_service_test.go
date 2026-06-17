package death_test

import (
	"errors"
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
		ships.StaticPlayerRankProvider{"player-1": 2},
		ships.BaseShipCargoCapacityProvider{},
		clock,
	)
	if err != nil {
		t.Fatalf("ships.NewHangarService() error = %v", err)
	}
	ensureDeathServiceActiveFighter(t, shipService)

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

func disableRepairServiceFighterAndSwapToStarter(t *testing.T, service *ships.HangarService) {
	t.Helper()

	if _, err := service.DisableActiveShipForDeath(ships.DisableActiveShipForDeathInput{PlayerID: "player-1"}); err != nil {
		t.Fatalf("DisableActiveShipForDeath() error = %v", err)
	}
	if _, err := service.SetActiveShip(ships.SetActiveShipInput{
		PlayerID: "player-1",
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

	referenceKey, err := foundation.QuestRewardIdempotencyKey(foundation.QuestID(reference))
	if err != nil {
		t.Fatalf("QuestRewardIdempotencyKey(%q) error = %v", reference, err)
	}
	_, err = wallet.CreditWallet(economy.CreditWalletInput{
		PlayerID:     "player-1",
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

	referenceKey, err := foundation.ShipRepairIdempotencyKey(shipID, reference)
	if err != nil {
		t.Fatalf("ShipRepairIdempotencyKey(%q, %q) error = %v", shipID, reference, err)
	}
	return death.RepairShipInput{
		PlayerID:     "player-1",
		ShipID:       shipID,
		ReferenceKey: referenceKey,
	}
}

func assertRepairServiceShipState(t *testing.T, service *ships.HangarService, shipID foundation.ShipID, want ships.ShipState) {
	t.Helper()

	hangar, err := service.GetHangar("player-1")
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

type failOnceRepairServiceShips struct {
	delegate death.ShipRepairer
	err      error
	calls    int
}

func (service *failOnceRepairServiceShips) GetHangar(playerID foundation.PlayerID) (ships.HangarSnapshot, error) {
	return service.delegate.GetHangar(playerID)
}

func (service *failOnceRepairServiceShips) RepairShip(input ships.RepairShipInput) (ships.RepairShipResult, error) {
	service.calls++
	if service.calls == 1 {
		return ships.RepairShipResult{}, service.err
	}
	return service.delegate.RepairShip(input)
}

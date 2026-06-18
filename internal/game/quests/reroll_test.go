package quests

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

func TestRerollBoardChargesCreditsOnce(t *testing.T) {
	fixture, wallet := newRerollFixture(t, 1_000)
	input := validBoardGenerationInput(t, fixture.catalog)
	if _, err := fixture.service.GenerateAndStoreBoard(input); err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}

	result, err := fixture.service.RerollBoard(validRerollBoardInput(input.Player, 20260618, fixture.clock.Now()))
	if err != nil {
		t.Fatalf("RerollBoard() = %v, want nil", err)
	}

	if len(wallet.calls) != 1 {
		t.Fatalf("wallet calls = %d, want 1", len(wallet.calls))
	}
	if wallet.debited != result.Cost.Amount {
		t.Fatalf("wallet debited = %d, want reroll cost %d", wallet.debited, result.Cost.Amount)
	}
	if result.WalletDebit.LedgerEntry.Reason != questRerollLedgerReason {
		t.Fatalf("wallet reason = %q, want %q", result.WalletDebit.LedgerEntry.Reason, questRerollLedgerReason)
	}
	if len(result.Offers) != BoardOfferCount {
		t.Fatalf("reroll offers = %d, want %d", len(result.Offers), BoardOfferCount)
	}
}

func TestRerollBoardDuplicateReferenceDoesNotDoubleCharge(t *testing.T) {
	fixture, wallet := newRerollFixture(t, 1_000)
	input := validBoardGenerationInput(t, fixture.catalog)
	if _, err := fixture.service.GenerateAndStoreBoard(input); err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}
	rerollInput := validRerollBoardInput(input.Player, 20260618, fixture.clock.Now())

	first, err := fixture.service.RerollBoard(rerollInput)
	if err != nil {
		t.Fatalf("RerollBoard(first) = %v, want nil", err)
	}
	fixture.clock.Advance(time.Hour)
	second, err := fixture.service.RerollBoard(rerollInput)
	if err != nil {
		t.Fatalf("RerollBoard(second) = %v, want nil", err)
	}

	if !second.Duplicate {
		t.Fatal("duplicate reroll Duplicate = false, want true")
	}
	if len(wallet.calls) != 1 {
		t.Fatalf("wallet calls after duplicate = %d, want 1", len(wallet.calls))
	}
	if wallet.debited != first.Cost.Amount {
		t.Fatalf("wallet debited after duplicate = %d, want %d", wallet.debited, first.Cost.Amount)
	}
	if first.ReferenceKey != second.ReferenceKey {
		t.Fatalf("duplicate reference = %q, want %q", second.ReferenceKey, first.ReferenceKey)
	}
	if !reflect.DeepEqual(first.Offers, second.Offers) {
		t.Fatalf("duplicate offers changed\nfirst=%#v\nsecond=%#v", first.Offers, second.Offers)
	}
	if !second.WalletDebit.Duplicate {
		t.Fatal("duplicate wallet result Duplicate = false, want true")
	}
}

func TestRerollBoardDuplicateReferenceDoesNotRecheckRareRewardCap(t *testing.T) {
	fixture, wallet := newRerollFixture(t, 1_000)
	input := validBoardGenerationInput(t, fixture.catalog)
	if _, err := fixture.service.GenerateAndStoreBoard(input); err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}
	rerollInput := validRerollBoardInput(input.Player, 20260618, fixture.clock.Now())
	capChecks := 0
	rerollInput.RareRewardCapHook = func(RareRewardCapCheck) (bool, error) {
		capChecks++
		return true, nil
	}

	first, err := fixture.service.RerollBoard(rerollInput)
	if err != nil {
		t.Fatalf("RerollBoard(first) = %v, want nil", err)
	}
	if capChecks == 0 {
		t.Fatal("first reroll did not call rare reward cap hook")
	}
	checksAfterFirst := capChecks
	fixture.clock.Advance(time.Hour)
	rerollInput.RareRewardCapHook = func(RareRewardCapCheck) (bool, error) {
		t.Fatal("duplicate reroll rechecked rare reward cap hook")
		return false, nil
	}

	second, err := fixture.service.RerollBoard(rerollInput)
	if err != nil {
		t.Fatalf("RerollBoard(second) = %v, want nil", err)
	}
	if !second.Duplicate {
		t.Fatal("duplicate reroll Duplicate = false, want true")
	}
	if len(wallet.calls) != 1 {
		t.Fatalf("wallet calls after duplicate = %d, want 1", len(wallet.calls))
	}
	if capChecks != checksAfterFirst {
		t.Fatalf("cap checks after duplicate = %d, want %d", capChecks, checksAfterFirst)
	}
	if !reflect.DeepEqual(first.Offers, second.Offers) {
		t.Fatalf("duplicate offers changed\nfirst=%#v\nsecond=%#v", first.Offers, second.Offers)
	}
}

func TestRerollBoardInsufficientCreditsLeavesOffersUnchanged(t *testing.T) {
	fixture, wallet := newRerollFixture(t, 0)
	input := validBoardGenerationInput(t, fixture.catalog)
	if _, err := fixture.service.GenerateAndStoreBoard(input); err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}
	before, err := fixture.service.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers(before) = %v, want nil", err)
	}

	_, err = fixture.service.RerollBoard(validRerollBoardInput(input.Player, 20260618, fixture.clock.Now()))
	if !errors.Is(err, economy.ErrInsufficientWalletFunds) {
		t.Fatalf("RerollBoard() error = %v, want ErrInsufficientWalletFunds", err)
	}
	after, err := fixture.service.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers(after) = %v, want nil", err)
	}

	if wallet.debited != 0 {
		t.Fatalf("wallet debited = %d, want 0", wallet.debited)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("offers changed after insufficient funds\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestRerollBoardExpiresOldUnacceptedOffersAndStoresExactlyTen(t *testing.T) {
	fixture, _ := newRerollFixture(t, 1_000)
	input := validBoardGenerationInput(t, fixture.catalog)
	oldOffers, err := fixture.service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}
	oldOfferIDs := offerIDSet(oldOffers)

	result, err := fixture.service.RerollBoard(validRerollBoardInput(input.Player, 20260618, fixture.clock.Now()))
	if err != nil {
		t.Fatalf("RerollBoard() = %v, want nil", err)
	}
	current, err := fixture.service.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers() = %v, want nil", err)
	}

	if len(result.Offers) != BoardOfferCount || len(current) != BoardOfferCount {
		t.Fatalf("reroll/result offers = %d/%d, want %d", len(result.Offers), len(current), BoardOfferCount)
	}
	for _, offer := range current {
		if oldOfferIDs[offer.OfferID] {
			t.Fatalf("old unaccepted offer %q still available after reroll", offer.OfferID)
		}
	}
}

func TestRerollBoardPreservesAcceptedQuestAndGeneratedRewards(t *testing.T) {
	fixture, _ := newRerollFixture(t, 1_000)
	input := validBoardGenerationInput(t, fixture.catalog)
	offers, err := fixture.service.GenerateAndStoreBoard(input)
	if err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}
	accepted, err := fixture.service.AcceptQuest(AcceptQuestInput{
		Player:  input.Player,
		OfferID: offers[0].OfferID,
	})
	if err != nil {
		t.Fatalf("AcceptQuest() = %v, want nil", err)
	}

	_, err = fixture.service.RerollBoard(validRerollBoardInput(input.Player, 20260618, fixture.clock.Now()))
	if err != nil {
		t.Fatalf("RerollBoard() = %v, want nil", err)
	}
	playerQuests, err := fixture.store.PlayerQuests(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("PlayerQuests() = %v, want nil", err)
	}
	current, err := fixture.service.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers() = %v, want nil", err)
	}

	if len(playerQuests) != 1 {
		t.Fatalf("player quests = %d, want 1", len(playerQuests))
	}
	if playerQuests[0].PlayerQuestID != accepted.PlayerQuestID {
		t.Fatalf("preserved quest id = %q, want %q", playerQuests[0].PlayerQuestID, accepted.PlayerQuestID)
	}
	if !reflect.DeepEqual(playerQuests[0].RewardPayload, accepted.RewardPayload) {
		t.Fatalf("accepted reward changed\nstored=%#v\nwant=%#v", playerQuests[0].RewardPayload, accepted.RewardPayload)
	}
	if len(current) != BoardOfferCount {
		t.Fatalf("available offers after reroll with accepted quest = %d, want %d", len(current), BoardOfferCount)
	}
}

func TestRerollBoardUsesStableReferenceAndExposesCostAndCapHooks(t *testing.T) {
	fixture, wallet := newRerollFixture(t, 1_000)
	input := validBoardGenerationInput(t, fixture.catalog)
	input.Player.Rank = 3
	input.Player.MainLevel = 3
	if _, err := fixture.service.GenerateAndStoreBoard(input); err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}

	result, err := fixture.service.RerollBoard(validRerollBoardInput(input.Player, 20260618, fixture.clock.Now()))
	if err != nil {
		t.Fatalf("RerollBoard() = %v, want nil", err)
	}

	wantReference := "quest_reroll:player_1:reroll-20260618"
	if result.ReferenceKey.String() != wantReference {
		t.Fatalf("reference = %q, want %q", result.ReferenceKey, wantReference)
	}
	if len(wallet.calls) != 1 || wallet.calls[0].ReferenceKey.String() != wantReference {
		t.Fatalf("wallet reference calls = %+v, want %q", wallet.calls, wantReference)
	}
	if result.Cost.Amount != defaultRerollBaseCost+2*defaultRerollRankCost {
		t.Fatalf("reroll cost = %d, want rank-scaled amount", result.Cost.Amount)
	}
	if len(result.Cost.Hooks) != 1 || result.Cost.Hooks[0].Kind != RerollCostHookRankScaling || result.Cost.Hooks[0].AppliedRank != 3 {
		t.Fatalf("cost hooks = %+v, want rank scaling hook for rank 3", result.Cost.Hooks)
	}
	if len(result.RareCapHooks) == 0 {
		t.Fatal("result rare cap hooks empty, want placeholder hook")
	}
	for _, offer := range result.Offers {
		if len(offer.RewardPayload.RareCapHooks) == 0 {
			t.Fatalf("offer %q rare cap hooks empty, want placeholder hook", offer.OfferID)
		}
	}
}

func TestRerollBoardRareRewardCapBlocksBeforeDebit(t *testing.T) {
	fixture, wallet := newRerollFixture(t, 1_000)
	input := validBoardGenerationInput(t, fixture.catalog)
	if _, err := fixture.service.GenerateAndStoreBoard(input); err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}
	before, err := fixture.service.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers(before) = %v, want nil", err)
	}

	rerollInput := validRerollBoardInput(input.Player, 20260618, fixture.clock.Now())
	rerollInput.RareRewardCapHook = func(RareRewardCapCheck) (bool, error) {
		return false, nil
	}
	_, err = fixture.service.RerollBoard(rerollInput)
	if !errors.Is(err, ErrInsufficientEligibleTemplates) {
		t.Fatalf("RerollBoard() error = %v, want ErrInsufficientEligibleTemplates", err)
	}
	if len(wallet.calls) != 0 {
		t.Fatalf("wallet calls = %d, want 0 when rare cap blocks generation", len(wallet.calls))
	}
	after, err := fixture.service.BoardOffers(input.Player.PlayerID)
	if err != nil {
		t.Fatalf("BoardOffers(after) = %v, want nil", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("offers changed after rare cap blocked reroll\nbefore=%#v\nafter=%#v", before, after)
	}
}

func TestRerollBoardAllowsSameGenerationSeedWithDifferentReference(t *testing.T) {
	fixture, wallet := newRerollFixture(t, 2_000)
	input := validBoardGenerationInput(t, fixture.catalog)
	if _, err := fixture.service.GenerateAndStoreBoard(input); err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}
	firstInput := validRerollBoardInput(input.Player, 20260618, fixture.clock.Now())
	secondInput := firstInput
	secondInput.ReferenceID = "reroll-20260618-b"
	fixture.clock.Advance(time.Hour)
	secondInput.CreatedAt = fixture.clock.Now()

	first, err := fixture.service.RerollBoard(firstInput)
	if err != nil {
		t.Fatalf("RerollBoard(first) = %v, want nil", err)
	}
	second, err := fixture.service.RerollBoard(secondInput)
	if err != nil {
		t.Fatalf("RerollBoard(second) = %v, want nil", err)
	}

	if second.Duplicate {
		t.Fatal("second reroll Duplicate = true, want false with new reference")
	}
	if len(wallet.calls) != 2 {
		t.Fatalf("wallet calls = %d, want 2", len(wallet.calls))
	}
	if first.ReferenceKey == second.ReferenceKey {
		t.Fatalf("references both %q, want different references", first.ReferenceKey)
	}
	if reflect.DeepEqual(first.Offers, second.Offers) {
		t.Fatal("offers for same seed with different references are equal, want new deterministic board")
	}
}

func TestRerollBoardDoesNotHoldStoreLockWhileDebiting(t *testing.T) {
	fixture, wallet := newRerollFixture(t, 1_000)
	input := validBoardGenerationInput(t, fixture.catalog)
	if _, err := fixture.service.GenerateAndStoreBoard(input); err != nil {
		t.Fatalf("GenerateAndStoreBoard() = %v, want nil", err)
	}
	fixture.service.SetRerollServices(QuestRerollServices{
		Wallet: &reentrantQuestRerollWallet{
			fake:     wallet,
			store:    fixture.store,
			playerID: input.Player.PlayerID,
		},
	})

	done := make(chan error, 1)
	go func() {
		_, err := fixture.service.RerollBoard(validRerollBoardInput(input.Player, 20260618, fixture.clock.Now()))
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RerollBoard() = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RerollBoard deadlocked while wallet debit re-entered quest store")
	}
}

func newRerollFixture(t *testing.T, walletBalance int64) (questServiceFixture, *fakeQuestRerollWallet) {
	t.Helper()
	fixture := newQuestServiceFixture(t, MustMVPQuestCatalog(), time.Date(2026, 6, 17, 13, 0, 0, 0, time.UTC))
	wallet := newFakeQuestRerollWallet(walletBalance)
	fixture.service.SetRerollServices(QuestRerollServices{Wallet: wallet})
	return fixture, wallet
}

func validRerollBoardInput(player PlayerQuestBoardSnapshot, seed int64, createdAt time.Time) RerollBoardInput {
	return RerollBoardInput{
		Player:      player,
		Seed:        seed,
		ReferenceID: fmt.Sprintf("reroll-%d", seed),
		CreatedAt:   createdAt,
	}
}

func offerIDSet(offers []GeneratedBoardOffer) map[foundation.QuestID]bool {
	seen := make(map[foundation.QuestID]bool, len(offers))
	for _, offer := range offers {
		seen[offer.OfferID] = true
	}
	return seen
}

type fakeQuestRerollWallet struct {
	calls   []economy.DebitWalletInput
	balance int64
	debited int64
	seen    map[foundation.IdempotencyKey]economy.DebitWalletResult
}

type reentrantQuestRerollWallet struct {
	fake     *fakeQuestRerollWallet
	store    *InMemoryQuestStore
	playerID foundation.PlayerID
}

func (wallet *reentrantQuestRerollWallet) DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error) {
	if _, err := wallet.store.BoardOffers(wallet.playerID); err != nil {
		return economy.DebitWalletResult{}, err
	}
	return wallet.fake.DebitWallet(input)
}

func newFakeQuestRerollWallet(balance int64) *fakeQuestRerollWallet {
	return &fakeQuestRerollWallet{
		balance: balance,
		seen:    make(map[foundation.IdempotencyKey]economy.DebitWalletResult),
	}
}

func (fake *fakeQuestRerollWallet) DebitWallet(input economy.DebitWalletInput) (economy.DebitWalletResult, error) {
	fake.calls = append(fake.calls, input)
	if err := input.ReferenceKey.Validate(); err != nil {
		return economy.DebitWalletResult{}, err
	}
	if err := input.Currency.Validate(); err != nil {
		return economy.DebitWalletResult{}, err
	}
	amount, err := foundation.NewMoney(input.Amount)
	if err != nil {
		return economy.DebitWalletResult{}, err
	}
	if previous, ok := fake.seen[input.ReferenceKey]; ok {
		previous.Duplicate = true
		return previous, nil
	}
	if fake.balance < input.Amount {
		return economy.DebitWalletResult{}, fmt.Errorf("have %d need %d: %w", fake.balance, input.Amount, economy.ErrInsufficientWalletFunds)
	}

	fake.balance -= input.Amount
	fake.debited += input.Amount
	result := economy.DebitWalletResult{
		Balance: economy.WalletBalance{
			PlayerID: input.PlayerID,
			Currency: input.Currency,
			Balance:  fake.balance,
		},
		LedgerEntry: economy.CurrencyLedgerEntry{
			LedgerID:     economy.LedgerID(fmt.Sprintf("quest-reroll-ledger-%d", len(fake.calls))),
			PlayerID:     input.PlayerID,
			Currency:     input.Currency,
			Amount:       amount,
			Action:       economy.LedgerActionDecrease,
			BalanceAfter: fake.balance,
			Reason:       input.Reason,
			ReferenceKey: input.ReferenceKey,
		},
	}
	fake.seen[input.ReferenceKey] = result
	return result, nil
}

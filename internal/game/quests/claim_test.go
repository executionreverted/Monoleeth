package quests

import (
	"errors"
	"testing"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

func TestClaimRewardRejectsIncompleteQuestWithoutGrants(t *testing.T) {
	fixture, wallet, inventory, xp := newClaimRewardFixture(t)
	quest := seedClaimQuest(t, fixture, QuestStateAccepted)

	_, err := fixture.service.ClaimReward(ClaimRewardInput{
		PlayerID:      quest.PlayerID,
		PlayerQuestID: quest.PlayerQuestID,
	})
	if !errors.Is(err, ErrInvalidQuestClaim) {
		t.Fatalf("ClaimReward() error = %v, want ErrInvalidQuestClaim", err)
	}
	if len(wallet.calls) != 0 || len(inventory.calls) != 0 || len(xp.calls) != 0 {
		t.Fatalf("grant calls = wallet %d inventory %d xp %d, want none", len(wallet.calls), len(inventory.calls), len(xp.calls))
	}
	stored := mustStoredQuest(t, fixture, quest.PlayerQuestID)
	if stored.State != QuestStateAccepted || stored.ClaimedAt != nil || stored.RewardClaimedAt != nil {
		t.Fatalf("stored quest after rejected claim = state %q claimed_at %v reward_claimed_at %v, want accepted unclaimed", stored.State, stored.ClaimedAt, stored.RewardClaimedAt)
	}
}

func TestClaimRewardGrantsAllRewardsOnceWithQuestRewardReference(t *testing.T) {
	fixture, wallet, inventory, xp := newClaimRewardFixture(t)
	quest := seedClaimQuest(t, fixture, QuestStateCompleted)
	wantReference := RewardReferenceForPlayerQuest(quest.PlayerQuestID)

	result, err := fixture.service.ClaimReward(ClaimRewardInput{
		PlayerID:      quest.PlayerID,
		PlayerQuestID: quest.PlayerQuestID,
	})
	if err != nil {
		t.Fatalf("ClaimReward() = %v, want nil", err)
	}

	if result.Duplicate {
		t.Fatal("ClaimReward() duplicate = true, want false on first claim")
	}
	if result.Quest.State != QuestStateClaimed {
		t.Fatalf("claimed quest state = %q, want %q", result.Quest.State, QuestStateClaimed)
	}
	if result.Quest.ClaimedAt == nil || result.Quest.RewardClaimedAt == nil {
		t.Fatalf("claimed timestamps = claimed_at %v reward_claimed_at %v, want both set", result.Quest.ClaimedAt, result.Quest.RewardClaimedAt)
	}
	if !result.Quest.ClaimedAt.Equal(*result.Quest.RewardClaimedAt) {
		t.Fatalf("claimed_at %s reward_claimed_at %s, want same", result.Quest.ClaimedAt, result.Quest.RewardClaimedAt)
	}
	if result.Quest.RewardReferenceID != wantReference || result.ReferenceKey.String() != wantReference {
		t.Fatalf("claim references = quest %q result %q, want %q", result.Quest.RewardReferenceID, result.ReferenceKey, wantReference)
	}

	assertClaimGrantTotals(t, wallet, inventory, xp, 100, 5, 25, 30)
	assertClaimGrantReferences(t, wallet, inventory, xp, wantReference)

	stored := mustStoredQuest(t, fixture, quest.PlayerQuestID)
	if stored.State != QuestStateClaimed || stored.RewardReferenceID != wantReference {
		t.Fatalf("stored claimed quest = state %q reference %q, want claimed %q", stored.State, stored.RewardReferenceID, wantReference)
	}
}

func TestClaimRewardDuplicateReturnsClaimedResultWithoutDuplicateGrants(t *testing.T) {
	fixture, wallet, inventory, xp := newClaimRewardFixture(t)
	quest := seedClaimQuest(t, fixture, QuestStateCompleted)

	first, err := fixture.service.ClaimReward(ClaimRewardInput{
		PlayerID:      quest.PlayerID,
		PlayerQuestID: quest.PlayerQuestID,
	})
	if err != nil {
		t.Fatalf("ClaimReward(first) = %v, want nil", err)
	}
	fixture.clock.Advance(time.Minute)
	second, err := fixture.service.ClaimReward(ClaimRewardInput{
		PlayerID:      quest.PlayerID,
		PlayerQuestID: quest.PlayerQuestID,
	})
	if err != nil {
		t.Fatalf("ClaimReward(second) = %v, want nil", err)
	}

	if !second.Duplicate {
		t.Fatal("duplicate ClaimReward() duplicate = false, want true")
	}
	if second.Quest.PlayerQuestID != first.Quest.PlayerQuestID {
		t.Fatalf("duplicate quest id = %q, want %q", second.Quest.PlayerQuestID, first.Quest.PlayerQuestID)
	}
	if second.Quest.ClaimedAt == nil || first.Quest.ClaimedAt == nil || !second.Quest.ClaimedAt.Equal(*first.Quest.ClaimedAt) {
		t.Fatalf("duplicate claimed_at = %v, want first %v", second.Quest.ClaimedAt, first.Quest.ClaimedAt)
	}
	if len(wallet.calls) != 1 || len(inventory.calls) != 1 || len(xp.calls) != 1 {
		t.Fatalf("grant calls after duplicate = wallet %d inventory %d xp %d, want one each", len(wallet.calls), len(inventory.calls), len(xp.calls))
	}
	assertClaimGrantTotals(t, wallet, inventory, xp, 100, 5, 25, 30)
}

func TestClaimRewardGrantFailureLeavesQuestRetryableWithSameReference(t *testing.T) {
	fixture, wallet, inventory, xp := newClaimRewardFixture(t)
	quest := seedClaimQuest(t, fixture, QuestStateCompleted)
	wantReference := RewardReferenceForPlayerQuest(quest.PlayerQuestID)
	xp.fail = errors.New("progression unavailable")

	_, err := fixture.service.ClaimReward(ClaimRewardInput{
		PlayerID:      quest.PlayerID,
		PlayerQuestID: quest.PlayerQuestID,
	})
	if !errors.Is(err, xp.fail) {
		t.Fatalf("ClaimReward() error = %v, want %v", err, xp.fail)
	}
	stored := mustStoredQuest(t, fixture, quest.PlayerQuestID)
	if stored.State != QuestStateCompleted || stored.ClaimedAt != nil || stored.RewardClaimedAt != nil {
		t.Fatalf("stored quest after failed claim = state %q claimed_at %v reward_claimed_at %v, want completed unclaimed", stored.State, stored.ClaimedAt, stored.RewardClaimedAt)
	}
	assertClaimGrantTotals(t, wallet, inventory, xp, 100, 5, 0, 0)

	xp.fail = nil
	result, err := fixture.service.ClaimReward(ClaimRewardInput{
		PlayerID:      quest.PlayerID,
		PlayerQuestID: quest.PlayerQuestID,
	})
	if err != nil {
		t.Fatalf("ClaimReward(retry) = %v, want nil", err)
	}
	if result.Credits == nil || !result.Credits.Duplicate {
		t.Fatalf("retry credit result = %+v, want duplicate true", result.Credits)
	}
	if result.Items == nil || !result.Items.Duplicate {
		t.Fatalf("retry item result = %+v, want duplicate true", result.Items)
	}
	if result.Quest.State != QuestStateClaimed {
		t.Fatalf("retry quest state = %q, want claimed", result.Quest.State)
	}
	assertClaimGrantTotals(t, wallet, inventory, xp, 100, 5, 25, 30)
	assertClaimGrantReferences(t, wallet, inventory, xp, wantReference)
}

func TestClaimRewardDoesNotHoldStoreLockWhileGranting(t *testing.T) {
	fixture, wallet, inventory, xp := newClaimRewardFixture(t)
	quest := seedClaimQuest(t, fixture, QuestStateCompleted)
	reentrantWallet := &reentrantQuestRewardWallet{
		fake:     wallet,
		store:    fixture.store,
		playerID: quest.PlayerID,
	}
	fixture.service.SetRewardServices(QuestRewardServices{
		Wallet:      reentrantWallet,
		Inventory:   inventory,
		Progression: xp,
	})

	done := make(chan error, 1)
	go func() {
		_, err := fixture.service.ClaimReward(ClaimRewardInput{
			PlayerID:      quest.PlayerID,
			PlayerQuestID: quest.PlayerQuestID,
		})
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ClaimReward() = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ClaimReward deadlocked while reward wallet re-entered quest store")
	}
}

func newClaimRewardFixture(t *testing.T) (questServiceFixture, *fakeQuestRewardWallet, *fakeQuestRewardInventory, *fakeQuestRewardProgression) {
	t.Helper()
	fixture := newQuestServiceFixture(t, MustMVPQuestCatalog(), time.Date(2026, 6, 17, 12, 5, 0, 0, time.UTC))
	wallet := newFakeQuestRewardWallet()
	inventory := newFakeQuestRewardInventory()
	xp := newFakeQuestRewardProgression()
	fixture.service.SetRewardServices(QuestRewardServices{
		Wallet:      wallet,
		Inventory:   inventory,
		Progression: xp,
	})
	return fixture, wallet, inventory, xp
}

func seedClaimQuest(t *testing.T, fixture questServiceFixture, state QuestState) PlayerQuest {
	t.Helper()
	quest := validPlayerQuest(t, state)
	quest.GeneratedPayload.Objective = validObjectiveSchema(t)
	if err := quest.Validate(); err != nil {
		t.Fatalf("claim test quest Validate() = %v, want nil", err)
	}
	fixture.store.mu.Lock()
	defer fixture.store.mu.Unlock()
	fixture.store.quests[quest.PlayerQuestID] = clonePlayerQuest(quest)
	fixture.store.appendPlayerQuestLocked(quest.PlayerID, quest.PlayerQuestID)
	return quest
}

func mustStoredQuest(t *testing.T, fixture questServiceFixture, questID foundation.QuestID) PlayerQuest {
	t.Helper()
	fixture.store.mu.Lock()
	defer fixture.store.mu.Unlock()
	quest, ok := fixture.store.quests[questID]
	if !ok {
		t.Fatalf("stored quest %q not found", questID)
	}
	return clonePlayerQuest(quest)
}

func assertClaimGrantTotals(
	t *testing.T,
	wallet *fakeQuestRewardWallet,
	inventory *fakeQuestRewardInventory,
	xp *fakeQuestRewardProgression,
	wantCredits int64,
	wantIron int64,
	wantMainXP int64,
	wantCombatXP int64,
) {
	t.Helper()
	if wallet.totalCredits != wantCredits {
		t.Fatalf("wallet credits = %d, want %d", wallet.totalCredits, wantCredits)
	}
	if inventory.itemTotals["iron_ore"] != wantIron {
		t.Fatalf("iron_ore grant total = %d, want %d", inventory.itemTotals["iron_ore"], wantIron)
	}
	if xp.mainXP != wantMainXP {
		t.Fatalf("main XP = %d, want %d", xp.mainXP, wantMainXP)
	}
	if xp.roleXP[progression.RoleTypeCombat] != wantCombatXP {
		t.Fatalf("combat XP = %d, want %d", xp.roleXP[progression.RoleTypeCombat], wantCombatXP)
	}
}

func assertClaimGrantReferences(
	t *testing.T,
	wallet *fakeQuestRewardWallet,
	inventory *fakeQuestRewardInventory,
	xp *fakeQuestRewardProgression,
	wantReference string,
) {
	t.Helper()
	if len(wallet.calls) == 0 || wallet.calls[len(wallet.calls)-1].ReferenceKey.String() != wantReference {
		t.Fatalf("wallet reference calls = %+v, want last %q", wallet.calls, wantReference)
	}
	if len(inventory.calls) == 0 || inventory.calls[len(inventory.calls)-1].ReferenceKey.String() != wantReference {
		t.Fatalf("inventory reference calls = %+v, want last %q", inventory.calls, wantReference)
	}
	if len(xp.calls) == 0 || xp.calls[len(xp.calls)-1].IdempotencyKey.String() != wantReference {
		t.Fatalf("xp reference calls = %+v, want last %q", xp.calls, wantReference)
	}
	if len(xp.calls) > 0 && xp.calls[len(xp.calls)-1].SourceID != progression.XPSourceID("player_quest_1") {
		t.Fatalf("xp source id = %q, want player_quest_1", xp.calls[len(xp.calls)-1].SourceID)
	}
	if len(xp.calls) > 0 && xp.calls[len(xp.calls)-1].Authority != progression.XPGrantAuthorityQuestService {
		t.Fatalf("xp authority = %q, want %q", xp.calls[len(xp.calls)-1].Authority, progression.XPGrantAuthorityQuestService)
	}
}

type fakeQuestRewardWallet struct {
	calls        []economy.CreditWalletInput
	totalCredits int64
	seen         map[foundation.IdempotencyKey]economy.CreditWalletResult
	fail         error
}

type reentrantQuestRewardWallet struct {
	fake     *fakeQuestRewardWallet
	store    *InMemoryQuestStore
	playerID foundation.PlayerID
}

func (wallet *reentrantQuestRewardWallet) CreditWallet(input economy.CreditWalletInput) (economy.CreditWalletResult, error) {
	if _, err := wallet.store.PlayerQuests(wallet.playerID); err != nil {
		return economy.CreditWalletResult{}, err
	}
	return wallet.fake.CreditWallet(input)
}

func newFakeQuestRewardWallet() *fakeQuestRewardWallet {
	return &fakeQuestRewardWallet{seen: make(map[foundation.IdempotencyKey]economy.CreditWalletResult)}
}

func (fake *fakeQuestRewardWallet) CreditWallet(input economy.CreditWalletInput) (economy.CreditWalletResult, error) {
	fake.calls = append(fake.calls, input)
	if fake.fail != nil {
		return economy.CreditWalletResult{}, fake.fail
	}
	if previous, ok := fake.seen[input.ReferenceKey]; ok {
		previous.Duplicate = true
		return previous, nil
	}
	fake.totalCredits += input.Amount
	result := economy.CreditWalletResult{
		Balance: economy.WalletBalance{
			PlayerID: input.PlayerID,
			Currency: input.Currency,
			Balance:  fake.totalCredits,
		},
		LedgerEntry: economy.CurrencyLedgerEntry{
			PlayerID:     input.PlayerID,
			Currency:     input.Currency,
			Reason:       input.Reason,
			ReferenceKey: input.ReferenceKey,
		},
	}
	fake.seen[input.ReferenceKey] = result
	return result, nil
}

type fakeQuestRewardInventory struct {
	calls      []QuestRewardItemGrantInput
	itemTotals map[foundation.ItemID]int64
	seen       map[foundation.IdempotencyKey]QuestRewardItemGrantResult
	fail       error
}

func newFakeQuestRewardInventory() *fakeQuestRewardInventory {
	return &fakeQuestRewardInventory{
		itemTotals: make(map[foundation.ItemID]int64),
		seen:       make(map[foundation.IdempotencyKey]QuestRewardItemGrantResult),
	}
}

func (fake *fakeQuestRewardInventory) GrantQuestRewardItems(input QuestRewardItemGrantInput) (QuestRewardItemGrantResult, error) {
	input.Items = cloneQuestRewardItemGrants(input.Items)
	fake.calls = append(fake.calls, input)
	if fake.fail != nil {
		return QuestRewardItemGrantResult{}, fake.fail
	}
	if previous, ok := fake.seen[input.ReferenceKey]; ok {
		previous.Duplicate = true
		return previous, nil
	}
	for _, item := range input.Items {
		fake.itemTotals[item.ItemID] += item.Quantity
	}
	result := QuestRewardItemGrantResult{
		Items:        cloneQuestRewardItemGrants(input.Items),
		ReferenceKey: input.ReferenceKey,
	}
	fake.seen[input.ReferenceKey] = result
	return result, nil
}

type fakeQuestRewardProgression struct {
	calls  []progression.GrantXPInput
	mainXP int64
	roleXP map[progression.RoleType]int64
	seen   map[progression.XPIdempotencyKey]progression.GrantXPResult
	fail   error
}

func newFakeQuestRewardProgression() *fakeQuestRewardProgression {
	return &fakeQuestRewardProgression{
		roleXP: make(map[progression.RoleType]int64),
		seen:   make(map[progression.XPIdempotencyKey]progression.GrantXPResult),
	}
}

func (fake *fakeQuestRewardProgression) GrantXP(input progression.GrantXPInput) (progression.GrantXPResult, error) {
	input.RoleXP = append([]progression.RoleXPGrant(nil), input.RoleXP...)
	fake.calls = append(fake.calls, input)
	if fake.fail != nil {
		return progression.GrantXPResult{}, fake.fail
	}
	if previous, ok := fake.seen[input.IdempotencyKey]; ok {
		previous.Duplicate = true
		return previous, nil
	}
	fake.mainXP += input.Amount
	for _, roleGrant := range input.RoleXP {
		fake.roleXP[roleGrant.Role] += roleGrant.Amount
	}
	result := progression.GrantXPResult{
		RecordedXPGrantSourceType: input.SourceType,
		RecordedXPGrantSourceID:   input.SourceID,
	}
	fake.seen[input.IdempotencyKey] = result
	return result, nil
}

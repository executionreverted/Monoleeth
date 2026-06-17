package quests

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
)

const (
	questRerollLedgerReason economy.LedgerReason = "quest_reroll"
	defaultRerollBaseCost   int64                = 250
	defaultRerollRankCost   int64                = 25
)

// QuestRerollWalletService is the wallet boundary used for board reroll
// credits. It intentionally uses the economy debit DTO so the ledger reference
// stays compatible with the wallet service.
type QuestRerollWalletService interface {
	DebitWallet(economy.DebitWalletInput) (economy.DebitWalletResult, error)
}

// QuestRerollServices wires RerollBoard to the owning wallet service.
type QuestRerollServices struct {
	Wallet QuestRerollWalletService
}

// RerollCostHookKind names placeholder policies used to calculate reroll cost.
type RerollCostHookKind string

const (
	RerollCostHookRankScaling RerollCostHookKind = "rank_scaling"
)

// QuestRerollCostHook calculates the server-owned reroll cost for a snapshot.
type QuestRerollCostHook func(PlayerQuestBoardSnapshot) (RerollCost, error)

// RerollCost describes the server-calculated credit sink for one reroll.
type RerollCost struct {
	Currency economy.CurrencyBucket `json:"currency_type"`
	Amount   int64                  `json:"amount"`
	Hooks    []RerollCostHook       `json:"hooks,omitempty"`
}

// RerollCostHook records which cost policy placeholder was applied.
type RerollCostHook struct {
	Kind          RerollCostHookKind `json:"kind"`
	Key           string             `json:"key,omitempty"`
	BaseAmount    int64              `json:"base_amount,omitempty"`
	PerRankAmount int64              `json:"per_rank_amount,omitempty"`
	AppliedRank   int                `json:"applied_rank,omitempty"`
}

// RerollBoardInput carries server-owned player state, a deterministic
// generation seed, and a one-shot reroll reference for idempotency.
type RerollBoardInput struct {
	Player      PlayerQuestBoardSnapshot `json:"player"`
	Seed        int64                    `json:"seed"`
	ReferenceID string                   `json:"reference_id"`
	CreatedAt   time.Time                `json:"created_at"`

	WeightHook QuestWeightHook     `json:"-"`
	CostHook   QuestRerollCostHook `json:"-"`
}

// RerollBoardResult reports the debited cost and newly stored board.
type RerollBoardResult struct {
	PlayerID     foundation.PlayerID       `json:"player_id"`
	Seed         int64                     `json:"seed"`
	Offers       []GeneratedBoardOffer     `json:"offers"`
	Cost         RerollCost                `json:"cost"`
	WalletDebit  economy.DebitWalletResult `json:"wallet_debit"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
	RerolledAt   time.Time                 `json:"rerolled_at"`
	RareCapHooks []RewardHook              `json:"rare_cap_hooks,omitempty"`
	Duplicate    bool                      `json:"duplicate"`
}

// RerollBoard debits the server-calculated credit cost once, expires old
// unaccepted offers, and stores a fresh deterministic board of ten offers.
func (service *QuestService) RerollBoard(input RerollBoardInput) (RerollBoardResult, error) {
	if input.CreatedAt.IsZero() {
		input.CreatedAt = service.clock.Now().UTC()
	} else {
		input.CreatedAt = input.CreatedAt.UTC()
	}
	if err := input.Validate(); err != nil {
		return RerollBoardResult{}, err
	}

	referenceKey, err := input.StableReferenceKey()
	if err != nil {
		return RerollBoardResult{}, err
	}
	cost, err := input.ResolveCost()
	if err != nil {
		return RerollBoardResult{}, err
	}

	offers, err := service.generateRerollOffers(input, referenceKey)
	if err != nil {
		return RerollBoardResult{}, err
	}
	rareCapHooks := rareCapHooksFromOffers(offers)

	service.store.mu.Lock()
	if previous, ok := service.store.rerollResults[referenceKey]; ok {
		service.store.mu.Unlock()
		result := cloneRerollBoardResult(previous)
		result.Duplicate = true
		result.WalletDebit.Duplicate = true
		return result, nil
	}
	if err := service.store.ensureRerollOffersCanStoreLocked(input.Player.PlayerID, offers); err != nil {
		service.store.mu.Unlock()
		return RerollBoardResult{}, err
	}
	service.store.mu.Unlock()

	rerollWallet := service.rerollWalletService()
	if rerollWallet == nil {
		return RerollBoardResult{}, ErrMissingQuestRerollWalletService
	}

	walletDebit, err := rerollWallet.DebitWallet(economy.DebitWalletInput{
		PlayerID:     input.Player.PlayerID,
		Currency:     cost.Currency,
		Amount:       cost.Amount,
		Reason:       questRerollLedgerReason,
		ReferenceKey: referenceKey,
	})
	if err != nil {
		return RerollBoardResult{}, err
	}

	service.store.mu.Lock()
	defer service.store.mu.Unlock()

	if previous, ok := service.store.rerollResults[referenceKey]; ok {
		result := cloneRerollBoardResult(previous)
		result.Duplicate = true
		result.WalletDebit.Duplicate = true
		return result, nil
	}
	if err := service.store.storeRerolledBoardOffersLocked(input.Player.PlayerID, offers); err != nil {
		return RerollBoardResult{}, err
	}

	result := RerollBoardResult{
		PlayerID:     input.Player.PlayerID,
		Seed:         input.Seed,
		Offers:       cloneGeneratedBoardOffers(offers),
		Cost:         cloneRerollCost(cost),
		WalletDebit:  walletDebit,
		ReferenceKey: referenceKey,
		RerolledAt:   input.CreatedAt,
		RareCapHooks: append([]RewardHook(nil), rareCapHooks...),
	}
	service.store.rerollResults[referenceKey] = cloneRerollBoardResult(result)
	return cloneRerollBoardResult(result), nil
}

// Validate reports whether the reroll request has server-owned player state,
// a deterministic positive seed, a one-shot reference, and a server timestamp.
func (input RerollBoardInput) Validate() error {
	if err := input.Player.Validate(); err != nil {
		return err
	}
	if input.Seed <= 0 {
		return fmt.Errorf("seed %d: %w", input.Seed, ErrInvalidQuestRerollSeed)
	}
	if strings.TrimSpace(input.ReferenceID) == "" || input.ReferenceID != strings.TrimSpace(input.ReferenceID) {
		return fmt.Errorf("reference_id %q: %w", input.ReferenceID, ErrInvalidQuestRerollReference)
	}
	if input.CreatedAt.IsZero() {
		return fmt.Errorf("created_at: %w", ErrZeroQuestTime)
	}
	return nil
}

// StableReferenceKey returns quest_reroll:<player_id>:<reference_id>.
func (input RerollBoardInput) StableReferenceKey() (foundation.IdempotencyKey, error) {
	return foundation.QuestRerollIdempotencyKey(input.Player.PlayerID, input.ReferenceID)
}

// ResolveCost applies the configured cost hook or the default rank-scaling
// placeholder, then validates the result.
func (input RerollBoardInput) ResolveCost() (RerollCost, error) {
	hook := input.CostHook
	if hook == nil {
		hook = DefaultQuestRerollCostHook
	}
	cost, err := hook(input.Player)
	if err != nil {
		return RerollCost{}, err
	}
	if err := cost.Validate(); err != nil {
		return RerollCost{}, err
	}
	return cloneRerollCost(cost), nil
}

// DefaultQuestRerollCostHook is the MVP deterministic reroll cost policy.
func DefaultQuestRerollCostHook(snapshot PlayerQuestBoardSnapshot) (RerollCost, error) {
	rank := snapshot.Rank
	amount := defaultRerollBaseCost + int64(rank-1)*defaultRerollRankCost
	return RerollCost{
		Currency: economy.CurrencyBucketCredits,
		Amount:   amount,
		Hooks: []RerollCostHook{{
			Kind:          RerollCostHookRankScaling,
			Key:           "mvp_rank_scaling",
			BaseAmount:    defaultRerollBaseCost,
			PerRankAmount: defaultRerollRankCost,
			AppliedRank:   rank,
		}},
	}, nil
}

// Validate reports whether kind is a supported reroll cost policy placeholder.
func (kind RerollCostHookKind) Validate() error {
	switch kind {
	case RerollCostHookRankScaling:
		return nil
	default:
		return fmt.Errorf("reroll cost hook kind %q: %w", kind, ErrInvalidQuestRerollHook)
	}
}

// Validate reports whether the cost is a positive credits debit with valid hooks.
func (cost RerollCost) Validate() error {
	if cost.Currency != economy.CurrencyBucketCredits {
		return fmt.Errorf("reroll currency %q: %w", cost.Currency, ErrInvalidQuestRerollCost)
	}
	if err := foundation.ValidatePositiveAmount(cost.Amount); err != nil {
		return fmt.Errorf("reroll amount %d: %w", cost.Amount, ErrInvalidQuestRerollCost)
	}
	if len(cost.Hooks) == 0 {
		return fmt.Errorf("reroll cost hooks: %w", ErrInvalidQuestRerollHook)
	}
	for _, hook := range cost.Hooks {
		if err := hook.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate reports whether the cost hook marker is well-formed.
func (hook RerollCostHook) Validate() error {
	if err := hook.Kind.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(hook.Key) == "" || hook.Key != strings.TrimSpace(hook.Key) {
		return fmt.Errorf("reroll cost hook key %q: %w", hook.Key, ErrInvalidQuestRerollHook)
	}
	if hook.BaseAmount <= 0 || hook.PerRankAmount < 0 || hook.AppliedRank <= 0 {
		return fmt.Errorf("reroll cost hook %+v: %w", hook, ErrInvalidQuestRerollHook)
	}
	return nil
}

func (kind RerollCostHookKind) String() string { return string(kind) }

func (service *QuestService) generateRerollOffers(input RerollBoardInput, referenceKey foundation.IdempotencyKey) ([]GeneratedBoardOffer, error) {
	boardInput := BoardGenerationInput{
		Player:     input.Player,
		Seed:       rerollGenerationSeed(input, referenceKey),
		Catalog:    service.catalog,
		CreatedAt:  input.CreatedAt,
		WeightHook: input.WeightHook,
	}
	offers, err := GenerateBoard(boardInput)
	if err != nil {
		return nil, err
	}
	if len(offers) != BoardOfferCount {
		return nil, fmt.Errorf("reroll generated %d offers want %d: %w", len(offers), BoardOfferCount, ErrInvalidQuestReroll)
	}
	return offers, nil
}

func rerollGenerationSeed(input RerollBoardInput, referenceKey foundation.IdempotencyKey) int64 {
	return stableInt64(
		"quest-board-reroll-generation",
		input.Player.canonicalKey(),
		strconv.FormatInt(input.Seed, 10),
		referenceKey.String(),
	)
}

func rareCapHooksForGeneratedRewards(snapshot PlayerQuestBoardSnapshot) []RewardHook {
	return []RewardHook{{
		Kind:  RewardHookRareCap,
		Key:   fmt.Sprintf("quest_board_rare_daily:%s:rank_%d", snapshot.PlayerID, snapshot.Rank),
		Limit: 1,
	}}
}

func rareCapHooksFromOffers(offers []GeneratedBoardOffer) []RewardHook {
	seen := make(map[RewardHook]struct{})
	for _, offer := range offers {
		for _, hook := range offer.RewardPayload.RareCapHooks {
			seen[hook] = struct{}{}
		}
		for _, hook := range offer.RewardPayload.Hooks {
			if hook.Kind == RewardHookRareCap {
				seen[hook] = struct{}{}
			}
		}
	}
	hooks := make([]RewardHook, 0, len(seen))
	for hook := range seen {
		hooks = append(hooks, hook)
	}
	sort.Slice(hooks, func(i, j int) bool {
		if hooks[i].Kind != hooks[j].Kind {
			return hooks[i].Kind < hooks[j].Kind
		}
		if hooks[i].Key != hooks[j].Key {
			return hooks[i].Key < hooks[j].Key
		}
		return hooks[i].Limit < hooks[j].Limit
	})
	return hooks
}

func cloneRerollBoardResult(result RerollBoardResult) RerollBoardResult {
	result.Offers = cloneGeneratedBoardOffers(result.Offers)
	result.Cost = cloneRerollCost(result.Cost)
	result.RareCapHooks = append([]RewardHook(nil), result.RareCapHooks...)
	return result
}

func cloneRerollCost(cost RerollCost) RerollCost {
	cost.Hooks = append([]RerollCostHook(nil), cost.Hooks...)
	return cost
}

func cloneGeneratedBoardOffers(offers []GeneratedBoardOffer) []GeneratedBoardOffer {
	cloned := make([]GeneratedBoardOffer, 0, len(offers))
	for _, offer := range offers {
		cloned = append(cloned, cloneGeneratedBoardOffer(offer))
	}
	return cloned
}

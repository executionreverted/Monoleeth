package quests

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
)

const questRewardLedgerReason economy.LedgerReason = "quest_reward"

// QuestRewardWalletService is the wallet boundary used for quest credit grants.
type QuestRewardWalletService interface {
	CreditWallet(economy.CreditWalletInput) (economy.CreditWalletResult, error)
}

// QuestRewardInventoryService is the inventory boundary used for quest item
// grants. It batches all generated item grants under one quest reward reference.
type QuestRewardInventoryService interface {
	GrantQuestRewardItems(QuestRewardItemGrantInput) (QuestRewardItemGrantResult, error)
}

// QuestRewardProgressionService is the progression boundary used for quest XP.
type QuestRewardProgressionService interface {
	GrantXP(progression.GrantXPInput) (progression.GrantXPResult, error)
}

// QuestRewardServices wires ClaimReward to the owning value services.
type QuestRewardServices struct {
	Wallet      QuestRewardWalletService
	Inventory   QuestRewardInventoryService
	Progression QuestRewardProgressionService
}

// ClaimRewardInput names one server-owned completed quest to claim.
type ClaimRewardInput struct {
	PlayerID      foundation.PlayerID `json:"player_id"`
	PlayerQuestID foundation.QuestID  `json:"player_quest_id"`
}

// QuestRewardItemGrant is one item grant inside a quest reward payload.
type QuestRewardItemGrant struct {
	ItemID   foundation.ItemID `json:"item_id"`
	Quantity int64             `json:"quantity"`
}

// QuestRewardItemGrantInput carries all quest item grants for one reference.
type QuestRewardItemGrantInput struct {
	PlayerID     foundation.PlayerID       `json:"player_id"`
	Items        []QuestRewardItemGrant    `json:"items"`
	Reason       economy.LedgerReason      `json:"reason"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
}

// QuestRewardItemGrantResult reports the inventory boundary result.
type QuestRewardItemGrantResult struct {
	Items        []QuestRewardItemGrant    `json:"items"`
	ReferenceKey foundation.IdempotencyKey `json:"reference_id"`
	Duplicate    bool                      `json:"duplicate"`
}

// ClaimRewardResult reports the claimed quest and value-service results.
type ClaimRewardResult struct {
	Quest        PlayerQuest                 `json:"quest"`
	ReferenceKey foundation.IdempotencyKey   `json:"reference_id"`
	Credits      *economy.CreditWalletResult `json:"credits,omitempty"`
	Items        *QuestRewardItemGrantResult `json:"items,omitempty"`
	XP           *progression.GrantXPResult  `json:"xp,omitempty"`
	Duplicate    bool                        `json:"duplicate"`
}

type rewardClaimPlan struct {
	creditAmount int64
	items        []QuestRewardItemGrant
	mainXP       int64
	roleXP       []progression.RoleXPGrant
}

type rewardClaimPreparation struct {
	quest        PlayerQuest
	claimedQuest PlayerQuest
	referenceKey foundation.IdempotencyKey
	claimPlan    rewardClaimPlan
	result       ClaimRewardResult
}

// ClaimReward grants a completed quest's generated rewards exactly once. Quest
// state is checked and finalized under the store lock, while value-service calls
// happen outside that lock and rely on the quest_reward reference for duplicate
// safety.
func (service *QuestService) ClaimReward(input ClaimRewardInput) (ClaimRewardResult, error) {
	result, err := service.claimReward(input)
	if err != nil {
		return result, PublicClaimRewardError(err)
	}
	return result, nil
}

func (service *QuestService) claimReward(input ClaimRewardInput) (ClaimRewardResult, error) {
	if err := input.Validate(); err != nil {
		return ClaimRewardResult{}, err
	}

	claimedAt := service.clock.Now().UTC()
	if claimedAt.IsZero() {
		return ClaimRewardResult{}, fmt.Errorf("claimed_at: %w", ErrZeroQuestTime)
	}

	service.store.mu.Lock()
	prep, duplicate, err := service.prepareClaimRewardLocked(input, claimedAt)
	service.store.mu.Unlock()
	if err != nil || duplicate {
		return cloneClaimRewardResult(prep.result), err
	}

	services := service.rewardServices()
	result := cloneClaimRewardResult(prep.result)

	if prep.claimPlan.creditAmount > 0 {
		if services.Wallet == nil {
			return ClaimRewardResult{}, ErrMissingQuestRewardWalletService
		}
		credits, err := services.Wallet.CreditWallet(economy.CreditWalletInput{
			PlayerID:     prep.quest.PlayerID,
			Currency:     economy.CurrencyBucketCredits,
			Amount:       prep.claimPlan.creditAmount,
			Reason:       questRewardLedgerReason,
			ReferenceKey: prep.referenceKey,
		})
		if err != nil {
			return result, err
		}
		result.Credits = &credits
	}

	if len(prep.claimPlan.items) > 0 {
		if services.Inventory == nil {
			return ClaimRewardResult{}, ErrMissingQuestRewardInventoryService
		}
		items, err := services.Inventory.GrantQuestRewardItems(QuestRewardItemGrantInput{
			PlayerID:     prep.quest.PlayerID,
			Items:        cloneQuestRewardItemGrants(prep.claimPlan.items),
			Reason:       questRewardLedgerReason,
			ReferenceKey: prep.referenceKey,
		})
		if err != nil {
			return result, err
		}
		result.Items = cloneQuestRewardItemGrantResultPtr(items)
	}

	if prep.claimPlan.mainXP > 0 || len(prep.claimPlan.roleXP) > 0 {
		if services.Progression == nil {
			return ClaimRewardResult{}, ErrMissingQuestRewardProgressionService
		}
		xp, err := services.Progression.GrantXP(progression.GrantXPInput{
			PlayerID:       prep.quest.PlayerID,
			Amount:         prep.claimPlan.mainXP,
			SourceType:     progression.XPSourceTypeQuest,
			SourceID:       progression.XPSourceID(prep.quest.PlayerQuestID.String()),
			IdempotencyKey: progression.XPIdempotencyKey(prep.referenceKey.String()),
			Authority:      progression.XPGrantAuthorityQuestService,
			RoleXP:         cloneRoleXPGrants(prep.claimPlan.roleXP),
		})
		if err != nil {
			return result, err
		}
		result.XP = cloneGrantXPResultPtr(xp)
	}

	service.store.mu.Lock()
	defer service.store.mu.Unlock()

	return service.finalizeClaimRewardLocked(input, prep.claimedQuest, result)
}

// PublicClaimRewardError returns a client-safe claim error while preserving the
// original cause for server-side errors.Is/errors.As checks and diagnostics.
func PublicClaimRewardError(err error) *foundation.DomainError {
	if err == nil {
		return nil
	}

	code := foundation.CodeInternal
	message := "Request failed."
	switch {
	case errors.Is(err, foundation.ErrEmptyID), errors.Is(err, foundation.ErrInvalidID):
		code = foundation.CodeInvalidPayload
		message = "Invalid quest reward claim."
	case errors.Is(err, ErrPlayerQuestNotFound), errors.Is(err, ErrPlayerQuestOwnerMismatch):
		code = foundation.CodeNotFound
		message = "Quest reward is not available."
	case isQuestRewardClaimStateError(err):
		code = foundation.CodeForbidden
		message = "Quest reward cannot be claimed."
	}

	return foundation.NewDomainError(code, message, foundation.WithCause(err))
}

// Validate reports whether input identifies a player-owned quest.
func (input ClaimRewardInput) Validate() error {
	if err := input.PlayerID.Validate(); err != nil {
		return err
	}
	if err := input.PlayerQuestID.Validate(); err != nil {
		return err
	}
	return nil
}

func isQuestRewardClaimStateError(err error) bool {
	for _, target := range []error{
		ErrInvalidQuestClaim,
		ErrInvalidQuestState,
		ErrInvalidQuestStateTransition,
		ErrInvalidQuestTime,
		ErrInvalidQuestProgress,
		ErrInvalidQuestCompletion,
		ErrEmptyRewardPayload,
		ErrInvalidRewardKind,
		ErrInvalidRewardAmount,
		ErrInvalidRewardCurrency,
		ErrInvalidRewardItem,
		ErrInvalidRewardRole,
		ErrInvalidRewardHook,
		ErrDuplicateRewardHook,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (service *QuestService) prepareClaimRewardLocked(input ClaimRewardInput, claimedAt time.Time) (rewardClaimPreparation, bool, error) {
	quest, ok := service.store.quests[input.PlayerQuestID]
	if !ok {
		return rewardClaimPreparation{}, false, fmt.Errorf("quest %q: %w", input.PlayerQuestID, ErrPlayerQuestNotFound)
	}
	if quest.PlayerID != input.PlayerID {
		return rewardClaimPreparation{}, false, fmt.Errorf("quest %q owner %q player %q: %w", input.PlayerQuestID, quest.PlayerID, input.PlayerID, ErrPlayerQuestOwnerMismatch)
	}

	if quest.State == QuestStateClaimed {
		result, err := service.duplicateClaimResultLocked(quest)
		return rewardClaimPreparation{result: result}, true, err
	}
	if quest.State != QuestStateCompleted {
		return rewardClaimPreparation{}, false, fmt.Errorf("quest %q state %q: %w", input.PlayerQuestID, quest.State, ErrInvalidQuestClaim)
	}
	if err := quest.Validate(); err != nil {
		return rewardClaimPreparation{}, false, err
	}

	referenceKey, err := rewardReferenceKeyForQuest(quest)
	if err != nil {
		return rewardClaimPreparation{}, false, err
	}
	claimPlan, err := buildRewardClaimPlan(quest.RewardPayload)
	if err != nil {
		return rewardClaimPreparation{}, false, err
	}

	claimedQuest := clonePlayerQuest(quest)
	claimedQuest.State = QuestStateClaimed
	claimedQuest.ClaimedAt = cloneTimePtr(&claimedAt)
	claimedQuest.RewardClaimedAt = cloneTimePtr(&claimedAt)
	claimedQuest.RewardReferenceID = referenceKey.String()
	if err := claimedQuest.Validate(); err != nil {
		return rewardClaimPreparation{}, false, err
	}

	return rewardClaimPreparation{
		quest:        clonePlayerQuest(quest),
		claimedQuest: clonePlayerQuest(claimedQuest),
		referenceKey: referenceKey,
		claimPlan:    claimPlan,
		result: ClaimRewardResult{
			Quest:        clonePlayerQuest(claimedQuest),
			ReferenceKey: referenceKey,
		},
	}, false, nil
}

func (service *QuestService) finalizeClaimRewardLocked(input ClaimRewardInput, claimedQuest PlayerQuest, result ClaimRewardResult) (ClaimRewardResult, error) {
	current, ok := service.store.quests[input.PlayerQuestID]
	if !ok {
		return ClaimRewardResult{}, fmt.Errorf("quest %q: %w", input.PlayerQuestID, ErrPlayerQuestNotFound)
	}
	if current.PlayerID != input.PlayerID {
		return ClaimRewardResult{}, fmt.Errorf("quest %q owner %q player %q: %w", input.PlayerQuestID, current.PlayerID, input.PlayerID, ErrPlayerQuestOwnerMismatch)
	}
	if current.State == QuestStateClaimed {
		return service.duplicateClaimResultLocked(current)
	}
	if current.State != QuestStateCompleted {
		return ClaimRewardResult{}, fmt.Errorf("quest %q state %q: %w", input.PlayerQuestID, current.State, ErrInvalidQuestClaim)
	}

	service.store.quests[claimedQuest.PlayerQuestID] = clonePlayerQuest(claimedQuest)
	service.store.claimResults[claimedQuest.PlayerQuestID] = cloneClaimRewardResult(result)
	return cloneClaimRewardResult(result), nil
}

func (service *QuestService) duplicateClaimResultLocked(quest PlayerQuest) (ClaimRewardResult, error) {
	if err := quest.Validate(); err != nil {
		return ClaimRewardResult{}, err
	}
	if result, ok := service.store.claimResults[quest.PlayerQuestID]; ok {
		duplicate := cloneClaimRewardResult(result)
		duplicate.Quest = clonePlayerQuest(quest)
		duplicate.Duplicate = true
		return duplicate, nil
	}
	referenceKey, err := rewardReferenceKeyForQuest(quest)
	if err != nil {
		return ClaimRewardResult{}, err
	}
	return ClaimRewardResult{
		Quest:        clonePlayerQuest(quest),
		ReferenceKey: referenceKey,
		Duplicate:    true,
	}, nil
}

func rewardReferenceKeyForQuest(quest PlayerQuest) (foundation.IdempotencyKey, error) {
	referenceKey, err := foundation.QuestRewardIdempotencyKey(quest.PlayerQuestID)
	if err != nil {
		return "", err
	}
	if quest.RewardReferenceID != "" && quest.RewardReferenceID != referenceKey.String() {
		return "", fmt.Errorf("reward reference %q want %q: %w", quest.RewardReferenceID, referenceKey, ErrInvalidQuestClaim)
	}
	return referenceKey, nil
}

func buildRewardClaimPlan(payload RewardPayload) (rewardClaimPlan, error) {
	if err := payload.Validate(); err != nil {
		return rewardClaimPlan{}, err
	}

	plan := rewardClaimPlan{}
	itemAmounts := make(map[foundation.ItemID]int64)
	roleAmounts := make(map[progression.RoleType]int64)
	for _, grant := range payload.Grants {
		if err := grant.Validate(); err != nil {
			return rewardClaimPlan{}, err
		}
		var err error
		switch grant.Kind {
		case RewardKindCredits:
			plan.creditAmount, err = addRewardGrantAmount(plan.creditAmount, grant.Amount)
		case RewardKindItem:
			itemAmounts[grant.ItemID], err = addRewardGrantAmount(itemAmounts[grant.ItemID], grant.Amount)
		case RewardKindMainXP:
			plan.mainXP, err = addRewardGrantAmount(plan.mainXP, grant.Amount)
		case RewardKindRoleXP:
			roleAmounts[grant.Role], err = addRewardGrantAmount(roleAmounts[grant.Role], grant.Amount)
		default:
			err = grant.Kind.Validate()
		}
		if err != nil {
			return rewardClaimPlan{}, err
		}
	}

	itemIDs := make([]foundation.ItemID, 0, len(itemAmounts))
	for itemID := range itemAmounts {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Slice(itemIDs, func(i, j int) bool { return itemIDs[i] < itemIDs[j] })
	for _, itemID := range itemIDs {
		plan.items = append(plan.items, QuestRewardItemGrant{ItemID: itemID, Quantity: itemAmounts[itemID]})
	}

	roles := make([]progression.RoleType, 0, len(roleAmounts))
	for role := range roleAmounts {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i] < roles[j] })
	for _, role := range roles {
		plan.roleXP = append(plan.roleXP, progression.RoleXPGrant{Role: role, Amount: roleAmounts[role]})
	}

	return plan, nil
}

func addRewardGrantAmount(current int64, amount int64) (int64, error) {
	if amount > math.MaxInt64-current {
		return 0, fmt.Errorf("reward amount overflow: %w", ErrInvalidRewardAmount)
	}
	return current + amount, nil
}

func cloneClaimRewardResult(result ClaimRewardResult) ClaimRewardResult {
	result.Quest = clonePlayerQuest(result.Quest)
	if result.Credits != nil {
		credits := *result.Credits
		result.Credits = &credits
	}
	if result.Items != nil {
		result.Items = cloneQuestRewardItemGrantResultPtr(*result.Items)
	}
	if result.XP != nil {
		result.XP = cloneGrantXPResultPtr(*result.XP)
	}
	return result
}

func cloneQuestRewardItemGrantResultPtr(result QuestRewardItemGrantResult) *QuestRewardItemGrantResult {
	cloned := result
	cloned.Items = cloneQuestRewardItemGrants(result.Items)
	return &cloned
}

func cloneQuestRewardItemGrants(items []QuestRewardItemGrant) []QuestRewardItemGrant {
	return append([]QuestRewardItemGrant(nil), items...)
}

func cloneGrantXPResultPtr(result progression.GrantXPResult) *progression.GrantXPResult {
	cloned := result
	cloned.Snapshot = result.Snapshot.Clone()
	cloned.RoleLevelUps = append([]progression.RoleLevelChange(nil), result.RoleLevelUps...)
	cloned.StatInvalidationSignals = append([]progression.StatInvalidationSignal(nil), result.StatInvalidationSignals...)
	return &cloned
}

func cloneRoleXPGrants(grants []progression.RoleXPGrant) []progression.RoleXPGrant {
	return append([]progression.RoleXPGrant(nil), grants...)
}

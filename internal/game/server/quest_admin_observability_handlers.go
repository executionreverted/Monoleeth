package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"gameproject/internal/game/admin"
	"gameproject/internal/game/auth"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/realtime"
)

type questBoardSummaryPayload struct {
	Offers       []questOfferPayload `json:"offers"`
	Active       []questPayload      `json:"active"`
	Counts       questCountsPayload  `json:"counts"`
	RerollCost   questRerollCost     `json:"reroll_cost"`
	CanReroll    bool                `json:"can_reroll"`
	LockedReason string              `json:"locked_reason,omitempty"`
	ResetAt      int64               `json:"reset_at,omitempty"`
	GeneratedAt  int64               `json:"generated_at"`
	Revision     int64               `json:"revision"`
}

type questCountsPayload struct {
	Offers    int `json:"offers"`
	Active    int `json:"active"`
	Completed int `json:"completed"`
	Claimable int `json:"claimable"`
	Claimed   int `json:"claimed"`
}

type questOfferPayload struct {
	OfferID      string                  `json:"offer_id"`
	Type         string                  `json:"quest_type"`
	Title        string                  `json:"title"`
	Description  string                  `json:"description"`
	Objectives   []questObjectivePayload `json:"objectives"`
	Rewards      []questRewardPayload    `json:"rewards"`
	ExpiresAt    int64                   `json:"expires_at"`
	CanAccept    bool                    `json:"can_accept"`
	LockedReason string                  `json:"locked_reason,omitempty"`
}

type questPayload struct {
	QuestID         string                  `json:"quest_id"`
	AcceptedOfferID string                  `json:"accepted_offer_id,omitempty"`
	Type            string                  `json:"quest_type"`
	Title           string                  `json:"title"`
	Description     string                  `json:"description"`
	State           string                  `json:"state"`
	Objectives      []questObjectivePayload `json:"objectives"`
	Rewards         []questRewardPayload    `json:"rewards"`
	AcceptedAt      int64                   `json:"accepted_at"`
	CompletedAt     int64                   `json:"completed_at,omitempty"`
	ClaimedAt       int64                   `json:"claimed_at,omitempty"`
	CanClaim        bool                    `json:"can_claim"`
}

type questObjectivePayload struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Target      string `json:"target,omitempty"`
	DisplayName string `json:"display_name"`
	CatalogRef  string `json:"catalog_ref,omitempty"`
	ArtKey      string `json:"art_key,omitempty"`
	Current     int64  `json:"current"`
	Required    int64  `json:"required"`
	Completed   bool   `json:"completed"`
}

type questRewardPayload struct {
	Kind        string `json:"kind"`
	Currency    string `json:"currency_type,omitempty"`
	ItemID      string `json:"item_id,omitempty"`
	Role        string `json:"role,omitempty"`
	DisplayName string `json:"display_name"`
	CatalogRef  string `json:"catalog_ref,omitempty"`
	ArtKey      string `json:"art_key,omitempty"`
	Amount      int64  `json:"amount"`
}

type questRerollCost struct {
	Currency string `json:"currency_type"`
	Amount   int64  `json:"amount"`
}

type questMutationPayload struct {
	Accepted    bool                        `json:"accepted"`
	Duplicate   bool                        `json:"duplicate,omitempty"`
	Quest       *questPayload               `json:"quest,omitempty"`
	QuestBoard  questBoardSummaryPayload    `json:"quest_board"`
	Wallet      walletSnapshotPayload       `json:"wallet"`
	Inventory   *inventorySnapshotPayload   `json:"inventory,omitempty"`
	Progression *progressionSnapshotPayload `json:"progression,omitempty"`
}

type adminPlayerInspectionPayload struct {
	Target      string                   `json:"target"`
	Inventory   adminInventoryInspection `json:"inventory"`
	Wallet      adminWalletInspection    `json:"wallet"`
	GeneratedAt int64                    `json:"generated_at"`
}

type adminInventoryInspection struct {
	StackableItems int                    `json:"stackable_items"`
	InstanceItems  int                    `json:"instance_items"`
	ItemLedger     []adminItemLedgerEntry `json:"item_ledger"`
}

type adminWalletInspection struct {
	Balances []adminWalletBalance       `json:"balances"`
	Ledger   []adminCurrencyLedgerEntry `json:"ledger"`
}

type adminWalletBalance struct {
	Currency string `json:"currency_type"`
	Balance  int64  `json:"balance"`
}

type adminCurrencyLedgerEntry struct {
	LedgerID     string `json:"ledger_id"`
	Currency     string `json:"currency_type"`
	Amount       int64  `json:"amount"`
	Action       string `json:"action"`
	BalanceAfter int64  `json:"balance_after"`
	Reason       string `json:"reason"`
	CreatedAt    int64  `json:"created_at"`
}

type adminItemLedgerEntry struct {
	LedgerID     string `json:"ledger_id"`
	ItemID       string `json:"item_id"`
	Quantity     int64  `json:"quantity"`
	Action       string `json:"action"`
	BalanceAfter int64  `json:"balance_after"`
	Location     string `json:"location"`
	Reason       string `json:"reason"`
	CreatedAt    int64  `json:"created_at"`
}

type adminRepairCraftJobPayload struct {
	Accepted        bool   `json:"accepted"`
	JobID           string `json:"job_id,omitempty"`
	Status          string `json:"status"`
	AlreadyComplete bool   `json:"already_complete,omitempty"`
	Message         string `json:"message,omitempty"`
}

type commandLogSummaryPayload struct {
	Entries     []commandLogEntryPayload `json:"entries"`
	Total       int                      `json:"total"`
	GeneratedAt int64                    `json:"generated_at"`
}

type commandLogEntryPayload struct {
	RequestID  string `json:"request_id"`
	Operation  string `json:"operation"`
	Status     string `json:"status"`
	ErrorCode  string `json:"error_code,omitempty"`
	DurationMS int64  `json:"duration_ms"`
	Timestamp  int64  `json:"timestamp"`
}

type metricsSummaryPayload struct {
	Snapshot    observability.MetricSnapshot `json:"snapshot"`
	GeneratedAt int64                        `json:"generated_at"`
}

type releaseGatePayload struct {
	Report      observability.ReleaseGateCoverageReport `json:"report"`
	Coverage    []releaseGateModuleCoveragePayload      `json:"coverage"`
	Evidence    int                                     `json:"evidence"`
	GeneratedAt int64                                   `json:"generated_at"`
}

type releaseGateModuleCoveragePayload struct {
	Module   string   `json:"module"`
	Passed   bool     `json:"passed"`
	Missing  []string `json:"missing,omitempty"`
	Evidence int      `json:"evidence"`
}

type abuseCoveragePayload struct {
	Report      observability.AbuseTestCoverageReport `json:"report"`
	Coverage    []observability.AbuseTestCoverage     `json:"coverage"`
	GeneratedAt int64                                 `json:"generated_at"`
}

type questRewardInventoryAdapter struct {
	inventory   *economy.InventoryService
	itemCatalog map[foundation.ItemID]economy.ItemDefinition
}

func (runtime *Runtime) handleQuestBoard(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	board, generated, err := runtime.ensureQuestBoardLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	if generated {
		runtime.queueEventLocked(authSessionID(ctx.SessionID), realtime.EventQuestBoardGenerated, board)
	}
	return marshalPayload(map[string]any{"quest_board": board})
}

func (runtime *Runtime) handleQuestAccept(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		OfferID string `json:"offer_id"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	offerID, err := foundation.ParseQuestID(payload.OfferID)
	if err != nil {
		return nil, invalidPayload("Quest offer is invalid.", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	player, err := runtime.questBoardSnapshotLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	quest, err := runtime.Quest.AcceptQuest(quests.AcceptQuestInput{
		Player:  player,
		OfferID: offerID,
	})
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	questPayload := runtime.questPayloadFromQuest(quest)
	questPayload.AcceptedOfferID = offerID.String()
	runtime.queueEventLocked(authSessionID(ctx.SessionID), realtime.EventQuestAccepted, questPayload)
	board, _, err := runtime.ensureQuestBoardLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	return marshalPayload(questMutationPayload{
		Accepted:   true,
		Quest:      &questPayload,
		QuestBoard: board,
		Wallet:     runtime.walletSnapshotLocked(ctx.PlayerID),
	})
}

func (runtime *Runtime) handleQuestProgress(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	board, _, err := runtime.ensureQuestBoardLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	return marshalPayload(map[string]any{"quest_board": board})
}

func (runtime *Runtime) handleQuestClaimReward(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct {
		QuestID string `json:"quest_id"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	questID, err := foundation.ParseQuestID(payload.QuestID)
	if err != nil {
		return nil, invalidPayload("Quest is invalid.", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	result, err := runtime.Quest.ClaimReward(quests.ClaimRewardInput{
		PlayerID:      ctx.PlayerID,
		PlayerQuestID: questID,
	})
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	runtime.recordQuestRewardMetrics(result)
	questPayload := runtime.questPayloadFromQuest(result.Quest)
	wallet := runtime.walletSnapshotLocked(ctx.PlayerID)
	state := runtime.players[ctx.PlayerID]
	state.Wallet = wallet
	state.Cargo = runtime.cargoSnapshotFromInventoryLocked(ctx.PlayerID)
	runtime.players[ctx.PlayerID] = state

	progressionSnapshot, err := runtime.Progression.GetProgressionSnapshot(ctx.PlayerID)
	if err != nil {
		return nil, err
	}
	progressionPayload := progressionPayload(progressionSnapshot)
	sessionID := authSessionID(ctx.SessionID)
	runtime.queueEventLocked(sessionID, realtime.EventQuestRewardClaimed, questPayload)
	runtime.queueEventLocked(sessionID, realtime.EventWalletSnapshot, wallet)
	runtime.queueEventLocked(sessionID, realtime.EventInventorySnapshot, runtime.inventorySnapshotLocked(ctx.PlayerID))
	runtime.queueEventLocked(sessionID, realtime.EventProgressionSnapshot, progressionPayload)

	board, _, err := runtime.ensureQuestBoardLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	inventory := runtime.inventorySnapshotLocked(ctx.PlayerID)
	return marshalPayload(questMutationPayload{
		Accepted:    true,
		Duplicate:   result.Duplicate,
		Quest:       &questPayload,
		QuestBoard:  board,
		Wallet:      wallet,
		Inventory:   &inventory,
		Progression: &progressionPayload,
	})
}

func (runtime *Runtime) handleQuestReroll(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	player, err := runtime.questBoardSnapshotLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	result, err := runtime.Quest.RerollBoard(quests.RerollBoardInput{
		Player:      player,
		Seed:        runtime.clock.Now().UTC().UnixNano(),
		ReferenceID: request.RequestID.String(),
		CreatedAt:   runtime.clock.Now().UTC(),
	})
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	wallet := runtime.walletSnapshotLocked(ctx.PlayerID)
	state := runtime.players[ctx.PlayerID]
	state.Wallet = wallet
	runtime.players[ctx.PlayerID] = state

	board, _, err := runtime.ensureQuestBoardLocked(ctx.PlayerID)
	if err != nil {
		return nil, domainErrorForQuest(err)
	}
	runtime.queueEventLocked(authSessionID(ctx.SessionID), realtime.EventQuestBoardRerolled, map[string]any{
		"quest_board": board,
		"cost":        questRerollCost{Currency: result.Cost.Currency.String(), Amount: result.Cost.Amount},
	})
	runtime.queueEventLocked(authSessionID(ctx.SessionID), realtime.EventWalletSnapshot, wallet)
	return marshalPayload(questMutationPayload{
		Accepted:   true,
		Duplicate:  result.Duplicate,
		QuestBoard: board,
		Wallet:     wallet,
	})
}

func (runtime *Runtime) handleAdminInspectPlayer(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayloadAllowing(request.Payload, "target_player_id"); err != nil {
		return nil, err
	}
	resolved, err := runtime.requireAdmin(ctx, "Admin player inspection is restricted.")
	if err != nil {
		return nil, err
	}
	var payload struct {
		TargetPlayerID string `json:"target_player_id,omitempty"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	targetID := resolved.PlayerID
	targetLabel := "self"
	if strings.TrimSpace(payload.TargetPlayerID) != "" {
		targetID, err = foundation.ParsePlayerID(payload.TargetPlayerID)
		if err != nil {
			return nil, invalidPayload("Target player is invalid.", err)
		}
		targetLabel = "requested"
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	inspection, err := runtime.adminPlayerInspectionPayload(targetID, targetLabel)
	if err != nil {
		return nil, domainErrorForAdmin(err)
	}
	return marshalPayload(map[string]any{"admin": inspection})
}

func (runtime *Runtime) handleAdminRepairCraftJob(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if _, err := runtime.requireAdmin(ctx, "Admin craft repair is restricted."); err != nil {
		return nil, err
	}
	var payload struct {
		JobID string `json:"job_id"`
	}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	jobID := crafting.CraftJobID(payload.JobID)
	if err := jobID.Validate(); err != nil {
		return nil, invalidPayload("Craft job is invalid.", err)
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	result, err := runtime.Admin.RepairStuckCraftJob(admin.RepairCraftJobInput{JobID: jobID})
	if err != nil {
		if errors.Is(err, admin.ErrMissingCraftingService) {
			response := adminRepairCraftJobPayload{
				Accepted: false,
				JobID:    jobID.String(),
				Status:   "unavailable",
				Message:  "Craft job repair is not wired for this runtime.",
			}
			runtime.queueEventLocked(authSessionID(ctx.SessionID), realtime.EventAdminActionCompleted, response)
			return marshalPayload(map[string]any{"admin_repair": response})
		}
		return nil, domainErrorForAdmin(err)
	}
	response := adminRepairCraftJobPayload{
		Accepted:        true,
		JobID:           result.Job.JobID.String(),
		Status:          result.Job.State.String(),
		AlreadyComplete: result.AlreadyComplete,
	}
	runtime.queueEventLocked(authSessionID(ctx.SessionID), realtime.EventAdminActionCompleted, response)
	return marshalPayload(map[string]any{"admin_repair": response})
}

func (runtime *Runtime) handleObservabilityCommandLog(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if _, err := runtime.requireAdmin(ctx, "Command log is restricted."); err != nil {
		return nil, err
	}
	entries := runtime.CommandLog.Snapshot()
	payload := commandLogSummaryPayload{
		Entries:     commandLogEntriesPayload(entries),
		Total:       len(entries),
		GeneratedAt: runtime.clock.Now().UTC().UnixMilli(),
	}
	return marshalPayload(map[string]any{"command_log": payload})
}

func (runtime *Runtime) handleObservabilityMetrics(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if _, err := runtime.requireAdmin(ctx, "Metrics are restricted."); err != nil {
		return nil, err
	}
	payload := metricsSummaryPayload{
		Snapshot:    runtime.Metrics.Snapshot(),
		GeneratedAt: runtime.clock.Now().UTC().UnixMilli(),
	}
	runtime.mu.Lock()
	runtime.queueEventLocked(authSessionID(ctx.SessionID), realtime.EventObservabilityMetric, map[string]any{
		"generated_at": payload.GeneratedAt,
	})
	runtime.mu.Unlock()
	return marshalPayload(map[string]any{"metrics": payload})
}

func (runtime *Runtime) handleObservabilityReleaseGate(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if _, err := runtime.requireAdmin(ctx, "Release gate is restricted."); err != nil {
		return nil, err
	}
	coverage := observability.Phase12ReleaseGateCoverage()
	summary, evidenceCount := releaseGateCoveragePayload(coverage)
	payload := releaseGatePayload{
		Report:      observability.NewReleaseGateCoverageReport(coverage),
		Coverage:    summary,
		Evidence:    evidenceCount,
		GeneratedAt: runtime.clock.Now().UTC().UnixMilli(),
	}
	runtime.mu.Lock()
	runtime.queueEventLocked(authSessionID(ctx.SessionID), realtime.EventReleaseGateUpdated, map[string]any{
		"passed":       payload.Report.Passed,
		"generated_at": payload.GeneratedAt,
	})
	runtime.mu.Unlock()
	return marshalPayload(map[string]any{"release_gate": payload})
}

func (runtime *Runtime) handleObservabilityAbuseCoverage(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	if _, err := runtime.requireAdmin(ctx, "Abuse coverage is restricted."); err != nil {
		return nil, err
	}
	coverage := observability.Phase12AbuseTestCoverage()
	payload := abuseCoveragePayload{
		Report:      observability.NewAbuseTestCoverageReport(coverage),
		Coverage:    coverage,
		GeneratedAt: runtime.clock.Now().UTC().UnixMilli(),
	}
	return marshalPayload(map[string]any{"abuse_coverage": payload})
}

func (runtime *Runtime) requireAdmin(ctx realtime.CommandContext, message string) (auth.ResolvedSession, error) {
	resolved, err := runtime.Auth.ResolveSessionID(context.Background(), authSessionID(ctx.SessionID))
	if err != nil {
		return auth.ResolvedSession{}, err
	}
	if !hasRole(resolved.Roles, auth.RoleAdmin) {
		return auth.ResolvedSession{}, foundation.NewDomainError(foundation.CodeForbidden, message)
	}
	return resolved, nil
}

func (runtime *Runtime) ensureQuestBoardLocked(playerID foundation.PlayerID) (questBoardSummaryPayload, bool, error) {
	offers, err := runtime.Quest.BoardOffers(playerID)
	if err != nil {
		return questBoardSummaryPayload{}, false, err
	}
	generated := false
	if len(offers) == 0 {
		player, err := runtime.questBoardSnapshotLocked(playerID)
		if err != nil {
			return questBoardSummaryPayload{}, false, err
		}
		offers, err = runtime.Quest.GenerateAndStoreBoard(quests.BoardGenerationInput{
			Player:    player,
			Seed:      runtime.questBoardSeedLocked(player),
			CreatedAt: runtime.clock.Now().UTC(),
		})
		if err != nil {
			return questBoardSummaryPayload{}, false, err
		}
		generated = true
	}
	questsForPlayer, err := runtime.Quest.PlayerQuests(playerID)
	if err != nil {
		return questBoardSummaryPayload{}, false, err
	}
	board, err := runtime.questBoardSummaryPayload(playerID, offers, questsForPlayer)
	return board, generated, err
}

func (runtime *Runtime) questBoardSnapshotLocked(playerID foundation.PlayerID) (quests.PlayerQuestBoardSnapshot, error) {
	progressionSnapshot, err := runtime.Progression.GetProgressionSnapshot(playerID)
	if err != nil {
		return quests.PlayerQuestBoardSnapshot{}, err
	}
	roleLevels := make(map[progression.RoleType]int)
	for _, role := range progressionSnapshot.RoleLevels() {
		roleLevels[role.Role] = role.Level
	}
	knownPlanets, err := runtime.knownPlanetsPayload(playerID)
	if err != nil {
		return quests.PlayerQuestBoardSnapshot{}, err
	}
	sector := runtime.sectorPayloadLocked()
	return quests.PlayerQuestBoardSnapshot{
		PlayerID:         playerID,
		Rank:             progressionSnapshot.Player.Rank,
		MainLevel:        progressionSnapshot.Player.MainLevel,
		RoleLevels:       roleLevels,
		CurrentRegion:    sector.Region,
		KnownPlanetCount: knownPlanets.Counts.Known,
		OwnedPlanetCount: knownPlanets.Counts.Owned,
	}, nil
}

func (runtime *Runtime) questBoardSeedLocked(player quests.PlayerQuestBoardSnapshot) int64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(player.PlayerID.String()))
	_, _ = hash.Write([]byte(fmt.Sprintf(":%d:%d:%s", player.Rank, player.MainLevel, runtime.clock.Now().UTC().Format("2006-01-02"))))
	return int64(hash.Sum64() & 0x7fffffffffffffff)
}

func (runtime *Runtime) questBoardSummaryPayload(playerID foundation.PlayerID, offers []quests.GeneratedBoardOffer, playerQuests []quests.PlayerQuest) (questBoardSummaryPayload, error) {
	activePayloads := make([]questPayload, 0, len(playerQuests))
	counts := questCountsPayload{}
	activeLimitCount := 0
	for _, quest := range playerQuests {
		payload := runtime.questPayloadFromQuest(quest)
		activePayloads = append(activePayloads, payload)
		if questCountsAgainstActiveLimitForPayload(quest) {
			activeLimitCount++
		}
		switch quest.State {
		case quests.QuestStateAccepted:
			counts.Active++
		case quests.QuestStateCompleted:
			counts.Completed++
			if quest.ClaimedAt == nil && quest.RewardClaimedAt == nil {
				counts.Claimable++
			}
		case quests.QuestStateClaimed:
			counts.Claimed++
		}
	}
	sort.Slice(activePayloads, func(i, j int) bool { return activePayloads[i].AcceptedAt < activePayloads[j].AcceptedAt })

	player, err := runtime.questBoardSnapshotLocked(playerID)
	if err != nil {
		return questBoardSummaryPayload{}, err
	}
	cost, err := quests.DefaultQuestRerollCostHook(player)
	if err != nil {
		return questBoardSummaryPayload{}, err
	}
	now := runtime.clock.Now().UTC()
	offerPayloads := make([]questOfferPayload, 0, len(offers))
	resetAt := int64(0)
	for _, offer := range offers {
		offerPayload := runtime.questOfferPayloadFromOffer(offer, activeLimitCount, now)
		offerPayloads = append(offerPayloads, offerPayload)
		if offerPayload.ExpiresAt > 0 && (resetAt == 0 || offerPayload.ExpiresAt < resetAt) {
			resetAt = offerPayload.ExpiresAt
		}
	}
	sort.Slice(offerPayloads, func(i, j int) bool { return offerPayloads[i].OfferID < offerPayloads[j].OfferID })
	counts.Offers = len(offerPayloads)
	canReroll := runtime.Wallet.Balance(playerID, cost.Currency) >= cost.Amount
	lockedReason := ""
	if !canReroll {
		lockedReason = "Insufficient balance."
	}
	return questBoardSummaryPayload{
		Offers: offerPayloads,
		Active: activePayloads,
		Counts: counts,
		RerollCost: questRerollCost{
			Currency: cost.Currency.String(),
			Amount:   cost.Amount,
		},
		CanReroll:    canReroll,
		LockedReason: lockedReason,
		ResetAt:      resetAt,
		GeneratedAt:  now.UnixMilli(),
		Revision:     questBoardRevisionMillis(now, offers, playerQuests),
	}, nil
}

func questBoardRevisionMillis(generatedAt time.Time, offers []quests.GeneratedBoardOffer, playerQuests []quests.PlayerQuest) int64 {
	revision := generatedAt.UnixMilli()
	for _, offer := range offers {
		revision = max(revision, offer.CreatedAt.UTC().UnixMilli())
	}
	for _, quest := range playerQuests {
		revision = max(revision, quest.AcceptedAt.UTC().UnixMilli())
		if quest.CompletedAt != nil {
			revision = max(revision, quest.CompletedAt.UTC().UnixMilli())
		}
		if quest.ClaimedAt != nil {
			revision = max(revision, quest.ClaimedAt.UTC().UnixMilli())
		}
		if quest.RewardClaimedAt != nil {
			revision = max(revision, quest.RewardClaimedAt.UTC().UnixMilli())
		}
	}
	return revision
}

func (runtime *Runtime) questOfferPayloadFromOffer(offer quests.GeneratedBoardOffer, activeLimitCount int, now time.Time) questOfferPayload {
	canAccept := activeLimitCount < quests.MaxActivePlayerQuests && offer.ExpiresAt.After(now)
	lockedReason := ""
	if !offer.ExpiresAt.After(now) {
		lockedReason = "Offer expired."
	} else if activeLimitCount >= quests.MaxActivePlayerQuests {
		lockedReason = "Quest slots full."
	}
	return questOfferPayload{
		OfferID:      offer.OfferID.String(),
		Type:         offer.Type.String(),
		Title:        questTitle(offer.TemplateID.String()),
		Description:  questDescription(offer.Type),
		Objectives:   runtime.questObjectivesFromSchema(offer.GeneratedPayload.Objective, nil),
		Rewards:      runtime.questRewardsFromPayload(offer.RewardPayload),
		ExpiresAt:    offer.ExpiresAt.UTC().UnixMilli(),
		CanAccept:    canAccept,
		LockedReason: lockedReason,
	}
}

func questCountsAgainstActiveLimitForPayload(quest quests.PlayerQuest) bool {
	switch quest.State {
	case quests.QuestStateAccepted:
		return true
	case quests.QuestStateCompleted:
		return quest.ClaimedAt == nil && quest.RewardClaimedAt == nil
	default:
		return false
	}
}

func (runtime *Runtime) questPayloadFromQuest(quest quests.PlayerQuest) questPayload {
	payload := questPayload{
		QuestID:     quest.PlayerQuestID.String(),
		Type:        quest.Type.String(),
		Title:       questTitle(quest.TemplateID.String()),
		Description: questDescription(quest.Type),
		State:       quest.State.String(),
		Objectives:  runtime.questObjectivesFromSchema(quest.GeneratedPayload.Objective, quest.Progress.Objectives),
		Rewards:     runtime.questRewardsFromPayload(quest.RewardPayload),
		AcceptedAt:  quest.AcceptedAt.UTC().UnixMilli(),
		CanClaim:    quest.State == quests.QuestStateCompleted && quest.ClaimedAt == nil && quest.RewardClaimedAt == nil,
	}
	if quest.CompletedAt != nil {
		payload.CompletedAt = quest.CompletedAt.UTC().UnixMilli()
	}
	if quest.ClaimedAt != nil {
		payload.ClaimedAt = quest.ClaimedAt.UTC().UnixMilli()
	} else if quest.RewardClaimedAt != nil {
		payload.ClaimedAt = quest.RewardClaimedAt.UTC().UnixMilli()
	}
	return payload
}

func (runtime *Runtime) questObjectivesFromSchema(schema quests.ObjectiveSchema, progress []quests.ObjectiveProgress) []questObjectivePayload {
	progressByID := make(map[string]quests.ObjectiveProgress, len(progress))
	for _, item := range progress {
		progressByID[item.ObjectiveID] = item
	}
	objectives := schema.Objectives
	payload := make([]questObjectivePayload, 0, len(objectives))
	for _, objective := range objectives {
		item := questObjectivePayload{
			ID:          objective.ID,
			Kind:        objective.Kind.String(),
			DisplayName: questObjectiveKindLabel(objective.Kind),
		}
		switch objective.Kind {
		case quests.ObjectiveKindKill:
			item.Target = objective.Kill.TargetNPCType
			item.Required = objective.Kill.RequiredCount.Int64()
			item.DisplayName = "Destroy " + questPublicTargetName(item.Target)
			item.ArtKey = "quest.objective.kill"
		case quests.ObjectiveKindCollect:
			item.Target = objective.Collect.ItemID.String()
			item.Required = objective.Collect.Quantity.Int64()
			item.DisplayName = runtime.questItemDisplayName(objective.Collect.ItemID)
			item.CatalogRef = "item:" + item.Target
			item.ArtKey = "item." + item.Target
		case quests.ObjectiveKindCraft:
			item.Target = objective.Craft.RecipeID.String()
			if item.Target == "" {
				item.Target = objective.Craft.ItemID.String()
			}
			item.Required = objective.Craft.Quantity.Int64()
			item.DisplayName = "Fabricate " + questPublicTargetName(item.Target)
			item.CatalogRef = "recipe:" + item.Target
			item.ArtKey = "quest.objective.craft"
		case quests.ObjectiveKindScan:
			item.Target = objective.Scan.TargetSignalType
			item.Required = objective.Scan.RequiredCount.Int64()
			item.DisplayName = "Scan " + questPublicTargetName(item.Target)
			item.ArtKey = "quest.objective.scan"
		case quests.ObjectiveKindBuild:
			item.Target = objective.Build.BuildingType
			item.Required = objective.Build.RequiredCount.Int64()
			item.DisplayName = "Build " + questPublicTargetName(item.Target)
			item.CatalogRef = "building:" + item.Target
			item.ArtKey = "quest.objective.build"
		case quests.ObjectiveKindDeliver:
			item.Target = objective.Deliver.ItemID.String()
			item.Required = objective.Deliver.Quantity.Int64()
			item.DisplayName = "Deliver " + runtime.questItemDisplayName(objective.Deliver.ItemID)
			item.CatalogRef = "item:" + item.Target
			item.ArtKey = "item." + item.Target
		}
		if current, ok := progressByID[objective.ID]; ok {
			item.Current = current.Current
			item.Required = current.Required
			item.Completed = current.Completed
		}
		payload = append(payload, item)
	}
	return payload
}

func (runtime *Runtime) questRewardsFromPayload(payload quests.RewardPayload) []questRewardPayload {
	rewards := make([]questRewardPayload, 0, len(payload.Grants))
	for _, grant := range payload.Grants {
		reward := questRewardPayload{
			Kind:     grant.Kind.String(),
			Currency: grant.Currency.String(),
			ItemID:   grant.ItemID.String(),
			Role:     grant.Role.String(),
			Amount:   grant.Amount,
		}
		reward.DisplayName, reward.CatalogRef, reward.ArtKey = runtime.questRewardDisplayMetadata(reward)
		rewards = append(rewards, questRewardPayload{
			Kind:        reward.Kind,
			Currency:    reward.Currency,
			ItemID:      reward.ItemID,
			Role:        reward.Role,
			DisplayName: reward.DisplayName,
			CatalogRef:  reward.CatalogRef,
			ArtKey:      reward.ArtKey,
			Amount:      reward.Amount,
		})
	}
	return rewards
}

func (runtime *Runtime) questRewardDisplayMetadata(reward questRewardPayload) (string, string, string) {
	if reward.Currency != "" {
		return questCurrencyDisplayName(reward.Currency), "currency:" + reward.Currency, "currency." + reward.Currency
	}
	if reward.ItemID != "" {
		return runtime.questItemDisplayName(foundation.ItemID(reward.ItemID)), "item:" + reward.ItemID, "item." + reward.ItemID
	}
	if reward.Role != "" {
		return questRoleDisplayName(reward.Role) + " XP", "role:" + reward.Role, "role." + reward.Role
	}
	return questPublicTargetName(reward.Kind), "", "quest.reward"
}

func (runtime *Runtime) questItemDisplayName(itemID foundation.ItemID) string {
	if name := runtime.itemDisplayName(itemID); name != "" && name != itemID.String() {
		return name
	}
	return questPublicTargetName(itemID.String())
}

func questObjectiveKindLabel(kind quests.ObjectiveKind) string {
	switch kind {
	case quests.ObjectiveKindKill:
		return "Destroy target"
	case quests.ObjectiveKindCollect:
		return "Recover cargo"
	case quests.ObjectiveKindCraft:
		return "Fabricate item"
	case quests.ObjectiveKindScan:
		return "Scan signal"
	case quests.ObjectiveKindBuild:
		return "Build structure"
	case quests.ObjectiveKindDeliver:
		return "Deliver cargo"
	default:
		return "Complete objective"
	}
}

func questCurrencyDisplayName(currency string) string {
	switch currency {
	case "credits":
		return "Credits"
	case "premium_paid":
		return "Premium Credits"
	case "premium_earned":
		return "Bonus Premium"
	default:
		return questPublicTargetName(currency)
	}
}

func questRoleDisplayName(role string) string {
	switch role {
	case progression.RoleTypeCombat.String():
		return "Combat"
	case progression.RoleTypeScout.String():
		return "Scout"
	case progression.RoleTypeCrafting.String():
		return "Crafting"
	case progression.RoleTypeConstruction.String():
		return "Construction"
	default:
		return questPublicTargetName(role)
	}
}

func questPublicTargetName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Target"
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func (runtime *Runtime) queueQuestProgressEventsLocked(sessionID auth.SessionID, updated []quests.PlayerQuest) {
	for _, quest := range updated {
		payload := runtime.questPayloadFromQuest(quest)
		runtime.queueEventLocked(sessionID, realtime.EventQuestProgressed, payload)
		if quest.State == quests.QuestStateCompleted {
			runtime.queueEventLocked(sessionID, realtime.EventQuestCompleted, payload)
		}
	}
}

func (runtime *Runtime) adminPlayerInspectionPayload(playerID foundation.PlayerID, targetLabel string) (adminPlayerInspectionPayload, error) {
	inventoryReport, err := runtime.Admin.InspectPlayerInventory(playerID)
	if err != nil {
		return adminPlayerInspectionPayload{}, err
	}
	walletReport, err := runtime.Admin.InspectPlayerWalletLedger(playerID)
	if err != nil {
		return adminPlayerInspectionPayload{}, err
	}
	itemLedgerReport, err := runtime.Admin.InspectPlayerItemLedger(playerID)
	if err != nil {
		return adminPlayerInspectionPayload{}, err
	}

	return adminPlayerInspectionPayload{
		Target: targetLabel,
		Inventory: adminInventoryInspection{
			StackableItems: len(inventoryReport.StackableItems),
			InstanceItems:  len(inventoryReport.InstanceItems),
			ItemLedger:     adminItemLedgerPayload(itemLedgerReport.LedgerEntries),
		},
		Wallet: adminWalletInspection{
			Balances: adminWalletBalancesPayload(walletReport.Balances),
			Ledger:   adminCurrencyLedgerPayload(walletReport.LedgerEntries),
		},
		GeneratedAt: runtime.clock.Now().UTC().UnixMilli(),
	}, nil
}

func adminWalletBalancesPayload(balances []economy.WalletBalance) []adminWalletBalance {
	payload := make([]adminWalletBalance, 0, len(balances))
	for _, balance := range balances {
		payload = append(payload, adminWalletBalance{
			Currency: balance.Currency.String(),
			Balance:  balance.Balance,
		})
	}
	return payload
}

func adminCurrencyLedgerPayload(entries []economy.CurrencyLedgerEntry) []adminCurrencyLedgerEntry {
	payload := make([]adminCurrencyLedgerEntry, 0, len(entries))
	for _, entry := range entries {
		payload = append(payload, adminCurrencyLedgerEntry{
			LedgerID:     entry.LedgerID.String(),
			Currency:     entry.Currency.String(),
			Amount:       entry.Amount.Int64(),
			Action:       entry.Action.String(),
			BalanceAfter: entry.BalanceAfter,
			Reason:       entry.Reason.String(),
			CreatedAt:    entry.CreatedAt.UTC().UnixMilli(),
		})
	}
	return payload
}

func adminItemLedgerPayload(entries []economy.ItemLedgerEntry) []adminItemLedgerEntry {
	payload := make([]adminItemLedgerEntry, 0, len(entries))
	for _, entry := range entries {
		payload = append(payload, adminItemLedgerEntry{
			LedgerID:     entry.LedgerID.String(),
			ItemID:       entry.ItemID.String(),
			Quantity:     entry.Quantity.Int64(),
			Action:       entry.Action.String(),
			BalanceAfter: entry.BalanceAfter,
			Location:     entry.Location.Kind.String(),
			Reason:       entry.Reason.String(),
			CreatedAt:    entry.CreatedAt.UTC().UnixMilli(),
		})
	}
	return payload
}

func commandLogEntriesPayload(entries []observability.CommandLogEntry) []commandLogEntryPayload {
	payload := make([]commandLogEntryPayload, 0, len(entries))
	for _, entry := range entries {
		payload = append(payload, commandLogEntryPayload{
			RequestID:  entry.RequestID.String(),
			Operation:  entry.Operation.String(),
			Status:     entry.Status.String(),
			ErrorCode:  entry.ErrorCode.String(),
			DurationMS: entry.Duration.Milliseconds(),
			Timestamp:  entry.Timestamp.UTC().UnixMilli(),
		})
	}
	return payload
}

func releaseGateCoveragePayload(coverage []observability.ReleaseGateCoverage) ([]releaseGateModuleCoveragePayload, int) {
	byModule := make(map[string]*releaseGateModuleCoveragePayload)
	order := make([]string, 0)
	evidenceCount := 0
	for _, item := range coverage {
		current, ok := byModule[item.Module]
		if !ok {
			current = &releaseGateModuleCoveragePayload{Module: item.Module, Passed: true}
			byModule[item.Module] = current
			order = append(order, item.Module)
		}
		evidenceCount += len(item.Evidence)
		current.Evidence += len(item.Evidence)
		if item.Status != observability.GateStatusSatisfied && item.Status != observability.GateStatusNotApplicable {
			current.Passed = false
			current.Missing = append(current.Missing, string(item.Check))
		}
	}
	sort.Strings(order)
	payload := make([]releaseGateModuleCoveragePayload, 0, len(order))
	for _, module := range order {
		item := *byModule[module]
		sort.Strings(item.Missing)
		payload = append(payload, item)
	}
	return payload, evidenceCount
}

func (adapter questRewardInventoryAdapter) GrantQuestRewardItems(input quests.QuestRewardItemGrantInput) (quests.QuestRewardItemGrantResult, error) {
	if adapter.inventory == nil {
		return quests.QuestRewardItemGrantResult{}, quests.ErrMissingQuestRewardInventoryService
	}
	location, err := economy.NewItemLocation(economy.LocationKindAccountInventory, input.PlayerID.String())
	if err != nil {
		return quests.QuestRewardItemGrantResult{}, err
	}
	granted := make([]quests.QuestRewardItemGrant, 0, len(input.Items))
	duplicate := len(input.Items) > 0
	for _, item := range input.Items {
		definition, ok := adapter.itemCatalog[item.ItemID]
		if !ok {
			return quests.QuestRewardItemGrantResult{}, foundation.NewDomainError(foundation.CodeNotFound, "Quest reward item was not found.")
		}
		referenceKey := input.ReferenceKey
		if len(input.Items) > 1 {
			referenceKey = foundation.IdempotencyKey(input.ReferenceKey.String() + ":" + item.ItemID.String())
		}
		result, err := adapter.inventory.AddItem(economy.AddItemInput{
			PlayerID:       input.PlayerID,
			ItemDefinition: definition,
			Quantity:       item.Quantity,
			Location:       location,
			Reason:         input.Reason,
			ReferenceKey:   referenceKey,
		})
		if err != nil {
			return quests.QuestRewardItemGrantResult{}, err
		}
		if !result.Duplicate {
			duplicate = false
		}
		granted = append(granted, item)
	}
	return quests.QuestRewardItemGrantResult{
		Items:        granted,
		ReferenceKey: input.ReferenceKey,
		Duplicate:    duplicate,
	}, nil
}

func questTitle(templateID string) string {
	title := strings.TrimPrefix(templateID, "quest_")
	title = strings.TrimSuffix(title, "_r1")
	title = strings.TrimSuffix(title, "_r2")
	title = strings.TrimSuffix(title, "_r3")
	parts := strings.Split(title, "_")
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + part[1:]
	}
	return strings.Join(parts, " ")
}

func questDescription(questType quests.QuestType) string {
	switch questType {
	case quests.QuestTypeKill:
		return "Destroy confirmed hostile targets."
	case quests.QuestTypeCollect:
		return "Recover field loot and return it to your hold."
	case quests.QuestTypeCraft:
		return "Complete a workshop fabrication job."
	case quests.QuestTypeScan:
		return "Resolve scanner intel from the discovery service."
	case quests.QuestTypeBuild:
		return "Finish a planet building through production ownership."
	case quests.QuestTypeDeliver:
		return "Settle a delivery through route or station ownership."
	default:
		return "Complete the assigned objective."
	}
}

func domainErrorForQuest(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, quests.ErrQuestOfferNotFound), errors.Is(err, quests.ErrPlayerQuestNotFound):
		return foundation.NewDomainError(foundation.CodeNotFound, "Quest was not found.", foundation.WithCause(err))
	case errors.Is(err, quests.ErrQuestOfferOwnerMismatch), errors.Is(err, quests.ErrPlayerQuestOwnerMismatch):
		return foundation.NewDomainError(foundation.CodeForbidden, "Quest is not owned by this player.", foundation.WithCause(err))
	case errors.Is(err, quests.ErrTooManyActiveQuests), errors.Is(err, quests.ErrQuestRequirementsNotMet), errors.Is(err, quests.ErrInvalidQuestClaim), errors.Is(err, quests.ErrInvalidQuestState):
		return foundation.NewDomainError(foundation.CodeForbidden, "Quest action is not allowed.", foundation.WithCause(err))
	case errors.Is(err, economy.ErrInsufficientWalletFunds):
		return foundation.NewDomainError(foundation.CodeNotEnoughFunds, "Not enough funds.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Quest command failed.", foundation.WithCause(err))
	}
}

func domainErrorForAdmin(err error) error {
	if err == nil {
		return nil
	}
	var domainErr *foundation.DomainError
	if errors.As(err, &domainErr) {
		return domainErr
	}
	switch {
	case errors.Is(err, crafting.ErrCraftJobNotFound):
		return foundation.NewDomainError(foundation.CodeNotFound, "Craft job was not found.", foundation.WithCause(err))
	case errors.Is(err, admin.ErrCraftJobNotReady):
		return foundation.NewDomainError(foundation.CodeForbidden, "Craft job is not ready.", foundation.WithCause(err))
	default:
		return foundation.NewDomainError(foundation.CodeInternal, "Admin command failed.", foundation.WithCause(err))
	}
}

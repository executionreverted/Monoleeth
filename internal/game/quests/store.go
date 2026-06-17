package quests

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// InMemoryQuestStore is a mutex-protected Phase 07 store for generated board
// offers and accepted player quest state.
type InMemoryQuestStore struct {
	mu sync.Mutex

	offers          map[questOfferStoreKey]GeneratedBoardOffer
	acceptedByOffer map[questOfferStoreKey]foundation.QuestID
	quests          map[foundation.QuestID]PlayerQuest
	questsByPlayer  map[foundation.PlayerID][]foundation.QuestID
	progressEvents  map[foundation.EventID]struct{}
	claimResults    map[foundation.QuestID]ClaimRewardResult
}

type questOfferStoreKey struct {
	playerID foundation.PlayerID
	offerID  foundation.QuestID
}

// NewInMemoryQuestStore returns an empty in-memory quest store.
func NewInMemoryQuestStore() *InMemoryQuestStore {
	return &InMemoryQuestStore{
		offers:          make(map[questOfferStoreKey]GeneratedBoardOffer),
		acceptedByOffer: make(map[questOfferStoreKey]foundation.QuestID),
		quests:          make(map[foundation.QuestID]PlayerQuest),
		questsByPlayer:  make(map[foundation.PlayerID][]foundation.QuestID),
		progressEvents:  make(map[foundation.EventID]struct{}),
		claimResults:    make(map[foundation.QuestID]ClaimRewardResult),
	}
}

// StoreGeneratedBoardOffers stores server-generated, unaccepted board offers.
// Existing accepted offers are preserved so regeneration cannot make an
// accepted offer available again.
func (store *InMemoryQuestStore) StoreGeneratedBoardOffers(offers []GeneratedBoardOffer) error {
	cloned := make([]GeneratedBoardOffer, 0, len(offers))
	seen := make(map[questOfferStoreKey]struct{}, len(offers))
	for _, offer := range offers {
		if err := offer.Validate(); err != nil {
			return err
		}
		if offer.AcceptedAt != nil {
			return fmt.Errorf("offer %q: %w", offer.OfferID, ErrQuestOfferAlreadyAccepted)
		}
		key := newQuestOfferStoreKey(offer.PlayerID, offer.OfferID)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("offer %q player %q: %w", offer.OfferID, offer.PlayerID, ErrDuplicateQuestOffer)
		}
		seen[key] = struct{}{}
		cloned = append(cloned, cloneGeneratedBoardOffer(offer))
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	for _, offer := range cloned {
		key := newQuestOfferStoreKey(offer.PlayerID, offer.OfferID)
		if _, accepted := store.acceptedByOffer[key]; accepted {
			continue
		}
		if existing, ok := store.offers[key]; ok && existing.AcceptedAt != nil {
			continue
		}
		store.offers[key] = cloneGeneratedBoardOffer(offer)
	}
	return nil
}

// BoardOffers returns unaccepted board offers currently available for playerID.
func (store *InMemoryQuestStore) BoardOffers(playerID foundation.PlayerID) ([]GeneratedBoardOffer, error) {
	if err := playerID.Validate(); err != nil {
		return nil, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	offers := make([]GeneratedBoardOffer, 0)
	for key, offer := range store.offers {
		if key.playerID != playerID || offer.AcceptedAt != nil {
			continue
		}
		offers = append(offers, cloneGeneratedBoardOffer(offer))
	}
	sort.Slice(offers, func(i, j int) bool {
		if !offers[i].CreatedAt.Equal(offers[j].CreatedAt) {
			return offers[i].CreatedAt.Before(offers[j].CreatedAt)
		}
		return offers[i].OfferID < offers[j].OfferID
	})
	return offers, nil
}

// PlayerQuests returns accepted and later-state quests for playerID.
func (store *InMemoryQuestStore) PlayerQuests(playerID foundation.PlayerID) ([]PlayerQuest, error) {
	if err := playerID.Validate(); err != nil {
		return nil, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	quests := make([]PlayerQuest, 0, len(store.questsByPlayer[playerID]))
	for _, questID := range store.questsByPlayer[playerID] {
		quest, ok := store.quests[questID]
		if !ok {
			continue
		}
		quests = append(quests, clonePlayerQuest(quest))
	}
	sort.Slice(quests, func(i, j int) bool {
		if !quests[i].AcceptedAt.Equal(quests[j].AcceptedAt) {
			return quests[i].AcceptedAt.Before(quests[j].AcceptedAt)
		}
		return quests[i].PlayerQuestID < quests[j].PlayerQuestID
	})
	return quests, nil
}

type objectiveProgressMatcher func(Objective) (int64, bool)

// ApplyProgressEvent applies one validated server event to matching accepted
// active quests for playerID under the store lock. Duplicate event ids are
// accepted as no-ops.
func (store *InMemoryQuestStore) ApplyProgressEvent(
	eventID foundation.EventID,
	playerID foundation.PlayerID,
	occurredAt time.Time,
	matcher objectiveProgressMatcher,
) ([]PlayerQuest, error) {
	if err := eventID.Validate(); err != nil {
		return nil, fmt.Errorf("event_id: %w", err)
	}
	if err := playerID.Validate(); err != nil {
		return nil, fmt.Errorf("player_id: %w", err)
	}
	if occurredAt.IsZero() {
		return nil, fmt.Errorf("occurred_at: %w", ErrZeroQuestTime)
	}
	if matcher == nil {
		return nil, fmt.Errorf("progress matcher: %w", ErrInvalidQuestEvent)
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if _, consumed := store.progressEvents[eventID]; consumed {
		return nil, nil
	}

	updated, err := store.applyProgressEventLocked(playerID, occurredAt.UTC(), matcher)
	if err != nil {
		return nil, err
	}
	store.progressEvents[eventID] = struct{}{}
	return updated, nil
}

func newQuestOfferStoreKey(playerID foundation.PlayerID, offerID foundation.QuestID) questOfferStoreKey {
	return questOfferStoreKey{playerID: playerID, offerID: offerID}
}

func (store *InMemoryQuestStore) offerOwnerLocked(offerID foundation.QuestID) (foundation.PlayerID, bool) {
	for key := range store.offers {
		if key.offerID == offerID {
			return key.playerID, true
		}
	}
	return "", false
}

func (store *InMemoryQuestStore) activeQuestCountLocked(playerID foundation.PlayerID) int {
	count := 0
	for _, questID := range store.questsByPlayer[playerID] {
		quest, ok := store.quests[questID]
		if !ok {
			continue
		}
		if questCountsAgainstActiveLimit(quest) {
			count++
		}
	}
	return count
}

func questCountsAgainstActiveLimit(quest PlayerQuest) bool {
	switch quest.State {
	case QuestStateAccepted:
		return true
	case QuestStateCompleted:
		return quest.ClaimedAt == nil && quest.RewardClaimedAt == nil
	default:
		return false
	}
}

func (store *InMemoryQuestStore) appendPlayerQuestLocked(playerID foundation.PlayerID, questID foundation.QuestID) {
	for _, existing := range store.questsByPlayer[playerID] {
		if existing == questID {
			return
		}
	}
	store.questsByPlayer[playerID] = append(store.questsByPlayer[playerID], questID)
}

func (store *InMemoryQuestStore) applyProgressEventLocked(
	playerID foundation.PlayerID,
	occurredAt time.Time,
	matcher objectiveProgressMatcher,
) ([]PlayerQuest, error) {
	pending := make([]PlayerQuest, 0)
	for _, questID := range store.questsByPlayer[playerID] {
		quest, ok := store.quests[questID]
		if !ok {
			continue
		}
		next, changed, err := progressQuestFromEvent(quest, occurredAt, matcher)
		if err != nil {
			return nil, err
		}
		if changed {
			pending = append(pending, next)
		}
	}

	updated := make([]PlayerQuest, 0, len(pending))
	for _, quest := range pending {
		store.quests[quest.PlayerQuestID] = clonePlayerQuest(quest)
		updated = append(updated, clonePlayerQuest(quest))
	}
	return updated, nil
}

func progressQuestFromEvent(
	quest PlayerQuest,
	occurredAt time.Time,
	matcher objectiveProgressMatcher,
) (PlayerQuest, bool, error) {
	if quest.State != QuestStateAccepted {
		return PlayerQuest{}, false, nil
	}
	if err := quest.Validate(); err != nil {
		return PlayerQuest{}, false, err
	}

	objectives, err := progressObjectiveList(quest.GeneratedPayload.Objective)
	if err != nil {
		return PlayerQuest{}, false, err
	}
	progressByID := make(map[string]int, len(quest.Progress.Objectives))
	for index, progress := range quest.Progress.Objectives {
		progressByID[progress.ObjectiveID] = index
	}

	next := clonePlayerQuest(quest)
	changed := false
	for _, objective := range objectives {
		delta, matched := matcher(objective)
		if !matched {
			continue
		}
		if delta <= 0 {
			return PlayerQuest{}, false, fmt.Errorf("progress delta %d: %w", delta, ErrInvalidQuestProgress)
		}
		index, ok := progressByID[objective.ID]
		if !ok {
			return PlayerQuest{}, false, fmt.Errorf("objective progress %q: %w", objective.ID, ErrUnexpectedQuestProgress)
		}
		objectiveProgress := next.Progress.Objectives[index]
		if objectiveProgress.Completed {
			continue
		}

		current := objectiveProgress.Current + delta
		if current >= objectiveProgress.Required {
			current = objectiveProgress.Required
			objectiveProgress.Completed = true
		}
		if current != objectiveProgress.Current {
			objectiveProgress.Current = current
			next.Progress.Objectives[index] = objectiveProgress
			changed = true
		}
	}
	if !changed {
		return PlayerQuest{}, false, nil
	}

	if next.Progress.Complete() {
		next.State = QuestStateCompleted
		next.CompletedAt = cloneTimePtr(&occurredAt)
	}
	if err := next.Validate(); err != nil {
		return PlayerQuest{}, false, err
	}
	return next, true, nil
}

func progressObjectiveList(schema ObjectiveSchema) ([]Objective, error) {
	if len(schema.Objectives) > 0 {
		return schema.Objectives, nil
	}

	quantity, err := foundation.NewQuantity(schemaRequiredAmount(schema))
	if err != nil {
		return nil, err
	}

	objective := Objective{
		ID:   schema.Kind.String(),
		Kind: schema.Kind,
	}
	switch schema.Kind {
	case ObjectiveKindKill:
		objective.Kill = &KillObjective{
			TargetNPCType: schema.Kill.NPCType,
			RequiredCount: quantity,
		}
	case ObjectiveKindCollect:
		objective.Collect = &CollectObjective{
			ItemID:   schema.Collect.ItemID,
			Quantity: quantity,
		}
	case ObjectiveKindCraft:
		objective.Craft = &CraftObjective{
			RecipeID: schema.Craft.RecipeID,
			ItemID:   schema.Craft.ItemID,
			Quantity: quantity,
		}
	case ObjectiveKindScan:
		objective.Scan = &ScanObjective{
			TargetSignalType: schema.Scan.TargetKind.String(),
			RequiredCount:    quantity,
		}
	case ObjectiveKindBuild:
		objective.Build = &BuildObjective{
			BuildingType:  schema.Build.BuildingID,
			RequiredCount: quantity,
		}
	case ObjectiveKindDeliver:
		objective.Deliver = &DeliverObjective{
			ItemID:          schema.Deliver.ItemID,
			Quantity:        quantity,
			DestinationType: schema.Deliver.DestinationKind.String(),
			DestinationID:   schema.Deliver.DestinationID,
		}
	default:
		return nil, fmt.Errorf("objective kind %q: %w", schema.Kind, ErrInvalidObjectiveKind)
	}
	return []Objective{objective}, nil
}

func cloneGeneratedBoardOffer(offer GeneratedBoardOffer) GeneratedBoardOffer {
	offer.GeneratedPayload = cloneGeneratedPayload(offer.GeneratedPayload)
	offer.RewardPayload = cloneRewardPayload(offer.RewardPayload)
	offer.AcceptedAt = cloneTimePtr(offer.AcceptedAt)
	return offer
}

func clonePlayerQuest(quest PlayerQuest) PlayerQuest {
	quest.GeneratedPayload = cloneGeneratedPayload(quest.GeneratedPayload)
	quest.RewardPayload = cloneRewardPayload(quest.RewardPayload)
	quest.Progress = cloneQuestProgress(quest.Progress)
	quest.ExpiresAt = cloneTimePtr(quest.ExpiresAt)
	quest.CompletedAt = cloneTimePtr(quest.CompletedAt)
	quest.ClaimedAt = cloneTimePtr(quest.ClaimedAt)
	quest.RewardClaimedAt = cloneTimePtr(quest.RewardClaimedAt)
	return quest
}

func cloneGeneratedPayload(payload GeneratedPayload) GeneratedPayload {
	payload.Objective = cloneObjectiveSchema(payload.Objective)
	payload.MetadataJSON = cloneRawMessage(payload.MetadataJSON)
	payload.Data = cloneRawMessage(payload.Data)
	return payload
}

func cloneRewardPayload(payload RewardPayload) RewardPayload {
	payload.Grants = append([]RewardGrant(nil), payload.Grants...)
	payload.RareCapHooks = append([]RewardHook(nil), payload.RareCapHooks...)
	payload.Hooks = append([]RewardHook(nil), payload.Hooks...)
	return payload
}

func cloneQuestProgress(progress QuestProgress) QuestProgress {
	progress.Objectives = append([]ObjectiveProgress(nil), progress.Objectives...)
	return progress
}

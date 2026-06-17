package quests

import (
	"fmt"
	"sort"
	"sync"

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

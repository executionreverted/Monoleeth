package quests

import (
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// MaxActivePlayerQuests is the Phase 07 MVP active quest cap.
const MaxActivePlayerQuests = 3

// QuestService owns Phase 07 quest board persistence and acceptance.
type QuestService struct {
	clock        foundation.Clock
	catalog      QuestCatalog
	store        *InMemoryQuestStore
	servicesMu   sync.RWMutex
	wallet       QuestRewardWalletService
	rerollWallet QuestRerollWalletService
	inventory    QuestRewardInventoryService
	progression  QuestRewardProgressionService
}

// AcceptQuestInput carries server-owned player state plus the offer id from a
// client accept intent. It never accepts client-authored quest progress.
type AcceptQuestInput struct {
	Player  PlayerQuestBoardSnapshot `json:"player"`
	OfferID foundation.QuestID       `json:"offer_id"`
}

// NewQuestService returns a quest service backed by store.
func NewQuestService(clock foundation.Clock, catalog QuestCatalog, store *InMemoryQuestStore) (*QuestService, error) {
	if clock == nil {
		clock = foundation.RealClock{}
	}
	if store == nil {
		store = NewInMemoryQuestStore()
	}
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	return &QuestService{
		clock:   clock,
		catalog: catalog,
		store:   store,
	}, nil
}

// SetRewardServices wires the economy and progression service boundaries used
// by ClaimReward.
func (service *QuestService) SetRewardServices(services QuestRewardServices) {
	service.servicesMu.Lock()
	defer service.servicesMu.Unlock()

	service.wallet = services.Wallet
	if rerollWallet, ok := services.Wallet.(QuestRerollWalletService); ok {
		service.rerollWallet = rerollWallet
	}
	service.inventory = services.Inventory
	service.progression = services.Progression
}

// SetRerollServices wires the wallet boundary used by RerollBoard.
func (service *QuestService) SetRerollServices(services QuestRerollServices) {
	service.servicesMu.Lock()
	defer service.servicesMu.Unlock()

	service.rerollWallet = services.Wallet
}

func (service *QuestService) rewardServices() QuestRewardServices {
	service.servicesMu.RLock()
	defer service.servicesMu.RUnlock()

	return QuestRewardServices{
		Wallet:      service.wallet,
		Inventory:   service.inventory,
		Progression: service.progression,
	}
}

func (service *QuestService) rerollWalletService() QuestRerollWalletService {
	service.servicesMu.RLock()
	defer service.servicesMu.RUnlock()

	return service.rerollWallet
}

// GenerateAndStoreBoard generates a server-owned board and persists its offers.
func (service *QuestService) GenerateAndStoreBoard(input BoardGenerationInput) ([]GeneratedBoardOffer, error) {
	input.Catalog = service.catalog
	if input.CreatedAt.IsZero() {
		input.CreatedAt = service.clock.Now().UTC()
	} else {
		input.CreatedAt = input.CreatedAt.UTC()
	}

	offers, err := GenerateBoard(input)
	if err != nil {
		return nil, err
	}
	if err := service.store.StoreGeneratedBoardOffers(offers); err != nil {
		return nil, err
	}
	return offers, nil
}

// StoreGeneratedBoardOffers persists already generated server-owned offers.
func (service *QuestService) StoreGeneratedBoardOffers(offers []GeneratedBoardOffer) error {
	return service.store.StoreGeneratedBoardOffers(offers)
}

// BoardOffers returns player-visible unaccepted offers and expires old
// unaccepted rows using the service-owned clock.
func (service *QuestService) BoardOffers(playerID foundation.PlayerID) ([]GeneratedBoardOffer, error) {
	now := service.clock.Now().UTC()
	if now.IsZero() {
		return nil, fmt.Errorf("now: %w", ErrZeroQuestTime)
	}
	return service.store.BoardOffersAt(playerID, now)
}

// AcceptQuest accepts a stored offer into durable server-owned player quest
// state. Repeated accepts of the same stored offer return the existing quest.
func (service *QuestService) AcceptQuest(input AcceptQuestInput) (PlayerQuest, error) {
	if err := input.Validate(); err != nil {
		return PlayerQuest{}, err
	}

	acceptedAt := service.clock.Now().UTC()
	if acceptedAt.IsZero() {
		return PlayerQuest{}, fmt.Errorf("accepted_at: %w", ErrZeroQuestTime)
	}

	service.store.mu.Lock()
	defer service.store.mu.Unlock()

	return service.acceptQuestLocked(input, acceptedAt)
}

// Validate reports whether input has a server-owned player snapshot and offer id.
func (input AcceptQuestInput) Validate() error {
	if err := input.Player.Validate(); err != nil {
		return err
	}
	if err := input.OfferID.Validate(); err != nil {
		return err
	}
	return nil
}

func (service *QuestService) acceptQuestLocked(input AcceptQuestInput, acceptedAt time.Time) (PlayerQuest, error) {
	key := newQuestOfferStoreKey(input.Player.PlayerID, input.OfferID)
	offer, ok := service.store.offers[key]
	if !ok {
		if owner, found := service.store.offerOwnerLocked(input.OfferID); found {
			return PlayerQuest{}, fmt.Errorf("offer %q owner %q player %q: %w", input.OfferID, owner, input.Player.PlayerID, ErrQuestOfferOwnerMismatch)
		}
		return PlayerQuest{}, fmt.Errorf("offer %q: %w", input.OfferID, ErrQuestOfferNotFound)
	}
	if offer.PlayerID != input.Player.PlayerID {
		return PlayerQuest{}, fmt.Errorf("offer %q owner %q player %q: %w", input.OfferID, offer.PlayerID, input.Player.PlayerID, ErrQuestOfferOwnerMismatch)
	}

	if quest, ok := service.existingQuestForOfferLocked(key); ok {
		return quest, nil
	}
	if !offer.ExpiresAt.After(acceptedAt) {
		delete(service.store.offers, key)
		return PlayerQuest{}, fmt.Errorf("offer %q expired at %s: %w", offer.OfferID, offer.ExpiresAt, ErrQuestOfferExpired)
	}

	template, ok := service.catalog.Lookup(offer.TemplateID)
	if !ok {
		return PlayerQuest{}, fmt.Errorf("template %q: %w", offer.TemplateID, ErrUnknownQuestTemplate)
	}
	if template.Source != offer.TemplateSource {
		return PlayerQuest{}, fmt.Errorf("source %v template source %v: %w", offer.TemplateSource, template.Source, ErrQuestSourceMismatch)
	}
	if !input.Player.MeetsRequirements(template.Requirements) {
		return PlayerQuest{}, fmt.Errorf("offer %q template %q: %w", offer.OfferID, offer.TemplateID, ErrQuestRequirementsNotMet)
	}
	if service.store.activeQuestCountLocked(input.Player.PlayerID) >= MaxActivePlayerQuests {
		return PlayerQuest{}, fmt.Errorf("player %q active quest cap %d: %w", input.Player.PlayerID, MaxActivePlayerQuests, ErrTooManyActiveQuests)
	}

	playerQuestID := acceptedPlayerQuestID(input.Player.PlayerID, offer.OfferID)
	quest, err := NewAcceptedPlayerQuest(playerQuestID, offer, template.ObjectiveSchema, acceptedAt, nil)
	if err != nil {
		return PlayerQuest{}, err
	}

	offer.AcceptedAt = cloneTimePtr(&acceptedAt)
	service.store.offers[key] = cloneGeneratedBoardOffer(offer)
	service.store.acceptedByOffer[key] = quest.PlayerQuestID
	service.store.quests[quest.PlayerQuestID] = clonePlayerQuest(quest)
	service.store.appendPlayerQuestLocked(quest.PlayerID, quest.PlayerQuestID)

	return clonePlayerQuest(quest), nil
}

func (service *QuestService) existingQuestForOfferLocked(key questOfferStoreKey) (PlayerQuest, bool) {
	questID, ok := service.store.acceptedByOffer[key]
	if !ok {
		questID = acceptedPlayerQuestID(key.playerID, key.offerID)
	}
	quest, found := service.store.quests[questID]
	if !found {
		return PlayerQuest{}, false
	}
	service.store.acceptedByOffer[key] = quest.PlayerQuestID
	offer := service.store.offers[key]
	if offer.AcceptedAt == nil {
		offer.AcceptedAt = cloneTimePtr(&quest.AcceptedAt)
		service.store.offers[key] = cloneGeneratedBoardOffer(offer)
	}
	return clonePlayerQuest(quest), true
}

func acceptedPlayerQuestID(playerID foundation.PlayerID, offerID foundation.QuestID) foundation.QuestID {
	key := "accepted-player-quest|" + playerID.String() + "|" + offerID.String()
	return foundation.QuestID("player_quest_" + stableHex([]byte(key), 24))
}

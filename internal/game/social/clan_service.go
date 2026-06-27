package social

import (
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// Clan is a durable player organization.
type Clan struct {
	ClanID    ClanID              `json:"clan_id"`
	Name      string              `json:"name"`
	Tag       ClanTag             `json:"tag"`
	OwnerID   foundation.PlayerID `json:"owner_id"`
	CreatedAt time.Time           `json:"created_at"`
}

// ClanMembership records one player's clan rank.
type ClanMembership struct {
	ClanID   ClanID              `json:"clan_id"`
	PlayerID foundation.PlayerID `json:"player_id"`
	Rank     ClanRank            `json:"rank"`
	JoinedAt time.Time           `json:"joined_at"`
}

// ClanStore is the persistence boundary for clans and memberships.
type ClanStore interface {
	CreateClan(clan Clan, ownerMembership ClanMembership) error
	Clan(clanID ClanID) (Clan, bool, error)
	ClanByTag(tag ClanTag) (Clan, bool, error)
	Memberships(clanID ClanID) ([]ClanMembership, error)
	Membership(playerID foundation.PlayerID) (ClanMembership, bool, error)
	AddMembership(membership ClanMembership) error
	RemoveMembership(playerID foundation.PlayerID) error
	SetOwner(clanID ClanID, ownerID foundation.PlayerID) error
}

// ClanService owns server-authoritative clan lifecycle.
type ClanService struct {
	mu    sync.Mutex
	store ClanStore
	clock foundation.Clock
	seq   uint64
}

type ClanServiceConfig struct {
	Store ClanStore
	Clock foundation.Clock
}

func NewClanService(config ClanServiceConfig) (*ClanService, error) {
	if config.Store == nil {
		return nil, fmt.Errorf("clan store: %w", ErrClanNotFound)
	}
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &ClanService{
		store: config.Store,
		clock: clock,
	}, nil
}

// CreateClanInput is the player intent for creating a clan.
type CreateClanInput struct {
	OwnerID foundation.PlayerID
	Name    string
	Tag     ClanTag
}

// CreateClan validates uniqueness, creates the clan, and assigns the creator
// the owner rank exactly once.
func (svc *ClanService) CreateClan(input CreateClanInput) (Clan, error) {
	if err := ValidateClanName(input.Name); err != nil {
		return Clan{}, err
	}
	if err := ValidateClanTag(input.Tag); err != nil {
		return Clan{}, err
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()

	if _, ok, err := svc.store.Membership(input.OwnerID); err != nil {
		return Clan{}, err
	} else if ok {
		return Clan{}, fmt.Errorf("player %q: %w", input.OwnerID, ErrAlreadyInClan)
	}
	if _, ok, err := svc.store.ClanByTag(input.Tag); err != nil {
		return Clan{}, err
	} else if ok {
		return Clan{}, ErrClanAlreadyExists
	}

	svc.seq++
	now := svc.clock.Now()
	clanID := ClanID(fmt.Sprintf("clan-%s-%d-%d", input.Tag, now.UnixNano(), svc.seq))
	clan := Clan{
		ClanID:    clanID,
		Name:      input.Name,
		Tag:       input.Tag,
		OwnerID:   input.OwnerID,
		CreatedAt: now,
	}
	ownerMembership := ClanMembership{
		ClanID:   clanID,
		PlayerID: input.OwnerID,
		Rank:     ClanRankOwner,
		JoinedAt: now,
	}
	if err := svc.store.CreateClan(clan, ownerMembership); err != nil {
		return Clan{}, err
	}
	return clan, nil
}

// JoinClan adds a player to a clan at member rank.
func (svc *ClanService) JoinClan(clanID ClanID, playerID foundation.PlayerID) (ClanMembership, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	if existing, ok, err := svc.store.Membership(playerID); err != nil {
		return ClanMembership{}, err
	} else if ok {
		return ClanMembership{}, fmt.Errorf("player %q in clan %q: %w", playerID, existing.ClanID, ErrAlreadyInClan)
	}
	clan, ok, err := svc.store.Clan(clanID)
	if err != nil {
		return ClanMembership{}, err
	}
	if !ok {
		return ClanMembership{}, ErrClanNotFound
	}
	membership := ClanMembership{
		ClanID:   clan.ClanID,
		PlayerID: playerID,
		Rank:     ClanRankMember,
		JoinedAt: svc.clock.Now(),
	}
	if err := svc.store.AddMembership(membership); err != nil {
		return ClanMembership{}, err
	}
	return membership, nil
}

// JoinClanByTag resolves a durable clan tag and joins the matching clan.
func (svc *ClanService) JoinClanByTag(tag ClanTag, playerID foundation.PlayerID) (Clan, ClanMembership, error) {
	if err := ValidateClanTag(tag); err != nil {
		return Clan{}, ClanMembership{}, err
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()

	if existing, ok, err := svc.store.Membership(playerID); err != nil {
		return Clan{}, ClanMembership{}, err
	} else if ok {
		return Clan{}, ClanMembership{}, fmt.Errorf("player %q in clan %q: %w", playerID, existing.ClanID, ErrAlreadyInClan)
	}
	clan, ok, err := svc.store.ClanByTag(tag)
	if err != nil {
		return Clan{}, ClanMembership{}, err
	}
	if !ok {
		return Clan{}, ClanMembership{}, ErrClanNotFound
	}
	membership := ClanMembership{
		ClanID:   clan.ClanID,
		PlayerID: playerID,
		Rank:     ClanRankMember,
		JoinedAt: svc.clock.Now(),
	}
	if err := svc.store.AddMembership(membership); err != nil {
		return Clan{}, ClanMembership{}, err
	}
	return clan, membership, nil
}

func (svc *ClanService) Clan(clanID ClanID) (Clan, bool, error) {
	return svc.store.Clan(clanID)
}

func (svc *ClanService) ClanByTag(tag ClanTag) (Clan, bool, error) {
	return svc.store.ClanByTag(tag)
}

func (svc *ClanService) Membership(playerID foundation.PlayerID) (ClanMembership, bool, error) {
	return svc.store.Membership(playerID)
}

func (svc *ClanService) Memberships(clanID ClanID) ([]ClanMembership, error) {
	return svc.store.Memberships(clanID)
}

// LeaveClan removes a player's membership. If the owner leaves, ownership
// passes to the oldest officer, or the oldest member.
func (svc *ClanService) LeaveClan(playerID foundation.PlayerID) error {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	membership, ok, err := svc.store.Membership(playerID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotInClan
	}

	if err := svc.store.RemoveMembership(playerID); err != nil {
		return err
	}

	if membership.Rank == ClanRankOwner {
		members, err := svc.store.Memberships(membership.ClanID)
		if err != nil {
			return err
		}
		if len(members) == 0 {
			return nil
		}
		newOwner := selectClanSuccessor(members)
		newOwner.Rank = ClanRankOwner
		if err := svc.store.AddMembership(newOwner); err != nil {
			return err
		}
		if err := svc.store.SetOwner(membership.ClanID, newOwner.PlayerID); err != nil {
			return err
		}
	}
	return nil
}

// ClanChatMembers returns the player ids who can read a clan chat channel.
func (svc *ClanService) ClanChatMembers(clanID ClanID) ([]foundation.PlayerID, error) {
	members, err := svc.store.Memberships(clanID)
	if err != nil {
		return nil, err
	}
	ids := make([]foundation.PlayerID, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.PlayerID)
	}
	return ids, nil
}

func selectClanSuccessor(members []ClanMembership) ClanMembership {
	var bestOfficer *ClanMembership
	var bestMember *ClanMembership
	for i := range members {
		if members[i].Rank == ClanRankOfficer {
			if bestOfficer == nil || members[i].JoinedAt.Before(bestOfficer.JoinedAt) {
				bestOfficer = &members[i]
			}
		}
		if members[i].Rank == ClanRankMember {
			if bestMember == nil || members[i].JoinedAt.Before(bestMember.JoinedAt) {
				bestMember = &members[i]
			}
		}
	}
	if bestOfficer != nil {
		return *bestOfficer
	}
	return *bestMember
}

// InMemoryClanStore is a process-local clan store.
type InMemoryClanStore struct {
	mu          sync.RWMutex
	clans       map[ClanID]Clan
	clansByTag  map[ClanTag]ClanID
	memberships map[foundation.PlayerID]ClanMembership
	clanMembers map[ClanID][]ClanMembership
}

func NewInMemoryClanStore() *InMemoryClanStore {
	return &InMemoryClanStore{
		clans:       make(map[ClanID]Clan),
		clansByTag:  make(map[ClanTag]ClanID),
		memberships: make(map[foundation.PlayerID]ClanMembership),
		clanMembers: make(map[ClanID][]ClanMembership),
	}
}

func (store *InMemoryClanStore) CreateClan(clan Clan, ownerMembership ClanMembership) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.clans[clan.ClanID] = clan
	store.clansByTag[clan.Tag] = clan.ClanID
	store.memberships[ownerMembership.PlayerID] = ownerMembership
	store.clanMembers[clan.ClanID] = append(store.clanMembers[clan.ClanID], ownerMembership)
	return nil
}

func (store *InMemoryClanStore) Clan(clanID ClanID) (Clan, bool, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	clan, ok := store.clans[clanID]
	return clan, ok, nil
}

func (store *InMemoryClanStore) ClanByTag(tag ClanTag) (Clan, bool, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	clanID, ok := store.clansByTag[tag]
	if !ok {
		return Clan{}, false, nil
	}
	clan := store.clans[clanID]
	return clan, true, nil
}

func (store *InMemoryClanStore) Memberships(clanID ClanID) ([]ClanMembership, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	members := store.clanMembers[clanID]
	result := make([]ClanMembership, len(members))
	copy(result, members)
	return result, nil
}

func (store *InMemoryClanStore) Membership(playerID foundation.PlayerID) (ClanMembership, bool, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	m, ok := store.memberships[playerID]
	return m, ok, nil
}

func (store *InMemoryClanStore) AddMembership(membership ClanMembership) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if existing, ok := store.memberships[membership.PlayerID]; ok {
		members := store.clanMembers[existing.ClanID]
		for i, m := range members {
			if m.PlayerID == membership.PlayerID {
				store.clanMembers[existing.ClanID] = append(members[:i], members[i+1:]...)
				break
			}
		}
	}
	store.memberships[membership.PlayerID] = membership
	store.clanMembers[membership.ClanID] = append(store.clanMembers[membership.ClanID], membership)
	return nil
}

func (store *InMemoryClanStore) RemoveMembership(playerID foundation.PlayerID) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	membership, ok := store.memberships[playerID]
	if !ok {
		return nil
	}
	delete(store.memberships, playerID)
	members := store.clanMembers[membership.ClanID]
	for i, m := range members {
		if m.PlayerID == playerID {
			store.clanMembers[membership.ClanID] = append(members[:i], members[i+1:]...)
			break
		}
	}
	return nil
}

func (store *InMemoryClanStore) SetOwner(clanID ClanID, ownerID foundation.PlayerID) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	clan, ok := store.clans[clanID]
	if !ok {
		return ErrClanNotFound
	}
	clan.OwnerID = ownerID
	store.clans[clanID] = clan
	return nil
}

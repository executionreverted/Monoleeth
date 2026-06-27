package social

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

type ContributionScopeKind string

const (
	ContributionScopeParty ContributionScopeKind = "party"
	ContributionScopeClan  ContributionScopeKind = "clan"
)

type ContributionEvent struct {
	EventID       foundation.EventID    `json:"event_id"`
	ScopeKind     ContributionScopeKind `json:"scope_kind"`
	ScopeID       string                `json:"scope_id"`
	SourceKind    string                `json:"source_kind"`
	SourceID      string                `json:"source_id"`
	ActorPlayerID foundation.PlayerID   `json:"actor_player_id"`
	TargetID      string                `json:"target_id"`
	Amount        float64               `json:"amount"`
	OccurredAt    time.Time             `json:"occurred_at"`
}

type ContributionMember struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	Amount   float64             `json:"amount"`
}

type ContributionSnapshot struct {
	ScopeKind  ContributionScopeKind `json:"scope_kind"`
	ScopeID    string                `json:"scope_id"`
	SourceKind string                `json:"source_kind"`
	SourceID   string                `json:"source_id"`
	TargetID   string                `json:"target_id"`
	Members    []ContributionMember  `json:"members"`
	UpdatedAt  time.Time             `json:"updated_at"`
}

type ContributionStore interface {
	RecordContribution(event ContributionEvent) (ContributionSnapshot, error)
}

type ContributionService struct {
	mu      sync.Mutex
	store   ContributionStore
	parties *PartyService
	clans   *ClanService
	clock   foundation.Clock
	seq     uint64
}

type ContributionServiceConfig struct {
	Store   ContributionStore
	Parties *PartyService
	Clans   *ClanService
	Clock   foundation.Clock
}

type RecordNPCContributionInput struct {
	NPCEntityID   world.EntityID
	NPCType       string
	Contributions map[foundation.PlayerID]float64
	OccurredAt    time.Time
}

func NewContributionService(config ContributionServiceConfig) (*ContributionService, error) {
	if config.Store == nil {
		return nil, fmt.Errorf("contribution store: %w", ErrPartyNotFound)
	}
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &ContributionService{
		store:   config.Store,
		parties: config.Parties,
		clans:   config.Clans,
		clock:   clock,
	}, nil
}

func (svc *ContributionService) RecordNPCContributions(input RecordNPCContributionInput) ([]ContributionSnapshot, error) {
	if svc == nil || len(input.Contributions) == 0 {
		return nil, nil
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = svc.clock.Now()
	}
	sourceID := svc.nextContributionSourceID(input.OccurredAt)
	players := sortedContributionPlayers(input.Contributions)
	snapshots := make(map[string]ContributionSnapshot)
	for _, playerID := range players {
		amount := input.Contributions[playerID]
		if amount <= 0 || playerID.IsZero() {
			continue
		}
		if svc.parties != nil {
			if party, ok := svc.parties.GetParty(playerID); ok {
				event := npcContributionEvent(sourceID, playerID, amount, ContributionScopeParty, string(party.PartyID), input.OccurredAt)
				snapshot, err := svc.store.RecordContribution(event)
				if err != nil {
					return nil, err
				}
				snapshots[contributionSnapshotKey(snapshot)] = snapshot
			}
		}
		if svc.clans != nil {
			if membership, ok, err := svc.clans.Membership(playerID); err != nil {
				return nil, err
			} else if ok {
				event := npcContributionEvent(sourceID, playerID, amount, ContributionScopeClan, string(membership.ClanID), input.OccurredAt)
				snapshot, err := svc.store.RecordContribution(event)
				if err != nil {
					return nil, err
				}
				snapshots[contributionSnapshotKey(snapshot)] = snapshot
			}
		}
	}
	return sortedContributionSnapshots(snapshots), nil
}

func (svc *ContributionService) nextContributionSourceID(occurredAt time.Time) string {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.seq++
	return fmt.Sprintf("npc_kill:%d:%d", occurredAt.UnixNano(), svc.seq)
}

func npcContributionEvent(sourceID string, playerID foundation.PlayerID, amount float64, scopeKind ContributionScopeKind, scopeID string, occurredAt time.Time) ContributionEvent {
	return ContributionEvent{
		EventID:       foundation.EventID(fmt.Sprintf("social_contribution:%s:%s:%s:%s", scopeKind, scopeID, sourceID, playerID.String())),
		ScopeKind:     scopeKind,
		ScopeID:       scopeID,
		SourceKind:    "npc_kill",
		SourceID:      sourceID,
		ActorPlayerID: playerID,
		TargetID:      sourceID,
		Amount:        amount,
		OccurredAt:    occurredAt,
	}
}

func sortedContributionPlayers(contributions map[foundation.PlayerID]float64) []foundation.PlayerID {
	players := make([]foundation.PlayerID, 0, len(contributions))
	for playerID := range contributions {
		players = append(players, playerID)
	}
	sort.Slice(players, func(i, j int) bool { return players[i] < players[j] })
	return players
}

func contributionSnapshotKey(snapshot ContributionSnapshot) string {
	return string(snapshot.ScopeKind) + ":" + snapshot.ScopeID + ":" + snapshot.SourceKind + ":" + snapshot.SourceID
}

func sortedContributionSnapshots(snapshots map[string]ContributionSnapshot) []ContributionSnapshot {
	keys := make([]string, 0, len(snapshots))
	for key := range snapshots {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]ContributionSnapshot, 0, len(keys))
	for _, key := range keys {
		result = append(result, snapshots[key])
	}
	return result
}

type InMemoryContributionStore struct {
	mu        sync.RWMutex
	seen      map[foundation.EventID]struct{}
	snapshots map[string]ContributionSnapshot
	totals    map[string]map[foundation.PlayerID]float64
}

func NewInMemoryContributionStore() *InMemoryContributionStore {
	return &InMemoryContributionStore{
		seen:      make(map[foundation.EventID]struct{}),
		snapshots: make(map[string]ContributionSnapshot),
		totals:    make(map[string]map[foundation.PlayerID]float64),
	}
}

func (store *InMemoryContributionStore) RecordContribution(event ContributionEvent) (ContributionSnapshot, error) {
	if store == nil {
		return ContributionSnapshot{}, fmt.Errorf("contribution store: %w", ErrPartyNotFound)
	}
	if event.EventID == "" || event.ScopeKind == "" || event.ScopeID == "" || event.SourceKind == "" || event.SourceID == "" || event.ActorPlayerID.IsZero() || event.Amount <= 0 {
		return ContributionSnapshot{}, ErrInvalidPartyTarget
	}
	key := string(event.ScopeKind) + ":" + event.ScopeID + ":" + event.SourceKind + ":" + event.SourceID
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.totals[key]; !ok {
		store.totals[key] = make(map[foundation.PlayerID]float64)
	}
	if _, seen := store.seen[event.EventID]; !seen {
		store.totals[key][event.ActorPlayerID] += event.Amount
		store.seen[event.EventID] = struct{}{}
	}
	snapshot := ContributionSnapshot{
		ScopeKind:  event.ScopeKind,
		ScopeID:    event.ScopeID,
		SourceKind: event.SourceKind,
		SourceID:   event.SourceID,
		TargetID:   event.TargetID,
		Members:    contributionMembersFromTotals(store.totals[key]),
		UpdatedAt:  event.OccurredAt,
	}
	store.snapshots[key] = snapshot
	return snapshot, nil
}

func contributionMembersFromTotals(totals map[foundation.PlayerID]float64) []ContributionMember {
	players := make([]foundation.PlayerID, 0, len(totals))
	for playerID := range totals {
		players = append(players, playerID)
	}
	sort.Slice(players, func(i, j int) bool { return players[i] < players[j] })
	members := make([]ContributionMember, 0, len(players))
	for _, playerID := range players {
		members = append(members, ContributionMember{PlayerID: playerID, Amount: totals[playerID]})
	}
	return members
}

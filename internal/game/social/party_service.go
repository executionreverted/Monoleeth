package social

import (
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/foundation"
)

// PartyMember records one player's membership in a party.
type PartyMember struct {
	PlayerID foundation.PlayerID `json:"player_id"`
	JoinedAt time.Time           `json:"joined_at"`
	IsLeader bool                `json:"is_leader"`
}

// Party is a transient group with server-owned membership.
type Party struct {
	PartyID   PartyID       `json:"party_id"`
	Members   []PartyMember `json:"members"`
	CreatedAt time.Time     `json:"created_at"`
}

// PartyInvite is a pending invite from a leader to a player.
type PartyInvite struct {
	InviteID  string              `json:"invite_id"`
	PartyID   PartyID             `json:"party_id"`
	InviterID foundation.PlayerID `json:"inviter_id"`
	InviteeID foundation.PlayerID `json:"invitee_id"`
	CreatedAt time.Time           `json:"created_at"`
	ExpiresAt time.Time           `json:"expires_at"`
}

// PartyService owns server-authoritative party lifecycle.
type PartyService struct {
	mu          sync.Mutex
	clock       foundation.Clock
	parties     map[PartyID]*Party
	playerParty map[foundation.PlayerID]PartyID
	invites     map[string]*PartyInvite
	seq         uint64
}

func NewPartyService(clock foundation.Clock) *PartyService {
	if clock == nil {
		clock = foundation.RealClock{}
	}
	return &PartyService{
		clock:       clock,
		parties:     make(map[PartyID]*Party),
		playerParty: make(map[foundation.PlayerID]PartyID),
		invites:     make(map[string]*PartyInvite),
	}
}

// CreateParty creates a new party with the creator as leader.
func (svc *PartyService) CreateParty(leaderID foundation.PlayerID) (Party, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	if existing, ok := svc.playerParty[leaderID]; ok {
		return Party{}, fmt.Errorf("player %q in party %q: %w", leaderID, existing, ErrAlreadyInParty)
	}

	svc.seq++
	partyID := PartyID(fmt.Sprintf("party-%d", svc.seq))
	now := svc.clock.Now()
	party := &Party{
		PartyID:   partyID,
		CreatedAt: now,
		Members: []PartyMember{
			{PlayerID: leaderID, JoinedAt: now, IsLeader: true},
		},
	}
	svc.parties[partyID] = party
	svc.playerParty[leaderID] = partyID
	return *cloneParty(party), nil
}

// InvitePlayer creates a pending invite. The invitee must not already be in a party.
func (svc *PartyService) InvitePlayer(inviterID, inviteeID foundation.PlayerID) (PartyInvite, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	partyID, ok := svc.playerParty[inviterID]
	if !ok {
		return PartyInvite{}, ErrNotInParty
	}
	party := svc.parties[partyID]

	leader := false
	for _, m := range party.Members {
		if m.PlayerID == inviterID && m.IsLeader {
			leader = true
			break
		}
	}
	if !leader {
		return PartyInvite{}, fmt.Errorf("inviter %q is not leader: %w", inviterID, ErrChannelAccessDenied)
	}
	if len(party.Members) >= maxPartyMembers {
		return PartyInvite{}, ErrPartyFull
	}
	if _, ok := svc.playerParty[inviteeID]; ok {
		return PartyInvite{}, ErrAlreadyInParty
	}

	now := svc.clock.Now()
	svc.seq++
	inviteID := fmt.Sprintf("invite-%d", svc.seq)
	invite := &PartyInvite{
		InviteID:  inviteID,
		PartyID:   partyID,
		InviterID: inviterID,
		InviteeID: inviteeID,
		CreatedAt: now,
		ExpiresAt: now.Add(60 * time.Second),
	}
	svc.invites[inviteID] = invite
	return *invite, nil
}

// AcceptInvite adds the invitee to the party and consumes the invite.
func (svc *PartyService) AcceptInvite(inviteID string, playerID foundation.PlayerID) (Party, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	invite, ok := svc.invites[inviteID]
	if !ok {
		return Party{}, ErrPartyInviteNotFound
	}
	if invite.InviteeID != playerID {
		return Party{}, ErrPartyInviteNotFound
	}
	if svc.clock.Now().After(invite.ExpiresAt) {
		delete(svc.invites, inviteID)
		return Party{}, ErrPartyInviteNotFound
	}
	party, ok := svc.parties[invite.PartyID]
	if !ok {
		delete(svc.invites, inviteID)
		return Party{}, ErrPartyNotFound
	}
	if len(party.Members) >= maxPartyMembers {
		delete(svc.invites, inviteID)
		return Party{}, ErrPartyFull
	}
	if _, ok := svc.playerParty[playerID]; ok {
		delete(svc.invites, inviteID)
		return Party{}, ErrAlreadyInParty
	}

	now := svc.clock.Now()
	party.Members = append(party.Members, PartyMember{
		PlayerID: playerID,
		JoinedAt: now,
	})
	svc.playerParty[playerID] = invite.PartyID
	delete(svc.invites, inviteID)
	return *cloneParty(party), nil
}

// LeaveParty removes a player from their party. If the leader leaves, leadership
// passes to the next member or the party disbands if empty.
func (svc *PartyService) LeaveParty(playerID foundation.PlayerID) error {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	partyID, ok := svc.playerParty[playerID]
	if !ok {
		return ErrNotInParty
	}
	party := svc.parties[partyID]

	idx := -1
	for i, m := range party.Members {
		if m.PlayerID == playerID {
			idx = i
			break
		}
	}
	if idx < 0 {
		delete(svc.playerParty, playerID)
		return ErrNotInParty
	}

	wasLeader := party.Members[idx].IsLeader
	party.Members = append(party.Members[:idx], party.Members[idx+1:]...)
	delete(svc.playerParty, playerID)

	if len(party.Members) == 0 {
		delete(svc.parties, partyID)
		return nil
	}
	if wasLeader {
		party.Members[0].IsLeader = true
	}
	return nil
}

// GetParty returns the party a player belongs to.
func (svc *PartyService) GetParty(playerID foundation.PlayerID) (Party, bool) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	partyID, ok := svc.playerParty[playerID]
	if !ok {
		return Party{}, false
	}
	party, ok := svc.parties[partyID]
	if !ok {
		return Party{}, false
	}
	return *cloneParty(party), true
}

func cloneParty(p *Party) *Party {
	c := &Party{
		PartyID:   p.PartyID,
		CreatedAt: p.CreatedAt,
		Members:   make([]PartyMember, len(p.Members)),
	}
	copy(c.Members, p.Members)
	return c
}

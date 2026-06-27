package social

import (
	"fmt"
	"sync"

	"gameproject/internal/game/foundation"
)

// ChannelMembershipService resolves channel membership for chat routing.
// It delegates to party and clan services for party/clan channels, and
// resolves local-map/system channels from the runtime session map.
type ChannelMembershipService struct {
	mu         sync.RWMutex
	parties    *PartyService
	clans      *ClanService
	playerMap  map[foundation.PlayerID]string
	mapPlayers map[string]map[foundation.PlayerID]bool
}

// NewChannelMembershipService creates a channel resolver backed by the given
// party and clan services. Player-map bindings are updated by the runtime as
// players enter/leave maps.
func NewChannelMembershipService(parties *PartyService, clans *ClanService) *ChannelMembershipService {
	return &ChannelMembershipService{
		parties:    parties,
		clans:      clans,
		playerMap:  make(map[foundation.PlayerID]string),
		mapPlayers: make(map[string]map[foundation.PlayerID]bool),
	}
}

// SetPlayerMap binds a player to a map for local-map chat routing.
func (svc *ChannelMembershipService) SetPlayerMap(playerID foundation.PlayerID, mapID string) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if old, ok := svc.playerMap[playerID]; ok {
		if players, ok := svc.mapPlayers[old]; ok {
			delete(players, playerID)
		}
	}
	svc.playerMap[playerID] = mapID
	if svc.mapPlayers[mapID] == nil {
		svc.mapPlayers[mapID] = make(map[foundation.PlayerID]bool)
	}
	svc.mapPlayers[mapID][playerID] = true
}

// RemovePlayerMap unbinds a player from map routing (e.g. on disconnect).
func (svc *ChannelMembershipService) RemovePlayerMap(playerID foundation.PlayerID) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if old, ok := svc.playerMap[playerID]; ok {
		if players, ok := svc.mapPlayers[old]; ok {
			delete(players, playerID)
		}
	}
	delete(svc.playerMap, playerID)
}

// ResolveChannel resolves whether a player can read/write a channel and
// returns the canonical channel id.
func (svc *ChannelMembershipService) ResolveChannel(input ResolveChannelInput) (ResolveChannelResult, error) {
	if err := ValidateChannelKind(input.Kind); err != nil {
		return ResolveChannelResult{}, err
	}

	switch input.Kind {
	case ChannelKindSystem:
		return ResolveChannelResult{
			ChannelID: ChannelID("system"),
			CanRead:   true,
			CanWrite:  false,
		}, nil

	case ChannelKindLocalMap:
		svc.mu.RLock()
		mapID, ok := svc.playerMap[input.PlayerID]
		svc.mu.RUnlock()
		if !ok {
			return ResolveChannelResult{}, ErrChannelAccessDenied
		}
		return ResolveChannelResult{
			ChannelID: ChannelID(fmt.Sprintf("map:%s", mapID)),
			CanRead:   true,
			CanWrite:  true,
		}, nil

	case ChannelKindParty:
		if svc.parties == nil {
			return ResolveChannelResult{}, ErrChannelAccessDenied
		}
		party, ok := svc.parties.GetParty(input.PlayerID)
		if !ok {
			return ResolveChannelResult{}, ErrChannelAccessDenied
		}
		return ResolveChannelResult{
			ChannelID: ChannelID(fmt.Sprintf("party:%s", party.PartyID)),
			CanRead:   true,
			CanWrite:  true,
		}, nil

	case ChannelKindClan:
		if svc.clans == nil {
			return ResolveChannelResult{}, ErrChannelAccessDenied
		}
		membership, ok, err := svc.clans.store.Membership(input.PlayerID)
		if err != nil || !ok {
			return ResolveChannelResult{}, ErrChannelAccessDenied
		}
		return ResolveChannelResult{
			ChannelID: ChannelID(fmt.Sprintf("clan:%s", membership.ClanID)),
			CanRead:   true,
			CanWrite:  true,
		}, nil
	}

	return ResolveChannelResult{}, ErrChannelAccessDenied
}

// ChannelMembers returns the player ids who should receive messages on a channel.
func (svc *ChannelMembershipService) ChannelMembers(channelID ChannelID) ([]foundation.PlayerID, error) {
	idStr := string(channelID)
	if idStr == "system" {
		svc.mu.RLock()
		defer svc.mu.RUnlock()
		ids := make([]foundation.PlayerID, 0, len(svc.playerMap))
		for pid := range svc.playerMap {
			ids = append(ids, pid)
		}
		return ids, nil
	}

	prefix, rest := splitChannelID(idStr)
	switch prefix {
	case "map":
		svc.mu.RLock()
		defer svc.mu.RUnlock()
		players := svc.mapPlayers[rest]
		ids := make([]foundation.PlayerID, 0, len(players))
		for pid := range players {
			ids = append(ids, pid)
		}
		return ids, nil

	case "party":
		if svc.parties == nil {
			return nil, nil
		}
		svc.parties.mu.Lock()
		defer svc.parties.mu.Unlock()
		party, ok := svc.parties.parties[PartyID(rest)]
		if !ok {
			return nil, nil
		}
		ids := make([]foundation.PlayerID, 0, len(party.Members))
		for _, m := range party.Members {
			ids = append(ids, m.PlayerID)
		}
		return ids, nil

	case "clan":
		if svc.clans == nil {
			return nil, nil
		}
		return svc.clans.ClanChatMembers(ClanID(rest))
	}

	return nil, nil
}

func splitChannelID(id string) (prefix, rest string) {
	for i := 0; i < len(id); i++ {
		if id[i] == ':' {
			return id[:i], id[i+1:]
		}
	}
	return id, ""
}

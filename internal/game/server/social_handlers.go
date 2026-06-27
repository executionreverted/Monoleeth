package server

import (
	"encoding/json"
	"errors"
	"strings"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/social"
	"gameproject/internal/game/world"
)

type chatSendPayload struct {
	Kind    social.ChannelKind `json:"kind"`
	Content string             `json:"content"`
}

type chatSendResponse struct {
	Accepted bool               `json:"accepted"`
	Message  social.ChatMessage `json:"message"`
}

type partyInvitePayload struct {
	InviteeCallsign string `json:"invitee_callsign"`
}

type partyInviteResponse struct {
	Accepted bool               `json:"accepted"`
	Invite   social.PartyInvite `json:"invite"`
	Party    social.Party       `json:"party"`
}

type partyAcceptPayload struct {
	InviteID string `json:"invite_id"`
}

type partyResponse struct {
	Accepted bool         `json:"accepted"`
	Party    social.Party `json:"party"`
}

type partyLeaveResponse struct {
	Accepted bool           `json:"accepted"`
	PartyID  social.PartyID `json:"party_id,omitempty"`
}

type partyTargetSetPayload struct {
	TargetID string `json:"target_id"`
}

type partyTargetSetResponse struct {
	Accepted bool                     `json:"accepted"`
	Party    social.Party             `json:"party"`
	Target   social.PartySharedTarget `json:"target"`
}

type clanCreatePayload struct {
	Name string         `json:"name"`
	Tag  social.ClanTag `json:"tag"`
}

type clanJoinPayload struct {
	Tag social.ClanTag `json:"tag"`
}

type clanResponse struct {
	Accepted   bool                    `json:"accepted"`
	Clan       social.Clan             `json:"clan,omitempty"`
	Membership social.ClanMembership   `json:"membership,omitempty"`
	Members    []social.ClanMembership `json:"members,omitempty"`
	ClanID     social.ClanID           `json:"clan_id,omitempty"`
}

func (runtime *Runtime) handleChatSend(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload chatSendPayload
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.SocialChat == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Social chat is unavailable.")
	}
	result, err := runtime.SocialChat.SendMessage(social.SendChatInput{
		Kind:     payload.Kind,
		SenderID: ctx.PlayerID,
		Content:  payload.Content,
	})
	if err != nil {
		return nil, socialDomainError(err)
	}
	runtime.mu.Lock()
	for _, playerID := range result.Members {
		runtime.queueSocialEventToPlayerSessionsLocked(playerID, realtime.EventChatMessage, result.Message)
	}
	runtime.mu.Unlock()
	return marshalPayload(chatSendResponse{Accepted: true, Message: result.Message})
}

func (runtime *Runtime) handlePartyTargetSet(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload partyTargetSetPayload
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	targetID, err := foundation.ParseEntityID(strings.TrimSpace(payload.TargetID))
	if err != nil {
		return nil, invalidPayload("Party target is invalid.", err)
	}
	if runtime.SocialParty == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Social party is unavailable.")
	}
	if err := runtime.requireVisiblePartyTarget(ctx.PlayerID, targetID); err != nil {
		return nil, err
	}
	party, target, err := runtime.SocialParty.SetSharedTarget(ctx.PlayerID, targetID.String())
	if err != nil {
		return nil, socialDomainError(err)
	}
	response := partyTargetSetResponse{Accepted: true, Party: party, Target: target}
	runtime.mu.Lock()
	runtime.queuePartyEventLocked(party, realtime.EventPartyTargetUpdated, response)
	runtime.mu.Unlock()
	return marshalPayload(response)
}

func (runtime *Runtime) handleClanCreate(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload clanCreatePayload
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.SocialClan == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Social clan is unavailable.")
	}
	clan, err := runtime.SocialClan.CreateClan(social.CreateClanInput{
		OwnerID: ctx.PlayerID,
		Name:    strings.TrimSpace(payload.Name),
		Tag:     social.ClanTag(strings.ToUpper(strings.TrimSpace(string(payload.Tag)))),
	})
	if err != nil {
		return nil, socialDomainError(err)
	}
	snapshot, err := runtime.clanSnapshotFor(ctx.PlayerID)
	if err != nil {
		return nil, err
	}
	snapshot.Accepted = true
	snapshot.Clan = clan
	runtime.mu.Lock()
	runtime.queueSocialEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventClanUpdated, snapshot)
	runtime.mu.Unlock()
	return marshalPayload(snapshot)
}

func (runtime *Runtime) handleClanJoin(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload clanJoinPayload
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.SocialClan == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Social clan is unavailable.")
	}
	clan, _, err := runtime.SocialClan.JoinClanByTag(social.ClanTag(strings.ToUpper(strings.TrimSpace(string(payload.Tag)))), ctx.PlayerID)
	if err != nil {
		return nil, socialDomainError(err)
	}
	snapshot, err := runtime.clanSnapshotFor(ctx.PlayerID)
	if err != nil {
		return nil, err
	}
	snapshot.Accepted = true
	snapshot.Clan = clan
	runtime.mu.Lock()
	runtime.queueClanSnapshotsLocked(clan.ClanID)
	runtime.mu.Unlock()
	return marshalPayload(snapshot)
}

func (runtime *Runtime) handleClanLeave(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct{}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.SocialClan == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Social clan is unavailable.")
	}
	membership, ok, err := runtime.SocialClan.Membership(ctx.PlayerID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, socialDomainError(social.ErrNotInClan)
	}
	if err := runtime.SocialClan.LeaveClan(ctx.PlayerID); err != nil {
		return nil, socialDomainError(err)
	}
	response := clanResponse{Accepted: true, ClanID: membership.ClanID}
	runtime.mu.Lock()
	runtime.queueSocialEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventClanLeft, response)
	runtime.queueClanUpdatedAfterLeaveLocked(membership.ClanID)
	runtime.mu.Unlock()
	return marshalPayload(response)
}

func (runtime *Runtime) handlePartyInvite(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload partyInvitePayload
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	inviteeID, err := runtime.resolveOnlinePlayerByCallsign(payload.InviteeCallsign)
	if err != nil {
		return nil, err
	}
	if inviteeID == ctx.PlayerID {
		return nil, foundation.NewDomainError(foundation.CodeForbidden, "Cannot invite yourself.")
	}
	if runtime.SocialParty == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Social party is unavailable.")
	}
	if _, ok := runtime.SocialParty.GetParty(ctx.PlayerID); !ok {
		if _, err := runtime.SocialParty.CreateParty(ctx.PlayerID); err != nil {
			return nil, socialDomainError(err)
		}
	}
	invite, err := runtime.SocialParty.InvitePlayer(ctx.PlayerID, inviteeID)
	if err != nil {
		return nil, socialDomainError(err)
	}
	party, _ := runtime.SocialParty.GetParty(ctx.PlayerID)
	runtime.mu.Lock()
	runtime.queueSocialEventToPlayerSessionsLocked(inviteeID, realtime.EventPartyInvite, invite)
	runtime.queuePartyEventLocked(party, realtime.EventPartyUpdated, party)
	runtime.mu.Unlock()
	return marshalPayload(partyInviteResponse{Accepted: true, Invite: invite, Party: party})
}

func (runtime *Runtime) handlePartyAccept(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload partyAcceptPayload
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if payload.InviteID == "" {
		return nil, invalidPayload("Invite id is required.", nil)
	}
	if runtime.SocialParty == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Social party is unavailable.")
	}
	party, err := runtime.SocialParty.AcceptInvite(payload.InviteID, ctx.PlayerID)
	if err != nil {
		return nil, socialDomainError(err)
	}
	runtime.mu.Lock()
	runtime.queuePartyEventLocked(party, realtime.EventPartyUpdated, party)
	runtime.mu.Unlock()
	return marshalPayload(partyResponse{Accepted: true, Party: party})
}

func (runtime *Runtime) handlePartyLeave(ctx realtime.CommandContext, request realtime.RequestEnvelope) (json.RawMessage, error) {
	if err := rejectTrustedPayload(request.Payload); err != nil {
		return nil, err
	}
	var payload struct{}
	if err := decodeStrict(request.Payload, &payload); err != nil {
		return nil, err
	}
	if runtime.SocialParty == nil {
		return nil, foundation.NewDomainError(foundation.CodeInternal, "Social party is unavailable.")
	}
	party, ok := runtime.SocialParty.GetParty(ctx.PlayerID)
	if !ok {
		return nil, socialDomainError(social.ErrNotInParty)
	}
	if err := runtime.SocialParty.LeaveParty(ctx.PlayerID); err != nil {
		return nil, socialDomainError(err)
	}
	updated, stillInParty := runtime.partyForRemainingMember(party, ctx.PlayerID)
	runtime.mu.Lock()
	runtime.queueSocialEventToPlayerSessionsLocked(ctx.PlayerID, realtime.EventPartyLeft, partyLeaveResponse{Accepted: true, PartyID: party.PartyID})
	if stillInParty {
		runtime.queuePartyEventLocked(updated, realtime.EventPartyUpdated, updated)
	}
	runtime.mu.Unlock()
	return marshalPayload(partyLeaveResponse{Accepted: true, PartyID: party.PartyID})
}

func (runtime *Runtime) partyForRemainingMember(previous social.Party, leavingPlayerID foundation.PlayerID) (social.Party, bool) {
	for _, member := range previous.Members {
		if member.PlayerID == leavingPlayerID {
			continue
		}
		if party, ok := runtime.SocialParty.GetParty(member.PlayerID); ok {
			return party, true
		}
	}
	return social.Party{}, false
}

func (runtime *Runtime) queuePartyEventLocked(party social.Party, eventType realtime.ClientEventType, payload any) {
	for _, member := range party.Members {
		runtime.queueSocialEventToPlayerSessionsLocked(member.PlayerID, eventType, payload)
	}
}

func (runtime *Runtime) queueClanEventLocked(clanID social.ClanID, eventType realtime.ClientEventType, payload any) {
	if runtime.SocialClan == nil {
		return
	}
	members, err := runtime.SocialClan.ClanChatMembers(clanID)
	if err != nil {
		return
	}
	for _, memberID := range members {
		runtime.queueSocialEventToPlayerSessionsLocked(memberID, eventType, payload)
	}
}

func (runtime *Runtime) queueClanUpdatedAfterLeaveLocked(clanID social.ClanID) {
	if runtime.SocialClan == nil {
		return
	}
	clan, ok, err := runtime.SocialClan.Clan(clanID)
	if err != nil || !ok {
		return
	}
	memberships, err := runtime.SocialClan.Memberships(clanID)
	if err != nil {
		return
	}
	for _, membership := range memberships {
		payload := clanResponse{
			Accepted:   true,
			Clan:       clan,
			Membership: membership,
			Members:    memberships,
			ClanID:     clan.ClanID,
		}
		runtime.queueSocialEventToPlayerSessionsLocked(membership.PlayerID, realtime.EventClanUpdated, payload)
	}
}

func (runtime *Runtime) queueClanSnapshotsLocked(clanID social.ClanID) {
	if runtime.SocialClan == nil {
		return
	}
	clan, ok, err := runtime.SocialClan.Clan(clanID)
	if err != nil || !ok {
		return
	}
	memberships, err := runtime.SocialClan.Memberships(clanID)
	if err != nil {
		return
	}
	for _, membership := range memberships {
		payload := clanResponse{
			Accepted:   true,
			Clan:       clan,
			Membership: membership,
			Members:    memberships,
			ClanID:     clan.ClanID,
		}
		runtime.queueSocialEventToPlayerSessionsLocked(membership.PlayerID, realtime.EventClanUpdated, payload)
	}
}

func (runtime *Runtime) clanSnapshotFor(playerID foundation.PlayerID) (clanResponse, error) {
	membership, ok, err := runtime.SocialClan.Membership(playerID)
	if err != nil {
		return clanResponse{}, err
	}
	if !ok {
		return clanResponse{}, social.ErrNotInClan
	}
	clan, ok, err := runtime.SocialClan.Clan(membership.ClanID)
	if err != nil {
		return clanResponse{}, err
	}
	if !ok {
		return clanResponse{}, social.ErrClanNotFound
	}
	members, err := runtime.SocialClan.Memberships(membership.ClanID)
	if err != nil {
		return clanResponse{}, err
	}
	return clanResponse{Clan: clan, Membership: membership, Members: members, ClanID: clan.ClanID}, nil
}

func (runtime *Runtime) requireVisiblePartyTarget(playerID foundation.PlayerID, targetID world.EntityID) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if !runtime.entityVisibleToPlayerLocked(playerID, targetID) {
		return foundation.NewDomainError(foundation.CodeNotFound, "Party target was not found.")
	}
	return nil
}

func (runtime *Runtime) queueSocialEventToPlayerSessionsLocked(playerID foundation.PlayerID, eventType realtime.ClientEventType, payload any) {
	for _, sessionID := range runtime.sessionIDsForPlayerLocked(playerID, "") {
		runtime.queueEventLocked(sessionID, eventType, payload)
	}
}

func (runtime *Runtime) resolveOnlinePlayerByCallsign(callsign string) (foundation.PlayerID, error) {
	trimmed := strings.TrimSpace(callsign)
	if trimmed == "" {
		return "", invalidPayload("Invitee callsign is required.", nil)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	for playerID, state := range runtime.players {
		if strings.EqualFold(state.Callsign, trimmed) && runtime.playerHasActiveSessionLocked(playerID) {
			return playerID, nil
		}
	}
	return "", foundation.NewDomainError(foundation.CodeNotFound, "Invitee is not online.")
}

type runtimeSocialNameResolver struct {
	runtime *Runtime
}

func (resolver runtimeSocialNameResolver) PlayerName(playerID foundation.PlayerID) (social.PlayerName, error) {
	if resolver.runtime == nil {
		return "", foundation.NewDomainError(foundation.CodeInternal, "Runtime is unavailable.")
	}
	resolver.runtime.mu.Lock()
	defer resolver.runtime.mu.Unlock()
	state, ok := resolver.runtime.players[playerID]
	if !ok {
		return "", foundation.NewDomainError(foundation.CodeNotFound, "Player was not found.")
	}
	return social.PlayerName(state.Callsign), nil
}

func socialDomainError(err error) error {
	switch {
	case errors.Is(err, social.ErrInvalidChannelKind),
		errors.Is(err, social.ErrInvalidChannelID),
		errors.Is(err, social.ErrInvalidMessageContent),
		errors.Is(err, social.ErrInvalidPlayerName),
		errors.Is(err, social.ErrInvalidPartyID),
		errors.Is(err, foundation.ErrEmptyID),
		errors.Is(err, foundation.ErrInvalidID):
		return invalidPayload("Social request is invalid.", err)
	case errors.Is(err, social.ErrMessageRateLimited):
		return foundation.NewDomainError(foundation.CodeRateLimited, "Chat message rate limited.", foundation.WithCause(err))
	case errors.Is(err, social.ErrChannelAccessDenied),
		errors.Is(err, social.ErrAlreadyInParty),
		errors.Is(err, social.ErrNotInParty),
		errors.Is(err, social.ErrPartyFull),
		errors.Is(err, social.ErrAlreadyInClan),
		errors.Is(err, social.ErrNotInClan):
		return foundation.NewDomainError(foundation.CodeForbidden, "Social action is not allowed.", foundation.WithCause(err))
	case errors.Is(err, social.ErrPartyNotFound),
		errors.Is(err, social.ErrPartyInviteNotFound),
		errors.Is(err, social.ErrClanNotFound):
		return foundation.NewDomainError(foundation.CodeNotFound, "Social target was not found.", foundation.WithCause(err))
	case errors.Is(err, social.ErrInvalidClanTag),
		errors.Is(err, social.ErrInvalidClanName),
		errors.Is(err, social.ErrInvalidClanRank),
		errors.Is(err, social.ErrInvalidPartyTarget):
		return invalidPayload("Social request is invalid.", err)
	case errors.Is(err, social.ErrClanAlreadyExists):
		return foundation.NewDomainError(foundation.CodeForbidden, "Clan already exists.", foundation.WithCause(err))
	default:
		return domainErrorForRuntime(err)
	}
}

var _ social.PlayerNameResolver = runtimeSocialNameResolver{}

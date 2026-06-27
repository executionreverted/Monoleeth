package server

import (
	"encoding/json"
	"errors"
	"strings"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/social"
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
		errors.Is(err, social.ErrPartyFull):
		return foundation.NewDomainError(foundation.CodeForbidden, "Social action is not allowed.", foundation.WithCause(err))
	case errors.Is(err, social.ErrPartyNotFound),
		errors.Is(err, social.ErrPartyInviteNotFound):
		return foundation.NewDomainError(foundation.CodeNotFound, "Social target was not found.", foundation.WithCause(err))
	default:
		return domainErrorForRuntime(err)
	}
}

var _ social.PlayerNameResolver = runtimeSocialNameResolver{}

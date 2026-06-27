package server

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/social"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

func TestChatSendQueuesLocalMapMembersOnly(t *testing.T) {
	gameServer, _ := newTestServer(t, true)
	sender := createResolvedRuntimeSession(t, gameServer, "chat-sender@example.com", "Chat Sender")
	nearby := createResolvedRuntimeSession(t, gameServer, "chat-nearby@example.com", "Chat Nearby")
	distant := createResolvedRuntimeSessionOnMap(t, gameServer, "chat-distant@example.com", "Chat Distant", worldmaps.MapID("map_1_2"), worldmaps.SpawnID("west_gate"))

	response := handleSocialRequestForTest(t, gameServer, sender, realtime.OperationChatSend, `{"kind":"local_map","content":"hello sector"}`)
	if response.HasError {
		t.Fatalf("chat.send error = %+v, want success", response.Error)
	}
	events := postSocialEventsForTest(t, gameServer, sender, realtime.OperationChatSend)

	if got := countEventTypeForTest(events[sender.SessionID], realtime.EventChatMessage); got != 1 {
		t.Fatalf("sender chat events = %d, want 1", got)
	}
	if got := countEventTypeForTest(events[nearby.SessionID], realtime.EventChatMessage); got != 1 {
		t.Fatalf("nearby chat events = %d, want 1", got)
	}
	if got := countEventTypeForTest(events[distant.SessionID], realtime.EventChatMessage); got != 0 {
		t.Fatalf("distant chat events = %d, want 0", got)
	}
}

func TestChatSendRejectsSpoofedPlayerPayload(t *testing.T) {
	gameServer, _ := newTestServer(t, true)
	sender := createResolvedRuntimeSession(t, gameServer, "chat-spoof@example.com", "Chat Spoof")

	response := handleSocialRequestForTest(t, gameServer, sender, realtime.OperationChatSend, `{"kind":"local_map","content":"hello","player_id":"player-spoof"}`)
	if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("chat.send spoof response = %+v, want %s", response, foundation.CodeInvalidPayload)
	}
}

func TestPartyInviteAcceptAddsMembership(t *testing.T) {
	gameServer, _ := newTestServer(t, true)
	leader := createResolvedRuntimeSession(t, gameServer, "party-leader@example.com", "Party Leader")
	member := createResolvedRuntimeSession(t, gameServer, "party-member@example.com", "Party Member")

	inviteResponse := handleSocialRequestForTest(t, gameServer, leader, realtime.OperationPartyInvite, `{"invitee_callsign":"Party Member"}`)
	if inviteResponse.HasError {
		t.Fatalf("party.invite error = %+v, want success", inviteResponse.Error)
	}
	var invitePayload partyInviteResponse
	decodeSocialResponseForTest(t, inviteResponse, &invitePayload)

	acceptResponse := handleSocialRequestForTest(t, gameServer, member, realtime.OperationPartyAccept, `{"invite_id":"`+invitePayload.Invite.InviteID+`"}`)
	if acceptResponse.HasError {
		t.Fatalf("party.accept error = %+v, want success", acceptResponse.Error)
	}
	var acceptPayload partyResponse
	decodeSocialResponseForTest(t, acceptResponse, &acceptPayload)

	if len(acceptPayload.Party.Members) != 2 {
		t.Fatalf("party members = %d, want 2", len(acceptPayload.Party.Members))
	}
}

func TestPartyLeaveQueuesLeftEventForLeaver(t *testing.T) {
	gameServer, _ := newTestServer(t, true)
	leader := createResolvedRuntimeSession(t, gameServer, "party-leave-leader@example.com", "Leave Leader")
	member := createResolvedRuntimeSession(t, gameServer, "party-leave-member@example.com", "Leave Member")
	inviteResponse := handleSocialRequestForTest(t, gameServer, leader, realtime.OperationPartyInvite, `{"invitee_callsign":"Leave Member"}`)
	var invitePayload partyInviteResponse
	decodeSocialResponseForTest(t, inviteResponse, &invitePayload)
	acceptResponse := handleSocialRequestForTest(t, gameServer, member, realtime.OperationPartyAccept, `{"invite_id":"`+invitePayload.Invite.InviteID+`"}`)
	if acceptResponse.HasError {
		t.Fatalf("party.accept error = %+v, want success", acceptResponse.Error)
	}
	_ = postSocialEventsForTest(t, gameServer, member, realtime.OperationPartyAccept)

	leaveResponse := handleSocialRequestForTest(t, gameServer, member, realtime.OperationPartyLeave, `{}`)
	if leaveResponse.HasError {
		t.Fatalf("party.leave error = %+v, want success", leaveResponse.Error)
	}
	events := postSocialEventsForTest(t, gameServer, member, realtime.OperationPartyLeave)

	if got := countEventTypeForTest(events[member.SessionID], realtime.EventPartyLeft); got != 1 {
		t.Fatalf("leaver party.left events = %d, want 1", got)
	}
}

func TestPartyLeaveQueuesUpdatedPartyForRemainingMember(t *testing.T) {
	gameServer, _ := newTestServer(t, true)
	leader := createResolvedRuntimeSession(t, gameServer, "party-leave-update-leader@example.com", "Update Leader")
	member := createResolvedRuntimeSession(t, gameServer, "party-leave-update-member@example.com", "Update Member")
	inviteResponse := handleSocialRequestForTest(t, gameServer, leader, realtime.OperationPartyInvite, `{"invitee_callsign":"Update Member"}`)
	var invitePayload partyInviteResponse
	decodeSocialResponseForTest(t, inviteResponse, &invitePayload)
	acceptResponse := handleSocialRequestForTest(t, gameServer, member, realtime.OperationPartyAccept, `{"invite_id":"`+invitePayload.Invite.InviteID+`"}`)
	if acceptResponse.HasError {
		t.Fatalf("party.accept error = %+v, want success", acceptResponse.Error)
	}
	_ = postSocialEventsForTest(t, gameServer, member, realtime.OperationPartyAccept)

	leaveResponse := handleSocialRequestForTest(t, gameServer, member, realtime.OperationPartyLeave, `{}`)
	if leaveResponse.HasError {
		t.Fatalf("party.leave error = %+v, want success", leaveResponse.Error)
	}
	events := postSocialEventsForTest(t, gameServer, member, realtime.OperationPartyLeave)

	if got := countEventTypeForTest(events[leader.SessionID], realtime.EventPartyUpdated); got != 1 {
		t.Fatalf("remaining member party.updated events = %d, want 1", got)
	}
}

func TestPartyTargetSetRequiresVisibleEntityAndQueuesUpdate(t *testing.T) {
	gameServer, _ := newTestServer(t, true)
	leader := createResolvedRuntimeSession(t, gameServer, "party-target-leader@example.com", "Target Leader")
	member := createResolvedRuntimeSession(t, gameServer, "party-target-member@example.com", "Target Member")
	inviteResponse := handleSocialRequestForTest(t, gameServer, leader, realtime.OperationPartyInvite, `{"invitee_callsign":"Target Member"}`)
	var invitePayload partyInviteResponse
	decodeSocialResponseForTest(t, inviteResponse, &invitePayload)
	acceptResponse := handleSocialRequestForTest(t, gameServer, member, realtime.OperationPartyAccept, `{"invite_id":"`+invitePayload.Invite.InviteID+`"}`)
	if acceptResponse.HasError {
		t.Fatalf("party.accept error = %+v, want success", acceptResponse.Error)
	}
	_ = postSocialEventsForTest(t, gameServer, member, realtime.OperationPartyAccept)

	targetID := testPlayerEntityID(t, gameServer, member.PlayerID)
	response := handleSocialRequestForTest(t, gameServer, leader, realtime.OperationPartyTargetSet, `{"target_id":"`+targetID.String()+`"}`)
	if response.HasError {
		t.Fatalf("party.target.set error = %+v, want success", response.Error)
	}
	var payload partyTargetSetResponse
	decodeSocialResponseForTest(t, response, &payload)
	if payload.Target.TargetID != targetID.String() || payload.Party.SharedTarget == nil {
		t.Fatalf("shared target payload = %+v, want %q", payload, targetID)
	}
	events := postSocialEventsForTest(t, gameServer, leader, realtime.OperationPartyTargetSet)
	if got := countEventTypeForTest(events[member.SessionID], realtime.EventPartyTargetUpdated); got != 1 {
		t.Fatalf("member party.target_updated events = %d, want 1", got)
	}
}

func TestPartyTargetSetRejectsHiddenEntity(t *testing.T) {
	gameServer, _ := newTestServer(t, true)
	leader := createResolvedRuntimeSession(t, gameServer, "party-target-hidden-leader@example.com", "Hidden Leader")
	member := createResolvedRuntimeSession(t, gameServer, "party-target-hidden-member@example.com", "Hidden Member")
	inviteResponse := handleSocialRequestForTest(t, gameServer, leader, realtime.OperationPartyInvite, `{"invitee_callsign":"Hidden Member"}`)
	var invitePayload partyInviteResponse
	decodeSocialResponseForTest(t, inviteResponse, &invitePayload)
	acceptResponse := handleSocialRequestForTest(t, gameServer, member, realtime.OperationPartyAccept, `{"invite_id":"`+invitePayload.Invite.InviteID+`"}`)
	if acceptResponse.HasError {
		t.Fatalf("party.accept error = %+v, want success", acceptResponse.Error)
	}

	hiddenTargetID := world.EntityID("entity_hidden_party_target")
	insertTestWorldEntity(t, gameServer, hiddenTargetID, world.EntityTypeNPC, world.Vec2{X: 180, Y: 0}, true)
	response := handleSocialRequestForTest(t, gameServer, leader, realtime.OperationPartyTargetSet, `{"target_id":"`+hiddenTargetID.String()+`"}`)
	if !response.HasError || response.Error.Error.Code != foundation.CodeNotFound {
		t.Fatalf("party.target.set hidden response = %+v, want %s", response, foundation.CodeNotFound)
	}
}

func TestClanCreateJoinLeaveQueuesDurableSnapshots(t *testing.T) {
	gameServer, _ := newTestServer(t, true)
	owner := createResolvedRuntimeSession(t, gameServer, "clan-owner@example.com", "Clan Owner")
	member := createResolvedRuntimeSession(t, gameServer, "clan-member@example.com", "Clan Member")

	createResponse := handleSocialRequestForTest(t, gameServer, owner, realtime.OperationClanCreate, `{"name":"Alpha Fleet","tag":"ALF"}`)
	if createResponse.HasError {
		t.Fatalf("clan.create error = %+v, want success", createResponse.Error)
	}
	var createPayload clanResponse
	decodeSocialResponseForTest(t, createResponse, &createPayload)
	if createPayload.Clan.Tag != "ALF" || createPayload.Membership.Rank != social.ClanRankOwner || len(createPayload.Members) != 1 {
		t.Fatalf("create clan payload = %+v, want owner snapshot", createPayload)
	}
	_ = postSocialEventsForTest(t, gameServer, owner, realtime.OperationClanCreate)

	joinResponse := handleSocialRequestForTest(t, gameServer, member, realtime.OperationClanJoin, `{"tag":"alf"}`)
	if joinResponse.HasError {
		t.Fatalf("clan.join error = %+v, want success", joinResponse.Error)
	}
	var joinPayload clanResponse
	decodeSocialResponseForTest(t, joinResponse, &joinPayload)
	if joinPayload.Membership.Rank != social.ClanRankMember || len(joinPayload.Members) != 2 {
		t.Fatalf("join clan payload = %+v, want two durable members", joinPayload)
	}
	events := postSocialEventsForTest(t, gameServer, member, realtime.OperationClanJoin)
	if got := countEventTypeForTest(events[owner.SessionID], realtime.EventClanUpdated); got != 1 {
		t.Fatalf("owner clan.updated events = %d, want 1", got)
	}
	var ownerUpdate clanResponse
	decodeSocialEventForTest(t, events[owner.SessionID], realtime.EventClanUpdated, &ownerUpdate)
	if ownerUpdate.Membership.PlayerID != owner.PlayerID || ownerUpdate.Membership.Rank != social.ClanRankOwner {
		t.Fatalf("owner clan.updated membership = %+v, want owner snapshot", ownerUpdate.Membership)
	}

	leaveResponse := handleSocialRequestForTest(t, gameServer, member, realtime.OperationClanLeave, `{}`)
	if leaveResponse.HasError {
		t.Fatalf("clan.leave error = %+v, want success", leaveResponse.Error)
	}
	events = postSocialEventsForTest(t, gameServer, member, realtime.OperationClanLeave)
	if got := countEventTypeForTest(events[member.SessionID], realtime.EventClanLeft); got != 1 {
		t.Fatalf("member clan.left events = %d, want 1", got)
	}
	if got := countEventTypeForTest(events[owner.SessionID], realtime.EventClanUpdated); got != 1 {
		t.Fatalf("owner clan.updated after leave events = %d, want 1", got)
	}
}

func TestChatSendRateLimitRejectsWithoutQueuedMutation(t *testing.T) {
	gameServer, _ := newTestServer(t, true)
	sender := createResolvedRuntimeSession(t, gameServer, "chat-rate@example.com", "Chat Rate")

	first := handleSocialRequestForTest(t, gameServer, sender, realtime.OperationChatSend, `{"kind":"local_map","content":"first"}`)
	if first.HasError {
		t.Fatalf("first chat.send error = %+v, want success", first.Error)
	}
	_ = postSocialEventsForTest(t, gameServer, sender, realtime.OperationChatSend)

	second := handleSocialRequestForTest(t, gameServer, sender, realtime.OperationChatSend, `{"kind":"local_map","content":"second"}`)
	if !second.HasError || second.Error.Error.Code != foundation.CodeRateLimited {
		t.Fatalf("second chat.send = %+v, want %s", second, foundation.CodeRateLimited)
	}
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	if queued := len(gameServer.runtime.queuedEvents[sender.SessionID]); queued != 0 {
		t.Fatalf("rate-limited chat queued events = %d, want 0", queued)
	}
}

func TestChatSendModerationRejectsWithoutQueuedMutation(t *testing.T) {
	runtime := newRateLimitRuntimeForTest(t, RuntimeConfig{
		SocialModeration: rejectingSocialModeration{},
	})
	sender := createRateLimitRuntimeSession(t, runtime, "chat-moderation@example.com", "Chat Mod")

	response := handleRuntimeSocialRequestForTest(t, runtime, sender, realtime.OperationChatSend, `{"kind":"local_map","content":"blocked"}`)
	if !response.HasError || response.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("moderated chat.send = %+v, want %s", response, foundation.CodeInvalidPayload)
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if queued := len(runtime.queuedEvents[sender.SessionID]); queued != 0 {
		t.Fatalf("moderated chat queued events = %d, want 0", queued)
	}
}

func handleSocialRequestForTest(t *testing.T, gameServer *Server, resolved auth.ResolvedSession, op realtime.Operation, payload string) realtime.CachedResponse {
	t.Helper()
	return handleRuntimeSocialRequestForTest(t, gameServer.runtime, resolved, op, payload)
}

func handleRuntimeSocialRequestForTest(t *testing.T, runtime *Runtime, resolved auth.ResolvedSession, op realtime.Operation, payload string) realtime.CachedResponse {
	t.Helper()
	seq := atomic.AddUint64(&socialRequestSeqForTest, 1)
	requestID := foundation.RequestID(fmt.Sprintf("request-social-%s-%d", resolved.PlayerID.String(), seq))
	switch op {
	case realtime.OperationChatSend:
		requestID = foundation.RequestID(fmt.Sprintf("request-social-chat-%s-%d", resolved.PlayerID.String(), seq))
	case realtime.OperationPartyInvite:
		requestID = foundation.RequestID(fmt.Sprintf("request-social-invite-%s-%d", resolved.PlayerID.String(), seq))
	case realtime.OperationPartyAccept:
		requestID = foundation.RequestID(fmt.Sprintf("request-social-accept-%s-%d", resolved.PlayerID.String(), seq))
	case realtime.OperationPartyLeave:
		requestID = foundation.RequestID(fmt.Sprintf("request-social-leave-%s-%d", resolved.PlayerID.String(), seq))
	case realtime.OperationPartyTargetSet:
		requestID = foundation.RequestID(fmt.Sprintf("request-social-target-%s-%d", resolved.PlayerID.String(), seq))
	case realtime.OperationClanCreate:
		requestID = foundation.RequestID(fmt.Sprintf("request-social-clan-create-%s-%d", resolved.PlayerID.String(), seq))
	case realtime.OperationClanJoin:
		requestID = foundation.RequestID(fmt.Sprintf("request-social-clan-join-%s-%d", resolved.PlayerID.String(), seq))
	case realtime.OperationClanLeave:
		requestID = foundation.RequestID(fmt.Sprintf("request-social-clan-leave-%s-%d", resolved.PlayerID.String(), seq))
	}
	request := realtime.NewRequestEnvelope(requestID, op, json.RawMessage(payload), 1)
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return runtime.Gateway.HandleRequest(realtime.SessionID(resolved.SessionID.String()), data)
}

var socialRequestSeqForTest uint64

func postSocialEventsForTest(t *testing.T, gameServer *Server, resolved auth.ResolvedSession, op realtime.Operation) map[auth.SessionID][]realtime.EventEnvelope {
	t.Helper()
	events, err := gameServer.runtime.postCommandEventsBySession(resolved.SessionID, op, resolved.PlayerID)
	if err != nil {
		t.Fatalf("postCommandEventsBySession(%s) error = %v, want nil", op, err)
	}
	return events
}

func decodeSocialResponseForTest(t *testing.T, response realtime.CachedResponse, target any) {
	t.Helper()
	if response.HasError {
		t.Fatalf("response error = %+v, want success", response.Error)
	}
	if err := json.Unmarshal(response.Response.Payload, target); err != nil {
		t.Fatalf("decode response %s: %v", response.Response.Payload, err)
	}
}

func decodeSocialEventForTest(t *testing.T, events []realtime.EventEnvelope, eventType realtime.ClientEventType, target any) {
	t.Helper()
	for _, event := range events {
		if event.Type != eventType {
			continue
		}
		if err := json.Unmarshal(event.Payload, target); err != nil {
			t.Fatalf("decode event %s payload %s: %v", eventType, event.Payload, err)
		}
		return
	}
	t.Fatalf("event %s not found in %+v", eventType, events)
}

func countEventTypeForTest(events []realtime.EventEnvelope, eventType realtime.ClientEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

type rejectingSocialModeration struct{}

func (rejectingSocialModeration) ModerateMessage(msg social.ChatMessage) (social.ChatMessage, bool, error) {
	return msg, false, nil
}

package social

import (
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

type stubPlayerNameResolver struct {
	names map[foundation.PlayerID]PlayerName
}

func (r stubPlayerNameResolver) PlayerName(id foundation.PlayerID) (PlayerName, error) {
	return r.names[id], nil
}

func TestChatLocalMapReachesSameMapMembersNotOthers(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	clans, _ := NewClanService(ClanServiceConfig{Store: NewInMemoryClanStore(), Clock: clock})
	parties := NewPartyService(clock)
	membership := NewChannelMembershipService(parties, clans)

	p1 := foundation.PlayerID("player-1")
	p2 := foundation.PlayerID("player-2")
	p3 := foundation.PlayerID("player-3")
	membership.SetPlayerMap(p1, "map-alpha")
	membership.SetPlayerMap(p2, "map-alpha")
	membership.SetPlayerMap(p3, "map-beta")

	chat := mustChatService(t, membership, clock)

	result, err := chat.SendMessage(SendChatInput{
		Kind:     ChannelKindLocalMap,
		SenderID: p1,
		Content:  "hello alpha",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if !containsPlayer(result.Members, p1) || !containsPlayer(result.Members, p2) {
		t.Fatalf("members = %v, want p1 and p2 in map-alpha", result.Members)
	}
	if containsPlayer(result.Members, p3) {
		t.Fatal("p3 from map-beta received local-map message")
	}
}

func TestChatOverRateLimitIsThrottledWithoutMutation(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	clans, _ := NewClanService(ClanServiceConfig{Store: NewInMemoryClanStore(), Clock: clock})
	parties := NewPartyService(clock)
	membership := NewChannelMembershipService(parties, clans)
	p1 := foundation.PlayerID("player-rl")
	membership.SetPlayerMap(p1, "map-rl")

	chat := mustChatService(t, membership, clock)

	if _, err := chat.SendMessage(SendChatInput{Kind: ChannelKindLocalMap, SenderID: p1, Content: "first"}); err != nil {
		t.Fatalf("first message error = %v", err)
	}
	if _, err := chat.SendMessage(SendChatInput{Kind: ChannelKindLocalMap, SenderID: p1, Content: "second"}); err == nil {
		t.Fatal("second message within cooldown succeeded, want rate limit error")
	}
}

func TestChatNonMemberCannotReadClanChat(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	clanStore := NewInMemoryClanStore()
	clans, _ := NewClanService(ClanServiceConfig{Store: clanStore, Clock: clock})
	parties := NewPartyService(clock)
	membership := NewChannelMembershipService(parties, clans)

	member := foundation.PlayerID("player-clan-member")
	nonMember := foundation.PlayerID("player-clan-outsider")
	clan, _ := clans.CreateClan(CreateClanInput{OwnerID: member, Name: "Alpha", Tag: "ALPH"})

	membership.SetPlayerMap(nonMember, "map-x")
	chat := mustChatService(t, membership, clock)

	if _, err := chat.SendMessage(SendChatInput{Kind: ChannelKindClan, SenderID: nonMember, Content: "sneak"}); err == nil {
		t.Fatal("non-member sent clan chat, want access denied")
	}

	membership.SetPlayerMap(member, "map-x")
	if _, err := chat.SendMessage(SendChatInput{Kind: ChannelKindClan, SenderID: member, Content: "hello clan"}); err != nil {
		t.Fatalf("member clan chat error = %v", err)
	}
	_ = clan
}

func TestChatMessageTooLongRejected(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	clans, _ := NewClanService(ClanServiceConfig{Store: NewInMemoryClanStore(), Clock: clock})
	parties := NewPartyService(clock)
	membership := NewChannelMembershipService(parties, clans)
	p1 := foundation.PlayerID("player-long")
	membership.SetPlayerMap(p1, "map-long")

	chat := mustChatService(t, membership, clock)

	long := make([]byte, maxMessageLength+1)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := chat.SendMessage(SendChatInput{Kind: ChannelKindLocalMap, SenderID: p1, Content: string(long)}); err == nil {
		t.Fatal("overlong message accepted, want error")
	}
}

func mustChatService(t *testing.T, membership *ChannelMembershipService, clock foundation.Clock) *ChatService {
	t.Helper()
	svc, err := NewChatService(ChatServiceConfig{
		Membership: membership,
		Names:      stubPlayerNameResolver{names: make(map[foundation.PlayerID]PlayerName)},
		Clock:      clock,
	})
	if err != nil {
		t.Fatalf("NewChatService() error = %v", err)
	}
	return svc
}

func containsPlayer(players []foundation.PlayerID, id foundation.PlayerID) bool {
	for _, p := range players {
		if p == id {
			return true
		}
	}
	return false
}

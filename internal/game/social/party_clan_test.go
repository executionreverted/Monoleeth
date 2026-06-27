package social

import (
	"testing"
	"time"

	"gameproject/internal/game/foundation"
)

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func TestPartyInviteAcceptAddsExactlyOneMembership(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	svc := NewPartyService(clock)

	leader := foundation.PlayerID("player-leader")
	member := foundation.PlayerID("player-member")

	party, err := svc.CreateParty(leader)
	if err != nil {
		t.Fatalf("CreateParty() error = %v", err)
	}
	if len(party.Members) != 1 {
		t.Fatalf("initial members = %d, want 1", len(party.Members))
	}

	invite, err := svc.InvitePlayer(leader, member)
	if err != nil {
		t.Fatalf("InvitePlayer() error = %v", err)
	}

	updated, err := svc.AcceptInvite(invite.InviteID, member)
	if err != nil {
		t.Fatalf("AcceptInvite() error = %v", err)
	}
	if len(updated.Members) != 2 {
		t.Fatalf("members after accept = %d, want 2", len(updated.Members))
	}
}

func TestPartyNonLeaderCannotInvite(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	svc := NewPartyService(clock)

	leader := foundation.PlayerID("player-leader")
	member := foundation.PlayerID("player-member")
	other := foundation.PlayerID("player-other")

	svc.CreateParty(leader)
	invite, _ := svc.InvitePlayer(leader, member)
	svc.AcceptInvite(invite.InviteID, member)

	if _, err := svc.InvitePlayer(member, other); err == nil {
		t.Fatal("non-member invite succeeded, want error")
	}
}

func TestPartyLeavePassesLeadership(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	svc := NewPartyService(clock)

	leader := foundation.PlayerID("player-leader")
	member := foundation.PlayerID("player-member")

	svc.CreateParty(leader)
	invite, _ := svc.InvitePlayer(leader, member)
	svc.AcceptInvite(invite.InviteID, member)

	if err := svc.LeaveParty(leader); err != nil {
		t.Fatalf("LeaveParty(leader) error = %v", err)
	}

	party, ok := svc.GetParty(member)
	if !ok {
		t.Fatal("party not found after leader left")
	}
	if len(party.Members) != 1 {
		t.Fatalf("members after leader leave = %d, want 1", len(party.Members))
	}
	if !party.Members[0].IsLeader {
		t.Fatal("remaining member is not leader after promotion")
	}
}

func TestClanCreateAssignsOwnerRankOnce(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	store := NewInMemoryClanStore()
	svc, err := NewClanService(ClanServiceConfig{Store: store, Clock: clock})
	if err != nil {
		t.Fatalf("NewClanService() error = %v", err)
	}

	owner := foundation.PlayerID("player-owner")
	clan, err := svc.CreateClan(CreateClanInput{
		OwnerID: owner,
		Name:    "Test Clan",
		Tag:     "TEST",
	})
	if err != nil {
		t.Fatalf("CreateClan() error = %v", err)
	}

	membership, ok, _ := store.Membership(owner)
	if !ok {
		t.Fatal("owner membership not found")
	}
	if membership.Rank != ClanRankOwner {
		t.Fatalf("owner rank = %q, want %q", membership.Rank, ClanRankOwner)
	}
	if clan.Tag != "TEST" {
		t.Fatalf("clan tag = %q, want TEST", clan.Tag)
	}
}

func TestClanCreateRejectsDuplicateTag(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	store := NewInMemoryClanStore()
	svc, _ := NewClanService(ClanServiceConfig{Store: store, Clock: clock})

	svc.CreateClan(CreateClanInput{
		OwnerID: foundation.PlayerID("player-1"),
		Name:    "First Clan",
		Tag:     "FRST",
	})
	if _, err := svc.CreateClan(CreateClanInput{
		OwnerID: foundation.PlayerID("player-2"),
		Name:    "Second Clan",
		Tag:     "FRST",
	}); err == nil {
		t.Fatal("duplicate tag clan created, want error")
	}
}

func TestClanLeaveRemovesMembershipAndChatAccess(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	store := NewInMemoryClanStore()
	svc, _ := NewClanService(ClanServiceConfig{Store: store, Clock: clock})

	owner := foundation.PlayerID("player-owner")
	clan, _ := svc.CreateClan(CreateClanInput{OwnerID: owner, Name: "Test", Tag: "TEST"})
	member := foundation.PlayerID("player-member")
	svc.JoinClan(clan.ClanID, member)

	membership, ok, _ := store.Membership(member)
	if !ok || membership.Rank != ClanRankMember {
		t.Fatal("member not in clan before leave")
	}

	if err := svc.LeaveClan(member); err != nil {
		t.Fatalf("LeaveClan() error = %v", err)
	}

	if _, ok, _ := store.Membership(member); ok {
		t.Fatal("membership still exists after leave")
	}
}

func TestClanOwnerLeavePassesOwnership(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)}
	store := NewInMemoryClanStore()
	svc, _ := NewClanService(ClanServiceConfig{Store: store, Clock: clock})

	owner := foundation.PlayerID("player-owner")
	clan, _ := svc.CreateClan(CreateClanInput{OwnerID: owner, Name: "Test", Tag: "TEST"})
	member := foundation.PlayerID("player-member")
	svc.JoinClan(clan.ClanID, member)

	if err := svc.LeaveClan(owner); err != nil {
		t.Fatalf("LeaveClan(owner) error = %v", err)
	}

	membership, ok, _ := store.Membership(member)
	if !ok {
		t.Fatal("remaining member not found after owner leave")
	}
	if membership.Rank != ClanRankOwner {
		t.Fatalf("successor rank = %q, want owner", membership.Rank)
	}
}

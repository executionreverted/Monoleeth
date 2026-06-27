package social

import (
	"testing"
	"time"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/world"
)

func TestContributionServiceRecordsPartyAndClanSnapshots(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)}
	parties := NewPartyService(clock)
	clans, _ := NewClanService(ClanServiceConfig{Store: NewInMemoryClanStore(), Clock: clock})
	svc, err := NewContributionService(ContributionServiceConfig{
		Store:   NewInMemoryContributionStore(),
		Parties: parties,
		Clans:   clans,
		Clock:   clock,
	})
	if err != nil {
		t.Fatalf("NewContributionService() error = %v", err)
	}

	leader := foundation.PlayerID("player-leader")
	member := foundation.PlayerID("player-member")
	parties.CreateParty(leader)
	invite, _ := parties.InvitePlayer(leader, member)
	parties.AcceptInvite(invite.InviteID, member)
	clan, _ := clans.CreateClan(CreateClanInput{OwnerID: leader, Name: "Alpha", Tag: "ALPH"})
	clans.JoinClan(clan.ClanID, member)

	snapshots, err := svc.RecordNPCContributions(RecordNPCContributionInput{
		NPCEntityID: world.EntityID("npc-contrib"),
		NPCType:     "training_drone",
		Contributions: map[foundation.PlayerID]float64{
			leader: 7,
			member: 3,
		},
		OccurredAt: clock.Now(),
	})
	if err != nil {
		t.Fatalf("RecordNPCContributions() error = %v", err)
	}

	if got := contributionSnapshotMemberAmount(snapshots, ContributionScopeParty, leader); got != 7 {
		t.Fatalf("party leader contribution = %v, want 7", got)
	}
	if got := contributionSnapshotMemberAmount(snapshots, ContributionScopeClan, member); got != 3 {
		t.Fatalf("clan member contribution = %v, want 3", got)
	}
}

func TestContributionStoreIgnoresDuplicateEvent(t *testing.T) {
	store := NewInMemoryContributionStore()
	event := ContributionEvent{
		EventID:       foundation.EventID("contrib-dup"),
		ScopeKind:     ContributionScopeParty,
		ScopeID:       "party-1",
		SourceKind:    "npc_kill",
		SourceID:      "npc-1",
		ActorPlayerID: foundation.PlayerID("player-1"),
		TargetID:      "npc-1",
		Amount:        5,
		OccurredAt:    time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC),
	}

	if _, err := store.RecordContribution(event); err != nil {
		t.Fatalf("first RecordContribution() error = %v", err)
	}
	snapshot, err := store.RecordContribution(event)
	if err != nil {
		t.Fatalf("duplicate RecordContribution() error = %v", err)
	}
	if len(snapshot.Members) != 1 || snapshot.Members[0].Amount != 5 {
		t.Fatalf("duplicate snapshot = %+v, want one amount 5", snapshot)
	}
}

func TestContributionServiceUsesOpaqueOccurrenceIDsForRespawnedNPC(t *testing.T) {
	clock := fixedClock{t: time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)}
	parties := NewPartyService(clock)
	playerID := foundation.PlayerID("player-respawn")
	parties.CreateParty(playerID)
	svc, err := NewContributionService(ContributionServiceConfig{
		Store:   NewInMemoryContributionStore(),
		Parties: parties,
		Clock:   clock,
	})
	if err != nil {
		t.Fatalf("NewContributionService() error = %v", err)
	}

	first, err := svc.RecordNPCContributions(RecordNPCContributionInput{
		NPCEntityID:   world.EntityID("npc-respawned"),
		NPCType:       "training_drone",
		Contributions: map[foundation.PlayerID]float64{playerID: 5},
		OccurredAt:    clock.Now(),
	})
	if err != nil {
		t.Fatalf("first RecordNPCContributions() error = %v", err)
	}
	second, err := svc.RecordNPCContributions(RecordNPCContributionInput{
		NPCEntityID:   world.EntityID("npc-respawned"),
		NPCType:       "training_drone",
		Contributions: map[foundation.PlayerID]float64{playerID: 4},
		OccurredAt:    clock.Now(),
	})
	if err != nil {
		t.Fatalf("second RecordNPCContributions() error = %v", err)
	}
	if len(first) != 1 || len(second) != 1 || first[0].SourceID == second[0].SourceID {
		t.Fatalf("source ids first=%+v second=%+v, want distinct occurrences", first, second)
	}
	if first[0].SourceID == "npc-respawned" || second[0].SourceID == "npc-respawned" {
		t.Fatalf("source ids leaked npc entity id: first=%q second=%q", first[0].SourceID, second[0].SourceID)
	}
	if second[0].Members[0].Amount != 4 {
		t.Fatalf("second kill contribution = %+v, want 4", second[0].Members)
	}
}

func contributionSnapshotMemberAmount(snapshots []ContributionSnapshot, scope ContributionScopeKind, playerID foundation.PlayerID) float64 {
	for _, snapshot := range snapshots {
		if snapshot.ScopeKind != scope {
			continue
		}
		for _, member := range snapshot.Members {
			if member.PlayerID == playerID {
				return member.Amount
			}
		}
	}
	return 0
}

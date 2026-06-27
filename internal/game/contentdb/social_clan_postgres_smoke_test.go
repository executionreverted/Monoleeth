package contentdb_test

import (
	"context"
	"testing"
	"time"

	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/social"
)

func TestPostgresSocialClanStorePersistsClanMembershipAcrossStoreReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, store := openPostgresSmokeStore(t, ctx)

	if err := store.Migrate(ctx, contentdb.MigrationModeAuto); err != nil {
		t.Fatalf("Migrate(auto) error = %v, want nil", err)
	}
	clanStore, err := contentdb.NewSocialClanStore(store)
	if err != nil {
		t.Fatalf("NewSocialClanStore() error = %v, want nil", err)
	}
	createdAt := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	clan := social.Clan{
		ClanID:    social.ClanID("clan-postgres-social"),
		Name:      "Smoke Fleet",
		Tag:       social.ClanTag("SMK"),
		OwnerID:   foundation.PlayerID("player-postgres-social-owner"),
		CreatedAt: createdAt,
	}
	owner := social.ClanMembership{
		ClanID:   clan.ClanID,
		PlayerID: clan.OwnerID,
		Rank:     social.ClanRankOwner,
		JoinedAt: createdAt,
	}
	if err := clanStore.CreateClan(clan, owner); err != nil {
		t.Fatalf("CreateClan() error = %v, want nil", err)
	}
	member := social.ClanMembership{
		ClanID:   clan.ClanID,
		PlayerID: foundation.PlayerID("player-postgres-social-member"),
		Rank:     social.ClanRankMember,
		JoinedAt: createdAt.Add(time.Minute),
	}
	if err := clanStore.AddMembership(member); err != nil {
		t.Fatalf("AddMembership() error = %v, want nil", err)
	}

	reopened, err := contentdb.NewSocialClanStore(store)
	if err != nil {
		t.Fatalf("NewSocialClanStore(reopen) error = %v, want nil", err)
	}
	storedClan, ok, err := reopened.ClanByTag(clan.Tag)
	if err != nil || !ok {
		t.Fatalf("ClanByTag(reopen) = %+v, %v, %v; want stored clan", storedClan, ok, err)
	}
	members, err := reopened.Memberships(clan.ClanID)
	if err != nil {
		t.Fatalf("Memberships(reopen) error = %v, want nil", err)
	}
	if storedClan.ClanID != clan.ClanID || storedClan.OwnerID != clan.OwnerID || len(members) != 2 {
		t.Fatalf("stored clan/members = %+v / %+v, want durable owner plus member", storedClan, members)
	}
}

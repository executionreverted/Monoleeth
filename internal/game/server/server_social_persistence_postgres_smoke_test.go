package server

import (
	"context"
	"os"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/social"
)

func TestPostgresRuntimeSocialClanPersistsAcrossRestart(t *testing.T) {
	databaseURL := os.Getenv(contentdb.EnvDatabaseURL)
	if databaseURL == "" {
		t.Skipf("%s unset; skipping runtime social clan persistence smoke", contentdb.EnvDatabaseURL)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	schemaURL := createRuntimeAuthSmokeSchema(t, ctx, databaseURL)

	first, err := NewRuntime(RuntimeConfig{
		SessionTTL:        time.Hour,
		WorldID:           foundation.WorldID("world-1"),
		ContentDB:         contentdb.Config{DatabaseURL: schemaURL, Mode: contentdb.ContentModeRequired, Migrations: contentdb.MigrationModeAuto},
		CoreStoreMode:     contentdb.ContentModeRequired,
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)},
		Passwords:         auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("NewRuntime(first) error = %v, want nil", err)
	}
	owner, err := first.Auth.Register(ctx, auth.RegisterInput{Email: "social-owner@example.com", Password: "correct-password", Callsign: "Social Owner"})
	if err != nil {
		t.Fatalf("Register(owner) error = %v, want nil", err)
	}
	clan, err := first.SocialClan.CreateClan(social.CreateClanInput{OwnerID: owner.Session.PlayerID, Name: "Restart Fleet", Tag: social.ClanTag("RST")})
	if err != nil {
		t.Fatalf("CreateClan(first runtime) error = %v, want nil", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close(first runtime) error = %v, want nil", err)
	}

	second, err := NewRuntime(RuntimeConfig{
		SessionTTL:        time.Hour,
		WorldID:           foundation.WorldID("world-1"),
		ContentDB:         contentdb.Config{DatabaseURL: schemaURL, Mode: contentdb.ContentModeRequired, Migrations: contentdb.MigrationModeAuto},
		CoreStoreMode:     contentdb.ContentModeRequired,
		ContentRepository: &fakeRuntimeRepository{bundle: runtimeTestBundleWithLaserDamage(t, 35)},
		Passwords:         auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("NewRuntime(second) error = %v, want nil", err)
	}
	defer second.Close()
	stored, ok, err := second.SocialClan.ClanByTag(social.ClanTag("RST"))
	if err != nil || !ok {
		t.Fatalf("ClanByTag(second runtime) = %+v, %v, %v; want persisted clan", stored, ok, err)
	}
	members, err := second.SocialClan.Memberships(clan.ClanID)
	if err != nil {
		t.Fatalf("Memberships(second runtime) error = %v, want nil", err)
	}
	if stored.ClanID != clan.ClanID || stored.OwnerID != owner.Session.PlayerID || len(members) != 1 || members[0].Rank != social.ClanRankOwner {
		t.Fatalf("stored clan/members = %+v / %+v, want owner persisted once", stored, members)
	}
	loggedIn, err := second.Auth.Login(ctx, auth.LoginInput{Email: "social-owner@example.com", Password: "correct-password"})
	if err != nil {
		t.Fatalf("Login(second runtime) error = %v, want nil", err)
	}
	if err := second.ensurePlayerSession(loggedIn.Session); err != nil {
		t.Fatalf("ensurePlayerSession(second runtime) error = %v, want nil", err)
	}
	events, err := second.bootstrapEvents(loggedIn.Session)
	if err != nil {
		t.Fatalf("bootstrapEvents(second runtime) error = %v, want nil", err)
	}
	if got := countEventTypeForTest(events, realtime.EventClanUpdated); got != 1 {
		t.Fatalf("bootstrap clan.updated events = %d, want 1", got)
	}
}

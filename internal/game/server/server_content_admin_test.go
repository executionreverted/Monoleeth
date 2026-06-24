package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/admin"
	"gameproject/internal/game/auth"
	"gameproject/internal/game/content"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/testutil"
)

func TestAdminContentVersionsRequiresAdminAndReturnsSafeVersionList(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	now := time.Date(2026, 6, 24, 15, 0, 0, 0, time.UTC)
	gameServer.runtime.ContentAdmin = admin.NewContentService(admin.ContentServiceConfig{
		Versions: &fakeServerContentVersionStore{list: content.VersionList{
			Total: 1,
			Versions: []content.VersionSummary{{
				ID:          "11111111-1111-5111-8111-111111111111",
				Version:     "content_mvp_seed_v1",
				Status:      "published",
				Current:     true,
				Notes:       "starter balance",
				BalanceTag:  "starter_balance",
				CreatedAt:   now.Add(-time.Hour),
				PublishedBy: "seed",
				PublishedAt: now.Add(-time.Hour),
			}},
		}},
		Clock: testutil.NewFakeClock(now),
	})

	if _, err := gameServer.runtime.Auth.SeedAdmin(context.Background(), auth.AdminSeedInput{
		Enabled:  true,
		Email:    "cms-admin@example.com",
		Password: "admin-password",
		Callsign: "CMS-Admin",
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	userConn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer userConn.CloseNow()
	readBootstrapEvents(t, userConn)
	writeText(t, userConn, `{"request_id":"request-content-versions-non-admin","op":"admin.content.versions","payload":{},"client_seq":1,"v":1}`)
	nonAdmin := readError(t, userConn)
	if nonAdmin.Error.Code != foundation.CodeForbidden {
		t.Fatalf("non-admin error = %+v, want %s", nonAdmin.Error, foundation.CodeForbidden)
	}

	adminConn := dialWebSocket(t, httpServer, loginPilot(t, httpServer, "cms-admin@example.com", "admin-password"))
	defer adminConn.CloseNow()
	readBootstrapEvents(t, adminConn)
	writeText(t, adminConn, `{"request_id":"request-content-versions-spoof","op":"admin.content.versions","payload":{"actor_account_id":"spoof"},"client_seq":1,"v":1}`)
	spoof := readError(t, adminConn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("actor spoof error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, adminConn, `{"request_id":"request-content-versions-admin","op":"admin.content.versions","payload":{"limit":1},"client_seq":2,"v":1}`)
	response := readResponse(t, adminConn)
	if !response.OK {
		t.Fatalf("admin.content.versions response = %+v, want success", response)
	}
	assertNoPhase09Leak(t, "admin content versions", response.Payload)
	for _, forbidden := range []string{"snapshot_json", "loot_table", "spawn_candidates", "procedural_seed"} {
		if raw := string(response.Payload); strings.Contains(raw, forbidden) {
			t.Fatalf("admin content versions leaked %q in %s", forbidden, raw)
		}
	}
	var payload struct {
		ContentVersions adminContentVersionsPayload `json:"content_versions"`
	}
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode content versions: %v", err)
	}
	if payload.ContentVersions.Total != 1 || payload.ContentVersions.Limit != 1 || payload.ContentVersions.GeneratedAt != now.UnixMilli() {
		t.Fatalf("content versions metadata = %+v, want total/limit/generated", payload.ContentVersions)
	}
	if len(payload.ContentVersions.Versions) != 1 || !payload.ContentVersions.Versions[0].Current ||
		payload.ContentVersions.Versions[0].Version != "content_mvp_seed_v1" {
		t.Fatalf("content versions = %+v, want current seed version", payload.ContentVersions.Versions)
	}
}

type fakeServerContentVersionStore struct {
	list content.VersionList
}

func (store *fakeServerContentVersionStore) ListContentVersions(_ context.Context, input content.VersionListInput) (content.VersionList, error) {
	list := store.list
	list.Limit = input.Limit
	list.Offset = input.Offset
	return list, nil
}

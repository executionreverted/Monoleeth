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
	contentStore := &fakeServerContentStore{
		versionList: content.VersionList{
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
		},
		draftRows: map[content.ContentType][]content.DraftRow{
			content.ContentTypeModule: {
				{
					ContentID:    "laser_alpha_t1",
					DraftVersion: "11111111-1111-5111-8111-111111111111",
					Enabled:      true,
					DisplayJSON:  json.RawMessage(`{"display_name":"Prism Lance I"}`),
					DataJSON:     json.RawMessage(`{"attack_damage":8,"range":650,"cooldown_ms":1200}`),
					UpdatedBy:    "seed",
				},
			},
		},
	}
	gameServer.runtime.ContentAdmin = admin.NewContentService(admin.ContentServiceConfig{
		Versions: contentStore,
		Drafts:   contentStore,
		Clock:    testutil.NewFakeClock(now),
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
	nonAdminRequests := []string{
		`{"request_id":"request-content-versions-non-admin","op":"admin.content.versions","payload":{},"client_seq":1,"v":1}`,
		`{"request_id":"request-content-list-non-admin","op":"admin.content.list","payload":{"content_type":"module"},"client_seq":2,"v":1}`,
		`{"request_id":"request-content-get-non-admin","op":"admin.content.get","payload":{"content_type":"module","content_id":"laser_alpha_t1"},"client_seq":3,"v":1}`,
	}
	for _, body := range nonAdminRequests {
		writeText(t, userConn, body)
		nonAdmin := readError(t, userConn)
		if nonAdmin.Error.Code != foundation.CodeForbidden {
			t.Fatalf("non-admin error = %+v, want %s for %s", nonAdmin.Error, foundation.CodeForbidden, body)
		}
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

	writeText(t, adminConn, `{"request_id":"request-content-list-admin","op":"admin.content.list","payload":{"content_type":"module","limit":1},"client_seq":3,"v":1}`)
	listResponse := readResponse(t, adminConn)
	if !listResponse.OK {
		t.Fatalf("admin.content.list response = %+v, want success", listResponse)
	}
	assertNoPhase09Leak(t, "admin content list", listResponse.Payload)
	var listPayload struct {
		Content adminContentDraftListPayload `json:"content"`
	}
	if err := json.Unmarshal(listResponse.Payload, &listPayload); err != nil {
		t.Fatalf("decode content list: %v", err)
	}
	if listPayload.Content.ContentType != "module" || listPayload.Content.Total != 1 || len(listPayload.Content.Rows) != 1 {
		t.Fatalf("content list = %+v, want one module row", listPayload.Content)
	}
	if row := listPayload.Content.Rows[0]; row.ContentID != "laser_alpha_t1" || !strings.Contains(string(row.DataJSON), "attack_damage") {
		t.Fatalf("content list row = %+v, want LC1-style stats", row)
	}

	writeText(t, adminConn, `{"request_id":"request-content-get-admin","op":"admin.content.get","payload":{"content_type":"module","content_id":"laser_alpha_t1"},"client_seq":4,"v":1}`)
	getResponse := readResponse(t, adminConn)
	if !getResponse.OK {
		t.Fatalf("admin.content.get response = %+v, want success", getResponse)
	}
	var getPayload struct {
		ContentRow adminContentDraftRowPayload `json:"content_row"`
	}
	if err := json.Unmarshal(getResponse.Payload, &getPayload); err != nil {
		t.Fatalf("decode content get: %v", err)
	}
	if getPayload.ContentRow.ContentID != "laser_alpha_t1" || getPayload.ContentRow.ContentType != "module" {
		t.Fatalf("content row = %+v, want laser module", getPayload.ContentRow)
	}

	writeText(t, adminConn, `{"request_id":"request-content-get-missing","op":"admin.content.get","payload":{"content_type":"module","content_id":"missing_module"},"client_seq":5,"v":1}`)
	missing := readError(t, adminConn)
	if missing.Error.Code != foundation.CodeNotFound {
		t.Fatalf("missing content error = %+v, want %s", missing.Error, foundation.CodeNotFound)
	}
}

type fakeServerContentStore struct {
	versionList content.VersionList
	draftRows   map[content.ContentType][]content.DraftRow
}

func (store *fakeServerContentStore) ListContentVersions(_ context.Context, input content.VersionListInput) (content.VersionList, error) {
	list := store.versionList
	list.Limit = input.Limit
	list.Offset = input.Offset
	return list, nil
}

func (store *fakeServerContentStore) LoadDraftRows(_ context.Context, contentType content.ContentType) ([]content.DraftRow, error) {
	return append([]content.DraftRow(nil), store.draftRows[contentType]...), nil
}

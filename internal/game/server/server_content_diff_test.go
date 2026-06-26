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
	"gameproject/internal/game/observability"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
)

// TestAdminContentDiffRequiresAdminAndReportsChangedFields verifies the P09
// lane-F admin.content.diff endpoint is admin-gated and returns safe changed
// fields between the current published snapshot and the draft.
func TestAdminContentDiffRequiresAdminAndReportsChangedFields(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	now := time.Date(2026, 6, 26, 15, 0, 0, 0, time.UTC)
	currentSnapshot := content.SnapshotVersionRecord{
		ID:      "11111111-1111-5111-8111-111111111111",
		Version: "content_mvp_seed_v1",
		Status:  "published",
		Current: true,
		Snapshot: content.Snapshot{
			Version: "content_mvp_seed_v1",
			Modules: []content.SnapshotRow{{
				ContentID: content.ContentID("laser_alpha_t1"),
				Enabled:   true,
				DataJSON:  json.RawMessage(`{"attack_damage":8}`),
			}},
		},
		PublishedAt: now.Add(-time.Hour),
		CreatedAt:   now.Add(-time.Hour),
	}
	contentStore := &fakeServerContentStore{
		currentSnapshot: currentSnapshot,
		draftRows: map[content.ContentType][]content.DraftRow{
			content.ContentTypeModule: {
				{
					ContentID:   "laser_alpha_t1",
					Enabled:     true,
					DataJSON:    json.RawMessage(`{"attack_damage":9,"api_token":"diff-handler-secret"}`),
					DisplayJSON: json.RawMessage(`{"display_name":"Prism Lance II"}`),
				},
			},
		},
	}
	gameServer.runtime.ContentAdmin = admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    contentStore,
		Snapshots: contentStore,
		Clock:     testutil.NewFakeClock(now),
	})

	if _, err := gameServer.runtime.Auth.SeedAdmin(context.Background(), auth.AdminSeedInput{
		Enabled:  true,
		Email:    "diff-admin@example.com",
		Password: "admin-password",
		Callsign: "Diff-Admin",
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	userConn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer userConn.CloseNow()
	readBootstrapEvents(t, userConn)
	writeText(t, userConn, `{"request_id":"request-content-diff-non-admin","op":"admin.content.diff","payload":{},"client_seq":1,"v":1}`)
	nonAdmin := readError(t, userConn)
	if nonAdmin.Error.Code != foundation.CodeForbidden {
		t.Fatalf("non-admin diff error = %+v, want forbidden", nonAdmin.Error)
	}

	adminConn := dialWebSocket(t, httpServer, loginPilot(t, httpServer, "diff-admin@example.com", "admin-password"))
	defer adminConn.CloseNow()
	readBootstrapEvents(t, adminConn)
	writeText(t, adminConn, `{"request_id":"request-content-diff-admin","op":"admin.content.diff","payload":{"target_version_id":"draft"},"client_seq":2,"v":1}`)
	response := readResponse(t, adminConn)
	if !response.OK {
		t.Fatalf("admin.content.diff response = %+v, want success", response)
	}
	raw := string(response.Payload)
	if strings.Contains(raw, "diff-handler-secret") {
		t.Fatalf("diff payload leaked secret value: %s", raw)
	}
	var payload struct {
		ContentDiff adminContentDiffPayload `json:"content_diff"`
	}
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode diff response: %v", err)
	}
	if payload.ContentDiff.Total != 1 {
		t.Fatalf("diff total = %d, want 1 modified module", payload.ContentDiff.Total)
	}
	entry := payload.ContentDiff.Entries[0]
	if entry.ContentType != string(content.ContentTypeModule) ||
		entry.ContentID != "laser_alpha_t1" ||
		entry.Change != content.DiffChangeModified {
		t.Fatalf("diff entry = %+v, want modified laser_alpha_t1", entry)
	}

	diffLog := requireCommandLogEntryForTest(t, gameServer, "request-content-diff-admin", observability.Operation(realtime.OperationAdminContentDiff))
	if diffLog.Operation != observability.Operation(realtime.OperationAdminContentDiff) {
		t.Fatalf("diff command log op = %q, want %s", diffLog.Operation, realtime.OperationAdminContentDiff)
	}
}

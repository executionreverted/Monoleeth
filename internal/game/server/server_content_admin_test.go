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
		`{"request_id":"request-content-update-non-admin","op":"admin.content.update_draft","payload":{"content_type":"module","content_id":"laser_alpha_t1","enabled":true,"data_json":{"damage":9}},"client_seq":4,"v":1}`,
		`{"request_id":"request-content-validate-non-admin","op":"admin.content.validate_draft","payload":{},"client_seq":5,"v":1}`,
		`{"request_id":"request-content-publish-non-admin","op":"admin.content.publish","payload":{"version":"content_balance_v2"},"client_seq":6,"v":1}`,
		`{"request_id":"request-content-rollback-non-admin","op":"admin.content.rollback","payload":{"target_version_id":"11111111-1111-5111-8111-111111111111"},"client_seq":7,"v":1}`,
		`{"request_id":"request-content-audit-non-admin","op":"admin.content.audit_log","payload":{},"client_seq":8,"v":1}`,
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
	assertNoForbiddenLeakCanary(t, "admin content versions", response.Payload)
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
	assertNoForbiddenLeakCanary(t, "admin content list", listResponse.Payload)
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
	assertNoForbiddenLeakCanary(t, "admin content get", getResponse.Payload)
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

func TestAdminContentUpdateDraftAndValidateUseServerActor(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	now := time.Date(2026, 6, 24, 16, 0, 0, 0, time.UTC)
	contentStore := &fakeServerContentStore{
		draftRows: map[content.ContentType][]content.DraftRow{
			content.ContentTypeModule: {
				{
					ContentID:   "laser_alpha_t1",
					Enabled:     true,
					DisplayJSON: json.RawMessage(`{"display_name":"Prism Lance I"}`),
					DataJSON:    json.RawMessage(`{"attack_damage":8}`),
				},
			},
		},
	}
	gameServer.runtime.ContentAdmin = admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    contentStore,
		Writer:    contentStore,
		Validator: contentStore,
		Clock:     testutil.NewFakeClock(now),
	})
	seeded, err := gameServer.runtime.Auth.SeedAdmin(context.Background(), auth.AdminSeedInput{
		Enabled:  true,
		Email:    "cms-editor@example.com",
		Password: "admin-password",
		Callsign: "CMS-Editor",
	})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	adminConn := dialWebSocket(t, httpServer, loginPilot(t, httpServer, "cms-editor@example.com", "admin-password"))
	defer adminConn.CloseNow()
	readBootstrapEvents(t, adminConn)

	writeText(t, adminConn, `{"request_id":"request-content-update-updated-by","op":"admin.content.update_draft","payload":{"content_type":"module","content_id":"laser_alpha_t1","enabled":true,"updated_by":"spoof","data_json":{"attack_damage":9}},"client_seq":1,"v":1}`)
	spoof := readError(t, adminConn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("updated_by spoof error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, adminConn, `{"request_id":"request-content-update-admin","op":"admin.content.update_draft","payload":{"content_type":"module","content_id":"laser_alpha_t1","enabled":true,"display_json":{"display_name":"Prism Lance II"},"data_json":{"attack_damage":9,"damage":9,"cooldown_ms":1100,"map_id":"cms-visible-content-field"}},"client_seq":2,"v":1}`)
	response := readResponse(t, adminConn)
	if !response.OK {
		t.Fatalf("admin.content.update_draft response = %+v, want success", response)
	}
	assertNoForbiddenLeakCanary(t, "admin content update draft", response.Payload)
	var updatePayload struct {
		ContentRow adminContentDraftRowPayload `json:"content_row"`
	}
	if err := json.Unmarshal(response.Payload, &updatePayload); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updatePayload.ContentRow.ContentID != "laser_alpha_t1" ||
		updatePayload.ContentRow.UpdatedBy != string(seeded.AccountID) ||
		!strings.Contains(string(updatePayload.ContentRow.DataJSON), `"damage":9`) {
		t.Fatalf("updated row = %+v, want server actor and accepted nested stat fields", updatePayload.ContentRow)
	}
	if !contentStore.upsertCalled || contentStore.upsertRow.UpdatedBy != string(seeded.AccountID) {
		t.Fatalf("store upsert = called %v row %+v, want server actor", contentStore.upsertCalled, contentStore.upsertRow)
	}

	writeText(t, adminConn, `{"request_id":"request-content-validate-admin","op":"admin.content.validate_draft","payload":{"version":"draft_candidate_v1"},"client_seq":3,"v":1}`)
	validateResponse := readResponse(t, adminConn)
	if !validateResponse.OK {
		t.Fatalf("admin.content.validate_draft response = %+v, want success", validateResponse)
	}
	assertNoForbiddenLeakCanary(t, "admin content validate draft", validateResponse.Payload)
	var validatePayload struct {
		Validation adminContentDraftValidationPayload `json:"validation"`
	}
	if err := json.Unmarshal(validateResponse.Payload, &validatePayload); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if !validatePayload.Validation.Valid || validatePayload.Validation.Version != "draft_candidate_v1" ||
		validatePayload.Validation.CheckedAt != now.UnixMilli() {
		t.Fatalf("validation = %+v, want valid report at fake now", validatePayload.Validation)
	}
	if !contentStore.validateCalled || len(contentStore.validatedSnapshot.Modules) != 1 {
		t.Fatalf("validated snapshot = called %v snapshot %+v, want one module", contentStore.validateCalled, contentStore.validatedSnapshot)
	}

	writeText(t, adminConn, `{"request_id":"request-content-update-invalid-script","op":"admin.content.update_draft","payload":{"content_type":"module","content_id":"laser_alpha_t1","enabled":true,"data_json":{"script":"bad"}},"client_seq":4,"v":1}`)
	invalid := readError(t, adminConn)
	if invalid.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("invalid content field error = %+v, want %s", invalid.Error, foundation.CodeInvalidPayload)
	}
}

func TestAdminContentPublishRollbackAndAuditUseServerActor(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	now := time.Date(2026, 6, 25, 11, 0, 0, 0, time.UTC)
	currentSnapshot := content.Snapshot{
		Version: "content_mvp_seed_v1",
		Modules: []content.SnapshotRow{{
			ContentID:   "laser_alpha_t1",
			Enabled:     true,
			DisplayJSON: json.RawMessage(`{}`),
			DataJSON:    json.RawMessage(`{"attack_damage":8}`),
		}},
	}
	targetSnapshot := content.Snapshot{
		Version: "content_target_v1",
		Modules: []content.SnapshotRow{{
			ContentID:   "laser_alpha_t1",
			Enabled:     true,
			DisplayJSON: json.RawMessage(`{}`),
			DataJSON:    json.RawMessage(`{"attack_damage":7}`),
		}},
	}
	contentStore := &fakeServerContentStore{
		draftRows: map[content.ContentType][]content.DraftRow{
			content.ContentTypeModule: {
				{
					ContentID:   "laser_alpha_t1",
					Enabled:     true,
					DisplayJSON: json.RawMessage(`{}`),
					DataJSON:    json.RawMessage(`{"attack_damage":9}`),
				},
			},
		},
		currentSnapshot: content.SnapshotVersionRecord{
			ID:          "11111111-1111-5111-8111-111111111111",
			Version:     "content_mvp_seed_v1",
			Status:      "published",
			Current:     true,
			Snapshot:    currentSnapshot,
			PublishedAt: now.Add(-time.Hour),
			CreatedAt:   now.Add(-time.Hour),
		},
		targetSnapshots: map[string]content.SnapshotVersionRecord{
			"00000000-0000-5000-8000-000000000001": {
				ID:          "00000000-0000-5000-8000-000000000001",
				Version:     "content_target_v1",
				Status:      "archived",
				Snapshot:    targetSnapshot,
				PublishedAt: now.Add(-2 * time.Hour),
				CreatedAt:   now.Add(-2 * time.Hour),
			},
		},
		auditLog: content.AuditLog{
			Total: 1,
			Entries: []content.AuditLogEntry{{
				ID:               "99999999-9999-5999-8999-999999999999",
				ContentVersionID: "22222222-2222-5222-8222-222222222222",
				ContentType:      content.ContentTypeModule,
				ContentID:        "laser_alpha_t1",
				FieldPath:        "$",
				NewValueJSON:     json.RawMessage(`{"content_id":"laser_alpha_t1","enabled":true}`),
				ActorAccountID:   "account-admin",
				Note:             "LC1 buff",
				BalanceTag:       "starter_balance",
				CreatedAt:        now,
			}},
		},
	}
	gameServer.runtime.ContentAdmin = admin.NewContentService(admin.ContentServiceConfig{
		Drafts:    contentStore,
		Publisher: contentStore,
		Snapshots: contentStore,
		Audit:     contentStore,
		Validator: contentStore,
		Clock:     testutil.NewFakeClock(now),
	})
	seeded, err := gameServer.runtime.Auth.SeedAdmin(context.Background(), auth.AdminSeedInput{
		Enabled:  true,
		Email:    "cms-publisher@example.com",
		Password: "admin-password",
		Callsign: "CMS-Publisher",
	})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	adminConn := dialWebSocket(t, httpServer, loginPilot(t, httpServer, "cms-publisher@example.com", "admin-password"))
	defer adminConn.CloseNow()
	readBootstrapEvents(t, adminConn)

	writeText(t, adminConn, `{"request_id":"request-content-publish-spoof","op":"admin.content.publish","payload":{"version":"content_balance_v2","published_by":"spoof"},"client_seq":1,"v":1}`)
	spoof := readError(t, adminConn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("publish spoof error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, adminConn, `{"request_id":"request-content-publish-admin","op":"admin.content.publish","payload":{"version":"content_balance_v2","notes":"LC1 buff","balance_tag":"starter_balance"},"client_seq":2,"v":1}`)
	publishResponse := readResponse(t, adminConn)
	if !publishResponse.OK {
		t.Fatalf("admin.content.publish response = %+v, want success", publishResponse)
	}
	assertNoForbiddenLeakCanary(t, "admin content publish", publishResponse.Payload)
	if strings.Contains(string(publishResponse.Payload), "actor_account_id") || strings.Contains(string(publishResponse.Payload), "snapshot_json") {
		t.Fatalf("publish payload leaked forbidden fields: %s", publishResponse.Payload)
	}
	var publishPayload struct {
		ContentPublish adminContentPublishPayload `json:"content_publish"`
	}
	if err := json.Unmarshal(publishResponse.Payload, &publishPayload); err != nil {
		t.Fatalf("decode publish response: %v", err)
	}
	if !publishPayload.ContentPublish.Published || !publishPayload.ContentPublish.Validation.Valid || publishPayload.ContentPublish.RowCount != 1 {
		t.Fatalf("publish payload = %+v, want published valid one row", publishPayload.ContentPublish)
	}
	if !contentStore.publishCalled || contentStore.publishedInput.PublishedBy != string(seeded.AccountID) ||
		contentStore.publishedInput.CreatedBy != string(seeded.AccountID) ||
		contentStore.publishedInput.ExpectedCurrentID != "11111111-1111-5111-8111-111111111111" ||
		contentStore.publishedInput.BalanceTag != "starter_balance" {
		t.Fatalf("publish input = called %v input %+v, want server actor", contentStore.publishCalled, contentStore.publishedInput)
	}

	writeText(t, adminConn, `{"request_id":"request-content-rollback-spoof","op":"admin.content.rollback","payload":{"target_version_id":"00000000-0000-5000-8000-000000000001","idempotency_key":"client-picked"},"client_seq":3,"v":1}`)
	rollbackSpoof := readError(t, adminConn)
	if rollbackSpoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("rollback spoof error = %+v, want %s", rollbackSpoof.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, adminConn, `{"request_id":"request-content-rollback-admin","op":"admin.content.rollback","payload":{"target_version_id":"00000000-0000-5000-8000-000000000001","version":"content_rollback_v3","notes":"restore starter"},"client_seq":4,"v":1}`)
	rollbackResponse := readResponse(t, adminConn)
	if !rollbackResponse.OK {
		t.Fatalf("admin.content.rollback response = %+v, want success", rollbackResponse)
	}
	assertNoForbiddenLeakCanary(t, "admin content rollback", rollbackResponse.Payload)
	var rollbackPayload struct {
		ContentRollback adminContentRollbackPayload `json:"content_rollback"`
	}
	if err := json.Unmarshal(rollbackResponse.Payload, &rollbackPayload); err != nil {
		t.Fatalf("decode rollback response: %v", err)
	}
	if !rollbackPayload.ContentRollback.RolledBack ||
		rollbackPayload.ContentRollback.TargetVersionID != "00000000-0000-5000-8000-000000000001" ||
		contentStore.publishedInput.RolledBackFrom != "00000000-0000-5000-8000-000000000001" ||
		contentStore.publishedInput.IdempotencyKey != "content_rollback:00000000-0000-5000-8000-000000000001:request-content-rollback-admin" {
		t.Fatalf("rollback payload = %+v input %+v, want immutable rollback", rollbackPayload.ContentRollback, contentStore.publishedInput)
	}

	writeText(t, adminConn, `{"request_id":"request-content-audit-admin","op":"admin.content.audit_log","payload":{"content_type":"module","limit":1},"client_seq":5,"v":1}`)
	auditResponse := readResponse(t, adminConn)
	if !auditResponse.OK {
		t.Fatalf("admin.content.audit_log response = %+v, want success", auditResponse)
	}
	assertNoForbiddenLeakCanary(t, "admin content audit log", auditResponse.Payload)
	if strings.Contains(string(auditResponse.Payload), "actor_account_id") {
		t.Fatalf("audit payload leaked actor_account_id key: %s", auditResponse.Payload)
	}
	var auditPayload struct {
		ContentAuditLog adminContentAuditLogPayload `json:"content_audit_log"`
	}
	if err := json.Unmarshal(auditResponse.Payload, &auditPayload); err != nil {
		t.Fatalf("decode audit response: %v", err)
	}
	if auditPayload.ContentAuditLog.Total != 1 || len(auditPayload.ContentAuditLog.Entries) != 1 ||
		auditPayload.ContentAuditLog.Entries[0].ActorRef == "" ||
		contentStore.auditInput.ContentType != content.ContentTypeModule {
		t.Fatalf("audit payload = %+v input %+v, want module audit by actor_ref", auditPayload.ContentAuditLog, contentStore.auditInput)
	}
}

type fakeServerContentStore struct {
	versionList       content.VersionList
	draftRows         map[content.ContentType][]content.DraftRow
	upsertCalled      bool
	upsertContentType content.ContentType
	upsertRow         content.DraftRow
	validateCalled    bool
	validatedSnapshot content.Snapshot
	validateErr       error
	currentSnapshot   content.SnapshotVersionRecord
	targetSnapshots   map[string]content.SnapshotVersionRecord
	publishCalled     bool
	publishedInput    content.PublishSnapshotInput
	publishResult     content.PublishSnapshotResult
	auditInput        content.AuditLogInput
	auditLog          content.AuditLog
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

func (store *fakeServerContentStore) UpsertDraftRow(_ context.Context, contentType content.ContentType, row content.DraftRow) error {
	store.upsertCalled = true
	store.upsertContentType = contentType
	store.upsertRow = cloneServerContentDraftRow(row)
	rows := store.draftRows[contentType]
	replaced := false
	for index, existing := range rows {
		if existing.ContentID == row.ContentID {
			rows[index] = cloneServerContentDraftRow(row)
			replaced = true
			break
		}
	}
	if !replaced {
		rows = append(rows, cloneServerContentDraftRow(row))
	}
	if store.draftRows == nil {
		store.draftRows = make(map[content.ContentType][]content.DraftRow)
	}
	store.draftRows[contentType] = rows
	return nil
}

func (store *fakeServerContentStore) ValidateContentSnapshot(_ context.Context, snapshot content.Snapshot) error {
	store.validateCalled = true
	store.validatedSnapshot = snapshot
	return store.validateErr
}

func (store *fakeServerContentStore) LoadCurrentContentSnapshot(context.Context) (content.SnapshotVersionRecord, error) {
	return store.currentSnapshot, nil
}

func (store *fakeServerContentStore) LoadContentSnapshotByID(_ context.Context, id string) (content.SnapshotVersionRecord, error) {
	return store.targetSnapshots[id], nil
}

func (store *fakeServerContentStore) PublishContentSnapshot(_ context.Context, input content.PublishSnapshotInput) (content.PublishSnapshotResult, error) {
	store.publishCalled = true
	store.publishedInput = input
	if store.publishResult.Record.ID == "" {
		store.publishResult.Record = content.SnapshotVersionRecord{
			ID:             input.ID,
			Version:        input.Version,
			Status:         "published",
			Current:        true,
			Notes:          input.Notes,
			BalanceTag:     input.BalanceTag,
			CreatedBy:      input.CreatedBy,
			CreatedAt:      input.PublishedAt,
			PublishedBy:    input.PublishedBy,
			PublishedAt:    input.PublishedAt,
			RolledBackFrom: input.RolledBackFrom,
			Snapshot:       input.Snapshot,
		}
	}
	return store.publishResult, nil
}

func (store *fakeServerContentStore) ListContentAudit(_ context.Context, input content.AuditLogInput) (content.AuditLog, error) {
	store.auditInput = input
	return store.auditLog, nil
}

func cloneServerContentDraftRow(row content.DraftRow) content.DraftRow {
	row.DisplayJSON = append([]byte(nil), row.DisplayJSON...)
	row.DataJSON = append([]byte(nil), row.DataJSON...)
	return row
}

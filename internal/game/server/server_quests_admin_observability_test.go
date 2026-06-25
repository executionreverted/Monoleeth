package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/realtime"
)

func TestPhase09QuestAdminObservabilityUseServerState(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	if _, err := gameServer.runtime.Auth.SeedAdmin(context.Background(), auth.AdminSeedInput{
		Enabled:  true,
		Email:    "admin@example.com",
		Password: "admin-password",
		Callsign: "Ops-Admin",
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)

	writeText(t, conn, `{"request_id":"request-quest-board","op":"quest.board","payload":{},"client_seq":1,"v":1}`)
	boardResponse := readResponse(t, conn)
	if !boardResponse.OK {
		t.Fatalf("quest board response = %+v, want success", boardResponse)
	}
	assertNoPhase09Leak(t, "quest board", boardResponse.Payload)
	var boardPayload struct {
		QuestBoard questBoardSummaryPayload `json:"quest_board"`
	}
	if err := json.Unmarshal(boardResponse.Payload, &boardPayload); err != nil {
		t.Fatalf("decode quest board: %v", err)
	}
	if len(boardPayload.QuestBoard.Offers) != quests.BoardOfferCount || boardPayload.QuestBoard.RerollCost.Amount <= 0 {
		t.Fatalf("quest board = %+v, want ten server offers and reroll cost", boardPayload.QuestBoard)
	}
	if !boardPayload.QuestBoard.CanReroll || boardPayload.QuestBoard.ResetAt <= boardPayload.QuestBoard.GeneratedAt || boardPayload.QuestBoard.Revision <= 0 || !boardPayload.QuestBoard.Offers[0].CanAccept {
		t.Fatalf("quest board action state = %+v first offer %+v, want server-owned reroll/accept/reset state", boardPayload.QuestBoard, boardPayload.QuestBoard.Offers[0])
	}
	for _, offer := range boardPayload.QuestBoard.Offers {
		for _, objective := range offer.Objectives {
			if objective.DisplayName == "" || objective.DisplayName == objective.Target {
				t.Fatalf("quest objective display metadata = %+v, want safe display name separate from raw target", objective)
			}
		}
		for _, reward := range offer.Rewards {
			if reward.DisplayName == "" || reward.DisplayName == reward.ItemID || reward.DisplayName == reward.Currency || reward.DisplayName == reward.Role {
				t.Fatalf("quest reward display metadata = %+v, want safe display name separate from raw ids", reward)
			}
		}
	}
	drainEventTypes(t, conn, realtime.EventQuestBoardGenerated)

	writeText(t, conn, `{"request_id":"request-quest-progress-spoof","op":"quest.progress","payload":{"progress":{"current":999,"completed":true}},"client_seq":2,"v":1}`)
	spoof := readError(t, conn)
	if spoof.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("quest progress spoof error = %+v, want %s", spoof.Error, foundation.CodeInvalidPayload)
	}

	offer := progressableQuestOfferWithItemReward(t, boardPayload.QuestBoard.Offers)
	itemReward := questItemReward(t, offer.Rewards)
	writeText(t, conn, `{"request_id":"request-quest-accept","op":"quest.accept","payload":{"offer_id":"`+offer.OfferID+`"},"client_seq":3,"v":1}`)
	acceptResponse := readResponse(t, conn)
	if !acceptResponse.OK {
		t.Fatalf("quest accept response = %+v, want success", acceptResponse)
	}
	var accepted questMutationPayload
	if err := json.Unmarshal(acceptResponse.Payload, &accepted); err != nil {
		t.Fatalf("decode quest accept: %v", err)
	}
	if accepted.Quest == nil || accepted.Quest.QuestID == "" || accepted.Quest.State != quests.QuestStateAccepted.String() {
		t.Fatalf("accepted quest = %+v, want accepted quest", accepted.Quest)
	}
	if accepted.Quest.AcceptedOfferID != offer.OfferID || accepted.QuestBoard.Counts.Offers != quests.BoardOfferCount-1 {
		t.Fatalf("accepted quest offer reconciliation = quest %+v board counts %+v, want accepted offer removed", accepted.Quest, accepted.QuestBoard.Counts)
	}
	drainEventTypes(t, conn, realtime.EventQuestAccepted)

	completeQuestWithServerEvents(t, gameServer, resolved.PlayerID, *accepted.Quest)
	writeText(t, conn, `{"request_id":"request-quest-progress","op":"quest.progress","payload":{},"client_seq":4,"v":1}`)
	progressResponse := readResponse(t, conn)
	if !progressResponse.OK {
		t.Fatalf("quest progress response = %+v, want success", progressResponse)
	}
	var progressPayload struct {
		QuestBoard questBoardSummaryPayload `json:"quest_board"`
	}
	if err := json.Unmarshal(progressResponse.Payload, &progressPayload); err != nil {
		t.Fatalf("decode quest progress: %v", err)
	}
	if progressPayload.QuestBoard.Counts.Claimable != 1 {
		t.Fatalf("quest counts = %+v, want one claimable quest", progressPayload.QuestBoard.Counts)
	}

	writeText(t, conn, `{"request_id":"request-quest-claim","op":"quest.claim_reward","payload":{"quest_id":"`+accepted.Quest.QuestID+`"},"client_seq":5,"v":1}`)
	claimResponse := readResponse(t, conn)
	if !claimResponse.OK {
		t.Fatalf("quest claim response = %+v, want success", claimResponse)
	}
	var claim questMutationPayload
	if err := json.Unmarshal(claimResponse.Payload, &claim); err != nil {
		t.Fatalf("decode quest claim: %v", err)
	}
	if claim.Quest == nil || claim.Quest.State != quests.QuestStateClaimed.String() || claim.Wallet.Credits <= starterWalletCredits || claim.Progression == nil || claim.Progression.MainXP == 0 {
		t.Fatalf("quest claim = %+v, want claimed quest, credits, and XP", claim)
	}
	if claim.Inventory == nil || len(claim.Inventory.Stackable) == 0 {
		t.Fatalf("quest claim inventory = %+v, want reward item grant", claim.Inventory)
	}
	assertQuestRewardInventorySnapshot(t, *claim.Inventory, itemReward)
	questRewardReference, err := foundation.QuestRewardIdempotencyKey(foundation.QuestID(accepted.Quest.QuestID))
	if err != nil {
		t.Fatalf("quest reward reference: %v", err)
	}
	questRewardLedger := questRewardItemLedgerEntries(gameServer, resolved.PlayerID, questRewardReference)
	if len(questRewardLedger) != 1 {
		t.Fatalf("quest reward item ledger entries = %+v, want one AddItem ledger entry for %s", questRewardLedger, questRewardReference)
	}
	assertQuestRewardLedgerEntry(t, questRewardLedger[0], itemReward, questRewardReference)
	drainEventTypes(t, conn, realtime.EventQuestRewardClaimed, realtime.EventWalletSnapshot, realtime.EventInventorySnapshot, realtime.EventProgressionSnapshot)

	writeText(t, conn, `{"request_id":"request-quest-claim-duplicate","op":"quest.claim_reward","payload":{"quest_id":"`+accepted.Quest.QuestID+`"},"client_seq":6,"v":1}`)
	duplicateClaimResponse := readResponse(t, conn)
	if !duplicateClaimResponse.OK {
		t.Fatalf("duplicate quest claim response = %+v, want success", duplicateClaimResponse)
	}
	var duplicateClaim questMutationPayload
	if err := json.Unmarshal(duplicateClaimResponse.Payload, &duplicateClaim); err != nil {
		t.Fatalf("decode duplicate quest claim: %v", err)
	}
	if !duplicateClaim.Duplicate || duplicateClaim.Quest == nil || duplicateClaim.Quest.State != quests.QuestStateClaimed.String() {
		t.Fatalf("duplicate quest claim = %+v, want duplicate claimed result", duplicateClaim)
	}
	if got := questRewardItemLedgerEntries(gameServer, resolved.PlayerID, questRewardReference); len(got) != len(questRewardLedger) {
		t.Fatalf("duplicate quest claim ledger entries = %+v, want unchanged from %+v", got, questRewardLedger)
	}
	drainEventTypes(t, conn, realtime.EventQuestRewardClaimed, realtime.EventWalletSnapshot, realtime.EventInventorySnapshot, realtime.EventProgressionSnapshot)

	writeText(t, conn, `{"request_id":"request-quest-reroll","op":"quest.reroll","payload":{},"client_seq":7,"v":1}`)
	rerollResponse := readResponse(t, conn)
	if !rerollResponse.OK {
		t.Fatalf("quest reroll response = %+v, want success", rerollResponse)
	}
	var reroll questMutationPayload
	if err := json.Unmarshal(rerollResponse.Payload, &reroll); err != nil {
		t.Fatalf("decode quest reroll: %v", err)
	}
	if len(reroll.QuestBoard.Offers) != quests.BoardOfferCount || reroll.Wallet.Credits >= claim.Wallet.Credits {
		t.Fatalf("quest reroll = %+v, want fresh board and wallet debit", reroll)
	}
	drainEventTypes(t, conn, realtime.EventQuestBoardRerolled, realtime.EventWalletSnapshot)

	for _, request := range []struct {
		name string
		body string
	}{
		{"admin inspect", `{"request_id":"request-non-admin-inspect","op":"admin.inspect_player","payload":{},"client_seq":8,"v":1}`},
		{"admin inspect target", fmt.Sprintf(`{"request_id":"request-non-admin-inspect-target","op":"admin.inspect_player","payload":{"target_player_id":"%s"},"client_seq":9,"v":1}`, resolved.PlayerID.String())},
		{"admin repair", `{"request_id":"request-non-admin-repair","op":"admin.repair_craft_job","payload":{"job_id":"job-missing"},"client_seq":10,"v":1}`},
		{"command log", `{"request_id":"request-non-admin-log","op":"observability.command_log","payload":{},"client_seq":11,"v":1}`},
		{"metrics", `{"request_id":"request-non-admin-metrics","op":"observability.metrics","payload":{},"client_seq":12,"v":1}`},
		{"release gate", `{"request_id":"request-non-admin-gate","op":"observability.release_gate","payload":{},"client_seq":13,"v":1}`},
		{"abuse coverage", `{"request_id":"request-non-admin-abuse","op":"observability.abuse_coverage","payload":{},"client_seq":14,"v":1}`},
	} {
		t.Run("non-admin "+request.name, func(t *testing.T) {
			writeText(t, conn, request.body)
			got := readError(t, conn)
			if got.Error.Code != foundation.CodeForbidden {
				t.Fatalf("%s error = %+v, want %s", request.name, got.Error, foundation.CodeForbidden)
			}
		})
	}

	adminCookie := loginPilot(t, httpServer, "admin@example.com", "admin-password")
	adminConn := dialWebSocket(t, httpServer, adminCookie)
	defer adminConn.CloseNow()
	readBootstrapEvents(t, adminConn)

	adminRequests := []struct {
		name       string
		body       string
		decode     func(*testing.T, json.RawMessage)
		eventTypes []realtime.ClientEventType
	}{
		{
			name: "inspect",
			body: `{"request_id":"request-admin-inspect","op":"admin.inspect_player","payload":{},"client_seq":1,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Admin adminPlayerInspectionPayload `json:"admin"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode admin inspect: %v", err)
				}
				if payload.Admin.Target != "self" || len(payload.Admin.Wallet.Balances) == 0 {
					t.Fatalf("admin inspect = %+v, want self wallet balances", payload.Admin)
				}
			},
		},
		{
			name: "inspect target",
			body: fmt.Sprintf(`{"request_id":"request-admin-inspect-target","op":"admin.inspect_player","payload":{"target_player_id":"%s"},"client_seq":2,"v":1}`, resolved.PlayerID.String()),
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Admin adminPlayerInspectionPayload `json:"admin"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode admin inspect target: %v", err)
				}
				if payload.Admin.Target != "requested" || len(payload.Admin.Wallet.Balances) == 0 {
					t.Fatalf("admin inspect target = %+v, want requested wallet balances", payload.Admin)
				}
			},
		},
		{
			name: "economy",
			body: `{"request_id":"request-admin-economy-ok","op":"admin.economy_dashboard","payload":{},"client_seq":3,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Economy economyDashboardPayload `json:"economy"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode admin economy: %v", err)
				}
				if payload.Economy.Wallets.Credits == 0 {
					t.Fatalf("admin economy = %+v, want wallet totals", payload.Economy)
				}
			},
		},
		{
			name: "repair",
			body: `{"request_id":"request-admin-repair","op":"admin.repair_craft_job","payload":{"job_id":"job-missing"},"client_seq":4,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Repair adminRepairCraftJobPayload `json:"admin_repair"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode admin repair: %v", err)
				}
				if payload.Repair.Status != "unavailable" {
					t.Fatalf("admin repair = %+v, want unavailable runtime status", payload.Repair)
				}
			},
			eventTypes: []realtime.ClientEventType{realtime.EventAdminActionCompleted},
		},
		{
			name: "command log",
			body: `{"request_id":"request-admin-command-log","op":"observability.command_log","payload":{},"client_seq":5,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					CommandLog commandLogSummaryPayload `json:"command_log"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode command log: %v", err)
				}
				if payload.CommandLog.Total == 0 || len(payload.CommandLog.Entries) == 0 {
					t.Fatalf("command log = %+v, want recorded commands", payload.CommandLog)
				}
			},
		},
		{
			name: "metrics",
			body: `{"request_id":"request-admin-metrics","op":"observability.metrics","payload":{},"client_seq":6,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Metrics metricsSummaryPayload `json:"metrics"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode metrics: %v", err)
				}
				if len(payload.Metrics.Snapshot.Counters) == 0 {
					t.Fatalf("metrics = %+v, want command counters", payload.Metrics)
				}
			},
			eventTypes: []realtime.ClientEventType{realtime.EventObservabilityMetric},
		},
		{
			name: "release gate",
			body: `{"request_id":"request-admin-release-gate","op":"observability.release_gate","payload":{},"client_seq":7,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					ReleaseGate releaseGatePayload `json:"release_gate"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode release gate: %v", err)
				}
				if !payload.ReleaseGate.Report.Passed || len(payload.ReleaseGate.Coverage) == 0 {
					t.Fatalf("release gate = %+v, want passing coverage", payload.ReleaseGate.Report)
				}
			},
			eventTypes: []realtime.ClientEventType{realtime.EventReleaseGateUpdated},
		},
		{
			name: "abuse",
			body: `{"request_id":"request-admin-abuse","op":"observability.abuse_coverage","payload":{},"client_seq":8,"v":1}`,
			decode: func(t *testing.T, raw json.RawMessage) {
				var payload struct {
					Abuse abuseCoveragePayload `json:"abuse_coverage"`
				}
				if err := json.Unmarshal(raw, &payload); err != nil {
					t.Fatalf("decode abuse coverage: %v", err)
				}
				if !payload.Abuse.Report.Passed || len(payload.Abuse.Coverage) == 0 {
					t.Fatalf("abuse coverage = %+v, want passing coverage", payload.Abuse.Report)
				}
			},
		},
	}

	for _, request := range adminRequests {
		t.Run("admin "+request.name, func(t *testing.T) {
			writeText(t, adminConn, request.body)
			response := readResponse(t, adminConn)
			if !response.OK {
				t.Fatalf("%s response = %+v, want success", request.name, response)
			}
			if request.name == "command log" {
				assertCommandLogHasOperationalFieldsNoSecrets(t, request.name, response.Payload)
			} else {
				assertNoPhase09Leak(t, request.name, response.Payload)
			}
			if request.name != "metrics" {
				assertNoForbiddenLeakCanary(t, request.name, response.Payload)
			}
			request.decode(t, response.Payload)
			if len(request.eventTypes) > 0 {
				drainEventTypes(t, adminConn, request.eventTypes...)
			}
		})
	}

	snapshot := gameServer.runtime.Metrics.Snapshot()
	requireMetricCounter(t, snapshot, observability.MetricQuestRewardsClaimed, 1, []observability.Label{
		{Name: "reward_type", Value: quests.RewardKindCredits.String()},
	})
	requireMetricCounter(t, snapshot, observability.MetricQuestRewardsClaimed, 1, []observability.Label{
		{Name: "reward_type", Value: quests.RewardKindMainXP.String()},
	})
	requireMetricCounter(t, snapshot, observability.MetricQuestRewardsClaimed, 1, []observability.Label{
		{Name: "reward_type", Value: quests.RewardKindItem.String()},
	})
	requireMetricCounter(t, snapshot, observability.MetricWalletDeltaByReason, claim.Wallet.Credits-starterWalletCredits, []observability.Label{
		{Name: "action", Value: economy.LedgerActionIncrease.String()},
		{Name: "currency_type", Value: economy.CurrencyBucketCredits.String()},
		{Name: "reason", Value: runtimeQuestRewardLedgerReason.String()},
	})
}

func TestCommandLogLeakCanaryOmitsRejectedPayloadInternals(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()

	if _, err := gameServer.runtime.Auth.SeedAdmin(context.Background(), auth.AdminSeedInput{
		Enabled:  true,
		Email:    "log-admin@example.com",
		Password: "admin-password",
		Callsign: "Log-Admin",
	}); err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	adminConn := dialWebSocket(t, httpServer, loginPilot(t, httpServer, "log-admin@example.com", "admin-password"))
	defer adminConn.CloseNow()
	readBootstrapEvents(t, adminConn)

	writeText(t, adminConn, `{"request_id":"request-log-canary-source","op":"move_to","payload":{"target":{"x":10,"y":20},"canary_note":"HIDDEN_RUNTIME_METADATA_SENTINEL map_1_2 procedural_seed loot_roll"},"client_seq":1,"v":1}`)
	rejected := readError(t, adminConn)
	if rejected.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("canary command error = %+v, want %s", rejected.Error, foundation.CodeInvalidPayload)
	}

	writeText(t, adminConn, `{"request_id":"request-command-log-canary","op":"observability.command_log","payload":{},"client_seq":2,"v":1}`)
	response := readResponse(t, adminConn)
	if !response.OK {
		t.Fatalf("command log response = %+v, want success", response)
	}
	assertNoForbiddenLeakCanary(t, "command log", response.Payload)

	var payload struct {
		CommandLog commandLogSummaryPayload `json:"command_log"`
	}
	if err := json.Unmarshal(response.Payload, &payload); err != nil {
		t.Fatalf("decode command log: %v", err)
	}
	foundRejectedCommand := false
	for _, entry := range payload.CommandLog.Entries {
		if entry.RequestID == "request-log-canary-source" {
			foundRejectedCommand = true
			break
		}
	}
	if !foundRejectedCommand {
		t.Fatalf("command log entries = %+v, want rejected canary command", payload.CommandLog.Entries)
	}
}

func assertCommandLogHasOperationalFieldsNoSecrets(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{
		"account_id",
		"password",
		"password_hash",
		"session_token",
		"token",
		"cookie",
		"hash",
		"provider_reference",
		"reference_id",
		"generated_payload",
		"generated_seed",
		"reward_payload",
		"rare_cap",
		"world_seed",
		"gameplay_seed",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}

	var decoded struct {
		CommandLog commandLogSummaryPayload `json:"command_log"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode %s command log: %v", label, err)
	}
	for _, entry := range decoded.CommandLog.Entries {
		if entry.PlayerID == "" || entry.SessionID == "" || entry.RequestID == "" || entry.Operation == "" || entry.Result == "" {
			t.Fatalf("%s command log entry missing operational identity/result fields: %+v", label, entry)
		}
		if entry.DurationMS < 0 {
			t.Fatalf("%s command log entry duration_ms = %d, want non-negative", label, entry.DurationMS)
		}
	}
}

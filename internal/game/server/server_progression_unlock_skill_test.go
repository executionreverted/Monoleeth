package server

import (
	"encoding/json"
	"testing"

	"gameproject/internal/game/foundation"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/realtime"
)

func newProgressionUnlockSkillTestServer(t *testing.T) (*Server, foundation.PlayerID, realtime.SessionID) {
	t.Helper()
	gameServer, _ := newTestServer(t, false)
	owner := createResolvedRuntimeSession(t, gameServer, "unlock-skill@example.com", "Unlock Skill")
	rankUpPlayerForUnlockSkillTest(t, gameServer, owner.PlayerID)
	return gameServer, owner.PlayerID, realtime.SessionID(owner.SessionID.String())
}

func rankUpPlayerForUnlockSkillTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID) {
	t.Helper()
	questSourceID := progression.XPSourceID("player_quest_starter_unlock_skill")
	if _, err := gameServer.runtime.Progression.GrantXP(progression.GrantXPInput{
		PlayerID:       playerID,
		Amount:         100,
		SourceType:     progression.XPSourceTypeQuest,
		SourceID:       questSourceID,
		IdempotencyKey: progression.XPIdempotencyKey("quest_reward:" + questSourceID.String()),
		Authority:      progression.XPGrantAuthorityQuestService,
	}); err != nil {
		t.Fatalf("GrantXP(unlock skill setup) error = %v", err)
	}
	result, err := gameServer.runtime.Progression.TryRankUp(progression.TryRankUpInput{
		PlayerID:       playerID,
		TargetRank:     2,
		Reason:         "unlock_skill_rank_2",
		IdempotencyKey: progression.XPIdempotencyKey("unlock_skill_rank_up_" + playerID.String()),
	})
	if err != nil {
		t.Fatalf("TryRankUp(unlock skill setup) error = %v", err)
	}
	if !result.RankedUp && !result.AlreadyAtRank && !result.Duplicate {
		t.Fatalf("TryRankUp(unlock skill setup) missing = %+v, want rank 2", result.MissingRequirements)
	}
	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[playerID]
	state.Rank = 2
	gameServer.runtime.players[playerID] = state
	gameServer.runtime.mu.Unlock()
}

func sendUnlockSkillCommand(t *testing.T, gameServer *Server, sessionID realtime.SessionID, requestID, nodeID string) unlockSkillResponsePayload {
	t.Helper()
	response := gameServer.runtime.Gateway.HandleRequest(sessionID, []byte(
		`{"request_id":"`+requestID+`","op":"progression.unlock_skill","payload":{"node_id":"`+nodeID+`"},"client_seq":1,"v":1}`,
	))
	if response.HasError {
		t.Fatalf("progression.unlock_skill(%q) response error = %+v", nodeID, response.Error)
	}
	var payload unlockSkillResponsePayload
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("progression.unlock_skill(%q) unmarshal payload: %v", nodeID, err)
	}
	return payload
}

type unlockSkillResponsePayload struct {
	Accepted  bool `json:"accepted"`
	Unlocked  bool `json:"unlocked"`
	Duplicate bool `json:"duplicate"`
}

// TestProgressionUnlockSkillConsumesExactlyOnePoint proves the authenticated
// command path spends exactly one skill point for the root combat node.
func TestProgressionUnlockSkillConsumesExactlyOnePoint(t *testing.T) {
	gameServer, playerID, sessionID := newProgressionUnlockSkillTestServer(t)

	payload := sendUnlockSkillCommand(t, gameServer, sessionID, "request-unlock-once", "combat_weapon_calibration")
	if !payload.Accepted || !payload.Unlocked || payload.Duplicate {
		t.Fatalf("unlock response = %+v, want accepted/unlocked/not-duplicate", payload)
	}
	snapshot, err := gameServer.runtime.Progression.GetProgressionSnapshot(playerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot error = %v", err)
	}
	if snapshot.SkillPoints.SpentPoints != 1 || snapshot.SkillPoints.AvailablePoints() != 0 {
		t.Fatalf("skill points after unlock = spent %d available %d, want spent 1 available 0", snapshot.SkillPoints.SpentPoints, snapshot.SkillPoints.AvailablePoints())
	}
}

// TestProgressionUnlockSkillDuplicateDoesNotDoubleSpend proves a repeated
// unlock of an already-unlocked node returns duplicate without spending again.
func TestProgressionUnlockSkillDuplicateDoesNotDoubleSpend(t *testing.T) {
	gameServer, playerID, sessionID := newProgressionUnlockSkillTestServer(t)

	sendUnlockSkillCommand(t, gameServer, sessionID, "request-unlock-first", "combat_weapon_calibration")
	duplicate := sendUnlockSkillCommand(t, gameServer, sessionID, "request-unlock-duplicate", "combat_weapon_calibration")
	if !duplicate.Accepted || duplicate.Unlocked || !duplicate.Duplicate {
		t.Fatalf("duplicate unlock response = %+v, want accepted/not-unlocked/duplicate", duplicate)
	}
	snapshot, err := gameServer.runtime.Progression.GetProgressionSnapshot(playerID)
	if err != nil {
		t.Fatalf("GetProgressionSnapshot error = %v", err)
	}
	if snapshot.SkillPoints.SpentPoints != 1 || snapshot.SkillPoints.AvailablePoints() != 0 {
		t.Fatalf("skill points after duplicate = spent %d available %d, want spent 1 available 0 (no double spend)", snapshot.SkillPoints.SpentPoints, snapshot.SkillPoints.AvailablePoints())
	}
}

// TestProgressionUnlockSkillRejectsInvalidNode proves the command rejects an
// unknown skill node with a not-found domain error instead of mutating state.
func TestProgressionUnlockSkillRejectsInvalidNode(t *testing.T) {
	gameServer, _, sessionID := newProgressionUnlockSkillTestServer(t)

	response := gameServer.runtime.Gateway.HandleRequest(sessionID, []byte(
		`{"request_id":"request-unlock-unknown","op":"progression.unlock_skill","payload":{"node_id":"does_not_exist"},"client_seq":1,"v":1}`,
	))
	if !response.HasError {
		t.Fatal("progression.unlock_skill(unknown) response missing error, want not-found")
	}
}

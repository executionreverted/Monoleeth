package server

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	deathdomain "gameproject/internal/game/death"
	"gameproject/internal/game/economy"
	gameevents "gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/worker"
)

func TestCombatKillCreatesLootAndPickupUpdatesCargo(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-combat-1","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
	response := readResponse(t, conn)
	if !response.OK {
		t.Fatalf("combat response = %+v, want success", response)
	}
	var combatPayload struct {
		Accepted bool `json:"accepted"`
		Killed   bool `json:"killed"`
		Amount   int  `json:"amount"`
		Ship     struct {
			Capacitor int `json:"capacitor"`
		} `json:"ship"`
	}
	if err := json.Unmarshal(response.Payload, &combatPayload); err != nil {
		t.Fatalf("decode combat response: %v", err)
	}
	if !combatPayload.Accepted || !combatPayload.Killed || combatPayload.Amount <= 0 || combatPayload.Ship.Capacitor >= 100 {
		t.Fatalf("combat payload = %+v, want accepted killed with energy spent", combatPayload)
	}

	var dropID string
	seen := map[realtime.ClientEventType]bool{}
	for attempts := 0; attempts < 12 && (dropID == "" || !seen[realtime.EventAOIEntityEntered] || !seen[realtime.EventAOIEntityLeft]); attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
		raw := string(event.Payload)
		for _, forbidden := range []string{"player_id", "damage", "loot_table", "gameplay_seed"} {
			if strings.Contains(raw, forbidden) {
				t.Fatalf("combat event leaked %q in %s", forbidden, raw)
			}
		}
		if event.Type == realtime.EventLootCreated {
			var payload struct {
				DropID   string `json:"drop_id"`
				EntityID string `json:"entity_id"`
				ItemID   string `json:"item_id"`
				Quantity int64  `json:"quantity"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode loot.created: %v", err)
			}
			if payload.DropID == "" || payload.EntityID != payload.DropID || payload.ItemID != "raw_ore" || payload.Quantity != 3 {
				t.Fatalf("loot.created payload = %+v, want raw ore drop", payload)
			}
			dropID = payload.DropID
		}
	}
	for _, want := range []realtime.ClientEventType{
		realtime.EventCombatDamage,
		realtime.EventCombatCooldownStarted,
		realtime.EventTargetUpdated,
		realtime.EventCombatNPCKilled,
		realtime.EventProgressionSnapshot,
		realtime.EventLootCreated,
	} {
		if !seen[want] {
			t.Fatalf("combat events seen = %#v, missing %s", seen, want)
		}
	}

	request := `{"request_id":"request-loot-1","op":"loot.pickup","payload":{"drop_id":"` + dropID + `"},"client_seq":2,"v":1}`
	writeText(t, conn, request)
	pickup := readResponse(t, conn)
	if !pickup.OK {
		t.Fatalf("pickup response = %+v, want success", pickup)
	}
	var pickupPayload struct {
		Accepted bool `json:"accepted"`
		Cargo    struct {
			Used     int64 `json:"used"`
			Capacity int64 `json:"capacity"`
			Items    []struct {
				ItemID       string `json:"item_id"`
				DisplayName  string `json:"display_name"`
				Category     string `json:"category"`
				ArtKey       string `json:"art_key"`
				Quantity     int64  `json:"quantity"`
				UnitWeight   int64  `json:"unit_weight"`
				UsedUnits    int64  `json:"used_units"`
				Location     string `json:"location"`
				MoveEligible bool   `json:"move_eligible"`
				LockedReason string `json:"locked_reason"`
			} `json:"items"`
		} `json:"cargo"`
	}
	if err := json.Unmarshal(pickup.Payload, &pickupPayload); err != nil {
		t.Fatalf("decode pickup response: %v", err)
	}
	if !pickupPayload.Accepted || pickupPayload.Cargo.Used != 6 || len(pickupPayload.Cargo.Items) != 1 || pickupPayload.Cargo.Items[0].Quantity != 3 {
		t.Fatalf("pickup payload = %+v, want cargo with three raw ore", pickupPayload)
	}
	rawOreCargo := pickupPayload.Cargo.Items[0]
	if rawOreCargo.ItemID != "raw_ore" ||
		rawOreCargo.DisplayName != "Raw Ore" ||
		rawOreCargo.Category != "resource" ||
		rawOreCargo.ArtKey != "item.raw_ore" ||
		rawOreCargo.UnitWeight != 2 ||
		rawOreCargo.UsedUnits != 6 ||
		rawOreCargo.Location != economy.LocationKindShipCargo.String() ||
		rawOreCargo.MoveEligible ||
		rawOreCargo.LockedReason != "cargo_transfer_unavailable" {
		t.Fatalf("cargo metadata = %+v, want server-owned raw ore metadata and move locked", rawOreCargo)
	}

	seen = map[realtime.ClientEventType]bool{}
	for attempts := 0; attempts < 8 && !seen[realtime.EventAOIEntityLeft]; attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
	}
	for _, want := range []realtime.ClientEventType{realtime.EventLootPickedUp, realtime.EventLootRemoved, realtime.EventCargoSnapshot, realtime.EventProgressionSnapshot, realtime.EventAOIEntityLeft} {
		if !seen[want] {
			t.Fatalf("pickup events seen = %#v, missing %s", seen, want)
		}
	}
	writeText(t, conn, request)
	duplicatePickup := readResponse(t, conn)
	if !bytes.Equal(duplicatePickup.Payload, pickup.Payload) {
		t.Fatalf("duplicate pickup payload changed:\nfirst=%s\nsecond=%s", pickup.Payload, duplicatePickup.Payload)
	}
}
func TestCombatRejectsClientAuthoredGameplayTruth(t *testing.T) {
	_, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	conn := dialWebSocket(t, httpServer, registerPilot(t, httpServer))
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)

	writeText(t, conn, `{"request_id":"request-combat-spoof","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc","player_id":"spoofed-player","damage":9999},"client_seq":1,"v":1}`)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("spoofed combat error = %+v, want %s", got.Error, foundation.CodeInvalidPayload)
	}
}
func TestCombatRejectsHiddenOutOfRangeAndDisabledWithoutEnergySpend(t *testing.T) {
	t.Run("hidden target", func(t *testing.T) {
		gameServer, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		cookie := registerPilot(t, httpServer)
		conn := dialWebSocket(t, httpServer, cookie)
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)
		resolved := resolvedSessionForCookie(t, gameServer, cookie)
		setTestHidden(gameServer, "entity_training_npc", true)

		writeText(t, conn, `{"request_id":"request-combat-hidden","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeNotVisible {
			t.Fatalf("hidden combat error = %+v, want %s", got.Error, foundation.CodeNotVisible)
		}
		if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != 100 {
			t.Fatalf("hidden combat capacitor = %d, want unchanged", capacitor)
		}
	})

	t.Run("out of range", func(t *testing.T) {
		gameServer, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		cookie := registerPilot(t, httpServer)
		conn := dialWebSocket(t, httpServer, cookie)
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)
		resolved := resolvedSessionForCookie(t, gameServer, cookie)
		setTestWeaponRange(gameServer, resolved.PlayerID, 10)

		writeText(t, conn, `{"request_id":"request-combat-range","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeOutOfRange {
			t.Fatalf("out-of-range combat error = %+v, want %s", got.Error, foundation.CodeOutOfRange)
		}
		if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != 100 {
			t.Fatalf("out-of-range combat capacitor = %d, want unchanged", capacitor)
		}
	})

	t.Run("disabled ship", func(t *testing.T) {
		gameServer, httpServer := newTestServer(t, false)
		defer httpServer.Close()
		cookie := registerPilot(t, httpServer)
		conn := dialWebSocket(t, httpServer, cookie)
		defer conn.CloseNow()
		readBootstrapEvents(t, conn)
		resolved := resolvedSessionForCookie(t, gameServer, cookie)
		setTestShipDisabled(gameServer, resolved.PlayerID, true)

		writeText(t, conn, `{"request_id":"request-combat-disabled","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
		got := readError(t, conn)
		if got.Error.Code != foundation.CodeShipDisabled {
			t.Fatalf("disabled combat error = %+v, want %s", got.Error, foundation.CodeShipDisabled)
		}
		if capacitor := testShipCapacitor(gameServer, resolved.PlayerID); capacitor != 100 {
			t.Fatalf("disabled combat capacitor = %d, want unchanged", capacitor)
		}
	})
}
func TestLootPickupRejectsOutOfRangeDropWithoutCargoMutation(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	dropID := killTrainingNPCForDrop(t, conn)
	moveTestPlayerEntity(gameServer, resolved.PlayerID, world.Vec2{X: 1000, Y: 0})
	setTestRadarRange(gameServer, resolved.PlayerID, 2000)

	writeText(t, conn, `{"request_id":"request-loot-far","op":"loot.pickup","payload":{"drop_id":"`+dropID+`"},"client_seq":2,"v":1}`)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeOutOfRange {
		t.Fatalf("out-of-range pickup error = %+v, want %s", got.Error, foundation.CodeOutOfRange)
	}
	if used := testCargoUsed(gameServer, resolved.PlayerID); used != 0 {
		t.Fatalf("out-of-range pickup cargo used = %d, want unchanged", used)
	}
}
func TestRepairQuoteAndRepairUseServerOwnedActiveShip(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	setTestShipDisabled(gameServer, resolved.PlayerID, true)

	writeText(t, conn, `{"request_id":"request-repair-quote","op":"death.repair_quote","payload":{},"client_seq":1,"v":1}`)
	quoteResponse := readResponse(t, conn)
	if !quoteResponse.OK {
		t.Fatalf("repair quote response = %+v, want success", quoteResponse)
	}
	var quote repairQuotePayload
	if err := json.Unmarshal(quoteResponse.Payload, &quote); err != nil {
		t.Fatalf("decode quote: %v", err)
	}
	if !quote.Disabled || quote.ShipID != starterShipID.String() || quote.Cost != 0 {
		t.Fatalf("repair quote = %+v, want disabled free starter repair", quote)
	}

	writeText(t, conn, `{"request_id":"request-repair-ship","op":"death.repair_ship","payload":{},"client_seq":2,"v":1}`)
	repairResponse := readResponse(t, conn)
	if !repairResponse.OK {
		t.Fatalf("repair response = %+v, want success", repairResponse)
	}
	var repaired struct {
		Repaired bool `json:"repaired"`
		Ship     struct {
			Disabled bool `json:"disabled"`
			Hull     int  `json:"hull"`
		} `json:"ship"`
	}
	if err := json.Unmarshal(repairResponse.Payload, &repaired); err != nil {
		t.Fatalf("decode repair response: %v", err)
	}
	if !repaired.Repaired || repaired.Ship.Disabled || repaired.Ship.Hull != 100 {
		t.Fatalf("repair payload = %+v, want restored ship", repaired)
	}
	seen := map[realtime.ClientEventType]bool{}
	for attempts := 0; attempts < 6 &&
		(!seen[realtime.EventDeathRepaired] ||
			!seen[realtime.EventShipSnapshot] ||
			!seen[realtime.EventPlayerSnapshot] ||
			!seen[realtime.EventWalletSnapshot]); attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
	}
	for _, want := range []realtime.ClientEventType{realtime.EventDeathRepaired, realtime.EventShipSnapshot, realtime.EventPlayerSnapshot, realtime.EventWalletSnapshot} {
		if !seen[want] {
			t.Fatalf("repair events seen = %#v, missing %s", seen, want)
		}
	}
}
func TestShipDisabledDomainEventQueuesClientSafeRealtimeEvents(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "death-bridge@example.com", "Death-Bridge")

	intent, err := world.NewMovementIntent(world.Vec2{X: 400, Y: 0})
	if err != nil {
		t.Fatalf("movement intent: %v", err)
	}
	gameServer.runtime.mu.Lock()
	if err := gameServer.runtime.Worker.Submit(worker.MoveToCommand{PlayerID: resolved.PlayerID, Intent: intent}); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("submit move: %v", err)
	}
	if err := commandErrors(gameServer.runtime.Worker.Tick()); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("tick move: %v", err)
	}
	before, ok := gameServer.runtime.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()
	if !ok || !before.Movement.Moving {
		t.Fatalf("player movement before death = %+v ok=%v, want moving", before.Movement, ok)
	}

	lethalKey, err := deathdomain.NewLethalEventKey(foundation.EventID("lethal-runtime-1"))
	if err != nil {
		t.Fatalf("lethal key: %v", err)
	}
	disabledAt := time.Unix(1_720_000_000, 0).UTC()
	domainPayload := deathdomain.ShipDisabledEvent{
		DeathID:           foundation.EventID("death-runtime-1"),
		LethalEventKey:    lethalKey,
		PlayerID:          resolved.PlayerID,
		ShipID:            starterShipID,
		DisabledReason:    "death",
		DisabledAt:        disabledAt,
		RespawnLocationID: deathdomain.RespawnLocationID("origin-station"),
	}
	raw, err := json.Marshal(domainPayload)
	if err != nil {
		t.Fatalf("marshal domain payload: %v", err)
	}

	gameServer.runtime.Record(gameevents.NewEventEnvelope(
		foundation.EventID("domain-ship-disabled-1"),
		deathdomain.EventShipDisabled,
		raw,
		disabledAt.UnixMilli(),
		1,
	))

	gameServer.runtime.mu.Lock()
	queued := gameServer.runtime.drainQueuedEventsLocked(resolved.SessionID)
	state := gameServer.runtime.players[resolved.PlayerID]
	actor, actorOK := gameServer.runtime.Combat.Actor(state.EntityID)
	after, entityOK := gameServer.runtime.Worker.PlayerEntity(resolved.PlayerID)
	gameServer.runtime.mu.Unlock()

	if !state.Ship.Disabled || state.Ship.RepairState != "disabled" || state.Ship.Hull != 0 || state.Ship.Shield != 0 || state.Ship.Capacitor != 0 {
		t.Fatalf("runtime ship after domain event = %+v, want disabled depleted active ship", state.Ship)
	}
	if !actorOK || !actor.Dead || actor.HP != 0 {
		t.Fatalf("combat actor after domain event = %+v ok=%v, want dead actor", actor, actorOK)
	}
	if !entityOK || after.Movement.Moving {
		t.Fatalf("player movement after domain event = %+v ok=%v, want stopped", after.Movement, entityOK)
	}

	seen := map[realtime.ClientEventType]realtime.EventEnvelope{}
	for _, event := range queued {
		seen[event.Type] = event
		rawEvent := string(mustJSON(t, event))
		for _, forbidden := range []string{
			resolved.PlayerID.String(),
			resolved.SessionID.String(),
			"player_id",
			"session_id",
			"death_id",
			"lethal_event_key",
			"respawn_location_id",
		} {
			if strings.Contains(rawEvent, forbidden) {
				t.Fatalf("death bridge event %s leaked %q in %s", event.Type, forbidden, rawEvent)
			}
		}
	}
	for _, want := range []realtime.ClientEventType{
		realtime.EventDeathShipDisabled,
		realtime.EventShipSnapshot,
		realtime.EventPlayerSnapshot,
		realtime.EventMovementStopped,
	} {
		if _, ok := seen[want]; !ok {
			t.Fatalf("queued death bridge events = %#v, missing %s", seen, want)
		}
	}

	var publicDisabled struct {
		ShipID         string              `json:"ship_id"`
		DisabledReason string              `json:"disabled_reason"`
		Ship           shipSnapshotPayload `json:"ship"`
		RepairQuote    repairQuotePayload  `json:"repair_quote"`
	}
	if err := json.Unmarshal(seen[realtime.EventDeathShipDisabled].Payload, &publicDisabled); err != nil {
		t.Fatalf("decode public death disabled event: %v", err)
	}
	if publicDisabled.ShipID != starterShipID.String() ||
		publicDisabled.DisabledReason != "death" ||
		publicDisabled.Ship.ActiveShipID != starterShipID.String() ||
		!publicDisabled.Ship.Disabled ||
		publicDisabled.Ship.RepairState != "disabled" ||
		publicDisabled.Ship.Hull != 0 ||
		publicDisabled.Ship.Shield != 0 ||
		publicDisabled.Ship.Capacitor != 0 ||
		publicDisabled.RepairQuote.ShipID != starterShipID.String() ||
		!publicDisabled.RepairQuote.Disabled {
		t.Fatalf("public death disabled payload = %+v, want client-safe disabled ship state", publicDisabled)
	}
}

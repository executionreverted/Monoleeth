package server

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	deathdomain "gameproject/internal/game/death"
	"gameproject/internal/game/economy"
	gameevents "gameproject/internal/game/events"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/worker"
)

func TestCombatKillCreatesLootAndPickupUpdatesCargo(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	primeTrainingNPCForOneShot(t, gameServer)
	gameServer.runtime.tickAndCollectAOIEvents()

	writeText(t, conn, `{"request_id":"request-combat-1","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
	response := readResponseSkippingEvents(t, conn)
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
	func() {
		gameServer.runtime.mu.Lock()
		defer gameServer.runtime.mu.Unlock()

		instance, _, err := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
		if err != nil {
			t.Fatalf("active map instance: %v", err)
		}
		targetID := world.EntityID("entity_training_npc")
		record, ok := instance.Worker.EnemySpawnRecord(targetID)
		if !ok {
			t.Fatalf("starter spawner missing %q after kill; snapshot=%+v", targetID, instance.Worker.EnemySpawnSnapshot())
		}
		if record.Alive || record.DeadAt.IsZero() || !record.NextRespawnAt.Equal(record.DeadAt.Add(instance.Definition.EnemyPools[0].KillRespawnDelay)) {
			t.Fatalf("starter spawn record after kill = %+v, want dead with respawn timing", record)
		}
		snapshot := instance.Worker.EnemySpawnSnapshot()
		if snapshot.MapAliveCount != 0 || snapshot.PoolAliveCounts[record.EnemyPoolID] != 0 {
			t.Fatalf("starter alive counts after kill = pool %+v map %d, want zero", snapshot.PoolAliveCounts, snapshot.MapAliveCount)
		}
		if _, ok := instance.Worker.Entity(targetID); ok {
			t.Fatalf("target entity %q still present in worker after kill", targetID)
		}
		if !instance.HiddenEntities[targetID] {
			t.Fatalf("target entity %q not hidden after worker accepted death", targetID)
		}
	}()

	var dropID string
	seen := map[realtime.ClientEventType]bool{}
	for attempts := 0; attempts < 12 && dropID == ""; attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
		raw := string(event.Payload)
		for _, forbidden := range []string{"player_id", "damage", "loot_table", "drop_profile", trainingDroneSalvageLootTableID, "gameplay_seed"} {
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

	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, world.EntityID(dropID), world.Vec2{})
	request := `{"request_id":"request-loot-1","op":"loot.pickup","payload":{"drop_id":"` + dropID + `"},"client_seq":2,"v":1}`
	writeText(t, conn, request)
	pickup := readResponseSkippingEvents(t, conn)
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
	for attempts := 0; attempts < 8 && !pickupEventsComplete(seen); attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
	}
	for _, want := range []realtime.ClientEventType{realtime.EventLootPickedUp, realtime.EventLootRemoved, realtime.EventCargoSnapshot, realtime.EventInventorySnapshot, realtime.EventProgressionSnapshot} {
		if !seen[want] {
			t.Fatalf("pickup events seen = %#v, missing %s", seen, want)
		}
	}
	writeText(t, conn, request)
	duplicatePickup := readResponseSkippingEvents(t, conn)
	if !bytes.Equal(duplicatePickup.Payload, pickup.Payload) {
		t.Fatalf("duplicate pickup payload changed:\nfirst=%s\nsecond=%s", pickup.Payload, duplicatePickup.Payload)
	}
}

func pickupEventsComplete(seen map[realtime.ClientEventType]bool) bool {
	return seen[realtime.EventLootPickedUp] &&
		seen[realtime.EventLootRemoved] &&
		seen[realtime.EventCargoSnapshot] &&
		seen[realtime.EventInventorySnapshot] &&
		seen[realtime.EventProgressionSnapshot]
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
		moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{X: -700})
		setTestRadarRange(gameServer, resolved.PlayerID, 1000)

		writeText(t, conn, `{"request_id":"request-combat-range","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
		got := readErrorSkippingEvents(t, conn)
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
	equipStarterLaserForTest(t, gameServer, resolved.PlayerID)
	moveTestPlayerNearEntity(t, gameServer, resolved.PlayerID, "entity_training_npc", world.Vec2{})
	gameServer.runtime.tickAndCollectAOIEvents()
	dropID := killTrainingNPCForDrop(t, gameServer, conn)
	moveTestPlayerEntity(gameServer, resolved.PlayerID, world.Vec2{X: 1000, Y: 0})
	setTestRadarRange(gameServer, resolved.PlayerID, 2000)

	writeText(t, conn, `{"request_id":"request-loot-far","op":"loot.pickup","payload":{"drop_id":"`+dropID+`"},"client_seq":2,"v":1}`)
	got := readErrorSkippingEvents(t, conn)
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

	quote := requestRepairQuoteForTest(t, conn, "request-repair-quote", 1)
	if !quote.Disabled || quote.ShipID != starterShipID.String() || quote.Cost != 0 {
		t.Fatalf("repair quote = %+v, want disabled free starter repair", quote)
	}

	writeDeathRepairShipForTest(t, conn, "request-repair-ship", 2, quote)
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

func TestDeathRepairShipRejectsStaleRepairQuote(t *testing.T) {
	gameServer, httpServer, clock := newTestServerWithFakeClock(t)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	setTestShipDisabled(gameServer, resolved.PlayerID, true)

	quote := requestRepairQuoteForTest(t, conn, "request-repair-stale-quote", 1)
	clock.Advance(repairQuoteTTL + time.Millisecond)
	writeDeathRepairShipForTest(t, conn, "request-repair-stale-ship", 2, quote)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("stale repair quote error = %+v, want %s", got.Error, foundation.CodeInvalidPayload)
	}
	state := testPlayerState(t, gameServer, resolved.PlayerID)
	if !state.Ship.Disabled || state.Ship.RepairState != "disabled" {
		t.Fatalf("state after stale quote = %+v, want disabled", state.Ship)
	}
}

func TestDeathRepairShipRejectsTamperedRepairQuote(t *testing.T) {
	gameServer, httpServer := newTestServer(t, false)
	defer httpServer.Close()
	cookie := registerPilot(t, httpServer)
	conn := dialWebSocket(t, httpServer, cookie)
	defer conn.CloseNow()
	readBootstrapEvents(t, conn)
	resolved := resolvedSessionForCookie(t, gameServer, cookie)
	setTestShipDisabled(gameServer, resolved.PlayerID, true)

	quote := requestRepairQuoteForTest(t, conn, "request-repair-tamper-quote", 1)
	quote.Cost++
	writeDeathRepairShipForTest(t, conn, "request-repair-tamper-ship", 2, quote)
	got := readError(t, conn)
	if got.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("tampered repair quote error = %+v, want %s", got.Error, foundation.CodeInvalidPayload)
	}
	state := testPlayerState(t, gameServer, resolved.PlayerID)
	if !state.Ship.Disabled || state.Ship.RepairState != "disabled" {
		t.Fatalf("state after tampered quote = %+v, want disabled", state.Ship)
	}
}

func requestRepairQuoteForTest(t *testing.T, conn *websocket.Conn, requestID string, clientSeq int) repairQuotePayload {
	t.Helper()
	envelope := map[string]any{
		"request_id": requestID,
		"op":         realtime.OperationDeathRepairQuote,
		"payload":    map[string]any{},
		"client_seq": clientSeq,
		"v":          1,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode repair quote request: %v", err)
	}
	writeText(t, conn, string(raw))
	quoteResponse := readResponse(t, conn)
	if !quoteResponse.OK {
		t.Fatalf("repair quote response = %+v, want success", quoteResponse)
	}
	var quote repairQuotePayload
	if err := json.Unmarshal(quoteResponse.Payload, &quote); err != nil {
		t.Fatalf("decode quote: %v", err)
	}
	assertRepairQuoteBoundForTest(t, quote)
	return quote
}

func writeDeathRepairShipForTest(t *testing.T, conn *websocket.Conn, requestID string, clientSeq int, quote repairQuotePayload) {
	t.Helper()
	envelope := map[string]any{
		"request_id": requestID,
		"op":         realtime.OperationDeathRepairShip,
		"payload":    quote,
		"client_seq": clientSeq,
		"v":          1,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode repair request: %v", err)
	}
	writeText(t, conn, string(raw))
}

func repairShipPayloadForTest(t *testing.T, quote repairQuotePayload) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(quote)
	if err != nil {
		t.Fatalf("encode repair payload: %v", err)
	}
	return raw
}

func issueRepairQuoteForTest(t *testing.T, gameServer *Server, playerID foundation.PlayerID) repairQuotePayload {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state, ok := gameServer.runtime.players[playerID]
	if !ok {
		t.Fatalf("player %q missing runtime state", playerID)
	}
	quote := gameServer.runtime.issueRepairQuoteLocked(playerID, state)
	assertRepairQuoteBoundForTest(t, quote)
	return quote
}

func assertRepairQuoteBoundForTest(t *testing.T, quote repairQuotePayload) {
	t.Helper()
	if quote.QuoteID == "" || quote.IssuedAtMS <= 0 || quote.ExpiresAtMS <= quote.IssuedAtMS {
		t.Fatalf("repair quote = %+v, want token and server expiry", quote)
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
		DeathID:           foundation.EventID("death-secret-canary-runtime-1"),
		LethalEventKey:    lethalKey,
		PlayerID:          resolved.PlayerID,
		ShipID:            starterShipID,
		DisabledReason:    "attacker-secret-reason",
		DisabledAt:        disabledAt,
		RespawnLocationID: deathdomain.RespawnLocationID("hidden-respawn-canary"),
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
			"hidden-respawn-canary",
			"attacker-secret",
			"source_stack_id",
			"loot_drop_id",
			"killer_entity_id",
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

	publicDisabled := assertDeathShipDisabledPayloadClientSafeForTest(
		t,
		seen[realtime.EventDeathShipDisabled],
		resolved.PlayerID.String(),
		resolved.SessionID.String(),
		"hidden-respawn-canary",
		"attacker-secret",
	)
	if publicDisabled.ShipID != starterShipID.String() ||
		publicDisabled.DisabledReason != "death" ||
		publicDisabled.RepairQuote.ShipID != starterShipID.String() ||
		!publicDisabled.RepairQuote.Disabled {
		t.Fatalf("public death disabled payload = %+v, want client-safe disabled ship state", publicDisabled)
	}
}

func TestPVPDeathQueuesClientSafeShipDisabledEvent(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	attacker := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-disabled-safe-attacker@example.com", "PVP Disabled Safe Attacker", seededPVPMapID, "west_gate")
	target := createResolvedRuntimeSessionOnMap(t, gameServer, "pvp-disabled-safe-target@example.com", "PVP Disabled Safe Target", seededPVPMapID, "west_gate")
	moveTestPlayerEntity(gameServer, attacker.PlayerID, world.Vec2{X: 500, Y: 500})
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 520, Y: 500})
	setTestPlayerShipCombatValues(t, gameServer, target.PlayerID, 1, 0, 100)
	addTestCargoStack(t, gameServer, target.PlayerID, "raw_ore", 3, "pvp-disabled-safe-hidden-cargo-stack")
	attackerEntityID := testPlayerEntityID(t, gameServer, attacker.PlayerID)

	attack := performLethalPVPAttackForTest(t, gameServer, attacker, target, foundation.RequestID("request-pvp-disabled-safe"))
	if len(attack.Drops) != 1 {
		t.Fatalf("pvp death drops = %+v, want one cargo-backed drop", attack.Drops)
	}

	gameServer.runtime.mu.Lock()
	targetEvents := gameServer.runtime.drainQueuedEventsLocked(target.SessionID)
	gameServer.runtime.mu.Unlock()

	disabledEvent := requireEventTypeForTest(t, targetEvents, realtime.EventDeathShipDisabled)
	publicDisabled := assertDeathShipDisabledPayloadClientSafeForTest(
		t,
		disabledEvent,
		attacker.PlayerID.String(),
		attacker.SessionID.String(),
		attackerEntityID.String(),
		target.PlayerID.String(),
		target.SessionID.String(),
		"raw_ore",
		"pvp-disabled-safe-hidden-cargo-stack",
		attack.Drops[0].DropID,
		attack.Drops[0].EntityID,
	)
	if publicDisabled.ShipID != starterShipID.String() ||
		publicDisabled.DisabledReason != "death" ||
		publicDisabled.RepairQuote.ShipID != starterShipID.String() ||
		!publicDisabled.RepairQuote.Disabled {
		t.Fatalf("pvp death disabled payload = %+v, want public disabled starter ship", publicDisabled)
	}
}

func assertDeathShipDisabledPayloadClientSafeForTest(
	t *testing.T,
	event realtime.EventEnvelope,
	forbiddenValues ...string,
) deathShipDisabledPayload {
	t.Helper()
	if event.Type != realtime.EventDeathShipDisabled {
		t.Fatalf("event type = %s, want %s", event.Type, realtime.EventDeathShipDisabled)
	}

	var keys map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &keys); err != nil {
		t.Fatalf("decode death.ship_disabled keys: %v", err)
	}
	expectedKeys := map[string]struct{}{
		"ship_id":         {},
		"disabled_reason": {},
		"ship":            {},
		"repair_quote":    {},
	}
	for key := range keys {
		if _, ok := expectedKeys[key]; !ok {
			t.Fatalf("death.ship_disabled key %q not client-safe in %s", key, string(event.Payload))
		}
	}
	for key := range expectedKeys {
		if _, ok := keys[key]; !ok {
			t.Fatalf("death.ship_disabled payload missing %q in %s", key, string(event.Payload))
		}
	}

	raw := string(event.Payload)
	for _, forbidden := range append([]string{
		"player_id",
		"session_id",
		"death_id",
		"lethal_event_key",
		"player_death:",
		"respawn_location_id",
		"killer_entity_id",
		"attacker",
		"cargo",
		"drop",
		"loot",
		"owner_player_id",
		"source_stack_id",
		"item_instance_id",
		"disabled_at",
		"stat_invalidation",
	}, forbiddenValues...) {
		if forbidden == "" {
			continue
		}
		if strings.Contains(raw, forbidden) {
			t.Fatalf("death.ship_disabled leaked %q in %s", forbidden, raw)
		}
	}

	var payload deathShipDisabledPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode death.ship_disabled payload: %v", err)
	}
	return payload
}

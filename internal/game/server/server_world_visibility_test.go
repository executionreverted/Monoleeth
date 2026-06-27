package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/testutil"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

func TestAOIDiffEventsAreFilteredPerSession(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "aoi-filter@example.com", "AOI-Filter")

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	_ = decodeWorldSnapshotForTest(t, events)
	insertTestWorldEntity(t, gameServer, "entity_projection_hidden", world.EntityTypeNPC, world.Vec2{X: 180, Y: 0}, true)
	insertTestWorldEntity(t, gameServer, "entity_projection_visible", world.EntityTypeNPC, world.Vec2{X: 200, Y: 0}, false)

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	sessionEvents := eventsBySession[resolved.SessionID]
	if len(sessionEvents) == 0 || sessionEvents[0].Type != realtime.EventAOIEntityEntered {
		t.Fatalf("AOI events after visible entity insert = %+v, want entity entered", sessionEvents)
	}
	var entered aoiEntityPayloadForTest
	if err := json.Unmarshal(sessionEvents[0].Payload, &entered); err != nil {
		t.Fatalf("decode entered event: %v", err)
	}
	if entered.EntityID != "entity_projection_visible" || entered.EntityType != "npc" {
		t.Fatalf("entered entity = %+v, want visible projection npc", entered)
	}
	for _, event := range sessionEvents {
		if strings.Contains(string(event.Payload), "entity_projection_hidden") {
			t.Fatalf("hidden entity leaked into AOI event %+v", event)
		}
	}
}

func requireMetricDuration(t *testing.T, snapshot observability.MetricSnapshot, name string, count int64, labels []observability.Label) {
	t.Helper()
	for _, duration := range snapshot.Durations {
		if duration.Name != name || !sameMetricLabels(duration.Labels, labels) {
			continue
		}
		if duration.Count != count {
			t.Fatalf("metric %s labels %+v count = %d, want %d", name, labels, duration.Count, count)
		}
		return
	}
	t.Fatalf("missing duration metric %s labels %+v in snapshot %+v", name, labels, snapshot)
}

func TestAOIDiffSkipsFailedWorkerTickInstance(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "aoi-worker-error@example.com", "AOI-Worker-Error")

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	_ = decodeWorldSnapshotForTest(t, events)
	insertTestWorldEntity(t, gameServer, "entity_tick_error_visible", world.EntityTypeNPC, world.Vec2{X: 200, Y: 0}, false)

	gameServer.runtime.mu.Lock()
	instance, _, err := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("active map instance: %v", err)
	}
	if err := instance.Worker.Submit(worker.RemoveEntityCommand{EntityID: "entity_missing_for_aoi_tick"}); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("Submit(bad remove) error = %v, want nil", err)
	}
	gameServer.runtime.mu.Unlock()

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	if got := eventsBySession[resolved.SessionID]; len(got) != 0 {
		t.Fatalf("events after failed worker tick = %+v, want none for failed instance", got)
	}
	gameServer.runtime.mu.Lock()
	lastAOI := instance.LastAOI[resolved.SessionID]
	gameServer.runtime.mu.Unlock()
	if hasEntityID(lastAOI.Entities, "entity_tick_error_visible") {
		t.Fatalf("LastAOI advanced after failed worker tick: %+v", lastAOI.Entities)
	}
	requireMetricCounter(t, gameServer.runtime.Metrics.Snapshot(), observability.MetricErrorsByCode, 1, []observability.Label{
		{Name: "code", Value: foundation.CodeInternal.String()},
		{Name: "op", Value: "runtime_aoi_tick"},
		{Name: "stage", Value: "worker_tick"},
		{Name: "world_id", Value: "world-1"},
		{Name: "zone_id", Value: "map_1_1"},
	})

	eventsBySession = gameServer.runtime.tickAndCollectAOIEvents()
	sessionEvents := eventsBySession[resolved.SessionID]
	if len(sessionEvents) == 0 || sessionEvents[0].Type != realtime.EventAOIEntityEntered {
		t.Fatalf("events after next clean tick = %+v, want delayed entity entered", sessionEvents)
	}
	var entered aoiEntityPayloadForTest
	if err := json.Unmarshal(sessionEvents[0].Payload, &entered); err != nil {
		t.Fatalf("decode delayed entered event: %v", err)
	}
	if entered.EntityID != "entity_tick_error_visible" {
		t.Fatalf("delayed entered entity = %+v, want entity_tick_error_visible", entered)
	}
}

func TestAOIDiffDoesNotSerializeUnchangedEntityAfterSharedSnapshot(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "aoi-unchanged@example.com", "AOI-Unchanged")

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	_ = decodeWorldSnapshotForTest(t, events)
	insertTestWorldEntity(t, gameServer, "entity_unchanged_visible", world.EntityTypeNPC, world.Vec2{X: 200, Y: 0}, false)

	firstTick := gameServer.runtime.tickAndCollectAOIEvents()
	if got := countEventTypeForTest(firstTick[resolved.SessionID], realtime.EventAOIEntityEntered); got != 1 {
		t.Fatalf("first tick entered events = %d, want 1", got)
	}

	secondTick := gameServer.runtime.tickAndCollectAOIEvents()
	if got := countEventTypeForTest(secondTick[resolved.SessionID], realtime.EventAOIEntityUpdated); got != 0 {
		t.Fatalf("unchanged second tick updated events = %d, want 0", got)
	}
}

func TestAOISharedSnapshotKeepsHiddenEntityExcluded(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "aoi-hidden-shared@example.com", "AOI-Hidden-Shared")

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	_ = decodeWorldSnapshotForTest(t, events)
	insertTestWorldEntity(t, gameServer, "entity_shared_snapshot_hidden", world.EntityTypeNPC, world.Vec2{X: 180, Y: 0}, true)
	insertTestWorldEntity(t, gameServer, "entity_shared_snapshot_visible", world.EntityTypeNPC, world.Vec2{X: 220, Y: 0}, false)

	eventsBySession := gameServer.runtime.tickAndCollectAOIEvents()
	raw := string(mustJSON(t, eventsBySession[resolved.SessionID]))
	if strings.Contains(raw, "entity_shared_snapshot_hidden") {
		t.Fatalf("shared snapshot AOI leaked hidden entity in %s", raw)
	}
}

func TestAOITickEmitsSubphaseMetrics(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "aoi-metrics@example.com", "AOI-Metrics")

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	_ = decodeWorldSnapshotForTest(t, events)

	gameServer.runtime.tickAndCollectAOIEvents()
	snapshot := gameServer.runtime.Metrics.Snapshot()
	for _, phase := range []string{worker.TickPhaseMovement, worker.TickPhaseAggro, runtimeAOITickPhaseAOI, runtimeAOITickPhaseEnqueue} {
		requireMetricDuration(t, snapshot, observability.MetricZoneTickPhaseMS, 1, []observability.Label{
			{Name: "phase", Value: phase},
			{Name: "world_id", Value: "world-1"},
			{Name: "zone_id", Value: "map_1_1"},
		})
	}
}

func TestHiddenPlayerWitnessVisibilityIsViewerSpecificAndExpires(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		Clock:             clock,
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	target := createResolvedRuntimeSession(t, gameServer, "hidden-target@example.com", "Hidden")
	viewer := createResolvedRuntimeSession(t, gameServer, "scanner-viewer@example.com", "Scanner")
	other := createResolvedRuntimeSession(t, gameServer, "other-viewer@example.com", "Other")
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	setTestHiddenPlayer(gameServer, target.PlayerID, true)

	targetEvents, err := gameServer.runtime.bootstrapEvents(target)
	if err != nil {
		t.Fatalf("target bootstrap events: %v", err)
	}
	targetSnapshot := decodeWorldSnapshotForTest(t, targetEvents)
	if !hasEntityID(targetSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("hidden target self snapshot missing self entity %s: %+v", targetEntityID, targetSnapshot.Entities)
	}

	viewerEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("viewer bootstrap events: %v", err)
	}
	viewerSnapshot := decodeWorldSnapshotForTest(t, viewerEvents)
	if hasEntityID(viewerSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw hidden target without witness: %+v", viewerSnapshot.Entities)
	}

	otherEvents, err := gameServer.runtime.bootstrapEvents(other)
	if err != nil {
		t.Fatalf("other bootstrap events: %v", err)
	}
	otherSnapshot := decodeWorldSnapshotForTest(t, otherEvents)
	if hasEntityID(otherSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("other viewer saw hidden target without witness: %+v", otherSnapshot.Entities)
	}

	setTestHiddenPlayerWitness(gameServer, viewer.PlayerID, target.PlayerID, clock.Now().Add(runtimeHiddenPlayerWitnessDuration))
	witnessedEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("witnessed bootstrap events: %v", err)
	}
	witnessedSnapshot := decodeWorldSnapshotForTest(t, witnessedEvents)
	witnessedTarget, ok := entityPayloadByID(witnessedSnapshot.Entities, targetEntityID.String())
	if !ok {
		t.Fatalf("witnessed viewer snapshot missing hidden target %s: %+v", targetEntityID, witnessedSnapshot.Entities)
	}
	if !hasStatusFlag(witnessedTarget.StatusFlags, "scan_revealed") {
		t.Fatalf("witnessed target flags = %+v, want scan_revealed", witnessedTarget.StatusFlags)
	}
	rawWitnessed := string(mustJSON(t, witnessedSnapshot))
	for _, forbidden := range []string{"hidden", "target_player_id", "witness_expires_at", "player_id", target.PlayerID.String(), viewer.PlayerID.String()} {
		if strings.Contains(rawWitnessed, forbidden) {
			t.Fatalf("witnessed snapshot leaked %q in %s", forbidden, rawWitnessed)
		}
	}

	otherAfterWitnessEvents, err := gameServer.runtime.bootstrapEvents(other)
	if err != nil {
		t.Fatalf("other after witness bootstrap events: %v", err)
	}
	otherAfterWitness := decodeWorldSnapshotForTest(t, otherAfterWitnessEvents)
	if hasEntityID(otherAfterWitness.Entities, targetEntityID.String()) {
		t.Fatalf("unrelated viewer saw witnessed hidden target: %+v", otherAfterWitness.Entities)
	}

	clock.Advance(runtimeHiddenPlayerWitnessDuration)
	expiredEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("expired witness bootstrap events: %v", err)
	}
	expiredSnapshot := decodeWorldSnapshotForTest(t, expiredEvents)
	if hasEntityID(expiredSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw hidden target after witness expiry: %+v", expiredSnapshot.Entities)
	}
}
func TestScanPulseRevealsHiddenPlayerWithoutPlanetIntelOrXP(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		Clock:             clock,
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	target := createResolvedRuntimeSession(t, gameServer, "hidden-scan-target@example.com", "Hidden Scan")
	viewer := createResolvedRuntimeSession(t, gameServer, "hidden-scan-viewer@example.com", "Scanner Scan")
	other := createResolvedRuntimeSession(t, gameServer, "hidden-scan-other@example.com", "Other Scan")
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	setTestHiddenPlayer(gameServer, target.PlayerID, true)

	viewerEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("viewer bootstrap events: %v", err)
	}
	viewerSnapshot := decodeWorldSnapshotForTest(t, viewerEvents)
	if hasEntityID(viewerSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw hidden target before scan: %+v", viewerSnapshot.Entities)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(viewer.SessionID.String()),
		[]byte(`{"request_id":"request-scan-player-reveal","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("scan hidden player response error = %+v, want success", response.Error)
	}
	rawResponse := string(response.Response.Payload)
	for _, forbidden := range []string{
		"known_planets",
		"progression",
		"planet_id",
		"target_player_id",
		"witness_expires_at",
		"witness_expiry",
		"hidden",
		"detection_roll",
		"scan_candidate",
		"candidate_key",
		"procedural_seed",
		target.PlayerID.String(),
		viewer.PlayerID.String(),
	} {
		if strings.Contains(rawResponse, forbidden) {
			t.Fatalf("scan hidden player response leaked %q in %s", forbidden, rawResponse)
		}
	}
	var payload struct {
		Scan         scanPulsePayload            `json:"scan"`
		KnownPlanets *knownPlanetsPayload        `json:"known_planets,omitempty"`
		Progression  *progressionSnapshotPayload `json:"progression,omitempty"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode scan hidden player payload: %v", err)
	}
	if payload.Scan.Status != string(discovery.ScanPulseStatusPlayerRevealed) {
		t.Fatalf("scan status = %q, want %q", payload.Scan.Status, discovery.ScanPulseStatusPlayerRevealed)
	}
	if payload.Scan.PlanetID != "" || payload.Scan.Signal != nil || payload.Scan.XPGranted || payload.KnownPlanets != nil || payload.Progression != nil {
		t.Fatalf("scan hidden player payload = %+v, want no planet/intel/progression", payload)
	}

	events, err := gameServer.runtime.postCommandEvents(viewer.SessionID, realtime.OperationScanPulse, viewer.PlayerID)
	if err != nil {
		t.Fatalf("post scan events: %v", err)
	}
	seenResolved := false
	seenEntered := false
	for _, event := range events {
		rawEvent := string(mustJSON(t, event))
		for _, forbidden := range []string{"target_player_id", "witness_expires_at", "hidden", "detection_roll", "candidate_key", "procedural_seed", target.PlayerID.String()} {
			if strings.Contains(rawEvent, forbidden) {
				t.Fatalf("scan hidden player event leaked %q in %s", forbidden, rawEvent)
			}
		}
		if event.Type == realtime.EventScanPulseResolved {
			seenResolved = true
			if !strings.Contains(rawEvent, string(discovery.ScanPulseStatusPlayerRevealed)) {
				t.Fatalf("scan resolved event = %s, want player_revealed", rawEvent)
			}
		}
		if event.Type == realtime.EventAOIEntityEntered && strings.Contains(rawEvent, targetEntityID.String()) {
			seenEntered = true
			if !strings.Contains(rawEvent, "scan_revealed") {
				t.Fatalf("aoi entered event = %s, want scan_revealed", rawEvent)
			}
		}
	}
	if !seenResolved || !seenEntered {
		t.Fatalf("post scan events = %+v, want scan resolved and AOI entered hidden target", events)
	}

	otherEvents, err := gameServer.runtime.bootstrapEvents(other)
	if err != nil {
		t.Fatalf("other bootstrap events: %v", err)
	}
	otherSnapshot := decodeWorldSnapshotForTest(t, otherEvents)
	if hasEntityID(otherSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("unrelated viewer saw scan-revealed hidden target: %+v", otherSnapshot.Entities)
	}

	clock.Advance(runtimeHiddenPlayerWitnessDuration)
	expiredEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("expired bootstrap events: %v", err)
	}
	expiredSnapshot := decodeWorldSnapshotForTest(t, expiredEvents)
	if hasEntityID(expiredSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw scan-revealed target after expiry: %+v", expiredSnapshot.Entities)
	}
}
func TestScanPulseDoesNotRevealHiddenPlayerOutsideEffectiveRadarRange(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		Clock:             clock,
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	target := createResolvedRuntimeSession(t, gameServer, "hidden-scan-far-target@example.com", "Hidden Far")
	viewer := createResolvedRuntimeSession(t, gameServer, "hidden-scan-far-viewer@example.com", "Scanner Far")
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: defaultRadarRange + 250, Y: 0})
	setTestHiddenPlayer(gameServer, target.PlayerID, true)

	viewerEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("viewer bootstrap events: %v", err)
	}
	viewerSnapshot := decodeWorldSnapshotForTest(t, viewerEvents)
	if hasEntityID(viewerSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw out-of-range hidden target before scan: %+v", viewerSnapshot.Entities)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(viewer.SessionID.String()),
		[]byte(`{"request_id":"request-scan-player-range-miss","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("scan range miss response error = %+v, want success", response.Error)
	}
	rawResponse := string(response.Response.Payload)
	for _, forbidden := range []string{targetEntityID.String(), target.PlayerID.String(), "target_player_id", "witness_expires_at", "hidden"} {
		if strings.Contains(rawResponse, forbidden) {
			t.Fatalf("scan range miss response leaked %q in %s", forbidden, rawResponse)
		}
	}
	var payload struct {
		Scan scanPulsePayload `json:"scan"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode scan range miss payload: %v", err)
	}
	if payload.Scan.Status != string(discovery.ScanPulseStatusNoSignal) {
		t.Fatalf("scan status = %q, want %q for hidden-player reveal outside effective radar range", payload.Scan.Status, discovery.ScanPulseStatusNoSignal)
	}

	gameServer.runtime.mu.Lock()
	instance, _, instanceErr := gameServer.runtime.activeMapInstanceLocked(viewer.PlayerID)
	if instanceErr != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("active instance: %v", instanceErr)
	}
	witnessed := gameServer.runtime.hiddenPlayerWitnessActiveLocked(instance, viewer.PlayerID, target.PlayerID, clock.Now())
	gameServer.runtime.mu.Unlock()
	if witnessed {
		t.Fatal("hidden player witness active outside effective radar range, want none")
	}

	events, err := gameServer.runtime.postCommandEvents(viewer.SessionID, realtime.OperationScanPulse, viewer.PlayerID)
	if err != nil {
		t.Fatalf("post scan range miss events: %v", err)
	}
	for _, event := range events {
		rawEvent := string(mustJSON(t, event))
		if strings.Contains(rawEvent, targetEntityID.String()) || strings.Contains(rawEvent, target.PlayerID.String()) {
			t.Fatalf("scan range miss event leaked target in %s", rawEvent)
		}
	}

	afterEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("viewer after range miss bootstrap events: %v", err)
	}
	afterSnapshot := decodeWorldSnapshotForTest(t, afterEvents)
	if hasEntityID(afterSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw out-of-range hidden target after scan: %+v", afterSnapshot.Entities)
	}
}
func TestScanPulseRevealsHiddenPlayerInsideEffectiveRadarRangeBeyondOldProjectionWindow(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		Clock:             clock,
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	target := createResolvedRuntimeSession(t, gameServer, "hidden-scan-authoritative-target@example.com", "Hidden Authoritative")
	viewer := createResolvedRuntimeSession(t, gameServer, "hidden-scan-authoritative-viewer@example.com", "Scanner Authoritative")
	targetEntityID := testPlayerEntityID(t, gameServer, target.PlayerID)
	moveTestPlayerEntity(gameServer, target.PlayerID, world.Vec2{X: 1250, Y: 0})
	setTestHiddenPlayer(gameServer, target.PlayerID, true)
	setTestRadarRange(gameServer, viewer.PlayerID, 1500)

	viewerEvents, err := gameServer.runtime.bootstrapEvents(viewer)
	if err != nil {
		t.Fatalf("viewer bootstrap events: %v", err)
	}
	viewerSnapshot := decodeWorldSnapshotForTest(t, viewerEvents)
	if hasEntityID(viewerSnapshot.Entities, targetEntityID.String()) {
		t.Fatalf("viewer saw hidden target before scan: %+v", viewerSnapshot.Entities)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(viewer.SessionID.String()),
		[]byte(`{"request_id":"request-scan-player-authoritative-range","op":"scan.pulse","payload":{},"client_seq":1,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("scan authoritative range response error = %+v, want success", response.Error)
	}
	var payload struct {
		Scan scanPulsePayload `json:"scan"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode scan authoritative range payload: %v", err)
	}
	if payload.Scan.Status != string(discovery.ScanPulseStatusPlayerRevealed) {
		t.Fatalf("scan status = %q, want %q for same-map target inside authoritative range", payload.Scan.Status, discovery.ScanPulseStatusPlayerRevealed)
	}

	events, err := gameServer.runtime.postCommandEvents(viewer.SessionID, realtime.OperationScanPulse, viewer.PlayerID)
	if err != nil {
		t.Fatalf("post scan authoritative range events: %v", err)
	}
	seenEntered := false
	for _, event := range events {
		rawEvent := string(mustJSON(t, event))
		if strings.Contains(rawEvent, "target_player_id") || strings.Contains(rawEvent, target.PlayerID.String()) {
			t.Fatalf("scan authoritative range event leaked target internals in %s", rawEvent)
		}
		if event.Type == realtime.EventAOIEntityEntered && strings.Contains(rawEvent, targetEntityID.String()) {
			seenEntered = true
			if !strings.Contains(rawEvent, "scan_revealed") {
				t.Fatalf("aoi entered event = %s, want scan_revealed", rawEvent)
			}
		}
	}
	if !seenEntered {
		t.Fatalf("post scan authoritative range events = %+v, want AOI entered for revealed hidden target", events)
	}
}
func TestRuntimePlayerStealthAppliesSpeedPenaltyWithoutStackingAndRecalculatesRoute(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		Clock:             clock,
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	resolved := createResolvedRuntimeSession(t, gameServer, "stealth-speed@example.com", "Stealth")

	gameServer.runtime.mu.Lock()
	if err := gameServer.runtime.Worker.Submit(worker.MoveToCommand{
		PlayerID: resolved.PlayerID,
		Intent:   mustMovementIntentForServerTest(t, world.Vec2{X: 1_000, Y: 0}),
	}); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("Submit(move) error = %v, want nil", err)
	}
	moveResult := gameServer.runtime.Worker.Tick()
	gameServer.runtime.mu.Unlock()
	if len(moveResult.CommandErrors) > 0 {
		t.Fatalf("move command errors = %+v, want none", moveResult.CommandErrors)
	}

	clock.Advance(500 * time.Millisecond)
	if err := gameServer.runtime.setPlayerStealth(resolved.PlayerID, true); err != nil {
		t.Fatalf("set stealth true error = %v, want nil", err)
	}

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[resolved.PlayerID]
	entity, ok := gameServer.runtime.Worker.PlayerEntity(resolved.PlayerID)
	instance, _, instanceErr := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if instanceErr != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("active instance: %v", instanceErr)
	}
	hidden := instance.HiddenPlayers[resolved.PlayerID]
	speed, speedOK := gameServer.runtime.Worker.EntitySpeed(state.EntityID)
	gameServer.runtime.mu.Unlock()
	if !ok || !speedOK || !hidden {
		t.Fatalf("runtime stealth state entity=%v speed=%v hidden=%v, want all true", ok, speedOK, hidden)
	}
	assertServerFloatNear(t, state.Stats.Speed, defaultPlayerSpeed*runtimeStealthSpeedMultiplier)
	assertServerFloatNear(t, speed, state.Stats.Speed)
	assertServerVecNear(t, entity.Movement.Origin, world.Vec2{X: 90, Y: 0})
	if entity.Movement.Target != (world.Vec2{X: 1_000, Y: 0}) {
		t.Fatalf("stealth movement target = %+v, want original target", entity.Movement.Target)
	}
	assertServerFloatNear(t, entity.Movement.Speed, defaultPlayerSpeed*runtimeStealthSpeedMultiplier)

	if err := gameServer.runtime.setPlayerStealth(resolved.PlayerID, true); err != nil {
		t.Fatalf("set stealth true duplicate error = %v, want nil", err)
	}
	gameServer.runtime.mu.Lock()
	duplicateState := gameServer.runtime.players[resolved.PlayerID]
	duplicateSpeed, _ := gameServer.runtime.Worker.EntitySpeed(duplicateState.EntityID)
	gameServer.runtime.mu.Unlock()
	assertServerFloatNear(t, duplicateState.Stats.Speed, defaultPlayerSpeed*runtimeStealthSpeedMultiplier)
	assertServerFloatNear(t, duplicateSpeed, duplicateState.Stats.Speed)

	if err := gameServer.runtime.setPlayerStealth(resolved.PlayerID, false); err != nil {
		t.Fatalf("set stealth false error = %v, want nil", err)
	}
	gameServer.runtime.mu.Lock()
	restoredState := gameServer.runtime.players[resolved.PlayerID]
	instance, _, instanceErr = gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if instanceErr != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("active instance: %v", instanceErr)
	}
	restoredHidden := instance.HiddenPlayers[resolved.PlayerID]
	restoredSpeed, _ := gameServer.runtime.Worker.EntitySpeed(restoredState.EntityID)
	gameServer.runtime.mu.Unlock()
	if restoredHidden {
		t.Fatal("hiddenPlayers[player] = true after disable, want false")
	}
	if restoredState.Stats.Speed != defaultPlayerSpeed || restoredSpeed != defaultPlayerSpeed {
		t.Fatalf("restored speed state=%v worker=%v, want %v", restoredState.Stats.Speed, restoredSpeed, defaultPlayerSpeed)
	}
}
func TestRuntimePlayerStealthRestoresServerEffectiveSpeed(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "stealth-effective-speed@example.com", "Stealth Effective")
	const baseSpeed = 260.0

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[resolved.PlayerID]
	state.Stats.Speed = baseSpeed
	gameServer.runtime.players[resolved.PlayerID] = state
	if err := gameServer.runtime.Worker.Submit(worker.SetPlayerSpeedCommand{PlayerID: resolved.PlayerID, Speed: baseSpeed}); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("Submit(base speed) error = %v, want nil", err)
	}
	result := gameServer.runtime.Worker.Tick()
	gameServer.runtime.mu.Unlock()
	if len(result.CommandErrors) > 0 {
		t.Fatalf("base speed command errors = %+v, want none", result.CommandErrors)
	}

	if err := gameServer.runtime.setPlayerStealth(resolved.PlayerID, true); err != nil {
		t.Fatalf("set stealth true error = %v, want nil", err)
	}
	gameServer.runtime.mu.Lock()
	hiddenState := gameServer.runtime.players[resolved.PlayerID]
	hiddenSpeed, _ := gameServer.runtime.Worker.EntitySpeed(hiddenState.EntityID)
	gameServer.runtime.mu.Unlock()
	assertServerFloatNear(t, hiddenState.Stats.Speed, baseSpeed*runtimeStealthSpeedMultiplier)
	assertServerFloatNear(t, hiddenSpeed, hiddenState.Stats.Speed)

	if err := gameServer.runtime.setPlayerStealth(resolved.PlayerID, false); err != nil {
		t.Fatalf("set stealth false error = %v, want nil", err)
	}
	gameServer.runtime.mu.Lock()
	restoredState := gameServer.runtime.players[resolved.PlayerID]
	restoredSpeed, _ := gameServer.runtime.Worker.EntitySpeed(restoredState.EntityID)
	gameServer.runtime.mu.Unlock()
	if restoredState.Stats.Speed != baseSpeed || restoredSpeed != baseSpeed {
		t.Fatalf("restored speed state=%v worker=%v, want %v", restoredState.Stats.Speed, restoredSpeed, baseSpeed)
	}
}
func TestStealthToggleCommandUsesServerOwnedStateAndSafePayload(t *testing.T) {
	clock := testutil.NewFakeClock(time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC))
	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		SessionTTL:        time.Hour,
		TickDelta:         50 * time.Millisecond,
		Clock:             clock,
		ContentRepository: staticContentRepositoryForTest(),
		PasswordHasher:    auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	resolved := createResolvedRuntimeSession(t, gameServer, "stealth-command@example.com", "Stealth Command")

	bootstrapEvents, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	initialSnapshot := decodeWorldSnapshotForTest(t, bootstrapEvents)
	selfEntityID := testPlayerEntityID(t, gameServer, resolved.PlayerID).String()
	if entity, ok := entityPayloadByID(initialSnapshot.Entities, selfEntityID); !ok || hasStatusFlag(entity.StatusFlags, "stealthed") {
		t.Fatalf("initial self entity = %+v ok=%v, want visible without stealthed", entity, ok)
	}

	forged := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-stealth-forged","op":"stealth.toggle","payload":{"enabled":true,"hidden":true},"client_seq":1,"v":1}`),
	)
	if !forged.HasError || forged.Error.Error.Code != foundation.CodeInvalidPayload {
		t.Fatalf("forged stealth response = %+v, want invalid payload", forged)
	}

	response := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-stealth-enable","op":"stealth.toggle","payload":{"enabled":true},"client_seq":2,"v":1}`),
	)
	if response.HasError {
		t.Fatalf("stealth enable response error = %+v, want success", response.Error)
	}
	rawResponse := string(response.Response.Payload)
	for _, forbidden := range []string{"hidden", "player_id", resolved.PlayerID.String()} {
		if strings.Contains(rawResponse, forbidden) {
			t.Fatalf("stealth response leaked %q in %s", forbidden, rawResponse)
		}
	}
	var payload struct {
		Accepted bool `json:"accepted"`
		Stealth  struct {
			Enabled bool `json:"enabled"`
		} `json:"stealth"`
		Stats statSnapshotPayload `json:"stats"`
	}
	if err := json.Unmarshal(response.Response.Payload, &payload); err != nil {
		t.Fatalf("decode stealth response: %v", err)
	}
	if !payload.Accepted || !payload.Stealth.Enabled {
		t.Fatalf("stealth payload = %+v, want accepted enabled", payload)
	}
	assertServerFloatNear(t, payload.Stats.Speed, defaultPlayerSpeed*runtimeStealthSpeedMultiplier)

	events, err := gameServer.runtime.postCommandEvents(resolved.SessionID, realtime.OperationStealthToggle, resolved.PlayerID)
	if err != nil {
		t.Fatalf("post stealth events: %v", err)
	}
	seenStats := false
	seenSelfUpdate := false
	for _, event := range events {
		rawEvent := string(mustJSON(t, event))
		for _, forbidden := range []string{"hidden", "player_id", resolved.PlayerID.String()} {
			if strings.Contains(rawEvent, forbidden) {
				t.Fatalf("stealth event leaked %q in %s", forbidden, rawEvent)
			}
		}
		switch event.Type {
		case realtime.EventStatsUpdated:
			seenStats = true
		case realtime.EventAOIEntityUpdated, realtime.EventAOIEntityEntered:
			var entity aoi.EntityPayload
			if err := json.Unmarshal(event.Payload, &entity); err != nil {
				t.Fatalf("decode stealth AOI entity: %v", err)
			}
			if entity.ID.String() == selfEntityID && hasStatusFlag(entity.StatusFlags, "stealthed") {
				seenSelfUpdate = true
			}
		}
	}
	if !seenStats || !seenSelfUpdate {
		t.Fatalf("stealth post events = %+v, want stats update and self AOI stealthed update", events)
	}

	disable := gameServer.runtime.Gateway.HandleRequest(
		realtime.SessionID(resolved.SessionID.String()),
		[]byte(`{"request_id":"request-stealth-disable","op":"stealth.toggle","payload":{"enabled":false},"client_seq":3,"v":1}`),
	)
	if disable.HasError {
		t.Fatalf("stealth disable response error = %+v, want success", disable.Error)
	}
	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[resolved.PlayerID]
	instance, _, instanceErr := gameServer.runtime.activeMapInstanceLocked(resolved.PlayerID)
	if instanceErr != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("active instance: %v", instanceErr)
	}
	hidden := instance.HiddenPlayers[resolved.PlayerID]
	gameServer.runtime.mu.Unlock()
	if hidden || state.Stats.Speed != defaultPlayerSpeed {
		t.Fatalf("disable hidden=%v speed=%v, want false/%v", hidden, state.Stats.Speed, defaultPlayerSpeed)
	}
}
func TestWorldSnapshotProjectionUsesServerOwnedRadarStat(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "projection@example.com", "Projection")
	other := createResolvedRuntimeSession(t, gameServer, "projection-other@example.com", "Projection Other")
	moveTestPlayerEntity(gameServer, other.PlayerID, world.Vec2{X: 90, Y: 0})
	insertTestWorldEntity(t, gameServer, "entity_projection_inside", world.EntityTypeNPC, world.Vec2{X: 100, Y: 0}, false)
	insertTestWorldEntity(t, gameServer, "entity_projection_loot", world.EntityTypeLoot, world.Vec2{X: 120, Y: 0}, false)
	insertTestWorldEntity(t, gameServer, "entity_projection_outside", world.EntityTypeNPC, world.Vec2{X: 151, Y: 0}, false)
	insertTestWorldEntity(t, gameServer, "entity_projection_hidden_inside", world.EntityTypeNPC, world.Vec2{X: 100, Y: 100}, true)

	gameServer.runtime.mu.Lock()
	state := gameServer.runtime.players[resolved.PlayerID]
	state.Stats.RadarRange = 150
	gameServer.runtime.players[resolved.PlayerID] = state
	gameServer.runtime.mu.Unlock()

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	snapshot := decodeWorldSnapshotForTest(t, events)

	if snapshot.Minimap.RadarRange != 150 || snapshot.Minimap.ProjectionWindowSize != 300 {
		t.Fatalf("projection payload = %+v, want server-owned radar range/window", snapshot.Minimap)
	}
	otherEntityID := testPlayerEntityID(t, gameServer, other.PlayerID)
	for _, want := range []string{"entity_projection_inside", "entity_projection_loot", otherEntityID.String()} {
		if !hasEntityID(snapshot.Entities, want) {
			t.Fatalf("projection snapshot missing %s: %+v", want, snapshot.Entities)
		}
		contact, ok := minimapContactByID(snapshot.Minimap.LiveContacts, want)
		if !ok || contact.ProjectionSource != runtimeProjectionSourceWorker {
			t.Fatalf("projection contact %s = %+v, want source %q", want, contact, runtimeProjectionSourceWorker)
		}
	}
	for _, forbidden := range []string{"entity_projection_outside", "entity_projection_hidden_inside"} {
		if hasEntityID(snapshot.Entities, forbidden) {
			t.Fatalf("projection snapshot included %s: %+v", forbidden, snapshot.Entities)
		}
		for _, contact := range snapshot.Minimap.LiveContacts {
			if contact.EntityID == forbidden {
				t.Fatalf("projection minimap leaked %s: %+v", forbidden, snapshot.Minimap.LiveContacts)
			}
		}
	}
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	if got := gameServer.runtime.players[resolved.PlayerID].Stats.RadarRange; got != 150 {
		t.Fatalf("player stat radar range = %v, want unchanged 150", got)
	}
}
func TestWorldSnapshotFarRememberedPlanetStaysMemoryNotLiveContact(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "projection-memory@example.com", "Projection Memory")
	now := gameServer.runtime.clock.Now().UTC()
	planetID := foundation.PlanetID("planet-far-memory")
	coordinates := world.Vec2{X: 5200, Y: -3800}

	if _, err := gameServer.runtime.Discovery.MaterializePlanet(discovery.MaterializePlanetInput{
		CandidateKey: "candidate-far-memory",
		Planet: discovery.Planet{
			ID:           planetID,
			WorldID:      gameServer.runtime.worldID,
			ZoneID:       gameServer.runtime.zoneID,
			Coordinates:  coordinates,
			Biome:        discovery.PlanetBiomeOuterDrift,
			Type:         discovery.PlanetTypeIce,
			Rarity:       discovery.PlanetRarityUncommon,
			Level:        2,
			DiscoveredAt: now,
			DiscoveredBy: resolved.PlayerID,
		},
	}); err != nil {
		t.Fatalf("MaterializePlanet() error = %v, want nil", err)
	}
	if _, _, err := gameServer.runtime.Discovery.UpsertPlayerPlanetIntel(discovery.PlayerPlanetIntel{
		PlayerID:        resolved.PlayerID,
		PlanetID:        planetID,
		WorldID:         gameServer.runtime.worldID,
		ZoneID:          gameServer.runtime.zoneID,
		Coordinates:     coordinates,
		State:           discovery.IntelStateFresh,
		Confidence:      100,
		LastSeenAt:      now,
		SourceType:      discovery.IntelSourceAdmin,
		SourceReference: "far-memory-fixture",
	}); err != nil {
		t.Fatalf("UpsertPlayerPlanetIntel() error = %v, want nil", err)
	}

	events, err := gameServer.runtime.bootstrapEvents(resolved)
	if err != nil {
		t.Fatalf("bootstrap events: %v", err)
	}
	snapshot := decodeWorldSnapshotForTest(t, events)

	for _, contact := range snapshot.Minimap.LiveContacts {
		if contact.EntityID == planetID.String() {
			t.Fatalf("far remembered planet became live radar contact: %+v", snapshot.Minimap.LiveContacts)
		}
	}
	if len(snapshot.Minimap.Remembered) != 1 {
		t.Fatalf("remembered minimap = %+v, want one far planet memory", snapshot.Minimap.Remembered)
	}
	memory := snapshot.Minimap.Remembered[0]
	if memory.PlanetID != planetID.String() || memory.DetailID != planetID.String() {
		t.Fatalf("far memory ids = %+v, want planet/detail %s", memory, planetID)
	}
	if memory.Position != coordinates {
		t.Fatalf("far memory position = %+v, want unclipped coordinates %+v", memory.Position, coordinates)
	}
	if memory.Freshness != string(discovery.IntelStateFresh) {
		t.Fatalf("far memory freshness = %q, want fresh", memory.Freshness)
	}
	if memory.SectorKey != "1-1" || memory.PublicMapKey != "1-1" || memory.ProjectionSource != runtimeProjectionSourceKnownIntel {
		t.Fatalf("far memory sector/source = %+v, want %s/%s", memory, "1-1", runtimeProjectionSourceKnownIntel)
	}
}
func TestKnownPlanetMemoryIsFilteredToActiveMapPublicKey(t *testing.T) {
	gameServer, _ := newTestServer(t, false)
	resolved := createResolvedRuntimeSession(t, gameServer, "known-map-filter@example.com", "Known Map")
	now := gameServer.runtime.clock.Now().UTC()
	starterPlanetID := foundation.PlanetID("planet-known-map-1-1")
	mapTwoPlanetID := foundation.PlanetID("planet-known-map-1-2")
	mapTwoZoneID := worldmaps.MapID("map_1_2").ZoneID()

	fixtures := []struct {
		planetID     foundation.PlanetID
		zoneID       foundation.ZoneID
		coordinates  world.Vec2
		candidateKey discovery.PlanetMaterializationKey
	}{
		{
			planetID:     starterPlanetID,
			zoneID:       gameServer.runtime.zoneID,
			coordinates:  world.Vec2{X: 1400, Y: 1500},
			candidateKey: "candidate-known-map-1-1",
		},
		{
			planetID:     mapTwoPlanetID,
			zoneID:       mapTwoZoneID,
			coordinates:  world.Vec2{X: 1600, Y: 5200},
			candidateKey: "candidate-known-map-1-2",
		},
	}
	for _, fixture := range fixtures {
		if _, err := gameServer.runtime.Discovery.MaterializePlanet(discovery.MaterializePlanetInput{
			CandidateKey: fixture.candidateKey,
			Planet: discovery.Planet{
				ID:           fixture.planetID,
				WorldID:      gameServer.runtime.worldID,
				ZoneID:       fixture.zoneID,
				Coordinates:  fixture.coordinates,
				Biome:        discovery.PlanetBiomeOuterDrift,
				Type:         discovery.PlanetTypeIce,
				Rarity:       discovery.PlanetRarityUncommon,
				Level:        2,
				DiscoveredAt: now,
				DiscoveredBy: resolved.PlayerID,
			},
		}); err != nil {
			t.Fatalf("MaterializePlanet(%s) error = %v, want nil", fixture.planetID, err)
		}
		if _, _, err := gameServer.runtime.Discovery.UpsertPlayerPlanetIntel(discovery.PlayerPlanetIntel{
			PlayerID:        resolved.PlayerID,
			PlanetID:        fixture.planetID,
			WorldID:         gameServer.runtime.worldID,
			ZoneID:          fixture.zoneID,
			Coordinates:     fixture.coordinates,
			State:           discovery.IntelStateFresh,
			Confidence:      100,
			LastSeenAt:      now,
			SourceType:      discovery.IntelSourceAdmin,
			SourceReference: string(fixture.candidateKey),
		}); err != nil {
			t.Fatalf("UpsertPlayerPlanetIntel(%s) error = %v, want nil", fixture.planetID, err)
		}
	}

	known, err := gameServer.runtime.knownPlanetsPayload(resolved.PlayerID)
	if err != nil {
		t.Fatalf("knownPlanetsPayload(starter) error = %v, want nil", err)
	}
	if len(known.Planets) != 1 || known.Planets[0].PlanetID != starterPlanetID.String() || known.Planets[0].PublicMapKey != "1-1" {
		t.Fatalf("starter known planets = %+v, want only %s on public map 1-1", known, starterPlanetID)
	}
	minimap, err := gameServer.runtime.currentMinimapPayload(resolved.PlayerID)
	if err != nil {
		t.Fatalf("currentMinimapPayload(starter) error = %v, want nil", err)
	}
	if len(minimap.Remembered) != 1 || minimap.Remembered[0].PlanetID != starterPlanetID.String() || minimap.Remembered[0].PublicMapKey != "1-1" {
		t.Fatalf("starter remembered minimap = %+v, want only %s on public map 1-1", minimap.Remembered, starterPlanetID)
	}

	gameServer.runtime.mu.Lock()
	if _, err := gameServer.runtime.mapRouter.SetActiveLocationFromSpawn(resolved.PlayerID, "map_1_2", "west_gate"); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("SetActiveLocationFromSpawn(map_1_2) error = %v, want nil", err)
	}
	gameServer.runtime.mu.Unlock()
	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("ensurePlayerSession(map_1_2) error = %v, want nil", err)
	}

	known, err = gameServer.runtime.knownPlanetsPayload(resolved.PlayerID)
	if err != nil {
		t.Fatalf("knownPlanetsPayload(map_1_2) error = %v, want nil", err)
	}
	if len(known.Planets) != 1 || known.Planets[0].PlanetID != mapTwoPlanetID.String() || known.Planets[0].PublicMapKey != "1-2" {
		t.Fatalf("map_1_2 known planets = %+v, want only %s on public map 1-2", known, mapTwoPlanetID)
	}
	minimap, err = gameServer.runtime.currentMinimapPayload(resolved.PlayerID)
	if err != nil {
		t.Fatalf("currentMinimapPayload(map_1_2) error = %v, want nil", err)
	}
	if len(minimap.Remembered) != 1 || minimap.Remembered[0].PlanetID != mapTwoPlanetID.String() || minimap.Remembered[0].PublicMapKey != "1-2" {
		t.Fatalf("map_1_2 remembered minimap = %+v, want only %s on public map 1-2", minimap.Remembered, mapTwoPlanetID)
	}
}

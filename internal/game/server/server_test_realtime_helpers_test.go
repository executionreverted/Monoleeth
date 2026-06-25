package server

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/economy"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
)

func hasStatusFlag(flags []aoi.StatusFlag, want aoi.StatusFlag) bool {
	for _, flag := range flags {
		if flag == want {
			return true
		}
	}
	return false
}
func hasEntityID(entities []aoi.EntityPayload, want string) bool {
	for _, entity := range entities {
		if entity.ID.String() == want {
			return true
		}
	}
	return false
}
func assertEventsContainEntityOnly(t *testing.T, events []realtime.EventEnvelope, want string, forbidden string) {
	t.Helper()
	found := false
	for _, event := range events {
		raw := string(event.Payload)
		if strings.Contains(raw, forbidden) {
			t.Fatalf("events leaked forbidden entity %q in %+v", forbidden, events)
		}
		if strings.Contains(raw, want) {
			found = true
		}
	}
	if !found {
		t.Fatalf("events = %+v, missing entity %q", events, want)
	}
}
func assertMinimapMirrorsEntities(t *testing.T, label string, entities []aoi.EntityPayload, minimap minimapPayload) {
	t.Helper()
	if minimap.RadarRange <= 0 || minimap.ProjectionWindowSize != minimap.RadarRange*2 {
		t.Fatalf("%s minimap projection = range %v window %v, want positive range and 2x window", label, minimap.RadarRange, minimap.ProjectionWindowSize)
	}
	if len(minimap.LiveContacts) != len(entities) {
		t.Fatalf("%s minimap contacts = %d, entities = %d", label, len(minimap.LiveContacts), len(entities))
	}
	entitiesByID := make(map[string]aoi.EntityPayload, len(entities))
	for _, entity := range entities {
		entitiesByID[entity.ID.String()] = entity
	}
	for _, contact := range minimap.LiveContacts {
		if contact.EntityID == "" || contact.EntityType == "" {
			t.Fatalf("%s minimap contact missing stable identity: %+v", label, contact)
		}
		entity, ok := entitiesByID[contact.EntityID]
		if !ok {
			t.Fatalf("%s minimap contact %+v missing matching entity", label, contact)
		}
		if contact.EntityType != entity.Type || contact.Position != entity.Position {
			t.Fatalf("%s minimap contact %+v does not mirror entity %+v", label, contact, entity)
		}
		if contact.ProjectionSource != entity.ProjectionSource {
			t.Fatalf("%s minimap contact %+v projection source does not mirror entity %+v", label, contact, entity)
		}
	}
}
func minimapContactByID(contacts []minimapContactPayload, want string) (minimapContactPayload, bool) {
	for _, contact := range contacts {
		if contact.EntityID == want {
			return contact, true
		}
	}
	return minimapContactPayload{}, false
}
func entityPayloadByID(entities []aoi.EntityPayload, want string) (aoi.EntityPayload, bool) {
	for _, entity := range entities {
		if entity.ID.String() == want {
			return entity, true
		}
	}
	return aoi.EntityPayload{}, false
}
func mustMovementIntentForServerTest(t *testing.T, target world.Vec2) world.MovementIntent {
	t.Helper()
	intent, err := world.NewMovementIntent(target)
	if err != nil {
		t.Fatalf("NewMovementIntent(%+v) error = %v, want nil", target, err)
	}
	return intent
}
func assertServerVecNear(t *testing.T, got world.Vec2, want world.Vec2) {
	t.Helper()
	if math.Abs(got.X-want.X) > 0.05 || math.Abs(got.Y-want.Y) > 0.05 {
		t.Fatalf("vector = %+v, want near %+v", got, want)
	}
}
func assertServerFloatNear(t *testing.T, got float64, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("float = %v, want near %v", got, want)
	}
}
func decodeWorldSnapshotForTest(t *testing.T, events []realtime.EventEnvelope) worldSnapshotPayload {
	t.Helper()
	var snapshot worldSnapshotPayload
	if err := json.Unmarshal(events[len(events)-1].Payload, &snapshot); err != nil {
		t.Fatalf("decode world snapshot: %v", err)
	}
	return snapshot
}

type aoiEntityPayloadForTest struct {
	EntityID   string `json:"entity_id"`
	EntityType string `json:"entity_type"`
}

type movementPayloadForTest struct {
	Moving      bool       `json:"moving"`
	Origin      world.Vec2 `json:"origin"`
	Target      world.Vec2 `json:"target"`
	Speed       float64    `json:"speed"`
	StartedAtMS int64      `json:"started_at_ms"`
	ArriveAtMS  int64      `json:"arrive_at_ms"`
}

func logoutPilot(t *testing.T, httpServer *httptest.Server, cookie *http.Cookie) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/auth/logout", nil)
	if err != nil {
		t.Fatalf("new logout request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("logout request error = %v, want nil", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", resp.StatusCode)
	}
}
func dialWebSocket(t *testing.T, httpServer *httptest.Server, cookie *http.Cookie) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(httpServer), &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin": []string{testOrigin},
			"Cookie": []string{cookie.String()},
		},
	})
	if err != nil {
		t.Fatalf("websocket dial error = %v, want nil", err)
	}
	return conn
}
func wsURL(httpServer *httptest.Server) string {
	return "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
}
func readBootstrapEvents(t *testing.T, conn *websocket.Conn) []realtime.EventEnvelope {
	t.Helper()
	events := make([]realtime.EventEnvelope, 0, 8)
	for len(events) < 8 {
		events = append(events, readEvent(t, conn))
	}
	return events
}
func writeText(t *testing.T, conn *websocket.Conn, payload string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, []byte(payload)); err != nil {
		t.Fatalf("websocket Write() error = %v, want nil", err)
	}
}
func readRawText(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("websocket Read() error = %v, want nil", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("message type = %v, want text", messageType)
	}
	return data
}
func readEvent(t *testing.T, conn *websocket.Conn) realtime.EventEnvelope {
	t.Helper()
	data := readRawText(t, conn)
	var event realtime.EventEnvelope
	if err := json.Unmarshal(data, &event); err != nil {
		t.Fatalf("decode event %s: %v", data, err)
	}
	if event.Type == "" || event.Payload == nil {
		t.Fatalf("message %s is not an event", data)
	}
	return event
}
func readEventOfTypeSkipping(t *testing.T, conn *websocket.Conn, want realtime.ClientEventType) realtime.EventEnvelope {
	t.Helper()
	for range 8 {
		event := readEvent(t, conn)
		if event.Type == want {
			return event
		}
	}
	t.Fatalf("no %s event received before skip limit", want)
	return realtime.EventEnvelope{}
}
func drainEventTypes(t *testing.T, conn *websocket.Conn, wants ...realtime.ClientEventType) {
	t.Helper()
	seen := make(map[realtime.ClientEventType]bool, len(wants))
	for range wants {
		event := readEvent(t, conn)
		seen[event.Type] = true
	}
	for _, want := range wants {
		if !seen[want] {
			t.Fatalf("events seen = %#v, missing %s", seen, want)
		}
	}
}
func assertNoEconomyLeak(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{
		"seller_player_id",
		"buyer_player_id",
		"bidder_player_id",
		"current_bidder_id",
		"winning_player_id",
		"provider_reference",
		"provider",
		"escrow_location",
		"source_return_location",
		"world_id",
		"zone_id",
		"account_id",
		"session_id",
		"server_recalculates",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked %q in %s", label, forbidden, raw)
		}
	}
}
func assertPassiveMarketEventSafe(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	assertNoEconomyLeak(t, label, payload)
	raw := string(payload)
	for _, forbidden := range []string{"wallet", "inventory"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked private refresh %q in %s", label, forbidden, raw)
		}
	}
}
func assertPassiveMarketEvent(t *testing.T, label string, event realtime.EventEnvelope, want realtime.ClientEventType) marketListingPayload {
	t.Helper()
	if event.Type != want {
		t.Fatalf("%s event type = %s, want %s", label, event.Type, want)
	}
	assertPassiveMarketEventSafe(t, label, event.Payload)
	var listing marketListingPayload
	if err := json.Unmarshal(event.Payload, &listing); err != nil {
		t.Fatalf("decode %s listing event: %v", label, err)
	}
	if listing.OwnedByYou {
		t.Fatalf("%s listing = %+v, want public non-owned listing", label, listing)
	}
	return listing
}

type auctionLotEventForTest struct {
	auctionLotPayload
	Sequence uint64
}

type walletSnapshotEventForTest struct {
	walletSnapshotPayload
	Sequence uint64
}

type auctionClosedEventForTest struct {
	Lot      auctionLotPayload    `json:"lot"`
	Grant    *auctionGrantPayload `json:"grant,omitempty"`
	Sequence uint64
}

type premiumEntitlementEventForTest struct {
	premiumEntitlementPayload
	Sequence uint64
}

type premiumStockEventForTest struct {
	premiumStockPayload
	Sequence uint64
}

func assertAuctionLotEvent(t *testing.T, label string, event realtime.EventEnvelope, want realtime.ClientEventType) auctionLotEventForTest {
	t.Helper()
	if event.Type != want {
		t.Fatalf("%s event type = %s, want %s", label, event.Type, want)
	}
	assertAuctionLotEventSafe(t, label, event.Payload)
	var lot auctionLotPayload
	if err := json.Unmarshal(event.Payload, &lot); err != nil {
		t.Fatalf("decode %s auction lot event: %v", label, err)
	}
	if lot.AuctionID != seedAuctionID.String() {
		t.Fatalf("%s auction lot = %+v, want %s", label, lot, seedAuctionID)
	}
	return auctionLotEventForTest{auctionLotPayload: lot, Sequence: event.Sequence}
}
func assertAuctionLotEventSafe(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	assertNoEconomyLeak(t, label, payload)
	raw := string(payload)
	for _, forbidden := range []string{
		"wallet",
		"inventory",
		"grant",
		"reference_id",
		"refund_reference_id",
		"ledger",
		"bidder_debit",
		"buyer_debit",
		"previous_refund",
		"current_refund",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked private auction field %q in %s", label, forbidden, raw)
		}
	}
}
func assertWalletSnapshotEvent(t *testing.T, label string, event realtime.EventEnvelope) walletSnapshotEventForTest {
	t.Helper()
	if event.Type != realtime.EventWalletSnapshot {
		t.Fatalf("%s event type = %s, want %s", label, event.Type, realtime.EventWalletSnapshot)
	}
	raw := string(event.Payload)
	for _, forbidden := range []string{
		"grant",
		"reference_id",
		"ledger",
		"provider",
		"bidder_player_id",
		"buyer_player_id",
		"current_bidder_id",
		"winning_player_id",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s wallet leaked %q in %s", label, forbidden, raw)
		}
	}
	var wallet walletSnapshotPayload
	if err := json.Unmarshal(event.Payload, &wallet); err != nil {
		t.Fatalf("decode %s wallet event: %v", label, err)
	}
	return walletSnapshotEventForTest{walletSnapshotPayload: wallet, Sequence: event.Sequence}
}
func assertAuctionClosedEvent(t *testing.T, label string, event realtime.EventEnvelope) auctionClosedEventForTest {
	t.Helper()
	if event.Type != realtime.EventAuctionClosed {
		t.Fatalf("%s event type = %s, want %s", label, event.Type, realtime.EventAuctionClosed)
	}
	assertNoEconomyLeak(t, label, event.Payload)
	raw := string(event.Payload)
	for _, forbidden := range []string{"reference_id", "refund_reference_id", "ledger", "buyer_debit", "current_refund"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s closed event leaked %q in %s", label, forbidden, raw)
		}
	}
	var payload auctionClosedEventForTest
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("decode %s auction closed event: %v", label, err)
	}
	if payload.Lot.AuctionID != seedAuctionID.String() {
		t.Fatalf("%s closed lot = %+v, want %s", label, payload.Lot, seedAuctionID)
	}
	payload.Sequence = event.Sequence
	return payload
}
func assertPremiumClaimedEvent(t *testing.T, label string, event realtime.EventEnvelope, entitlementID string) premiumEntitlementEventForTest {
	t.Helper()
	if event.Type != realtime.EventPremiumClaimed {
		t.Fatalf("%s event type = %s, want %s", label, event.Type, realtime.EventPremiumClaimed)
	}
	assertPremiumClaimedEventSafe(t, label, event.Payload)
	var entitlement premiumEntitlementPayload
	if err := json.Unmarshal(event.Payload, &entitlement); err != nil {
		t.Fatalf("decode %s premium claim event: %v", label, err)
	}
	if entitlement.EntitlementID != entitlementID {
		t.Fatalf("%s entitlement = %+v, want %s", label, entitlement, entitlementID)
	}
	return premiumEntitlementEventForTest{premiumEntitlementPayload: entitlement, Sequence: event.Sequence}
}
func assertPremiumClaimedEventSafe(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	assertNoEconomyLeak(t, label, payload)
	raw := string(payload)
	for _, forbidden := range []string{
		"wallet",
		"purchase_reference",
		"claim_request_reference",
		"ledger",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked private premium claim field %q in %s", label, forbidden, raw)
		}
	}
}
func assertPremiumStockConsumedEvent(t *testing.T, label string, event realtime.EventEnvelope) premiumStockEventForTest {
	t.Helper()
	if event.Type != realtime.EventPremiumStockConsumed {
		t.Fatalf("%s event type = %s, want %s", label, event.Type, realtime.EventPremiumStockConsumed)
	}
	assertPremiumStockConsumedEventSafe(t, label, event.Payload)
	var stock premiumStockPayload
	if err := json.Unmarshal(event.Payload, &stock); err != nil {
		t.Fatalf("decode %s premium stock event: %v", label, err)
	}
	return premiumStockEventForTest{premiumStockPayload: stock, Sequence: event.Sequence}
}
func assertPremiumStockConsumedEventSafe(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	assertNoEconomyLeak(t, label, payload)
	raw := string(payload)
	for _, forbidden := range []string{
		"wallet",
		"entitlement",
		"purchase",
		"provider",
		"reference",
		"_ref",
		"ledger",
		"player_id",
		"private",
	} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked private premium stock field %q in %s", label, forbidden, raw)
		}
	}
}
func assertNoRealtimeMessageWithin(t *testing.T, label string, conn *websocket.Conn, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	messageType, data, err := conn.Read(ctx)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return
		}
		t.Fatalf("%s read error = %v, want only timeout", label, err)
	}
	t.Fatalf("%s unexpected realtime message type=%v data=%s", label, messageType, data)
}
func countInventoryInstances(items []economy.InstanceItem, itemID string) int {
	count := 0
	for _, item := range items {
		if item.ItemID.String() == itemID {
			count++
		}
	}
	return count
}
func inventorySnapshotHasInstance(snapshot inventorySnapshotPayload, itemID string) bool {
	for _, item := range snapshot.Instances {
		if item.ItemID == itemID {
			return true
		}
	}
	return false
}
func runtimeWalletCredits(t *testing.T, runtime *Runtime) int64 {
	t.Helper()
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	for _, playerID := range runtime.sessions {
		return runtime.walletSnapshotLocked(playerID).Credits
	}
	t.Fatal("runtime has no authenticated player session")
	return 0
}
func assertNoPhase09Leak(t *testing.T, label string, payload json.RawMessage) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range []string{
		"account_id",
		"player_id",
		"session_id",
		"password",
		"password_hash",
		"token",
		"cookie",
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
}

var forbiddenLeakCanaryTokens = []string{
	"internal_map_id",
	"source_map_id",
	"destination_map_id",
	"map_1_",
	"worker_id",
	"map_worker_id",
	"worker_topology",
	"entity_hidden_planet_signal",
	"HIDDEN_RUNTIME_METADATA_SENTINEL",
	"HIDDEN_PROJECTION_SENTINEL",
	"phase07-static-seed",
	`"hidden":`,
	"hidden_target_metadata",
	"target_player_id",
	"witness_expires_at",
	"stealth_score",
	"jammer_strength",
	"detection_roll",
	"scan_roll",
	"scan_candidates",
	"candidate_key",
	"procedural_seed",
	"gameplay_seed",
	"world_seed",
	"generated_seed",
	"generated_payload",
	"loot_roll",
	"future_spawn",
	"spawn_candidates",
	"spawn_areas",
	"enemy_pools",
	"npc_drop_profiles",
	"loot_table",
	"drop_profile",
	"server_only",
	"password",
	"password_hash",
	"session_token",
	"reset_secret",
}

func assertNoForbiddenLeakCanary(t *testing.T, label string, payload []byte) {
	t.Helper()
	raw := string(payload)
	for _, forbidden := range forbiddenLeakCanaryTokens {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked forbidden canary token %q in %s", label, forbidden, raw)
		}
	}
}
func readResponse(t *testing.T, conn *websocket.Conn) realtime.ResponseEnvelope {
	t.Helper()
	return decodeRawResponse(t, readRawText(t, conn))
}
func readResponseSkippingEvents(t *testing.T, conn *websocket.Conn) realtime.ResponseEnvelope {
	t.Helper()
	for range 32 {
		data := readRawText(t, conn)
		if !rawRealtimeMessageIsResponse(data) {
			continue
		}
		return decodeRawResponse(t, data)
	}
	t.Fatal("no response received before event skip limit")
	return realtime.ResponseEnvelope{}
}
func readErrorSkippingEvents(t *testing.T, conn *websocket.Conn) realtime.ErrorEnvelope {
	t.Helper()
	for range 8 {
		data := readRawText(t, conn)
		if !rawRealtimeMessageIsResponse(data) {
			continue
		}
		var response realtime.ErrorEnvelope
		if err := json.Unmarshal(data, &response); err != nil {
			t.Fatalf("decode error %s: %v", data, err)
		}
		if response.OK {
			t.Fatalf("error response %s had ok=true", data)
		}
		return response
	}
	t.Fatal("no error response received before event skip limit")
	return realtime.ErrorEnvelope{}
}
func rawRealtimeMessageIsResponse(data []byte) bool {
	var probe struct {
		OK *bool `json:"ok"`
	}
	return json.Unmarshal(data, &probe) == nil && probe.OK != nil
}
func decodeRawResponse(t *testing.T, data []byte) realtime.ResponseEnvelope {
	t.Helper()
	var response realtime.ResponseEnvelope
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode response %s: %v", data, err)
	}
	return response
}
func readError(t *testing.T, conn *websocket.Conn) realtime.ErrorEnvelope {
	t.Helper()
	data := readRawText(t, conn)
	var response realtime.ErrorEnvelope
	if err := json.Unmarshal(data, &response); err != nil {
		t.Fatalf("decode error %s: %v", data, err)
	}
	if response.OK {
		t.Fatalf("error response %s had ok=true", data)
	}
	return response
}
func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("Marshal(%T) error = %v", value, err)
	}
	return data
}

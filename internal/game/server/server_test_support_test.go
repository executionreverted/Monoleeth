package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/production"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/world"
	worldmaps "gameproject/internal/game/world/maps"
)

const testOrigin = "http://example.com"

func newTestServer(t *testing.T, devMode bool) (*Server, *httptest.Server) {
	t.Helper()
	gameServer, err := New(Config{
		AllowedOrigins: []string{testOrigin},
		DevMode:        devMode,
		SessionTTL:     time.Hour,
		TickDelta:      50 * time.Millisecond,
		PasswordHasher: auth.PBKDF2PasswordHasher{Iterations: 2, SaltBytes: 8, KeyBytes: 16},
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	httpServer := httptest.NewServer(gameServer.Handler())
	return gameServer, httpServer
}
func registerPilot(t *testing.T, httpServer *httptest.Server) *http.Cookie {
	t.Helper()
	return registerPilotWithIdentity(t, httpServer, "pilot@example.com", "Frontier-01")
}
func registerPilotWithIdentity(t *testing.T, httpServer *httptest.Server, email string, callsign string) *http.Cookie {
	t.Helper()
	body := strings.NewReader(fmt.Sprintf(`{"email":%q,"password":"correct-password","callsign":%q}`, email, callsign))
	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/auth/register", body)
	if err != nil {
		t.Fatalf("new register request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("register request error = %v, want nil", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, want 201", resp.StatusCode)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == auth.DefaultSessionCookieName {
			return cookie
		}
	}
	t.Fatal("register response missing session cookie")
	return nil
}
func loginPilot(t *testing.T, httpServer *httptest.Server, email string, password string) *http.Cookie {
	t.Helper()
	body := strings.NewReader(`{"email":"` + email + `","password":"` + password + `"}`)
	req, err := http.NewRequest(http.MethodPost, httpServer.URL+"/api/auth/login", body)
	if err != nil {
		t.Fatalf("new login request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("login request error = %v, want nil", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == auth.DefaultSessionCookieName {
			return cookie
		}
	}
	t.Fatal("login response missing session cookie")
	return nil
}
func resolvedSessionForCookie(t *testing.T, gameServer *Server, cookie *http.Cookie) auth.ResolvedSession {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	if err != nil {
		t.Fatalf("new resolve request: %v", err)
	}
	req.Header.Set("Origin", testOrigin)
	req.AddCookie(cookie)
	resolved, err := auth.ResolveCookie(context.Background(), gameServer.runtime.Auth, auth.DefaultSessionCookieName, gameServer.config.originPolicy(), req)
	if err != nil {
		t.Fatalf("resolve test cookie: %v", err)
	}
	return resolved
}
func createResolvedRuntimeSession(t *testing.T, gameServer *Server, email string, callsign string) auth.ResolvedSession {
	t.Helper()
	result, err := gameServer.runtime.Auth.Register(context.Background(), auth.RegisterInput{
		Email:    email,
		Password: "correct-password",
		Callsign: callsign,
	})
	if err != nil {
		t.Fatalf("register runtime session: %v", err)
	}
	if err := gameServer.runtime.ensurePlayerSession(result.Session); err != nil {
		t.Fatalf("ensure runtime player session: %v", err)
	}
	return result.Session
}
func createResolvedRuntimeSessionOnMap(t *testing.T, gameServer *Server, email string, callsign string, mapID worldmaps.MapID, spawnID worldmaps.SpawnID) auth.ResolvedSession {
	t.Helper()
	resolved := createResolvedRuntimeSession(t, gameServer, email, callsign)
	gameServer.runtime.mu.Lock()
	if _, err := gameServer.runtime.mapRouter.SetActiveLocationFromSpawn(resolved.PlayerID, mapID, spawnID); err != nil {
		gameServer.runtime.mu.Unlock()
		t.Fatalf("SetActiveLocationFromSpawn(%q, %q) error = %v, want nil", mapID, spawnID, err)
	}
	gameServer.runtime.mu.Unlock()
	if err := gameServer.runtime.ensurePlayerSession(resolved); err != nil {
		t.Fatalf("ensurePlayerSession(%q) error = %v, want nil", mapID, err)
	}
	return resolved
}
func seedOwnedProductionPlanetForTest(
	t *testing.T,
	gameServer *Server,
	ownerID foundation.PlayerID,
	planetID foundation.PlanetID,
	zoneID foundation.ZoneID,
	coordinates world.Vec2,
	candidateKey discovery.PlanetMaterializationKey,
) {
	t.Helper()
	now := gameServer.runtime.clock.Now().UTC()
	ownerChangedAt := now
	if _, err := gameServer.runtime.Discovery.MaterializePlanet(discovery.MaterializePlanetInput{
		CandidateKey: candidateKey,
		Planet: discovery.Planet{
			ID:             planetID,
			WorldID:        gameServer.runtime.worldID,
			ZoneID:         zoneID,
			Coordinates:    coordinates,
			Biome:          discovery.PlanetBiomeOuterDrift,
			Type:           discovery.PlanetTypeIce,
			Rarity:         discovery.PlanetRarityUncommon,
			Level:          2,
			DiscoveredAt:   now,
			DiscoveredBy:   ownerID,
			OwnerPlayerID:  ownerID,
			OwnerChangedAt: &ownerChangedAt,
		},
	}); err != nil {
		t.Fatalf("MaterializePlanet(%s) error = %v, want nil", planetID, err)
	}
	if _, err := gameServer.runtime.Production.InitializePlanetProduction(production.InitializePlanetProductionInput{
		PlanetID:              planetID,
		LastCalculatedAt:      now,
		StorageCapacityUnits:  250,
		EnergyCapacityPerHour: 40,
		UpdatedAt:             now,
	}); err != nil {
		t.Fatalf("InitializePlanetProduction(%s) error = %v, want nil", planetID, err)
	}
}
func assertProductionSummaryPlanetIDs(t *testing.T, gameServer *Server, playerID foundation.PlayerID, planetID foundation.PlanetID, want []foundation.PlanetID) {
	t.Helper()
	payload, err := gameServer.runtime.productionSummaryPayload(playerID, planetID)
	if err != nil {
		t.Fatalf("productionSummaryPayload(%q) error = %v, want nil", planetID, err)
	}
	got := make([]foundation.PlanetID, 0, len(payload.Planets))
	for _, planet := range payload.Planets {
		got = append(got, foundation.PlanetID(planet.PlanetID))
	}
	assertPlanetIDListForTest(t, "production summary", got, want)
}
func assertStorageSummaryPlanetIDs(t *testing.T, gameServer *Server, playerID foundation.PlayerID, planetID foundation.PlanetID, want []foundation.PlanetID) {
	t.Helper()
	payload, err := gameServer.runtime.storageSummaryPayload(playerID, planetID)
	if err != nil {
		t.Fatalf("storageSummaryPayload(%q) error = %v, want nil", planetID, err)
	}
	got := make([]foundation.PlanetID, 0, len(payload.Planets))
	for _, planet := range payload.Planets {
		got = append(got, foundation.PlanetID(planet.PlanetID))
	}
	assertPlanetIDListForTest(t, "storage summary", got, want)
}
func assertPlanetIDListForTest(t *testing.T, label string, got []foundation.PlanetID, want []foundation.PlanetID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s planet ids = %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s planet ids = %v, want %v", label, got, want)
		}
	}
}
func setTestShipDisabled(gameServer *Server, playerID foundation.PlayerID, disabled bool) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state := gameServer.runtime.players[playerID]
	state.Ship.Disabled = disabled
	if disabled {
		state.Ship.RepairState = "disabled"
	} else {
		state.Ship.RepairState = "ready"
	}
	gameServer.runtime.players[playerID] = state
}
func setTestHidden(gameServer *Server, entityID world.EntityID, hidden bool) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	instance := testActiveMapInstanceLocked(gameServer, "")
	instance.HiddenEntities[entityID] = hidden
}
func setTestHiddenPlayer(gameServer *Server, playerID foundation.PlayerID, hidden bool) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	instance := testActiveMapInstanceLocked(gameServer, playerID)
	instance.HiddenPlayers[playerID] = hidden
}
func setTestHiddenPlayerWitness(gameServer *Server, viewerID foundation.PlayerID, targetID foundation.PlayerID, expiresAt time.Time) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	instance := testActiveMapInstanceLocked(gameServer, viewerID)
	instance.HiddenPlayerWitnesses[hiddenPlayerWitnessKey{
		ViewerPlayerID: viewerID,
		TargetPlayerID: targetID,
	}] = expiresAt
}
func testPlayerEntityID(t *testing.T, gameServer *Server, playerID foundation.PlayerID) world.EntityID {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state, ok := gameServer.runtime.players[playerID]
	if !ok {
		t.Fatalf("player %q missing runtime state", playerID)
	}
	return state.EntityID
}
func insertTestWorldEntity(t *testing.T, gameServer *Server, entityID world.EntityID, entityType world.EntityType, position world.Vec2, hidden bool) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	insertTestWorldEntityInMapLocked(t, gameServer, worldmaps.StarterMapID, entityID, entityType, position, hidden)
}
func insertTestWorldEntityInMapLocked(t *testing.T, gameServer *Server, mapID worldmaps.MapID, entityID world.EntityID, entityType world.EntityType, position world.Vec2, hidden bool) {
	t.Helper()
	instance, err := gameServer.runtime.mapInstanceLocked(mapID)
	if err != nil {
		t.Fatalf("mapInstanceLocked(%q) = %v, want nil", mapID, err)
	}
	entity, err := world.NewEntity(instance.Definition.WorldID, instance.Definition.ZoneID, entityID, entityType, position)
	if err != nil {
		t.Fatalf("NewEntity(%q) = %v, want nil", entityID, err)
	}
	if err := instance.Worker.InsertEntity(entity, 0); err != nil {
		t.Fatalf("InsertEntity(%q) = %v, want nil", entityID, err)
	}
	instance.HiddenEntities[entityID] = hidden
}
func setTestWeaponRange(gameServer *Server, playerID foundation.PlayerID, weaponRange float64) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state := gameServer.runtime.players[playerID]
	state.Stats.WeaponRange = weaponRange
	gameServer.runtime.players[playerID] = state
}
func setTestRadarRange(gameServer *Server, playerID foundation.PlayerID, radarRange float64) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state := gameServer.runtime.players[playerID]
	state.Stats.RadarRange = radarRange
	gameServer.runtime.players[playerID] = state
}
func moveTestPlayerEntity(gameServer *Server, playerID foundation.PlayerID, position world.Vec2) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	instance, _, err := gameServer.runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		return
	}
	entity, ok := instance.Worker.PlayerEntity(playerID)
	if !ok {
		return
	}
	entity.Position = position
	entity.Movement = world.MovementState{}
	_ = instance.Worker.UpdateEntity(entity)
}
func moveTestPlayerNearEntity(t *testing.T, gameServer *Server, playerID foundation.PlayerID, entityID world.EntityID, offset world.Vec2) {
	t.Helper()
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	instance, _, err := gameServer.runtime.activeMapInstanceLocked(playerID)
	if err != nil {
		t.Fatalf("activeMapInstanceLocked() error = %v, want nil", err)
	}
	target, ok := instance.Worker.Entity(entityID)
	if !ok {
		t.Fatalf("target entity %q missing", entityID)
	}
	player, ok := instance.Worker.PlayerEntity(playerID)
	if !ok {
		t.Fatalf("player entity for %q missing", playerID)
	}
	player.Position = world.Vec2{X: target.Position.X + offset.X, Y: target.Position.Y + offset.Y}
	player.Movement = world.MovementState{}
	if err := instance.Worker.UpdateEntity(player); err != nil {
		t.Fatalf("UpdateEntity(player near %q) = %v, want nil", entityID, err)
	}
}
func testActiveMapInstanceLocked(gameServer *Server, playerID foundation.PlayerID) *mapInstance {
	if playerID != "" {
		instance, _, err := gameServer.runtime.activeMapInstanceLocked(playerID)
		if err == nil {
			return instance
		}
	}
	instance, err := gameServer.runtime.mapInstanceLocked(worldmaps.StarterMapID)
	if err != nil {
		panic(err)
	}
	return instance
}
func testShipCapacitor(gameServer *Server, playerID foundation.PlayerID) int {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	return gameServer.runtime.players[playerID].Ship.Capacitor
}
func setTestShipCapacitor(gameServer *Server, playerID foundation.PlayerID, capacitor int) {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	state := gameServer.runtime.players[playerID]
	state.Ship.Capacitor = capacitor
	gameServer.runtime.players[playerID] = state
}
func testCargoUsed(gameServer *Server, playerID foundation.PlayerID) int64 {
	gameServer.runtime.mu.Lock()
	defer gameServer.runtime.mu.Unlock()
	return gameServer.runtime.players[playerID].Cargo.Used
}
func requireMetricCounter(t *testing.T, snapshot observability.MetricSnapshot, name string, value int64, labels []observability.Label) {
	t.Helper()
	for _, counter := range snapshot.Counters {
		if counter.Name != name || !sameMetricLabels(counter.Labels, labels) {
			continue
		}
		if counter.Value != value {
			t.Fatalf("metric %s labels %+v value = %d, want %d", name, labels, counter.Value, value)
		}
		return
	}
	t.Fatalf("missing metric %s labels %+v in snapshot %+v", name, labels, snapshot)
}
func sameMetricLabels(got, want []observability.Label) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
func progressableQuestOfferWithItemReward(t *testing.T, offers []questOfferPayload) questOfferPayload {
	t.Helper()
	for _, offer := range offers {
		if questItemRewardCount(offer.Rewards) != 1 {
			continue
		}
		for _, objective := range offer.Objectives {
			switch objective.Kind {
			case quests.ObjectiveKindKill.String(), quests.ObjectiveKindCollect.String(), quests.ObjectiveKindCraft.String():
				if objective.Required > 0 {
					return offer
				}
			}
		}
	}
	t.Fatalf("no progressable kill/collect/craft offer with one item reward in %+v", offers)
	return questOfferPayload{}
}
func questItemRewardCount(rewards []questRewardPayload) int {
	count := 0
	for _, reward := range rewards {
		if reward.Kind == quests.RewardKindItem.String() {
			count++
		}
	}
	return count
}
func questItemReward(t *testing.T, rewards []questRewardPayload) questRewardPayload {
	t.Helper()
	var itemReward questRewardPayload
	for _, reward := range rewards {
		if reward.Kind != quests.RewardKindItem.String() {
			continue
		}
		if itemReward.Kind != "" {
			t.Fatalf("quest rewards = %+v, want exactly one item reward", rewards)
		}
		itemReward = reward
	}
	if itemReward.Kind == "" || itemReward.ItemID == "" || itemReward.Amount <= 0 {
		t.Fatalf("quest rewards = %+v, want positive item reward", rewards)
	}
	return itemReward
}
func assertQuestRewardInventorySnapshot(t *testing.T, inventory inventorySnapshotPayload, reward questRewardPayload) {
	t.Helper()
	for _, stack := range inventory.Stackable {
		if stack.ItemID == reward.ItemID && stack.Location == economy.LocationKindAccountInventory.String() {
			if stack.Quantity != reward.Amount {
				t.Fatalf("quest reward inventory stack = %+v, want quantity %d", stack, reward.Amount)
			}
			return
		}
	}
	t.Fatalf("quest reward inventory = %+v, missing %s x%d in account inventory", inventory, reward.ItemID, reward.Amount)
}
func requireInventoryStack(t *testing.T, inventory inventorySnapshotPayload, itemID string, location string) inventoryStackPayload {
	t.Helper()
	for _, stack := range inventory.Stackable {
		if stack.ItemID == itemID && stack.Location == location {
			return stack
		}
	}
	t.Fatalf("inventory = %+v, missing stack %s at %s", inventory, itemID, location)
	return inventoryStackPayload{}
}
func requireInventoryInstance(t *testing.T, inventory inventorySnapshotPayload, itemID string, location string) string {
	t.Helper()
	for _, item := range inventory.Instances {
		if item.ItemID == itemID && item.Location == location {
			if item.DurabilityCurrent <= 0 {
				t.Fatalf("inventory instance = %+v, want positive durability", item)
			}
			return item.ItemInstanceID
		}
	}
	t.Fatalf("inventory = %+v, missing instance %s at %s", inventory, itemID, location)
	return ""
}
func requireInventoryInstanceLocation(t *testing.T, inventory inventorySnapshotPayload, itemInstanceID string, location string) {
	t.Helper()
	for _, item := range inventory.Instances {
		if item.ItemInstanceID == itemInstanceID {
			if item.Location != location {
				t.Fatalf("inventory instance = %+v, want location %s", item, location)
			}
			return
		}
	}
	t.Fatalf("inventory = %+v, missing instance %s", inventory, itemInstanceID)
}
func testStackableDefinition(t *testing.T, itemID string, name string, flags []economy.TradeFlag) economy.ItemDefinition {
	t.Helper()
	source, err := catalog.NewVersionedDefinitionFromStrings(itemID, "v1")
	if err != nil {
		t.Fatalf("definition source: %v", err)
	}
	maxStack, err := foundation.NewQuantity(999)
	if err != nil {
		t.Fatalf("max stack: %v", err)
	}
	weight, err := foundation.NewQuantity(1)
	if err != nil {
		t.Fatalf("weight: %v", err)
	}
	definition, err := economy.NewItemDefinition(
		source,
		foundation.ItemID(itemID),
		name,
		economy.ItemTypeStackable,
		economy.ItemRarityCommon,
		maxStack,
		weight,
		flags,
		[]economy.BindRule{economy.BindRuleNone},
		nil,
	)
	if err != nil {
		t.Fatalf("stackable definition: %v", err)
	}
	return definition
}
func addTestInventoryStack(t *testing.T, gameServer *Server, playerID foundation.PlayerID, definition economy.ItemDefinition, quantity int64, location economy.ItemLocation, referenceSuffix string) {
	t.Helper()
	referenceKey, err := foundation.AdminCompensationIdempotencyKey(playerID.String(), referenceSuffix)
	if err != nil {
		t.Fatalf("inventory reference: %v", err)
	}
	if _, err := gameServer.runtime.Inventory.AddItem(economy.AddItemInput{
		PlayerID:       playerID,
		ItemDefinition: definition,
		Quantity:       quantity,
		Location:       location,
		Reason:         runtimeSeedLedgerReason,
		ReferenceKey:   referenceKey,
	}); err != nil {
		t.Fatalf("add inventory stack: %v", err)
	}
}
func requireLoadoutSlot(t *testing.T, loadout loadoutSnapshotPayload, slotID string) loadoutSlotPayload {
	t.Helper()
	for _, slot := range loadout.Slots {
		if slot.SlotID == slotID {
			return slot
		}
	}
	t.Fatalf("loadout = %+v, missing slot %s", loadout, slotID)
	return loadoutSlotPayload{}
}
func questRewardItemLedgerEntries(gameServer *Server, playerID foundation.PlayerID, referenceKey foundation.IdempotencyKey) []economy.ItemLedgerEntry {
	entries := gameServer.runtime.Inventory.ItemLedgerEntries()
	matched := make([]economy.ItemLedgerEntry, 0, 1)
	for _, entry := range entries {
		if entry.PlayerID == playerID &&
			entry.Action == economy.LedgerActionIncrease &&
			entry.Reason == runtimeQuestRewardLedgerReason &&
			entry.ReferenceKey == referenceKey {
			matched = append(matched, entry)
		}
	}
	return matched
}
func assertQuestRewardLedgerEntry(t *testing.T, entry economy.ItemLedgerEntry, reward questRewardPayload, referenceKey foundation.IdempotencyKey) {
	t.Helper()
	if entry.ItemID != foundation.ItemID(reward.ItemID) ||
		entry.Quantity.Int64() != reward.Amount ||
		entry.Location.Kind != economy.LocationKindAccountInventory ||
		entry.Action != economy.LedgerActionIncrease ||
		entry.Reason != runtimeQuestRewardLedgerReason ||
		entry.ReferenceKey != referenceKey {
		t.Fatalf("quest reward item ledger = %+v, want %s x%d in account inventory with reference %s", entry, reward.ItemID, reward.Amount, referenceKey)
	}
}
func completeQuestWithServerEvents(t *testing.T, gameServer *Server, playerID foundation.PlayerID, quest questPayload) {
	t.Helper()
	for _, objective := range quest.Objectives {
		switch objective.Kind {
		case quests.ObjectiveKindKill.String():
			for index := int64(0); index < objective.Required; index++ {
				updated, err := gameServer.runtime.Quest.ConsumeCombatNPCKilled(quests.CombatNPCKilledInput{
					EventID:          foundation.EventID(fmt.Sprintf("event-quest-test-kill-%d", index)),
					ProgressEventKey: quests.QuestProgressEventKey(fmt.Sprintf("test.quest.kill:%s:%d", quest.QuestID, index)),
					PlayerID:         playerID,
					NPCType:          objective.Target,
				})
				if err != nil {
					t.Fatalf("complete kill quest: %v", err)
				}
				if index == objective.Required-1 && len(updated) == 0 {
					t.Fatalf("complete kill quest updated no quests on final event")
				}
			}
		case quests.ObjectiveKindCollect.String():
			quantity, err := foundation.NewQuantity(objective.Required)
			if err != nil {
				t.Fatalf("collect quantity: %v", err)
			}
			updated, err := gameServer.runtime.Quest.ConsumeLootPickedUp(quests.LootPickedUpInput{
				EventID:          "event-quest-test-collect",
				ProgressEventKey: quests.QuestProgressEventKey("test.quest.collect:" + quest.QuestID),
				PlayerID:         playerID,
				ItemID:           foundation.ItemID(objective.Target),
				Quantity:         quantity,
			})
			if err != nil {
				t.Fatalf("complete collect quest: %v", err)
			}
			if len(updated) == 0 {
				t.Fatalf("complete collect quest updated no quests")
			}
		case quests.ObjectiveKindCraft.String():
			quantity, err := foundation.NewQuantity(objective.Required)
			if err != nil {
				t.Fatalf("craft quantity: %v", err)
			}
			updated, err := gameServer.runtime.Quest.ConsumeCraftJobCompleted(quests.CraftJobCompletedInput{
				EventID:          "event-quest-test-craft",
				ProgressEventKey: quests.QuestProgressEventKey("test.quest.craft:" + quest.QuestID),
				PlayerID:         playerID,
				RecipeID:         catalog.DefinitionID(objective.Target),
				Quantity:         quantity,
			})
			if err != nil {
				t.Fatalf("complete craft quest: %v", err)
			}
			if len(updated) == 0 {
				t.Fatalf("complete craft quest updated no quests")
			}
		default:
			t.Fatalf("unsupported test quest objective %+v", objective)
		}
	}
}
func killTrainingNPCForDrop(t *testing.T, conn *websocket.Conn) string {
	t.Helper()
	writeText(t, conn, `{"request_id":"request-combat-drop","op":"combat.use_skill","payload":{"skill_id":"basic_laser","target_id":"entity_training_npc"},"client_seq":1,"v":1}`)
	response := readResponseSkippingEvents(t, conn)
	if !response.OK {
		t.Fatalf("combat-for-drop response = %+v, want success", response)
	}

	var dropID string
	enteredIDs := make(map[string]struct{})
	sawDropEntered := false
	sawTrainingLeft := false
	seen := map[realtime.ClientEventType]bool{}
	for attempts := 0; attempts < 16 && (dropID == "" || !sawDropEntered || !sawTrainingLeft); attempts++ {
		event := readEvent(t, conn)
		seen[event.Type] = true
		switch event.Type {
		case realtime.EventLootCreated:
			var payload struct {
				DropID string `json:"drop_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode loot.created: %v", err)
			}
			dropID = payload.DropID
			if _, ok := enteredIDs[dropID]; ok {
				sawDropEntered = true
			}
		case realtime.EventAOIEntityEntered:
			var payload struct {
				EntityID string `json:"entity_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode aoi.entity_entered: %v", err)
			}
			enteredIDs[payload.EntityID] = struct{}{}
			if payload.EntityID == dropID && dropID != "" {
				sawDropEntered = true
			}
		case realtime.EventAOIEntityLeft:
			var payload struct {
				EntityID string `json:"entity_id"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("decode aoi.entity_left: %v", err)
			}
			if payload.EntityID == "entity_training_npc" {
				sawTrainingLeft = true
			}
		}
	}
	if dropID == "" || !sawDropEntered || !sawTrainingLeft {
		t.Fatalf("combat-for-drop events seen = %#v dropID=%q dropEntered=%v trainingLeft=%v", seen, dropID, sawDropEntered, sawTrainingLeft)
	}
	return dropID
}

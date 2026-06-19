package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/auth"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	"gameproject/internal/game/world/visibility"
	"gameproject/internal/game/world/worker"
)

const (
	starterShipID          foundation.ShipID = "starter_ship"
	starterShipDisplayName                   = "Sparrow"
	defaultPlayerSpeed                       = 180
	defaultRadarRange                        = 420
	defaultMaxMoveDistance                   = 1200
	minMoveCommandInterval                   = 75 * time.Millisecond
)

// RuntimeConfig wires the single-process game runtime.
type RuntimeConfig struct {
	Clock      foundation.Clock
	SessionTTL time.Duration
	TickDelta  time.Duration
	WorldID    foundation.WorldID
	ZoneID     foundation.ZoneID
	DevMode    bool
	AdminSeed  auth.AdminSeedInput
	Passwords  auth.PasswordHasher
}

// Runtime composes auth, realtime gateway, and the Phase 02 world worker.
type Runtime struct {
	mu      sync.Mutex
	clock   foundation.Clock
	devMode bool

	Auth    *auth.Service
	Gateway *realtime.Gateway
	Worker  *worker.Worker

	worldID foundation.WorldID
	zoneID  foundation.ZoneID

	players  map[foundation.PlayerID]playerRuntimeState
	hidden   map[world.EntityID]bool
	eventSeq map[auth.SessionID]uint64
	sessions map[auth.SessionID]foundation.PlayerID
	lastAOI  map[auth.SessionID]aoi.Snapshot
	lastMove map[foundation.PlayerID]time.Time

	nextPlayerEntity int
}

type playerRuntimeState struct {
	EntityID world.EntityID
	Callsign string
	Rank     int
	Ship     shipSnapshotPayload
	Stats    statSnapshotPayload
	Wallet   walletSnapshotPayload
	Cargo    cargoSnapshotPayload
}

type sessionReadyPayload struct {
	Authenticated   bool                `json:"authenticated"`
	Account         *auth.PublicAccount `json:"account,omitempty"`
	Player          *auth.PublicPlayer  `json:"player,omitempty"`
	Roles           []string            `json:"roles,omitempty"`
	ExpiresAt       int64               `json:"expires_at"`
	ProtocolVersion int                 `json:"protocol_version"`
	ReconnectCursor uint64              `json:"reconnect_cursor"`
}

type playerSnapshotPayload struct {
	Callsign  string `json:"callsign"`
	Rank      int    `json:"rank"`
	HP        int    `json:"hp"`
	MaxHP     int    `json:"max_hp"`
	Shield    int    `json:"shield"`
	MaxShield int    `json:"max_shield"`
	Energy    int    `json:"energy"`
	MaxEnergy int    `json:"max_energy"`
}

type shipSnapshotPayload struct {
	ActiveShipID string `json:"active_ship_id"`
	DisplayName  string `json:"display_name"`
	Hull         int    `json:"hull"`
	MaxHull      int    `json:"max_hull"`
	Shield       int    `json:"shield"`
	MaxShield    int    `json:"max_shield"`
	Capacitor    int    `json:"capacitor"`
	MaxCapacitor int    `json:"max_capacitor"`
	Disabled     bool   `json:"disabled"`
	RepairState  string `json:"repair_state"`
}

type statSnapshotPayload struct {
	Speed         float64 `json:"speed"`
	RadarRange    float64 `json:"radar_range"`
	WeaponRange   float64 `json:"weapon_range"`
	CargoCapacity int64   `json:"cargo_capacity"`
}

type walletSnapshotPayload struct {
	Credits       int64 `json:"credits"`
	PremiumPaid   int64 `json:"premium_paid"`
	PremiumEarned int64 `json:"premium_earned"`
}

type cargoSnapshotPayload struct {
	Used     int64            `json:"used"`
	Capacity int64            `json:"capacity"`
	Items    []cargoItemStack `json:"items"`
}

type cargoItemStack struct {
	ItemID   string `json:"item_id"`
	Quantity int64  `json:"quantity"`
}

type worldSnapshotPayload struct {
	Sector         sectorPayload       `json:"sector"`
	Entities       []aoi.EntityPayload `json:"entities"`
	Minimap        minimapPayload      `json:"minimap"`
	SnapshotCursor uint64              `json:"snapshot_cursor"`
}

type sectorPayload struct {
	Name      string `json:"name"`
	Region    string `json:"region"`
	Danger    string `json:"danger"`
	Contested bool   `json:"contested"`
}

type minimapPayload struct {
	RadarRange   float64                 `json:"radar_range"`
	LiveContacts []minimapContactPayload `json:"live_contacts"`
	Remembered   []minimapMemoryPayload  `json:"remembered"`
}

type minimapContactPayload struct {
	EntityID    string            `json:"entity_id"`
	EntityType  world.EntityType  `json:"entity_type"`
	Position    world.Vec2        `json:"position"`
	Disposition string            `json:"disposition,omitempty"`
	StatusFlags []aoi.StatusFlag  `json:"status_flags,omitempty"`
}

type minimapMemoryPayload struct {
	Kind      string     `json:"kind"`
	Label     string     `json:"label"`
	Position  world.Vec2 `json:"position"`
	Freshness string     `json:"freshness"`
}

// NewRuntime creates the single-process runtime.
func NewRuntime(config RuntimeConfig) (*Runtime, error) {
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	authStore := auth.NewInMemoryStore()
	authService, err := auth.NewService(auth.ServiceConfig{
		Store:          authStore,
		Clock:          clock,
		PasswordHasher: config.Passwords,
		SessionTTL:     config.SessionTTL,
	})
	if err != nil {
		return nil, err
	}
	if config.AdminSeed.Enabled {
		if _, err := authService.SeedAdmin(context.Background(), config.AdminSeed); err != nil {
			return nil, err
		}
	}
	zoneWorker, err := worker.NewWorker(worker.Config{
		WorldID:   config.WorldID,
		ZoneID:    config.ZoneID,
		TickDelta: config.TickDelta,
		Clock:     clock,
	})
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{
		clock:   clock,
		devMode: config.DevMode,
		Auth:    authService,
		Worker:  zoneWorker,
		worldID: config.WorldID,
		zoneID:  config.ZoneID,
		players: make(map[foundation.PlayerID]playerRuntimeState),
		hidden: map[world.EntityID]bool{
			"entity_hidden_planet_signal": true,
		},
		eventSeq: make(map[auth.SessionID]uint64),
		sessions: make(map[auth.SessionID]foundation.PlayerID),
		lastAOI:  make(map[auth.SessionID]aoi.Snapshot),
		lastMove: make(map[foundation.PlayerID]time.Time),
	}
	if err := runtime.seedWorld(); err != nil {
		return nil, err
	}
	gateway, err := realtime.NewGateway(realtime.GatewayOptions{
		Clock:    clock,
		Sessions: runtimeSessionResolver{runtime: runtime},
		Executor: realtime.ObservedCommandExecutor{
			Clock:   clock,
			Logger:  observability.NewMemoryCommandLogger(),
			Metrics: observability.NewMetricRecorder(),
		},
		Handlers: runtime.commandHandlers(),
	})
	if err != nil {
		return nil, err
	}
	runtime.Gateway = gateway
	return runtime, nil
}

// Start runs the worker tick lifecycle until ctx is canceled.
func (runtime *Runtime) Start(ctx context.Context) {
	runtime.StartWithEventSink(ctx, nil)
}

// StartWithEventSink runs the worker lifecycle and publishes per-session
// filtered AOI diffs after authoritative ticks.
func (runtime *Runtime) StartWithEventSink(ctx context.Context, sink func(auth.SessionID, []realtime.EventEnvelope)) {
	if runtime == nil || runtime.Worker == nil {
		return
	}
	ticker := time.NewTicker(runtime.Worker.TickDelta())
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				eventsBySession := runtime.tickAndCollectAOIEvents()
				if sink == nil {
					continue
				}
				for sessionID, events := range eventsBySession {
					if len(events) > 0 {
						sink(sessionID, events)
					}
				}
			}
		}
	}()
}

func (runtime *Runtime) seedWorld() error {
	visible, err := world.NewEntity(runtime.worldID, runtime.zoneID, "entity_training_npc", world.EntityTypeNPCPlaceholder, world.Vec2{X: 80, Y: 0})
	if err != nil {
		return err
	}
	if err := runtime.Worker.InsertEntity(visible, 0); err != nil {
		return err
	}
	hidden, err := world.NewEntity(runtime.worldID, runtime.zoneID, "entity_hidden_planet_signal", world.EntityTypePlanetSignalPlaceholder, world.Vec2{X: 120, Y: 0})
	if err != nil {
		return err
	}
	return runtime.Worker.InsertEntity(hidden, 0)
}

func (runtime *Runtime) ensurePlayerSession(resolved auth.ResolvedSession) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	state, ok := runtime.players[resolved.PlayerID]
	if !ok {
		runtime.nextPlayerEntity++
		entityID := foundation.EntityID(fmt.Sprintf("entity_pilot_%d", runtime.nextPlayerEntity))
		state = newPlayerRuntimeState(resolved.Callsign, entityID)
		runtime.players[resolved.PlayerID] = state
	} else if resolved.Callsign != "" && state.Callsign != resolved.Callsign {
		state.Callsign = resolved.Callsign
		runtime.players[resolved.PlayerID] = state
	}
	var err error
	if _, ok := runtime.Worker.PlayerEntity(resolved.PlayerID); !ok {
		if err = runtime.Worker.Submit(worker.SpawnPlayerCommand{
			PlayerID:  resolved.PlayerID,
			EntityID:  state.EntityID,
			Position:  world.Vec2{},
			Speed:     defaultPlayerSpeed,
			SessionID: realtime.SessionID(resolved.SessionID.String()),
		}); err != nil {
			return err
		}
		err = commandErrors(runtime.Worker.Tick())
	} else {
		if err = runtime.Worker.Submit(worker.AttachSessionCommand{
			SessionID: realtime.SessionID(resolved.SessionID.String()),
			PlayerID:  resolved.PlayerID,
		}); err != nil {
			return err
		}
		err = commandErrors(runtime.Worker.Tick())
	}
	if err != nil {
		return err
	}
	runtime.sessions[resolved.SessionID] = resolved.PlayerID
	return nil
}

func (runtime *Runtime) detachSession(sessionID auth.SessionID) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	_ = runtime.Worker.Submit(worker.DetachSessionCommand{SessionID: realtime.SessionID(sessionID.String())})
	runtime.Worker.Tick()
	delete(runtime.sessions, sessionID)
	delete(runtime.lastAOI, sessionID)
}

func (runtime *Runtime) bootstrapEvents(resolved auth.ResolvedSession) ([]realtime.EventEnvelope, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	state := runtime.players[resolved.PlayerID]
	worldSnapshot, err := runtime.worldSnapshotLocked(resolved.PlayerID)
	if err != nil {
		return nil, err
	}
	runtime.lastAOI[resolved.SessionID] = aoi.Snapshot{Entities: cloneAOIEntities(worldSnapshot.Entities)}
	events := make([]realtime.EventEnvelope, 0, 7)
	sessionPayload := sessionReadyPayload{
		Authenticated: true,
		Account: &auth.PublicAccount{
			Email: resolved.Email.String(),
			Admin: hasRole(resolved.Roles, auth.RoleAdmin),
		},
		Player: &auth.PublicPlayer{
			Callsign: resolved.Callsign,
		},
		Roles:           roleStrings(resolved.Roles),
		ExpiresAt:       resolved.ExpiresAt.UTC().UnixMilli(),
		ProtocolVersion: realtime.CurrentVersion,
		ReconnectCursor: runtime.eventSeq[resolved.SessionID],
	}
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventSessionReady, sessionPayload))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventPlayerSnapshot, state.playerSnapshot()))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventShipSnapshot, state.Ship))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventStatsUpdated, state.Stats))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventWalletSnapshot, state.Wallet))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventCargoSnapshot, state.Cargo))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventWorldSnapshot, worldSnapshot))
	return events, nil
}

func (runtime *Runtime) postCommandEvents(sessionID auth.SessionID, op realtime.Operation, playerID foundation.PlayerID) ([]realtime.EventEnvelope, error) {
	switch op {
	case realtime.OperationMoveTo, realtime.OperationStop:
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		entity, ok := runtime.Worker.PlayerEntity(playerID)
		if !ok {
			return nil, worker.ErrUnknownPlayer
		}
		payload := map[string]any{
			"entity_id": entity.ID.String(),
			"position":  entity.Position,
		}
		events := []realtime.EventEnvelope{runtime.eventLocked(sessionID, realtime.EventPositionCorrected, payload)}
		if op == realtime.OperationStop {
			events = append(events, runtime.eventLocked(sessionID, realtime.EventMovementStopped, payload))
		}
		events = append(events, runtime.aoiDiffEventsLocked(sessionID, playerID)...)
		return events, nil
	default:
		return nil, nil
	}
}

func (runtime *Runtime) eventLocked(sessionID auth.SessionID, eventType realtime.ClientEventType, payload any) realtime.EventEnvelope {
	runtime.eventSeq[sessionID]++
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{}`)
	}
	return realtime.NewEventEnvelope(
		foundation.EventID(fmt.Sprintf("event_%d", runtime.eventSeq[sessionID])),
		eventType,
		data,
		runtime.clock.Now().UTC().UnixMilli(),
		runtime.eventSeq[sessionID],
	)
}

func (runtime *Runtime) worldSnapshotLocked(playerID foundation.PlayerID) (worldSnapshotPayload, error) {
	snapshot, radarRange, tick, err := runtime.aoiSnapshotForPlayerLocked(playerID)
	if err != nil {
		return worldSnapshotPayload{}, err
	}
	return worldSnapshotPayload{
		Sector:         runtime.sectorPayloadLocked(),
		Entities:       cloneAOIEntities(snapshot.Entities),
		Minimap:        minimapFromAOI(snapshot, radarRange),
		SnapshotCursor: tick,
	}, nil
}

func (runtime *Runtime) aoiSnapshotForPlayerLocked(playerID foundation.PlayerID) (aoi.Snapshot, float64, uint64, error) {
	state := runtime.players[playerID]
	playerEntity, ok := runtime.Worker.PlayerEntity(playerID)
	if !ok {
		return aoi.Snapshot{}, 0, 0, worker.ErrUnknownPlayer
	}
	statSnapshot := stats.NewStatSnapshot(playerID, starterShipID, 1, stats.EffectiveStats{
		Exploration: stats.ExplorationStats{RadarRange: state.Stats.RadarRange},
	}, runtime.clock.Now())
	radarRange := visibility.RadarRangeFromStatSnapshot(statSnapshot)
	viewer := visibility.Viewer{
		WorldID:    runtime.worldID,
		ZoneID:     runtime.zoneID,
		Position:   playerEntity.Position,
		RadarRange: radarRange,
	}
	workerSnapshot := runtime.Worker.Snapshot()
	states := make([]aoi.EntityState, 0, len(workerSnapshot.Entities))
	for _, entity := range workerSnapshot.Entities {
		flags, display := runtime.publicEntityMetadataLocked(playerID, entity)
		states = append(states, aoi.EntityState{
			Entity:            entity,
			Signature:         visibility.EntitySignature(1),
			Hidden:            runtime.hidden[entity.ID],
			PublicStatusFlags: flags,
			PublicDisplay:     display,
		})
	}
	snapshot := aoi.BuildVisibleSnapshot(viewer, states)
	return snapshot, radarRange.Units(), workerSnapshot.Tick, nil
}

func (runtime *Runtime) tickAndCollectAOIEvents() map[auth.SessionID][]realtime.EventEnvelope {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.Worker.Tick()
	eventsBySession := make(map[auth.SessionID][]realtime.EventEnvelope)
	for sessionID, playerID := range runtime.sessions {
		events := runtime.aoiDiffEventsLocked(sessionID, playerID)
		if len(events) > 0 {
			eventsBySession[sessionID] = events
		}
	}
	return eventsBySession
}

func (runtime *Runtime) aoiDiffEventsLocked(sessionID auth.SessionID, playerID foundation.PlayerID) []realtime.EventEnvelope {
	current, _, _, err := runtime.aoiSnapshotForPlayerLocked(playerID)
	if err != nil {
		return nil
	}
	previous := runtime.lastAOI[sessionID]
	diff := aoi.DiffSnapshots(previous, current)
	runtime.lastAOI[sessionID] = aoi.Snapshot{Entities: cloneAOIEntities(current.Entities)}

	events := make([]realtime.EventEnvelope, 0, len(diff.Entered)+len(diff.Updated)+len(diff.Left))
	for _, entity := range diff.Entered {
		events = append(events, runtime.eventLocked(sessionID, realtime.EventAOIEntityEntered, entity))
	}
	for _, entity := range diff.Updated {
		events = append(events, runtime.eventLocked(sessionID, realtime.EventAOIEntityUpdated, entity))
	}
	for _, entityID := range diff.Left {
		events = append(events, runtime.eventLocked(sessionID, realtime.EventAOIEntityLeft, map[string]string{"entity_id": entityID.String()}))
	}
	return events
}

func (runtime *Runtime) publicEntityMetadataLocked(viewerID foundation.PlayerID, entity world.Entity) ([]aoi.StatusFlag, *aoi.EntityDisplay) {
	switch entity.Type {
	case world.EntityTypePlayer:
		if playerID, playerState, ok := runtime.playerByEntityLocked(entity.ID); ok {
			if playerID == viewerID {
				return []aoi.StatusFlag{"friendly", "self"}, &aoi.EntityDisplay{Label: playerState.Callsign, Disposition: "self"}
			}
			return []aoi.StatusFlag{"friendly"}, &aoi.EntityDisplay{Label: playerState.Callsign, Disposition: "friendly"}
		}
		return []aoi.StatusFlag{"friendly"}, &aoi.EntityDisplay{Label: "Pilot", Disposition: "friendly"}
	case world.EntityTypeNPC:
		return []aoi.StatusFlag{"hostile"}, &aoi.EntityDisplay{Label: displayLabelForEntity(entity.ID, "Training Drone"), Disposition: "hostile"}
	case world.EntityTypeLoot:
		return []aoi.StatusFlag{"loot"}, &aoi.EntityDisplay{Label: "Loot Cache", Disposition: "neutral"}
	case world.EntityTypePlanetSignal:
		return []aoi.StatusFlag{"unknown_signal"}, &aoi.EntityDisplay{Label: "Unknown Signal", Disposition: "unknown"}
	default:
		return nil, nil
	}
}

func (runtime *Runtime) playerByEntityLocked(entityID world.EntityID) (foundation.PlayerID, playerRuntimeState, bool) {
	for playerID, state := range runtime.players {
		if state.EntityID == entityID {
			return playerID, state, true
		}
	}
	return "", playerRuntimeState{}, false
}

func (runtime *Runtime) sectorPayloadLocked() sectorPayload {
	return sectorPayload{
		Name:      "Origin Fringe",
		Region:    "Origin Belt",
		Danger:    "low",
		Contested: false,
	}
}

func minimapFromAOI(snapshot aoi.Snapshot, radarRange float64) minimapPayload {
	contacts := make([]minimapContactPayload, 0, len(snapshot.Entities))
	for _, entity := range snapshot.Entities {
		disposition := ""
		if entity.Display != nil {
			disposition = entity.Display.Disposition
		}
		contacts = append(contacts, minimapContactPayload{
			EntityID:    entity.ID.String(),
			EntityType:  entity.Type,
			Position:    entity.Position,
			Disposition: disposition,
			StatusFlags: append([]aoi.StatusFlag(nil), entity.StatusFlags...),
		})
	}
	return minimapPayload{
		RadarRange:   radarRange,
		LiveContacts: contacts,
		Remembered:   []minimapMemoryPayload{},
	}
}

func cloneAOIEntities(entities []aoi.EntityPayload) []aoi.EntityPayload {
	if len(entities) == 0 {
		return nil
	}
	cloned := make([]aoi.EntityPayload, 0, len(entities))
	for _, entity := range entities {
		entity.StatusFlags = append([]aoi.StatusFlag(nil), entity.StatusFlags...)
		if entity.Display != nil {
			display := *entity.Display
			entity.Display = &display
		}
		cloned = append(cloned, entity)
	}
	return cloned
}

func displayLabelForEntity(entityID world.EntityID, fallback string) string {
	switch entityID {
	case "entity_training_npc":
		return "Training Drone"
	default:
		return fallback
	}
}

func newPlayerRuntimeState(callsign string, entityID world.EntityID) playerRuntimeState {
	if callsign == "" {
		callsign = "Pilot"
	}
	return playerRuntimeState{
		EntityID: entityID,
		Callsign: callsign,
		Rank:     1,
		Ship: shipSnapshotPayload{
			ActiveShipID: starterShipID.String(),
			DisplayName:  starterShipDisplayName,
			Hull:         100,
			MaxHull:      100,
			Shield:       100,
			MaxShield:    100,
			Capacitor:    100,
			MaxCapacitor: 100,
			RepairState:  "ready",
		},
		Stats: statSnapshotPayload{
			Speed:         defaultPlayerSpeed,
			RadarRange:    defaultRadarRange,
			WeaponRange:   260,
			CargoCapacity: 60,
		},
		Wallet: walletSnapshotPayload{},
		Cargo: cargoSnapshotPayload{
			Capacity: 60,
			Items:    []cargoItemStack{},
		},
	}
}

func (state playerRuntimeState) playerSnapshot() playerSnapshotPayload {
	return playerSnapshotPayload{
		Callsign:  state.Callsign,
		Rank:      state.Rank,
		HP:        state.Ship.Hull,
		MaxHP:     state.Ship.MaxHull,
		Shield:    state.Ship.Shield,
		MaxShield: state.Ship.MaxShield,
		Energy:    state.Ship.Capacitor,
		MaxEnergy: state.Ship.MaxCapacitor,
	}
}

func commandErrors(result worker.TickResult) error {
	if len(result.CommandErrors) == 0 && len(result.ScheduledTaskErrors) == 0 {
		return nil
	}
	if len(result.CommandErrors) > 0 {
		return result.CommandErrors[0].Err
	}
	return result.ScheduledTaskErrors[0].Err
}

func hasRole(roles []auth.Role, want auth.Role) bool {
	for _, role := range roles {
		if role == want {
			return true
		}
	}
	return false
}

func roleStrings(roles []auth.Role) []string {
	if len(roles) == 0 {
		return nil
	}
	out := make([]string, 0, len(roles))
	for _, role := range roles {
		out = append(out, string(role))
	}
	return out
}

type runtimeSessionResolver struct {
	runtime *Runtime
}

func (resolver runtimeSessionResolver) ResolveSession(sessionID realtime.SessionID) (realtime.CommandContext, error) {
	if resolver.runtime == nil || resolver.runtime.Auth == nil {
		return realtime.CommandContext{}, errors.New("nil runtime session resolver")
	}
	resolved, err := resolver.runtime.Auth.ResolveSessionID(context.Background(), auth.SessionID(sessionID.String()))
	if err != nil {
		return realtime.CommandContext{}, err
	}
	return realtime.CommandContext{
		SessionID: sessionID,
		PlayerID:  resolved.PlayerID,
		WorldID:   resolver.runtime.worldID,
		ZoneID:    resolver.runtime.zoneID,
	}, nil
}

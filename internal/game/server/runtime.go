package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"gameproject/internal/game/admin"
	"gameproject/internal/game/auction"
	"gameproject/internal/game/auth"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/crafting"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/loot"
	"gameproject/internal/game/market"
	"gameproject/internal/game/modules"
	"gameproject/internal/game/observability"
	"gameproject/internal/game/premium"
	"gameproject/internal/game/production"
	"gameproject/internal/game/progression"
	"gameproject/internal/game/quests"
	"gameproject/internal/game/realtime"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/stats"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	"gameproject/internal/game/world/visibility"
	"gameproject/internal/game/world/worker"
)

const (
	starterShipID                      foundation.ShipID = ships.ShipIDStarter
	starterShipDisplayName                               = "Sparrow"
	defaultPlayerSpeed                                   = 180
	defaultRadarRange                                    = 420
	runtimeLiveProjectionDiameter                        = 2000.0
	runtimeLiveProjectionHalfExtent                      = runtimeLiveProjectionDiameter / 2
	runtimeLiveProjectionDiagonalRange                   = runtimeLiveProjectionHalfExtent * math.Sqrt2
	defaultMaxMoveDistance                               = 1200
	runtimeLootPickupRange                               = 120.0
	runtimeBasicLaserEnergyCost                          = 10
	runtimeBasicLaserCooldownMS                          = 350
	minMoveCommandInterval                               = 75 * time.Millisecond
	runtimeHangarSafeRadius                              = 250.0
	runtimeStealthSpeedMultiplier                        = 0.70
	starterScannerItemID                                 = "scanner_t1"
	starterScannerModuleID                               = "scanner_t1"
	starterScannerScanPower                              = 500
	starterScannerScanRadius                             = 2000
	starterScannerScanInterval                           = time.Second
	starterScannerEnergyCost                             = 8
	runtimeHiddenPlayerWitnessDuration                   = 15 * time.Minute
	starterWalletCredits                                 = 1200
	starterWalletPremiumPaid                             = 300
	weeklyXCorePremiumPrice                              = 100
	weeklyXCoreStockTotal                                = 5
	runtimeQuestRewardLedgerReason                       = economy.LedgerReason("quest_reward")
	runtimeSectorKey                                     = "origin-fringe"
	runtimeProjectionSourceWorker                        = "worker_projection"
	runtimeProjectionSourceKnownIntel                    = "known_intel"
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

	players               map[foundation.PlayerID]playerRuntimeState
	hidden                map[world.EntityID]bool
	hiddenPlayers         map[foundation.PlayerID]bool
	stealthBaseSpeeds     map[foundation.PlayerID]float64
	hiddenPlayerWitnesses map[hiddenPlayerWitnessKey]time.Time
	eventSeq              map[auth.SessionID]uint64
	sessions              map[auth.SessionID]foundation.PlayerID
	lastAOI               map[auth.SessionID]aoi.Snapshot
	lastMove              map[foundation.PlayerID]time.Time
	queuedEvents          map[auth.SessionID][]realtime.EventEnvelope

	nextPlayerEntity int

	Combat        *combat.Service
	Loot          *loot.Service
	Inventory     *economy.InventoryService
	CargoService  *economy.CargoService
	Wallet        *economy.WalletService
	Market        *market.MarketService
	Auction       *auction.Service
	Premium       *premium.PremiumEntitlementService
	Quest         *quests.QuestService
	Admin         *admin.Service
	Progression   *progression.ProgressionService
	ShipCatalog   ships.Catalog
	HangarStore   *ships.InMemoryHangarStore
	Hangar        *ships.HangarService
	ModuleCatalog modules.Catalog
	Content       catalog.ContentRegistry
	LoadoutStore  *modules.InMemoryLoadoutStore
	Loadout       modules.LoadoutService
	Recipes       crafting.RecipeCatalog
	Discovery     *discovery.InMemoryStore
	Scanner       *discovery.ScannerService
	Production    *production.InMemoryStore
	CommandLog    *observability.MemoryCommandLogger
	Metrics       *observability.MetricRecorder

	combatXP            *combat.NPCKillXPHandler
	lootTable           loot.LootTable
	itemCatalog         map[foundation.ItemID]economy.ItemDefinition
	repairAttempts      map[foundation.IdempotencyKey]repairAttemptRecord
	shopPurchases       map[foundation.IdempotencyKey]shopPurchaseRecord
	scanCooldowns       map[scanCooldownKey]time.Time
	scanCapacitorSpends map[discovery.ScanPulseReference]scanCapacitorSpendRecord
}

type hiddenPlayerWitnessKey struct {
	ViewerPlayerID foundation.PlayerID
	TargetPlayerID foundation.PlayerID
}

type scanCooldownKey struct {
	PlayerID foundation.PlayerID
	ShipID   foundation.ShipID
	WorldID  foundation.WorldID
	ZoneID   foundation.ZoneID
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
	Speed                float64 `json:"speed"`
	RadarRange           float64 `json:"radar_range"`
	WeaponRange          float64 `json:"weapon_range"`
	CargoCapacity        int64   `json:"cargo_capacity"`
	LootPickupRange      float64 `json:"loot_pickup_range"`
	BasicLaserEnergyCost int     `json:"basic_laser_energy_cost"`
	BasicLaserCooldownMS int     `json:"basic_laser_cooldown_ms"`
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
	ItemID       string `json:"item_id"`
	DisplayName  string `json:"display_name"`
	Category     string `json:"category"`
	ArtKey       string `json:"art_key"`
	Rarity       string `json:"rarity,omitempty"`
	Quantity     int64  `json:"quantity"`
	UnitWeight   int64  `json:"unit_weight"`
	UsedUnits    int64  `json:"used_units"`
	Location     string `json:"location"`
	MoveEligible bool   `json:"move_eligible"`
	LockedReason string `json:"locked_reason,omitempty"`
}

type progressionSnapshotPayload struct {
	MainLevel   int   `json:"main_level"`
	MainXP      int64 `json:"main_xp"`
	Rank        int   `json:"rank"`
	CombatLevel int   `json:"combat_level,omitempty"`
	CombatXP    int64 `json:"combat_xp,omitempty"`
}

type worldSnapshotPayload struct {
	Sector         sectorPayload       `json:"sector"`
	Entities       []aoi.EntityPayload `json:"entities"`
	Minimap        minimapPayload      `json:"minimap"`
	SnapshotCursor uint64              `json:"snapshot_cursor"`
}

type sectorPayload struct {
	SectorKey string `json:"sector_key,omitempty"`
	Name      string `json:"name"`
	Region    string `json:"region"`
	Danger    string `json:"danger"`
	Contested bool   `json:"contested"`
}

type minimapPayload struct {
	RadarRange           float64                 `json:"radar_range"`
	ProjectionWindowSize float64                 `json:"projection_window_size"`
	LiveContacts         []minimapContactPayload `json:"live_contacts"`
	Remembered           []minimapMemoryPayload  `json:"remembered"`
}

type minimapContactPayload struct {
	EntityID         string           `json:"entity_id"`
	EntityType       world.EntityType `json:"entity_type"`
	Position         world.Vec2       `json:"position"`
	Disposition      string           `json:"disposition,omitempty"`
	StatusFlags      []aoi.StatusFlag `json:"status_flags,omitempty"`
	ProjectionSource string           `json:"projection_source"`
}

type minimapMemoryPayload struct {
	Kind             string     `json:"kind"`
	SectorKey        string     `json:"sector_key,omitempty"`
	PlanetID         string     `json:"planet_id,omitempty"`
	DetailID         string     `json:"detail_id,omitempty"`
	Label            string     `json:"label"`
	Position         world.Vec2 `json:"position"`
	Freshness        string     `json:"freshness"`
	ProjectionSource string     `json:"projection_source"`
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
	inventory := economy.NewInventoryService(clock)
	cargoService := economy.NewCargoService(inventory)
	walletService := economy.NewWalletService(clock)
	progressionService := progression.NewProgressionService(clock, nil)
	shipCatalog, err := ships.MVPShipCatalog()
	if err != nil {
		return nil, err
	}
	hangarStore := ships.NewInMemoryHangarStore()
	hangarService, err := ships.NewHangarService(
		shipCatalog,
		hangarStore,
		runtimeShipRankProvider{progression: progressionService},
		ships.BaseShipCargoCapacityProvider{},
		clock,
	)
	if err != nil {
		return nil, err
	}
	lootService, err := loot.NewService(loot.Config{
		Clock:       clock,
		Cargo:       cargoService,
		Progression: progressionService,
		PickupRange: runtimeLootPickupRange,
	})
	if err != nil {
		return nil, err
	}
	combatService := combat.NewService(clock, nil)
	combatXP, err := combat.NewNPCKillXPHandler(progressionService, combat.DefaultNPCKillXPReward())
	if err != nil {
		return nil, err
	}
	moduleCatalog := modules.MustMVPCatalog()
	lootTable, itemCatalog, err := runtimeLootCatalog()
	if err != nil {
		return nil, err
	}
	if err := appendRuntimeModuleItems(itemCatalog, moduleCatalog); err != nil {
		return nil, err
	}
	contentRegistry, err := buildRuntimeContentRegistry(itemCatalog, moduleCatalog, shipCatalog)
	if err != nil {
		return nil, err
	}
	recipeCatalog, err := crafting.MVPRecipeCatalog()
	if err != nil {
		return nil, err
	}
	loadoutStore := modules.NewInMemoryLoadoutStoreWithItemMover(runtimeModuleItemMover{
		inventory:   inventory,
		itemCatalog: itemCatalog,
	})
	loadoutService, err := modules.NewLoadoutService(
		moduleCatalog,
		loadoutStore,
		runtimeShipSlotLayoutProvider{},
		runtimeLoadoutProgressionProvider{progression: progressionService},
		clock,
	)
	if err != nil {
		return nil, err
	}
	marketService, err := market.NewMarketService(market.MarketServiceConfig{
		Clock:     clock,
		Inventory: inventory,
		Wallet:    walletService,
	})
	if err != nil {
		return nil, err
	}
	auctionService, err := auction.NewService(auction.ServiceConfig{
		Clock:  clock,
		Wallet: walletService,
	})
	if err != nil {
		return nil, err
	}
	premiumService, err := premium.NewPremiumEntitlementService(walletService, clock)
	if err != nil {
		return nil, err
	}
	questService, err := quests.NewQuestService(clock, quests.MustMVPQuestCatalog(), quests.NewInMemoryQuestStore())
	if err != nil {
		return nil, err
	}
	questService.SetRewardServices(quests.QuestRewardServices{
		Wallet:      walletService,
		Inventory:   questRewardInventoryAdapter{inventory: inventory, itemCatalog: itemCatalog},
		Progression: progressionService,
	})
	questService.SetRerollServices(quests.QuestRerollServices{Wallet: walletService})
	discoveryStore := discovery.NewInMemoryStore()
	productionStore := production.NewInMemoryStore()
	adminService := admin.NewService(admin.ServiceConfig{
		Inventory:  inventory,
		Wallet:     walletService,
		Market:     marketService,
		Auction:    auctionService,
		Production: productionStore,
		Clock:      clock,
	})
	commandLogger := observability.NewMemoryCommandLogger()
	metricRecorder := observability.NewMetricRecorder()
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
		hiddenPlayers:         make(map[foundation.PlayerID]bool),
		stealthBaseSpeeds:     make(map[foundation.PlayerID]float64),
		hiddenPlayerWitnesses: make(map[hiddenPlayerWitnessKey]time.Time),
		eventSeq:              make(map[auth.SessionID]uint64),
		sessions:              make(map[auth.SessionID]foundation.PlayerID),
		lastAOI:               make(map[auth.SessionID]aoi.Snapshot),
		lastMove:              make(map[foundation.PlayerID]time.Time),
		queuedEvents:          make(map[auth.SessionID][]realtime.EventEnvelope),
		Combat:                combatService,
		Loot:                  lootService,
		Inventory:             inventory,
		CargoService:          cargoService,
		Wallet:                walletService,
		Market:                marketService,
		Auction:               auctionService,
		Premium:               premiumService,
		Quest:                 questService,
		Admin:                 adminService,
		Progression:           progressionService,
		ShipCatalog:           shipCatalog,
		HangarStore:           hangarStore,
		Hangar:                hangarService,
		ModuleCatalog:         moduleCatalog,
		Content:               contentRegistry,
		LoadoutStore:          loadoutStore,
		Loadout:               loadoutService,
		Recipes:               recipeCatalog,
		Discovery:             discoveryStore,
		Production:            productionStore,
		CommandLog:            commandLogger,
		Metrics:               metricRecorder,
		combatXP:              combatXP,
		lootTable:             lootTable,
		itemCatalog:           itemCatalog,
		repairAttempts:        make(map[foundation.IdempotencyKey]repairAttemptRecord),
		shopPurchases:         make(map[foundation.IdempotencyKey]shopPurchaseRecord),
		scanCooldowns:         make(map[scanCooldownKey]time.Time),
		scanCapacitorSpends:   make(map[discovery.ScanPulseReference]scanCapacitorSpendRecord),
	}
	scannerSeed, err := discovery.NewWorldSeed(discovery.WorldSeedInput{
		StaticSeed: []byte("phase07-static-seed"),
	})
	if err != nil {
		return nil, err
	}
	scanner, err := discovery.NewScannerService(discovery.ScannerServiceConfig{
		Store:     discoveryStore,
		WorldSeed: scannerSeed,
		Clock:     clock,
		Modules:   runtimeScannerModuleProvider{runtime: runtime},
		Stats:     runtimeScannerStatsProvider{runtime: runtime},
		Positions: runtimeScannerPositionProvider{runtime: runtime},
		Cooldowns: runtimeScannerCooldownProvider{runtime: runtime},
		Energy:    runtimeScannerEnergyProvider{runtime: runtime},
		Reveals:   runtimeScannerPlayerRevealProvider{runtime: runtime},
		XP:        runtimeScanXPProvider{progression: progressionService},
		CandidateOptions: discovery.CandidateGenerationOptions{
			DiscoveryHorizon: 200_000,
			SpawnBudget:      8,
			ScanCellSize:     discovery.DefaultScanCellSize,
		},
		RadarLevelUnit:    defaultRadarRange,
		DiscoveryXPAmount: 25,
	})
	if err != nil {
		return nil, err
	}
	runtime.Scanner = scanner
	if err := runtime.seedWorld(); err != nil {
		return nil, err
	}
	if err := runtime.seedSharedEconomy(); err != nil {
		return nil, err
	}
	gateway, err := realtime.NewGateway(realtime.GatewayOptions{
		Clock:    clock,
		Sessions: runtimeSessionResolver{runtime: runtime},
		Executor: realtime.ObservedCommandExecutor{
			Clock:   clock,
			Logger:  commandLogger,
			Metrics: metricRecorder,
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
	if err := runtime.Combat.UpsertActor(runtime.trainingNPCActor(visible)); err != nil {
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
	if err := runtime.ensurePlayerHangarLocked(resolved.PlayerID); err != nil {
		return err
	}
	if err := runtime.ensurePlayerEconomyLocked(resolved.PlayerID); err != nil {
		return err
	}
	if _, err := runtime.syncPlayerCombatActorLocked(resolved.PlayerID); err != nil {
		return err
	}
	runtime.sessions[resolved.SessionID] = resolved.PlayerID
	return nil
}

func (runtime *Runtime) ensurePlayerHangarLocked(playerID foundation.PlayerID) error {
	result, err := runtime.Hangar.EnsureStarterShip(playerID)
	if err != nil {
		return err
	}
	if result.HasActiveShip {
		return runtime.applyActiveShipLocked(playerID, result.ActiveShip.ShipID)
	}
	return nil
}

func (runtime *Runtime) applyActiveShipLocked(playerID foundation.PlayerID, shipID foundation.ShipID) error {
	definition, err := runtime.ShipCatalog.MustGet(shipID)
	if err != nil {
		return err
	}
	state, ok := runtime.players[playerID]
	if !ok {
		return worker.ErrUnknownPlayer
	}

	previousShipID := state.Ship.ActiveShipID
	state.Ship.ActiveShipID = shipID.String()
	if shipID == starterShipID {
		state.Ship.DisplayName = starterShipDisplayName
	} else {
		state.Ship.DisplayName = definition.Name
	}
	if previousShipID != "" && previousShipID != shipID.String() {
		state.Ship.Hull = int(definition.BaseStats.HP)
		state.Ship.MaxHull = int(definition.BaseStats.HP)
		state.Ship.Shield = int(definition.BaseStats.Shield)
		state.Ship.MaxShield = int(definition.BaseStats.Shield)
		state.Ship.Capacitor = int(definition.BaseStats.Energy)
		state.Ship.MaxCapacitor = int(definition.BaseStats.Energy)
		baseSpeed := float64(definition.BaseStats.Speed)
		if runtime.hiddenPlayers[playerID] {
			runtime.stealthBaseSpeeds[playerID] = baseSpeed
			state.Stats.Speed = runtimePlayerSpeedForStealth(baseSpeed, true)
		} else {
			state.Stats.Speed = baseSpeed
		}
		state.Stats.RadarRange = float64(definition.BaseStats.Radar)
		state.Stats.CargoCapacity = definition.BaseStats.CargoCapacity
		state.Cargo.Capacity = definition.BaseStats.CargoCapacity
	}
	if state.Ship.RepairState == "" {
		state.Ship.RepairState = "ready"
	}
	runtime.players[playerID] = state
	return runtime.LoadoutStore.SetActiveShip(playerID, shipID)
}

func (runtime *Runtime) shipSwapContextLocked(playerID foundation.PlayerID) ships.ShipSwapContext {
	state := runtime.players[playerID]
	return ships.ShipSwapContext{
		InSafeHangarArea:  runtime.playerInSafeHangarAreaLocked(playerID),
		InCombat:          runtime.playerInCombatLocked(playerID),
		CurrentCargoUnits: state.Cargo.Used,
	}
}

func (runtime *Runtime) playerInSafeHangarAreaLocked(playerID foundation.PlayerID) bool {
	entity, ok := runtime.Worker.PlayerEntity(playerID)
	if !ok {
		return false
	}
	if entity.Movement.Moving {
		return false
	}
	return math.Hypot(entity.Position.X, entity.Position.Y) <= runtimeHangarSafeRadius
}

func (runtime *Runtime) playerInCombatLocked(playerID foundation.PlayerID) bool {
	state, ok := runtime.players[playerID]
	if !ok {
		return false
	}
	return state.Ship.Disabled || state.Ship.RepairState == "disabled"
}

func (runtime *Runtime) setPlayerStealth(playerID foundation.PlayerID, enabled bool) error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.setPlayerStealthLocked(playerID, enabled)
}

func (runtime *Runtime) setPlayerStealthLocked(playerID foundation.PlayerID, enabled bool) error {
	state, ok := runtime.players[playerID]
	if !ok {
		return worker.ErrUnknownPlayer
	}
	baseSpeed := runtime.stealthBaseSpeedLocked(playerID, state)
	speed := runtimePlayerSpeedForStealth(baseSpeed, enabled)
	if err := runtime.Worker.Submit(worker.SetPlayerSpeedCommand{PlayerID: playerID, Speed: speed}); err != nil {
		return err
	}
	result := runtime.Worker.Tick()
	if len(result.CommandErrors) > 0 {
		return result.CommandErrors[0].Err
	}
	state.Stats.Speed = speed
	runtime.players[playerID] = state
	if enabled {
		runtime.hiddenPlayers[playerID] = true
		runtime.stealthBaseSpeeds[playerID] = baseSpeed
	} else {
		delete(runtime.hiddenPlayers, playerID)
		delete(runtime.stealthBaseSpeeds, playerID)
	}
	return nil
}

func (runtime *Runtime) stealthBaseSpeedLocked(playerID foundation.PlayerID, state playerRuntimeState) float64 {
	if baseSpeed := runtime.stealthBaseSpeeds[playerID]; baseSpeed > 0 {
		return baseSpeed
	}
	if state.Stats.Speed > 0 {
		if runtime.hiddenPlayers[playerID] {
			return state.Stats.Speed / runtimeStealthSpeedMultiplier
		}
		return state.Stats.Speed
	}
	return defaultPlayerSpeed
}

func runtimePlayerSpeedForStealth(baseSpeed float64, enabled bool) float64 {
	if baseSpeed <= 0 {
		baseSpeed = defaultPlayerSpeed
	}
	if enabled {
		return baseSpeed * runtimeStealthSpeedMultiplier
	}
	return baseSpeed
}

func (runtime *Runtime) detachSession(sessionID auth.SessionID) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	_ = runtime.Worker.Submit(worker.SettleAndDetachSessionCommand{SessionID: realtime.SessionID(sessionID.String())})
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
	progressionSnapshot, err := runtime.Progression.GetProgressionSnapshot(resolved.PlayerID)
	if err != nil {
		return nil, err
	}
	runtime.lastAOI[resolved.SessionID] = aoi.Snapshot{Entities: cloneAOIEntities(worldSnapshot.Entities)}
	events := make([]realtime.EventEnvelope, 0, 8)
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
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventWalletSnapshot, runtime.walletSnapshotLocked(resolved.PlayerID)))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventCargoSnapshot, state.Cargo))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventProgressionSnapshot, progressionPayload(progressionSnapshot)))
	events = append(events, runtime.eventLocked(resolved.SessionID, realtime.EventWorldSnapshot, worldSnapshot))
	return events, nil
}

func (runtime *Runtime) postCommandEvents(sessionID auth.SessionID, op realtime.Operation, playerID foundation.PlayerID) ([]realtime.EventEnvelope, error) {
	eventsBySession, err := runtime.postCommandEventsBySession(sessionID, op, playerID)
	if err != nil {
		return nil, err
	}
	return eventsBySession[sessionID], nil
}

func (runtime *Runtime) postCommandEventsBySession(sessionID auth.SessionID, op realtime.Operation, playerID foundation.PlayerID) (map[auth.SessionID][]realtime.EventEnvelope, error) {
	switch op {
	case realtime.OperationMoveTo, realtime.OperationStop:
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		entity, ok := runtime.Worker.PlayerEntity(playerID)
		if !ok {
			return nil, worker.ErrUnknownPlayer
		}
		now := runtime.clock.Now()
		payload := map[string]any{
			"entity_id": entity.ID.String(),
			"position":  entity.Position,
		}
		if movement := runtime.publicMovementPayloadLocked(entity, now); movement != nil {
			payload["movement"] = movement
		}
		events := []realtime.EventEnvelope{runtime.eventAtLocked(sessionID, realtime.EventPositionCorrected, payload, now)}
		if op == realtime.OperationStop {
			events = append(events, runtime.eventAtLocked(sessionID, realtime.EventMovementStopped, payload, now))
		}
		events = append(events, runtime.aoiDiffEventsLocked(sessionID, playerID)...)
		return map[auth.SessionID][]realtime.EventEnvelope{sessionID: events}, nil
	case realtime.OperationCombatUseSkill,
		realtime.OperationLootPickup,
		realtime.OperationDeathRepairQuote,
		realtime.OperationDeathRepairShip,
		realtime.OperationHangarActivateShip,
		realtime.OperationLoadoutEquipModule,
		realtime.OperationLoadoutUnequipModule,
		realtime.OperationStealthToggle,
		realtime.OperationScanPulse,
		realtime.OperationMarketCreateListing,
		realtime.OperationMarketBuy,
		realtime.OperationMarketCancel,
		realtime.OperationAuctionBid,
		realtime.OperationAuctionBuyNow,
		realtime.OperationAuctionGrants,
		realtime.OperationPremiumClaim,
		realtime.OperationPremiumWeeklyXCore,
		realtime.OperationQuestBoard,
		realtime.OperationQuestAccept,
		realtime.OperationQuestClaimReward,
		realtime.OperationQuestReroll,
		realtime.OperationAdminRepairCraftJob,
		realtime.OperationObservabilityMetric,
		realtime.OperationObservabilityGate:
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		eventsBySession := runtime.drainQueuedEventsBySessionLocked()
		actorEvents := eventsBySession[sessionID]
		if opEmitsPostCommandAOIDiff(op) {
			actorEvents = append(actorEvents, runtime.aoiDiffEventsLocked(sessionID, playerID)...)
		}
		if len(actorEvents) > 0 {
			if eventsBySession == nil {
				eventsBySession = make(map[auth.SessionID][]realtime.EventEnvelope, 1)
			}
			eventsBySession[sessionID] = actorEvents
		}
		return eventsBySession, nil
	default:
		return nil, nil
	}
}

func opEmitsPostCommandAOIDiff(op realtime.Operation) bool {
	switch op {
	case realtime.OperationMarketCreateListing,
		realtime.OperationMarketBuy,
		realtime.OperationMarketCancel,
		realtime.OperationAuctionBid,
		realtime.OperationAuctionBuyNow:
		return false
	default:
		return true
	}
}

func (runtime *Runtime) eventLocked(sessionID auth.SessionID, eventType realtime.ClientEventType, payload any) realtime.EventEnvelope {
	return runtime.eventAtLocked(sessionID, eventType, payload, runtime.clock.Now())
}

func (runtime *Runtime) eventAtLocked(sessionID auth.SessionID, eventType realtime.ClientEventType, payload any, at time.Time) realtime.EventEnvelope {
	runtime.eventSeq[sessionID]++
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{}`)
	}
	return realtime.NewEventEnvelope(
		foundation.EventID(fmt.Sprintf("event_%d", runtime.eventSeq[sessionID])),
		eventType,
		data,
		at.UTC().UnixMilli(),
		runtime.eventSeq[sessionID],
	)
}

func (runtime *Runtime) worldSnapshotLocked(playerID foundation.PlayerID) (worldSnapshotPayload, error) {
	snapshot, radarRange, tick, err := runtime.aoiSnapshotForPlayerLocked(playerID)
	if err != nil {
		return worldSnapshotPayload{}, err
	}
	minimap, err := runtime.minimapForPlayerLocked(playerID, snapshot, radarRange)
	if err != nil {
		return worldSnapshotPayload{}, err
	}
	return worldSnapshotPayload{
		Sector:         runtime.sectorPayloadLocked(),
		Entities:       cloneAOIEntities(snapshot.Entities),
		Minimap:        minimap,
		SnapshotCursor: tick,
	}, nil
}

func (runtime *Runtime) currentMinimapPayload(playerID foundation.PlayerID) (minimapPayload, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	snapshot, radarRange, _, err := runtime.aoiSnapshotForPlayerLocked(playerID)
	if err != nil {
		return minimapPayload{}, err
	}
	return runtime.minimapForPlayerLocked(playerID, snapshot, radarRange)
}

func (runtime *Runtime) aoiSnapshotForPlayerLocked(playerID foundation.PlayerID) (aoi.Snapshot, float64, uint64, error) {
	playerEntity, ok := runtime.Worker.PlayerEntity(playerID)
	if !ok {
		return aoi.Snapshot{}, 0, 0, worker.ErrUnknownPlayer
	}
	now := runtime.clock.Now()
	statSnapshot := stats.NewStatSnapshot(playerID, starterShipID, 1, stats.EffectiveStats{
		Exploration: stats.ExplorationStats{RadarRange: runtimeLiveProjectionDiagonalRange},
	}, now)
	projectionRange := visibility.RadarRangeFromStatSnapshot(statSnapshot)
	viewer := visibility.Viewer{
		PlayerID:   playerID,
		WorldID:    runtime.worldID,
		ZoneID:     runtime.zoneID,
		Position:   playerEntity.Position,
		RadarRange: projectionRange,
		Witnesses:  runtime.hiddenPlayerWitnessesForViewerLocked(playerID, now),
		ObservedAt: now,
	}
	workerSnapshot, err := runtime.Worker.EntitiesWithinWindow(playerEntity.Position, runtimeLiveProjectionHalfExtent)
	if err != nil {
		return aoi.Snapshot{}, 0, 0, err
	}
	states := make([]aoi.EntityState, 0, len(workerSnapshot.Entities))
	for _, entity := range workerSnapshot.Entities {
		flags, display, combatStatus := runtime.publicEntityMetadataLocked(playerID, entity)
		entityPlayerID, _, _ := runtime.playerByEntityLocked(entity.ID)
		hidden := runtime.hidden[entity.ID]
		if !entityPlayerID.IsZero() && runtime.hiddenPlayers[entityPlayerID] {
			hidden = true
			if entityPlayerID != playerID && runtime.hiddenPlayerWitnessActiveLocked(playerID, entityPlayerID, now) {
				flags = append(flags, "scan_revealed")
			}
		}
		states = append(states, aoi.EntityState{
			Entity:            entity,
			PlayerID:          entityPlayerID,
			Signature:         visibility.EntitySignature(1),
			Hidden:            hidden,
			PublicStatusFlags: flags,
			PublicDisplay:     display,
			PublicCombat:      combatStatus,
			PublicMovement:    runtime.publicMovementPayloadLocked(entity, now),
			ProjectionSource:  runtimeProjectionSourceWorker,
		})
	}
	snapshot := aoi.BuildVisibleSnapshot(viewer, states)
	return snapshot, runtimeLiveProjectionHalfExtent, workerSnapshot.Tick, nil
}

func (runtime *Runtime) hiddenPlayerWitnessesForViewerLocked(viewerID foundation.PlayerID, now time.Time) []visibility.Witness {
	witnesses := make([]visibility.Witness, 0)
	for key, expiresAt := range runtime.hiddenPlayerWitnesses {
		if !expiresAt.After(now) {
			delete(runtime.hiddenPlayerWitnesses, key)
			continue
		}
		if key.ViewerPlayerID != viewerID {
			continue
		}
		witnesses = append(witnesses, visibility.Witness{
			TargetPlayerID: key.TargetPlayerID,
			ExpiresAt:      expiresAt,
		})
	}
	return witnesses
}

func (runtime *Runtime) hiddenPlayerWitnessActiveLocked(viewerID foundation.PlayerID, targetID foundation.PlayerID, now time.Time) bool {
	expiresAt, ok := runtime.hiddenPlayerWitnesses[hiddenPlayerWitnessKey{
		ViewerPlayerID: viewerID,
		TargetPlayerID: targetID,
	}]
	if !ok {
		return false
	}
	if !expiresAt.After(now) {
		delete(runtime.hiddenPlayerWitnesses, hiddenPlayerWitnessKey{
			ViewerPlayerID: viewerID,
			TargetPlayerID: targetID,
		})
		return false
	}
	return true
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

func (runtime *Runtime) recordCurrencyLedgerMetric(entry economy.CurrencyLedgerEntry) {
	if runtime == nil || runtime.Metrics == nil || entry.LedgerID.IsZero() {
		return
	}
	_ = runtime.Metrics.RecordWalletDelta(
		entry.Reason.String(),
		entry.Currency.String(),
		entry.Action.String(),
		entry.Amount.Int64(),
	)
}

func (runtime *Runtime) recordItemLedgerMetrics(entries []economy.ItemLedgerEntry) {
	if runtime == nil || runtime.Metrics == nil {
		return
	}
	for _, entry := range entries {
		if entry.LedgerID.IsZero() {
			continue
		}
		_ = runtime.Metrics.RecordItemDelta(
			entry.Reason.String(),
			entry.ItemID,
			entry.Action.String(),
			entry.Quantity.Int64(),
		)
	}
}

func (runtime *Runtime) recordQuestRewardMetrics(result quests.ClaimRewardResult) {
	if runtime == nil || runtime.Metrics == nil || result.Duplicate {
		return
	}
	for _, grant := range result.Quest.RewardPayload.Grants {
		_ = runtime.Metrics.RecordQuestReward(grant.Kind.String())
	}
	itemReason := runtimeQuestRewardLedgerReason
	if result.Credits != nil {
		itemReason = result.Credits.LedgerEntry.Reason
		runtime.recordCurrencyLedgerMetric(result.Credits.LedgerEntry)
	}
	if result.Items != nil {
		for _, item := range result.Items.Items {
			_ = runtime.Metrics.RecordItemDelta(itemReason.String(), item.ItemID, economy.LedgerActionIncrease.String(), item.Quantity)
		}
	}
}

func (runtime *Runtime) publicEntityMetadataLocked(viewerID foundation.PlayerID, entity world.Entity) ([]aoi.StatusFlag, *aoi.EntityDisplay, *aoi.EntityCombatStatus) {
	switch entity.Type {
	case world.EntityTypePlayer:
		if playerID, playerState, ok := runtime.playerByEntityLocked(entity.ID); ok {
			if playerID == viewerID {
				flags := []aoi.StatusFlag{"friendly", "self"}
				if runtime.hiddenPlayers[playerID] {
					flags = append(flags, "stealthed")
				}
				return flags, &aoi.EntityDisplay{Label: playerState.Callsign, Disposition: "self"}, nil
			}
			return []aoi.StatusFlag{"friendly"}, &aoi.EntityDisplay{Label: playerState.Callsign, Disposition: "friendly"}, nil
		}
		return []aoi.StatusFlag{"friendly"}, &aoi.EntityDisplay{Label: "Pilot", Disposition: "friendly"}, nil
	case world.EntityTypeNPC:
		flags := []aoi.StatusFlag{"hostile"}
		combatStatus := runtime.entityCombatStatusLocked(entity.ID)
		if combatStatus == nil {
			combatStatus = combatStatusFromActor(runtime.trainingNPCActor(entity))
		}
		if combatStatus != nil && combatStatus.HP < combatStatus.MaxHP {
			flags = append(flags, "damaged")
		}
		return flags, &aoi.EntityDisplay{Label: displayLabelForEntity(entity.ID, "Training Drone"), Disposition: "hostile"}, combatStatus
	case world.EntityTypeLoot:
		return []aoi.StatusFlag{"loot"}, &aoi.EntityDisplay{Label: "Loot Cache", Disposition: "neutral"}, nil
	case world.EntityTypePlanetSignal:
		return []aoi.StatusFlag{"unknown_signal"}, &aoi.EntityDisplay{Label: "Unknown Signal", Disposition: "unknown"}, nil
	default:
		return nil, nil, nil
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
		SectorKey: runtimeSectorKey,
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
			EntityID:         entity.ID.String(),
			EntityType:       entity.Type,
			Position:         entity.Position,
			Disposition:      disposition,
			StatusFlags:      append([]aoi.StatusFlag(nil), entity.StatusFlags...),
			ProjectionSource: runtimeProjectionSourceWorker,
		})
	}
	return minimapPayload{
		RadarRange:           radarRange,
		ProjectionWindowSize: runtimeLiveProjectionDiameter,
		LiveContacts:         contacts,
		Remembered:           []minimapMemoryPayload{},
	}
}

func (runtime *Runtime) minimapForPlayerLocked(playerID foundation.PlayerID, snapshot aoi.Snapshot, radarRange float64) (minimapPayload, error) {
	payload := minimapFromAOI(snapshot, radarRange)
	remembered, err := runtime.rememberedMinimapPayloadLocked(playerID)
	if err != nil {
		return minimapPayload{}, err
	}
	payload.Remembered = remembered
	return payload, nil
}

func (runtime *Runtime) rememberedMinimapPayloadLocked(playerID foundation.PlayerID) ([]minimapMemoryPayload, error) {
	intelRows, err := runtime.Discovery.PlayerPlanetIntelRecords(playerID)
	if err != nil {
		return nil, err
	}
	remembered := make([]minimapMemoryPayload, 0, len(intelRows))
	for _, intel := range intelRows {
		planet, ok, err := runtime.Discovery.Planet(intel.PlanetID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		remembered = append(remembered, minimapMemoryPayload{
			Kind:             "known_planet",
			SectorKey:        runtimeSectorKey,
			PlanetID:         intel.PlanetID.String(),
			DetailID:         intel.PlanetID.String(),
			Label:            planetMemoryLabel(planet),
			Position:         intel.Coordinates,
			Freshness:        string(intel.State),
			ProjectionSource: runtimeProjectionSourceKnownIntel,
		})
	}
	return remembered, nil
}

func planetMemoryLabel(planet discovery.Planet) string {
	if planet.Type != "" && planet.Biome != "" {
		return string(planet.Type) + " " + string(planet.Biome)
	}
	if planet.Type != "" {
		return string(planet.Type)
	}
	return planet.ID.String()
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
		if entity.Combat != nil {
			combatStatus := *entity.Combat
			entity.Combat = &combatStatus
		}
		if entity.Movement != nil {
			movementStatus := *entity.Movement
			entity.Movement = &movementStatus
		}
		cloned = append(cloned, entity)
	}
	return cloned
}

func (runtime *Runtime) publicMovementPayloadLocked(entity world.Entity, _ time.Time) *aoi.EntityMovementStatus {
	if !entity.Movement.Moving {
		return nil
	}
	if entity.Movement.Speed <= 0 || entity.Movement.ArriveAtMS < entity.Movement.StartedAtMS {
		return nil
	}
	return &aoi.EntityMovementStatus{
		Moving:      true,
		Origin:      entity.Movement.Origin,
		Target:      entity.Movement.Target,
		Speed:       entity.Movement.Speed,
		StartedAtMS: entity.Movement.StartedAtMS,
		ArriveAtMS:  entity.Movement.ArriveAtMS,
	}
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
			Speed:                defaultPlayerSpeed,
			RadarRange:           defaultRadarRange,
			WeaponRange:          260,
			CargoCapacity:        60,
			LootPickupRange:      runtimeLootPickupRange,
			BasicLaserEnergyCost: runtimeBasicLaserEnergyCost,
			BasicLaserCooldownMS: runtimeBasicLaserCooldownMS,
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

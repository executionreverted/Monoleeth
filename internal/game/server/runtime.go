package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/admin"
	"gameproject/internal/game/auction"
	"gameproject/internal/game/auth"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/combat"
	"gameproject/internal/game/crafting"
	deathdomain "gameproject/internal/game/death"
	"gameproject/internal/game/discovery"
	"gameproject/internal/game/economy"
	"gameproject/internal/game/foundation"
	"gameproject/internal/game/intel"
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
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

const (
	starterShipID                      foundation.ShipID = ships.ShipIDStarter
	starterShipDisplayName                               = "Sparrow"
	defaultPlayerSpeed                                   = 180
	defaultRadarRange                                    = 420
	defaultMaxMoveDistance                               = 1200
	runtimeLootPickupRange                               = 120.0
	runtimeBasicLaserEnergyCost                          = 10
	runtimeBasicLaserCooldownMS                          = 350
	minMoveCommandInterval                               = 75 * time.Millisecond
	runtimeStealthSpeedMultiplier                        = 0.70
	starterScannerItemID                                 = "scanner_t1"
	starterScannerModuleID                               = "scanner_t1"
	starterScannerScanPower                              = 500
	starterScannerScanRadius                             = 2000
	starterScannerScanInterval                           = time.Second
	starterScannerEnergyCost                             = 8
	runtimeHiddenPlayerWitnessDuration                   = 15 * time.Minute
	runtimePortalCooldown                                = 30 * time.Second
	runtimePortalProtectionDuration                      = 10 * time.Second
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
	Clock              foundation.Clock
	RNG                foundation.RNG
	SessionTTL         time.Duration
	TickDelta          time.Duration
	WorldID            foundation.WorldID
	ZoneID             foundation.ZoneID
	DevMode            bool
	E2EPlanetClaimSeed bool
	E2ERouteSeed       bool
	AdminSeed          auth.AdminSeedInput
	Passwords          auth.PasswordHasher
}

// Runtime composes auth, realtime gateway, and the Phase 02 world worker.
type Runtime struct {
	mu sync.Mutex

	// buildingMutationMu serializes in-process building build/upgrade commands
	// so wallet debit cannot outrun production commit.
	buildingMutationMu sync.Mutex

	clock              foundation.Clock
	devMode            bool
	e2ePlanetClaimSeed bool
	e2eRouteSeed       bool

	Auth    *auth.Service
	Gateway *realtime.Gateway
	Worker  *worker.Worker

	worldID      foundation.WorldID
	zoneID       foundation.ZoneID
	mapCatalog   *worldmaps.Catalog
	mapRouter    *worldmaps.Router
	mapInstances map[worldmaps.MapID]*mapInstance

	players           map[foundation.PlayerID]playerRuntimeState
	stealthBaseSpeeds map[foundation.PlayerID]float64
	eventSeq          map[auth.SessionID]uint64
	sessions          map[auth.SessionID]foundation.PlayerID
	sessionLocations  map[auth.SessionID]worldmaps.MapID
	sessionEpochs     map[auth.SessionID]uint64
	nextSessionEpoch  uint64
	lastMove          map[foundation.PlayerID]time.Time
	queuedEvents      map[auth.SessionID][]realtime.EventEnvelope
	activeTransfers   map[foundation.PlayerID]portalTransferState
	activeScanPulses  map[foundation.PlayerID]scanPulseMapGuard
	portalCooldowns   map[portalCooldownKey]time.Time
	portalAttempts    map[portalRequestKey]portalTransferRecord
	playerProtections map[protectionKey]playerProtectionState
	pendingRespawns   map[foundation.PlayerID]pendingRespawnTarget

	nextPlayerEntity int

	Combat        *combat.Service
	Death         *deathdomain.DeathService
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
	Claim         *discovery.ClaimService
	Intel         *intel.Service
	Production    *production.InMemoryStore
	CommandLog    *observability.MemoryCommandLogger
	Metrics       *observability.MetricRecorder

	combatXP            *combat.NPCKillXPHandler
	lootTables          map[string]loot.LootTable
	itemCatalog         map[foundation.ItemID]economy.ItemDefinition
	repairAttempts      map[foundation.IdempotencyKey]repairAttemptRecord
	shopPurchases       map[foundation.IdempotencyKey]shopPurchaseRecord
	scanCooldowns       map[scanCooldownKey]time.Time
	scanCapacitorSpends map[discovery.ScanPulseReference]scanCapacitorSpendRecord
}

type scanCooldownKey struct {
	PlayerID foundation.PlayerID
	ShipID   foundation.ShipID
	WorldID  foundation.WorldID
	ZoneID   foundation.ZoneID
}

// NewRuntime creates the single-process runtime.
func NewRuntime(config RuntimeConfig) (*Runtime, error) {
	if config.E2EPlanetClaimSeed && !config.DevMode {
		return nil, fmt.Errorf("%s requires %s=true", EnvE2EPlanetClaimSeed, EnvDevMode)
	}
	if config.E2ERouteSeed && !config.DevMode {
		return nil, fmt.Errorf("%s requires %s=true", EnvE2ERouteSeed, EnvDevMode)
	}
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	rng := config.RNG
	if rng == nil {
		rng = newRuntimeRNG(clock.Now().UnixNano())
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
	mapCatalog, err := worldmaps.StarterCatalog(config.WorldID)
	if err != nil {
		return nil, err
	}
	mapRouter, err := worldmaps.NewRouter(mapCatalog)
	if err != nil {
		return nil, err
	}
	starterMap, _, err := mapCatalog.StarterDefinition()
	if err != nil {
		return nil, err
	}
	if config.ZoneID != "" && config.ZoneID != starterMap.ZoneID {
		return nil, fmt.Errorf("initial zone_id %q must equal starter internal map id %q", config.ZoneID, starterMap.InternalMapID)
	}
	mapInstances := make(map[worldmaps.MapID]*mapInstance)
	var zoneWorker *worker.Worker
	for _, definition := range mapCatalog.Definitions() {
		instanceWorker, err := worker.NewWorker(worker.Config{
			WorldID:   definition.WorldID,
			ZoneID:    definition.ZoneID,
			TickDelta: config.TickDelta,
			Clock:     clock,
		})
		if err != nil {
			return nil, err
		}
		mapInstances[definition.InternalMapID] = &mapInstance{
			Definition:            definition,
			Worker:                instanceWorker,
			ActiveSessions:        make(map[auth.SessionID]foundation.PlayerID),
			LastAOI:               make(map[auth.SessionID]aoi.Snapshot),
			HiddenEntities:        make(map[world.EntityID]bool),
			HiddenPlayers:         make(map[foundation.PlayerID]bool),
			HiddenPlayerWitnesses: make(map[hiddenPlayerWitnessKey]time.Time),
		}
		if definition.InternalMapID == worldmaps.StarterMapID {
			zoneWorker = instanceWorker
		}
	}
	if zoneWorker == nil {
		return nil, fmt.Errorf("starter map instance: %w", errMapInstanceNotFound)
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
	lootTables, itemCatalog, err := runtimeLootCatalog()
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
	deathService, err := deathdomain.NewDeathService(deathdomain.Config{
		Clock:           clock,
		RNG:             rng,
		Inventory:       inventory,
		Loot:            lootService,
		Ships:           hangarService,
		EquippedModules: loadoutService,
	})
	if err != nil {
		return nil, err
	}
	inventory.SetCargoTransferGuard(deathService)
	cargoService.SetCargoTransferGuard(deathService)
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
		clock:               clock,
		devMode:             config.DevMode,
		e2ePlanetClaimSeed:  config.E2EPlanetClaimSeed,
		e2eRouteSeed:        config.E2ERouteSeed,
		Auth:                authService,
		Worker:              zoneWorker,
		worldID:             config.WorldID,
		zoneID:              starterMap.ZoneID,
		mapCatalog:          mapCatalog,
		mapRouter:           mapRouter,
		mapInstances:        mapInstances,
		players:             make(map[foundation.PlayerID]playerRuntimeState),
		stealthBaseSpeeds:   make(map[foundation.PlayerID]float64),
		eventSeq:            make(map[auth.SessionID]uint64),
		sessions:            make(map[auth.SessionID]foundation.PlayerID),
		sessionLocations:    make(map[auth.SessionID]worldmaps.MapID),
		sessionEpochs:       make(map[auth.SessionID]uint64),
		lastMove:            make(map[foundation.PlayerID]time.Time),
		queuedEvents:        make(map[auth.SessionID][]realtime.EventEnvelope),
		activeTransfers:     make(map[foundation.PlayerID]portalTransferState),
		activeScanPulses:    make(map[foundation.PlayerID]scanPulseMapGuard),
		portalCooldowns:     make(map[portalCooldownKey]time.Time),
		portalAttempts:      make(map[portalRequestKey]portalTransferRecord),
		playerProtections:   make(map[protectionKey]playerProtectionState),
		pendingRespawns:     make(map[foundation.PlayerID]pendingRespawnTarget),
		Combat:              combatService,
		Death:               deathService,
		Loot:                lootService,
		Inventory:           inventory,
		CargoService:        cargoService,
		Wallet:              walletService,
		Market:              marketService,
		Auction:             auctionService,
		Premium:             premiumService,
		Quest:               questService,
		Admin:               adminService,
		Progression:         progressionService,
		ShipCatalog:         shipCatalog,
		HangarStore:         hangarStore,
		Hangar:              hangarService,
		ModuleCatalog:       moduleCatalog,
		Content:             contentRegistry,
		LoadoutStore:        loadoutStore,
		Loadout:             loadoutService,
		Recipes:             recipeCatalog,
		Discovery:           discoveryStore,
		Intel:               intel.NewService(clock),
		Production:          productionStore,
		CommandLog:          commandLogger,
		Metrics:             metricRecorder,
		combatXP:            combatXP,
		lootTables:          lootTables,
		itemCatalog:         itemCatalog,
		repairAttempts:      make(map[foundation.IdempotencyKey]repairAttemptRecord),
		shopPurchases:       make(map[foundation.IdempotencyKey]shopPurchaseRecord),
		scanCooldowns:       make(map[scanCooldownKey]time.Time),
		scanCapacitorSpends: make(map[discovery.ScanPulseReference]scanCapacitorSpendRecord),
	}
	scannerSeed, err := discovery.NewWorldSeed(discovery.WorldSeedInput{
		StaticSeed: []byte("phase07-static-seed"),
	})
	if err != nil {
		return nil, err
	}
	scannerBounds := worldmaps.ExactPlayableBounds()
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
			ProfileVersion: "runtime_phase06_bounded_v1",
			MapBounds: discovery.CandidateMapBounds{
				MinX: scannerBounds.MinX,
				MinY: scannerBounds.MinY,
				MaxX: scannerBounds.MaxX,
				MaxY: scannerBounds.MaxY,
			},
			LevelMin:     1,
			LevelMax:     4,
			Density:      1,
			SpawnBudget:  8,
			ScanCellSize: discovery.DefaultScanCellSize,
		},
		RadarLevelUnit:    defaultRadarRange,
		DiscoveryXPAmount: 25,
	})
	if err != nil {
		return nil, err
	}
	runtime.Scanner = scanner
	xCoreDefinition, ok := itemCatalog["x_core"]
	if !ok {
		return nil, fmt.Errorf("x_core definition missing")
	}
	claimProductionInitializer, err := production.NewClaimProductionInitializer(production.ClaimProductionInitializerConfig{
		Store: productionStore,
		Defaults: production.ClaimProductionInitializationDefaults{
			StorageCapacityUnits:  runtimeClaimProductionStorageCapacity,
			EnergyCapacityPerHour: runtimeClaimProductionEnergyCapacity,
		},
	})
	if err != nil {
		return nil, err
	}
	claimService, err := discovery.NewClaimService(discovery.ClaimServiceConfig{
		Store:                 discoveryStore,
		Clock:                 clock,
		Ranks:                 runtimeClaimRankProvider{progression: progressionService},
		Proximity:             runtimeClaimProximityProvider{runtime: runtime},
		XCoreConsumer:         runtimeClaimXCoreConsumer{inventory: inventory},
		ProductionInitializer: claimProductionInitializer,
		XCoreItemDefinition:   xCoreDefinition,
	})
	if err != nil {
		return nil, err
	}
	runtime.Claim = claimService
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
	if runtime == nil || len(runtime.mapInstances) == 0 {
		return
	}
	ticker := time.NewTicker(runtime.tickDelta())
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

func (runtime *Runtime) tickDelta() time.Duration {
	if runtime == nil {
		return worker.DefaultTickDelta
	}
	for _, instance := range runtime.mapInstances {
		if instance != nil && instance.Worker != nil {
			return instance.Worker.TickDelta()
		}
	}
	if runtime.Worker != nil {
		return runtime.Worker.TickDelta()
	}
	return worker.DefaultTickDelta
}

func (runtime *Runtime) seedWorld() error {
	for mapID, instance := range runtime.mapInstances {
		if instance == nil || instance.Worker == nil {
			return fmt.Errorf("map %q: %w", mapID, errMapInstanceNotFound)
		}
		overrides := map[worldmaps.EnemyPoolID][]world.EntityID(nil)
		if mapID == worldmaps.StarterMapID {
			overrides = map[worldmaps.EnemyPoolID][]world.EntityID{
				"starter_training_drone_pool": {"entity_training_npc"},
			}
		}
		if err := runtime.submitWorkerCommandAndRecordMetricsLocked(instance, worker.InitializeEnemyPoolsCommand{
			Definition:        instance.Definition,
			EntityIDOverrides: overrides,
		}); err != nil {
			return err
		}
		for _, record := range instance.Worker.EnemySpawnSnapshot().Records {
			if !record.Alive {
				continue
			}
			entity, ok := instance.Worker.Entity(record.EntityID)
			if !ok {
				return fmt.Errorf("spawned npc %q: %w", record.EntityID, worker.ErrUnknownEntity)
			}
			if _, err := runtime.upsertNPCCombatActorProjectionLocked(instance, entity); err != nil {
				return err
			}
		}

		spawnPosition := world.Vec2{}
		if len(instance.Definition.SpawnPoints) > 0 {
			spawnPosition = instance.Definition.SpawnPoints[0].Position
		}
		hiddenPosition := boundedOffset(instance.Definition.Bounds, spawnPosition, world.Vec2{X: 120, Y: 0})
		hiddenID := world.EntityID("entity_hidden_planet_signal_" + mapID.String())
		if mapID == worldmaps.StarterMapID {
			hiddenID = "entity_hidden_planet_signal"
		}
		hidden, err := world.NewEntity(instance.Definition.WorldID, instance.Definition.ZoneID, hiddenID, world.EntityTypePlanetSignalPlaceholder, hiddenPosition)
		if err != nil {
			return err
		}
		if err := instance.Worker.InsertEntity(hidden, 0); err != nil {
			return err
		}
		instance.HiddenEntities[hidden.ID] = true
	}
	return nil
}

func boundedOffset(bounds worldmaps.Bounds, origin world.Vec2, offset world.Vec2) world.Vec2 {
	position := world.Vec2{X: origin.X + offset.X, Y: origin.Y + offset.Y}
	if position.X < bounds.MinX {
		position.X = bounds.MinX
	}
	if position.Y < bounds.MinY {
		position.Y = bounds.MinY
	}
	if position.X > bounds.MaxX {
		position.X = bounds.MaxX
	}
	if position.Y > bounds.MaxY {
		position.Y = bounds.MaxY
	}
	return position
}

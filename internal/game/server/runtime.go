package server

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gameproject/internal/game/admin"
	"gameproject/internal/game/auction"
	"gameproject/internal/game/auth"
	"gameproject/internal/game/catalog"
	"gameproject/internal/game/combat"
	gamecontent "gameproject/internal/game/content"
	"gameproject/internal/game/contentdb"
	"gameproject/internal/game/contentseed"
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
	gameruntime "gameproject/internal/game/runtime"
	"gameproject/internal/game/ships"
	"gameproject/internal/game/world"
	"gameproject/internal/game/world/aoi"
	worldmaps "gameproject/internal/game/world/maps"
	"gameproject/internal/game/world/worker"
)

const (
	starterShipID                      foundation.ShipID = gamecontent.DefaultStarterShipID
	starterShipDisplayName                               = gamecontent.DefaultStarterShipDisplayName
	defaultPlayerSpeed                                   = gamecontent.DefaultPlayerSpeed
	defaultRadarRange                                    = gamecontent.DefaultRadarRange
	defaultMaxMoveDistance                               = 1200
	runtimeLootPickupRange                               = gamecontent.DefaultLootPickupRange
	runtimeBasicLaserEnergyCost                          = gamecontent.DefaultBasicLaserEnergyCost
	runtimeBasicLaserCooldownMS                          = gamecontent.DefaultBasicLaserCooldownMS
	minMoveCommandInterval                               = 75 * time.Millisecond
	runtimeStealthSpeedMultiplier                        = 0.70
	starterScannerItemID                                 = gamecontent.DefaultStarterScannerItemID
	starterScannerModuleID                               = gamecontent.DefaultStarterScannerModuleID
	starterScannerScanPower                              = gamecontent.DefaultStarterScannerScanPower
	starterScannerScanRadius                             = gamecontent.DefaultStarterScannerScanRadius
	starterScannerScanInterval                           = gamecontent.DefaultStarterScannerScanInterval
	starterScannerEnergyCost                             = gamecontent.DefaultStarterScannerEnergyCost
	runtimeHiddenPlayerWitnessDuration                   = 15 * time.Minute
	runtimePortalCooldown                                = 30 * time.Second
	runtimePortalProtectionDuration                      = 10 * time.Second
	starterWalletCredits                                 = gamecontent.DefaultStarterWalletCredits
	starterWalletPremiumPaid                             = gamecontent.DefaultStarterWalletPremiumPaid
	weeklyXCorePremiumPrice                              = gamecontent.DefaultWeeklyXCorePremiumPrice
	weeklyXCoreStockTotal                                = gamecontent.DefaultWeeklyXCoreStockTotal
	runtimeQuestRewardLedgerReason                       = economy.LedgerReason("quest_reward")
	runtimeSectorKey                                     = "origin-fringe"
	runtimeProjectionSourceWorker                        = "worker_projection"
	runtimeProjectionSourceKnownIntel                    = "known_intel"
)

// RuntimeConfig wires the single-process game runtime.
type RuntimeConfig struct {
	Clock               foundation.Clock
	RNG                 foundation.RNG
	SessionTTL          time.Duration
	TickDelta           time.Duration
	WorldID             foundation.WorldID
	ZoneID              foundation.ZoneID
	PlaytestSeed        bool
	DevMode             bool
	ContentDB           contentdb.Config
	CoreStoreMode       contentdb.ContentMode
	ContentRepository   gamecontent.Repository
	ContentAdmin        *admin.ContentService
	E2EPlanetClaimSeed  bool
	E2EPlanetClaimCores int
	E2ERouteSeed        bool
	E2EScanNoPlanetSeed bool
	AdminSeed           auth.AdminSeedInput
	Passwords           auth.PasswordHasher

	realtimeLimiter        realtime.RateLimiter
	disableRealtimeLimiter bool
	contentDBOpen          func(context.Context, contentdb.Config) (runtimeContentStore, error)
	contentRepositoryStore func(runtimeContentStore) (gamecontent.Repository, error)
	contentSeedSnapshot    func(world.WorldID) (gamecontent.Snapshot, error)
}

// Runtime composes auth, realtime gateway, and the Phase 02 world worker.
type Runtime struct {
	// mu is the runtime-coordinator lock. Map workers own per-map live entity
	// mutation; Runtime.mu is not the per-map command/tick gate. It serializes
	// runtime-owned cross-worker metadata: session/player and session/map routing,
	// replay/queued-event maps, player runtime bookkeeping, per-instance
	// session/AOI cursors, and transitional command guards/cooldowns still held
	// on Runtime until decomposition.
	mu sync.Mutex

	// buildingMutationMu serializes in-process building build/upgrade commands
	// so wallet debit cannot outrun production commit.
	buildingMutationMu sync.Mutex
	tickMu             sync.Mutex

	clock               foundation.Clock
	devMode             bool
	playtestSeed        bool
	e2ePlanetClaimSeed  bool
	e2ePlanetClaimCores int
	e2eRouteSeed        bool

	Auth    *auth.Service
	Gateway *realtime.Gateway
	Worker  *worker.Worker

	worldID    foundation.WorldID
	zoneID     foundation.ZoneID
	mapCatalog *worldmaps.Catalog
	mapRouter  *worldmaps.Router
	// mapInstances is populated at boot and treated as immutable. Runtime.mu
	// protects mutable per-instance routing/projection cursors such as
	// ActiveSessions, LastAOI, and hidden-visibility overlays.
	mapInstances     map[worldmaps.MapID]*mapInstance
	mapTickInstances []*mapInstance

	players            map[foundation.PlayerID]playerRuntimeState
	stealthBaseSpeeds  map[foundation.PlayerID]float64
	eventSeq           map[auth.SessionID]uint64
	eventRings         map[auth.SessionID]*sessionEventRing
	sessions           map[auth.SessionID]foundation.PlayerID
	sessionLocations   map[auth.SessionID]worldmaps.MapID
	sessionEpochs      map[auth.SessionID]uint64
	nextSessionEpoch   uint64
	lastMove           map[foundation.PlayerID]time.Time
	queuedEvents       map[auth.SessionID][]realtime.EventEnvelope
	activeTransfers    map[foundation.PlayerID]portalTransferState
	activeScanPulses   map[foundation.PlayerID]scanPulseMapGuard
	activePlanetClaims map[foundation.PlanetID]int
	portalCooldowns    map[portalCooldownKey]time.Time
	portalAttempts     map[portalRequestKey]portalTransferRecord
	playerProtections  map[protectionKey]playerProtectionState
	pendingRespawns    map[foundation.PlayerID]pendingRespawnTarget
	combatLocks        map[foundation.PlayerID]time.Time
	shieldRepairTicks  map[foundation.PlayerID]time.Time

	nextPlayerEntity int

	Combat                         *combat.Service
	Death                          *deathdomain.DeathService
	Loot                           *loot.Service
	Inventory                      *economy.InventoryService
	CargoService                   *economy.CargoService
	Wallet                         *economy.WalletService
	Market                         *market.MarketService
	Auction                        *auction.Service
	Premium                        *premium.PremiumEntitlementService
	Quest                          *quests.QuestService
	Admin                          *admin.Service
	ContentAdmin                   *admin.ContentService
	Progression                    *progression.ProgressionService
	ShipCatalog                    ships.Catalog
	HangarStore                    ships.HangarStore
	Hangar                         *ships.HangarService
	ModuleCatalog                  modules.Catalog
	Content                        catalog.ContentRegistry
	LoadoutStore                   runtimeLoadoutStore
	Loadout                        modules.LoadoutService
	StatInputs                     *gameruntime.StatInputProvider
	Recipes                        crafting.RecipeCatalog
	ProductionCatalog              production.Catalog
	Crafting                       *crafting.CraftingService
	Discovery                      *discovery.InMemoryStore
	Scanner                        *discovery.ScannerService
	Claim                          *discovery.ClaimService
	ClaimLifecycles                *discovery.InMemoryClaimDurableLifecycleStore
	ClaimProductionInitializations *discovery.InMemoryClaimProductionInitializationDurableStore
	Intel                          *intel.Service
	Production                     *production.InMemoryStore
	Settlements                    *production.InMemorySettlementDurableCommitStore
	BuildingMutations              *production.InMemoryBuildingMutationDurableCommitStore
	CommandLog                     *observability.MemoryCommandLogger
	Metrics                        *observability.MetricRecorder
	contentAdminCloser             func() error
	authStoreCloser                func() error
	walletStoreCloser              func() error
	inventoryStoreCloser           func() error
	progressionStoreCloser         func() error
	economyStoreCloser             func() error
	hangarStoreCloser              func() error
	loadoutStoreCloser             func() error

	combatXP                 *combat.NPCKillXPHandler
	lootTables               map[string]loot.LootTable
	itemCatalog              map[foundation.ItemID]economy.ItemDefinition
	starterContent           gamecontent.StarterContent
	routeContent             gamecontent.RouteContent
	productionRules          gamecontent.ProductionRulesContent
	combatRules              gamecontent.CombatRulesContent
	contentCatalogProjection gamecontent.PlayerContentProjection
	contentCatalogVersion    string
	repairAttempts           map[foundation.IdempotencyKey]repairAttemptRecord
	repairQuotes             map[foundation.PlayerID]repairQuoteRecord
	repairQuoteSeq           uint64
	shopPurchases            map[foundation.IdempotencyKey]shopPurchaseRecord
	scanCooldowns            map[scanCooldownKey]time.Time
	scanCapacitorSpends      map[discovery.ScanPulseReference]scanCapacitorSpendRecord
}

type scanCooldownKey struct {
	PlayerID foundation.PlayerID
	ShipID   foundation.ShipID
	WorldID  foundation.WorldID
	ZoneID   foundation.ZoneID
}

type runtimeContentStore interface {
	contentseed.SeedStore
	Migrate(context.Context, contentdb.MigrationMode) error
	Close() error
}

type runtimeContentVersionStore interface {
	runtimeContentStore
	admin.ContentVersionStore
	admin.ContentDraftStore
	admin.ContentDraftWriter
	admin.ContentPublisher
	admin.ContentSnapshotReader
	admin.ContentAuditStore
}

type runtimeEconomyStores struct {
	idempotency       economy.IdempotencyStore
	outbox            economy.OutboxStore
	marketRepository  market.MarketListingRepository
	auctionRepository auction.AuctionLotRepository
	premiumRepository premium.PremiumEntitlementRepository
	lootPickup        loot.LootPickupTransactionRepository
}

func loadRuntimeContent(ctx context.Context, config RuntimeConfig) (gamecontent.GameplayContent, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	contentConfig := config.ContentDB.WithDefaults()
	if err := contentConfig.Validate(); err != nil {
		return gamecontent.GameplayContent{}, err
	}
	if config.ContentRepository != nil {
		return gamecontent.LoadPublishedContent(ctx, config.ContentRepository, config.WorldID)
	}
	if contentConfig.Enabled() {
		return loadRuntimeContentFromDB(ctx, contentConfig, config)
	}
	if !config.DevMode {
		return gamecontent.GameplayContent{}, fmt.Errorf("content db: %w", contentdb.ErrContentDatabaseDisabled)
	}
	return gamecontent.LoadPublishedContent(ctx, gamecontent.NewStaticRepository(), config.WorldID)
}

func loadRuntimeContentFromDB(ctx context.Context, contentConfig contentdb.Config, config RuntimeConfig) (gamecontent.GameplayContent, error) {
	openStore := config.contentDBOpen
	if openStore == nil {
		openStore = defaultRuntimeContentDBOpen
	}
	store, err := openStore(ctx, contentConfig)
	if err != nil {
		return gamecontent.GameplayContent{}, err
	}
	if store == nil {
		return gamecontent.GameplayContent{}, contentdb.ErrNilDatabase
	}
	defer store.Close()

	if err := store.Migrate(ctx, contentConfig.Migrations); err != nil {
		return gamecontent.GameplayContent{}, fmt.Errorf("migrate content db: %w", err)
	}
	hasContent, err := store.HasAnyContent(ctx)
	if err != nil {
		return gamecontent.GameplayContent{}, fmt.Errorf("check content db seed state: %w", err)
	}
	if !hasContent {
		buildSnapshot := config.contentSeedSnapshot
		if buildSnapshot == nil {
			buildSnapshot = contentseed.BuildMVPSnapshot
		}
		snapshot, err := buildSnapshot(config.WorldID)
		if err != nil {
			return gamecontent.GameplayContent{}, fmt.Errorf("build content seed snapshot: %w", err)
		}
		if _, err := contentseed.EnsurePublishedSeed(ctx, store, snapshot, contentseed.SeedOptions{}); err != nil {
			return gamecontent.GameplayContent{}, err
		}
	}

	newRepository := config.contentRepositoryStore
	if newRepository == nil {
		newRepository = defaultRuntimeContentRepositoryStore
	}
	repository, err := newRepository(store)
	if err != nil {
		return gamecontent.GameplayContent{}, err
	}
	return gamecontent.LoadPublishedContent(ctx, repository, config.WorldID)
}

func defaultRuntimeContentDBOpen(ctx context.Context, config contentdb.Config) (runtimeContentStore, error) {
	db, err := contentdb.Open(ctx, config)
	if err != nil {
		return nil, err
	}
	store, err := contentdb.NewStore(db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func defaultRuntimeContentRepositoryStore(store runtimeContentStore) (gamecontent.Repository, error) {
	contentStore, ok := store.(*contentdb.Store)
	if !ok {
		return nil, fmt.Errorf("content db store %T: %w", store, contentdb.ErrNilDatabase)
	}
	return contentdb.NewRepository(contentStore)
}

func (runtime *Runtime) Close() error {
	if runtime == nil {
		return nil
	}
	var errs []error
	if runtime.contentAdminCloser != nil {
		closeContentAdmin := runtime.contentAdminCloser
		runtime.contentAdminCloser = nil
		errs = append(errs, closeContentAdmin())
	}
	if runtime.authStoreCloser != nil {
		closeAuthStore := runtime.authStoreCloser
		runtime.authStoreCloser = nil
		errs = append(errs, closeAuthStore())
	}
	if runtime.walletStoreCloser != nil {
		closeWalletStore := runtime.walletStoreCloser
		runtime.walletStoreCloser = nil
		errs = append(errs, closeWalletStore())
	}
	if runtime.inventoryStoreCloser != nil {
		closeInventoryStore := runtime.inventoryStoreCloser
		runtime.inventoryStoreCloser = nil
		errs = append(errs, closeInventoryStore())
	}
	if runtime.progressionStoreCloser != nil {
		closeProgressionStore := runtime.progressionStoreCloser
		runtime.progressionStoreCloser = nil
		errs = append(errs, closeProgressionStore())
	}
	if runtime.economyStoreCloser != nil {
		closeEconomyStore := runtime.economyStoreCloser
		runtime.economyStoreCloser = nil
		errs = append(errs, closeEconomyStore())
	}
	if runtime.hangarStoreCloser != nil {
		closeHangarStore := runtime.hangarStoreCloser
		runtime.hangarStoreCloser = nil
		errs = append(errs, closeHangarStore())
	}
	if runtime.loadoutStoreCloser != nil {
		closeLoadoutStore := runtime.loadoutStoreCloser
		runtime.loadoutStoreCloser = nil
		errs = append(errs, closeLoadoutStore())
	}
	return errors.Join(errs...)
}

func loadRuntimeAuthStore(ctx context.Context, config RuntimeConfig) (auth.Store, func() error, error) {
	contentConfig := runtimeCoreStoreDBConfig(config)
	if err := contentConfig.Validate(); err != nil {
		return nil, nil, err
	}
	if !contentConfig.Enabled() {
		return auth.NewInMemoryStore(), nil, nil
	}
	openStore := config.contentDBOpen
	if openStore == nil {
		openStore = defaultRuntimeContentDBOpen
	}
	store, err := openStore(ctx, contentConfig)
	if err != nil {
		return nil, nil, err
	}
	if store == nil {
		return nil, nil, contentdb.ErrNilDatabase
	}
	closeStore := func() error { return store.Close() }
	if err := store.Migrate(ctx, contentConfig.Migrations); err != nil {
		_ = closeStore()
		return nil, nil, fmt.Errorf("migrate auth db: %w", err)
	}
	contentStore, ok := store.(*contentdb.Store)
	if !ok {
		_ = closeStore()
		return nil, nil, fmt.Errorf("auth db store %T: %w", store, contentdb.ErrNilDatabase)
	}
	authStore, err := contentdb.NewAuthStore(contentStore)
	if err != nil {
		_ = closeStore()
		return nil, nil, err
	}
	return authStore, closeStore, nil
}

func loadRuntimeWalletStore(ctx context.Context, config RuntimeConfig) (economy.WalletRepository, func() error, error) {
	contentConfig := runtimeCoreStoreDBConfig(config)
	if err := contentConfig.Validate(); err != nil {
		return nil, nil, err
	}
	if !contentConfig.Enabled() {
		return nil, nil, nil
	}
	openStore := config.contentDBOpen
	if openStore == nil {
		openStore = defaultRuntimeContentDBOpen
	}
	store, err := openStore(ctx, contentConfig)
	if err != nil {
		return nil, nil, err
	}
	if store == nil {
		return nil, nil, contentdb.ErrNilDatabase
	}
	closeStore := func() error { return store.Close() }
	if err := store.Migrate(ctx, contentConfig.Migrations); err != nil {
		_ = closeStore()
		return nil, nil, fmt.Errorf("migrate wallet db: %w", err)
	}
	contentStore, ok := store.(*contentdb.Store)
	if !ok {
		_ = closeStore()
		return nil, nil, fmt.Errorf("wallet db store %T: %w", store, contentdb.ErrNilDatabase)
	}
	walletStore, err := contentdb.NewWalletStore(contentStore)
	if err != nil {
		_ = closeStore()
		return nil, nil, err
	}
	return walletStore, closeStore, nil
}

func loadRuntimeInventoryStore(ctx context.Context, config RuntimeConfig) (economy.InventoryRepository, func() error, error) {
	contentConfig := runtimeCoreStoreDBConfig(config)
	if err := contentConfig.Validate(); err != nil {
		return nil, nil, err
	}
	if !contentConfig.Enabled() {
		return nil, nil, nil
	}
	openStore := config.contentDBOpen
	if openStore == nil {
		openStore = defaultRuntimeContentDBOpen
	}
	store, err := openStore(ctx, contentConfig)
	if err != nil {
		return nil, nil, err
	}
	if store == nil {
		return nil, nil, contentdb.ErrNilDatabase
	}
	closeStore := func() error { return store.Close() }
	if err := store.Migrate(ctx, contentConfig.Migrations); err != nil {
		_ = closeStore()
		return nil, nil, fmt.Errorf("migrate inventory db: %w", err)
	}
	contentStore, ok := store.(*contentdb.Store)
	if !ok {
		_ = closeStore()
		return nil, nil, fmt.Errorf("inventory db store %T: %w", store, contentdb.ErrNilDatabase)
	}
	inventoryStore, err := contentdb.NewInventoryStore(contentStore)
	if err != nil {
		_ = closeStore()
		return nil, nil, err
	}
	return inventoryStore, closeStore, nil
}

func loadRuntimeProgressionStore(ctx context.Context, config RuntimeConfig) (progression.Repository, func() error, error) {
	contentConfig := runtimeCoreStoreDBConfig(config)
	if err := contentConfig.Validate(); err != nil {
		return nil, nil, err
	}
	if !contentConfig.Enabled() {
		return nil, nil, nil
	}
	openStore := config.contentDBOpen
	if openStore == nil {
		openStore = defaultRuntimeContentDBOpen
	}
	store, err := openStore(ctx, contentConfig)
	if err != nil {
		return nil, nil, err
	}
	if store == nil {
		return nil, nil, contentdb.ErrNilDatabase
	}
	closeStore := func() error { return store.Close() }
	if err := store.Migrate(ctx, contentConfig.Migrations); err != nil {
		_ = closeStore()
		return nil, nil, fmt.Errorf("migrate progression db: %w", err)
	}
	contentStore, ok := store.(*contentdb.Store)
	if !ok {
		_ = closeStore()
		return nil, nil, fmt.Errorf("progression db store %T: %w", store, contentdb.ErrNilDatabase)
	}
	progressionStore, err := contentdb.NewProgressionStore(contentStore)
	if err != nil {
		_ = closeStore()
		return nil, nil, err
	}
	return progressionStore, closeStore, nil
}

func loadRuntimeHangarStore(ctx context.Context, config RuntimeConfig) (ships.HangarStore, func() error, error) {
	contentConfig := runtimeCoreStoreDBConfig(config)
	if err := contentConfig.Validate(); err != nil {
		return nil, nil, err
	}
	if !contentConfig.Enabled() {
		return ships.NewInMemoryHangarStore(), nil, nil
	}
	openStore := config.contentDBOpen
	if openStore == nil {
		openStore = defaultRuntimeContentDBOpen
	}
	store, err := openStore(ctx, contentConfig)
	if err != nil {
		return nil, nil, err
	}
	if store == nil {
		return nil, nil, contentdb.ErrNilDatabase
	}
	closeStore := func() error { return store.Close() }
	if err := store.Migrate(ctx, contentConfig.Migrations); err != nil {
		_ = closeStore()
		return nil, nil, fmt.Errorf("migrate hangar db: %w", err)
	}
	contentStore, ok := store.(*contentdb.Store)
	if !ok {
		_ = closeStore()
		return nil, nil, fmt.Errorf("hangar db store %T: %w", store, contentdb.ErrNilDatabase)
	}
	hangarStore, err := contentdb.NewHangarStore(contentStore)
	if err != nil {
		_ = closeStore()
		return nil, nil, err
	}
	return hangarStore, closeStore, nil
}

func loadRuntimeLoadoutStore(
	ctx context.Context,
	config RuntimeConfig,
	inventory *economy.InventoryService,
	itemCatalog map[foundation.ItemID]economy.ItemDefinition,
) (runtimeLoadoutStore, func() error, error) {
	mover := runtimeModuleItemMover{
		inventory:   inventory,
		itemCatalog: itemCatalog,
	}
	contentConfig := runtimeCoreStoreDBConfig(config)
	if err := contentConfig.Validate(); err != nil {
		return nil, nil, err
	}
	if !contentConfig.Enabled() {
		return modules.NewInMemoryLoadoutStoreWithItemMover(mover), nil, nil
	}
	openStore := config.contentDBOpen
	if openStore == nil {
		openStore = defaultRuntimeContentDBOpen
	}
	store, err := openStore(ctx, contentConfig)
	if err != nil {
		return nil, nil, err
	}
	if store == nil {
		return nil, nil, contentdb.ErrNilDatabase
	}
	closeStore := func() error { return store.Close() }
	if err := store.Migrate(ctx, contentConfig.Migrations); err != nil {
		_ = closeStore()
		return nil, nil, fmt.Errorf("migrate loadout db: %w", err)
	}
	contentStore, ok := store.(*contentdb.Store)
	if !ok {
		_ = closeStore()
		return nil, nil, fmt.Errorf("loadout db store %T: %w", store, contentdb.ErrNilDatabase)
	}
	inventoryStore, err := contentdb.NewInventoryStore(contentStore)
	if err != nil {
		_ = closeStore()
		return nil, nil, err
	}
	loadoutStore, err := contentdb.NewLoadoutStoreWithItemMover(contentStore, runtimeDurableModuleItemMover{
		inventory:   inventory,
		itemCatalog: itemCatalog,
		repository:  inventoryStore,
	})
	if err != nil {
		_ = closeStore()
		return nil, nil, err
	}
	return runtimeDurableLoadoutStore{LoadoutStore: loadoutStore}, closeStore, nil
}

func loadRuntimeEconomyStores(ctx context.Context, config RuntimeConfig) (runtimeEconomyStores, func() error, error) {
	contentConfig := runtimeCoreStoreDBConfig(config)
	if err := contentConfig.Validate(); err != nil {
		return runtimeEconomyStores{}, nil, err
	}
	if !contentConfig.Enabled() {
		return runtimeEconomyStores{}, nil, nil
	}
	openStore := config.contentDBOpen
	if openStore == nil {
		openStore = defaultRuntimeContentDBOpen
	}
	store, err := openStore(ctx, contentConfig)
	if err != nil {
		return runtimeEconomyStores{}, nil, err
	}
	if store == nil {
		return runtimeEconomyStores{}, nil, contentdb.ErrNilDatabase
	}
	closeStore := func() error { return store.Close() }
	if err := store.Migrate(ctx, contentConfig.Migrations); err != nil {
		_ = closeStore()
		return runtimeEconomyStores{}, nil, fmt.Errorf("migrate economy db: %w", err)
	}
	contentStore, ok := store.(*contentdb.Store)
	if !ok {
		_ = closeStore()
		return runtimeEconomyStores{}, nil, fmt.Errorf("economy db store %T: %w", store, contentdb.ErrNilDatabase)
	}
	marketRepository, err := contentdb.NewMarketListingStore(contentStore)
	if err != nil {
		_ = closeStore()
		return runtimeEconomyStores{}, nil, err
	}
	auctionRepository, err := contentdb.NewAuctionLotStore(contentStore)
	if err != nil {
		_ = closeStore()
		return runtimeEconomyStores{}, nil, err
	}
	premiumRepository, err := contentdb.NewPremiumEntitlementStore(contentStore)
	if err != nil {
		_ = closeStore()
		return runtimeEconomyStores{}, nil, err
	}
	lootPickupStore, err := contentdb.NewLootPickupStore(contentStore)
	if err != nil {
		_ = closeStore()
		return runtimeEconomyStores{}, nil, err
	}
	return runtimeEconomyStores{
		idempotency:       contentStore,
		outbox:            contentStore,
		marketRepository:  marketRepository,
		auctionRepository: auctionRepository,
		premiumRepository: premiumRepository,
		lootPickup:        lootPickupStore,
	}, closeStore, nil
}

func runtimeCoreStoreDBConfig(config RuntimeConfig) contentdb.Config {
	contentConfig := config.ContentDB.WithDefaults()
	mode := config.CoreStoreMode
	if mode == "" {
		if contentConfig.Enabled() {
			mode = contentdb.ContentModeRequired
		} else if config.DevMode {
			mode = contentdb.ContentModeDevFallback
		} else {
			mode = contentdb.ContentModeOff
		}
	}
	contentConfig.Mode = mode
	if mode == contentdb.ContentModeOff {
		contentConfig.DatabaseURL = ""
	}
	return contentConfig.WithDefaults()
}

func loadRuntimeContentAdmin(ctx context.Context, config RuntimeConfig, clock foundation.Clock) (*admin.ContentService, func() error, error) {
	if config.ContentAdmin != nil {
		return config.ContentAdmin, nil, nil
	}
	contentConfig := config.ContentDB.WithDefaults()
	if !contentConfig.Enabled() {
		return nil, nil, nil
	}
	openStore := config.contentDBOpen
	if openStore == nil {
		openStore = defaultRuntimeContentDBOpen
	}
	store, err := openStore(ctx, contentConfig)
	if err != nil {
		return nil, nil, err
	}
	if store == nil {
		return nil, nil, contentdb.ErrNilDatabase
	}
	closeStore := func() error { return store.Close() }
	if err := store.Migrate(ctx, contentConfig.Migrations); err != nil {
		_ = closeStore()
		return nil, nil, fmt.Errorf("migrate content admin db: %w", err)
	}
	versionStore, ok := store.(runtimeContentVersionStore)
	if !ok {
		_ = closeStore()
		return nil, nil, fmt.Errorf("content admin store %T: %w", store, contentdb.ErrNilDatabase)
	}
	return admin.NewContentService(admin.ContentServiceConfig{
		Versions:  versionStore,
		Drafts:    versionStore,
		Writer:    versionStore,
		Publisher: versionStore,
		Snapshots: versionStore,
		Audit:     versionStore,
		Validator: contentdb.NewSnapshotValidator(config.WorldID),
		Clock:     clock,
	}), closeStore, nil
}

// NewRuntime creates the single-process runtime.
func NewRuntime(config RuntimeConfig) (*Runtime, error) {
	if config.E2EPlanetClaimSeed && !config.DevMode {
		return nil, fmt.Errorf("%s requires %s=true", EnvE2EPlanetClaimSeed, EnvDevMode)
	}
	if config.E2ERouteSeed && !config.DevMode {
		return nil, fmt.Errorf("%s requires %s=true", EnvE2ERouteSeed, EnvDevMode)
	}
	if config.E2EScanNoPlanetSeed && !config.DevMode {
		return nil, fmt.Errorf("%s requires %s=true", EnvE2EScanNoPlanetSeed, EnvDevMode)
	}
	clock := config.Clock
	if clock == nil {
		clock = foundation.RealClock{}
	}
	rng := config.RNG
	if rng == nil {
		rng = newRuntimeRNG(clock.Now().UnixNano())
	}
	contentBundle, err := loadRuntimeContent(context.Background(), config)
	if err != nil {
		return nil, err
	}
	contentCatalogProjection, err := gamecontent.ProjectGameplayContentForPlayers(contentBundle)
	if err != nil {
		return nil, fmt.Errorf("project player content catalog: %w", err)
	}
	contentAdmin, contentAdminCloser, err := loadRuntimeContentAdmin(context.Background(), config, clock)
	if err != nil {
		return nil, err
	}
	authStore, authStoreCloser, err := loadRuntimeAuthStore(context.Background(), config)
	if err != nil {
		if contentAdminCloser != nil {
			_ = contentAdminCloser()
		}
		return nil, err
	}
	authService, err := auth.NewService(auth.ServiceConfig{
		Store:          authStore,
		Clock:          clock,
		PasswordHasher: config.Passwords,
		SessionTTL:     config.SessionTTL,
	})
	if err != nil {
		if authStoreCloser != nil {
			_ = authStoreCloser()
		}
		if contentAdminCloser != nil {
			_ = contentAdminCloser()
		}
		return nil, err
	}
	if config.AdminSeed.Enabled {
		if _, err := authService.SeedAdmin(context.Background(), config.AdminSeed); err != nil {
			if authStoreCloser != nil {
				_ = authStoreCloser()
			}
			if contentAdminCloser != nil {
				_ = contentAdminCloser()
			}
			return nil, err
		}
	}
	walletStore, walletStoreCloser, err := loadRuntimeWalletStore(context.Background(), config)
	if err != nil {
		if authStoreCloser != nil {
			_ = authStoreCloser()
		}
		if contentAdminCloser != nil {
			_ = contentAdminCloser()
		}
		return nil, err
	}
	inventoryStore, inventoryStoreCloser, err := loadRuntimeInventoryStore(context.Background(), config)
	if err != nil {
		if walletStoreCloser != nil {
			_ = walletStoreCloser()
		}
		if authStoreCloser != nil {
			_ = authStoreCloser()
		}
		if contentAdminCloser != nil {
			_ = contentAdminCloser()
		}
		return nil, err
	}
	progressionStore, progressionStoreCloser, err := loadRuntimeProgressionStore(context.Background(), config)
	if err != nil {
		if inventoryStoreCloser != nil {
			_ = inventoryStoreCloser()
		}
		if walletStoreCloser != nil {
			_ = walletStoreCloser()
		}
		if authStoreCloser != nil {
			_ = authStoreCloser()
		}
		if contentAdminCloser != nil {
			_ = contentAdminCloser()
		}
		return nil, err
	}
	economyStores, economyStoreCloser, err := loadRuntimeEconomyStores(context.Background(), config)
	if err != nil {
		if progressionStoreCloser != nil {
			_ = progressionStoreCloser()
		}
		if inventoryStoreCloser != nil {
			_ = inventoryStoreCloser()
		}
		if walletStoreCloser != nil {
			_ = walletStoreCloser()
		}
		if authStoreCloser != nil {
			_ = authStoreCloser()
		}
		if contentAdminCloser != nil {
			_ = contentAdminCloser()
		}
		return nil, err
	}
	closeEconomyStoreOnError := true
	defer func() {
		if closeEconomyStoreOnError && economyStoreCloser != nil {
			_ = economyStoreCloser()
		}
	}()
	mapCatalog := contentBundle.Maps
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
	var inventory *economy.InventoryService
	if inventoryStore != nil {
		inventory, err = economy.NewInventoryServiceWithRepository(clock, inventoryStore)
		if err != nil {
			if inventoryStoreCloser != nil {
				_ = inventoryStoreCloser()
			}
			if progressionStoreCloser != nil {
				_ = progressionStoreCloser()
			}
			if walletStoreCloser != nil {
				_ = walletStoreCloser()
			}
			if authStoreCloser != nil {
				_ = authStoreCloser()
			}
			if contentAdminCloser != nil {
				_ = contentAdminCloser()
			}
			return nil, err
		}
	} else {
		inventory = economy.NewInventoryService(clock)
	}
	cargoService := economy.NewCargoService(inventory)
	var walletService *economy.WalletService
	if walletStore != nil {
		walletService, err = economy.NewWalletServiceWithRepository(clock, walletStore)
		if err != nil {
			if walletStoreCloser != nil {
				_ = walletStoreCloser()
			}
			if authStoreCloser != nil {
				_ = authStoreCloser()
			}
			if contentAdminCloser != nil {
				_ = contentAdminCloser()
			}
			return nil, err
		}
	} else {
		walletService = economy.NewWalletService(clock)
	}
	progressionMemoryStore, err := progression.NewInMemoryProgressionStoreWithRepository(context.Background(), progressionStore)
	if err != nil {
		if progressionStoreCloser != nil {
			_ = progressionStoreCloser()
		}
		if inventoryStoreCloser != nil {
			_ = inventoryStoreCloser()
		}
		if walletStoreCloser != nil {
			_ = walletStoreCloser()
		}
		if authStoreCloser != nil {
			_ = authStoreCloser()
		}
		if contentAdminCloser != nil {
			_ = contentAdminCloser()
		}
		return nil, err
	}
	progressionService := progression.NewProgressionService(clock, progressionMemoryStore)
	shipCatalog := contentBundle.Ships
	hangarStore, hangarStoreCloser, err := loadRuntimeHangarStore(context.Background(), config)
	if err != nil {
		if progressionStoreCloser != nil {
			_ = progressionStoreCloser()
		}
		if inventoryStoreCloser != nil {
			_ = inventoryStoreCloser()
		}
		if walletStoreCloser != nil {
			_ = walletStoreCloser()
		}
		if authStoreCloser != nil {
			_ = authStoreCloser()
		}
		if contentAdminCloser != nil {
			_ = contentAdminCloser()
		}
		return nil, err
	}
	closeHangarStoreOnError := true
	defer func() {
		if closeHangarStoreOnError && hangarStoreCloser != nil {
			_ = hangarStoreCloser()
		}
	}()
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
		Clock:              clock,
		Cargo:              cargoService,
		Progression:        progressionService,
		XPOutbox:           economyStores.outbox,
		PickupTransactions: economyStores.lootPickup,
		PickupRange:        contentBundle.Combat.LootPickupRange,
	})
	if err != nil {
		return nil, err
	}
	combatService := combat.NewService(clock, nil)
	combatXP, err := combat.NewNPCKillXPHandler(progressionService, contentBundle.Combat.NPCKillXPReward())
	if err != nil {
		return nil, err
	}
	moduleCatalog := contentBundle.Modules
	itemCatalog, lootTables, err := contentBundle.RuntimeItemsAndLootTables()
	if err != nil {
		return nil, err
	}
	contentRegistry := contentBundle.Shop
	recipeCatalog := contentBundle.Recipes
	reservationService := economy.NewReservationService(inventory)
	discoveryStore := discovery.NewInMemoryStore()
	productionStore, err := production.NewInMemoryStoreWithCatalog(contentBundle.Production)
	if err != nil {
		return nil, err
	}
	craftLocationAuthorizer, err := production.NewCraftLocationAuthorizer(production.CraftLocationAuthorizerConfig{
		Planets:    discoveryStore,
		Production: productionStore,
	})
	if err != nil {
		return nil, err
	}
	loadoutStore, loadoutStoreCloser, err := loadRuntimeLoadoutStore(context.Background(), config, inventory, itemCatalog)
	if err != nil {
		return nil, err
	}
	closeLoadoutStoreOnError := true
	defer func() {
		if closeLoadoutStoreOnError && loadoutStoreCloser != nil {
			_ = loadoutStoreCloser()
		}
	}()
	loadoutService, err := modules.NewLoadoutService(
		moduleCatalog,
		loadoutStore,
		runtimeShipSlotLayoutProvider{shipCatalog: shipCatalog},
		runtimeLoadoutProgressionProvider{progression: progressionService},
		clock,
	)
	if err != nil {
		return nil, err
	}
	statInputs, err := gameruntime.NewStatInputProviderWithProgression(shipCatalog, moduleCatalog, loadoutStore, progressionService)
	if err != nil {
		return nil, err
	}
	craftingService, err := crafting.NewCraftingService(crafting.CraftingServiceConfig{
		Clock:              clock,
		Recipes:            recipeCatalog,
		ItemDefinitions:    crafting.ItemDefinitionMap(itemCatalog),
		Reservations:       reservationService,
		Inventory:          inventory,
		Wallet:             walletService,
		Progression:        progressionService,
		Ships:              hangarService,
		LocationAuthorizer: craftLocationAuthorizer,
		XPTracker:          crafting.NewInMemoryCraftXPTracker(),
	})
	if err != nil {
		return nil, err
	}
	if contentAdmin != nil {
		contentAdmin.SetPublishSafetyReaders(craftingService, productionStore)
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
		Clock:             clock,
		Inventory:         inventory,
		Wallet:            walletService,
		ListingRepository: economyStores.marketRepository,
		IdempotencyStore:  economyStores.idempotency,
		OutboxStore:       economyStores.outbox,
	})
	if err != nil {
		return nil, err
	}
	auctionService, err := auction.NewService(auction.ServiceConfig{
		Clock:            clock,
		Wallet:           walletService,
		LotRepository:    economyStores.auctionRepository,
		IdempotencyStore: economyStores.idempotency,
	})
	if err != nil {
		return nil, err
	}
	premiumService, err := premium.NewPremiumEntitlementServiceWithConfig(premium.PremiumEntitlementServiceConfig{
		Wallet:                walletService,
		Clock:                 clock,
		IdempotencyStore:      economyStores.idempotency,
		EntitlementRepository: economyStores.premiumRepository,
	})
	if err != nil {
		return nil, err
	}
	questService, err := quests.NewQuestService(clock, contentBundle.Quests, quests.NewInMemoryQuestStore())
	if err != nil {
		return nil, err
	}
	questService.SetRewardServices(quests.QuestRewardServices{
		Wallet:      walletService,
		Inventory:   questRewardInventoryAdapter{inventory: inventory, itemCatalog: itemCatalog},
		Progression: progressionService,
	})
	questService.SetRerollServices(quests.QuestRerollServices{Wallet: walletService})
	claimLifecycleStore := discovery.NewInMemoryClaimDurableLifecycleStore()
	claimProductionInitializationStore := discovery.NewInMemoryClaimProductionInitializationDurableStore()
	settlementStore := production.NewInMemorySettlementDurableCommitStore()
	buildingMutationStore := production.NewInMemoryBuildingMutationDurableCommitStore()
	intelService := intel.NewService(clock)
	adminService := admin.NewService(admin.ServiceConfig{
		Inventory:  inventory,
		Wallet:     walletService,
		Market:     marketService,
		Auction:    auctionService,
		Crafting:   craftingService,
		Production: productionStore,
		Clock:      clock,
	})
	commandLogger := observability.NewMemoryCommandLogger()
	metricRecorder := observability.NewMetricRecorder()
	runtime := &Runtime{
		clock:                          clock,
		devMode:                        config.DevMode,
		playtestSeed:                   config.PlaytestSeed,
		e2ePlanetClaimSeed:             config.E2EPlanetClaimSeed,
		e2ePlanetClaimCores:            e2ePlanetClaimCoreQuantity(config.E2EPlanetClaimCores),
		e2eRouteSeed:                   config.E2ERouteSeed,
		Auth:                           authService,
		Worker:                         zoneWorker,
		worldID:                        config.WorldID,
		zoneID:                         starterMap.ZoneID,
		mapCatalog:                     mapCatalog,
		mapRouter:                      mapRouter,
		mapInstances:                   mapInstances,
		mapTickInstances:               sortedMapInstances(mapInstances),
		players:                        make(map[foundation.PlayerID]playerRuntimeState),
		stealthBaseSpeeds:              make(map[foundation.PlayerID]float64),
		eventSeq:                       make(map[auth.SessionID]uint64),
		eventRings:                     make(map[auth.SessionID]*sessionEventRing),
		sessions:                       make(map[auth.SessionID]foundation.PlayerID),
		sessionLocations:               make(map[auth.SessionID]worldmaps.MapID),
		sessionEpochs:                  make(map[auth.SessionID]uint64),
		lastMove:                       make(map[foundation.PlayerID]time.Time),
		queuedEvents:                   make(map[auth.SessionID][]realtime.EventEnvelope),
		activeTransfers:                make(map[foundation.PlayerID]portalTransferState),
		activeScanPulses:               make(map[foundation.PlayerID]scanPulseMapGuard),
		activePlanetClaims:             make(map[foundation.PlanetID]int),
		portalCooldowns:                make(map[portalCooldownKey]time.Time),
		portalAttempts:                 make(map[portalRequestKey]portalTransferRecord),
		playerProtections:              make(map[protectionKey]playerProtectionState),
		pendingRespawns:                make(map[foundation.PlayerID]pendingRespawnTarget),
		combatLocks:                    make(map[foundation.PlayerID]time.Time),
		shieldRepairTicks:              make(map[foundation.PlayerID]time.Time),
		Combat:                         combatService,
		Death:                          deathService,
		Loot:                           lootService,
		Inventory:                      inventory,
		CargoService:                   cargoService,
		Wallet:                         walletService,
		Market:                         marketService,
		Auction:                        auctionService,
		Premium:                        premiumService,
		Quest:                          questService,
		Admin:                          adminService,
		ContentAdmin:                   contentAdmin,
		Progression:                    progressionService,
		ShipCatalog:                    shipCatalog,
		HangarStore:                    hangarStore,
		Hangar:                         hangarService,
		ModuleCatalog:                  moduleCatalog,
		Content:                        contentRegistry,
		LoadoutStore:                   loadoutStore,
		Loadout:                        loadoutService,
		StatInputs:                     statInputs,
		Recipes:                        recipeCatalog,
		ProductionCatalog:              contentBundle.Production,
		Crafting:                       craftingService,
		Discovery:                      discoveryStore,
		Intel:                          intelService,
		Production:                     productionStore,
		ClaimLifecycles:                claimLifecycleStore,
		ClaimProductionInitializations: claimProductionInitializationStore,
		Settlements:                    settlementStore,
		BuildingMutations:              buildingMutationStore,
		CommandLog:                     commandLogger,
		Metrics:                        metricRecorder,
		contentAdminCloser:             contentAdminCloser,
		authStoreCloser:                authStoreCloser,
		walletStoreCloser:              walletStoreCloser,
		inventoryStoreCloser:           inventoryStoreCloser,
		progressionStoreCloser:         progressionStoreCloser,
		economyStoreCloser:             economyStoreCloser,
		hangarStoreCloser:              hangarStoreCloser,
		loadoutStoreCloser:             loadoutStoreCloser,
		combatXP:                       combatXP,
		lootTables:                     lootTables,
		itemCatalog:                    itemCatalog,
		starterContent:                 contentBundle.Starter,
		routeContent:                   contentBundle.Route,
		productionRules:                contentBundle.Rules,
		combatRules:                    contentBundle.Combat,
		contentCatalogProjection:       contentCatalogProjection,
		contentCatalogVersion:          contentCatalogProjection.Version,
		repairAttempts:                 make(map[foundation.IdempotencyKey]repairAttemptRecord),
		repairQuotes:                   make(map[foundation.PlayerID]repairQuoteRecord),
		shopPurchases:                  make(map[foundation.IdempotencyKey]shopPurchaseRecord),
		scanCooldowns:                  make(map[scanCooldownKey]time.Time),
		scanCapacitorSpends:            make(map[discovery.ScanPulseReference]scanCapacitorSpendRecord),
	}
	scannerSeed, err := contentBundle.Scanner.WorldSeed()
	if err != nil {
		return nil, err
	}
	scannerCandidateOptions := contentBundle.Scanner.CandidateOptionsForRuntime(config.E2EScanNoPlanetSeed)
	var scannerProfiles discovery.ScannerCandidateOptionsProvider
	if !config.E2EScanNoPlanetSeed {
		scannerProfiles = contentBundle.Scanner
	}
	scanner, err := discovery.NewScannerService(discovery.ScannerServiceConfig{
		Store:             discoveryStore,
		WorldSeed:         scannerSeed,
		Clock:             clock,
		Modules:           runtimeScannerModuleProvider{runtime: runtime},
		Stats:             runtimeScannerStatsProvider{runtime: runtime},
		Positions:         runtimeScannerPositionProvider{runtime: runtime},
		Cooldowns:         runtimeScannerCooldownProvider{runtime: runtime},
		Energy:            runtimeScannerEnergyProvider{runtime: runtime},
		Reveals:           runtimeScannerPlayerRevealProvider{runtime: runtime},
		Profiles:          scannerProfiles,
		XP:                runtimeScanXPProvider{progression: progressionService},
		CandidateOptions:  scannerCandidateOptions,
		RadarLevelUnit:    contentBundle.Scanner.RadarLevelUnit,
		DiscoveryXPAmount: contentBundle.Scanner.DiscoveryXPAmount,
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
			StorageCapacityUnits:  contentBundle.Rules.ClaimStorageCapacityUnits,
			EnergyCapacityPerHour: contentBundle.Rules.ClaimEnergyCapacityPerHour,
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
		ListedIntelStaleMarker: runtimeClaimListedIntelStaleMarker{
			market: marketService,
			intel:  intelService,
		},
		XCoreItemDefinition: xCoreDefinition,
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
	if runtime.devMode {
		_ = metricRecorder.SetGauge(observability.MetricDevModeEnabled, nil, 1)
	}
	gateway, err := realtime.NewGateway(realtime.GatewayOptions{
		Clock:    clock,
		Sessions: runtimeSessionResolver{runtime: runtime},
		Executor: realtime.ObservedCommandExecutor{
			Clock:   clock,
			Logger:  commandLogger,
			Metrics: metricRecorder,
		},
		Limiter:  runtimeRealtimeLimiter(config, clock),
		Handlers: runtime.commandHandlers(),
	})
	if err != nil {
		return nil, err
	}
	runtime.Gateway = gateway
	closeHangarStoreOnError = false
	closeLoadoutStoreOnError = false
	closeEconomyStoreOnError = false
	return runtime, nil
}

func runtimeRealtimeLimiter(config RuntimeConfig, clock foundation.Clock) realtime.RateLimiter {
	if config.disableRealtimeLimiter {
		return nil
	}
	if config.realtimeLimiter != nil {
		return config.realtimeLimiter
	}
	return realtime.NewInMemoryRealtimeLimiter(realtime.InMemoryRealtimeLimiterOptions{
		Clock: clock,
	})
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
				var durableEvents map[auth.SessionID][]realtime.EventEnvelope
				if sink != nil {
					durableEvents = runtime.runDurableOutboxRealtimePumpTick()
				}
				eventsBySession := runtime.tickAndCollectAOIEvents()
				if sink != nil {
					mergeRuntimeRealtimeEvents(eventsBySession, durableEvents)
				}
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
		overrides := runtime.starterWorldSeedOverrides(mapID)
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
		if err := instance.Worker.Submit(worker.InsertEntityCommand{Entity: hidden, Speed: 0}); err != nil {
			return err
		}
		if err := commandErrors(instance.Worker.FlushCommands()); err != nil {
			return err
		}
		instance.HiddenEntities[hidden.ID] = true
	}
	return nil
}

func (runtime *Runtime) starterWorldSeedOverrides(mapID worldmaps.MapID) map[worldmaps.EnemyPoolID][]world.EntityID {
	overrides := make(map[worldmaps.EnemyPoolID][]world.EntityID)
	for _, seed := range runtime.starterContent.WorldSeeds {
		if seed.MapID != mapID || len(seed.EntityIDOverrides) == 0 {
			continue
		}
		overrides[seed.EnemyPoolID] = append([]world.EntityID(nil), seed.EntityIDOverrides...)
	}
	if len(overrides) == 0 {
		return nil
	}
	return overrides
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

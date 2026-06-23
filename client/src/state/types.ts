import { EntityPayload, ErrorPayload, EventEnvelope, JsonObject, RequestEnvelope, ResponseEnvelope, Vec2 } from '../protocol/envelope';

export type ConnectionStatus =
  | 'restoring'
  | 'logged_out'
  | 'authenticated_pending_socket'
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'auth_expired'
  | 'offline'
  | 'error';

export type ClientMode = 'real' | 'demo';

export interface PublicAccount {
  email: string;
  admin: boolean;
}

export interface PublicPlayer {
  callsign: string;
}

export interface PublicSession extends JsonObject {
  authenticated: boolean;
  account?: PublicAccount;
  player?: PublicPlayer;
  roles?: string[];
  expires_at?: number;
  server_time: number;
}

export interface ClientAuthState {
  mode: ClientMode;
  session: PublicSession | null;
  submitting: boolean;
  error: string | null;
}

export interface PlayerSnapshot extends JsonObject {
  hp?: number;
  shield?: number;
  energy?: number;
  max_hp?: number;
  max_shield?: number;
  max_energy?: number;
  rank?: number;
  callsign?: string;
}

export interface LogLine {
  id: string;
  level: 'info' | 'warn' | 'error';
  text: string;
  at: number;
}

export interface CargoSummary {
  used: number;
  capacity: number;
  items: CargoItemSummary[];
}

export interface CargoItemSummary {
  item_id: string;
  display_name?: string;
  category?: string;
  art_key?: string;
  rarity?: string;
  quantity: number;
  unit_weight?: number;
  used_units?: number;
  location?: string;
  move_eligible?: boolean;
  locked_reason?: string;
}

export interface WalletSummary {
  credits: number;
  premium_paid: number;
  premium_earned: number;
}

export interface ShopCategorySummary {
  category_id: string;
  display_name: string;
  sort_order: number;
}

export interface ShopProductSummary {
  product_id: string;
  product_type: string;
  display_name: string;
  description: string;
  category_id: string;
  subcategory?: string;
  art_key: string;
  rarity?: string;
  tier?: number;
  sort_order: number;
  grant_target: {
    kind: string;
    ref_id: string;
    quantity?: number;
  };
  price: {
    currency_type: string;
    amount: number;
    fixed: boolean;
  };
  stock: {
    kind: string;
    stock_remaining?: number;
    stock_total?: number;
  };
  availability: {
    available: boolean;
    locked_reason?: string;
    required_rank?: number;
  };
}

export interface ShopCatalogSummary {
  catalog_version: string;
  categories: ShopCategorySummary[];
  products: ShopProductSummary[];
}

export interface MarketListingSummary {
  listing_id: string;
  item_id: string;
  display_name: string;
  rarity: string;
  remaining_quantity: number;
  unit_price: number;
  currency_type: string;
  status: string;
  expires_at?: number;
  owned_by_you: boolean;
  final_price_pending: boolean;
  estimated_unit_purchase: {
    quantity: number;
    subtotal: number;
    currency_type: string;
    pending: boolean;
  };
}

export interface MarketSummary {
  listings: MarketListingSummary[];
  counts: {
    active: number;
    mine: number;
  };
}

export interface AuctionLotSummary {
  auction_id: string;
  payload_type: string;
  definition_id: string;
  quantity: number;
  currency_type: string;
  start_price: number;
  current_bid: number;
  has_bid: boolean;
  leading: boolean;
  buy_now_price?: number;
  status: string;
  starts_at: number;
  ends_at: number;
  final_price_pending: boolean;
}

export interface AuctionGrantSummary {
  auction_id: string;
  payload_type: string;
  definition_id: string;
  quantity: number;
  reason: string;
  granted_at: number;
}

export interface AuctionSummary {
  lots: AuctionLotSummary[];
  grants: AuctionGrantSummary[];
}

export interface PremiumEntitlementSummary {
  entitlement_id: string;
  type: string;
  state: string;
  payload: {
    currency_bucket?: string;
    amount?: number;
    loadout_slot_scope?: string;
    loadout_slot_count?: number;
    period_key?: string;
    cosmetic_id?: string;
    badge_id?: string;
  };
  created_at: number;
  claimed_at?: number;
}

export interface PremiumStockSummary {
  period_key: string;
  stock_total: number;
  stock_remaining: number;
  price_amount: number;
  payment_currency: string;
}

export interface PremiumPurchaseSummary {
  period_key: string;
  payment_currency: string;
  granted_at: number;
}

export interface PremiumSummary {
  entitlements: PremiumEntitlementSummary[];
  stock: PremiumStockSummary[];
  purchases: PremiumPurchaseSummary[];
}

export interface QuestObjectiveSummary {
  id: string;
  kind: string;
  target?: string;
  display_name: string;
  catalog_ref?: string;
  art_key?: string;
  current: number;
  required: number;
  completed: boolean;
}

export interface QuestRewardSummary {
  kind: string;
  currency_type?: string;
  item_id?: string;
  role?: string;
  display_name: string;
  catalog_ref?: string;
  art_key?: string;
  amount: number;
}

export interface QuestOfferSummary {
  offer_id: string;
  quest_type: string;
  title: string;
  description: string;
  objectives: QuestObjectiveSummary[];
  rewards: QuestRewardSummary[];
  expires_at: number;
  can_accept: boolean;
  locked_reason?: string;
}

export interface QuestSummary {
  quest_id: string;
  accepted_offer_id?: string;
  quest_type: string;
  title: string;
  description: string;
  state: string;
  objectives: QuestObjectiveSummary[];
  rewards: QuestRewardSummary[];
  accepted_at: number;
  completed_at?: number;
  claimed_at?: number;
  can_claim: boolean;
}

export interface QuestBoardSummary {
  offers: QuestOfferSummary[];
  active: QuestSummary[];
  counts: {
    offers: number;
    active: number;
    completed: number;
    claimable: number;
    claimed: number;
  };
  reroll_cost: {
    currency_type: string;
    amount: number;
  };
  can_reroll: boolean;
  locked_reason?: string;
  reset_at?: number;
  generated_at: number;
  revision: number;
}

export interface EconomyDashboardSummary {
  wallets: {
    credits: number;
    premium_paid: number;
    premium_earned: number;
  };
  market: {
    active_listings: number;
    sold_listings: number;
    volume_credits: number;
  };
  auction: {
    active_lots: number;
    closed_lots: number;
    grants: number;
  };
  premium: {
    pending_entitlements: number;
    claimed_entitlements: number;
    weekly_stock_remaining: number;
  };
  generated_at: number;
}

export interface AdminInspectionSummary {
  target: string;
  inventory: {
    stackable_items: number;
    instance_items: number;
    item_ledger: Array<{
      ledger_id: string;
      item_id: string;
      quantity: number;
      action: string;
      balance_after: number;
      location: string;
      reason: string;
      created_at: number;
    }>;
  };
  wallet: {
    balances: Array<{ currency_type: string; balance: number }>;
    ledger: Array<{
      ledger_id: string;
      currency_type: string;
      amount: number;
      action: string;
      balance_after: number;
      reason: string;
      created_at: number;
    }>;
  };
  generated_at: number;
}

export interface AdminRepairCraftJobSummary {
  accepted: boolean;
  job_id?: string;
  status: string;
  already_complete?: boolean;
  message?: string;
}

export interface CommandLogSummary {
  entries: Array<{
    request_id: string;
    operation: string;
    status: string;
    error_code?: string;
    duration_ms: number;
    timestamp: number;
  }>;
  total: number;
  generated_at: number;
}

export interface MetricsSummary {
  snapshot: {
    counters: Array<{ name: string; value: number; labels: Array<{ name: string; value: string }> }>;
    gauges: Array<{ name: string; value: number; labels: Array<{ name: string; value: string }> }>;
    durations: Array<{
      name: string;
      labels: Array<{ name: string; value: string }>;
      count: number;
      total: number;
      minimum: number;
      maximum: number;
      p50: number;
      p95: number;
      p99: number;
    }>;
  };
  generated_at: number;
}

export interface ReleaseGateSummary {
  report: {
    covered: boolean;
    passed: boolean;
    missing: Array<{ module: string; check: string }>;
  };
  coverage: Array<{
    module: string;
    passed: boolean;
    missing: string[];
    evidence: number;
  }>;
  evidence: number;
  generated_at: number;
}

export interface AbuseCoverageSummary {
  report: {
    passed: boolean;
    missing: string[];
  };
  coverage: Array<{
    case: string;
    evidence: Array<{ package: string; test_name: string; note: string }>;
  }>;
  generated_at: number;
}

export interface ShipSummary {
  active_ship_id: string;
  display_name: string;
  hull: number;
  max_hull: number;
  shield: number;
  max_shield: number;
  capacitor: number;
  max_capacitor: number;
  disabled: boolean;
  repair_state: string;
}

export interface StatSummary {
  speed: number;
  radar_range: number;
  weapon_range: number;
  cargo_capacity: number;
  loot_pickup_range: number;
  basic_laser_energy_cost: number;
  basic_laser_cooldown_ms: number;
}

export interface ProgressionSummary {
  main_level: number;
  main_xp: number;
  rank: number;
  combat_level?: number;
  combat_xp?: number;
}

export interface InventorySummary {
  stackable: Array<{
    item_id: string;
    display_name?: string;
    quantity: number;
    location: string;
    list_eligible?: boolean;
    locked_reason?: string;
  }>;
  instances: Array<{
    item_instance_id: string;
    item_id: string;
    display_name?: string;
    location: string;
    rarity?: string;
    item_type?: string;
    module_slot_type?: string;
    module_category?: string;
    durability_current?: number;
    durability_max?: number;
    bound_state?: string;
  }>;
  counts: {
    cargo_stacks: number;
    storage_stacks: number;
    equipped_instances: number;
  };
}

export interface HangarSummary {
  active_ship_id: string;
  ships: Array<{
    ship_id: string;
    display_name: string;
    state: string;
    role?: string;
    tier?: number;
    rank_requirement?: number;
    hull: number;
    max_hull: number;
    shield: number;
    max_shield: number;
    capacitor?: number;
    max_capacitor?: number;
    speed?: number;
    radar?: number;
    cargo_capacity?: number;
    slot_offensive?: number;
    slot_defensive?: number;
    slot_utility?: number;
    disabled: boolean;
    active?: boolean;
    locked_reason?: string;
  }>;
}

export interface LoadoutSummary {
  active_ship_id: string;
  slots: Array<{
    slot_id: string;
    slot_type: string;
    module_item_id?: string;
    item_instance_id?: string;
    module_id?: string;
    display_name?: string;
    module_state?: string;
    durability?: number;
    durability_max?: number;
  }>;
}

export interface CraftingSummary {
  recipes: Array<{
    recipe_id: string;
    category: string;
    output: {
      kind: string;
      item_id?: string;
      ship_id?: string;
      quantity: number;
      tradeable: boolean;
    };
    inputs: Array<{ item_id: string; quantity: number }>;
    required_credits: number;
    required_rank: number;
    required_role_levels: Array<{ role: string; level: number }>;
    required_location_type: string;
    craft_duration_ms: number;
    repeatable: boolean;
  }>;
  active_jobs: Array<{
    job_id: string;
    recipe_id: string;
    state: string;
    started_at: number;
    completes_at: number;
  }>;
}

export interface ScanPulseSummary {
  pulse_reference: string;
  status: string;
  resolve_after?: number;
  message?: string;
  signal?: {
    biome: string;
    signal_band: string;
    approx_distance: string;
  };
  planet_id?: string;
  xp_granted?: boolean;
  duplicate?: boolean;
}

export interface ScanModeState {
  enabled: boolean;
  nextPulseAt: number | null;
  lastRejectedAt: number | null;
  lastError: string | null;
}

export interface KnownPlanetSummary {
  planet_id: string;
  sector_key?: string;
  biome: string;
  planet_type: string;
  rarity: string;
  level: number;
  intel_state: string;
  confidence: number;
  last_seen_at: number;
  owner_status: string;
  discovered_at: number;
}

export interface PlanetStorageSummary {
  planet_id: string;
  public_map_key?: string;
  used_units: number;
  free_units: number;
  capacity_units: number;
  updated_at: number;
  items: Array<{ item_id: string; quantity: number }>;
}

export interface PlanetBuildingSummary {
  planet_id?: string;
  public_map_key?: string;
  building_id: string;
  building_type: string;
  category: string;
  level: number;
  state: string;
  updated_at: number;
}

export interface PlanetProductionSummary {
  planet_id: string;
  public_map_key?: string;
  production_enabled: boolean;
  last_calculated_at: number;
  energy_capacity_per_hour: number;
  energy_reserved_per_hour: number;
  storage: PlanetStorageSummary;
  buildings: PlanetBuildingSummary[];
}

export interface RouteSummary {
  route_id: string;
  source_planet_id: string;
  from_public_map_key?: string;
  to_public_map_key?: string;
  destination: { type: string; id: string };
  resource_item_id: string;
  amount_per_hour: number;
  energy_cost_per_hour: number;
  enabled: boolean;
  risk: {
    loss_chance: number;
    min_loss_percent: number;
    max_loss_percent: number;
  };
  last_calculated_at: number;
  updated_at: number;
  last_settlement?: RouteSettlementSummary;
}

export interface RouteSettlementSummary {
  route_id: string;
  resource_item_id: string;
  settled_at: number;
  elapsed_applied_ms: number;
  wanted_amount: number;
  taken_amount: number;
  lost_amount: number;
  delivered_amount: number;
  added_amount: number;
  source_empty: boolean;
  destination_full: boolean;
  loss_applied: boolean;
  no_op: boolean;
}

export interface PlanetDetailSummary extends KnownPlanetSummary {
  coordinates: Vec2 | null;
  production?: PlanetProductionSummary;
  routes: RouteSummary[];
  production_locked: boolean;
  available_commands: string[];
}

export interface PlanetIntelSummary {
  knownSignals: number;
  staleIntel: number | null;
  ownedPlanets: number;
  planets: KnownPlanetSummary[];
  selectedPlanet: PlanetDetailSummary | null;
  lastScan: ScanPulseSummary | null;
}

export interface WorldMapMemoryMarker {
  id: string;
  kind: 'known_planet';
  label: string;
  position: Vec2;
  detailID: string;
  state: string;
  projectionSource?: string;
}

export interface ProductionCollectionSummary {
  planets: PlanetProductionSummary[];
}

export interface RouteListSummary {
  routes: RouteSummary[];
}

export interface RepairQuote {
  ship_id: string;
  currency: string;
  cost: number;
  disabled: boolean;
}

export interface MapBounds {
  min_x: number;
  min_y: number;
  max_x: number;
  max_y: number;
}

export interface PublicPortalSummary {
  portal_id: string;
  label?: string;
  display_name?: string;
  position: Vec2;
  interaction_radius: number;
  destination_label?: string;
  state?: 'available' | 'cooldown' | 'locked' | 'offline';
  cooldown_ready_at_ms?: number;
  locked_reason?: string;
}

export interface SafeZoneProjection {
  safe_area_id: string;
  display_name?: string;
  center: Vec2;
  radius: number;
  blocks_pvp: boolean;
  hangar_actions: boolean;
}

export interface ViewerSafeZoneSummary {
  inside: boolean;
  blocks_pvp: boolean;
  protection_expires_at?: number;
}

export interface ViewerProtectionSummary {
  reason: string;
  expires_at: number;
  blocks_pvp: boolean;
  break_on_pvp_action: boolean;
}

export interface MapSummary {
  map_key?: string;
  public_map_key?: string;
  display_name?: string;
  region?: string;
  risk_band?: string;
  pvp_policy?: string;
  visual_theme_key?: string;
  bounds: MapBounds;
  visible_portals: PublicPortalSummary[];
  safe_zones: SafeZoneProjection[];
  safe_zone?: ViewerSafeZoneSummary;
  protection?: ViewerProtectionSummary;
}

export interface SectorSummary {
  sector_key?: string;
  name: string;
  region: string;
  danger: string;
  contested: boolean;
}

export interface MinimapContact {
  entity_id: string;
  entity_type: EntityPayload['entity_type'];
  position: Vec2;
  disposition?: string;
  status_flags?: string[];
  projection_source?: string;
}

export interface MinimapMemory {
  kind: string;
  sector_key?: string;
  public_map_key?: string;
  planet_id?: string;
  detail_id?: string;
  label: string;
  position: Vec2;
  freshness: string;
  invalidated?: boolean;
  projection_source?: string;
}

export interface MinimapSummary {
  public_map_key?: string;
  bounds?: MapBounds;
  visible_portals?: PublicPortalSummary[];
  safe_zones?: SafeZoneProjection[];
  radar_range: number;
  projection_window_size?: number;
  live_contacts: MinimapContact[];
  remembered: MinimapMemory[];
}

export interface PendingCommand {
  requestID: string;
  op: string;
  queuedAt: number;
  payload?: JsonObject;
}

export interface KnownLootDrop {
  drop_id: string;
  item_id: string;
  quantity: number;
  state?: string;
  expires_at?: number;
  position?: Vec2;
}

export interface MapTransferState {
  state: 'started' | 'failed';
  portal_id?: string;
  from_public_map_key?: string;
  to_public_map_key?: string;
  reason?: string;
  started_at: number;
}

export type WorldFeedbackKind = 'laser' | 'damage' | 'miss' | 'destroyed' | 'loot_spawn' | 'loot_pickup';

export interface WorldFeedbackEffect {
  id: string;
  kind: WorldFeedbackKind;
  targetID?: string;
  sourceID?: string;
  position?: Vec2;
  sourcePosition?: Vec2;
  amount?: number;
  shieldAmount?: number;
  hullAmount?: number;
  itemID?: string;
  quantity?: number;
  createdAt: number;
  expiresAt: number;
}

export interface ClientState {
  auth: ClientAuthState;
  connectionStatus: ConnectionStatus;
  socketURL: string;
  lastServerTime: number | null;
  lastSequence: number;
  mapSubscriptionEpoch: number | null;
  mapTransfer: MapTransferState | null;
  currentMap: MapSummary | null;
  portalCooldowns: Record<string, number>;
  playerSnapshot: PlayerSnapshot | null;
  sector: SectorSummary | null;
  minimap: MinimapSummary | null;
  visibleEntities: Record<string, EntityPayload>;
  selectedTargetID: string | null;
  movementTarget: Vec2 | null;
  lastCorrection: { entityID: string; position: Vec2 } | null;
  knownLoot: Record<string, KnownLootDrop>;
  worldEffects: WorldFeedbackEffect[];
  pendingCommands: Record<string, PendingCommand>;
  commandLog: LogLine[];
  combatLog: LogLine[];
  cargo: CargoSummary | null;
  wallet: WalletSummary | null;
  ship: ShipSummary | null;
  stats: StatSummary | null;
  progression: ProgressionSummary | null;
  inventory: InventorySummary | null;
  hangar: HangarSummary | null;
  loadout: LoadoutSummary | null;
  crafting: CraftingSummary | null;
  repairQuote: RepairQuote | null;
  skillCooldowns: Record<string, number>;
  questBoard: QuestBoardSummary | null;
  planetIntel: PlanetIntelSummary | null;
  scanMode: ScanModeState;
  production: ProductionCollectionSummary | null;
  routes: RouteListSummary | null;
  shopCatalog: ShopCatalogSummary | null;
  market: MarketSummary | null;
  auction: AuctionSummary | null;
  premium: PremiumSummary | null;
  economyDashboard: EconomyDashboardSummary | null;
  adminInspection: AdminInspectionSummary | null;
  adminRepair: AdminRepairCraftJobSummary | null;
  commandLogSummary: CommandLogSummary | null;
  metrics: MetricsSummary | null;
  releaseGate: ReleaseGateSummary | null;
  abuseCoverage: AbuseCoverageSummary | null;
  lastError: ErrorPayload | null;
}

export type ClientAction =
  | { type: 'demoModeStarted' }
  | { type: 'authRestoreStarted' }
  | { type: 'authSubmitStarted' }
  | { type: 'authSessionLoaded'; session: PublicSession }
  | { type: 'authLoggedOut' }
  | { type: 'authExpired'; message?: string }
  | { type: 'authFailed'; message: string }
  | { type: 'connectionChanged'; status: ConnectionStatus; socketURL?: string }
  | { type: 'requestQueued'; envelope: RequestEnvelope }
  | { type: 'scanModeToggled'; enabled?: boolean; now?: number }
  | { type: 'scanPulseScheduled'; nextPulseAt: number | null; lastError?: string | null }
  | { type: 'scanPulseAccepted'; nextPulseAt?: number | null }
  | { type: 'scanPulseRejected'; message: string; backoffUntil: number; rejectedAt?: number }
  | {
      type: 'responseReceived';
      envelope: ResponseEnvelope | { ok: false; error: ErrorPayload; request_id: string; server_time: number; v?: number };
    }
  | { type: 'replaceVisibleEntities'; entities: EntityPayload[]; serverTime?: number | null; sequence?: number }
  | { type: 'eventReceived'; envelope: EventEnvelope }
  | { type: 'serverCorrection'; entityID: string; position: Vec2; serverTime?: number }
  | { type: 'selectTarget'; entityID: string | null }
  | { type: 'appendLog'; level: LogLine['level']; text: string };

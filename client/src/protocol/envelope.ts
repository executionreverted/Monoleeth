export const PROTOCOL_VERSION = 1 as const;

export const OPERATIONS = {
  sessionSnapshot: 'session.snapshot',
  worldSnapshot: 'world.snapshot',
  portalEnter: 'portal.enter',
  moveTo: 'move_to',
  stop: 'stop',
  debugSpawnNPC: 'debug_spawn_npc',
  debugSnapshot: 'debug_snapshot',
  combatUseSkill: 'combat.use_skill',
  lootPickup: 'loot.pickup',
  shieldRepairTick: 'repair.shield_tick',
  deathRepairQuote: 'death.repair_quote',
  deathRepairShip: 'death.repair_ship',
  progressionSnapshot: 'progression.snapshot',
  inventorySnapshot: 'inventory.snapshot',
  hangarSnapshot: 'hangar.snapshot',
  hangarActivateShip: 'hangar.activate_ship',
  loadoutSnapshot: 'loadout.snapshot',
  loadoutEquipModule: 'loadout.equip_module',
  loadoutUnequipModule: 'loadout.unequip_module',
  statsSnapshot: 'stats.snapshot',
  stealthToggle: 'stealth.toggle',
  craftingRecipes: 'crafting.recipes',
  craftingStart: 'crafting.start',
  craftingComplete: 'crafting.complete',
  craftingCancel: 'crafting.cancel',
  scanPulse: 'scan.pulse',
  knownPlanets: 'discovery.known_planets',
  planetDetail: 'discovery.planet_detail',
  discoveryClaimPlanet: 'discovery.claim_planet',
  intelShare: 'intel.share',
  intelCoordinateItemCreate: 'intel.coordinate_item.create',
  intelCoordinateItemUse: 'intel.coordinate_item.use',
  productionSummary: 'planet.production_summary',
  planetStorageSummary: 'planet.storage_summary',
  planetBuildingBuild: 'planet.building_build',
  planetBuildingUpgrade: 'planet.building_upgrade',
  routeCreate: 'route.create',
  routeUpdate: 'route.update',
  routeEnable: 'route.enable',
  routeDisable: 'route.disable',
  routeList: 'route.list',
  routeSnapshot: 'route.snapshot',
  routeSettle: 'route.settle',
  walletSnapshot: 'wallet.snapshot',
  contentCatalog: 'content.catalog',
  shopCatalog: 'shop.catalog',
  shopBuyProduct: 'shop.buy_product',
  marketSearch: 'market.search',
  marketCreateListing: 'market.create_listing',
  marketBuy: 'market.buy',
  marketCancel: 'market.cancel',
  auctionSearch: 'auction.search',
  auctionBid: 'auction.bid',
  auctionBuyNow: 'auction.buy_now',
  auctionGrants: 'auction.grants',
  premiumEntitlements: 'premium.entitlements',
  premiumClaim: 'premium.claim',
  premiumPurchaseWeeklyXCore: 'premium.purchase_weekly_xcore',
  questBoard: 'quest.board',
  questAccept: 'quest.accept',
  questProgress: 'quest.progress',
  questClaimReward: 'quest.claim_reward',
  questReroll: 'quest.reroll',
  chatSend: 'chat.send',
  partyInvite: 'party.invite',
  partyAccept: 'party.accept',
  partyLeave: 'party.leave',
  partyTargetSet: 'party.target.set',
  clanCreate: 'clan.create',
  clanJoin: 'clan.join',
  clanLeave: 'clan.leave',
  adminInspectPlayer: 'admin.inspect_player',
  adminRepairCraftJob: 'admin.repair_craft_job',
  adminEconomyDashboard: 'admin.economy_dashboard',
  adminContentVersions: 'admin.content.versions',
  adminContentList: 'admin.content.list',
  adminContentGet: 'admin.content.get',
  adminContentUpdateDraft: 'admin.content.update_draft',
  adminContentValidateDraft: 'admin.content.validate_draft',
  adminContentPublish: 'admin.content.publish',
  adminContentRollback: 'admin.content.rollback',
  adminContentAuditLog: 'admin.content.audit_log',
  observabilityCommandLog: 'observability.command_log',
  observabilityMetrics: 'observability.metrics',
  observabilityReleaseGate: 'observability.release_gate',
  observabilityAbuseCoverage: 'observability.abuse_coverage',
} as const;

export type Operation = (typeof OPERATIONS)[keyof typeof OPERATIONS];

export const CLIENT_EVENTS = {
  sessionReady: 'session.ready',
  playerSnapshot: 'player.snapshot',
  shipSnapshot: 'ship.snapshot',
  cargoSnapshot: 'cargo.snapshot',
  walletSnapshot: 'wallet.snapshot',
  statsSnapshot: 'stats.updated',
  worldSnapshot: 'world.snapshot',
  mapSnapshot: 'map.snapshot',
  mapChanged: 'map.changed',
  mapTransferStarted: 'map.transfer_started',
  mapTransferCompleted: 'map.transfer_completed',
  mapTransferFailed: 'map.transfer_failed',
  portalCooldownStarted: 'portal.cooldown_started',
  mapPolicyUpdated: 'map.policy_updated',
  playerProtectionUpdated: 'player.protection_updated',
  entityEntered: 'aoi.entity_entered',
  entityUpdated: 'aoi.entity_updated',
  entityLeft: 'aoi.entity_left',
  positionCorrected: 'position.corrected',
  movementStopped: 'movement.stopped',
  serverNotice: 'server.notice',
  targetUpdated: 'target.updated',
  combatDamage: 'combat.damage',
  combatMiss: 'combat.miss',
  combatCooldownStarted: 'combat.cooldown_started',
  combatNPCKilled: 'combat.npc_killed',
  lootCreated: 'loot.created',
  lootUpdated: 'loot.updated',
  lootRemoved: 'loot.removed',
  lootPickedUp: 'loot.picked_up',
  progressionSnapshot: 'progression.snapshot',
  inventorySnapshot: 'inventory.snapshot',
  hangarSnapshot: 'hangar.snapshot',
  loadoutSnapshot: 'loadout.snapshot',
  craftingRecipes: 'crafting.recipes',
  scanPulseStarted: 'scan.pulse_started',
  scanPulseResolved: 'scan.pulse_resolved',
  scanPlanetDiscovered: 'scan.planet_discovered',
  knownPlanets: 'discovery.known_planets',
  planetDetail: 'discovery.planet_detail',
  planetClaimed: 'planet.claimed',
  productionSummary: 'planet.production_summary',
  planetStorageSummary: 'planet.storage_summary',
  routeUpdated: 'route.updated',
  routeList: 'route.list',
  routeSnapshot: 'route.snapshot',
  routeSettled: 'route.settled',
  marketListingCreated: 'market.listing_created',
  marketListingUpdated: 'market.listing_updated',
  marketSaleCompleted: 'market.sale_completed',
  marketListingCancelled: 'market.listing_cancelled',
  auctionLotUpdated: 'auction.lot_updated',
  auctionBidPlaced: 'auction.bid_placed',
  auctionClosed: 'auction.closed',
  premiumEntitlementCreated: 'premium.entitlement_created',
  premiumEntitlementClaimed: 'premium.entitlement_claimed',
  premiumStockConsumed: 'premium.stock_consumed',
  economyFlowUpdated: 'economy.flow_updated',
  questBoardGenerated: 'quest.board_generated',
  questAccepted: 'quest.accepted',
  questProgressed: 'quest.progressed',
  questCompleted: 'quest.completed',
  questRewardClaimed: 'quest.reward_claimed',
  questBoardRerolled: 'quest.board_rerolled',
  questAbandoned: 'quest.abandoned',
  chatMessage: 'chat.message',
  partyInvite: 'party.invite',
  partyUpdated: 'party.updated',
  partyLeft: 'party.left',
  partyTargetUpdated: 'party.target_updated',
  partyContributionUpdated: 'party.contribution_updated',
  clanUpdated: 'clan.updated',
  clanLeft: 'clan.left',
  clanContributionUpdated: 'clan.contribution_updated',
  adminActionCompleted: 'admin.action_completed',
  observabilityMetricUpdated: 'observability.metric_updated',
  releaseGateUpdated: 'release_gate.updated',
  deathShipDisabled: 'death.ship_disabled',
  deathRepaired: 'death.repaired',
} as const;

export type ClientEventType = (typeof CLIENT_EVENTS)[keyof typeof CLIENT_EVENTS] | string;

export interface Vec2 {
  x: number;
  y: number;
}

export type EntityType = 'player' | 'npc' | 'loot' | 'planet_signal';

export interface EntityDisplay {
  label?: string;
  disposition?: string;
}

export interface EntityMovementPayload {
  moving: boolean;
  origin: Vec2;
  target: Vec2;
  speed: number;
  started_at_ms: number;
  arrive_at_ms: number;
}

export interface EntityPayload {
  entity_id: string;
  entity_type: EntityType;
  position: Vec2;
  status_flags?: string[];
  display?: EntityDisplay;
  combat?: {
    hp: number;
    max_hp: number;
    shield: number;
    max_shield: number;
    status?: string;
  };
  movement?: EntityMovementPayload;
  projection_source?: string;
}

export interface RequestEnvelope<TPayload extends JsonObject = JsonObject> {
  request_id: string;
  op: Operation;
  payload: TPayload;
  client_seq: number;
  v: number;
}

export interface ResponseEnvelope<TPayload extends JsonObject = JsonObject> {
  request_id: string;
  ok: true;
  payload: TPayload;
  server_time: number;
  v: number;
}

export interface ErrorPayload {
  code: string;
  message: string;
  retryable: boolean;
}

export interface ErrorEnvelope {
  request_id: string;
  ok: false;
  error: ErrorPayload;
  server_time: number;
  v: number;
}

export interface EventEnvelope<TPayload extends JsonObject = JsonObject> {
  event_id: string;
  type: ClientEventType;
  payload: TPayload;
  server_time: number;
  seq: number;
  v: number;
}

export type ServerMessage = ResponseEnvelope | ErrorEnvelope | EventEnvelope;

export type JsonValue = unknown;
export type JsonObject = Record<string, unknown>;

const forbiddenPayloadKeys = new Set([
  'account_id',
  'session_id',
  'world_id',
  'zone_id',
  'map_id',
  'internal_map_id',
  'worker_id',
  'map_worker_id',
  'transfer_id',
  'transfer_token',
  'destination_worker',
  'origin_worker',
  'destination_map_id',
  'destination_spawn_id',
  'client_player_id',
  'player_id',
  'damage',
  'loot',
  'cooldown',
  'wallet_amount',
  'hidden',
  'internal',
  'internal_metadata',
  'gameplay_seed',
  'procedural_seed',
  'world_seed',
  'future_spawn',
  'future_spawn_data',
  'spawn_candidates',
  'candidate',
  'candidate_key',
  'planet_candidate',
  'detection_roll',
  'scan_roll',
  'scan_cell',
  'scan_result',
  'scan_candidate',
  'scan_candidates',
  'candidate_data',
  'target_player_id',
  'witness_expires_at',
  'witness_expiry',
  'hidden_target_metadata',
  'loss_percent',
  'loot_roll',
  'loot_table',
  'seller_player_id',
  'buyer_player_id',
  'bidder_player_id',
  'current_bidder_id',
  'winning_player_id',
  'provider',
  'provider_reference',
  'escrow_location',
  'source_return_location',
  'generated_payload',
  'generated_seed',
  'reward_payload',
  'rare_cap',
  'reference_id',
  'password',
  'password_hash',
  'token',
  'session_token',
  'reset_secret',
  'auth_header',
  'cookie',
]);

export const adminContentResponseAllowedPayloadKeys = new Set([
  'map_id',
  'map_key',
  'public_map_key',
  'damage',
  'speed',
  'rank',
  'loot',
  'cooldown',
  'expires_at',
  'hidden',
  'internal',
  'internal_metadata',
  'total',
  'stock_total',
  'stock_remaining',
  'spawn',
  'spawn_point',
  'spawn_position',
  'destination',
  'destination_id',
  'destination_map_id',
  'destination_map_key',
  'destination_public_key',
  'destination_public_map_key',
  'to_map_key',
  'to_public_map_key',
  'gameplay_seed',
  'procedural_seed',
  'future_spawn',
  'future_spawn_data',
  'spawn_candidates',
  'candidate',
  'candidate_key',
  'candidate_data',
  'loot_table',
]);

export const socialResponseAllowedPayloadKeys = new Set([
  'player_id',
  'sender_id',
  'inviter_id',
  'invitee_id',
  'owner_id',
  'set_by_player_id',
]);

function isSocialOperation(operation: string | null | undefined): boolean {
  return (
    operation === OPERATIONS.chatSend ||
    operation === OPERATIONS.partyInvite ||
    operation === OPERATIONS.partyAccept ||
    operation === OPERATIONS.partyLeave ||
    operation === OPERATIONS.partyTargetSet ||
    operation === OPERATIONS.clanCreate ||
    operation === OPERATIONS.clanJoin ||
    operation === OPERATIONS.clanLeave
  );
}

function isSocialEvent(eventType: string): boolean {
  return (
    eventType === CLIENT_EVENTS.chatMessage ||
    eventType === CLIENT_EVENTS.partyInvite ||
    eventType === CLIENT_EVENTS.partyUpdated ||
    eventType === CLIENT_EVENTS.partyLeft ||
    eventType === CLIENT_EVENTS.partyTargetUpdated ||
    eventType === CLIENT_EVENTS.partyContributionUpdated ||
    eventType === CLIENT_EVENTS.clanUpdated ||
    eventType === CLIENT_EVENTS.clanLeft ||
    eventType === CLIENT_EVENTS.clanContributionUpdated
  );
}

export const playerContentCatalogResponseAllowedPayloadKeys = new Set([
  'total',
]);

const playerContentCatalogForbiddenKeys = new Set([
  'audit_note',
  'content_id',
  'content_type',
  'data_json',
  'display_json',
  'draft_version',
  'enabled',
  'enemy_pool_caps',
  'enemy_pools',
  'loot_chance',
  'loot_table',
  'loot_tables',
  'metadata_schema',
  'npc_drop_profiles',
  'procedural_seed',
  'server_only',
  'spawn_areas',
  'spawn_timer_ms',
  'updated_by',
]);

export interface ServerParseOptions {
  operationForRequestID?(requestID: string): string | null;
}

export function parseServerMessage(raw: string, options: ServerParseOptions = {}): ServerMessage {
  const parsed = parseJson(raw);
  assertJsonObject(parsed, 'server message');
  assertVersion(parsed);

  if ('ok' in parsed) {
    const requestID = requireString(parsed.request_id, 'request_id');
    if (parsed.ok === true) {
      const payload = requireObject(parsed.payload, 'response payload');
      const operation = options.operationForRequestID?.(requestID) ?? null;
      assertPlayerContentCatalogResponseAllowed(payload, operation);
      rejectForbiddenPayloadKeys(payload, {
        allowedKeys: allowedResponsePayloadKeys(operation),
      });
      assertPlayerContentCatalogPayloadSafe(payload);
      return {
        request_id: requestID,
        ok: true,
        payload,
        server_time: requireNumber(parsed.server_time, 'server_time'),
        v: requireNumber(parsed.v, 'v'),
      };
    }

    if (parsed.ok === false) {
      const error = requireObject(parsed.error, 'error');
      return {
        request_id: requestID,
        ok: false,
        error: {
          code: requireString(error.code, 'error.code'),
          message: requireString(error.message, 'error.message'),
          retryable: requireBoolean(error.retryable, 'error.retryable'),
        },
        server_time: requireNumber(parsed.server_time, 'server_time'),
        v: requireNumber(parsed.v, 'v'),
      };
    }
  }

  const eventType = requireString(parsed.type, 'type');
  const payload = requireObject(parsed.payload, 'event payload');
  assertPlayerContentCatalogResponseAllowed(payload, null);
  rejectForbiddenPayloadKeys(payload, { allowedKeys: allowedEventPayloadKeys(eventType) });

  return {
    event_id: requireString(parsed.event_id, 'event_id'),
    type: eventType,
    payload,
    server_time: requireNumber(parsed.server_time, 'server_time'),
    seq: requireNumber(parsed.seq, 'seq'),
    v: requireNumber(parsed.v, 'v'),
  };
}

export function isAdminContentOperation(operation: string | null | undefined): boolean {
  return (
    operation === OPERATIONS.adminContentVersions ||
    operation === OPERATIONS.adminContentList ||
    operation === OPERATIONS.adminContentGet ||
    operation === OPERATIONS.adminContentUpdateDraft ||
    operation === OPERATIONS.adminContentValidateDraft ||
    operation === OPERATIONS.adminContentPublish ||
    operation === OPERATIONS.adminContentRollback ||
    operation === OPERATIONS.adminContentAuditLog
  );
}

function allowedResponsePayloadKeys(operation: string | null | undefined): ReadonlySet<string> | undefined {
  if (isAdminContentOperation(operation)) {
    return adminContentResponseAllowedPayloadKeys;
  }
  if (operation === OPERATIONS.contentCatalog) {
    return playerContentCatalogResponseAllowedPayloadKeys;
  }
  if (isSocialOperation(operation)) {
    return socialResponseAllowedPayloadKeys;
  }
  return undefined;
}

function allowedEventPayloadKeys(eventType: string): ReadonlySet<string> | undefined {
  return isSocialEvent(eventType) ? socialResponseAllowedPayloadKeys : undefined;
}

export interface ForbiddenPayloadOptions {
  allowedKeys?: ReadonlySet<string>;
}

export function rejectForbiddenPayloadKeys(value: JsonValue, options: ForbiddenPayloadOptions = {}): void {
  const found = findForbiddenPayloadKey(value, options.allowedKeys);
  if (found) {
    throw new Error('Forbidden server payload rejected.');
  }
}

export function findForbiddenPayloadKey(value: JsonValue, allowedKeys?: ReadonlySet<string>): string | null {
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findForbiddenPayloadKey(item, allowedKeys);
      if (found) {
        return found;
      }
    }
    return null;
  }

  if (!isJsonObject(value)) {
    return null;
  }

  for (const [key, child] of Object.entries(value)) {
    const normalized = key.toLowerCase();
    if (forbiddenPayloadKeys.has(normalized) && !allowedKeys?.has(normalized)) {
      return key;
    }
    const childFound = findForbiddenPayloadKey(child, allowedKeys);
    if (childFound) {
      return childFound;
    }
  }

  return null;
}

export function isJsonObject(value: unknown): value is JsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export function assertPlayerContentCatalogPayloadSafe(payload: JsonObject): void {
  const catalog = payload.content_catalog;
  if (!isJsonObject(catalog)) {
    return;
  }
  const found = findForbiddenPlayerContentCatalogEntry(catalog);
  if (found) {
    throw new Error('Forbidden player content catalog payload rejected.');
  }
}

export function assertPlayerContentCatalogObjectSafe(catalog: JsonObject): void {
  const found = findForbiddenPlayerContentCatalogEntry(catalog);
  if (found) {
    throw new Error('Forbidden player content catalog payload rejected.');
  }
}

function findForbiddenPlayerContentCatalogEntry(value: JsonValue): string | null {
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findForbiddenPlayerContentCatalogEntry(item);
      if (found) {
        return found;
      }
    }
    return null;
  }

  if (typeof value === 'string') {
    return /(?:HIDDEN|SERVER_ONLY)_[A-Z0-9_]*SENTINEL/i.test(value) ? value : null;
  }

  if (!isJsonObject(value)) {
    return null;
  }

  for (const [key, child] of Object.entries(value)) {
    const normalized = key.toLowerCase();
    if (isForbiddenPlayerContentCatalogKey(normalized)) {
      return key;
    }
    const childFound = findForbiddenPlayerContentCatalogEntry(child);
    if (childFound) {
      return childFound;
    }
  }

  return null;
}

function assertPlayerContentCatalogResponseAllowed(
  payload: JsonObject,
  operation: string | null | undefined,
): void {
  if (!Object.prototype.hasOwnProperty.call(payload, 'content_catalog')) {
    return;
  }
  if (operation !== OPERATIONS.contentCatalog) {
    throw new Error('content_catalog payload rejected for non-content.catalog response.');
  }
  if (!isJsonObject(payload.content_catalog)) {
    throw new Error('content_catalog payload must be a JSON object.');
  }
}

function isForbiddenPlayerContentCatalogKey(normalized: string): boolean {
  return (
    playerContentCatalogForbiddenKeys.has(normalized) ||
    normalized.includes('hidden') ||
    normalized.includes('server_only') ||
    normalized.includes('secret') ||
    normalized.includes('audit') ||
    normalized.includes('spawn') ||
    normalized.includes('loot') ||
    normalized.includes('chance') ||
    normalized.includes('seed') ||
    normalized.startsWith('internal')
  );
}

function parseJson(raw: string): unknown {
  try {
    return JSON.parse(raw) as unknown;
  } catch (error) {
    throw new Error(`Invalid JSON message: ${error instanceof Error ? error.message : String(error)}`);
  }
}

function assertJsonObject(value: unknown, label: string): asserts value is JsonObject {
  if (!isJsonObject(value)) {
    throw new Error(`${label} must be a JSON object.`);
  }
}

function requireObject(value: unknown, label: string): JsonObject {
  assertJsonObject(value, label);
  return value;
}

function requireString(value: unknown, label: string): string {
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`${label} must be a non-empty string.`);
  }
  return value;
}

function requireNumber(value: unknown, label: string): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new Error(`${label} must be a finite number.`);
  }
  return value;
}

function requireBoolean(value: unknown, label: string): boolean {
  if (typeof value !== 'boolean') {
    throw new Error(`${label} must be a boolean.`);
  }
  return value;
}

function assertVersion(value: JsonObject): void {
  const version = value.v;
  if (version !== undefined && version !== PROTOCOL_VERSION) {
    throw new Error(`Unsupported protocol version: ${String(version)}`);
  }
}

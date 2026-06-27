import { createRequestId } from './request-id';
import { JsonObject, OPERATIONS, Operation, PROTOCOL_VERSION, RequestEnvelope, Vec2 } from './envelope';

type CommandPayload = JsonObject;
type AdminContentJSON = JsonObject;

export type RouteDestinationType = 'planet' | 'storage' | 'station';

export interface RouteDestinationInput {
  type: RouteDestinationType;
  id: string;
}

type CraftingStartInput = string | {
  recipeID: string;
  locationType?: string;
  locationID?: string;
};

export class CommandBuilder {
  private clientSeq = 0;

  sessionSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.sessionSnapshot, {});
  }

  worldSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.worldSnapshot, {});
  }

  portalEnter(portalID: string): RequestEnvelope<{ portal_id: string }> {
    return this.build(OPERATIONS.portalEnter, {
      portal_id: portalID,
    });
  }

  moveTo(target: Vec2): RequestEnvelope<{ target: Vec2 }> {
    return this.build(OPERATIONS.moveTo, { target });
  }

  stop(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.stop, {});
  }

  debugSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.debugSnapshot, {});
  }

  combatUseSkill(targetID: string, skillID = 'basic_laser'): RequestEnvelope<{ target_id: string; skill_id: string }> {
    return this.build(OPERATIONS.combatUseSkill, {
      target_id: targetID,
      skill_id: skillID,
    });
  }

  lootPickup(dropID: string): RequestEnvelope<{ drop_id: string }> {
    return this.build(OPERATIONS.lootPickup, {
      drop_id: dropID,
    });
  }

  shieldRepairTick(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.shieldRepairTick, {});
  }

  deathRepairQuote(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.deathRepairQuote, {});
  }

  deathRepairShip(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.deathRepairShip, {});
  }

  progressionSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.progressionSnapshot, {});
  }

  inventorySnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.inventorySnapshot, {});
  }

  hangarSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.hangarSnapshot, {});
  }

  hangarActivateShip(shipID: string): RequestEnvelope<{ ship_id: string }> {
    return this.build(OPERATIONS.hangarActivateShip, {
      ship_id: shipID,
    });
  }

  loadoutSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.loadoutSnapshot, {});
  }

  loadoutEquipModule(
    slotID: string,
    itemInstanceID: string,
  ): RequestEnvelope<{ slot_id: string; item_instance_id: string }> {
    return this.build(OPERATIONS.loadoutEquipModule, {
      slot_id: slotID,
      item_instance_id: itemInstanceID,
    });
  }

  loadoutUnequipModule(slotID: string): RequestEnvelope<{ slot_id: string }> {
    return this.build(OPERATIONS.loadoutUnequipModule, {
      slot_id: slotID,
    });
  }

  statsSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.statsSnapshot, {});
  }

  stealthToggle(enabled: boolean): RequestEnvelope<{ enabled: boolean }> {
    return this.build(OPERATIONS.stealthToggle, { enabled });
  }

  craftingRecipes(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.craftingRecipes, {});
  }

  craftingStart(input: CraftingStartInput): RequestEnvelope<{ recipe_id: string; location_type?: string; location_id?: string }> {
    if (typeof input === 'string') {
      return this.build(OPERATIONS.craftingStart, { recipe_id: input });
    }
    const payload: { recipe_id: string; location_type?: string; location_id?: string } = {
      recipe_id: input.recipeID,
    };
    if (input.locationType) {
      payload.location_type = input.locationType;
    }
    if (input.locationID) {
      payload.location_id = input.locationID;
    }
    return this.build(OPERATIONS.craftingStart, payload);
  }

  craftingComplete(jobID: string): RequestEnvelope<{ job_id: string }> {
    return this.build(OPERATIONS.craftingComplete, { job_id: jobID });
  }

  craftingCancel(jobID: string): RequestEnvelope<{ job_id: string }> {
    return this.build(OPERATIONS.craftingCancel, { job_id: jobID });
  }

  scanPulse(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.scanPulse, {});
  }

  knownPlanets(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.knownPlanets, {});
  }

  planetDetail(planetID: string): RequestEnvelope<{ planet_id: string }> {
    return this.build(OPERATIONS.planetDetail, { planet_id: planetID });
  }

  claimPlanet(planetID: string): RequestEnvelope<{ planet_id: string }> {
    return this.build(OPERATIONS.discoveryClaimPlanet, { planet_id: planetID });
  }

  intelShare(planetID: string, toPlayerID: string): RequestEnvelope<{ planet_id: string; to_player_id: string }> {
    return this.build(OPERATIONS.intelShare, {
      planet_id: planetID,
      to_player_id: toPlayerID,
    });
  }

  intelShareToEntity(planetID: string, toEntityID: string): RequestEnvelope<{ planet_id: string; to_entity_id: string }> {
    return this.build(OPERATIONS.intelShare, {
      planet_id: planetID,
      to_entity_id: toEntityID,
    });
  }

  intelCoordinateItemCreate(planetID: string): RequestEnvelope<{ planet_id: string }> {
    return this.build(OPERATIONS.intelCoordinateItemCreate, { planet_id: planetID });
  }

  intelCoordinateItemUse(itemInstanceID: string): RequestEnvelope<{ item_instance_id: string }> {
    return this.build(OPERATIONS.intelCoordinateItemUse, { item_instance_id: itemInstanceID });
  }

  productionSummary(planetID?: string): RequestEnvelope<{ planet_id?: string }> {
    return this.build(OPERATIONS.productionSummary, planetID ? { planet_id: planetID } : {});
  }

  planetStorageSummary(planetID?: string): RequestEnvelope<{ planet_id?: string }> {
    return this.build(OPERATIONS.planetStorageSummary, planetID ? { planet_id: planetID } : {});
  }

  planetBuildingBuild(input: {
    planetID: string;
    buildingType: string;
    slot: string;
  }): RequestEnvelope<{
    planet_id: string;
    building_type: string;
    slot: string;
  }> {
    return this.build(OPERATIONS.planetBuildingBuild, {
      planet_id: input.planetID,
      building_type: input.buildingType,
      slot: input.slot,
    });
  }

  planetBuildingUpgrade(input: {
    planetID: string;
    buildingID: string;
    targetLevel: number;
  }): RequestEnvelope<{
    planet_id: string;
    building_id: string;
    target_level: number;
  }> {
    return this.build(OPERATIONS.planetBuildingUpgrade, {
      planet_id: input.planetID,
      building_id: input.buildingID,
      target_level: Math.max(1, Math.round(input.targetLevel)),
    });
  }

  routeCreate(input: {
    sourcePlanetID: string;
    destinationPlanetID?: string;
    destination?: RouteDestinationInput;
    resourceItemID: string;
    amountPerHour: number;
  }): RequestEnvelope<{
    source_planet_id: string;
    destination_planet_id?: string;
    destination_type?: RouteDestinationType;
    destination_id?: string;
    resource_item_id: string;
    amount_per_hour: number;
  }> {
    return this.build(
      OPERATIONS.routeCreate,
      {
        source_planet_id: input.sourcePlanetID,
        ...routeDestinationPayload(input),
        resource_item_id: input.resourceItemID,
        amount_per_hour: Math.max(1, Math.round(input.amountPerHour)),
      },
      ['destination_id'],
    );
  }

  routeUpdate(input: {
    routeID: string;
    destinationPlanetID?: string;
    destination?: RouteDestinationInput;
    resourceItemID: string;
    amountPerHour: number;
  }): RequestEnvelope<{
    route_id: string;
    destination_planet_id?: string;
    destination_type?: RouteDestinationType;
    destination_id?: string;
    resource_item_id: string;
    amount_per_hour: number;
  }> {
    return this.build(
      OPERATIONS.routeUpdate,
      {
        route_id: input.routeID,
        ...routeDestinationPayload(input),
        resource_item_id: input.resourceItemID,
        amount_per_hour: Math.max(1, Math.round(input.amountPerHour)),
      },
      ['destination_id'],
    );
  }

  routeEnable(routeID: string): RequestEnvelope<{ route_id: string }> {
    return this.build(OPERATIONS.routeEnable, { route_id: routeID });
  }

  routeDisable(routeID: string): RequestEnvelope<{ route_id: string }> {
    return this.build(OPERATIONS.routeDisable, { route_id: routeID });
  }

  routeList(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.routeList, {});
  }

  routeSnapshot(routeID: string): RequestEnvelope<{ route_id: string }> {
    return this.build(OPERATIONS.routeSnapshot, { route_id: routeID });
  }

  routeSettle(routeID?: string): RequestEnvelope<{ route_id?: string }> {
    return this.build(OPERATIONS.routeSettle, routeID === undefined ? {} : { route_id: routeID });
  }

  walletSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.walletSnapshot, {});
  }

  contentCatalog(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.contentCatalog, {});
  }

  shopCatalog(categoryID?: string): RequestEnvelope<{ category_id?: string }> {
    return this.build(OPERATIONS.shopCatalog, categoryID ? { category_id: categoryID } : {});
  }

  shopBuyProduct(productID: string, quantity = 1): RequestEnvelope<{ product_id: string; quantity: number }> {
    return this.build(OPERATIONS.shopBuyProduct, {
      product_id: productID,
      quantity,
    });
  }

  marketSearch(itemID?: string): RequestEnvelope<{ item_id?: string }> {
    return this.build(OPERATIONS.marketSearch, itemID ? { item_id: itemID } : {});
  }

  marketCreateListing(input: {
    itemID: string;
    quantity: number;
    unitPrice: number;
    sourceLocation?: string;
    itemInstanceID?: string;
  }): RequestEnvelope<{
    item_id: string;
    quantity: number;
    unit_price: number;
    source_location?: string;
    item_instance_id?: string;
  }> {
    return this.build(OPERATIONS.marketCreateListing, {
      item_id: input.itemID,
      quantity: input.quantity,
      unit_price: input.unitPrice,
      ...(input.sourceLocation ? { source_location: input.sourceLocation } : {}),
      ...(input.itemInstanceID ? { item_instance_id: input.itemInstanceID } : {}),
    });
  }

  marketBuy(listingID: string, quantity = 1): RequestEnvelope<{ listing_id: string; quantity: number }> {
    return this.build(OPERATIONS.marketBuy, {
      listing_id: listingID,
      quantity,
    });
  }

  marketCancel(listingID: string): RequestEnvelope<{ listing_id: string }> {
    return this.build(OPERATIONS.marketCancel, {
      listing_id: listingID,
    });
  }

  auctionSearch(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.auctionSearch, {});
  }

  auctionBid(auctionID: string, amount: number): RequestEnvelope<{ auction_id: string; amount: number }> {
    return this.build(OPERATIONS.auctionBid, {
      auction_id: auctionID,
      amount,
    });
  }

  auctionBuyNow(auctionID: string): RequestEnvelope<{ auction_id: string }> {
    return this.build(OPERATIONS.auctionBuyNow, {
      auction_id: auctionID,
    });
  }

  auctionGrants(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.auctionGrants, {});
  }

  premiumEntitlements(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.premiumEntitlements, {});
  }

  premiumClaim(entitlementID: string): RequestEnvelope<{ entitlement_id: string }> {
    return this.build(OPERATIONS.premiumClaim, {
      entitlement_id: entitlementID,
    });
  }

  premiumPurchaseWeeklyXCore(productID: string, periodKey: string): RequestEnvelope<{ product_id: string; period_key: string }> {
    return this.build(OPERATIONS.premiumPurchaseWeeklyXCore, {
      product_id: productID,
      period_key: periodKey,
    });
  }

  questBoard(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.questBoard, {});
  }

  questAccept(offerID: string): RequestEnvelope<{ offer_id: string }> {
    return this.build(OPERATIONS.questAccept, { offer_id: offerID });
  }

  questProgress(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.questProgress, {});
  }

  questClaimReward(questID: string): RequestEnvelope<{ quest_id: string }> {
    return this.build(OPERATIONS.questClaimReward, { quest_id: questID });
  }

  questReroll(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.questReroll, {});
  }

  chatSend(kind: 'local_map' | 'party' | 'clan', content: string): RequestEnvelope<{ kind: string; content: string }> {
    return this.build(OPERATIONS.chatSend, { kind, content });
  }

  partyInvite(inviteeCallsign: string): RequestEnvelope<{ invitee_callsign: string }> {
    return this.build(OPERATIONS.partyInvite, { invitee_callsign: inviteeCallsign });
  }

  partyAccept(inviteID: string): RequestEnvelope<{ invite_id: string }> {
    return this.build(OPERATIONS.partyAccept, { invite_id: inviteID });
  }

  partyLeave(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.partyLeave, {});
  }

  partyTargetSet(targetID: string): RequestEnvelope<{ target_id: string }> {
    return this.build(OPERATIONS.partyTargetSet, { target_id: targetID });
  }

  clanCreate(name: string, tag: string): RequestEnvelope<{ name: string; tag: string }> {
    return this.build(OPERATIONS.clanCreate, { name, tag });
  }

  clanJoin(tag: string): RequestEnvelope<{ tag: string }> {
    return this.build(OPERATIONS.clanJoin, { tag });
  }

  clanLeave(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.clanLeave, {});
  }

  adminInspectPlayer(targetPlayerID?: string): RequestEnvelope<{ target_player_id?: string }> {
    return this.build(OPERATIONS.adminInspectPlayer, targetPlayerID ? { target_player_id: targetPlayerID } : {}, [
      'target_player_id',
    ]);
  }

  adminRepairCraftJob(jobID: string): RequestEnvelope<{ job_id: string }> {
    return this.build(OPERATIONS.adminRepairCraftJob, { job_id: jobID });
  }

  adminEconomyDashboard(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.adminEconomyDashboard, {});
  }

  adminContentVersions(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.adminContentVersions, {});
  }

  adminContentList(contentType = 'module'): RequestEnvelope<{ content_type: string }> {
    return this.build(OPERATIONS.adminContentList, { content_type: contentType });
  }

  adminContentGet(contentType: string, contentID: string): RequestEnvelope<{ content_type: string; content_id: string }> {
    return this.build(OPERATIONS.adminContentGet, { content_type: contentType, content_id: contentID });
  }

  adminContentUpdateDraft(input: {
    contentType: string;
    contentID: string;
    enabled: boolean;
    displayJSON?: AdminContentJSON;
    dataJSON: AdminContentJSON;
  }): RequestEnvelope<{
    content_type: string;
    content_id: string;
    enabled: boolean;
    display_json?: AdminContentJSON;
    data_json: AdminContentJSON;
  }> {
    const payload: {
      content_type: string;
      content_id: string;
      enabled: boolean;
      display_json?: AdminContentJSON;
      data_json: AdminContentJSON;
    } = {
      content_type: input.contentType,
      content_id: input.contentID,
      enabled: input.enabled,
      data_json: input.dataJSON,
    };
    if (input.displayJSON) {
      payload.display_json = input.displayJSON;
    }
    return this.build(OPERATIONS.adminContentUpdateDraft, payload, [], true);
  }

  adminContentValidateDraft(version?: string): RequestEnvelope<{ version?: string }> {
    return this.build(OPERATIONS.adminContentValidateDraft, version ? { version } : {});
  }

  adminContentPublish(input: {
    version?: string;
    notes?: string;
    balanceTag?: string;
  } = {}): RequestEnvelope<{ version?: string; notes?: string; balance_tag?: string }> {
    return this.build(OPERATIONS.adminContentPublish, {
      ...(input.version ? { version: input.version } : {}),
      ...(input.notes ? { notes: input.notes } : {}),
      ...(input.balanceTag ? { balance_tag: input.balanceTag } : {}),
    });
  }

  adminContentRollback(input: {
    targetVersionID: string;
    version?: string;
    notes?: string;
    balanceTag?: string;
  }): RequestEnvelope<{ target_version_id: string; version?: string; notes?: string; balance_tag?: string }> {
    return this.build(OPERATIONS.adminContentRollback, {
      target_version_id: input.targetVersionID,
      ...(input.version ? { version: input.version } : {}),
      ...(input.notes ? { notes: input.notes } : {}),
      ...(input.balanceTag ? { balance_tag: input.balanceTag } : {}),
    });
  }

  adminContentAuditLog(input: {
    versionID?: string;
    contentType?: string;
    contentID?: string;
    limit?: number;
    offset?: number;
  } = {}): RequestEnvelope<{
    version_id?: string;
    content_type?: string;
    content_id?: string;
    limit?: number;
    offset?: number;
  }> {
    return this.build(OPERATIONS.adminContentAuditLog, {
      ...(input.versionID ? { version_id: input.versionID } : {}),
      ...(input.contentType ? { content_type: input.contentType } : {}),
      ...(input.contentID ? { content_id: input.contentID } : {}),
      ...(input.limit ? { limit: input.limit } : {}),
      ...(input.offset ? { offset: input.offset } : {}),
    });
  }

  observabilityCommandLog(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.observabilityCommandLog, {});
  }

  observabilityMetrics(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.observabilityMetrics, {});
  }

  observabilityReleaseGate(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.observabilityReleaseGate, {});
  }

  observabilityAbuseCoverage(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.observabilityAbuseCoverage, {});
  }

  debugSpawnNPC(entityID: string, position: Vec2): RequestEnvelope<{ entity_id: string; position: Vec2 }> {
    return this.build(OPERATIONS.debugSpawnNPC, {
      entity_id: entityID,
      position,
    });
  }

  private build<TPayload extends CommandPayload>(
    op: Operation,
    payload: TPayload,
    trustedTopLevelFieldAllowlist: readonly string[] = [],
    allowAdminContentFields = false,
  ): RequestEnvelope<TPayload> {
    assertClientSafePayloadInternal(payload, trustedTopLevelFieldAllowlist, allowAdminContentFields);
    this.clientSeq += 1;
    return {
      request_id: createRequestId(),
      op,
      payload,
      client_seq: this.clientSeq,
      v: PROTOCOL_VERSION,
    };
  }
}

export function assertClientSafePayload(payload: CommandPayload): void {
  assertClientSafePayloadInternal(payload);
}

function routeDestinationPayload(input: {
  destinationPlanetID?: string;
  destination?: RouteDestinationInput;
}): { destination_planet_id?: string; destination_type?: RouteDestinationType; destination_id?: string } {
  if (input.destination) {
    if (input.destination.type === 'planet') {
      return { destination_planet_id: input.destination.id };
    }
    return {
      destination_type: input.destination.type,
      destination_id: input.destination.id,
    };
  }
  return { destination_planet_id: input.destinationPlanetID ?? '' };
}

function assertClientSafePayloadInternal(
  payload: CommandPayload,
  trustedTopLevelFieldAllowlist: readonly string[] = [],
  allowAdminContentFields = false,
): void {
  const forbidden = findTrustedClientField(
    payload,
    new Set(trustedTopLevelFieldAllowlist.map((field) => field.toLowerCase())),
    allowAdminContentFields,
  );
  if (forbidden) {
    throw new Error(`Command payload must not include trusted field: ${forbidden}`);
  }
}

const adminContentTrustedFieldAllowlist = new Set([
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

function findTrustedClientField(
  value: unknown,
  trustedTopLevelFieldAllowlist: ReadonlySet<string>,
  allowAdminContentFields: boolean,
  depth = 0,
): string | null {
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findTrustedClientField(item, trustedTopLevelFieldAllowlist, allowAdminContentFields, depth + 1);
      if (found) {
        return found;
      }
    }
    return null;
  }

  if (typeof value !== 'object' || value === null) {
    return null;
  }

  for (const [key, child] of Object.entries(value)) {
    const normalized = key.toLowerCase();
    if (depth === 0 && trustedTopLevelFieldAllowlist.has(normalized)) {
      const childFound = findTrustedClientField(child, new Set(), allowAdminContentFields, depth + 1);
      if (childFound) {
        return childFound;
      }
      continue;
    }
    if (
      normalized === 'player_id' ||
      normalized === 'client_player_id' ||
      normalized === 'account_id' ||
      normalized === 'session_id' ||
      normalized === 'world_id' ||
      normalized === 'zone_id' ||
      normalized === 'map_id' ||
      normalized === 'map_key' ||
      normalized === 'internal_map_id' ||
      normalized === 'public_map_key' ||
      normalized === 'worker_id' ||
      normalized === 'map_worker_id' ||
      normalized === 'transfer_id' ||
      normalized === 'transfer_token' ||
      normalized === 'destination' ||
      normalized === 'destination_worker' ||
      normalized === 'origin_worker' ||
      normalized === 'destination_id' ||
      normalized === 'destination_map_id' ||
      normalized === 'destination_map_key' ||
      normalized === 'destination_public_key' ||
      normalized === 'destination_public_map_key' ||
      normalized === 'destination_spawn_id' ||
      normalized === 'spawn' ||
      normalized === 'spawn_point' ||
      normalized === 'spawn_position' ||
      normalized === 'to_map_key' ||
      normalized === 'to_public_map_key' ||
      normalized === 'damage' ||
      normalized === 'speed' ||
      normalized === 'xp' ||
      normalized === 'main_xp' ||
      normalized === 'combat_xp' ||
      normalized === 'role_xp' ||
      normalized === 'rank' ||
      normalized === 'skill_points' ||
      normalized === 'loot' ||
      normalized === 'cooldown' ||
      normalized === 'cooldown_ready_at_ms' ||
      normalized === 'ready_at_ms' ||
      normalized === 'expires_at' ||
      normalized === 'wallet_amount' ||
      normalized === 'hidden' ||
      normalized === 'internal' ||
      normalized === 'internal_metadata' ||
      normalized === 'balance' ||
      normalized === 'balance_after' ||
      normalized === 'total' ||
      normalized === 'total_amount' ||
      normalized === 'price_total' ||
      normalized === 'fee' ||
      normalized === 'fee_amount' ||
      normalized === 'seller_proceeds' ||
      normalized === 'escrow' ||
      normalized === 'escrow_location' ||
      normalized === 'source_return_location' ||
      normalized === 'seller_player_id' ||
      normalized === 'buyer_player_id' ||
      normalized === 'bidder_player_id' ||
      normalized === 'current_bid' ||
      normalized === 'current_bidder_id' ||
      normalized === 'winning_player_id' ||
      normalized === 'stock_total' ||
      normalized === 'stock_remaining' ||
      normalized === 'provider' ||
      normalized === 'provider_reference' ||
      normalized === 'entitlement_state' ||
      normalized === 'quest_progress' ||
      normalized === 'progress' ||
      normalized === 'progress_json' ||
      normalized === 'objective_progress' ||
      normalized === 'completed' ||
      normalized === 'completed_at' ||
      normalized === 'claimed_at' ||
      normalized === 'reward' ||
      normalized === 'reward_payload' ||
      normalized === 'reward_claimed_at' ||
      normalized === 'generated_payload' ||
      normalized === 'generated_seed' ||
      normalized === 'rare_cap' ||
      normalized === 'reference_id' ||
      normalized === 'token' ||
      normalized === 'session_token' ||
      normalized === 'reset_secret' ||
      normalized === 'auth_header' ||
      normalized === 'hit' ||
      normalized === 'crit' ||
      normalized === 'gameplay_seed' ||
      normalized === 'procedural_seed' ||
      normalized === 'world_seed' ||
      normalized === 'future_spawn' ||
      normalized === 'future_spawn_data' ||
      normalized === 'spawn_candidates' ||
      normalized === 'candidate' ||
      normalized === 'candidate_key' ||
      normalized === 'planet_candidate' ||
      normalized === 'detection_roll' ||
      normalized === 'scan_roll' ||
      normalized === 'scan_cell' ||
      normalized === 'scan_result' ||
      normalized === 'scan_candidate' ||
      normalized === 'scan_candidates' ||
      normalized === 'candidate_data' ||
      normalized === 'target_player_id' ||
      normalized === 'witness_expires_at' ||
      normalized === 'witness_expiry' ||
      normalized === 'hidden_target_metadata' ||
      normalized === 'loot_roll' ||
      normalized === 'loot_table' ||
      normalized === 'password' ||
      normalized === 'password_hash' ||
      normalized === 'cookie'
    ) {
      if (allowAdminContentFields && adminContentTrustedFieldAllowlist.has(normalized)) {
        const childFound = findTrustedClientField(child, trustedTopLevelFieldAllowlist, allowAdminContentFields, depth + 1);
        if (childFound) {
          return childFound;
        }
        continue;
      }
      return key;
    }

    const childFound = findTrustedClientField(child, trustedTopLevelFieldAllowlist, allowAdminContentFields, depth + 1);
    if (childFound) {
      return childFound;
    }
  }

  return null;
}

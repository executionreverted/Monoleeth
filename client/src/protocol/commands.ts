import { createRequestId } from './request-id';
import { JsonObject, OPERATIONS, Operation, PROTOCOL_VERSION, RequestEnvelope, Vec2 } from './envelope';

type CommandPayload = JsonObject;

export class CommandBuilder {
  private clientSeq = 0;

  sessionSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.sessionSnapshot, {});
  }

  worldSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.worldSnapshot, {});
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

  scanPulse(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.scanPulse, {});
  }

  knownPlanets(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.knownPlanets, {});
  }

  planetDetail(planetID: string): RequestEnvelope<{ planet_id: string }> {
    return this.build(OPERATIONS.planetDetail, { planet_id: planetID });
  }

  productionSummary(planetID?: string): RequestEnvelope<{ planet_id?: string }> {
    return this.build(OPERATIONS.productionSummary, planetID ? { planet_id: planetID } : {});
  }

  planetStorageSummary(planetID?: string): RequestEnvelope<{ planet_id?: string }> {
    return this.build(OPERATIONS.planetStorageSummary, planetID ? { planet_id: planetID } : {});
  }

  routeList(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.routeList, {});
  }

  routeSnapshot(routeID: string): RequestEnvelope<{ route_id: string }> {
    return this.build(OPERATIONS.routeSnapshot, { route_id: routeID });
  }

  walletSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.walletSnapshot, {});
  }

  shopCatalog(categoryID?: string): RequestEnvelope<{ category_id?: string }> {
    return this.build(OPERATIONS.shopCatalog, categoryID ? { category_id: categoryID } : {});
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
  ): RequestEnvelope<TPayload> {
    assertClientSafePayloadInternal(payload, trustedTopLevelFieldAllowlist);
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

function assertClientSafePayloadInternal(
  payload: CommandPayload,
  trustedTopLevelFieldAllowlist: readonly string[] = [],
): void {
  const forbidden = findTrustedClientField(
    payload,
    new Set(trustedTopLevelFieldAllowlist.map((field) => field.toLowerCase())),
  );
  if (forbidden) {
    throw new Error(`Command payload must not include trusted field: ${forbidden}`);
  }
}

function findTrustedClientField(value: unknown, trustedTopLevelFieldAllowlist: ReadonlySet<string>, depth = 0): string | null {
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findTrustedClientField(item, trustedTopLevelFieldAllowlist, depth + 1);
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
      const childFound = findTrustedClientField(child, new Set(), depth + 1);
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
      return key;
    }

    const childFound = findTrustedClientField(child, trustedTopLevelFieldAllowlist, depth + 1);
    if (childFound) {
      return childFound;
    }
  }

  return null;
}

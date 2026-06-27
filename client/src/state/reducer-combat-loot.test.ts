import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event, expectServerOwnedGameplayCleared, stateWithServerOwnedGameplay } from './reducer-fixtures.test-support';
import { isWithinMinimapProjectionWindow, worldMapMemoryMarkers } from './world-memory';

describe('reduceClientState', () => {
  test('authoritative realtime events clear matching pending commands when responses are lost', () => {
    const withSelf = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.entityEntered,
        {
          entity_id: 'player-local',
          entity_type: 'player',
          position: { x: 0, y: 0 },
          status_flags: ['self'],
        },
        1,
      ),
    });
    const queuedMove = reduceClientState(withSelf, {
      type: 'requestQueued',
      envelope: {
        request_id: 'move-1',
        op: OPERATIONS.moveTo,
        payload: { target: { x: 100, y: 0 } },
        client_seq: 1,
        v: 1,
      },
    });
    const corrected = reduceClientState(queuedMove, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.positionCorrected,
        {
          entity_id: 'player-local',
          position: { x: 12, y: 0 },
          movement: {
            moving: true,
            origin: { x: 12, y: 0 },
            target: { x: 100, y: 0 },
            speed: 180,
            started_at_ms: 1000,
            arrive_at_ms: 1489,
          },
        },
        2,
      ),
    });

    expect(corrected.pendingCommands['move-1']).toBeUndefined();
    expect(corrected.movementTarget).toEqual({ x: 100, y: 0 });

    const queuedActions = {
      ...corrected,
      pendingCommands: {
        'scan-1': { requestID: 'scan-1', op: OPERATIONS.scanPulse, queuedAt: 1 },
        'loot-1': { requestID: 'loot-1', op: OPERATIONS.lootPickup, queuedAt: 1 },
        'combat-1': { requestID: 'combat-1', op: OPERATIONS.combatUseSkill, queuedAt: 1 },
        'auction-1': { requestID: 'auction-1', op: OPERATIONS.auctionBid, queuedAt: 1 },
        'quote-1': { requestID: 'quote-1', op: OPERATIONS.deathRepairQuote, queuedAt: 1 },
      },
    };
    const scanStarted = reduceClientState(queuedActions, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.scanPulseStarted, { pulse_reference: 'pulse-1', status: 'started' }, 3),
    });
    const lootPickedUp = reduceClientState(scanStarted, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.lootPickedUp, { drop_id: 'drop-1', item_id: 'raw_ore', quantity: 2 }, 4),
    });
    const combatCooldown = reduceClientState(lootPickedUp, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.combatCooldownStarted, { skill_id: 'basic_laser', target_id: 'npc-1' }, 5),
    });
    const auctionBidPlaced = reduceClientState(combatCooldown, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.auctionBidPlaced, { auction_id: 'auction-1' }, 6),
    });

    expect(auctionBidPlaced.pendingCommands['scan-1']).toBeUndefined();
    expect(auctionBidPlaced.pendingCommands['loot-1']).toBeUndefined();
    expect(auctionBidPlaced.pendingCommands['combat-1']).toBeUndefined();
    expect(auctionBidPlaced.pendingCommands['auction-1']).toBeUndefined();
    expect(auctionBidPlaced.pendingCommands['quote-1']).toBeDefined();

    const queuedEconomyActions = {
      ...auctionBidPlaced,
      pendingCommands: {
        ...auctionBidPlaced.pendingCommands,
        'market-create-1': { requestID: 'market-create-1', op: OPERATIONS.marketCreateListing, queuedAt: 1 },
        'market-buy-1': { requestID: 'market-buy-1', op: OPERATIONS.marketBuy, queuedAt: 1 },
        'market-cancel-1': { requestID: 'market-cancel-1', op: OPERATIONS.marketCancel, queuedAt: 1 },
        'auction-buy-now-1': { requestID: 'auction-buy-now-1', op: OPERATIONS.auctionBuyNow, queuedAt: 1 },
        'premium-claim-1': { requestID: 'premium-claim-1', op: OPERATIONS.premiumClaim, queuedAt: 1 },
        'premium-weekly-1': { requestID: 'premium-weekly-1', op: OPERATIONS.premiumPurchaseWeeklyXCore, queuedAt: 1 },
      },
    };
    const marketUpdated = reduceClientState(
      {
        ...auctionBidPlaced,
        pendingCommands: {
          ...auctionBidPlaced.pendingCommands,
          'market-update-buy-1': { requestID: 'market-update-buy-1', op: OPERATIONS.marketBuy, queuedAt: 1 },
          'market-update-create-1': { requestID: 'market-update-create-1', op: OPERATIONS.marketCreateListing, queuedAt: 1 },
          'market-update-cancel-1': { requestID: 'market-update-cancel-1', op: OPERATIONS.marketCancel, queuedAt: 1 },
        },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.marketListingUpdated, { listing_id: 'listing-1' }, 7),
      },
    );
    expect(marketUpdated.pendingCommands['market-update-buy-1']).toBeDefined();
    expect(marketUpdated.pendingCommands['market-update-create-1']).toBeDefined();
    expect(marketUpdated.pendingCommands['market-update-cancel-1']).toBeDefined();

    const auctionLotUpdated = reduceClientState(
      {
        ...auctionBidPlaced,
        pendingCommands: {
          ...auctionBidPlaced.pendingCommands,
          'auction-update-bid-1': { requestID: 'auction-update-bid-1', op: OPERATIONS.auctionBid, queuedAt: 1 },
          'auction-update-buy-now-1': { requestID: 'auction-update-buy-now-1', op: OPERATIONS.auctionBuyNow, queuedAt: 1 },
        },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.auctionLotUpdated, { auction_id: 'auction-1' }, 8),
      },
    );
    expect(auctionLotUpdated.pendingCommands['auction-update-bid-1']).toBeDefined();
    expect(auctionLotUpdated.pendingCommands['auction-update-buy-now-1']).toBeDefined();

    const marketCreated = reduceClientState(queuedEconomyActions, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.marketListingCreated, { listing_id: 'listing-1' }, 9),
    });
    const marketSold = reduceClientState(marketCreated, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.marketSaleCompleted, { listing_id: 'listing-1' }, 10),
    });
    const marketCancelled = reduceClientState(marketSold, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.marketListingCancelled, { listing_id: 'listing-1' }, 11),
    });
    const auctionClosed = reduceClientState(marketCancelled, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.auctionClosed, { auction_id: 'auction-1' }, 12),
    });
    const premiumClaimed = reduceClientState(auctionClosed, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.premiumEntitlementClaimed, { entitlement_id: 'entitlement-1' }, 13),
    });
    const premiumStockConsumed = reduceClientState(premiumClaimed, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.premiumStockConsumed, { product_id: 'weekly_xcore', period_key: '2026-W25' }, 14),
    });

    expect(premiumStockConsumed.pendingCommands['market-create-1']).toBeUndefined();
    expect(premiumStockConsumed.pendingCommands['market-buy-1']).toBeUndefined();
    expect(premiumStockConsumed.pendingCommands['market-cancel-1']).toBeUndefined();
    expect(premiumStockConsumed.pendingCommands['auction-buy-now-1']).toBeUndefined();
    expect(premiumStockConsumed.pendingCommands['premium-claim-1']).toBeUndefined();
    expect(premiumStockConsumed.pendingCommands['premium-weekly-1']).toBeUndefined();
    expect(premiumStockConsumed.pendingCommands['quote-1']).toBeDefined();
  });

  test('phase 05 combat, loot, progression, and repair events reconcile server-owned state', () => {
    const withPlayer = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'player-1',
        entity_type: 'player',
        position: { x: 0, y: 0 },
        status_flags: ['self'],
      }),
    });
    const withNPC = reduceClientState(withPlayer, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 80, y: 0 },
        display: { label: 'Training Drone', disposition: 'hostile' },
        combat: { hp: 40, max_hp: 40, shield: 10, max_shield: 10, status: 'active' },
      }, 2),
    });
    const targeted = reduceClientState({ ...withNPC, selectedTargetID: 'npc-1' }, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.targetUpdated, {
        entity_id: 'npc-1',
        combat: { hp: 0, max_hp: 40, shield: 0, max_shield: 10, status: 'destroyed' },
      }, 3),
    });
    const withCooldown = reduceClientState(targeted, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.combatCooldownStarted, {
        skill_id: 'basic_laser',
        target_id: 'npc-1',
        cooldown_ready_at_ms: 9000,
      }, 4),
    });
    const withDuplicateCooldown = reduceClientState(withCooldown, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.combatCooldownStarted, {
        skill_id: 'basic_laser',
        target_id: 'npc-1',
        cooldown_ready_at_ms: 9000,
      }, 4),
    });
    const withDamage = reduceClientState(withCooldown, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.combatDamage, {
        target_id: 'npc-1',
        amount: 45,
      }, 5),
    });
    const withDropNotice = reduceClientState(withDamage, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.lootCreated, {
        drop_id: 'drop-1',
        item_id: 'raw_ore',
        quantity: 3,
        state: 'active',
        position: { x: 80, y: 0 },
      }, 6),
    });
    const withLootEntity = reduceClientState(withDropNotice, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'drop-1',
        entity_type: 'loot',
        position: { x: 80, y: 0 },
        display: { label: 'Raw Ore', disposition: 'neutral' },
      }, 7),
    });
    const leftClearsKnownLoot = reduceClientState(withLootEntity, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityLeft, { entity_id: 'drop-1' }, 8),
    });
    const afterPickup = reduceClientState(
      {
        ...withLootEntity,
        selectedTargetID: 'drop-1',
      },
      {
        type: 'responseReceived',
        envelope: {
          request_id: 'pickup-1',
          ok: true,
          payload: {
            cargo: { used: 6, capacity: 60, items: [{ item_id: 'raw_ore', quantity: 3 }] },
          },
          server_time: 1006,
          v: 1,
        },
      },
    );
    const withPickupNotice = reduceClientState(afterPickup, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.lootPickedUp, { drop_id: 'drop-1', item_id: 'raw_ore', quantity: 3 }, 8),
    });
    const withoutLootEntity = reduceClientState(withPickupNotice, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.lootRemoved, { entity_id: 'drop-1' }, 9),
    });
    const progressed = reduceClientState(withoutLootEntity, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.progressionSnapshot, { main_level: 2, main_xp: 100, rank: 2, combat_xp: 40 }, 10),
    });
    const quoted = reduceClientState(progressed, {
      type: 'responseReceived',
      envelope: {
        request_id: 'quote-1',
        ok: true,
        payload: { ship_id: 'starter', cost: 0, currency: 'credits', disabled: true },
        server_time: 1009,
        v: 1,
      },
    });
    const repaired = reduceClientState(quoted, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.deathRepaired, { ship_id: 'starter' }, 11),
    });

    expect(targeted.visibleEntities['npc-1'].combat).toMatchObject({ hp: 0, shield: 0, status: 'destroyed' });
    expect(withCooldown.skillCooldowns.basic_laser).toBe(9000);
    expect(withCooldown.worldEffects).toContainEqual(
      expect.objectContaining({
        id: 'event-4:laser',
        kind: 'laser',
        sourceID: 'player-1',
        sourcePosition: { x: 0, y: 0 },
        targetID: 'npc-1',
        position: { x: 80, y: 0 },
      }),
    );
    expect(withDuplicateCooldown.worldEffects.filter((effect) => effect.id === 'event-4:laser')).toHaveLength(1);
    expect(withDamage.combatLog.at(-1)?.text).toContain('Hit Training Drone for 45.');
    expect(withDamage.worldEffects.some((effect) => effect.kind === 'damage' && effect.amount === 45)).toBe(true);
    expect(withDropNotice.knownLoot['drop-1']).toMatchObject({ item_id: 'raw_ore', quantity: 3 });
    expect(withDropNotice.worldEffects.some((effect) => effect.kind === 'loot_spawn' && effect.targetID === 'drop-1')).toBe(true);
    expect(leftClearsKnownLoot.knownLoot['drop-1']).toBeUndefined();
    expect(afterPickup.cargo?.items).toEqual([{ item_id: 'raw_ore', quantity: 3 }]);
    expect(withPickupNotice.worldEffects.some((effect) => effect.kind === 'loot_pickup' && effect.itemID === 'raw_ore')).toBe(true);
    expect(withoutLootEntity.visibleEntities['drop-1']).toBeUndefined();
    expect(withoutLootEntity.knownLoot['drop-1']).toBeUndefined();
    expect(withoutLootEntity.selectedTargetID).toBeNull();
    expect(progressed.progression).toMatchObject({ main_level: 2, rank: 2, combat_xp: 40 });
    expect(quoted.repairQuote).toMatchObject({ ship_id: 'starter', cost: 0, currency: 'credits', disabled: true });
    expect(repaired.repairQuote).toBeNull();
  });

  test('death disabled event marks active ship disabled without inventing a repair quote', () => {
    const withShip = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'ship-snapshot',
        ok: true,
        payload: {
          ship: {
            active_ship_id: 'starter',
            display_name: 'Sparrow',
            hull: 44,
            max_hull: 100,
            shield: 12,
            max_shield: 60,
            capacitor: 20,
            max_capacitor: 40,
            disabled: false,
            repair_state: 'ready',
          },
        },
        server_time: 1000,
        v: 1,
      },
    });

    const disabled = reduceClientState(withShip, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.deathShipDisabled,
        {
          ship_id: 'starter',
          disabled_reason: 'combat',
          ship: {
            active_ship_id: 'starter',
            display_name: 'Sparrow',
            hull: 0,
            max_hull: 100,
            shield: 0,
            max_shield: 60,
            capacitor: 0,
            max_capacitor: 40,
            disabled: true,
            repair_state: 'disabled',
          },
          repair_quote: {
            ship_id: 'starter',
            cost: 15,
            currency: 'credits',
            disabled: true,
            quote_id: 'quote-1',
            issued_at_ms: 1000,
            expires_at_ms: 61000,
          },
          respawn_location_id: 'origin-station',
        },
        2,
      ),
    });

    expect(disabled.ship).toMatchObject({
      active_ship_id: 'starter',
      disabled: true,
      repair_state: 'disabled',
      hull: 0,
    });
    expect(disabled.repairQuote).toMatchObject({ ship_id: 'starter', cost: 15, currency: 'credits', disabled: true });
    expect(disabled.combatLog.at(-1)?.text).toBe('Ship disabled.');

    const quoted = reduceClientState(disabled, {
      type: 'responseReceived',
      envelope: {
        request_id: 'repair-quote',
        ok: true,
        payload: {
          ship_id: 'starter',
          cost: 15,
          currency: 'credits',
          disabled: true,
          quote_id: 'quote-2',
          issued_at_ms: 1003,
          expires_at_ms: 61003,
        },
        server_time: 1003,
        v: 1,
      },
    });

    expect(quoted.repairQuote).toMatchObject({ ship_id: 'starter', cost: 15, currency: 'credits', disabled: true });
  });
});

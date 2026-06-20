import { describe, expect, test } from 'vitest';

import { CommandBuilder } from './commands';
import { CLIENT_EVENTS, OPERATIONS, parseServerMessage, rejectForbiddenPayloadKeys } from './envelope';

const UNIMPLEMENTED_MUTATION_OPS = [
  'crafting.start',
  'crafting.complete',
  'crafting.cancel',
  'inventory.move',
  'progression.unlock_skill',
  'progression.respec_skills',
  'discovery.claim_planet',
  'planet.building_build',
  'planet.building_upgrade',
  'route.create',
  'route.update',
  'route.enable',
  'route.disable',
  'route.settle',
  'intel.share',
  'intel.coordinate_item_create',
  'intel.coordinate_item_use',
  'coordinate_scroll.create',
  'coordinate_scroll.use',
  'mail.send',
  'social.friend_request',
  'social.party_invite',
] as const;

describe('parseServerMessage', () => {
  test('parses a successful response envelope', () => {
    const message = parseServerMessage(
      JSON.stringify({
        request_id: 'request-1',
        ok: true,
        payload: { accepted: true },
        server_time: 182736123,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      request_id: 'request-1',
      ok: true,
      payload: { accepted: true },
      server_time: 182736123,
      v: 1,
    });
  });

  test('parses a client-safe event envelope', () => {
    const message = parseServerMessage(
      JSON.stringify({
        event_id: 'event-1',
        type: CLIENT_EVENTS.entityEntered,
        payload: {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 10, y: 20 },
          display: { label: 'Training Drone', disposition: 'hostile' },
        },
        server_time: 182736124,
        seq: 9,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      event_id: 'event-1',
      type: CLIENT_EVENTS.entityEntered,
      payload: {
        entity_id: 'npc-1',
      },
    });
  });

  test.each([
    'account_id',
    'client_player_id',
    'player_id',
    'session_id',
    'world_id',
    'zone_id',
    'gameplay_seed',
    'procedural_seed',
    'world_seed',
    'future_spawn_data',
    'detection_roll',
    'target_player_id',
    'witness_expires_at',
    'witness_expiry',
    'hidden_target_metadata',
    'provider',
    'provider_reference',
    'source_return_location',
    'seller_player_id',
    'buyer_player_id',
    'bidder_player_id',
    'winning_player_id',
    'generated_payload',
    'generated_seed',
    'loot_roll',
    'password',
    'password_hash',
    'token',
    'session_token',
    'reset_secret',
    'auth_header',
    'cookie',
    'scan_cell',
    'scan_result',
    'scan_roll',
    'scan_candidate',
    'scan_candidates',
    'candidate_data',
  ])('rejects hidden or internal server payload field %s', (field) => {
    expect(() =>
      parseServerMessage(
        JSON.stringify({
          event_id: `event-${field}`,
          type: CLIENT_EVENTS.entityEntered,
          payload: {
            entity_id: 'visible-contact',
            entity_type: 'player',
            position: { x: 1, y: 2 },
            nested: { [field]: 'server-only' },
          },
          server_time: 1,
          seq: 1,
          v: 1,
        }),
      ),
    ).toThrow(/Forbidden server payload rejected/);
  });

  test('accepts public progression fields from server payloads', () => {
    const message = parseServerMessage(
      JSON.stringify({
        request_id: 'request-progression',
        ok: true,
        payload: {
          progression: {
            main_level: 2,
            main_xp: 175,
            rank: 2,
            combat_level: 1,
            combat_xp: 25,
            role_rewards: [{ role: 'combat', xp: 25 }],
          },
        },
        server_time: 182736125,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      request_id: 'request-progression',
      ok: true,
      payload: { progression: { rank: 2, main_xp: 175, role_rewards: [{ xp: 25 }] } },
    });
  });

  test('accepts phase 05 combat payloads that do not expose server-only truth fields', () => {
    const message = parseServerMessage(
      JSON.stringify({
        event_id: 'combat-1',
        type: CLIENT_EVENTS.combatDamage,
        payload: {
          target_id: 'npc-1',
          amount: 32,
          shield_amount: 10,
          hull_amount: 22,
        },
        server_time: 182736125,
        seq: 10,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      event_id: 'combat-1',
      type: CLIENT_EVENTS.combatDamage,
      payload: { amount: 32 },
    });
  });

  test('accepts phase 09 public quest payloads without raw generation data', () => {
    const message = parseServerMessage(
      JSON.stringify({
        event_id: 'quest-1',
        type: CLIENT_EVENTS.questProgressed,
        payload: {
          quest_id: 'quest-1',
          quest_type: 'kill',
          title: 'Training Sweep',
          state: 'completed',
          objectives: [{ id: 'kill', kind: 'kill', target: 'pirate', current: 3, required: 3, completed: true }],
          rewards: [{ kind: 'currency', currency_type: 'credits', amount: 100 }],
          accepted_at: 1000,
          completed_at: 2000,
          can_claim: true,
        },
        server_time: 182736126,
        seq: 11,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      event_id: 'quest-1',
      type: CLIENT_EVENTS.questProgressed,
      payload: { can_claim: true },
    });
  });

  test('rejects phase 09 raw quest internals', () => {
    expect(() =>
      parseServerMessage(
        JSON.stringify({
          event_id: 'quest-hidden',
          type: CLIENT_EVENTS.questBoardGenerated,
          payload: {
            offers: [{ offer_id: 'offer-1', generated_payload: { rare_cap: 99 } }],
          },
          server_time: 1,
          seq: 1,
          v: 1,
        }),
      ),
    ).toThrow(/Forbidden server payload rejected/);
  });

  test('rejects unsupported protocol versions', () => {
    expect(() =>
      parseServerMessage(
        JSON.stringify({
          request_id: 'request-1',
          ok: true,
          payload: {},
          server_time: 1,
          v: 999,
        }),
      ),
    ).toThrow(/Unsupported protocol version/);
  });
});

describe('rejectForbiddenPayloadKeys', () => {
  test('checks nested payloads', () => {
    expect(() => rejectForbiddenPayloadKeys({ entities: [{ future_spawn_data: ['x'] }] })).toThrow(
      /Forbidden server payload rejected/,
    );
  });
});

describe('default outbound operations', () => {
  test('include server-owned hangar and loadout mutation contracts', () => {
    expect(OPERATIONS.hangarActivateShip).toBe('hangar.activate_ship');
    expect(OPERATIONS.loadoutEquipModule).toBe('loadout.equip_module');
    expect(OPERATIONS.loadoutUnequipModule).toBe('loadout.unequip_module');
    expect(OPERATIONS.stealthToggle).toBe('stealth.toggle');
    expect(OPERATIONS.shopCatalog).toBe('shop.catalog');
    expect(OPERATIONS.shopBuyProduct).toBe('shop.buy_product');

    const builder = new CommandBuilder();
    expect(builder.hangarActivateShip('starter').payload).toEqual({ ship_id: 'starter' });
    expect(builder.loadoutEquipModule('offensive_1', 'laser_alpha_t1-instance-2').payload).toEqual({
      slot_id: 'offensive_1',
      item_instance_id: 'laser_alpha_t1-instance-2',
    });
    expect(builder.loadoutUnequipModule('offensive_1').payload).toEqual({ slot_id: 'offensive_1' });
    expect(builder.stealthToggle(true).payload).toEqual({ enabled: true });
    expect(builder.shopCatalog().payload).toEqual({});
    expect(builder.shopCatalog('weapons').payload).toEqual({ category_id: 'weapons' });
    expect(builder.shopBuyProduct('product_module_laser_alpha_t1', 1).payload).toEqual({
      product_id: 'product_module_laser_alpha_t1',
      quantity: 1,
    });
  });

  test('exclude unimplemented browser mutation contracts', () => {
    const allowedOperations = new Set<string>(Object.values(OPERATIONS));

    for (const operation of UNIMPLEMENTED_MUTATION_OPS) {
      expect(allowedOperations.has(operation)).toBe(false);
    }
  });

  test('do not expose command-builder helpers for unimplemented browser mutations', () => {
    const builderMethods = new Set(Object.getOwnPropertyNames(CommandBuilder.prototype));
    const forbiddenMethodNames = [
      'craftingStart',
      'craftingComplete',
      'craftingCancel',
      'inventoryMove',
      'progressionUnlockSkill',
      'progressionRespecSkills',
      'discoveryClaimPlanet',
      'planetBuildingBuild',
      'planetBuildingUpgrade',
      'routeCreate',
      'routeUpdate',
      'routeEnable',
      'routeDisable',
      'routeSettle',
      'intelShare',
      'intelCoordinateItemCreate',
      'intelCoordinateItemUse',
      'coordinateScrollCreate',
      'coordinateScrollUse',
      'mailSend',
      'socialFriendRequest',
      'socialPartyInvite',
    ];

    for (const method of forbiddenMethodNames) {
      expect(builderMethods.has(method)).toBe(false);
    }
  });
});

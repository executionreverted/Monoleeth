import { describe, expect, test } from 'vitest';

import { assertClientSafePayload, CommandBuilder } from './commands';
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

  test('accepts public map keys and map subscription epoch in snapshots', () => {
    const message = parseServerMessage(
      JSON.stringify({
        event_id: 'world-snapshot-1',
        type: CLIENT_EVENTS.worldSnapshot,
        payload: {
          map_subscription_epoch: 2,
          map: {
            map_key: '1-2',
            public_map_key: '1-2',
            display_name: 'Outer Ring',
            bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
          },
          entities: [],
          minimap: { radar_range: 420, live_contacts: [], remembered: [] },
        },
        server_time: 182736126,
        seq: 12,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      type: CLIENT_EVENTS.worldSnapshot,
      payload: { map_subscription_epoch: 2, map: { public_map_key: '1-2' } },
    });
  });

  test('accepts direct public map snapshot payloads', () => {
    const message = parseServerMessage(
      JSON.stringify({
        event_id: 'map-snapshot-1',
        type: CLIENT_EVENTS.mapSnapshot,
        payload: {
          map_subscription_epoch: 3,
          public_map_key: '2-1',
          display_name: 'Rust Frontier',
          bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
          visible_portals: [],
          safe_zones: [],
        },
        server_time: 182736127,
        seq: 13,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      type: CLIENT_EVENTS.mapSnapshot,
      payload: { map_subscription_epoch: 3, public_map_key: '2-1' },
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
    expect(OPERATIONS.portalEnter).toBe('portal.enter');
    expect(CLIENT_EVENTS.mapSnapshot).toBe('map.snapshot');
    expect(CLIENT_EVENTS.mapChanged).toBe('map.changed');
    expect(CLIENT_EVENTS.mapTransferStarted).toBe('map.transfer_started');
    expect(CLIENT_EVENTS.mapTransferCompleted).toBe('map.transfer_completed');
    expect(CLIENT_EVENTS.mapTransferFailed).toBe('map.transfer_failed');
    expect(CLIENT_EVENTS.portalCooldownStarted).toBe('portal.cooldown_started');
    expect(CLIENT_EVENTS.mapPolicyUpdated).toBe('map.policy_updated');
    expect(CLIENT_EVENTS.playerProtectionUpdated).toBe('player.protection_updated');
    expect(OPERATIONS.loadoutEquipModule).toBe('loadout.equip_module');
    expect(OPERATIONS.loadoutUnequipModule).toBe('loadout.unequip_module');
    expect(OPERATIONS.stealthToggle).toBe('stealth.toggle');
    expect(OPERATIONS.shopCatalog).toBe('shop.catalog');
    expect(OPERATIONS.shopBuyProduct).toBe('shop.buy_product');

    const builder = new CommandBuilder();
    const portalEnter = builder.portalEnter('east_gate');
    expect(portalEnter.payload).toEqual({ portal_id: 'east_gate' });
    expect(Object.keys(portalEnter.payload)).toEqual(['portal_id']);
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

  test.each([
    'map_id',
    'map_key',
    'internal_map_id',
    'public_map_key',
    'worker_id',
    'map_worker_id',
    'transfer_id',
    'transfer_token',
    'destination',
    'destination_worker',
    'origin_worker',
    'destination_id',
    'destination_map_id',
    'destination_map_key',
    'destination_public_key',
    'destination_public_map_key',
    'destination_spawn_id',
    'spawn',
    'spawn_point',
    'spawn_position',
    'to_map_key',
    'to_public_map_key',
    'cooldown_ready_at_ms',
    'ready_at_ms',
    'expires_at',
  ])('rejects trusted client command field %s', (field) => {
    expect(() => assertClientSafePayload({ portal_id: 'east_gate', [field]: 'client-authored' })).toThrow(
      /trusted field/,
    );
  });

  test('rejects nested trusted portal command destination and spawn fields', () => {
    expect(() => assertClientSafePayload({ portal_id: 'east_gate', destination: { public_map_key: '2-1' } })).toThrow(
      /trusted field: destination/,
    );
    expect(() => assertClientSafePayload({ portal_id: 'east_gate', spawn: { position: { x: 1, y: 2 } } })).toThrow(
      /trusted field: spawn/,
    );
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

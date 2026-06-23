import { describe, expect, test } from 'vitest';

import { assertClientSafePayload, CommandBuilder } from './commands';
import { CLIENT_EVENTS, OPERATIONS, parseServerMessage, rejectForbiddenPayloadKeys } from './envelope';

const UNIMPLEMENTED_MUTATION_OPS = [
  'inventory.move',
  'progression.unlock_skill',
  'progression.respec_skills',
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
    'loss_percent',
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
    expect(OPERATIONS.craftingStart).toBe('crafting.start');
    expect(OPERATIONS.craftingComplete).toBe('crafting.complete');
    expect(OPERATIONS.craftingCancel).toBe('crafting.cancel');
    expect(OPERATIONS.shopCatalog).toBe('shop.catalog');
    expect(OPERATIONS.shopBuyProduct).toBe('shop.buy_product');
    expect(OPERATIONS.discoveryClaimPlanet).toBe('discovery.claim_planet');
    expect(OPERATIONS.intelShare).toBe('intel.share');
    expect(OPERATIONS.intelCoordinateItemCreate).toBe('intel.coordinate_item.create');
    expect(OPERATIONS.intelCoordinateItemUse).toBe('intel.coordinate_item.use');
    expect(CLIENT_EVENTS.planetClaimed).toBe('planet.claimed');
    expect(OPERATIONS.routeCreate).toBe('route.create');
    expect(OPERATIONS.routeUpdate).toBe('route.update');
    expect(OPERATIONS.routeEnable).toBe('route.enable');
    expect(OPERATIONS.routeDisable).toBe('route.disable');
    expect(CLIENT_EVENTS.routeUpdated).toBe('route.updated');

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
    const craftStart = builder.craftingStart({ recipeID: 'refined_alloy_batch' });
    expect(craftStart.op).toBe(OPERATIONS.craftingStart);
    expect(craftStart.payload).toEqual({ recipe_id: 'refined_alloy_batch' });
    expect(Object.keys(craftStart.payload)).toEqual(['recipe_id']);
    const craftComplete = builder.craftingComplete('craft-job-1');
    expect(craftComplete.op).toBe(OPERATIONS.craftingComplete);
    expect(craftComplete.payload).toEqual({ job_id: 'craft-job-1' });
    expect(Object.keys(craftComplete.payload)).toEqual(['job_id']);
    const craftCancel = builder.craftingCancel('craft-job-1');
    expect(craftCancel.op).toBe(OPERATIONS.craftingCancel);
    expect(craftCancel.payload).toEqual({ job_id: 'craft-job-1' });
    expect(Object.keys(craftCancel.payload)).toEqual(['job_id']);
    expect(builder.shopCatalog().payload).toEqual({});
    expect(builder.shopCatalog('weapons').payload).toEqual({ category_id: 'weapons' });
    expect(builder.shopBuyProduct('product_module_laser_alpha_t1', 1).payload).toEqual({
      product_id: 'product_module_laser_alpha_t1',
      quantity: 1,
    });
  });

  test('planet building commands send only client intent fields', () => {
    const builder = new CommandBuilder();
    const build = builder.planetBuildingBuild({ planetID: 'planet-eris', buildingType: 'alloy_foundry', slot: 'alpha' });
    expect(build.op).toBe(OPERATIONS.planetBuildingBuild);
    expect(build.payload).toEqual({
      planet_id: 'planet-eris',
      building_type: 'alloy_foundry',
      slot: 'alpha',
    });
    expect(Object.keys(build.payload)).toEqual(['planet_id', 'building_type', 'slot']);

    const upgrade = builder.planetBuildingUpgrade({ planetID: 'planet-eris', buildingID: 'building-alpha', targetLevel: 2.4 });
    expect(upgrade.op).toBe(OPERATIONS.planetBuildingUpgrade);
    expect(upgrade.payload).toEqual({
      planet_id: 'planet-eris',
      building_id: 'building-alpha',
      target_level: 2,
    });
    expect(Object.keys(upgrade.payload)).toEqual(['planet_id', 'building_id', 'target_level']);

    for (const payload of [build.payload, upgrade.payload]) {
      for (const forbidden of [
        'owner',
        'owner_player_id',
        'player_id',
        'wallet',
        'cost',
        'materials',
        'storage',
        'production',
        'coordinates',
        'position',
        'public_map_key',
        'level',
        'definition_id',
      ]) {
        expect(payload).not.toHaveProperty(forbidden);
      }
    }
  });

  test('claim planet command sends only planet id intent', () => {
    const claim = new CommandBuilder().claimPlanet('planet-eris');

    expect(claim.op).toBe(OPERATIONS.discoveryClaimPlanet);
    expect(claim.payload).toEqual({ planet_id: 'planet-eris' });
    expect(Object.keys(claim.payload)).toEqual(['planet_id']);
    for (const forbidden of [
      'player_id',
      'map_id',
      'position',
      'coordinates',
      'owner',
      'owner_player_id',
      'x_core',
      'production',
      'inventory',
      'storage',
      'claim_reference',
    ]) {
      expect(claim.payload).not.toHaveProperty(forbidden);
    }
  });

  test('intel commands send only client intent fields', () => {
    const builder = new CommandBuilder();
    const share = builder.intelShare('planet-eris', 'player-friend');
    expect(share.op).toBe(OPERATIONS.intelShare);
    expect(share.payload).toEqual({ planet_id: 'planet-eris', to_player_id: 'player-friend' });
    expect(Object.keys(share.payload)).toEqual(['planet_id', 'to_player_id']);

    const shareEntity = builder.intelShareToEntity('planet-eris', 'entity_pilot_2');
    expect(shareEntity.op).toBe(OPERATIONS.intelShare);
    expect(shareEntity.payload).toEqual({ planet_id: 'planet-eris', to_entity_id: 'entity_pilot_2' });
    expect(Object.keys(shareEntity.payload)).toEqual(['planet_id', 'to_entity_id']);

    const create = builder.intelCoordinateItemCreate('planet-eris');
    expect(create.op).toBe(OPERATIONS.intelCoordinateItemCreate);
    expect(create.payload).toEqual({ planet_id: 'planet-eris' });
    expect(Object.keys(create.payload)).toEqual(['planet_id']);

    const use = builder.intelCoordinateItemUse('coord-player-planet-request');
    expect(use.op).toBe(OPERATIONS.intelCoordinateItemUse);
    expect(use.payload).toEqual({ item_instance_id: 'coord-player-planet-request' });
    expect(Object.keys(use.payload)).toEqual(['item_instance_id']);

    for (const payload of [share.payload, create.payload, use.payload]) {
      for (const forbidden of [
        'from_player_id',
        'player_id',
        'owner_player_id',
        'world_id',
        'zone_id',
        'map_id',
        'coordinates',
        'position',
        'x',
        'y',
        'state',
        'intel_state',
        'confidence',
        'source_type',
        'source_reference',
        'last_seen_at',
        'last_verified_at',
        'created_at',
        'used_at',
        'used_by',
        'inventory',
      ]) {
        expect(payload).not.toHaveProperty(forbidden);
      }
    }
  });

  test('route settle command sends only route intent or empty reconcile intent', () => {
    expect(OPERATIONS.routeSettle).toBe('route.settle');
    expect(CLIENT_EVENTS.routeSettled).toBe('route.settled');

    const builder = new CommandBuilder();
    const singleRoute = builder.routeSettle('route-alpha');
    expect(singleRoute.op).toBe(OPERATIONS.routeSettle);
    expect(singleRoute.payload).toEqual({ route_id: 'route-alpha' });
    expect(Object.keys(singleRoute.payload)).toEqual(['route_id']);

    const reconcile = builder.routeSettle();
    expect(reconcile.op).toBe(OPERATIONS.routeSettle);
    expect(reconcile.payload).toEqual({});
    expect(Object.keys(reconcile.payload)).toEqual([]);

    for (const payload of [singleRoute.payload, reconcile.payload]) {
      for (const forbidden of [
        'owner',
        'owner_player_id',
        'player_id',
        'session_id',
        'map_id',
        'source',
        'destination',
        'enabled',
        'settled_at',
        'elapsed_applied_ms',
        'storage',
        'energy',
        'risk',
        'loss_percent',
        'wanted_amount',
        'taken_amount',
        'lost_amount',
        'delivered_amount',
        'added_amount',
        'amount',
        'rate',
        'resource_item_id',
        'cooldown',
        'position',
        'coordinates',
        'x',
        'y',
      ]) {
        expect(payload).not.toHaveProperty(forbidden);
      }
    }
  });

  test('route mutation commands send only client intent fields', () => {
    const builder = new CommandBuilder();
    const create = builder.routeCreate({
      sourcePlanetID: 'planet-source',
      destinationPlanetID: 'planet-destination',
      resourceItemID: 'refined_alloy',
      amountPerHour: 40.2,
    });
    expect(create.op).toBe(OPERATIONS.routeCreate);
    expect(create.payload).toEqual({
      source_planet_id: 'planet-source',
      destination_planet_id: 'planet-destination',
      resource_item_id: 'refined_alloy',
      amount_per_hour: 40,
    });
    expect(Object.keys(create.payload)).toEqual([
      'source_planet_id',
      'destination_planet_id',
      'resource_item_id',
      'amount_per_hour',
    ]);

    const createStorage = builder.routeCreate({
      sourcePlanetID: 'planet-source',
      destination: { type: 'storage', id: 'storage-alpha' },
      resourceItemID: 'refined_alloy',
      amountPerHour: 12.8,
    });
    expect(createStorage.payload).toEqual({
      source_planet_id: 'planet-source',
      destination_type: 'storage',
      destination_id: 'storage-alpha',
      resource_item_id: 'refined_alloy',
      amount_per_hour: 13,
    });
    expect(Object.keys(createStorage.payload)).toEqual([
      'source_planet_id',
      'destination_type',
      'destination_id',
      'resource_item_id',
      'amount_per_hour',
    ]);

    const update = builder.routeUpdate({
      routeID: 'route-alpha',
      destinationPlanetID: 'planet-new-destination',
      resourceItemID: 'raw_ore',
      amountPerHour: 75,
    });
    expect(update.op).toBe(OPERATIONS.routeUpdate);
    expect(update.payload).toEqual({
      route_id: 'route-alpha',
      destination_planet_id: 'planet-new-destination',
      resource_item_id: 'raw_ore',
      amount_per_hour: 75,
    });
    expect(Object.keys(update.payload)).toEqual([
      'route_id',
      'destination_planet_id',
      'resource_item_id',
      'amount_per_hour',
    ]);

    const updateStation = builder.routeUpdate({
      routeID: 'route-alpha',
      destination: { type: 'station', id: 'station-alpha' },
      resourceItemID: 'raw_ore',
      amountPerHour: 76.2,
    });
    expect(updateStation.payload).toEqual({
      route_id: 'route-alpha',
      destination_type: 'station',
      destination_id: 'station-alpha',
      resource_item_id: 'raw_ore',
      amount_per_hour: 76,
    });
    expect(Object.keys(updateStation.payload)).toEqual([
      'route_id',
      'destination_type',
      'destination_id',
      'resource_item_id',
      'amount_per_hour',
    ]);

    const enable = builder.routeEnable('route-alpha');
    const disable = builder.routeDisable('route-alpha');
    expect(enable.op).toBe(OPERATIONS.routeEnable);
    expect(disable.op).toBe(OPERATIONS.routeDisable);
    expect(enable.payload).toEqual({ route_id: 'route-alpha' });
    expect(disable.payload).toEqual({ route_id: 'route-alpha' });

    for (const payload of [create.payload, update.payload, enable.payload, disable.payload]) {
      for (const forbidden of [
        'owner',
        'owner_player_id',
        'player_id',
        'session_id',
        'source',
        'destination',
        'source_map_id',
        'destination_map_id',
        'map_id',
        'from_public_map_key',
        'to_public_map_key',
        'enabled',
        'settlement',
        'storage',
        'energy_cost_per_hour',
        'risk',
        'loss_chance',
        'last_calculated_at',
        'position',
        'coordinates',
      ]) {
        expect(payload).not.toHaveProperty(forbidden);
      }
    }
  });

  test('building mutation commands send only client intent fields', () => {
    expect(OPERATIONS.planetBuildingBuild).toBe('planet.building_build');
    expect(OPERATIONS.planetBuildingUpgrade).toBe('planet.building_upgrade');

    const builder = new CommandBuilder();
    const build = builder.planetBuildingBuild({
      planetID: 'planet-eris',
      buildingType: 'alloy_foundry',
      slot: 'alpha',
    });
    expect(build.op).toBe(OPERATIONS.planetBuildingBuild);
    expect(build.payload).toEqual({
      planet_id: 'planet-eris',
      building_type: 'alloy_foundry',
      slot: 'alpha',
    });
    expect(Object.keys(build.payload)).toEqual(['planet_id', 'building_type', 'slot']);

    const upgrade = builder.planetBuildingUpgrade({
      planetID: 'planet-eris',
      buildingID: 'planet-eris-building-iron_extractor-alpha',
      targetLevel: 2.4,
    });
    expect(upgrade.op).toBe(OPERATIONS.planetBuildingUpgrade);
    expect(upgrade.payload).toEqual({
      planet_id: 'planet-eris',
      building_id: 'planet-eris-building-iron_extractor-alpha',
      target_level: 2,
    });
    expect(Object.keys(upgrade.payload)).toEqual(['planet_id', 'building_id', 'target_level']);

    for (const payload of [build.payload, upgrade.payload]) {
      for (const forbidden of [
        'owner',
        'owner_player_id',
        'player_id',
        'session_id',
        'map_id',
        'public_map_key',
        'production',
        'storage',
        'wallet',
        'cost',
        'materials',
        'reference',
        'reference_key',
        'created_at',
        'updated_at',
      ]) {
        expect(payload).not.toHaveProperty(forbidden);
      }
    }
  });

  test('crafting mutation commands send only client intent fields', () => {
    expect(OPERATIONS.craftingStart).toBe('crafting.start');
    expect(OPERATIONS.craftingComplete).toBe('crafting.complete');
    expect(OPERATIONS.craftingCancel).toBe('crafting.cancel');

    const builder = new CommandBuilder();
    const start = builder.craftingStart('refined_alloy_batch');
    expect(start.op).toBe(OPERATIONS.craftingStart);
    expect(start.payload).toEqual({ recipe_id: 'refined_alloy_batch' });
    expect(Object.keys(start.payload)).toEqual(['recipe_id']);

    const startAtStation = builder.craftingStart({ recipeID: 'refined_alloy_batch', locationType: 'station' });
    expect(startAtStation.op).toBe(OPERATIONS.craftingStart);
    expect(startAtStation.payload).toEqual({ recipe_id: 'refined_alloy_batch', location_type: 'station' });
    expect(Object.keys(startAtStation.payload)).toEqual(['recipe_id', 'location_type']);

    const complete = builder.craftingComplete('craft-job-1');
    expect(complete.op).toBe(OPERATIONS.craftingComplete);
    expect(complete.payload).toEqual({ job_id: 'craft-job-1' });
    expect(Object.keys(complete.payload)).toEqual(['job_id']);

    const cancel = builder.craftingCancel('craft-job-1');
    expect(cancel.op).toBe(OPERATIONS.craftingCancel);
    expect(cancel.payload).toEqual({ job_id: 'craft-job-1' });
    expect(Object.keys(cancel.payload)).toEqual(['job_id']);

    for (const payload of [start.payload, startAtStation.payload, complete.payload, cancel.payload]) {
      for (const forbidden of [
        'owner',
        'owner_player_id',
        'player_id',
        'session_id',
        'location',
        'location_id',
        'materials',
        'inputs',
        'output',
        'wallet',
        'wallet_amount',
        'required_credits',
        'cost',
        'reference',
        'reference_key',
        'reservation_id',
        'started_at',
        'completes_at',
        'completed_at',
      ]) {
        expect(payload).not.toHaveProperty(forbidden);
      }
    }
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
      'inventoryMove',
      'progressionUnlockSkill',
      'progressionRespecSkills',
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

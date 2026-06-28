import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event, expectServerOwnedGameplayCleared, stateWithServerOwnedGameplay } from './reducer-fixtures.test-support';
import { isWithinMinimapProjectionWindow, worldMapMemoryMarkers } from './world-memory';

describe('reduceClientState', () => {
  test('snapshot response reconciles player, cargo, wallet, ship, progression, inventory, hangar, loadout, crafting, and stat panels', () => {
    const reconciled = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'snapshot-panels',
        ok: true,
        payload: {
          player: { callsign: 'Server-Pilot', hp: 77, shield: 44, energy: 33, rank: 2 },
          cargo: {
            used: 4,
            capacity: 80,
            items: [{
              item_id: 'raw_ore',
              display_name: 'Raw Ore',
              category: 'resource',
              art_key: 'item.raw_ore',
              rarity: 'common',
              quantity: 4,
              unit_weight: 2,
              used_units: 8,
              location: 'ship_cargo',
              move_eligible: false,
              locked_reason: 'cargo_transfer_unavailable',
            }],
          },
          wallet: { credits: 980, premium_paid: 3, premium_earned: 9 },
          ship: {
            active_ship_id: 'starter',
            display_name: 'Starter Hull',
            hull: 88,
            max_hull: 120,
            shield: 42,
            max_shield: 60,
            capacitor: 31,
            max_capacitor: 50,
            disabled: false,
            repair_state: 'active',
          },
          progression: { main_level: 2, main_xp: 175, rank: 2, combat_level: 1, combat_xp: 25 },
          inventory: {
            stackable: [{ item_id: 'raw_ore', display_name: 'Raw Ore', quantity: 3, location: 'ship_cargo' }],
            instances: [],
            counts: { cargo_stacks: 1, storage_stacks: 0, equipped_instances: 0 },
          },
          hangar: {
            active_ship_id: 'starter',
            ships: [
              {
                ship_id: 'starter',
                display_name: 'Phoenix',
                state: 'ready',
                hull: 88,
                max_hull: 120,
                shield: 42,
                max_shield: 60,
                disabled: false,
              },
            ],
          },
          loadout: {
            active_ship_id: 'starter',
            slots: [
              { slot_id: 'offensive_1', slot_type: 'offensive' },
              { slot_id: 'defensive_1', slot_type: 'defensive' },
            ],
          },
          crafting: {
            recipes: [
              {
                recipe_id: 'refined_alloy_batch',
                category: 'processed_material',
                output: { kind: 'item', item_id: 'refined_alloy', quantity: 5, tradeable: true },
                inputs: [{ item_id: 'raw_ore', quantity: 20 }],
                required_credits: 100,
                required_rank: 1,
                required_role_levels: [{ role: 'crafting', level: 1 }],
                required_location_type: 'station',
                craft_duration_ms: 300000,
                repeatable: true,
              },
            ],
            active_jobs: [],
          },
          stats: {
            speed: 220,
            radar_range: 510,
            weapon_range: 280,
            cargo_capacity: 80,
            loot_pickup_range: 120,
            basic_laser_energy_cost: 10,
            basic_laser_cooldown_ms: 350,
          },
        },
        server_time: 1400,
        v: 1,
      },
    });

    expect(reconciled.playerSnapshot?.callsign).toBe('Server-Pilot');
    expect(reconciled.cargo).toMatchObject({ used: 4, capacity: 80 });
    expect(reconciled.cargo?.items).toEqual([{
      item_id: 'raw_ore',
      display_name: 'Raw Ore',
      category: 'resource',
      art_key: 'item.raw_ore',
      rarity: 'common',
      quantity: 4,
      unit_weight: 2,
      used_units: 8,
      location: 'ship_cargo',
      move_eligible: false,
      locked_reason: 'cargo_transfer_unavailable',
    }]);
    expect(reconciled.wallet).toEqual({ credits: 980, premium_paid: 3, premium_earned: 9 });
    expect(reconciled.ship).toMatchObject({ active_ship_id: 'starter', hull: 88, capacitor: 31, disabled: false });
    expect(reconciled.playerSnapshot).toMatchObject({ hp: 88, max_hp: 120, shield: 42, energy: 31 });
    expect(reconciled.progression).toMatchObject({ main_level: 2, main_xp: 175, rank: 2, combat_xp: 25 });
    expect(reconciled.inventory?.stackable).toEqual([
      { item_id: 'raw_ore', display_name: 'Raw Ore', quantity: 3, location: 'ship_cargo' },
    ]);
    expect(reconciled.hangar?.active_ship_id).toBe('starter');
    expect(reconciled.loadout?.slots).toHaveLength(2);
    expect(reconciled.crafting?.recipes[0]).toMatchObject({ recipe_id: 'refined_alloy_batch', craft_duration_ms: 300000 });
    expect(reconciled.stats).toMatchObject({
      speed: 220,
      radar_range: 510,
      weapon_range: 280,
      cargo_capacity: 80,
      loot_pickup_range: 120,
      basic_laser_energy_cost: 10,
      basic_laser_cooldown_ms: 350,
    });
  });

  test('snapshot events reconcile cargo, wallet, stats, inventory, hangar, loadout, and crafting independently', () => {
    const withCargo = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.cargoSnapshot, {
        used: 12,
        capacity: 70,
        items: [{
          item_id: 'salvage_thread',
          display_name: 'Salvage Thread',
          category: 'resource',
          art_key: 'item.salvage_thread',
          quantity: 12,
          unit_weight: 1,
          used_units: 12,
          location: 'ship_cargo',
          move_eligible: false,
          locked_reason: 'cargo_transfer_unavailable',
        }],
      }),
    });
    const withWallet = reduceClientState(withCargo, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.walletSnapshot, { credits: 444, premium_paid: 1, premium_earned: 2 }, 2),
    });
    const withStats = reduceClientState(withWallet, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.statsSnapshot, { speed: 210, radar_range: 500, weapon_range: 275, cargo_capacity: 70 }, 3),
    });
    const withInventory = reduceClientState(withStats, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.inventorySnapshot, {
        stackable: [{ item_id: 'raw_ore', quantity: 3, location: 'ship_cargo' }],
        instances: [],
        counts: { cargo_stacks: 1, storage_stacks: 0, equipped_instances: 0 },
      }, 4),
    });
    const withHangar = reduceClientState(withInventory, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.hangarSnapshot, {
        active_ship_id: 'starter',
        ships: [{ ship_id: 'starter', display_name: 'Phoenix', state: 'ready', hull: 100, max_hull: 100, shield: 100, max_shield: 100, disabled: false }],
      }, 5),
    });
    const withLoadout = reduceClientState(withHangar, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.loadoutSnapshot, {
        active_ship_id: 'starter',
        slots: [{ slot_id: 'offensive_1', slot_type: 'offensive' }],
      }, 6),
    });
    const withCrafting = reduceClientState(withLoadout, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.craftingRecipes, {
        recipes: [
          {
            recipe_id: 'refined_alloy_batch',
            category: 'processed_material',
            output: { kind: 'item', item_id: 'refined_alloy', quantity: 5, tradeable: true },
            inputs: [{ item_id: 'raw_ore', quantity: 20 }],
            required_credits: 100,
            required_rank: 1,
            required_role_levels: [{ role: 'crafting', level: 1 }],
            required_location_type: 'station',
            craft_duration_ms: 300000,
            repeatable: true,
          },
        ],
        active_jobs: [],
      }, 7),
    });

    expect(withCrafting.cargo?.items).toEqual([{
      item_id: 'salvage_thread',
      display_name: 'Salvage Thread',
      category: 'resource',
      art_key: 'item.salvage_thread',
      quantity: 12,
      unit_weight: 1,
      used_units: 12,
      location: 'ship_cargo',
      move_eligible: false,
      locked_reason: 'cargo_transfer_unavailable',
    }]);
    expect(withCrafting.wallet?.credits).toBe(444);
    expect(withCrafting.stats?.weapon_range).toBe(275);
    expect(withCrafting.inventory?.counts.cargo_stacks).toBe(1);
    expect(withCrafting.hangar?.ships[0].display_name).toBe('Phoenix');
    expect(withCrafting.loadout?.slots[0].slot_type).toBe('offensive');
    expect(withCrafting.crafting?.recipes[0].recipe_id).toBe('refined_alloy_batch');
  });

  test('inventory stack snapshots preserve server sell eligibility and fail closed when missing', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.inventorySnapshot, {
        stackable: [
          {
            item_id: 'iron_ore',
            display_name: 'Iron Ore',
            quantity: 4,
            location: 'account_inventory',
            list_eligible: true,
          },
          {
            item_id: 'bound_relic',
            display_name: 'Bound Relic',
            quantity: 1,
            location: 'account_inventory',
            list_eligible: false,
            locked_reason: 'Bound cargo.',
          },
          {
            item_id: 'legacy_payload',
            display_name: 'Legacy Payload',
            quantity: 2,
            location: 'ship_cargo',
          },
        ],
        instances: [],
        counts: { cargo_stacks: 1, storage_stacks: 0, equipped_instances: 0 },
      }),
    });

    expect(state.inventory?.stackable).toEqual([
      {
        item_id: 'iron_ore',
        display_name: 'Iron Ore',
        quantity: 4,
        location: 'account_inventory',
        list_eligible: true,
      },
      {
        item_id: 'bound_relic',
        display_name: 'Bound Relic',
        quantity: 1,
        location: 'account_inventory',
        list_eligible: false,
        locked_reason: 'Bound cargo.',
      },
      {
        item_id: 'legacy_payload',
        display_name: 'Legacy Payload',
        quantity: 2,
        location: 'ship_cargo',
      },
    ]);
    expect(state.inventory?.stackable.map((item) => item.list_eligible)).toEqual([true, false, undefined]);
  });

  test('inventory snapshots clear consumed coordinate scroll pending only from authoritative instance lists', () => {
    const pending = {
      ...createInitialState(),
      pendingCommands: {
        'coordinate-use-1': {
          requestID: 'coordinate-use-1',
          op: OPERATIONS.intelCoordinateItemUse,
          queuedAt: 1,
          payload: { item_instance_id: 'coord-scroll-1' },
        },
      },
      inventory: {
        stackable: [],
        instances: [
          {
            item_instance_id: 'coord-scroll-1',
            item_id: 'planet_coordinate_scroll',
            display_name: 'Planet Coordinate Scroll',
            location: 'account_inventory',
          },
        ],
        counts: { cargo_stacks: 0, storage_stacks: 0, equipped_instances: 0 },
      },
    };

    const partial = reduceClientState(pending, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.inventorySnapshot, {
        stackable: [],
        counts: { cargo_stacks: 0, storage_stacks: 0, equipped_instances: 0 },
      }),
    });
    expect(partial.pendingCommands['coordinate-use-1']).toMatchObject({ op: OPERATIONS.intelCoordinateItemUse });

    const consumed = reduceClientState(pending, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.inventorySnapshot, {
        stackable: [],
        instances: [],
        counts: { cargo_stacks: 0, storage_stacks: 0, equipped_instances: 0 },
      }, 2),
    });
    expect(consumed.pendingCommands['coordinate-use-1']).toBeUndefined();

    const responsePartial = reduceClientState(pending, {
      type: 'responseReceived',
      envelope: {
        request_id: 'inventory-refresh',
        ok: true,
        payload: {
          inventory: {
            stackable: [],
            counts: { cargo_stacks: 0, storage_stacks: 0, equipped_instances: 0 },
          },
        },
        server_time: 3,
        v: 1,
      },
    });
    expect(responsePartial.pendingCommands['coordinate-use-1']).toMatchObject({ op: OPERATIONS.intelCoordinateItemUse });

    const responseConsumed = reduceClientState(pending, {
      type: 'responseReceived',
      envelope: {
        request_id: 'inventory-refresh',
        ok: true,
        payload: {
          inventory: {
            stackable: [],
            instances: [],
            counts: { cargo_stacks: 0, storage_stacks: 0, equipped_instances: 0 },
          },
        },
        server_time: 4,
        v: 1,
      },
    });
    expect(responseConsumed.pendingCommands['coordinate-use-1']).toBeUndefined();
  });

  test('shop catalog response stores server-owned categories and products', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'request-shop-catalog',
        ok: true,
        payload: {
          shop: {
            catalog_version: 'content_registry_task001_v1',
            categories: [
              { category_id: 'ships', display_name: 'Ships', sort_order: 10 },
              { category_id: 'weapons', display_name: 'Weapons', sort_order: 20 },
            ],
            products: [
              {
                product_id: 'product_module_laser_alpha_t1',
                product_type: 'module',
                display_name: 'LF-1',
                description: 'Entry laser array.',
                category_id: 'weapons',
                subcategory: 'Laser',
                art_key: 'module.lf_1',
                rarity: 'common',
                tier: 1,
                sort_order: 20,
                grant_target: { kind: 'module', ref_id: 'laser_alpha_t1', quantity: 1 },
                price: { currency_type: 'credits', amount: 450, fixed: true },
                stock: { kind: 'unlimited' },
                availability: { available: false, locked_reason: 'Module purchase unavailable in this playtest.' },
              },
            ],
          },
        },
        server_time: 99,
        v: 1,
      },
    });

    expect(state.shopCatalog?.catalog_version).toBe('content_registry_task001_v1');
    expect(state.shopCatalog?.categories.map((category) => category.display_name)).toEqual(['Ships', 'Weapons']);
    expect(state.shopCatalog?.products[0]).toMatchObject({
      product_id: 'product_module_laser_alpha_t1',
      display_name: 'LF-1',
      category_id: 'weapons',
      price: { amount: 450, currency_type: 'credits' },
      availability: { available: false },
    });
  });

  test('shop buy response clears pending command and applies server snapshots', () => {
    const queued = reduceClientState(createInitialState(), {
      type: 'requestQueued',
      envelope: {
        request_id: 'request-shop-buy',
        op: OPERATIONS.shopBuyProduct,
        payload: { product_id: 'product_module_laser_alpha_t1', quantity: 1 },
        client_seq: 1,
        v: 1,
      },
    });

    expect(queued.pendingCommands['request-shop-buy']?.op).toBe(OPERATIONS.shopBuyProduct);

    const state = reduceClientState(queued, {
      type: 'responseReceived',
      envelope: {
        request_id: 'request-shop-buy',
        ok: true,
        payload: {
          accepted: true,
          product: {
            product_id: 'product_module_laser_alpha_t1',
            display_name: 'LF-1',
          },
          quantity: 1,
          server_total: 450,
          wallet: { credits: 750, premium_paid: 300, premium_earned: 0 },
          inventory: {
            stackable: [],
            instances: [
              {
                item_instance_id: 'module-shop-1',
                item_id: 'laser_alpha_t1',
                display_name: 'LF-1',
                location: 'account_inventory',
                item_type: 'module',
                module_slot_type: 'offensive',
                durability_current: 100,
                durability_max: 100,
              },
            ],
            counts: { cargo_stacks: 0, storage_stacks: 0, equipped_instances: 1 },
          },
          shop: {
            catalog_version: 'content_registry_task001_v1',
            categories: [{ category_id: 'weapons', display_name: 'Weapons', sort_order: 20 }],
            products: [
              {
                product_id: 'product_module_laser_alpha_t1',
                product_type: 'module',
                display_name: 'LF-1',
                description: 'Entry laser array.',
                category_id: 'weapons',
                subcategory: 'Laser',
                art_key: 'module.lf_1',
                rarity: 'common',
                tier: 1,
                sort_order: 20,
                grant_target: { kind: 'module', ref_id: 'laser_alpha_t1', quantity: 1 },
                price: { currency_type: 'credits', amount: 450, fixed: true },
                stock: { kind: 'unlimited' },
                availability: { available: true },
              },
            ],
          },
        },
        server_time: 101,
        v: 1,
      },
    });

    expect(state.pendingCommands['request-shop-buy']).toBeUndefined();
    expect(state.wallet?.credits).toBe(750);
    expect(state.inventory?.instances).toContainEqual(
      expect.objectContaining({
        item_instance_id: 'module-shop-1',
        item_id: 'laser_alpha_t1',
        location: 'account_inventory',
      }),
    );
    expect(state.shopCatalog?.products[0]).toMatchObject({
      product_id: 'product_module_laser_alpha_t1',
      availability: { available: true },
    });
  });
});

import { describe, expect, test } from 'vitest';

import { OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';

describe('player content catalog reducer', () => {
  test('stores player-safe content catalog metadata separate from admin content', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'request-content-catalog',
        ok: true,
        payload: {
          content_catalog: {
            version: 'content_projection_v1',
            categories: [{ category_id: 'weapons', display_name: 'Weapons', sort_order: 20 }],
            items: [
              {
                item_id: 'raw_ore',
                display: {
                  display_name: 'Raw Ore',
                  category: 'materials',
                  art_key: 'item.raw_ore',
                  rarity: 'common',
                  sort_order: 10,
                },
                item_type: 'stackable',
                rarity: 'common',
                max_stack: 100,
                weight_units: 1,
                trade_flags: ['tradeable'],
                bind_rules: ['none'],
              },
            ],
            modules: [
              {
                item_id: 'laser_alpha_t1',
                display: {
                  display_name: 'Prism Lance I',
                  category: 'weapons',
                  subcategory: 'Laser',
                  art_key: 'module.prism_lance_1',
                  rarity: 'common',
                  tier: 1,
                },
                name: 'Laser Gun Alpha I',
                module_category: 'offensive',
                slot_type: 'offensive',
                tier: 1,
                rarity: 'common',
                required_rank: 1,
                required_role_levels: [{ role: 'combat', level: 1 }],
                stat_modifiers: [{ stat: 'weapon_damage', kind: 'flat', value: 12 }],
                energy: { activation_cost: 8 },
                cooldowns: [{ key: 'basic_attack', duration_ms: 1200 }],
                durability_max: 100,
                trade_flags: ['tradeable'],
                bind_rules: ['bind_on_equip'],
                compatible_slot_types: ['offensive'],
                compatible_categories: ['offensive'],
              },
            ],
            shop_products: [
              {
                product_id: 'product_module_laser_alpha_t1',
                product_type: 'module',
                display: {
                  display_name: 'Prism Lance I',
                  description: 'Entry laser array.',
                  category: 'weapons',
                  subcategory: 'Laser',
                  art_key: 'module.prism_lance_1',
                  rarity: 'common',
                  tier: 1,
                  sort_order: 20,
                },
                grant_target: { kind: 'module', ref_id: 'laser_alpha_t1', quantity: 1 },
                price_policy: { currency_type: 'credits', amount: 450, fixed: true },
                stock_policy: { kind: 'limited', remaining: 3, total: 5 },
                availability: { available: true, required_rank: 1 },
              },
            ],
          },
        },
        server_time: 99,
        v: 1,
      },
    });

    expect(state.contentCatalog?.version).toBe('content_projection_v1');
    expect(state.contentCatalog?.categories[0]).toMatchObject({ category_id: 'weapons', display_name: 'Weapons' });
    expect(state.contentCatalog?.items[0]).toMatchObject({
      item_id: 'raw_ore',
      display: { display_name: 'Raw Ore', art_key: 'item.raw_ore' },
      max_stack: 100,
    });
    expect(state.contentCatalog?.modules[0]).toMatchObject({
      item_id: 'laser_alpha_t1',
      display: { display_name: 'Prism Lance I' },
      stat_modifiers: [{ stat: 'weapon_damage', kind: 'flat', value: 12 }],
      cooldowns: [{ key: 'basic_attack', duration_ms: 1200 }],
    });
    expect(state.contentCatalog?.shop_products[0]).toMatchObject({
      product_id: 'product_module_laser_alpha_t1',
      price_policy: { amount: 450, currency_type: 'credits' },
      stock_policy: { remaining: 3, total: 5 },
    });
    expect(state.adminContent).toBeNull();
  });

  test('rejects hidden keys and sentinels in player catalog payloads', () => {
    expect(() =>
      reduceClientState(createInitialState(), {
        type: 'responseReceived',
        envelope: {
          request_id: 'request-content-catalog-hidden',
          ok: true,
          payload: {
            content_catalog: {
              version: 'content_projection_v1',
              items: [
                {
                  item_id: 'raw_ore',
                  display: { display_name: 'Raw Ore' },
                  hidden_note: 'HIDDEN_PROJECTION_SENTINEL',
                },
              ],
            },
          },
          server_time: 1,
          v: 1,
        },
      }),
    ).toThrow(/Forbidden player content catalog payload rejected/);
  });

  test('fully replaces catalog arrays when a new response omits collections', () => {
    const state = reduceClientState(
      {
        ...createInitialState(),
        contentCatalog: {
          version: 'content_projection_v1',
          categories: [{ category_id: 'weapons', display_name: 'Weapons', sort_order: 1 }],
          items: [
            {
              item_id: 'raw_ore',
              display: { display_name: 'Raw Ore' },
              trade_flags: ['tradeable'],
              bind_rules: ['none'],
            },
          ],
          modules: [
            {
              item_id: 'laser_alpha_t1',
              display: { display_name: 'Prism Lance I' },
              required_role_levels: [],
              stat_modifiers: [],
              energy: {},
              cooldowns: [],
              trade_flags: ['tradeable'],
              bind_rules: ['bind_on_equip'],
              compatible_slot_types: ['offensive'],
              compatible_categories: ['offensive'],
            },
          ],
          shop_products: [
            {
              product_id: 'product_module_laser_alpha_t1',
              display: { display_name: 'Prism Lance I' },
              grant_target: {},
              price_policy: {},
              stock_policy: {},
              availability: { available: true },
            },
          ],
        },
        pendingCommands: {
          catalog: { requestID: 'catalog', op: OPERATIONS.contentCatalog, queuedAt: 1, payload: {} },
        },
      },
      {
        type: 'responseReceived',
        envelope: {
          request_id: 'catalog',
          ok: true,
          payload: {
            content_catalog: {
              version: 'content_projection_v2',
            },
          },
          server_time: 2,
          v: 1,
        },
      },
    );

    expect(state.contentCatalog).toMatchObject({
      version: 'content_projection_v2',
      categories: [],
      items: [],
      modules: [],
      shop_products: [],
    });
  });

  test('does not populate player content catalog from crafting recipes payload', () => {
    const state = reduceClientState(
      {
        ...createInitialState(),
        pendingCommands: {
          recipes: { requestID: 'recipes', op: OPERATIONS.craftingRecipes, queuedAt: 1, payload: {} },
        },
      },
      {
        type: 'responseReceived',
        envelope: {
          request_id: 'recipes',
          ok: true,
          payload: {
            crafting_recipes: {
              recipes: [
                {
                  recipe_id: 'laser_alpha_t1_recipe',
                  category: 'module',
                  output: { kind: 'module', item_id: 'laser_alpha_t1', quantity: 1, tradeable: true },
                  inputs: [{ item_id: 'raw_ore', quantity: 4 }],
                  required_rank: 1,
                },
              ],
              version: 'content_projection_v1',
              categories: [{ category_id: 'weapons', display_name: 'Weapons' }],
              modules: [{ item_id: 'laser_alpha_t1', display: { display_name: 'Prism Lance I' } }],
              shop_products: [{ product_id: 'product_module_laser_alpha_t1' }],
            },
          },
          server_time: 2,
          v: 1,
        },
      },
    );

    expect(state.crafting?.recipes[0]?.recipe_id).toBe('laser_alpha_t1_recipe');
    expect(state.contentCatalog).toBeNull();
  });
});

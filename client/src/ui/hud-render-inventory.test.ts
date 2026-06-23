import { beforeEach, describe, expect, test } from 'vitest';

import { OPERATIONS } from '../protocol/envelope';
import { createInitialState } from '../state/reducer';
import type { ClientState } from '../state/types';
import { hudSelection } from './hud-selection';
import { cargoPanel } from './hud-render-inventory';

describe('cargoPanel crafting tab', () => {
  beforeEach(() => {
    hudSelection.selectedInventoryTab = 'crafting';
  });

  test('renders server-owned recipes with start intent controls', () => {
    const state = craftingState();
    const html = cargoPanel(state);

    expect(html).toContain('data-crafting-tab="true"');
    expect(html).toContain('data-action="crafting-start"');
    expect(html).toContain('data-recipe-id="refined_alloy_batch"');
    expect(html).toMatch(/data-action="crafting-start"[^>]*>Start/);
    expect(html).not.toMatch(/data-action="crafting-start"[^>]*disabled[^>]*>Start/);
  });

  test('renders pending start and ready completion states', () => {
    const state = craftingState({
      pendingCommands: {
        'request-craft-start': {
          requestID: 'request-craft-start',
          op: OPERATIONS.craftingStart,
          payload: { recipe_id: 'refined_alloy_batch' },
          queuedAt: 1000,
        },
      },
      crafting: {
        recipes: [craftingRecipe()],
        active_jobs: [
          {
            job_id: 'craft-job-ready',
            recipe_id: 'refined_alloy_batch',
            state: 'running',
            started_at: 1000,
            completes_at: 1500,
          },
        ],
      },
      lastServerTime: 2000,
    });
    const html = cargoPanel(state);

    expect(html).toMatch(/data-action="crafting-start"[^>]*disabled[^>]*>Starting/);
    expect(html).toMatch(/data-action="crafting-complete"[^>]*data-job-id="craft-job-ready"[^>]*>Complete/);
    expect(html).not.toMatch(/data-action="crafting-complete"[^>]*disabled[^>]*>Complete/);
  });
});

function craftingState(patch: Partial<ClientState> = {}): ClientState {
  return {
    ...createInitialState(),
    connectionStatus: 'connected',
    lastServerTime: 1000,
    cargo: { used: 0, capacity: 60, items: [] },
    wallet: { credits: 500, premium_paid: 0, premium_earned: 0 },
    inventory: {
      stackable: [
        { item_id: 'iron_ore', display_name: 'Iron Ore', quantity: 25, location: 'account_inventory' },
        { item_id: 'carbon_shards', display_name: 'Carbon Shards', quantity: 5, location: 'account_inventory' },
      ],
      instances: [],
      counts: { cargo_stacks: 0, storage_stacks: 0, equipped_instances: 0 },
    },
    hangar: {
      active_ship_id: 'starter',
      ships: [
        {
          ship_id: 'starter',
          display_name: 'Starter',
          state: 'active',
          hull: 100,
          max_hull: 100,
          shield: 50,
          max_shield: 50,
          disabled: false,
          active: true,
        },
      ],
    },
    loadout: { active_ship_id: 'starter', slots: [] },
    crafting: {
      recipes: [craftingRecipe()],
      active_jobs: [],
    },
    ...patch,
  };
}

function craftingRecipe(): NonNullable<ClientState['crafting']>['recipes'][number] {
  return {
    recipe_id: 'refined_alloy_batch',
    category: 'processed_material',
    output: { kind: 'item', item_id: 'refined_alloy', quantity: 5, tradeable: true },
    inputs: [
      { item_id: 'iron_ore', quantity: 20 },
      { item_id: 'carbon_shards', quantity: 5 },
    ],
    required_credits: 100,
    required_rank: 1,
    required_role_levels: [{ role: 'crafting', level: 1 }],
    required_location_type: 'station',
    craft_duration_ms: 300000,
    repeatable: true,
  };
}

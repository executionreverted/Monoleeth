import { beforeEach, describe, expect, test } from 'vitest';

import { OPERATIONS } from '../protocol/envelope';
import { createInitialState } from '../state/reducer';
import type { ClientState } from '../state/types';
import { cargoPanel } from './hud-render-inventory';
import { hudSelection } from './hud-selection';

describe('cargoPanel crafting tab', () => {
  beforeEach(() => {
    hudSelection.selectedInventoryTab = 'crafting';
  });

  test('renders server crafting recipes and active jobs with minimal command payload ids', () => {
    const state = craftingState();
    const html = cargoPanel(state, 2_000);

    expect(html).toContain('data-crafting-state="ready"');
    expect(html).toContain('refined_alloy x2');
    expect(html).toContain('raw_ore x20');
    expect(html).toContain('Ready');
    expect(html).toContain('data-action="crafting-start"');
    expect(html).toContain('data-recipe-id="refined_alloy_batch"');
    expect(html).toContain('data-action="crafting-complete"');
    expect(html).toContain('data-job-id="craft-job-1"');
    expect(html).not.toContain('data-required-credits');
    expect(html).not.toContain('data-inputs');
    expect(html).not.toContain('data-wallet');
    expect(html).not.toContain('data-owner');
  });

  test('locks crafting controls while matching commands are pending or jobs are still running', () => {
    const state = craftingState();
    state.pendingCommands = {
      'craft-start-1': {
        requestID: 'craft-start-1',
        op: OPERATIONS.craftingStart,
        payload: { recipe_id: 'refined_alloy_batch' },
        queuedAt: 1,
      },
      'craft-complete-1': {
        requestID: 'craft-complete-1',
        op: OPERATIONS.craftingComplete,
        payload: { job_id: 'craft-job-1' },
        queuedAt: 1,
      },
    };
    state.crafting!.active_jobs[0] = {
      ...state.crafting!.active_jobs[0],
      completes_at: 31_000,
    };

    const html = cargoPanel(state, 1_000);

    expect(html).toContain('30s remaining');
    expect(buttonHTML(html, 'crafting-start')).toContain('disabled');
    expect(buttonHTML(html, 'crafting-start')).toContain('Pending');
    expect(buttonHTML(html, 'crafting-complete')).toContain('disabled');
    expect(buttonHTML(html, 'crafting-complete')).toContain('Pending');
  });

  test('renders loading copy instead of fake crafting data when server crafting summary is absent', () => {
    const state = craftingState();
    state.crafting = null;

    const html = cargoPanel(state, 2_000);

    expect(html).toContain('data-crafting-state="loading"');
    expect(html).toContain('Awaiting crafting recipes from server.');
    expect(html).not.toContain('data-action="crafting-start"');
    expect(html).not.toContain('data-action="crafting-complete"');
  });

  test('renders owned coordinate scroll item use intent without hidden coordinate payload', () => {
    hudSelection.selectedInventoryTab = 'inventory';
    const state = craftingState();
    state.inventory!.instances.push({
      item_instance_id: 'coord-scroll-1',
      item_id: 'planet_coordinate_scroll',
      display_name: 'Planet Coordinate Scroll',
      location: 'account_inventory',
    });

    const html = cargoPanel(state, 2_000);

    expect(html).toContain('data-coordinate-scrolls="true"');
    expect(html).toContain('Planet Coordinate Scroll');
    expect(html).toContain('data-action="coordinate-item-use"');
    expect(html).toContain('data-item-instance-id="coord-scroll-1"');
    expect(html).not.toContain('data-planet-id');
    expect(html).not.toContain('data-coordinates');

    state.pendingCommands = {
      'coordinate-use-1': {
        requestID: 'coordinate-use-1',
        op: OPERATIONS.intelCoordinateItemUse,
        payload: { item_instance_id: 'coord-scroll-1' },
        queuedAt: 1,
      },
    };

    const pendingHTML = cargoPanel(state, 2_000);
    expect(buttonHTML(pendingHTML, 'coordinate-item-use')).toContain('disabled');
    expect(buttonHTML(pendingHTML, 'coordinate-item-use')).toContain('Using');
  });
});

function craftingState(): ClientState {
  const state = createInitialState();
  state.connectionStatus = 'connected';
  state.lastServerTime = 2_000;
  state.inventory = {
    stackable: [],
    instances: [],
    counts: { cargo_stacks: 0, storage_stacks: 0, equipped_instances: 0 },
  };
  state.hangar = {
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
      },
    ],
  };
  state.loadout = { active_ship_id: 'starter', slots: [] };
  state.cargo = { used: 0, capacity: 100, items: [] };
  state.wallet = { credits: 1_000, premium_paid: 0, premium_earned: 0 };
  state.crafting = {
    recipes: [
      {
        recipe_id: 'refined_alloy_batch',
        category: 'resource',
        output: {
          kind: 'item',
          item_id: 'refined_alloy',
          quantity: 2,
          tradeable: true,
        },
        inputs: [{ item_id: 'raw_ore', quantity: 20 }],
        required_credits: 100,
        required_rank: 1,
        required_role_levels: [{ role: 'crafting', level: 1 }],
        required_location_type: 'planet_workshop',
        craft_duration_ms: 30_000,
        repeatable: true,
      },
    ],
    active_jobs: [
      {
        job_id: 'craft-job-1',
        recipe_id: 'refined_alloy_batch',
        state: 'running',
        started_at: 1_000,
        completes_at: 1_500,
      },
    ],
  };
  return state;
}

function buttonHTML(html: string, action: string): string {
  const match = html.match(new RegExp(`<button[^>]+data-action="${action}"[\\s\\S]*?</button>`));
  return match?.[0] ?? '';
}

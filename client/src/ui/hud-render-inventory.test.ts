import { beforeEach, describe, expect, test } from 'vitest';

import { OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from '../state/reducer';
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
    expect(html).toContain('data-action="crafting-cancel"');
    expect(html).toContain('data-job-id="craft-job-1"');
    expect(html).not.toContain('data-required-credits');
    expect(html).not.toContain('data-inputs');
    expect(html).not.toContain('data-wallet');
    expect(html).not.toContain('data-owner');
    expect(html).not.toContain('data-location-id');
  });

  test('adds explicit station crafting location intent without inventing a location id', () => {
    const state = craftingState();
    state.crafting!.recipes[0] = {
      ...state.crafting!.recipes[0],
      required_location_type: 'station',
    };

    const html = cargoPanel(state, 2_000);

    expect(buttonHTML(html, 'crafting-start')).toContain('data-location-type="station"');
    expect(buttonHTML(html, 'crafting-start')).not.toContain('data-location-id');
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
      'craft-cancel-1': {
        requestID: 'craft-cancel-1',
        op: OPERATIONS.craftingCancel,
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
    expect(buttonHTML(html, 'crafting-cancel')).toContain('disabled');
    expect(buttonHTML(html, 'crafting-cancel')).toContain('Pending');
  });

  test('renders loading copy instead of fake crafting data when server crafting summary is absent', () => {
    const state = craftingState();
    state.crafting = null;

    const html = cargoPanel(state, 2_000);

    expect(html).toContain('data-crafting-state="loading"');
    expect(html).toContain('Awaiting crafting recipes from server.');
    expect(html).not.toContain('data-action="crafting-start"');
    expect(html).not.toContain('data-action="crafting-complete"');
    expect(html).not.toContain('data-action="crafting-cancel"');
  });

  test('uses reconnect snapshot server time to unlock completed crafting jobs', () => {
    const state = craftingState();
    state.lastServerTime = 2_000;
    state.crafting!.active_jobs[0] = {
      ...state.crafting!.active_jobs[0],
      completes_at: 5_000,
    };

    const runningHTML = cargoPanel(state);
    expect(runningHTML).toContain('3.0s remaining');
    expect(buttonHTML(runningHTML, 'crafting-complete')).toContain('disabled');

    const reconnected = reduceClientState(state, {
      type: 'responseReceived',
      envelope: {
        request_id: 'crafting-reconnect',
        ok: true,
        payload: {
          crafting: {
            recipes: state.crafting!.recipes,
            active_jobs: state.crafting!.active_jobs,
          },
        },
        server_time: 6_000,
        v: 1,
      },
    });

    const readyHTML = cargoPanel(reconnected);
    expect(reconnected.lastServerTime).toBe(6_000);
    expect(readyHTML).toContain('Ready');
    expect(buttonHTML(readyHTML, 'crafting-complete')).toContain('Complete');
    expect(buttonHTML(readyHTML, 'crafting-complete')).not.toContain('disabled');
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

  test('renders laser ammo select intent without client-authored combat facts', () => {
    hudSelection.selectedInventoryTab = 'inventory';
    const state = craftingState();
    state.inventory!.stackable.push({
      item_id: 'ammunition_laser_mcb_50',
      display_name: 'MCB-50',
      quantity: 25,
      location: 'account_inventory',
    });

    const html = cargoPanel(state, 2_000);
    const button = buttonHTML(html, 'combat-ammo-select');
    const assignButton = buttonHTML(html, 'quickbar-ammo-assign');

    expect(button).toContain('data-ammo-family="laser"');
    expect(button).toContain('data-item-id="ammunition_laser_mcb_50"');
    expect(assignButton).toContain('data-quickbar-slot="2"');
    expect(assignButton).toContain('data-ammo-family="laser"');
    expect(assignButton).toContain('data-item-id="ammunition_laser_mcb_50"');
    expect(button).not.toContain('data-quantity');
    expect(button).not.toContain('data-damage');
    expect(button).not.toContain('data-multiplier');
    expect(assignButton).not.toContain('data-quantity');
    expect(assignButton).not.toContain('data-damage');
    expect(assignButton).not.toContain('data-multiplier');
    expect(html).toContain('draggable="true"');
    expect(html).toContain('data-quickbar-ammo-family="laser"');
    expect(html).toContain('data-quickbar-ammo-item-id="ammunition_laser_mcb_50"');

    state.combatEngagement.activeAmmo = {
      laser: { itemID: 'ammunition_laser_mcb_50', ammoKey: 'mcb_50', quantity: 25, powerMultiplier: 3 },
    };
    const selectedHTML = cargoPanel(state, 2_000);
    expect(buttonHTML(selectedHTML, 'combat-ammo-select')).toContain('Selected');
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

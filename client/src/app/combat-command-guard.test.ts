import { describe, expect, test } from 'vitest';

import { createInitialState } from '../state/reducer';
import { combatCommandsDisabled, SHIP_DISABLED_COMBAT_MESSAGE } from './combat-command-guard';

describe('combat command guard', () => {
  test('follows server-owned ship disabled state', () => {
    const state = createInitialState();

    expect(combatCommandsDisabled(state)).toBe(false);

    state.ship = {
      active_ship_id: 'starter',
      display_name: 'Starter',
      hull: 100,
      max_hull: 100,
      shield: 50,
      max_shield: 50,
      capacitor: 20,
      max_capacitor: 20,
      disabled: false,
      repair_state: 'active',
    };
    expect(combatCommandsDisabled(state)).toBe(false);

    state.ship = {
      ...state.ship,
      disabled: true,
      repair_state: 'disabled',
    };
    expect(combatCommandsDisabled(state)).toBe(true);
    expect(SHIP_DISABLED_COMBAT_MESSAGE).toBe('Fire rejected: ship disabled.');
  });
});

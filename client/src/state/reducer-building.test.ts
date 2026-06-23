import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';

describe('building production reconciliation', () => {
  test('production summary keeps build pending until the requested building is visible', () => {
    const state = createInitialState();
    state.pendingCommands = {
      'build-1': {
        requestID: 'build-1',
        op: OPERATIONS.planetBuildingBuild,
        queuedAt: 1,
        payload: { planet_id: 'planet-source', building_type: 'iron_extractor', slot: 'alpha' },
      },
    };

    const next = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.productionSummary,
        {
          planets: [
            {
              planet_id: 'planet-source',
              public_map_key: '1-1',
              production_enabled: true,
              last_calculated_at: 1200,
              energy_capacity_per_hour: 80,
              energy_reserved_per_hour: 0,
              storage: {
                planet_id: 'planet-source',
                public_map_key: '1-1',
                used_units: 0,
                free_units: 500,
                capacity_units: 500,
                updated_at: 1200,
                items: [],
              },
              buildings: [],
            },
          ],
        },
        2,
      ),
    });

    expect(next.pendingCommands['build-1']).toMatchObject({ op: OPERATIONS.planetBuildingBuild });
  });

  test('production summary event clears matching building mutations and applies server buildings', () => {
    const state = createInitialState();
    state.pendingCommands = {
      'build-1': {
        requestID: 'build-1',
        op: OPERATIONS.planetBuildingBuild,
        queuedAt: 1,
        payload: { planet_id: 'planet-source', building_type: 'iron_extractor', slot: 'alpha' },
      },
      'upgrade-1': {
        requestID: 'upgrade-1',
        op: OPERATIONS.planetBuildingUpgrade,
        queuedAt: 1,
        payload: { planet_id: 'planet-source', building_id: 'planet-source-building-iron_extractor-alpha', target_level: 2 },
      },
      'other-build': {
        requestID: 'other-build',
        op: OPERATIONS.planetBuildingBuild,
        queuedAt: 1,
        payload: { planet_id: 'planet-other', building_type: 'iron_extractor', slot: 'alpha' },
      },
    };

    const next = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.productionSummary,
        {
          planets: [
            {
              planet_id: 'planet-source',
              public_map_key: '1-1',
              production_enabled: true,
              last_calculated_at: 1200,
              energy_capacity_per_hour: 80,
              energy_reserved_per_hour: 0,
              storage: {
                planet_id: 'planet-source',
                public_map_key: '1-1',
                used_units: 0,
                free_units: 500,
                capacity_units: 500,
                updated_at: 1200,
                items: [],
              },
              buildings: [
                {
                  planet_id: 'planet-source',
                  public_map_key: '1-1',
                  building_id: 'planet-source-building-iron_extractor-alpha',
                  building_type: 'iron_extractor',
                  category: 'extractor',
                  level: 2,
                  state: 'active',
                  updated_at: 1200,
                },
              ],
            },
          ],
        },
        2,
      ),
    });

    expect(next.pendingCommands['build-1']).toBeUndefined();
    expect(next.pendingCommands['upgrade-1']).toBeUndefined();
    expect(next.pendingCommands['other-build']).toMatchObject({ op: OPERATIONS.planetBuildingBuild });
    expect(next.production?.planets[0].buildings[0]).toMatchObject({
      building_id: 'planet-source-building-iron_extractor-alpha',
      level: 2,
    });
  });
});

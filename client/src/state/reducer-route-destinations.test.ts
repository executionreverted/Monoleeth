import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';

describe('route destination reducer behavior', () => {
  test('non-planet route update clears matching typed create pending command with masked response id', () => {
    const state = createInitialState();
    state.pendingCommands = {
      'route-create-storage': {
        requestID: 'route-create-storage',
        op: OPERATIONS.routeCreate,
        queuedAt: 1,
        payload: {
          source_planet_id: 'planet-source',
          destination_type: 'storage',
          destination_id: 'storage-alpha',
          resource_item_id: 'refined_alloy',
          amount_per_hour: 40,
        },
      },
    };

    const next = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.routeUpdated,
        {
          route: {
            route_id: 'route-storage',
            source_planet_id: 'planet-source',
            from_public_map_key: '1-1',
            to_public_map_key: '1-1',
            destination: { type: 'storage' },
            resource_item_id: 'refined_alloy',
            amount_per_hour: 40,
            energy_cost_per_hour: 3,
            enabled: true,
            risk: { loss_chance: 0, min_loss_percent: 0, max_loss_percent: 0 },
            last_calculated_at: 1000,
            updated_at: 1000,
          },
        },
        2,
      ),
    });

    expect(next.pendingCommands['route-create-storage']).toBeUndefined();
    expect(next.routes?.routes[0]).toMatchObject({
      route_id: 'route-storage',
      destination: { type: 'storage', id: '' },
    });
  });
});

import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { parsePlanetDetail } from './reducer-discovery';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';

describe('route destination reducer behavior', () => {
  test('planet detail parser preserves server-owned route endpoint catalog', () => {
    const detail = parsePlanetDetail(
      {
        planet_id: 'planet-source',
        coordinates: { x: 1, y: 2 },
        route_endpoints: [
          { type: 'storage', id: 'storage-alpha', label: 'Storage' },
          { type: 'station', id: 'station-alpha', label: 'Station' },
        ],
      },
      null,
    );

    expect(detail.route_endpoints).toEqual([
      { type: 'storage', id: 'storage-alpha', label: 'Storage' },
      { type: 'station', id: 'station-alpha', label: 'Station' },
    ]);
  });

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

  test('route settled event stores server settlement result and clears matching pending settle command', () => {
    const state = createInitialState();
    state.pendingCommands = {
      'route-settle-1': {
        requestID: 'route-settle-1',
        op: OPERATIONS.routeSettle,
        queuedAt: 1,
        payload: { route_id: 'route-storage' },
      },
    };

    const next = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.routeSettled,
        {
          route: {
            route_id: 'route-storage',
            source_planet_id: 'planet-source',
            destination: { type: 'storage' },
            resource_item_id: 'refined_alloy',
            amount_per_hour: 40,
            energy_cost_per_hour: 3,
            enabled: true,
            risk: { loss_chance: 0, min_loss_percent: 0, max_loss_percent: 0 },
            last_calculated_at: 1000,
            updated_at: 1000,
          },
          settlement: {
            route_id: 'route-storage',
            resource_item_id: 'refined_alloy',
            settled_at: 1300,
            elapsed_applied_ms: 300000,
            wanted_amount: 40,
            taken_amount: 0,
            lost_amount: 0,
            delivered_amount: 0,
            added_amount: 0,
            source_empty: true,
            destination_full: false,
            loss_applied: false,
            no_op: true,
          },
        },
        2,
      ),
    });

    expect(next.pendingCommands['route-settle-1']).toBeUndefined();
    expect(next.routeSettlements?.['route-storage']).toMatchObject({
      route_id: 'route-storage',
      resource_item_id: 'refined_alloy',
      wanted_amount: 40,
      source_empty: true,
      no_op: true,
    });
    expect(next.routes?.routes[0]).toMatchObject({
      route_id: 'route-storage',
      destination: { type: 'storage', id: '' },
    });
  });

  test('route settle all response stores each settlement result', () => {
    const state = createInitialState();
    state.pendingCommands = {
      'route-settle-all': {
        requestID: 'route-settle-all',
        op: OPERATIONS.routeSettle,
        queuedAt: 1,
        payload: {},
      },
    };

    const next = reduceClientState(state, {
      type: 'responseReceived',
      envelope: {
        request_id: 'route-settle-all',
        ok: true,
        server_time: 1400,
        v: 1,
        payload: {
          settlements: [
            {
              route_id: 'route-a',
              resource_item_id: 'raw_ore',
              settled_at: 1400,
              elapsed_applied_ms: 300000,
              wanted_amount: 20,
              taken_amount: 20,
              lost_amount: 0,
              delivered_amount: 20,
              added_amount: 20,
              source_empty: false,
              destination_full: false,
              loss_applied: false,
              no_op: false,
            },
            {
              route_id: 'route-b',
              resource_item_id: 'refined_alloy',
              settled_at: 1400,
              elapsed_applied_ms: 300000,
              wanted_amount: 40,
              taken_amount: 15,
              lost_amount: 0,
              delivered_amount: 15,
              added_amount: 10,
              source_empty: false,
              destination_full: true,
              loss_applied: false,
              no_op: false,
            },
          ],
        },
      },
    });

    expect(next.pendingCommands['route-settle-all']).toBeUndefined();
    expect(next.routeSettlements?.['route-a']).toMatchObject({
      route_id: 'route-a',
      added_amount: 20,
      destination_full: false,
    });
    expect(next.routeSettlements?.['route-b']).toMatchObject({
      route_id: 'route-b',
      added_amount: 10,
      destination_full: true,
    });
  });
});

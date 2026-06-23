import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';

describe('route reducer reconciliation', () => {
  test('route snapshot upserts global and selected planet routes', () => {
    const initialRoute = routeFixture({
      resource_item_id: 'ferrite_ore',
      amount_per_hour: 10,
      energy_cost_per_hour: 2,
      enabled: false,
      risk: { loss_chance: 0.1, min_loss_percent: 1, max_loss_percent: 3 },
    });
    const seeded = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'route-bootstrap',
        ok: true,
        payload: {
          planet_detail: {
            planet_id: 'planet-eris',
            biome: 'ice',
            planet_type: 'dwarf_planet',
            rarity: 'uncommon',
            level: 2,
            intel_state: 'fresh',
            confidence: 88,
            last_seen_at: 1000,
            owner_status: 'owned',
            discovered_at: 900,
            production_locked: false,
            available_commands: ['route.snapshot'],
            routes: [initialRoute],
          },
          routes: { routes: [initialRoute] },
        },
        server_time: 1000,
        v: 1,
      },
    });

    const updatedRoute = {
      ...initialRoute,
      amount_per_hour: 25,
      enabled: true,
      last_calculated_at: 1250,
      updated_at: 1250,
    };
    const withRoute = reduceClientState(seeded, {
      type: 'responseReceived',
      envelope: {
        request_id: 'route-snapshot',
        ok: true,
        payload: { route: updatedRoute },
        server_time: 1250,
        v: 1,
      },
    });

    expect(withRoute.routes?.routes).toHaveLength(1);
    expect(withRoute.routes?.routes[0]).toMatchObject({
      route_id: 'route-1',
      amount_per_hour: 25,
      enabled: true,
      from_public_map_key: '1-1',
      to_public_map_key: '1-2',
    });
    expect(withRoute.planetIntel?.selectedPlanet?.routes[0]).toEqual(withRoute.routes?.routes[0]);

    const withEventRoute = reduceClientState(withRoute, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.routeSnapshot, { route: { ...updatedRoute, amount_per_hour: 30, updated_at: 1300 } }, 3),
    });

    expect(withEventRoute.routes?.routes[0]).toMatchObject({
      route_id: 'route-1',
      amount_per_hour: 30,
      enabled: true,
      from_public_map_key: '1-1',
      to_public_map_key: '1-2',
    });
    expect(withEventRoute.planetIntel?.selectedPlanet?.routes[0]).toEqual(withEventRoute.routes?.routes[0]);
  });

  test('route updated event clears matching pending route mutations and upserts safe route state', () => {
    const seeded = routeSeedState(routeFixture({ enabled: false, amount_per_hour: 10 }));
    seeded.pendingCommands = {
      'route-create-1': {
        requestID: 'route-create-1',
        op: OPERATIONS.routeCreate,
        queuedAt: 1,
        payload: {
          source_planet_id: 'planet-eris',
          destination_planet_id: 'planet-nova',
          resource_item_id: 'refined_alloy',
          amount_per_hour: 25,
        },
      },
      'route-create-other': {
        requestID: 'route-create-other',
        op: OPERATIONS.routeCreate,
        queuedAt: 1,
        payload: {
          source_planet_id: 'planet-eris',
          destination_planet_id: 'planet-nova',
          resource_item_id: 'raw_ore',
          amount_per_hour: 25,
        },
      },
      'route-update-1': {
        requestID: 'route-update-1',
        op: OPERATIONS.routeUpdate,
        queuedAt: 1,
        payload: {
          route_id: 'route-1',
          destination_planet_id: 'planet-nova',
          resource_item_id: 'refined_alloy',
          amount_per_hour: 25,
        },
      },
      'route-update-other': {
        requestID: 'route-update-other',
        op: OPERATIONS.routeUpdate,
        queuedAt: 1,
        payload: {
          route_id: 'route-other',
          destination_planet_id: 'planet-nova',
          resource_item_id: 'refined_alloy',
          amount_per_hour: 25,
        },
      },
      'route-enable-1': { requestID: 'route-enable-1', op: OPERATIONS.routeEnable, queuedAt: 1, payload: { route_id: 'route-1' } },
      'route-disable-1': { requestID: 'route-disable-1', op: OPERATIONS.routeDisable, queuedAt: 1, payload: { route_id: 'route-other' } },
      'claim-1': { requestID: 'claim-1', op: OPERATIONS.discoveryClaimPlanet, queuedAt: 1, payload: { planet_id: 'planet-eris' } },
    };

    const next = reduceClientState(seeded, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.routeUpdated, { route: routeFixture({ enabled: true, amount_per_hour: 25, updated_at: 1400 }) }, 4),
    });

    expect(next.pendingCommands['route-create-1']).toBeUndefined();
    expect(next.pendingCommands['route-create-other']).toMatchObject({ op: OPERATIONS.routeCreate });
    expect(next.pendingCommands['route-update-1']).toBeUndefined();
    expect(next.pendingCommands['route-update-other']).toMatchObject({ op: OPERATIONS.routeUpdate });
    expect(next.pendingCommands['route-enable-1']).toBeUndefined();
    expect(next.pendingCommands['route-disable-1']).toMatchObject({ op: OPERATIONS.routeDisable });
    expect(next.pendingCommands['claim-1']).toMatchObject({ op: OPERATIONS.discoveryClaimPlanet });
    expect(next.routes?.routes[0]).toMatchObject({ route_id: 'route-1', enabled: true, amount_per_hour: 25 });
    expect(next.planetIntel?.selectedPlanet?.routes[0]).toEqual(next.routes?.routes[0]);
    expect(next.commandLog.some((line) => line.text === 'Route state updated.')).toBe(true);
  });

  test('route settled event clears pending settlement and reconciles route list plus storage', () => {
    const seeded = routeSeedState(routeFixture({ amount_per_hour: 40 }));
    seeded.pendingCommands = {
      'route-settle-1': { requestID: 'route-settle-1', op: OPERATIONS.routeSettle, queuedAt: 1, payload: { route_id: 'route-1' } },
      'route-settle-other': { requestID: 'route-settle-other', op: OPERATIONS.routeSettle, queuedAt: 1, payload: { route_id: 'route-other' } },
      'route-settle-reconcile': { requestID: 'route-settle-reconcile', op: OPERATIONS.routeSettle, queuedAt: 1, payload: {} },
    };

    const next = reduceClientState(seeded, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.routeSettled,
        {
          route: routeFixture({ last_calculated_at: 1800, updated_at: 1800 }),
          routes: { routes: [routeFixture({ last_calculated_at: 1800, updated_at: 1800 })] },
          settlement: settlementFixture({ added_amount: 40, delivered_amount: 40, taken_amount: 40, settled_at: 1800 }),
          planet_storage: {
            planet_id: 'planet-eris',
            used_units: 60,
            free_units: 190,
            capacity_units: 250,
            updated_at: 1800,
            items: [{ item_id: 'refined_alloy', quantity: 60 }],
          },
        },
        5,
      ),
    });

    expect(next.pendingCommands['route-settle-1']).toBeUndefined();
    expect(next.pendingCommands['route-settle-other']).toMatchObject({ op: OPERATIONS.routeSettle });
    expect(next.pendingCommands['route-settle-reconcile']).toMatchObject({ op: OPERATIONS.routeSettle });
    expect(next.routes?.routes[0].last_settlement).toMatchObject({
      route_id: 'route-1',
      resource_item_id: 'refined_alloy',
      settled_at: 1800,
      added_amount: 40,
      source_empty: false,
      destination_full: false,
      no_op: false,
    });
    expect(next.planetIntel?.selectedPlanet?.routes[0].last_settlement).toEqual(next.routes?.routes[0].last_settlement);
    expect(next.planetIntel?.selectedPlanet?.production?.storage).toMatchObject({
      planet_id: 'planet-eris',
      used_units: 60,
      items: [{ item_id: 'refined_alloy', quantity: 60 }],
    });
    expect(next.commandLog.some((line) => line.text === 'Route settlement reconciled.')).toBe(true);
  });

  test('route settled reconcile event clears empty settle pending without clearing route-specific settle', () => {
    const seeded = routeSeedState(routeFixture({ amount_per_hour: 40 }));
    seeded.pendingCommands = {
      'route-settle-1': { requestID: 'route-settle-1', op: OPERATIONS.routeSettle, queuedAt: 1, payload: { route_id: 'route-1' } },
      'route-settle-reconcile': { requestID: 'route-settle-reconcile', op: OPERATIONS.routeSettle, queuedAt: 1, payload: {} },
    };

    const next = reduceClientState(seeded, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.routeSettled, { routes: { routes: [routeFixture({ last_calculated_at: 1800, updated_at: 1800 })] } }, 5),
    });

    expect(next.pendingCommands['route-settle-reconcile']).toBeUndefined();
    expect(next.pendingCommands['route-settle-1']).toMatchObject({ op: OPERATIONS.routeSettle });
    expect(next.routes?.routes[0]).toMatchObject({ route_id: 'route-1', last_calculated_at: 1800 });
    expect(next.commandLog.some((line) => line.text.includes('Unhandled event'))).toBe(false);
  });

  test('route settle response reconciles settlement arrays onto durable route list fallback', () => {
    const seeded = routeSeedState(routeFixture({ amount_per_hour: 40 }));
    seeded.pendingCommands = {
      'route-settle-reconcile': { requestID: 'route-settle-reconcile', op: OPERATIONS.routeSettle, queuedAt: 1, payload: {} },
    };

    const next = reduceClientState(seeded, {
      type: 'responseReceived',
      envelope: {
        request_id: 'route-settle-reconcile',
        ok: true,
        payload: {
          routes: { routes: [routeFixture({ last_calculated_at: 1900, updated_at: 1900 })] },
          settlements: [settlementFixture({ settled_at: 1900, source_empty: true, no_op: true })],
        },
        server_time: 1900,
        v: 1,
      },
    });

    expect(next.pendingCommands['route-settle-reconcile']).toBeUndefined();
    expect(next.routes?.routes[0]).toMatchObject({
      route_id: 'route-1',
      last_calculated_at: 1900,
      last_settlement: { route_id: 'route-1', source_empty: true, no_op: true },
    });
    expect(next.planetIntel?.selectedPlanet?.routes[0].last_settlement).toEqual(next.routes?.routes[0].last_settlement);
  });

  test('route snapshot and list events preserve the last server settlement result', () => {
    const seeded = routeSeedState(routeFixture({ amount_per_hour: 40 }));
    const settled = reduceClientState(seeded, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.routeSettled,
        {
          route: routeFixture({ last_calculated_at: 1800, updated_at: 1800 }),
          settlement: settlementFixture({ settled_at: 1800, source_empty: true, no_op: true }),
        },
        5,
      ),
    });

    const snapshotted = reduceClientState(settled, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.routeSnapshot, { route: routeFixture({ last_calculated_at: 1800, updated_at: 1801 }) }, 6),
    });
    const listed = reduceClientState(snapshotted, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.routeList, { routes: [routeFixture({ last_calculated_at: 1800, updated_at: 1802 })] }, 7),
    });

    expect(listed.routes?.routes[0]).toMatchObject({
      route_id: 'route-1',
      updated_at: 1802,
      last_settlement: { route_id: 'route-1', source_empty: true, no_op: true },
    });
    expect(listed.planetIntel?.selectedPlanet?.routes[0].last_settlement).toEqual(listed.routes?.routes[0].last_settlement);
  });
});

function routeFixture(overrides: Partial<NonNullable<ReturnType<typeof createInitialState>['routes']>['routes'][number]> = {}) {
  return {
    route_id: 'route-1',
    source_planet_id: 'planet-eris',
    from_public_map_key: '1-1',
    to_public_map_key: '1-2',
    destination: { type: 'planet', id: 'planet-nova' },
    resource_item_id: 'refined_alloy',
    amount_per_hour: 40,
    energy_cost_per_hour: 4,
    enabled: true,
    risk: { loss_chance: 0.05, min_loss_percent: 0, max_loss_percent: 0 },
    last_calculated_at: 1000,
    updated_at: 1000,
    ...overrides,
  };
}

function settlementFixture(overrides: Record<string, unknown> = {}) {
  return {
    route_id: 'route-1',
    resource_item_id: 'refined_alloy',
    settled_at: 1800,
    elapsed_applied_ms: 3_600_000,
    wanted_amount: 40,
    taken_amount: 0,
    lost_amount: 0,
    delivered_amount: 0,
    added_amount: 0,
    source_empty: false,
    destination_full: false,
    loss_applied: false,
    no_op: false,
    ...overrides,
  };
}

function routeSeedState(route: NonNullable<ReturnType<typeof createInitialState>['routes']>['routes'][number]) {
  const state = createInitialState();
  state.routes = { routes: [route] };
  state.planetIntel = {
    knownSignals: 2,
    staleIntel: 0,
    ownedPlanets: 2,
    planets: [
      {
        planet_id: 'planet-eris',
        biome: 'ice',
        planet_type: 'dwarf_planet',
        rarity: 'uncommon',
        level: 2,
        intel_state: 'verified',
        confidence: 100,
        last_seen_at: 1000,
        owner_status: 'owned_by_you',
        discovered_at: 900,
      },
      {
        planet_id: 'planet-nova',
        biome: 'rock',
        planet_type: 'terrestrial',
        rarity: 'common',
        level: 1,
        intel_state: 'verified',
        confidence: 100,
        last_seen_at: 1000,
        owner_status: 'owned_by_you',
        discovered_at: 900,
      },
    ],
    selectedPlanet: {
      planet_id: 'planet-eris',
      biome: 'ice',
      planet_type: 'dwarf_planet',
      rarity: 'uncommon',
      level: 2,
      intel_state: 'verified',
      confidence: 100,
      last_seen_at: 1000,
      owner_status: 'owned_by_you',
      discovered_at: 900,
      coordinates: { x: 320, y: 140 },
      production_locked: false,
      available_commands: ['route.list'],
      routes: [route],
      production: {
        planet_id: 'planet-eris',
        production_enabled: true,
        last_calculated_at: 1000,
        energy_capacity_per_hour: 40,
        energy_reserved_per_hour: 0,
        storage: {
          planet_id: 'planet-eris',
          used_units: 100,
          free_units: 150,
          capacity_units: 250,
          updated_at: 1000,
          items: [{ item_id: 'refined_alloy', quantity: 100 }],
        },
        buildings: [],
      },
    },
    lastScan: null,
  };
  state.production = { planets: [state.planetIntel.selectedPlanet!.production!] };
  return state;
}

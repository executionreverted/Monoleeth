import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';
import type { ClientState, RouteSettlementSummary, RouteSummary } from './types';

describe('route list reconciliation', () => {
  test('route.list response updates global and selected planet route caches', () => {
    const state = routeListState(staleRoute());
    const recovered = route({
      route_id: 'route-1',
      amount_per_hour: 55,
      enabled: true,
      updated_at: 2000,
    });

    const next = reduceClientState(state, {
      type: 'responseReceived',
      envelope: {
        request_id: 'route-list-durable-fallback',
        ok: true,
        payload: { routes: { routes: [recovered] } },
        server_time: 2000,
        v: 1,
      },
    });

    expect(next.routes?.routes).toEqual([recovered]);
    expect(next.planetIntel?.selectedPlanet?.routes).toEqual([recovered]);
  });

  test('route.list response preserves last settlement feedback during durable fallback refresh', () => {
    const lastSettlement = settlement({ source_empty: true, no_op: true, settled_at: 1900 });
    const state = routeListState(staleRoute({ last_settlement: lastSettlement }));
    const recovered = route({
      route_id: 'route-1',
      amount_per_hour: 55,
      enabled: true,
      updated_at: 2000,
    });

    const next = reduceClientState(state, {
      type: 'responseReceived',
      envelope: {
        request_id: 'route-list-durable-fallback',
        ok: true,
        payload: { routes: { routes: [recovered] } },
        server_time: 2000,
        v: 1,
      },
    });

    expect(next.routes?.routes).toEqual([{ ...recovered, last_settlement: lastSettlement }]);
    expect(next.planetIntel?.selectedPlanet?.routes).toEqual(next.routes?.routes);
  });

  test('route.list event removes stale selected planet routes missing from server list', () => {
    const stale = staleRoute();
    const otherSourceRoute = route({
      route_id: 'route-other',
      source_planet_id: 'planet-nova',
      destination: { type: 'planet', id: 'planet-eris' },
    });
    const state = routeListState(stale);

    const next = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.routeList, { routes: [otherSourceRoute] }),
    });

    expect(next.routes?.routes).toEqual([otherSourceRoute]);
    expect(next.planetIntel?.selectedPlanet?.routes).toEqual([]);
  });
});

function routeListState(selectedRoute: RouteSummary): ClientState {
  const state = createInitialState();
  state.routes = { routes: [selectedRoute] };
  state.planetIntel = {
    knownSignals: 2,
    staleIntel: 0,
    ownedPlanets: 2,
    planets: [
      planetSummary('planet-eris', 'ice'),
      planetSummary('planet-nova', 'rock'),
    ],
    selectedPlanet: {
      ...planetSummary('planet-eris', 'ice'),
      coordinates: { x: 320, y: 140 },
      production_locked: false,
      available_commands: ['route.list'],
      routes: [selectedRoute],
    },
    lastScan: null,
  };
  return state;
}

function planetSummary(planetID: string, biome: string) {
  return {
    planet_id: planetID,
    biome,
    planet_type: 'dwarf_planet',
    rarity: 'uncommon',
    level: 2,
    intel_state: 'verified',
    confidence: 100,
    last_seen_at: 1000,
    owner_status: 'owned_by_you',
    discovered_at: 900,
  };
}

function staleRoute(patch: Partial<RouteSummary> = {}): RouteSummary {
  return route({ amount_per_hour: 10, enabled: false, updated_at: 1000, ...patch });
}

function route(patch: Partial<RouteSummary>): RouteSummary {
  return {
    route_id: 'route-1',
    source_planet_id: 'planet-eris',
    destination: { type: 'planet', id: 'planet-nova' },
    resource_item_id: 'refined_alloy',
    amount_per_hour: 40,
    energy_cost_per_hour: 2,
    enabled: false,
    risk: { loss_chance: 0.1, min_loss_percent: 1, max_loss_percent: 3 },
    last_calculated_at: 1000,
    updated_at: 1000,
    from_public_map_key: '1-1',
    to_public_map_key: '1-2',
    ...patch,
  };
}

function settlement(patch: Partial<RouteSettlementSummary> = {}): RouteSettlementSummary {
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
    ...patch,
  };
}

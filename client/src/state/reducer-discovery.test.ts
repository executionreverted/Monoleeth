import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event, expectServerOwnedGameplayCleared, stateWithServerOwnedGameplay } from './reducer-fixtures.test-support';
import { isWithinMinimapProjectionWindow, worldMapMemoryMarkers } from './world-memory';

describe('reduceClientState', () => {
  test('scan mode is local control state and does not invent gameplay values', () => {
    const enabled = reduceClientState(createInitialState(), { type: 'scanModeToggled', enabled: true, now: 1234 });

    expect(enabled.scanMode).toEqual({
      enabled: true,
      nextPulseAt: 1234,
      lastRejectedAt: null,
      lastError: null,
    });
    expect(enabled.planetIntel).toBeNull();
    expect(enabled.progression).toBeNull();
    expect(enabled.visibleEntities).toEqual({});

    const rejected = reduceClientState(enabled, {
      type: 'scanPulseRejected',
      message: 'Scanner cooldown active.',
      rejectedAt: 2000,
      backoffUntil: 5200,
    });

    expect(rejected.scanMode).toEqual({
      enabled: true,
      nextPulseAt: 5200,
      lastRejectedAt: 2000,
      lastError: 'Scanner cooldown active.',
    });
    expect(rejected.planetIntel).toBeNull();

    const disabled = reduceClientState(rejected, { type: 'scanModeToggled', enabled: false });
    expect(disabled.scanMode).toEqual({
      enabled: false,
      nextPulseAt: null,
      lastRejectedAt: null,
      lastError: null,
    });
  });

  test('scan events update scan mode timing from server-safe summaries only', () => {
    const enabled = reduceClientState(createInitialState(), { type: 'scanModeToggled', enabled: true, now: 1000 });
    const resolveAfter = Date.now() + 9000;
    const started = reduceClientState(enabled, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.scanPulseStarted, {
        pulse_reference: 'pulse-1',
        status: 'started',
        resolve_after: resolveAfter,
      }),
    });

    expect(started.planetIntel?.lastScan).toMatchObject({
      pulse_reference: 'pulse-1',
      status: 'started',
      resolve_after: resolveAfter,
    });
    expect(started.scanMode.enabled).toBe(true);
    expect(started.scanMode.nextPulseAt).toBe(resolveAfter);
    expect(started.scanMode.lastError).toBeNull();
    expect(started.progression).toBeNull();

    const beforeResolve = Date.now();
    const resolved = reduceClientState(started, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.scanPulseResolved, {
        pulse_reference: 'pulse-1',
        status: 'no_signal',
        message: 'Scanner pulse resolved with no signal.',
      }, 2),
    });

    expect(resolved.planetIntel?.lastScan?.status).toBe('no_signal');
    expect(resolved.scanMode.enabled).toBe(true);
    expect(resolved.scanMode.nextPulseAt ?? 0).toBeGreaterThanOrEqual(beforeResolve + 2500);
  });

  test('player reveal scan status does not carry planet intel or progression truth', () => {
    const discovered = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.scanPulseResolved, {
        pulse_reference: 'pulse-planet',
        status: 'planet_discovered',
        planet_id: 'planet-1',
        xp_granted: true,
        signal: {
          biome: 'origin_belt',
          signal_band: 'low',
          approx_distance: 'near',
        },
      }),
    });

    const revealed = reduceClientState(discovered, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.scanPulseResolved, {
        pulse_reference: 'pulse-player',
        status: 'player_revealed',
        message: 'Scanner revealed a radar contact.',
      }, 2),
    });

    expect(revealed.planetIntel?.knownSignals).toBe(discovered.planetIntel?.knownSignals);
    expect(revealed.planetIntel?.lastScan).toMatchObject({
      pulse_reference: 'pulse-player',
      status: 'player_revealed',
      message: 'Scanner revealed a radar contact.',
      xp_granted: false,
    });
    expect(revealed.planetIntel?.lastScan?.signal).toBeUndefined();
    expect(revealed.planetIntel?.lastScan?.planet_id).toBeUndefined();
    expect(revealed.progression).toBeNull();
  });

  test('known planets event refreshes remembered minimap contacts without sync', () => {
    const state = reduceClientState(
      {
        ...createInitialState(),
        minimap: {
          radar_range: 1000,
          projection_window_size: 2000,
          live_contacts: [
            {
              entity_id: 'player-local',
              entity_type: 'player',
              position: { x: 0, y: 0 },
              disposition: 'self',
              status_flags: ['self'],
            },
          ],
          remembered: [],
        },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.knownPlanets, {
          planets: [
            {
              planet_id: 'planet-eris',
              biome: 'ice',
              planet_type: 'terran',
              rarity: 'common',
              level: 2,
              intel_state: 'fresh',
              confidence: 100,
              last_seen_at: 1500,
              owner_status: 'unclaimed',
              discovered_at: 1400,
            },
          ],
          counts: { known: 1, stale: 0, owned: 0 },
          minimap: {
            radar_range: 1000,
            projection_window_size: 2000,
            live_contacts: [
              {
                entity_id: 'player-local',
                entity_type: 'player',
                position: { x: 0, y: 0 },
                disposition: 'self',
                status_flags: ['self'],
              },
            ],
            remembered: [
              {
                kind: 'known_planet',
                planet_id: 'planet-eris',
                detail_id: 'planet-eris',
                label: 'terran ice',
                position: { x: 520, y: -240 },
                freshness: 'fresh',
              },
            ],
          },
        }),
      },
    );

    expect(state.planetIntel?.planets[0]?.planet_id).toBe('planet-eris');
    expect(state.minimap?.remembered).toEqual([
      {
        kind: 'known_planet',
        planet_id: 'planet-eris',
        detail_id: 'planet-eris',
        label: 'terran ice',
        position: { x: 520, y: -240 },
        freshness: 'fresh',
      },
    ]);
    expect(worldMapMemoryMarkers(state)).toEqual([
      {
        id: 'known_planet:planet-eris',
        kind: 'known_planet',
        label: 'terran ice',
        position: { x: 520, y: -240 },
        detailID: 'planet-eris',
        state: 'fresh',
      },
    ]);
  });

  test('far remembered planets stay map memory without becoming nearby radar contacts', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.knownPlanets, {
        planets: [
          {
            planet_id: 'planet-far',
            biome: 'ice',
            planet_type: 'dwarf_planet',
            rarity: 'uncommon',
            level: 2,
            intel_state: 'fresh',
            confidence: 92,
            last_seen_at: 1500,
            owner_status: 'unclaimed',
            discovered_at: 1400,
          },
        ],
        counts: { known: 1, stale: 0, owned: 0 },
        minimap: {
          radar_range: 1000,
          projection_window_size: 2000,
          live_contacts: [
            {
              entity_id: 'player-local',
              entity_type: 'player',
              position: { x: 0, y: 0 },
              disposition: 'self',
              status_flags: ['self'],
            },
          ],
          remembered: [
            {
              kind: 'known_planet',
              planet_id: 'planet-far',
              detail_id: 'planet-far',
              label: 'far planet',
              position: { x: 5200, y: -3800 },
              freshness: 'fresh',
            },
          ],
        },
      }),
    });

    expect(isWithinMinimapProjectionWindow({ x: 0, y: 0 }, { x: 5200, y: -3800 }, 1000)).toBe(false);
    expect(worldMapMemoryMarkers(state)).toEqual([
      {
        id: 'known_planet:planet-far',
        kind: 'known_planet',
        label: 'far planet',
        position: { x: 5200, y: -3800 },
        detailID: 'planet-far',
        state: 'fresh',
      },
    ]);
  });

  test('remembered intel keeps stale source evidence but drops invalidated and blocks wrong-sector render', () => {
    const state = reduceClientState(
      {
        ...createInitialState(),
        sector: { sector_key: 'origin', name: 'Origin', region: 'Belt', danger: 'low', contested: false },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.knownPlanets, {
          planets: [
            {
              planet_id: 'planet-stale',
              sector_key: 'origin',
              biome: 'ice',
              planet_type: 'dwarf_planet',
              rarity: 'uncommon',
              level: 2,
              intel_state: 'stale',
              confidence: 40,
              last_seen_at: 1500,
              owner_status: 'unclaimed',
              discovered_at: 1400,
            },
          ],
          counts: { known: 3, stale: 1, owned: 0 },
          minimap: {
            radar_range: 1000,
            projection_window_size: 2000,
            live_contacts: [
              {
                entity_id: 'player-local',
                entity_type: 'player',
                position: { x: 0, y: 0 },
                disposition: 'self',
                status_flags: ['self'],
                projection_source: 'worker_projection',
              },
            ],
            remembered: [
              {
                kind: 'known_planet',
                sector_key: 'origin',
                planet_id: 'planet-stale',
                detail_id: 'planet-stale',
                label: 'stale planet',
                position: { x: 420, y: -180 },
                freshness: 'stale',
                projection_source: 'player_intel',
              },
              {
                kind: 'known_planet',
                sector_key: 'origin',
                planet_id: 'planet-invalidated',
                detail_id: 'planet-invalidated',
                label: 'invalidated planet',
                position: { x: 460, y: -220 },
                freshness: 'fresh',
                invalidated: true,
              },
              {
                kind: 'known_planet',
                sector_key: 'origin',
                planet_id: 'planet-wrong-zone',
                detail_id: 'planet-wrong-zone',
                label: 'wrong zone planet',
                position: { x: 500, y: -260 },
                freshness: 'wrong_zone',
              },
              {
                kind: 'known_planet',
                sector_key: 'other-sector',
                planet_id: 'planet-wrong-sector',
                detail_id: 'planet-wrong-sector',
                label: 'wrong sector planet',
                position: { x: 540, y: -280 },
                freshness: 'fresh',
                projection_source: 'player_intel',
              },
            ],
          },
        }),
      },
    );

    expect(state.minimap?.remembered).toEqual([
      {
        kind: 'known_planet',
        sector_key: 'origin',
        planet_id: 'planet-stale',
        detail_id: 'planet-stale',
        label: 'stale planet',
        position: { x: 420, y: -180 },
        freshness: 'stale',
        projection_source: 'player_intel',
      },
      {
        kind: 'known_planet',
        sector_key: 'other-sector',
        planet_id: 'planet-wrong-sector',
        detail_id: 'planet-wrong-sector',
        label: 'wrong sector planet',
        position: { x: 540, y: -280 },
        freshness: 'fresh',
        projection_source: 'player_intel',
      },
    ]);
    expect(state.minimap?.live_contacts[0]?.projection_source).toBe('worker_projection');
    expect(worldMapMemoryMarkers(state)).toEqual([
      {
        id: 'known_planet:planet-stale',
        kind: 'known_planet',
        label: 'stale planet',
        position: { x: 420, y: -180 },
        detailID: 'planet-stale',
        state: 'stale',
        projectionSource: 'player_intel',
      },
    ]);
  });

  test('planet detail coordinates create a selectable world memory marker', () => {
    const withKnownPlanets = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.knownPlanets, {
        planets: [
          {
            planet_id: 'planet-eris',
            biome: 'ice',
            planet_type: 'dwarf_planet',
            rarity: 'uncommon',
            level: 2,
            intel_state: 'fresh',
            confidence: 88,
            last_seen_at: 1000,
            owner_status: 'unclaimed',
            discovered_at: 900,
          },
        ],
        counts: { known: 1, stale: 0, owned: 0 },
      }),
    });
    const withDetail = reduceClientState(withKnownPlanets, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.planetDetail,
        {
          planet_id: 'planet-eris',
          biome: 'ice',
          planet_type: 'dwarf_planet',
          rarity: 'uncommon',
          level: 2,
          intel_state: 'fresh',
          confidence: 88,
          last_seen_at: 1000,
          owner_status: 'unclaimed',
          discovered_at: 900,
          coordinates: { x: 320, y: -140 },
          production_locked: true,
          routes: [],
          available_commands: [],
        },
        2,
      ),
    });

    expect(withDetail.planetIntel?.selectedPlanet?.coordinates).toEqual({ x: 320, y: -140 });
    expect(worldMapMemoryMarkers(withKnownPlanets)).toEqual([]);
    expect(worldMapMemoryMarkers(withDetail)).toEqual([
      {
        id: 'known_planet:planet-eris',
        kind: 'known_planet',
        label: 'dwarf planet / ice',
        position: { x: 320, y: -140 },
        detailID: 'planet-eris',
        state: 'unclaimed',
      },
    ]);
  });

  test('planet detail without coordinates does not create an origin memory marker', () => {
    const withDetail = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.planetDetail, {
        planet_id: 'planet-locked',
        biome: 'ice',
        planet_type: 'dwarf_planet',
        intel_state: 'known',
        production_locked: true,
        routes: [],
        available_commands: [],
      }),
    });

    expect(withDetail.planetIntel?.selectedPlanet?.coordinates).toBeNull();
    expect(worldMapMemoryMarkers(withDetail)).toEqual([]);
  });

  test('planet detail does not reuse stale coordinates from another selected planet', () => {
    const withFirstDetail = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.planetDetail, {
        planet_id: 'planet-a',
        biome: 'ice',
        planet_type: 'dwarf_planet',
        level: 2,
        intel_state: 'fresh',
        confidence: 90,
        coordinates: { x: 720, y: -320 },
        production_locked: true,
        routes: [],
        available_commands: [],
      }),
    });

    const withSecondDetail = reduceClientState(withFirstDetail, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.planetDetail,
        {
          planet_id: 'planet-b',
          biome: 'rock',
          planet_type: 'moon',
          level: 1,
          intel_state: 'stale',
          confidence: 42,
          production_locked: true,
          routes: [],
          available_commands: [],
        },
        2,
      ),
    });

    expect(withSecondDetail.planetIntel?.selectedPlanet?.planet_id).toBe('planet-b');
    expect(withSecondDetail.planetIntel?.selectedPlanet?.coordinates).toBeNull();
    expect(worldMapMemoryMarkers(withSecondDetail)).toEqual([]);
  });

  test('planet storage summary updates selected planet and production state', () => {
    const seeded = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'planet-bootstrap',
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
            available_commands: ['planet.storage_summary'],
            routes: [],
            production: {
              planet_id: 'planet-eris',
              production_enabled: true,
              last_calculated_at: 1000,
              energy_capacity_per_hour: 20,
              energy_reserved_per_hour: 5,
              storage: {
                planet_id: 'planet-eris',
                used_units: 2,
                free_units: 98,
                capacity_units: 100,
                updated_at: 1000,
                items: [{ item_id: 'ferrite_ore', quantity: 2 }],
              },
              buildings: [],
            },
          },
          production: {
            planets: [
              {
                planet_id: 'planet-eris',
                production_enabled: true,
                last_calculated_at: 1000,
                energy_capacity_per_hour: 20,
                energy_reserved_per_hour: 5,
                storage: {
                  planet_id: 'planet-eris',
                  used_units: 2,
                  free_units: 98,
                  capacity_units: 100,
                  updated_at: 1000,
                  items: [{ item_id: 'ferrite_ore', quantity: 2 }],
                },
                buildings: [],
              },
            ],
          },
        },
        server_time: 1000,
        v: 1,
      },
    });

    const withStorage = reduceClientState(seeded, {
      type: 'responseReceived',
      envelope: {
        request_id: 'planet-storage',
        ok: true,
        payload: {
          planet_storage: {
            planet_id: 'planet-eris',
            used_units: 12,
            free_units: 88,
            capacity_units: 100,
            updated_at: 1200,
            items: [
              { item_id: 'ferrite_ore', quantity: 8 },
              { item_id: 'ion_residue', quantity: 4 },
            ],
          },
        },
        server_time: 1200,
        v: 1,
      },
    });

    expect(withStorage.planetIntel?.selectedPlanet?.production?.storage).toMatchObject({
      used_units: 12,
      free_units: 88,
      updated_at: 1200,
      items: [
        { item_id: 'ferrite_ore', quantity: 8 },
        { item_id: 'ion_residue', quantity: 4 },
      ],
    });
    expect(withStorage.production?.planets[0].storage).toEqual(withStorage.planetIntel?.selectedPlanet?.production?.storage);

    const withEventStorage = reduceClientState(withStorage, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.planetStorageSummary,
        {
          planet_id: 'planet-eris',
          used_units: 16,
          free_units: 84,
          capacity_units: 100,
          updated_at: 1300,
          items: [{ item_id: 'ion_residue', quantity: 16 }],
        },
        3,
      ),
    });

    expect(withEventStorage.planetIntel?.selectedPlanet?.production?.storage).toMatchObject({
      used_units: 16,
      free_units: 84,
      updated_at: 1300,
      items: [{ item_id: 'ion_residue', quantity: 16 }],
    });
    expect(withEventStorage.production?.planets[0].storage).toEqual(
      withEventStorage.planetIntel?.selectedPlanet?.production?.storage,
    );
  });

  test('route snapshot upserts global and selected planet routes', () => {
    const initialRoute = {
      route_id: 'route-1',
      source_planet_id: 'planet-eris',
      destination: { type: 'planet', id: 'planet-nova' },
      resource_item_id: 'ferrite_ore',
      amount_per_hour: 10,
      energy_cost_per_hour: 2,
      enabled: false,
      risk: { loss_chance: 0.1, min_loss_percent: 1, max_loss_percent: 3 },
      last_calculated_at: 1000,
      updated_at: 1000,
    };
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
    expect(withRoute.routes?.routes[0]).toMatchObject({ route_id: 'route-1', amount_per_hour: 25, enabled: true });
    expect(withRoute.planetIntel?.selectedPlanet?.routes).toHaveLength(1);
    expect(withRoute.planetIntel?.selectedPlanet?.routes[0]).toEqual(withRoute.routes?.routes[0]);

    const withEventRoute = reduceClientState(withRoute, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.routeSnapshot,
        {
          route: {
            ...updatedRoute,
            amount_per_hour: 30,
            updated_at: 1300,
          },
        },
        3,
      ),
    });

    expect(withEventRoute.routes?.routes).toHaveLength(1);
    expect(withEventRoute.routes?.routes[0]).toMatchObject({ route_id: 'route-1', amount_per_hour: 30, enabled: true });
    expect(withEventRoute.planetIntel?.selectedPlanet?.routes[0]).toEqual(withEventRoute.routes?.routes[0]);
  });
});

import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, EventEnvelope, JsonObject, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import type { ClientState } from './types';
import { isWithinMinimapProjectionWindow, worldMapMemoryMarkers } from './world-memory';

describe('reduceClientState', () => {
  test('initial state has no fake gameplay values', () => {
    const state = createInitialState();

    expect(state.connectionStatus).toBe('restoring');
    expectServerOwnedGameplayCleared(state);
  });

  test('logout and auth expiry clear gameplay state', () => {
    const withGameplay = stateWithServerOwnedGameplay();
    expect(withGameplay.playerSnapshot).not.toBeNull();
    expect(withGameplay.wallet).not.toBeNull();
    expect(Object.keys(withGameplay.visibleEntities)).toEqual(['npc-1']);

    const loggedOut = reduceClientState(withGameplay, { type: 'authLoggedOut' });
    expect(loggedOut.connectionStatus).toBe('logged_out');
    expectServerOwnedGameplayCleared(loggedOut);

    const expired = reduceClientState(withGameplay, { type: 'authExpired', message: 'Session expired.' });
    expect(expired.connectionStatus).toBe('auth_expired');
    expectServerOwnedGameplayCleared(expired);
    expect(expired.auth.error).toBe('Session expired.');
  });

  test('loaded real session clears stale gameplay and waits for session.ready before connected', () => {
    const loaded = reduceClientState(stateWithServerOwnedGameplay(), {
      type: 'authSessionLoaded',
      session: {
        authenticated: true,
        account: { email: 'pilot@example.com', admin: false },
        player: { callsign: 'Pilot' },
        server_time: 2000,
      },
    });

    expect(loaded.connectionStatus).toBe('authenticated_pending_socket');
    expect(loaded.pendingCommands).toEqual({});
    expect(loaded.playerSnapshot).toBeNull();
    expect(loaded.visibleEntities).toEqual({});
    expect(loaded.wallet).toBeNull();
    expect(loaded.inventory).toBeNull();
    expect(loaded.market).toBeNull();
    expect(loaded.auth.session?.authenticated).toBe(true);
    expect(loaded.lastServerTime).toBe(2000);

    const ready = reduceClientState(loaded, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.sessionReady, { authenticated: true }, 1),
    });

    expect(ready.connectionStatus).toBe('connected');
    expect(ready.auth.session?.authenticated).toBe(true);
  });

  test('demo mode is explicit and isolated from real auth session state', () => {
    const demo = reduceClientState(stateWithServerOwnedGameplay(), { type: 'demoModeStarted' });

    expect(demo.auth.mode).toBe('demo');
    expect(demo.auth.session).toBeNull();
    expect(demo.connectionStatus).toBe('offline');
    expectServerOwnedGameplayCleared(demo);
    expect(demo.commandLog.some((line) => line.text.includes('Demo mode'))).toBe(true);
  });

  test('AOI diff events reconcile minimap live contacts', () => {
    const state = {
      ...createInitialState(),
      minimap: { radar_range: 1000, projection_window_size: 2000, live_contacts: [], remembered: [] },
    };
    const entered = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
        display: { label: 'Training Drone', disposition: 'hostile' },
        status_flags: ['hostile', 'hidden', 'scan_revealed', 'stealthed'],
        projection_source: 'worker_projection',
      }),
    });

    expect(entered.visibleEntities['npc-1']).toMatchObject({
      entity_id: 'npc-1',
      position: { x: 10, y: 20 },
    });
    expect(entered.minimap?.live_contacts).toEqual([
      {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
        disposition: 'hostile',
        status_flags: ['hostile', 'scan_revealed'],
        projection_source: 'worker_projection',
      },
    ]);
    expect(entered.visibleEntities['npc-1'].status_flags).toEqual(['hostile', 'scan_revealed']);

    const selfState = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'player-local',
        entity_type: 'player',
        position: { x: 0, y: 0 },
        display: { label: 'Pilot', disposition: 'self' },
        status_flags: ['self', 'hidden', 'stealthed'],
      }),
    });
    expect(selfState.visibleEntities['player-local'].status_flags).toEqual(['self', 'stealthed']);

    const updated = reduceClientState(entered, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityUpdated, {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 15, y: 25 },
        display: { label: 'Training Drone', disposition: 'hostile' },
      }, 2),
    });
    expect(updated.minimap?.live_contacts[0]).toMatchObject({
      entity_id: 'npc-1',
      position: { x: 15, y: 25 },
    });

    const left = reduceClientState(updated, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityLeft, { entity_id: 'npc-1' }, 3),
    });

    expect(left.visibleEntities['npc-1']).toBeUndefined();
    expect(left.minimap?.live_contacts).toEqual([]);
  });

  test('server correction updates authoritative entity position and clears local target', () => {
    const state = reduceClientState({
      ...createInitialState(),
      minimap: { radar_range: 1000, projection_window_size: 2000, live_contacts: [], remembered: [] },
    }, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'player-1',
        entity_type: 'player',
        position: { x: 0, y: 0 },
      }),
    });

    const corrected = reduceClientState(
      {
        ...state,
        movementTarget: { x: 100, y: 100 },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.positionCorrected, {
          entity_id: 'player-1',
          position: { x: 12, y: 16 },
        }, 2),
      },
    );

    expect(corrected.visibleEntities['player-1'].position).toEqual({ x: 12, y: 16 });
    expect(corrected.minimap?.live_contacts[0].position).toEqual({ x: 12, y: 16 });
    expect(corrected.movementTarget).toBeNull();
    expect(corrected.lastCorrection).toEqual({ entityID: 'player-1', position: { x: 12, y: 16 } });
  });

  test('server correction preserves authoritative movement route target', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'player-1',
        entity_type: 'player',
        position: { x: 0, y: 0 },
        status_flags: ['self'],
      }),
    });

    const corrected = reduceClientState(
      {
        ...state,
        movementTarget: { x: 100, y: 100 },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.positionCorrected, {
          entity_id: 'player-1',
          position: { x: 12, y: 16 },
          movement: {
            moving: true,
            origin: { x: 9, y: 12 },
            target: { x: 80, y: 120 },
            speed: 180,
            started_at_ms: 1000,
            arrive_at_ms: 1600,
          },
        }, 2),
      },
    );

    expect(corrected.visibleEntities['player-1'].movement).toMatchObject({
      origin: { x: 9, y: 12 },
      target: { x: 80, y: 120 },
      speed: 180,
    });
    expect(corrected.movementTarget).toEqual({ x: 80, y: 120 });
  });

  test('rejects hidden debug payloads before state mutation', () => {
    const state = createInitialState();

    expect(() =>
      reduceClientState(state, {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.entityEntered, {
          entity_id: 'planet-1',
          entity_type: 'planet_signal',
          position: { x: 4, y: 8 },
          internal_metadata: { seed: 'nope' },
        }),
      }),
    ).toThrow(/Forbidden server payload rejected/);
  });

  test('ignores duplicate or stale realtime events by sequence', () => {
    const entered = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.entityEntered,
        {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 10, y: 20 },
          display: { label: 'Training Drone', disposition: 'hostile' },
        },
        5,
      ),
    });

    const staleUpdate = reduceClientState(entered, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.entityUpdated,
        {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 999, y: 999 },
          display: { label: 'Stale Drone', disposition: 'hostile' },
        },
        4,
      ),
    });
    const duplicateLeave = reduceClientState(staleUpdate, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityLeft, { entity_id: 'npc-1' }, 5),
    });
    const freshUpdate = reduceClientState(duplicateLeave, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.entityUpdated,
        {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 30, y: 40 },
          display: { label: 'Fresh Drone', disposition: 'hostile' },
        },
        6,
      ),
    });

    expect(entered.lastSequence).toBe(5);
    expect(staleUpdate.visibleEntities['npc-1'].position).toEqual({ x: 10, y: 20 });
    expect(duplicateLeave.visibleEntities['npc-1']).toBeDefined();
    expect(freshUpdate.visibleEntities['npc-1'].position).toEqual({ x: 30, y: 40 });
    expect(freshUpdate.lastSequence).toBe(6);
  });

  test('logs unknown realtime events without mutating gameplay state', () => {
    const base = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.entityEntered,
        {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 10, y: 20 },
          display: { label: 'Training Drone', disposition: 'hostile' },
        },
        2,
      ),
    });

    const unknown = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event('server.experimental_future', { message: 'future event' }, 3),
    });

    expect(unknown.visibleEntities).toEqual(base.visibleEntities);
    expect(unknown.selectedTargetID).toBe(base.selectedTargetID);
    expect(unknown.lastSequence).toBe(3);
    expect(unknown.commandLog.some((line) => line.text === 'Unhandled event server.experimental_future.')).toBe(true);
  });

  test('new realtime stream statuses reset event sequence cursor', () => {
    const staleState = {
      ...createInitialState(),
      connectionStatus: 'connected' as const,
      lastSequence: 42,
    };
    const reconnecting = reduceClientState(staleState, {
      type: 'connectionChanged',
      status: 'reconnecting',
    });
    const ready = reduceClientState(reconnecting, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.sessionReady, { authenticated: true }, 1),
    });

    expect(reconnecting.lastSequence).toBe(0);
    expect(ready.lastSequence).toBe(1);
    expect(ready.connectionStatus).toBe('connected');
    expect(ready.auth.session?.authenticated).toBe(true);
  });

  test('reconnect bootstrap accepts fresh world snapshot and replaces stale visible state', () => {
    const staleState = {
      ...createInitialState(),
      auth: {
        mode: 'real' as const,
        session: {
          authenticated: true,
          account: { email: 'pilot@example.com', admin: false },
          player: { callsign: 'Pilot' },
          server_time: 1000,
        },
        submitting: false,
        error: null,
      },
      connectionStatus: 'connected' as const,
      lastSequence: 42,
      visibleEntities: {
        'stale-npc': {
          entity_id: 'stale-npc',
          entity_type: 'npc' as const,
          position: { x: 999, y: 999 },
        },
      },
      selectedTargetID: 'stale-npc',
      movementTarget: { x: 900, y: 900 },
    };
    const reconnecting = reduceClientState(staleState, {
      type: 'connectionChanged',
      status: 'reconnecting',
    });
    const bootstrapped = reduceClientState(reconnecting, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.worldSnapshot,
        {
          sector: { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
          entities: [
            {
              entity_id: 'player-local',
              entity_type: 'player',
              position: { x: 10, y: 20 },
              status_flags: ['self'],
            },
          ],
          minimap: { radar_range: 420, live_contacts: [], remembered: [] },
        },
        1,
      ),
    });

    expect(reconnecting.lastSequence).toBe(0);
    expect(bootstrapped.connectionStatus).toBe('connected');
    expect(bootstrapped.lastSequence).toBe(1);
    expect(bootstrapped.visibleEntities['stale-npc']).toBeUndefined();
    expect(bootstrapped.visibleEntities['player-local'].position).toEqual({ x: 10, y: 20 });
    expect(bootstrapped.selectedTargetID).toBeNull();
    expect(bootstrapped.movementTarget).toBeNull();
  });

  test('request and response flow tracks pending commands', () => {
    const state = createInitialState();
    const queued = reduceClientState(state, {
      type: 'requestQueued',
      envelope: {
        request_id: 'request-1',
        op: 'move_to',
        payload: { target: { x: 1, y: 2 } },
        client_seq: 1,
        v: 1,
      },
    });

    expect(queued.pendingCommands['request-1']).toBeDefined();
    expect(queued.movementTarget).toEqual({ x: 1, y: 2 });

    const accepted = reduceClientState(queued, {
      type: 'responseReceived',
      envelope: {
        request_id: 'request-1',
        ok: true,
        payload: {},
        server_time: 99,
        v: 1,
      },
    });

    expect(accepted.pendingCommands['request-1']).toBeUndefined();
    expect(accepted.lastServerTime).toBe(99);
  });

  test('authoritative realtime events clear matching pending commands when responses are lost', () => {
    const withSelf = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.entityEntered,
        {
          entity_id: 'player-local',
          entity_type: 'player',
          position: { x: 0, y: 0 },
          status_flags: ['self'],
        },
        1,
      ),
    });
    const queuedMove = reduceClientState(withSelf, {
      type: 'requestQueued',
      envelope: {
        request_id: 'move-1',
        op: OPERATIONS.moveTo,
        payload: { target: { x: 100, y: 0 } },
        client_seq: 1,
        v: 1,
      },
    });
    const corrected = reduceClientState(queuedMove, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.positionCorrected,
        {
          entity_id: 'player-local',
          position: { x: 12, y: 0 },
          movement: {
            moving: true,
            origin: { x: 12, y: 0 },
            target: { x: 100, y: 0 },
            speed: 180,
            started_at_ms: 1000,
            arrive_at_ms: 1489,
          },
        },
        2,
      ),
    });

    expect(corrected.pendingCommands['move-1']).toBeUndefined();
    expect(corrected.movementTarget).toEqual({ x: 100, y: 0 });

    const queuedActions = {
      ...corrected,
      pendingCommands: {
        'scan-1': { requestID: 'scan-1', op: OPERATIONS.scanPulse, queuedAt: 1 },
        'loot-1': { requestID: 'loot-1', op: OPERATIONS.lootPickup, queuedAt: 1 },
        'combat-1': { requestID: 'combat-1', op: OPERATIONS.combatUseSkill, queuedAt: 1 },
        'auction-1': { requestID: 'auction-1', op: OPERATIONS.auctionBid, queuedAt: 1 },
        'quote-1': { requestID: 'quote-1', op: OPERATIONS.deathRepairQuote, queuedAt: 1 },
      },
    };
    const scanStarted = reduceClientState(queuedActions, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.scanPulseStarted, { pulse_reference: 'pulse-1', status: 'started' }, 3),
    });
    const lootPickedUp = reduceClientState(scanStarted, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.lootPickedUp, { drop_id: 'drop-1', item_id: 'raw_ore', quantity: 2 }, 4),
    });
    const combatCooldown = reduceClientState(lootPickedUp, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.combatCooldownStarted, { skill_id: 'basic_laser', target_id: 'npc-1' }, 5),
    });
    const auctionBidPlaced = reduceClientState(combatCooldown, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.auctionBidPlaced, { auction_id: 'auction-1' }, 6),
    });

    expect(auctionBidPlaced.pendingCommands['scan-1']).toBeUndefined();
    expect(auctionBidPlaced.pendingCommands['loot-1']).toBeUndefined();
    expect(auctionBidPlaced.pendingCommands['combat-1']).toBeUndefined();
    expect(auctionBidPlaced.pendingCommands['auction-1']).toBeUndefined();
    expect(auctionBidPlaced.pendingCommands['quote-1']).toBeDefined();
  });

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

  test('snapshot response replaces visible entities atomically', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'stale-npc',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
      }),
    });

    const replaced = reduceClientState(
      {
        ...state,
        selectedTargetID: 'stale-npc',
        movementTarget: { x: 100, y: 100 },
        lastCorrection: { entityID: 'stale-npc', position: { x: 10, y: 20 } },
      },
      {
        type: 'responseReceived',
        envelope: {
          request_id: 'snapshot-1',
          ok: true,
          payload: {
            entities: [
              {
                entity_id: 'signal-1',
                entity_type: 'planet_signal',
                position: { x: 50, y: 60 },
                status_flags: ['known_intel'],
                display: { label: 'Unknown Signal', disposition: 'unknown' },
              },
            ],
            sector: { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
            minimap: {
              radar_range: 420,
              live_contacts: [
                {
                  entity_id: 'signal-1',
                  entity_type: 'planet_signal',
                  position: { x: 50, y: 60 },
                  disposition: 'unknown',
                  status_flags: ['known_intel'],
                },
              ],
              remembered: [],
            },
          },
          server_time: 1200,
          v: 1,
        },
      },
    );

    expect(replaced.visibleEntities['stale-npc']).toBeUndefined();
    expect(replaced.visibleEntities['signal-1']).toMatchObject({
      entity_type: 'planet_signal',
      position: { x: 50, y: 60 },
    });
    expect(replaced.sector).toMatchObject({ name: 'Origin Fringe', danger: 'low' });
    expect(replaced.minimap?.live_contacts).toHaveLength(1);
    expect(replaced.selectedTargetID).toBeNull();
    expect(replaced.movementTarget).toBeNull();
    expect(replaced.lastCorrection).toBeNull();
    expect(replaced.planetIntel?.knownSignals).toBe(1);
  });

  test('snapshot response without minimap rebuilds live contacts from replacement entities', () => {
    const state = {
      ...createInitialState(),
      selectedTargetID: 'stale-npc',
      minimap: {
        radar_range: 1000,
        projection_window_size: 2000,
        live_contacts: [
          {
            entity_id: 'stale-npc',
            entity_type: 'npc' as const,
            position: { x: 10, y: 20 },
            disposition: 'hostile',
            status_flags: ['hostile'],
          },
        ],
        remembered: [
          {
            kind: 'known_planet',
            planet_id: 'planet-eris',
            detail_id: 'planet-eris',
            label: 'Eris',
            position: { x: 500, y: -250 },
            freshness: 'fresh',
          },
        ],
      },
    };

    const replaced = reduceClientState(state, {
      type: 'responseReceived',
      envelope: {
        request_id: 'move-accepted',
        ok: true,
        payload: {
          accepted: true,
          entities: [
            {
              entity_id: 'player-local',
              entity_type: 'player',
              position: { x: 2, y: 4 },
              status_flags: ['self', 'hidden', 'stealthed'],
              display: { label: 'Pilot', disposition: 'self' },
              projection_source: 'worker_projection',
            },
            {
              entity_id: 'loot-1',
              entity_type: 'loot',
              position: { x: 30, y: 40 },
              status_flags: ['loot', 'hidden'],
              display: { label: 'Cache', disposition: 'neutral' },
              projection_source: 'worker_projection',
            },
          ],
        },
        server_time: 1500,
        v: 1,
      },
    });

    expect(replaced.visibleEntities['stale-npc']).toBeUndefined();
    expect(replaced.selectedTargetID).toBeNull();
    expect(replaced.minimap?.radar_range).toBe(1000);
    expect(replaced.minimap?.projection_window_size).toBe(2000);
    expect(replaced.minimap?.remembered).toEqual(state.minimap.remembered);
    expect(replaced.minimap?.live_contacts).toEqual([
      {
        entity_id: 'loot-1',
        entity_type: 'loot',
        position: { x: 30, y: 40 },
        disposition: 'neutral',
        status_flags: ['loot'],
        projection_source: 'worker_projection',
      },
      {
        entity_id: 'player-local',
        entity_type: 'player',
        position: { x: 2, y: 4 },
        disposition: 'self',
        status_flags: ['self', 'stealthed'],
        projection_source: 'worker_projection',
      },
    ]);
  });

  test('snapshot response rejects hidden debug payloads before state mutation', () => {
    const state = createInitialState();

    expect(() =>
      reduceClientState(state, {
        type: 'responseReceived',
        envelope: {
          request_id: 'snapshot-hidden',
          ok: true,
          payload: {
            entities: [
              {
                entity_id: 'planet-1',
                entity_type: 'planet_signal',
                position: { x: 4, y: 8 },
                internal_metadata: { seed: 'nope' },
              },
            ],
          },
          server_time: 1200,
          v: 1,
        },
      }),
    ).toThrow(/Forbidden server payload rejected/);
  });

  test('snapshot response reconciles player, cargo, wallet, ship, progression, inventory, hangar, loadout, crafting, and stat panels', () => {
    const reconciled = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'snapshot-panels',
        ok: true,
        payload: {
          player: { callsign: 'Server-Pilot', hp: 77, shield: 44, energy: 33, rank: 2 },
          cargo: {
            used: 4,
            capacity: 80,
            items: [{
              item_id: 'raw_ore',
              display_name: 'Raw Ore',
              category: 'resource',
              art_key: 'item.raw_ore',
              rarity: 'common',
              quantity: 4,
              unit_weight: 2,
              used_units: 8,
              location: 'ship_cargo',
              move_eligible: false,
              locked_reason: 'cargo_transfer_unavailable',
            }],
          },
          wallet: { credits: 980, premium_paid: 3, premium_earned: 9 },
          ship: {
            active_ship_id: 'starter',
            display_name: 'Starter Hull',
            hull: 88,
            max_hull: 120,
            shield: 42,
            max_shield: 60,
            capacitor: 31,
            max_capacitor: 50,
            disabled: false,
            repair_state: 'active',
          },
          progression: { main_level: 2, main_xp: 175, rank: 2, combat_level: 1, combat_xp: 25 },
          inventory: {
            stackable: [{ item_id: 'raw_ore', display_name: 'Raw Ore', quantity: 3, location: 'ship_cargo' }],
            instances: [],
            counts: { cargo_stacks: 1, storage_stacks: 0, equipped_instances: 0 },
          },
          hangar: {
            active_ship_id: 'starter',
            ships: [
              {
                ship_id: 'starter',
                display_name: 'Sparrow',
                state: 'ready',
                hull: 88,
                max_hull: 120,
                shield: 42,
                max_shield: 60,
                disabled: false,
              },
            ],
          },
          loadout: {
            active_ship_id: 'starter',
            slots: [
              { slot_id: 'offensive_1', slot_type: 'offensive' },
              { slot_id: 'defensive_1', slot_type: 'defensive' },
            ],
          },
          crafting: {
            recipes: [
              {
                recipe_id: 'refined_alloy_batch',
                category: 'processed_material',
                output: { kind: 'item', item_id: 'refined_alloy', quantity: 5, tradeable: true },
                inputs: [{ item_id: 'raw_ore', quantity: 20 }],
                required_credits: 100,
                required_rank: 1,
                required_role_levels: [{ role: 'crafting', level: 1 }],
                required_location_type: 'station',
                craft_duration_ms: 300000,
                repeatable: true,
              },
            ],
            active_jobs: [],
          },
          stats: {
            speed: 220,
            radar_range: 510,
            weapon_range: 280,
            cargo_capacity: 80,
            loot_pickup_range: 120,
            basic_laser_energy_cost: 10,
            basic_laser_cooldown_ms: 350,
          },
        },
        server_time: 1400,
        v: 1,
      },
    });

    expect(reconciled.playerSnapshot?.callsign).toBe('Server-Pilot');
    expect(reconciled.cargo).toMatchObject({ used: 4, capacity: 80 });
    expect(reconciled.cargo?.items).toEqual([{
      item_id: 'raw_ore',
      display_name: 'Raw Ore',
      category: 'resource',
      art_key: 'item.raw_ore',
      rarity: 'common',
      quantity: 4,
      unit_weight: 2,
      used_units: 8,
      location: 'ship_cargo',
      move_eligible: false,
      locked_reason: 'cargo_transfer_unavailable',
    }]);
    expect(reconciled.wallet).toEqual({ credits: 980, premium_paid: 3, premium_earned: 9 });
    expect(reconciled.ship).toMatchObject({ active_ship_id: 'starter', hull: 88, capacitor: 31, disabled: false });
    expect(reconciled.playerSnapshot).toMatchObject({ hp: 88, max_hp: 120, shield: 42, energy: 31 });
    expect(reconciled.progression).toMatchObject({ main_level: 2, main_xp: 175, rank: 2, combat_xp: 25 });
    expect(reconciled.inventory?.stackable).toEqual([
      { item_id: 'raw_ore', display_name: 'Raw Ore', quantity: 3, location: 'ship_cargo' },
    ]);
    expect(reconciled.hangar?.active_ship_id).toBe('starter');
    expect(reconciled.loadout?.slots).toHaveLength(2);
    expect(reconciled.crafting?.recipes[0]).toMatchObject({ recipe_id: 'refined_alloy_batch', craft_duration_ms: 300000 });
    expect(reconciled.stats).toMatchObject({
      speed: 220,
      radar_range: 510,
      weapon_range: 280,
      cargo_capacity: 80,
      loot_pickup_range: 120,
      basic_laser_energy_cost: 10,
      basic_laser_cooldown_ms: 350,
    });
  });

  test('world snapshot event stores sector and minimap projection', () => {
    const state = reduceClientState(
      {
        ...createInitialState(),
        auth: {
          mode: 'real',
          session: { authenticated: true, server_time: 1 },
          submitting: false,
          error: null,
        },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.worldSnapshot, {
          sector: { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
          entities: [
            {
              entity_id: 'player-local',
              entity_type: 'player',
              position: { x: 0, y: 0 },
              status_flags: ['self'],
              display: { label: 'Smoke', disposition: 'self' },
              movement: {
                moving: true,
                origin: { x: 0, y: 0 },
                target: { x: 100, y: 0 },
                speed: 180,
                started_at_ms: 1000,
                arrive_at_ms: 1556,
              },
            },
          ],
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
                label: 'terran / ice',
                position: { x: 500, y: -250 },
                freshness: 'fresh',
              },
            ],
          },
        }),
      },
    );

    expect(state.connectionStatus).toBe('connected');
    expect(state.sector).toEqual({ name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false });
    expect(state.minimap?.radar_range).toBe(1000);
    expect(state.minimap?.projection_window_size).toBe(2000);
    expect(state.minimap?.remembered[0]).toMatchObject({ planet_id: 'planet-eris', detail_id: 'planet-eris' });
    expect(state.visibleEntities['player-local'].status_flags).toContain('self');
    expect(state.visibleEntities['player-local'].movement?.target).toEqual({ x: 100, y: 0 });
    expect(state.movementTarget).toEqual({ x: 100, y: 0 });
  });

  test('failed move response clears speculative target marker back to authoritative movement', () => {
    const state = reduceClientState(
      {
        ...createInitialState(),
        movementTarget: { x: 9000, y: 0 },
        pendingCommands: { 'move-request': { requestID: 'move-request', op: 'move_to', queuedAt: 1 } },
        visibleEntities: {
          'player-local': {
            entity_id: 'player-local',
            entity_type: 'player',
            position: { x: 0, y: 0 },
            status_flags: ['self'],
            movement: {
              moving: true,
              origin: { x: 0, y: 0 },
              target: { x: 100, y: 0 },
              speed: 180,
              started_at_ms: 1000,
              arrive_at_ms: 1556,
            },
          },
        },
      },
      {
        type: 'responseReceived',
        envelope: {
          request_id: 'move-request',
          ok: false,
          error: { code: 'ERR_OUT_OF_RANGE', message: 'Move target is out of range.', retryable: false },
          server_time: 1002,
          v: 1,
        },
      },
    );

    expect(state.pendingCommands['move-request']).toBeUndefined();
    expect(state.movementTarget).toEqual({ x: 100, y: 0 });
  });

  test('rejects invalid entity movement timing', () => {
    expect(() =>
      reduceClientState(createInitialState(), {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.entityEntered, {
          entity_id: 'player-1',
          entity_type: 'player',
          position: { x: 0, y: 0 },
          movement: {
            moving: true,
            origin: { x: 0, y: 0 },
            target: { x: 100, y: 0 },
            speed: 0,
            started_at_ms: 2000,
            arrive_at_ms: 1000,
          },
        }),
      }),
    ).toThrow(/Invalid entity movement/);
  });

  test('snapshot events reconcile cargo, wallet, stats, inventory, hangar, loadout, and crafting independently', () => {
    const withCargo = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.cargoSnapshot, {
        used: 12,
        capacity: 70,
        items: [{
          item_id: 'salvage_thread',
          display_name: 'Salvage Thread',
          category: 'resource',
          art_key: 'item.salvage_thread',
          quantity: 12,
          unit_weight: 1,
          used_units: 12,
          location: 'ship_cargo',
          move_eligible: false,
          locked_reason: 'cargo_transfer_unavailable',
        }],
      }),
    });
    const withWallet = reduceClientState(withCargo, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.walletSnapshot, { credits: 444, premium_paid: 1, premium_earned: 2 }, 2),
    });
    const withStats = reduceClientState(withWallet, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.statsSnapshot, { speed: 210, radar_range: 500, weapon_range: 275, cargo_capacity: 70 }, 3),
    });
    const withInventory = reduceClientState(withStats, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.inventorySnapshot, {
        stackable: [{ item_id: 'raw_ore', quantity: 3, location: 'ship_cargo' }],
        instances: [],
        counts: { cargo_stacks: 1, storage_stacks: 0, equipped_instances: 0 },
      }, 4),
    });
    const withHangar = reduceClientState(withInventory, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.hangarSnapshot, {
        active_ship_id: 'starter',
        ships: [{ ship_id: 'starter', display_name: 'Sparrow', state: 'ready', hull: 100, max_hull: 100, shield: 100, max_shield: 100, disabled: false }],
      }, 5),
    });
    const withLoadout = reduceClientState(withHangar, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.loadoutSnapshot, {
        active_ship_id: 'starter',
        slots: [{ slot_id: 'offensive_1', slot_type: 'offensive' }],
      }, 6),
    });
    const withCrafting = reduceClientState(withLoadout, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.craftingRecipes, {
        recipes: [
          {
            recipe_id: 'refined_alloy_batch',
            category: 'processed_material',
            output: { kind: 'item', item_id: 'refined_alloy', quantity: 5, tradeable: true },
            inputs: [{ item_id: 'raw_ore', quantity: 20 }],
            required_credits: 100,
            required_rank: 1,
            required_role_levels: [{ role: 'crafting', level: 1 }],
            required_location_type: 'station',
            craft_duration_ms: 300000,
            repeatable: true,
          },
        ],
        active_jobs: [],
      }, 7),
    });

    expect(withCrafting.cargo?.items).toEqual([{
      item_id: 'salvage_thread',
      display_name: 'Salvage Thread',
      category: 'resource',
      art_key: 'item.salvage_thread',
      quantity: 12,
      unit_weight: 1,
      used_units: 12,
      location: 'ship_cargo',
      move_eligible: false,
      locked_reason: 'cargo_transfer_unavailable',
    }]);
    expect(withCrafting.wallet?.credits).toBe(444);
    expect(withCrafting.stats?.weapon_range).toBe(275);
    expect(withCrafting.inventory?.counts.cargo_stacks).toBe(1);
    expect(withCrafting.hangar?.ships[0].display_name).toBe('Sparrow');
    expect(withCrafting.loadout?.slots[0].slot_type).toBe('offensive');
    expect(withCrafting.crafting?.recipes[0].recipe_id).toBe('refined_alloy_batch');
  });

  test('shop catalog response stores server-owned categories and products', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'request-shop-catalog',
        ok: true,
        payload: {
          shop: {
            catalog_version: 'content_registry_task001_v1',
            categories: [
              { category_id: 'ships', display_name: 'Ships', sort_order: 10 },
              { category_id: 'weapons', display_name: 'Weapons', sort_order: 20 },
            ],
            products: [
              {
                product_id: 'product_module_laser_alpha_t1',
                product_type: 'module',
                display_name: 'Prism Lance I',
                description: 'Entry laser array.',
                category_id: 'weapons',
                subcategory: 'Laser',
                art_key: 'module.prism_lance_1',
                rarity: 'common',
                tier: 1,
                sort_order: 20,
                grant_target: { kind: 'module', ref_id: 'laser_alpha_t1', quantity: 1 },
                price: { currency_type: 'credits', amount: 450, fixed: true },
                stock: { kind: 'unlimited' },
                availability: { available: false, locked_reason: 'Module purchase unavailable in this playtest.' },
              },
            ],
          },
        },
        server_time: 99,
        v: 1,
      },
    });

    expect(state.shopCatalog?.catalog_version).toBe('content_registry_task001_v1');
    expect(state.shopCatalog?.categories.map((category) => category.display_name)).toEqual(['Ships', 'Weapons']);
    expect(state.shopCatalog?.products[0]).toMatchObject({
      product_id: 'product_module_laser_alpha_t1',
      display_name: 'Prism Lance I',
      category_id: 'weapons',
      price: { amount: 450, currency_type: 'credits' },
      availability: { available: false },
    });
  });

  test('phase 05 combat, loot, progression, and repair events reconcile server-owned state', () => {
    const withPlayer = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'player-1',
        entity_type: 'player',
        position: { x: 0, y: 0 },
        status_flags: ['self'],
      }),
    });
    const withNPC = reduceClientState(withPlayer, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 80, y: 0 },
        display: { label: 'Training Drone', disposition: 'hostile' },
        combat: { hp: 40, max_hp: 40, shield: 10, max_shield: 10, status: 'active' },
      }, 2),
    });
    const targeted = reduceClientState({ ...withNPC, selectedTargetID: 'npc-1' }, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.targetUpdated, {
        entity_id: 'npc-1',
        combat: { hp: 0, max_hp: 40, shield: 0, max_shield: 10, status: 'destroyed' },
      }, 3),
    });
    const withCooldown = reduceClientState(targeted, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.combatCooldownStarted, {
        skill_id: 'basic_laser',
        target_id: 'npc-1',
        cooldown_ready_at_ms: 9000,
      }, 4),
    });
    const withDuplicateCooldown = reduceClientState(withCooldown, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.combatCooldownStarted, {
        skill_id: 'basic_laser',
        target_id: 'npc-1',
        cooldown_ready_at_ms: 9000,
      }, 4),
    });
    const withDamage = reduceClientState(withCooldown, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.combatDamage, {
        target_id: 'npc-1',
        amount: 45,
      }, 5),
    });
    const withDropNotice = reduceClientState(withDamage, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.lootCreated, {
        drop_id: 'drop-1',
        item_id: 'raw_ore',
        quantity: 3,
        state: 'active',
        position: { x: 80, y: 0 },
      }, 6),
    });
    const withLootEntity = reduceClientState(withDropNotice, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'drop-1',
        entity_type: 'loot',
        position: { x: 80, y: 0 },
        display: { label: 'Raw Ore', disposition: 'neutral' },
      }, 7),
    });
    const leftClearsKnownLoot = reduceClientState(withLootEntity, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityLeft, { entity_id: 'drop-1' }, 8),
    });
    const afterPickup = reduceClientState(
      {
        ...withLootEntity,
        selectedTargetID: 'drop-1',
      },
      {
        type: 'responseReceived',
        envelope: {
          request_id: 'pickup-1',
          ok: true,
          payload: {
            cargo: { used: 6, capacity: 60, items: [{ item_id: 'raw_ore', quantity: 3 }] },
          },
          server_time: 1006,
          v: 1,
        },
      },
    );
    const withPickupNotice = reduceClientState(afterPickup, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.lootPickedUp, { drop_id: 'drop-1', item_id: 'raw_ore', quantity: 3 }, 8),
    });
    const withoutLootEntity = reduceClientState(withPickupNotice, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.lootRemoved, { entity_id: 'drop-1' }, 9),
    });
    const progressed = reduceClientState(withoutLootEntity, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.progressionSnapshot, { main_level: 2, main_xp: 100, rank: 2, combat_xp: 40 }, 10),
    });
    const quoted = reduceClientState(progressed, {
      type: 'responseReceived',
      envelope: {
        request_id: 'quote-1',
        ok: true,
        payload: { ship_id: 'starter', cost: 0, currency: 'credits', disabled: true },
        server_time: 1009,
        v: 1,
      },
    });
    const repaired = reduceClientState(quoted, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.deathRepaired, { ship_id: 'starter' }, 11),
    });

    expect(targeted.visibleEntities['npc-1'].combat).toMatchObject({ hp: 0, shield: 0, status: 'destroyed' });
    expect(withCooldown.skillCooldowns.basic_laser).toBe(9000);
    expect(withCooldown.worldEffects).toContainEqual(
      expect.objectContaining({
        id: 'event-4:laser',
        kind: 'laser',
        sourceID: 'player-1',
        sourcePosition: { x: 0, y: 0 },
        targetID: 'npc-1',
        position: { x: 80, y: 0 },
      }),
    );
    expect(withDuplicateCooldown.worldEffects.filter((effect) => effect.id === 'event-4:laser')).toHaveLength(1);
    expect(withDamage.combatLog.at(-1)?.text).toContain('Hit Training Drone for 45.');
    expect(withDamage.worldEffects.some((effect) => effect.kind === 'damage' && effect.amount === 45)).toBe(true);
    expect(withDropNotice.knownLoot['drop-1']).toMatchObject({ item_id: 'raw_ore', quantity: 3 });
    expect(withDropNotice.worldEffects.some((effect) => effect.kind === 'loot_spawn' && effect.targetID === 'drop-1')).toBe(true);
    expect(leftClearsKnownLoot.knownLoot['drop-1']).toBeUndefined();
    expect(afterPickup.cargo?.items).toEqual([{ item_id: 'raw_ore', quantity: 3 }]);
    expect(withPickupNotice.worldEffects.some((effect) => effect.kind === 'loot_pickup' && effect.itemID === 'raw_ore')).toBe(true);
    expect(withoutLootEntity.visibleEntities['drop-1']).toBeUndefined();
    expect(withoutLootEntity.knownLoot['drop-1']).toBeUndefined();
    expect(withoutLootEntity.selectedTargetID).toBeNull();
    expect(progressed.progression).toMatchObject({ main_level: 2, rank: 2, combat_xp: 40 });
    expect(quoted.repairQuote).toEqual({ ship_id: 'starter', cost: 0, currency: 'credits', disabled: true });
    expect(repaired.repairQuote).toBeNull();
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

  test('death disabled event marks active ship disabled without inventing a repair quote', () => {
    const withShip = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'ship-snapshot',
        ok: true,
        payload: {
          ship: {
            active_ship_id: 'starter',
            display_name: 'Sparrow',
            hull: 44,
            max_hull: 100,
            shield: 12,
            max_shield: 60,
            capacitor: 20,
            max_capacitor: 40,
            disabled: false,
            repair_state: 'ready',
          },
        },
        server_time: 1000,
        v: 1,
      },
    });

    const disabled = reduceClientState(withShip, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.deathShipDisabled,
        {
          ship_id: 'starter',
          disabled_reason: 'combat',
          ship: {
            active_ship_id: 'starter',
            display_name: 'Sparrow',
            hull: 0,
            max_hull: 100,
            shield: 0,
            max_shield: 60,
            capacitor: 0,
            max_capacitor: 40,
            disabled: true,
            repair_state: 'disabled',
          },
          repair_quote: { ship_id: 'starter', cost: 15, currency: 'credits', disabled: true },
          respawn_location_id: 'origin-station',
        },
        2,
      ),
    });

    expect(disabled.ship).toMatchObject({
      active_ship_id: 'starter',
      disabled: true,
      repair_state: 'combat',
      hull: 0,
    });
    expect(disabled.repairQuote).toEqual({ ship_id: 'starter', cost: 15, currency: 'credits', disabled: true });
    expect(disabled.combatLog.at(-1)?.text).toBe('Ship disabled.');

    const quoted = reduceClientState(disabled, {
      type: 'responseReceived',
      envelope: {
        request_id: 'repair-quote',
        ok: true,
        payload: { ship_id: 'starter', cost: 15, currency: 'credits', disabled: true },
        server_time: 1003,
        v: 1,
      },
    });

    expect(quoted.repairQuote).toEqual({ ship_id: 'starter', cost: 15, currency: 'credits', disabled: true });
  });

  test('phase 09 quest, admin, and observability payloads reconcile server-owned state', () => {
    const questBoard = {
      offers: [
        {
          offer_id: 'offer-1',
          quest_type: 'kill',
          title: 'Training Sweep',
          description: 'Destroy hostile targets confirmed by the server.',
          objectives: [{ id: 'kill', kind: 'kill', target: 'pirate_drone', current: 0, required: 2, completed: false }],
          rewards: [{ kind: 'currency', currency_type: 'credits', amount: 100 }],
          expires_at: 5000,
          can_accept: true,
        },
      ],
      active: [],
      counts: { offers: 1, active: 0, completed: 0, claimable: 0, claimed: 0 },
      reroll_cost: { currency_type: 'credits', amount: 25 },
      can_reroll: true,
      reset_at: 5000,
      generated_at: 1000,
      revision: 1000,
    };
    const withBoard = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'quest-board',
        ok: true,
        payload: { quest_board: questBoard },
        server_time: 1001,
        v: 1,
      },
    });

    const acceptedQuest = {
      quest_id: 'quest-1',
      accepted_offer_id: 'offer-1',
      quest_type: 'kill',
      title: 'Training Sweep',
      description: 'Destroy hostile targets confirmed by the server.',
      state: 'accepted',
      objectives: [{ id: 'kill', kind: 'kill', target: 'pirate_drone', current: 1, required: 2, completed: false }],
      rewards: [{ kind: 'currency', currency_type: 'credits', amount: 100 }],
      accepted_at: 1002,
      can_claim: false,
    };
    const withAccepted = reduceClientState(withBoard, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.questAccepted, acceptedQuest, 2),
    });
    const withProgress = reduceClientState(withAccepted, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.questProgressed,
        {
          ...acceptedQuest,
          state: 'completed',
          objectives: [{ id: 'kill', kind: 'kill', target: 'pirate_drone', current: 2, required: 2, completed: true }],
          completed_at: 1100,
          can_claim: true,
        },
        3,
      ),
    });

    const afterClaim = reduceClientState(withProgress, {
      type: 'responseReceived',
      envelope: {
        request_id: 'quest-claim',
        ok: true,
        payload: {
          quest_board: {
            ...questBoard,
            active: [
              {
                ...acceptedQuest,
                state: 'claimed',
                completed_at: 1100,
                claimed_at: 1200,
                can_claim: false,
              },
            ],
            counts: { offers: 1, active: 0, completed: 0, claimable: 0, claimed: 1 },
            generated_at: 1200,
            revision: 1200,
          },
          wallet: { credits: 600, premium_paid: 0, premium_earned: 0 },
          inventory: {
            stackable: [{ item_id: 'iron_ore', quantity: 4, location: 'account_inventory' }],
            instances: [],
            counts: { cargo_stacks: 0, storage_stacks: 1, equipped_instances: 0 },
          },
          progression: { main_level: 1, main_xp: 25, rank: 1, combat_xp: 0 },
        },
        server_time: 1004,
        v: 1,
      },
    });

    const withAdmin = reduceClientState(afterClaim, {
      type: 'responseReceived',
      envelope: {
        request_id: 'ops',
        ok: true,
        payload: {
          admin: {
            target: 'self',
            inventory: { stackable_items: 1, instance_items: 0, item_ledger: [] },
            wallet: { balances: [{ currency_type: 'credits', balance: 600 }], ledger: [] },
            generated_at: 1300,
          },
          admin_repair: { accepted: false, job_id: 'craft-job-1', status: 'unavailable' },
          command_log: {
            entries: [{ request_id: 'quest-board', operation: 'quest.board', status: 'success', duration_ms: 2, timestamp: 1300 }],
            total: 1,
            generated_at: 1300,
          },
          metrics: { snapshot: { counters: [{ name: 'commands_per_sec', value: 1 }], gauges: [], durations: [] }, generated_at: 1300 },
          release_gate: {
            report: { covered: true, passed: true },
            coverage: [{ module: '10-quest-board-generation', passed: true, evidence: 3 }],
            evidence: 3,
            generated_at: 1300,
          },
          abuse_coverage: { report: { passed: true }, coverage: [{ case: 'negative_amounts', evidence: [] }], generated_at: 1300 },
        },
        server_time: 1005,
        v: 1,
      },
    });

    expect(withBoard.questBoard?.offers[0].offer_id).toBe('offer-1');
    expect(withAccepted.questBoard?.counts.active).toBe(1);
    expect(withAccepted.questBoard?.offers).toHaveLength(0);
    expect(withAccepted.questBoard?.counts.offers).toBe(0);
    expect(withProgress.questBoard?.counts.claimable).toBe(1);
    expect(afterClaim.questBoard?.counts.claimed).toBe(1);
    expect(afterClaim.wallet?.credits).toBe(600);
    expect(afterClaim.inventory?.stackable[0].item_id).toBe('iron_ore');
    expect(afterClaim.progression?.main_xp).toBe(25);
    expect(withAdmin.adminInspection?.wallet.balances[0].balance).toBe(600);
    expect(withAdmin.adminRepair?.status).toBe('unavailable');
    expect(withAdmin.commandLogSummary?.entries[0].operation).toBe('quest.board');
    expect(withAdmin.metrics?.snapshot.counters[0].name).toBe('commands_per_sec');
    expect(withAdmin.releaseGate?.report.passed).toBe(true);
    expect(withAdmin.abuseCoverage?.report.passed).toBe(true);
  });

  test('phase 09 quest board rejects stale revisions and fails closed on expired offers', () => {
    const freshBoard = {
      offers: [
        {
          offer_id: 'offer-fresh',
          quest_type: 'collect',
          title: 'Cargo Recovery',
          description: 'Recover cargo marked by the server.',
          objectives: [
            {
              id: 'collect',
              kind: 'collect',
              target: 'iron_ore',
              display_name: 'Iron Ore',
              catalog_ref: 'item:iron_ore',
              art_key: 'item.iron_ore',
              current: 0,
              required: 2,
              completed: false,
            },
          ],
          rewards: [
            {
              kind: 'item',
              item_id: 'scanner_circuit',
              display_name: 'Scanner Circuit',
              catalog_ref: 'item:scanner_circuit',
              art_key: 'item.scanner_circuit',
              amount: 1,
            },
          ],
          expires_at: 3000,
          can_accept: true,
        },
      ],
      active: [],
      counts: { offers: 1, active: 0, completed: 0, claimable: 0, claimed: 0 },
      reroll_cost: { currency_type: 'credits', amount: 25 },
      can_reroll: true,
      reset_at: 3000,
      generated_at: 2000,
      revision: 2000,
    };
    const withFresh = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'quest-board-fresh',
        ok: true,
        payload: { quest_board: freshBoard },
        server_time: 2001,
        v: 1,
      },
    });
    const stale = reduceClientState(withFresh, {
      type: 'responseReceived',
      envelope: {
        request_id: 'quest-board-stale',
        ok: true,
        payload: {
          quest_board: {
            ...freshBoard,
            offers: [],
            counts: { offers: 0, active: 0, completed: 0, claimable: 0, claimed: 0 },
            generated_at: 1000,
            revision: 1000,
          },
        },
        server_time: 2002,
        v: 1,
      },
    });
    const expired = reduceClientState(withFresh, {
      type: 'responseReceived',
      envelope: {
        request_id: 'quest-board-expired',
        ok: true,
        payload: {
          quest_board: {
            ...freshBoard,
            offers: [{ ...freshBoard.offers[0], offer_id: 'offer-expired', expires_at: 2500, can_accept: true }],
            reset_at: 2500,
            generated_at: 2000,
            revision: 3000,
          },
        },
        server_time: 3001,
        v: 1,
      },
    });

    expect(withFresh.questBoard?.offers[0].objectives[0].display_name).toBe('Iron Ore');
    expect(withFresh.questBoard?.offers[0].rewards[0].display_name).toBe('Scanner Circuit');
    expect(stale.questBoard?.offers).toHaveLength(1);
    expect(stale.questBoard?.generated_at).toBe(2000);
    expect(stale.questBoard?.revision).toBe(2000);
    expect(expired.questBoard?.offers[0].can_accept).toBe(false);
    expect(expired.questBoard?.offers[0].locked_reason).toBe('Offer expired.');
  });
});

function event(type: string, payload: JsonObject, seq = 1): EventEnvelope {
  return {
    event_id: `event-${seq}`,
    type,
    payload,
    server_time: 1000 + seq,
    seq,
    v: 1,
  };
}

function expectServerOwnedGameplayCleared(state: ClientState): void {
  expect(state.lastServerTime).toBeNull();
  expect(state.lastSequence).toBe(0);
  expect(state.playerSnapshot).toBeNull();
  expect(state.sector).toBeNull();
  expect(state.minimap).toBeNull();
  expect(state.visibleEntities).toEqual({});
  expect(state.selectedTargetID).toBeNull();
  expect(state.movementTarget).toBeNull();
  expect(state.lastCorrection).toBeNull();
  expect(state.knownLoot).toEqual({});
  expect(state.worldEffects).toEqual([]);
  expect(state.pendingCommands).toEqual({});
  expect(state.combatLog).toEqual([]);
  expect(state.cargo).toBeNull();
  expect(state.wallet).toBeNull();
  expect(state.ship).toBeNull();
  expect(state.stats).toBeNull();
  expect(state.progression).toBeNull();
  expect(state.inventory).toBeNull();
  expect(state.hangar).toBeNull();
  expect(state.loadout).toBeNull();
  expect(state.crafting).toBeNull();
  expect(state.repairQuote).toBeNull();
  expect(state.skillCooldowns).toEqual({});
  expect(state.questBoard).toBeNull();
  expect(state.planetIntel).toBeNull();
  expect(state.scanMode).toEqual({
    enabled: false,
    nextPulseAt: null,
    lastRejectedAt: null,
    lastError: null,
  });
  expect(state.production).toBeNull();
  expect(state.routes).toBeNull();
  expect(state.shopCatalog).toBeNull();
  expect(state.market).toBeNull();
  expect(state.auction).toBeNull();
  expect(state.premium).toBeNull();
  expect(state.economyDashboard).toBeNull();
  expect(state.adminInspection).toBeNull();
  expect(state.adminRepair).toBeNull();
  expect(state.commandLogSummary).toBeNull();
  expect(state.metrics).toBeNull();
  expect(state.releaseGate).toBeNull();
  expect(state.abuseCoverage).toBeNull();
  expect(state.lastError).toBeNull();
}

function stateWithServerOwnedGameplay(): ClientState {
  return {
    ...createInitialState(),
    auth: {
      mode: 'real',
      session: {
        authenticated: true,
        account: { email: 'pilot@example.com', admin: true },
        player: { callsign: 'Server-Pilot' },
        server_time: 1000,
      },
      submitting: false,
      error: null,
    },
    connectionStatus: 'connected',
    lastServerTime: 1000,
    lastSequence: 42,
    playerSnapshot: { callsign: 'Server-Pilot', hp: 80, shield: 70, energy: 60 },
    sector: { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
    minimap: { radar_range: 420, live_contacts: [], remembered: [] },
    visibleEntities: {
      'npc-1': {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
        display: { label: 'Training Drone', disposition: 'hostile' },
        combat: { hp: 20, max_hp: 30, shield: 4, max_shield: 10, status: 'hostile' },
      },
    },
    selectedTargetID: 'npc-1',
    movementTarget: { x: 100, y: 100 },
    lastCorrection: { entityID: 'player-1', position: { x: 1, y: 2 } },
    knownLoot: { 'drop-1': { drop_id: 'drop-1', item_id: 'raw_ore', quantity: 3 } },
    worldEffects: [{ id: 'effect-1', kind: 'damage', targetID: 'npc-1', amount: 4, createdAt: 1, expiresAt: 2 }],
    pendingCommands: { 'request-1': { requestID: 'request-1', op: 'move_to', queuedAt: 1 } },
    commandLog: [{ id: 'log-1', level: 'info', text: 'Server log.', at: 1 }],
    combatLog: [{ id: 'combat-1', level: 'info', text: 'Hit Training Drone.', at: 1 }],
    cargo: { used: 3, capacity: 60, items: [{ item_id: 'raw_ore', quantity: 3 }] },
    wallet: { credits: 1200, premium_paid: 300, premium_earned: 0 },
    ship: serverOwnedStub<ClientState['ship']>(),
    stats: serverOwnedStub<ClientState['stats']>(),
    progression: serverOwnedStub<ClientState['progression']>(),
    inventory: serverOwnedStub<ClientState['inventory']>(),
    hangar: serverOwnedStub<ClientState['hangar']>(),
    loadout: serverOwnedStub<ClientState['loadout']>(),
    crafting: serverOwnedStub<ClientState['crafting']>(),
    repairQuote: serverOwnedStub<ClientState['repairQuote']>(),
    skillCooldowns: { basic_laser: 2000 },
    questBoard: serverOwnedStub<ClientState['questBoard']>(),
    planetIntel: serverOwnedStub<ClientState['planetIntel']>(),
    scanMode: {
      enabled: true,
      nextPulseAt: 1234,
      lastRejectedAt: 1200,
      lastError: 'Cooldown.',
    },
    production: serverOwnedStub<ClientState['production']>(),
    routes: serverOwnedStub<ClientState['routes']>(),
    shopCatalog: serverOwnedStub<ClientState['shopCatalog']>(),
    market: serverOwnedStub<ClientState['market']>(),
    auction: serverOwnedStub<ClientState['auction']>(),
    premium: serverOwnedStub<ClientState['premium']>(),
    economyDashboard: serverOwnedStub<ClientState['economyDashboard']>(),
    adminInspection: serverOwnedStub<ClientState['adminInspection']>(),
    adminRepair: serverOwnedStub<ClientState['adminRepair']>(),
    commandLogSummary: serverOwnedStub<ClientState['commandLogSummary']>(),
    metrics: serverOwnedStub<ClientState['metrics']>(),
    releaseGate: serverOwnedStub<ClientState['releaseGate']>(),
    abuseCoverage: serverOwnedStub<ClientState['abuseCoverage']>(),
    lastError: { code: 'server_error', message: 'Server error.', retryable: true },
  };
}

function serverOwnedStub<T>(): NonNullable<T> {
  return { from_server: true } as unknown as NonNullable<T>;
}

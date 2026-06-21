import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event, expectServerOwnedGameplayCleared, stateWithServerOwnedGameplay } from './reducer-fixtures.test-support';
import { isWithinMinimapProjectionWindow, worldMapMemoryMarkers } from './world-memory';

describe('reduceClientState', () => {
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
});

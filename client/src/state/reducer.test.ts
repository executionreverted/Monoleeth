import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, EventEnvelope, JsonObject } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';

describe('reduceClientState', () => {
  test('initial state has no fake gameplay values', () => {
    const state = createInitialState();

    expect(state.connectionStatus).toBe('restoring');
    expect(state.playerSnapshot).toBeNull();
    expect(state.sector).toBeNull();
    expect(state.minimap).toBeNull();
    expect(state.cargo).toBeNull();
    expect(state.wallet).toBeNull();
    expect(state.stats).toBeNull();
    expect(state.questBoard).toBeNull();
    expect(state.inventory).toBeNull();
    expect(state.planetIntel).toBeNull();
    expect(state.visibleEntities).toEqual({});
  });

  test('logout and auth expiry clear gameplay state', () => {
    const withGameplay = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.playerSnapshot, {
        callsign: 'Server-Pilot',
        hp: 80,
        shield: 70,
        energy: 60,
      }),
    });

    const loggedOut = reduceClientState(withGameplay, { type: 'authLoggedOut' });
    expect(loggedOut.connectionStatus).toBe('logged_out');
    expect(loggedOut.playerSnapshot).toBeNull();
    expect(loggedOut.visibleEntities).toEqual({});

    const expired = reduceClientState(withGameplay, { type: 'authExpired', message: 'Session expired.' });
    expect(expired.connectionStatus).toBe('auth_expired');
    expect(expired.playerSnapshot).toBeNull();
    expect(expired.auth.error).toBe('Session expired.');
  });

  test('demo mode is explicit and isolated from real auth session state', () => {
    const demo = reduceClientState(createInitialState(), { type: 'demoModeStarted' });

    expect(demo.auth.mode).toBe('demo');
    expect(demo.auth.session).toBeNull();
    expect(demo.playerSnapshot).toBeNull();
    expect(demo.commandLog.some((line) => line.text.includes('Demo mode'))).toBe(true);
  });

  test('handles AOI enter and leave events', () => {
    const state = createInitialState();
    const entered = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
        display: { label: 'Training Drone', disposition: 'hostile' },
      }),
    });

    expect(entered.visibleEntities['npc-1']).toMatchObject({
      entity_id: 'npc-1',
      position: { x: 10, y: 20 },
    });

    const left = reduceClientState(entered, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityLeft, { entity_id: 'npc-1' }, 2),
    });

    expect(left.visibleEntities['npc-1']).toBeUndefined();
  });

  test('server correction updates authoritative entity position and clears local target', () => {
    const state = reduceClientState(createInitialState(), {
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
        }),
      },
    );

    expect(corrected.visibleEntities['player-1'].position).toEqual({ x: 12, y: 16 });
    expect(corrected.movementTarget).toBeNull();
    expect(corrected.lastCorrection).toEqual({ entityID: 'player-1', position: { x: 12, y: 16 } });
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

  test('snapshot response reconciles player, cargo, wallet, and stat panels', () => {
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
            items: [{ item_id: 'raw_ore', quantity: 4 }],
          },
          wallet: { credits: 980, premium_paid: 3, premium_earned: 9 },
          stats: { speed: 220, radar_range: 510, weapon_range: 280, cargo_capacity: 80 },
        },
        server_time: 1400,
        v: 1,
      },
    });

    expect(reconciled.playerSnapshot?.callsign).toBe('Server-Pilot');
    expect(reconciled.cargo).toMatchObject({ used: 4, capacity: 80 });
    expect(reconciled.cargo?.items).toEqual([{ item_id: 'raw_ore', quantity: 4 }]);
    expect(reconciled.wallet).toEqual({ credits: 980, premium_paid: 3, premium_earned: 9 });
    expect(reconciled.stats).toMatchObject({ speed: 220, radar_range: 510, weapon_range: 280, cargo_capacity: 80 });
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
            },
          ],
          minimap: {
            radar_range: 420,
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
        }),
      },
    );

    expect(state.connectionStatus).toBe('connected');
    expect(state.sector).toEqual({ name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false });
    expect(state.minimap?.radar_range).toBe(420);
    expect(state.visibleEntities['player-local'].status_flags).toContain('self');
  });

  test('snapshot events reconcile cargo, wallet, and stats independently', () => {
    const withCargo = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.cargoSnapshot, {
        used: 12,
        capacity: 70,
        items: [{ item_id: 'salvage_thread', quantity: 12 }],
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

    expect(withStats.cargo?.items).toEqual([{ item_id: 'salvage_thread', quantity: 12 }]);
    expect(withStats.wallet?.credits).toBe(444);
    expect(withStats.stats?.weapon_range).toBe(275);
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

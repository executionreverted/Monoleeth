import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';

const portalLeakCases: Array<[string, Record<string, unknown>]> = [
  ['nested destination public map key', { destination: { public_map_key: '2-1' } }],
  ['nested spawn position', { spawn: { position: { x: 20, y: 30 } } }],
  ['destination id alias', { destination_id: 'server-only-destination' }],
  ['destination map key alias', { destination_map_key: '2-1' }],
  ['destination public map key alias', { destination_public_map_key: '2-1' }],
  ['spawn position alias', { spawn_position: { x: 20, y: 30 } }],
  ['to map key alias', { to_map_key: '2-1' }],
  ['to public map key alias', { to_public_map_key: '2-1' }],
  ['portal-level public map key', { public_map_key: '1-1' }],
];

describe('reduceClientState world payload guards', () => {
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

  test('world snapshot rejects forbidden map payload keys before state mutation', () => {
    const state = createInitialState();

    expect(() =>
      reduceClientState(state, {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.worldSnapshot, {
          map: {
            public_map_key: '1-1',
            display_name: 'Origin Fringe',
            internal_map_id: 'server-only',
            bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
          },
          entities: [],
        }),
      }),
    ).toThrow(/Forbidden server payload rejected/);
  });

  test('direct map snapshot rejects forbidden map payload keys before state mutation', () => {
    const state = createInitialState();

    expect(() =>
      reduceClientState(state, {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.mapSnapshot, {
          map_subscription_epoch: 3,
          public_map_key: '1-2',
          display_name: 'Outer Ring',
          bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
          visible_portals: [
            {
              portal_id: 'west_gate',
              position: { x: 100, y: 200 },
              interaction_radius: 160,
              internal_map_id: 'server-only',
            },
          ],
          safe_zones: [],
        }),
      }),
    ).toThrow(/Forbidden server payload rejected/);
  });

  test.each(portalLeakCases)('portal summaries reject %s before state mutation', (_name, leak) => {
    const state = createInitialState();

    expect(() =>
      reduceClientState(state, {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.mapSnapshot, {
          map_subscription_epoch: 3,
          public_map_key: '1-2',
          display_name: 'Outer Ring',
          bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
          visible_portals: [
            {
              portal_id: 'west_gate',
              position: { x: 100, y: 200 },
              interaction_radius: 160,
              ...leak,
            },
          ],
          safe_zones: [],
        }),
      }),
    ).toThrow(/Forbidden server payload rejected/);
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

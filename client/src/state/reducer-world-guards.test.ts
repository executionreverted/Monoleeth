import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';

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

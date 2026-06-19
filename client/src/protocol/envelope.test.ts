import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, parseServerMessage, rejectForbiddenPayloadKeys } from './envelope';

describe('parseServerMessage', () => {
  test('parses a successful response envelope', () => {
    const message = parseServerMessage(
      JSON.stringify({
        request_id: 'request-1',
        ok: true,
        payload: { accepted: true },
        server_time: 182736123,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      request_id: 'request-1',
      ok: true,
      payload: { accepted: true },
      server_time: 182736123,
      v: 1,
    });
  });

  test('parses a client-safe event envelope', () => {
    const message = parseServerMessage(
      JSON.stringify({
        event_id: 'event-1',
        type: CLIENT_EVENTS.entityEntered,
        payload: {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 10, y: 20 },
          display: { label: 'Training Drone', disposition: 'hostile' },
        },
        server_time: 182736124,
        seq: 9,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      event_id: 'event-1',
      type: CLIENT_EVENTS.entityEntered,
      payload: {
        entity_id: 'npc-1',
      },
    });
  });

  test('rejects hidden or internal server payload fields', () => {
    expect(() =>
      parseServerMessage(
        JSON.stringify({
          event_id: 'event-hidden',
          type: CLIENT_EVENTS.entityEntered,
          payload: {
            entity_id: 'hidden-planet',
            entity_type: 'planet_signal',
            position: { x: 1, y: 2 },
            gameplay_seed: 'server-only',
          },
          server_time: 1,
          seq: 1,
          v: 1,
        }),
      ),
    ).toThrow(/Forbidden server payload rejected/);
  });

  test('rejects unsupported protocol versions', () => {
    expect(() =>
      parseServerMessage(
        JSON.stringify({
          request_id: 'request-1',
          ok: true,
          payload: {},
          server_time: 1,
          v: 999,
        }),
      ),
    ).toThrow(/Unsupported protocol version/);
  });
});

describe('rejectForbiddenPayloadKeys', () => {
  test('checks nested payloads', () => {
    expect(() => rejectForbiddenPayloadKeys({ entities: [{ future_spawn_data: ['x'] }] })).toThrow(
      /Forbidden server payload rejected/,
    );
  });
});

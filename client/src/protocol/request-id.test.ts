import { expect, test } from 'vitest';

import { CommandBuilder } from './commands';
import { createRequestId } from './request-id';

test('createRequestId returns non-empty unique ids', () => {
  const ids = new Set(Array.from({ length: 20 }, () => createRequestId()));

  expect(ids.size).toBe(20);
  for (const id of ids) {
    expect(id.length).toBeGreaterThan(8);
  }
});

test('command builders include request ids and omit trusted fields', () => {
  const builder = new CommandBuilder();
  const move = builder.moveTo({ x: 120, y: -40 });
  const serialized = JSON.stringify(move);

  expect(move.request_id).toBeTruthy();
  expect(move.client_seq).toBe(1);
  expect(move.payload).toEqual({ target: { x: 120, y: -40 } });
  expect(serialized).not.toContain('player_id');
  expect(serialized).not.toContain('damage');
  expect(serialized).not.toContain('xp');
  expect(serialized).not.toContain('loot');
  expect(serialized).not.toContain('cooldown');
});

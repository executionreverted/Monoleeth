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
  const fire = builder.combatUseSkill('npc-1');
  const pickup = builder.lootPickup('drop-1');
  const scan = builder.scanPulse();
  const serializedPayloads = JSON.stringify([move.payload, fire.payload, pickup.payload, scan.payload]);

  expect(move.request_id).toBeTruthy();
  expect(move.client_seq).toBe(1);
  expect(move.payload).toEqual({ target: { x: 120, y: -40 } });
  expect(fire.op).toBe('combat.use_skill');
  expect(fire.payload).toEqual({ target_id: 'npc-1', skill_id: 'basic_laser' });
  expect(pickup.op).toBe('loot.pickup');
  expect(pickup.payload).toEqual({ drop_id: 'drop-1' });
  expect(scan.op).toBe('scan.pulse');
  expect(scan.payload).toEqual({});
  expect(serializedPayloads).not.toContain('player_id');
  expect(serializedPayloads).not.toContain('damage');
  expect(serializedPayloads).not.toContain('xp');
  expect(serializedPayloads).not.toContain('loot');
  expect(serializedPayloads).not.toContain('cooldown');
});

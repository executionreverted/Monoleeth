import { describe, expect, test } from 'vitest';

import type { WorldFeedbackEffect } from '../state/types';
import { damageKindForEffect, projectileDebugFromEffect } from './world-renderer-types';
import { spriteDirectionForMovementVector, spriteScaleForEntity } from './world-renderer-sprites';

describe('world renderer sprite sizing', () => {
  test('keeps large curated 512px entity sprites inside the HUD canvas scale', () => {
    const worldScale = 1;

    expect(spriteScaleForEntity({ entity_id: 'self', entity_type: 'player', position: { x: 0, y: 0 } }, worldScale)).toBeLessThanOrEqual(
      0.18,
    );
    expect(spriteScaleForEntity({ entity_id: 'npc', entity_type: 'npc', position: { x: 0, y: 0 } }, worldScale)).toBeLessThanOrEqual(0.16);
    expect(spriteScaleForEntity({ entity_id: 'drop', entity_type: 'loot', position: { x: 0, y: 0 } }, worldScale)).toBeLessThanOrEqual(0.14);
  });

  test('maps movement vectors to the eight isometric direction frame codes', () => {
    expect(spriteDirectionForMovementVector(10, 0)).toBe('10');
    expect(spriteDirectionForMovementVector(10, 10)).toBe('12');
    expect(spriteDirectionForMovementVector(0, 10)).toBe('14');
    expect(spriteDirectionForMovementVector(-10, 10)).toBe('00');
    expect(spriteDirectionForMovementVector(-10, 0)).toBe('02');
    expect(spriteDirectionForMovementVector(-10, -10)).toBe('04');
    expect(spriteDirectionForMovementVector(0, -10)).toBe('06');
    expect(spriteDirectionForMovementVector(10, -10)).toBe('08');
    expect(spriteDirectionForMovementVector(0, 0)).toBeNull();
  });

  test('classifies shield, hull, and mixed damage effects for renderer impacts', () => {
    expect(damageKindForEffect(effect({ shieldAmount: 7 }))).toBe('shield');
    expect(damageKindForEffect(effect({ hullAmount: 5 }))).toBe('hull');
    expect(damageKindForEffect(effect({ shieldAmount: 3, hullAmount: 4 }))).toBe('mixed');
    expect(damageKindForEffect(effect({ damageKind: 'mixed', amount: 9 }))).toBe('mixed');
  });

  test('projectile debug snapshots preserve shot phase and source-target ids', () => {
    const snapshot = projectileDebugFromEffect(
      effect({
        kind: 'laser',
        phase: 'started',
        sourceID: 'player-1',
        sourceEntityID: 'player-1',
        targetID: 'npc-1',
        targetEntityID: 'npc-1',
        createdAt: 1000,
        expiresAt: 1700,
      }),
      1130,
      { x: 0, y: 0 },
      { x: 100, y: 0 },
    );

    expect(snapshot).toMatchObject({
      id: 'effect-1',
      phase: 'started',
      sourceEntityID: 'player-1',
      targetEntityID: 'npc-1',
      progress: 0.5,
      active: true,
    });
    expect(snapshot?.head).toEqual({ x: 50, y: 0 });
  });
});

function effect(overrides: Partial<WorldFeedbackEffect>): WorldFeedbackEffect {
  return {
    id: 'effect-1',
    kind: 'damage',
    createdAt: 1000,
    expiresAt: 2000,
    ...overrides,
  };
}

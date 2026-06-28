import { describe, expect, test } from 'vitest';

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
});

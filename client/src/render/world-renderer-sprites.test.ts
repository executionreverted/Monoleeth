import { describe, expect, test } from 'vitest';

import { spriteScaleForEntity } from './world-renderer-sprites';

describe('world renderer sprite sizing', () => {
  test('keeps large curated 512px entity sprites inside the HUD canvas scale', () => {
    const worldScale = 1;

    expect(spriteScaleForEntity({ entity_id: 'self', entity_type: 'player', position: { x: 0, y: 0 } }, worldScale)).toBeLessThanOrEqual(
      0.18,
    );
    expect(spriteScaleForEntity({ entity_id: 'npc', entity_type: 'npc', position: { x: 0, y: 0 } }, worldScale)).toBeLessThanOrEqual(0.16);
    expect(spriteScaleForEntity({ entity_id: 'drop', entity_type: 'loot', position: { x: 0, y: 0 } }, worldScale)).toBeLessThanOrEqual(0.14);
  });
});

import { describe, expect, test } from 'vitest';

import { clampMovementTargetToMapBounds } from './client-app-commands';

describe('clampMovementTargetToMapBounds', () => {
  const bounds = { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 };

  test('keeps in-bounds movement targets unchanged', () => {
    expect(clampMovementTargetToMapBounds({ x: 120, y: 45 }, bounds)).toEqual({ x: 120, y: 45 });
  });

  test('clamps screen-click movement targets to current map bounds', () => {
    expect(clampMovementTargetToMapBounds({ x: 33, y: -39 }, bounds)).toEqual({ x: 33, y: 0 });
    expect(clampMovementTargetToMapBounds({ x: 10032, y: 10080 }, bounds)).toEqual({ x: 10000, y: 10000 });
  });

  test('leaves targets unchanged when map bounds are unavailable', () => {
    expect(clampMovementTargetToMapBounds({ x: 33, y: -39 }, null)).toEqual({ x: 33, y: -39 });
  });
});

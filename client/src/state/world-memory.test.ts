import { describe, expect, test } from 'vitest';

import { isWithinMinimapProjectionWindow, minimapPointPercent } from './world-memory';

describe('world memory projection helpers', () => {
  test('square projection keeps corner contacts even outside circular radius', () => {
    const center = { x: 0, y: 0 };
    const corner = { x: 1000, y: 1000 };

    expect(isWithinMinimapProjectionWindow(center, corner, 1000)).toBe(true);
    expect(Math.hypot(corner.x - center.x, corner.y - center.y)).toBeGreaterThan(1000);
    expect(minimapPointPercent(center, corner, 1000)).toEqual({ left: 96, top: 96, clamped: true });
  });

  test('far remembered coordinates remain outside current minimap projection', () => {
    const center = { x: 0, y: 0 };
    const far = { x: 5200, y: -3800 };

    expect(isWithinMinimapProjectionWindow(center, far, 1000)).toBe(false);
    expect(minimapPointPercent(center, far, 1000)).toEqual({ left: 96, top: 4, clamped: true });
  });
});

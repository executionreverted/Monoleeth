import { describe, expect, test } from 'vitest';

import {
  isRenderableMinimapMemory,
  isWithinMinimapProjectionWindow,
  minimapPointPercent,
  rememberedIntelState,
  rememberedMinimapDetailID,
  shouldRenderRememberedMinimapMemory,
  worldMapMemoryMarkers,
} from './world-memory';
import { createInitialState } from './reducer';
import type { MinimapMemory } from './types';

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

  test('invalidated and wrong-zone remembered intel are not renderable memory markers', () => {
    const validMemory: MinimapMemory = {
      kind: 'known_planet',
      sector_key: 'origin',
      planet_id: 'planet-stale',
      detail_id: 'planet-stale',
      label: 'Stale Planet',
      position: { x: 120, y: -80 },
      freshness: 'stale',
      projection_source: 'known_intel',
    };
    const invalidatedMemory = {
      ...validMemory,
      planet_id: 'planet-invalidated',
      detail_id: 'planet-invalidated',
      freshness: 'fresh',
      invalidated: true,
    };
    const wrongZoneMemory = {
      ...validMemory,
      planet_id: 'planet-wrong-zone',
      detail_id: 'planet-wrong-zone',
      freshness: 'wrong_zone',
    };
    const wrongSectorMemory = {
      ...validMemory,
      planet_id: 'planet-wrong-sector',
      detail_id: 'planet-wrong-sector',
      sector_key: 'other-sector',
      freshness: 'fresh',
    };

    expect(isRenderableMinimapMemory(validMemory)).toBe(true);
    expect(isRenderableMinimapMemory(invalidatedMemory)).toBe(false);
    expect(isRenderableMinimapMemory(wrongZoneMemory)).toBe(false);
    expect(isRenderableMinimapMemory(wrongSectorMemory)).toBe(true);
    expect(rememberedIntelState(invalidatedMemory)).toBe('invalidated');

    const state = {
      ...createInitialState(),
      sector: { sector_key: 'origin', name: 'Origin', region: 'Belt', danger: 'low', contested: false },
      minimap: {
        radar_range: 1000,
        projection_window_size: 2000,
        live_contacts: [],
        remembered: [validMemory, invalidatedMemory, wrongZoneMemory, wrongSectorMemory],
      },
    };

    expect(rememberedMinimapDetailID(state, validMemory)).toBe('planet-stale');
    expect(rememberedMinimapDetailID(state, wrongSectorMemory)).toBeNull();
    expect(shouldRenderRememberedMinimapMemory(state, wrongSectorMemory, { x: 0, y: 0 }, 1000)).toBe(false);
    expect(worldMapMemoryMarkers(state)).toEqual([
      {
        id: 'known_planet:planet-stale',
        kind: 'known_planet',
        label: 'Stale Planet',
        position: { x: 120, y: -80 },
        detailID: 'planet-stale',
        state: 'stale',
        projectionSource: 'known_intel',
      },
    ]);
  });
});

import { describe, expect, test } from 'vitest';

import { summarizeMapOverlay } from './map-overlay';
import { WorldRenderer } from './world-renderer';

const projection = {
  center: { x: 5000, y: 5000 },
  screen: { width: 1000, height: 800 },
  scale: 0.1,
};

describe('map overlay projection', () => {
  test('without current map or minimap bounds it reports no overlay markers', () => {
    const overlay = summarizeMapOverlay(
      {
        currentMap: null,
        minimap: {
          radar_range: 900,
          live_contacts: [],
          remembered: [],
          visible_portals: [{ portal_id: 'server-gate', position: { x: 9800, y: 5000 }, interaction_radius: 160 }],
          safe_zones: [
            {
              safe_area_id: 'station-alpha',
              center: { x: 500, y: 500 },
              radius: 700,
              blocks_pvp: true,
              hangar_actions: true,
            },
          ],
        },
      },
      projection,
    );

    expect(overlay.active).toBe(false);
    expect(overlay.bounds).toBeNull();
    expect(overlay.portalMarkers).toEqual([]);
    expect(overlay.safeZones).toEqual([]);
  });

  test('bounded current map payload reports an active projected frame', () => {
    const overlay = summarizeMapOverlay(
      {
        currentMap: {
          public_map_key: '1-1',
          display_name: 'Origin Fringe',
          bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
          visible_portals: [],
          safe_zones: [],
        },
        minimap: null,
      },
      projection,
    );

    expect(overlay.active).toBe(true);
    expect(overlay.source).toBe('currentMap');
    expect(overlay.bounds?.world).toEqual({ min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 });
    expect(overlay.bounds?.topLeft).toEqual({ x: 0, y: -100 });
    expect(overlay.bounds?.bottomRight).toEqual({ x: 1000, y: 900 });
  });

  test('server portal and safe-zone payloads report marker positions without fake data', () => {
    const overlay = summarizeMapOverlay(
      {
        currentMap: {
          public_map_key: '1-1',
          bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
          visible_portals: [
            { portal_id: 'east_gate', display_name: 'East Gate', position: { x: 6000, y: 5200 }, interaction_radius: 160 },
          ],
          safe_zones: [
            {
              safe_area_id: 'station-alpha',
              display_name: 'Station Alpha',
              center: { x: 4500, y: 4800 },
              radius: 700,
              blocks_pvp: true,
              hangar_actions: true,
            },
          ],
        },
        minimap: null,
      },
      projection,
    );

    expect(overlay.portalMarkers).toEqual([
      {
        source: 'currentMap',
        portalID: 'east_gate',
        label: 'East Gate',
        world: { x: 6000, y: 5200 },
        screen: { x: 600, y: 420 },
        interactionRadius: 160,
        screenRadius: 16,
      },
    ]);
    expect(overlay.safeZones).toEqual([
      {
        source: 'currentMap',
        safeAreaID: 'station-alpha',
        label: 'Station Alpha',
        center: { x: 4500, y: 4800 },
        screen: { x: 450, y: 380 },
        radius: 700,
        screenRadius: 70,
        blocksPVP: true,
        hangarActions: true,
      },
    ]);
  });

  test('renderer debug snapshot exposes map overlay state instead of fog', () => {
    const renderer = new WorldRenderer({
      onMoveIntent: () => undefined,
      onSelectTarget: () => undefined,
      onSelectMemoryMarker: () => undefined,
    });

    const snapshot = renderer.debugSnapshot() as Record<string, unknown>;

    expect(snapshot.mapOverlay).toMatchObject({ active: false, source: null });
    expect(snapshot).not.toHaveProperty('fog');
  });
});

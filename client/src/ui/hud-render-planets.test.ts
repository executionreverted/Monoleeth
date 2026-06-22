import { describe, expect, test } from 'vitest';

import { createInitialState } from '../state/reducer';
import type { ClientState, MapSummary, MinimapSummary } from '../state/types';
import { minimapPanel } from './hud-render-planets';
import { topbarDangerText, topbarLocationText } from './hud-topbar';

describe('minimapPanel', () => {
  test('no minimap and no current map bounds renders awaiting without fake markers', () => {
    const html = minimapPanel(createInitialState());

    expect(html).toContain('Awaiting map projection.');
    expect(html).not.toContain('minimap__point');
    expect(html).not.toContain('minimap__portal');
    expect(html).not.toContain('minimap__safe-zone');
    expect(html).not.toContain('data-action');
  });

  test('bounds-only current map renders bounded frame metadata without fake contacts', () => {
    const state = withCurrentMap(createInitialState(), {
      display_name: 'Origin Gate',
      public_map_key: '1-1',
      risk_band: 'low',
      pvp_policy: 'pve',
    });

    const html = minimapPanel(state);

    expect(html).toContain('minimap--bounded');
    expect(html).toContain('Origin Gate');
    expect(html).toContain('Bounds 10K x 10K');
    expect(html).toContain('low/pve');
    expect(html).not.toContain('Awaiting map projection.');
    expect(html).not.toContain('minimap__point');
  });

  test('server portal and safe-zone payloads render display-only markers and labels', () => {
    const state = withCurrentMap(createInitialState(), {
      visible_portals: [
        { portal_id: 'east_gate', display_name: 'East Gate', position: { x: 9800, y: 5000 }, interaction_radius: 160 },
      ],
      safe_zones: [
        { safe_area_id: 'dock_ring', display_name: 'Dock Ring', center: { x: 5000, y: 5000 }, radius: 900, blocks_pvp: true, hangar_actions: true },
      ],
    });
    state.minimap = minimap({
      visible_portals: [
        { portal_id: 'east_gate', display_name: 'Duplicate Gate', position: { x: 100, y: 100 }, interaction_radius: 100 },
      ],
      safe_zones: [
        { safe_area_id: 'dock_ring', display_name: 'Duplicate Dock', center: { x: 200, y: 200 }, radius: 400, blocks_pvp: true, hangar_actions: false },
      ],
    });

    const html = minimapPanel(state);

    expect(count(html, 'class="minimap__portal"')).toBe(1);
    expect(count(html, 'class="minimap__safe-zone"')).toBe(1);
    expect(html).toContain('East Gate');
    expect(html).toContain('Dock Ring');
    expect(html).not.toContain('Duplicate Gate');
    expect(html).not.toContain('Duplicate Dock');
    expect(html).not.toContain('portal.enter');
    expect(html).not.toContain('data-action');
  });

  test('remembered minimap memory from another public map is not rendered when current map key exists', () => {
    const state = withCurrentMap(createInitialState(), {
      public_map_key: '1-1',
      display_name: 'Origin Gate',
    });
    state.minimap = minimap({
      public_map_key: '1-1',
      remembered: [
        {
          kind: 'known_planet',
          public_map_key: '1-1',
          planet_id: 'planet-home',
          detail_id: 'planet-home',
          label: 'Home Planet',
          position: { x: 2500, y: 2500 },
          freshness: 'fresh',
        },
        {
          kind: 'known_planet',
          public_map_key: '2-1',
          planet_id: 'planet-away',
          detail_id: 'planet-away',
          label: 'Away Planet',
          position: { x: 2500, y: 2500 },
          freshness: 'fresh',
        },
      ],
    });

    const html = minimapPanel(state);

    expect(html).toContain('Home Planet');
    expect(html).toContain('data-planet-id="planet-home"');
    expect(html).not.toContain('Away Planet');
    expect(html).not.toContain('planet-away');
  });

  test('legacy radar-only minimap contact keeps radar-centered projection behavior', () => {
    const state = createInitialState();
    state.minimap = minimap({
      radar_range: 100,
      live_contacts: [
        {
          entity_id: 'self',
          entity_type: 'player',
          position: { x: 100, y: 100 },
          status_flags: ['self'],
        },
        {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 200, y: 100 },
          disposition: 'hostile',
          status_flags: ['hostile'],
        },
      ],
    });

    const html = minimapPanel(state);

    expect(html).toContain('minimap--radar');
    expect(html).toContain('data-map-mode="radar"');
    expect(html).toContain('data-action="target-select"');
    expect(html).toContain('data-entity-id="npc-1"');
    expect(html).toContain('style="left:96%;top:50%"');
  });
});

describe('topbar map labels', () => {
  test('location prefers current map display, public key, map key, then sector', () => {
    expect(topbarLocationText(withCurrentMap(createInitialState(), { display_name: 'Veil-03' }))).toBe('Veil-03');
    expect(topbarLocationText(withCurrentMap(createInitialState(), { display_name: '', public_map_key: '1-2' }))).toBe('1-2');
    expect(topbarLocationText(withCurrentMap(createInitialState(), { display_name: '', public_map_key: '', map_key: 'legacy-1' }))).toBe('legacy-1');

    const state = createInitialState();
    state.sector = { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false };
    expect(topbarLocationText(state)).toBe('Origin Fringe');
  });

  test('danger prefers public map safety/policy and preserves sector fallback', () => {
    expect(topbarDangerText(withCurrentMap(createInitialState(), { protection: { reason: 'spawn', expires_at: 123, blocks_pvp: true, break_on_pvp_action: true } }))).toBe('protected');
    expect(topbarDangerText(withCurrentMap(createInitialState(), { safe_zone: { inside: true, blocks_pvp: true } }))).toBe('safe zone');
    expect(topbarDangerText(withCurrentMap(createInitialState(), { risk_band: 'medium', pvp_policy: 'contested' }))).toBe('medium/contested');

    const state = createInitialState();
    state.sector = { name: 'Veil', region: 'Belt', danger: 'high', contested: true };
    expect(topbarDangerText(state)).toBe('contested');
  });
});

function withCurrentMap(state: ClientState, overrides: Partial<MapSummary>): ClientState {
  state.currentMap = {
    map_key: '1-1',
    public_map_key: '1-1',
    display_name: 'Origin',
    region: 'Origin Belt',
    risk_band: 'low',
    pvp_policy: 'pve',
    bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
    visible_portals: [],
    safe_zones: [],
    ...overrides,
  };
  return state;
}

function minimap(overrides: Partial<MinimapSummary>): MinimapSummary {
  return {
    radar_range: 420,
    live_contacts: [],
    remembered: [],
    ...overrides,
  };
}

function count(value: string, needle: string): number {
  return value.split(needle).length - 1;
}

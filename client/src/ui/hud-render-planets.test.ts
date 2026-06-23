import { beforeEach, describe, expect, test, vi } from 'vitest';

import { OPERATIONS } from '../protocol/envelope';
import { createInitialState } from '../state/reducer';
import type { ClientState, MapSummary, MinimapSummary } from '../state/types';
import { hudSelection } from './hud-selection';
import { minimapPanel, planetCatalogPanel, planetDetailModal } from './hud-render-planets';
import { actionBar, targetPanel } from './hud-render-panels';
import { topbarDangerText, topbarLocationText } from './hud-topbar';

describe('minimapPanel', () => {
  beforeEach(() => {
    hudSelection.selectedPortalID = null;
    hudSelection.selectedPortalScope = null;
  });

  test('missing map snapshot while bootstrapping renders loading states without fake markers', () => {
    const cases: Array<{
      status: ClientState['connectionStatus'];
      label: string;
    }> = [
      { status: 'restoring', label: 'Loading map snapshot.' },
      { status: 'connecting', label: 'Loading map snapshot.' },
      { status: 'authenticated_pending_socket', label: 'Loading map snapshot.' },
      { status: 'connected', label: 'Awaiting map snapshot.' },
    ];

    for (const item of cases) {
      const state = createInitialState();
      state.connectionStatus = item.status;
      const html = minimapPanel(state);

      expect(html, item.status).toContain('data-map-state="loading"');
      expect(html, item.status).toContain(item.label);
      expect(html, item.status).toContain('Awaiting portal list');
      expect(html, item.status).toContain('Awaiting server map snapshot.');
      expectNoMinimapActions(html, item.status);
    }
  });

  test('missing map snapshot while locked or disconnected renders no marker actions', () => {
    const cases: Array<{
      status: ClientState['connectionStatus'];
      mapState: string;
      label: string;
      detail: string;
      portalLabel: string;
      portalDetail: string;
    }> = [
      {
        status: 'logged_out',
        mapState: 'locked',
        label: 'Map snapshot locked.',
        detail: 'Log in to load server-owned map state.',
        portalLabel: 'Portal list locked',
        portalDetail: 'Locked map snapshot required.',
      },
      {
        status: 'auth_expired',
        mapState: 'locked',
        label: 'Map snapshot locked.',
        detail: 'Session expired. Log in again.',
        portalLabel: 'Portal list locked',
        portalDetail: 'Locked map snapshot required.',
      },
      {
        status: 'offline',
        mapState: 'disconnected',
        label: 'Map snapshot disconnected.',
        detail: 'Realtime connection required for server-owned map state.',
        portalLabel: 'Portal list disconnected',
        portalDetail: 'Disconnected map snapshot required.',
      },
      {
        status: 'error',
        mapState: 'disconnected',
        label: 'Map snapshot disconnected.',
        detail: 'Realtime connection required for server-owned map state.',
        portalLabel: 'Portal list disconnected',
        portalDetail: 'Disconnected map snapshot required.',
      },
      {
        status: 'reconnecting',
        mapState: 'disconnected',
        label: 'Map snapshot disconnected.',
        detail: 'Realtime connection required for server-owned map state.',
        portalLabel: 'Portal list disconnected',
        portalDetail: 'Disconnected map snapshot required.',
      },
    ];

    for (const item of cases) {
      const state = createInitialState();
      state.connectionStatus = item.status;
      const itemHTML = minimapPanel(state);

      expect(itemHTML, item.status).toContain(`data-map-state="${item.mapState}"`);
      expect(itemHTML, item.status).toContain(item.label);
      expect(itemHTML, item.status).toContain(item.detail);
      expect(itemHTML, item.status).toContain(item.portalLabel);
      expect(itemHTML, item.status).toContain(item.portalDetail);
      expectNoMinimapActions(itemHTML, item.status);
    }
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
    expect(html).toContain('No visible portals');
    expect(html).not.toContain('Awaiting map projection.');
    expect(html).not.toContain('minimap__point');
  });

  test('server portal and safe-zone payloads render selectable portal controls and a detail strip', () => {
    const state = withCurrentMap(createInitialState(), {
      visible_portals: [
        {
          portal_id: 'east_gate',
          display_name: 'East Gate',
          destination_label: 'Outer Ring',
          state: 'locked',
          locked_reason: 'Rank gate',
          position: { x: 9800, y: 5000 },
          interaction_radius: 160,
        },
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

    const initialHTML = minimapPanel(state);
    hudSelection.selectedPortalID = 'east_gate';
    hudSelection.selectedPortalScope = firstPortalScope(initialHTML);
    const html = minimapPanel(state);

    expect(count(html, 'class="minimap__portal"')).toBe(1);
    expect(count(html, 'class="minimap__safe-zone"')).toBe(1);
    expect(html).toContain('data-action="portal-select"');
    expect(html).toContain('data-selected="true"');
    expect(html).toContain('data-action="portal-enter"');
    expect(html).toContain('East Gate');
    expect(html).toContain('Outer Ring');
    expect(html).toContain('locked');
    expect(html).toContain('Rank gate');
    expect(html).toContain('Dock Ring');
    expect(html).not.toContain('Duplicate Gate');
    expect(html).not.toContain('Duplicate Dock');
    expect(html).not.toContain('portal.enter');
  });

  test('unavailable portal states render disabled enter without fake destination', () => {
    const cases: Array<{
      name: string;
      portal: Partial<MapSummary['visible_portals'][number]>;
      statePatch?: Partial<ClientState>;
      serverNow?: number;
    }> = [
      { name: 'missing state', portal: {} },
      { name: 'cooldown', portal: { state: 'cooldown', cooldown_ready_at_ms: 5_000 }, serverNow: 1_000 },
      { name: 'locked', portal: { state: 'locked', locked_reason: 'Faction gate' } },
      { name: 'offline', portal: { state: 'offline' } },
      { name: 'local cooldown', portal: { state: 'available' }, statePatch: { portalCooldowns: { east_gate: 5_000 } }, serverNow: 1_000 },
      {
        name: 'pending',
        portal: { state: 'available' },
        statePatch: { pendingCommands: { 'portal-1': { requestID: 'portal-1', op: OPERATIONS.portalEnter, queuedAt: 1 } } },
      },
    ];

    for (const item of cases) {
      hudSelection.selectedPortalID = null;
      hudSelection.selectedPortalScope = null;
      const state = withCurrentMap(createInitialState(), {
        visible_portals: [
          {
            portal_id: 'east_gate',
            display_name: 'East Gate',
            position: { x: 9800, y: 5000 },
            interaction_radius: 160,
            ...item.portal,
          },
        ],
      });
      if (item.statePatch) {
        Object.assign(state, item.statePatch);
      }
      const initialHTML = minimapPanel(state, item.serverNow ?? 1_000);
      hudSelection.selectedPortalID = 'east_gate';
      hudSelection.selectedPortalScope = firstPortalScope(initialHTML);

      const html = minimapPanel(state, item.serverNow ?? 1_000);

      expect(html, item.name).toContain('data-action="portal-enter"');
      expect(html, item.name).toMatch(/data-action="portal-enter"[^>]*disabled/);
      expect(html, item.name).not.toContain('Unknown destination');
      expect(html, item.name).not.toContain('<em>Dest</em>');
    }
  });

  test('available portal renders enabled enter action for the current map scope', () => {
    const state = withCurrentMap(createInitialState(), {
      visible_portals: [
        {
          portal_id: 'east_gate',
          label: 'East Transit',
          destination_label: 'Outer Ring',
          state: 'available',
          position: { x: 9800, y: 5000 },
          interaction_radius: 160,
        },
      ],
    });
    const initialHTML = minimapPanel(state, 1_000);
    hudSelection.selectedPortalID = 'east_gate';
    hudSelection.selectedPortalScope = firstPortalScope(initialHTML);

    const html = minimapPanel(state, 1_000);

    expect(html).toContain('data-action="portal-enter"');
    expect(html).toContain('data-portal-id="east_gate"');
    expect(html).toContain('East Transit');
    expect(html).toContain('Outer Ring');
    expect(html).toMatch(/data-action="portal-enter"[^>]*data-portal-id="east_gate"[^>]*>Enter/);
    expect(html).not.toMatch(/data-action="portal-enter"[^>]*disabled/);
  });

  test('map and epoch scoped selection prevents reused portal ids from staying enterable', () => {
    const origin = withCurrentMap(createInitialState(), {
      public_map_key: '1-1',
      visible_portals: [
        { portal_id: 'east_gate', display_name: 'East Gate', state: 'available', position: { x: 9800, y: 5000 }, interaction_radius: 160 },
      ],
    });
    origin.mapSubscriptionEpoch = 11;
    const originHTML = minimapPanel(origin, 1_000);
    hudSelection.selectedPortalID = 'east_gate';
    hudSelection.selectedPortalScope = firstPortalScope(originHTML);

    const destination = withCurrentMap(createInitialState(), {
      public_map_key: '1-2',
      display_name: 'Outer Ring',
      visible_portals: [
        { portal_id: 'east_gate', display_name: 'Reused Gate', state: 'available', position: { x: 120, y: 5000 }, interaction_radius: 160 },
      ],
    });
    destination.mapSubscriptionEpoch = 12;
    const staleHTML = minimapPanel(destination, 1_000);

    expect(staleHTML).toContain('Fresh click required.');
    expect(staleHTML).toContain('data-selected="false"');
    expect(staleHTML).not.toMatch(/data-action="portal-enter"[^>]*data-portal-id="east_gate"[^>]*>Enter/);
  });

  test('reconnect clears HUD-local portal selection before the same scope can be reused', () => {
    const state = withCurrentMap(createInitialState(), {
      visible_portals: [
        { portal_id: 'east_gate', display_name: 'East Gate', state: 'available', position: { x: 9800, y: 5000 }, interaction_radius: 160 },
      ],
    });
    const scope = firstPortalScope(minimapPanel(state, 1_000));
    hudSelection.selectedPortalID = 'east_gate';
    hudSelection.selectedPortalScope = scope;

    state.connectionStatus = 'reconnecting';
    const reconnectingHTML = minimapPanel(state, 1_000);

    expect(reconnectingHTML).toContain('class="minimap__portal"');
    expect(reconnectingHTML).toMatch(/class="minimap__portal"[^>]*disabled/);
    expect(reconnectingHTML).toContain('Portal actions locked');
    expect(reconnectingHTML).toContain('Realtime map connection required.');
    expect(reconnectingHTML).not.toContain('data-action="portal-enter"');
    expect(hudSelection.selectedPortalID).toBeNull();
    expect(hudSelection.selectedPortalScope).toBeNull();

    state.connectionStatus = 'connected';
    const connectedHTML = minimapPanel(state, 1_000);

    expect(connectedHTML).toContain('Select a portal');
    expect(connectedHTML).toContain('data-selected="false"');
    expect(connectedHTML).not.toMatch(/data-action="portal-enter"[^>]*data-portal-id="east_gate"[^>]*>Enter/);
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

describe('planet claim controls', () => {
  test('catalog and modal enable claim for connected unclaimed planet without inventing X Core truth', () => {
    const state = planetClaimState('unclaimed');

    const catalogHTML = planetCatalogPanel(state);
    const modalHTML = planetDetailModal(state, 'planet-eris');

    expect(catalogHTML).toMatch(/data-action="planet-claim"[^>]*data-planet-id="planet-eris"[^>]*>Claim/);
    expect(catalogHTML).not.toMatch(/data-action="planet-claim"[^>]*disabled/);
    expect(modalHTML).toMatch(/data-action="planet-claim"[^>]*data-planet-id="planet-eris"[^>]*>Claim/);
    expect(modalHTML).not.toMatch(/data-action="planet-claim"[^>]*disabled/);
    expect(catalogHTML).not.toContain('x_core');
    expect(modalHTML).not.toContain('x_core');
  });

  test('claim button disables when planet is owned or claim is pending', () => {
    const owned = planetClaimState('owned_by_you');
    const ownedHTML = planetCatalogPanel(owned);

    expect(ownedHTML).toMatch(/data-action="planet-claim"[^>]*disabled[^>]*>Claim/);
    expect(ownedHTML).toContain('Planet already claimed');

    const pending = planetClaimState('unclaimed');
    pending.pendingCommands = {
      'claim-1': {
        requestID: 'claim-1',
        op: OPERATIONS.discoveryClaimPlanet,
        queuedAt: 1,
        payload: { planet_id: 'planet-eris' },
      },
    };
    const pendingHTML = planetCatalogPanel(pending);

    expect(pendingHTML).toMatch(/data-action="planet-claim"[^>]*disabled[^>]*>Claiming/);
    expect(pendingHTML).toContain('Planet claim pending');

    const otherPending = planetClaimState('unclaimed');
    otherPending.pendingCommands = {
      'claim-2': {
        requestID: 'claim-2',
        op: OPERATIONS.discoveryClaimPlanet,
        queuedAt: 1,
        payload: { planet_id: 'planet-other' },
      },
    };
    const otherPendingHTML = planetCatalogPanel(otherPending);

    expect(otherPendingHTML).toMatch(/data-action="planet-claim"[^>]*data-planet-id="planet-eris"[^>]*>Claim/);
    expect(otherPendingHTML).not.toMatch(/data-action="planet-claim"[^>]*disabled/);
  });
});

describe('route controls', () => {
  test('owned selected planet renders building build and upgrade actions from production state', () => {
    const state = planetRouteState();
    state.planetIntel!.selectedPlanet!.production!.buildings = [
      {
        planet_id: 'planet-source',
        public_map_key: '1-1',
        building_id: 'planet-source-building-iron_extractor-alpha',
        building_type: 'iron_extractor',
        category: 'extractor',
        level: 1,
        state: 'active',
        updated_at: 1000,
      },
    ];
    state.production = { planets: [state.planetIntel!.selectedPlanet!.production!] };

    const catalogHTML = planetCatalogPanel(state);
    const modalHTML = planetDetailModal(state, 'planet-source');

    for (const html of [catalogHTML, modalHTML]) {
      expect(html).toContain('data-building-build-control="true"');
      expect(html).toContain('data-planet-id="planet-source"');
      expect(html).toContain('data-building-build-type');
      expect(html).toContain('value="iron_extractor"');
      expect(html).toContain('value="alloy_foundry"');
      expect(html).toContain('data-building-build-slot');
      expect(html).toContain('value="beta"');
      expect(html).toMatch(/data-action="planet-building-build"[^>]*data-planet-id="planet-source"[^>]*>Build/);
      expect(html).toContain('data-action="planet-building-upgrade"');
      expect(html).toContain('data-building-id="planet-source-building-iron_extractor-alpha"');
      expect(html).toContain('data-target-level="2"');
      expect(html).not.toContain('owner_player_id');
      expect(html).not.toContain('materials');
      expect(html).not.toContain('cost');
    }
  });

  test('owned selected planet renders route create, update, control, and settle actions from server state', () => {
    const state = planetRouteState();

    const catalogHTML = planetCatalogPanel(state);
    const modalHTML = planetDetailModal(state, 'planet-source');

    for (const html of [catalogHTML, modalHTML]) {
      expect(html).toContain('data-route-create-control="true"');
      expect(html).toContain('data-route-source-planet-id="planet-source"');
      expect(html).toMatch(/data-action="route-create"[^>]*data-source-planet-id="planet-source"[^>]*>Create/);
      expect(html).toContain('data-route-create-destination');
      expect(html).toContain('value="planet-destination"');
      expect(html).toContain('data-route-create-resource');
      expect(html).toContain('value="refined_alloy"');
      expect(html).toContain('data-route-rate');
      expect(html).toContain('data-action="route-update"');
      expect(html).toContain('data-action="route-disable"');
      expect(html).toContain('data-action="route-settle" data-route-id="route-1"');
      expect(html).toMatch(/data-action="route-settle"[^>]*>Settle All/);
      expect(html).not.toContain('owner_player_id');
      expect(html).not.toContain('source_map_id');
      expect(html).not.toContain('destination_map_id');
    }
  });

  test('known planet renders coordinate item create action without coordinate truth payload', () => {
    const state = planetRouteState();

    const catalogHTML = planetCatalogPanel(state);
    const modalHTML = planetDetailModal(state, 'planet-source');

    for (const html of [catalogHTML, modalHTML]) {
      expect(html).toMatch(/data-action="coordinate-item-create"[^>]*data-planet-id="planet-source"[^>]*>Coord/);
      expect(html).not.toContain('data-coordinates');
      expect(html).not.toContain('source_player_id');
      expect(html).not.toContain('confidence_override');
    }

    state.pendingCommands = {
      'coordinate-create-1': {
        requestID: 'coordinate-create-1',
        op: OPERATIONS.intelCoordinateItemCreate,
        payload: { planet_id: 'planet-source' },
        queuedAt: 1,
      },
    };

    const pendingHTML = planetDetailModal(state, 'planet-source');
    expect(pendingHTML).toMatch(/data-action="coordinate-item-create"[^>]*disabled[^>]*>Creating/);
  });

  test('route create disables without another owned endpoint or source storage resource', () => {
    const state = planetRouteState();
    state.planetIntel!.planets = [state.planetIntel!.planets[0]];
    state.planetIntel!.selectedPlanet!.production!.storage.items = [];
    state.production = { planets: [state.planetIntel!.selectedPlanet!.production!] };
    state.routes = { routes: [] };
    state.planetIntel!.selectedPlanet!.routes = [];

    const html = planetCatalogPanel(state);

    expect(html).toContain('data-route-create-control="true"');
    expect(html).toMatch(/data-action="route-create"[^>]*disabled[^>]*>Create/);
    expect(html).toContain('No endpoint');
    expect(html).toContain('No resource');
    expect(html).toContain('No routes for this planet.');
  });

  test('route rows render server-owned settlement outcome flags', () => {
    const state = planetRouteState();
    state.planetIntel!.selectedPlanet!.routes[0].last_settlement = {
      route_id: 'route-1',
      resource_item_id: 'refined_alloy',
      settled_at: 1800,
      elapsed_applied_ms: 3_600_000,
      wanted_amount: 40,
      taken_amount: 10,
      lost_amount: 3,
      delivered_amount: 7,
      added_amount: 0,
      source_empty: true,
      destination_full: true,
      loss_applied: true,
      no_op: true,
    };

    const html = planetDetailModal(state, 'planet-source');

    expect(html).toContain('data-route-settlement-result="true"');
    expect(html).toContain('No transfer / Source empty / Storage full / Loss applied');
    expect(html).toContain('0/40 refined_alloy');
    expect(html).not.toContain('owner_player_id');
    expect(html).not.toContain('settlement_window');
  });

  test('storage and station destination routes can settle without exposing internal endpoint ids or browser update', () => {
    const cases = [
      { type: 'storage', label: 'Storage (1-1)', internalID: 'storage-route-settle-destination' },
      { type: 'station', label: 'Station (1-1)', internalID: 'station-route-settle-destination' },
    ];

    for (const item of cases) {
      const state = planetRouteState();
      state.planetIntel!.selectedPlanet!.routes[0] = {
        ...state.planetIntel!.selectedPlanet!.routes[0],
        route_id: `route-${item.type}`,
        destination: { type: item.type, id: '' },
      };

      const html = planetDetailModal(state, 'planet-source');

      expect(html).toContain(`data-route-destination-type="${item.type}"`);
      expect(html).toContain(item.label);
      expect(html).toMatch(new RegExp(`data-action="route-update"[^>]*data-route-id="route-${item.type}"[^>]*disabled`));
      expect(html).toContain(`data-action="route-settle" data-route-id="route-${item.type}"`);
      expect(html).not.toContain(item.internalID);
      expect(html).not.toContain('destination_map_id');
    }
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

describe('actionBar', () => {
  test('basic laser cooldown readiness uses server time instead of local clock', () => {
    const dateNow = vi.spyOn(Date, 'now').mockReturnValue(10_000);
    try {
      const state = combatReadyState();
      state.skillCooldowns.basic_laser = 15_000;

      const html = actionBar(state, 20_000);
      const laserSlot = actionSlot(html, 'laser');

      expect(laserSlot).toContain('data-state="ready"');
      expect(laserSlot).toContain('Laser');
      expect(laserSlot).toContain('10 cap');
      expect(laserSlot).not.toContain('Cooling');
      expect(laserSlot).not.toContain('disabled');
    } finally {
      dateNow.mockRestore();
    }
  });

  test('visible non-self player target enables laser and target Fire controls even with friendly projection', () => {
    const state = combatReadyState();
    state.selectedTargetID = 'pilot-rival';
    state.visibleEntities = {
      'pilot-self': selfEntity(),
      'pilot-rival': {
        entity_id: 'pilot-rival',
        entity_type: 'player',
        position: { x: 120, y: 20 },
        status_flags: ['friendly'],
        display: { label: 'Rival Pilot', disposition: 'friendly' },
        combat: { hp: 100, max_hp: 100, shield: 50, max_shield: 50, status: 'active' },
      },
    };

    const barHTML = actionBar(state, 20_000);
    const laserSlot = actionSlot(barHTML, 'laser');
    const targetHTML = targetPanel(state, 20_000);

    expect(laserSlot).toContain('data-state="ready"');
    expect(laserSlot).toContain('data-action="fire"');
    expect(laserSlot).not.toContain('disabled');
    expect(targetHTML).toContain('data-target-kind="player"');
    expect(targetHTML).toMatch(/data-action="fire"[^>]*>Fire/);
    expect(targetHTML).not.toMatch(/data-action="fire"[^>]*disabled/);
  });

  test('self player target keeps laser blocked and hides target Fire control', () => {
    const state = combatReadyState();
    const self = selfEntity();
    state.selectedTargetID = self.entity_id;
    state.visibleEntities = {
      [self.entity_id]: self,
    };

    const barHTML = actionBar(state, 20_000);
    const laserSlot = actionSlot(barHTML, 'laser');
    const targetHTML = targetPanel(state, 20_000);

    expect(laserSlot).toContain('data-state="blocked"');
    expect(laserSlot).toContain('Select an attackable target.');
    expect(laserSlot).toMatch(/data-action="fire"[^>]*disabled/);
    expect(targetHTML).toContain('data-target-kind="player"');
    expect(targetHTML).not.toContain('data-action="fire"');
  });
});

function withCurrentMap(state: ClientState, overrides: Partial<MapSummary>): ClientState {
  state.connectionStatus = 'connected';
  state.mapSubscriptionEpoch = 1;
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

function planetClaimState(ownerStatus: string): ClientState {
  const state = createInitialState();
  state.connectionStatus = 'connected';
  state.planetIntel = {
    knownSignals: 1,
    staleIntel: 0,
    ownedPlanets: ownerStatus === 'owned_by_you' ? 1 : 0,
    planets: [
      {
        planet_id: 'planet-eris',
        biome: 'ice',
        planet_type: 'dwarf_planet',
        rarity: 'uncommon',
        level: 2,
        intel_state: 'fresh',
        confidence: 88,
        last_seen_at: 1000,
        owner_status: ownerStatus,
        discovered_at: 900,
      },
    ],
    selectedPlanet: {
      planet_id: 'planet-eris',
      biome: 'ice',
      planet_type: 'dwarf_planet',
      rarity: 'uncommon',
      level: 2,
      intel_state: 'fresh',
      confidence: 88,
      last_seen_at: 1000,
      owner_status: ownerStatus,
      discovered_at: 900,
      coordinates: { x: 320, y: 140 },
      production_locked: true,
      available_commands: ['discovery.claim_planet'],
      routes: [],
    },
    lastScan: null,
  };
  return state;
}

function planetRouteState(): ClientState {
  const state = createInitialState();
  state.connectionStatus = 'connected';
  const route = {
    route_id: 'route-1',
    source_planet_id: 'planet-source',
    from_public_map_key: '1-1',
    to_public_map_key: '1-1',
    destination: { type: 'planet', id: 'planet-destination' },
    resource_item_id: 'refined_alloy',
    amount_per_hour: 40,
    energy_cost_per_hour: 4,
    enabled: true,
    risk: { loss_chance: 0.02, min_loss_percent: 0, max_loss_percent: 0 },
    last_calculated_at: 1000,
    updated_at: 1000,
  };
  state.planetIntel = {
    knownSignals: 2,
    staleIntel: 0,
    ownedPlanets: 2,
    planets: [
      {
        planet_id: 'planet-source',
        biome: 'origin_belt',
        planet_type: 'terrestrial',
        rarity: 'common',
        level: 1,
        intel_state: 'verified',
        confidence: 100,
        last_seen_at: 1000,
        owner_status: 'owned_by_you',
        discovered_at: 900,
      },
      {
        planet_id: 'planet-destination',
        biome: 'origin_belt',
        planet_type: 'ice',
        rarity: 'common',
        level: 1,
        intel_state: 'verified',
        confidence: 100,
        last_seen_at: 1000,
        owner_status: 'owned_by_you',
        discovered_at: 900,
      },
    ],
    selectedPlanet: {
      planet_id: 'planet-source',
      biome: 'origin_belt',
      planet_type: 'terrestrial',
      rarity: 'common',
      level: 1,
      intel_state: 'verified',
      confidence: 100,
      last_seen_at: 1000,
      owner_status: 'owned_by_you',
      discovered_at: 900,
      coordinates: { x: 2400, y: 2600 },
      production_locked: false,
      available_commands: ['route.list'],
      routes: [route],
      production: {
        planet_id: 'planet-source',
        production_enabled: true,
        last_calculated_at: 1000,
        energy_capacity_per_hour: 80,
        energy_reserved_per_hour: 0,
        storage: {
          planet_id: 'planet-source',
          used_units: 340,
          free_units: 160,
          capacity_units: 500,
          updated_at: 1000,
          items: [
            { item_id: 'refined_alloy', quantity: 160 },
            { item_id: 'raw_ore', quantity: 180 },
          ],
        },
        buildings: [],
      },
    },
    lastScan: null,
  };
  state.production = { planets: [state.planetIntel.selectedPlanet!.production!] };
  state.routes = { routes: [route] };
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

function firstPortalScope(html: string): string {
  const match = html.match(/data-portal-scope="([^"]+)"/);
  expect(match?.[1]).toBeTruthy();
  return match?.[1] ?? '';
}

function expectNoMinimapActions(html: string, label: string): void {
  expect(html, label).toContain('data-portal-strip="true"');
  expect(html, label).toMatch(/<button class="portal-strip__action"[^>]*disabled[^>]*>Enter<\/button>/);
  expect(html, label).not.toContain('minimap__point');
  expect(html, label).not.toContain('minimap__portal');
  expect(html, label).not.toContain('minimap__safe-zone');
  expect(html, label).not.toContain('data-action');
  expect(html, label).not.toContain('data-marker');
}

function combatReadyState(): ClientState {
  const state = createInitialState();
  state.connectionStatus = 'connected';
  state.selectedTargetID = 'npc-1';
  state.visibleEntities = {
    'npc-1': {
      entity_id: 'npc-1',
      entity_type: 'npc',
      position: { x: 10, y: 20 },
      status_flags: ['hostile'],
      display: { label: 'Training Drone', disposition: 'hostile' },
      combat: { hp: 20, max_hp: 30, shield: 4, max_shield: 10, status: 'hostile' },
    },
  };
  state.ship = {
    active_ship_id: 'starter',
    display_name: 'Starter',
    hull: 100,
    max_hull: 100,
    shield: 50,
    max_shield: 50,
    capacitor: 20,
    max_capacitor: 20,
    disabled: false,
    repair_state: 'intact',
  };
  state.stats = {
    speed: 100,
    radar_range: 420,
    weapon_range: 600,
    cargo_capacity: 60,
    loot_pickup_range: 120,
    basic_laser_energy_cost: 10,
    basic_laser_cooldown_ms: 800,
  };
  return state;
}

function selfEntity() {
  return {
    entity_id: 'pilot-self',
    entity_type: 'player' as const,
    position: { x: 0, y: 20 },
    status_flags: ['self'],
    display: { label: 'You', disposition: 'self' },
    combat: { hp: 100, max_hp: 100, shield: 50, max_shield: 50, status: 'active' },
  };
}

function actionSlot(html: string, id: string): string {
  const match = html.match(new RegExp(`<div class="action-slot"[^>]*data-quick-action-slot="${id}"[\\s\\S]*?<\\/div>`));
  expect(match?.[0]).toBeTruthy();
  return match?.[0] ?? '';
}

import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';
import type { ClientState } from './types';

describe('reduceClientState world requests and snapshots', () => {
  test('request and response flow tracks pending commands', () => {
    const state = createInitialState();
    const queued = reduceClientState(state, {
      type: 'requestQueued',
      envelope: {
        request_id: 'request-1',
        op: 'move_to',
        payload: { target: { x: 1, y: 2 } },
        client_seq: 1,
        v: 1,
      },
    });

    expect(queued.pendingCommands['request-1']).toBeDefined();
    expect(queued.movementTarget).toEqual({ x: 1, y: 2 });

    const accepted = reduceClientState(queued, {
      type: 'responseReceived',
      envelope: {
        request_id: 'request-1',
        ok: true,
        payload: {},
        server_time: 99,
        v: 1,
      },
    });

    expect(accepted.pendingCommands['request-1']).toBeUndefined();
    expect(accepted.lastServerTime).toBe(99);
  });

  test('snapshot response replaces visible entities atomically', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'stale-npc',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
      }),
    });

    const replaced = reduceClientState(
      {
        ...state,
        selectedTargetID: 'stale-npc',
        movementTarget: { x: 100, y: 100 },
        lastCorrection: { entityID: 'stale-npc', position: { x: 10, y: 20 } },
      },
      {
        type: 'responseReceived',
        envelope: {
          request_id: 'snapshot-1',
          ok: true,
          payload: {
            map_subscription_epoch: 3,
            map: {
              public_map_key: '1-1',
              display_name: 'Origin Fringe',
              risk_band: 'low',
              pvp_policy: 'pve',
              bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
              visible_portals: [],
              safe_zones: [],
            },
            entities: [
              {
                entity_id: 'signal-1',
                entity_type: 'planet_signal',
                position: { x: 50, y: 60 },
                status_flags: ['known_intel'],
                display: { label: 'Unknown Signal', disposition: 'unknown' },
              },
            ],
            sector: { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
            minimap: {
              radar_range: 420,
              live_contacts: [
                {
                  entity_id: 'signal-1',
                  entity_type: 'planet_signal',
                  position: { x: 50, y: 60 },
                  disposition: 'unknown',
                  status_flags: ['known_intel'],
                },
              ],
              remembered: [],
            },
          },
          server_time: 1200,
          v: 1,
        },
      },
    );

    expect(replaced.visibleEntities['stale-npc']).toBeUndefined();
    expect(replaced.mapSubscriptionEpoch).toBe(3);
    expect(replaced.currentMap).toMatchObject({ public_map_key: '1-1', display_name: 'Origin Fringe' });
    expect(replaced.visibleEntities['signal-1']).toMatchObject({
      entity_type: 'planet_signal',
      position: { x: 50, y: 60 },
    });
    expect(replaced.sector).toMatchObject({ name: 'Origin Fringe', danger: 'low' });
    expect(replaced.currentMap).toMatchObject({ public_map_key: '1-1', bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 } });
    expect(replaced.minimap?.live_contacts).toHaveLength(1);
    expect(replaced.selectedTargetID).toBeNull();
    expect(replaced.movementTarget).toBeNull();
    expect(replaced.lastCorrection).toBeNull();
    expect(replaced.planetIntel?.knownSignals).toBe(1);
  });

  test('snapshot response without minimap rebuilds live contacts from replacement entities', () => {
    const state = {
      ...createInitialState(),
      selectedTargetID: 'stale-npc',
      minimap: {
        radar_range: 1000,
        projection_window_size: 2000,
        live_contacts: [
          {
            entity_id: 'stale-npc',
            entity_type: 'npc' as const,
            position: { x: 10, y: 20 },
            disposition: 'hostile',
            status_flags: ['hostile'],
          },
        ],
        remembered: [
          {
            kind: 'known_planet',
            planet_id: 'planet-eris',
            detail_id: 'planet-eris',
            label: 'Eris',
            position: { x: 500, y: -250 },
            freshness: 'fresh',
          },
        ],
      },
    };

    const replaced = reduceClientState(state, {
      type: 'responseReceived',
      envelope: {
        request_id: 'move-accepted',
        ok: true,
        payload: {
          accepted: true,
          entities: [
            {
              entity_id: 'player-local',
              entity_type: 'player',
              position: { x: 2, y: 4 },
              status_flags: ['self', 'hidden', 'stealthed'],
              display: { label: 'Pilot', disposition: 'self' },
              projection_source: 'worker_projection',
            },
            {
              entity_id: 'loot-1',
              entity_type: 'loot',
              position: { x: 30, y: 40 },
              status_flags: ['loot', 'hidden'],
              display: { label: 'Cache', disposition: 'neutral' },
              projection_source: 'worker_projection',
            },
          ],
        },
        server_time: 1500,
        v: 1,
      },
    });

    expect(replaced.visibleEntities['stale-npc']).toBeUndefined();
    expect(replaced.selectedTargetID).toBeNull();
    expect(replaced.minimap?.radar_range).toBe(1000);
    expect(replaced.minimap?.projection_window_size).toBe(2000);
    expect(replaced.minimap?.remembered).toEqual(state.minimap.remembered);
    expect(replaced.minimap?.live_contacts).toEqual([
      {
        entity_id: 'loot-1',
        entity_type: 'loot',
        position: { x: 30, y: 40 },
        disposition: 'neutral',
        status_flags: ['loot'],
        projection_source: 'worker_projection',
      },
      {
        entity_id: 'player-local',
        entity_type: 'player',
        position: { x: 2, y: 4 },
        disposition: 'self',
        status_flags: ['self', 'stealthed'],
        projection_source: 'worker_projection',
      },
    ]);
  });

  test('world snapshot event stores sector and minimap projection', () => {
    const state = reduceClientState(
      {
        ...createInitialState(),
        auth: {
          mode: 'real',
          session: { authenticated: true, server_time: 1 },
          submitting: false,
          error: null,
        },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.worldSnapshot, {
          map_subscription_epoch: 4,
          sector: { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
          map: {
            map_key: '1-1',
            public_map_key: '1-1',
            display_name: 'Origin Fringe',
            region: 'Origin Belt',
            risk_band: 'low',
            pvp_policy: 'pve',
            visual_theme_key: 'origin_blue',
            bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
            visible_portals: [
              { portal_id: 'east_gate', display_name: 'East Gate', position: { x: 9800, y: 5000 }, interaction_radius: 160 },
              { portal_id: 'missing-position', display_name: 'Invalid Gate', interaction_radius: 160 },
              { portal_id: 'zero-radius', position: { x: 10, y: 20 }, interaction_radius: 0 },
            ],
            safe_zones: [
              {
                safe_area_id: 'station-alpha',
                display_name: 'Station Alpha',
                center: { x: 500, y: 500 },
                radius: 700,
                blocks_pvp: true,
                hangar_actions: true,
              },
              { safe_area_id: 'invalid-safe-zone', center: { x: 100, y: 100 }, radius: 400, blocks_pvp: true },
            ],
            safe_zone: { inside: true, blocks_pvp: true, protection_expires_at: 3000 },
            protection: { reason: 'portal_spawn', expires_at: 3000, blocks_pvp: true, break_on_pvp_action: true },
          },
          entities: [
            {
              entity_id: 'player-local',
              entity_type: 'player',
              position: { x: 0, y: 0 },
              status_flags: ['self'],
              display: { label: 'Smoke', disposition: 'self' },
              movement: {
                moving: true,
                origin: { x: 0, y: 0 },
                target: { x: 100, y: 0 },
                speed: 180,
                started_at_ms: 1000,
                arrive_at_ms: 1556,
              },
            },
          ],
          minimap: {
            public_map_key: '1-1',
            bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
            visible_portals: [
              { portal_id: 'east_gate', display_name: 'East Gate', position: { x: 9800, y: 5000 }, interaction_radius: 160 },
              { portal_id: 'bad-gate', interaction_radius: 160 },
            ],
            safe_zones: [
              {
                safe_area_id: 'station-alpha',
                display_name: 'Station Alpha',
                center: { x: 500, y: 500 },
                radius: 700,
                blocks_pvp: true,
                hangar_actions: true,
              },
              { safe_area_id: 'bad-zone', center: { x: 0, y: 0 }, radius: -1, blocks_pvp: true, hangar_actions: true },
            ],
            radar_range: 1000,
            projection_window_size: 2000,
            live_contacts: [
              {
                entity_id: 'player-local',
                entity_type: 'player',
                position: { x: 0, y: 0 },
                disposition: 'self',
                status_flags: ['self'],
              },
            ],
            remembered: [
              {
                kind: 'known_planet',
                public_map_key: '1-1',
                planet_id: 'planet-eris',
                detail_id: 'planet-eris',
                label: 'terran / ice',
                position: { x: 500, y: -250 },
                freshness: 'fresh',
              },
            ],
          },
        }),
      },
    );

    expect(state.connectionStatus).toBe('connected');
    expect(state.mapSubscriptionEpoch).toBe(4);
    expect(state.currentMap).toMatchObject({
      map_key: '1-1',
      public_map_key: '1-1',
      display_name: 'Origin Fringe',
      region: 'Origin Belt',
      risk_band: 'low',
      pvp_policy: 'pve',
      visual_theme_key: 'origin_blue',
      bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
      safe_zone: { inside: true, blocks_pvp: true, protection_expires_at: 3000 },
      protection: { reason: 'portal_spawn', expires_at: 3000, blocks_pvp: true, break_on_pvp_action: true },
    });
    expect(state.currentMap?.visible_portals).toEqual([
      { portal_id: 'east_gate', display_name: 'East Gate', position: { x: 9800, y: 5000 }, interaction_radius: 160 },
    ]);
    expect(state.currentMap?.safe_zones).toEqual([
      {
        safe_area_id: 'station-alpha',
        display_name: 'Station Alpha',
        center: { x: 500, y: 500 },
        radius: 700,
        blocks_pvp: true,
        hangar_actions: true,
      },
    ]);
    expect(state.sector).toEqual({ name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false });
    expect(state.minimap?.public_map_key).toBe('1-1');
    expect(state.minimap?.bounds).toEqual({ min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 });
    expect(state.minimap?.visible_portals).toEqual([
      { portal_id: 'east_gate', display_name: 'East Gate', position: { x: 9800, y: 5000 }, interaction_radius: 160 },
    ]);
    expect(state.minimap?.safe_zones).toHaveLength(1);
    expect(state.minimap?.radar_range).toBe(1000);
    expect(state.minimap?.projection_window_size).toBe(2000);
    expect(state.minimap?.remembered[0]).toMatchObject({ public_map_key: '1-1', planet_id: 'planet-eris', detail_id: 'planet-eris' });
    expect(state.visibleEntities['player-local'].status_flags).toContain('self');
    expect(state.visibleEntities['player-local'].movement?.target).toEqual({ x: 100, y: 0 });
    expect(state.movementTarget).toEqual({ x: 100, y: 0 });
  });

  test('portal parser keeps sanitized public optional fields and ignores invalid optional values', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapSnapshot, {
        map_subscription_epoch: 4,
        public_map_key: '1-1',
        display_name: 'Origin Fringe',
        bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
        visible_portals: [
          {
            portal_id: ' north_gate ',
            label: ' North Gate ',
            display_name: ' Legacy North ',
            destination_label: ' Outer Ring ',
            state: 'available',
            cooldown_ready_at_ms: 1234.6,
            locked_reason: ' Access pass ',
            position: { x: 5000, y: 100 },
            interaction_radius: 160,
          },
          {
            portal_id: 'invalid_optional_gate',
            display_name: 'Invalid Optional Gate',
            destination_label: ' ',
            state: 'charging',
            cooldown_ready_at_ms: -10,
            locked_reason: '',
            position: { x: 100, y: 5000 },
            interaction_radius: 160,
          },
          {
            portal_id: 'missing-position',
            state: 'available',
            interaction_radius: 160,
          },
        ],
        safe_zones: [],
      }, 2),
    });

    expect(state.currentMap?.visible_portals).toEqual([
      {
        portal_id: 'north_gate',
        label: 'North Gate',
        display_name: 'Legacy North',
        destination_label: 'Outer Ring',
        state: 'available',
        cooldown_ready_at_ms: 1235,
        locked_reason: 'Access pass',
        position: { x: 5000, y: 100 },
        interaction_radius: 160,
      },
      {
        portal_id: 'invalid_optional_gate',
        display_name: 'Invalid Optional Gate',
        position: { x: 100, y: 5000 },
        interaction_radius: 160,
      },
    ]);
  });

  test('world snapshot without map does not invent current map state', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.worldSnapshot, {
        sector: { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
        entities: [],
        minimap: { radar_range: 420, live_contacts: [], remembered: [] },
      }),
    });

    expect(state.currentMap).toBeNull();
    expect(state.portalCooldowns).toEqual({});
    expect(state.visibleEntities).toEqual({});
  });

  test('map snapshot event clears stale live state before applying a different map and epoch', () => {
    const base = mapScopedLiveState();
    const state = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapSnapshot, {
        map_subscription_epoch: 2,
        map: {
          public_map_key: '1-2',
          display_name: 'Outer Ring',
          region: 'Origin Belt',
          risk_band: 'medium',
          pvp_policy: 'contested',
          bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
          visible_portals: [{ portal_id: 'west_gate', position: { x: 200, y: 5000 }, interaction_radius: 160 }],
          safe_zones: [],
        },
      }, 2),
    });

    expect(state.mapSubscriptionEpoch).toBe(2);
    expect(state.currentMap).toMatchObject({
      public_map_key: '1-2',
      display_name: 'Outer Ring',
      risk_band: 'medium',
      pvp_policy: 'contested',
    });
    expectMapScopedLiveStateCleared(state);
  });

  test('same-map same-epoch map metadata refresh preserves live entities', () => {
    const base = mapScopedLiveState();
    const state = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapSnapshot, {
        map_subscription_epoch: 1,
        map: {
          public_map_key: '1-1',
          display_name: 'Origin Fringe Updated',
          region: 'Origin Belt',
          risk_band: 'medium',
          pvp_policy: 'contested',
          bounds: { min_x: 0, min_y: 0, max_x: 12000, max_y: 12000 },
          visible_portals: [{ portal_id: 'east_gate', position: { x: 9800, y: 5000 }, interaction_radius: 160 }],
          safe_zones: [],
        },
      }, 2),
    });

    expect(state.mapSubscriptionEpoch).toBe(1);
    expect(state.currentMap).toMatchObject({
      public_map_key: '1-1',
      display_name: 'Origin Fringe Updated',
      risk_band: 'medium',
      pvp_policy: 'contested',
    });
    expect(state.visibleEntities).toEqual(base.visibleEntities);
    expect(state.knownLoot).toEqual(base.knownLoot);
    expect(state.selectedTargetID).toBe('old-npc');
    expect(state.movementTarget).toEqual({ x: 9900, y: 5000 });
    expect(state.worldEffects).toEqual(base.worldEffects);
    expect(state.minimap).toEqual(base.minimap);
  });

  test('direct map snapshot event applies map summary, epoch, and filters malformed children', () => {
    const base = {
      ...mapScopedLiveState(),
      mapSubscriptionEpoch: 6,
      currentMap: {
        public_map_key: '2-1',
        display_name: 'Rust Frontier',
        bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
        visible_portals: [],
        safe_zones: [],
      },
    };

    const state = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapSnapshot, {
        map_subscription_epoch: 6,
        public_map_key: '2-1',
        display_name: 'Rust Frontier',
        region: 'Outer Belt',
        risk_band: 'high',
        pvp_policy: 'pvp',
        bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
        visible_portals: [
          { portal_id: 'south_gate', display_name: 'South Gate', position: { x: 5000, y: 9800 }, interaction_radius: 160 },
          { portal_id: 'missing-position', display_name: 'Invalid Gate', interaction_radius: 160 },
          { portal_id: 'zero-radius', position: { x: 10, y: 20 }, interaction_radius: 0 },
        ],
        safe_zones: [
          {
            safe_area_id: 'station-beta',
            display_name: 'Station Beta',
            center: { x: 800, y: 900 },
            radius: 600,
            blocks_pvp: true,
            hangar_actions: true,
          },
          { safe_area_id: 'missing-flags', center: { x: 100, y: 100 }, radius: 300 },
          { safe_area_id: 'negative-radius', center: { x: 120, y: 140 }, radius: -1, blocks_pvp: true, hangar_actions: true },
        ],
      }, 3),
    });

    expect(state.mapSubscriptionEpoch).toBe(6);
    expect(state.currentMap).toMatchObject({
      public_map_key: '2-1',
      display_name: 'Rust Frontier',
      region: 'Outer Belt',
      risk_band: 'high',
      pvp_policy: 'pvp',
      bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
    });
    expect(state.currentMap?.visible_portals).toEqual([
      { portal_id: 'south_gate', display_name: 'South Gate', position: { x: 5000, y: 9800 }, interaction_radius: 160 },
    ]);
    expect(state.currentMap?.safe_zones).toEqual([
      {
        safe_area_id: 'station-beta',
        display_name: 'Station Beta',
        center: { x: 800, y: 900 },
        radius: 600,
        blocks_pvp: true,
        hangar_actions: true,
      },
    ]);
    expect(state.visibleEntities).toEqual(base.visibleEntities);
    expect(state.minimap).toEqual(base.minimap);
  });

  test('epoch-only newer map snapshot clears stale live state and current map', () => {
    const base = mapScopedLiveState();
    const state = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapSnapshot, { map_subscription_epoch: 7 }, 4),
    });

    expect(state.mapSubscriptionEpoch).toBe(7);
    expect(state.currentMap).toBeNull();
    expectMapScopedLiveStateCleared(state);
  });

  test('map changed clears old live state and applies destination map summary', () => {
    const state = reduceClientState(mapScopedLiveState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapChanged, {
        map_subscription_epoch: 2,
        map: {
          public_map_key: '1-2',
          display_name: 'Outer Ring',
          region: 'Origin Belt',
          risk_band: 'medium',
          pvp_policy: 'contested',
          bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
          visible_portals: [],
          safe_zones: [],
        },
      }, 5),
    });

    expect(state.mapSubscriptionEpoch).toBe(2);
    expect(state.currentMap).toMatchObject({ public_map_key: '1-2', display_name: 'Outer Ring' });
    expectMapScopedLiveStateCleared(state);
    expect(state.lastServerTime).toBe(1005);
    expect(state.lastSequence).toBe(5);
  });

  test('map changed without destination map summary still clears origin state and applies epoch', () => {
    const state = reduceClientState(mapScopedLiveState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapChanged, { map_subscription_epoch: 2, reason: 'portal_handoff' }, 6),
    });

    expect(state.mapSubscriptionEpoch).toBe(2);
    expect(state.currentMap).toBeNull();
    expectMapScopedLiveStateCleared(state);
    expect(state.lastServerTime).toBe(1006);
    expect(state.lastSequence).toBe(6);
  });

  test('portal cooldown events update local cooldown state without creating portal data', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.portalCooldownStarted, {
        portal_id: 'east_gate',
        cooldown_ready_at_ms: 2400,
        map_subscription_epoch: 1,
      }),
    });

    expect(state.portalCooldowns).toEqual({ east_gate: 2400 });
    expect(state.currentMap).toBeNull();
  });

  test('map policy update applies public policy without clearing live state', () => {
    const base = mapScopedLiveState();
    const state = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapPolicyUpdated, {
        map_subscription_epoch: 1,
        public_map_key: '1-1',
        display_name: 'Spoofed Name',
        region: 'Outer Belt',
        risk_band: 'high',
        pvp_policy: 'pvp',
        visual_theme_key: 'red_frontier',
        safe_zone: { inside: true, blocks_pvp: true, protection_expires_at: 2500 },
        protection: { reason: 'portal_spawn', expires_at: 2500, blocks_pvp: true, break_on_pvp_action: true },
        safe_zones: [
          {
            safe_area_id: 'dock-ring',
            display_name: 'Dock Ring',
            center: { x: 500, y: 600 },
            radius: 300,
            blocks_pvp: true,
            hangar_actions: true,
          },
          { safe_area_id: 'bad-zone', center: { x: 1, y: 2 }, radius: -1, blocks_pvp: true, hangar_actions: true },
        ],
      }, 7),
    });

    expect(state.currentMap).toMatchObject({
      public_map_key: '1-1',
      display_name: 'Origin Fringe',
      region: 'Outer Belt',
      risk_band: 'high',
      pvp_policy: 'pvp',
      visual_theme_key: 'red_frontier',
      safe_zone: { inside: true, blocks_pvp: true, protection_expires_at: 2500 },
      protection: { reason: 'portal_spawn', expires_at: 2500, blocks_pvp: true, break_on_pvp_action: true },
    });
    expect(state.currentMap?.bounds).toEqual(base.currentMap?.bounds);
    expect(state.currentMap?.visible_portals).toEqual(base.currentMap?.visible_portals);
    expect(state.currentMap?.safe_zones).toEqual([
      {
        safe_area_id: 'dock-ring',
        display_name: 'Dock Ring',
        center: { x: 500, y: 600 },
        radius: 300,
        blocks_pvp: true,
        hangar_actions: true,
      },
    ]);
    expect(state.minimap?.safe_zones).toEqual(state.currentMap?.safe_zones);
    expect(state.minimap?.live_contacts).toEqual(base.minimap?.live_contacts);
    expect(state.minimap?.bounds).toEqual(base.minimap?.bounds);
    expect(state.minimap?.visible_portals).toEqual(base.minimap?.visible_portals);
    expect(state.minimap?.remembered).toEqual(base.minimap?.remembered);
    expect(state.visibleEntities).toEqual(base.visibleEntities);
    expect(state.knownLoot).toEqual(base.knownLoot);
    expect(state.selectedTargetID).toBe('old-npc');
    expect(state.movementTarget).toEqual(base.movementTarget);
    expect(state.portalCooldowns).toEqual(base.portalCooldowns);
    expect(state.mapTransfer).toEqual(base.mapTransfer);
    expect(state.lastServerTime).toBe(1007);
    expect(state.lastSequence).toBe(7);
  });

  test('nested map policy update clears viewer policy summaries without changing map truth', () => {
    const base = {
      ...mapScopedLiveState(),
      currentMap: {
        ...mapScopedLiveState().currentMap!,
        safe_zone: { inside: true, blocks_pvp: true, protection_expires_at: 3000 },
        protection: { reason: 'portal_spawn', expires_at: 3000, blocks_pvp: true, break_on_pvp_action: true },
        safe_zones: [
          {
            safe_area_id: 'dock-ring',
            display_name: 'Dock Ring',
            center: { x: 500, y: 600 },
            radius: 300,
            blocks_pvp: true,
            hangar_actions: true,
          },
        ],
      },
      minimap: {
        ...mapScopedLiveState().minimap!,
        safe_zones: [
          {
            safe_area_id: 'dock-ring',
            display_name: 'Dock Ring',
            center: { x: 500, y: 600 },
            radius: 300,
            blocks_pvp: true,
            hangar_actions: true,
          },
        ],
      },
    };

    const state = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapPolicyUpdated, {
        map_subscription_epoch: 1,
        map: {
          public_map_key: '1-1',
          display_name: 'Spoofed Name',
          bounds: { min_x: -100, min_y: -100, max_x: 20000, max_y: 20000 },
          visible_portals: [{ portal_id: 'fake_gate', position: { x: 10, y: 20 }, interaction_radius: 999 }],
          safe_zone: null,
          protection: null,
          safe_zones: [],
        },
      }, 8),
    });

    expect(state.currentMap?.safe_zone).toBeUndefined();
    expect(state.currentMap?.protection).toBeUndefined();
    expect(state.currentMap?.safe_zones).toEqual([]);
    expect(state.minimap?.safe_zones).toEqual([]);
    expect(state.currentMap?.display_name).toBe('Origin Fringe');
    expect(state.currentMap?.bounds).toEqual(base.currentMap?.bounds);
    expect(state.currentMap?.visible_portals).toEqual(base.currentMap?.visible_portals);
  });

  test('malformed map policy summaries are ignored without clearing existing summaries', () => {
    const base = {
      ...mapScopedLiveState(),
      currentMap: {
        ...mapScopedLiveState().currentMap!,
        safe_zone: { inside: true, blocks_pvp: true, protection_expires_at: 3000 },
        protection: { reason: 'portal_spawn', expires_at: 3000, blocks_pvp: true, break_on_pvp_action: true },
      },
    };

    const state = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapPolicyUpdated, {
        map_subscription_epoch: 1,
        public_map_key: '1-1',
        safe_zone: { inside: true },
        protection: { reason: 'portal_spawn' },
      }, 9),
    });

    expect(state.currentMap?.safe_zone).toEqual(base.currentMap?.safe_zone);
    expect(state.currentMap?.protection).toEqual(base.currentMap?.protection);
  });

  test('map policy update without current map does not invent map state', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapPolicyUpdated, {
        map_subscription_epoch: 1,
        public_map_key: '1-1',
        risk_band: 'high',
        pvp_policy: 'pvp',
      }, 10),
    });

    expect(state.currentMap).toBeNull();
    expect(state.minimap).toBeNull();
    expect(state.lastServerTime).toBe(1010);
    expect(state.lastSequence).toBe(10);
  });

  test('stale map policy update is ignored by subscription epoch guard', () => {
    const base = {
      ...mapScopedLiveState(),
      mapSubscriptionEpoch: 2,
    };

    const state = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapPolicyUpdated, {
        map_subscription_epoch: 1,
        public_map_key: '1-1',
        risk_band: 'high',
      }, 11),
    });

    expect(state).toBe(base);
  });

  test('map policy update without subscription epoch does not mutate current map', () => {
    const base = mapScopedLiveState();
    const state = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapPolicyUpdated, {
        public_map_key: '1-1',
        risk_band: 'high',
        pvp_policy: 'pvp',
      }, 12),
    });

    expect(state.currentMap).toEqual(base.currentMap);
    expect(state.minimap).toEqual(base.minimap);
    expect(state.lastServerTime).toBe(1012);
    expect(state.lastSequence).toBe(12);
  });

  test('map policy update ignores mismatched public or map keys while advancing event clock', () => {
    const base = {
      ...mapScopedLiveState(),
      currentMap: { ...mapScopedLiveState().currentMap!, map_key: 'origin-internal-public' },
    };

    const publicMismatch = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapPolicyUpdated, {
        map_subscription_epoch: 1,
        public_map_key: '1-2',
        risk_band: 'high',
      }, 13),
    });
    const mapKeyMismatch = reduceClientState(base, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapPolicyUpdated, {
        map_subscription_epoch: 1,
        map_key: 'other-map-key',
        risk_band: 'high',
      }, 14),
    });

    expect(publicMismatch.currentMap?.risk_band).toBe('low');
    expect(publicMismatch.lastSequence).toBe(13);
    expect(mapKeyMismatch.currentMap?.risk_band).toBe('low');
    expect(mapKeyMismatch.lastSequence).toBe(14);
  });

  test('display name mismatch alone does not block map policy update or mutate display name', () => {
    const state = reduceClientState(mapScopedLiveState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapPolicyUpdated, {
        map_subscription_epoch: 1,
        map: {
          display_name: 'Spoofed Name',
          risk_band: 'medium',
        },
      }, 15),
    });

    expect(state.currentMap?.display_name).toBe('Origin Fringe');
    expect(state.currentMap?.risk_band).toBe('medium');
  });

  test('failed move response clears speculative target marker back to authoritative movement', () => {
    const state = reduceClientState(
      {
        ...createInitialState(),
        movementTarget: { x: 9000, y: 0 },
        pendingCommands: { 'move-request': { requestID: 'move-request', op: 'move_to', queuedAt: 1 } },
        visibleEntities: {
          'player-local': {
            entity_id: 'player-local',
            entity_type: 'player',
            position: { x: 0, y: 0 },
            status_flags: ['self'],
            movement: {
              moving: true,
              origin: { x: 0, y: 0 },
              target: { x: 100, y: 0 },
              speed: 180,
              started_at_ms: 1000,
              arrive_at_ms: 1556,
            },
          },
        },
      },
      {
        type: 'responseReceived',
        envelope: {
          request_id: 'move-request',
          ok: false,
          error: { code: 'ERR_OUT_OF_RANGE', message: 'Move target is out of range.', retryable: false },
          server_time: 1002,
          v: 1,
        },
      },
    );

    expect(state.pendingCommands['move-request']).toBeUndefined();
    expect(state.movementTarget).toEqual({ x: 100, y: 0 });
  });
});

function mapScopedLiveState(): ClientState {
  return {
    ...createInitialState(),
    mapSubscriptionEpoch: 1,
    mapTransfer: {
      state: 'started',
      portal_id: 'east_gate',
      from_public_map_key: '1-1',
      to_public_map_key: '1-2',
      started_at: 500,
    },
    currentMap: {
      public_map_key: '1-1',
      display_name: 'Origin Fringe',
      region: 'Origin Belt',
      risk_band: 'low',
      pvp_policy: 'pve',
      bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
      visible_portals: [{ portal_id: 'east_gate', display_name: 'East Gate', position: { x: 9800, y: 5000 }, interaction_radius: 160 }],
      safe_zones: [],
    },
    portalCooldowns: { east_gate: 1500 },
    minimap: {
      public_map_key: '1-1',
      bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
      visible_portals: [{ portal_id: 'east_gate', display_name: 'East Gate', position: { x: 9800, y: 5000 }, interaction_radius: 160 }],
      safe_zones: [],
      radar_range: 420,
      projection_window_size: 840,
      live_contacts: [{ entity_id: 'old-npc', entity_type: 'npc', position: { x: 9800, y: 5000 } }],
      remembered: [{ kind: 'known_planet', label: 'Old Intel', position: { x: 1, y: 2 }, freshness: 'known' }],
    },
    visibleEntities: {
      'old-npc': { entity_id: 'old-npc', entity_type: 'npc', position: { x: 9800, y: 5000 } },
      'player-self': {
        entity_id: 'player-self',
        entity_type: 'player',
        position: { x: 9700, y: 5000 },
        status_flags: ['self'],
      },
    },
    selectedTargetID: 'old-npc',
    movementTarget: { x: 9900, y: 5000 },
    lastCorrection: { entityID: 'player-self', position: { x: 9700, y: 5000 } },
    knownLoot: { 'old-drop': { drop_id: 'old-drop', item_id: 'ore', quantity: 1 } },
    worldEffects: [{ id: 'effect-old', kind: 'damage', targetID: 'old-npc', createdAt: 1, expiresAt: 999999 }],
    skillCooldowns: { basic_laser: 3000 },
    scanMode: { enabled: true, nextPulseAt: 2000, lastRejectedAt: null, lastError: null },
    planetIntel: {
      knownSignals: 1,
      staleIntel: 0,
      ownedPlanets: 0,
      planets: [],
      selectedPlanet: null,
      lastScan: { pulse_reference: 'pulse-old', status: 'no_signal' },
    },
  };
}

function expectMapScopedLiveStateCleared(state: ClientState): void {
  expect(state.mapTransfer).toBeNull();
  expect(state.portalCooldowns).toEqual({});
  expect(state.visibleEntities).toEqual({});
  expect(state.selectedTargetID).toBeNull();
  expect(state.movementTarget).toBeNull();
  expect(state.lastCorrection).toBeNull();
  expect(state.knownLoot).toEqual({});
  expect(state.worldEffects).toEqual([]);
  expect(state.skillCooldowns).toEqual({});
  expect(state.scanMode.enabled).toBe(false);
  expect(state.planetIntel?.knownSignals).toBe(0);
  expect(state.planetIntel?.selectedPlanet).toBeNull();
  expect(state.planetIntel?.lastScan).toBeNull();
  expect(state.minimap?.public_map_key).toBeUndefined();
  expect(state.minimap?.bounds).toBeUndefined();
  expect(state.minimap?.visible_portals).toEqual([]);
  expect(state.minimap?.safe_zones).toEqual([]);
  expect(state.minimap?.live_contacts).toEqual([]);
}

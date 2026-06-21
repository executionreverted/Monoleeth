import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';
import type { ClientState } from './types';

describe('reduceClientState world transfer lifecycle', () => {
  test('ignores stale map-scoped events from old subscription epochs', () => {
    const state = {
      ...createInitialState(),
      mapSubscriptionEpoch: 2,
      minimap: { radar_range: 1000, projection_window_size: 2000, live_contacts: [], remembered: [] },
    };

    const stale = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'old-map-npc',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
        map_subscription_epoch: 1,
      }),
    });
    expect(stale.visibleEntities).toEqual({});
    expect(stale.minimap?.live_contacts).toEqual([]);
    expect(stale.lastSequence).toBe(0);

    const current = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'current-map-npc',
        entity_type: 'npc',
        position: { x: 30, y: 40 },
        map_subscription_epoch: 2,
      }),
    });
    expect(current.visibleEntities['current-map-npc']).toMatchObject({ position: { x: 30, y: 40 } });
    expect(current.minimap?.live_contacts.map((contact) => contact.entity_id)).toEqual(['current-map-npc']);
  });

  test('map transfer completion clears origin live state and applies destination snapshot', () => {
    const state = {
      ...createInitialState(),
      mapSubscriptionEpoch: 1,
      mapTransfer: {
        state: 'started' as const,
        portal_id: 'east_gate',
        from_public_map_key: '1-1',
        to_public_map_key: '1-2',
        started_at: 1000,
      },
      sector: { sector_key: '1-1', name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
      minimap: {
        radar_range: 420,
        projection_window_size: 840,
        live_contacts: [{ entity_id: 'old-npc', entity_type: 'npc' as const, position: { x: 9800, y: 5000 } }],
        remembered: [{ kind: 'known_planet', label: 'Old Intel', position: { x: 1, y: 2 }, freshness: 'known' }],
      },
      visibleEntities: {
        'old-npc': { entity_id: 'old-npc', entity_type: 'npc' as const, position: { x: 9800, y: 5000 } },
        'player-self': {
          entity_id: 'player-self',
          entity_type: 'player' as const,
          position: { x: 9800, y: 5000 },
          status_flags: ['self'],
        },
      },
      selectedTargetID: 'old-npc',
      movementTarget: { x: 9900, y: 5000 },
      knownLoot: { 'old-drop': { drop_id: 'old-drop', item_id: 'ore', quantity: 1 } },
      worldEffects: [{ id: 'effect-old', kind: 'damage' as const, targetID: 'old-npc', createdAt: 1, expiresAt: 999999 }],
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

    const transferred = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapTransferCompleted, {
        portal_id: 'east_gate',
        from_public_map_key: '1-1',
        to_public_map_key: '1-2',
        position: { x: 400, y: 5000 },
        map_subscription_epoch: 2,
        snapshot: {
          map_subscription_epoch: 2,
          sector: { sector_key: '1-2', name: 'Outer Ring', region: 'Origin Belt', danger: 'low', contested: false },
          map: {
            map_key: '1-2',
            public_map_key: '1-2',
            display_name: 'Outer Ring',
            region: 'Origin Belt',
            risk_band: 'low',
            pvp_policy: 'pve',
            bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
            visible_portals: [],
          },
          entities: [
            {
              entity_id: 'player-self',
              entity_type: 'player',
              position: { x: 400, y: 5000 },
              status_flags: ['self'],
            },
            {
              entity_id: 'destination-npc',
              entity_type: 'npc',
              position: { x: 450, y: 5000 },
              display: { label: 'Outer Drone', disposition: 'hostile' },
            },
          ],
          minimap: {
            radar_range: 420,
            projection_window_size: 840,
            live_contacts: [
              { entity_id: 'player-self', entity_type: 'player', position: { x: 400, y: 5000 }, status_flags: ['self'] },
              { entity_id: 'destination-npc', entity_type: 'npc', position: { x: 450, y: 5000 }, disposition: 'hostile' },
            ],
            remembered: [],
          },
          snapshot_cursor: 12,
        },
      }, 7),
    });

    expect(transferred.mapSubscriptionEpoch).toBe(2);
    expect(transferred.mapTransfer).toBeNull();
    expect(transferred.sector).toMatchObject({ sector_key: '1-2', name: 'Outer Ring' });
    expect(Object.keys(transferred.visibleEntities).sort()).toEqual(['destination-npc', 'player-self']);
    expect(transferred.visibleEntities['old-npc']).toBeUndefined();
    expect(transferred.selectedTargetID).toBeNull();
    expect(transferred.movementTarget).toBeNull();
    expect(transferred.knownLoot).toEqual({});
    expect(transferred.worldEffects).toEqual([]);
    expect(transferred.skillCooldowns).toEqual({});
    expect(transferred.scanMode.enabled).toBe(false);
    expect(transferred.planetIntel?.knownSignals).toBe(0);
    expect(transferred.planetIntel?.lastScan).toBeNull();
    expect(transferred.minimap?.live_contacts.map((contact) => contact.entity_id).sort()).toEqual([
      'destination-npc',
      'player-self',
    ]);
  });

  test('portal enter response clears origin live state and applies nested destination snapshot', () => {
    const transferred = reduceClientState(originPortalResponseState(), {
      type: 'responseReceived',
      envelope: portalEnterResponseEnvelope(),
    });

    expectDestinationTransferState(transferred);
    expect(transferred.pendingCommands['portal-request']).toBeUndefined();
    expect(transferred.lastServerTime).toBe(2000);
    expect(transferred.commandLog.at(-1)?.text).toBe('Accepted portal-request.');
  });

  test('portal enter response without valid nested snapshot does not clear origin live state', () => {
    for (const payload of [
      partialTopLevelDestinationPayload(),
      { ...partialTopLevelDestinationPayload(), snapshot: 'not-a-snapshot-object' },
      { ...partialTopLevelDestinationPayload(), snapshot: partialNestedDestinationSnapshot() },
    ]) {
      const responseApplied = reduceClientState(originPortalResponseState(), {
        type: 'responseReceived',
        envelope: {
          ...portalEnterResponseEnvelope(),
          payload,
        },
      });

      expectOriginLiveStatePreserved(responseApplied);
      expect(responseApplied.pendingCommands['portal-request']).toBeUndefined();
      expect(responseApplied.lastServerTime).toBe(2000);
      expect(responseApplied.commandLog.at(-1)?.text).toBe('Accepted portal-request.');
    }
  });

  test('transfer completed event without valid nested snapshot does not clear origin live state', () => {
    const originState: ClientState = {
      ...originPortalResponseState(),
      pendingCommands: {},
      mapTransfer: {
        state: 'started',
        portal_id: 'east_gate',
        from_public_map_key: '1-1',
        to_public_map_key: '1-2',
        started_at: 1000,
      },
    };

    for (const payload of [
      partialTopLevelDestinationPayload(),
      { ...partialTopLevelDestinationPayload(), snapshot: 'not-a-snapshot-object' },
      { ...partialTopLevelDestinationPayload(), snapshot: partialNestedDestinationSnapshot() },
    ]) {
      const completed = reduceClientState(originState, {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.mapTransferCompleted, payload, 8),
      });

      expectOriginLiveStatePreserved(completed);
      expect(completed.mapTransfer).toEqual(originState.mapTransfer);
      expect(completed.lastSequence).toBe(8);
      expect(completed.lastServerTime).toBe(1008);
    }
  });

  test('portal response followed by transfer lifecycle events does not regress destination truth', () => {
    const responseApplied = reduceClientState(originPortalResponseState(), {
      type: 'responseReceived',
      envelope: portalEnterResponseEnvelope(),
    });
    expectDestinationTransferState(responseApplied);

    const afterStarted = reduceClientState(responseApplied, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapTransferStarted, {
        portal_id: 'east_gate',
        from_public_map_key: '1-1',
        to_public_map_key: '1-2',
        map_subscription_epoch: 1,
      }, 4),
    });
    expectDestinationTransferState(afterStarted);
    expect(afterStarted.mapTransfer).toBeNull();
    expect(afterStarted.lastSequence).toBe(4);

    const afterCompleted = reduceClientState(afterStarted, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.mapTransferCompleted, {
        portal_id: 'east_gate',
        from_public_map_key: '1-1',
        to_public_map_key: '1-2',
        position: { x: 400, y: 5000 },
        map_subscription_epoch: 2,
        snapshot: destinationTransferSnapshot(),
      }, 5),
    });

    expectDestinationTransferState(afterCompleted);
    expect(afterCompleted.mapTransfer).toBeNull();
    expect(afterCompleted.lastSequence).toBe(5);
  });

  test('new realtime stream statuses reset event sequence cursor', () => {
    const staleState = {
      ...createInitialState(),
      connectionStatus: 'connected' as const,
      lastSequence: 42,
    };
    const reconnecting = reduceClientState(staleState, {
      type: 'connectionChanged',
      status: 'reconnecting',
    });
    const ready = reduceClientState(reconnecting, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.sessionReady, { authenticated: true }, 1),
    });

    expect(reconnecting.lastSequence).toBe(0);
    expect(ready.lastSequence).toBe(1);
    expect(ready.connectionStatus).toBe('connected');
    expect(ready.auth.session?.authenticated).toBe(true);
  });

  test('reconnect bootstrap accepts fresh world snapshot and replaces stale visible state', () => {
    const staleState = {
      ...createInitialState(),
      auth: {
        mode: 'real' as const,
        session: {
          authenticated: true,
          account: { email: 'pilot@example.com', admin: false },
          player: { callsign: 'Pilot' },
          server_time: 1000,
        },
        submitting: false,
        error: null,
      },
      connectionStatus: 'connected' as const,
      lastSequence: 42,
      visibleEntities: {
        'stale-npc': {
          entity_id: 'stale-npc',
          entity_type: 'npc' as const,
          position: { x: 999, y: 999 },
        },
      },
      selectedTargetID: 'stale-npc',
      movementTarget: { x: 900, y: 900 },
    };
    const reconnecting = reduceClientState(staleState, {
      type: 'connectionChanged',
      status: 'reconnecting',
    });
    const bootstrapped = reduceClientState(reconnecting, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.worldSnapshot,
        {
          sector: { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
          entities: [
            {
              entity_id: 'player-local',
              entity_type: 'player',
              position: { x: 10, y: 20 },
              status_flags: ['self'],
            },
          ],
          minimap: { radar_range: 420, live_contacts: [], remembered: [] },
        },
        1,
      ),
    });

    expect(reconnecting.lastSequence).toBe(0);
    expect(bootstrapped.connectionStatus).toBe('connected');
    expect(bootstrapped.lastSequence).toBe(1);
    expect(bootstrapped.visibleEntities['stale-npc']).toBeUndefined();
    expect(bootstrapped.visibleEntities['player-local'].position).toEqual({ x: 10, y: 20 });
    expect(bootstrapped.selectedTargetID).toBeNull();
    expect(bootstrapped.movementTarget).toBeNull();
  });
});

function originPortalResponseState(): ClientState {
  return {
    ...createInitialState(),
    mapSubscriptionEpoch: 1,
    sector: { sector_key: '1-1', name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
    minimap: {
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
        position: { x: 9800, y: 5000 },
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
    pendingCommands: {
      'portal-request': { requestID: 'portal-request', op: OPERATIONS.portalEnter, queuedAt: 1 },
    },
  };
}

function destinationTransferSnapshot() {
  return {
    map_subscription_epoch: 2,
    sector: { sector_key: '1-2', name: 'Outer Ring', region: 'Origin Belt', danger: 'low', contested: false },
    map: {
      map_key: '1-2',
      public_map_key: '1-2',
      display_name: 'Outer Ring',
      region: 'Origin Belt',
      risk_band: 'low',
      pvp_policy: 'pve',
      bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
      visible_portals: [],
    },
    entities: [
      {
        entity_id: 'player-self',
        entity_type: 'player',
        position: { x: 400, y: 5000 },
        status_flags: ['self'],
      },
      {
        entity_id: 'destination-npc',
        entity_type: 'npc',
        position: { x: 450, y: 5000 },
        display: { label: 'Outer Drone', disposition: 'hostile' },
      },
    ],
    minimap: {
      radar_range: 420,
      projection_window_size: 840,
      live_contacts: [
        { entity_id: 'player-self', entity_type: 'player', position: { x: 400, y: 5000 }, status_flags: ['self'] },
        { entity_id: 'destination-npc', entity_type: 'npc', position: { x: 450, y: 5000 }, disposition: 'hostile' },
      ],
      remembered: [],
    },
    snapshot_cursor: 12,
  };
}

function portalEnterResponseEnvelope() {
  return {
    request_id: 'portal-request',
    ok: true as const,
    server_time: 2000,
    v: 1,
    payload: {
      accepted: true,
      portal_id: 'east_gate',
      from_public_map_key: '1-1',
      to_public_map_key: '1-2',
      position: { x: 400, y: 5000 },
      map_subscription_epoch: 2,
      snapshot: destinationTransferSnapshot(),
    },
  };
}

function partialTopLevelDestinationPayload() {
  return {
    accepted: true,
    portal_id: 'east_gate',
    from_public_map_key: '1-1',
    to_public_map_key: '1-2',
    position: { x: 400, y: 5000 },
    map_subscription_epoch: 2,
    sector: { sector_key: '1-2', name: 'Outer Ring', region: 'Origin Belt', danger: 'low', contested: false },
    entities: [],
    minimap: {
      radar_range: 420,
      projection_window_size: 840,
      live_contacts: [],
      remembered: [],
    },
  };
}

function partialNestedDestinationSnapshot() {
  return {
    map_subscription_epoch: 2,
    sector: { sector_key: '1-2', name: 'Outer Ring', region: 'Origin Belt', danger: 'low', contested: false },
    minimap: {
      radar_range: 420,
      projection_window_size: 840,
      live_contacts: [],
      remembered: [],
    },
  };
}

function expectOriginLiveStatePreserved(state: ClientState): void {
  expect(state.mapSubscriptionEpoch).toBe(1);
  expect(state.sector).toMatchObject({ sector_key: '1-1', name: 'Origin Fringe' });
  expect(Object.keys(state.visibleEntities).sort()).toEqual(['old-npc', 'player-self']);
  expect(state.visibleEntities['old-npc']).toMatchObject({ position: { x: 9800, y: 5000 } });
  expect(state.selectedTargetID).toBe('old-npc');
  expect(state.movementTarget).toEqual({ x: 9900, y: 5000 });
  expect(state.lastCorrection).toEqual({ entityID: 'player-self', position: { x: 9700, y: 5000 } });
  expect(state.knownLoot).toEqual({ 'old-drop': { drop_id: 'old-drop', item_id: 'ore', quantity: 1 } });
  expect(state.worldEffects.map((effect) => effect.id)).toEqual(['effect-old']);
  expect(state.skillCooldowns).toEqual({ basic_laser: 3000 });
  expect(state.scanMode.enabled).toBe(true);
  expect(state.planetIntel?.knownSignals).toBe(1);
  expect(state.planetIntel?.lastScan).toEqual({ pulse_reference: 'pulse-old', status: 'no_signal' });
  expect(state.minimap?.live_contacts.map((contact) => contact.entity_id)).toEqual(['old-npc']);
}

function expectDestinationTransferState(state: ClientState): void {
  expect(state.mapSubscriptionEpoch).toBe(2);
  expect(state.mapTransfer).toBeNull();
  expect(state.sector).toMatchObject({ sector_key: '1-2', name: 'Outer Ring' });
  expect(Object.keys(state.visibleEntities).sort()).toEqual(['destination-npc', 'player-self']);
  expect(state.visibleEntities['old-npc']).toBeUndefined();
  expect(state.selectedTargetID).toBeNull();
  expect(state.movementTarget).toBeNull();
  expect(state.lastCorrection).toBeNull();
  expect(state.knownLoot).toEqual({});
  expect(state.worldEffects).toEqual([]);
  expect(state.skillCooldowns).toEqual({});
  expect(state.scanMode.enabled).toBe(false);
  expect(state.planetIntel?.knownSignals).toBe(0);
  expect(state.planetIntel?.lastScan).toBeNull();
  expect(state.minimap?.live_contacts.map((contact) => contact.entity_id).sort()).toEqual([
    'destination-npc',
    'player-self',
  ]);
}

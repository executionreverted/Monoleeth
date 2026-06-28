import { expect } from 'vitest';

import { EventEnvelope, JsonObject } from '../protocol/envelope';
import { createInitialState } from './reducer';
import type { ClientState } from './types';

export function event(type: string, payload: JsonObject, seq = 1): EventEnvelope {
  return {
    event_id: `event-${seq}`,
    type,
    payload,
    server_time: 1000 + seq,
    seq,
    v: 1,
  };
}

export function expectServerOwnedGameplayCleared(state: ClientState): void {
  expect(state.lastServerTime).toBeNull();
  expect(state.lastSequence).toBe(0);
  expect(state.mapSubscriptionEpoch).toBeNull();
  expect(state.mapTransfer).toBeNull();
  expect(state.currentMap).toBeNull();
  expect(state.portalCooldowns).toEqual({});
  expect(state.playerSnapshot).toBeNull();
  expect(state.sector).toBeNull();
  expect(state.minimap).toBeNull();
  expect(state.visibleEntities).toEqual({});
  expect(state.selectedTargetID).toBeNull();
  expect(state.movementTarget).toBeNull();
  expect(state.lastCorrection).toBeNull();
  expect(state.knownLoot).toEqual({});
  expect(state.worldEffects).toEqual([]);
  expect(state.combatEngagement).toMatchObject({
    active: false,
    targetID: null,
    skillID: null,
    startedAt: null,
    nextFireAt: null,
    lastStopReason: null,
  });
  expect(state.pendingCommands).toEqual({});
  expect(state.combatLog).toEqual([]);
  expect(state.cargo).toBeNull();
  expect(state.wallet).toBeNull();
  expect(state.ship).toBeNull();
  expect(state.stats).toBeNull();
  expect(state.progression).toBeNull();
  expect(state.inventory).toBeNull();
  expect(state.hangar).toBeNull();
  expect(state.loadout).toBeNull();
  expect(state.crafting).toBeNull();
  expect(state.repairQuote).toBeNull();
  expect(state.skillCooldowns).toEqual({});
  expect(state.questBoard).toBeNull();
  expect(state.planetIntel).toBeNull();
  expect(state.scanMode).toEqual({
    enabled: false,
    nextPulseAt: null,
    lastRejectedAt: null,
    lastError: null,
  });
  expect(state.production).toBeNull();
  expect(state.routes).toBeNull();
  expect(state.contentCatalog).toBeNull();
  expect(state.shopCatalog).toBeNull();
  expect(state.market).toBeNull();
  expect(state.auction).toBeNull();
  expect(state.premium).toBeNull();
  expect(state.economyDashboard).toBeNull();
  expect(state.adminInspection).toBeNull();
  expect(state.adminRepair).toBeNull();
  expect(state.commandLogSummary).toBeNull();
  expect(state.metrics).toBeNull();
  expect(state.releaseGate).toBeNull();
  expect(state.abuseCoverage).toBeNull();
  expect(state.lastError).toBeNull();
}

export function stateWithServerOwnedGameplay(): ClientState {
  return {
    ...createInitialState(),
    auth: {
      mode: 'real',
      session: {
        authenticated: true,
        account: { email: 'pilot@example.com', admin: true },
        player: { callsign: 'Server-Pilot' },
        server_time: 1000,
      },
      submitting: false,
      error: null,
    },
    connectionStatus: 'connected',
    lastServerTime: 1000,
    lastSequence: 42,
    mapSubscriptionEpoch: 7,
    mapTransfer: {
      state: 'started',
      portal_id: 'east_gate',
      from_public_map_key: '1-1',
      to_public_map_key: '1-2',
      started_at: 999,
    },
    currentMap: {
      map_key: '1-1',
      public_map_key: '1-1',
      display_name: 'Origin Fringe',
      region: 'Origin Belt',
      risk_band: 'low',
      pvp_policy: 'pve',
      bounds: { min_x: 0, min_y: 0, max_x: 10000, max_y: 10000 },
      visible_portals: [{ portal_id: 'east_gate', display_name: 'East Gate', position: { x: 9800, y: 5000 }, interaction_radius: 160 }],
      safe_zones: [],
    },
    portalCooldowns: { east_gate: 2000 },
    playerSnapshot: { callsign: 'Server-Pilot', hp: 80, shield: 70, energy: 60 },
    sector: { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
    minimap: { radar_range: 420, live_contacts: [], remembered: [] },
    visibleEntities: {
      'npc-1': {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
        display: { label: 'Training Drone', disposition: 'hostile' },
        combat: { hp: 20, max_hp: 30, shield: 4, max_shield: 10, status: 'hostile' },
      },
    },
    selectedTargetID: 'npc-1',
    movementTarget: { x: 100, y: 100 },
    lastCorrection: { entityID: 'player-1', position: { x: 1, y: 2 } },
    knownLoot: { 'drop-1': { drop_id: 'drop-1', item_id: 'raw_ore', quantity: 3 } },
    worldEffects: [{ id: 'effect-1', kind: 'damage', targetID: 'npc-1', amount: 4, createdAt: 1, expiresAt: 2 }],
    combatEngagement: {
      active: true,
      targetID: 'npc-1',
      skillID: 'basic_laser',
      startedAt: 1000,
      nextFireAt: 1250,
      lastStopReason: null,
    },
    pendingCommands: { 'request-1': { requestID: 'request-1', op: 'move_to', queuedAt: 1 } },
    commandLog: [{ id: 'log-1', level: 'info', text: 'Server log.', at: 1 }],
    combatLog: [{ id: 'combat-1', level: 'info', text: 'Hit Training Drone.', at: 1 }],
    cargo: { used: 3, capacity: 60, items: [{ item_id: 'raw_ore', quantity: 3 }] },
    wallet: { credits: 1200, premium_paid: 300, premium_earned: 0 },
    ship: serverOwnedStub<ClientState['ship']>(),
    stats: serverOwnedStub<ClientState['stats']>(),
    progression: serverOwnedStub<ClientState['progression']>(),
    inventory: serverOwnedStub<ClientState['inventory']>(),
    hangar: serverOwnedStub<ClientState['hangar']>(),
    loadout: serverOwnedStub<ClientState['loadout']>(),
    crafting: serverOwnedStub<ClientState['crafting']>(),
    repairQuote: serverOwnedStub<ClientState['repairQuote']>(),
    skillCooldowns: { basic_laser: 2000 },
    questBoard: serverOwnedStub<ClientState['questBoard']>(),
    planetIntel: serverOwnedStub<ClientState['planetIntel']>(),
    scanMode: {
      enabled: true,
      nextPulseAt: 1234,
      lastRejectedAt: 1200,
      lastError: 'Cooldown.',
    },
    production: serverOwnedStub<ClientState['production']>(),
    routes: serverOwnedStub<ClientState['routes']>(),
    contentCatalog: serverOwnedStub<ClientState['contentCatalog']>(),
    shopCatalog: serverOwnedStub<ClientState['shopCatalog']>(),
    market: serverOwnedStub<ClientState['market']>(),
    auction: serverOwnedStub<ClientState['auction']>(),
    premium: serverOwnedStub<ClientState['premium']>(),
    economyDashboard: serverOwnedStub<ClientState['economyDashboard']>(),
    adminInspection: serverOwnedStub<ClientState['adminInspection']>(),
    adminRepair: serverOwnedStub<ClientState['adminRepair']>(),
    commandLogSummary: serverOwnedStub<ClientState['commandLogSummary']>(),
    metrics: serverOwnedStub<ClientState['metrics']>(),
    releaseGate: serverOwnedStub<ClientState['releaseGate']>(),
    abuseCoverage: serverOwnedStub<ClientState['abuseCoverage']>(),
    lastError: { code: 'server_error', message: 'Server error.', retryable: true },
  };
}

function serverOwnedStub<T>(): NonNullable<T> {
  return { from_server: true } as unknown as NonNullable<T>;
}

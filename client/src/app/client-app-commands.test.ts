import { describe, expect, test } from 'vitest';

import { OPERATIONS, type EntityPayload, type RequestEnvelope, type ServerMessage, type Vec2 } from '../protocol/envelope';
import { createInitialState } from '../state/reducer';
import type { ClientAction, ClientState, WorldMapMemoryMarker } from '../state/types';
import { clampMovementTargetToMapBounds } from './client-app-commands';
import { ClientAppCommands } from './client-app-commands';

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

describe('ClientApp combat attack stance commands', () => {
  test('starts attack stance for the selected visible target', () => {
    const app = new CombatCommandHarness(combatCommandState());

    app.fire();

    expect(app.sent).toHaveLength(1);
    expect(app.sent[0]).toMatchObject({
      op: OPERATIONS.combatStartAttack,
      payload: { target_id: 'npc-1' },
    });
  });

  test('stops attack stance when already attacking the selected target', () => {
    const state = combatCommandState();
    state.combatEngagement = {
      active: true,
      targetID: 'npc-1',
      skillID: 'basic_laser',
      startedAt: 1000,
      nextFireAt: 1200,
      lastStopReason: null,
    };
    const app = new CombatCommandHarness(state);

    app.fire();

    expect(app.sent).toHaveLength(1);
    expect(app.sent[0]).toMatchObject({
      op: OPERATIONS.combatStopAttack,
      payload: {},
    });
  });

  test('blocks attack stance without realtime, target, or an enabled ship', () => {
    for (const state of [
      { ...combatCommandState(), connectionStatus: 'offline' as const },
      { ...combatCommandState(), selectedTargetID: null },
      { ...combatCommandState(), ship: { ...combatCommandState().ship!, disabled: true } },
    ]) {
      const app = new CombatCommandHarness(state);

      app.fire();

      expect(app.sent).toHaveLength(0);
    }
  });

  test('does not send duplicate start or stop commands while one is pending', () => {
    const pendingStart = combatCommandState();
    pendingStart.pendingCommands = {
      start: { requestID: 'start', op: OPERATIONS.combatStartAttack, queuedAt: 1 },
    };
    const startHarness = new CombatCommandHarness(pendingStart);

    startHarness.fire();

    const pendingStop = combatCommandState();
    pendingStop.combatEngagement = {
      active: true,
      targetID: 'npc-1',
      skillID: 'basic_laser',
      startedAt: 1000,
      nextFireAt: 1200,
      lastStopReason: null,
    };
    pendingStop.pendingCommands = {
      stop: { requestID: 'stop', op: OPERATIONS.combatStopAttack, queuedAt: 1 },
    };
    const stopHarness = new CombatCommandHarness(pendingStop);

    stopHarness.fire();

    expect(startHarness.sent).toHaveLength(0);
    expect(stopHarness.sent).toHaveLength(0);
  });
});

function combatCommandState(): ClientState {
  const state = createInitialState();
  state.connectionStatus = 'connected';
  state.selectedTargetID = 'npc-1';
  state.visibleEntities = {
    'pilot-self': selfEntity(),
    'npc-1': {
      entity_id: 'npc-1',
      entity_type: 'npc',
      position: { x: 120, y: 0 },
      status_flags: ['hostile'],
      display: { label: 'Saimon', disposition: 'hostile' },
      combat: { hp: 60, max_hp: 60, shield: 20, max_shield: 20, status: 'active' },
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
    repair_state: 'ready',
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

function selfEntity(): EntityPayload {
  return {
    entity_id: 'pilot-self',
    entity_type: 'player',
    position: { x: 0, y: 0 },
    status_flags: ['self'],
    display: { label: 'You', disposition: 'self' },
    combat: { hp: 100, max_hp: 100, shield: 50, max_shield: 50, status: 'active' },
  };
}

class CombatCommandHarness extends ClientAppCommands {
  readonly sent: RequestEnvelope[] = [];
  readonly actions: ClientAction[] = [];

  constructor(state: ClientState) {
    super({} as HTMLElement);
    this.state = state;
  }

  fire(): void {
    this.sendBasicSkill();
  }

  protected sendCommand(envelope: RequestEnvelope): boolean {
    if (this.state.connectionStatus !== 'connected') {
      return false;
    }
    this.sent.push(envelope);
    return true;
  }

  protected activateLootTarget(_target: EntityPayload, _source: 'click' | 'action'): void {}
  protected cancelNavigation(): void {}
  protected estimatedServerTime(): number | null {
    return null;
  }
  protected findLocalPlayerID(): string | null {
    return null;
  }
  protected hasPendingOperation(op: string): boolean {
    return Object.values(this.state.pendingCommands).some((pending) => pending.op === op);
  }
  protected scheduleNavigationLoop(_serverNow?: number | null): void {}
  protected selectedTarget(): EntityPayload | null {
    return this.state.selectedTargetID ? this.state.visibleEntities[this.state.selectedTargetID] ?? null : null;
  }
  protected selfEntity(): EntityPayload | null {
    return this.state.visibleEntities['pilot-self'] ?? null;
  }
  protected selfStealthEnabled(): boolean {
    return false;
  }
  protected applyServerMessage(_message: ServerMessage): void {}
  protected dispatch(action: ClientAction): void {
    this.actions.push(action);
  }
  protected handleRealtimeStatus(_status: ClientState['connectionStatus']): void {}
  protected selectEntity(_entityID: string | null): void {}
  protected selectMemoryMarker(_marker: WorldMapMemoryMarker): void {}
  protected handleWorldMoveIntent(_target: Vec2): void {}
}

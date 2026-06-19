import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, EventEnvelope, JsonObject } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';

describe('reduceClientState', () => {
  test('initial state has no fake gameplay values', () => {
    const state = createInitialState();

    expect(state.connectionStatus).toBe('restoring');
    expect(state.playerSnapshot).toBeNull();
    expect(state.sector).toBeNull();
    expect(state.minimap).toBeNull();
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
    expect(state.inventory).toBeNull();
    expect(state.planetIntel).toBeNull();
    expect(state.visibleEntities).toEqual({});
  });

  test('logout and auth expiry clear gameplay state', () => {
    const withGameplay = reduceClientState(
      reduceClientState(createInitialState(), {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.playerSnapshot, {
          callsign: 'Server-Pilot',
          hp: 80,
          shield: 70,
          energy: 60,
        }),
      }),
      {
        type: 'responseReceived',
        envelope: {
          request_id: 'repair-quote',
          ok: true,
          payload: { ship_id: 'starter_ship', cost: 0, currency: 'credits', disabled: true },
          server_time: 1002,
          v: 1,
        },
      },
    );

    const loggedOut = reduceClientState(withGameplay, { type: 'authLoggedOut' });
    expect(loggedOut.connectionStatus).toBe('logged_out');
    expect(loggedOut.playerSnapshot).toBeNull();
    expect(loggedOut.inventory).toBeNull();
    expect(loggedOut.hangar).toBeNull();
    expect(loggedOut.loadout).toBeNull();
    expect(loggedOut.crafting).toBeNull();
    expect(loggedOut.repairQuote).toBeNull();
    expect(loggedOut.skillCooldowns).toEqual({});
    expect(loggedOut.visibleEntities).toEqual({});

    const expired = reduceClientState(withGameplay, { type: 'authExpired', message: 'Session expired.' });
    expect(expired.connectionStatus).toBe('auth_expired');
    expect(expired.playerSnapshot).toBeNull();
    expect(expired.inventory).toBeNull();
    expect(expired.hangar).toBeNull();
    expect(expired.loadout).toBeNull();
    expect(expired.crafting).toBeNull();
    expect(expired.repairQuote).toBeNull();
    expect(expired.auth.error).toBe('Session expired.');
  });

  test('demo mode is explicit and isolated from real auth session state', () => {
    const demo = reduceClientState(createInitialState(), { type: 'demoModeStarted' });

    expect(demo.auth.mode).toBe('demo');
    expect(demo.auth.session).toBeNull();
    expect(demo.playerSnapshot).toBeNull();
    expect(demo.commandLog.some((line) => line.text.includes('Demo mode'))).toBe(true);
  });

  test('handles AOI enter and leave events', () => {
    const state = createInitialState();
    const entered = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
        display: { label: 'Training Drone', disposition: 'hostile' },
      }),
    });

    expect(entered.visibleEntities['npc-1']).toMatchObject({
      entity_id: 'npc-1',
      position: { x: 10, y: 20 },
    });

    const left = reduceClientState(entered, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityLeft, { entity_id: 'npc-1' }, 2),
    });

    expect(left.visibleEntities['npc-1']).toBeUndefined();
  });

  test('server correction updates authoritative entity position and clears local target', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'player-1',
        entity_type: 'player',
        position: { x: 0, y: 0 },
      }),
    });

    const corrected = reduceClientState(
      {
        ...state,
        movementTarget: { x: 100, y: 100 },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.positionCorrected, {
          entity_id: 'player-1',
          position: { x: 12, y: 16 },
        }),
      },
    );

    expect(corrected.visibleEntities['player-1'].position).toEqual({ x: 12, y: 16 });
    expect(corrected.movementTarget).toBeNull();
    expect(corrected.lastCorrection).toEqual({ entityID: 'player-1', position: { x: 12, y: 16 } });
  });

  test('rejects hidden debug payloads before state mutation', () => {
    const state = createInitialState();

    expect(() =>
      reduceClientState(state, {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.entityEntered, {
          entity_id: 'planet-1',
          entity_type: 'planet_signal',
          position: { x: 4, y: 8 },
          internal_metadata: { seed: 'nope' },
        }),
      }),
    ).toThrow(/Forbidden server payload rejected/);
  });

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
    expect(replaced.visibleEntities['signal-1']).toMatchObject({
      entity_type: 'planet_signal',
      position: { x: 50, y: 60 },
    });
    expect(replaced.sector).toMatchObject({ name: 'Origin Fringe', danger: 'low' });
    expect(replaced.minimap?.live_contacts).toHaveLength(1);
    expect(replaced.selectedTargetID).toBeNull();
    expect(replaced.movementTarget).toBeNull();
    expect(replaced.lastCorrection).toBeNull();
    expect(replaced.planetIntel?.knownSignals).toBe(1);
  });

  test('snapshot response rejects hidden debug payloads before state mutation', () => {
    const state = createInitialState();

    expect(() =>
      reduceClientState(state, {
        type: 'responseReceived',
        envelope: {
          request_id: 'snapshot-hidden',
          ok: true,
          payload: {
            entities: [
              {
                entity_id: 'planet-1',
                entity_type: 'planet_signal',
                position: { x: 4, y: 8 },
                internal_metadata: { seed: 'nope' },
              },
            ],
          },
          server_time: 1200,
          v: 1,
        },
      }),
    ).toThrow(/Forbidden server payload rejected/);
  });

  test('snapshot response reconciles player, cargo, wallet, ship, progression, inventory, hangar, loadout, crafting, and stat panels', () => {
    const reconciled = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'snapshot-panels',
        ok: true,
        payload: {
          player: { callsign: 'Server-Pilot', hp: 77, shield: 44, energy: 33, rank: 2 },
          cargo: {
            used: 4,
            capacity: 80,
            items: [{ item_id: 'raw_ore', quantity: 4 }],
          },
          wallet: { credits: 980, premium_paid: 3, premium_earned: 9 },
          ship: {
            active_ship_id: 'starter_ship',
            display_name: 'Starter Hull',
            hull: 88,
            max_hull: 120,
            shield: 42,
            max_shield: 60,
            capacitor: 31,
            max_capacitor: 50,
            disabled: false,
            repair_state: 'active',
          },
          progression: { main_level: 2, main_xp: 175, rank: 2, combat_level: 1, combat_xp: 25 },
          inventory: {
            stackable: [{ item_id: 'raw_ore', display_name: 'Raw Ore', quantity: 3, location: 'ship_cargo' }],
            instances: [],
            counts: { cargo_stacks: 1, storage_stacks: 0, equipped_instances: 0 },
          },
          hangar: {
            active_ship_id: 'starter_ship',
            ships: [
              {
                ship_id: 'starter_ship',
                display_name: 'Sparrow',
                state: 'ready',
                hull: 88,
                max_hull: 120,
                shield: 42,
                max_shield: 60,
                disabled: false,
              },
            ],
          },
          loadout: {
            active_ship_id: 'starter_ship',
            slots: [
              { slot_id: 'offensive_1', slot_type: 'offensive' },
              { slot_id: 'defensive_1', slot_type: 'defensive' },
            ],
          },
          crafting: {
            recipes: [
              {
                recipe_id: 'refined_alloy_batch',
                category: 'processed_material',
                output: { kind: 'item', item_id: 'refined_alloy', quantity: 5, tradeable: true },
                inputs: [{ item_id: 'raw_ore', quantity: 20 }],
                required_credits: 100,
                required_rank: 1,
                required_role_levels: [{ role: 'crafting', level: 1 }],
                required_location_type: 'station',
                craft_duration_ms: 300000,
                repeatable: true,
              },
            ],
            active_jobs: [],
          },
          stats: { speed: 220, radar_range: 510, weapon_range: 280, cargo_capacity: 80 },
        },
        server_time: 1400,
        v: 1,
      },
    });

    expect(reconciled.playerSnapshot?.callsign).toBe('Server-Pilot');
    expect(reconciled.cargo).toMatchObject({ used: 4, capacity: 80 });
    expect(reconciled.cargo?.items).toEqual([{ item_id: 'raw_ore', quantity: 4 }]);
    expect(reconciled.wallet).toEqual({ credits: 980, premium_paid: 3, premium_earned: 9 });
    expect(reconciled.ship).toMatchObject({ active_ship_id: 'starter_ship', hull: 88, capacitor: 31, disabled: false });
    expect(reconciled.playerSnapshot).toMatchObject({ hp: 88, max_hp: 120, shield: 42, energy: 31 });
    expect(reconciled.progression).toMatchObject({ main_level: 2, main_xp: 175, rank: 2, combat_xp: 25 });
    expect(reconciled.inventory?.stackable).toEqual([
      { item_id: 'raw_ore', display_name: 'Raw Ore', quantity: 3, location: 'ship_cargo' },
    ]);
    expect(reconciled.hangar?.active_ship_id).toBe('starter_ship');
    expect(reconciled.loadout?.slots).toHaveLength(2);
    expect(reconciled.crafting?.recipes[0]).toMatchObject({ recipe_id: 'refined_alloy_batch', craft_duration_ms: 300000 });
    expect(reconciled.stats).toMatchObject({ speed: 220, radar_range: 510, weapon_range: 280, cargo_capacity: 80 });
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
          sector: { name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false },
          entities: [
            {
              entity_id: 'player-local',
              entity_type: 'player',
              position: { x: 0, y: 0 },
              status_flags: ['self'],
              display: { label: 'Smoke', disposition: 'self' },
            },
          ],
          minimap: {
            radar_range: 420,
            live_contacts: [
              {
                entity_id: 'player-local',
                entity_type: 'player',
                position: { x: 0, y: 0 },
                disposition: 'self',
                status_flags: ['self'],
              },
            ],
            remembered: [],
          },
        }),
      },
    );

    expect(state.connectionStatus).toBe('connected');
    expect(state.sector).toEqual({ name: 'Origin Fringe', region: 'Origin Belt', danger: 'low', contested: false });
    expect(state.minimap?.radar_range).toBe(420);
    expect(state.visibleEntities['player-local'].status_flags).toContain('self');
  });

  test('snapshot events reconcile cargo, wallet, stats, inventory, hangar, loadout, and crafting independently', () => {
    const withCargo = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.cargoSnapshot, {
        used: 12,
        capacity: 70,
        items: [{ item_id: 'salvage_thread', quantity: 12 }],
      }),
    });
    const withWallet = reduceClientState(withCargo, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.walletSnapshot, { credits: 444, premium_paid: 1, premium_earned: 2 }, 2),
    });
    const withStats = reduceClientState(withWallet, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.statsSnapshot, { speed: 210, radar_range: 500, weapon_range: 275, cargo_capacity: 70 }, 3),
    });
    const withInventory = reduceClientState(withStats, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.inventorySnapshot, {
        stackable: [{ item_id: 'raw_ore', quantity: 3, location: 'ship_cargo' }],
        instances: [],
        counts: { cargo_stacks: 1, storage_stacks: 0, equipped_instances: 0 },
      }, 4),
    });
    const withHangar = reduceClientState(withInventory, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.hangarSnapshot, {
        active_ship_id: 'starter_ship',
        ships: [{ ship_id: 'starter_ship', display_name: 'Sparrow', state: 'ready', hull: 100, max_hull: 100, shield: 100, max_shield: 100, disabled: false }],
      }, 5),
    });
    const withLoadout = reduceClientState(withHangar, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.loadoutSnapshot, {
        active_ship_id: 'starter_ship',
        slots: [{ slot_id: 'offensive_1', slot_type: 'offensive' }],
      }, 6),
    });
    const withCrafting = reduceClientState(withLoadout, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.craftingRecipes, {
        recipes: [
          {
            recipe_id: 'refined_alloy_batch',
            category: 'processed_material',
            output: { kind: 'item', item_id: 'refined_alloy', quantity: 5, tradeable: true },
            inputs: [{ item_id: 'raw_ore', quantity: 20 }],
            required_credits: 100,
            required_rank: 1,
            required_role_levels: [{ role: 'crafting', level: 1 }],
            required_location_type: 'station',
            craft_duration_ms: 300000,
            repeatable: true,
          },
        ],
        active_jobs: [],
      }, 7),
    });

    expect(withCrafting.cargo?.items).toEqual([{ item_id: 'salvage_thread', quantity: 12 }]);
    expect(withCrafting.wallet?.credits).toBe(444);
    expect(withCrafting.stats?.weapon_range).toBe(275);
    expect(withCrafting.inventory?.counts.cargo_stacks).toBe(1);
    expect(withCrafting.hangar?.ships[0].display_name).toBe('Sparrow');
    expect(withCrafting.loadout?.slots[0].slot_type).toBe('offensive');
    expect(withCrafting.crafting?.recipes[0].recipe_id).toBe('refined_alloy_batch');
  });

  test('phase 05 combat, loot, progression, and repair events reconcile server-owned state', () => {
    const withNPC = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 80, y: 0 },
        display: { label: 'Training Drone', disposition: 'hostile' },
        combat: { hp: 40, max_hp: 40, shield: 10, max_shield: 10, status: 'active' },
      }),
    });
    const targeted = reduceClientState({ ...withNPC, selectedTargetID: 'npc-1' }, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.targetUpdated, {
        entity_id: 'npc-1',
        combat: { hp: 0, max_hp: 40, shield: 0, max_shield: 10, status: 'destroyed' },
      }, 2),
    });
    const withCooldown = reduceClientState(targeted, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.combatCooldownStarted, {
        skill_id: 'basic_laser',
        cooldown_ready_at_ms: 9000,
      }, 3),
    });
    const withDamage = reduceClientState(withCooldown, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.combatDamage, {
        target_id: 'npc-1',
        amount: 45,
      }, 4),
    });
    const withLootEntity = reduceClientState(withDamage, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'drop-1',
        entity_type: 'loot',
        position: { x: 80, y: 0 },
        display: { label: 'Raw Ore', disposition: 'neutral' },
      }, 5),
    });
    const afterPickup = reduceClientState(
      {
        ...withLootEntity,
        selectedTargetID: 'drop-1',
      },
      {
        type: 'responseReceived',
        envelope: {
          request_id: 'pickup-1',
          ok: true,
          payload: {
            cargo: { used: 6, capacity: 60, items: [{ item_id: 'raw_ore', quantity: 3 }] },
          },
          server_time: 1006,
          v: 1,
        },
      },
    );
    const withoutLootEntity = reduceClientState(afterPickup, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.lootRemoved, { entity_id: 'drop-1' }, 7),
    });
    const progressed = reduceClientState(withoutLootEntity, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.progressionSnapshot, { main_level: 2, main_xp: 100, rank: 2, combat_xp: 40 }, 8),
    });
    const quoted = reduceClientState(progressed, {
      type: 'responseReceived',
      envelope: {
        request_id: 'quote-1',
        ok: true,
        payload: { ship_id: 'starter_ship', cost: 0, currency: 'credits', disabled: true },
        server_time: 1009,
        v: 1,
      },
    });
    const repaired = reduceClientState(quoted, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.deathRepaired, { ship_id: 'starter_ship' }, 10),
    });

    expect(targeted.visibleEntities['npc-1'].combat).toMatchObject({ hp: 0, shield: 0, status: 'destroyed' });
    expect(withCooldown.skillCooldowns.basic_laser).toBe(9000);
    expect(withDamage.combatLog.at(-1)?.text).toContain('Hit npc-1 for 45.');
    expect(afterPickup.cargo?.items).toEqual([{ item_id: 'raw_ore', quantity: 3 }]);
    expect(withoutLootEntity.visibleEntities['drop-1']).toBeUndefined();
    expect(withoutLootEntity.selectedTargetID).toBeNull();
    expect(progressed.progression).toMatchObject({ main_level: 2, rank: 2, combat_xp: 40 });
    expect(quoted.repairQuote).toEqual({ ship_id: 'starter_ship', cost: 0, currency: 'credits', disabled: true });
    expect(repaired.repairQuote).toBeNull();
  });
});

function event(type: string, payload: JsonObject, seq = 1): EventEnvelope {
  return {
    event_id: `event-${seq}`,
    type,
    payload,
    server_time: 1000 + seq,
    seq,
    v: 1,
  };
}

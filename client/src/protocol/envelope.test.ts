import { describe, expect, test } from 'vitest';

import { CommandBuilder } from './commands';
import { CLIENT_EVENTS, OPERATIONS, parseServerMessage, rejectForbiddenPayloadKeys } from './envelope';

const UNIMPLEMENTED_MUTATION_OPS = [
  'crafting.start',
  'crafting.complete',
  'crafting.cancel',
  'discovery.claim_planet',
  'planet.building_build',
  'planet.building_upgrade',
  'inventory.move',
  'progression.unlock_skill',
  'progression.respec_skills',
  'route.create',
  'route.update',
  'route.enable',
  'route.disable',
  'route.settle',
  'intel.share',
  'intel.coordinate_create',
  'intel.coordinate_use',
  'intel.coordinate_item.create',
  'intel.coordinate_item.use',
  'mail.send',
  'social.friend_request',
  'social.party_invite',
] as const;

describe('parseServerMessage', () => {
  test('parses a successful response envelope', () => {
    const message = parseServerMessage(
      JSON.stringify({
        request_id: 'request-1',
        ok: true,
        payload: { accepted: true },
        server_time: 182736123,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      request_id: 'request-1',
      ok: true,
      payload: { accepted: true },
      server_time: 182736123,
      v: 1,
    });
  });

  test('parses a client-safe event envelope', () => {
    const message = parseServerMessage(
      JSON.stringify({
        event_id: 'event-1',
        type: CLIENT_EVENTS.entityEntered,
        payload: {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 10, y: 20 },
          display: { label: 'Training Drone', disposition: 'hostile' },
        },
        server_time: 182736124,
        seq: 9,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      event_id: 'event-1',
      type: CLIENT_EVENTS.entityEntered,
      payload: {
        entity_id: 'npc-1',
      },
    });
  });

  test('rejects hidden or internal server payload fields', () => {
    expect(() =>
      parseServerMessage(
        JSON.stringify({
          event_id: 'event-hidden',
          type: CLIENT_EVENTS.entityEntered,
          payload: {
            entity_id: 'hidden-planet',
            entity_type: 'planet_signal',
            position: { x: 1, y: 2 },
            gameplay_seed: 'server-only',
          },
          server_time: 1,
          seq: 1,
          v: 1,
        }),
      ),
    ).toThrow(/Forbidden server payload rejected/);
  });

  test('accepts phase 05 combat payloads that do not expose server-only truth fields', () => {
    const message = parseServerMessage(
      JSON.stringify({
        event_id: 'combat-1',
        type: CLIENT_EVENTS.combatDamage,
        payload: {
          target_id: 'npc-1',
          amount: 32,
          shield_amount: 10,
          hull_amount: 22,
        },
        server_time: 182736125,
        seq: 10,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      event_id: 'combat-1',
      type: CLIENT_EVENTS.combatDamage,
      payload: { amount: 32 },
    });
  });

  test('accepts phase 09 public quest payloads without raw generation data', () => {
    const message = parseServerMessage(
      JSON.stringify({
        event_id: 'quest-1',
        type: CLIENT_EVENTS.questProgressed,
        payload: {
          quest_id: 'quest-1',
          quest_type: 'kill',
          title: 'Training Sweep',
          state: 'completed',
          objectives: [{ id: 'kill', kind: 'kill', target: 'pirate', current: 3, required: 3, completed: true }],
          rewards: [{ kind: 'currency', currency_type: 'credits', amount: 100 }],
          accepted_at: 1000,
          completed_at: 2000,
          can_claim: true,
        },
        server_time: 182736126,
        seq: 11,
        v: 1,
      }),
    );

    expect(message).toMatchObject({
      event_id: 'quest-1',
      type: CLIENT_EVENTS.questProgressed,
      payload: { can_claim: true },
    });
  });

  test('rejects phase 09 raw quest internals', () => {
    expect(() =>
      parseServerMessage(
        JSON.stringify({
          event_id: 'quest-hidden',
          type: CLIENT_EVENTS.questBoardGenerated,
          payload: {
            offers: [{ offer_id: 'offer-1', generated_payload: { rare_cap: 99 } }],
          },
          server_time: 1,
          seq: 1,
          v: 1,
        }),
      ),
    ).toThrow(/Forbidden server payload rejected/);
  });

  test('rejects unsupported protocol versions', () => {
    expect(() =>
      parseServerMessage(
        JSON.stringify({
          request_id: 'request-1',
          ok: true,
          payload: {},
          server_time: 1,
          v: 999,
        }),
      ),
    ).toThrow(/Unsupported protocol version/);
  });
});

describe('rejectForbiddenPayloadKeys', () => {
  test('checks nested payloads', () => {
    expect(() => rejectForbiddenPayloadKeys({ entities: [{ future_spawn_data: ['x'] }] })).toThrow(
      /Forbidden server payload rejected/,
    );
  });
});

describe('default outbound operations', () => {
  test('include server-owned hangar and loadout mutation contracts', () => {
    expect(OPERATIONS.hangarActivateShip).toBe('hangar.activate_ship');
    expect(OPERATIONS.loadoutEquipModule).toBe('loadout.equip_module');
    expect(OPERATIONS.loadoutUnequipModule).toBe('loadout.unequip_module');

    const builder = new CommandBuilder();
    expect(builder.hangarActivateShip('starter').payload).toEqual({ ship_id: 'starter' });
    expect(builder.loadoutEquipModule('offensive_1', 'laser_alpha_t1-instance-2').payload).toEqual({
      slot_id: 'offensive_1',
      item_instance_id: 'laser_alpha_t1-instance-2',
    });
    expect(builder.loadoutUnequipModule('offensive_1').payload).toEqual({ slot_id: 'offensive_1' });
  });

  test('exclude unimplemented browser mutation contracts', () => {
    const allowedOperations = new Set<string>(Object.values(OPERATIONS));

    for (const operation of UNIMPLEMENTED_MUTATION_OPS) {
      expect(allowedOperations.has(operation)).toBe(false);
    }
  });

  test('do not expose command-builder helpers for unimplemented browser mutations', () => {
    const builderMethods = new Set(Object.getOwnPropertyNames(CommandBuilder.prototype));
    const forbiddenMethodNames = [
      'craftingStart',
      'craftingComplete',
      'craftingCancel',
      'discoveryClaimPlanet',
      'planetBuildingBuild',
      'planetBuildingUpgrade',
      'inventoryMove',
      'progressionUnlockSkill',
      'progressionRespecSkills',
      'routeCreate',
      'routeUpdate',
      'routeEnable',
      'routeDisable',
      'routeSettle',
      'intelShare',
      'intelCoordinateCreate',
      'intelCoordinateUse',
      'intelCoordinateItemCreate',
      'intelCoordinateItemUse',
      'mailSend',
      'socialFriendRequest',
      'socialPartyInvite',
    ];

    for (const method of forbiddenMethodNames) {
      expect(builderMethods.has(method)).toBe(false);
    }
  });
});

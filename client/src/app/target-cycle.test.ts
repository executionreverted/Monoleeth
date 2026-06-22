import { describe, expect, test } from 'vitest';

import { EntityPayload } from '../protocol/envelope';
import { nextCycleTargetID } from './target-cycle';

const self: EntityPayload = {
  entity_id: 'player-self',
  entity_type: 'player',
  position: { x: 0, y: 0 },
  status_flags: ['self'],
  display: { label: 'Self', disposition: 'self' },
};

function target(entity_id: string, patch: Partial<EntityPayload>): EntityPayload {
  return {
    entity_id,
    entity_type: 'npc',
    position: { x: 0, y: 0 },
    status_flags: ['hostile'],
    display: { label: entity_id, disposition: 'hostile' },
    combat: { hp: 10, max_hp: 10, shield: 0, max_shield: 0 },
    ...patch,
  };
}

function state(selectedTargetID: string | null, entities: EntityPayload[]) {
  return {
    selectedTargetID,
    visibleEntities: Object.fromEntries([self, ...entities].map((entity) => [entity.entity_id, entity])),
    stats: {
      speed: 100,
      radar_range: 1000,
      weapon_range: 320,
      cargo_capacity: 60,
      loot_pickup_range: 120,
      basic_laser_energy_cost: 10,
      basic_laser_cooldown_ms: 800,
    },
  };
}

describe('nextCycleTargetID', () => {
  test('cycles hostile targets by range then id', () => {
    const first = target('npc-near', { position: { x: 80, y: 0 } });
    const second = target('npc-far', { position: { x: 160, y: 0 } });

    expect(nextCycleTargetID(state(null, [second, first]), 1000)).toBe('npc-near');
    expect(nextCycleTargetID(state('npc-near', [second, first]), 1000)).toBe('npc-far');
    expect(nextCycleTargetID(state('npc-far', [second, first]), 1000)).toBe('npc-near');
  });

  test('skips self, nonhostile contacts, loot, planet signals, destroyed, hidden, and out of range', () => {
    const visibleHostile = target('npc-valid', { position: { x: 120, y: 0 } });
    const friendlyNPC = target('npc-friendly', {
      display: { label: 'Friendly', disposition: 'friendly' },
      status_flags: ['friendly'],
      position: { x: 90, y: 0 },
    });
    const loot = target('loot-cache', { entity_type: 'loot', position: { x: 40, y: 0 } });
    const signal = target('signal-1', { entity_type: 'planet_signal', position: { x: 50, y: 0 } });
    const destroyed = target('npc-destroyed', {
      position: { x: 60, y: 0 },
      combat: { hp: 0, max_hp: 10, shield: 0, max_shield: 0, status: 'destroyed' },
    });
    const hidden = target('npc-hidden', { status_flags: ['hostile', 'hidden'], position: { x: 70, y: 0 } });
    const outOfRange = target('npc-out', { position: { x: 900, y: 0 } });

    expect(nextCycleTargetID(state(null, [friendlyNPC, loot, signal, destroyed, hidden, outOfRange, visibleHostile]), 1000)).toBe(
      'npc-valid',
    );
  });

  test('includes visible non-friendly players', () => {
    const visiblePlayer = target('pilot-visible', {
      entity_type: 'player',
      position: { x: 100, y: 0 },
      status_flags: [],
      display: { label: 'Visible Pilot', disposition: 'neutral' },
    });

    expect(nextCycleTargetID(state(null, [visiblePlayer]), 1000)).toBe('pilot-visible');
  });

  test('skips hidden players even if scan-revealed', () => {
    const hidden = target('pilot-hidden', {
      entity_type: 'player',
      position: { x: 100, y: 0 },
      status_flags: ['hostile', 'hidden', 'scan_revealed'],
      display: { label: 'Hidden Pilot', disposition: 'hostile' },
    });

    expect(nextCycleTargetID(state(null, [hidden]), 1000)).toBeNull();
  });

  test('starts at nearest hostile when current selection is not a candidate', () => {
    const loot = target('loot-selected', { entity_type: 'loot', position: { x: 40, y: 0 } });
    const hostile = target('npc-valid', { position: { x: 90, y: 0 } });

    expect(nextCycleTargetID(state('loot-selected', [loot, hostile]), 1000)).toBe('npc-valid');
  });

  test('returns null without self or candidates', () => {
    const hostile = target('npc-valid', { position: { x: 90, y: 0 } });
    const withoutSelf = {
      ...state(null, [hostile]),
      visibleEntities: { [hostile.entity_id]: hostile },
    };

    expect(nextCycleTargetID(withoutSelf, 1000)).toBeNull();
    expect(nextCycleTargetID(state(null, [target('loot-only', { entity_type: 'loot' })]), 1000)).toBeNull();
  });

  test('uses server-timed movement positions for range', () => {
    const movingTarget = target('npc-moving-in', {
      movement: {
        moving: true,
        origin: { x: 500, y: 0 },
        target: { x: 100, y: 0 },
        speed: 100,
        started_at_ms: 1000,
        arrive_at_ms: 5000,
      },
      position: { x: 500, y: 0 },
    });

    expect(nextCycleTargetID(state(null, [movingTarget]), 1000)).toBeNull();
    expect(nextCycleTargetID(state(null, [movingTarget]), 4000)).toBe('npc-moving-in');
  });

  test('includes friendly-projected visible players', () => {
    const friendly = target('pilot-friendly-revealed', {
      entity_type: 'player',
      status_flags: ['friendly', 'scan_revealed'],
      display: { label: 'Friendly', disposition: 'friendly' },
      position: { x: 100, y: 0 },
    });

    expect(nextCycleTargetID(state(null, [friendly]), 1000)).toBe('pilot-friendly-revealed');
  });
});

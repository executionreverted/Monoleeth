import { EntityPayload } from '../protocol/envelope';
import { ClientState } from '../state/types';
import { currentEntityPosition, distanceBetween, selfEntity } from '../state/movement';

type TargetCycleState = Pick<ClientState, 'visibleEntities' | 'selectedTargetID' | 'stats'>;

export function nextCycleTargetID(state: TargetCycleState, serverNow: number | null): string | null {
  const entities = Object.values(state.visibleEntities);
  const self = selfEntity(entities);
  if (!self) {
    return null;
  }

  const weaponRange = state.stats?.weapon_range ?? 0;
  const candidates = entities
    .filter((entity) => isCycleTarget(entity, self, weaponRange, serverNow ?? Date.now()))
    .sort((left, right) => targetDistance(left, self, serverNow ?? Date.now()) - targetDistance(right, self, serverNow ?? Date.now()) || left.entity_id.localeCompare(right.entity_id));
  if (candidates.length === 0) {
    return null;
  }

  const currentIndex = candidates.findIndex((entity) => entity.entity_id === state.selectedTargetID);
  return candidates[(currentIndex + 1) % candidates.length].entity_id;
}

function isCycleTarget(entity: EntityPayload, self: EntityPayload, weaponRange: number, serverNow: number): boolean {
  if (entity.entity_id === self.entity_id || hasAnyFlag(entity, ['self', 'local']) || entity.display?.disposition === 'self') {
    return false;
  }
  if (hasAnyFlag(entity, ['hidden', 'stealthed']) && !hasAnyFlag(entity, ['scan_revealed'])) {
    return false;
  }
  if (entity.combat && (entity.combat.status === 'destroyed' || entity.combat.hp <= 0)) {
    return false;
  }
  if (!isHostileTarget(entity)) {
    return false;
  }
  if (weaponRange > 0 && Number.isFinite(weaponRange) && targetDistance(entity, self, serverNow) > weaponRange) {
    return false;
  }
  return true;
}

function isHostileTarget(entity: EntityPayload): boolean {
  if (entity.entity_type !== 'npc' && entity.entity_type !== 'player') {
    return false;
  }
  if (entity.display?.disposition === 'friendly' || hasAnyFlag(entity, ['friendly'])) {
    return false;
  }
  return entity.display?.disposition === 'hostile' || hasAnyFlag(entity, ['hostile', 'scan_revealed']);
}

function targetDistance(entity: EntityPayload, self: EntityPayload, serverNow: number): number {
  return distanceBetween(currentEntityPosition(self, serverNow), currentEntityPosition(entity, serverNow));
}

function hasAnyFlag(entity: EntityPayload, flags: string[]): boolean {
  const ownFlags = entity.status_flags ?? [];
  return flags.some((flag) => ownFlags.includes(flag));
}

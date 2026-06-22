import { EntityPayload } from '../protocol/envelope';
import { ClientState } from '../state/types';
import { currentEntityPosition, distanceBetween, selfEntity } from '../state/movement';
import { isAttackableVisibleTarget } from '../state/target-eligibility';

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
  if (!isAttackableVisibleTarget(entity, self)) {
    return false;
  }
  if (weaponRange > 0 && Number.isFinite(weaponRange) && targetDistance(entity, self, serverNow) > weaponRange) {
    return false;
  }
  return true;
}

function targetDistance(entity: EntityPayload, self: EntityPayload, serverNow: number): number {
  return distanceBetween(currentEntityPosition(self, serverNow), currentEntityPosition(entity, serverNow));
}

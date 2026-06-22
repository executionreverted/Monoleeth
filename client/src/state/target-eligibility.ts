import type { EntityPayload } from '../protocol/envelope';

export function isAttackableVisibleTarget(target: EntityPayload | null | undefined, self?: EntityPayload | null): boolean {
  if (!target) {
    return false;
  }
  if (target.entity_type !== 'npc' && target.entity_type !== 'player') {
    return false;
  }
  if (isSelfVisibleTarget(target, self) || isHiddenVisibleTarget(target) || isDestroyedVisibleTarget(target)) {
    return false;
  }
  if (target.entity_type === 'npc') {
    return !isFriendlyVisibleTarget(target) && isHostileVisibleTarget(target);
  }
  return true;
}

export function isHostileVisibleTarget(target: EntityPayload): boolean {
  const flags = target.status_flags ?? [];
  return flags.includes('hostile') || flags.includes('scan_revealed') || target.display?.disposition === 'hostile';
}

export function isSelfVisibleTarget(target: EntityPayload, self?: EntityPayload | null): boolean {
  const flags = target.status_flags ?? [];
  return target.entity_id === self?.entity_id || flags.includes('self') || flags.includes('local') || target.display?.disposition === 'self';
}

export function isFriendlyVisibleTarget(target: EntityPayload): boolean {
  const flags = target.status_flags ?? [];
  return flags.includes('friendly') || target.display?.disposition === 'friendly';
}

export function isHiddenVisibleTarget(target: EntityPayload): boolean {
  const flags = target.status_flags ?? [];
  return flags.includes('hidden');
}

export function isDestroyedVisibleTarget(target: EntityPayload): boolean {
  const status = target.combat?.status?.toLowerCase() ?? '';
  return status === 'destroyed' || status === 'disabled' || status === 'dead' || Boolean(target.combat && target.combat.hp <= 0);
}

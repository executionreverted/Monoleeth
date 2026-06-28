import type { EntityPayload } from '../protocol/envelope';
import { isSelfEntity } from '../state/movement';
import type { EntityAssetDirectionCode } from './world-entity-asset-catalog';

export function spriteAlphaForEntity(entity: EntityPayload): number {
  switch (entity.entity_type) {
    case 'player':
      return isSelfEntity(entity) ? 0.64 : 0.5;
    case 'npc':
      return 0.6;
    case 'loot':
      return 0.7;
    case 'planet_signal':
      return 0.66;
  }
}

export function spriteScaleForEntity(entity: EntityPayload, worldScale: number): number {
  const base =
    entity.entity_type === 'player'
      ? 0.18
      : entity.entity_type === 'npc'
        ? 0.16
        : entity.entity_type === 'loot'
          ? 0.14
          : 0.5;
  return base * Math.max(0.86, worldScale);
}

export function spriteDirectionForMovementVector(dx: number, dy: number): EntityAssetDirectionCode | null {
  if (Math.hypot(dx, dy) < 0.001) {
    return null;
  }

  const degrees = (Math.atan2(dy, dx) * 180) / Math.PI;
  const normalized = (degrees + 360) % 360;
  if (normalized < 22.5 || normalized >= 337.5) {
    return '10';
  }
  if (normalized < 67.5) {
    return '12';
  }
  if (normalized < 112.5) {
    return '14';
  }
  if (normalized < 157.5) {
    return '00';
  }
  if (normalized < 202.5) {
    return '02';
  }
  if (normalized < 247.5) {
    return '04';
  }
  if (normalized < 292.5) {
    return '06';
  }
  return '08';
}

export function spriteDirectionForEntity(entity: EntityPayload): EntityAssetDirectionCode | null {
  const movement = entity.movement;
  if (!movement?.moving) {
    return null;
  }
  return spriteDirectionForMovementVector(movement.target.x - movement.origin.x, movement.target.y - movement.origin.y);
}

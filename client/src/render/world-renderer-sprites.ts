import type { EntityPayload } from '../protocol/envelope';
import { isSelfEntity } from '../state/movement';

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

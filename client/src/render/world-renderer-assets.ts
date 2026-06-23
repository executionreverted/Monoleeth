import type { EntityPayload } from '../protocol/envelope';
import type { WorldMapMemoryMarker } from '../state/types';
import type { MapOverlayPortalDebug, MapOverlaySafeZoneDebug } from './map-overlay';

export type WorldRenderAssetKey =
  | 'background.starfield'
  | 'ship.player.self'
  | 'ship.player.friendly'
  | 'ship.player.neutral'
  | 'npc.swarm.hostile'
  | 'loot.cache'
  | 'planet.signal.unknown'
  | 'planet.signal.known'
  | 'planet.memory.owned'
  | 'planet.memory.known'
  | 'portal.gate.visible'
  | 'zone.safe.pvp-blocked'
  | 'zone.safe.warning'
  | 'projectile.laser.basic'
  | 'marker.movement.target'
  | 'marker.selection.target'
  | 'effect.damage'
  | 'effect.loot';

export type WorldRenderAssetLayer = 'background' | 'world' | 'overlay' | 'effect';

export interface WorldRenderAssetDescriptor {
  key: WorldRenderAssetKey;
  layer: WorldRenderAssetLayer;
  accentColor: number;
  glowColor: number;
}

export const WORLD_RENDER_ASSETS: Record<WorldRenderAssetKey, WorldRenderAssetDescriptor> = {
  'background.starfield': { key: 'background.starfield', layer: 'background', accentColor: 0x2bdfff, glowColor: 0x8af5ff },
  'ship.player.self': { key: 'ship.player.self', layer: 'world', accentColor: 0x2bdfff, glowColor: 0x8af5ff },
  'ship.player.friendly': { key: 'ship.player.friendly', layer: 'world', accentColor: 0x44e878, glowColor: 0x8af5ff },
  'ship.player.neutral': { key: 'ship.player.neutral', layer: 'world', accentColor: 0x8af5ff, glowColor: 0x2bdfff },
  'npc.swarm.hostile': { key: 'npc.swarm.hostile', layer: 'world', accentColor: 0xff4236, glowColor: 0xff5c7a },
  'loot.cache': { key: 'loot.cache', layer: 'world', accentColor: 0xf4c95d, glowColor: 0xfff0a8 },
  'planet.signal.unknown': { key: 'planet.signal.unknown', layer: 'world', accentColor: 0xf4c95d, glowColor: 0xfff0a8 },
  'planet.signal.known': { key: 'planet.signal.known', layer: 'world', accentColor: 0x2bdfff, glowColor: 0x8af5ff },
  'planet.memory.owned': { key: 'planet.memory.owned', layer: 'world', accentColor: 0x44e878, glowColor: 0x8af5ff },
  'planet.memory.known': { key: 'planet.memory.known', layer: 'world', accentColor: 0x44e878, glowColor: 0x8af5ff },
  'portal.gate.visible': { key: 'portal.gate.visible', layer: 'overlay', accentColor: 0x8af5ff, glowColor: 0x2bdfff },
  'zone.safe.pvp-blocked': { key: 'zone.safe.pvp-blocked', layer: 'overlay', accentColor: 0x44e878, glowColor: 0x8af5ff },
  'zone.safe.warning': { key: 'zone.safe.warning', layer: 'overlay', accentColor: 0xf4c95d, glowColor: 0xfff0a8 },
  'projectile.laser.basic': { key: 'projectile.laser.basic', layer: 'effect', accentColor: 0xf4c95d, glowColor: 0x2bdfff },
  'marker.movement.target': { key: 'marker.movement.target', layer: 'overlay', accentColor: 0xf4c95d, glowColor: 0xfff0a8 },
  'marker.selection.target': { key: 'marker.selection.target', layer: 'overlay', accentColor: 0x8af5ff, glowColor: 0x2bdfff },
  'effect.damage': { key: 'effect.damage', layer: 'effect', accentColor: 0xff5c7a, glowColor: 0xff4236 },
  'effect.loot': { key: 'effect.loot', layer: 'effect', accentColor: 0xf4c95d, glowColor: 0xfff0a8 },
};

export function worldAssetForEntity(entity: EntityPayload, self = false): WorldRenderAssetDescriptor {
  switch (entity.entity_type) {
    case 'player':
      if (self) {
        return WORLD_RENDER_ASSETS['ship.player.self'];
      }
      return entity.display?.disposition === 'friendly'
        ? WORLD_RENDER_ASSETS['ship.player.friendly']
        : WORLD_RENDER_ASSETS['ship.player.neutral'];
    case 'npc':
      return WORLD_RENDER_ASSETS['npc.swarm.hostile'];
    case 'loot':
      return WORLD_RENDER_ASSETS['loot.cache'];
    case 'planet_signal':
      return isUnknownSignalAsset(entity) ? WORLD_RENDER_ASSETS['planet.signal.unknown'] : WORLD_RENDER_ASSETS['planet.signal.known'];
  }
}

export function worldAssetForPortal(_portal: MapOverlayPortalDebug): WorldRenderAssetDescriptor {
  return WORLD_RENDER_ASSETS['portal.gate.visible'];
}

export function worldAssetForSafeZone(zone: MapOverlaySafeZoneDebug): WorldRenderAssetDescriptor {
  return zone.blocksPVP ? WORLD_RENDER_ASSETS['zone.safe.pvp-blocked'] : WORLD_RENDER_ASSETS['zone.safe.warning'];
}

export function worldAssetForMemoryMarker(marker: WorldMapMemoryMarker): WorldRenderAssetDescriptor {
  return marker.state === 'owned' ? WORLD_RENDER_ASSETS['planet.memory.owned'] : WORLD_RENDER_ASSETS['planet.memory.known'];
}

export function isUnknownSignalAsset(entity: EntityPayload): boolean {
  return (
    entity.entity_type === 'planet_signal' &&
    (entity.status_flags?.includes('unknown_signal') || entity.display?.disposition === 'unknown' || /unknown/i.test(entity.display?.label ?? ''))
  );
}

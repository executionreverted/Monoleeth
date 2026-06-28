import type { EntityPayload } from '../protocol/envelope';
import type { WorldMapMemoryMarker } from '../state/types';
import type { MapOverlayPortalDebug, MapOverlaySafeZoneDebug } from './map-overlay';
import { CURATED_ENTITY_ASSETS, EntityAssetDirectionCode } from './world-entity-asset-catalog';
import damageBurstURL from '../assets/world/damage_burst.svg?url';
import lootSparkURL from '../assets/world/loot_spark.svg?url';
import movementMarkerURL from '../assets/world/movement_marker.svg?url';
import planetKnownURL from '../assets/world/planet_known.svg?url';
import planetUnknownURL from '../assets/world/planet_unknown.svg?url';
import portalGateURL from '../assets/world/portal_gate.svg?url';
import projectileLaserURL from '../assets/world/projectile_laser.svg?url';
import radarWarningURL from '../assets/world/radar_warning.svg?url';
import safeZoneURL from '../assets/world/safe_zone.svg?url';
import selectionReticleURL from '../assets/world/selection_reticle.svg?url';
import starfieldURL from '../assets/starfield_2048x1152.png?url';

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
  assetURL: string;
  directionURLs?: Record<EntityAssetDirectionCode, string>;
  visualRole: string;
  accentColor: number;
  glowColor: number;
}

export const WORLD_RENDER_ASSETS: Record<WorldRenderAssetKey, WorldRenderAssetDescriptor> = {
  'background.starfield': {
    key: 'background.starfield',
    layer: 'background',
    assetURL: starfieldURL,
    visualRole: 'map background',
    accentColor: 0x2bdfff,
    glowColor: 0x8af5ff,
  },
  'ship.player.self': {
    key: 'ship.player.self',
    layer: 'world',
    assetURL: CURATED_ENTITY_ASSETS['player.ship.vanguard'].runtimeURL,
    directionURLs: CURATED_ENTITY_ASSETS['player.ship.vanguard'].directionURLs,
    visualRole: 'player ship',
    accentColor: 0x2bdfff,
    glowColor: 0x8af5ff,
  },
  'ship.player.friendly': {
    key: 'ship.player.friendly',
    layer: 'world',
    assetURL: CURATED_ENTITY_ASSETS['player.ship.vanguard'].runtimeURL,
    directionURLs: CURATED_ENTITY_ASSETS['player.ship.vanguard'].directionURLs,
    visualRole: 'friendly player ship',
    accentColor: 0x44e878,
    glowColor: 0x8af5ff,
  },
  'ship.player.neutral': {
    key: 'ship.player.neutral',
    layer: 'world',
    assetURL: CURATED_ENTITY_ASSETS['player.ship.vanguard'].runtimeURL,
    directionURLs: CURATED_ENTITY_ASSETS['player.ship.vanguard'].directionURLs,
    visualRole: 'neutral player ship',
    accentColor: 0x8af5ff,
    glowColor: 0x2bdfff,
  },
  'npc.swarm.hostile': {
    key: 'npc.swarm.hostile',
    layer: 'world',
    assetURL: CURATED_ENTITY_ASSETS['npc.hostile.crab'].runtimeURL,
    directionURLs: CURATED_ENTITY_ASSETS['npc.hostile.crab'].directionURLs,
    visualRole: 'hostile npc',
    accentColor: 0xff4236,
    glowColor: 0xff5c7a,
  },
  'loot.cache': {
    key: 'loot.cache',
    layer: 'world',
    assetURL: CURATED_ENTITY_ASSETS['loot.cache.cube'].runtimeURL,
    directionURLs: CURATED_ENTITY_ASSETS['loot.cache.cube'].directionURLs,
    visualRole: 'loot crate',
    accentColor: 0xf4c95d,
    glowColor: 0xfff0a8,
  },
  'planet.signal.unknown': {
    key: 'planet.signal.unknown',
    layer: 'world',
    assetURL: planetUnknownURL,
    visualRole: 'unknown planet signal',
    accentColor: 0xf4c95d,
    glowColor: 0xfff0a8,
  },
  'planet.signal.known': {
    key: 'planet.signal.known',
    layer: 'world',
    assetURL: planetKnownURL,
    visualRole: 'known planet signal',
    accentColor: 0x2bdfff,
    glowColor: 0x8af5ff,
  },
  'planet.memory.owned': {
    key: 'planet.memory.owned',
    layer: 'world',
    assetURL: planetKnownURL,
    visualRole: 'owned planet memory marker',
    accentColor: 0x44e878,
    glowColor: 0x8af5ff,
  },
  'planet.memory.known': {
    key: 'planet.memory.known',
    layer: 'world',
    assetURL: planetKnownURL,
    visualRole: 'known planet memory marker',
    accentColor: 0x44e878,
    glowColor: 0x8af5ff,
  },
  'portal.gate.visible': {
    key: 'portal.gate.visible',
    layer: 'overlay',
    assetURL: portalGateURL,
    visualRole: 'portal gate',
    accentColor: 0x8af5ff,
    glowColor: 0x2bdfff,
  },
  'zone.safe.pvp-blocked': {
    key: 'zone.safe.pvp-blocked',
    layer: 'overlay',
    assetURL: safeZoneURL,
    visualRole: 'safe zone',
    accentColor: 0x44e878,
    glowColor: 0x8af5ff,
  },
  'zone.safe.warning': {
    key: 'zone.safe.warning',
    layer: 'overlay',
    assetURL: radarWarningURL,
    visualRole: 'radar warning zone',
    accentColor: 0xf4c95d,
    glowColor: 0xfff0a8,
  },
  'projectile.laser.basic': {
    key: 'projectile.laser.basic',
    layer: 'effect',
    assetURL: projectileLaserURL,
    visualRole: 'laser projectile',
    accentColor: 0xf4c95d,
    glowColor: 0x2bdfff,
  },
  'marker.movement.target': {
    key: 'marker.movement.target',
    layer: 'overlay',
    assetURL: movementMarkerURL,
    visualRole: 'movement target marker',
    accentColor: 0xf4c95d,
    glowColor: 0xfff0a8,
  },
  'marker.selection.target': {
    key: 'marker.selection.target',
    layer: 'overlay',
    assetURL: selectionReticleURL,
    visualRole: 'selection reticle',
    accentColor: 0x8af5ff,
    glowColor: 0x2bdfff,
  },
  'effect.damage': {
    key: 'effect.damage',
    layer: 'effect',
    assetURL: damageBurstURL,
    visualRole: 'damage burst',
    accentColor: 0xff5c7a,
    glowColor: 0xff4236,
  },
  'effect.loot': {
    key: 'effect.loot',
    layer: 'effect',
    assetURL: lootSparkURL,
    visualRole: 'loot pickup spark',
    accentColor: 0xf4c95d,
    glowColor: 0xfff0a8,
  },
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

export function worldTextureKeyForAsset(descriptor: WorldRenderAssetDescriptor, direction?: EntityAssetDirectionCode): string {
  return direction && descriptor.directionURLs?.[direction] ? `${descriptor.key}:${direction}` : descriptor.key;
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

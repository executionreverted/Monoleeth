import lootCacheIsoEastURL from '../assets/world/entities/loot_cache_iso_east.png?url';
import npcHostileIsoEastURL from '../assets/world/entities/npc_hostile_iso_east.png?url';
import shipPlayerIsoEastURL from '../assets/world/entities/ship_player_iso_east.png?url';

export type EntityAssetDirectionCode = '00' | '02' | '04' | '06' | '08' | '10' | '12' | '14';

export const ENTITY_ASSET_DIRECTION_LABELS: Record<EntityAssetDirectionCode, string> = {
  '00': 'southwest',
  '02': 'west',
  '04': 'northwest',
  '06': 'north',
  '08': 'northeast',
  '10': 'east',
  '12': 'southeast',
  '14': 'south',
};

export type CuratedEntityAssetKey = 'player.ship.vanguard' | 'npc.hostile.crab' | 'loot.cache.cube';

export interface CuratedEntityAsset {
  key: CuratedEntityAssetKey;
  kind: 'ship' | 'npc' | 'lootable';
  displayName: string;
  runtimeURL: string;
  direction: EntityAssetDirectionCode;
}

export const CURATED_ENTITY_ASSETS: Record<CuratedEntityAssetKey, CuratedEntityAsset> = {
  'player.ship.vanguard': {
    key: 'player.ship.vanguard',
    kind: 'ship',
    displayName: 'Player Vanguard',
    runtimeURL: shipPlayerIsoEastURL,
    direction: '10',
  },
  'npc.hostile.crab': {
    key: 'npc.hostile.crab',
    kind: 'npc',
    displayName: 'War Crab Raider',
    runtimeURL: npcHostileIsoEastURL,
    direction: '10',
  },
  'loot.cache.cube': {
    key: 'loot.cache.cube',
    kind: 'lootable',
    displayName: 'Hypercube Cache',
    runtimeURL: lootCacheIsoEastURL,
    direction: '10',
  },
};

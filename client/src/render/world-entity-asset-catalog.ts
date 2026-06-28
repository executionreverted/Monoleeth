export type EntityAssetDirectionCode = '00' | '02' | '04' | '06' | '08' | '10' | '12' | '14';

const playerVanguardFrameURLs = import.meta.glob<string>('../assets/world/entities/ship_player_iso_*.png', {
  eager: true,
  import: 'default',
  query: '?url',
});
const npcHostileFrameURLs = import.meta.glob<string>('../assets/world/entities/npc_hostile_iso_*.png', {
  eager: true,
  import: 'default',
  query: '?url',
});
const lootCacheFrameURLs = import.meta.glob<string>('../assets/world/entities/loot_cache_iso_*.png', {
  eager: true,
  import: 'default',
  query: '?url',
});

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

export const ENTITY_ASSET_DIRECTION_CODES = Object.keys(ENTITY_ASSET_DIRECTION_LABELS) as EntityAssetDirectionCode[];

const playerVanguardDirections = frameSet(playerVanguardFrameURLs);
const npcHostileDirections = frameSet(npcHostileFrameURLs);
const lootCacheDirections = frameSet(lootCacheFrameURLs);

export type CuratedEntityAssetKey = 'player.ship.vanguard' | 'npc.hostile.crab' | 'loot.cache.cube';

export interface CuratedEntityAsset {
  key: CuratedEntityAssetKey;
  kind: 'ship' | 'npc' | 'lootable';
  displayName: string;
  runtimeURL: string;
  defaultDirection: EntityAssetDirectionCode;
  directionURLs: Record<EntityAssetDirectionCode, string>;
}

export const CURATED_ENTITY_ASSETS: Record<CuratedEntityAssetKey, CuratedEntityAsset> = {
  'player.ship.vanguard': {
    key: 'player.ship.vanguard',
    kind: 'ship',
    displayName: 'Player Vanguard',
    runtimeURL: directionURL(playerVanguardDirections, '10'),
    defaultDirection: '10',
    directionURLs: playerVanguardDirections,
  },
  'npc.hostile.crab': {
    key: 'npc.hostile.crab',
    kind: 'npc',
    displayName: 'War Crab Raider',
    runtimeURL: directionURL(npcHostileDirections, '10'),
    defaultDirection: '10',
    directionURLs: npcHostileDirections,
  },
  'loot.cache.cube': {
    key: 'loot.cache.cube',
    kind: 'lootable',
    displayName: 'Hypercube Cache',
    runtimeURL: directionURL(lootCacheDirections, '10'),
    defaultDirection: '10',
    directionURLs: lootCacheDirections,
  },
};

function frameSet(modules: Record<string, string>): Record<EntityAssetDirectionCode, string> {
  const urls = {} as Record<EntityAssetDirectionCode, string>;
  for (const [path, url] of Object.entries(modules)) {
    const match = /_(00|02|04|06|08|10|12|14)\.png$/u.exec(path);
    if (match) {
      urls[match[1] as EntityAssetDirectionCode] = url;
    }
  }
  for (const code of ENTITY_ASSET_DIRECTION_CODES) {
    if (!urls[code]) {
      throw new Error(`Missing entity frame direction ${code}`);
    }
  }
  return urls;
}

function directionURL(urls: Record<EntityAssetDirectionCode, string>, direction: EntityAssetDirectionCode): string {
  return urls[direction];
}

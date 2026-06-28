import { describe, expect, test } from 'vitest';

import { CURATED_ENTITY_ASSETS, ENTITY_ASSET_DIRECTION_CODES, ENTITY_ASSET_DIRECTION_LABELS } from './world-entity-asset-catalog';
import {
  isUnknownSignalAsset,
  WORLD_RENDER_ASSETS,
  worldAssetForEntity,
  worldAssetForMemoryMarker,
  worldAssetForPortal,
  worldAssetForSafeZone,
} from './world-renderer-assets';

describe('world renderer asset registry', () => {
  test('binds every render asset to a concrete client asset URL', () => {
    const descriptors = Object.values(WORLD_RENDER_ASSETS);
    expect(descriptors).toHaveLength(Object.keys(WORLD_RENDER_ASSETS).length);
    for (const descriptor of descriptors) {
      expect(descriptor.assetURL).toMatch(/^(data:image\/svg\+xml|.*\.(png|svg)(\?|$))/);
      expect(descriptor.visualRole).toMatch(/\S/);
      expect(descriptor.key).toMatch(/^[a-z0-9.-]+$/);
    }
  });

  test('covers the first playable asset set required by the real map loop', () => {
    const roles = new Set(Object.values(WORLD_RENDER_ASSETS).map((descriptor) => descriptor.visualRole));
    expect(roles.has('map background')).toBe(true);
    expect(roles.has('player ship')).toBe(true);
    expect(roles.has('hostile npc')).toBe(true);
    expect(roles.has('laser projectile')).toBe(true);
    expect(roles.has('loot crate')).toBe(true);
    expect(roles.has('known planet signal')).toBe(true);
    expect(roles.has('portal gate')).toBe(true);
    expect(roles.has('safe zone')).toBe(true);
    expect(roles.has('radar warning zone')).toBe(true);
  });

  test('uses the curated runtime-safe PNG catalog for player, hostile NPC, and loot sprites', () => {
    expect(ENTITY_ASSET_DIRECTION_LABELS['10']).toBe('east');
    expect(Object.values(CURATED_ENTITY_ASSETS).map((asset) => asset.key)).toEqual([
      'player.ship.vanguard',
      'npc.hostile.crab',
      'loot.cache.cube',
    ]);

    expect(WORLD_RENDER_ASSETS['ship.player.self'].assetURL).toContain('ship_player_iso_10');
    expect(WORLD_RENDER_ASSETS['npc.swarm.hostile'].assetURL).toContain('npc_hostile_iso_10');
    expect(WORLD_RENDER_ASSETS['loot.cache'].assetURL).toContain('loot_cache_iso_10');
    expect(WORLD_RENDER_ASSETS['ship.player.self'].assetURL).toMatch(/\.png(\?|$)/);
    expect(WORLD_RENDER_ASSETS['npc.swarm.hostile'].assetURL).toMatch(/\.png(\?|$)/);
    expect(WORLD_RENDER_ASSETS['loot.cache'].assetURL).toMatch(/\.png(\?|$)/);
    for (const asset of Object.values(CURATED_ENTITY_ASSETS)) {
      expect(asset.defaultDirection).toBe('10');
      expect(Object.keys(asset.directionURLs).sort()).toEqual([...ENTITY_ASSET_DIRECTION_CODES].sort());
      for (const code of ENTITY_ASSET_DIRECTION_CODES) {
        expect(asset.directionURLs[code]).toContain(`_${code}`);
      }
    }
    expect(JSON.stringify(CURATED_ENTITY_ASSETS)).not.toMatch(/spin_512|Nebula_Vanguard|Nebula_War_Crab|Nebula_Hypercube|\.gif/i);
  });

  test('maps server-visible entities to stable render assets', () => {
    expect(worldAssetForEntity({ entity_id: 'self', entity_type: 'player', position: { x: 0, y: 0 } }, true).key).toBe('ship.player.self');
    expect(
      worldAssetForEntity({
        entity_id: 'ally',
        entity_type: 'player',
        position: { x: 0, y: 0 },
        display: { disposition: 'friendly' },
      }).key,
    ).toBe('ship.player.friendly');
    expect(worldAssetForEntity({ entity_id: 'npc-1', entity_type: 'npc', position: { x: 0, y: 0 } }).key).toBe('npc.swarm.hostile');
    expect(worldAssetForEntity({ entity_id: 'drop-1', entity_type: 'loot', position: { x: 0, y: 0 } }).key).toBe('loot.cache');
  });

  test('keeps unknown and known planet signals visually distinct without leaking hidden scan data', () => {
    const unknown = {
      entity_id: 'signal-unknown',
      entity_type: 'planet_signal' as const,
      position: { x: 10, y: 20 },
      status_flags: ['unknown_signal'],
      display: { label: 'Unknown Signal' },
    };
    const known = {
      entity_id: 'signal-known',
      entity_type: 'planet_signal' as const,
      position: { x: 10, y: 20 },
      display: { label: 'Eris' },
    };

    expect(isUnknownSignalAsset(unknown)).toBe(true);
    expect(isUnknownSignalAsset(known)).toBe(false);
    expect(worldAssetForEntity(unknown).key).toBe('planet.signal.unknown');
    expect(worldAssetForEntity(known).key).toBe('planet.signal.known');
    expect(JSON.stringify(WORLD_RENDER_ASSETS)).not.toMatch(/seed|candidate|loot_table|internal_map_id|spawn_area|drop_profile/i);
  });

  test('maps map utility objects to DarkOrbit-style portal and safe-zone assets', () => {
    expect(
      worldAssetForPortal({
        source: 'currentMap',
        portalID: 'east_gate',
        label: 'East Gate',
        world: { x: 100, y: 100 },
        screen: { x: 10, y: 10 },
        interactionRadius: 160,
        screenRadius: 16,
      }).key,
    ).toBe('portal.gate.visible');
    expect(
      worldAssetForSafeZone({
        source: 'currentMap',
        safeAreaID: 'station-alpha',
        label: 'Station Alpha',
        center: { x: 100, y: 100 },
        screen: { x: 10, y: 10 },
        radius: 700,
        screenRadius: 70,
        blocksPVP: true,
        hangarActions: true,
      }).key,
    ).toBe('zone.safe.pvp-blocked');
    expect(
      worldAssetForSafeZone({
        source: 'currentMap',
        safeAreaID: 'radiation-warning',
        label: 'Radiation',
        center: { x: 100, y: 100 },
        screen: { x: 10, y: 10 },
        radius: 700,
        screenRadius: 70,
        blocksPVP: false,
        hangarActions: false,
      }).key,
    ).toBe('zone.safe.warning');
  });

  test('maps remembered planet markers by public ownership state', () => {
    expect(
      worldAssetForMemoryMarker({
        id: 'owned-memory',
        kind: 'known_planet',
        detailID: 'planet-owned',
        label: 'Owned',
        position: { x: 0, y: 0 },
        state: 'owned',
      }).key,
    ).toBe('planet.memory.owned');
    expect(
      worldAssetForMemoryMarker({
        id: 'known-memory',
        kind: 'known_planet',
        detailID: 'planet-known',
        label: 'Known',
        position: { x: 0, y: 0 },
        state: 'known',
      }).key,
    ).toBe('planet.memory.known');
  });
});

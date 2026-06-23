import { describe, expect, test } from 'vitest';

import {
  isUnknownSignalAsset,
  WORLD_RENDER_ASSETS,
  worldAssetForEntity,
  worldAssetForMemoryMarker,
  worldAssetForPortal,
  worldAssetForSafeZone,
} from './world-renderer-assets';

describe('world renderer asset registry', () => {
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
    expect(JSON.stringify(WORLD_RENDER_ASSETS)).not.toMatch(/seed|candidate|loot_table|internal_map_id/i);
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

import { describe, expect, test } from 'vitest';

import { resolvePlanetNavigationTarget } from './planet-navigation';
import type { PlanetDetailSummary, PlanetIntelSummary } from '../state/types';

function planetDetail(overrides: Partial<PlanetDetailSummary> = {}): PlanetDetailSummary {
  return {
    planet_id: 'planet-eris',
    biome: 'ice',
    planet_type: 'dwarf_planet',
    rarity: 'uncommon',
    level: 2,
    intel_state: 'fresh',
    confidence: 88,
    last_seen_at: 1000,
    owner_status: 'unclaimed',
    discovered_at: 900,
    coordinates: { x: 320, y: -140 },
    routes: [],
    production_locked: true,
    available_commands: [],
    ...overrides,
  };
}

function planetIntel(selectedPlanet: PlanetDetailSummary | null): PlanetIntelSummary {
  return {
    knownSignals: selectedPlanet ? 1 : 0,
    staleIntel: null,
    ownedPlanets: 0,
    planets: selectedPlanet ? [selectedPlanet] : [],
    selectedPlanet,
    lastScan: null,
  };
}

describe('resolvePlanetNavigationTarget', () => {
  test('requires selected server planet detail for requested planet id', () => {
    expect(resolvePlanetNavigationTarget(null, 'planet-eris')).toBeNull();
    expect(resolvePlanetNavigationTarget(planetIntel(null), 'planet-eris')).toBeNull();
    expect(resolvePlanetNavigationTarget(planetIntel(planetDetail({ planet_id: 'planet-other' })), 'planet-eris')).toBeNull();
  });

  test('does not invent a coordinate when server detail has not returned one', () => {
    expect(resolvePlanetNavigationTarget(planetIntel(planetDetail({ coordinates: null })), 'planet-eris')).toBeNull();
    expect(
      resolvePlanetNavigationTarget(
        planetIntel(planetDetail({ coordinates: { x: Number.NaN, y: -140 } })),
        'planet-eris',
      ),
    ).toBeNull();
  });

  test('returns a copy of the selected server-returned detail coordinates', () => {
    const detail = planetDetail({ coordinates: { x: 725, y: -360 } });
    const target = resolvePlanetNavigationTarget(planetIntel(detail), 'planet-eris');

    expect(target).toEqual({ x: 725, y: -360 });
    expect(target).not.toBe(detail.coordinates);
  });
});

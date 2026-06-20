import type { ClientState, PlanetDetailSummary, WorldMapMemoryMarker } from './types';

export function worldMapMemoryMarkers(state: ClientState): WorldMapMemoryMarker[] {
  const markers = new Map<string, WorldMapMemoryMarker>();
  const planet = state.planetIntel?.selectedPlanet;
  if (planet?.planet_id && isFiniteVec(planet.coordinates)) {
    const marker = planetMemoryMarker(planet, planet.coordinates);
    markers.set(marker.id, marker);
  }
  for (const memory of state.minimap?.remembered ?? []) {
    const detailID = memory.detail_id || memory.planet_id || '';
    if (memory.kind !== 'known_planet' || !detailID || !isFiniteVec(memory.position)) {
      continue;
    }
    markers.set(`known_planet:${detailID}`, {
      id: `known_planet:${detailID}`,
      kind: 'known_planet',
      label: memory.label || 'known planet',
      position: { ...memory.position },
      detailID,
      state: memory.freshness || 'known',
    });
  }
  return [...markers.values()].sort((a, b) => a.id.localeCompare(b.id));
}

function planetMemoryMarker(planet: PlanetDetailSummary, coordinates: { x: number; y: number }): WorldMapMemoryMarker {
  return {
    id: `known_planet:${planet.planet_id}`,
    kind: 'known_planet',
    label: publicPlanetName(planet),
    position: { ...coordinates },
    detailID: planet.planet_id,
    state: planet.owner_status || planet.intel_state || 'known',
  };
}

function isFiniteVec(value: unknown): value is { x: number; y: number } {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const candidate = value as { x?: unknown; y?: unknown };
  return (
    typeof candidate.x === 'number' &&
    typeof candidate.y === 'number' &&
    Number.isFinite(candidate.x) &&
    Number.isFinite(candidate.y)
  );
}

function publicPlanetName(planet: Pick<PlanetDetailSummary, 'planet_type' | 'biome'>): string {
  const type = planet.planet_type ? planet.planet_type.replace(/_/g, ' ') : 'planet';
  const biome = planet.biome ? planet.biome.replace(/_/g, ' ') : 'unknown';
  return `${type} / ${biome}`;
}

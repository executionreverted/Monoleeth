import type { ClientState, PlanetDetailSummary, WorldMapMemoryMarker } from './types';

export function worldMapMemoryMarkers(state: ClientState): WorldMapMemoryMarker[] {
  const planet = state.planetIntel?.selectedPlanet;
  if (!planet?.planet_id) {
    return [];
  }
  return [planetMemoryMarker(planet)];
}

function planetMemoryMarker(planet: PlanetDetailSummary): WorldMapMemoryMarker {
  return {
    id: `known_planet:${planet.planet_id}`,
    kind: 'known_planet',
    label: publicPlanetName(planet),
    position: { ...planet.coordinates },
    detailID: planet.planet_id,
    state: planet.owner_status || planet.intel_state || 'known',
  };
}

function publicPlanetName(planet: Pick<PlanetDetailSummary, 'planet_type' | 'biome'>): string {
  const type = planet.planet_type ? planet.planet_type.replace(/_/g, ' ') : 'planet';
  const biome = planet.biome ? planet.biome.replace(/_/g, ' ') : 'unknown';
  return `${type} / ${biome}`;
}

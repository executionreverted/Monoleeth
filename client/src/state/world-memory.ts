import type { ClientState, MinimapMemory, PlanetDetailSummary, WorldMapMemoryMarker } from './types';

export interface MinimapPointPercent {
  left: number;
  top: number;
  clamped: boolean;
}

export function worldMapMemoryMarkers(state: ClientState): WorldMapMemoryMarker[] {
  const markers = new Map<string, WorldMapMemoryMarker>();
  const planet = state.planetIntel?.selectedPlanet;
  if (
    planet?.planet_id &&
    isFiniteVec(planet.coordinates) &&
    rememberedIntelMatchesCurrentSector(state, planet) &&
    rememberedIntelState(planet) !== 'invalidated'
  ) {
    const marker = planetMemoryMarker(planet, planet.coordinates);
    markers.set(marker.id, marker);
  }
  for (const memory of state.minimap?.remembered ?? []) {
    const detailID = rememberedMinimapDetailID(state, memory);
    if (!detailID) {
      continue;
    }
    const marker: WorldMapMemoryMarker = {
      id: `known_planet:${detailID}`,
      kind: 'known_planet',
      label: memory.label || 'known planet',
      position: { ...memory.position },
      detailID,
      state: rememberedIntelState(memory),
    };
    if (memory.projection_source) {
      marker.projectionSource = memory.projection_source;
    }
    markers.set(marker.id, marker);
  }
  return [...markers.values()].sort((a, b) => a.id.localeCompare(b.id));
}

export type RememberedIntelState = 'known' | 'fresh' | 'verified' | 'stale' | 'invalidated' | 'colonized_by_other';

export function rememberedIntelState(value: { freshness?: string; intel_state?: string; invalidated?: boolean }): RememberedIntelState {
  if (value.invalidated) {
    return 'invalidated';
  }
  const state = normalizeState(value.freshness ?? value.intel_state ?? '');
  if (
    state === 'invalid' ||
    state === 'invalidated' ||
    state === 'revoked' ||
    state === 'expired' ||
    state === 'wrong_zone' ||
    state === 'cross_zone' ||
    state === 'out_of_zone'
  ) {
    return 'invalidated';
  }
  if (state === 'stale' || state === 'old' || state === 'outdated') {
    return 'stale';
  }
  if (state === 'fresh' || state === 'verified' || state === 'colonized_by_other') {
    return state;
  }
  return 'known';
}

export function rememberedIntelMatchesCurrentSector(
  state: Pick<ClientState, 'sector'>,
  value: { sector_key?: string },
): boolean {
  if (!value.sector_key) {
    return true;
  }
  return Boolean(state.sector?.sector_key && state.sector.sector_key === value.sector_key);
}

export function rememberedMinimapDetailID(
  state: Pick<ClientState, 'sector'>,
  memory: MinimapMemory,
): string | null {
  const detailID = memory.detail_id || memory.planet_id || '';
  if (!isRenderableMinimapMemory(memory) || !rememberedIntelMatchesCurrentSector(state, memory)) {
    return null;
  }
  return detailID;
}

export function shouldRenderRememberedMinimapMemory(
  state: Pick<ClientState, 'sector'>,
  memory: MinimapMemory,
  center: { x: number; y: number },
  halfExtent: number,
): boolean {
  return Boolean(
    rememberedMinimapDetailID(state, memory) &&
      isWithinMinimapProjectionWindow(center, memory.position, halfExtent),
  );
}

export function isRenderableMinimapMemory(
  memory: Pick<MinimapMemory, 'kind' | 'planet_id' | 'detail_id' | 'position' | 'freshness' | 'invalidated'>,
): boolean {
  if (memory.kind !== 'known_planet' || !(memory.detail_id || memory.planet_id) || !isFiniteVec(memory.position)) {
    return false;
  }
  return rememberedIntelState(memory) !== 'invalidated';
}

export function isClickableMinimapMemory(memory: MinimapMemory): boolean {
  return isRenderableMinimapMemory(memory) && Boolean(memory.detail_id || memory.planet_id);
}

export function isWithinMinimapProjectionWindow(
  center: { x: number; y: number },
  position: { x: number; y: number },
  halfExtent: number,
): boolean {
  if (!isFiniteVec(center) || !isFiniteVec(position) || !Number.isFinite(halfExtent) || halfExtent <= 0) {
    return false;
  }
  return Math.abs(position.x - center.x) <= halfExtent && Math.abs(position.y - center.y) <= halfExtent;
}

export function minimapPointPercent(
  center: { x: number; y: number },
  position: { x: number; y: number },
  radarRange: number,
): MinimapPointPercent | null {
  if (!isFiniteVec(center) || !isFiniteVec(position) || !Number.isFinite(radarRange) || radarRange <= 0) {
    return null;
  }
  const rawLeft = 50 + ((position.x - center.x) / (radarRange * 2)) * 100;
  const rawTop = 50 + ((position.y - center.y) / (radarRange * 2)) * 100;
  const left = clampPercent(rawLeft);
  const top = clampPercent(rawTop);
  return {
    left,
    top,
    clamped: left !== rawLeft || top !== rawTop,
  };
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

function clampPercent(value: number): number {
  return Math.min(Math.max(value, 4), 96);
}

function normalizeState(value: string): string {
  return value.trim().toLowerCase().replace(/[\s-]+/g, '_');
}

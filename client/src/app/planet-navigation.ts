import type { Vec2 } from '../protocol/envelope';
import type { PlanetIntelSummary } from '../state/types';

export function resolvePlanetNavigationTarget(intel: PlanetIntelSummary | null, planetID: string): Vec2 | null {
  if (!planetID) {
    return null;
  }
  const detail = intel?.selectedPlanet ?? null;
  if (!detail || detail.planet_id !== planetID) {
    return null;
  }
  const coordinates = detail.coordinates;
  if (!coordinates || !Number.isFinite(coordinates.x) || !Number.isFinite(coordinates.y)) {
    return null;
  }
  return { x: coordinates.x, y: coordinates.y };
}

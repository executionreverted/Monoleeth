import { Vec2 } from '../protocol/envelope';
import { MapBounds, MapSummary, MinimapSummary, PublicPortalSummary, SafeZoneProjection } from '../state/types';

export type MapOverlaySource = 'currentMap' | 'minimap';

export interface MapOverlayProjection {
  center: Vec2;
  screen: { width: number; height: number };
  scale: number;
}

export interface MapOverlayFrameDebug {
  source: MapOverlaySource;
  world: MapBounds;
  topLeft: Vec2;
  topRight: Vec2;
  bottomRight: Vec2;
  bottomLeft: Vec2;
  width: number;
  height: number;
}

export interface MapOverlayPortalDebug {
  source: MapOverlaySource;
  portalID: string;
  label: string | null;
  world: Vec2;
  screen: Vec2;
  interactionRadius: number;
  screenRadius: number;
}

export interface MapOverlaySafeZoneDebug {
  source: MapOverlaySource;
  safeAreaID: string;
  label: string | null;
  center: Vec2;
  screen: Vec2;
  radius: number;
  screenRadius: number;
  blocksPVP: boolean;
  hangarActions: boolean;
}

export interface MapOverlayDebugState {
  active: boolean;
  source: MapOverlaySource | null;
  bounds: MapOverlayFrameDebug | null;
  portalMarkers: MapOverlayPortalDebug[];
  safeZones: MapOverlaySafeZoneDebug[];
}

export interface MapOverlayInput {
  currentMap: MapSummary | null;
  minimap: MinimapSummary | null;
}

export function emptyMapOverlayDebug(): MapOverlayDebugState {
  return {
    active: false,
    source: null,
    bounds: null,
    portalMarkers: [],
    safeZones: [],
  };
}

export function cloneMapOverlayDebug(debug: MapOverlayDebugState): MapOverlayDebugState {
  return {
    active: debug.active,
    source: debug.source,
    bounds: debug.bounds
      ? {
          source: debug.bounds.source,
          world: { ...debug.bounds.world },
          topLeft: { ...debug.bounds.topLeft },
          topRight: { ...debug.bounds.topRight },
          bottomRight: { ...debug.bounds.bottomRight },
          bottomLeft: { ...debug.bounds.bottomLeft },
          width: debug.bounds.width,
          height: debug.bounds.height,
        }
      : null,
    portalMarkers: debug.portalMarkers.map((marker) => ({
      ...marker,
      world: { ...marker.world },
      screen: { ...marker.screen },
    })),
    safeZones: debug.safeZones.map((zone) => ({
      ...zone,
      center: { ...zone.center },
      screen: { ...zone.screen },
    })),
  };
}

export function summarizeMapOverlay(input: MapOverlayInput, projection: MapOverlayProjection): MapOverlayDebugState {
  const bounds = chooseOverlayBounds(input);
  if (!bounds) {
    return emptyMapOverlayDebug();
  }

  return {
    active: true,
    source: bounds.source,
    bounds: projectBounds(bounds.bounds, bounds.source, projection),
    portalMarkers: collectPortalMarkers(input, projection),
    safeZones: collectSafeZoneMarkers(input, projection),
  };
}

export function projectWorldToScreen(world: Vec2, projection: MapOverlayProjection): Vec2 {
  return {
    x: projection.screen.width / 2 + (world.x - projection.center.x) * projection.scale,
    y: projection.screen.height / 2 + (world.y - projection.center.y) * projection.scale,
  };
}

function chooseOverlayBounds(input: MapOverlayInput): { source: MapOverlaySource; bounds: MapBounds } | null {
  if (input.currentMap && isUsableBounds(input.currentMap.bounds)) {
    return { source: 'currentMap', bounds: input.currentMap.bounds };
  }
  if (input.minimap?.bounds && isUsableBounds(input.minimap.bounds)) {
    return { source: 'minimap', bounds: input.minimap.bounds };
  }
  return null;
}

function projectBounds(bounds: MapBounds, source: MapOverlaySource, projection: MapOverlayProjection): MapOverlayFrameDebug {
  const topLeft = projectWorldToScreen({ x: bounds.min_x, y: bounds.min_y }, projection);
  const topRight = projectWorldToScreen({ x: bounds.max_x, y: bounds.min_y }, projection);
  const bottomRight = projectWorldToScreen({ x: bounds.max_x, y: bounds.max_y }, projection);
  const bottomLeft = projectWorldToScreen({ x: bounds.min_x, y: bounds.max_y }, projection);
  return {
    source,
    world: { ...bounds },
    topLeft,
    topRight,
    bottomRight,
    bottomLeft,
    width: bottomRight.x - topLeft.x,
    height: bottomRight.y - topLeft.y,
  };
}

function collectPortalMarkers(input: MapOverlayInput, projection: MapOverlayProjection): MapOverlayPortalDebug[] {
  const seen = new Set<string>();
  const markers: MapOverlayPortalDebug[] = [];
  const add = (source: MapOverlaySource, portals: PublicPortalSummary[] | undefined): void => {
    for (const portal of portals ?? []) {
      if (seen.has(portal.portal_id) || !isFiniteVec(portal.position) || !Number.isFinite(portal.interaction_radius) || portal.interaction_radius <= 0) {
        continue;
      }
      seen.add(portal.portal_id);
      markers.push({
        source,
        portalID: portal.portal_id,
        label: cleanLabel(portal.display_name),
        world: { ...portal.position },
        screen: projectWorldToScreen(portal.position, projection),
        interactionRadius: portal.interaction_radius,
        screenRadius: Math.max(0, portal.interaction_radius * projection.scale),
      });
    }
  };

  add('currentMap', input.currentMap?.visible_portals);
  add('minimap', input.minimap?.visible_portals);
  return markers;
}

function collectSafeZoneMarkers(input: MapOverlayInput, projection: MapOverlayProjection): MapOverlaySafeZoneDebug[] {
  const seen = new Set<string>();
  const markers: MapOverlaySafeZoneDebug[] = [];
  const add = (source: MapOverlaySource, zones: SafeZoneProjection[] | undefined): void => {
    for (const zone of zones ?? []) {
      if (seen.has(zone.safe_area_id) || !isFiniteVec(zone.center) || !Number.isFinite(zone.radius) || zone.radius <= 0) {
        continue;
      }
      seen.add(zone.safe_area_id);
      markers.push({
        source,
        safeAreaID: zone.safe_area_id,
        label: cleanLabel(zone.display_name),
        center: { ...zone.center },
        screen: projectWorldToScreen(zone.center, projection),
        radius: zone.radius,
        screenRadius: Math.max(0, zone.radius * projection.scale),
        blocksPVP: zone.blocks_pvp,
        hangarActions: zone.hangar_actions,
      });
    }
  };

  add('currentMap', input.currentMap?.safe_zones);
  add('minimap', input.minimap?.safe_zones);
  return markers;
}

function isUsableBounds(bounds: MapBounds): boolean {
  return (
    Number.isFinite(bounds.min_x) &&
    Number.isFinite(bounds.min_y) &&
    Number.isFinite(bounds.max_x) &&
    Number.isFinite(bounds.max_y) &&
    bounds.max_x > bounds.min_x &&
    bounds.max_y > bounds.min_y
  );
}

function isFiniteVec(value: Vec2): boolean {
  return Number.isFinite(value.x) && Number.isFinite(value.y);
}

function cleanLabel(value: string | undefined): string | null {
  const label = value?.trim();
  return label ? label : null;
}

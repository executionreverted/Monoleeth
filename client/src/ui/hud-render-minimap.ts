import type { ClientState, MapBounds, MinimapContact, MinimapMemory, MinimapSummary, PublicPortalSummary, SafeZoneProjection } from '../state/types';
import {
  isClickableMinimapMemory,
  minimapPointPercent,
  rememberedIntelState,
  rememberedMinimapDetailID,
  shouldRenderRememberedMinimapMemory,
} from '../state/world-memory';
import { clamp, dispositionForType, escapeHTML, formatCompactNumber, lockedValue, publicEntityType } from './hud-formatters';
import { topbarLocationText } from './hud-topbar';

export function minimapPanel(state: ClientState): string {
  const bounds = chooseMinimapBounds(state);
  if (!bounds && !hasRenderableMinimapPayload(state.minimap)) {
    return '<div class="minimap minimap--empty"><div class="empty-line">Awaiting map projection.</div></div>';
  }

  const contacts = state.minimap?.live_contacts ?? [];
  const memories = state.minimap?.remembered ?? [];
  const self = contacts.find((contact) => contact.status_flags?.includes('self')) ?? contacts.find((contact) => contact.entity_type === 'player');
  const center = self?.position ?? { x: 0, y: 0 };
  const radius = Math.max(state.minimap?.radar_range ?? 0, 1);
  const projectionHalfExtent = Math.max((state.minimap?.projection_window_size ?? radius * 2) / 2, 1);
  const projection = { bounds, center, radius };
  const points = contacts
    .map((contact) => {
      const point = projectMinimapPoint(projection, contact.position);
      if (!point) {
        return '';
      }
      const disposition = contact.status_flags?.includes('self') ? 'self' : contact.disposition || dispositionForType(contact.entity_type);
      const action = minimapLiveContactAction(contact);
      const actionAttr = action ? ` data-action="${action}"` : '';
      const disabledAttr = action ? '' : ' disabled';
      const source = contact.projection_source ? ` data-projection-source="${escapeHTML(contact.projection_source)}"` : '';
      return `<button class="minimap__point" type="button"${actionAttr}${disabledAttr} data-target-source="radar" data-kind="${escapeHTML(disposition)}" data-entity-id="${escapeHTML(contact.entity_id)}" data-entity-type="${escapeHTML(contact.entity_type)}"${source} style="${minimapPointStyle(point)}" title="${escapeHTML(publicEntityType(contact.entity_type))}"></button>`;
    })
    .join('');
  const memoryPoints = memories
    .map((memory) => {
      const planetID = minimapMemoryDetailID(state, memory, bounds, center, projectionHalfExtent);
      if (!planetID) {
        return '';
      }
      const point = projectMinimapPoint(projection, memory.position);
      if (!point) {
        return '';
      }
      const clickable = Boolean(planetID);
      const action = clickable ? ' data-action="planet-detail"' : '';
      const planet = planetID ? ` data-planet-id="${escapeHTML(planetID)}"` : '';
      const disabled = clickable ? '' : ' disabled';
      const intelState = rememberedIntelState(memory);
      const sector = memory.sector_key ? ` data-sector-key="${escapeHTML(memory.sector_key)}"` : '';
      const source = memory.projection_source ? ` data-projection-source="${escapeHTML(memory.projection_source)}"` : '';
      const publicMap = memory.public_map_key ? ` data-public-map-key="${escapeHTML(memory.public_map_key)}"` : '';
      return `<button class="minimap__memory" type="button"${action}${planet}${disabled} data-kind="${escapeHTML(memory.kind)}" data-freshness="${escapeHTML(intelState)}"${sector}${publicMap}${source} style="${minimapPointStyle(point)}" title="${escapeHTML(memory.label || memory.kind)}"></button>`;
    })
    .join('');
  const portalMarkers = collectMinimapPortals(state)
    .map((portal) => {
      const point = projectMinimapPoint(projection, portal.position);
      if (!point) {
        return '';
      }
      const label = markerLabel(portal.display_name, 'Portal');
      return `<span class="minimap__portal" data-marker="portal" data-portal-id="${escapeHTML(portal.portal_id)}" style="${minimapPointStyle(point)}" title="${escapeHTML(label)}"><span>${escapeHTML(label)}</span></span>`;
    })
    .join('');
  const safeZoneMarkers = collectMinimapSafeZones(state)
    .map((zone) => {
      const point = projectMinimapPoint(projection, zone.center);
      if (!point) {
        return '';
      }
      const label = markerLabel(zone.display_name, zone.blocks_pvp ? 'Safe zone' : 'Safe area');
      const size = zoneMarkerSizePercent(projection, zone.radius);
      return `<span class="minimap__safe-zone" data-marker="safe-zone" data-safe-area-id="${escapeHTML(zone.safe_area_id)}" data-blocks-pvp="${zone.blocks_pvp ? 'true' : 'false'}" style="${minimapPointStyle(point)};--zone-size:${minimapPercent(size)}%" title="${escapeHTML(label)}"><span>${escapeHTML(label)}</span></span>`;
    })
    .join('');
  const location = topbarLocationText(state);
  const mapLabel = location === lockedValue() ? '' : location;
  const detailLabels = minimapDetailLabels(state, bounds);

  return `
    <div class="minimap ${bounds ? 'minimap--bounded' : 'minimap--radar'}" aria-label="Sector map" data-map-mode="${bounds ? 'bounded' : 'radar'}">
      <span class="minimap__ring minimap__ring--outer"></span>
      <span class="minimap__ring minimap__ring--middle"></span>
      <span class="minimap__axis minimap__axis--x"></span>
      <span class="minimap__axis minimap__axis--y"></span>
      ${
        mapLabel || detailLabels.length > 0
          ? `<div class="minimap__meta">${mapLabel ? `<strong>${escapeHTML(mapLabel)}</strong>` : ''}${detailLabels.map((label) => `<span>${escapeHTML(label)}</span>`).join('')}</div>`
          : ''
      }
      ${safeZoneMarkers}
      ${portalMarkers}
      ${memoryPoints}
      ${points}
    </div>
    <div class="minimap-legend">
      <span data-kind="self">You</span>
      <span data-kind="hostile">Hostile</span>
      <span data-kind="portal">Portal</span>
      <span data-kind="safe-zone">Safe</span>
    </div>
  `;
}

export function minimapLiveContactAction(contact: MinimapContact): 'target-select' | 'loot-select' | null {
  const flags = new Set(contact.status_flags ?? []);
  if (flags.has('self') || flags.has('local') || flags.has('friendly')) {
    return null;
  }
  if (contact.entity_type === 'loot') {
    return 'loot-select';
  }
  if (contact.entity_type === 'npc' && isHostileMinimapContact(contact, flags)) {
    return 'target-select';
  }
  if (contact.entity_type === 'player' && isHostileMinimapContact(contact, flags)) {
    return 'target-select';
  }
  return null;
}

export function isHostileMinimapContact(contact: MinimapContact, flags = new Set(contact.status_flags ?? [])): boolean {
  return flags.has('hostile') || flags.has('scan_revealed') || contact.disposition === 'hostile';
}

interface MinimapProjection {
  bounds: MapBounds | null;
  center: { x: number; y: number };
  radius: number;
}

function chooseMinimapBounds(state: ClientState): MapBounds | null {
  if (isUsableBounds(state.currentMap?.bounds)) {
    return state.currentMap.bounds;
  }
  if (isUsableBounds(state.minimap?.bounds)) {
    return state.minimap.bounds;
  }
  return null;
}

function hasRenderableMinimapPayload(minimap: MinimapSummary | null): boolean {
  if (!minimap) {
    return false;
  }
  return (
    minimap.live_contacts.length > 0 ||
    minimap.remembered.length > 0 ||
    (minimap.visible_portals?.length ?? 0) > 0 ||
    (minimap.safe_zones?.length ?? 0) > 0
  );
}

function projectMinimapPoint(projection: MinimapProjection, position: { x: number; y: number }): { left: number; top: number } | null {
  if (!isFinitePoint(position)) {
    return null;
  }
  if (projection.bounds) {
    return boundedMinimapPointPercent(projection.bounds, position);
  }
  return minimapPointPercent(projection.center, position, projection.radius);
}

function boundedMinimapPointPercent(bounds: MapBounds, position: { x: number; y: number }): { left: number; top: number } | null {
  if (!isUsableBounds(bounds)) {
    return null;
  }
  const left = clamp(((position.x - bounds.min_x) / (bounds.max_x - bounds.min_x)) * 100, 4, 96);
  const top = clamp(((position.y - bounds.min_y) / (bounds.max_y - bounds.min_y)) * 100, 4, 96);
  return { left, top };
}

function minimapMemoryDetailID(
  state: ClientState,
  memory: MinimapMemory,
  bounds: MapBounds | null,
  center: { x: number; y: number },
  halfExtent: number,
): string | null {
  if (!isClickableMinimapMemory(memory)) {
    return null;
  }
  const currentMapKey = currentPublicMapKey(state);
  if (currentMapKey) {
    return memory.public_map_key === currentMapKey ? memory.detail_id || memory.planet_id || null : null;
  }
  if (bounds) {
    return rememberedMinimapDetailID(state, memory);
  }
  return shouldRenderRememberedMinimapMemory(state, memory, center, halfExtent)
    ? rememberedMinimapDetailID(state, memory)
    : null;
}

function currentPublicMapKey(state: ClientState): string | null {
  return cleanText(state.currentMap?.public_map_key) ?? cleanText(state.currentMap?.map_key) ?? cleanText(state.minimap?.public_map_key);
}

function collectMinimapPortals(state: ClientState): PublicPortalSummary[] {
  const portals: PublicPortalSummary[] = [];
  const seen = new Set<string>();
  const add = (items: PublicPortalSummary[] | undefined): void => {
    for (const portal of items ?? []) {
      if (!portal.portal_id || seen.has(portal.portal_id) || !isFinitePoint(portal.position)) {
        continue;
      }
      seen.add(portal.portal_id);
      portals.push(portal);
    }
  };
  add(state.currentMap?.visible_portals);
  add(state.minimap?.visible_portals);
  return portals;
}

function collectMinimapSafeZones(state: ClientState): SafeZoneProjection[] {
  const safeZones: SafeZoneProjection[] = [];
  const seen = new Set<string>();
  const add = (items: SafeZoneProjection[] | undefined): void => {
    for (const zone of items ?? []) {
      if (!zone.safe_area_id || seen.has(zone.safe_area_id) || !isFinitePoint(zone.center) || !Number.isFinite(zone.radius) || zone.radius <= 0) {
        continue;
      }
      seen.add(zone.safe_area_id);
      safeZones.push(zone);
    }
  };
  add(state.currentMap?.safe_zones);
  add(state.minimap?.safe_zones);
  return safeZones;
}

function minimapDetailLabels(state: ClientState, bounds: MapBounds | null): string[] {
  const labels: string[] = [];
  if (state.minimap && state.minimap.radar_range > 0) {
    labels.push(`Radar ${formatCompactNumber(state.minimap.radar_range)}`);
  }
  if (bounds) {
    labels.push(`Bounds ${formatCompactNumber(bounds.max_x - bounds.min_x)} x ${formatCompactNumber(bounds.max_y - bounds.min_y)}`);
  }
  const policy = currentMapPolicyLabel(state);
  if (policy) {
    labels.push(policy);
  }
  return labels.slice(0, 3);
}

function currentMapPolicyLabel(state: ClientState): string | null {
  const risk = cleanText(state.currentMap?.risk_band);
  const policy = cleanText(state.currentMap?.pvp_policy);
  if (risk && policy && risk.toLowerCase() !== policy.toLowerCase()) {
    return `${risk}/${policy}`;
  }
  return risk ?? policy;
}

function zoneMarkerSizePercent(projection: MinimapProjection, radius: number): number {
  if (projection.bounds) {
    const width = projection.bounds.max_x - projection.bounds.min_x;
    const height = projection.bounds.max_y - projection.bounds.min_y;
    return clamp((radius * 2 * 100) / Math.min(width, height), 7, 60);
  }
  return clamp((radius * 100) / projection.radius, 7, 60);
}

function minimapPointStyle(point: { left: number; top: number }): string {
  return `left:${minimapPercent(point.left)}%;top:${minimapPercent(point.top)}%`;
}

function minimapPercent(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(2).replace(/0+$/, '').replace(/\.$/, '');
}

function markerLabel(value: string | undefined, fallback: string): string {
  return cleanText(value) ?? fallback;
}

function cleanText(value: string | undefined): string | null {
  const text = value?.trim();
  return text ? text : null;
}

function isUsableBounds(bounds: MapBounds | undefined): bounds is MapBounds {
  return Boolean(
    bounds &&
      Number.isFinite(bounds.min_x) &&
      Number.isFinite(bounds.min_y) &&
      Number.isFinite(bounds.max_x) &&
      Number.isFinite(bounds.max_y) &&
      bounds.max_x > bounds.min_x &&
      bounds.max_y > bounds.min_y,
  );
}

function isFinitePoint(position: { x: number; y: number }): boolean {
  return Number.isFinite(position.x) && Number.isFinite(position.y);
}

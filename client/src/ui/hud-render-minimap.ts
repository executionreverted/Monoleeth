import { OPERATIONS } from '../protocol/envelope';
import type { ClientState, MapBounds, MinimapContact, MinimapMemory, MinimapSummary, PublicPortalSummary, SafeZoneProjection } from '../state/types';
import {
  isClickableMinimapMemory,
  minimapPointPercent,
  rememberedIntelState,
  rememberedMinimapDetailID,
  shouldRenderRememberedMinimapMemory,
} from '../state/world-memory';
import { hudSelection } from './hud-selection';
import { clamp, dispositionForType, escapeHTML, formatCompactNumber, formatCooldown, hasPendingOp, lockedValue, publicEntityType, realtimeReady } from './hud-formatters';
import { topbarLocationText } from './hud-topbar';

export function minimapPanel(state: ClientState, serverNow: number | null = Date.now()): string {
  const bounds = chooseMinimapBounds(state);
  if (!bounds && !hasRenderableMinimapPayload(state.minimap)) {
    reconcilePortalSelectionForRender(state, null);
    return `
      <div class="minimap minimap--empty"><div class="empty-line">Awaiting map projection.</div></div>
      ${portalActionStrip(state, [], serverNow)}
    `;
  }

  const portals = collectMinimapPortals(state);
  const portalScope = portalSelectionScope(state);
  reconcilePortalSelectionForRender(state, portalScope);
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
  const portalMarkers = portals
    .map((portal) => {
      const point = projectMinimapPoint(projection, portal.position);
      if (!point) {
        return '';
      }
      const selected = isSelectedPortal(portal.portal_id, portalScope);
      const label = portalLabel(portal);
      const scopeAttr = portalScope ? ` data-portal-scope="${escapeHTML(portalScope)}"` : '';
      const disabledAttr = portalScope ? '' : ' disabled';
      const stateAttr = portal.state ? ` data-portal-state="${escapeHTML(portal.state)}"` : '';
      return `<button class="minimap__portal" type="button" data-action="portal-select" data-marker="portal" data-portal-id="${escapeHTML(portal.portal_id)}"${scopeAttr}${stateAttr}${disabledAttr} data-selected="${selected ? 'true' : 'false'}" aria-pressed="${selected ? 'true' : 'false'}" style="${minimapPointStyle(point)}" title="${escapeHTML(label)}"><span>${escapeHTML(label)}</span></button>`;
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
    ${portalActionStrip(state, portals, serverNow)}
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
  const seen = new Map<string, number>();
  const add = (items: PublicPortalSummary[] | undefined): void => {
    for (const portal of items ?? []) {
      if (!portal.portal_id || !isFinitePoint(portal.position)) {
        continue;
      }
      const existingIndex = seen.get(portal.portal_id);
      if (existingIndex === undefined) {
        seen.set(portal.portal_id, portals.length);
        portals.push(portal);
        continue;
      }
      portals[existingIndex] = mergePortalSummary(portals[existingIndex], portal);
    }
  };
  add(state.currentMap?.visible_portals);
  add(state.minimap?.visible_portals);
  return portals;
}

function mergePortalSummary(primary: PublicPortalSummary, secondary: PublicPortalSummary): PublicPortalSummary {
  return {
    ...secondary,
    ...primary,
    label: primary.label ?? secondary.label,
    display_name: primary.display_name ?? secondary.display_name,
    destination_label: primary.destination_label ?? secondary.destination_label,
    state: primary.state ?? secondary.state,
    cooldown_ready_at_ms: primary.cooldown_ready_at_ms ?? secondary.cooldown_ready_at_ms,
    locked_reason: primary.locked_reason ?? secondary.locked_reason,
  };
}

function portalActionStrip(state: ClientState, portals: PublicPortalSummary[], serverNow: number | null): string {
  const currentScope = portalSelectionScope(state);
  const selectedID = hudSelection.selectedPortalID;
  const selectedScope = hudSelection.selectedPortalScope;
  const selectedPortal =
    selectedID && selectedScope && selectedScope === currentScope
      ? portals.find((portal) => portal.portal_id === selectedID) ?? null
      : null;
  const now = serverNow ?? Date.now();

  if (!state.currentMap && !state.minimap) {
    const label = realtimePortalListLoading(state) ? 'Awaiting portal list' : 'Portal list locked';
    return emptyPortalStrip(label, 'Map snapshot required.');
  }

  if (portals.length === 0) {
    return emptyPortalStrip('No visible portals', 'Server snapshot has no selectable portal.');
  }

  if (selectedID && selectedScope !== currentScope) {
    return emptyPortalStrip('Select a portal', 'Fresh click required.');
  }

  if (selectedID && !selectedPortal) {
    return emptyPortalStrip('Portal signal lost', 'Selected portal is no longer visible.');
  }

  if (!selectedPortal) {
    return emptyPortalStrip('Select a portal', `${portals.length} visible`);
  }

  const readiness = portalEnterReadiness(state, selectedPortal, currentScope, now);
  const destination = cleanText(selectedPortal.destination_label);
  const reason = readiness.reason ?? cleanText(selectedPortal.locked_reason);
  const stateLabel = selectedPortal.state ?? lockedValue();
  const scopeAttr = currentScope ? ` data-portal-scope="${escapeHTML(currentScope)}"` : '';
  return `
    <div class="portal-strip" data-portal-strip="true" data-portal-state="${escapeHTML(selectedPortal.state ?? 'unknown')}"${scopeAttr}>
      <div class="portal-strip__main">
        <span>Portal</span>
        <strong>${escapeHTML(portalLabel(selectedPortal))}</strong>
      </div>
      <div class="portal-strip__facts">
        ${destination ? `<span><em>Dest</em><strong>${escapeHTML(destination)}</strong></span>` : ''}
        <span><em>State</em><strong>${escapeHTML(stateLabel)}</strong></span>
        ${reason ? `<span><em>Status</em><strong>${escapeHTML(reason)}</strong></span>` : ''}
      </div>
      <button class="portal-strip__action" type="button" data-action="portal-enter" data-portal-id="${escapeHTML(selectedPortal.portal_id)}"${scopeAttr} ${readiness.enabled ? '' : 'disabled'} title="${escapeHTML(readiness.title)}">Enter</button>
    </div>
  `;
}

function emptyPortalStrip(label: string, detail: string): string {
  return `
    <div class="portal-strip portal-strip--empty" data-portal-strip="true" data-portal-state="empty">
      <div class="portal-strip__main">
        <span>Portal</span>
        <strong>${escapeHTML(label)}</strong>
      </div>
      <div class="portal-strip__facts">
        <span><em>Status</em><strong>${escapeHTML(detail)}</strong></span>
      </div>
      <button class="portal-strip__action" type="button" disabled>Enter</button>
    </div>
  `;
}

function portalEnterReadiness(
  state: ClientState,
  portal: PublicPortalSummary,
  currentScope: string | null,
  now: number,
): { enabled: boolean; title: string; reason: string | null } {
  if (!currentScope || hudSelection.selectedPortalScope !== currentScope) {
    return { enabled: false, title: 'Select a current-map portal.', reason: 'Fresh click required' };
  }
  if (!realtimeReady(state)) {
    return { enabled: false, title: 'Realtime link is not authenticated.', reason: 'Offline' };
  }
  if (hasPendingOp(state, OPERATIONS.portalEnter)) {
    return { enabled: false, title: 'Portal entry is pending.', reason: 'Pending' };
  }

  const readyAt = Math.max(portal.cooldown_ready_at_ms ?? 0, state.portalCooldowns[portal.portal_id] ?? 0);
  const remainingCooldown = readyAt - now;
  if (portal.state !== 'available') {
    if (portal.state === 'cooldown' && remainingCooldown > 0) {
      return { enabled: false, title: `Portal cooldown ${formatCooldown(remainingCooldown)}.`, reason: `Ready ${formatCooldown(remainingCooldown)}` };
    }
    return { enabled: false, title: 'Portal entry unavailable.', reason: cleanText(portal.locked_reason) };
  }
  if (remainingCooldown > 0) {
    return { enabled: false, title: `Portal cooldown ${formatCooldown(remainingCooldown)}.`, reason: `Ready ${formatCooldown(remainingCooldown)}` };
  }
  return { enabled: true, title: 'Enter selected portal.', reason: null };
}

function realtimePortalListLoading(state: ClientState): boolean {
  return state.connectionStatus === 'connecting' || state.connectionStatus === 'connected' || state.connectionStatus === 'authenticated_pending_socket';
}

function isSelectedPortal(portalID: string, currentScope: string | null): boolean {
  return Boolean(portalID && currentScope && hudSelection.selectedPortalID === portalID && hudSelection.selectedPortalScope === currentScope);
}

function reconcilePortalSelectionForRender(state: ClientState, currentScope: string | null): void {
  if (!hudSelection.selectedPortalID && !hudSelection.selectedPortalScope) {
    return;
  }
  if (!currentScope || (state.auth.mode === 'real' && state.connectionStatus !== 'connected')) {
    hudSelection.selectedPortalID = null;
    hudSelection.selectedPortalScope = null;
  }
}

function portalSelectionScope(state: ClientState): string | null {
  if (!realtimeReady(state)) {
    return null;
  }
  const mapIdentity =
    cleanText(state.currentMap?.public_map_key) ??
    cleanText(state.currentMap?.map_key) ??
    cleanText(state.currentMap?.display_name) ??
    cleanText(state.minimap?.public_map_key);
  if (!mapIdentity || state.mapSubscriptionEpoch === null) {
    return null;
  }
  return `${mapIdentity}:${state.mapSubscriptionEpoch}`;
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

function portalLabel(portal: PublicPortalSummary): string {
  return cleanText(portal.label) ?? cleanText(portal.display_name) ?? portal.portal_id;
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

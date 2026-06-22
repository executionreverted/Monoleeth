import { rejectForbiddenPayloadKeys, type JsonObject } from '../protocol/envelope';
import type {
  ClientState,
  MapBounds,
  MapSummary,
  PublicPortalSummary,
  SafeZoneProjection,
  ViewerProtectionSummary,
  ViewerSafeZoneSummary,
} from './types';
import { booleanField, isJsonObject, isVec2, numberField, objectField, stringField } from './reducer-helpers';

export interface ParsedMapSnapshotSummary {
  present: boolean;
  summary: MapSummary | null;
}

export interface ApplyMapSnapshotOptions {
  clearMapScopedState?: (state: ClientState) => ClientState;
  forceClearMapScopedState?: boolean;
}

export function applyMapSnapshotPayload(
  state: ClientState,
  payload: JsonObject,
  options: ApplyMapSnapshotOptions = {},
): ClientState {
  rejectForbiddenPayloadKeys(payload);

  const mapSubscriptionEpoch = mapSubscriptionEpochFromPayload(payload);
  const mapSnapshot = parseMapSnapshotSummary(payload);
  const shouldClear =
    options.forceClearMapScopedState === true ||
    shouldClearMapScopedState(state, mapSnapshot, mapSubscriptionEpoch);
  let next =
    shouldClear && options.clearMapScopedState
      ? options.clearMapScopedState(state)
      : state;

  if (mapSubscriptionEpoch !== null) {
    next = {
      ...next,
      mapSubscriptionEpoch,
    };
  }

  if (mapSnapshot.present) {
    next = {
      ...next,
      currentMap: mapSnapshot.summary,
    };
  }

  return next;
}

export function parseMapSnapshotSummary(payload: JsonObject): ParsedMapSnapshotSummary {
  if ('map' in payload) {
    const mapPayload = objectField(payload, 'map');
    return {
      present: true,
      summary: mapPayload ? parseMapSummary(mapPayload) : null,
    };
  }

  if (!isDirectMapSummaryPayload(payload)) {
    return { present: false, summary: null };
  }

  const summary = parseMapSummary(payload);
  return summary ? { present: true, summary } : { present: false, summary: null };
}

export function mapSubscriptionEpochFromPayload(payload: JsonObject): number | null {
  const epoch = numberField(payload, 'map_subscription_epoch');
  if (epoch === null || epoch <= 0) {
    return null;
  }
  return Math.round(epoch);
}

export function parseMapSummary(payload: JsonObject): MapSummary | null {
  rejectForbiddenPayloadKeys(payload);

  const boundsPayload = objectField(payload, 'bounds');
  const bounds = boundsPayload ? parseMapBounds(boundsPayload) : null;
  if (!bounds || !hasPublicMapIdentity(payload)) {
    return null;
  }

  const map: MapSummary = {
    bounds,
    visible_portals: parsePortalSummaries(payload.visible_portals),
    safe_zones: parseSafeZoneProjections(payload.safe_zones),
  };

  copyOptionalString(payload, map, 'map_key');
  copyOptionalString(payload, map, 'public_map_key');
  copyOptionalString(payload, map, 'display_name');
  copyOptionalString(payload, map, 'region');
  copyOptionalString(payload, map, 'risk_band');
  copyOptionalString(payload, map, 'pvp_policy');
  copyOptionalString(payload, map, 'visual_theme_key');

  const safeZonePayload = objectField(payload, 'safe_zone');
  const safeZone = safeZonePayload ? parseViewerSafeZoneSummary(safeZonePayload) : null;
  if (safeZone) {
    map.safe_zone = safeZone;
  }

  const protectionPayload = objectField(payload, 'protection');
  const protection = protectionPayload ? parseViewerProtectionSummary(protectionPayload) : null;
  if (protection) {
    map.protection = protection;
  }

  return map;
}

function shouldClearMapScopedState(
  state: ClientState,
  mapSnapshot: ParsedMapSnapshotSummary,
  mapSubscriptionEpoch: number | null,
): boolean {
  if (mapSubscriptionEpoch !== null && mapSubscriptionEpoch !== state.mapSubscriptionEpoch) {
    return true;
  }

  if (!mapSnapshot.present || !mapSnapshot.summary) {
    return false;
  }

  const nextIdentity = publicMapIdentity(mapSnapshot.summary);
  const currentIdentity = state.currentMap ? publicMapIdentity(state.currentMap) : null;
  if (currentIdentity && nextIdentity) {
    return currentIdentity !== nextIdentity;
  }
  return nextIdentity !== null && currentIdentity === null && hasMapScopedLiveState(state);
}

function publicMapIdentity(map: MapSummary): string | null {
  return map.public_map_key ?? map.map_key ?? map.display_name ?? null;
}

function hasMapScopedLiveState(state: ClientState): boolean {
  return (
    Object.keys(state.visibleEntities).length > 0 ||
    Object.keys(state.knownLoot).length > 0 ||
    state.selectedTargetID !== null ||
    state.movementTarget !== null ||
    state.lastCorrection !== null ||
    state.worldEffects.length > 0 ||
    (state.minimap?.live_contacts.length ?? 0) > 0 ||
    state.minimap?.public_map_key !== undefined ||
    state.minimap?.bounds !== undefined ||
    (state.minimap?.visible_portals?.length ?? 0) > 0 ||
    (state.minimap?.safe_zones?.length ?? 0) > 0
  );
}

export function parseMapBounds(payload: JsonObject): MapBounds | null {
  const minX = numberField(payload, 'min_x');
  const minY = numberField(payload, 'min_y');
  const maxX = numberField(payload, 'max_x');
  const maxY = numberField(payload, 'max_y');
  if (minX === null || minY === null || maxX === null || maxY === null || maxX <= minX || maxY <= minY) {
    return null;
  }
  return {
    min_x: minX,
    min_y: minY,
    max_x: maxX,
    max_y: maxY,
  };
}

export function parsePortalSummaries(value: unknown): PublicPortalSummary[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter(isJsonObject).map(parsePortalSummary).filter((portal): portal is PublicPortalSummary => portal !== null);
}

export function parseSafeZoneProjections(value: unknown): SafeZoneProjection[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter(isJsonObject).map(parseSafeZoneProjection).filter((zone): zone is SafeZoneProjection => zone !== null);
}

function parsePortalSummary(payload: JsonObject): PublicPortalSummary | null {
  rejectForbiddenPayloadKeys(payload);
  rejectForbiddenPortalSummaryKeys(payload);

  const portalID = nonEmptyString(payload, 'portal_id');
  const radius = numberField(payload, 'interaction_radius');
  if (!portalID || !isVec2(payload.position) || radius === null || radius <= 0) {
    return null;
  }

  const portal: PublicPortalSummary = {
    portal_id: portalID,
    position: payload.position,
    interaction_radius: radius,
  };
  copyOptionalString(payload, portal, 'label');
  copyOptionalString(payload, portal, 'display_name');

  copyOptionalString(payload, portal, 'destination_label');
  copyOptionalString(payload, portal, 'locked_reason');

  const state = portalState(payload);
  if (state) {
    portal.state = state;
  }

  const cooldownReadyAt = numberField(payload, 'cooldown_ready_at_ms');
  if (cooldownReadyAt !== null && cooldownReadyAt >= 0) {
    portal.cooldown_ready_at_ms = Math.round(cooldownReadyAt);
  }

  return portal;
}

function parseSafeZoneProjection(payload: JsonObject): SafeZoneProjection | null {
  const safeAreaID = nonEmptyString(payload, 'safe_area_id');
  const radius = numberField(payload, 'radius');
  const blocksPVP = booleanField(payload, 'blocks_pvp');
  const hangarActions = booleanField(payload, 'hangar_actions');
  if (!safeAreaID || !isVec2(payload.center) || radius === null || radius <= 0 || blocksPVP === null || hangarActions === null) {
    return null;
  }

  const zone: SafeZoneProjection = {
    safe_area_id: safeAreaID,
    center: payload.center,
    radius,
    blocks_pvp: blocksPVP,
    hangar_actions: hangarActions,
  };
  copyOptionalString(payload, zone, 'display_name');
  return zone;
}

function parseViewerSafeZoneSummary(payload: JsonObject): ViewerSafeZoneSummary | null {
  const inside = booleanField(payload, 'inside');
  const blocksPVP = booleanField(payload, 'blocks_pvp');
  if (inside === null || blocksPVP === null) {
    return null;
  }
  const summary: ViewerSafeZoneSummary = {
    inside,
    blocks_pvp: blocksPVP,
  };
  const protectionExpiresAt = numberField(payload, 'protection_expires_at');
  if (protectionExpiresAt !== null) {
    summary.protection_expires_at = Math.round(protectionExpiresAt);
  }
  return summary;
}

function parseViewerProtectionSummary(payload: JsonObject): ViewerProtectionSummary | null {
  const reason = nonEmptyString(payload, 'reason');
  const expiresAt = numberField(payload, 'expires_at');
  const blocksPVP = booleanField(payload, 'blocks_pvp');
  const breakOnPVPAction = booleanField(payload, 'break_on_pvp_action');
  if (!reason || expiresAt === null || blocksPVP === null || breakOnPVPAction === null) {
    return null;
  }
  return {
    reason,
    expires_at: Math.round(expiresAt),
    blocks_pvp: blocksPVP,
    break_on_pvp_action: breakOnPVPAction,
  };
}

function hasPublicMapIdentity(payload: JsonObject): boolean {
  return Boolean(
    nonEmptyString(payload, 'public_map_key') ||
      nonEmptyString(payload, 'map_key') ||
      nonEmptyString(payload, 'display_name'),
  );
}

function isDirectMapSummaryPayload(payload: JsonObject): boolean {
  return objectField(payload, 'bounds') !== null && hasPublicMapIdentity(payload);
}

function nonEmptyString(payload: JsonObject, key: string): string | null {
  const value = stringField(payload, key)?.trim();
  return value ? value : null;
}

function copyOptionalString<T extends object>(source: JsonObject, target: T, key: string): void {
  const value = nonEmptyString(source, key);
  if (value) {
    (target as Record<string, unknown>)[key] = value;
  }
}

function portalState(payload: JsonObject): PublicPortalSummary['state'] | null {
  const value = nonEmptyString(payload, 'state');
  if (value === 'available' || value === 'cooldown' || value === 'locked' || value === 'offline') {
    return value;
  }
  return null;
}

const forbiddenPortalSummaryKeys = new Set([
  'destination',
  'destination_id',
  'destination_key',
  'destination_map_id',
  'destination_map_key',
  'destination_position',
  'destination_public_key',
  'destination_public_map_key',
  'destination_spawn_id',
  'from_map_key',
  'from_public_map_key',
  'internal_map_id',
  'map_id',
  'map_key',
  'public_map_key',
  'spawn',
  'spawn_map_key',
  'spawn_point',
  'spawn_position',
  'spawn_public_map_key',
  'to_map_key',
  'to_public_map_key',
]);

function rejectForbiddenPortalSummaryKeys(payload: JsonObject): void {
  const found = findForbiddenPortalSummaryKey(payload);
  if (found) {
    throw new Error(`Forbidden server payload rejected: ${found}`);
  }
}

function findForbiddenPortalSummaryKey(value: unknown, depth = 0): string | null {
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findForbiddenPortalSummaryKey(item, depth + 1);
      if (found) {
        return found;
      }
    }
    return null;
  }

  if (!isJsonObject(value)) {
    return null;
  }

  for (const [key, child] of Object.entries(value)) {
    const normalized = key.toLowerCase();
    if (forbiddenPortalSummaryKeys.has(normalized)) {
      return key;
    }
    const found = findForbiddenPortalSummaryKey(child, depth + 1);
    if (found) {
      return found;
    }
  }
  return null;
}

import { EntityPayload, JsonObject } from '../protocol/envelope';
import type {
  ClientState,
  MinimapContact,
  MinimapMemory,
  MinimapSummary,
  KnownPlanetSummary,
  PlanetDetailSummary,
  PlanetIntelSummary,
  PlanetProductionSummary,
  PlanetStorageSummary,
  ProductionCollectionSummary,
  RouteListSummary,
  RouteSummary,
  ScanModeState,
  ScanPulseSummary,
  SectorSummary,
} from './types';
import {
  booleanField,
  isJsonObject,
  isKnownEntityType,
  isVec2,
  numberField,
  objectField,
  optionalRoundedNumber,
  parsePublicStatusFlags,
  roundedOptional,
  stringField,
} from './reducer-helpers';
import { isRenderableMinimapMemory } from './world-memory';

const SCAN_REPEAT_DELAY_MS = 2_800;
const SCAN_STARTED_RECHECK_MS = 350;

export function parseScanPulse(payload: JsonObject, fallback: ScanPulseSummary | null): ScanPulseSummary {
  const signal = objectField(payload, 'signal');
  const status = stringField(payload, 'status') ?? fallback?.status ?? 'unknown';
  const clearsPlanetResult = status === 'started' || status === 'no_signal' || status === 'player_revealed';
  return {
    pulse_reference: stringField(payload, 'pulse_reference') ?? fallback?.pulse_reference ?? '',
    status,
    resolve_after: optionalRoundedNumber(payload, 'resolve_after', fallback?.resolve_after),
    message: stringField(payload, 'message') ?? fallback?.message,
    signal: signal
      ? {
          biome: stringField(signal, 'biome') ?? '',
          signal_band: stringField(signal, 'signal_band') ?? '',
          approx_distance: stringField(signal, 'approx_distance') ?? '',
        }
      : clearsPlanetResult
        ? undefined
        : fallback?.signal,
    planet_id: stringField(payload, 'planet_id') ?? (clearsPlanetResult ? undefined : fallback?.planet_id),
    xp_granted: booleanField(payload, 'xp_granted') ?? (clearsPlanetResult ? false : fallback?.xp_granted),
    duplicate: booleanField(payload, 'duplicate') ?? fallback?.duplicate,
  };
}

export function parseKnownPlanets(payload: JsonObject, fallback: PlanetIntelSummary | null): PlanetIntelSummary {
  const planets = Array.isArray(payload.planets)
    ? payload.planets
        .filter(isJsonObject)
        .map(parseKnownPlanet)
        .filter((planet): planet is KnownPlanetSummary => planet !== null)
    : fallback?.planets ?? [];
  const counts = objectField(payload, 'counts');
  const stale = Math.max(0, Math.round(numberField(counts ?? {}, 'stale') ?? fallback?.staleIntel ?? 0));
  const owned = Math.max(0, Math.round(numberField(counts ?? {}, 'owned') ?? fallback?.ownedPlanets ?? 0));
  const known = Math.max(
    planets.length,
    Math.round(numberField(counts ?? {}, 'known') ?? fallback?.knownSignals ?? planets.length),
  );
  const selectedPlanet =
    fallback?.selectedPlanet && planets.some((planet) => planet.planet_id === fallback.selectedPlanet?.planet_id)
      ? fallback.selectedPlanet
      : null;

  return {
    knownSignals: known,
    staleIntel: stale,
    ownedPlanets: owned,
    planets,
    selectedPlanet,
    lastScan: fallback?.lastScan ?? null,
  };
}

function parseKnownPlanet(payload: JsonObject): KnownPlanetSummary | null {
  const planetID = stringField(payload, 'planet_id') ?? '';
  if (!planetID) {
    return null;
  }
  const planet: KnownPlanetSummary = {
    planet_id: planetID,
    biome: stringField(payload, 'biome') ?? '',
    planet_type: stringField(payload, 'planet_type') ?? '',
    rarity: stringField(payload, 'rarity') ?? '',
    level: Math.max(0, Math.round(numberField(payload, 'level') ?? 0)),
    intel_state: stringField(payload, 'intel_state') ?? '',
    confidence: Math.max(0, Math.round(numberField(payload, 'confidence') ?? 0)),
    last_seen_at: Math.max(0, Math.round(numberField(payload, 'last_seen_at') ?? 0)),
    owner_status: stringField(payload, 'owner_status') ?? '',
    discovered_at: Math.max(0, Math.round(numberField(payload, 'discovered_at') ?? 0)),
  };
  const sectorKey = stringField(payload, 'sector_key');
  if (sectorKey) {
    planet.sector_key = sectorKey;
  }
  return planet;
}

export function parsePlanetDetail(payload: JsonObject, fallback: PlanetDetailSummary | null): PlanetDetailSummary {
  const parsed = parseKnownPlanet(payload);
  const planetID = parsed?.planet_id ?? stringField(payload, 'planet_id') ?? fallback?.planet_id ?? '';
  const matchingFallback = fallback?.planet_id === planetID ? fallback : null;
  const base = parsed ?? matchingFallback;
  const coordinates = isVec2(payload.coordinates) ? payload.coordinates : matchingFallback?.coordinates ?? null;
  const production = objectField(payload, 'production');
  const routes = Array.isArray(payload.routes)
    ? payload.routes
        .filter(isJsonObject)
        .map(parseRoute)
        .filter((route): route is RouteSummary => route !== null)
    : matchingFallback?.routes ?? [];
  return {
    ...(base ?? {
      planet_id: '',
      biome: '',
      planet_type: '',
      rarity: '',
      level: 0,
      intel_state: '',
      confidence: 0,
      last_seen_at: 0,
      owner_status: '',
      discovered_at: 0,
    }),
    coordinates,
    production: production ? parseProductionPlanet(production) ?? undefined : matchingFallback?.production,
    routes,
    production_locked: booleanField(payload, 'production_locked') ?? matchingFallback?.production_locked ?? true,
    available_commands: Array.isArray(payload.available_commands)
      ? payload.available_commands.filter((command): command is string => typeof command === 'string')
      : matchingFallback?.available_commands ?? [],
  };
}

export function parseProductionCollection(payload: JsonObject, fallback: ProductionCollectionSummary | null): ProductionCollectionSummary {
  const planets = Array.isArray(payload.planets)
    ? payload.planets
        .filter(isJsonObject)
        .map(parseProductionPlanet)
        .filter((planet): planet is PlanetProductionSummary => planet !== null)
    : fallback?.planets ?? [];
  return { planets };
}

function parseProductionPlanet(payload: JsonObject): PlanetProductionSummary | null {
  const planetID = stringField(payload, 'planet_id') ?? '';
  const storage = objectField(payload, 'storage');
  if (!planetID || !storage) {
    return null;
  }
  const planet: PlanetProductionSummary = {
    planet_id: planetID,
    production_enabled: booleanField(payload, 'production_enabled') ?? false,
    last_calculated_at: Math.max(0, Math.round(numberField(payload, 'last_calculated_at') ?? 0)),
    energy_capacity_per_hour: Math.max(0, Math.round(numberField(payload, 'energy_capacity_per_hour') ?? 0)),
    energy_reserved_per_hour: Math.max(0, Math.round(numberField(payload, 'energy_reserved_per_hour') ?? 0)),
    storage: parsePlanetStorage(storage),
    buildings: Array.isArray(payload.buildings)
      ? payload.buildings
          .filter(isJsonObject)
          .map(parsePlanetBuilding)
          .filter((building): building is PlanetProductionSummary['buildings'][number] => building !== null)
      : [],
  };
  const publicMapKey = stringField(payload, 'public_map_key');
  if (publicMapKey) {
    planet.public_map_key = publicMapKey;
  }
  return planet;
}

export function parsePlanetStorage(payload: JsonObject): PlanetStorageSummary {
  const storage: PlanetStorageSummary = {
    planet_id: stringField(payload, 'planet_id') ?? '',
    used_units: Math.max(0, Math.round(numberField(payload, 'used_units') ?? 0)),
    free_units: Math.max(0, Math.round(numberField(payload, 'free_units') ?? 0)),
    capacity_units: Math.max(0, Math.round(numberField(payload, 'capacity_units') ?? 0)),
    updated_at: Math.max(0, Math.round(numberField(payload, 'updated_at') ?? 0)),
    items: Array.isArray(payload.items)
      ? payload.items
          .filter(isJsonObject)
          .map((item) => ({
            item_id: stringField(item, 'item_id') ?? '',
            quantity: Math.max(0, Math.round(numberField(item, 'quantity') ?? 0)),
          }))
          .filter((item) => item.item_id !== '' && item.quantity > 0)
      : [],
  };
  const publicMapKey = stringField(payload, 'public_map_key');
  if (publicMapKey) {
    storage.public_map_key = publicMapKey;
  }
  return storage;
}

export function applyPlanetStorageSummary(state: ClientState, storage: PlanetStorageSummary): ClientState {
  if (!storage.planet_id) {
    return state;
  }

  const production = state.production
    ? {
        planets: state.production.planets.map((planet) =>
          planet.planet_id === storage.planet_id ? { ...planet, storage } : planet,
        ),
      }
    : state.production;

  const currentPlanetIntel = state.planetIntel;
  const selectedPlanet = currentPlanetIntel?.selectedPlanet;
  const planetIntel =
    currentPlanetIntel && selectedPlanet && selectedPlanet.planet_id === storage.planet_id
      ? {
          ...currentPlanetIntel,
          selectedPlanet: {
            ...selectedPlanet,
            production: selectedPlanet.production
              ? { ...selectedPlanet.production, storage }
              : storageOnlyProductionSummary(storage),
          },
        }
      : currentPlanetIntel;

  return {
    ...state,
    production,
    planetIntel,
  };
}

function storageOnlyProductionSummary(storage: PlanetStorageSummary): PlanetProductionSummary {
  const production: PlanetProductionSummary = {
    planet_id: storage.planet_id,
    production_enabled: false,
    last_calculated_at: storage.updated_at,
    energy_capacity_per_hour: 0,
    energy_reserved_per_hour: 0,
    storage,
    buildings: [],
  };
  if (storage.public_map_key) {
    production.public_map_key = storage.public_map_key;
  }
  return production;
}

function parsePlanetBuilding(payload: JsonObject): PlanetProductionSummary['buildings'][number] | null {
  const buildingID = stringField(payload, 'building_id') ?? '';
  if (!buildingID) {
    return null;
  }
  const building: PlanetProductionSummary['buildings'][number] = {
    building_id: buildingID,
    building_type: stringField(payload, 'building_type') ?? '',
    category: stringField(payload, 'category') ?? '',
    level: Math.max(0, Math.round(numberField(payload, 'level') ?? 0)),
    state: stringField(payload, 'state') ?? '',
    updated_at: Math.max(0, Math.round(numberField(payload, 'updated_at') ?? 0)),
  };
  const planetID = stringField(payload, 'planet_id');
  const publicMapKey = stringField(payload, 'public_map_key');
  if (planetID) {
    building.planet_id = planetID;
  }
  if (publicMapKey) {
    building.public_map_key = publicMapKey;
  }
  return building;
}

export function parseRouteList(payload: JsonObject, fallback: RouteListSummary | null): RouteListSummary {
  const routes = Array.isArray(payload.routes)
    ? payload.routes
        .filter(isJsonObject)
        .map(parseRoute)
        .filter((route): route is RouteSummary => route !== null)
    : fallback?.routes ?? [];
  return { routes };
}

export function parseRoute(payload: JsonObject): RouteSummary | null {
  const routeID = stringField(payload, 'route_id') ?? '';
  const destination = objectField(payload, 'destination');
  if (!routeID || !destination) {
    return null;
  }
  const risk = objectField(payload, 'risk') ?? {};
  const route: RouteSummary = {
    route_id: routeID,
    source_planet_id: stringField(payload, 'source_planet_id') ?? '',
    destination: {
      type: stringField(destination, 'type') ?? '',
      id: stringField(destination, 'id') ?? '',
    },
    resource_item_id: stringField(payload, 'resource_item_id') ?? '',
    amount_per_hour: Math.max(0, Math.round(numberField(payload, 'amount_per_hour') ?? 0)),
    energy_cost_per_hour: Math.max(0, Math.round(numberField(payload, 'energy_cost_per_hour') ?? 0)),
    enabled: booleanField(payload, 'enabled') ?? false,
    risk: {
      loss_chance: Math.max(0, numberField(risk, 'loss_chance') ?? 0),
      min_loss_percent: Math.max(0, numberField(risk, 'min_loss_percent') ?? 0),
      max_loss_percent: Math.max(0, numberField(risk, 'max_loss_percent') ?? 0),
    },
    last_calculated_at: Math.max(0, Math.round(numberField(payload, 'last_calculated_at') ?? 0)),
    updated_at: Math.max(0, Math.round(numberField(payload, 'updated_at') ?? 0)),
  };
  const fromPublicMapKey = stringField(payload, 'from_public_map_key');
  const toPublicMapKey = stringField(payload, 'to_public_map_key');
  if (fromPublicMapKey) {
    route.from_public_map_key = fromPublicMapKey;
  }
  if (toPublicMapKey) {
    route.to_public_map_key = toPublicMapKey;
  }
  return route;
}

export function applyRouteSnapshot(state: ClientState, route: RouteSummary): ClientState {
  const routes = { routes: upsertRoute(state.routes?.routes ?? [], route) };
  const currentPlanetIntel = state.planetIntel;
  const selectedPlanet = currentPlanetIntel?.selectedPlanet;
  const shouldUpdateSelected =
    selectedPlanet &&
    (selectedPlanet.planet_id === route.source_planet_id ||
      selectedPlanet.routes.some((existingRoute) => existingRoute.route_id === route.route_id));
  const planetIntel =
    currentPlanetIntel && selectedPlanet && shouldUpdateSelected
      ? {
          ...currentPlanetIntel,
          selectedPlanet: {
            ...selectedPlanet,
            routes: upsertRoute(selectedPlanet.routes, route),
          },
        }
      : currentPlanetIntel;

  return {
    ...state,
    routes,
    planetIntel,
  };
}

function upsertRoute(routes: RouteSummary[], route: RouteSummary): RouteSummary[] {
  const index = routes.findIndex((existingRoute) => existingRoute.route_id === route.route_id);
  if (index === -1) {
    return [...routes, route];
  }
  return routes.map((existingRoute, routeIndex) => (routeIndex === index ? route : existingRoute));
}

export function parseSectorSummary(payload: JsonObject, fallback: SectorSummary | null): SectorSummary {
  const sector: SectorSummary = {
    name: stringField(payload, 'name') ?? fallback?.name ?? '',
    region: stringField(payload, 'region') ?? fallback?.region ?? '',
    danger: stringField(payload, 'danger') ?? fallback?.danger ?? '',
    contested: booleanField(payload, 'contested') ?? fallback?.contested ?? false,
  };
  const sectorKey = stringField(payload, 'sector_key') ?? fallback?.sector_key;
  if (sectorKey) {
    sector.sector_key = sectorKey;
  }
  return sector;
}

export function parseMinimapSummary(payload: JsonObject, fallback: MinimapSummary | null): MinimapSummary {
  const liveContacts = Array.isArray(payload.live_contacts)
    ? payload.live_contacts
        .filter(isJsonObject)
        .map(parseMinimapContact)
        .filter((contact): contact is MinimapContact => contact !== null)
    : fallback?.live_contacts ?? [];
  const remembered = Array.isArray(payload.remembered)
    ? payload.remembered
        .filter(isJsonObject)
        .map(parseMinimapMemory)
        .filter((memory): memory is MinimapMemory => memory !== null)
    : fallback?.remembered ?? [];

  return {
    radar_range: Math.max(0, numberField(payload, 'radar_range') ?? fallback?.radar_range ?? 0),
    projection_window_size: optionalRoundedNumber(payload, 'projection_window_size', fallback?.projection_window_size),
    live_contacts: liveContacts,
    remembered,
  };
}

function parseMinimapContact(payload: JsonObject): MinimapContact | null {
  const entityType = stringField(payload, 'entity_type') ?? '';
  const entityID = stringField(payload, 'entity_id') ?? '';
  const position = isVec2(payload.position) ? payload.position : null;
  if (!entityID || !isKnownEntityType(entityType) || !position) {
    return null;
  }
  const contact: MinimapContact = {
    entity_id: entityID,
    entity_type: entityType,
    position,
    disposition: stringField(payload, 'disposition') ?? undefined,
    status_flags: parsePublicStatusFlags(payload.status_flags),
  };
  const projectionSource = stringField(payload, 'projection_source');
  if (projectionSource) {
    contact.projection_source = projectionSource;
  }
  return contact;
}

function parseMinimapMemory(payload: JsonObject): MinimapMemory | null {
  if (!isVec2(payload.position)) {
    return null;
  }
  const memory: MinimapMemory = {
    kind: stringField(payload, 'kind') ?? '',
    planet_id: stringField(payload, 'planet_id') ?? undefined,
    detail_id: stringField(payload, 'detail_id') ?? undefined,
    label: stringField(payload, 'label') ?? '',
    position: payload.position,
    freshness: stringField(payload, 'freshness') ?? 'known',
  };
  const sectorKey = stringField(payload, 'sector_key');
  const invalidated = booleanField(payload, 'invalidated');
  const projectionSource = stringField(payload, 'projection_source');
  if (sectorKey) {
    memory.sector_key = sectorKey;
  }
  if (invalidated !== null) {
    memory.invalidated = invalidated;
  }
  if (projectionSource) {
    memory.projection_source = projectionSource;
  }
  return isRenderableMinimapMemory(memory) ? memory : null;
}

export function countPlanetSignals(entities: Record<string, EntityPayload>): number {
  return Object.values(entities).filter((entity) => entity.entity_type === 'planet_signal').length;
}

function emptyPlanetIntel(): PlanetIntelSummary {
  return {
    knownSignals: 0,
    staleIntel: null,
    ownedPlanets: 0,
    planets: [],
    selectedPlanet: null,
    lastScan: null,
  };
}

export function updateVisibleSignalCount(fallback: PlanetIntelSummary | null, visibleSignals: number): PlanetIntelSummary {
  const next = fallback ?? emptyPlanetIntel();
  return {
    ...next,
    knownSignals: Math.max(visibleSignals, next.planets.length, next.knownSignals),
  };
}

export function applyScanPulse(fallback: PlanetIntelSummary | null, scan: ScanPulseSummary): PlanetIntelSummary {
  const next = fallback ?? emptyPlanetIntel();
  return {
    ...next,
    knownSignals: scan.planet_id ? Math.max(next.knownSignals, next.planets.length, 1) : next.knownSignals,
    lastScan: scan,
  };
}

export function scanModeAfterPulseSummary(mode: ScanModeState, scan: ScanPulseSummary): ScanModeState {
  if (scan.status === 'started') {
    return scanModeAfterPulseStarted(mode, scan);
  }
  if (scan.status === 'resolved' || scan.status === 'planet_discovered' || scan.status === 'no_signal') {
    return scanModeAfterPulseResolved(mode);
  }
  return mode;
}

export function scanModeAfterPulseStarted(mode: ScanModeState, scan: ScanPulseSummary): ScanModeState {
  if (!mode.enabled) {
    return mode;
  }
  return {
    ...mode,
    nextPulseAt: scanResolveWakeAt(scan.resolve_after),
    lastError: null,
  };
}

export function scanModeAfterPulseResolved(mode: ScanModeState): ScanModeState {
  if (!mode.enabled) {
    return mode;
  }
  return {
    ...mode,
    nextPulseAt: Date.now() + SCAN_REPEAT_DELAY_MS,
    lastError: null,
  };
}

function scanResolveWakeAt(resolveAfter: number | undefined): number {
  const now = Date.now();
  if (Number.isFinite(resolveAfter) && typeof resolveAfter === 'number' && resolveAfter > now) {
    return Math.round(resolveAfter);
  }
  return now + SCAN_STARTED_RECHECK_MS;
}

export function applyPlanetDetail(fallback: PlanetIntelSummary | null, detail: PlanetDetailSummary): PlanetIntelSummary {
  const next = fallback ?? emptyPlanetIntel();
  const planets = next.planets.some((planet) => planet.planet_id === detail.planet_id)
    ? next.planets.map((planet) => (planet.planet_id === detail.planet_id ? { ...planet, ...detail } : planet))
    : detail.planet_id
      ? [...next.planets, detail]
      : next.planets;
  return {
    ...next,
    planets,
    knownSignals: Math.max(next.knownSignals, planets.length),
    selectedPlanet: detail.planet_id ? detail : next.selectedPlanet,
  };
}

export function scanLogLine(scan: ScanPulseSummary): string {
  if (scan.status === 'planet_discovered') {
    return `Scanner resolved ${scan.signal?.signal_band ?? 'unknown'} ${scan.signal?.biome ?? 'signal'}.`;
  }
  if (scan.status === 'started') {
    return 'Scanner pulse started.';
  }
  if (scan.status === 'player_revealed') {
    return scan.message || 'Scanner revealed a radar contact.';
  }
  return scan.message || 'Scanner pulse resolved with no signal.';
}

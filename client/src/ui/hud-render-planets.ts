import { OPERATIONS } from '../protocol/envelope';
import type { ClientState, KnownPlanetSummary, RouteSummary } from '../state/types';
import { escapeHTML, formatVec, hasPendingOpPayloadField, lockedValue, publicPlanetName, realtimeReady } from './hud-formatters';
import { hudSelection } from './hud-selection';

export { minimapPanel } from './hud-render-minimap';

export function planetsPanel(state: ClientState): string {
  const intel = state.planetIntel;
  const planets = intel?.planets.slice(0, 4) ?? [];
  const selected = intel?.selectedPlanet ?? null;
  return `
    <h2>Planets</h2>
    ${
      intel
        ? planets.length > 0
          ? `<ul class="planet-stack">
               ${planets
                 .map(
                   (planet) => {
                     const selectedPlanet = selected?.planet_id === planet.planet_id;
                     return (
                     `<li>
                        <button class="planet-row" type="button" data-action="planet-detail" data-planet-id="${escapeHTML(planet.planet_id)}" data-selected="${selectedPlanet ? 'true' : 'false'}" title="Open planet detail">
                          <span class="planet-orb" aria-hidden="true"></span>
                          <span><strong>${escapeHTML(publicPlanetName(planet))}</strong><small>${escapeHTML(planet.rarity || planet.intel_state || 'known')}</small></span>
                          <em>${escapeHTML(selectedPlanet ? 'selected' : planet.owner_status || 'intel')}</em>
                        </button>
                      </li>`
                     );
                   },
                 )
                 .join('')}
             </ul>`
          : '<div class="empty-line">No known planets yet.</div>'
        : '<div class="empty-line">Awaiting planet intel.</div>'
    }
    ${
      selected
        ? `<div class="empty-line">Selected ${escapeHTML(publicPlanetName(selected))}. Opened in planet detail.</div>`
        : '<div class="empty-line">Select a known planet to open detail.</div>'
    }
  `;
}

export function planetCatalogPanel(state: ClientState): string {
  const intel = state.planetIntel;
  if (!intel) {
    return `
      <h2>Planets</h2>
      <div class="empty-line">Awaiting planet catalog.</div>
    `;
  }
  const selectedSummary = intel.selectedPlanet ?? intel.planets[0] ?? null;
  const selectedDetail =
    intel.selectedPlanet && selectedSummary?.planet_id === intel.selectedPlanet.planet_id ? intel.selectedPlanet : null;
  const production = selectedSummary ? planetProductionFor(state, selectedSummary.planet_id, selectedDetail?.production) : null;
  const storage = production?.storage ?? null;
  const routes = selectedSummary ? planetRoutesFor(state, selectedSummary.planet_id, selectedDetail?.routes) : [];
  const routeControls = selectedSummary ? routeControlsPanel(state, selectedSummary, production, routes) : '';
  const buildingControls = selectedSummary ? buildingControlsPanel(state, selectedSummary, production) : '';
  const coordinates = selectedDetail?.coordinates ?? null;
  const canNavigate = Boolean(coordinates && Number.isFinite(coordinates.x) && Number.isFinite(coordinates.y));
  return `
    <h2>Planets</h2>
    <section class="planet-catalog">
      <div class="planet-catalog__list">
        <div class="planet-catalog__head">
          <strong>Catalog</strong>
          <span>${intel.knownSignals} known</span>
        </div>
        ${
          intel.planets.length > 0
            ? intel.planets.map((planet) => planetCatalogRow(planet, selectedSummary?.planet_id ?? '')).join('')
            : '<div class="empty-line">No known planets yet.</div>'
        }
      </div>
      <div class="planet-catalog__detail">
        ${
          selectedSummary
            ? `<div class="planet-catalog__hero" data-intel-state="${escapeHTML(selectedSummary.intel_state || 'unknown')}">
                 <span class="planet-orb planet-orb--large" aria-hidden="true"></span>
                 <div>
                   <span>${escapeHTML(selectedSummary.planet_type || 'planet')}</span>
                   <strong>${escapeHTML(publicPlanetName(selectedSummary))}</strong>
                   <em>${escapeHTML(selectedSummary.owner_status || selectedSummary.intel_state || 'intel')}</em>
                 </div>
               </div>
               <div class="planet-catalog__actions">
                 <button type="button" data-action="planet-navigate" data-planet-id="${escapeHTML(selectedSummary.planet_id)}" ${canNavigate ? '' : 'disabled'} title="${canNavigate ? 'Navigate to this known coordinate' : 'Select planet coordinates first'}">Navigate</button>
                 ${planetClaimButton(state, selectedSummary)}
               </div>
               <div class="planet-tabs" aria-label="Planet detail sections">
                 <span>Overview</span>
                 <span>Production</span>
                 <span>Storage</span>
                 <span>Routes</span>
                 <span>Intel</span>
               </div>
               <div class="planet-section-grid">
                 <section>
                   <h3>Overview</h3>
                   <div class="meta-row"><span>Coord</span><strong>${coordinates ? escapeHTML(formatVec(coordinates)) : lockedValue()}</strong></div>
                   <div class="meta-row"><span>Level</span><strong>${selectedSummary.level}</strong></div>
                   <div class="meta-row"><span>Rarity</span><strong>${escapeHTML(selectedSummary.rarity || 'known')}</strong></div>
                   <div class="meta-row"><span>Biome</span><strong>${escapeHTML(selectedSummary.biome || '--')}</strong></div>
                 </section>
                 <section>
                   <h3>Production</h3>
                   ${
                     production
                       ? `<div class="meta-row"><span>State</span><strong>${production.production_enabled ? 'online' : 'offline'}</strong></div>
                          <div class="meta-row"><span>Energy</span><strong>${production.energy_reserved_per_hour}/${production.energy_capacity_per_hour}/h</strong></div>
                          <div class="meta-row"><span>Buildings</span><strong>${production.buildings.length}</strong></div>
                          ${buildingControls}`
                       : `<div class="empty-line">${selectedDetail?.production_locked ? 'Production locked.' : 'No production summary.'}</div>`
                   }
                 </section>
                 <section>
                   <h3>Storage</h3>
                   ${
                     storage
                       ? `<div class="meta-row"><span>Used</span><strong>${storage.used_units}/${storage.capacity_units}</strong></div>
                          <div class="meta-row"><span>Free</span><strong>${storage.free_units}</strong></div>
                          ${
                            storage.items.length > 0
                              ? `<ul class="compact-list">${storage.items
                                  .slice(0, 3)
                                  .map((item) => `<li><span>${escapeHTML(item.item_id)}</span><strong>${item.quantity}</strong></li>`)
                                  .join('')}</ul>`
                              : '<div class="empty-line">Storage empty.</div>'
                          }`
                       : '<div class="empty-line">No storage summary.</div>'
                   }
                 </section>
                 <section>
                   <h3>Routes</h3>
                   ${routeControls}
                 </section>
                 <section>
                   <h3>Intel</h3>
                   <div class="meta-row"><span>State</span><strong>${escapeHTML(selectedSummary.intel_state || 'known')}</strong></div>
                   <div class="meta-row"><span>Confidence</span><strong>${selectedSummary.confidence}%</strong></div>
                   <div class="meta-row"><span>Owner</span><strong>${escapeHTML(selectedSummary.owner_status || 'intel')}</strong></div>
                 </section>
               </div>`
            : '<div class="empty-line">Select a discovered planet.</div>'
        }
      </div>
    </section>
  `;
}

function planetClaimButton(
  state: ClientState,
  planet: NonNullable<ClientState['planetIntel']>['planets'][number] | NonNullable<ClientState['planetIntel']>['selectedPlanet'],
): string {
  const planetID = planet?.planet_id ?? '';
  const pending = planetID ? hasPendingOpPayloadField(state, OPERATIONS.discoveryClaimPlanet, 'planet_id', planetID) : false;
  const ownerStatus = (planet?.owner_status ?? '').toLowerCase();
  const owned = ownerStatus === 'owned_by_you' || ownerStatus === 'owned' || ownerStatus.startsWith('owned_');
  const claimable = ownerStatus === 'unclaimed' || ownerStatus === 'claimable';
  const enabled = Boolean(planetID && realtimeReady(state) && claimable && !owned && !pending);
  const title = !planetID
    ? 'Planet id unavailable'
    : pending
      ? 'Planet claim pending'
      : owned
        ? 'Planet already claimed'
        : !realtimeReady(state)
          ? 'Realtime connection required'
          : claimable
            ? 'Send planet claim intent'
            : 'Planet cannot be claimed from current public state';
  return `<button type="button" data-action="planet-claim" data-planet-id="${escapeHTML(planetID)}" ${enabled ? '' : 'disabled'} title="${escapeHTML(title)}">${pending ? 'Claiming' : 'Claim'}</button>`;
}

export function planetCatalogRow(planet: NonNullable<ClientState['planetIntel']>['planets'][number], selectedPlanetID: string): string {
  const selected = planet.planet_id === selectedPlanetID;
  return `
    <button class="planet-catalog-row" type="button" data-action="planet-select" data-planet-id="${escapeHTML(planet.planet_id)}" data-selected="${selected ? 'true' : 'false'}" data-intel-state="${escapeHTML(planet.intel_state || 'known')}">
      <span class="planet-orb" aria-hidden="true"></span>
      <span>
        <strong>${escapeHTML(publicPlanetName(planet))}</strong>
        <em>${escapeHTML(planet.planet_type || planet.biome || 'planet')}</em>
      </span>
      <small>${escapeHTML(planet.owner_status || planet.intel_state || 'intel')}</small>
    </button>
  `;
}

export function planetProductionFor(
  state: ClientState,
  planetID: string,
  detailProduction?: NonNullable<ClientState['production']>['planets'][number],
): NonNullable<ClientState['production']>['planets'][number] | null {
  return detailProduction ?? state.production?.planets.find((planet) => planet.planet_id === planetID) ?? null;
}

export function planetRoutesFor(
  state: ClientState,
  planetID: string,
  detailRoutes?: NonNullable<ClientState['routes']>['routes'],
): NonNullable<ClientState['routes']>['routes'] {
  if (detailRoutes && detailRoutes.length > 0) {
    return detailRoutes;
  }
  return state.routes?.routes.filter((route) => route.source_planet_id === planetID) ?? [];
}

export function planetDetailModal(state: ClientState, planetID?: string): string {
  const intel = state.planetIntel;
  const selected = intel?.selectedPlanet ?? null;
  const detail = selected && (!planetID || selected.planet_id === planetID) ? selected : null;
  const summary = detail ?? intel?.planets.find((planet) => planet.planet_id === planetID) ?? null;
  if (!summary) {
    return `
      <h2>Planet</h2>
      <div class="empty-line">Awaiting planet intel.</div>
    `;
  }
  const coordinates = detail?.coordinates ?? null;
  const canNavigate = Boolean(coordinates && Number.isFinite(coordinates.x) && Number.isFinite(coordinates.y));
  const production = detail?.production;
  const storage = production?.storage;
  const routes = summary ? planetRoutesFor(state, summary.planet_id, detail?.routes) : [];
  const routeControls = routeControlsPanel(state, summary, production, routes);
  const buildingControls = buildingControlsPanel(state, summary, production);
  return `
    <h2>Planet</h2>
    <div class="planet-detail planet-detail--modal" data-planet-detail="${escapeHTML(summary.planet_id)}">
      <div class="planet-detail__hero">
        <span class="planet-orb planet-orb--large" aria-hidden="true"></span>
        <div>
          <div class="target-name">${escapeHTML(publicPlanetName(summary))}</div>
          <div class="target-kind">${escapeHTML(summary.rarity || summary.intel_state || 'known intel')}</div>
        </div>
      </div>
      <div class="systems-block">
        <div class="meta-row"><span>Coord</span><strong>${coordinates ? escapeHTML(formatVec(coordinates)) : lockedValue()}</strong></div>
        <div class="meta-row"><span>Level</span><strong>${summary.level}</strong></div>
        <div class="meta-row"><span>Owner</span><strong>${escapeHTML(summary.owner_status || 'intel')}</strong></div>
        <div class="meta-row"><span>Intel</span><strong>${escapeHTML(summary.intel_state || 'known')}</strong></div>
        <div class="meta-row"><span>Confidence</span><strong>${summary.confidence}%</strong></div>
      </div>
      <div class="segmented planet-actions">
        <button type="button" data-action="planet-navigate" data-planet-id="${escapeHTML(summary.planet_id)}" ${canNavigate ? '' : 'disabled'} title="${canNavigate ? 'Navigate to this known coordinate' : 'Request coordinates before navigating'}">Navigate</button>
        ${planetClaimButton(state, summary)}
      </div>
      <div class="systems-subhead">Production</div>
      ${
        production
          ? `<div class="systems-block">
               <div class="meta-row"><span>State</span><strong>${production.production_enabled ? 'online' : 'offline'}</strong></div>
               <div class="meta-row"><span>Energy</span><strong>${production.energy_reserved_per_hour}/${production.energy_capacity_per_hour}/h</strong></div>
               <div class="meta-row"><span>Storage</span><strong>${storage ? `${storage.used_units}/${storage.capacity_units}` : lockedValue()}</strong></div>
               <div class="meta-row"><span>Buildings</span><strong>${production.buildings.length}</strong></div>
               ${buildingControls}
             </div>`
          : `<div class="empty-line">${detail?.production_locked ? 'Production locked.' : 'No production snapshot for this planet yet.'}</div>`
      }
      <div class="systems-subhead">Routes</div>
      ${routeControls}
    </div>
  `;
}

export function planetModalTitle(state: ClientState, planetID?: string): string {
  const intel = state.planetIntel;
  const selected = intel?.selectedPlanet ?? null;
  const summary =
    (selected && (!planetID || selected.planet_id === planetID) ? selected : null) ??
    intel?.planets.find((planet) => planet.planet_id === planetID) ??
    null;
  return summary ? `Planet: ${publicPlanetName(summary)}` : 'Planet Detail';
}

function buildingControlsPanel(
  state: ClientState,
  planet: KnownPlanetSummary,
  production: NonNullable<ClientState['production']>['planets'][number] | null | undefined,
): string {
  if (!production) {
    return '';
  }
  const owned = isOwnedPlanet(planet);
  const buildPending = hasPendingOpPayloadField(state, OPERATIONS.planetBuildingBuild, 'planet_id', planet.planet_id);
  const controlsReady = realtimeReady(state) && owned && !buildPending;
  const slot = nextBuildingSlot(production);
  const buildTitle = !owned
    ? 'Own this planet before building'
    : !realtimeReady(state)
      ? 'Realtime connection required'
      : buildPending
        ? 'Building mutation pending'
        : 'Build planet structure';
  const buildingRows =
    production.buildings.length > 0
      ? `<ul class="compact-list">${production.buildings
          .slice(0, 4)
          .map((building) => {
            const upgradePending = hasPendingOpPayloadField(state, OPERATIONS.planetBuildingUpgrade, 'building_id', building.building_id);
            const upgradeEnabled = realtimeReady(state) && owned && !upgradePending;
            return `<li data-building-id="${escapeHTML(building.building_id)}">
              <span>${escapeHTML(building.building_type)} L${building.level}</span>
              <button type="button" data-action="planet-building-upgrade" data-planet-id="${escapeHTML(planet.planet_id)}" data-building-id="${escapeHTML(building.building_id)}" data-target-level="${building.level + 1}" ${upgradeEnabled ? '' : 'disabled'} title="${escapeHTML(upgradePending ? 'Building upgrade pending' : 'Upgrade building')}">${upgradePending ? 'Upgrading' : 'Upgrade'}</button>
            </li>`;
          })
          .join('')}</ul>`
      : '<div class="empty-line">No buildings yet.</div>';
  return `
    <div class="building-controls" data-building-build-control="true" data-planet-id="${escapeHTML(planet.planet_id)}">
      ${buildingRows}
      <div class="route-create">
        <select data-building-build-type ${controlsReady ? '' : 'disabled'} aria-label="Building type">
          <option value="iron_extractor">iron_extractor</option>
          <option value="alloy_foundry">alloy_foundry</option>
        </select>
        <input type="text" value="${escapeHTML(slot)}" data-building-build-slot ${controlsReady ? '' : 'disabled'} aria-label="Building slot" />
        <button type="button" data-action="planet-building-build" data-planet-id="${escapeHTML(planet.planet_id)}" ${controlsReady ? '' : 'disabled'} title="${escapeHTML(buildTitle)}">${buildPending ? 'Building' : 'Build'}</button>
      </div>
    </div>
  `;
}

function nextBuildingSlot(production: NonNullable<ClientState['production']>['planets'][number]): string {
  const slots = ['alpha', 'beta', 'gamma', 'delta'];
  const used = new Set<string>();
  for (const building of production.buildings) {
    for (const slot of slots) {
      if (building.building_id.endsWith(`-${slot}`)) {
        used.add(slot);
      }
    }
  }
  return slots.find((slot) => !used.has(slot)) ?? slots[0];
}

function routeControlsPanel(
  state: ClientState,
  source: KnownPlanetSummary,
  production: NonNullable<ClientState['production']>['planets'][number] | null | undefined,
  routes: RouteSummary[],
): string {
  const ownedSource = isOwnedPlanet(source);
  const endpoints = ownedRouteEndpoints(state, source.planet_id);
  const resources = routeableStorageResources(production, routes);
  const createPending = hasPendingOpPayloadField(state, OPERATIONS.routeCreate, 'source_planet_id', source.planet_id);
  const controlsReady = realtimeReady(state) && ownedSource;
  const createEnabled = controlsReady && endpoints.length > 0 && resources.length > 0 && !createPending;
  const createTitle = !ownedSource
    ? 'Own this planet before creating routes'
    : !realtimeReady(state)
      ? 'Realtime connection required'
      : endpoints.length === 0
        ? 'No other owned known endpoint available'
        : resources.length === 0
          ? 'No server-owned storage resource available'
          : createPending
            ? 'Route create pending'
            : 'Create owned planet route';
  const selectedRoute = selectedRouteFor(routes);
  const routeRows =
    routes.length > 0
      ? routes
          .slice(0, 4)
          .map((route) => routeControlRow(state, route, endpoints, resources, route.route_id === selectedRoute?.route_id))
          .join('')
      : '<div class="empty-line">No routes for this planet.</div>';
  const reconcilePending = hasPendingRouteSettle(state, undefined);

  return `
    <div class="route-controls">
      <div class="route-create" data-route-create-control="true" data-route-source-planet-id="${escapeHTML(source.planet_id)}">
        <select data-route-create-destination ${createEnabled ? '' : 'disabled'} aria-label="Route destination">
          ${routeEndpointOptions(endpoints, endpoints[0]?.planet_id ?? '')}
        </select>
        <select data-route-create-resource ${createEnabled ? '' : 'disabled'} aria-label="Route resource">
          ${resourceOptions(resources, resources[0] ?? '')}
        </select>
        <input type="number" min="1" step="1" value="${defaultRouteRate(resources[0])}" data-route-rate ${createEnabled ? '' : 'disabled'} aria-label="Route amount per hour" />
        <button type="button" data-action="route-create" data-source-planet-id="${escapeHTML(source.planet_id)}" ${createEnabled ? '' : 'disabled'} title="${escapeHTML(createTitle)}">${createPending ? 'Creating' : 'Create'}</button>
      </div>
      <div class="route-list">
        ${routeRows}
      </div>
      <button type="button" data-action="route-settle" ${controlsReady && routes.length > 0 && !reconcilePending ? '' : 'disabled'} title="${escapeHTML(reconcilePending ? 'Route reconcile pending' : 'Reconcile all owned routes')}">${reconcilePending ? 'Reconciling' : 'Settle All'}</button>
    </div>
  `;
}

function routeControlRow(
  state: ClientState,
  route: RouteSummary,
  endpoints: KnownPlanetSummary[],
  resources: string[],
  selected: boolean,
): string {
  const routePending = hasPendingRouteMutation(state, route.route_id);
  const settlePending = hasPendingRouteSettle(state, route.route_id);
  const controlsReady = realtimeReady(state) && !routePending;
  const controlAction = route.enabled ? 'route-disable' : 'route-enable';
  const controlLabel = route.enabled ? 'Disable' : 'Enable';
  const endpointOptions = routeEndpointOptions(endpoints, route.destination.id);
  const mergedResources = resources.includes(route.resource_item_id) ? resources : [route.resource_item_id, ...resources];
  const resourceSelect = resourceOptions(mergedResources, route.resource_item_id);
  const updateEnabled = controlsReady && endpoints.length > 0 && mergedResources.length > 0;
  const settlementHTML = routeSettlementResult(route);
  return `
    <div class="route-row" data-route-update-control="true" data-route-id="${escapeHTML(route.route_id)}" data-selected="${selected ? 'true' : 'false'}">
      <button type="button" data-action="route-select" data-route-id="${escapeHTML(route.route_id)}" title="Select route">${escapeHTML(route.resource_item_id)} ${route.enabled ? 'on' : 'off'}</button>
      <select data-route-update-destination ${updateEnabled ? '' : 'disabled'} aria-label="Update route destination">${endpointOptions}</select>
      <select data-route-update-resource ${updateEnabled ? '' : 'disabled'} aria-label="Update route resource">${resourceSelect}</select>
      <input type="number" min="1" step="1" value="${Math.max(1, Math.round(route.amount_per_hour))}" data-route-rate ${updateEnabled ? '' : 'disabled'} aria-label="Update route amount per hour" />
      <button type="button" data-action="route-update" data-route-id="${escapeHTML(route.route_id)}" ${updateEnabled ? '' : 'disabled'} title="${escapeHTML(routePending ? 'Route mutation pending' : 'Update route terms')}">Update</button>
      <button type="button" data-action="${controlAction}" data-route-id="${escapeHTML(route.route_id)}" ${controlsReady ? '' : 'disabled'} title="${escapeHTML(routePending ? 'Route mutation pending' : `${controlLabel} route`)}">${controlLabel}</button>
      <button type="button" data-action="route-settle" data-route-id="${escapeHTML(route.route_id)}" ${realtimeReady(state) && !settlePending ? '' : 'disabled'} title="${escapeHTML(settlePending ? 'Route settlement pending' : 'Settle route')}">${settlePending ? 'Settling' : 'Settle'}</button>
      ${settlementHTML}
    </div>
  `;
}

function routeSettlementResult(route: RouteSummary): string {
  const settlement = route.last_settlement;
  if (!settlement) {
    return '';
  }
  const flags: string[] = [];
  if (settlement.no_op) {
    flags.push('No transfer');
  }
  if (settlement.source_empty) {
    flags.push('Source empty');
  }
  if (settlement.destination_full) {
    flags.push('Storage full');
  }
  if (settlement.loss_applied) {
    flags.push('Loss applied');
  }
  const flagText = flags.length > 0 ? flags.join(' / ') : 'Settled';
  const transferText = `${settlement.added_amount}/${settlement.wanted_amount} ${settlement.resource_item_id}`;
  return `
    <div class="route-settlement" data-route-settlement-result="true" data-route-id="${escapeHTML(route.route_id)}">
      <span>${escapeHTML(flagText)}</span>
      <strong>${escapeHTML(transferText)}</strong>
    </div>
  `;
}

function selectedRouteFor(routes: RouteSummary[]): RouteSummary | null {
  if (hudSelection.selectedRouteID) {
    return routes.find((route) => route.route_id === hudSelection.selectedRouteID) ?? routes[0] ?? null;
  }
  return routes[0] ?? null;
}

function ownedRouteEndpoints(state: ClientState, sourcePlanetID: string): KnownPlanetSummary[] {
  return state.planetIntel?.planets.filter((planet) => planet.planet_id !== sourcePlanetID && isOwnedPlanet(planet)) ?? [];
}

function routeableStorageResources(
  production: NonNullable<ClientState['production']>['planets'][number] | null | undefined,
  routes: RouteSummary[],
): string[] {
  const seen = new Set<string>();
  const resources: string[] = [];
  for (const item of production?.storage.items ?? []) {
    if (item.item_id && item.quantity > 0 && !seen.has(item.item_id)) {
      seen.add(item.item_id);
      resources.push(item.item_id);
    }
  }
  for (const route of routes) {
    if (route.resource_item_id && !seen.has(route.resource_item_id)) {
      seen.add(route.resource_item_id);
      resources.push(route.resource_item_id);
    }
  }
  return resources;
}

function routeEndpointOptions(planets: KnownPlanetSummary[], selectedPlanetID: string): string {
  if (planets.length === 0) {
    return '<option value="">No endpoint</option>';
  }
  return planets
    .map(
      (planet) =>
        `<option value="${escapeHTML(planet.planet_id)}" ${planet.planet_id === selectedPlanetID ? 'selected' : ''}>${escapeHTML(publicPlanetName(planet))}</option>`,
    )
    .join('');
}

function resourceOptions(resources: string[], selectedResource: string): string {
  if (resources.length === 0) {
    return '<option value="">No resource</option>';
  }
  return resources
    .map((resource) => `<option value="${escapeHTML(resource)}" ${resource === selectedResource ? 'selected' : ''}>${escapeHTML(resource)}</option>`)
    .join('');
}

function defaultRouteRate(resource?: string): number {
  return resource ? 40 : 1;
}

function hasPendingRouteMutation(state: ClientState, routeID: string): boolean {
  return (
    hasPendingOpPayloadField(state, OPERATIONS.routeUpdate, 'route_id', routeID) ||
    hasPendingOpPayloadField(state, OPERATIONS.routeEnable, 'route_id', routeID) ||
    hasPendingOpPayloadField(state, OPERATIONS.routeDisable, 'route_id', routeID)
  );
}

function hasPendingRouteSettle(state: ClientState, routeID: string | undefined): boolean {
  return Object.values(state.pendingCommands).some((command) => {
    if (command.op !== OPERATIONS.routeSettle) {
      return false;
    }
    if (!routeID) {
      return !command.payload || typeof command.payload.route_id !== 'string';
    }
    return command.payload?.route_id === routeID;
  });
}

function isOwnedPlanet(planet: KnownPlanetSummary): boolean {
  const ownerStatus = (planet.owner_status ?? '').toLowerCase();
  return ownerStatus === 'owned_by_you' || ownerStatus === 'owned' || ownerStatus.startsWith('owned_');
}

import { OPERATIONS } from '../protocol/envelope';
import type { ClientState } from '../state/types';
import { escapeHTML, formatVec, hasPendingOpPayloadField, lockedValue, publicPlanetName, realtimeReady } from './hud-formatters';

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
                          <div class="meta-row"><span>Buildings</span><strong>${production.buildings.length}</strong></div>`
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
                   ${
                     routes.length > 0
                       ? `<ul class="compact-list">${routes
                           .slice(0, 4)
                           .map((route) => `<li><span>${escapeHTML(route.resource_item_id)}</span><strong>${route.enabled ? 'on' : 'off'}</strong></li>`)
                           .join('')}</ul>`
                       : '<div class="empty-line">No routes.</div>'
                   }
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
  const routes = detail?.routes ?? [];
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
             </div>`
          : `<div class="empty-line">${detail?.production_locked ? 'Production locked.' : 'No production snapshot for this planet yet.'}</div>`
      }
      <div class="systems-subhead">Routes</div>
      ${
        routes.length > 0
          ? `<ul class="compact-list">
               ${routes
                 .slice(0, 4)
                 .map((route) => `<li><span>${escapeHTML(route.resource_item_id)}</span><strong>${route.enabled ? 'on' : 'off'}</strong></li>`)
                 .join('')}
             </ul>`
          : '<div class="empty-line">No routes for this planet.</div>'
      }
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

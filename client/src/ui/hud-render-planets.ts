import type { ClientState, MinimapContact } from '../state/types';
import {
  isClickableMinimapMemory,
  minimapPointPercent,
  rememberedIntelState,
  rememberedMinimapDetailID,
  shouldRenderRememberedMinimapMemory,
} from '../state/world-memory';
import { dispositionForType, escapeHTML, formatCompactNumber, formatVec, lockedValue, publicEntityType, publicPlanetName } from './hud-formatters';

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

export function minimapPanel(state: ClientState): string {
  if (!state.minimap || (state.minimap.live_contacts.length === 0 && state.minimap.remembered.length === 0)) {
    return '<div class="minimap minimap--empty"><div class="empty-line">Awaiting map projection.</div></div>';
  }

  const contacts = state.minimap.live_contacts;
  const memories = state.minimap.remembered;
  const self = contacts.find((contact) => contact.status_flags?.includes('self')) ?? contacts.find((contact) => contact.entity_type === 'player');
  const center = self?.position ?? { x: 0, y: 0 };
  const radius = Math.max(state.minimap.radar_range, 1);
  const projectionHalfExtent = Math.max((state.minimap.projection_window_size ?? radius * 2) / 2, 1);
  const points = contacts
    .map((contact) => {
      const point = minimapPointPercent(center, contact.position, radius);
      if (!point) {
        return '';
      }
      const disposition = contact.status_flags?.includes('self') ? 'self' : contact.disposition || dispositionForType(contact.entity_type);
      const action = minimapLiveContactAction(contact);
      const actionAttr = action ? ` data-action="${action}"` : '';
      const disabledAttr = action ? '' : ' disabled';
      const source = contact.projection_source ? ` data-projection-source="${escapeHTML(contact.projection_source)}"` : '';
      return `<button class="minimap__point" type="button"${actionAttr}${disabledAttr} data-target-source="radar" data-kind="${escapeHTML(disposition)}" data-entity-id="${escapeHTML(contact.entity_id)}" data-entity-type="${escapeHTML(contact.entity_type)}"${source} style="left:${point.left}%;top:${point.top}%" title="${escapeHTML(publicEntityType(contact.entity_type))}"></button>`;
    })
    .join('');
  const memoryPoints = memories
    .filter((memory) => shouldRenderRememberedMinimapMemory(state, memory, center, projectionHalfExtent))
    .map((memory) => {
      const point = minimapPointPercent(center, memory.position, radius);
      if (!point) {
        return '';
      }
      const planetID = rememberedMinimapDetailID(state, memory) ?? '';
      const clickable = isClickableMinimapMemory(memory);
      const action = clickable ? ' data-action="planet-detail"' : '';
      const planet = planetID ? ` data-planet-id="${escapeHTML(planetID)}"` : '';
      const disabled = clickable ? '' : ' disabled';
      const intelState = rememberedIntelState(memory);
      const sector = memory.sector_key ? ` data-sector-key="${escapeHTML(memory.sector_key)}"` : '';
      const source = memory.projection_source ? ` data-projection-source="${escapeHTML(memory.projection_source)}"` : '';
      return `<button class="minimap__memory" type="button"${action}${planet}${disabled} data-kind="${escapeHTML(memory.kind)}" data-freshness="${escapeHTML(intelState)}"${sector}${source} style="left:${point.left}%;top:${point.top}%" title="${escapeHTML(memory.label || memory.kind)}"></button>`;
    })
    .join('');

  return `
    <div class="minimap" aria-label="Sector map">
      <span class="minimap__ring minimap__ring--outer"></span>
      <span class="minimap__ring minimap__ring--middle"></span>
      <span class="minimap__axis minimap__axis--x"></span>
      <span class="minimap__axis minimap__axis--y"></span>
      ${memoryPoints}
      ${points}
    </div>
    <div class="minimap-legend">
      <span data-kind="self">You</span>
      <span data-kind="hostile">Hostile</span>
      <span data-kind="loot">Loot</span>
      <span data-kind="memory">Memory</span>
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

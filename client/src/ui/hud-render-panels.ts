import type { ClientState } from '../state/types';
import { activeEntityMovement, currentEntityPosition, distanceBetween, selfEntity } from '../state/movement';
import { isAttackableVisibleTarget } from '../state/target-eligibility';
import { galaxyIconURL, gatherIconURL, hangarIconURL, inventoryIconURL, laserIconURL, menuIconURL, planetsIconURL, rocketIconURL, scanIconURL, shieldIconURL, shopIconURL, warpIconURL } from './hud-icons';
import { hudSelection } from './hud-selection';
import { economyPanel } from './hud-render-economy';
import { cargoPanel } from './hud-render-inventory';
import { planetCatalogPanel, planetDetailModal, planetModalTitle, planetsPanel, minimapPanel } from './hud-render-planets';
import { questsPanel } from './hud-render-quests';
import { adminContentBlock, adminContentEditModal } from './hud-render-admin-content';
import type { ActionState, EntityCombatStatus, HUDHelpTopicID, HUDModalID, HUDModalState, HUDPanelDefinition, HUDWindowID, KnownLootDropStatus, QuickActionCommand, QuickActionID, QuickActionState, VisibleEntity } from './hud-types';
import { actionScanLabel, clamp, escapeHTML, formatCooldown, formatDuration, formatPair, formatVec, hasPendingOp, isHelpTopicID, lockedValue, publicEntityType, publicPlanetName, realtimeReady, scanModeTimeDetail, scanStatusLabel } from './hud-formatters';

export const baseWindowDefinitions: HUDPanelDefinition[] = [
  { id: 'cargo', label: 'Inv', title: 'Inventory', iconURL: inventoryIconURL, helpTopic: 'inventory', render: cargoPanel },
  { id: 'economy', label: 'Shop', title: 'Shop', iconURL: shopIconURL, helpTopic: 'shop', render: economyPanel },
  { id: 'quests', label: 'Quests', title: 'Quest Board', iconURL: galaxyIconURL, helpTopic: 'quests', render: questsPanel },
  { id: 'intel', label: 'Planets', title: 'Planets', iconURL: planetsIconURL, helpTopic: 'planets', render: planetCatalogPanel },
  { id: 'systems', label: 'Hangar', title: 'Hangar', iconURL: hangarIconURL, helpTopic: 'hangar', render: hangarPanel },
  { id: 'ops', label: 'Ops', title: 'Admin Ops', iconURL: menuIconURL, helpTopic: 'ops', render: opsPanel, hidden: (state) => !state.auth.session?.account?.admin },
];

export function windowLayout(id: HUDWindowID): { width: number; preferredHeight: number; size: 'compact' | 'dual-pane' | 'triple-pane' | 'system' } {
  switch (id) {
    case 'economy':
      return { width: 640, preferredHeight: 540, size: 'triple-pane' };
    case 'quests':
      return { width: 620, preferredHeight: 760, size: 'dual-pane' };
    case 'intel':
      return { width: 620, preferredHeight: 710, size: 'dual-pane' };
    case 'systems':
      return { width: 540, preferredHeight: 470, size: 'system' };
    case 'ops':
      return { width: 640, preferredHeight: 700, size: 'dual-pane' };
    case 'cargo':
    default:
      return { width: 760, preferredHeight: 610, size: 'triple-pane' };
  }
}

export function isControlElement(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && Boolean(target.closest('button, input, select, textarea, a[href], [data-action]'));
}

export function isQuickActionKey(key: string): boolean {
  return key === '1' || key === '2' || key === '3' || key === '4' || key === '5' || key === '6';
}

export function parseLoadoutDragPayload(raw: string): { itemInstanceID: string; slotID?: string; moduleSlotType?: string } | null {
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as { itemInstanceID?: unknown; slotID?: unknown; moduleSlotType?: unknown };
    if (typeof parsed.itemInstanceID !== 'string' || parsed.itemInstanceID === '') {
      return null;
    }
    return {
      itemInstanceID: parsed.itemInstanceID,
      slotID: typeof parsed.slotID === 'string' && parsed.slotID !== '' ? parsed.slotID : undefined,
      moduleSlotType: typeof parsed.moduleSlotType === 'string' && parsed.moduleSlotType !== '' ? parsed.moduleSlotType : undefined,
    };
  } catch {
    return null;
  }
}

export function windowDefinitions(state: ClientState): HUDPanelDefinition[] {
  return baseWindowDefinitions.filter((definition) => !definition.hidden?.(state));
}

export function modalDefinition(id: HUDModalID, state: ClientState, detailID?: string): HUDModalState | null {
  switch (id) {
    case 'tutorial':
      return tutorialModalDefinition(detailID);
    case 'target':
      return { id, title: 'Target Detail', body: targetPanel(state) };
    case 'planet-detail': {
      const title = planetModalTitle(state, detailID);
      return { id, detailID, title, body: planetDetailModal(state, detailID) };
    }
    case 'admin-content-module-edit':
      return { id, detailID, title: 'CMS Draft', body: adminContentEditModal(state, detailID) };
    case 'planets':
      return { id, title: 'Planet Intel', body: planetsPanel(state) };
    case 'ship':
      return { id, title: 'Ship Detail', body: shipPanel(state) };
    default:
      return null;
  }
}

export function tutorialModalDefinition(topicID: string | undefined): HUDModalState | null {
  if (!isHelpTopicID(topicID)) {
    return null;
  }
  const topic = helpTopicCatalog[topicID];
  return {
    id: 'tutorial',
    detailID: topicID,
    helpTopic: topicID,
    title: topic.title,
    body: tutorialHelpBody(topicID, topic),
  };
}

const helpTopicCatalog: Record<HUDHelpTopicID, { title: string; lead: string; points: string[] }> = {
  inventory: {
    title: 'Inventory Help',
    lead: 'Equipment, inventory, cargo, and crafting stay separated so ship fitting does not mix with carried resources.',
    points: [
      'Equipment shows the active ship layout and usable module slots.',
      'Inventory lists account-stored modules and stackable supplies.',
      'Cargo is the current ship hold. Transfer actions appear only when available.',
      'Crafting stays locked until a recipe action is available for the current location.',
    ],
  },
  shop: {
    title: 'Shop Help',
    lead: 'Shop, sell orders, auctions, and premium grants share one economy window but keep separate lanes.',
    points: [
      'Category buttons switch between listings, cargo sale, auction lots, and grants.',
      'Product detail updates from the selected row.',
      'Buy, sell, bid, and claim buttons appear only for actions that can be sent now.',
    ],
  },
  quests: {
    title: 'Quest Board Help',
    lead: 'Quest Board groups offers, active jobs, claimable rewards, and completed records.',
    points: [
      'Select a row to inspect objectives and rewards.',
      'Accept, reroll, and claim actions use the board state shown in the row.',
      'Expired or unavailable jobs stay quiet until the board refreshes.',
    ],
  },
  planets: {
    title: 'Planet Intel Help',
    lead: 'Planet Intel is a catalog of discovered worlds and remembered signals.',
    points: [
      'Known planets open a detail modal with the last visible coordinates.',
      'Navigate uses the coordinates returned with planet detail.',
      'Claim, production, storage, and route actions appear only when that planet exposes them.',
    ],
  },
  hangar: {
    title: 'Hangar Help',
    lead: 'Hangar manages owned hulls and the active ship loadout.',
    points: [
      'Owned ships show selection and activation state.',
      'Module slots are grouped by the active ship layout.',
      'Equip and unequip actions reconcile from inventory and loadout snapshots.',
    ],
  },
  ops: {
    title: 'Ops Help',
    lead: 'Admin Ops is a role-gated diagnostic surface for local playtest control.',
    points: [
      'Refresh pulls the latest admin summary.',
      'Repair controls are visible only for admin sessions.',
      'Normal player sessions do not show this window.',
    ],
  },
};

export function tutorialHelpBody(topicID: HUDHelpTopicID, topic: { lead: string; points: string[] }): string {
  return `
    <section class="tutorial-help" data-help-body="true" data-help-topic="${escapeHTML(topicID)}">
      <p>${escapeHTML(topic.lead)}</p>
      <ul>
        ${topic.points.map((point) => `<li>${escapeHTML(point)}</li>`).join('')}
      </ul>
    </section>
  `;
}

export function statusPanel(state: ClientState): string {
  const snapshot = state.playerSnapshot;
  const ship = state.ship;
  const stats = state.stats;
  const progression = state.progression;
  const shipLabel = ship?.display_name || snapshot?.callsign || state.auth.session?.player?.callsign || 'Awaiting Ship';
  return `
    <h2><span>Ship</span> ${escapeHTML(String(shipLabel))}</h2>
    <div class="ship-status-card">
      <div class="ship-silhouette" aria-hidden="true"></div>
      <div class="status-grid">
        ${meter('Hull', ship?.hull ?? snapshot?.hp, ship?.max_hull ?? snapshot?.max_hp)}
        ${meter('Shield', ship?.shield ?? snapshot?.shield, ship?.max_shield ?? snapshot?.max_shield)}
        ${meter('Cap', ship?.capacitor ?? snapshot?.energy, ship?.max_capacitor ?? snapshot?.max_energy)}
      </div>
    </div>
    <div class="meta-row"><span>Rank</span><strong>${snapshot?.rank ?? lockedValue()}</strong></div>
    <div class="meta-row"><span>Level</span><strong>${progression?.main_level ?? lockedValue()}</strong></div>
    <div class="meta-row"><span>Speed</span><strong>${stats ? Math.round(stats.speed) : lockedValue()}</strong></div>
    <div class="meta-row"><span>Radar</span><strong>${stats ? Math.round(stats.radar_range) : lockedValue()}</strong></div>
    <div class="meta-row"><span>Link</span><strong>${escapeHTML(state.connectionStatus)}</strong></div>
  `;
}

export function systemsPanel(state: ClientState): string {
  const inventory = state.inventory;
  const hangar = state.hangar;
  const loadout = state.loadout;
  const crafting = state.crafting;
  const loaded = Boolean(inventory && hangar && loadout && crafting);
  const topItems = inventory?.stackable.slice(0, 3) ?? [];
  const activeShip =
    hangar?.ships.find((ship) => ship.ship_id === hangar.active_ship_id) ?? hangar?.ships[0] ?? null;
  const equipped = loadout?.slots.filter((slot) => slot.module_item_id).length ?? 0;
  const recipe = crafting?.recipes[0] ?? null;
  const recipeLabel = recipe ? recipe.output.item_id ?? recipe.output.ship_id ?? recipe.recipe_id : null;

  return `
    <h2>Systems</h2>
    ${
      loaded
        ? `<div class="systems-block">
             <div class="meta-row"><span>Hangar</span><strong>${escapeHTML(activeShip?.display_name ?? hangar?.active_ship_id ?? lockedValue())}</strong></div>
             <div class="meta-row"><span>Loadout</span><strong>${equipped}/${loadout?.slots.length ?? 0}</strong></div>
             <div class="meta-row"><span>Storage</span><strong>${inventory?.counts.storage_stacks ?? 0}</strong></div>
             <div class="meta-row"><span>Recipes</span><strong>${crafting?.recipes.length ?? 0}</strong></div>
           </div>
           ${
             topItems.length > 0
               ? `<ul class="compact-list systems-list">
                    ${topItems.map((item) => `<li><span>${escapeHTML(item.display_name || item.item_id)}</span><strong>${item.quantity}</strong></li>`).join('')}
                  </ul>`
               : '<div class="empty-line">Inventory empty.</div>'
           }
           <div class="meta-row"><span>Next</span><strong>${recipeLabel ? escapeHTML(recipeLabel) : lockedValue()}</strong></div>`
        : '<div class="empty-line">Awaiting systems data.</div>'
    }
  `;
}

export function hangarPanel(state: ClientState): string {
  const hangar = state.hangar;
  const ship = state.ship;
  const stats = state.stats;
  const cargo = state.cargo;
  const loadout = state.loadout;
  if (!hangar || !ship || !stats || !cargo || !loadout) {
    return `
      <h2>Hangar</h2>
      <div class="empty-line">Awaiting hangar, ship, cargo, stats, and loadout data.</div>
    `;
  }
  const activeShip =
    hangar.ships.find((item) => item.ship_id === hangar.active_ship_id) ??
    hangar.ships.find((item) => item.active) ??
    hangar.ships[0] ??
    null;
  const selectedShip = hangar.ships.find((item) => item.ship_id === hudSelection.selectedHangarShipID) ?? activeShip;
  hudSelection.selectedHangarShipID = selectedShip?.ship_id ?? null;
  const selectedSlots = `${selectedShip?.slot_offensive ?? 0}/${selectedShip?.slot_defensive ?? 0}/${selectedShip?.slot_utility ?? 0}`;
  const selectedActive = Boolean(selectedShip && (selectedShip.active || selectedShip.ship_id === hangar.active_ship_id));
  const canActivate = Boolean(selectedShip && !selectedActive && !selectedShip.disabled && !selectedShip.locked_reason);
  return `
    <h2>Hangar</h2>
    <section class="hangar-console">
      <div class="hangar-list" aria-label="Owned ships">
        <div class="hangar-list__head">
          <strong>Owned Ships</strong>
          <span>${hangar.ships.length}</span>
        </div>
        ${
          hangar.ships.length > 0
            ? hangar.ships.map((item) => hangarShipRow(item, hangar.active_ship_id, selectedShip?.ship_id ?? '')).join('')
            : '<div class="empty-line">No owned ships.</div>'
        }
      </div>
      <div class="hangar-detail">
        ${
          selectedShip
            ? `<div class="hangar-preview">
                 <div class="hangar-preview__ship" aria-hidden="true"></div>
                 <div>
                   <span>${escapeHTML(selectedShip.role ?? 'ship')}</span>
                   <strong>${escapeHTML(selectedShip.display_name || selectedShip.ship_id)}</strong>
                   <em>${escapeHTML(selectedActive ? 'active' : selectedShip.state || 'available')}</em>
                 </div>
               </div>
               <div class="hangar-stat-grid">
                 ${hangarStat('Hull', selectedShip.hull, selectedShip.max_hull)}
                 ${hangarStat('Shield', selectedShip.shield, selectedShip.max_shield)}
                 ${hangarStat('Cap', selectedShip.capacitor ?? ship.capacitor, selectedShip.max_capacitor ?? ship.max_capacitor)}
                 ${hangarScalar('Speed', selectedShip.speed ?? stats.speed)}
                 ${hangarScalar('Radar', selectedShip.radar ?? stats.radar_range)}
                 ${hangarScalar('Cargo', selectedShip.cargo_capacity ?? cargo.capacity)}
               </div>
               <div class="hangar-slots">
                 <span>OFF <strong>${selectedShip.slot_offensive ?? 0}</strong></span>
                 <span>DEF <strong>${selectedShip.slot_defensive ?? 0}</strong></span>
                 <span>UTL <strong>${selectedShip.slot_utility ?? 0}</strong></span>
               </div>
               <div class="hangar-actions">
                 ${hangarPrimaryAction(selectedShip, selectedActive, canActivate)}
                 <button type="button" data-action="open-window" data-panel-id="cargo">Loadout</button>
               </div>
               <div class="hangar-foot">
                 <span>Slots ${escapeHTML(selectedSlots)}</span>
                 <span>Cargo ${cargo.used}/${cargo.capacity}</span>
               </div>`
            : '<div class="empty-line">No selected ship in hangar.</div>'
        }
      </div>
    </section>
  `;
}

export function hangarPrimaryAction(
  ship: NonNullable<ClientState['hangar']>['ships'][number],
  selectedActive: boolean,
  canActivate: boolean,
): string {
  if (selectedActive) {
    return '<span class="hangar-status-badge" data-hangar-active-badge="true">Active</span>';
  }
  if (!canActivate) {
    return `<span class="hangar-status-badge" data-hangar-locked-badge="true">${escapeHTML(ship.locked_reason || ship.state || 'Unavailable')}</span>`;
  }
  return `
    <button type="button"
      data-action="hangar-activate"
      data-ship-id="${escapeHTML(ship.ship_id)}"
      title="Activate this ship">Activate</button>
  `;
}

export function hangarShipRow(ship: NonNullable<ClientState['hangar']>['ships'][number], activeShipID: string, selectedShipID: string): string {
  const active = ship.active || ship.ship_id === activeShipID;
  const selected = ship.ship_id === selectedShipID;
  return `
    <button class="hangar-row" type="button" data-action="hangar-select" data-ship-id="${escapeHTML(ship.ship_id)}" data-active="${active ? 'true' : 'false'}" data-selected="${selected ? 'true' : 'false'}">
      <span class="hangar-row__mark"></span>
      <span>
        <strong>${escapeHTML(ship.display_name || ship.ship_id)}</strong>
        <em>${escapeHTML(active ? 'active' : ship.locked_reason || ship.state || 'available')}</em>
      </span>
      <small>${escapeHTML(ship.role ?? 'ship')}</small>
    </button>
  `;
}

export function hangarStat(label: string, value: number | undefined, max: number | undefined): string {
  const safeValue = Math.max(0, Math.round(value ?? 0));
  const safeMax = Math.max(0, Math.round(max ?? 0));
  return `<div class="hangar-stat"><span>${escapeHTML(label)}</span><strong>${safeValue}/${safeMax}</strong></div>`;
}

export function hangarScalar(label: string, value: number | undefined): string {
  return `<div class="hangar-stat"><span>${escapeHTML(label)}</span><strong>${Math.max(0, Math.round(value ?? 0))}</strong></div>`;
}

export function opsPanel(state: ClientState): string {
  if (!state.auth.session?.account?.admin) {
    return `
      <h2>Ops</h2>
      <div class="empty-line">Admin session required.</div>
    `;
  }
  return `
    <h2>Ops</h2>
    ${adminOpsBlock(state)}
    ${adminContentBlock(state)}
  `;
}

export function adminOpsBlock(state: ClientState): string {
  if (!state.auth.session?.account?.admin) {
    return '';
  }

  const dashboard = state.economyDashboard;
  const inspection = state.adminInspection;
  const commandLog = state.commandLogSummary;
  const metrics = state.metrics;
  const releaseGate = state.releaseGate;
  const abuse = state.abuseCoverage;
  const repairJob = state.crafting?.active_jobs.find((job) => job.state !== 'completed' && job.state !== 'complete') ?? null;
  const metricSeries =
    metrics ? metrics.snapshot.counters.length + metrics.snapshot.gauges.length + metrics.snapshot.durations.length : null;

  return `
    <div class="systems-subhead">Ops</div>
    <div class="systems-block">
      <div class="meta-row"><span>Credits</span><strong>${dashboard ? dashboard.wallets.credits : lockedValue()}</strong></div>
      <div class="meta-row"><span>Logs</span><strong>${commandLog ? commandLog.total : lockedValue()}</strong></div>
      <div class="meta-row"><span>Series</span><strong>${metricSeries ?? lockedValue()}</strong></div>
      <div class="meta-row"><span>Gate</span><strong>${releaseGate ? (releaseGate.report.passed ? 'pass' : `${releaseGate.report.missing.length} gap`) : lockedValue()}</strong></div>
      <div class="meta-row"><span>Abuse</span><strong>${abuse ? (abuse.report.passed ? 'pass' : `${abuse.report.missing.length} gap`) : lockedValue()}</strong></div>
      <div class="meta-row"><span>Inspect</span><strong>${inspection ? `${inspection.inventory.stackable_items}/${inspection.wallet.balances.length}` : lockedValue()}</strong></div>
    </div>
    <div class="segmented segmented--ops">
      <button type="button" data-action="admin-refresh">Ops</button>
      <button type="button" data-action="admin-repair-craft-job" data-job-id="${escapeHTML(repairJob?.job_id ?? '')}" ${repairJob ? '' : 'disabled'}>Repair</button>
    </div>
    ${
      state.adminRepair
        ? `<div class="empty-line">${escapeHTML(state.adminRepair.status || state.adminRepair.message || 'Admin action recorded.')}</div>`
        : ''
    }
  `;
}

export function targetPanel(state: ClientState, serverNow: number | null = Date.now()): string {
  const target = state.selectedTargetID ? state.visibleEntities[state.selectedTargetID] : null;
  const actions = quickActionMap(state, serverNow);
  const laser = actions.laser;
  const loot = actions.gather;
  const targetLabel = target?.display?.label ?? target?.entity_id ?? '';
  const distance = target ? distanceToTarget(state, target.entity_id, serverNow) : null;
  const knownLoot = target ? state.knownLoot[target.entity_id] : null;
  const targetActions = targetActionButtons(target, laser, loot, selfEntity(state.visibleEntities));
  return `
    <h2>Target</h2>
    ${
      target
        ? `<div class="target-lock" data-target-kind="${escapeHTML(target.entity_type)}">
             <span class="target-lock__mark"></span>
             <div>
               <div class="target-name">${escapeHTML(targetLabel)}</div>
               <div class="target-kind">${escapeHTML(publicEntityType(target.entity_type))}</div>
             </div>
           </div>
           <div class="meta-row"><span>Type</span><strong>${escapeHTML(publicEntityType(target.entity_type))}</strong></div>
           <div class="meta-row"><span>State</span><strong>${escapeHTML(target.display?.disposition ?? '--')}</strong></div>
           <div class="meta-row"><span>Range</span><strong>${distance === null ? lockedValue() : `${Math.round(distance)}u`}</strong></div>
           <div class="meta-row"><span>X/Y</span><strong>${Math.round(target.position.x)} / ${Math.round(target.position.y)}</strong></div>
           ${target.combat ? combatStatusBlock(target.combat) : ''}
           ${knownLoot ? lootStatusBlock(knownLoot) : ''}`
        : '<div class="empty-line target-empty" data-target-empty="true">Select a contact.</div>'
    }
    ${targetActions}
  `;
}

export function targetActionButtons(target: VisibleEntity | null, laser: QuickActionState, loot: QuickActionState, self: VisibleEntity | null = null): string {
  const buttons: string[] = [];
  if (isAttackableVisibleTarget(target, self)) {
    buttons.push(
      `<button type="button" data-action="fire" ${laser.enabled ? '' : 'disabled'} title="${escapeHTML(laser.title)}">Fire</button>`,
    );
  }
  if (target?.entity_type === 'loot') {
    buttons.push(
      `<button type="button" data-action="loot" ${loot.enabled ? '' : 'disabled'} title="${escapeHTML(loot.title)}">${escapeHTML(loot.label)}</button>`,
    );
  }
  if (buttons.length === 0) {
    return '';
  }
  return `<div class="segmented target-actions">${buttons.join('')}</div>`;
}

export function shipPanel(state: ClientState): string {
  if (!state.ship) {
    return `
      <h2>Ship</h2>
      <div class="empty-line">Awaiting ship data.</div>
    `;
  }

  const ship = state.ship;
  const quote = state.repairQuote && state.repairQuote.ship_id === ship.active_ship_id ? state.repairQuote : null;
  const repairDisabled = !ship.disabled || !quote;
  const repairState = ship.repair_state || (ship.disabled ? 'disabled' : lockedValue());
  return `
    <h2>Ship</h2>
    <div class="target-name">${escapeHTML(ship.display_name || ship.active_ship_id)}</div>
    ${meter('Hull', ship.hull, ship.max_hull)}
    ${meter('SHD', ship.shield, ship.max_shield)}
    ${meter('Cap', ship.capacitor, ship.max_capacitor)}
    <div class="meta-row"><span>State</span><strong>${escapeHTML(repairState)}</strong></div>
    ${
      ship.disabled
        ? `<div class="repair-box">
             <div class="meta-row"><span>Quote</span><strong>${quote ? `${quote.cost} ${escapeHTML(quote.currency)}` : lockedValue()}</strong></div>
             <div class="segmented">
               <button type="button" data-action="repair-quote">Quote</button>
               <button type="button" data-action="repair" ${repairDisabled ? 'disabled' : ''}>Repair</button>
             </div>
           </div>`
        : ''
    }
  `;
}

export function actionBar(state: ClientState, serverNow: number | null): string {
  return quickActionStates(state, serverNow).map(actionSlotHTML).join('');
}

export function movementEtaPanel(state: ClientState, serverNow: number | null): string {
  if (serverNow === null) {
    return '';
  }
  const self = selfEntity(state.visibleEntities);
  if (!self) {
    return '';
  }
  const timing = activeEntityMovement(self, serverNow);
  if (!timing) {
    return '';
  }
  const progress = Math.round(timing.progress * 1000) / 1000;
  return `
    <div class="movement-eta" data-movement-eta-active="true" style="--movement-progress: ${progress}">
      <div class="movement-eta__header">
        <span>Arrival</span>
        <strong>${formatDuration(timing.remainingMs)}</strong>
      </div>
      <div class="movement-eta__route">
        <span>${formatVec(timing.origin)}</span>
        <span>${formatVec(timing.target)}</span>
      </div>
      <div class="movement-eta__bar" aria-hidden="true"><span></span></div>
    </div>
  `;
}

export function intelPanel(state: ClientState, serverNow: number | null = Date.now()): string {
  const intel = state.planetIntel;
  const lastScan = intel?.lastScan ?? null;
  const knownPlanets = intel?.planets.slice(0, 2) ?? [];
  const routes = state.routes?.routes.length ?? null;
  const production = state.production?.planets.length ?? null;
  const scanAction = scanActionState(state);
  return `
    <h2>Sector Map</h2>
    ${minimapPanel(state, serverNow)}
    <div class="intel-metrics">
      <span>Known<strong>${intel ? intel.knownSignals : lockedValue()}</strong></span>
      <span>Stale<strong>${intel?.staleIntel ?? lockedValue()}</strong></span>
      <span>Owned<strong>${intel?.ownedPlanets ?? lockedValue()}</strong></span>
      <span>Routes<strong>${routes ?? lockedValue()}</strong></span>
      <span>Prod<strong>${production ?? lockedValue()}</strong></span>
    </div>
    <button class="ghost-action ghost-action--scan" type="button" data-action="scan" data-state="${escapeHTML(
      actionStateKind(scanAction),
    )}" ${scanAction.enabled ? '' : 'disabled'} title="${escapeHTML(scanAction.title)}">${escapeHTML(scanAction.label)}</button>
    ${
      lastScan
        ? `<div class="scan-readout">
             <span>${escapeHTML(scanStatusLabel(lastScan.status))}</span>
             <strong>${escapeHTML(lastScan.signal?.signal_band ?? lastScan.message ?? '--')}</strong>
           </div>`
        : '<div class="empty-line">No scanner pulse recorded.</div>'
    }
    ${
      knownPlanets.length > 0
        ? `<ul class="compact-list planet-list">
             ${knownPlanets
               .map(
                 (planet) =>
                   `<li>
                     <button class="inline-row-action" type="button" data-action="planet-detail" data-planet-id="${escapeHTML(planet.planet_id)}" title="Open planet detail">
                       <span>${escapeHTML(publicPlanetName(planet))}</span><strong>${escapeHTML(planet.owner_status || 'intel')}</strong>
                     </button>
                   </li>`,
               )
               .join('')}
           </ul>`
        : ''
    }
  `;
}

export function logPanel(state: ClientState): string {
  const lines = [...state.combatLog, ...state.commandLog].slice(-7).reverse();
  return `
    <h2>Log</h2>
    <ol class="log-lines">
      ${lines.map((line) => `<li data-level="${line.level}">${escapeHTML(line.text)}</li>`).join('')}
    </ol>
  `;
}

export function meter(label: string, current?: number, max?: number): string {
  const hasValue = current !== undefined && max !== undefined;
  const safeMax = Math.max(max ?? 0, 1);
  const safeCurrent = hasValue ? Math.max(0, Math.min(current, safeMax)) : 0;
  const percent = hasValue ? Math.round((safeCurrent / safeMax) * 100) : 0;
  return `
    <div class="stat-meter">
      <div class="stat-meter__label">${label}</div>
      <div class="meter"><span style="width:${percent}%"></span></div>
      <strong>${hasValue ? safeCurrent : lockedValue()}</strong>
    </div>
  `;
}

export function combatStatusBlock(combat: EntityCombatStatus): string {
  return `
    <div class="combat-status">
      ${meter('Hull', combat.hp, combat.max_hp)}
      ${meter('SHD', combat.shield, combat.max_shield)}
      <div class="meta-row"><span>Combat</span><strong>${escapeHTML(combat.status ?? 'active')}</strong></div>
    </div>
  `;
}

export function lootStatusBlock(drop: KnownLootDropStatus): string {
  return `
    <div class="loot-status">
      <div class="meta-row"><span>Item</span><strong>${escapeHTML(drop.item_id)}</strong></div>
      <div class="meta-row"><span>Qty</span><strong>${drop.quantity}</strong></div>
      <div class="meta-row"><span>Drop</span><strong>${escapeHTML(drop.state ?? 'visible')}</strong></div>
    </div>
  `;
}

export function quickActionMap(state: ClientState, serverNow: number | null): Record<QuickActionID, QuickActionState> {
  return Object.fromEntries(quickActionStates(state, serverNow).map((action) => [action.id, action])) as Record<QuickActionID, QuickActionState>;
}

export function quickActionStates(state: ClientState, serverNow: number | null): QuickActionState[] {
  const target = state.selectedTargetID ? state.visibleEntities[state.selectedTargetID] : null;
  const loot = lootActionState(state, target, serverNow);
  return [
    liveQuickAction('laser', 'fire', 1, '1', laserIconURL, 'combat.use_skill', laserActionState(state, target, serverNow)),
    lockedQuickAction('rocket', 'rocket', 2, '2', rocketIconURL, 'Rocket', 'Missile systems are not installed yet.'),
    liveQuickAction('scan', 'scan', 3, '3', scanIconURL, 'scan.pulse', scanActionState(state)),
    liveQuickAction('stealth', 'stealth', 4, '4', shieldIconURL, 'stealth.toggle', stealthActionState(state)),
    lockedQuickAction('warp', 'warp', 5, '5', warpIconURL, 'Warp', 'Warp drive is not installed yet.'),
    liveQuickAction('gather', 'loot', 6, '6', gatherIconURL, lootCommandOp(loot), loot),
  ];
}

export function liveQuickAction(
  id: QuickActionID,
  action: QuickActionCommand,
  slot: QuickActionState['slot'],
  key: string,
  iconURL: string,
  commandOp: string | null,
  state: ActionState,
): QuickActionState {
  return {
    ...state,
    id,
    action,
    slot,
    key,
    iconURL,
    commandOp,
    locked: false,
    state: actionStateKind(state),
  };
}

export function lockedQuickAction(
  id: QuickActionID,
  action: QuickActionCommand,
  slot: QuickActionState['slot'],
  key: string,
  iconURL: string,
  label: string,
  title: string,
): QuickActionState {
  return {
    id,
    action,
    slot,
    key,
    iconURL,
    commandOp: null,
    enabled: false,
    locked: true,
    label,
    detail: 'Locked',
    title,
    state: 'locked',
  };
}

export function actionSlotHTML(action: QuickActionState): string {
  const commandAttr = action.commandOp ? ` data-command-op="${escapeHTML(action.commandOp)}"` : '';
  return `
    <div class="action-slot" data-slot="${action.slot}" data-quick-action-slot="${action.id}" data-state="${action.state}"${commandAttr}>
      <button class="action-button" type="button" data-action="${action.action}" data-quick-action="${action.id}" data-state="${action.state}" aria-label="${escapeHTML(action.label)} action" aria-keyshortcuts="${action.key}" ${action.enabled ? '' : 'disabled'} title="${escapeHTML(action.title)}">
        <img class="action-button__icon" src="${escapeHTML(action.iconURL)}" alt="" aria-hidden="true" draggable="false" />
        <span class="action-button__label">${escapeHTML(action.label)}</span>
        <small>${escapeHTML(action.detail)}</small>
      </button>
    </div>
  `;
}

export function actionStateKind(action: ActionState): QuickActionState['state'] {
  if (/scanning|paused/i.test(action.label)) {
    return 'scanning';
  }
  if (action.enabled) {
    return 'ready';
  }
  if (/pending/i.test(action.detail)) {
    return 'pending';
  }
  if (/cool|ready in|<1s|\d+s/i.test(action.label) || /cool|ready in|<1s|\d+s/i.test(action.detail)) {
    return 'cooldown';
  }
  return 'blocked';
}

export function lootCommandOp(action: ActionState): string {
  return action.label === 'Approach' ? 'move_to' : 'loot.pickup';
}

export function laserActionState(state: ClientState, target: VisibleEntity | null, serverNow: number | null = Date.now()): ActionState {
  if (!realtimeReady(state)) {
    return { enabled: false, label: 'Laser', detail: 'Offline', title: 'Realtime link is not authenticated.' };
  }
  if (state.ship?.disabled === true) {
    return { enabled: false, label: 'Laser', detail: 'Disabled', title: 'Repair the ship before firing.' };
  }
  if (!isAttackableVisibleTarget(target, selfEntity(state.visibleEntities))) {
    return { enabled: false, label: 'Laser', detail: 'Standby', title: 'Select an attackable target.' };
  }
  if (hasPendingOp(state, 'combat.use_skill')) {
    return { enabled: false, label: 'Laser', detail: 'Pending', title: 'Basic laser is pending.' };
  }

  const cooldownRemaining = (state.skillCooldowns.basic_laser ?? 0) - (serverNow ?? Date.now());
  if (cooldownRemaining > 0) {
    return {
      enabled: false,
      label: 'Cooling',
      detail: formatCooldown(cooldownRemaining),
      title: `Basic laser ready in ${formatCooldown(cooldownRemaining)}.`,
    };
  }

  const energyCost = state.stats?.basic_laser_energy_cost ?? null;
  const capacitor = state.ship?.capacitor ?? null;
  if (energyCost === null || energyCost <= 0 || capacitor === null) {
    return { enabled: false, label: 'Laser', detail: 'Stats', title: 'Awaiting combat stats.' };
  }
  if (capacitor < energyCost) {
    return {
      enabled: false,
      label: 'Laser',
      detail: `Need ${Math.ceil(energyCost - capacitor)}`,
      title: `Basic laser needs ${Math.round(energyCost)} capacitor.`,
    };
  }

  return {
    enabled: true,
    label: 'Laser',
    detail: `${Math.round(energyCost)} cap`,
    title: `Fire basic laser for ${Math.round(energyCost)} capacitor.`,
  };
}

export function scanActionState(state: ClientState): ActionState {
  const mode = state.scanMode;
  if (mode.enabled) {
    if (!realtimeReady(state)) {
      return { enabled: true, label: 'Paused', detail: 'Offline', title: 'Scanner automation is waiting for realtime. Click to stop.' };
    }
    if (state.ship?.disabled === true) {
      return { enabled: true, label: 'Paused', detail: 'Disabled', title: 'Ship is disabled. Click to stop scanner automation.' };
    }
    if (hasPendingOp(state, 'scan.pulse')) {
      return { enabled: true, label: 'Scanning', detail: 'Pending', title: 'Scanner pulse is pending. Click to stop.' };
    }
    if (state.planetIntel?.lastScan?.status === 'started') {
      return {
        enabled: true,
        label: 'Scanning',
        detail: scanModeTimeDetail(mode.nextPulseAt, 'Resolving'),
        title: 'Scanner pulse is resolving. Click to stop.',
      };
    }
    if (mode.lastError) {
      return {
        enabled: true,
        label: 'Scanning',
        detail: scanModeTimeDetail(mode.nextPulseAt, 'Backoff'),
        title: `${mode.lastError} Click to stop scanner automation.`,
      };
    }
    return {
      enabled: true,
      label: 'Scanning',
      detail: scanModeTimeDetail(mode.nextPulseAt, 'Ready'),
      title: 'Automatic scanner pulses are enabled. Click to stop.',
    };
  }

  if (!realtimeReady(state)) {
    return { enabled: false, label: 'Scan', detail: 'Offline', title: 'Realtime link is not authenticated.' };
  }
  if (state.ship?.disabled === true) {
    return { enabled: false, label: 'Scan', detail: 'Disabled', title: 'Repair the ship before scanning.' };
  }
  if (hasPendingOp(state, 'scan.pulse')) {
    return { enabled: false, label: 'Scan', detail: 'Pending', title: 'Scanner pulse is pending.' };
  }
  return {
    enabled: true,
    label: 'Scan',
    detail: state.planetIntel?.lastScan?.status ? actionScanLabel(state.planetIntel.lastScan.status) : 'Ready',
    title: 'Start automatic scanner pulses.',
  };
}

export function stealthActionState(state: ClientState): ActionState {
  const self = selfEntity(state.visibleEntities);
  const enabled = self?.status_flags?.includes('stealthed') ?? false;
  if (!realtimeReady(state)) {
    return { enabled: false, label: 'Cloak', detail: 'Offline', title: 'Realtime link is not authenticated.' };
  }
  if (state.ship?.disabled === true) {
    return { enabled: false, label: 'Cloak', detail: 'Disabled', title: 'Repair the ship before toggling cloak.' };
  }
  if (hasPendingOp(state, 'stealth.toggle')) {
    return { enabled: false, label: enabled ? 'Cloaked' : 'Cloak', detail: 'Pending', title: 'Cloak toggle is pending.' };
  }
  if (enabled) {
    const speed = state.stats?.speed ? `${Math.round(state.stats.speed)} spd` : 'Slow';
    return { enabled: true, label: 'Cloaked', detail: speed, title: 'Disable cloak and restore normal movement speed.' };
  }
  return { enabled: true, label: 'Cloak', detail: 'Ready', title: 'Enable cloak. Movement speed is reduced while cloaked.' };
}

export function lootActionState(state: ClientState, target: VisibleEntity | null, serverNow: number | null): ActionState {
  if (!realtimeReady(state)) {
    return { enabled: false, label: 'Gather', detail: 'Offline', title: 'Realtime link is not authenticated.' };
  }
  if (state.ship?.disabled === true) {
    return { enabled: false, label: 'Gather', detail: 'Disabled', title: 'Repair the ship before gathering drops.' };
  }
  if (!target || target.entity_type !== 'loot') {
    return { enabled: false, label: 'Gather', detail: 'Standby', title: 'Select a visible drop.' };
  }
  if (hasPendingOp(state, 'loot.pickup')) {
    return { enabled: false, label: 'Gather', detail: 'Pending', title: 'Loot pickup is pending.' };
  }

  const pickupRange = state.stats?.loot_pickup_range ?? 0;
  const distance = distanceToTarget(state, target.entity_id, serverNow);
  if (pickupRange <= 0) {
    return { enabled: false, label: 'Gather', detail: 'No range', title: 'Awaiting pickup range.' };
  }
  if (distance === null) {
    return { enabled: false, label: 'Gather', detail: 'No fix', title: 'Awaiting ship position.' };
  }
  if (distance <= pickupRange) {
    return {
      enabled: true,
      label: 'Gather',
      detail: `${Math.round(distance)}u`,
      title: `Pickup visible drop within ${Math.round(pickupRange)}u.`,
    };
  }
  if (hasPendingOp(state, 'move_to')) {
    return {
      enabled: false,
      label: 'Approach',
      detail: 'Pending',
      title: `Approach movement is pending for a drop ${Math.round(distance)}u away.`,
    };
  }
  return {
    enabled: true,
    label: 'Approach',
    detail: `${Math.round(distance)}u`,
    title: `Move toward drop, then request pickup within ${Math.round(pickupRange)}u.`,
  };
}

export function distanceToTarget(state: ClientState, targetID: string, serverNow: number | null): number | null {
  const target = state.visibleEntities[targetID];
  const local = selfEntity(state.visibleEntities);
  if (!target || !local) {
    return null;
  }
  const now = serverNow ?? Date.now();
  return distanceBetween(currentEntityPosition(local, now), currentEntityPosition(target, now));
}

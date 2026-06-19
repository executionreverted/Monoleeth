import { ClientState } from '../state/types';
import { renderToast } from './toast';

export interface HUDHandlers {
  onConnect(url: string): void;
  onDisconnect(): void;
  onLogout(): void;
  onStop(): void;
  onSync(): void;
  onFire(): void;
  onLoot(): void;
  onRepairQuote(): void;
  onRepair(): void;
  onScan(): void;
}

type EntityCombatStatus = NonNullable<ClientState['visibleEntities'][string]['combat']>;

export class HUD {
  private readonly root: HTMLElement;
  private readonly socketInput: HTMLInputElement;
  private readonly panels: Record<string, HTMLElement>;
  private readonly toast: HTMLElement;

  constructor(container: HTMLElement, private readonly handlers: HUDHandlers) {
    this.root = document.createElement('section');
    this.root.className = 'hud';
    this.root.innerHTML = `
      <header class="hud__topbar">
        <div class="brand-lockup" aria-label="Space MORPG">
          <span class="brand-lockup__mark"></span>
          <strong>Frontier Console</strong>
        </div>
        <div class="top-status" aria-label="Server-owned status">
          <div><span>Sector</span><strong data-top-sector>${lockedValue()}</strong></div>
          <div><span>Danger</span><strong data-top-danger>${lockedValue()}</strong></div>
          <div><span>Cargo</span><strong data-top-cargo>${lockedValue()}</strong></div>
          <div><span>Credits</span><strong data-top-credits>${lockedValue()}</strong></div>
        </div>
        <label class="socket-field demo-only">
          <span>WS</span>
          <input class="socket-field__input" type="url" value="" aria-label="WebSocket URL" />
        </label>
        <div class="toolbar" aria-label="Connection and intent controls">
          <button class="tool-button demo-only" data-action="connect" type="button" title="Connect fixture">Link</button>
          <button class="tool-button demo-only" data-action="disconnect" type="button" title="Disconnect fixture">Cut</button>
          <button class="tool-button" data-action="stop" type="button" title="Stop">Stop</button>
          <button class="tool-button" data-action="sync" type="button" title="Request snapshot">Sync</button>
          <button class="tool-button" data-action="logout" type="button" title="Logout">Logout</button>
        </div>
      </header>
      <aside class="hud__rail hud__rail--left">
        <div class="panel panel--status" data-panel="status"></div>
        <div class="panel" data-panel="cargo"></div>
        <div class="panel" data-panel="quest"></div>
      </aside>
      <aside class="hud__rail hud__rail--right">
        <div class="panel" data-panel="target"></div>
        <div class="panel" data-panel="ship"></div>
        <div class="panel" data-panel="intel"></div>
      </aside>
      <footer class="hud__actionbar panel" data-panel="actions"></footer>
      <footer class="hud__log panel" data-panel="log"></footer>
      <div class="toast" role="status" aria-live="polite"></div>
    `;

    container.appendChild(this.root);
    this.socketInput = this.root.querySelector<HTMLInputElement>('.socket-field__input')!;
    this.toast = this.root.querySelector<HTMLElement>('.toast')!;
    this.panels = {
      status: this.panel('status'),
      cargo: this.panel('cargo'),
      quest: this.panel('quest'),
      target: this.panel('target'),
      ship: this.panel('ship'),
      intel: this.panel('intel'),
      actions: this.panel('actions'),
      log: this.panel('log'),
    };

    this.bindEvents();
  }

  render(state: ClientState): void {
    this.socketInput.value = state.socketURL;
    this.root.dataset.connection = state.connectionStatus;
    this.root.dataset.mode = state.auth.mode;
    const sector = this.root.querySelector<HTMLElement>('[data-top-sector]');
    const danger = this.root.querySelector<HTMLElement>('[data-top-danger]');
    const cargo = this.root.querySelector<HTMLElement>('[data-top-cargo]');
    const credits = this.root.querySelector<HTMLElement>('[data-top-credits]');
    if (sector) {
      sector.textContent = state.sector?.name || '--';
    }
    if (danger) {
      danger.textContent = state.sector ? (state.sector.contested ? 'contested' : state.sector.danger) : '--';
    }
    if (cargo) {
      cargo.textContent = state.cargo ? `${state.cargo.used}/${state.cargo.capacity}` : '--';
    }
    if (credits) {
      credits.textContent = state.wallet ? String(state.wallet.credits) : '--';
    }
    this.panels.status.innerHTML = statusPanel(state);
    this.panels.cargo.innerHTML = cargoPanel(state);
    this.panels.quest.innerHTML = questPanel(state);
    this.panels.target.innerHTML = targetPanel(state);
    this.panels.ship.innerHTML = shipPanel(state);
    this.panels.intel.innerHTML = intelPanel(state);
    this.panels.actions.innerHTML = actionBar(state);
    this.panels.log.innerHTML = logPanel(state);
    renderToast(this.toast, state.lastError?.message ?? null);
  }

  private bindEvents(): void {
    this.root.addEventListener('click', (event) => {
      const button = (event.target as HTMLElement).closest<HTMLButtonElement>('[data-action]');
      if (!button) {
        return;
      }

      switch (button.dataset.action) {
        case 'connect':
          this.handlers.onConnect(this.socketInput.value);
          break;
        case 'disconnect':
          this.handlers.onDisconnect();
          break;
        case 'stop':
          this.handlers.onStop();
          break;
        case 'sync':
          this.handlers.onSync();
          break;
        case 'logout':
          this.handlers.onLogout();
          break;
        case 'fire':
          this.handlers.onFire();
          break;
        case 'loot':
          this.handlers.onLoot();
          break;
        case 'repair-quote':
          this.handlers.onRepairQuote();
          break;
        case 'repair':
          this.handlers.onRepair();
          break;
        case 'scan':
          this.handlers.onScan();
          break;
      }
    });
  }

  private panel(name: string): HTMLElement {
    const panel = this.root.querySelector<HTMLElement>(`[data-panel="${name}"]`);
    if (!panel) {
      throw new Error(`Missing HUD panel ${name}`);
    }
    return panel;
  }
}

function statusPanel(state: ClientState): string {
  const snapshot = state.playerSnapshot;
  const stats = state.stats;
  const progression = state.progression;
  return `
    <h2>${escapeHTML(String(snapshot?.callsign ?? state.auth.session?.player?.callsign ?? 'Awaiting Pilot'))}</h2>
    <div class="status-grid">
      ${meter('HP', snapshot?.hp, snapshot?.max_hp)}
      ${meter('SHD', snapshot?.shield, snapshot?.max_shield)}
      ${meter('ENG', snapshot?.energy, snapshot?.max_energy)}
    </div>
    <div class="meta-row"><span>Rank</span><strong>${snapshot?.rank ?? lockedValue()}</strong></div>
    <div class="meta-row"><span>Level</span><strong>${progression?.main_level ?? lockedValue()}</strong></div>
    <div class="meta-row"><span>Speed</span><strong>${stats ? Math.round(stats.speed) : lockedValue()}</strong></div>
    <div class="meta-row"><span>Radar</span><strong>${stats ? Math.round(stats.radar_range) : lockedValue()}</strong></div>
    <div class="meta-row"><span>Link</span><strong>${escapeHTML(state.connectionStatus)}</strong></div>
  `;
}

function cargoPanel(state: ClientState): string {
  if (!state.cargo || !state.wallet) {
    return `
      <h2>Cargo</h2>
      <div class="empty-line">Awaiting server cargo and wallet snapshots.</div>
    `;
  }
  const percent = state.cargo.capacity > 0 ? Math.min(100, Math.round((state.cargo.used / state.cargo.capacity) * 100)) : 0;
  return `
    <h2>Cargo</h2>
    <div class="meter"><span style="width:${percent}%"></span></div>
    <div class="meta-row"><span>Hold</span><strong>${state.cargo.used}/${state.cargo.capacity}</strong></div>
    <div class="meta-row"><span>Credits</span><strong>${state.wallet.credits}</strong></div>
    <ul class="compact-list">
      ${state.cargo.items.map((item) => `<li><span>${escapeHTML(item.item_id)}</span><strong>${item.quantity}</strong></li>`).join('')}
    </ul>
  `;
}

function questPanel(state: ClientState): string {
  if (!state.questBoard) {
    return `
      <h2>Quest Board</h2>
      <div class="empty-line">Locked until quest snapshots are exposed.</div>
      <button class="ghost-action" type="button" disabled>Board</button>
    `;
  }
  return `
    <h2>Quest Board</h2>
    <div class="meta-row"><span>Available</span><strong>${state.questBoard.available}</strong></div>
    <div class="meta-row"><span>Active</span><strong>${state.questBoard.active}</strong></div>
    <button class="ghost-action" type="button" disabled title="Quest operations wait for gateway wiring">Board</button>
  `;
}

function targetPanel(state: ClientState): string {
  const target = state.selectedTargetID ? state.visibleEntities[state.selectedTargetID] : null;
  const shipDisabled = state.ship?.disabled === true;
  const laserReadyAt = state.skillCooldowns.basic_laser ?? 0;
  const canFire = target?.entity_type === 'npc' && !shipDisabled && laserReadyAt <= Date.now();
  const canLoot = target?.entity_type === 'loot' && !shipDisabled;
  const targetLabel = target?.display?.label ?? target?.entity_id ?? '';
  return `
    <h2>Target</h2>
    ${
      target
        ? `<div class="target-name">${escapeHTML(targetLabel)}</div>
           <div class="meta-row"><span>Type</span><strong>${escapeHTML(publicEntityType(target.entity_type))}</strong></div>
           <div class="meta-row"><span>State</span><strong>${escapeHTML(target.display?.disposition ?? '--')}</strong></div>
           <div class="meta-row"><span>X/Y</span><strong>${Math.round(target.position.x)} / ${Math.round(target.position.y)}</strong></div>
           ${target.combat ? combatStatusBlock(target.combat) : ''}`
        : '<div class="empty-line">No lock</div>'
    }
    <div class="segmented">
      <button type="button" disabled title="Click a visible entity on the map to target it">Aim</button>
      <button type="button" data-action="fire" ${canFire ? '' : 'disabled'} title="Use the basic server-side skill">Fire</button>
      <button type="button" data-action="loot" ${canLoot ? '' : 'disabled'} title="Request visible drop pickup">Loot</button>
    </div>
  `;
}

function shipPanel(state: ClientState): string {
  if (!state.ship) {
    return `
      <h2>Ship</h2>
      <div class="empty-line">Awaiting server ship snapshot.</div>
    `;
  }

  const ship = state.ship;
  const quote = state.repairQuote && state.repairQuote.ship_id === ship.active_ship_id ? state.repairQuote : null;
  const repairDisabled = !ship.disabled || !quote;
  return `
    <h2>Ship</h2>
    <div class="target-name">${escapeHTML(ship.display_name || ship.active_ship_id)}</div>
    ${meter('Hull', ship.hull, ship.max_hull)}
    ${meter('SHD', ship.shield, ship.max_shield)}
    ${meter('Cap', ship.capacitor, ship.max_capacitor)}
    <div class="meta-row"><span>State</span><strong>${escapeHTML(ship.repair_state || (ship.disabled ? 'disabled' : 'active'))}</strong></div>
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

function actionBar(state: ClientState): string {
  const target = state.selectedTargetID ? state.visibleEntities[state.selectedTargetID] : null;
  const shipDisabled = state.ship?.disabled === true;
  const laserReadyAt = state.skillCooldowns.basic_laser ?? 0;
  const laserCooling = laserReadyAt > Date.now();
  const canLaser = target?.entity_type === 'npc' && !shipDisabled && !laserCooling;
  const canLoot = target?.entity_type === 'loot' && !shipDisabled;
  const laserLabel = laserCooling ? 'Cooling' : 'Laser';

  return `
    <div class="action-slot">
      <button class="action-button" type="button" data-action="fire" ${canLaser ? '' : 'disabled'} title="Basic laser">
        <span>${escapeHTML(laserLabel)}</span>
        <small>${target?.entity_type === 'npc' ? 'Ready' : 'No target'}</small>
      </button>
    </div>
    <div class="action-slot">
      <button class="action-button" type="button" disabled title="Missile skills are not exposed yet">
        <span>Rocket</span>
        <small>Locked</small>
      </button>
    </div>
    <div class="action-slot">
      <button class="action-button" type="button" disabled title="Shield skills are not exposed yet">
        <span>Shield</span>
        <small>Locked</small>
      </button>
    </div>
    <div class="action-slot">
      <button class="action-button" type="button" data-action="loot" ${canLoot ? '' : 'disabled'} title="Pickup selected visible drop">
        <span>Loot</span>
        <small>${target?.entity_type === 'loot' ? 'Visible' : 'No drop'}</small>
      </button>
    </div>
  `;
}

function intelPanel(state: ClientState): string {
  return `
    <h2>Sector Map</h2>
    ${minimapPanel(state)}
    <div class="meta-row"><span>Signals</span><strong>${state.planetIntel ? state.planetIntel.knownSignals : lockedValue()}</strong></div>
    <div class="meta-row"><span>Stale</span><strong>${state.planetIntel?.staleIntel ?? lockedValue()}</strong></div>
    <button class="ghost-action" type="button" data-action="scan" disabled title="Scanner command is wired in a later phase">Pulse</button>
  `;
}

function minimapPanel(state: ClientState): string {
  if (!state.minimap || state.minimap.live_contacts.length === 0) {
    return '<div class="minimap minimap--empty"><div class="empty-line">Awaiting server map projection.</div></div>';
  }

  const contacts = state.minimap.live_contacts;
  const self = contacts.find((contact) => contact.status_flags?.includes('self')) ?? contacts.find((contact) => contact.entity_type === 'player');
  const center = self?.position ?? { x: 0, y: 0 };
  const radius = Math.max(state.minimap.radar_range, 1);
  const points = contacts
    .map((contact) => {
      const left = clamp(50 + ((contact.position.x - center.x) / (radius * 2)) * 100, 4, 96);
      const top = clamp(50 + ((contact.position.y - center.y) / (radius * 2)) * 100, 4, 96);
      const disposition = contact.status_flags?.includes('self') ? 'self' : contact.disposition || dispositionForType(contact.entity_type);
      return `<span class="minimap__point" data-kind="${escapeHTML(disposition)}" style="left:${left}%;top:${top}%" title="${escapeHTML(publicEntityType(contact.entity_type))}"></span>`;
    })
    .join('');

  return `
    <div class="minimap" aria-label="Server-filtered sector map">
      <span class="minimap__ring minimap__ring--outer"></span>
      <span class="minimap__ring minimap__ring--middle"></span>
      <span class="minimap__axis minimap__axis--x"></span>
      <span class="minimap__axis minimap__axis--y"></span>
      ${points}
    </div>
    <div class="minimap-legend">
      <span data-kind="self">You</span>
      <span data-kind="hostile">Hostile</span>
      <span data-kind="unknown">Unknown</span>
    </div>
  `;
}

function logPanel(state: ClientState): string {
  const lines = [...state.combatLog, ...state.commandLog].slice(-7).reverse();
  return `
    <h2>Log</h2>
    <ol class="log-lines">
      ${lines.map((line) => `<li data-level="${line.level}">${escapeHTML(line.text)}</li>`).join('')}
    </ol>
  `;
}

function meter(label: string, current?: number, max?: number): string {
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

function combatStatusBlock(combat: EntityCombatStatus): string {
  return `
    <div class="combat-status">
      ${meter('Hull', combat.hp, combat.max_hp)}
      ${meter('SHD', combat.shield, combat.max_shield)}
      <div class="meta-row"><span>Combat</span><strong>${escapeHTML(combat.status ?? 'active')}</strong></div>
    </div>
  `;
}

function lockedValue(): string {
  return '--';
}

function publicEntityType(entityType: string): string {
  switch (entityType) {
    case 'npc':
      return 'hostile';
    case 'loot':
      return 'drop';
    case 'planet_signal':
      return 'signal';
    default:
      return entityType;
  }
}

function dispositionForType(entityType: string): string {
  switch (entityType) {
    case 'npc':
      return 'hostile';
    case 'planet_signal':
      return 'unknown';
    case 'loot':
      return 'neutral';
    default:
      return 'friendly';
  }
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function escapeHTML(value: string): string {
  return value.replace(/[&<>"']/g, (char) => {
    switch (char) {
      case '&':
        return '&amp;';
      case '<':
        return '&lt;';
      case '>':
        return '&gt;';
      case '"':
        return '&quot;';
      default:
        return '&#39;';
    }
  });
}

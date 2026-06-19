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
  onScan(): void;
}

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
          <div><span>Sector</span><strong>${lockedValue()}</strong></div>
          <div><span>Danger</span><strong>${lockedValue()}</strong></div>
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
        <div class="panel" data-panel="loadout"></div>
        <div class="panel" data-panel="intel"></div>
      </aside>
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
      loadout: this.panel('loadout'),
      intel: this.panel('intel'),
      log: this.panel('log'),
    };

    this.bindEvents();
  }

  render(state: ClientState): void {
    this.socketInput.value = state.socketURL;
    this.root.dataset.connection = state.connectionStatus;
    this.root.dataset.mode = state.auth.mode;
    const cargo = this.root.querySelector<HTMLElement>('[data-top-cargo]');
    const credits = this.root.querySelector<HTMLElement>('[data-top-credits]');
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
    this.panels.loadout.innerHTML = loadoutPanel(state);
    this.panels.intel.innerHTML = intelPanel(state);
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
  return `
    <h2>${escapeHTML(String(snapshot?.callsign ?? state.auth.session?.player?.callsign ?? 'Awaiting Pilot'))}</h2>
    <div class="status-grid">
      ${meter('HP', snapshot?.hp, snapshot?.max_hp)}
      ${meter('SHD', snapshot?.shield, snapshot?.max_shield)}
      ${meter('ENG', snapshot?.energy, snapshot?.max_energy)}
    </div>
    <div class="meta-row"><span>Rank</span><strong>${snapshot?.rank ?? lockedValue()}</strong></div>
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
  const canFire = target?.entity_type === 'npc_placeholder';
  const canLoot = target?.entity_type === 'loot_placeholder';
  return `
    <h2>Target</h2>
    ${
      target
        ? `<div class="target-name">${escapeHTML(target.entity_id)}</div>
           <div class="meta-row"><span>Type</span><strong>${escapeHTML(target.entity_type)}</strong></div>
           <div class="meta-row"><span>X/Y</span><strong>${Math.round(target.position.x)} / ${Math.round(target.position.y)}</strong></div>`
        : '<div class="empty-line">No lock</div>'
    }
    <div class="segmented">
      <button type="button" disabled title="Click a visible entity on the map to target it">Aim</button>
      <button type="button" data-action="fire" ${canFire ? '' : 'disabled'} title="Use the basic server-side skill">Fire</button>
      <button type="button" data-action="loot" ${canLoot ? '' : 'disabled'} title="Request visible drop pickup">Loot</button>
    </div>
  `;
}

function loadoutPanel(state: ClientState): string {
  if (!state.inventory) {
    return `
      <h2>Loadout</h2>
      <div class="empty-line">Locked until inventory and loadout snapshots are exposed.</div>
      <button class="ghost-action" type="button" disabled>Open</button>
    `;
  }
  return `
    <h2>Loadout</h2>
    <div class="meta-row"><span>Equipped</span><strong>${state.inventory.equipped}</strong></div>
    <div class="meta-row"><span>Stored</span><strong>${state.inventory.storage}</strong></div>
    <button class="ghost-action" type="button" disabled title="Inventory gateway is not exposed yet">Open</button>
  `;
}

function intelPanel(state: ClientState): string {
  return `
    <h2>Intel</h2>
    <div class="meta-row"><span>Signals</span><strong>${state.planetIntel ? state.planetIntel.knownSignals : lockedValue()}</strong></div>
    <div class="meta-row"><span>Stale</span><strong>${state.planetIntel?.staleIntel ?? lockedValue()}</strong></div>
    <button class="ghost-action" type="button" data-action="scan" disabled title="Scanner command is wired in a later phase">Pulse</button>
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

function lockedValue(): string {
  return '--';
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

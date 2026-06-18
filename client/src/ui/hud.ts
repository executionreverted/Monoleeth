import { ClientState } from '../state/types';
import { renderToast } from './toast';

export interface HUDHandlers {
  onConnect(url: string): void;
  onDisconnect(): void;
  onStop(): void;
  onDebugSnapshot(): void;
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
        <label class="socket-field">
          <span>WS</span>
          <input class="socket-field__input" type="url" value="" aria-label="WebSocket URL" />
        </label>
        <div class="toolbar" aria-label="Connection and intent controls">
          <button class="tool-button" data-action="connect" type="button" title="Connect">Link</button>
          <button class="tool-button" data-action="disconnect" type="button" title="Disconnect">Cut</button>
          <button class="tool-button" data-action="stop" type="button" title="Stop">Stop</button>
          <button class="tool-button" data-action="snapshot" type="button" title="Request snapshot">Sync</button>
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
        case 'snapshot':
          this.handlers.onDebugSnapshot();
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
  return `
    <h2>${escapeHTML(String(snapshot.callsign ?? 'Pilot'))}</h2>
    <div class="status-grid">
      ${meter('HP', snapshot.hp, snapshot.max_hp)}
      ${meter('SHD', snapshot.shield, snapshot.max_shield)}
      ${meter('ENG', snapshot.energy, snapshot.max_energy)}
    </div>
    <div class="meta-row"><span>Rank</span><strong>${escapeHTML(String(snapshot.rank ?? 1))}</strong></div>
    <div class="meta-row"><span>Link</span><strong>${escapeHTML(state.connectionStatus)}</strong></div>
  `;
}

function cargoPanel(state: ClientState): string {
  const percent = Math.min(100, Math.round((state.cargo.used / Math.max(state.cargo.capacity, 1)) * 100));
  return `
    <h2>Cargo</h2>
    <div class="meter"><span style="width:${percent}%"></span></div>
    <div class="meta-row"><span>Hold</span><strong>${state.cargo.used}/${state.cargo.capacity}</strong></div>
    <ul class="compact-list">
      ${state.cargo.items.map((item) => `<li><span>${escapeHTML(item.item_id)}</span><strong>${item.quantity}</strong></li>`).join('')}
    </ul>
  `;
}

function questPanel(state: ClientState): string {
  return `
    <h2>Quest Board</h2>
    <div class="meta-row"><span>Available</span><strong>${state.questBoard.available}</strong></div>
    <div class="meta-row"><span>Active</span><strong>${state.questBoard.active}</strong></div>
    <button class="ghost-action" type="button" disabled title="Quest operations wait for gateway wiring">Board</button>
  `;
}

function targetPanel(state: ClientState): string {
  const target = state.selectedTargetID ? state.visibleEntities[state.selectedTargetID] : null;
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
      <button type="button" disabled title="combat.set_target is not exposed yet">Aim</button>
      <button type="button" disabled title="combat.use_skill is not exposed yet">Fire</button>
      <button type="button" disabled title="loot.pickup is not exposed yet">Loot</button>
    </div>
  `;
}

function loadoutPanel(state: ClientState): string {
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
    <div class="meta-row"><span>Signals</span><strong>${state.planetIntel.knownSignals}</strong></div>
    <div class="meta-row"><span>Stale</span><strong>${state.planetIntel.staleIntel}</strong></div>
    <button class="ghost-action" type="button" disabled title="scan.pulse is not exposed yet">Pulse</button>
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
  const safeMax = Math.max(max ?? 100, 1);
  const safeCurrent = Math.max(0, Math.min(current ?? 0, safeMax));
  const percent = Math.round((safeCurrent / safeMax) * 100);
  return `
    <div class="stat-meter">
      <div class="stat-meter__label">${label}</div>
      <div class="meter"><span style="width:${percent}%"></span></div>
      <strong>${safeCurrent}</strong>
    </div>
  `;
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

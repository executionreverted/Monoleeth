import { ClientState } from '../state/types';
import { renderToast } from './toast';
import gatherIconURL from '../../../output/assets/hud-svg/icons/gather.svg?url';
import laserIconURL from '../../../output/assets/hud-svg/icons/laser.svg?url';
import rocketIconURL from '../../../output/assets/hud-svg/icons/rocket.svg?url';
import scanIconURL from '../../../output/assets/hud-svg/icons/scan.svg?url';
import shieldIconURL from '../../../output/assets/hud-svg/icons/shield.svg?url';
import warpIconURL from '../../../output/assets/hud-svg/icons/warp.svg?url';

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
  onPlanetDetail(planetID: string): void;
  onMarketCreateListing(input: {
    itemID: string;
    quantity: number;
    unitPrice: number;
    sourceLocation?: string;
    itemInstanceID?: string;
  }): void;
  onMarketBuy(listingID: string): void;
  onMarketCancel(listingID: string): void;
  onAuctionBid(auctionID: string, amount: number): void;
  onAuctionBuyNow(auctionID: string): void;
  onAuctionClaimGrant(): void;
  onPremiumClaim(entitlementID: string): void;
  onPremiumWeeklyXCore(): void;
  onQuestAccept(offerID: string): void;
  onQuestClaim(questID: string): void;
  onQuestReroll(): void;
  onAdminRefresh(): void;
  onAdminRepairCraftJob(jobID: string): void;
}

type EntityCombatStatus = NonNullable<ClientState['visibleEntities'][string]['combat']>;
type KnownLootDropStatus = ClientState['knownLoot'][string];
type VisibleEntity = ClientState['visibleEntities'][string];
type HUDWindowID = 'cargo' | 'economy' | 'quests' | 'intel' | 'systems' | 'ops';
type HUDModalID = HUDWindowID | 'target' | 'planets' | 'ship';
type QuickActionID = 'laser' | 'rocket' | 'scan' | 'shield' | 'warp' | 'gather';
type QuickActionCommand = 'fire' | 'rocket' | 'scan' | 'shield' | 'warp' | 'loot';

interface ActionState {
  enabled: boolean;
  label: string;
  detail: string;
  title: string;
}

interface QuickActionState extends ActionState {
  id: QuickActionID;
  action: QuickActionCommand;
  slot: 1 | 2 | 3 | 4 | 5 | 6;
  key: string;
  iconURL: string;
  commandOp: string | null;
  locked: boolean;
  state: 'ready' | 'pending' | 'cooldown' | 'blocked' | 'locked' | 'scanning';
}

interface HUDPanelDefinition {
  id: HUDWindowID;
  label: string;
  title: string;
  render(state: ClientState): string;
  hidden?(state: ClientState): boolean;
}

interface HUDModalState {
  id: HUDModalID;
  title: string;
  body: string;
}

interface HUDWindowState {
  id: HUDWindowID;
  x: number;
  y: number;
  z: number;
  open: boolean;
}

interface HUDDragState {
  id: HUDWindowID;
  pointerID: number;
  offsetX: number;
  offsetY: number;
}

export class HUD {
  private readonly root: HTMLElement;
  private readonly nav: HTMLElement;
  private readonly windowLayer: HTMLElement;
  private readonly modalLayer: HTMLElement;
  private readonly socketInput: HTMLInputElement;
  private readonly panels: Record<string, HTMLElement>;
  private readonly toast: HTMLElement;
  private readonly windowStates = new Map<HUDWindowID, HUDWindowState>();
  private dragState: HUDDragState | null = null;
  private focusedWindow: HUDWindowID | null = null;
  private modal: HUDModalState | null = null;
  private currentState: ClientState | null = null;
  private nextWindowZ = 20;
  private readonly dragMove = (event: PointerEvent) => this.handleDragMove(event);
  private readonly dragEnd = (event: PointerEvent) => this.handleDragEnd(event);
  private readonly shortcutKeyDown = (event: KeyboardEvent) => this.handleShortcutKeyDown(event);

  constructor(container: HTMLElement, private readonly handlers: HUDHandlers) {
    this.root = document.createElement('section');
    this.root.className = 'hud';
    this.root.innerHTML = `
      <header class="hud__topbar">
        <div class="top-status" aria-label="Server-owned status">
          <div class="top-status__cell" data-icon="sector"><span>Sector</span><strong data-top-sector>${lockedValue()}</strong></div>
          <div class="top-status__cell top-status__cell--danger" data-icon="danger"><span>Danger</span><strong data-top-danger>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="energy"><span>Energy</span><strong data-top-energy>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="cargo"><span>Cargo</span><strong data-top-cargo>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="credits"><span>Credits</span><strong data-top-credits>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="cap"><span>Cap</span><strong data-top-cap>${lockedValue()}</strong></div>
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
          <button class="tool-button tool-button--locked" type="button" disabled title="Mail contracts are not exposed yet">Mail</button>
          <button class="tool-button tool-button--locked" type="button" disabled title="Social contracts are not exposed yet">Social</button>
          <button class="tool-button" data-action="logout" type="button" title="Logout">Logout</button>
        </div>
      </header>
      <aside class="hud__rail hud__rail--left">
        <div class="panel panel--status" data-panel="status"></div>
        <nav class="panel hud__nav" data-hud-nav aria-label="HUD panels"></nav>
      </aside>
      <div class="hud__window-layer" data-window-layer></div>
      <div class="hud__aux-panels" aria-hidden="true">
        <div class="panel panel--cargo" data-panel="cargo"></div>
        <div class="panel panel--economy" data-panel="economy"></div>
        <div class="panel panel--systems" data-panel="systems"></div>
        <div class="panel panel--quests" data-panel="quests"></div>
        <div class="panel panel--ops" data-panel="ops"></div>
        <div class="panel panel--drawer" data-panel="drawer" data-open="false"></div>
      </div>
      <aside class="hud__rail hud__rail--right">
        <div class="panel panel--planets" data-panel="planets"></div>
        <div class="panel" data-panel="target"></div>
        <div class="panel" data-panel="ship"></div>
        <div class="panel" data-panel="intel"></div>
      </aside>
      <footer class="hud__actionbar panel" data-panel="actions"></footer>
      <footer class="hud__log panel" data-panel="log"></footer>
      <div class="hud__modal-layer" data-modal-layer></div>
      <div class="toast" role="status" aria-live="polite"></div>
    `;

    container.appendChild(this.root);
    this.nav = this.root.querySelector<HTMLElement>('[data-hud-nav]')!;
    this.windowLayer = this.root.querySelector<HTMLElement>('[data-window-layer]')!;
    this.modalLayer = this.root.querySelector<HTMLElement>('[data-modal-layer]')!;
    this.socketInput = this.root.querySelector<HTMLInputElement>('.socket-field__input')!;
    this.toast = this.root.querySelector<HTMLElement>('.toast')!;
    this.panels = {
      status: this.panel('status'),
      cargo: this.panel('cargo'),
      economy: this.panel('economy'),
      systems: this.panel('systems'),
      quests: this.panel('quests'),
      ops: this.panel('ops'),
      drawer: this.panel('drawer'),
      planets: this.panel('planets'),
      target: this.panel('target'),
      ship: this.panel('ship'),
      intel: this.panel('intel'),
      actions: this.panel('actions'),
      log: this.panel('log'),
    };

    this.bindEvents();
  }

  render(state: ClientState): void {
    this.currentState = state;
    this.socketInput.value = state.socketURL;
    this.root.dataset.connection = state.connectionStatus;
    this.root.dataset.mode = state.auth.mode;
    this.root.dataset.activePanel = this.focusedWindow ?? 'none';
    const sector = this.root.querySelector<HTMLElement>('[data-top-sector]');
    const danger = this.root.querySelector<HTMLElement>('[data-top-danger]');
    const energy = this.root.querySelector<HTMLElement>('[data-top-energy]');
    const cargo = this.root.querySelector<HTMLElement>('[data-top-cargo]');
    const credits = this.root.querySelector<HTMLElement>('[data-top-credits]');
    const cap = this.root.querySelector<HTMLElement>('[data-top-cap]');
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
      credits.textContent = state.wallet ? formatCompactNumber(state.wallet.credits) : '--';
    }
    if (energy) {
      energy.textContent = formatPair(state.playerSnapshot?.energy, state.playerSnapshot?.max_energy);
    }
    if (cap) {
      cap.textContent = formatPercent(state.ship?.capacitor, state.ship?.max_capacitor);
    }
    this.panels.status.innerHTML = statusPanel(state);
    this.panels.cargo.innerHTML = cargoPanel(state);
    this.panels.economy.innerHTML = economyPanel(state);
    this.panels.systems.innerHTML = systemsPanel(state);
    this.panels.quests.innerHTML = questsPanel(state);
    this.panels.ops.innerHTML = opsPanel(state);
    this.panels.planets.innerHTML = planetsPanel(state);
    this.panels.target.innerHTML = targetPanel(state);
    this.panels.ship.innerHTML = shipPanel(state);
    this.panels.intel.innerHTML = intelPanel(state);
    this.panels.actions.innerHTML = actionBar(state);
    this.panels.log.innerHTML = logPanel(state);
    this.renderNav(state);
    this.renderWindows(state);
    this.renderModal();
    renderToast(this.toast, state.lastError?.message ?? null);
  }

  private bindEvents(): void {
    this.root.addEventListener(
      'pointerdown',
      (event) => {
        const target = event.target;
        if (blocksWorldInput(target)) {
          markHUDInputSuppressed();
        }
        const targetElement = target instanceof HTMLElement ? target : null;
        if (!targetElement) {
          return;
        }

        const windowPanel = targetElement.closest<HTMLElement>('[data-window-panel]');
        if (windowPanel) {
          const panel = normalizePanelID(windowPanel.dataset.windowPanel);
          if (panel && this.isWindowOpen(panel)) {
            this.raiseWindow(panel);
          }
        }

        const dragHandle = targetElement.closest<HTMLElement>('[data-window-drag]');
        if (!dragHandle || event.button !== 0 || isControlElement(target)) {
          return;
        }
        const panel = normalizePanelID(dragHandle.dataset.windowDrag);
        if (!panel) {
          return;
        }
        this.startDrag(panel, event);
      },
      { capture: true },
    );

    window.addEventListener('pointermove', this.dragMove);
    window.addEventListener('pointerup', this.dragEnd);
    window.addEventListener('pointercancel', this.dragEnd);
    window.addEventListener('keydown', this.shortcutKeyDown);

    this.root.addEventListener('click', (event) => {
      if (blocksWorldInput(event.target)) {
        markHUDInputSuppressed();
        event.stopPropagation();
      }
      const panelToggle = (event.target as HTMLElement).closest<HTMLButtonElement>('[data-panel-toggle]');
      if (panelToggle) {
        const nextPanel = normalizePanelID(panelToggle.dataset.panelToggle);
        if (nextPanel) {
          this.toggleWindow(nextPanel);
        } else if (panelToggle.dataset.panelToggle === 'none') {
          this.closeFocusedWindow();
        }
        if (this.currentState) {
          this.render(this.currentState);
        }
        return;
      }

      const panelFocus = (event.target as HTMLElement).closest<HTMLElement>('[data-window-panel]');
      if (panelFocus) {
        const panel = normalizePanelID(panelFocus.dataset.windowPanel);
        if (panel) {
          this.raiseWindow(panel);
        }
      }

      const panelClose = (event.target as HTMLElement).closest<HTMLButtonElement>('[data-panel-close]');
      if (panelClose) {
        const panel = normalizePanelID(panelClose.dataset.panelClose);
        if (panel) {
          this.closeWindow(panel);
          if (this.currentState) {
            this.render(this.currentState);
          }
        }
        return;
      }

      const modalOpen = (event.target as HTMLElement).closest<HTMLButtonElement>('[data-modal-open]');
      if (modalOpen) {
        const panel = normalizeModalID(modalOpen.dataset.modalOpen);
        if (panel && this.currentState) {
          this.openModal(panel, this.currentState);
          this.render(this.currentState);
        }
        return;
      }

      const modalClose = (event.target as HTMLElement).closest<HTMLElement>('[data-modal-close]');
      if (modalClose) {
        this.closeModal();
        if (this.currentState) {
          this.render(this.currentState);
        }
        return;
      }

      const button = (event.target as HTMLElement).closest<HTMLButtonElement>('[data-action]');
      if (!button) {
        return;
      }
      if (button.disabled) {
        return;
      }

      this.dispatchButtonAction(button);
    });

    this.root.addEventListener('keydown', (event) => {
      if (event.key !== 'Escape' || !this.modal || !this.currentState) {
        return;
      }
      this.closeModal();
      this.render(this.currentState);
    });
  }

  private dispatchAction(action: string | undefined): boolean {
    switch (action) {
      case 'connect':
        this.handlers.onConnect(this.socketInput.value);
        return true;
      case 'disconnect':
        this.handlers.onDisconnect();
        return true;
      case 'stop':
        this.handlers.onStop();
        return true;
      case 'sync':
        this.handlers.onSync();
        return true;
      case 'logout':
        this.handlers.onLogout();
        return true;
      case 'fire':
        this.handlers.onFire();
        return true;
      case 'loot':
        this.handlers.onLoot();
        return true;
      case 'repair-quote':
        this.handlers.onRepairQuote();
        return true;
      case 'repair':
        this.handlers.onRepair();
        return true;
      case 'scan':
        this.handlers.onScan();
        return true;
      case 'auction-claim':
        this.handlers.onAuctionClaimGrant();
        return true;
      case 'premium-weekly-xcore':
        this.handlers.onPremiumWeeklyXCore();
        return true;
      case 'quest-reroll':
        this.handlers.onQuestReroll();
        return true;
      case 'admin-refresh':
        this.handlers.onAdminRefresh();
        return true;
      default:
        return false;
    }
  }

  private dispatchButtonAction(button: HTMLButtonElement): void {
    if (this.dispatchAction(button.dataset.action)) {
      return;
    }
    switch (button.dataset.action) {
        case 'planet-detail':
          if (button.dataset.planetId) {
            this.handlers.onPlanetDetail(button.dataset.planetId);
          }
          break;
        case 'market-buy':
          if (button.dataset.listingId) {
            this.handlers.onMarketBuy(button.dataset.listingId);
          }
          break;
        case 'market-create':
          if (button.dataset.itemId) {
            this.handlers.onMarketCreateListing({
              itemID: button.dataset.itemId,
              quantity: Number(button.dataset.quantity ?? '1'),
              unitPrice: Number(button.dataset.unitPrice ?? '0'),
              sourceLocation: button.dataset.sourceLocation,
              itemInstanceID: button.dataset.itemInstanceId,
            });
          }
          break;
        case 'market-cancel':
          if (button.dataset.listingId) {
            this.handlers.onMarketCancel(button.dataset.listingId);
          }
          break;
        case 'auction-bid':
          if (button.dataset.auctionId) {
            this.handlers.onAuctionBid(button.dataset.auctionId, Number(button.dataset.amount ?? '0'));
          }
          break;
        case 'auction-buy-now':
          if (button.dataset.auctionId) {
            this.handlers.onAuctionBuyNow(button.dataset.auctionId);
          }
          break;
        case 'premium-claim':
          if (button.dataset.entitlementId) {
            this.handlers.onPremiumClaim(button.dataset.entitlementId);
          }
          break;
        case 'quest-accept':
          if (button.dataset.offerId) {
            this.handlers.onQuestAccept(button.dataset.offerId);
          }
          break;
        case 'quest-claim':
          if (button.dataset.questId) {
            this.handlers.onQuestClaim(button.dataset.questId);
          }
          break;
        case 'admin-repair-craft-job':
          if (button.dataset.jobId) {
            this.handlers.onAdminRepairCraftJob(button.dataset.jobId);
          }
          break;
      }
  }

  private handleShortcutKeyDown(event: KeyboardEvent): void {
    if (!this.currentState || !isQuickActionKey(event.key) || event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) {
      return;
    }
    if (!quickActionShortcutSafe(event.target, document.activeElement, this.modal, this.focusedWindow, this.dragState)) {
      return;
    }
    const action = quickActionStates(this.currentState).find((entry) => entry.key === event.key);
    if (!action?.enabled) {
      return;
    }
    event.preventDefault();
    event.stopPropagation();
    markHUDInputSuppressed();
    this.dispatchAction(action.action);
  }

  private panel(name: string): HTMLElement {
    const panel = this.root.querySelector<HTMLElement>(`[data-panel="${name}"]`);
    if (!panel) {
      throw new Error(`Missing HUD panel ${name}`);
    }
    return panel;
  }

  private renderNav(state: ClientState): void {
    this.nav.innerHTML = windowDefinitions(state)
      .map((definition) => {
        const active = this.isWindowOpen(definition.id);
        const focused = this.focusedWindow === definition.id;
        return `<button class="hud-nav-button" type="button" data-panel-toggle="${definition.id}" data-active="${active ? 'true' : 'false'}" data-focused="${focused ? 'true' : 'false'}" aria-pressed="${active ? 'true' : 'false'}"><span>${escapeHTML(definition.label)}</span></button>`;
      })
      .join('');
  }

  private renderWindows(state: ClientState): void {
    const definitions = windowDefinitions(state);
    const allowed = new Set(definitions.map((definition) => definition.id));
    for (const windowState of this.windowStates.values()) {
      if (!allowed.has(windowState.id)) {
        windowState.open = false;
      }
    }
    const openStates = this.openWindowStates(definitions);
    if (this.focusedWindow && !openStates.some((windowState) => windowState.id === this.focusedWindow)) {
      this.focusedWindow = openStates.at(-1)?.id ?? null;
    }

    this.windowLayer.innerHTML = openStates
      .map((windowState) => {
        const definition = definitions.find((entry) => entry.id === windowState.id);
        if (!definition) {
          return '';
        }
        const focused = windowState.id === this.focusedWindow;
        const size = windowSize(definition.id);
        return `
          <section class="hud-window" data-window-panel="${definition.id}" data-focused="${focused ? 'true' : 'false'}" data-open="true" data-x="${Math.round(windowState.x)}" data-y="${Math.round(windowState.y)}" style="--window-x:${windowState.x}px;--window-y:${windowState.y}px;--window-z:${windowState.z};--window-width:${size.width}px;--window-height:${size.height}px" tabindex="-1" aria-label="${escapeHTML(definition.title)}">
            <header class="hud-window__header" data-window-drag="${definition.id}">
              <strong>${escapeHTML(definition.title)}</strong>
              <div>
                <button type="button" data-modal-open="${definition.id}" title="Open detail">Inspect</button>
                <button type="button" data-panel-close="${definition.id}" title="Close panel">Close</button>
              </div>
            </header>
            <div class="hud-window__body">${definition.render(state)}</div>
          </section>
        `;
      })
      .join('');
  }

  private renderModal(): void {
    if (!this.modal) {
      this.modalLayer.innerHTML = '';
      this.modalLayer.dataset.open = 'false';
      return;
    }

    this.modalLayer.dataset.open = 'true';
    this.modalLayer.innerHTML = `
      <div class="hud-modal-backdrop" data-modal-close="backdrop"></div>
      <section class="hud-modal" data-modal="${this.modal.id}" role="dialog" aria-modal="true" aria-label="${escapeHTML(this.modal.title)}" tabindex="-1">
        <header class="hud-modal__header">
          <strong>${escapeHTML(this.modal.title)}</strong>
          <button type="button" data-modal-close="button" title="Close modal">Close</button>
        </header>
        <div class="hud-modal__body">${this.modal.body}</div>
      </section>
    `;
    this.modalLayer.querySelector<HTMLElement>('.hud-modal')?.focus();
  }

  private toggleWindow(panel: HUDWindowID): void {
    if (this.isWindowOpen(panel)) {
      this.raiseWindow(panel);
      return;
    }
    this.openWindow(panel);
  }

  private closeWindow(panel: HUDWindowID): void {
    const state = this.windowStates.get(panel);
    if (state) {
      state.open = false;
    }
    if (this.focusedWindow === panel) {
      this.focusedWindow = this.openWindowStates().at(-1)?.id ?? null;
    }
  }

  private closeFocusedWindow(): void {
    if (!this.focusedWindow) {
      return;
    }
    this.closeWindow(this.focusedWindow);
  }

  private openWindow(panel: HUDWindowID): void {
    let state = this.windowStates.get(panel);
    if (!state) {
      state = { id: panel, ...this.defaultWindowPosition(panel), z: ++this.nextWindowZ, open: true };
      this.windowStates.set(panel, state);
    }
    state.open = true;
    this.raiseWindow(panel);
  }

  private raiseWindow(panel: HUDWindowID): void {
    const state = this.windowStates.get(panel);
    if (!state?.open) {
      return;
    }
    state.z = ++this.nextWindowZ;
    this.focusedWindow = panel;
    this.root.dataset.activePanel = panel;
    this.applyWindowFocus();
  }

  private openModal(id: HUDModalID, state: ClientState): void {
    const modal = modalDefinition(id, state);
    if (!modal) {
      return;
    }
    this.modal = modal;
  }

  private closeModal(): void {
    this.modal = null;
  }

  private isWindowOpen(panel: HUDWindowID): boolean {
    return this.windowStates.get(panel)?.open === true;
  }

  private openWindowStates(definitions?: HUDPanelDefinition[]): HUDWindowState[] {
    const allowed = new Set((definitions ?? baseWindowDefinitions).map((definition) => definition.id));
    return [...this.windowStates.values()]
      .filter((windowState) => windowState.open && allowed.has(windowState.id))
      .sort((left, right) => left.z - right.z);
  }

  private defaultWindowPosition(panel: HUDWindowID): { x: number; y: number } {
    const size = windowSize(panel);
    const x = (window.innerWidth - Math.min(size.width, window.innerWidth - 16)) / 2;
    const y = (window.innerHeight - Math.min(size.height, window.innerHeight - 72)) / 2;
    return this.clampWindowPosition(panel, x, y);
  }

  private clampWindowPosition(panel: HUDWindowID, x: number, y: number): { x: number; y: number } {
    const size = windowSize(panel);
    const width = Math.min(size.width, Math.max(320, window.innerWidth - 16));
    const margin = 8;
    const topMargin = window.innerWidth < 768 ? margin : 56;
    const maxX = Math.max(margin, window.innerWidth - width - margin);
    const maxY = Math.max(topMargin, window.innerHeight - 52);
    return {
      x: clamp(x, margin, maxX),
      y: clamp(y, topMargin, maxY),
    };
  }

  private startDrag(panel: HUDWindowID, event: PointerEvent): void {
    if (window.innerWidth < 768) {
      this.raiseWindow(panel);
      return;
    }
    const windowPanel = (event.target as HTMLElement).closest<HTMLElement>('[data-window-panel]');
    const state = this.windowStates.get(panel);
    if (!windowPanel || !state?.open) {
      return;
    }
    const rect = windowPanel.getBoundingClientRect();
    this.raiseWindow(panel);
    this.dragState = {
      id: panel,
      pointerID: event.pointerId,
      offsetX: event.clientX - rect.left,
      offsetY: event.clientY - rect.top,
    };
    windowPanel.dataset.dragging = 'true';
    event.preventDefault();
  }

  private handleDragMove(event: PointerEvent): void {
    const drag = this.dragState;
    if (!drag || drag.pointerID !== event.pointerId) {
      return;
    }
    const state = this.windowStates.get(drag.id);
    if (!state) {
      return;
    }
    const next = this.clampWindowPosition(drag.id, event.clientX - drag.offsetX, event.clientY - drag.offsetY);
    state.x = next.x;
    state.y = next.y;
    this.applyWindowPosition(state);
    markHUDInputSuppressed();
    event.preventDefault();
  }

  private handleDragEnd(event: PointerEvent): void {
    if (!this.dragState || this.dragState.pointerID !== event.pointerId) {
      return;
    }
    const windowPanel = this.windowLayer.querySelector<HTMLElement>(`[data-window-panel="${this.dragState.id}"]`);
    if (windowPanel) {
      delete windowPanel.dataset.dragging;
    }
    this.dragState = null;
    markHUDInputSuppressed();
  }

  private applyWindowPosition(state: HUDWindowState): void {
    const element = this.windowLayer.querySelector<HTMLElement>(`[data-window-panel="${state.id}"]`);
    if (!element) {
      return;
    }
    element.style.setProperty('--window-x', `${state.x}px`);
    element.style.setProperty('--window-y', `${state.y}px`);
    element.style.setProperty('--window-z', String(state.z));
    element.dataset.x = String(Math.round(state.x));
    element.dataset.y = String(Math.round(state.y));
  }

  private applyWindowFocus(): void {
    for (const element of this.windowLayer.querySelectorAll<HTMLElement>('[data-window-panel]')) {
      const panel = normalizePanelID(element.dataset.windowPanel);
      const state = panel ? this.windowStates.get(panel) : null;
      element.dataset.focused = panel === this.focusedWindow ? 'true' : 'false';
      if (state) {
        element.style.setProperty('--window-z', String(state.z));
      }
    }
    for (const button of this.nav.querySelectorAll<HTMLButtonElement>('[data-panel-toggle]')) {
      const panel = normalizePanelID(button.dataset.panelToggle);
      if (!panel) {
        continue;
      }
      const active = this.isWindowOpen(panel);
      button.dataset.active = active ? 'true' : 'false';
      button.dataset.focused = panel === this.focusedWindow ? 'true' : 'false';
      button.setAttribute('aria-pressed', active ? 'true' : 'false');
    }
  }
}

const baseWindowDefinitions: HUDPanelDefinition[] = [
  { id: 'cargo', label: 'Inv', title: 'Inventory / Cargo', render: cargoPanel },
  { id: 'economy', label: 'Shop', title: 'Market / Auction / Premium', render: economyPanel },
  { id: 'quests', label: 'Galaxy', title: 'Quest Board', render: questsPanel },
  { id: 'intel', label: 'Planets', title: 'Intel / Scanner', render: intelPanel },
  { id: 'systems', label: 'Hangar', title: 'Systems / Loadout / Crafting', render: systemsPanel },
  { id: 'ops', label: 'Ops', title: 'Admin Ops', render: opsPanel, hidden: (state) => !state.auth.session?.account?.admin },
];

function windowSize(id: HUDWindowID): { width: number; height: number } {
  switch (id) {
    case 'economy':
      return { width: 440, height: 500 };
    case 'quests':
      return { width: 410, height: 460 };
    case 'intel':
      return { width: 420, height: 500 };
    case 'systems':
      return { width: 400, height: 480 };
    case 'ops':
      return { width: 450, height: 520 };
    case 'cargo':
    default:
      return { width: 380, height: 440 };
  }
}

function markHUDInputSuppressed(): void {
  (window as Window & { __SPACE_MORPG_HUD_INPUT_UNTIL__?: number }).__SPACE_MORPG_HUD_INPUT_UNTIL__ = performance.now() + 220;
}

function blocksWorldInput(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && Boolean(target.closest('.hud, .auth-panel, .hud-modal, .hud-window, button, input, select, textarea'));
}

function isControlElement(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && Boolean(target.closest('button, input, select, textarea, a[href], [data-action]'));
}

function isQuickActionKey(key: string): boolean {
  return key === '1' || key === '2' || key === '3' || key === '4' || key === '5' || key === '6';
}

function quickActionShortcutSafe(
  eventTarget: EventTarget | null,
  activeElement: Element | null,
  modal: HUDModalState | null,
  focusedWindow: HUDWindowID | null,
  dragState: HUDDragState | null,
): boolean {
  if (modal || focusedWindow || dragState) {
    return false;
  }
  const target = eventTarget instanceof HTMLElement ? eventTarget : null;
  return !isTextEntryElement(target) && !isTextEntryElement(activeElement);
}

function isTextEntryElement(element: Element | null): boolean {
  if (!(element instanceof HTMLElement)) {
    return false;
  }
  return Boolean(element.closest('input, textarea, select, [contenteditable="true"], .auth-panel, .hud-modal, .hud-window'));
}

function windowDefinitions(state: ClientState): HUDPanelDefinition[] {
  return baseWindowDefinitions.filter((definition) => !definition.hidden?.(state));
}

function modalDefinition(id: HUDModalID, state: ClientState): HUDModalState | null {
  const windowDefinition = windowDefinitions(state).find((definition) => definition.id === id);
  if (windowDefinition) {
    return {
      id,
      title: windowDefinition.title,
      body: windowDefinition.render(state),
    };
  }

  switch (id) {
    case 'target':
      return { id, title: 'Target Detail', body: targetPanel(state) };
    case 'planets':
      return { id, title: 'Planet Intel', body: planetsPanel(state) };
    case 'ship':
      return { id, title: 'Ship Detail', body: shipPanel(state) };
    default:
      return null;
  }
}

function statusPanel(state: ClientState): string {
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

function planetsPanel(state: ClientState): string {
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
                        <button class="planet-row" type="button" data-action="planet-detail" data-planet-id="${escapeHTML(planet.planet_id)}" data-selected="${selectedPlanet ? 'true' : 'false'}" title="Request server planet detail">
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
          : '<div class="empty-line">No server-known planets yet.</div>'
        : '<div class="empty-line">Awaiting server planet intel.</div>'
    }
    ${selected ? planetDetailBlock(selected) : '<div class="empty-line">Select a known planet for server coordinates.</div>'}
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

function economyPanel(state: ClientState): string {
  const wallet = state.wallet;
  const market = state.market;
  const auction = state.auction;
  const premium = state.premium;
  if (!wallet || !market || !auction || !premium) {
    return `
      <h2>Shop</h2>
      <div class="empty-line">Awaiting server economy snapshots.</div>
    `;
  }

  const listing = market.listings.find((item) => item.status === 'active' && !item.owned_by_you) ?? market.listings[0] ?? null;
  const lot = auction.lots.find((item) => item.status === 'active') ?? auction.lots[0] ?? null;
  const pendingEntitlement = premium.entitlements.find((item) => item.state === 'pending') ?? null;
  const stock = premium.stock[0] ?? null;
  const ownListing = market.listings.find((item) => item.status === 'active' && item.owned_by_you) ?? null;
  const sellableStack = ownListing ? null : sellableInventoryStack(state);
  const sellUnitPrice = sellableStack ? defaultListingPrice(sellableStack.item_id, market) : 0;
  const alreadyPurchased = stock ? premium.purchases.some((purchase) => purchase.period_key === stock.period_key) : false;
  const canBuyListing = Boolean(listing && listing.status === 'active' && !listing.owned_by_you && wallet.credits >= listing.unit_price);
  const bidAmount = lot ? Math.max(lot.start_price, lot.current_bid + 50) : 0;
  const canBid = Boolean(lot && lot.status === 'active' && wallet.credits >= bidAmount && !lot.leading);
  const canBuyNow = Boolean(lot && lot.status === 'active' && lot.buy_now_price !== undefined && wallet.credits >= lot.buy_now_price);
  const canBuyWeekly = Boolean(stock && stock.stock_remaining > 0 && !alreadyPurchased && wallet.premium_paid >= stock.price_amount);

  return `
    <h2>Shop</h2>
    <div class="economy-metrics">
      <span>CR<strong>${wallet.credits}</strong></span>
      <span>Paid<strong>${wallet.premium_paid}</strong></span>
      <span>Earned<strong>${wallet.premium_earned}</strong></span>
    </div>
    ${
      listing
        ? `<div class="shop-row">
             <div><span>Market</span><strong>${escapeHTML(listing.display_name || listing.item_id)}</strong></div>
             <button type="button" data-action="${listing.owned_by_you ? 'market-cancel' : 'market-buy'}" data-listing-id="${escapeHTML(listing.listing_id)}" ${listing.owned_by_you || canBuyListing ? '' : 'disabled'} title="Server recalculates price and escrow">
               ${listing.owned_by_you ? 'Cancel' : `${listing.unit_price} ${escapeHTML(listing.currency_type)}`}
             </button>
           </div>`
        : '<div class="empty-line">No market listings.</div>'
    }
    ${
      ownListing
        ? `<div class="shop-row">
             <div><span>Mine</span><strong>${escapeHTML(ownListing.display_name || ownListing.item_id)} x${ownListing.remaining_quantity}</strong><small>Escrow</small></div>
             <button type="button" data-action="market-cancel" data-listing-id="${escapeHTML(ownListing.listing_id)}" title="Return remaining escrow through the server">Cancel</button>
           </div>`
        : sellableStack
          ? `<div class="shop-row">
               <div><span>Sell</span><strong>${escapeHTML(sellableStack.display_name || sellableStack.item_id)} x1</strong><small>Ask ${sellUnitPrice} CR pending</small></div>
               <button type="button"
                 data-action="market-create"
                 data-item-id="${escapeHTML(sellableStack.item_id)}"
                 data-source-location="${escapeHTML(sellableStack.location)}"
                 data-quantity="1"
                 data-unit-price="${sellUnitPrice}"
                 title="Create listing from a server-owned inventory row">List</button>
             </div>`
          : ''
    }
    ${
      lot
        ? `<div class="shop-row">
             <div><span>Auction</span><strong>${escapeHTML(publicAuctionName(lot.payload_type, lot.definition_id))}</strong></div>
             <button type="button" data-action="auction-bid" data-auction-id="${escapeHTML(lot.auction_id)}" data-amount="${bidAmount}" ${canBid ? '' : 'disabled'} title="Bid amount is checked server-side">${bidAmount}</button>
             <button type="button" data-action="auction-buy-now" data-auction-id="${escapeHTML(lot.auction_id)}" ${canBuyNow ? '' : 'disabled'} title="Buy-now closes under server lock">${lot.buy_now_price ?? '--'}</button>
           </div>`
        : '<div class="empty-line">No auction lots.</div>'
    }
    <div class="shop-row">
      <div><span>Premium</span><strong>${stock ? `X Core ${stock.stock_remaining}/${stock.stock_total}` : 'Locked'}</strong></div>
      <button type="button" data-action="premium-weekly-xcore" ${canBuyWeekly ? '' : 'disabled'} title="Weekly stock and limit are server-owned">${stock ? `${stock.price_amount}` : '--'}</button>
      <button type="button" data-action="premium-claim" data-entitlement-id="${escapeHTML(pendingEntitlement?.entitlement_id ?? '')}" ${pendingEntitlement ? '' : 'disabled'} title="Claim pending entitlement">Claim</button>
    </div>
    ${
      auction.grants.length > 0
        ? `<button class="ghost-action ghost-action--compact" type="button" data-action="auction-claim" title="Refresh auction grant snapshots">Grants ${auction.grants.length}</button>`
        : ''
    }
  `;
}

function systemsPanel(state: ClientState): string {
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
        : '<div class="empty-line">Awaiting server systems snapshots.</div>'
    }
  `;
}

function questsPanel(state: ClientState): string {
  return `
    <h2>Quests</h2>
    ${questBlock(state)}
  `;
}

function opsPanel(state: ClientState): string {
  if (!state.auth.session?.account?.admin) {
    return `
      <h2>Ops</h2>
      <div class="empty-line">Admin session required.</div>
    `;
  }
  return `
    <h2>Ops</h2>
    ${adminOpsBlock(state)}
  `;
}

function questBlock(state: ClientState): string {
  const board = state.questBoard;
  if (!board) {
    return `
      <div class="systems-subhead">Quests</div>
      <div class="empty-line">Awaiting server quest board.</div>
    `;
  }

  const claimable = board.active.find((quest) => quest.can_claim) ?? null;
  const active = board.active.find((quest) => quest.state === 'accepted') ?? board.active[0] ?? null;
  const offer = board.offers[0] ?? null;
  const focusQuest = claimable ?? active;
  const focusObjective = focusQuest?.objectives[0] ?? offer?.objectives[0] ?? null;
  const balance = state.wallet ? walletBalanceForCurrency(state.wallet, board.reroll_cost.currency_type) : null;
  const canReroll = balance !== null && balance >= board.reroll_cost.amount;

  return `
    <div class="systems-subhead">Quests</div>
    <div class="systems-block">
      <div class="meta-row"><span>Offers</span><strong>${board.counts.offers}</strong></div>
      <div class="meta-row"><span>Active</span><strong>${board.counts.active}</strong></div>
      <div class="meta-row"><span>Claim</span><strong>${board.counts.claimable}</strong></div>
    </div>
    ${
      focusQuest
        ? `<div class="shop-row">
             <div><span>${escapeHTML(focusQuest.state || 'quest')}</span><strong>${escapeHTML(focusQuest.title)}</strong><small>${focusObjective ? escapeHTML(questObjectiveLabel(focusObjective)) : escapeHTML(questRewardLabel(focusQuest.rewards[0]))}</small></div>
             <button type="button" data-action="quest-claim" data-quest-id="${escapeHTML(focusQuest.quest_id)}" ${focusQuest.can_claim ? '' : 'disabled'}>Claim</button>
           </div>`
        : offer
          ? `<div class="shop-row">
               <div><span>${escapeHTML(offer.quest_type || 'offer')}</span><strong>${escapeHTML(offer.title)}</strong><small>${focusObjective ? escapeHTML(questObjectiveLabel(focusObjective)) : escapeHTML(questRewardLabel(offer.rewards[0]))}</small></div>
               <button type="button" data-action="quest-accept" data-offer-id="${escapeHTML(offer.offer_id)}">Accept</button>
             </div>`
          : '<div class="empty-line">No server quest offers.</div>'
    }
    <div class="shop-row">
      <div><span>Reroll</span><strong>${board.reroll_cost.amount} ${escapeHTML(board.reroll_cost.currency_type)}</strong></div>
      <button type="button" data-action="quest-reroll" ${canReroll ? '' : 'disabled'}>Roll</button>
    </div>
  `;
}

function adminOpsBlock(state: ClientState): string {
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

function targetPanel(state: ClientState): string {
  const target = state.selectedTargetID ? state.visibleEntities[state.selectedTargetID] : null;
  const actions = quickActionMap(state);
  const laser = actions.laser;
  const loot = actions.gather;
  const targetLabel = target?.display?.label ?? target?.entity_id ?? '';
  const distance = target ? distanceToTarget(state, target.entity_id) : null;
  const knownLoot = target ? state.knownLoot[target.entity_id] : null;
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
        : '<div class="empty-line">No lock</div>'
    }
    <div class="segmented">
      <button type="button" disabled title="Click a visible entity on the map to target it">Aim</button>
      <button type="button" data-action="fire" ${laser.enabled ? '' : 'disabled'} title="${escapeHTML(laser.title)}">Fire</button>
      <button type="button" data-action="loot" ${loot.enabled ? '' : 'disabled'} title="${escapeHTML(loot.title)}">${escapeHTML(loot.label)}</button>
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
  return quickActionStates(state).map(actionSlotHTML).join('');
}

function intelPanel(state: ClientState): string {
  const intel = state.planetIntel;
  const lastScan = intel?.lastScan ?? null;
  const knownPlanets = intel?.planets.slice(0, 2) ?? [];
  const routes = state.routes?.routes.length ?? null;
  const production = state.production?.planets.length ?? null;
  const scanAction = scanActionState(state);
  return `
    <h2>Sector Map</h2>
    ${minimapPanel(state)}
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
                     <button class="inline-row-action" type="button" data-action="planet-detail" data-planet-id="${escapeHTML(planet.planet_id)}" title="Request server planet detail">
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

function planetDetailBlock(planet: NonNullable<ClientState['planetIntel']>['selectedPlanet']): string {
  if (!planet) {
    return '';
  }
  return `
    <div class="planet-detail" data-planet-detail="${escapeHTML(planet.planet_id)}">
      <div class="meta-row"><span>Selected</span><strong>${escapeHTML(publicPlanetName(planet))}</strong></div>
      <div class="meta-row"><span>Coord</span><strong>${Math.round(planet.coordinates.x)}, ${Math.round(planet.coordinates.y)}</strong></div>
      <div class="meta-row"><span>Level</span><strong>${planet.level}</strong></div>
      <div class="meta-row"><span>Owner</span><strong>${escapeHTML(planet.owner_status || 'intel')}</strong></div>
      <div class="meta-row"><span>Production</span><strong>${planet.production_locked ? 'locked' : 'ready'}</strong></div>
    </div>
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
      return `<span class="minimap__point" data-kind="${escapeHTML(disposition)}" data-entity-type="${escapeHTML(contact.entity_type)}" style="left:${left}%;top:${top}%" title="${escapeHTML(publicEntityType(contact.entity_type))}"></span>`;
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
      <span data-kind="loot">Loot</span>
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

function lootStatusBlock(drop: KnownLootDropStatus): string {
  return `
    <div class="loot-status">
      <div class="meta-row"><span>Item</span><strong>${escapeHTML(drop.item_id)}</strong></div>
      <div class="meta-row"><span>Qty</span><strong>${drop.quantity}</strong></div>
      <div class="meta-row"><span>Drop</span><strong>${escapeHTML(drop.state ?? 'visible')}</strong></div>
    </div>
  `;
}

function quickActionMap(state: ClientState): Record<QuickActionID, QuickActionState> {
  return Object.fromEntries(quickActionStates(state).map((action) => [action.id, action])) as Record<QuickActionID, QuickActionState>;
}

function quickActionStates(state: ClientState): QuickActionState[] {
  const target = state.selectedTargetID ? state.visibleEntities[state.selectedTargetID] : null;
  const loot = lootActionState(state, target);
  return [
    liveQuickAction('laser', 'fire', 1, '1', laserIconURL, 'combat.use_skill', laserActionState(state, target)),
    lockedQuickAction('rocket', 'rocket', 2, '2', rocketIconURL, 'Rocket', 'Missile skills are not exposed by a server contract yet.'),
    liveQuickAction('scan', 'scan', 3, '3', scanIconURL, 'scan.pulse', scanActionState(state)),
    lockedQuickAction('shield', 'shield', 4, '4', shieldIconURL, 'Shield', 'Defensive skill contracts are not exposed yet.'),
    lockedQuickAction('warp', 'warp', 5, '5', warpIconURL, 'Warp', 'Warp route commands are not exposed by a server contract yet.'),
    liveQuickAction('gather', 'loot', 6, '6', gatherIconURL, lootCommandOp(loot), loot),
  ];
}

function liveQuickAction(
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

function lockedQuickAction(
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

function actionSlotHTML(action: QuickActionState): string {
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

function actionStateKind(action: ActionState): QuickActionState['state'] {
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

function lootCommandOp(action: ActionState): string {
  return action.label === 'Approach' ? 'move_to' : 'loot.pickup';
}

function laserActionState(state: ClientState, target: VisibleEntity | null): ActionState {
  if (!realtimeReady(state)) {
    return { enabled: false, label: 'Laser', detail: 'Offline', title: 'Realtime link is not authenticated.' };
  }
  if (state.ship?.disabled === true) {
    return { enabled: false, label: 'Laser', detail: 'Disabled', title: 'Repair the ship before firing.' };
  }
  if (!target || target.entity_type !== 'npc') {
    return { enabled: false, label: 'Laser', detail: 'No lock', title: 'Select a hostile target.' };
  }
  if (hasPendingOp(state, 'combat.use_skill')) {
    return { enabled: false, label: 'Laser', detail: 'Pending', title: 'Basic laser intent is awaiting server response.' };
  }

  const cooldownRemaining = (state.skillCooldowns.basic_laser ?? 0) - Date.now();
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
    return { enabled: false, label: 'Laser', detail: 'Stats', title: 'Awaiting server combat stats.' };
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

function scanActionState(state: ClientState): ActionState {
  const mode = state.scanMode;
  if (mode.enabled) {
    if (!realtimeReady(state)) {
      return { enabled: true, label: 'Paused', detail: 'Offline', title: 'Scanner automation is waiting for realtime. Click to stop.' };
    }
    if (state.ship?.disabled === true) {
      return { enabled: true, label: 'Paused', detail: 'Disabled', title: 'Ship is disabled. Click to stop scanner automation.' };
    }
    if (hasPendingOp(state, 'scan.pulse')) {
      return { enabled: true, label: 'Scanning', detail: 'Pending', title: 'Scanner pulse is awaiting server response. Click to stop.' };
    }
    if (state.planetIntel?.lastScan?.status === 'started') {
      return {
        enabled: true,
        label: 'Scanning',
        detail: scanModeTimeDetail(mode.nextPulseAt, 'Resolving'),
        title: 'Scanner pulse is resolving server-side. Click to stop.',
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
    return { enabled: false, label: 'Scan', detail: 'Pending', title: 'Scanner pulse is awaiting server response.' };
  }
  return {
    enabled: true,
    label: 'Scan',
    detail: state.planetIntel?.lastScan?.status ? actionScanLabel(state.planetIntel.lastScan.status) : 'Ready',
    title: 'Start automatic server scanner pulses.',
  };
}

function lootActionState(state: ClientState, target: VisibleEntity | null): ActionState {
  if (!realtimeReady(state)) {
    return { enabled: false, label: 'Gather', detail: 'Offline', title: 'Realtime link is not authenticated.' };
  }
  if (state.ship?.disabled === true) {
    return { enabled: false, label: 'Gather', detail: 'Disabled', title: 'Repair the ship before gathering drops.' };
  }
  if (!target || target.entity_type !== 'loot') {
    return { enabled: false, label: 'Gather', detail: 'No drop', title: 'Select a visible drop.' };
  }
  if (hasPendingOp(state, 'loot.pickup')) {
    return { enabled: false, label: 'Gather', detail: 'Pending', title: 'Loot pickup intent is awaiting server response.' };
  }

  const pickupRange = state.stats?.loot_pickup_range ?? 0;
  const distance = distanceToTarget(state, target.entity_id);
  if (pickupRange <= 0) {
    return { enabled: false, label: 'Gather', detail: 'No range', title: 'Awaiting server pickup range.' };
  }
  if (distance === null) {
    return { enabled: false, label: 'Gather', detail: 'No fix', title: 'Awaiting server self position.' };
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
      title: `Approach movement is awaiting server response for a drop ${Math.round(distance)}u away.`,
    };
  }
  return {
    enabled: true,
    label: 'Approach',
    detail: `${Math.round(distance)}u`,
    title: `Move toward drop, then request pickup within ${Math.round(pickupRange)}u.`,
  };
}

function hasPendingOp(state: ClientState, op: string): boolean {
  return Object.values(state.pendingCommands).some((command) => command.op === op);
}

function distanceToTarget(state: ClientState, targetID: string): number | null {
  const target = state.visibleEntities[targetID];
  const local = Object.values(state.visibleEntities).find((entity) => entity.status_flags?.includes('self'));
  if (!target || !local) {
    return null;
  }
  const targetPosition = currentEntityPosition(target);
  const localPosition = currentEntityPosition(local);
  return Math.hypot(targetPosition.x - localPosition.x, targetPosition.y - localPosition.y);
}

function currentEntityPosition(entity: VisibleEntity): { x: number; y: number } {
  const movement = entity.movement;
  if (!movement?.moving || movement.arrive_at_ms <= movement.started_at_ms) {
    return entity.position;
  }

  const progress = Math.max(
    0,
    Math.min(1, (Date.now() - movement.started_at_ms) / (movement.arrive_at_ms - movement.started_at_ms)),
  );
  return {
    x: movement.origin.x + (movement.target.x - movement.origin.x) * progress,
    y: movement.origin.y + (movement.target.y - movement.origin.y) * progress,
  };
}

function realtimeReady(state: ClientState): boolean {
  return state.auth.mode === 'demo' || state.connectionStatus === 'connected';
}

function formatCooldown(milliseconds: number): string {
  if (milliseconds < 1000) {
    return '<1s';
  }
  return `${Math.ceil(milliseconds / 1000)}s`;
}

function scanModeTimeDetail(nextPulseAt: number | null, fallback: string): string {
  if (!nextPulseAt) {
    return fallback;
  }
  const remaining = nextPulseAt - Date.now();
  if (remaining <= 0) {
    return fallback;
  }
  return formatCooldown(remaining);
}


function lockedValue(): string {
  return '--';
}

function formatPair(current?: number, max?: number): string {
  return current !== undefined && max !== undefined ? `${Math.round(current)}/${Math.round(max)}` : lockedValue();
}

function formatPercent(current?: number, max?: number): string {
  if (current === undefined || max === undefined || max <= 0) {
    return lockedValue();
  }
  return `${Math.round((Math.max(0, Math.min(current, max)) / max) * 100)}%`;
}

function formatCompactNumber(value: number): string {
  const abs = Math.abs(value);
  if (abs >= 1_000_000) {
    return `${(value / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`;
  }
  if (abs >= 10_000) {
    return `${Math.round(value / 1_000)}K`;
  }
  return String(value);
}

function normalizePanelID(value: string | undefined): HUDWindowID | null {
  return value === 'cargo' ||
    value === 'economy' ||
    value === 'quests' ||
    value === 'intel' ||
    value === 'systems' ||
    value === 'ops'
    ? value
    : null;
}

function normalizeModalID(value: string | undefined): HUDModalID | null {
  if (value === 'target' || value === 'planets' || value === 'ship') {
    return value;
  }
  return normalizePanelID(value);
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

function scanStatusLabel(status: string): string {
  switch (status) {
    case 'planet_discovered':
      return 'Discovered';
    case 'no_signal':
      return 'No signal';
    case 'started':
      return 'Scanning';
    default:
      return status || 'Scanner';
  }
}

function actionScanLabel(status: string): string {
  return status === 'planet_discovered' ? 'Found' : scanStatusLabel(status);
}

function publicPlanetName(planet: NonNullable<ClientState['planetIntel']>['planets'][number]): string {
  const type = planet.planet_type ? planet.planet_type.replace(/_/g, ' ') : 'planet';
  const biome = planet.biome ? planet.biome.replace(/_/g, ' ') : 'unknown';
  return `${type} / ${biome}`;
}

function publicAuctionName(payloadType: string, definitionID: string): string {
  const label = payloadType ? payloadType.replace(/_/g, ' ') : 'lot';
  return definitionID ? `${label} ${definitionID.replace(/_/g, ' ')}` : label;
}

function questObjectiveLabel(objective: NonNullable<ClientState['questBoard']>['active'][number]['objectives'][number]): string {
  const target = objective.target ? ` ${objective.target.replace(/_/g, ' ')}` : '';
  return `${objective.current}/${objective.required} ${objective.kind.replace(/_/g, ' ')}${target}`;
}

function questRewardLabel(reward: NonNullable<ClientState['questBoard']>['offers'][number]['rewards'][number] | undefined): string {
  if (!reward) {
    return 'reward pending';
  }
  if (reward.currency_type) {
    return `${reward.amount} ${reward.currency_type.replace(/_/g, ' ')}`;
  }
  if (reward.item_id) {
    return `${reward.amount} ${reward.item_id.replace(/_/g, ' ')}`;
  }
  if (reward.role) {
    return `${reward.amount} ${reward.role.replace(/_/g, ' ')}`;
  }
  return `${reward.amount} ${reward.kind.replace(/_/g, ' ')}`;
}

function walletBalanceForCurrency(wallet: NonNullable<ClientState['wallet']>, currency: string): number | null {
  switch (currency) {
    case 'credits':
      return wallet.credits;
    case 'premium_paid':
      return wallet.premium_paid;
    case 'premium_earned':
      return wallet.premium_earned;
    default:
      return null;
  }
}

function sellableInventoryStack(state: ClientState): NonNullable<ClientState['inventory']>['stackable'][number] | null {
  const stack =
    state.inventory?.stackable.find(
      (item) => item.quantity > 0 && (item.location === 'account_inventory' || item.location === 'ship_cargo'),
    ) ?? null;
  return stack;
}

function defaultListingPrice(itemID: string, market: NonNullable<ClientState['market']>): number {
  const matchingListing = market.listings.find(
    (listing) => listing.item_id === itemID && listing.status === 'active' && !listing.owned_by_you,
  );
  return Math.max(1, Math.round(matchingListing?.unit_price ?? 25));
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

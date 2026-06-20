import { ClientState } from '../state/types';
import { activeEntityMovement, currentEntityPosition, distanceBetween, selfEntity } from '../state/movement';
import { isWithinMinimapProjectionWindow, minimapPointPercent } from '../state/world-memory';
import { markHUDInputSuppressed, pointerTargetOwnsUI, worldKeyboardShortcutAllowed } from '../input/world-input-authority';
import { renderToast } from './toast';
import capacityIconURL from '../../../output/assets/hud-svg/icons/capacity.svg?url';
import cargoIconURL from '../../../output/assets/hud-svg/icons/cargo.svg?url';
import creditsIconURL from '../../../output/assets/hud-svg/icons/credits.svg?url';
import dangerIconURL from '../../../output/assets/hud-svg/icons/danger.svg?url';
import energyIconURL from '../../../output/assets/hud-svg/icons/energy.svg?url';
import gatherIconURL from '../../../output/assets/hud-svg/icons/gather.svg?url';
import galaxyIconURL from '../../../output/assets/hud-svg/icons/galaxy.svg?url';
import hangarIconURL from '../../../output/assets/hud-svg/icons/hangar.svg?url';
import inventoryIconURL from '../../../output/assets/hud-svg/icons/inventory.svg?url';
import laserIconURL from '../../../output/assets/hud-svg/icons/laser.svg?url';
import menuIconURL from '../../../output/assets/hud-svg/icons/menu.svg?url';
import planetsIconURL from '../../../output/assets/hud-svg/icons/planets.svg?url';
import rocketIconURL from '../../../output/assets/hud-svg/icons/rocket.svg?url';
import scanIconURL from '../../../output/assets/hud-svg/icons/scan.svg?url';
import sectorIconURL from '../../../output/assets/hud-svg/icons/sector.svg?url';
import shieldIconURL from '../../../output/assets/hud-svg/icons/shield.svg?url';
import shopIconURL from '../../../output/assets/hud-svg/icons/shop.svg?url';
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
  onStealthToggle(): void;
  onSelectTarget(entityID: string, source?: 'hud' | 'radar'): void;
  onCycleTarget(): void;
  onPlanetDetail(planetID: string): void;
  onPlanetNavigate(planetID: string): void;
  onHangarActivateShip(shipID: string): void;
  onLoadoutEquipModule(slotID: string, itemInstanceID: string): void;
  onLoadoutUnequipModule(slotID: string): void;
  onMarketCreateListing(input: {
    itemID: string;
    quantity: number;
    unitPrice: number;
    sourceLocation?: string;
    itemInstanceID?: string;
  }): void;
  onMarketBuy(listingID: string, quantity: number): void;
  onMarketCancel(listingID: string): void;
  onAuctionBid(auctionID: string, amount: number): void;
  onAuctionBuyNow(auctionID: string): void;
  onAuctionGrants(): void;
  onPremiumClaim(entitlementID: string): void;
  onPremiumWeeklyXCore(productID: string, periodKey: string): void;
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
type HUDModalID = HUDWindowID | 'target' | 'planets' | 'ship' | 'planet-detail';
type QuickActionID = 'laser' | 'rocket' | 'scan' | 'stealth' | 'warp' | 'gather';
type QuickActionCommand = 'fire' | 'rocket' | 'scan' | 'stealth' | 'warp' | 'loot';
type QuestBoardSummary = NonNullable<ClientState['questBoard']>;
type QuestOfferSummary = QuestBoardSummary['offers'][number];
type QuestSummary = QuestBoardSummary['active'][number];
type QuestEntry =
  | { key: string; kind: 'offer'; item: QuestOfferSummary }
  | { key: string; kind: 'quest'; item: QuestSummary };
type ShopCategoryID = 'market' | 'sell' | 'auction' | 'premium';
type InventoryTabID = 'equipment' | 'inventory' | 'cargo' | 'crafting';
type ModuleFilterID = 'all' | 'offensive' | 'defensive' | 'utility';
type InventoryStackItem = NonNullable<ClientState['inventory']>['stackable'][number];
type ModuleInventoryItem = NonNullable<ClientState['inventory']>['instances'][number];
type MarketListingItem = NonNullable<ClientState['market']>['listings'][number];
type AuctionLotItem = NonNullable<ClientState['auction']>['lots'][number];
type AuctionGrantItem = NonNullable<ClientState['auction']>['grants'][number];
type PremiumEntitlementItem = NonNullable<ClientState['premium']>['entitlements'][number];
type PremiumStockItem = NonNullable<ClientState['premium']>['stock'][number];
type ShopEntry =
  | { key: string; category: 'market'; kind: 'market_listing'; item: MarketListingItem }
  | { key: string; category: 'sell'; kind: 'sell_stack'; item: InventoryStackItem; unitPrice: number }
  | { key: string; category: 'sell'; kind: 'owned_listing'; item: MarketListingItem }
  | { key: string; category: 'auction'; kind: 'auction_lot'; item: AuctionLotItem }
  | { key: string; category: 'premium'; kind: 'premium_entitlement'; item: PremiumEntitlementItem }
  | { key: string; category: 'premium'; kind: 'premium_stock'; item: PremiumStockItem; purchased: boolean }
  | { key: string; category: 'premium'; kind: 'auction_grant'; item: AuctionGrantItem };

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
  iconURL: string;
  render(state: ClientState): string;
  hidden?(state: ClientState): boolean;
}

let selectedQuestKey: string | null = null;
let selectedShopCategory: ShopCategoryID = 'market';
let selectedShopKey: string | null = null;
let selectedShopQuantity = 1;
let selectedInventoryTab: InventoryTabID = 'equipment';
let selectedModuleFilter: ModuleFilterID = 'all';
let selectedModuleInstanceID: string | null = null;
let selectedHangarShipID: string | null = null;

interface HUDModalState {
  id: HUDModalID;
  title: string;
  body: string;
  detailID?: string;
}

interface HUDWindowState {
  id: HUDWindowID;
  x: number;
  y: number;
  z: number;
  open: boolean;
}

interface HUDDragState {
  target: 'window';
  id: HUDWindowID;
  pointerID: number;
  offsetX: number;
  offsetY: number;
}

interface HUDModalDragState {
  target: 'modal';
  pointerID: number;
  offsetX: number;
  offsetY: number;
}

export class HUD {
  private readonly root: HTMLElement;
  private readonly nav: HTMLElement;
  private readonly windowLayer: HTMLElement;
  private readonly modalLayer: HTMLElement;
  private readonly movementEta: HTMLElement;
  private readonly socketInput: HTMLInputElement;
  private readonly panels: Record<string, HTMLElement>;
  private readonly toast: HTMLElement;
  private readonly windowStates = new Map<HUDWindowID, HUDWindowState>();
  private dragState: HUDDragState | HUDModalDragState | null = null;
  private focusedWindow: HUDWindowID | null = null;
  private modal: HUDModalState | null = null;
  private modalPosition: { x: number; y: number } | null = null;
  private windowRenderSignature: string | null = null;
  private modalRenderSignature: string | null = null;
  private currentState: ClientState | null = null;
  private currentServerNow: number | null = null;
  private nextWindowZ = 20;
  private readonly dragMove = (event: PointerEvent) => this.handleDragMove(event);
  private readonly dragEnd = (event: PointerEvent) => this.handleDragEnd(event);
  private readonly shortcutKeyDown = (event: KeyboardEvent) => this.handleShortcutKeyDown(event);

  constructor(container: HTMLElement, private readonly handlers: HUDHandlers) {
    this.root = document.createElement('section');
    this.root.className = 'hud';
    this.root.innerHTML = `
      <header class="hud__topbar">
        <div class="top-status" aria-label="Pilot status">
          <div class="top-status__cell" data-icon="sector"><img class="top-status__icon" src="${escapeHTML(sectorIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Sector</span><strong data-top-sector>${lockedValue()}</strong></div>
          <div class="top-status__cell top-status__cell--danger" data-icon="danger"><img class="top-status__icon" src="${escapeHTML(dangerIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Danger</span><strong data-top-danger>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="energy"><img class="top-status__icon" src="${escapeHTML(energyIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Energy</span><strong data-top-energy>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="cargo"><img class="top-status__icon" src="${escapeHTML(cargoIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Cargo</span><strong data-top-cargo>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="credits"><img class="top-status__icon" src="${escapeHTML(creditsIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Credits</span><strong data-top-credits>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="cap"><img class="top-status__icon" src="${escapeHTML(capacityIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Cap</span><strong data-top-cap>${lockedValue()}</strong></div>
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
          <button class="tool-button tool-button--locked" type="button" disabled title="Mail link unavailable">Mail</button>
          <button class="tool-button tool-button--locked" type="button" disabled title="Social link unavailable">Social</button>
          <button class="tool-button" data-action="logout" type="button" title="Logout">Logout</button>
        </div>
      </header>
      <div class="hud__movement-eta" data-movement-eta></div>
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
    this.movementEta = this.root.querySelector<HTMLElement>('[data-movement-eta]')!;
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

  render(state: ClientState, serverNow: number | null = Date.now()): void {
    this.currentState = state;
    this.currentServerNow = serverNow;
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
    this.panels.target.innerHTML = targetPanel(state, serverNow);
    this.panels.ship.innerHTML = shipPanel(state);
    this.panels.intel.innerHTML = intelPanel(state);
    this.panels.actions.innerHTML = actionBar(state, serverNow);
    this.panels.log.innerHTML = logPanel(state);
    this.movementEta.innerHTML = movementEtaPanel(state, serverNow);
    this.movementEta.hidden = this.movementEta.innerHTML === '';
    this.renderNav(state);
    this.renderWindows(state);
    this.refreshModal(state);
    this.renderModal();
    renderToast(this.toast, state.lastError?.message ?? null);
  }

  openPlanetDetailModal(planetID: string): void {
    if (!planetID || !this.currentState) {
      return;
    }
    this.openModal('planet-detail', this.currentState, planetID);
    this.render(this.currentState, this.currentServerNow);
  }

  private bindEvents(): void {
    this.root.addEventListener(
      'pointerdown',
      (event) => {
        const target = event.target;
        if (pointerTargetOwnsUI(target)) {
          markHUDInputSuppressed();
        }
        const targetElement = target instanceof HTMLElement ? target : null;
        if (!targetElement) {
          return;
        }

        const modalDrag = targetElement.closest<HTMLElement>('[data-modal-drag]');
        if (modalDrag && event.button === 0 && !isControlElement(target)) {
          this.startModalDrag(event);
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
      if (pointerTargetOwnsUI(event.target)) {
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

    this.root.addEventListener('dragstart', (event) => this.handleLoadoutDragStart(event));
    this.root.addEventListener('dragend', () => this.handleLoadoutDragEnd());
    this.root.addEventListener('dragover', (event) => this.handleLoadoutDragOver(event));
    this.root.addEventListener('drop', (event) => this.handleLoadoutDrop(event));
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
      case 'stealth':
        this.handlers.onStealthToggle();
        return true;
      case 'auction-refresh':
        this.handlers.onAuctionGrants();
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
            if (this.currentState) {
              this.openModal('planet-detail', this.currentState, button.dataset.planetId);
              this.render(this.currentState);
            }
            this.handlers.onPlanetDetail(button.dataset.planetId);
          }
          break;
        case 'target-select':
          if (button.dataset.entityId) {
            const source = button.dataset.targetSource === 'radar' ? 'radar' : 'hud';
            this.handlers.onSelectTarget(button.dataset.entityId, source);
          }
          break;
        case 'loot-select':
          if (button.dataset.entityId) {
            this.handlers.onSelectTarget(button.dataset.entityId, 'radar');
          }
          break;
        case 'planet-select':
          if (button.dataset.planetId) {
            this.handlers.onPlanetDetail(button.dataset.planetId);
          }
          break;
        case 'planet-navigate':
          if (button.dataset.planetId) {
            this.handlers.onPlanetNavigate(button.dataset.planetId);
          }
          break;
        case 'hangar-activate':
          if (button.dataset.shipId) {
            this.handlers.onHangarActivateShip(button.dataset.shipId);
          }
          break;
        case 'hangar-select':
          if (button.dataset.shipId) {
            selectedHangarShipID = button.dataset.shipId;
            if (this.currentState) {
              this.render(this.currentState);
            }
          }
          break;
        case 'open-window': {
          const panel = normalizePanelID(button.dataset.panelId);
          if (panel) {
            this.openWindow(panel);
            if (this.currentState) {
              this.render(this.currentState);
            }
          }
          break;
        }
        case 'inventory-tab':
          if (isInventoryTabID(button.dataset.inventoryTab)) {
            selectedInventoryTab = button.dataset.inventoryTab;
            if (this.currentState) {
              this.render(this.currentState);
            }
          }
          break;
        case 'module-filter':
          if (isModuleFilterID(button.dataset.moduleFilter)) {
            selectedModuleFilter = button.dataset.moduleFilter;
            selectedModuleInstanceID = null;
            if (this.currentState) {
              this.render(this.currentState);
            }
          }
          break;
        case 'loadout-equip':
          if (button.dataset.slotId && button.dataset.itemInstanceId) {
            this.handlers.onLoadoutEquipModule(button.dataset.slotId, button.dataset.itemInstanceId);
          }
          break;
        case 'loadout-unequip':
          if (button.dataset.slotId) {
            this.handlers.onLoadoutUnequipModule(button.dataset.slotId);
          }
          break;
        case 'module-select':
          if (button.dataset.moduleInstanceId) {
            selectedModuleInstanceID = button.dataset.moduleInstanceId;
            if (this.currentState) {
              this.render(this.currentState);
            }
          }
          break;
        case 'quest-select':
          if (button.dataset.questKey) {
            selectedQuestKey = button.dataset.questKey;
            if (this.currentState) {
              this.render(this.currentState);
            }
          }
          break;
        case 'shop-category':
          if (isShopCategoryID(button.dataset.shopCategory)) {
            selectedShopCategory = button.dataset.shopCategory;
            selectedShopKey = null;
            selectedShopQuantity = 1;
            if (this.currentState) {
              this.render(this.currentState);
            }
          }
          break;
        case 'shop-select':
          if (button.dataset.shopKey) {
            selectedShopKey = button.dataset.shopKey;
            selectedShopQuantity = 1;
            if (this.currentState) {
              this.render(this.currentState);
            }
          }
          break;
        case 'shop-qty': {
          const maxQuantity = Math.max(1, Number(button.dataset.maxQuantity ?? '1'));
          const nextQuantity =
            button.dataset.quantity !== undefined
              ? Number(button.dataset.quantity)
              : selectedShopQuantity + Number(button.dataset.quantityDelta ?? '0');
          selectedShopQuantity = Math.round(clamp(Number.isFinite(nextQuantity) ? nextQuantity : 1, 1, maxQuantity));
          if (this.currentState) {
            this.render(this.currentState);
          }
          break;
        }
        case 'market-buy':
          if (button.dataset.listingId) {
            this.handlers.onMarketBuy(button.dataset.listingId, Math.max(1, Number(button.dataset.quantity ?? '1')));
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
        case 'premium-weekly-xcore':
          if (button.dataset.productId && button.dataset.periodKey) {
            this.handlers.onPremiumWeeklyXCore(button.dataset.productId, button.dataset.periodKey);
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

  private handleLoadoutDragStart(event: DragEvent): void {
    const target = event.target instanceof HTMLElement ? event.target : null;
    const moduleCard = target?.closest<HTMLElement>('[data-module-instance-id]');
    if (!moduleCard || !event.dataTransfer) {
      return;
    }
    const payload = {
      itemInstanceID: moduleCard.dataset.moduleInstanceId ?? '',
      slotID: moduleCard.dataset.equippedSlotId ?? '',
    };
    if (!payload.itemInstanceID) {
      return;
    }
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('application/x-space-mORPG-module', JSON.stringify(payload));
    event.dataTransfer.setData('text/plain', payload.itemInstanceID);
    moduleCard.dataset.dragging = 'true';
    markHUDInputSuppressed();
  }

  private handleLoadoutDragEnd(): void {
    for (const element of this.root.querySelectorAll<HTMLElement>('[data-module-instance-id][data-dragging="true"]')) {
      delete element.dataset.dragging;
    }
    markHUDInputSuppressed();
  }

  private handleLoadoutDragOver(event: DragEvent): void {
    const target = event.target instanceof HTMLElement ? event.target : null;
    if (!target?.closest('[data-loadout-slot-id], [data-loadout-inventory-drop]')) {
      return;
    }
    event.preventDefault();
    if (event.dataTransfer) {
      event.dataTransfer.dropEffect = 'move';
    }
    markHUDInputSuppressed();
  }

  private handleLoadoutDrop(event: DragEvent): void {
    const target = event.target instanceof HTMLElement ? event.target : null;
    if (!target || !event.dataTransfer) {
      return;
    }
    const payload = parseLoadoutDragPayload(event.dataTransfer.getData('application/x-space-mORPG-module'));
    if (!payload?.itemInstanceID) {
      return;
    }
    const slotTarget = target.closest<HTMLElement>('[data-loadout-slot-id]');
    const inventoryTarget = target.closest<HTMLElement>('[data-loadout-inventory-drop]');
    if (slotTarget?.dataset.loadoutSlotId) {
      event.preventDefault();
      event.stopPropagation();
      this.handlers.onLoadoutEquipModule(slotTarget.dataset.loadoutSlotId, payload.itemInstanceID);
      markHUDInputSuppressed();
      return;
    }
    if (inventoryTarget && payload.slotID) {
      event.preventDefault();
      event.stopPropagation();
      this.handlers.onLoadoutUnequipModule(payload.slotID);
      markHUDInputSuppressed();
    }
  }

  private handleShortcutKeyDown(event: KeyboardEvent): void {
    if (!this.currentState || event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey || event.repeat) {
      return;
    }
    if (
      !worldKeyboardShortcutAllowed({
        eventTarget: event.target,
        activeElement: document.activeElement,
        uiOwnsFocus: Boolean(this.modal || this.focusedWindow || this.dragState),
      })
    ) {
      return;
    }
    if (event.key === 'Tab') {
      event.preventDefault();
      event.stopPropagation();
      markHUDInputSuppressed();
      this.handlers.onCycleTarget();
      return;
    }
    if (!isQuickActionKey(event.key)) {
      return;
    }
    const action = quickActionStates(this.currentState, this.currentServerNow).find((entry) => entry.key === event.key);
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
        return `<button class="hud-nav-button" type="button" data-panel-toggle="${definition.id}" data-active="${active ? 'true' : 'false'}" data-focused="${focused ? 'true' : 'false'}" aria-pressed="${active ? 'true' : 'false'}"><img class="hud-nav-button__icon" src="${escapeHTML(definition.iconURL)}" alt="" aria-hidden="true" draggable="false" /><span>${escapeHTML(definition.label)}</span></button>`;
      })
      .join('');
  }

  private renderWindows(state: ClientState): void {
    if (this.dragState?.target === 'window') {
      return;
    }
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

    const html = openStates
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
    if (html === this.windowRenderSignature) {
      return;
    }
    this.windowRenderSignature = html;
    this.windowLayer.innerHTML = html;
  }

  private refreshModal(state: ClientState): void {
    if (!this.modal) {
      return;
    }
    const refreshed = modalDefinition(this.modal.id, state, this.modal.detailID);
    if (!refreshed) {
      this.closeModal();
      return;
    }
    this.modal = refreshed;
  }

  private renderModal(): void {
    if (!this.modal) {
      if (this.modalRenderSignature !== null || this.modalLayer.innerHTML !== '') {
        this.modalLayer.innerHTML = '';
      }
      this.modalLayer.dataset.open = 'false';
      this.modalRenderSignature = null;
      return;
    }

    if (this.dragState?.target === 'modal') {
      this.modalLayer.dataset.open = 'true';
      const modal = this.modalLayer.querySelector<HTMLElement>('.hud-modal');
      if (modal) {
        modal.dataset.dragging = 'true';
      }
      return;
    }

    const positioned = this.modalPosition !== null && window.innerWidth >= 768;
    const modalStyle = positioned
      ? ` style="--modal-x:${this.modalPosition?.x ?? 0}px;--modal-y:${this.modalPosition?.y ?? 0}px;--modal-transform:none"`
      : '';
    const html = `
      <div class="hud-modal-backdrop" data-modal-close="backdrop"></div>
      <section class="hud-modal" data-modal="${this.modal.id}" data-positioned="${positioned ? 'true' : 'false'}" data-dragging="false" role="dialog" aria-modal="true" aria-label="${escapeHTML(this.modal.title)}" tabindex="-1"${modalStyle}>
        <header class="hud-modal__header" data-modal-drag="true">
          <strong>${escapeHTML(this.modal.title)}</strong>
          <button type="button" data-modal-close="button" title="Close modal">Close</button>
        </header>
        <div class="hud-modal__body">${this.modal.body}</div>
      </section>
    `;
    this.modalLayer.dataset.open = 'true';
    if (html === this.modalRenderSignature) {
      return;
    }
    this.modalRenderSignature = html;
    this.modalLayer.innerHTML = html;
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

  private openModal(id: HUDModalID, state: ClientState, detailID?: string): void {
    const modal = modalDefinition(id, state, detailID);
    if (!modal) {
      return;
    }
    this.modal = modal;
    this.modalPosition = this.defaultModalPosition();
  }

  private closeModal(): void {
    this.modal = null;
    this.modalPosition = null;
    this.modalRenderSignature = null;
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
      target: 'window',
      id: panel,
      pointerID: event.pointerId,
      offsetX: event.clientX - rect.left,
      offsetY: event.clientY - rect.top,
    };
    windowPanel.dataset.dragging = 'true';
    event.preventDefault();
  }

  private startModalDrag(event: PointerEvent): void {
    if (window.innerWidth < 768) {
      return;
    }
    const modal = (event.target as HTMLElement).closest<HTMLElement>('.hud-modal');
    if (!modal || !this.modal) {
      return;
    }
    const rect = modal.getBoundingClientRect();
    this.modalPosition = this.clampModalPosition(rect.left, rect.top);
    this.dragState = {
      target: 'modal',
      pointerID: event.pointerId,
      offsetX: event.clientX - rect.left,
      offsetY: event.clientY - rect.top,
    };
    modal.dataset.dragging = 'true';
    markHUDInputSuppressed();
    event.preventDefault();
  }

  private handleDragMove(event: PointerEvent): void {
    const drag = this.dragState;
    if (!drag || drag.pointerID !== event.pointerId) {
      return;
    }
    if (drag.target === 'modal') {
      this.modalPosition = this.clampModalPosition(event.clientX - drag.offsetX, event.clientY - drag.offsetY);
      this.applyModalPosition();
      markHUDInputSuppressed();
      event.preventDefault();
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
    const drag = this.dragState;
    if (!drag || drag.pointerID !== event.pointerId) {
      return;
    }
    if (drag.target === 'modal') {
      const modal = this.modalLayer.querySelector<HTMLElement>('.hud-modal');
      if (modal) {
        delete modal.dataset.dragging;
      }
      this.dragState = null;
      markHUDInputSuppressed();
      return;
    }
    const windowPanel = this.windowLayer.querySelector<HTMLElement>(`[data-window-panel="${drag.id}"]`);
    if (windowPanel) {
      delete windowPanel.dataset.dragging;
    }
    this.dragState = null;
    markHUDInputSuppressed();
  }

  private defaultModalPosition(): { x: number; y: number } | null {
    if (window.innerWidth < 768) {
      return null;
    }
    const width = Math.min(544, Math.max(320, window.innerWidth - 16));
    const height = Math.min(544, Math.max(320, window.innerHeight - 24));
    return this.clampModalPosition((window.innerWidth - width) / 2, (window.innerHeight - height) / 2);
  }

  private clampModalPosition(x: number, y: number): { x: number; y: number } {
    const width = Math.min(544, Math.max(320, window.innerWidth - 16));
    const height = Math.min(544, Math.max(320, window.innerHeight - 24));
    const margin = 8;
    return {
      x: clamp(x, margin, Math.max(margin, window.innerWidth - width - margin)),
      y: clamp(y, margin, Math.max(margin, window.innerHeight - height - margin)),
    };
  }

  private applyModalPosition(): void {
    const modal = this.modalLayer.querySelector<HTMLElement>('.hud-modal');
    if (!modal || !this.modalPosition) {
      return;
    }
    modal.style.setProperty('--modal-x', `${this.modalPosition.x}px`);
    modal.style.setProperty('--modal-y', `${this.modalPosition.y}px`);
    modal.style.setProperty('--modal-transform', 'none');
    modal.dataset.positioned = 'true';
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
  { id: 'cargo', label: 'Inv', title: 'Inventory', iconURL: inventoryIconURL, render: cargoPanel },
  { id: 'economy', label: 'Shop', title: 'Shop', iconURL: shopIconURL, render: economyPanel },
  { id: 'quests', label: 'Quests', title: 'Quest Board', iconURL: galaxyIconURL, render: questsPanel },
  { id: 'intel', label: 'Planets', title: 'Planets', iconURL: planetsIconURL, render: planetCatalogPanel },
  { id: 'systems', label: 'Hangar', title: 'Hangar', iconURL: hangarIconURL, render: hangarPanel },
  { id: 'ops', label: 'Ops', title: 'Admin Ops', iconURL: menuIconURL, render: opsPanel, hidden: (state) => !state.auth.session?.account?.admin },
];

function windowSize(id: HUDWindowID): { width: number; height: number } {
  switch (id) {
    case 'economy':
      return { width: 640, height: 560 };
    case 'quests':
      return { width: 620, height: 560 };
    case 'intel':
      return { width: 620, height: 620 };
    case 'systems':
      return { width: 540, height: 520 };
    case 'ops':
      return { width: 450, height: 520 };
    case 'cargo':
    default:
      return { width: 560, height: 560 };
  }
}

function isControlElement(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && Boolean(target.closest('button, input, select, textarea, a[href], [data-action]'));
}

function isQuickActionKey(key: string): boolean {
  return key === '1' || key === '2' || key === '3' || key === '4' || key === '5' || key === '6';
}

function parseLoadoutDragPayload(raw: string): { itemInstanceID: string; slotID?: string } | null {
  if (!raw) {
    return null;
  }
  try {
    const parsed = JSON.parse(raw) as { itemInstanceID?: unknown; slotID?: unknown };
    if (typeof parsed.itemInstanceID !== 'string' || parsed.itemInstanceID === '') {
      return null;
    }
    return {
      itemInstanceID: parsed.itemInstanceID,
      slotID: typeof parsed.slotID === 'string' && parsed.slotID !== '' ? parsed.slotID : undefined,
    };
  } catch {
    return null;
  }
}

function windowDefinitions(state: ClientState): HUDPanelDefinition[] {
  return baseWindowDefinitions.filter((definition) => !definition.hidden?.(state));
}

function modalDefinition(id: HUDModalID, state: ClientState, detailID?: string): HUDModalState | null {
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
    case 'planet-detail': {
      const title = planetModalTitle(state, detailID);
      return { id, detailID, title, body: planetDetailModal(state, detailID) };
    }
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

function cargoPanel(state: ClientState): string {
  const inventory = state.inventory;
  const hangar = state.hangar;
  const loadout = state.loadout;
  const cargo = state.cargo;
  const wallet = state.wallet;
  if (!inventory || !hangar || !loadout || !cargo || !wallet) {
    return `
      <h2>Inventory</h2>
      <section class="inventory-system" data-inventory-system="true" data-active-inventory-tab="${selectedInventoryTab}">
        ${inventoryTabBar(selectedInventoryTab, null)}
        <div class="inventory-tab-panel" data-inventory-tab-panel="${selectedInventoryTab}">
          <div class="empty-line">Awaiting inventory, hangar, and loadout data.</div>
        </div>
      </section>
    `;
  }
  const activeShip =
    hangar.ships.find((ship) => ship.ship_id === hangar.active_ship_id) ?? hangar.ships[0] ?? null;
  const moduleItems = inventory.instances.filter((item) => item.module_slot_type && item.location !== 'ship_equipped');
  const selectedModule = selectedModuleItem(filteredModuleItems(moduleItems, selectedModuleFilter));
  const equippedCount = loadout.slots.filter((slot) => slot.item_instance_id).length;
  const tabContext = { inventory, hangar, loadout, cargo, wallet, activeShip, moduleItems, selectedModule, equippedCount };
  return `
    <h2>Inventory</h2>
    <section class="inventory-system" data-inventory-system="true" data-active-inventory-tab="${selectedInventoryTab}">
      ${inventoryTabBar(selectedInventoryTab, tabContext)}
      <div class="inventory-tab-panel" data-inventory-tab-panel="${selectedInventoryTab}">
        ${inventoryTabPanel(selectedInventoryTab, tabContext)}
      </div>
    </section>
  `;
}

type InventoryTabContext = {
  inventory: NonNullable<ClientState['inventory']>;
  hangar: NonNullable<ClientState['hangar']>;
  loadout: NonNullable<ClientState['loadout']>;
  cargo: NonNullable<ClientState['cargo']>;
  wallet: NonNullable<ClientState['wallet']>;
  activeShip: NonNullable<ClientState['hangar']>['ships'][number] | null;
  moduleItems: ModuleInventoryItem[];
  selectedModule: ModuleInventoryItem | null;
  equippedCount: number;
};

function inventoryTabBar(activeTab: InventoryTabID, context: InventoryTabContext | null): string {
  const tabCounts: Record<InventoryTabID, string> = {
    equipment: context ? `${context.equippedCount}/${context.loadout.slots.length}` : '--',
    inventory: context ? String(context.inventory.instances.length + context.inventory.stackable.length) : '--',
    cargo: context ? `${context.cargo.used}/${context.cargo.capacity}` : '--',
    crafting: 'locked',
  };
  return `
    <div class="inventory-tabs" role="tablist" aria-label="Inventory systems">
      ${inventoryTabButton('equipment', 'Equipment', tabCounts.equipment, activeTab)}
      ${inventoryTabButton('inventory', 'Inventory', tabCounts.inventory, activeTab)}
      ${inventoryTabButton('cargo', 'Cargo', tabCounts.cargo, activeTab)}
      ${inventoryTabButton('crafting', 'Crafting', tabCounts.crafting, activeTab)}
    </div>
  `;
}

function inventoryTabButton(tabID: InventoryTabID, label: string, meta: string, activeTab: InventoryTabID): string {
  const active = tabID === activeTab;
  return `
    <button
      type="button"
      role="tab"
      data-action="inventory-tab"
      data-inventory-tab="${tabID}"
      data-active="${active ? 'true' : 'false'}"
      aria-selected="${active ? 'true' : 'false'}">
      <span>${escapeHTML(label)}</span>
      <strong>${escapeHTML(meta)}</strong>
    </button>
  `;
}

function inventoryTabPanel(tabID: InventoryTabID, context: InventoryTabContext): string {
  switch (tabID) {
    case 'inventory':
      return inventoryStoredPanel(context);
    case 'cargo':
      return cargoHoldPanel(context);
    case 'crafting':
      return craftingBlockedPanel();
    case 'equipment':
    default:
      return equipmentPanel(context);
  }
}

function equipmentPanel(context: InventoryTabContext): string {
  const { activeShip, loadout, moduleItems, selectedModule, equippedCount } = context;
  return `
    <section class="loadout-console loadout-console--equipment" data-loadout-inventory-drop="true">
      <div class="loadout-ship">
        <div class="loadout-ship__core" aria-hidden="true"></div>
        <div class="loadout-ship__meta">
          <strong>${escapeHTML(activeShip?.display_name ?? (context.hangar.active_ship_id || 'Ship'))}</strong>
          <span>${equippedCount}/${loadout.slots.length} online</span>
        </div>
      </div>
      ${loadoutSlotGroups(loadout.slots)}
      ${moduleBayPanel(moduleItems, loadout.slots, selectedModule, 'Available modules')}
    </section>
  `;
}

function inventoryStoredPanel(context: InventoryTabContext): string {
  const { inventory, loadout, moduleItems, selectedModule } = context;
  const stackRows = inventory.stackable
    .map(
      (item) => `
        <li>
          <span title="${escapeHTML(publicInventoryStateLabel(item.location))}">${escapeHTML(item.display_name || item.item_id)}</span>
          <strong>${item.quantity}</strong>
        </li>
      `,
    )
    .join('');
  return `
    <section class="inventory-storage-console" data-inventory-storage="true">
      ${moduleBayPanel(moduleItems, loadout.slots, selectedModule, 'Account modules')}
      <div class="inventory-stack-panel">
        <div class="module-bay__head">
          <strong>Stored cargo items</strong>
          <span>${inventory.stackable.length} stacks</span>
        </div>
        ${
          inventory.stackable.length > 0
            ? `<ul class="compact-list inventory-stack-list">${stackRows}</ul>`
            : '<div class="empty-line">No account cargo stacks.</div>'
        }
      </div>
    </section>
  `;
}

function cargoHoldPanel(context: InventoryTabContext): string {
  const { cargo, wallet } = context;
  const cargoPercent = cargo.capacity > 0 ? Math.min(100, Math.round((cargo.used / cargo.capacity) * 100)) : 0;
  return `
    <section class="cargo-hold-console" data-cargo-tab="true">
      <div class="cargo-strip">
        <div>
          <span>Cargo</span>
          <strong>${cargo.used}/${cargo.capacity}</strong>
        </div>
        <div class="meter"><span style="width:${cargoPercent}%"></span></div>
        <div>
          <span>Credits</span>
          <strong>${wallet.credits}</strong>
        </div>
      </div>
      ${
        cargo.items.length > 0
          ? `<ul class="compact-list cargo-strip__list">
              ${cargo.items.map((item) => `<li><span title="${escapeHTML(cargoItemMeta(item))}">${escapeHTML(cargoItemLabel(item))}</span><strong>${item.quantity}</strong></li>`).join('')}
            </ul>`
          : '<div class="empty-line">Cargo hold empty.</div>'
      }
      <div class="empty-line">Cargo transfer unavailable.</div>
    </section>
  `;
}

function craftingBlockedPanel(): string {
  return `
    <section class="crafting-locked-panel" data-crafting-tab="true">
      <div class="empty-line">Crafting station unavailable.</div>
    </section>
  `;
}

function moduleBayPanel(
  moduleItems: ModuleInventoryItem[],
  slots: NonNullable<ClientState['loadout']>['slots'],
  selectedModule: ModuleInventoryItem | null,
  title: string,
): string {
  const visibleItems = filteredModuleItems(moduleItems, selectedModuleFilter);
  return `
    <div class="module-bay" data-loadout-inventory-drop="true">
      <div class="module-bay__head">
        <strong>${escapeHTML(title)}</strong>
        <span>${visibleItems.length}/${moduleItems.length} stored</span>
      </div>
      ${moduleFilterBar(moduleItems, selectedModuleFilter)}
      ${
        visibleItems.length > 0
          ? `<div class="module-grid" data-module-grid="true">${visibleItems.map((item) => moduleInventoryCard(item, slots, selectedModule?.item_instance_id ?? '')).join('')}</div>
             ${selectedModule ? moduleDetailPanel(selectedModule, slots) : ''}`
          : `<div class="empty-line">No ${escapeHTML(publicModuleSlotGroupLabel(selectedModuleFilter).toLowerCase())} modules in inventory.</div>`
      }
    </div>
  `;
}

function moduleFilterBar(moduleItems: ModuleInventoryItem[], activeFilter: ModuleFilterID): string {
  const filterCounts: Record<ModuleFilterID, number> = {
    all: moduleItems.length,
    offensive: filteredModuleItems(moduleItems, 'offensive').length,
    defensive: filteredModuleItems(moduleItems, 'defensive').length,
    utility: filteredModuleItems(moduleItems, 'utility').length,
  };
  const filterOrder: ModuleFilterID[] = ['all', 'offensive', 'defensive', 'utility'];
  return `
    <div class="module-filter-bar" data-module-filter-bar="true" role="toolbar" aria-label="Module filters">
      ${filterOrder
        .map(
          (filterID) => `
            <button
              type="button"
              data-action="module-filter"
              data-module-filter="${filterID}"
              data-active="${activeFilter === filterID ? 'true' : 'false'}"
              aria-pressed="${activeFilter === filterID ? 'true' : 'false'}">
              <span>${escapeHTML(publicModuleSlotGroupLabel(filterID))}</span>
              <strong>${filterCounts[filterID]}</strong>
            </button>
          `,
        )
        .join('')}
    </div>
  `;
}

function cargoItemLabel(item: { item_id: string; display_name?: string }): string {
  return item.display_name || item.item_id;
}

function cargoItemMeta(item: { category?: string; location?: string; used_units?: number; unit_weight?: number; locked_reason?: string }): string {
  return [
    item.category,
    item.location ? publicInventoryStateLabel(item.location) : undefined,
    item.used_units !== undefined ? `${item.used_units}u used` : undefined,
    item.unit_weight !== undefined ? `${item.unit_weight}u each` : undefined,
    item.locked_reason ? 'Move unavailable' : undefined,
  ].filter(Boolean).join(' · ');
}

function publicInventoryStateLabel(value: string): string {
  switch (value) {
    case 'account_inventory':
      return 'Stored';
    case 'ship_equipped':
      return 'Equipped';
    case 'ship_cargo':
      return 'Cargo hold';
    case 'planet_storage':
      return 'Planet storage';
    case 'station_storage':
      return 'Station storage';
    case 'market_escrow':
    case 'auction_escrow':
      return 'Escrow';
    case 'crafting_reserved':
      return 'Reserved';
    case 'world_drop':
      return 'Drop';
    default:
      return value.replace(/_/g, ' ');
  }
}

function selectedModuleItem(items: ModuleInventoryItem[]): ModuleInventoryItem | null {
  if (items.length === 0) {
    selectedModuleInstanceID = null;
    return null;
  }
  const selected = items.find((item) => item.item_instance_id === selectedModuleInstanceID) ?? items[0];
  selectedModuleInstanceID = selected.item_instance_id;
  return selected;
}

function filteredModuleItems(items: ModuleInventoryItem[], filterID: ModuleFilterID): ModuleInventoryItem[] {
  if (filterID === 'all') {
    return items;
  }
  return items.filter((item) => item.module_slot_type === filterID);
}

function loadoutSlotGroups(slots: NonNullable<ClientState['loadout']>['slots']): string {
  const slotTypes = uniqueSlotTypes(slots);
  return `
    <div class="loadout-slot-groups" data-loadout-slot-groups="true">
      ${slotTypes
        .map((slotType) => {
          const groupSlots = slots.filter((slot) => slot.slot_type === slotType);
          return `
            <section class="loadout-slot-group" data-loadout-slot-group="${escapeHTML(slotType)}" data-slot-group="${escapeHTML(slotType)}">
              <div class="loadout-slot-group__head">
                <strong>${escapeHTML(publicModuleSlotGroupLabel(slotType))}</strong>
                <span>${groupSlots.filter((slot) => slot.item_instance_id).length}/${groupSlots.length}</span>
              </div>
              <div class="loadout-slot-grid">
                ${groupSlots.map((slot) => loadoutSlotCard(slot)).join('')}
              </div>
            </section>
          `;
        })
        .join('')}
    </div>
  `;
}

function uniqueSlotTypes(slots: NonNullable<ClientState['loadout']>['slots']): string[] {
  const preferred = ['offensive', 'defensive', 'utility'];
  const discovered = [...new Set(slots.map((slot) => slot.slot_type))];
  return [
    ...preferred.filter((slotType) => discovered.includes(slotType)),
    ...discovered.filter((slotType) => !preferred.includes(slotType)),
  ];
}

function publicModuleSlotGroupLabel(slotType: string): string {
  switch (slotType) {
    case 'all':
      return 'All';
    case 'offensive':
      return 'Weapons';
    case 'defensive':
      return 'Defense';
    case 'utility':
      return 'Utility';
    default:
      return slotType.replace(/_/g, ' ');
  }
}

function loadoutSlotCard(slot: NonNullable<ClientState['loadout']>['slots'][number]): string {
  const occupied = Boolean(slot.item_instance_id);
  return `
    <div class="loadout-slot" data-loadout-slot-id="${escapeHTML(slot.slot_id)}" data-slot-type="${escapeHTML(slot.slot_type)}" data-occupied="${occupied ? 'true' : 'false'}">
      <div class="loadout-slot__label">
        <span>${escapeHTML(slot.slot_type)}</span>
        <strong>${escapeHTML(slot.slot_id.replace('_', ' '))}</strong>
      </div>
      ${
        occupied
          ? `<div class="module-card module-card--equipped"
                draggable="true"
                data-module-instance-id="${escapeHTML(slot.item_instance_id ?? '')}"
                data-equipped-slot-id="${escapeHTML(slot.slot_id)}">
               <strong>${escapeHTML(slot.display_name || slot.module_item_id || slot.item_instance_id || 'Module')}</strong>
               <span>${escapeHTML(slot.module_state || 'online')} · ${formatDurability(slot.durability, slot.durability_max)}</span>
               <button type="button" data-action="loadout-unequip" data-slot-id="${escapeHTML(slot.slot_id)}">Unequip</button>
             </div>`
          : '<div class="loadout-slot__empty">Empty</div>'
      }
    </div>
  `;
}

function moduleInventoryCard(
  item: ModuleInventoryItem,
  slots: NonNullable<ClientState['loadout']>['slots'],
  selectedID: string,
): string {
  const compatibleSlot =
    slots.find((slot) => slot.slot_type === item.module_slot_type && !slot.item_instance_id) ??
    slots.find((slot) => slot.slot_type === item.module_slot_type) ??
    null;
  const compatible = Boolean(compatibleSlot);
  return `
    <div class="module-card"
      draggable="true"
      data-module-instance-id="${escapeHTML(item.item_instance_id)}"
      data-module-slot-type="${escapeHTML(item.module_slot_type ?? '')}"
      data-compatible="${compatible ? 'true' : 'false'}"
      data-selected="${item.item_instance_id === selectedID ? 'true' : 'false'}">
      <strong>${escapeHTML(item.display_name || item.item_id)}</strong>
      <span>${escapeHTML(item.module_slot_type ?? 'module')} · ${formatDurability(item.durability_current, item.durability_max)}</span>
      <em>${escapeHTML(item.rarity || item.bound_state || 'owned')}</em>
      <button type="button"
        data-action="module-select"
        data-module-instance-id="${escapeHTML(item.item_instance_id)}">Details</button>
      <button type="button"
        data-action="loadout-equip"
        data-slot-id="${escapeHTML(compatibleSlot?.slot_id ?? '')}"
        data-item-instance-id="${escapeHTML(item.item_instance_id)}"
        ${compatibleSlot ? '' : 'disabled'}>Equip</button>
    </div>
  `;
}

function moduleDetailPanel(item: ModuleInventoryItem, slots: NonNullable<ClientState['loadout']>['slots']): string {
  const compatibleSlot =
    slots.find((slot) => slot.slot_type === item.module_slot_type && !slot.item_instance_id) ??
    slots.find((slot) => slot.slot_type === item.module_slot_type) ??
    null;
  return `
    <div class="module-detail" data-module-detail="${escapeHTML(item.item_instance_id)}">
      <div>
        <span>Selected Module</span>
        <strong>${escapeHTML(item.display_name || item.item_id)}</strong>
      </div>
      <div class="module-detail__grid">
        <span>Slot <strong>${escapeHTML(item.module_slot_type ?? 'module')}</strong></span>
        <span>Dur <strong>${formatDurability(item.durability_current, item.durability_max)}</strong></span>
        <span>State <strong>${escapeHTML(publicInventoryStateLabel(item.bound_state || item.location))}</strong></span>
        <span>Fit <strong>${escapeHTML(compatibleSlot?.slot_id ?? 'locked')}</strong></span>
      </div>
      <button type="button"
        data-action="loadout-equip"
        data-slot-id="${escapeHTML(compatibleSlot?.slot_id ?? '')}"
        data-item-instance-id="${escapeHTML(item.item_instance_id)}"
        ${compatibleSlot ? '' : 'disabled'}>Equip Selected</button>
    </div>
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
      <div class="empty-line">Awaiting economy data.</div>
    `;
  }

  const sections = shopSections(state);
  const entries = sections[selectedShopCategory];
  const selected = selectedShopEntry(entries);

  return `
    <h2>Shop</h2>
    <div class="economy-metrics">
      <span>CR<strong>${wallet.credits}</strong></span>
      <span>Paid<strong>${wallet.premium_paid}</strong></span>
      <span>Earned<strong>${wallet.premium_earned}</strong></span>
    </div>
    <section class="shop-console" data-shop-console="true">
      <div class="shop-categories" role="list" aria-label="Shop categories">
        ${shopCategoryButton('market', 'Market', sections.market.length, selectedShopCategory)}
        ${shopCategoryButton('sell', 'Sell', sections.sell.length, selectedShopCategory)}
        ${shopCategoryButton('auction', 'Auction', sections.auction.length, selectedShopCategory)}
        ${shopCategoryButton('premium', 'Premium', sections.premium.length, selectedShopCategory)}
      </div>
      <div class="shop-catalog" data-shop-category-active="${selectedShopCategory}">
        ${
          entries.length > 0
            ? entries.map((entry) => shopEntryRow(entry, selected?.key)).join('')
            : `<div class="empty-line">No ${escapeHTML(selectedShopCategory)} entries.</div>`
        }
      </div>
      <div class="shop-detail" data-shop-detail="${escapeHTML(selected?.key ?? '')}">
        ${selected ? shopDetail(selected, state) : '<div class="empty-line">Select an economy row.</div>'}
      </div>
    </section>
  `;
}

function shopSections(state: ClientState): Record<ShopCategoryID, ShopEntry[]> {
  const market = state.market;
  const auction = state.auction;
  const premium = state.premium;
  if (!market || !auction || !premium) {
    return { market: [], sell: [], auction: [], premium: [] };
  }

  const marketEntries = market.listings
    .filter((listing) => listing.status === 'active' && !listing.owned_by_you)
    .map((listing): ShopEntry => ({ key: `market:${listing.listing_id}`, category: 'market', kind: 'market_listing', item: listing }));
  const ownedListings = market.listings
    .filter((listing) => listing.status === 'active' && listing.owned_by_you)
    .map((listing): ShopEntry => ({ key: `mine:${listing.listing_id}`, category: 'sell', kind: 'owned_listing', item: listing }));
  const sellableStacks = sellableInventoryStacks(state).map(
    (item): ShopEntry => ({
      key: `sell:${item.location}:${item.item_id}`,
      category: 'sell',
      kind: 'sell_stack',
      item,
      unitPrice: defaultListingPrice(item.item_id, market),
    }),
  );
  const auctionEntries = auction.lots.map(
    (lot): ShopEntry => ({ key: `auction:${lot.auction_id}`, category: 'auction', kind: 'auction_lot', item: lot }),
  );
  const stockEntries = premium.stock.map(
    (stock): ShopEntry => ({
      key: `premium-stock:${stock.period_key}`,
      category: 'premium',
      kind: 'premium_stock',
      item: stock,
      purchased: premium.purchases.some((purchase) => purchase.period_key === stock.period_key),
    }),
  );
  const entitlementEntries = premium.entitlements.map(
    (entitlement): ShopEntry => ({
      key: `premium-entitlement:${entitlement.entitlement_id}`,
      category: 'premium',
      kind: 'premium_entitlement',
      item: entitlement,
    }),
  );
  const grantEntries = auction.grants.map(
    (grant): ShopEntry => ({ key: `auction-grant:${grant.auction_id}`, category: 'premium', kind: 'auction_grant', item: grant }),
  );

  return {
    market: marketEntries,
    sell: [...ownedListings, ...sellableStacks],
    auction: auctionEntries,
    premium: [...stockEntries, ...entitlementEntries, ...grantEntries],
  };
}

function selectedShopEntry(entries: ShopEntry[]): ShopEntry | null {
  if (entries.length === 0) {
    selectedShopKey = null;
    selectedShopQuantity = 1;
    return null;
  }
  const selected = entries.find((entry) => entry.key === selectedShopKey) ?? entries[0];
  selectedShopKey = selected.key;
  return selected;
}

function shopCategoryButton(id: ShopCategoryID, label: string, count: number, active: ShopCategoryID): string {
  return `
    <button class="shop-category" type="button" data-action="shop-category" data-shop-category="${id}" data-active="${active === id ? 'true' : 'false'}">
      <span>${escapeHTML(label)}</span>
      <strong>${count}</strong>
    </button>
  `;
}

function shopEntryRow(entry: ShopEntry, selectedKey: string | undefined): string {
  return `
    <button class="shop-entry" type="button" data-action="shop-select" data-shop-key="${escapeHTML(entry.key)}" data-selected="${entry.key === selectedKey ? 'true' : 'false'}" data-shop-kind="${escapeHTML(entry.kind)}">
      <span class="shop-entry__mark"></span>
      <span>
        <strong>${escapeHTML(shopEntryTitle(entry))}</strong>
        <em>${escapeHTML(shopEntryMeta(entry))}</em>
      </span>
      <small>${escapeHTML(shopEntryState(entry))}</small>
    </button>
  `;
}

function shopEntryTitle(entry: ShopEntry): string {
  switch (entry.kind) {
    case 'market_listing':
    case 'owned_listing':
      return entry.item.display_name || entry.item.item_id;
    case 'sell_stack':
      return entry.item.display_name || entry.item.item_id;
    case 'auction_lot':
      return publicAuctionName(entry.item.payload_type, entry.item.definition_id);
    case 'premium_entitlement':
      return entry.item.type.replace(/_/g, ' ');
    case 'premium_stock':
      return `Weekly ${entry.item.period_key.replace(/_/g, ' ')}`;
    case 'auction_grant':
      return publicAuctionName(entry.item.payload_type, entry.item.definition_id);
  }
}

function shopEntryMeta(entry: ShopEntry): string {
  switch (entry.kind) {
    case 'market_listing':
      return `${entry.item.remaining_quantity} left / ${entry.item.unit_price} ${entry.item.currency_type}`;
    case 'owned_listing':
      return `${entry.item.remaining_quantity} escrowed / ${entry.item.unit_price} ${entry.item.currency_type}`;
    case 'sell_stack':
      return `${entry.item.quantity} stored / ask ${entry.unitPrice} credits`;
    case 'auction_lot':
      return `${entry.item.quantity} lot / bid ${entry.item.current_bid}`;
    case 'premium_entitlement':
      return entry.item.state;
    case 'premium_stock':
      return `${entry.item.stock_remaining}/${entry.item.stock_total} stock`;
    case 'auction_grant':
      return `${entry.item.quantity} grant / ${entry.item.reason}`;
  }
}

function shopEntryState(entry: ShopEntry): string {
  switch (entry.kind) {
    case 'market_listing':
      return entry.item.rarity || entry.item.status || 'market';
    case 'owned_listing':
      return 'mine';
    case 'sell_stack':
      return entry.item.location.replace(/_/g, ' ');
    case 'auction_lot':
      return entry.item.leading ? 'leading' : entry.item.status || 'auction';
    case 'premium_entitlement':
      return entry.item.state;
    case 'premium_stock':
      return entry.purchased ? 'bought' : entry.item.payment_currency;
    case 'auction_grant':
      return 'grant';
  }
}

function shopDetail(entry: ShopEntry, state: ClientState): string {
  switch (entry.kind) {
    case 'market_listing':
      return shopMarketDetail(entry.item, state.wallet);
    case 'owned_listing':
      return shopOwnedListingDetail(entry.item);
    case 'sell_stack':
      return shopSellDetail(entry.item, entry.unitPrice);
    case 'auction_lot':
      return shopAuctionDetail(entry.item, state.wallet);
    case 'premium_entitlement':
      return shopPremiumEntitlementDetail(entry.item);
    case 'premium_stock':
      return shopPremiumStockDetail(entry.item, entry.purchased, state.wallet);
    case 'auction_grant':
      return shopAuctionGrantDetail(entry.item);
  }
}

function shopMarketDetail(listing: MarketListingItem, wallet: ClientState['wallet']): string {
  const balance = wallet ? walletBalanceForCurrency(wallet, listing.currency_type) : null;
  const affordable = balance !== null && listing.unit_price > 0 ? Math.floor(balance / listing.unit_price) : listing.remaining_quantity;
  const maxQuantity = Math.max(1, Math.min(listing.remaining_quantity, Math.max(0, affordable)));
  const quantity = normalizeShopQuantity(maxQuantity);
  const estimatedSubtotal = listing.unit_price * quantity;
  const canBuy = listing.status === 'active' && !listing.owned_by_you && listing.remaining_quantity >= quantity && balance !== null && balance >= estimatedSubtotal;
  return `
    <article class="shop-detail-card" data-shop-detail-kind="market">
      ${shopDetailHeader('Market Listing', listing.display_name || listing.item_id, listing.rarity || listing.status)}
      <div class="shop-detail-grid">
        ${shopFact('Stock', `${listing.remaining_quantity}`)}
        ${shopFact('Unit', `${listing.unit_price} ${listing.currency_type}`)}
        ${shopFact('Estimate', `${estimatedSubtotal} ${listing.currency_type}`)}
        ${shopFact('Quote', listing.final_price_pending ? 'finalized on buy' : 'ready')}
      </div>
      ${shopQuantityControls(maxQuantity, quantity)}
      <button class="shop-primary-action" type="button" data-action="market-buy" data-listing-id="${escapeHTML(listing.listing_id)}" data-quantity="${quantity}" ${canBuy ? '' : 'disabled'} title="Buy the selected quantity">Buy</button>
    </article>
  `;
}

function shopOwnedListingDetail(listing: MarketListingItem): string {
  return `
    <article class="shop-detail-card" data-shop-detail-kind="owned-listing">
      ${shopDetailHeader('Your Listing', listing.display_name || listing.item_id, listing.status)}
      <div class="shop-detail-grid">
        ${shopFact('Escrow', `${listing.remaining_quantity}`)}
        ${shopFact('Unit', `${listing.unit_price} ${listing.currency_type}`)}
        ${shopFact('State', listing.final_price_pending ? 'escrow held' : 'quoted')}
        ${shopFact('Listing', listing.listing_id)}
      </div>
      <button class="shop-primary-action" type="button" data-action="market-cancel" data-listing-id="${escapeHTML(listing.listing_id)}" title="Cancel this listing">Cancel</button>
    </article>
  `;
}

function shopSellDetail(item: InventoryStackItem, unitPrice: number): string {
  const maxQuantity = Math.max(1, item.quantity);
  const quantity = normalizeShopQuantity(maxQuantity);
  return `
    <article class="shop-detail-card" data-shop-detail-kind="sell">
      ${shopDetailHeader('Sell Item', item.display_name || item.item_id, item.location.replace(/_/g, ' '))}
      <div class="shop-detail-grid">
        ${shopFact('Owned', `${item.quantity}`)}
        ${shopFact('Ask', `${unitPrice} credits`)}
        ${shopFact('Estimate', `${unitPrice * quantity} credits pending`)}
        ${shopFact('Escrow', 'held on listing')}
      </div>
      ${shopQuantityControls(maxQuantity, quantity)}
      <button class="shop-primary-action" type="button"
        data-action="market-create"
        data-item-id="${escapeHTML(item.item_id)}"
        data-source-location="${escapeHTML(item.location)}"
        data-quantity="${quantity}"
        data-unit-price="${unitPrice}"
        title="Create a listing from this inventory row">List</button>
    </article>
  `;
}

function shopAuctionDetail(lot: AuctionLotItem, wallet: ClientState['wallet']): string {
  const bidAmount = Math.max(lot.start_price, lot.current_bid + 50);
  const credits = wallet?.credits ?? 0;
  const canBid = lot.status === 'active' && credits >= bidAmount && !lot.leading;
  const canBuyNow = lot.status === 'active' && lot.buy_now_price !== undefined && credits >= lot.buy_now_price;
  return `
    <article class="shop-detail-card" data-shop-detail-kind="auction">
      ${shopDetailHeader('Auction Lot', publicAuctionName(lot.payload_type, lot.definition_id), lot.status)}
      <div class="shop-detail-grid">
        ${shopFact('Qty', `${lot.quantity}`)}
        ${shopFact('Bid', `${lot.current_bid} ${lot.currency_type}`)}
        ${shopFact('Next', `${bidAmount} ${lot.currency_type}`)}
        ${shopFact('Buy Now', lot.buy_now_price !== undefined ? `${lot.buy_now_price} ${lot.currency_type}` : lockedValue())}
      </div>
      <div class="shop-action-row">
        <button type="button" data-action="auction-bid" data-auction-id="${escapeHTML(lot.auction_id)}" data-amount="${bidAmount}" ${canBid ? '' : 'disabled'} title="Place the next bid">Bid</button>
        <button type="button" data-action="auction-buy-now" data-auction-id="${escapeHTML(lot.auction_id)}" ${canBuyNow ? '' : 'disabled'} title="Buy this lot now">Buy Now</button>
      </div>
    </article>
  `;
}

function shopPremiumEntitlementDetail(entitlement: PremiumEntitlementItem): string {
  const amount = entitlement.payload.amount ?? entitlement.payload.loadout_slot_count ?? 0;
  return `
    <article class="shop-detail-card" data-shop-detail-kind="premium-entitlement">
      ${shopDetailHeader('Entitlement', entitlement.type.replace(/_/g, ' '), entitlement.state)}
      <div class="shop-detail-grid">
        ${shopFact('State', entitlement.state)}
        ${shopFact('Amount', amount > 0 ? `${amount}` : lockedValue())}
        ${shopFact('Bucket', entitlement.payload.currency_bucket ?? lockedValue())}
        ${shopFact('Created', entitlement.created_at ? `${entitlement.created_at}` : lockedValue())}
      </div>
      <button class="shop-primary-action" type="button" data-action="premium-claim" data-entitlement-id="${escapeHTML(entitlement.entitlement_id)}" ${entitlement.state === 'pending' ? '' : 'disabled'} title="Claim pending entitlement">Claim</button>
    </article>
  `;
}

function shopPremiumStockDetail(stock: PremiumStockItem, purchased: boolean, wallet: ClientState['wallet']): string {
  const balance = walletBalanceForCurrency(wallet ?? { credits: 0, premium_paid: 0, premium_earned: 0 }, stock.payment_currency);
  const canBuy = stock.stock_remaining > 0 && !purchased && balance !== null && balance >= stock.price_amount;
  return `
    <article class="shop-detail-card" data-shop-detail-kind="premium-stock">
      ${shopDetailHeader('Premium Stock', `Weekly ${stock.period_key.replace(/_/g, ' ')}`, purchased ? 'bought' : stock.payment_currency)}
      <div class="shop-detail-grid">
        ${shopFact('Stock', `${stock.stock_remaining}/${stock.stock_total}`)}
        ${shopFact('Price', `${stock.price_amount} ${stock.payment_currency}`)}
        ${shopFact('Status', purchased ? 'claimed' : 'available')}
        ${shopFact('Window', purchased ? 'claimed' : 'weekly')}
      </div>
      <button class="shop-primary-action" type="button"
        data-action="premium-weekly-xcore"
        data-product-id="weekly_xcore"
        data-period-key="${escapeHTML(stock.period_key)}"
        ${canBuy ? '' : 'disabled'}
        title="Purchase this weekly stock">Purchase</button>
    </article>
  `;
}

function shopAuctionGrantDetail(grant: AuctionGrantItem): string {
  return `
    <article class="shop-detail-card" data-shop-detail-kind="auction-grant">
      ${shopDetailHeader('Auction Grant', publicAuctionName(grant.payload_type, grant.definition_id), grant.reason)}
      <div class="shop-detail-grid">
        ${shopFact('Qty', `${grant.quantity}`)}
        ${shopFact('Reason', grant.reason)}
        ${shopFact('Granted', `${grant.granted_at}`)}
        ${shopFact('Auction', grant.auction_id)}
      </div>
      <button class="shop-primary-action" type="button" data-action="auction-refresh" title="Refresh auction grants">Refresh</button>
    </article>
  `;
}

function shopDetailHeader(kind: string, title: string, state: string): string {
  return `
    <div class="shop-detail-card__head">
      <span>${escapeHTML(kind)}</span>
      <strong>${escapeHTML(title)}</strong>
      <em>${escapeHTML(state || 'owned')}</em>
    </div>
  `;
}

function shopFact(label: string, value: string): string {
  return `<div class="shop-fact"><span>${escapeHTML(label)}</span><strong>${escapeHTML(value)}</strong></div>`;
}

function shopQuantityControls(maxQuantity: number, quantity: number): string {
  const safeMax = Math.max(1, Math.round(maxQuantity));
  const options = uniqueNumbers([1, Math.min(5, safeMax), safeMax]);
  return `
    <div class="shop-quantity" data-shop-quantity="true">
      <span>Qty</span>
      <button type="button" data-action="shop-qty" data-quantity-delta="-1" data-max-quantity="${safeMax}" ${quantity <= 1 ? 'disabled' : ''}>-</button>
      ${options
        .map(
          (option) =>
            `<button type="button" data-action="shop-qty" data-quantity="${option}" data-max-quantity="${safeMax}" data-active="${option === quantity ? 'true' : 'false'}">${option}</button>`,
        )
        .join('')}
      <button type="button" data-action="shop-qty" data-quantity-delta="1" data-max-quantity="${safeMax}" ${quantity >= safeMax ? 'disabled' : ''}>+</button>
    </div>
  `;
}

function normalizeShopQuantity(maxQuantity: number): number {
  selectedShopQuantity = Math.round(clamp(selectedShopQuantity, 1, Math.max(1, maxQuantity)));
  return selectedShopQuantity;
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
        : '<div class="empty-line">Awaiting systems data.</div>'
    }
  `;
}

function hangarPanel(state: ClientState): string {
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
  const selectedShip = hangar.ships.find((item) => item.ship_id === selectedHangarShipID) ?? activeShip;
  selectedHangarShipID = selectedShip?.ship_id ?? null;
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

function hangarPrimaryAction(
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

function hangarShipRow(ship: NonNullable<ClientState['hangar']>['ships'][number], activeShipID: string, selectedShipID: string): string {
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

function hangarStat(label: string, value: number | undefined, max: number | undefined): string {
  const safeValue = Math.max(0, Math.round(value ?? 0));
  const safeMax = Math.max(0, Math.round(max ?? 0));
  return `<div class="hangar-stat"><span>${escapeHTML(label)}</span><strong>${safeValue}/${safeMax}</strong></div>`;
}

function hangarScalar(label: string, value: number | undefined): string {
  return `<div class="hangar-stat"><span>${escapeHTML(label)}</span><strong>${Math.max(0, Math.round(value ?? 0))}</strong></div>`;
}

function questsPanel(state: ClientState): string {
  return `
    <h2>Quests</h2>
    ${questBoardPanel(state)}
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

function questBoardPanel(state: ClientState): string {
  const board = state.questBoard;
  if (!board) {
    return `
      <div class="systems-subhead">Quest Board</div>
      <div class="empty-line">Awaiting quest board.</div>
    `;
  }

  const sections = questBoardSections(board);
  const entries = [...sections.claimable, ...sections.active, ...sections.offers, ...sections.completed];
  const selected = selectedQuestEntry(entries);
  const balance = state.wallet ? walletBalanceForCurrency(state.wallet, board.reroll_cost.currency_type) : null;
  const canReroll = balance !== null && balance >= board.reroll_cost.amount;

  return `
    <section class="quest-board" data-quest-board="true">
      <div class="quest-tabs" role="list" aria-label="Quest categories">
        ${questTab('Offers', board.counts.offers)}
        ${questTab('Active', board.counts.active)}
        ${questTab('Claimable', board.counts.claimable)}
        ${questTab('Completed', board.counts.completed + board.counts.claimed)}
      </div>
      <div class="quest-board__list">
        ${questSection('Claimable', sections.claimable, selected?.key)}
        ${questSection('Active', sections.active, selected?.key)}
        ${questSection('Offers', sections.offers, selected?.key)}
        ${questSection('Completed', sections.completed, selected?.key)}
      </div>
      <div class="quest-board__detail" data-quest-detail="${escapeHTML(selected?.key ?? '')}">
        ${selected ? questDetail(selected) : '<div class="empty-line">No quest entries.</div>'}
        <div class="quest-reroll">
          <div>
            <span>Reroll Cost</span>
            <strong>${board.reroll_cost.amount} ${escapeHTML(board.reroll_cost.currency_type.replace(/_/g, ' '))}</strong>
          </div>
          <button type="button" data-action="quest-reroll" ${canReroll ? '' : 'disabled'} title="${canReroll ? 'Reroll the quest board' : 'Insufficient wallet balance'}">Reroll</button>
        </div>
      </div>
    </section>
  `;
}

function questBoardSections(board: QuestBoardSummary): {
  offers: QuestEntry[];
  active: QuestEntry[];
  claimable: QuestEntry[];
  completed: QuestEntry[];
} {
  const offers = board.offers.map((offer) => questEntryForOffer(offer));
  const claimable = board.active.filter((quest) => quest.can_claim).map((quest) => questEntryForQuest(quest));
  const active = board.active
    .filter((quest) => !quest.can_claim && (quest.state === 'accepted' || (!quest.completed_at && !quest.claimed_at)))
    .map((quest) => questEntryForQuest(quest));
  const completed = board.active
    .filter((quest) => !quest.can_claim && (quest.state === 'completed' || quest.state === 'claimed' || quest.completed_at || quest.claimed_at))
    .map((quest) => questEntryForQuest(quest));
  return { offers, active, claimable, completed };
}

function questEntryForOffer(offer: QuestOfferSummary): QuestEntry {
  return { key: `offer:${offer.offer_id}`, kind: 'offer', item: offer };
}

function questEntryForQuest(quest: QuestSummary): QuestEntry {
  return { key: `quest:${quest.quest_id}`, kind: 'quest', item: quest };
}

function selectedQuestEntry(entries: QuestEntry[]): QuestEntry | null {
  if (entries.length === 0) {
    selectedQuestKey = null;
    return null;
  }
  const selected = entries.find((entry) => entry.key === selectedQuestKey) ?? entries[0];
  selectedQuestKey = selected.key;
  return selected;
}

function questTab(label: string, count: number): string {
  return `
    <span class="quest-tab" role="listitem">
      <em>${escapeHTML(label)}</em>
      <strong>${count}</strong>
    </span>
  `;
}

function questSection(label: string, entries: QuestEntry[], selectedKey: string | undefined): string {
  return `
    <section class="quest-section" data-quest-section="${escapeHTML(label.toLowerCase())}">
      <header><strong>${escapeHTML(label)}</strong><span>${entries.length}</span></header>
      ${
        entries.length > 0
          ? entries.map((entry) => questRow(entry, selectedKey)).join('')
          : `<div class="quest-section__empty">No ${escapeHTML(label.toLowerCase())} quests.</div>`
      }
    </section>
  `;
}

function questRow(entry: QuestEntry, selectedKey: string | undefined): string {
  const item = entry.item;
  const objective = item.objectives[0];
  const selected = entry.key === selectedKey;
  const state = entry.kind === 'offer' ? 'offer' : entry.item.can_claim ? 'claim' : entry.item.state || 'active';
  return `
    <button class="quest-row" type="button" data-action="quest-select" data-quest-key="${escapeHTML(entry.key)}" data-selected="${selected ? 'true' : 'false'}" data-quest-state="${escapeHTML(state)}">
      <span class="quest-row__pip"></span>
      <span>
        <strong>${escapeHTML(item.title)}</strong>
        <em>${escapeHTML(objective ? questObjectiveLabel(objective) : questRewardLabel(item.rewards[0]))}</em>
      </span>
      <small>${escapeHTML(entry.kind === 'offer' ? item.quest_type : questStatusLabel(entry.item))}</small>
    </button>
  `;
}

function questDetail(entry: QuestEntry): string {
  const item = entry.item;
  const objectives = item.objectives.length > 0 ? item.objectives : [];
  const rewards = item.rewards.length > 0 ? item.rewards : [];
  return `
    <article class="quest-detail-card">
      <div class="quest-detail-card__head">
        <span>${escapeHTML(entry.kind === 'offer' ? item.quest_type : questStatusLabel(entry.item))}</span>
        <strong>${escapeHTML(item.title)}</strong>
        <p>${escapeHTML(item.description || 'No description.')}</p>
      </div>
      <div class="quest-detail-card__grid">
        <section>
          <h3>Objectives</h3>
          ${
            objectives.length > 0
              ? objectives.map((objective) => questObjectiveRow(objective)).join('')
              : '<div class="empty-line">No objectives.</div>'
          }
        </section>
        <section>
          <h3>Rewards</h3>
          ${
            rewards.length > 0
              ? rewards.map((reward) => `<div class="quest-reward-row"><span>${escapeHTML(reward.kind.replace(/_/g, ' '))}</span><strong>${escapeHTML(questRewardLabel(reward))}</strong></div>`).join('')
              : '<div class="empty-line">No rewards.</div>'
          }
        </section>
      </div>
      <div class="quest-actions">
        ${questActionButton(entry)}
      </div>
    </article>
  `;
}

function questObjectiveRow(objective: QuestOfferSummary['objectives'][number]): string {
  const progress = objective.required > 0 ? clamp(objective.current / objective.required, 0, 1) : 0;
  return `
    <div class="quest-objective-row" data-complete="${objective.completed ? 'true' : 'false'}">
      <div>
        <span>${escapeHTML(objective.kind.replace(/_/g, ' '))}</span>
        <strong>${escapeHTML(questObjectiveLabel(objective))}</strong>
      </div>
      <div class="quest-progress" aria-hidden="true"><span style="width:${Math.round(progress * 100)}%"></span></div>
    </div>
  `;
}

function questActionButton(entry: QuestEntry): string {
  if (entry.kind === 'offer') {
    return `<button type="button" data-action="quest-accept" data-offer-id="${escapeHTML(entry.item.offer_id)}">Accept</button>`;
  }
  const claimable = entry.item.can_claim;
  return `<button type="button" data-action="quest-claim" data-quest-id="${escapeHTML(entry.item.quest_id)}" ${claimable ? '' : 'disabled'}>${claimable ? 'Claim' : 'Claim Locked'}</button>`;
}

function questStatusLabel(quest: QuestSummary): string {
  if (quest.can_claim) {
    return 'claimable';
  }
  if (quest.claimed_at || quest.state === 'claimed') {
    return 'claimed';
  }
  if (quest.completed_at || quest.state === 'completed') {
    return 'completed';
  }
  return quest.state || 'active';
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

function targetPanel(state: ClientState, serverNow: number | null = Date.now()): string {
  const target = state.selectedTargetID ? state.visibleEntities[state.selectedTargetID] : null;
  const actions = quickActionMap(state, serverNow);
  const laser = actions.laser;
  const loot = actions.gather;
  const targetLabel = target?.display?.label ?? target?.entity_id ?? '';
  const distance = target ? distanceToTarget(state, target.entity_id, serverNow) : null;
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
      <div class="empty-line">Awaiting ship data.</div>
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

function actionBar(state: ClientState, serverNow: number | null): string {
  return quickActionStates(state, serverNow).map(actionSlotHTML).join('');
}

function movementEtaPanel(state: ClientState, serverNow: number | null): string {
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

function planetCatalogPanel(state: ClientState): string {
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
                 <button type="button" disabled title="Claim controls unlock in a later planet phase">Claim</button>
                 <button type="button" disabled title="Building controls unlock in a later planet phase">Build</button>
                 <button type="button" disabled title="Upgrade controls unlock in a later planet phase">Upgrade</button>
                 <button type="button" disabled title="Route controls unlock in a later planet phase">Route</button>
                 <button type="button" disabled title="Automation controls unlock in a later planet phase">Auto</button>
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

function planetCatalogRow(planet: NonNullable<ClientState['planetIntel']>['planets'][number], selectedPlanetID: string): string {
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

function planetProductionFor(
  state: ClientState,
  planetID: string,
  detailProduction?: NonNullable<ClientState['production']>['planets'][number],
): NonNullable<ClientState['production']>['planets'][number] | null {
  return detailProduction ?? state.production?.planets.find((planet) => planet.planet_id === planetID) ?? null;
}

function planetRoutesFor(
  state: ClientState,
  planetID: string,
  detailRoutes?: NonNullable<ClientState['routes']>['routes'],
): NonNullable<ClientState['routes']>['routes'] {
  if (detailRoutes && detailRoutes.length > 0) {
    return detailRoutes;
  }
  return state.routes?.routes.filter((route) => route.source_planet_id === planetID) ?? [];
}

function planetDetailModal(state: ClientState, planetID?: string): string {
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
        <button type="button" disabled title="Claim controls unlock in a later planet phase">Claim</button>
        <button type="button" disabled title="Building controls unlock in a later planet phase">Build</button>
        <button type="button" disabled title="Route controls unlock in a later planet phase">Route</button>
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

function planetModalTitle(state: ClientState, planetID?: string): string {
  const intel = state.planetIntel;
  const selected = intel?.selectedPlanet ?? null;
  const summary =
    (selected && (!planetID || selected.planet_id === planetID) ? selected : null) ??
    intel?.planets.find((planet) => planet.planet_id === planetID) ??
    null;
  return summary ? `Planet: ${publicPlanetName(summary)}` : 'Planet Detail';
}

function minimapPanel(state: ClientState): string {
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
      const action = contact.entity_type === 'loot' ? 'loot-select' : 'target-select';
      return `<button class="minimap__point" type="button" data-action="${action}" data-target-source="radar" data-kind="${escapeHTML(disposition)}" data-entity-id="${escapeHTML(contact.entity_id)}" data-entity-type="${escapeHTML(contact.entity_type)}" style="left:${point.left}%;top:${point.top}%" title="${escapeHTML(publicEntityType(contact.entity_type))}"></button>`;
    })
    .join('');
  const memoryPoints = memories
    .filter((memory) => isWithinMinimapProjectionWindow(center, memory.position, projectionHalfExtent))
    .map((memory) => {
      const point = minimapPointPercent(center, memory.position, radius);
      if (!point) {
        return '';
      }
      const planetID = memory.detail_id || memory.planet_id || '';
      const action = planetID ? ' data-action="planet-detail"' : '';
      const planet = planetID ? ` data-planet-id="${escapeHTML(planetID)}"` : '';
      const disabled = planetID ? '' : ' disabled';
      return `<button class="minimap__memory" type="button"${action}${planet}${disabled} data-kind="${escapeHTML(memory.kind)}" data-freshness="${escapeHTML(memory.freshness)}" style="left:${point.left}%;top:${point.top}%" title="${escapeHTML(memory.label || memory.kind)}"></button>`;
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

function quickActionMap(state: ClientState, serverNow: number | null): Record<QuickActionID, QuickActionState> {
  return Object.fromEntries(quickActionStates(state, serverNow).map((action) => [action.id, action])) as Record<QuickActionID, QuickActionState>;
}

function quickActionStates(state: ClientState, serverNow: number | null): QuickActionState[] {
  const target = state.selectedTargetID ? state.visibleEntities[state.selectedTargetID] : null;
  const loot = lootActionState(state, target, serverNow);
  return [
    liveQuickAction('laser', 'fire', 1, '1', laserIconURL, 'combat.use_skill', laserActionState(state, target)),
    lockedQuickAction('rocket', 'rocket', 2, '2', rocketIconURL, 'Rocket', 'Missile systems are not installed yet.'),
    liveQuickAction('scan', 'scan', 3, '3', scanIconURL, 'scan.pulse', scanActionState(state)),
    liveQuickAction('stealth', 'stealth', 4, '4', shieldIconURL, 'stealth.toggle', stealthActionState(state)),
    lockedQuickAction('warp', 'warp', 5, '5', warpIconURL, 'Warp', 'Warp drive is not installed yet.'),
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
    return { enabled: false, label: 'Laser', detail: 'Pending', title: 'Basic laser is pending.' };
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

function stealthActionState(state: ClientState): ActionState {
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

function lootActionState(state: ClientState, target: VisibleEntity | null, serverNow: number | null): ActionState {
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

function hasPendingOp(state: ClientState, op: string): boolean {
  return Object.values(state.pendingCommands).some((command) => command.op === op);
}

function distanceToTarget(state: ClientState, targetID: string, serverNow: number | null): number | null {
  const target = state.visibleEntities[targetID];
  const local = selfEntity(state.visibleEntities);
  if (!target || !local) {
    return null;
  }
  const now = serverNow ?? Date.now();
  return distanceBetween(currentEntityPosition(local, now), currentEntityPosition(target, now));
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

function formatDuration(milliseconds: number): string {
  if (milliseconds < 1000) {
    return '<1s';
  }
  return `${(milliseconds / 1000).toFixed(milliseconds < 10_000 ? 1 : 0)}s`;
}

function formatVec(position: { x: number; y: number }): string {
  return `${Math.round(position.x)},${Math.round(position.y)}`;
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

function formatDurability(current?: number, max?: number): string {
  if (current === undefined || max === undefined || max <= 0) {
    return lockedValue();
  }
  return `${Math.max(0, Math.round(current))}/${Math.round(max)}`;
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

function isShopCategoryID(value: string | undefined): value is ShopCategoryID {
  return value === 'market' || value === 'sell' || value === 'auction' || value === 'premium';
}

function isInventoryTabID(value: string | undefined): value is InventoryTabID {
  return value === 'equipment' || value === 'inventory' || value === 'cargo' || value === 'crafting';
}

function isModuleFilterID(value: string | undefined): value is ModuleFilterID {
  return value === 'all' || value === 'offensive' || value === 'defensive' || value === 'utility';
}

function normalizeModalID(value: string | undefined): HUDModalID | null {
  if (value === 'target' || value === 'planets' || value === 'ship' || value === 'planet-detail') {
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

function uniqueNumbers(values: number[]): number[] {
  return [...new Set(values.map((value) => Math.max(1, Math.round(value))))].sort((left, right) => left - right);
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
  return sellableInventoryStacks(state)[0] ?? null;
}

function sellableInventoryStacks(state: ClientState): NonNullable<ClientState['inventory']>['stackable'] {
  return (
    state.inventory?.stackable.filter(
      (item) => item.quantity > 0 && (item.location === 'account_inventory' || item.location === 'ship_cargo'),
    ) ?? []
  );
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

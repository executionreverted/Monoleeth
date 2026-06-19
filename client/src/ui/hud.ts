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
type HUDWindowID = 'cargo' | 'economy' | 'quests' | 'intel' | 'systems' | 'ops';
type HUDModalID = HUDWindowID | 'target' | 'planets' | 'ship';

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

export class HUD {
  private readonly root: HTMLElement;
  private readonly nav: HTMLElement;
  private readonly windowLayer: HTMLElement;
  private readonly modalLayer: HTMLElement;
  private readonly socketInput: HTMLInputElement;
  private readonly panels: Record<string, HTMLElement>;
  private readonly toast: HTMLElement;
  private openWindows: HUDWindowID[] = [];
  private focusedWindow: HUDWindowID | null = null;
  private modal: HUDModalState | null = null;
  private currentState: ClientState | null = null;

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
    this.root.addEventListener('click', (event) => {
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
          this.focusWindow(panel);
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
        case 'auction-claim':
          this.handlers.onAuctionClaimGrant();
          break;
        case 'premium-claim':
          if (button.dataset.entitlementId) {
            this.handlers.onPremiumClaim(button.dataset.entitlementId);
          }
          break;
        case 'premium-weekly-xcore':
          this.handlers.onPremiumWeeklyXCore();
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
        case 'quest-reroll':
          this.handlers.onQuestReroll();
          break;
        case 'admin-refresh':
          this.handlers.onAdminRefresh();
          break;
        case 'admin-repair-craft-job':
          if (button.dataset.jobId) {
            this.handlers.onAdminRepairCraftJob(button.dataset.jobId);
          }
          break;
      }
    });

    this.root.addEventListener('keydown', (event) => {
      if (event.key !== 'Escape' || !this.modal || !this.currentState) {
        return;
      }
      this.closeModal();
      this.render(this.currentState);
    });
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
        const active = this.openWindows.includes(definition.id);
        const focused = this.focusedWindow === definition.id;
        return `<button class="hud-nav-button" type="button" data-panel-toggle="${definition.id}" data-active="${active ? 'true' : 'false'}" data-focused="${focused ? 'true' : 'false'}" aria-pressed="${active ? 'true' : 'false'}"><span>${escapeHTML(definition.label)}</span></button>`;
      })
      .join('');
  }

  private renderWindows(state: ClientState): void {
    const definitions = windowDefinitions(state);
    this.openWindows = this.openWindows.filter((id) => definitions.some((definition) => definition.id === id));
    if (this.focusedWindow && !this.openWindows.includes(this.focusedWindow)) {
      this.focusedWindow = this.openWindows.at(-1) ?? null;
    }

    this.windowLayer.innerHTML = this.openWindows
      .map((id, index) => {
        const definition = definitions.find((entry) => entry.id === id);
        if (!definition) {
          return '';
        }
        const focused = id === this.focusedWindow;
        return `
          <section class="hud-window" data-window-panel="${definition.id}" data-focused="${focused ? 'true' : 'false'}" data-open="true" style="--window-index:${index}" tabindex="-1" aria-label="${escapeHTML(definition.title)}">
            <header class="hud-window__header">
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
    if (this.openWindows.includes(panel)) {
      this.closeWindow(panel);
      return;
    }
    this.openWindows = [...this.openWindows, panel].slice(-3);
    this.focusWindow(panel);
  }

  private closeWindow(panel: HUDWindowID): void {
    this.openWindows = this.openWindows.filter((id) => id !== panel);
    if (this.focusedWindow === panel) {
      this.focusedWindow = this.openWindows.at(-1) ?? null;
    }
  }

  private closeFocusedWindow(): void {
    if (!this.focusedWindow) {
      return;
    }
    this.closeWindow(this.focusedWindow);
  }

  private focusWindow(panel: HUDWindowID): void {
    if (!this.openWindows.includes(panel)) {
      return;
    }
    this.focusedWindow = panel;
    this.openWindows = [...this.openWindows.filter((id) => id !== panel), panel];
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
}

const baseWindowDefinitions: HUDPanelDefinition[] = [
  { id: 'cargo', label: 'Inv', title: 'Inventory / Cargo', render: cargoPanel },
  { id: 'economy', label: 'Shop', title: 'Market / Auction / Premium', render: economyPanel },
  { id: 'quests', label: 'Galaxy', title: 'Quest Board', render: questsPanel },
  { id: 'intel', label: 'Planets', title: 'Intel / Scanner', render: intelPanel },
  { id: 'systems', label: 'Hangar', title: 'Systems / Loadout / Crafting', render: systemsPanel },
  { id: 'ops', label: 'Ops', title: 'Admin Ops', render: opsPanel, hidden: (state) => !state.auth.session?.account?.admin },
];

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
  return `
    <h2>Planets</h2>
    ${
      intel
        ? planets.length > 0
          ? `<ul class="planet-stack">
               ${planets
                 .map(
                   (planet) =>
                     `<li>
                        <span class="planet-orb" aria-hidden="true"></span>
                        <div><strong>${escapeHTML(publicPlanetName(planet))}</strong><small>${escapeHTML(planet.rarity || planet.intel_state || 'known')}</small></div>
                        <em>${escapeHTML(planet.owner_status || 'intel')}</em>
                      </li>`,
                 )
                 .join('')}
             </ul>`
          : '<div class="empty-line">No server-known planets yet.</div>'
        : '<div class="empty-line">Awaiting server planet intel.</div>'
    }
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
  const canScan = state.connectionStatus === 'connected' && !shipDisabled;
  const laserLabel = laserCooling ? 'Cooling' : 'Laser';

  return `
    <div class="action-slot" data-slot="1">
      <button class="action-button" type="button" data-action="fire" ${canLaser ? '' : 'disabled'} title="Basic laser">
        <span>${escapeHTML(laserLabel)}</span>
        <small>${target?.entity_type === 'npc' ? 'Ready' : 'No lock'}</small>
      </button>
    </div>
    <div class="action-slot" data-slot="2">
      <button class="action-button" type="button" disabled title="Missile skills are not exposed yet">
        <span>Rocket</span>
        <small>Locked</small>
      </button>
    </div>
    <div class="action-slot" data-slot="3">
      <button class="action-button" type="button" data-action="scan" ${canScan ? '' : 'disabled'} title="Run scanner pulse">
        <span>Scan</span>
        <small>${state.planetIntel?.lastScan?.status ? actionScanLabel(state.planetIntel.lastScan.status) : 'Pulse'}</small>
      </button>
    </div>
    <div class="action-slot" data-slot="4">
      <button class="action-button" type="button" disabled title="Shield skills are not exposed yet">
        <span>Shield</span>
        <small>Locked</small>
      </button>
    </div>
    <div class="action-slot" data-slot="5">
      <button class="action-button" type="button" disabled title="Warp route commands are not exposed yet">
        <span>Warp</span>
        <small>Locked</small>
      </button>
    </div>
    <div class="action-slot" data-slot="6">
      <button class="action-button" type="button" data-action="loot" ${canLoot ? '' : 'disabled'} title="Pickup selected visible drop">
        <span>Gather</span>
        <small>${target?.entity_type === 'loot' ? 'Visible' : 'No drop'}</small>
      </button>
    </div>
  `;
}

function intelPanel(state: ClientState): string {
  const intel = state.planetIntel;
  const lastScan = intel?.lastScan ?? null;
  const knownPlanets = intel?.planets.slice(0, 2) ?? [];
  const routes = state.routes?.routes.length ?? null;
  const production = state.production?.planets.length ?? null;
  const scanDisabled = state.connectionStatus !== 'connected' || state.ship?.disabled === true;
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
    <button class="ghost-action" type="button" data-action="scan" ${scanDisabled ? 'disabled' : ''} title="Run server scanner pulse">Pulse</button>
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
                   `<li><span>${escapeHTML(publicPlanetName(planet))}</span><strong>${escapeHTML(planet.owner_status || 'intel')}</strong></li>`,
               )
               .join('')}
           </ul>`
        : ''
    }
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

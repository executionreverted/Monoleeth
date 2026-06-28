import type { ClientState } from '../state/types';
import { markHUDInputSuppressed, pointerTargetOwnsUI, worldKeyboardShortcutAllowed } from '../input/world-input-authority';
import { renderToast } from './toast';
import { hudSelection } from './hud-selection';
import { collectHUDPanels, hudShellHTML } from './hud-render-shell';
import { cargoPanel } from './hud-render-inventory';
import { economyPanel } from './hud-render-economy';
import { questsPanel } from './hud-render-quests';
import { planetsPanel } from './hud-render-planets';
import { topbarDangerText, topbarLocationText } from './hud-topbar';
import { actionBar, baseWindowDefinitions, intelPanel, logPanel, modalDefinition, movementEtaPanel, opsPanel, quickActionStates, shipPanel, statusPanel, systemsPanel, targetPanel, windowDefinitions, windowLayout } from './hud-render-panels';
import { adminContentEditPatchFromForm, buildAdminContentDraftUpdate, findAdminContentDraftRow } from './hud-render-admin-content';
import type { HUDDragState, HUDHandlers, HUDModalDragState, HUDModalID, HUDModalState, HUDPanelDefinition, HUDResizeState, HUDWindowID, HUDWindowState } from './hud-types';
import { clamp, escapeHTML, formatCompactNumber, formatPair, formatPercent, isControlElement, isInventoryTabID, isModuleFilterID, isQuickActionKey, isShopCategoryID, isSocialTabID, normalizeModalID, normalizePanelID, parseLoadoutDragPayload } from './hud-formatters';
import { dispatchPlanetRouteButtonAction } from './hud-planet-route-actions';

export type { HUDHandlers } from './hud-types';

export class HUD {
  private readonly root: HTMLElement;
  private readonly nav: HTMLElement;
  private readonly windowLayer: HTMLElement;
  private readonly modalLayer: HTMLElement;
  private readonly moduleTooltip: HTMLElement;
  private readonly movementEta: HTMLElement;
  private readonly panels: Record<string, HTMLElement>;
  private readonly toast: HTMLElement;
  private readonly windowStates = new Map<HUDWindowID, HUDWindowState>();
  private dragState: HUDDragState | HUDModalDragState | HUDResizeState | null = null;
  private focusedWindow: HUDWindowID | null = null;
  private modal: HUDModalState | null = null;
  private modalReturnFocus: { element: HTMLElement | null; selector: string | null } | null = null;
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
    this.root.innerHTML = hudShellHTML();

    container.appendChild(this.root);
    this.nav = this.root.querySelector<HTMLElement>('[data-hud-nav]')!;
    this.windowLayer = this.root.querySelector<HTMLElement>('[data-window-layer]')!;
    this.modalLayer = this.root.querySelector<HTMLElement>('[data-modal-layer]')!;
    this.moduleTooltip = this.root.querySelector<HTMLElement>('[data-module-tooltip-layer]')!;
    this.movementEta = this.root.querySelector<HTMLElement>('[data-movement-eta]')!;
    this.toast = this.root.querySelector<HTMLElement>('.toast')!;
    this.panels = collectHUDPanels(this.root);

    this.bindEvents();
  }

  render(state: ClientState, serverNow: number | null = Date.now()): void {
    this.currentState = state;
    this.currentServerNow = serverNow;
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
      sector.textContent = topbarLocationText(state);
    }
    if (danger) {
      danger.textContent = topbarDangerText(state);
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
    this.panels.cargo.innerHTML = cargoPanel(state, serverNow);
    this.panels.economy.innerHTML = economyPanel(state);
    this.panels.systems.innerHTML = systemsPanel(state);
    this.panels.quests.innerHTML = questsPanel(state);
    this.panels.ops.innerHTML = opsPanel(state);
    this.panels.planets.innerHTML = planetsPanel(state);
    this.panels.target.innerHTML = targetPanel(state, serverNow);
    this.panels.ship.innerHTML = shipPanel(state);
    this.panels.intel.innerHTML = intelPanel(state, serverNow);
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

  private rerenderCurrent(): void {
    if (this.currentState) {
      this.render(this.currentState, this.currentServerNow);
    }
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

        const resizeHandle = targetElement.closest<HTMLElement>('[data-window-resize]');
        if (resizeHandle && event.button === 0) {
          const panel = normalizePanelID(resizeHandle.dataset.windowResize);
          if (panel) {
            this.startResize(panel, event);
          }
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
    window.addEventListener('resize', () => this.handleViewportResize());

    this.root.addEventListener('pointerover', (event) => this.handleModuleTooltipPointerOver(event));
    this.root.addEventListener('pointerout', (event) => this.handleModuleTooltipPointerOut(event));
    this.root.addEventListener('focusin', (event) => this.handleModuleTooltipFocus(event));
    this.root.addEventListener('focusout', (event) => this.handleModuleTooltipBlur(event));
    this.root.addEventListener('scroll', () => this.hideModuleTooltip(), { capture: true });

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
          this.openModal(panel, this.currentState, modalOpen.dataset.helpTopic, modalOpen);
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
        this.restoreModalFocus();
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
      this.restoreModalFocus();
    });

    this.root.addEventListener('dragstart', (event) => this.handleLoadoutDragStart(event));
    this.root.addEventListener('dragend', () => this.handleLoadoutDragEnd());
    this.root.addEventListener('dragover', (event) => this.handleLoadoutDragOver(event));
    this.root.addEventListener('drop', (event) => this.handleLoadoutDrop(event));
  }

  private handleModuleTooltipPointerOver(event: PointerEvent): void {
    const trigger = this.moduleTooltipTrigger(event.target);
    if (!trigger) {
      return;
    }
    const related = event.relatedTarget instanceof Node ? event.relatedTarget : null;
    if (related && trigger.contains(related)) {
      return;
    }
    this.showModuleTooltip(trigger);
  }

  private handleModuleTooltipPointerOut(event: PointerEvent): void {
    const trigger = this.moduleTooltipTrigger(event.target);
    if (!trigger) {
      return;
    }
    const related = event.relatedTarget instanceof Node ? event.relatedTarget : null;
    if (related && trigger.contains(related)) {
      return;
    }
    this.hideModuleTooltip();
  }

  private handleModuleTooltipFocus(event: FocusEvent): void {
    const trigger = this.moduleTooltipTrigger(event.target);
    if (trigger) {
      this.showModuleTooltip(trigger);
    }
  }

  private handleModuleTooltipBlur(event: FocusEvent): void {
    const trigger = this.moduleTooltipTrigger(event.target);
    if (!trigger) {
      return;
    }
    const related = event.relatedTarget instanceof Node ? event.relatedTarget : null;
    if (related && trigger.contains(related)) {
      return;
    }
    this.hideModuleTooltip();
  }

  private moduleTooltipTrigger(target: EventTarget | null): HTMLElement | null {
    if (!(target instanceof HTMLElement)) {
      return null;
    }
    return target.closest<HTMLElement>('[data-module-tooltip="true"]');
  }

  private showModuleTooltip(trigger: HTMLElement): void {
    const source = trigger.querySelector<HTMLElement>('.module-hover-card');
    const tooltipHTML = source?.innerHTML.trim();
    if (!tooltipHTML) {
      this.hideModuleTooltip();
      return;
    }
    this.clearModuleTooltipDescriptions();
    trigger.setAttribute('aria-describedby', this.moduleTooltip.id);
    this.moduleTooltip.innerHTML = tooltipHTML;
    this.moduleTooltip.style.left = '-9999px';
    this.moduleTooltip.style.top = '-9999px';
    this.moduleTooltip.dataset.open = 'true';
    this.moduleTooltip.removeAttribute('aria-hidden');
    this.positionModuleTooltip(trigger);
  }

  private positionModuleTooltip(trigger: HTMLElement): void {
    const triggerRect = trigger.getBoundingClientRect();
    const tooltipRect = this.moduleTooltip.getBoundingClientRect();
    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;
    const margin = 8;
    const gap = 8;
    const tooltipWidth = Math.min(tooltipRect.width || 208, Math.max(160, viewportWidth - margin * 2));
    const tooltipHeight = Math.min(tooltipRect.height || 96, Math.max(80, viewportHeight - margin * 2));
    const leftSpace = triggerRect.left - margin;
    const rightSpace = viewportWidth - triggerRect.right - margin;
    const topSpace = triggerRect.top - margin;
    const bottomSpace = viewportHeight - triggerRect.bottom - margin;
    const fitsRight = rightSpace >= tooltipWidth + gap;
    const fitsLeft = leftSpace >= tooltipWidth + gap;
    const fitsBottom = bottomSpace >= tooltipHeight + gap;
    const fitsTop = topSpace >= tooltipHeight + gap;
    let side: 'left' | 'right' | 'top' | 'bottom' = rightSpace >= leftSpace ? 'right' : 'left';
    if (side === 'right' && !fitsRight && fitsLeft) {
      side = 'left';
    } else if (side === 'left' && !fitsLeft && fitsRight) {
      side = 'right';
    } else if (!fitsRight && !fitsLeft) {
      side = bottomSpace >= topSpace ? 'bottom' : 'top';
      if (side === 'bottom' && !fitsBottom && fitsTop) {
        side = 'top';
      } else if (side === 'top' && !fitsTop && fitsBottom) {
        side = 'bottom';
      }
    }

    let x = 0;
    let y = 0;
    if (side === 'right') {
      x = triggerRect.right + gap;
      y = triggerRect.top + triggerRect.height / 2 - tooltipHeight / 2;
    } else if (side === 'left') {
      x = triggerRect.left - tooltipWidth - gap;
      y = triggerRect.top + triggerRect.height / 2 - tooltipHeight / 2;
    } else if (side === 'bottom') {
      x = triggerRect.left + triggerRect.width / 2 - tooltipWidth / 2;
      y = triggerRect.bottom + gap;
    } else {
      x = triggerRect.left + triggerRect.width / 2 - tooltipWidth / 2;
      y = triggerRect.top - tooltipHeight - gap;
    }

    this.moduleTooltip.dataset.side = side;
    this.moduleTooltip.style.left = `${clamp(x, margin, viewportWidth - tooltipWidth - margin)}px`;
    this.moduleTooltip.style.top = `${clamp(y, margin, viewportHeight - tooltipHeight - margin)}px`;
  }

  private hideModuleTooltip(): void {
    delete this.moduleTooltip.dataset.open;
    delete this.moduleTooltip.dataset.side;
    this.moduleTooltip.setAttribute('aria-hidden', 'true');
    this.clearModuleTooltipDescriptions();
  }

  private clearModuleTooltipDescriptions(): void {
    for (const element of this.root.querySelectorAll<HTMLElement>('[aria-describedby="module-floating-tooltip"]')) {
      element.removeAttribute('aria-describedby');
    }
  }

  private dispatchAction(action: string | undefined): boolean {
    switch (action) {
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
      case 'admin-content-refresh':
        this.handlers.onAdminContentRefresh();
        return true;
      case 'admin-content-validate':
        this.handlers.onAdminContentValidate();
        return true;
      case 'admin-content-publish':
        this.handlers.onAdminContentPublish();
        return true;
      case 'admin-content-audit':
        this.handlers.onAdminContentAudit();
        return true;
      default:
        return false;
    }
  }

  private dispatchButtonAction(button: HTMLButtonElement): void {
    if (this.dispatchAction(button.dataset.action)) {
      return;
    }
    if (dispatchPlanetRouteButtonAction(button, this.handlers, () => this.rerenderCurrent())) {
      return;
    }
    switch (button.dataset.action) {
      case 'planet-detail':
        if (button.dataset.planetId) {
          if (this.currentState) {
            this.openModal('planet-detail', this.currentState, button.dataset.planetId, button);
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
        case 'portal-select':
          if (button.dataset.portalId && button.dataset.portalScope) {
            hudSelection.selectedPortalID = button.dataset.portalId;
            hudSelection.selectedPortalScope = button.dataset.portalScope;
            this.rerenderCurrent();
          }
          break;
        case 'portal-enter':
          if (
            button.dataset.portalId &&
            button.dataset.portalScope &&
            button.dataset.portalId === hudSelection.selectedPortalID &&
            button.dataset.portalScope === hudSelection.selectedPortalScope
          ) {
            this.handlers.onPortalEnter(button.dataset.portalId);
          }
          break;
        case 'coordinate-item-use':
          if (button.dataset.itemInstanceId) {
            this.handlers.onCoordinateItemUse(button.dataset.itemInstanceId);
          }
          break;
        case 'hangar-activate':
          if (button.dataset.shipId) {
            this.handlers.onHangarActivateShip(button.dataset.shipId);
          }
          break;
        case 'hangar-select':
          if (button.dataset.shipId) {
            hudSelection.selectedHangarShipID = button.dataset.shipId;
            this.rerenderCurrent();
          }
          break;
        case 'open-window': {
          const panel = normalizePanelID(button.dataset.panelId);
          if (panel) {
            this.openWindow(panel);
            this.rerenderCurrent();
          }
          break;
        }
        case 'social-chat-send': {
          const kind = this.socialFieldValue('chat-kind');
          const content = this.socialFieldValue('chat-content');
          if ((kind === 'local_map' || kind === 'party' || kind === 'clan') && content) {
            this.handlers.onChatSend(kind, content);
          }
          break;
        }
        case 'party-invite': {
          const callsign = this.socialFieldValue('party-invite');
          if (callsign) {
            this.handlers.onPartyInvite(callsign);
          }
          break;
        }
        case 'party-accept':
          if (button.dataset.inviteId) {
            this.handlers.onPartyAccept(button.dataset.inviteId);
          }
          break;
        case 'party-leave':
          this.handlers.onPartyLeave();
          break;
        case 'party-target-set':
          if (this.currentState?.selectedTargetID) {
            this.handlers.onPartyTargetSet(this.currentState.selectedTargetID);
          }
          break;
        case 'clan-create': {
          const name = this.socialFieldValue('clan-name');
          const tag = this.socialFieldValue('clan-tag');
          if (name && tag) {
            this.handlers.onClanCreate(name, tag);
          }
          break;
        }
        case 'clan-join': {
          const tag = this.socialFieldValue('clan-join-tag');
          if (tag) {
            this.handlers.onClanJoin(tag);
          }
          break;
        }
        case 'clan-leave':
          this.handlers.onClanLeave();
          break;
        case 'admin-content-select':
          if (button.dataset.contentType && button.dataset.contentId) {
            hudSelection.selectedAdminContentType = button.dataset.contentType;
            hudSelection.selectedAdminContentID = button.dataset.contentId;
            this.rerenderCurrent();
          }
          break;
        case 'admin-content-type-select':
          if (button.dataset.contentType) {
            hudSelection.selectedAdminContentType = button.dataset.contentType;
            hudSelection.selectedAdminContentID = null;
            this.rerenderCurrent();
          }
          break;
        case 'admin-content-edit':
          if (button.dataset.contentId && this.currentState) {
            hudSelection.selectedAdminContentType = button.dataset.contentType ?? hudSelection.selectedAdminContentType;
            hudSelection.selectedAdminContentID = button.dataset.contentId;
            this.openModal('admin-content-module-edit', this.currentState, button.dataset.contentId, button);
            this.render(this.currentState);
          }
          break;
        case 'admin-content-module-edit':
          if (button.dataset.contentId && this.currentState) {
            hudSelection.selectedAdminContentType = button.dataset.contentType ?? 'module';
            hudSelection.selectedAdminContentID = button.dataset.contentId;
            this.openModal('admin-content-module-edit', this.currentState, button.dataset.contentId, button);
            this.render(this.currentState);
          }
          break;
        case 'admin-content-save':
        case 'admin-content-module-save':
          this.handleAdminContentSave(button);
          break;
        case 'admin-content-rollback':
          if (button.dataset.versionId) {
            this.handlers.onAdminContentRollback(button.dataset.versionId);
          }
          break;
        case 'inventory-tab':
          if (isInventoryTabID(button.dataset.inventoryTab)) {
            hudSelection.selectedInventoryTab = button.dataset.inventoryTab;
            this.rerenderCurrent();
          }
          break;
        case 'module-filter':
          if (isModuleFilterID(button.dataset.moduleFilter)) {
            hudSelection.selectedModuleFilter = button.dataset.moduleFilter;
            hudSelection.selectedModuleInstanceID = null;
            this.rerenderCurrent();
          }
          break;
        case 'social-tab':
          if (isSocialTabID(button.dataset.socialTab)) {
            hudSelection.selectedSocialTab = button.dataset.socialTab;
            this.rerenderCurrent();
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
            hudSelection.selectedModuleInstanceID = button.dataset.moduleInstanceId;
            this.rerenderCurrent();
          }
          break;
        case 'quest-select':
          if (button.dataset.questKey) {
            hudSelection.selectedQuestKey = button.dataset.questKey;
            this.rerenderCurrent();
          }
          break;
        case 'shop-category':
          if (isShopCategoryID(button.dataset.shopCategory)) {
            hudSelection.selectedShopCategory = button.dataset.shopCategory;
            hudSelection.selectedShopKey = null;
            hudSelection.selectedShopQuantity = 1;
            this.rerenderCurrent();
          }
          break;
        case 'shop-select':
          if (button.dataset.shopKey) {
            hudSelection.selectedShopKey = button.dataset.shopKey;
            hudSelection.selectedShopQuantity = 1;
            this.rerenderCurrent();
          }
          break;
        case 'shop-qty': {
          const maxQuantity = Math.max(1, Number(button.dataset.maxQuantity ?? '1'));
          const nextQuantity =
            button.dataset.quantity !== undefined
              ? Number(button.dataset.quantity)
              : hudSelection.selectedShopQuantity + Number(button.dataset.quantityDelta ?? '0');
          hudSelection.selectedShopQuantity = Math.round(clamp(Number.isFinite(nextQuantity) ? nextQuantity : 1, 1, maxQuantity));
          this.rerenderCurrent();
          break;
        }
        case 'shop-buy-product':
          if (button.dataset.productId) {
            this.handlers.onShopBuyProduct(button.dataset.productId, Math.max(1, Number(button.dataset.quantity ?? '1')));
          }
          break;
        case 'market-buy':
          if (button.dataset.listingId) {
            this.handlers.onMarketBuy(button.dataset.listingId, Math.max(1, Number(button.dataset.quantity ?? '1')));
          }
          break;
        case 'market-create':
          if (button.dataset.itemId) {
            const matchingStack = this.currentState?.inventory?.stackable.find(
              (item) =>
                item.item_id === button.dataset.itemId &&
                item.location === (button.dataset.sourceLocation ?? '') &&
                item.list_eligible === true &&
                item.quantity > 0,
            );
            if (!matchingStack) {
              break;
            }
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

  private handleAdminContentSave(button: HTMLButtonElement): void {
    if (!this.currentState?.auth.session?.account?.admin) {
      return;
    }
    const form = button.closest<HTMLFormElement>('form[data-admin-content-form="true"], form[data-admin-content-module-form="true"]');
    if (!form) {
      return;
    }
    if (!form.reportValidity()) {
      return;
    }
    const row = findAdminContentDraftRow(this.currentState, form.dataset.contentType ?? hudSelection.selectedAdminContentType, form.dataset.contentId ?? null);
    if (!row) {
      return;
    }
    this.handlers.onAdminContentUpdateDraft(buildAdminContentDraftUpdate(row, adminContentEditPatchFromForm(form)));
    this.closeModal();
    if (this.currentState) {
      this.render(this.currentState);
    }
  }

  private socialFieldValue(name: string): string {
    const value = this.root.querySelector<HTMLInputElement | HTMLSelectElement>(`[data-social-field="${HUD.cssAttributeValue(name)}"]`)?.value;
    return value?.trim() ?? '';
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
      moduleSlotType: moduleCard.dataset.moduleSlotType ?? '',
    };
    if (!payload.itemInstanceID) {
      return;
    }
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('application/x-space-mORPG-module', JSON.stringify(payload));
    event.dataTransfer.setData('text/plain', payload.itemInstanceID);
    moduleCard.dataset.dragging = 'true';
    this.markLoadoutDropTargets(payload);
    this.hideModuleTooltip();
    markHUDInputSuppressed();
  }

  private handleLoadoutDragEnd(): void {
    for (const element of this.root.querySelectorAll<HTMLElement>('[data-module-instance-id][data-dragging="true"]')) {
      delete element.dataset.dragging;
    }
    this.clearLoadoutDropTargets();
    markHUDInputSuppressed();
  }

  private handleLoadoutDragOver(event: DragEvent): void {
    const target = event.target instanceof HTMLElement ? event.target : null;
    this.clearLoadoutDropHover();
    const slotTarget = target?.closest<HTMLElement>('[data-loadout-slot-id]');
    const inventoryTarget = target?.closest<HTMLElement>('[data-loadout-inventory-drop]');
    if (!slotTarget && !inventoryTarget) {
      return;
    }
    if (slotTarget?.dataset.dropState === 'compatible') {
      slotTarget.dataset.dropHover = 'true';
    } else if (inventoryTarget?.dataset.dropState === 'compatible') {
      inventoryTarget.dataset.dropHover = 'true';
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
      this.clearLoadoutDropTargets();
      this.handlers.onLoadoutEquipModule(slotTarget.dataset.loadoutSlotId, payload.itemInstanceID);
      markHUDInputSuppressed();
      return;
    }
    if (inventoryTarget && payload.slotID) {
      event.preventDefault();
      event.stopPropagation();
      this.clearLoadoutDropTargets();
      this.handlers.onLoadoutUnequipModule(payload.slotID);
      markHUDInputSuppressed();
    }
  }

  private markLoadoutDropTargets(payload: { itemInstanceID: string; slotID?: string; moduleSlotType?: string }): void {
    this.clearLoadoutDropTargets();
    const moduleSlotType = payload.moduleSlotType ?? '';
    for (const slot of this.root.querySelectorAll<HTMLElement>('[data-loadout-slot-id]')) {
      slot.dataset.dropState = moduleSlotType && slot.dataset.slotType === moduleSlotType ? 'compatible' : 'blocked';
    }
    for (const bay of this.root.querySelectorAll<HTMLElement>('[data-loadout-inventory-drop]')) {
      if (payload.slotID) {
        bay.dataset.dropState = 'compatible';
      }
    }
  }

  private clearLoadoutDropTargets(): void {
    for (const element of this.root.querySelectorAll<HTMLElement>('[data-drop-state], [data-drop-hover]')) {
      delete element.dataset.dropState;
      delete element.dataset.dropHover;
    }
  }

  private clearLoadoutDropHover(): void {
    for (const element of this.root.querySelectorAll<HTMLElement>('[data-drop-hover]')) {
      delete element.dataset.dropHover;
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
    if (this.dragState?.target === 'window' || this.dragState?.target === 'window-resize') {
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
        const layout = windowLayout(definition.id);
        const helpButton = definition.helpTopic
          ? `<button class="hud-window__help-button" type="button" data-modal-open="tutorial" data-help-topic="${escapeHTML(definition.helpTopic)}" title="${escapeHTML(definition.title)} help" aria-label="${escapeHTML(definition.title)} help">?</button>`
          : '';
        return `
          <section class="hud-window" data-window-panel="${definition.id}" data-window-size="${layout.size}" data-focused="${focused ? 'true' : 'false'}" data-open="true" data-x="${Math.round(windowState.x)}" data-y="${Math.round(windowState.y)}" data-width="${Math.round(windowState.width)}" data-height="${Math.round(windowState.height)}" style="--window-x:${windowState.x}px;--window-y:${windowState.y}px;--window-z:${windowState.z};--window-width:${windowState.width}px;--window-height:${windowState.height}px" tabindex="-1" aria-label="${escapeHTML(definition.title)}">
            <header class="hud-window__header" data-window-drag="${definition.id}">
              <strong>${escapeHTML(definition.title)}</strong>
              <div>
                ${helpButton}
                <button type="button" data-panel-close="${definition.id}" title="Close panel">Close</button>
              </div>
            </header>
            <div class="hud-window__body">${definition.render(state)}</div>
            <button class="hud-window__resize" type="button" data-window-resize="${definition.id}" title="Resize panel" aria-label="Resize ${escapeHTML(definition.title)}"></button>
          </section>
        `;
      })
      .join('');
    if (html === this.windowRenderSignature) {
      return;
    }
    this.hideModuleTooltip();
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
      state = { id: panel, ...this.defaultWindowPlacement(panel), z: ++this.nextWindowZ, open: true };
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

  private openModal(id: HUDModalID, state: ClientState, detailID?: string, returnFocus?: HTMLElement | null): void {
    const modal = modalDefinition(id, state, detailID);
    if (!modal) {
      return;
    }
    this.modal = modal;
    this.modalReturnFocus = this.captureModalReturnFocus(returnFocus);
    this.modalPosition = this.defaultModalPosition();
  }

  private closeModal(): void {
    this.modal = null;
    this.modalPosition = null;
    this.modalRenderSignature = null;
  }

  private restoreModalFocus(): void {
    const returnFocus = this.modalReturnFocus;
    this.modalReturnFocus = null;
    if (!returnFocus) {
      return;
    }
    const target =
      returnFocus.element?.isConnected === true
        ? returnFocus.element
        : returnFocus.selector
          ? this.root.querySelector(returnFocus.selector)
          : null;
    if (target instanceof HTMLElement) {
      target.focus();
    }
  }

  private captureModalReturnFocus(returnFocus?: HTMLElement | null): { element: HTMLElement | null; selector: string | null } | null {
    if (!returnFocus) {
      return null;
    }
    return {
      element: returnFocus.isConnected ? returnFocus : null,
      selector: this.modalReturnFocusSelector(returnFocus),
    };
  }

  private modalReturnFocusSelector(element: HTMLElement): string | null {
    const windowPanel = element.closest<HTMLElement>('[data-window-panel]')?.dataset.windowPanel;
    const panelPrefix = windowPanel ? `[data-window-panel="${HUD.cssAttributeValue(windowPanel)}"] ` : '';
    if (element.dataset.modalOpen) {
      let selector = `${panelPrefix}[data-modal-open="${HUD.cssAttributeValue(element.dataset.modalOpen)}"]`;
      if (element.dataset.helpTopic) {
        selector += `[data-help-topic="${HUD.cssAttributeValue(element.dataset.helpTopic)}"]`;
      }
      return selector;
    }
    if (element.dataset.action) {
      let selector = `${panelPrefix}[data-action="${HUD.cssAttributeValue(element.dataset.action)}"]`;
      if (element.dataset.planetId) {
        selector += `[data-planet-id="${HUD.cssAttributeValue(element.dataset.planetId)}"]`;
      }
      return selector;
    }
    return null;
  }

  private static cssAttributeValue(value: string): string {
    return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"');
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

  private defaultWindowPlacement(panel: HUDWindowID): { x: number; y: number; width: number; height: number } {
    const layout = windowLayout(panel);
    const size = this.clampWindowSize(panel, layout.width, layout.preferredHeight);
    const x = (window.innerWidth - size.width) / 2;
    const y = (window.innerHeight - size.height) / 2;
    return { ...this.clampWindowPosition(panel, x, y, size.width, size.height), ...size };
  }

  private clampWindowPosition(panel: HUDWindowID, x: number, y: number, width?: number, height?: number): { x: number; y: number } {
    const layout = windowLayout(panel);
    const state = this.windowStates.get(panel);
    const effectiveWidth = Math.min(width ?? state?.width ?? layout.width, Math.max(320, window.innerWidth - 16));
    const effectiveHeight = Math.min(height ?? state?.height ?? layout.preferredHeight, Math.max(260, window.innerHeight - 72));
    const margin = 8;
    const topMargin = window.innerWidth < 768 ? margin : 56;
    const maxX = Math.max(margin, window.innerWidth - effectiveWidth - margin);
    const maxY = Math.max(topMargin, window.innerHeight - effectiveHeight - margin);
    return {
      x: clamp(x, margin, maxX),
      y: clamp(y, topMargin, maxY),
    };
  }

  private clampWindowSize(panel: HUDWindowID, width: number, height: number): { width: number; height: number } {
    const viewportWidth = Math.max(320, window.innerWidth || 1024);
    const viewportHeight = Math.max(360, window.innerHeight || 768);
    const maxWidth = Math.max(280, viewportWidth - 16);
    const maxHeight = Math.max(260, viewportHeight - (window.innerWidth < 768 ? 112 : 72));
    const minWidth = Math.min(maxWidth, window.innerWidth < 768 ? 280 : 360);
    const minHeight = Math.min(maxHeight, panel === 'chat' ? 360 : 280);
    return {
      width: clamp(width, minWidth, maxWidth),
      height: clamp(height, minHeight, maxHeight),
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

  private startResize(panel: HUDWindowID, event: PointerEvent): void {
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
      target: 'window-resize',
      id: panel,
      pointerID: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      startWidth: rect.width,
      startHeight: rect.height,
    };
    windowPanel.dataset.resizing = 'true';
    markHUDInputSuppressed();
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
    if (drag.target === 'window-resize') {
      const size = this.clampWindowSize(drag.id, drag.startWidth + event.clientX - drag.startX, drag.startHeight + event.clientY - drag.startY);
      const position = this.clampWindowPosition(drag.id, state.x, state.y, size.width, size.height);
      state.width = size.width;
      state.height = size.height;
      state.x = position.x;
      state.y = position.y;
      this.applyWindowPosition(state);
      this.applyWindowSize(state);
      markHUDInputSuppressed();
      event.preventDefault();
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
      delete windowPanel.dataset.resizing;
    }
    this.dragState = null;
    markHUDInputSuppressed();
  }

  private defaultModalPosition(): { x: number; y: number } | null {
    if (window.innerWidth < 768) {
      return null;
    }
    const width = Math.min(768, Math.max(320, window.innerWidth - 16));
    const height = Math.min(672, Math.max(320, window.innerHeight - 24));
    return this.clampModalPosition((window.innerWidth - width) / 2, (window.innerHeight - height) / 2);
  }

  private clampModalPosition(x: number, y: number): { x: number; y: number } {
    const width = Math.min(768, Math.max(320, window.innerWidth - 16));
    const height = Math.min(672, Math.max(320, window.innerHeight - 24));
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

  private applyWindowSize(state: HUDWindowState): void {
    const element = this.windowLayer.querySelector<HTMLElement>(`[data-window-panel="${state.id}"]`);
    if (!element) {
      return;
    }
    element.style.setProperty('--window-width', `${state.width}px`);
    element.style.setProperty('--window-height', `${state.height}px`);
    element.dataset.width = String(Math.round(state.width));
    element.dataset.height = String(Math.round(state.height));
  }

  private handleViewportResize(): void {
    this.hideModuleTooltip();
    for (const state of this.windowStates.values()) {
      if (!state.open) {
        continue;
      }
      const size = this.clampWindowSize(state.id, state.width, state.height);
      const position = this.clampWindowPosition(state.id, state.x, state.y, size.width, size.height);
      state.width = size.width;
      state.height = size.height;
      state.x = position.x;
      state.y = position.y;
      this.applyWindowPosition(state);
      this.applyWindowSize(state);
    }
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

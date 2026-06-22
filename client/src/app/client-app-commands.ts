import { canSendRealtimeCommand } from './command-gate';
import { resolvePlanetNavigationTarget } from './planet-navigation';
import { activeEntityMovement, boundedMovementTarget, currentEntityPosition, distanceBetween, LONG_RANGE_MOVE_STEP_UNITS } from '../state/movement';
import { isAttackableVisibleTarget } from '../state/target-eligibility';
import { CLIENT_EVENTS, EntityPayload, Operation, OPERATIONS, RequestEnvelope, Vec2 } from '../protocol/envelope';
import {
  ClientAppCore,
  DemoStateModule,
  ECONOMY_REFRESH_DEBOUNCE_MS,
  NAVIGATION_TARGET_TOLERANCE_UNITS,
} from './client-app-core';

export abstract class ClientAppCommands extends ClientAppCore {
  protected abstract activateLootTarget(target: EntityPayload, source: 'click' | 'action'): void;
  protected abstract cancelNavigation(): void;
  protected abstract estimatedServerTime(): number | null;
  protected abstract findLocalPlayerID(): string | null;
  protected abstract hasPendingOperation(op: string): boolean;
  protected abstract loadDemoState(): Promise<DemoStateModule>;
  protected abstract scheduleNavigationLoop(serverNow?: number | null): void;
  protected abstract selectedTarget(): EntityPayload | null;
  protected abstract selfEntity(): EntityPayload | null;
  protected abstract selfStealthEnabled(): boolean;

  protected handleWorldMoveIntent(target: Vec2): void {
    this.sendMove(target);
  }

  protected sendMove(target: Vec2, preservePendingLoot = false): void {
    if (!preservePendingLoot) {
      this.pendingLootPickupID = null;
      this.pendingLootApproachID = null;
    }

    this.navigationTarget = { ...target };
    this.sendNavigationStep();
  }

  protected sendNavigationStep(): void {
    const target = this.navigationTarget;
    if (!target) {
      return;
    }

    if (this.state.auth.mode === 'real' && this.state.connectionStatus !== 'connected') {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Move rejected: waiting for authenticated realtime link.' });
      this.cancelNavigation();
      return;
    }
    if (this.state.ship?.disabled === true) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Move rejected: ship disabled.' });
      this.cancelNavigation();
      return;
    }
    if (this.hasPendingOperation(OPERATIONS.moveTo)) {
      this.scheduleNavigationLoop();
      return;
    }

    const self = this.selfEntity();
    if (!self) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Move rejected: awaiting server position.' });
      this.cancelNavigation();
      return;
    }

    const serverNow = this.estimatedServerTime() ?? Date.now();
    const origin = currentEntityPosition(self, serverNow);
    const remainingDistance = distanceBetween(origin, target);
    if (remainingDistance <= NAVIGATION_TARGET_TOLERANCE_UNITS) {
      this.cancelNavigation();
      return;
    }

    const moveTarget = boundedMovementTarget(origin, target, LONG_RANGE_MOVE_STEP_UNITS);
    const stepDistance = distanceBetween(origin, moveTarget);
    const etaMs = movementEtaFromStats(stepDistance, this.state.stats?.speed ?? self.movement?.speed ?? null);
    const finalSuffix =
      remainingDistance - stepDistance > NAVIGATION_TARGET_TOLERANCE_UNITS
        ? `, final ${formatVec(target)} (${Math.round(remainingDistance)}u)`
        : '';
    this.dispatch({
      type: 'appendLog',
      level: 'info',
      text: `Move ${formatVec(origin)} -> ${formatVec(moveTarget)}, ${Math.round(stepDistance)}u, eta ${formatDuration(etaMs)}${finalSuffix}.`,
    });

    const command = this.commandBuilder.moveTo(moveTarget);
    this.sendCommand(command);

    if (this.state.auth.mode === 'demo' && !this.realtime.isConnected()) {
      const localID = this.findLocalPlayerID();
      if (!localID) {
        return;
      }
      window.setTimeout(() => {
        void this.loadDemoState().then(({ correctionEvent }) => {
          this.dispatch({ type: 'eventReceived', envelope: correctionEvent(localID, moveTarget) });
        });
      }, 120);
    }
  }

  protected stopMovement(): void {
    this.cancelNavigation();
    this.sendCommand(this.commandBuilder.stop());
  }

  protected sendBasicSkill(): void {
    const target = this.selectedTarget();
    if (!target || !isAttackableVisibleTarget(target, this.selfEntity())) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'No attackable target selected.' });
      return;
    }
    this.sendCommand(this.commandBuilder.combatUseSkill(target.entity_id));
  }

  protected sendLootPickup(): void {
    const target = this.selectedTarget();
    if (!target || target.entity_type !== 'loot') {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'No visible drop selected.' });
      return;
    }
    this.activateLootTarget(target, 'action');
  }

  protected sendAuctionBid(auctionID: string, amount: number): void {
    if (!auctionID || amount <= 0) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `auction-bid:${auctionID}`,
      () => this.commandBuilder.auctionBid(auctionID, amount),
      'Auction bid already pending.',
    );
  }

  protected sendHangarActivateShip(shipID: string): void {
    if (!shipID) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `activate:${shipID}`,
      () => this.commandBuilder.hangarActivateShip(shipID),
      'Ship activation already pending.',
    );
  }

  protected sendLoadoutEquipModule(slotID: string, itemInstanceID: string): void {
    if (!slotID || !itemInstanceID) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `equip:${slotID}:${itemInstanceID}`,
      () => this.commandBuilder.loadoutEquipModule(slotID, itemInstanceID),
      'Module equip already pending.',
    );
  }

  protected sendLoadoutUnequipModule(slotID: string): void {
    if (!slotID) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `unequip:${slotID}`,
      () => this.commandBuilder.loadoutUnequipModule(slotID),
      'Module unequip already pending.',
    );
  }

  protected sendShopBuyProduct(productID: string, quantity: number): void {
    if (!productID) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `shop-buy:${productID}`,
      () => this.commandBuilder.shopBuyProduct(productID, quantity),
      'Shop purchase already pending.',
    );
  }

  protected sendMarketCreateListing(input: {
    itemID: string;
    quantity: number;
    unitPrice: number;
    sourceLocation?: string;
    itemInstanceID?: string;
  }): void {
    if (!input.itemID) {
      return;
    }
    const quantity = Number.isFinite(input.quantity) ? Math.max(1, Math.round(input.quantity)) : 1;
    const unitPrice = Number.isFinite(input.unitPrice) ? Math.max(1, Math.round(input.unitPrice)) : 1;
    const source = input.itemInstanceID || input.sourceLocation || 'inventory';
    this.sendGuardedGameplayCommand(
      `market-create:${source}:${input.itemID}:${quantity}:${unitPrice}`,
      () => this.commandBuilder.marketCreateListing({ ...input, quantity, unitPrice }),
      'Market listing already pending.',
    );
  }

  protected sendMarketBuy(listingID: string, quantity: number): void {
    if (!listingID) {
      return;
    }
    const requestedQuantity = Number.isFinite(quantity) ? Math.max(1, Math.round(quantity)) : 1;
    this.sendGuardedGameplayCommand(
      `market-buy:${listingID}`,
      () => this.commandBuilder.marketBuy(listingID, requestedQuantity),
      'Market buy already pending.',
    );
  }

  protected sendMarketCancel(listingID: string): void {
    if (!listingID) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `market-cancel:${listingID}`,
      () => this.commandBuilder.marketCancel(listingID),
      'Market cancel already pending.',
    );
  }

  protected sendAuctionBuyNow(auctionID: string): void {
    if (!auctionID) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `auction-buy-now:${auctionID}`,
      () => this.commandBuilder.auctionBuyNow(auctionID),
      'Auction buy-now already pending.',
    );
  }

  protected sendPremiumClaim(entitlementID: string): void {
    if (!entitlementID) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `premium-claim:${entitlementID}`,
      () => this.commandBuilder.premiumClaim(entitlementID),
      'Premium claim already pending.',
    );
  }

  protected sendPremiumWeeklyXCore(productID: string, periodKey: string): void {
    if (!productID || !periodKey) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `premium-weekly-xcore:${productID}:${periodKey}`,
      () => this.commandBuilder.premiumPurchaseWeeklyXCore(productID, periodKey),
      'Premium purchase already pending.',
    );
  }

  protected sendPortalEnter(portalID: string): void {
    if (!portalID) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `portal-enter:${portalID}`,
      () => this.commandBuilder.portalEnter(portalID),
      'Portal entry already pending.',
    );
  }

  protected sendGuardedGameplayCommand(actionKey: string, buildEnvelope: () => RequestEnvelope, pendingMessage: string): void {
    if (this.pendingGameplayActionKeys.has(actionKey)) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: pendingMessage });
      return;
    }
    const envelope = buildEnvelope();
    if (this.sendCommand(envelope)) {
      this.pendingGameplayActionKeys.add(actionKey);
      this.pendingGameplayActionKeysByRequest.set(envelope.request_id, actionKey);
    }
  }

  protected requestPlanetDetail(planetID: string): void {
    if (!planetID) {
      return;
    }
    this.sendCommand(this.commandBuilder.planetDetail(planetID));
  }

  protected navigateToPlanet(planetID: string): void {
    if (!planetID) {
      return;
    }

    const detail = this.state.planetIntel?.selectedPlanet ?? null;
    if (!detail || detail.planet_id !== planetID) {
      this.dispatch({ type: 'appendLog', level: 'info', text: 'Planet navigation waiting for server detail.' });
      this.requestPlanetDetail(planetID);
      return;
    }

    const target = resolvePlanetNavigationTarget(this.state.planetIntel, planetID);
    if (!target) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Planet navigation rejected: awaiting server coordinates.' });
      this.requestPlanetDetail(planetID);
      return;
    }

    this.sendMove(target);
  }

  protected sendPlanetClaim(planetID: string): void {
    if (!planetID) {
      return;
    }
    this.sendGuardedGameplayCommand(
      `planet-claim:${planetID}`,
      () => this.commandBuilder.claimPlanet(planetID),
      'Planet claim already pending.',
    );
  }

  protected toggleScanMode(): void {
    this.dispatch({ type: 'scanModeToggled' });
  }

  protected toggleStealth(): void {
    if (this.hasPendingOperation(OPERATIONS.stealthToggle)) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Cloak toggle already pending.' });
      return;
    }
    this.sendCommand(this.commandBuilder.stealthToggle(!this.selfStealthEnabled()));
  }

  protected sendCommand(envelope: RequestEnvelope): boolean {
    if (!canSendRealtimeCommand(this.state.auth.mode, this.state.connectionStatus)) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Waiting for authenticated realtime link.' });
      return false;
    }

    this.dispatch({ type: 'requestQueued', envelope });
    if (!this.realtime.send(envelope)) {
      this.dispatch(
        this.state.auth.mode === 'demo'
          ? {
              type: 'appendLog',
              level: 'warn',
              text: 'Offline demo accepted local intent.',
            }
          : {
              type: 'appendLog',
              level: 'warn',
              text: 'Intent queued while realtime link is unavailable.',
            },
      );
    }
    return true;
  }


  protected syncSnapshot(): void {
    if (this.demoMode && this.state.auth.mode === 'demo') {
      this.sendCommand(this.commandBuilder.debugSnapshot());
      return;
    }
    this.sendCommand(this.commandBuilder.worldSnapshot());
    this.requestSystemsSnapshot(true);
  }

  protected requestSystemsSnapshotOnce(): void {
    if (this.systemsSnapshotRequested) {
      return;
    }
    this.requestSystemsSnapshot(false);
  }

  protected requestSystemsSnapshot(force: boolean): void {
    if (this.state.auth.mode !== 'real' || this.state.connectionStatus !== 'connected') {
      return;
    }
    if (this.systemsSnapshotRequested && !force) {
      return;
    }
    this.systemsSnapshotRequested = true;
    this.sendCommand(this.commandBuilder.progressionSnapshot());
    this.sendCommand(this.commandBuilder.inventorySnapshot());
    this.sendCommand(this.commandBuilder.walletSnapshot());
    this.sendCommand(this.commandBuilder.hangarSnapshot());
    this.sendCommand(this.commandBuilder.loadoutSnapshot());
    this.sendCommand(this.commandBuilder.statsSnapshot());
    this.sendCommand(this.commandBuilder.craftingRecipes());
    this.sendCommand(this.commandBuilder.knownPlanets());
    this.sendCommand(this.commandBuilder.productionSummary());
    this.sendCommand(this.commandBuilder.planetStorageSummary());
    this.sendCommand(this.commandBuilder.routeList());
    this.sendCommand(this.commandBuilder.shopCatalog());
    this.sendCommand(this.commandBuilder.marketSearch());
    this.sendCommand(this.commandBuilder.auctionSearch());
    this.sendCommand(this.commandBuilder.premiumEntitlements());
    this.sendCommand(this.commandBuilder.questBoard());
    this.sendCommand(this.commandBuilder.questProgress());
    if (this.state.auth.session?.account?.admin) {
      this.refreshAdminOps();
    }
  }

  protected refreshAdminOps(): void {
    if (!this.state.auth.session?.account?.admin) {
      return;
    }
    this.sendCommand(this.commandBuilder.adminEconomyDashboard());
    this.sendCommand(this.commandBuilder.adminInspectPlayer());
    this.sendCommand(this.commandBuilder.observabilityCommandLog());
    this.sendCommand(this.commandBuilder.observabilityMetrics());
    this.sendCommand(this.commandBuilder.observabilityReleaseGate());
    this.sendCommand(this.commandBuilder.observabilityAbuseCoverage());
  }

  protected handleEconomyRefreshForEvent(eventType: string): void {
    switch (eventType) {
      case CLIENT_EVENTS.marketListingCreated:
      case CLIENT_EVENTS.marketListingCancelled:
        this.scheduleEconomyRefresh(OPERATIONS.marketSearch, OPERATIONS.inventorySnapshot);
        return;
      case CLIENT_EVENTS.marketListingUpdated:
        this.scheduleEconomyRefresh(OPERATIONS.marketSearch);
        return;
      case CLIENT_EVENTS.marketSaleCompleted:
        this.scheduleEconomyRefresh(OPERATIONS.marketSearch, OPERATIONS.walletSnapshot, OPERATIONS.inventorySnapshot);
        return;
      case CLIENT_EVENTS.auctionLotUpdated:
        this.scheduleEconomyRefresh(OPERATIONS.auctionSearch);
        return;
      case CLIENT_EVENTS.auctionBidPlaced:
      case CLIENT_EVENTS.auctionClosed:
        this.scheduleEconomyRefresh(OPERATIONS.auctionSearch, OPERATIONS.walletSnapshot);
        return;
      case CLIENT_EVENTS.premiumEntitlementCreated:
        this.scheduleEconomyRefresh(OPERATIONS.premiumEntitlements);
        return;
      case CLIENT_EVENTS.premiumEntitlementClaimed:
      case CLIENT_EVENTS.premiumStockConsumed:
        this.scheduleEconomyRefresh(OPERATIONS.premiumEntitlements, OPERATIONS.walletSnapshot, OPERATIONS.shopCatalog);
        return;
      case CLIENT_EVENTS.economyFlowUpdated:
        this.scheduleEconomyRefresh(OPERATIONS.adminEconomyDashboard);
        return;
      default:
        return;
    }
  }

  protected scheduleEconomyRefresh(...ops: Operation[]): void {
    if (this.state.auth.mode !== 'real' || this.state.connectionStatus !== 'connected') {
      return;
    }
    for (const op of ops) {
      this.pendingEconomyRefreshOps.add(op);
    }
    if (this.economyRefreshTimer !== null) {
      return;
    }
    this.economyRefreshTimer = window.setTimeout(() => {
      this.economyRefreshTimer = null;
      const refreshOps = [...this.pendingEconomyRefreshOps];
      this.pendingEconomyRefreshOps.clear();
      if (this.state.auth.mode !== 'real' || this.state.connectionStatus !== 'connected') {
        return;
      }
      for (const op of refreshOps) {
        this.sendEconomyRefreshCommand(op);
      }
    }, ECONOMY_REFRESH_DEBOUNCE_MS);
  }

  protected sendEconomyRefreshCommand(op: Operation): void {
    switch (op) {
      case OPERATIONS.marketSearch:
        this.sendCommand(this.commandBuilder.marketSearch());
        return;
      case OPERATIONS.shopCatalog:
        this.sendCommand(this.commandBuilder.shopCatalog());
        return;
      case OPERATIONS.auctionSearch:
        this.sendCommand(this.commandBuilder.auctionSearch());
        return;
      case OPERATIONS.premiumEntitlements:
        this.sendCommand(this.commandBuilder.premiumEntitlements());
        return;
      case OPERATIONS.walletSnapshot:
        this.sendCommand(this.commandBuilder.walletSnapshot());
        return;
      case OPERATIONS.inventorySnapshot:
        this.sendCommand(this.commandBuilder.inventorySnapshot());
        return;
      case OPERATIONS.adminEconomyDashboard:
        if (this.state.auth.session?.account?.admin) {
          this.sendCommand(this.commandBuilder.adminEconomyDashboard());
        }
        return;
      default:
        return;
    }
  }

}

function movementEtaFromStats(distance: number, speed: number | null): number | null {
  return speed && speed > 0 ? (distance / speed) * 1000 : null;
}

export function formatDuration(milliseconds: number | null): string {
  if (milliseconds === null) {
    return '--';
  }
  if (milliseconds < 1000) {
    return '<1s';
  }
  return `${(milliseconds / 1000).toFixed(milliseconds < 10_000 ? 1 : 0)}s`;
}

export function formatVec(position: Vec2): string {
  return `${Math.round(position.x)},${Math.round(position.y)}`;
}

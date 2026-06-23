import { AuthPanel } from '../ui/auth-panel';
import { HUD } from '../ui/hud';
import { ClientAppHandlers } from './client-app-handlers';

export class ClientApp extends ClientAppHandlers {
  constructor(root: HTMLElement) {
    super(root);
  }

  async start(): Promise<void> {
    this.root.className = 'client-shell';
    this.root.innerHTML = `
      <div class="auth-host"></div>
      <main class="game-surface">
        <div class="world-host" aria-label="World view"></div>
        <div class="hud-host"></div>
      </main>
    `;

    const worldHost = this.root.querySelector<HTMLElement>('.world-host');
    const hudHost = this.root.querySelector<HTMLElement>('.hud-host');
    const authHost = this.root.querySelector<HTMLElement>('.auth-host');
    if (!worldHost || !hudHost || !authHost) {
      throw new Error('Client shell failed to mount.');
    }

    await this.renderer.mount(worldHost);
    this.startSmokeStatePublisher();
    this.authPanel = new AuthPanel(authHost, {
      onLogin: (email, password) => void this.login(email, password),
      onRegister: (email, password, callsign) => void this.register(email, password, callsign),
    });
    this.hud = new HUD(hudHost, {
      onConnect: (url) => this.connect(url),
      onDisconnect: () => this.disconnect(),
      onLogout: () => void this.logout(),
      onStop: () => this.stopMovement(),
      onSync: () => this.syncSnapshot(),
      onFire: () => this.sendBasicSkill(),
      onLoot: () => this.sendLootPickup(),
      onRepairQuote: () => this.sendCommand(this.commandBuilder.deathRepairQuote()),
      onRepair: () => this.sendCommand(this.commandBuilder.deathRepairShip()),
      onScan: () => this.toggleScanMode(),
      onStealthToggle: () => this.toggleStealth(),
      onSelectTarget: (entityID, source) => this.selectEntity(entityID, source ?? 'hud'),
      onCycleTarget: () => this.cycleTarget(),
      onPortalEnter: (portalID) => this.sendPortalEnter(portalID),
      onPlanetDetail: (planetID) => this.requestPlanetDetail(planetID),
      onPlanetNavigate: (planetID) => this.navigateToPlanet(planetID),
      onPlanetClaim: (planetID) => this.sendPlanetClaim(planetID),
      onPlanetBuildingBuild: (input) => this.sendPlanetBuildingBuild(input),
      onPlanetBuildingUpgrade: (input) => this.sendPlanetBuildingUpgrade(input),
      onCraftingStart: (recipeID) => this.sendCraftingStart(recipeID),
      onCraftingComplete: (jobID) => this.sendCraftingComplete(jobID),
      onCraftingCancel: (jobID) => this.sendCraftingCancel(jobID),
      onRouteCreate: (input) => this.sendRouteCreate(input),
      onRouteUpdate: (input) => this.sendRouteUpdate(input),
      onRouteEnable: (routeID) => this.sendRouteEnable(routeID),
      onRouteDisable: (routeID) => this.sendRouteDisable(routeID),
      onRouteSettle: (routeID) => this.sendRouteSettle(routeID),
      onHangarActivateShip: (shipID) => this.sendHangarActivateShip(shipID),
      onLoadoutEquipModule: (slotID, itemInstanceID) => this.sendLoadoutEquipModule(slotID, itemInstanceID),
      onLoadoutUnequipModule: (slotID) => this.sendLoadoutUnequipModule(slotID),
      onMarketCreateListing: (input) => this.sendMarketCreateListing(input),
      onShopBuyProduct: (productID, quantity) => this.sendShopBuyProduct(productID, quantity),
      onMarketBuy: (listingID, quantity) => this.sendMarketBuy(listingID, quantity),
      onMarketCancel: (listingID) => this.sendMarketCancel(listingID),
      onAuctionBid: (auctionID, amount) => this.sendAuctionBid(auctionID, amount),
      onAuctionBuyNow: (auctionID) => this.sendAuctionBuyNow(auctionID),
      onAuctionGrants: () => this.sendCommand(this.commandBuilder.auctionGrants()),
      onPremiumClaim: (entitlementID) => this.sendPremiumClaim(entitlementID),
      onPremiumWeeklyXCore: (productID, periodKey) => this.sendPremiumWeeklyXCore(productID, periodKey),
      onQuestAccept: (offerID) => this.sendCommand(this.commandBuilder.questAccept(offerID)),
      onQuestClaim: (questID) => this.sendCommand(this.commandBuilder.questClaimReward(questID)),
      onQuestReroll: () => this.sendCommand(this.commandBuilder.questReroll()),
      onAdminRefresh: () => this.refreshAdminOps(),
      onAdminRepairCraftJob: (jobID) => this.sendCommand(this.commandBuilder.adminRepairCraftJob(jobID)),
    });

    if (this.demoMode) {
      this.dispatch({ type: 'demoModeStarted' });
      await this.seedDemoState();
      this.render();
      return;
    }

    this.render();
    await this.restoreSession();
  }

  protected connect(url: string): void {
    this.intentionalDisconnect = false;
    this.cancelNavigation();
    this.clearEconomyRefreshTimer();
    this.clearPendingGameplayActionKeys();
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.dispatch({ type: 'replaceVisibleEntities', entities: [], serverTime: null });
    this.realtime.connect(url);
  }

  protected disconnect(): void {
    this.intentionalDisconnect = true;
    this.cancelNavigation();
    this.clearEconomyRefreshTimer();
    this.clearPendingGameplayActionKeys();
    this.systemsSnapshotRequested = false;
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.clearReconnectTimer();
    this.realtime.disconnect();
    if (this.state.auth.mode === 'demo') {
      void this.seedDemoState();
    }
  }

  protected async seedDemoState(): Promise<void> {
    const { demoEvents } = await this.loadDemoState();
    this.dispatch({ type: 'replaceVisibleEntities', entities: [], serverTime: null });
    for (const envelope of demoEvents()) {
      this.dispatch({ type: 'eventReceived', envelope });
    }
  }

}

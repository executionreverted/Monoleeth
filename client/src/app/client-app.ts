import { AuthClient, AuthClientError } from '../auth/auth-client';
import { RealtimeClient } from '../net/realtime-client';
import { CommandBuilder } from '../protocol/commands';
import { CLIENT_EVENTS, EntityPayload, ErrorEnvelope, OPERATIONS, RequestEnvelope, ServerMessage, Vec2 } from '../protocol/envelope';
import { WorldRenderer } from '../render/world-renderer';
import { AuthPanel } from '../ui/auth-panel';
import { HUD } from '../ui/hud';
import {
  activeEntityMovement,
  currentEntityPosition,
  distanceBetween,
  estimateServerTime,
  movementTiming,
  selfEntity as selectSelfEntity,
  serverClockOffset,
} from '../state/movement';
import { createInitialState, reduceClientState } from '../state/reducer';
import { ClientAction, ClientState, PublicSession, WorldMapMemoryMarker } from '../state/types';
import { worldMapMemoryMarkers } from '../state/world-memory';

type DemoStateModule = typeof import('./demo-state');

const SCAN_PENDING_RECHECK_MS = 500;
const SCAN_STARTED_RECHECK_MS = 350;
const SCAN_PAUSED_RECHECK_MS = 750;
const SCAN_ERROR_BACKOFF_MS = 3_200;
const MOVEMENT_ETA_RENDER_MS = 100;

export class ClientApp {
  private state: ClientState = createInitialState();
  private readonly authClient = new AuthClient();
  private readonly commandBuilder = new CommandBuilder();
  private readonly renderer = new WorldRenderer({
    onMoveIntent: (target) => this.sendMove(target),
    onSelectTarget: (entityID) => this.selectEntity(entityID),
    onSelectMemoryMarker: (marker) => this.selectMemoryMarker(marker),
  });
  private readonly realtime = new RealtimeClient({
    onStatus: (status) => this.handleRealtimeStatus(status),
    onMessage: (message) => this.applyServerMessage(message),
    onError: (message) => this.dispatch({ type: 'appendLog', level: 'error', text: message }),
  });
  private authPanel: AuthPanel | null = null;
  private hud: HUD | null = null;
  private reconnectTimer: number | null = null;
  private smokeStateTimer: number | null = null;
  private cooldownRenderTimer: number | null = null;
  private cooldownRenderAt = 0;
  private movementRenderTimer: number | null = null;
  private movementRenderAt = 0;
  private serverClockOffsetValue: number | null = null;
  private serverClockTime: number | null = null;
  private acceptedMovementSignature: string | null = null;
  private scanTimer: number | null = null;
  private scanTimerAt = 0;
  private pendingLootPickupID: string | null = null;
  private pendingLootApproachID: string | null = null;
  private intentionalDisconnect = false;
  private systemsSnapshotRequested = false;
  private demoState: DemoStateModule | null = null;
  private readonly demoMode = typeof window !== 'undefined' && new URLSearchParams(window.location.search).get('demo') === '1';

  constructor(private readonly root: HTMLElement) {}

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
      onStop: () => this.sendCommand(this.commandBuilder.stop()),
      onSync: () => this.syncSnapshot(),
      onFire: () => this.sendBasicSkill(),
      onLoot: () => this.sendLootPickup(),
      onRepairQuote: () => this.sendCommand(this.commandBuilder.deathRepairQuote()),
      onRepair: () => this.sendCommand(this.commandBuilder.deathRepairShip()),
      onScan: () => this.toggleScanMode(),
      onPlanetDetail: (planetID) => this.requestPlanetDetail(planetID),
      onMarketCreateListing: (input) => this.sendCommand(this.commandBuilder.marketCreateListing(input)),
      onMarketBuy: (listingID) => this.sendCommand(this.commandBuilder.marketBuy(listingID, 1)),
      onMarketCancel: (listingID) => this.sendCommand(this.commandBuilder.marketCancel(listingID)),
      onAuctionBid: (auctionID, amount) => this.sendCommand(this.commandBuilder.auctionBid(auctionID, amount)),
      onAuctionBuyNow: (auctionID) => this.sendCommand(this.commandBuilder.auctionBuyNow(auctionID)),
      onAuctionClaimGrant: () => this.sendCommand(this.commandBuilder.auctionClaimGrant()),
      onPremiumClaim: (entitlementID) => this.sendCommand(this.commandBuilder.premiumClaim(entitlementID)),
      onPremiumWeeklyXCore: () => this.sendCommand(this.commandBuilder.premiumPurchaseWeeklyXCore()),
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

  private connect(url: string): void {
    this.intentionalDisconnect = false;
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.dispatch({ type: 'replaceVisibleEntities', entities: [], serverTime: null });
    this.realtime.connect(url);
  }

  private disconnect(): void {
    this.intentionalDisconnect = true;
    this.systemsSnapshotRequested = false;
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.clearReconnectTimer();
    this.realtime.disconnect();
    if (this.state.auth.mode === 'demo') {
      void this.seedDemoState();
    }
  }

  private async seedDemoState(): Promise<void> {
    const { demoEvents } = await this.loadDemoState();
    this.dispatch({ type: 'replaceVisibleEntities', entities: [], serverTime: null });
    for (const envelope of demoEvents()) {
      this.dispatch({ type: 'eventReceived', envelope });
    }
  }

  private sendMove(target: Vec2, preservePendingLoot = false): void {
    if (!preservePendingLoot) {
      this.pendingLootPickupID = null;
      this.pendingLootApproachID = null;
    }

    if (this.state.auth.mode === 'real' && this.state.connectionStatus !== 'connected') {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Move rejected: waiting for authenticated realtime link.' });
      return;
    }
    if (this.state.ship?.disabled === true) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Move rejected: ship disabled.' });
      return;
    }

    const self = this.selfEntity();
    if (!self) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Move rejected: awaiting server position.' });
      return;
    }

    const serverNow = this.estimatedServerTime() ?? Date.now();
    const origin = currentEntityPosition(self, serverNow);
    const distance = distanceBetween(origin, target);
    const etaMs = movementEtaFromStats(distance, this.state.stats?.speed ?? self.movement?.speed ?? null);
    this.dispatch({
      type: 'appendLog',
      level: 'info',
      text: `Move ${formatVec(origin)} -> ${formatVec(target)}, ${Math.round(distance)}u, eta ${formatDuration(etaMs)}.`,
    });

    const command = this.commandBuilder.moveTo(target);
    this.sendCommand(command);

    if (this.state.auth.mode === 'demo' && !this.realtime.isConnected()) {
      const localID = this.findLocalPlayerID();
      if (!localID) {
        return;
      }
      window.setTimeout(() => {
        void this.loadDemoState().then(({ correctionEvent }) => {
          this.dispatch({ type: 'eventReceived', envelope: correctionEvent(localID, target) });
        });
      }, 120);
    }
  }

  private sendBasicSkill(): void {
    const target = this.selectedTarget();
    if (!target || target.entity_type !== 'npc') {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'No hostile target selected.' });
      return;
    }
    this.sendCommand(this.commandBuilder.combatUseSkill(target.entity_id));
  }

  private sendLootPickup(): void {
    const target = this.selectedTarget();
    if (!target || target.entity_type !== 'loot') {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'No visible drop selected.' });
      return;
    }
    this.activateLootTarget(target, 'action');
  }

  private requestPlanetDetail(planetID: string): void {
    if (!planetID) {
      return;
    }
    this.sendCommand(this.commandBuilder.planetDetail(planetID));
  }

  private toggleScanMode(): void {
    this.dispatch({ type: 'scanModeToggled' });
  }

  private sendCommand(envelope: RequestEnvelope): void {
    if (this.state.auth.mode === 'real' && this.state.connectionStatus !== 'connected') {
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Waiting for authenticated realtime link.' });
      return;
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
  }

  private applyServerMessage(message: ServerMessage): void {
    if ('event_id' in message) {
      this.dispatch({ type: 'eventReceived', envelope: message });
      this.logAcceptedSelfMovement();
      if (message.type === CLIENT_EVENTS.worldSnapshot) {
        this.requestSystemsSnapshotOnce();
      }
      return;
    }

    const pending = this.state.pendingCommands[message.request_id] ?? null;
    this.dispatch({ type: 'responseReceived', envelope: message });
    if (pending?.op === OPERATIONS.moveTo && message.ok === false) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: `Move rejected: ${message.error.message}` });
    }
    if (pending?.op === OPERATIONS.scanPulse) {
      if (message.ok === false) {
        const now = Date.now();
        this.dispatch({
          type: 'scanPulseRejected',
          message: message.error.message,
          rejectedAt: now,
          backoffUntil: now + SCAN_ERROR_BACKOFF_MS,
        });
      } else {
        this.dispatch({ type: 'scanPulseAccepted' });
      }
    }
    if (message.ok === false && isAuthError(message)) {
      this.handleAuthExpired(message.error.message);
    }
  }

  private handleRealtimeStatus(status: ClientState['connectionStatus']): void {
    if (status === 'connected') {
      this.dispatch({
        type: 'connectionChanged',
        status: this.state.auth.mode === 'real' ? 'authenticated_pending_socket' : 'connected',
      });
    } else if (status === 'auth_expired') {
      this.handleAuthExpired('Session expired. Please log in again.');
      return;
    } else if (status === 'offline' && this.shouldReconnect()) {
      this.dispatch({ type: 'connectionChanged', status: 'reconnecting' });
      this.scheduleReconnect();
      return;
    } else if (status === 'error' && this.shouldReconnect()) {
      this.dispatch({ type: 'connectionChanged', status: 'reconnecting' });
      this.scheduleReconnect();
      return;
    } else {
      this.dispatch({ type: 'connectionChanged', status });
    }

    if (this.state.auth.mode === 'demo' && status === 'connected') {
      this.sendCommand(this.commandBuilder.debugSnapshot());
    }
  }

  private async restoreSession(): Promise<void> {
    this.dispatch({ type: 'authRestoreStarted' });
    try {
      const session = await this.authClient.loadSession();
      if (!session.authenticated) {
        this.dispatch({ type: 'authLoggedOut' });
        return;
      }
      this.startAuthenticatedSession(session);
    } catch (error) {
      this.dispatch({ type: 'authFailed', message: safeAuthMessage(error) });
    }
  }

  private async login(email: string, password: string): Promise<void> {
    this.dispatch({ type: 'authSubmitStarted' });
    try {
      this.startAuthenticatedSession(await this.authClient.login({ email, password }));
    } catch (error) {
      this.dispatch({ type: 'authFailed', message: safeAuthMessage(error) });
    }
  }

  private async register(email: string, password: string, callsign: string): Promise<void> {
    this.dispatch({ type: 'authSubmitStarted' });
    try {
      this.startAuthenticatedSession(await this.authClient.register({ email, password, callsign }));
    } catch (error) {
      this.dispatch({ type: 'authFailed', message: safeAuthMessage(error) });
    }
  }

  private async logout(): Promise<void> {
    this.intentionalDisconnect = true;
    this.systemsSnapshotRequested = false;
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.clearReconnectTimer();
    this.realtime.disconnect();
    try {
      await this.authClient.logout();
    } catch (error) {
      this.dispatch({ type: 'appendLog', level: 'warn', text: safeAuthMessage(error) });
    }
    this.dispatch({ type: 'authLoggedOut' });
  }

  private startAuthenticatedSession(session: PublicSession): void {
    if (!session.authenticated) {
      this.dispatch({ type: 'authLoggedOut' });
      return;
    }
    this.intentionalDisconnect = false;
    this.systemsSnapshotRequested = false;
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.clearReconnectTimer();
    this.dispatch({ type: 'authSessionLoaded', session });
    this.realtime.connect(this.state.socketURL);
  }

  private syncSnapshot(): void {
    if (this.state.auth.mode === 'demo') {
      this.sendCommand(this.commandBuilder.debugSnapshot());
      return;
    }
    this.sendCommand(this.commandBuilder.worldSnapshot());
    this.requestSystemsSnapshot(true);
  }

  private requestSystemsSnapshotOnce(): void {
    if (this.systemsSnapshotRequested) {
      return;
    }
    this.requestSystemsSnapshot(false);
  }

  private requestSystemsSnapshot(force: boolean): void {
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
    this.sendCommand(this.commandBuilder.marketSearch());
    this.sendCommand(this.commandBuilder.auctionSearch());
    this.sendCommand(this.commandBuilder.premiumEntitlements());
    this.sendCommand(this.commandBuilder.questBoard());
    this.sendCommand(this.commandBuilder.questProgress());
    if (this.state.auth.session?.account?.admin) {
      this.refreshAdminOps();
    }
  }

  private refreshAdminOps(): void {
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

  private handleAuthExpired(message: string): void {
    this.intentionalDisconnect = true;
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.clearReconnectTimer();
    this.realtime.disconnect();
    this.dispatch({ type: 'authExpired', message });
  }

  private shouldReconnect(): boolean {
    return this.state.auth.mode === 'real' && Boolean(this.state.auth.session) && !this.intentionalDisconnect;
  }

  private scheduleReconnect(): void {
    if (this.reconnectTimer !== null) {
      return;
    }
    this.reconnectTimer = window.setTimeout(() => {
      this.reconnectTimer = null;
      if (this.shouldReconnect()) {
        this.realtime.connect(this.state.socketURL);
      }
    }, 750);
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer === null) {
      return;
    }
    window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
  }

  private dispatch(action: ClientAction): void {
    try {
      this.state = reduceClientState(this.state, action);
    } catch (error) {
      this.state = reduceClientState(this.state, {
        type: 'appendLog',
        level: 'error',
        text: error instanceof Error ? error.message : String(error),
      });
    }
    this.syncServerClock();
    this.render();
    this.tryPendingLootPickup();
    this.scheduleScanLoop();
  }

  private render(): void {
    const serverNow = this.estimatedServerTime();
    this.root.dataset.mode = this.state.auth.mode;
    this.root.dataset.connection = this.state.connectionStatus;
    this.renderer.render({
      entities: Object.values(this.state.visibleEntities),
      sector: this.state.sector,
      minimap: this.state.minimap,
      selectedTargetID: this.state.selectedTargetID,
      movementTarget: this.state.movementTarget,
      lastCorrection: this.state.lastCorrection,
      memoryMarkers: worldMapMemoryMarkers(this.state),
      worldEffects: this.state.worldEffects,
      scanMode: this.state.scanMode,
      lastServerTime: this.state.lastServerTime,
    });
    this.authPanel?.render(this.state);
    this.hud?.render(this.state, serverNow);
    this.publishSmokeState(serverNow);
    this.scheduleCooldownRender();
    this.scheduleMovementRender(serverNow);
  }

  private findLocalPlayerID(): string | null {
    return (
      Object.values(this.state.visibleEntities).find((entity) => entity.status_flags?.includes('self'))?.entity_id ??
      Object.values(this.state.visibleEntities).find((entity) => entity.entity_type === 'player')?.entity_id ??
      null
    );
  }

  private selectedTarget(): EntityPayload | null {
    return this.state.selectedTargetID ? this.state.visibleEntities[this.state.selectedTargetID] ?? null : null;
  }

  private selectEntity(entityID: string | null): void {
    if (!entityID || entityID !== this.pendingLootPickupID) {
      this.pendingLootPickupID = null;
      this.pendingLootApproachID = null;
    }
    this.dispatch({ type: 'selectTarget', entityID });
    if (!entityID) {
      return;
    }

    const target = this.state.visibleEntities[entityID];
    if (target?.entity_type === 'loot') {
      this.activateLootTarget(target, 'click');
      return;
    }
    this.pendingLootPickupID = null;
  }

  private selectMemoryMarker(marker: WorldMapMemoryMarker): void {
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.dispatch({ type: 'selectTarget', entityID: null });
    this.dispatch({ type: 'appendLog', level: 'info', text: `Selected known planet ${marker.label}.` });
    this.requestPlanetDetail(marker.detailID);
  }

  private activateLootTarget(target: EntityPayload, source: 'click' | 'action'): void {
    if (this.state.ship?.disabled === true) {
      this.pendingLootPickupID = null;
      this.pendingLootApproachID = null;
      this.dispatch({ type: 'appendLog', level: 'warn', text: 'Ship disabled. Repair before gathering drops.' });
      return;
    }

    const pickupRange = this.state.stats?.loot_pickup_range ?? 0;
    const distance = this.distanceToSelf(target);
    if (pickupRange > 0 && distance !== null && distance <= pickupRange) {
      this.pendingLootPickupID = null;
      this.pendingLootApproachID = null;
      this.sendCommand(this.commandBuilder.lootPickup(target.entity_id));
      return;
    }

    if (this.pendingLootPickupID === target.entity_id) {
      if (source === 'action') {
        this.dispatch({ type: 'appendLog', level: 'info', text: `Approach already queued for ${entityLabel(target)}.` });
      }
      return;
    }

    this.pendingLootPickupID = target.entity_id;
    if (pickupRange <= 0 || distance === null) {
      this.pendingLootApproachID = null;
      this.dispatch({ type: 'appendLog', level: 'warn', text: `Drop selected. Waiting for server pickup range.` });
      return;
    }

    this.pendingLootApproachID = target.entity_id;
    this.dispatch({
      type: 'appendLog',
      level: 'info',
      text: `Approaching ${entityLabel(target)} (${Math.round(distance)}u).`,
    });
    this.sendMove({ ...target.position }, true);
  }

  private tryPendingLootPickup(): void {
    const dropID = this.pendingLootPickupID;
    if (!dropID || (this.state.auth.mode === 'real' && this.state.connectionStatus !== 'connected')) {
      return;
    }

    const target = this.state.visibleEntities[dropID];
    if (!target || target.entity_type !== 'loot' || this.state.ship?.disabled === true) {
      this.pendingLootPickupID = null;
      this.pendingLootApproachID = null;
      return;
    }

    const pickupRange = this.state.stats?.loot_pickup_range ?? 0;
    const distance = this.distanceToSelf(target);
    if (pickupRange <= 0 || distance === null) {
      return;
    }
    if (distance > pickupRange) {
      if (this.pendingLootApproachID !== dropID) {
        this.pendingLootApproachID = dropID;
        this.dispatch({
          type: 'appendLog',
          level: 'info',
          text: `Approaching ${entityLabel(target)} (${Math.round(distance)}u).`,
        });
        this.sendMove({ ...target.position }, true);
      }
      return;
    }

    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.sendCommand(this.commandBuilder.lootPickup(target.entity_id));
  }

  private distanceToSelf(target: EntityPayload): number | null {
    const self = this.selfEntity();
    if (!self) {
      return null;
    }
    const serverNow = this.estimatedServerTime() ?? Date.now();
    const selfPosition = currentEntityPosition(self, serverNow);
    const targetPosition = currentEntityPosition(target, serverNow);
    return distanceBetween(selfPosition, targetPosition);
  }

  private selfEntity(): EntityPayload | null {
    return selectSelfEntity(this.state.visibleEntities);
  }

  private syncServerClock(): void {
    if (this.state.lastServerTime === null || this.state.lastServerTime === this.serverClockTime) {
      return;
    }
    this.serverClockOffsetValue = serverClockOffset(performance.now(), this.state.lastServerTime);
    this.serverClockTime = this.state.lastServerTime;
  }

  private estimatedServerTime(): number | null {
    if (this.serverClockOffsetValue === null) {
      return this.state.lastServerTime;
    }
    return estimateServerTime(performance.now(), this.serverClockOffsetValue);
  }

  private logAcceptedSelfMovement(): void {
    const self = this.selfEntity();
    const movement = self?.movement;
    if (!movement?.moving) {
      this.acceptedMovementSignature = null;
      return;
    }

    const signature = `${Math.round(movement.origin.x)},${Math.round(movement.origin.y)}:${Math.round(
      movement.target.x,
    )},${Math.round(movement.target.y)}:${movement.started_at_ms}:${movement.arrive_at_ms}`;
    if (signature === this.acceptedMovementSignature) {
      return;
    }

    this.acceptedMovementSignature = signature;
    const timing = movementTiming(movement, this.estimatedServerTime() ?? Date.now());
    this.dispatch({
      type: 'appendLog',
      level: 'info',
      text: `Route ${formatVec(movement.origin)} -> ${formatVec(movement.target)}, ${Math.round(timing.distance)}u, eta ${formatDuration(
        timing.remainingMs,
      )}.`,
    });
  }

  private scheduleCooldownRender(): void {
    const readyAt = this.state.skillCooldowns.basic_laser ?? 0;
    const delay = readyAt - Date.now();
    if (delay <= 0) {
      this.clearCooldownRenderTimer();
      return;
    }
    if (this.cooldownRenderTimer !== null && this.cooldownRenderAt === readyAt) {
      return;
    }

    this.clearCooldownRenderTimer();
    this.cooldownRenderAt = readyAt;
    this.cooldownRenderTimer = window.setTimeout(() => {
      this.cooldownRenderTimer = null;
      this.cooldownRenderAt = 0;
      this.render();
    }, Math.min(delay + 25, 2_147_483_647));
  }

  private clearCooldownRenderTimer(): void {
    if (this.cooldownRenderTimer === null) {
      return;
    }
    window.clearTimeout(this.cooldownRenderTimer);
    this.cooldownRenderTimer = null;
    this.cooldownRenderAt = 0;
  }

  private scheduleMovementRender(serverNow: number | null): void {
    const self = this.selfEntity();
    const timing = self && serverNow !== null ? activeEntityMovement(self, serverNow) : null;
    if (!timing) {
      this.clearMovementRenderTimer();
      return;
    }

    const wakeDelay = Math.min(MOVEMENT_ETA_RENDER_MS, Math.max(16, timing.remainingMs + 16));
    const wakeAt = Math.round(Date.now() + wakeDelay);
    if (this.movementRenderTimer !== null && Math.abs(this.movementRenderAt - wakeAt) < 24) {
      return;
    }

    this.clearMovementRenderTimer();
    this.movementRenderAt = wakeAt;
    this.movementRenderTimer = window.setTimeout(() => {
      this.movementRenderTimer = null;
      this.movementRenderAt = 0;
      this.render();
    }, wakeDelay);
  }

  private clearMovementRenderTimer(): void {
    if (this.movementRenderTimer === null) {
      return;
    }
    window.clearTimeout(this.movementRenderTimer);
    this.movementRenderTimer = null;
    this.movementRenderAt = 0;
  }

  private scheduleScanLoop(): void {
    const wakeAt = this.nextScanWakeAt();
    if (wakeAt === null) {
      this.clearScanTimer();
      return;
    }
    if (this.scanTimer !== null && Math.abs(this.scanTimerAt - wakeAt) < 20) {
      return;
    }

    this.clearScanTimer();
    this.scanTimerAt = wakeAt;
    this.scanTimer = window.setTimeout(() => {
      this.scanTimer = null;
      this.scanTimerAt = 0;
      this.runScanPulseIfDue();
    }, Math.min(Math.max(0, wakeAt - Date.now()), 2_147_483_647));
  }

  private clearScanTimer(): void {
    if (this.scanTimer === null) {
      return;
    }
    window.clearTimeout(this.scanTimer);
    this.scanTimer = null;
    this.scanTimerAt = 0;
  }

  private nextScanWakeAt(): number | null {
    if (!this.state.scanMode.enabled) {
      return null;
    }
    const now = Date.now();
    if (!this.scanRuntimeReady() || this.state.ship?.disabled === true) {
      return now + SCAN_PAUSED_RECHECK_MS;
    }
    if (this.hasPendingOperation(OPERATIONS.scanPulse)) {
      return now + SCAN_PENDING_RECHECK_MS;
    }
    if (this.scanPulseInProgress()) {
      return Math.max(now + SCAN_STARTED_RECHECK_MS, this.state.scanMode.nextPulseAt ?? 0);
    }
    return this.state.scanMode.nextPulseAt ?? now;
  }

  private runScanPulseIfDue(): void {
    if (!this.state.scanMode.enabled) {
      return;
    }

    const now = Date.now();
    if (!this.scanRuntimeReady()) {
      this.dispatch({ type: 'scanPulseScheduled', nextPulseAt: now + SCAN_PAUSED_RECHECK_MS, lastError: 'Realtime paused.' });
      return;
    }
    if (this.state.ship?.disabled === true) {
      this.dispatch({ type: 'scanPulseScheduled', nextPulseAt: now + SCAN_PAUSED_RECHECK_MS, lastError: 'Ship disabled.' });
      return;
    }
    if (this.hasPendingOperation(OPERATIONS.scanPulse)) {
      this.dispatch({ type: 'scanPulseScheduled', nextPulseAt: now + SCAN_PENDING_RECHECK_MS });
      return;
    }
    if (this.scanPulseInProgress()) {
      this.dispatch({ type: 'scanPulseScheduled', nextPulseAt: now + SCAN_STARTED_RECHECK_MS });
      return;
    }

    const dueAt = this.state.scanMode.nextPulseAt ?? now;
    if (dueAt > now + 20) {
      this.scheduleScanLoop();
      return;
    }

    this.dispatch({ type: 'scanPulseScheduled', nextPulseAt: now + SCAN_PENDING_RECHECK_MS, lastError: null });
    this.sendCommand(this.commandBuilder.scanPulse());
  }

  private scanRuntimeReady(): boolean {
    return this.state.auth.mode === 'demo' || this.state.connectionStatus === 'connected';
  }

  private hasPendingOperation(op: string): boolean {
    return Object.values(this.state.pendingCommands).some((command) => command.op === op);
  }

  private scanPulseInProgress(): boolean {
    return this.state.planetIntel?.lastScan?.status === 'started';
  }

  private async loadDemoState(): Promise<DemoStateModule> {
    if (import.meta.env.DEV) {
      this.demoState ??= await import('./demo-state');
      return this.demoState;
    }
    throw new Error('Demo fixture mode is available only in development builds.');
  }

  private publishSmokeState(serverNow: number | null = this.estimatedServerTime()): void {
    if (typeof window === 'undefined') {
      return;
    }
    const params = new URLSearchParams(window.location.search);
    if (!params.has('smoke')) {
      return;
    }
    const smokeWindow = window as Window & { __SPACE_MORPG_SMOKE_STATE__?: unknown };
    smokeWindow.__SPACE_MORPG_SMOKE_STATE__ = JSON.parse(
      JSON.stringify({
        connectionStatus: this.state.connectionStatus,
        lastServerTime: this.state.lastServerTime,
        lastSequence: this.state.lastSequence,
        playerSnapshot: this.state.playerSnapshot,
        sector: this.state.sector,
        minimap: this.state.minimap,
        visibleEntities: this.state.visibleEntities,
        selectedTargetID: this.state.selectedTargetID,
        knownLoot: this.state.knownLoot,
        worldEffects: this.state.worldEffects,
        cargo: this.state.cargo,
        wallet: this.state.wallet,
        ship: this.state.ship,
        stats: this.state.stats,
        progression: this.state.progression,
        inventory: this.state.inventory,
        hangar: this.state.hangar,
        loadout: this.state.loadout,
        crafting: this.state.crafting,
        planetIntel: this.state.planetIntel,
        scanMode: this.state.scanMode,
        production: this.state.production,
        routes: this.state.routes,
        market: this.state.market,
        auction: this.state.auction,
        premium: this.state.premium,
        questBoard: this.state.questBoard,
        economyDashboard: this.state.economyDashboard,
        adminInspection: this.state.adminInspection,
        adminRepair: this.state.adminRepair,
        commandLogSummary: this.state.commandLogSummary,
        metrics: this.state.metrics,
        releaseGate: this.state.releaseGate,
        abuseCoverage: this.state.abuseCoverage,
        repairQuote: this.state.repairQuote,
        skillCooldowns: this.state.skillCooldowns,
        commandLog: this.state.commandLog,
        combatLog: this.state.combatLog,
        auth: this.state.auth,
        serverNow,
        movementEta: movementEtaSmokeState(this.selfEntity(), serverNow),
        worldView: this.renderer.debugSnapshot(),
      }),
    );
  }

  private startSmokeStatePublisher(): void {
    if (typeof window === 'undefined' || this.smokeStateTimer !== null) {
      return;
    }
    const params = new URLSearchParams(window.location.search);
    if (!params.has('smoke')) {
      return;
    }
    this.smokeStateTimer = window.setInterval(() => this.publishSmokeState(), 50);
  }
}

function safeAuthMessage(error: unknown): string {
  if (error instanceof AuthClientError) {
    return error.message;
  }
  return 'Authentication failed.';
}

function isAuthError(message: ErrorEnvelope): boolean {
  return (
    message.error.code === 'ERR_AUTH_REQUIRED' ||
    message.error.code === 'ERR_SESSION_EXPIRED' ||
    message.error.code === 'ERR_SESSION_REVOKED' ||
    message.error.code === 'ERR_UNAUTHENTICATED'
  );
}

function entityLabel(entity: EntityPayload): string {
  return entity.display?.label || entity.entity_id;
}

function movementEtaFromStats(distance: number, speed: number | null): number | null {
  return speed && speed > 0 ? (distance / speed) * 1000 : null;
}

function movementEtaSmokeState(entity: EntityPayload | null, serverNow: number | null): unknown {
  if (!entity || serverNow === null) {
    return { active: false };
  }
  const timing = activeEntityMovement(entity, serverNow);
  if (!timing) {
    return { active: false };
  }
  return {
    active: true,
    origin: timing.origin,
    target: timing.target,
    current: timing.current,
    distance: timing.distance,
    remainingMs: timing.remainingMs,
    progress: timing.progress,
  };
}

function formatDuration(milliseconds: number | null): string {
  if (milliseconds === null) {
    return '--';
  }
  if (milliseconds < 1000) {
    return '<1s';
  }
  return `${(milliseconds / 1000).toFixed(milliseconds < 10_000 ? 1 : 0)}s`;
}

function formatVec(position: Vec2): string {
  return `${Math.round(position.x)},${Math.round(position.y)}`;
}

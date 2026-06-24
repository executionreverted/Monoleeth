import { AuthClientError } from '../auth/auth-client';
import { CLIENT_EVENTS, EntityPayload, ErrorEnvelope, EventEnvelope, OPERATIONS, ServerMessage } from '../protocol/envelope';
import { activeEntityMovement, currentEntityPosition, distanceBetween, estimateServerTime, movementTiming, selfEntity as selectSelfEntity, serverClockOffset } from '../state/movement';
import { reduceClientState } from '../state/reducer';
import { ClientAction, ClientState, PublicSession, WorldMapMemoryMarker } from '../state/types';
import { worldMapMemoryMarkers } from '../state/world-memory';
import { nextCycleTargetID } from './target-cycle';
import {
  MOVEMENT_ETA_RENDER_MS,
  NAVIGATION_RECHECK_MS,
  NAVIGATION_TARGET_TOLERANCE_UNITS,
  SCAN_ERROR_BACKOFF_MS,
  SCAN_PAUSED_RECHECK_MS,
  SCAN_PENDING_RECHECK_MS,
  SCAN_STARTED_RECHECK_MS,
  SHIELD_REPAIR_TICK_MS,
  TargetSelectionSource,
} from './client-app-core';
import { ClientAppCommands, formatDuration, formatVec } from './client-app-commands';

export abstract class ClientAppHandlers extends ClientAppCommands {
  protected applyServerMessage(message: ServerMessage): void {
    if ('event_id' in message) {
      this.dispatch({ type: 'eventReceived', envelope: message });
      this.clearPendingGameplayActionKeysForEvent(message);
      this.logAcceptedSelfMovement();
      this.handleEconomyRefreshForEvent(message.type);
      if (message.type === CLIENT_EVENTS.worldSnapshot) {
        this.requestSystemsSnapshotOnce();
      }
      return;
    }

    const pending = this.state.pendingCommands[message.request_id] ?? null;
    this.clearPendingGameplayActionKey(message.request_id);
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

  protected handleRealtimeStatus(status: ClientState['connectionStatus']): void {
    if (status === 'connected') {
      this.dispatch({
        type: 'connectionChanged',
        status: 'authenticated_pending_socket',
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

  }

  protected async restoreSession(): Promise<void> {
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

  protected async login(email: string, password: string): Promise<void> {
    this.dispatch({ type: 'authSubmitStarted' });
    try {
      this.startAuthenticatedSession(await this.authClient.login({ email, password }));
    } catch (error) {
      this.dispatch({ type: 'authFailed', message: safeAuthMessage(error) });
    }
  }

  protected async register(email: string, password: string, callsign: string): Promise<void> {
    this.dispatch({ type: 'authSubmitStarted' });
    try {
      this.startAuthenticatedSession(await this.authClient.register({ email, password, callsign }));
    } catch (error) {
      this.dispatch({ type: 'authFailed', message: safeAuthMessage(error) });
    }
  }

  protected async logout(): Promise<void> {
    this.intentionalDisconnect = true;
    this.cancelNavigation();
    this.clearEconomyRefreshTimer();
    this.clearPendingGameplayActionKeys();
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

  protected startAuthenticatedSession(session: PublicSession): void {
    if (!session.authenticated) {
      this.dispatch({ type: 'authLoggedOut' });
      return;
    }
    this.intentionalDisconnect = false;
    this.cancelNavigation();
    this.clearEconomyRefreshTimer();
    this.clearPendingGameplayActionKeys();
    this.systemsSnapshotRequested = false;
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.clearReconnectTimer();
    this.dispatch({ type: 'authSessionLoaded', session });
    this.realtime.connect(this.state.socketURL);
  }


  protected handleAuthExpired(message: string): void {
    this.intentionalDisconnect = true;
    this.cancelNavigation();
    this.clearEconomyRefreshTimer();
    this.clearPendingGameplayActionKeys();
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.clearReconnectTimer();
    this.realtime.disconnect();
    this.dispatch({ type: 'authExpired', message });
  }

  protected shouldReconnect(): boolean {
    return this.state.auth.mode === 'real' && Boolean(this.state.auth.session) && !this.intentionalDisconnect;
  }

  protected scheduleReconnect(): void {
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

  protected clearReconnectTimer(): void {
    if (this.reconnectTimer === null) {
      return;
    }
    window.clearTimeout(this.reconnectTimer);
    this.reconnectTimer = null;
  }

  protected clearEconomyRefreshTimer(): void {
    if (this.economyRefreshTimer !== null) {
      window.clearTimeout(this.economyRefreshTimer);
      this.economyRefreshTimer = null;
    }
    this.pendingEconomyRefreshOps.clear();
  }

  protected clearPendingGameplayActionKey(requestID: string): void {
    const actionKey = this.pendingGameplayActionKeysByRequest.get(requestID);
    if (!actionKey) {
      return;
    }
    this.pendingGameplayActionKeysByRequest.delete(requestID);
    this.pendingGameplayActionKeys.delete(actionKey);
  }

  protected clearPendingGameplayActionKeysForEvent(event: EventEnvelope): void {
    switch (event.type) {
      case CLIENT_EVENTS.marketListingCreated:
        this.clearPendingGameplayActionKeysByPrefix('market-create:');
        return;
      case CLIENT_EVENTS.marketSaleCompleted:
        this.clearPendingGameplayActionKeysByPrefix('market-buy:');
        return;
      case CLIENT_EVENTS.marketListingCancelled:
        this.clearPendingGameplayActionKeysByPrefix('market-cancel:');
        return;
      case CLIENT_EVENTS.auctionBidPlaced:
        this.clearPendingGameplayActionKeysByPrefix('auction-bid:');
        return;
      case CLIENT_EVENTS.auctionClosed:
        this.clearPendingGameplayActionKeysByPrefix('auction-bid:');
        this.clearPendingGameplayActionKeysByPrefix('auction-buy-now:');
        return;
      case CLIENT_EVENTS.premiumEntitlementClaimed:
        this.clearPendingGameplayActionKeysByPrefix('premium-claim:');
        return;
      case CLIENT_EVENTS.premiumStockConsumed:
        this.clearPendingGameplayActionKeysByPrefix('premium-weekly-xcore:');
        return;
      case CLIENT_EVENTS.mapTransferStarted:
      case CLIENT_EVENTS.mapTransferCompleted:
      case CLIENT_EVENTS.mapTransferFailed:
      case CLIENT_EVENTS.mapChanged:
        this.clearPendingGameplayActionKeysByPrefix('portal-enter:');
        return;
      case CLIENT_EVENTS.planetClaimed:
        this.clearPendingPlanetClaimActionKey(event);
        return;
      default:
        return;
    }
  }

  protected clearPendingPlanetClaimActionKey(event: EventEnvelope): void {
    const planet = event.payload.planet as Record<string, unknown> | undefined;
    const planetID =
      planet && typeof planet === 'object' && !Array.isArray(planet) && typeof planet.planet_id === 'string'
        ? planet.planet_id
        : '';
    if (planetID) {
      this.clearPendingGameplayActionKeyByValue(`planet-claim:${planetID}`);
    }
  }

  protected clearPendingGameplayActionKeyByValue(actionKey: string): void {
    if (!this.pendingGameplayActionKeys.delete(actionKey)) {
      return;
    }
    for (const [requestID, pendingActionKey] of [...this.pendingGameplayActionKeysByRequest.entries()]) {
      if (pendingActionKey === actionKey) {
        this.pendingGameplayActionKeysByRequest.delete(requestID);
      }
    }
  }

  protected clearPendingGameplayActionKeysByPrefix(prefix: string): void {
    for (const actionKey of [...this.pendingGameplayActionKeys]) {
      if (!actionKey.startsWith(prefix)) {
        continue;
      }
      this.pendingGameplayActionKeys.delete(actionKey);
      for (const [requestID, pendingActionKey] of [...this.pendingGameplayActionKeysByRequest.entries()]) {
        if (pendingActionKey === actionKey) {
          this.pendingGameplayActionKeysByRequest.delete(requestID);
        }
      }
    }
  }

  protected clearPendingGameplayActionKeys(): void {
    this.pendingGameplayActionKeys.clear();
    this.pendingGameplayActionKeysByRequest.clear();
  }

  protected dispatch(action: ClientAction): void {
    try {
      this.state = reduceClientState(this.state, action);
    } catch (error) {
      this.state = reduceClientState(this.state, {
        type: 'appendLog',
        level: 'error',
        text: error instanceof Error ? error.message : String(error),
      });
    }
    this.recordSmokePendingHistory(action);
    this.syncServerClock();
    this.render();
    this.tryPendingLootPickup();
    this.scheduleScanLoop();
    this.scheduleNavigationLoop();
  }

  protected render(): void {
    const serverNow = this.estimatedServerTime();
    this.root.dataset.mode = this.state.auth.mode;
    this.root.dataset.connection = this.state.connectionStatus;
    this.renderer.render({
      entities: Object.values(this.state.visibleEntities),
      currentMap: this.state.currentMap,
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

  protected findLocalPlayerID(): string | null {
    return (
      Object.values(this.state.visibleEntities).find((entity) => entity.status_flags?.includes('self'))?.entity_id ??
      Object.values(this.state.visibleEntities).find((entity) => entity.entity_type === 'player')?.entity_id ??
      null
    );
  }

  protected selectedTarget(): EntityPayload | null {
    return this.state.selectedTargetID ? this.state.visibleEntities[this.state.selectedTargetID] ?? null : null;
  }

  protected selfStealthEnabled(): boolean {
    return Object.values(this.state.visibleEntities).some((entity) => {
      const flags = entity.status_flags ?? [];
      return flags.includes('self') && flags.includes('stealthed');
    });
  }

  protected selectEntity(entityID: string | null, source: TargetSelectionSource = 'world'): void {
    const preservePendingLootApproach = source === 'world' && entityID === this.pendingLootPickupID;
    if (!preservePendingLootApproach) {
      this.pendingLootPickupID = null;
      this.pendingLootApproachID = null;
    }
    this.dispatch({ type: 'selectTarget', entityID });
    if (!entityID) {
      return;
    }

    const target = this.state.visibleEntities[entityID];
    if (target?.entity_type === 'loot') {
      if (source === 'world') {
        this.activateLootTarget(target, 'click');
      }
      return;
    }
    this.pendingLootPickupID = null;
  }

  protected cycleTarget(): void {
    const nextTargetID = nextCycleTargetID(this.state, this.estimatedServerTime());
    if (!nextTargetID) {
      return;
    }
    this.selectEntity(nextTargetID);
  }

  protected selectMemoryMarker(marker: WorldMapMemoryMarker): void {
    this.pendingLootPickupID = null;
    this.pendingLootApproachID = null;
    this.dispatch({ type: 'selectTarget', entityID: null });
    this.dispatch({ type: 'appendLog', level: 'info', text: `Selected known planet ${marker.label}.` });
    this.hud?.openPlanetDetailModal(marker.detailID);
    this.requestPlanetDetail(marker.detailID);
  }

  protected activateLootTarget(target: EntityPayload, source: 'click' | 'action'): void {
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

  protected tryPendingLootPickup(): void {
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

  protected distanceToSelf(target: EntityPayload): number | null {
    const self = this.selfEntity();
    if (!self) {
      return null;
    }
    const serverNow = this.estimatedServerTime() ?? Date.now();
    const selfPosition = currentEntityPosition(self, serverNow);
    const targetPosition = currentEntityPosition(target, serverNow);
    return distanceBetween(selfPosition, targetPosition);
  }

  protected selfEntity(): EntityPayload | null {
    return selectSelfEntity(this.state.visibleEntities);
  }

  protected syncServerClock(): void {
    if (this.state.lastServerTime === null || this.state.lastServerTime === this.serverClockTime) {
      return;
    }
    this.serverClockOffsetValue = serverClockOffset(performance.now(), this.state.lastServerTime);
    this.serverClockTime = this.state.lastServerTime;
  }

  protected estimatedServerTime(): number | null {
    if (this.serverClockOffsetValue === null) {
      return this.state.lastServerTime;
    }
    return estimateServerTime(performance.now(), this.serverClockOffsetValue);
  }

  protected logAcceptedSelfMovement(): void {
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

  protected scheduleCooldownRender(): void {
    const now = this.estimatedServerTime() ?? Date.now();
    const readyAt = nextCooldownRenderAt(this.state, now);
    const delay = readyAt - now;
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

  protected clearCooldownRenderTimer(): void {
    if (this.cooldownRenderTimer === null) {
      return;
    }
    window.clearTimeout(this.cooldownRenderTimer);
    this.cooldownRenderTimer = null;
    this.cooldownRenderAt = 0;
  }

  protected scheduleMovementRender(serverNow: number | null): void {
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

  protected clearMovementRenderTimer(): void {
    if (this.movementRenderTimer === null) {
      return;
    }
    window.clearTimeout(this.movementRenderTimer);
    this.movementRenderTimer = null;
    this.movementRenderAt = 0;
  }

  protected scheduleNavigationLoop(serverNow: number | null = this.estimatedServerTime()): void {
    if (!this.navigationTarget) {
      this.clearNavigationTimer();
      return;
    }
    if (this.state.auth.mode === 'real' && this.state.connectionStatus !== 'connected') {
      this.clearNavigationTimer();
      return;
    }
    if (this.hasPendingOperation(OPERATIONS.moveTo)) {
      this.scheduleNavigationWake(NAVIGATION_RECHECK_MS);
      return;
    }

    const self = this.selfEntity();
    if (!self || serverNow === null) {
      this.scheduleNavigationWake(NAVIGATION_RECHECK_MS);
      return;
    }

    const remainingDistance = distanceBetween(currentEntityPosition(self, serverNow), this.navigationTarget);
    if (remainingDistance <= NAVIGATION_TARGET_TOLERANCE_UNITS) {
      this.cancelNavigation();
      return;
    }

    const timing = activeEntityMovement(self, serverNow);
    this.scheduleNavigationWake(timing ? Math.max(NAVIGATION_RECHECK_MS, timing.remainingMs + NAVIGATION_RECHECK_MS) : NAVIGATION_RECHECK_MS);
  }

  protected scheduleNavigationWake(delay: number): void {
    const wakeDelay = Math.min(Math.max(16, delay), 2_147_483_647);
    const wakeAt = Math.round(Date.now() + wakeDelay);
    if (this.navigationTimer !== null && Math.abs(this.navigationTimerAt - wakeAt) < 24) {
      return;
    }

    this.clearNavigationTimer();
    this.navigationTimerAt = wakeAt;
    this.navigationTimer = window.setTimeout(() => {
      this.navigationTimer = null;
      this.navigationTimerAt = 0;
      this.sendNavigationStep();
    }, wakeDelay);
  }

  protected cancelNavigation(): void {
    this.navigationTarget = null;
    this.clearNavigationTimer();
  }

  protected clearNavigationTimer(): void {
    if (this.navigationTimer === null) {
      return;
    }
    window.clearTimeout(this.navigationTimer);
    this.navigationTimer = null;
    this.navigationTimerAt = 0;
  }

  protected scheduleScanLoop(): void {
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

  protected clearScanTimer(): void {
    if (this.scanTimer === null) {
      return;
    }
    window.clearTimeout(this.scanTimer);
    this.scanTimer = null;
    this.scanTimerAt = 0;
  }

  protected nextScanWakeAt(): number | null {
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

  protected runScanPulseIfDue(): void {
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

  protected scanRuntimeReady(): boolean {
    return this.state.connectionStatus === 'connected';
  }

  protected hasPendingOperation(op: string): boolean {
    return Object.values(this.state.pendingCommands).some((command) => command.op === op);
  }

  protected scanPulseInProgress(): boolean {
    return this.state.planetIntel?.lastScan?.status === 'started';
  }

  protected publishSmokeState(serverNow: number | null = this.estimatedServerTime()): void {
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
        mapSubscriptionEpoch: this.state.mapSubscriptionEpoch,
        mapTransfer: this.state.mapTransfer,
        currentMap: this.state.currentMap,
        portalCooldowns: this.state.portalCooldowns,
        playerSnapshot: this.state.playerSnapshot,
        sector: this.state.sector,
        minimap: this.state.minimap,
        visibleEntities: this.state.visibleEntities,
        selectedTargetID: this.state.selectedTargetID,
        movementTarget: this.state.movementTarget,
        lastCorrection: this.state.lastCorrection,
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
        shopCatalog: this.state.shopCatalog,
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
        pendingCommands: this.state.pendingCommands,
        pendingHistory: this.smokePendingHistory,
        commandLog: this.state.commandLog,
        combatLog: this.state.combatLog,
        auth: this.state.auth,
        navigationTarget: this.navigationTarget,
        serverNow,
        movementEta: movementEtaSmokeState(this.selfEntity(), serverNow),
        worldView: this.renderer.debugSnapshot(),
      }),
    );
  }

  protected startSmokeStatePublisher(): void {
    if (typeof window === 'undefined' || this.smokeStateTimer !== null) {
      return;
    }
    const params = new URLSearchParams(window.location.search);
    if (!params.has('smoke')) {
      return;
    }
    this.smokeStateTimer = window.setInterval(() => this.publishSmokeState(), 50);
  }

  protected startShieldRepairTicker(): void {
    if (typeof window === 'undefined' || this.shieldRepairTimer !== null) {
      return;
    }
    this.shieldRepairTimer = window.setInterval(() => this.sendShieldRepairTickIfNeeded(), SHIELD_REPAIR_TICK_MS);
  }

  protected sendShieldRepairTickIfNeeded(): void {
    if (this.state.auth.mode !== 'real' || this.state.connectionStatus !== 'connected') {
      return;
    }
    const ship = this.state.ship;
    if (!ship || ship.disabled || ship.shield >= ship.max_shield || ship.max_shield <= 0) {
      return;
    }
    if (!this.hasEquippedShieldRepairModule() || this.hasPendingOperation(OPERATIONS.shieldRepairTick)) {
      return;
    }
    this.sendCommand(this.commandBuilder.shieldRepairTick());
  }

  protected hasEquippedShieldRepairModule(): boolean {
    const slots = this.state.loadout?.slots ?? [];
    return slots.some((slot) => {
      const moduleID = slot.module_id || slot.module_item_id;
      return (
        moduleID === 'shield_generator_t1' &&
        slot.module_state !== 'broken' &&
        (slot.durability === undefined || slot.durability > 0)
      );
    });
  }

  protected recordSmokePendingHistory(action: ClientAction): void {
    if (typeof window === 'undefined' || !new URLSearchParams(window.location.search).has('smoke')) {
      return;
    }
    if (action.type === 'requestQueued') {
      this.smokePendingHistory.push({
        kind: 'queued',
        requestID: action.envelope.request_id,
        op: action.envelope.op,
        pendingCount: Object.keys(this.state.pendingCommands).length,
        at: Date.now(),
      });
    }
    if (action.type === 'authExpired') {
      this.smokePendingHistory.push({
        kind: 'auth_expired',
        pendingCount: Object.keys(this.state.pendingCommands).length,
        at: Date.now(),
      });
    }
    if (this.smokePendingHistory.length > 200) {
      this.smokePendingHistory.splice(0, this.smokePendingHistory.length - 200);
    }
  }}

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

function nextCooldownRenderAt(state: ClientState, now: number): number {
  const readyTimes = [
    state.skillCooldowns.basic_laser ?? 0,
    ...Object.values(state.portalCooldowns),
    ...(state.currentMap?.visible_portals.map((portal) => portal.cooldown_ready_at_ms ?? 0) ?? []),
    ...(state.minimap?.visible_portals?.map((portal) => portal.cooldown_ready_at_ms ?? 0) ?? []),
  ].filter((value) => Number.isFinite(value) && value > now);
  return readyTimes.length > 0 ? Math.min(...readyTimes) : 0;
}

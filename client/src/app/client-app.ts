import { AuthClient, AuthClientError } from '../auth/auth-client';
import { RealtimeClient } from '../net/realtime-client';
import { CommandBuilder } from '../protocol/commands';
import { CLIENT_EVENTS, ErrorEnvelope, RequestEnvelope, ServerMessage, Vec2 } from '../protocol/envelope';
import { WorldRenderer } from '../render/world-renderer';
import { AuthPanel } from '../ui/auth-panel';
import { HUD } from '../ui/hud';
import { correctionEvent, demoEvents } from './demo-state';
import { createInitialState, reduceClientState } from '../state/reducer';
import { ClientAction, ClientState, PublicSession } from '../state/types';

export class ClientApp {
  private state: ClientState = createInitialState();
  private readonly authClient = new AuthClient();
  private readonly commandBuilder = new CommandBuilder();
  private readonly renderer = new WorldRenderer({
    onMoveIntent: (target) => this.sendMove(target),
    onSelectTarget: (entityID) => this.dispatch({ type: 'selectTarget', entityID }),
  });
  private readonly realtime = new RealtimeClient({
    onStatus: (status) => this.handleRealtimeStatus(status),
    onMessage: (message) => this.applyServerMessage(message),
    onError: (message) => this.dispatch({ type: 'appendLog', level: 'error', text: message }),
  });
  private authPanel: AuthPanel | null = null;
  private hud: HUD | null = null;
  private reconnectTimer: number | null = null;
  private intentionalDisconnect = false;
  private systemsSnapshotRequested = false;
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
      onScan: () => this.sendCommand(this.commandBuilder.scanPulse()),
    });

    if (this.demoMode) {
      this.dispatch({ type: 'demoModeStarted' });
      this.seedDemoState();
      this.render();
      return;
    }

    this.render();
    await this.restoreSession();
  }

  private connect(url: string): void {
    this.intentionalDisconnect = false;
    this.dispatch({ type: 'replaceVisibleEntities', entities: [], serverTime: null });
    this.realtime.connect(url);
  }

  private disconnect(): void {
    this.intentionalDisconnect = true;
    this.systemsSnapshotRequested = false;
    this.clearReconnectTimer();
    this.realtime.disconnect();
    if (this.state.auth.mode === 'demo') {
      this.seedDemoState();
    }
  }

  private seedDemoState(): void {
    this.dispatch({ type: 'replaceVisibleEntities', entities: [], serverTime: null });
    for (const envelope of demoEvents()) {
      this.dispatch({ type: 'eventReceived', envelope });
    }
  }

  private sendMove(target: Vec2): void {
    const command = this.commandBuilder.moveTo(target);
    this.sendCommand(command);

    if (this.state.auth.mode === 'demo' && !this.realtime.isConnected()) {
      const localID = this.findLocalPlayerID();
      window.setTimeout(() => {
        this.dispatch({ type: 'eventReceived', envelope: correctionEvent(localID, target) });
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
    this.sendCommand(this.commandBuilder.lootPickup(target.entity_id));
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
      if (message.type === CLIENT_EVENTS.worldSnapshot) {
        this.requestSystemsSnapshotOnce();
      }
      return;
    }

    this.dispatch({ type: 'responseReceived', envelope: message });
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
    this.sendCommand(this.commandBuilder.hangarSnapshot());
    this.sendCommand(this.commandBuilder.loadoutSnapshot());
    this.sendCommand(this.commandBuilder.statsSnapshot());
    this.sendCommand(this.commandBuilder.craftingRecipes());
  }

  private handleAuthExpired(message: string): void {
    this.intentionalDisconnect = true;
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
    this.render();
  }

  private render(): void {
    this.root.dataset.mode = this.state.auth.mode;
    this.root.dataset.connection = this.state.connectionStatus;
    this.renderer.render({
      entities: Object.values(this.state.visibleEntities),
      sector: this.state.sector,
      minimap: this.state.minimap,
      selectedTargetID: this.state.selectedTargetID,
      movementTarget: this.state.movementTarget,
      lastCorrection: this.state.lastCorrection,
    });
    this.authPanel?.render(this.state);
    this.hud?.render(this.state);
    this.publishSmokeState();
  }

  private findLocalPlayerID(): string {
    return (
      Object.values(this.state.visibleEntities).find((entity) => entity.status_flags?.includes('self'))?.entity_id ??
      Object.values(this.state.visibleEntities).find((entity) => entity.entity_type === 'player')?.entity_id ??
      'player-local'
    );
  }

  private selectedTarget() {
    return this.state.selectedTargetID ? this.state.visibleEntities[this.state.selectedTargetID] ?? null : null;
  }

  private publishSmokeState(): void {
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
        cargo: this.state.cargo,
        wallet: this.state.wallet,
        ship: this.state.ship,
        stats: this.state.stats,
        progression: this.state.progression,
        inventory: this.state.inventory,
        hangar: this.state.hangar,
        loadout: this.state.loadout,
        crafting: this.state.crafting,
        repairQuote: this.state.repairQuote,
        skillCooldowns: this.state.skillCooldowns,
        commandLog: this.state.commandLog,
        combatLog: this.state.combatLog,
        auth: this.state.auth,
      }),
    );
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

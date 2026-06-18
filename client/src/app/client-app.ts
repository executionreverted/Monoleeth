import { RealtimeClient } from '../net/realtime-client';
import { CommandBuilder } from '../protocol/commands';
import { RequestEnvelope, ServerMessage, Vec2 } from '../protocol/envelope';
import { WorldRenderer } from '../render/world-renderer';
import { HUD } from '../ui/hud';
import { correctionEvent, demoEvents } from './demo-state';
import { createInitialState, reduceClientState } from '../state/reducer';
import { ClientAction, ClientState } from '../state/types';

export class ClientApp {
  private state: ClientState = createInitialState();
  private readonly commandBuilder = new CommandBuilder();
  private readonly renderer = new WorldRenderer({
    onMoveIntent: (target) => this.sendMove(target),
    onSelectTarget: (entityID) => this.dispatch({ type: 'selectTarget', entityID }),
  });
  private readonly realtime = new RealtimeClient({
    onStatus: (status) => this.dispatch({ type: 'connectionChanged', status }),
    onMessage: (message) => this.applyServerMessage(message),
    onError: (message) => this.dispatch({ type: 'appendLog', level: 'error', text: message }),
  });
  private hud: HUD | null = null;
  private demoMode = true;

  constructor(private readonly root: HTMLElement) {}

  async start(): Promise<void> {
    this.root.className = 'client-shell';
    this.root.innerHTML = `
      <main class="game-surface">
        <div class="world-host" aria-label="World view"></div>
        <div class="hud-host"></div>
      </main>
    `;

    const worldHost = this.root.querySelector<HTMLElement>('.world-host');
    const hudHost = this.root.querySelector<HTMLElement>('.hud-host');
    if (!worldHost || !hudHost) {
      throw new Error('Client shell failed to mount.');
    }

    await this.renderer.mount(worldHost);
    this.hud = new HUD(hudHost, {
      onConnect: (url) => this.connect(url),
      onDisconnect: () => this.disconnect(),
      onStop: () => this.sendCommand(this.commandBuilder.stop()),
      onDebugSnapshot: () => this.sendCommand(this.commandBuilder.debugSnapshot()),
    });

    this.seedDemoState();
    this.render();
  }

  private connect(url: string): void {
    this.demoMode = false;
    this.dispatch({ type: 'replaceVisibleEntities', entities: [], serverTime: null });
    this.realtime.connect(url);
  }

  private disconnect(): void {
    this.realtime.disconnect();
    this.demoMode = true;
    this.seedDemoState();
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

    if (this.demoMode && !this.realtime.isConnected()) {
      const localID = this.findLocalPlayerID();
      window.setTimeout(() => {
        this.dispatch({ type: 'eventReceived', envelope: correctionEvent(localID, target) });
      }, 120);
    }
  }

  private sendCommand(envelope: RequestEnvelope): void {
    this.dispatch({ type: 'requestQueued', envelope });
    if (!this.realtime.send(envelope)) {
      this.dispatch(
        this.demoMode
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
      return;
    }

    this.dispatch({ type: 'responseReceived', envelope: message });
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
    this.renderer.render({
      entities: Object.values(this.state.visibleEntities),
      selectedTargetID: this.state.selectedTargetID,
      movementTarget: this.state.movementTarget,
      lastCorrection: this.state.lastCorrection,
    });
    this.hud?.render(this.state);
  }

  private findLocalPlayerID(): string {
    return Object.values(this.state.visibleEntities).find((entity) => entity.entity_type === 'player')?.entity_id ?? 'player-local';
  }
}

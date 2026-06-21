import { AuthClient } from '../auth/auth-client';
import { RealtimeClient } from '../net/realtime-client';
import { CommandBuilder } from '../protocol/commands';
import { Operation, ServerMessage, Vec2 } from '../protocol/envelope';
import { WorldRenderer } from '../render/world-renderer';
import { AuthPanel } from '../ui/auth-panel';
import { HUD } from '../ui/hud';
import { createInitialState } from '../state/reducer';
import { ClientAction, ClientState, WorldMapMemoryMarker } from '../state/types';

export type DemoStateModule = typeof import('./demo-state');
export type TargetSelectionSource = 'world' | 'hud' | 'radar';

export const SCAN_PENDING_RECHECK_MS = 500;
export const SCAN_STARTED_RECHECK_MS = 350;
export const SCAN_PAUSED_RECHECK_MS = 750;
export const SCAN_ERROR_BACKOFF_MS = 3_200;
export const MOVEMENT_ETA_RENDER_MS = 100;
export const NAVIGATION_RECHECK_MS = 140;
export const NAVIGATION_TARGET_TOLERANCE_UNITS = 24;
export const ECONOMY_REFRESH_DEBOUNCE_MS = 50;

export abstract class ClientAppCore {
  protected state: ClientState = createInitialState();
  protected readonly authClient = new AuthClient();
  protected readonly commandBuilder = new CommandBuilder();
  protected readonly renderer = new WorldRenderer({
    onMoveIntent: (target) => this.handleWorldMoveIntent(target),
    onSelectTarget: (entityID) => this.selectEntity(entityID, 'world'),
    onSelectMemoryMarker: (marker) => this.selectMemoryMarker(marker),
  });
  protected readonly realtime = new RealtimeClient({
    onStatus: (status) => this.handleRealtimeStatus(status),
    onMessage: (message) => this.applyServerMessage(message),
    onError: (message) => this.dispatch({ type: 'appendLog', level: 'error', text: message }),
  });
  protected authPanel: AuthPanel | null = null;
  protected hud: HUD | null = null;
  protected reconnectTimer: number | null = null;
  protected smokeStateTimer: number | null = null;
  protected cooldownRenderTimer: number | null = null;
  protected cooldownRenderAt = 0;
  protected movementRenderTimer: number | null = null;
  protected movementRenderAt = 0;
  protected navigationTimer: number | null = null;
  protected navigationTimerAt = 0;
  protected navigationTarget: Vec2 | null = null;
  protected serverClockOffsetValue: number | null = null;
  protected serverClockTime: number | null = null;
  protected acceptedMovementSignature: string | null = null;
  protected scanTimer: number | null = null;
  protected scanTimerAt = 0;
  protected economyRefreshTimer: number | null = null;
  protected readonly pendingEconomyRefreshOps = new Set<Operation>();
  protected readonly pendingGameplayActionKeys = new Set<string>();
  protected readonly pendingGameplayActionKeysByRequest = new Map<string, string>();
  protected pendingLootPickupID: string | null = null;
  protected pendingLootApproachID: string | null = null;
  protected intentionalDisconnect = false;
  protected systemsSnapshotRequested = false;
  protected readonly smokePendingHistory: Array<{
    kind: 'queued' | 'auth_expired';
    requestID?: string;
    op?: Operation;
    pendingCount: number;
    at: number;
  }> = [];
  protected demoState: DemoStateModule | null = null;
  protected readonly demoMode = typeof window !== 'undefined' && new URLSearchParams(window.location.search).get('demo') === '1';

  protected constructor(protected readonly root: HTMLElement) {}

  protected abstract applyServerMessage(message: ServerMessage): void;
  protected abstract dispatch(action: ClientAction): void;
  protected abstract handleRealtimeStatus(status: ClientState['connectionStatus']): void;
  protected abstract handleWorldMoveIntent(target: Vec2): void;
  protected abstract selectEntity(entityID: string | null, source?: TargetSelectionSource): void;
  protected abstract selectMemoryMarker(marker: WorldMapMemoryMarker): void;
}

import { OPERATIONS } from '../protocol/envelope';
import { ClientAction, ClientState } from './types';
import {
  appendLog,
  clearGameplay,
  defaultSocketURL,
  initialCombatEngagement,
  initialScanMode,
  isVec2,
  newLog,
  objectField,
} from './reducer-helpers';
import { applyEvent } from './reducer-events';
import { applySnapshotPayload } from './reducer-snapshot';
import {
  applyCorrection,
  clearOriginMapLiveState,
  isStaleEvent,
  movementTargetFromAuthoritativeSelf,
  parseSnapshotEntities,
  replaceVisibleEntities,
  resetsRealtimeStream,
} from './reducer-world';

export function createInitialState(): ClientState {
  return {
    auth: {
      mode: 'real',
      session: null,
      submitting: false,
      error: null,
    },
    connectionStatus: 'restoring',
    socketURL: defaultSocketURL(),
    lastServerTime: null,
    lastSequence: 0,
    mapSubscriptionEpoch: null,
    mapTransfer: null,
    currentMap: null,
    portalCooldowns: {},
    playerSnapshot: null,
    sector: null,
    minimap: null,
    visibleEntities: {},
    selectedTargetID: null,
    movementTarget: null,
    lastCorrection: null,
    knownLoot: {},
    social: initialSocialState(),
    worldEffects: [],
    combatEngagement: initialCombatEngagement(),
    pendingCommands: {},
    commandLog: [],
    combatLog: [],
    cargo: null,
    wallet: null,
    ship: null,
    stats: null,
    progression: null,
    inventory: null,
    hangar: null,
    loadout: null,
    crafting: null,
    repairQuote: null,
    skillCooldowns: {},
    questBoard: null,
    planetIntel: null,
    scanMode: initialScanMode(),
    production: null,
    routes: null,
    routeSettlements: {},
    contentCatalog: null,
    shopCatalog: null,
    market: null,
    auction: null,
    premium: null,
    economyDashboard: null,
    adminInspection: null,
    adminRepair: null,
    adminContent: null,
    commandLogSummary: null,
    metrics: null,
    releaseGate: null,
    abuseCoverage: null,
    lastError: null,
  };
}

function initialSocialState() {
  return {
    chatMessages: [],
    party: null,
    pendingPartyInvite: null,
    clan: null,
    clanMembership: null,
    clanMembers: [],
    contributions: [],
  };
}

export function reduceClientState(state: ClientState, action: ClientAction): ClientState {
  switch (action.type) {
    case 'authRestoreStarted':
      return {
        ...clearGameplay(state),
        auth: {
          mode: 'real',
          session: null,
          submitting: false,
          error: null,
        },
        connectionStatus: 'restoring',
      };

    case 'authSubmitStarted':
      return {
        ...state,
        auth: {
          ...state.auth,
          submitting: true,
          error: null,
        },
        lastError: null,
      };

    case 'authSessionLoaded':
      return {
        ...clearGameplay(state),
        auth: {
          mode: 'real',
          session: action.session,
          submitting: false,
          error: null,
        },
        connectionStatus: 'authenticated_pending_socket',
        lastServerTime: action.session.server_time,
        commandLog: [newLog('info', 'Authenticated session restored.')],
      };

    case 'authLoggedOut':
      return {
        ...clearGameplay(state),
        auth: {
          mode: 'real',
          session: null,
          submitting: false,
          error: null,
        },
        connectionStatus: 'logged_out',
        commandLog: [newLog('info', 'Logged out.')],
      };

    case 'authExpired':
      return {
        ...clearGameplay(state),
        auth: {
          mode: 'real',
          session: null,
          submitting: false,
          error: action.message ?? 'Session expired. Please log in again.',
        },
        connectionStatus: 'auth_expired',
        commandLog: [newLog('warn', action.message ?? 'Session expired.')],
      };

    case 'authFailed':
      return {
        ...clearGameplay(state),
        auth: {
          mode: 'real',
          session: null,
          submitting: false,
          error: action.message,
        },
        connectionStatus: 'logged_out',
        commandLog: appendLog(state.commandLog, 'warn', action.message),
      };

    case 'connectionChanged':
      return {
        ...state,
        connectionStatus: action.status,
        socketURL: action.socketURL ?? state.socketURL,
        lastSequence: resetsRealtimeStream(action.status) ? 0 : state.lastSequence,
        commandLog: appendLog(state.commandLog, 'info', `Connection ${action.status}.`),
      };

    case 'requestQueued':
      return {
        ...state,
        movementTarget:
          action.envelope.op === 'move_to' && isVec2(action.envelope.payload.target)
            ? action.envelope.payload.target
            : state.movementTarget,
        pendingCommands: {
          ...state.pendingCommands,
          [action.envelope.request_id]: {
            requestID: action.envelope.request_id,
            op: action.envelope.op,
            queuedAt: Date.now(),
            payload: action.envelope.payload,
          },
        },
        commandLog: appendLog(state.commandLog, 'info', `Sent ${action.envelope.op}.`),
      };

    case 'scanModeToggled': {
      const enabled = action.enabled ?? !state.scanMode.enabled;
      return {
        ...state,
        scanMode: enabled
          ? {
              enabled: true,
              nextPulseAt: action.now ?? Date.now(),
              lastRejectedAt: null,
              lastError: null,
            }
          : initialScanMode(),
        commandLog: appendLog(state.commandLog, 'info', enabled ? 'Scanner automation enabled.' : 'Scanner automation disabled.'),
      };
    }

    case 'scanPulseScheduled':
      if (!state.scanMode.enabled) {
        return state;
      }
      return {
        ...state,
        scanMode: {
          ...state.scanMode,
          nextPulseAt: action.nextPulseAt,
          lastError: action.lastError === undefined ? state.scanMode.lastError : action.lastError,
        },
      };

    case 'scanPulseAccepted':
      if (!state.scanMode.enabled) {
        return state;
      }
      return {
        ...state,
        scanMode: {
          ...state.scanMode,
          nextPulseAt: action.nextPulseAt === undefined ? state.scanMode.nextPulseAt : action.nextPulseAt,
          lastError: null,
        },
      };

    case 'scanPulseRejected':
      if (!state.scanMode.enabled) {
        return state;
      }
      return {
        ...state,
        scanMode: {
          ...state.scanMode,
          nextPulseAt: action.backoffUntil,
          lastRejectedAt: action.rejectedAt ?? Date.now(),
          lastError: action.message,
        },
        commandLog: appendLog(state.commandLog, 'warn', `Scanner paused: ${action.message}`),
      };

    case 'responseReceived': {
      const pending = state.pendingCommands[action.envelope.request_id] ?? null;
      const pendingCommands = { ...state.pendingCommands };
      delete pendingCommands[action.envelope.request_id];
      if (action.envelope.ok === false) {
        const movementTarget =
          pending?.op === 'move_to' ? movementTargetFromAuthoritativeSelf(state.visibleEntities, null) : state.movementTarget;
        return {
          ...state,
          pendingCommands,
          movementTarget,
          lastError: action.envelope.error,
          lastServerTime: action.envelope.server_time,
          commandLog: appendLog(state.commandLog, 'error', action.envelope.error.message),
        };
      }

      const baseResponseState = {
        ...state,
        pendingCommands,
        lastServerTime: action.envelope.server_time,
        commandLog: appendLog(state.commandLog, 'info', `Accepted ${action.envelope.request_id}.`),
      };
      const isPortalTransferResponse = pending?.op === OPERATIONS.portalEnter;
      if (isPortalTransferResponse) {
        const snapshotPayload = objectField(action.envelope.payload, 'snapshot');
        if (!snapshotPayload) {
          return baseResponseState;
        }
        const snapshotEntities = parseSnapshotEntities(snapshotPayload);
        if (!snapshotEntities) {
          return baseResponseState;
        }
        const stateWithSnapshots = applySnapshotPayload(clearOriginMapLiveState(baseResponseState), snapshotPayload);
        return replaceVisibleEntities(
          { ...stateWithSnapshots, mapTransfer: null },
          snapshotEntities,
          action.envelope.server_time,
        );
      }

      const snapshotEntities = parseSnapshotEntities(action.envelope.payload);
      const stateWithSnapshots = applySnapshotPayload(
        baseResponseState,
        action.envelope.payload,
      );
      if (snapshotEntities) {
        return replaceVisibleEntities(
          stateWithSnapshots,
          snapshotEntities,
          action.envelope.server_time,
        );
      }

      return stateWithSnapshots;
    }

    case 'replaceVisibleEntities':
      return replaceVisibleEntities(
        state,
        action.entities,
        'serverTime' in action ? action.serverTime ?? null : state.lastServerTime,
        action.sequence,
      );

    case 'eventReceived':
      if (isStaleEvent(state, action.envelope)) {
        return state;
      }
      return applyEvent(state, action.envelope);

    case 'serverCorrection':
      return applyCorrection(state, action.entityID, action.position, undefined, action.serverTime ?? state.lastServerTime);

    case 'selectTarget':
      return {
        ...state,
        selectedTargetID: action.entityID,
      };

    case 'appendLog':
      return {
        ...state,
        commandLog: appendLog(state.commandLog, action.level, action.text),
      };
  }
}

import {
  CLIENT_EVENTS,
  EntityPayload,
  EventEnvelope,
  JsonObject,
  JsonValue,
  rejectForbiddenPayloadKeys,
  Vec2,
} from '../protocol/envelope';
import { CargoSummary, ClientAction, ClientState, LogLine, PublicSession, StatSummary, WalletSummary } from './types';

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
    playerSnapshot: null,
    visibleEntities: {},
    selectedTargetID: null,
    movementTarget: null,
    lastCorrection: null,
    pendingCommands: {},
    commandLog: [],
    combatLog: [],
    cargo: null,
    wallet: null,
    stats: null,
    questBoard: null,
    inventory: null,
    planetIntel: null,
    lastError: null,
  };
}

export function reduceClientState(state: ClientState, action: ClientAction): ClientState {
  switch (action.type) {
    case 'demoModeStarted':
      return {
        ...clearGameplay(state),
        auth: {
          mode: 'demo',
          session: null,
          submitting: false,
          error: null,
        },
        connectionStatus: 'offline',
        commandLog: [newLog('warn', 'Demo mode is using local fixture data.')],
      };

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
          },
        },
        commandLog: appendLog(state.commandLog, 'info', `Sent ${action.envelope.op}.`),
      };

    case 'responseReceived': {
      const pendingCommands = { ...state.pendingCommands };
      delete pendingCommands[action.envelope.request_id];
      if (action.envelope.ok === false) {
        return {
          ...state,
          pendingCommands,
          lastError: action.envelope.error,
          lastServerTime: action.envelope.server_time,
          commandLog: appendLog(state.commandLog, 'error', action.envelope.error.message),
        };
      }

      const snapshotEntities = parseSnapshotEntities(action.envelope.payload);
      const stateWithSnapshots = applySnapshotPayload(
        {
          ...state,
          pendingCommands,
          lastServerTime: action.envelope.server_time,
          commandLog: appendLog(state.commandLog, 'info', `Accepted ${action.envelope.request_id}.`),
        },
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
      return applyEvent(state, action.envelope);

    case 'serverCorrection':
      return applyCorrection(state, action.entityID, action.position, action.serverTime ?? state.lastServerTime);

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

function replaceVisibleEntities(
  state: ClientState,
  entities: EntityPayload[],
  serverTime: number | null,
  sequence?: number,
): ClientState {
  const visibleEntities: Record<string, EntityPayload> = {};
  for (const entity of entities) {
    rejectForbiddenPayloadKeys(entity);
    visibleEntities[entity.entity_id] = entity;
  }

  return {
    ...state,
    visibleEntities,
    selectedTargetID:
      state.selectedTargetID && visibleEntities[state.selectedTargetID] ? state.selectedTargetID : null,
    movementTarget: null,
    lastCorrection: null,
    planetIntel: { knownSignals: countPlanetSignals(visibleEntities), staleIntel: state.planetIntel?.staleIntel ?? null },
    lastServerTime: serverTime,
    lastSequence: sequence ? Math.max(state.lastSequence, sequence) : state.lastSequence,
  };
}

function applyEvent(state: ClientState, envelope: EventEnvelope): ClientState {
  rejectForbiddenPayloadKeys(envelope.payload);

  switch (envelope.type) {
    case CLIENT_EVENTS.sessionReady:
      return {
        ...state,
        auth: {
          mode: state.auth.mode,
          session: parseSessionReady(envelope.payload, envelope.server_time),
          submitting: false,
          error: null,
        },
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.playerSnapshot:
      return {
        ...state,
        playerSnapshot: { ...(state.playerSnapshot ?? {}), ...envelope.payload },
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.shipSnapshot:
      return {
        ...state,
        playerSnapshot: {
          ...(state.playerSnapshot ?? {}),
          hp: numberField(envelope.payload, 'hull') ?? state.playerSnapshot?.hp,
          max_hp: numberField(envelope.payload, 'max_hull') ?? state.playerSnapshot?.max_hp,
          shield: numberField(envelope.payload, 'shield') ?? state.playerSnapshot?.shield,
          max_shield: numberField(envelope.payload, 'max_shield') ?? state.playerSnapshot?.max_shield,
          energy: numberField(envelope.payload, 'capacitor') ?? state.playerSnapshot?.energy,
          max_energy: numberField(envelope.payload, 'max_capacitor') ?? state.playerSnapshot?.max_energy,
        },
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.cargoSnapshot:
      return {
        ...state,
        cargo: parseCargoSummary(envelope.payload, state.cargo),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.walletSnapshot:
      return {
        ...state,
        wallet: parseWalletSummary(envelope.payload, state.wallet),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.statsSnapshot:
      return {
        ...state,
        stats: parseStatSummary(envelope.payload, state.stats),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.worldSnapshot: {
      const entities = parseSnapshotEntities(envelope.payload) ?? [];
      return {
        ...replaceVisibleEntities(state, entities, envelope.server_time, envelope.seq),
        connectionStatus: state.auth.mode === 'real' && state.auth.session ? 'connected' : state.connectionStatus,
      };
    }

    case CLIENT_EVENTS.entityEntered:
    case CLIENT_EVENTS.entityUpdated: {
      const entity = parseEntityPayload(envelope.payload);
      const visibleEntities = {
        ...state.visibleEntities,
        [entity.entity_id]: entity,
      };
      return {
        ...state,
        visibleEntities,
        planetIntel:
          entity.entity_type === 'planet_signal_placeholder'
            ? { knownSignals: countPlanetSignals(visibleEntities), staleIntel: state.planetIntel?.staleIntel ?? null }
            : state.planetIntel,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.entityLeft: {
      const entityID = requireEntityID(envelope.payload);
      const visibleEntities = { ...state.visibleEntities };
      delete visibleEntities[entityID];
      return {
        ...state,
        visibleEntities,
        selectedTargetID: state.selectedTargetID === entityID ? null : state.selectedTargetID,
        planetIntel: { knownSignals: countPlanetSignals(visibleEntities), staleIntel: state.planetIntel?.staleIntel ?? null },
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.positionCorrected: {
      const entityID = requireEntityID(envelope.payload);
      const position = requirePosition(envelope.payload);
      return applyCorrection(state, entityID, position, envelope.server_time, envelope.seq);
    }

    case CLIENT_EVENTS.serverNotice:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', stringField(envelope.payload, 'message') ?? 'Server notice.'),
      };

    default:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'warn', `Unhandled event ${envelope.type}.`),
      };
  }
}

function applyCorrection(
  state: ClientState,
  entityID: string,
  position: Vec2,
  serverTime: number | null,
  sequence?: number,
): ClientState {
  const entity = state.visibleEntities[entityID];
  const visibleEntities = entity
    ? {
        ...state.visibleEntities,
        [entityID]: {
          ...entity,
          position,
        },
      }
    : state.visibleEntities;

  return {
    ...state,
    visibleEntities,
    movementTarget: null,
    lastCorrection: { entityID, position },
    lastServerTime: serverTime,
    lastSequence: sequence ? Math.max(state.lastSequence, sequence) : state.lastSequence,
  };
}

function parseSnapshotEntities(payload: JsonObject): EntityPayload[] | null {
  if (!('entities' in payload)) {
    return null;
  }

  rejectForbiddenPayloadKeys(payload);

  if (!Array.isArray(payload.entities)) {
    throw new Error('Snapshot entities must be an array.');
  }

  return payload.entities.map((entity) => {
    if (!isJsonObject(entity)) {
      throw new Error('Snapshot entity must be an object.');
    }
    return parseEntityPayload(entity);
  });
}

function applySnapshotPayload(state: ClientState, payload: JsonObject): ClientState {
  rejectForbiddenPayloadKeys(payload);

  let next = state;
  if (typeof payload.authenticated === 'boolean') {
    next = {
      ...next,
      auth: {
        ...next.auth,
        session: parseSessionReady(payload, state.lastServerTime ?? Date.now()),
        submitting: false,
        error: null,
      },
    };
  }

  const player = objectField(payload, 'player') ?? objectField(payload, 'player_snapshot');
  if (player) {
    next = {
      ...next,
      playerSnapshot: { ...(next.playerSnapshot ?? {}), ...player },
    };
  }

  const cargo = objectField(payload, 'cargo') ?? objectField(payload, 'cargo_snapshot');
  if (cargo) {
    next = {
      ...next,
      cargo: parseCargoSummary(cargo, next.cargo),
    };
  }

  const wallet = objectField(payload, 'wallet') ?? objectField(payload, 'wallet_snapshot');
  if (wallet) {
    next = {
      ...next,
      wallet: parseWalletSummary(wallet, next.wallet),
    };
  }

  const stats = objectField(payload, 'stats') ?? objectField(payload, 'stat_snapshot');
  if (stats) {
    next = {
      ...next,
      stats: parseStatSummary(stats, next.stats),
    };
  }

  return next;
}

function parseEntityPayload(payload: JsonObject): EntityPayload {
  const source = isJsonObject(payload.entity) ? payload.entity : payload;
  const entityID = typeof source.entity_id === 'string' ? source.entity_id : '';
  const entityType = typeof source.entity_type === 'string' ? source.entity_type : '';
  const position = isVec2(source.position) ? source.position : null;

  if (!entityID || !isKnownEntityType(entityType) || !position) {
    throw new Error('Invalid entity payload.');
  }

  return {
    entity_id: entityID,
    entity_type: entityType,
    position,
    status_flags: Array.isArray(source.status_flags)
      ? source.status_flags.filter((flag): flag is string => typeof flag === 'string')
      : undefined,
  };
}

function requireEntityID(payload: JsonObject): string {
  if (typeof payload.entity_id === 'string') {
    return payload.entity_id;
  }
  if (typeof payload.id === 'string') {
    return payload.id;
  }
  throw new Error('Missing entity id.');
}

function requirePosition(payload: JsonObject): Vec2 {
  if (isVec2(payload.position)) {
    return payload.position;
  }
  throw new Error('Missing correction position.');
}

function parseSessionReady(payload: JsonObject, serverTime: number): PublicSession {
  const account = objectField(payload, 'account');
  const player = objectField(payload, 'player');
  const roles = Array.isArray(payload.roles) ? payload.roles.filter((role): role is string => typeof role === 'string') : undefined;
  return {
    authenticated: payload.authenticated === true,
    account: account
      ? {
          email: stringField(account, 'email') ?? '',
          admin: booleanField(account, 'admin') ?? false,
        }
      : undefined,
    player: player ? { callsign: stringField(player, 'callsign') ?? '' } : undefined,
    roles,
    expires_at: numberField(payload, 'expires_at') ?? undefined,
    server_time: serverTime,
  };
}

function parseCargoSummary(payload: JsonObject, fallback: CargoSummary | null): CargoSummary {
  const used = numberField(payload, 'used') ?? fallback?.used ?? 0;
  const capacity = numberField(payload, 'capacity') ?? numberField(payload, 'cargo_capacity') ?? fallback?.capacity ?? 0;
  const rawItems = Array.isArray(payload.items) ? payload.items : fallback?.items ?? [];
  const items = rawItems
    .filter(isJsonObject)
    .map((item) => ({
      item_id: stringField(item, 'item_id') ?? '',
      quantity: numberField(item, 'quantity') ?? 0,
    }))
    .filter((item) => item.item_id !== '' && item.quantity > 0);

  return {
    used: Math.max(0, Math.round(used)),
    capacity: Math.max(0, Math.round(capacity)),
    items,
  };
}

function parseWalletSummary(payload: JsonObject, fallback: WalletSummary | null): WalletSummary {
  return {
    credits: Math.max(0, Math.round(numberField(payload, 'credits') ?? fallback?.credits ?? 0)),
    premium_paid: Math.max(0, Math.round(numberField(payload, 'premium_paid') ?? fallback?.premium_paid ?? 0)),
    premium_earned: Math.max(0, Math.round(numberField(payload, 'premium_earned') ?? fallback?.premium_earned ?? 0)),
  };
}

function parseStatSummary(payload: JsonObject, fallback: StatSummary | null): StatSummary {
  return {
    speed: Math.max(0, numberField(payload, 'speed') ?? fallback?.speed ?? 0),
    radar_range: Math.max(0, numberField(payload, 'radar_range') ?? fallback?.radar_range ?? 0),
    weapon_range: Math.max(0, numberField(payload, 'weapon_range') ?? fallback?.weapon_range ?? 0),
    cargo_capacity: Math.max(0, numberField(payload, 'cargo_capacity') ?? fallback?.cargo_capacity ?? 0),
  };
}

function isVec2(value: JsonValue | unknown): value is Vec2 {
  return (
    typeof value === 'object' &&
    value !== null &&
    !Array.isArray(value) &&
    typeof (value as Vec2).x === 'number' &&
    Number.isFinite((value as Vec2).x) &&
    typeof (value as Vec2).y === 'number' &&
    Number.isFinite((value as Vec2).y)
  );
}

function isJsonObject(value: JsonValue | unknown): value is JsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function objectField(payload: JsonObject, key: string): JsonObject | null {
  const value = payload[key];
  return isJsonObject(value) ? value : null;
}

function stringField(payload: JsonObject, key: string): string | null {
  const value = payload[key];
  return typeof value === 'string' ? value : null;
}

function numberField(payload: JsonObject, key: string): number | null {
  const value = payload[key];
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

function booleanField(payload: JsonObject, key: string): boolean | null {
  const value = payload[key];
  return typeof value === 'boolean' ? value : null;
}

function isKnownEntityType(entityType: string): entityType is EntityPayload['entity_type'] {
  return (
    entityType === 'player' ||
    entityType === 'npc_placeholder' ||
    entityType === 'loot_placeholder' ||
    entityType === 'planet_signal_placeholder'
  );
}

function countPlanetSignals(entities: Record<string, EntityPayload>): number {
  return Object.values(entities).filter((entity) => entity.entity_type === 'planet_signal_placeholder').length;
}

function appendLog(lines: LogLine[], level: LogLine['level'], text: string): LogLine[] {
  return [...lines.slice(-39), newLog(level, text)];
}

function clearGameplay(state: ClientState): ClientState {
  return {
    ...state,
    lastServerTime: null,
    lastSequence: 0,
    playerSnapshot: null,
    visibleEntities: {},
    selectedTargetID: null,
    movementTarget: null,
    lastCorrection: null,
    pendingCommands: {},
    commandLog: [],
    combatLog: [],
    cargo: null,
    wallet: null,
    stats: null,
    questBoard: null,
    inventory: null,
    planetIntel: null,
    lastError: null,
  };
}

function newLog(level: LogLine['level'], text: string): LogLine {
  return {
    id: `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`,
    level,
    text,
    at: Date.now(),
  };
}

function defaultSocketURL(): string {
  if (typeof window === 'undefined') {
    return 'ws://127.0.0.1:5173/ws';
  }

  const { protocol, host } = window.location;
  const wsProtocol = protocol === 'https:' ? 'wss:' : 'ws:';
  return `${wsProtocol}//${host}/ws`;
}

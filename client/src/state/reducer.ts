import {
  CLIENT_EVENTS,
  EntityPayload,
  EventEnvelope,
  JsonObject,
  JsonValue,
  rejectForbiddenPayloadKeys,
  Vec2,
} from '../protocol/envelope';
import { ClientAction, ClientState, LogLine } from './types';

export function createInitialState(): ClientState {
  return {
    connectionStatus: 'offline',
    socketURL: defaultSocketURL(),
    lastServerTime: null,
    lastSequence: 0,
    playerSnapshot: {
      hp: 84,
      shield: 61,
      energy: 72,
      max_hp: 100,
      max_shield: 100,
      max_energy: 100,
      rank: 1,
      callsign: 'Frontier-01',
    },
    visibleEntities: {},
    selectedTargetID: null,
    movementTarget: null,
    lastCorrection: null,
    pendingCommands: {},
    commandLog: [],
    combatLog: [
      newLog('info', 'Systems nominal.'),
      newLog('info', 'AOI filter armed.'),
    ],
    cargo: {
      used: 17,
      capacity: 60,
      items: [
        { item_id: 'raw_ore', quantity: 11 },
        { item_id: 'salvage_thread', quantity: 6 },
      ],
    },
    questBoard: { available: 3, active: 1 },
    inventory: { equipped: 4, storage: 12 },
    planetIntel: { knownSignals: 1, staleIntel: 0 },
    lastError: null,
  };
}

export function reduceClientState(state: ClientState, action: ClientAction): ClientState {
  switch (action.type) {
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
      if (snapshotEntities) {
        return replaceVisibleEntities(
          {
            ...state,
            pendingCommands,
            lastServerTime: action.envelope.server_time,
            commandLog: appendLog(state.commandLog, 'info', `Accepted ${action.envelope.request_id}.`),
          },
          snapshotEntities,
          action.envelope.server_time,
        );
      }

      return {
        ...state,
        pendingCommands,
        lastServerTime: action.envelope.server_time,
        commandLog: appendLog(state.commandLog, 'info', `Accepted ${action.envelope.request_id}.`),
      };
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
    planetIntel: { ...state.planetIntel, knownSignals: countPlanetSignals(visibleEntities) },
    lastServerTime: serverTime,
    lastSequence: sequence ? Math.max(state.lastSequence, sequence) : state.lastSequence,
  };
}

function applyEvent(state: ClientState, envelope: EventEnvelope): ClientState {
  rejectForbiddenPayloadKeys(envelope.payload);

  switch (envelope.type) {
    case CLIENT_EVENTS.playerSnapshot:
      return {
        ...state,
        playerSnapshot: { ...state.playerSnapshot, ...envelope.payload },
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.entityEntered: {
      const entity = parseEntityPayload(envelope.payload);
      return {
        ...state,
        visibleEntities: {
          ...state.visibleEntities,
          [entity.entity_id]: entity,
        },
        planetIntel:
          entity.entity_type === 'planet_signal_placeholder'
            ? { ...state.planetIntel, knownSignals: countPlanetSignals({ ...state.visibleEntities, [entity.entity_id]: entity }) }
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
        planetIntel: { ...state.planetIntel, knownSignals: countPlanetSignals(visibleEntities) },
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.positionCorrected: {
      const entityID = requireEntityID(envelope.payload);
      const position = requirePosition(envelope.payload);
      return applyCorrection(state, entityID, position, envelope.server_time, envelope.seq);
    }

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

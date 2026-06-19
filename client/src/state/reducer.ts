import {
  CLIENT_EVENTS,
  EntityPayload,
  EventEnvelope,
  JsonObject,
  JsonValue,
  rejectForbiddenPayloadKeys,
  Vec2,
} from '../protocol/envelope';
import {
  CargoSummary,
  ClientAction,
  ClientState,
  LogLine,
  MinimapContact,
  MinimapMemory,
  MinimapSummary,
  ProgressionSummary,
  PublicSession,
  RepairQuote,
  SectorSummary,
  ShipSummary,
  StatSummary,
  WalletSummary,
} from './types';

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
    sector: null,
    minimap: null,
    visibleEntities: {},
    selectedTargetID: null,
    movementTarget: null,
    lastCorrection: null,
    pendingCommands: {},
    commandLog: [],
    combatLog: [],
    cargo: null,
    wallet: null,
    ship: null,
    stats: null,
    progression: null,
    repairQuote: null,
    skillCooldowns: {},
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
    planetIntel: {
      knownSignals: countPlanetSignals(visibleEntities),
      staleIntel: state.planetIntel?.staleIntel ?? null,
    },
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
        ship: parseShipSummary(envelope.payload, state.ship),
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

    case CLIENT_EVENTS.targetUpdated:
      return applyTargetUpdated(state, envelope);

    case CLIENT_EVENTS.combatDamage:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        combatLog: appendLog(
          state.combatLog,
          'info',
          `Hit ${stringField(envelope.payload, 'target_id') ?? 'target'} for ${Math.round(numberField(envelope.payload, 'amount') ?? 0)}.`,
        ),
      };

    case CLIENT_EVENTS.combatMiss:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        combatLog: appendLog(state.combatLog, 'warn', `Missed ${stringField(envelope.payload, 'target_id') ?? 'target'}.`),
      };

    case CLIENT_EVENTS.combatCooldownStarted: {
      const skillID = stringField(envelope.payload, 'skill_id') ?? 'basic_laser';
      const readyAt = numberField(envelope.payload, 'cooldown_ready_at_ms') ?? envelope.server_time;
      return {
        ...state,
        skillCooldowns: { ...state.skillCooldowns, [skillID]: readyAt },
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.combatNPCKilled:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        combatLog: appendLog(state.combatLog, 'info', `${stringField(envelope.payload, 'npc_type') ?? 'Hostile'} destroyed.`),
      };

    case CLIENT_EVENTS.lootCreated:
    case CLIENT_EVENTS.lootUpdated:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        combatLog: appendLog(
          state.combatLog,
          'info',
          `Drop ${stringField(envelope.payload, 'item_id') ?? 'item'} x${Math.round(numberField(envelope.payload, 'quantity') ?? 0)}.`,
        ),
      };

    case CLIENT_EVENTS.lootRemoved: {
      const entityID = requireEntityID(envelope.payload);
      const visibleEntities = { ...state.visibleEntities };
      delete visibleEntities[entityID];
      return {
        ...state,
        visibleEntities,
        selectedTargetID: state.selectedTargetID === entityID ? null : state.selectedTargetID,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.lootPickedUp:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        combatLog: appendLog(
          state.combatLog,
          'info',
          `Recovered ${stringField(envelope.payload, 'item_id') ?? 'item'} x${Math.round(numberField(envelope.payload, 'quantity') ?? 0)}.`,
        ),
      };

    case CLIENT_EVENTS.progressionSnapshot:
      return {
        ...state,
        progression: parseProgressionSummary(envelope.payload, state.progression),
        playerSnapshot: {
          ...(state.playerSnapshot ?? {}),
          rank: numberField(envelope.payload, 'rank') ?? state.playerSnapshot?.rank,
        },
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.deathShipDisabled:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        combatLog: appendLog(state.combatLog, 'error', 'Ship disabled.'),
      };

    case CLIENT_EVENTS.deathRepaired:
      return {
        ...state,
        repairQuote: null,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        combatLog: appendLog(state.combatLog, 'info', 'Ship repaired.'),
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
      const withSnapshotPayload = applySnapshotPayload(state, envelope.payload);
      return {
        ...replaceVisibleEntities(withSnapshotPayload, entities, envelope.server_time, envelope.seq),
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
          entity.entity_type === 'planet_signal'
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

    case CLIENT_EVENTS.movementStopped:
      return {
        ...state,
        movementTarget: null,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

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

function applyTargetUpdated(state: ClientState, envelope: EventEnvelope): ClientState {
  const entityID = requireEntityID(envelope.payload);
  const combat = parseEntityCombat(envelope.payload);
  const entity = state.visibleEntities[entityID];
  const visibleEntities =
    entity && combat
      ? {
          ...state.visibleEntities,
          [entityID]: {
            ...entity,
            combat,
          },
        }
      : state.visibleEntities;

  return {
    ...state,
    visibleEntities,
    lastServerTime: envelope.server_time,
    lastSequence: Math.max(state.lastSequence, envelope.seq),
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

  const ship = objectField(payload, 'ship') ?? objectField(payload, 'ship_snapshot');
  if (ship) {
    const parsedShip = parseShipSummary(ship, next.ship);
    next = {
      ...next,
      ship: parsedShip,
      playerSnapshot: {
        ...(next.playerSnapshot ?? {}),
        hp: parsedShip.hull,
        max_hp: parsedShip.max_hull,
        shield: parsedShip.shield,
        max_shield: parsedShip.max_shield,
        energy: parsedShip.capacitor,
        max_energy: parsedShip.max_capacitor,
      },
    };
  }

  const progression = objectField(payload, 'progression') ?? objectField(payload, 'progression_snapshot');
  if (progression) {
    next = {
      ...next,
      progression: parseProgressionSummary(progression, next.progression),
    };
  }

  const quote = objectField(payload, 'repair_quote') ?? (typeof payload.cost === 'number' && typeof payload.ship_id === 'string' ? payload : null);
  if (quote) {
    next = {
      ...next,
      repairQuote: parseRepairQuote(quote, next.repairQuote),
    };
  }

  const sector = objectField(payload, 'sector');
  if (sector) {
    next = {
      ...next,
      sector: parseSectorSummary(sector, next.sector),
    };
  }

  const minimap = objectField(payload, 'minimap');
  if (minimap) {
    next = {
      ...next,
      minimap: parseMinimapSummary(minimap, next.minimap),
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
    display: parseEntityDisplay(source),
    combat: parseEntityCombat(source),
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

function parseShipSummary(payload: JsonObject, fallback: ShipSummary | null): ShipSummary {
  return {
    active_ship_id: stringField(payload, 'active_ship_id') ?? fallback?.active_ship_id ?? '',
    display_name: stringField(payload, 'display_name') ?? fallback?.display_name ?? '',
    hull: Math.max(0, Math.round(numberField(payload, 'hull') ?? fallback?.hull ?? 0)),
    max_hull: Math.max(0, Math.round(numberField(payload, 'max_hull') ?? fallback?.max_hull ?? 0)),
    shield: Math.max(0, Math.round(numberField(payload, 'shield') ?? fallback?.shield ?? 0)),
    max_shield: Math.max(0, Math.round(numberField(payload, 'max_shield') ?? fallback?.max_shield ?? 0)),
    capacitor: Math.max(0, Math.round(numberField(payload, 'capacitor') ?? fallback?.capacitor ?? 0)),
    max_capacitor: Math.max(0, Math.round(numberField(payload, 'max_capacitor') ?? fallback?.max_capacitor ?? 0)),
    disabled: booleanField(payload, 'disabled') ?? fallback?.disabled ?? false,
    repair_state: stringField(payload, 'repair_state') ?? fallback?.repair_state ?? '',
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

function parseProgressionSummary(payload: JsonObject, fallback: ProgressionSummary | null): ProgressionSummary {
  return {
    main_level: Math.max(0, Math.round(numberField(payload, 'main_level') ?? fallback?.main_level ?? 0)),
    main_xp: Math.max(0, Math.round(numberField(payload, 'main_xp') ?? fallback?.main_xp ?? 0)),
    rank: Math.max(0, Math.round(numberField(payload, 'rank') ?? fallback?.rank ?? 0)),
    combat_level: optionalRoundedNumber(payload, 'combat_level', fallback?.combat_level),
    combat_xp: optionalRoundedNumber(payload, 'combat_xp', fallback?.combat_xp),
  };
}

function parseRepairQuote(payload: JsonObject, fallback: RepairQuote | null): RepairQuote {
  return {
    ship_id: stringField(payload, 'ship_id') ?? fallback?.ship_id ?? '',
    currency: stringField(payload, 'currency') ?? fallback?.currency ?? 'credits',
    cost: Math.max(0, Math.round(numberField(payload, 'cost') ?? fallback?.cost ?? 0)),
    disabled: booleanField(payload, 'disabled') ?? fallback?.disabled ?? false,
  };
}

function parseSectorSummary(payload: JsonObject, fallback: SectorSummary | null): SectorSummary {
  return {
    name: stringField(payload, 'name') ?? fallback?.name ?? '',
    region: stringField(payload, 'region') ?? fallback?.region ?? '',
    danger: stringField(payload, 'danger') ?? fallback?.danger ?? '',
    contested: booleanField(payload, 'contested') ?? fallback?.contested ?? false,
  };
}

function parseMinimapSummary(payload: JsonObject, fallback: MinimapSummary | null): MinimapSummary {
  const liveContacts = Array.isArray(payload.live_contacts)
    ? payload.live_contacts
        .filter(isJsonObject)
        .map(parseMinimapContact)
        .filter((contact): contact is MinimapContact => contact !== null)
    : fallback?.live_contacts ?? [];
  const remembered = Array.isArray(payload.remembered)
    ? payload.remembered.filter(isJsonObject).map(parseMinimapMemory)
    : fallback?.remembered ?? [];

  return {
    radar_range: Math.max(0, numberField(payload, 'radar_range') ?? fallback?.radar_range ?? 0),
    live_contacts: liveContacts,
    remembered,
  };
}

function parseMinimapContact(payload: JsonObject): MinimapContact | null {
  const entityType = stringField(payload, 'entity_type') ?? '';
  const entityID = stringField(payload, 'entity_id') ?? '';
  const position = isVec2(payload.position) ? payload.position : null;
  if (!entityID || !isKnownEntityType(entityType) || !position) {
    return null;
  }
  return {
    entity_id: entityID,
    entity_type: entityType,
    position,
    disposition: stringField(payload, 'disposition') ?? undefined,
    status_flags: Array.isArray(payload.status_flags)
      ? payload.status_flags.filter((flag): flag is string => typeof flag === 'string')
      : undefined,
  };
}

function parseMinimapMemory(payload: JsonObject): MinimapMemory {
  return {
    kind: stringField(payload, 'kind') ?? '',
    label: stringField(payload, 'label') ?? '',
    position: isVec2(payload.position) ? payload.position : { x: 0, y: 0 },
    freshness: stringField(payload, 'freshness') ?? '',
  };
}

function parseEntityDisplay(payload: JsonObject): EntityPayload['display'] {
  const display = objectField(payload, 'display');
  if (!display) {
    return undefined;
  }
  const label = stringField(display, 'label') ?? undefined;
  const disposition = stringField(display, 'disposition') ?? undefined;
  return label || disposition ? { label, disposition } : undefined;
}

function parseEntityCombat(payload: JsonObject): EntityPayload['combat'] {
  const combat = objectField(payload, 'combat') ?? payload;
  const hp = numberField(combat, 'hp');
  const maxHP = numberField(combat, 'max_hp');
  const shield = numberField(combat, 'shield');
  const maxShield = numberField(combat, 'max_shield');
  if (hp === null && maxHP === null && shield === null && maxShield === null) {
    return undefined;
  }
  return {
    hp: Math.max(0, Math.round(hp ?? 0)),
    max_hp: Math.max(0, Math.round(maxHP ?? 0)),
    shield: Math.max(0, Math.round(shield ?? 0)),
    max_shield: Math.max(0, Math.round(maxShield ?? 0)),
    status: stringField(combat, 'status') ?? undefined,
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

function optionalRoundedNumber(payload: JsonObject, key: string, fallback: number | undefined): number | undefined {
  const value = numberField(payload, key);
  if (value === null) {
    return fallback;
  }
  return Math.max(0, Math.round(value));
}

function isKnownEntityType(entityType: string): entityType is EntityPayload['entity_type'] {
  return (
    entityType === 'player' ||
    entityType === 'npc' ||
    entityType === 'loot' ||
    entityType === 'planet_signal'
  );
}

function countPlanetSignals(entities: Record<string, EntityPayload>): number {
  return Object.values(entities).filter((entity) => entity.entity_type === 'planet_signal').length;
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
    sector: null,
    minimap: null,
    visibleEntities: {},
    selectedTargetID: null,
    movementTarget: null,
    lastCorrection: null,
    pendingCommands: {},
    commandLog: [],
    combatLog: [],
    cargo: null,
    wallet: null,
    ship: null,
    stats: null,
    progression: null,
    repairQuote: null,
    skillCooldowns: {},
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

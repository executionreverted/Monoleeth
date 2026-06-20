import {
  CLIENT_EVENTS,
  EntityPayload,
  EventEnvelope,
  JsonObject,
  JsonValue,
  OPERATIONS,
  rejectForbiddenPayloadKeys,
  Vec2,
} from '../protocol/envelope';
import {
  CargoItemSummary,
  CargoSummary,
  CraftingSummary,
  AbuseCoverageSummary,
  AdminInspectionSummary,
  AdminRepairCraftJobSummary,
  AuctionGrantSummary,
  AuctionLotSummary,
  AuctionSummary,
  CommandLogSummary,
  EconomyDashboardSummary,
  HangarSummary,
  InventorySummary,
  LoadoutSummary,
  ClientAction,
  ClientState,
  LogLine,
  MinimapContact,
  MinimapMemory,
  MinimapSummary,
  KnownPlanetSummary,
  PlanetDetailSummary,
  PlanetIntelSummary,
  PlanetProductionSummary,
  PlanetStorageSummary,
  ProgressionSummary,
  ProductionCollectionSummary,
  PublicSession,
  RepairQuote,
  RouteListSummary,
  RouteSummary,
  ScanPulseSummary,
  ScanModeState,
  SectorSummary,
  ShipSummary,
  StatSummary,
  MarketListingSummary,
  MarketSummary,
  PremiumEntitlementSummary,
  PremiumPurchaseSummary,
  PremiumStockSummary,
  PremiumSummary,
  MetricsSummary,
  KnownLootDrop,
  QuestBoardSummary,
  QuestObjectiveSummary,
  QuestOfferSummary,
  QuestRewardSummary,
  QuestSummary,
  ReleaseGateSummary,
  WalletSummary,
  WorldFeedbackEffect,
} from './types';

const SCAN_REPEAT_DELAY_MS = 2_800;
const SCAN_STARTED_RECHECK_MS = 350;

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
    knownLoot: {},
    worldEffects: [],
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
    market: null,
    auction: null,
    premium: null,
    economyDashboard: null,
    adminInspection: null,
    adminRepair: null,
    commandLogSummary: null,
    metrics: null,
    releaseGate: null,
    abuseCoverage: null,
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
    movementTarget: movementTargetFromAuthoritativeSelf(visibleEntities, null),
    lastCorrection: null,
    knownLoot: retainVisibleLootDrops(state.knownLoot, visibleEntities),
    minimap: rebuildMinimapLiveContacts(state.minimap, visibleEntities),
    planetIntel: updateVisibleSignalCount(state.planetIntel, countPlanetSignals(visibleEntities)),
    lastServerTime: serverTime,
    lastSequence: sequence ? Math.max(state.lastSequence, sequence) : state.lastSequence,
  };
}

function contactFromEntity(entity: EntityPayload): MinimapContact {
  return {
    entity_id: entity.entity_id,
    entity_type: entity.entity_type,
    position: entity.position,
    disposition: entity.display?.disposition,
    status_flags: entity.status_flags ? [...entity.status_flags] : undefined,
  };
}

const safeStatusFlags = new Set([
  'damaged',
  'friendly',
  'hostile',
  'known_intel',
  'load_smoke',
  'local',
  'loot',
  'moving',
  'neutral',
  'scan_revealed',
  'scannable',
  'self',
  'stealthed',
  'unknown_signal',
  'visible',
]);

function parsePublicStatusFlags(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const flags = value.filter((flag): flag is string => typeof flag === 'string' && safeStatusFlags.has(flag));
  if (!flags.includes('self')) {
    const selfOnly = flags.indexOf('stealthed');
    if (selfOnly >= 0) {
      flags.splice(selfOnly, 1);
    }
  }
  return flags.length > 0 ? flags : undefined;
}

function upsertMinimapContact(minimap: MinimapSummary | null, entity: EntityPayload): MinimapSummary | null {
  if (!minimap) {
    return minimap;
  }
  const contact = contactFromEntity(entity);
  const nextContacts = minimap.live_contacts.filter((entry) => entry.entity_id !== entity.entity_id);
  nextContacts.push(contact);
  nextContacts.sort((a, b) => a.entity_id.localeCompare(b.entity_id));
  return {
    ...minimap,
    live_contacts: nextContacts,
  };
}

function rebuildMinimapLiveContacts(
  minimap: MinimapSummary | null,
  visibleEntities: Record<string, EntityPayload>,
): MinimapSummary | null {
  if (!minimap) {
    return minimap;
  }
  const liveContacts = Object.values(visibleEntities)
    .map(contactFromEntity)
    .sort((a, b) => a.entity_id.localeCompare(b.entity_id));
  return {
    ...minimap,
    live_contacts: liveContacts,
  };
}

function removeMinimapContact(minimap: MinimapSummary | null, entityID: string): MinimapSummary | null {
  if (!minimap) {
    return minimap;
  }
  const nextContacts = minimap.live_contacts.filter((entry) => entry.entity_id !== entityID);
  if (nextContacts.length === minimap.live_contacts.length) {
    return minimap;
  }
  return {
    ...minimap,
    live_contacts: nextContacts,
  };
}

function isStaleEvent(state: ClientState, envelope: EventEnvelope): boolean {
  return envelope.seq > 0 && state.lastSequence > 0 && envelope.seq <= state.lastSequence;
}

function resetsRealtimeStream(status: ClientState['connectionStatus']): boolean {
  return status === 'authenticated_pending_socket' || status === 'connected' || status === 'reconnecting';
}

function withoutPendingOperations(state: ClientState, operations: readonly string[]): ClientState {
  if (operations.length === 0) {
    return state;
  }

  const operationSet = new Set(operations);
  let changed = false;
  const pendingCommands: ClientState['pendingCommands'] = {};
  for (const [requestID, pending] of Object.entries(state.pendingCommands)) {
    if (operationSet.has(pending.op)) {
      changed = true;
      continue;
    }
    pendingCommands[requestID] = pending;
  }

  return changed ? { ...state, pendingCommands } : state;
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
        connectionStatus: 'connected',
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

    case CLIENT_EVENTS.combatDamage: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.combatUseSkill]);
      return appendWorldEffect(
        {
          ...nextState,
          lastServerTime: envelope.server_time,
          lastSequence: Math.max(nextState.lastSequence, envelope.seq),
          combatLog: appendLog(
            nextState.combatLog,
            'info',
            `Hit ${displayNameForEntity(nextState, stringField(envelope.payload, 'target_id'))} for ${Math.round(numberField(envelope.payload, 'amount') ?? 0)}.`,
          ),
        },
        feedbackEffect(nextState, envelope, 'damage'),
      );
    }

    case CLIENT_EVENTS.combatMiss: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.combatUseSkill]);
      return appendWorldEffect(
        {
          ...nextState,
          lastServerTime: envelope.server_time,
          lastSequence: Math.max(nextState.lastSequence, envelope.seq),
          combatLog: appendLog(nextState.combatLog, 'warn', `Missed ${displayNameForEntity(nextState, stringField(envelope.payload, 'target_id'))}.`),
        },
        feedbackEffect(nextState, envelope, 'miss'),
      );
    }

    case CLIENT_EVENTS.combatCooldownStarted: {
      const skillID = stringField(envelope.payload, 'skill_id') ?? 'basic_laser';
      const readyAt = numberField(envelope.payload, 'cooldown_ready_at_ms') ?? envelope.server_time;
      const nextState = withoutPendingOperations(state, [OPERATIONS.combatUseSkill]);
      return appendWorldEffect(
        {
          ...nextState,
          skillCooldowns: { ...nextState.skillCooldowns, [skillID]: readyAt },
          lastServerTime: envelope.server_time,
          lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        },
        feedbackEffect(nextState, envelope, 'laser'),
      );
    }

    case CLIENT_EVENTS.combatNPCKilled: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.combatUseSkill]);
      return appendWorldEffect(
        {
          ...nextState,
          lastServerTime: envelope.server_time,
          lastSequence: Math.max(nextState.lastSequence, envelope.seq),
          combatLog: appendLog(nextState.combatLog, 'info', `${stringField(envelope.payload, 'npc_type') ?? 'Hostile'} destroyed.`),
        },
        feedbackEffect(nextState, envelope, 'destroyed'),
      );
    }

    case CLIENT_EVENTS.lootCreated:
    case CLIENT_EVENTS.lootUpdated: {
      const knownLoot = parseKnownLootDrop(envelope.payload);
      const nextState = {
        ...state,
        knownLoot: knownLoot ? { ...state.knownLoot, [knownLoot.drop_id]: knownLoot } : state.knownLoot,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        combatLog: appendLog(
          state.combatLog,
          'info',
          `Drop ${stringField(envelope.payload, 'item_id') ?? 'item'} x${Math.round(numberField(envelope.payload, 'quantity') ?? 0)}.`,
        ),
      };
      return appendWorldEffect(nextState, feedbackEffect(nextState, envelope, 'loot_spawn'));
    }

    case CLIENT_EVENTS.lootRemoved: {
      const entityID = requireEntityID(envelope.payload);
      const nextState = withoutPendingOperations(state, [OPERATIONS.lootPickup]);
      const visibleEntities = { ...nextState.visibleEntities };
      const knownLoot = { ...nextState.knownLoot };
      delete visibleEntities[entityID];
      delete knownLoot[entityID];
      return {
        ...nextState,
        visibleEntities,
        minimap: removeMinimapContact(nextState.minimap, entityID),
        knownLoot,
        selectedTargetID: nextState.selectedTargetID === entityID ? null : nextState.selectedTargetID,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.lootPickedUp: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.lootPickup]);
      return appendWorldEffect(
        {
          ...nextState,
          lastServerTime: envelope.server_time,
          lastSequence: Math.max(nextState.lastSequence, envelope.seq),
          combatLog: appendLog(
            nextState.combatLog,
            'info',
            `Recovered ${stringField(envelope.payload, 'item_id') ?? 'item'} x${Math.round(numberField(envelope.payload, 'quantity') ?? 0)}.`,
          ),
        },
        feedbackEffect(nextState, envelope, 'loot_pickup'),
      );
    }

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

    case CLIENT_EVENTS.inventorySnapshot:
      return {
        ...state,
        inventory: parseInventorySummary(envelope.payload, state.inventory),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.hangarSnapshot:
      return {
        ...state,
        hangar: parseHangarSummary(envelope.payload, state.hangar),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.loadoutSnapshot:
      return {
        ...state,
        loadout: parseLoadoutSummary(envelope.payload, state.loadout),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.craftingRecipes:
      return {
        ...state,
        crafting: parseCraftingSummary(envelope.payload, state.crafting),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.scanPulseStarted:
    {
      const scan = parseScanPulse(envelope.payload, state.planetIntel?.lastScan ?? null);
      const nextState = withoutPendingOperations(state, [OPERATIONS.scanPulse]);
      return {
        ...nextState,
        planetIntel: applyScanPulse(nextState.planetIntel, scan),
        scanMode: scanModeAfterPulseStarted(nextState.scanMode, scan),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.scanPulseResolved:
    case CLIENT_EVENTS.scanPlanetDiscovered: {
      const scan = parseScanPulse(envelope.payload, state.planetIntel?.lastScan ?? null);
      const nextState = withoutPendingOperations(state, [OPERATIONS.scanPulse]);
      return {
        ...nextState,
        planetIntel: applyScanPulse(nextState.planetIntel, scan),
        scanMode: scanModeAfterPulseResolved(nextState.scanMode),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        commandLog: appendLog(nextState.commandLog, scan.status === 'planet_discovered' ? 'info' : 'warn', scanLogLine(scan)),
      };
    }

    case CLIENT_EVENTS.knownPlanets: {
      const minimap = objectField(envelope.payload, 'minimap');
      return {
        ...state,
        planetIntel: parseKnownPlanets(envelope.payload, state.planetIntel),
        minimap: minimap ? parseMinimapSummary(minimap, state.minimap) : state.minimap,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.planetDetail:
      return {
        ...state,
        planetIntel: applyPlanetDetail(state.planetIntel, parsePlanetDetail(envelope.payload, state.planetIntel?.selectedPlanet ?? null)),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.productionSummary:
      return {
        ...state,
        production: parseProductionCollection(envelope.payload, state.production),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.planetStorageSummary: {
      const storagePayload = objectField(envelope.payload, 'planet_storage') ?? envelope.payload;
      return {
        ...applyPlanetStorageSummary(state, parsePlanetStorage(storagePayload)),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.routeList:
      return {
        ...state,
        routes: parseRouteList(envelope.payload, state.routes),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };

    case CLIENT_EVENTS.routeSnapshot: {
      const routePayload = objectField(envelope.payload, 'route') ?? envelope.payload;
      const route = parseRoute(routePayload);
      return {
        ...(route ? applyRouteSnapshot(state, route) : state),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.marketListingCreated:
    case CLIENT_EVENTS.marketListingUpdated:
    case CLIENT_EVENTS.marketSaleCompleted:
    case CLIENT_EVENTS.marketListingCancelled:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', economyEventLog(envelope.type)),
      };

    case CLIENT_EVENTS.auctionLotUpdated:
    case CLIENT_EVENTS.auctionClosed:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', economyEventLog(envelope.type)),
      };

    case CLIENT_EVENTS.auctionBidPlaced: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.auctionBid]);
      return {
        ...nextState,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        commandLog: appendLog(nextState.commandLog, 'info', economyEventLog(envelope.type)),
      };
    }

    case CLIENT_EVENTS.premiumEntitlementCreated:
    case CLIENT_EVENTS.premiumEntitlementClaimed:
    case CLIENT_EVENTS.premiumStockConsumed:
    case CLIENT_EVENTS.economyFlowUpdated:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', economyEventLog(envelope.type)),
      };

    case CLIENT_EVENTS.questBoardGenerated:
      return {
        ...state,
        questBoard: parseQuestBoardSummary(envelope.payload, state.questBoard),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', 'Quest board refreshed.'),
      };

    case CLIENT_EVENTS.questBoardRerolled: {
      const board = objectField(envelope.payload, 'quest_board');
      return {
        ...state,
        questBoard: board ? parseQuestBoardSummary(board, state.questBoard) : state.questBoard,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', 'Quest board rerolled.'),
      };
    }

    case CLIENT_EVENTS.questAccepted:
    case CLIENT_EVENTS.questProgressed:
    case CLIENT_EVENTS.questCompleted:
    case CLIENT_EVENTS.questRewardClaimed:
    case CLIENT_EVENTS.questAbandoned: {
      const quest = parseQuestSummary(envelope.payload, null);
      return {
        ...state,
        questBoard: quest ? applyQuestUpdate(state.questBoard, quest) : state.questBoard,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', questEventLog(envelope.type)),
      };
    }

    case CLIENT_EVENTS.adminActionCompleted:
      return {
        ...state,
        adminRepair: parseAdminRepairCraftJob(envelope.payload, state.adminRepair),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', 'Admin action completed.'),
      };

    case CLIENT_EVENTS.observabilityMetricUpdated:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', 'Metrics refreshed.'),
      };

    case CLIENT_EVENTS.releaseGateUpdated:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', 'Release gate refreshed.'),
      };

    case CLIENT_EVENTS.deathShipDisabled:
      return {
        ...applyDeathShipDisabled(state, envelope.payload),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        combatLog: appendLog(state.combatLog, 'error', 'Ship disabled.'),
      };

    case CLIENT_EVENTS.deathRepaired: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.deathRepairShip]);
      return {
        ...nextState,
        repairQuote: null,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        combatLog: appendLog(nextState.combatLog, 'info', 'Ship repaired.'),
      };
    }

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
        minimap: upsertMinimapContact(state.minimap, entity),
        movementTarget: movementTargetFromAuthoritativeSelf(visibleEntities, state.movementTarget),
        planetIntel:
          entity.entity_type === 'planet_signal'
            ? updateVisibleSignalCount(state.planetIntel, countPlanetSignals(visibleEntities))
            : state.planetIntel,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.entityLeft: {
      const entityID = requireEntityID(envelope.payload);
      const visibleEntities = { ...state.visibleEntities };
      const knownLoot = { ...state.knownLoot };
      delete visibleEntities[entityID];
      delete knownLoot[entityID];
      return {
        ...state,
        visibleEntities,
        minimap: removeMinimapContact(state.minimap, entityID),
        knownLoot,
        selectedTargetID: state.selectedTargetID === entityID ? null : state.selectedTargetID,
        planetIntel: updateVisibleSignalCount(state.planetIntel, countPlanetSignals(visibleEntities)),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.positionCorrected: {
      const entityID = requireEntityID(envelope.payload);
      const position = requirePosition(envelope.payload);
      const movement = parseEntityMovement(envelope.payload);
      return applyCorrection(withoutPendingOperations(state, [OPERATIONS.moveTo]), entityID, position, movement, envelope.server_time, envelope.seq);
    }

    case CLIENT_EVENTS.movementStopped: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.stop, OPERATIONS.moveTo]);
      return {
        ...nextState,
        movementTarget: null,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
      };
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
  movement: EntityPayload['movement'],
  serverTime: number | null,
  sequence?: number,
): ClientState {
  const entity = state.visibleEntities[entityID];
  let visibleEntities = state.visibleEntities;
  if (entity) {
    const correctedEntity: EntityPayload = {
      ...entity,
      position,
      movement,
    };
    if (!movement) {
      delete correctedEntity.movement;
    }
    visibleEntities = {
      ...state.visibleEntities,
      [entityID]: correctedEntity,
    };
  }

  return {
    ...state,
    visibleEntities,
    minimap: entity
      ? upsertMinimapContact(state.minimap, { ...entity, position, movement })
      : state.minimap,
    movementTarget: movement?.moving ? movement.target : null,
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

function appendWorldEffect(state: ClientState, effect: WorldFeedbackEffect | null): ClientState {
  if (!effect) {
    return state;
  }
  const now = Date.now();
  if (state.worldEffects.some((entry) => entry.id === effect.id)) {
    return state;
  }
  return {
    ...state,
    worldEffects: [...state.worldEffects.filter((entry) => entry.expiresAt > now).slice(-17), effect],
  };
}

function feedbackEffect(
  state: ClientState,
  envelope: EventEnvelope,
  kind: WorldFeedbackEffect['kind'],
): WorldFeedbackEffect | null {
  const targetID =
    stringField(envelope.payload, 'target_id') ??
    stringField(envelope.payload, 'entity_id') ??
    stringField(envelope.payload, 'drop_id') ??
    undefined;
  const sourceID = selfEntityID(state);
  const position =
    (isVec2(envelope.payload.position) ? envelope.payload.position : null) ??
    (targetID ? state.visibleEntities[targetID]?.position : undefined) ??
    (targetID ? state.knownLoot[targetID]?.position : undefined);

  if (!targetID && !position) {
    return null;
  }

  const createdAt = Date.now();
  const sourcePosition = sourceID ? state.visibleEntities[sourceID]?.position : null;
  return {
    id: `${envelope.event_id}:${kind}`,
    kind,
    targetID,
    sourceID: kind === 'laser' ? sourceID ?? undefined : undefined,
    position: position ? { ...position } : undefined,
    sourcePosition: kind === 'laser' && sourcePosition ? { ...sourcePosition } : undefined,
    amount: roundedOptional(envelope.payload, 'amount'),
    shieldAmount: roundedOptional(envelope.payload, 'shield_amount'),
    hullAmount: roundedOptional(envelope.payload, 'hull_amount'),
    itemID: stringField(envelope.payload, 'item_id') ?? (targetID ? state.knownLoot[targetID]?.item_id : undefined),
    quantity: roundedOptional(envelope.payload, 'quantity') ?? (targetID ? state.knownLoot[targetID]?.quantity : undefined),
    createdAt,
    expiresAt: createdAt + feedbackDuration(kind),
  };
}

function feedbackDuration(kind: WorldFeedbackEffect['kind']): number {
  switch (kind) {
    case 'laser':
      return 700;
    case 'damage':
    case 'miss':
      return 2200;
    case 'destroyed':
    case 'loot_spawn':
    case 'loot_pickup':
      return 3200;
  }
}

function parseKnownLootDrop(payload: JsonObject): KnownLootDrop | null {
  const dropID = stringField(payload, 'drop_id') ?? stringField(payload, 'entity_id');
  const itemID = stringField(payload, 'item_id');
  const quantity = numberField(payload, 'quantity');
  if (!dropID || !itemID || quantity === null || quantity <= 0) {
    return null;
  }

  return {
    drop_id: dropID,
    item_id: itemID,
    quantity: Math.round(quantity),
    state: stringField(payload, 'state') ?? undefined,
    expires_at: numberField(payload, 'expires_at') ?? undefined,
    position: isVec2(payload.position) ? { ...payload.position } : undefined,
  };
}

function retainVisibleLootDrops(
  knownLoot: Record<string, KnownLootDrop>,
  visibleEntities: Record<string, EntityPayload>,
): Record<string, KnownLootDrop> {
  const retained: Record<string, KnownLootDrop> = {};
  for (const [dropID, drop] of Object.entries(knownLoot)) {
    if (visibleEntities[dropID]?.entity_type === 'loot') {
      retained[dropID] = drop;
    }
  }
  return retained;
}

function selfEntityID(state: ClientState): string | null {
  return Object.values(state.visibleEntities).find(isSelfEntity)?.entity_id ?? null;
}

function displayNameForEntity(state: ClientState, entityID: string | null): string {
  if (!entityID) {
    return 'target';
  }
  return state.visibleEntities[entityID]?.display?.label ?? entityID;
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

  const inventory = objectField(payload, 'inventory') ?? objectField(payload, 'inventory_snapshot');
  if (inventory) {
    next = {
      ...next,
      inventory: parseInventorySummary(inventory, next.inventory),
    };
  }

  const hangar = objectField(payload, 'hangar') ?? objectField(payload, 'hangar_snapshot');
  if (hangar) {
    next = {
      ...next,
      hangar: parseHangarSummary(hangar, next.hangar),
    };
  }

  const loadout = objectField(payload, 'loadout') ?? objectField(payload, 'loadout_snapshot');
  if (loadout) {
    next = {
      ...next,
      loadout: parseLoadoutSummary(loadout, next.loadout),
    };
  }

  const crafting = objectField(payload, 'crafting') ?? objectField(payload, 'crafting_recipes');
  if (crafting) {
    next = {
      ...next,
      crafting: parseCraftingSummary(crafting, next.crafting),
    };
  }

  const scan = objectField(payload, 'scan') ?? objectField(payload, 'scan_pulse');
  if (scan) {
    const parsedScan = parseScanPulse(scan, next.planetIntel?.lastScan ?? null);
    next = {
      ...next,
      planetIntel: applyScanPulse(next.planetIntel, parsedScan),
      scanMode: scanModeAfterPulseSummary(next.scanMode, parsedScan),
    };
  }

  const knownPlanets = objectField(payload, 'known_planets') ?? objectField(payload, 'planet_intel');
  if (knownPlanets) {
    next = {
      ...next,
      planetIntel: parseKnownPlanets(knownPlanets, next.planetIntel),
    };
  }

  const planetDetail = objectField(payload, 'planet_detail');
  if (planetDetail) {
    next = {
      ...next,
      planetIntel: applyPlanetDetail(next.planetIntel, parsePlanetDetail(planetDetail, next.planetIntel?.selectedPlanet ?? null)),
    };
  }

  const production = objectField(payload, 'production') ?? objectField(payload, 'production_summary');
  if (production) {
    next = {
      ...next,
      production: parseProductionCollection(production, next.production),
    };
  }

  const planetStorage = objectField(payload, 'planet_storage');
  if (planetStorage) {
    next = applyPlanetStorageSummary(next, parsePlanetStorage(planetStorage));
  }

  const routes = objectField(payload, 'routes') ?? objectField(payload, 'route_list');
  if (routes) {
    next = {
      ...next,
      routes: parseRouteList(routes, next.routes),
    };
  }

  const route = objectField(payload, 'route');
  if (route) {
    const parsedRoute = parseRoute(route);
    if (parsedRoute) {
      next = applyRouteSnapshot(next, parsedRoute);
    }
  }

  const market = objectField(payload, 'market');
  if (market) {
    next = {
      ...next,
      market: parseMarketSummary(market, next.market),
    };
  }

  const auction = objectField(payload, 'auction');
  if (auction) {
    next = {
      ...next,
      auction: parseAuctionSummary(auction, next.auction),
    };
  }

  const premium = objectField(payload, 'premium');
  if (premium) {
    next = {
      ...next,
      premium: parsePremiumSummary(premium, next.premium),
    };
  }

  const questBoard = objectField(payload, 'quest_board');
  if (questBoard) {
    next = {
      ...next,
      questBoard: parseQuestBoardSummary(questBoard, next.questBoard),
    };
  }

  const economy = objectField(payload, 'economy');
  if (economy) {
    next = {
      ...next,
      economyDashboard: parseEconomyDashboard(economy, next.economyDashboard),
    };
  }

  const admin = objectField(payload, 'admin');
  if (admin) {
    next = {
      ...next,
      adminInspection: parseAdminInspection(admin, next.adminInspection),
    };
  }

  const adminRepair = objectField(payload, 'admin_repair');
  if (adminRepair) {
    next = {
      ...next,
      adminRepair: parseAdminRepairCraftJob(adminRepair, next.adminRepair),
    };
  }

  const commandLog = objectField(payload, 'command_log');
  if (commandLog) {
    next = {
      ...next,
      commandLogSummary: parseCommandLogSummary(commandLog, next.commandLogSummary),
    };
  }

  const metrics = objectField(payload, 'metrics');
  if (metrics) {
    next = {
      ...next,
      metrics: parseMetricsSummary(metrics, next.metrics),
    };
  }

  const releaseGate = objectField(payload, 'release_gate');
  if (releaseGate) {
    next = {
      ...next,
      releaseGate: parseReleaseGateSummary(releaseGate, next.releaseGate),
    };
  }

  const abuseCoverage = objectField(payload, 'abuse_coverage');
  if (abuseCoverage) {
    next = {
      ...next,
      abuseCoverage: parseAbuseCoverageSummary(abuseCoverage, next.abuseCoverage),
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
    status_flags: parsePublicStatusFlags(source.status_flags),
    display: parseEntityDisplay(source),
    combat: parseEntityCombat(source),
    movement: parseEntityMovement(source),
  };
}

function movementTargetFromAuthoritativeSelf(
  visibleEntities: Record<string, EntityPayload>,
  fallback: Vec2 | null,
): Vec2 | null {
  const self = Object.values(visibleEntities).find(isSelfEntity);
  if (!self) {
    return fallback;
  }
  return self.movement?.moving ? self.movement.target : null;
}

function isSelfEntity(entity: EntityPayload): boolean {
  return entity.status_flags?.includes('self') || entity.status_flags?.includes('local') || false;
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
    .map(parseCargoItemSummary)
    .filter((item) => item.item_id !== '' && item.quantity > 0);

  return {
    used: Math.max(0, Math.round(used)),
    capacity: Math.max(0, Math.round(capacity)),
    items,
  };
}

function parseCargoItemSummary(item: JsonObject): CargoItemSummary {
  const parsed: CargoItemSummary = {
    item_id: stringField(item, 'item_id') ?? '',
    quantity: numberField(item, 'quantity') ?? 0,
  };
  const displayName = stringField(item, 'display_name');
  if (displayName) {
    parsed.display_name = displayName;
  }
  const category = stringField(item, 'category');
  if (category) {
    parsed.category = category;
  }
  const artKey = stringField(item, 'art_key');
  if (artKey) {
    parsed.art_key = artKey;
  }
  const rarity = stringField(item, 'rarity');
  if (rarity) {
    parsed.rarity = rarity;
  }
  const unitWeight = optionalRoundedNumber(item, 'unit_weight', undefined);
  if (unitWeight !== undefined) {
    parsed.unit_weight = unitWeight;
  }
  const usedUnits = optionalRoundedNumber(item, 'used_units', undefined);
  if (usedUnits !== undefined) {
    parsed.used_units = usedUnits;
  }
  const location = stringField(item, 'location');
  if (location) {
    parsed.location = location;
  }
  const moveEligible = booleanField(item, 'move_eligible');
  if (moveEligible !== null) {
    parsed.move_eligible = moveEligible;
  }
  const lockedReason = stringField(item, 'locked_reason');
  if (lockedReason) {
    parsed.locked_reason = lockedReason;
  }
  return parsed;
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

function applyDeathShipDisabled(state: ClientState, payload: JsonObject): ClientState {
  const shipPayload = objectField(payload, 'ship');
  const shipID = stringField(payload, 'ship_id') ?? stringField(shipPayload ?? {}, 'active_ship_id') ?? '';
  const disabledReason = stringField(payload, 'disabled_reason') ?? 'disabled';
  const ship = shipPayload
    ? parseShipSummary({ ...shipPayload, disabled: true, repair_state: disabledReason }, state.ship)
    : state.ship && (!shipID || state.ship.active_ship_id === shipID)
      ? {
          ...state.ship,
          disabled: true,
          repair_state: disabledReason,
        }
      : state.ship;
  const quote = objectField(payload, 'repair_quote');

  return {
    ...state,
    ship,
    repairQuote: quote ? parseRepairQuote(quote, state.repairQuote) : state.repairQuote,
  };
}

function parseStatSummary(payload: JsonObject, fallback: StatSummary | null): StatSummary {
  return {
    speed: Math.max(0, numberField(payload, 'speed') ?? fallback?.speed ?? 0),
    radar_range: Math.max(0, numberField(payload, 'radar_range') ?? fallback?.radar_range ?? 0),
    weapon_range: Math.max(0, numberField(payload, 'weapon_range') ?? fallback?.weapon_range ?? 0),
    cargo_capacity: Math.max(0, numberField(payload, 'cargo_capacity') ?? fallback?.cargo_capacity ?? 0),
    loot_pickup_range: Math.max(0, numberField(payload, 'loot_pickup_range') ?? fallback?.loot_pickup_range ?? 0),
    basic_laser_energy_cost: Math.max(0, numberField(payload, 'basic_laser_energy_cost') ?? fallback?.basic_laser_energy_cost ?? 0),
    basic_laser_cooldown_ms: Math.max(0, numberField(payload, 'basic_laser_cooldown_ms') ?? fallback?.basic_laser_cooldown_ms ?? 0),
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

function parseInventorySummary(payload: JsonObject, fallback: InventorySummary | null): InventorySummary {
  const stackable = Array.isArray(payload.stackable)
    ? payload.stackable
        .filter(isJsonObject)
        .map(parseInventoryStack)
        .filter((item): item is InventorySummary['stackable'][number] => item !== null)
    : fallback?.stackable ?? [];
  const instances = Array.isArray(payload.instances)
    ? payload.instances
        .filter(isJsonObject)
        .map(parseInventoryInstance)
        .filter((item): item is InventorySummary['instances'][number] => item !== null)
    : fallback?.instances ?? [];
  const counts = objectField(payload, 'counts');

  return {
    stackable,
    instances,
    counts: {
      cargo_stacks: Math.max(0, Math.round(numberField(counts ?? {}, 'cargo_stacks') ?? fallback?.counts.cargo_stacks ?? 0)),
      storage_stacks: Math.max(0, Math.round(numberField(counts ?? {}, 'storage_stacks') ?? fallback?.counts.storage_stacks ?? 0)),
      equipped_instances: Math.max(
        0,
        Math.round(numberField(counts ?? {}, 'equipped_instances') ?? fallback?.counts.equipped_instances ?? 0),
      ),
    },
  };
}

function parseInventoryStack(payload: JsonObject): InventorySummary['stackable'][number] | null {
  const itemID = stringField(payload, 'item_id') ?? '';
  const quantity = numberField(payload, 'quantity') ?? 0;
  if (!itemID || quantity <= 0) {
    return null;
  }
  return {
    item_id: itemID,
    display_name: stringField(payload, 'display_name') ?? undefined,
    quantity: Math.max(0, Math.round(quantity)),
    location: stringField(payload, 'location') ?? '',
  };
}

function parseInventoryInstance(payload: JsonObject): InventorySummary['instances'][number] | null {
  const itemInstanceID = stringField(payload, 'item_instance_id') ?? '';
  const itemID = stringField(payload, 'item_id') ?? '';
  if (!itemInstanceID || !itemID) {
    return null;
  }
  return {
    item_instance_id: itemInstanceID,
    item_id: itemID,
    display_name: stringField(payload, 'display_name') ?? undefined,
    location: stringField(payload, 'location') ?? '',
    rarity: stringField(payload, 'rarity') ?? undefined,
    item_type: stringField(payload, 'item_type') ?? undefined,
    module_slot_type: stringField(payload, 'module_slot_type') ?? undefined,
    module_category: stringField(payload, 'module_category') ?? undefined,
    durability_current: optionalRoundedNumber(payload, 'durability_current', undefined),
    durability_max: optionalRoundedNumber(payload, 'durability_max', undefined),
    bound_state: stringField(payload, 'bound_state') ?? undefined,
  };
}

function parseHangarSummary(payload: JsonObject, fallback: HangarSummary | null): HangarSummary {
  const ships = Array.isArray(payload.ships)
    ? payload.ships
        .filter(isJsonObject)
        .map(parseHangarShip)
        .filter((ship): ship is HangarSummary['ships'][number] => ship !== null)
    : fallback?.ships ?? [];
  return {
    active_ship_id: stringField(payload, 'active_ship_id') ?? fallback?.active_ship_id ?? '',
    ships,
  };
}

function parseHangarShip(payload: JsonObject): HangarSummary['ships'][number] | null {
  const shipID = stringField(payload, 'ship_id') ?? '';
  if (!shipID) {
    return null;
  }
  return {
    ship_id: shipID,
    display_name: stringField(payload, 'display_name') ?? shipID,
    state: stringField(payload, 'state') ?? '',
    role: stringField(payload, 'role') ?? undefined,
    tier: optionalRoundedNumber(payload, 'tier', undefined),
    rank_requirement: optionalRoundedNumber(payload, 'rank_requirement', undefined),
    hull: Math.max(0, Math.round(numberField(payload, 'hull') ?? 0)),
    max_hull: Math.max(0, Math.round(numberField(payload, 'max_hull') ?? 0)),
    shield: Math.max(0, Math.round(numberField(payload, 'shield') ?? 0)),
    max_shield: Math.max(0, Math.round(numberField(payload, 'max_shield') ?? 0)),
    capacitor: optionalRoundedNumber(payload, 'capacitor', undefined),
    max_capacitor: optionalRoundedNumber(payload, 'max_capacitor', undefined),
    speed: optionalRoundedNumber(payload, 'speed', undefined),
    radar: optionalRoundedNumber(payload, 'radar', undefined),
    cargo_capacity: optionalRoundedNumber(payload, 'cargo_capacity', undefined),
    slot_offensive: optionalRoundedNumber(payload, 'slot_offensive', undefined),
    slot_defensive: optionalRoundedNumber(payload, 'slot_defensive', undefined),
    slot_utility: optionalRoundedNumber(payload, 'slot_utility', undefined),
    disabled: booleanField(payload, 'disabled') ?? false,
    active: booleanField(payload, 'active') ?? undefined,
    locked_reason: stringField(payload, 'locked_reason') ?? undefined,
  };
}

function parseLoadoutSummary(payload: JsonObject, fallback: LoadoutSummary | null): LoadoutSummary {
  const slots = Array.isArray(payload.slots)
    ? payload.slots
        .filter(isJsonObject)
        .map(parseLoadoutSlot)
        .filter((slot): slot is LoadoutSummary['slots'][number] => slot !== null)
    : fallback?.slots ?? [];
  return {
    active_ship_id: stringField(payload, 'active_ship_id') ?? fallback?.active_ship_id ?? '',
    slots,
  };
}

function parseLoadoutSlot(payload: JsonObject): LoadoutSummary['slots'][number] | null {
  const slotID = stringField(payload, 'slot_id') ?? '';
  const slotType = stringField(payload, 'slot_type') ?? '';
  if (!slotID || !slotType) {
    return null;
  }
  return {
    slot_id: slotID,
    slot_type: slotType,
    module_item_id: stringField(payload, 'module_item_id') ?? undefined,
    item_instance_id: stringField(payload, 'item_instance_id') ?? undefined,
    module_id: stringField(payload, 'module_id') ?? undefined,
    display_name: stringField(payload, 'display_name') ?? undefined,
    module_state: stringField(payload, 'module_state') ?? undefined,
    durability: optionalRoundedNumber(payload, 'durability', undefined),
    durability_max: optionalRoundedNumber(payload, 'durability_max', undefined),
  };
}

function parseCraftingSummary(payload: JsonObject, fallback: CraftingSummary | null): CraftingSummary {
  const recipes = Array.isArray(payload.recipes)
    ? payload.recipes
        .filter(isJsonObject)
        .map(parseCraftingRecipe)
        .filter((recipe): recipe is CraftingSummary['recipes'][number] => recipe !== null)
    : fallback?.recipes ?? [];
  const activeJobs = Array.isArray(payload.active_jobs)
    ? payload.active_jobs
        .filter(isJsonObject)
        .map(parseCraftingJob)
        .filter((job): job is CraftingSummary['active_jobs'][number] => job !== null)
    : fallback?.active_jobs ?? [];
  return {
    recipes,
    active_jobs: activeJobs,
  };
}

function parseCraftingRecipe(payload: JsonObject): CraftingSummary['recipes'][number] | null {
  const recipeID = stringField(payload, 'recipe_id') ?? '';
  const output = objectField(payload, 'output');
  if (!recipeID || !output) {
    return null;
  }
  return {
    recipe_id: recipeID,
    category: stringField(payload, 'category') ?? '',
    output: {
      kind: stringField(output, 'kind') ?? '',
      item_id: stringField(output, 'item_id') ?? undefined,
      ship_id: stringField(output, 'ship_id') ?? undefined,
      quantity: Math.max(0, Math.round(numberField(output, 'quantity') ?? 0)),
      tradeable: booleanField(output, 'tradeable') ?? false,
    },
    inputs: Array.isArray(payload.inputs)
      ? payload.inputs
          .filter(isJsonObject)
          .map((input) => ({
            item_id: stringField(input, 'item_id') ?? '',
            quantity: Math.max(0, Math.round(numberField(input, 'quantity') ?? 0)),
          }))
          .filter((input) => input.item_id !== '' && input.quantity > 0)
      : [],
    required_credits: Math.max(0, Math.round(numberField(payload, 'required_credits') ?? 0)),
    required_rank: Math.max(0, Math.round(numberField(payload, 'required_rank') ?? 0)),
    required_role_levels: Array.isArray(payload.required_role_levels)
      ? payload.required_role_levels
          .filter(isJsonObject)
          .map((requirement) => ({
            role: stringField(requirement, 'role') ?? '',
            level: Math.max(0, Math.round(numberField(requirement, 'level') ?? 0)),
          }))
          .filter((requirement) => requirement.role !== '' && requirement.level > 0)
      : [],
    required_location_type: stringField(payload, 'required_location_type') ?? '',
    craft_duration_ms: Math.max(0, Math.round(numberField(payload, 'craft_duration_ms') ?? 0)),
    repeatable: booleanField(payload, 'repeatable') ?? false,
  };
}

function parseCraftingJob(payload: JsonObject): CraftingSummary['active_jobs'][number] | null {
  const jobID = stringField(payload, 'job_id') ?? '';
  const recipeID = stringField(payload, 'recipe_id') ?? '';
  if (!jobID || !recipeID) {
    return null;
  }
  return {
    job_id: jobID,
    recipe_id: recipeID,
    state: stringField(payload, 'state') ?? '',
    started_at: Math.max(0, Math.round(numberField(payload, 'started_at') ?? 0)),
    completes_at: Math.max(0, Math.round(numberField(payload, 'completes_at') ?? 0)),
  };
}

function parseScanPulse(payload: JsonObject, fallback: ScanPulseSummary | null): ScanPulseSummary {
  const signal = objectField(payload, 'signal');
  const status = stringField(payload, 'status') ?? fallback?.status ?? 'unknown';
  const clearsPlanetResult = status === 'started' || status === 'no_signal' || status === 'player_revealed';
  return {
    pulse_reference: stringField(payload, 'pulse_reference') ?? fallback?.pulse_reference ?? '',
    status,
    resolve_after: optionalRoundedNumber(payload, 'resolve_after', fallback?.resolve_after),
    message: stringField(payload, 'message') ?? fallback?.message,
    signal: signal
      ? {
          biome: stringField(signal, 'biome') ?? '',
          signal_band: stringField(signal, 'signal_band') ?? '',
          approx_distance: stringField(signal, 'approx_distance') ?? '',
        }
      : clearsPlanetResult
        ? undefined
        : fallback?.signal,
    planet_id: stringField(payload, 'planet_id') ?? (clearsPlanetResult ? undefined : fallback?.planet_id),
    xp_granted: booleanField(payload, 'xp_granted') ?? (clearsPlanetResult ? false : fallback?.xp_granted),
    duplicate: booleanField(payload, 'duplicate') ?? fallback?.duplicate,
  };
}

function parseKnownPlanets(payload: JsonObject, fallback: PlanetIntelSummary | null): PlanetIntelSummary {
  const planets = Array.isArray(payload.planets)
    ? payload.planets
        .filter(isJsonObject)
        .map(parseKnownPlanet)
        .filter((planet): planet is KnownPlanetSummary => planet !== null)
    : fallback?.planets ?? [];
  const counts = objectField(payload, 'counts');
  const stale = Math.max(0, Math.round(numberField(counts ?? {}, 'stale') ?? fallback?.staleIntel ?? 0));
  const owned = Math.max(0, Math.round(numberField(counts ?? {}, 'owned') ?? fallback?.ownedPlanets ?? 0));
  const known = Math.max(
    planets.length,
    Math.round(numberField(counts ?? {}, 'known') ?? fallback?.knownSignals ?? planets.length),
  );
  const selectedPlanet =
    fallback?.selectedPlanet && planets.some((planet) => planet.planet_id === fallback.selectedPlanet?.planet_id)
      ? fallback.selectedPlanet
      : null;

  return {
    knownSignals: known,
    staleIntel: stale,
    ownedPlanets: owned,
    planets,
    selectedPlanet,
    lastScan: fallback?.lastScan ?? null,
  };
}

function parseKnownPlanet(payload: JsonObject): KnownPlanetSummary | null {
  const planetID = stringField(payload, 'planet_id') ?? '';
  if (!planetID) {
    return null;
  }
  return {
    planet_id: planetID,
    biome: stringField(payload, 'biome') ?? '',
    planet_type: stringField(payload, 'planet_type') ?? '',
    rarity: stringField(payload, 'rarity') ?? '',
    level: Math.max(0, Math.round(numberField(payload, 'level') ?? 0)),
    intel_state: stringField(payload, 'intel_state') ?? '',
    confidence: Math.max(0, Math.round(numberField(payload, 'confidence') ?? 0)),
    last_seen_at: Math.max(0, Math.round(numberField(payload, 'last_seen_at') ?? 0)),
    owner_status: stringField(payload, 'owner_status') ?? '',
    discovered_at: Math.max(0, Math.round(numberField(payload, 'discovered_at') ?? 0)),
  };
}

function parsePlanetDetail(payload: JsonObject, fallback: PlanetDetailSummary | null): PlanetDetailSummary {
  const parsed = parseKnownPlanet(payload);
  const planetID = parsed?.planet_id ?? stringField(payload, 'planet_id') ?? fallback?.planet_id ?? '';
  const matchingFallback = fallback?.planet_id === planetID ? fallback : null;
  const base = parsed ?? matchingFallback;
  const coordinates = isVec2(payload.coordinates) ? payload.coordinates : matchingFallback?.coordinates ?? null;
  const production = objectField(payload, 'production');
  const routes = Array.isArray(payload.routes)
    ? payload.routes
        .filter(isJsonObject)
        .map(parseRoute)
        .filter((route): route is RouteSummary => route !== null)
    : matchingFallback?.routes ?? [];
  return {
    ...(base ?? {
      planet_id: '',
      biome: '',
      planet_type: '',
      rarity: '',
      level: 0,
      intel_state: '',
      confidence: 0,
      last_seen_at: 0,
      owner_status: '',
      discovered_at: 0,
    }),
    coordinates,
    production: production ? parseProductionPlanet(production) ?? undefined : matchingFallback?.production,
    routes,
    production_locked: booleanField(payload, 'production_locked') ?? matchingFallback?.production_locked ?? true,
    available_commands: Array.isArray(payload.available_commands)
      ? payload.available_commands.filter((command): command is string => typeof command === 'string')
      : matchingFallback?.available_commands ?? [],
  };
}

function parseProductionCollection(payload: JsonObject, fallback: ProductionCollectionSummary | null): ProductionCollectionSummary {
  const planets = Array.isArray(payload.planets)
    ? payload.planets
        .filter(isJsonObject)
        .map(parseProductionPlanet)
        .filter((planet): planet is PlanetProductionSummary => planet !== null)
    : fallback?.planets ?? [];
  return { planets };
}

function parseProductionPlanet(payload: JsonObject): PlanetProductionSummary | null {
  const planetID = stringField(payload, 'planet_id') ?? '';
  const storage = objectField(payload, 'storage');
  if (!planetID || !storage) {
    return null;
  }
  return {
    planet_id: planetID,
    production_enabled: booleanField(payload, 'production_enabled') ?? false,
    last_calculated_at: Math.max(0, Math.round(numberField(payload, 'last_calculated_at') ?? 0)),
    energy_capacity_per_hour: Math.max(0, Math.round(numberField(payload, 'energy_capacity_per_hour') ?? 0)),
    energy_reserved_per_hour: Math.max(0, Math.round(numberField(payload, 'energy_reserved_per_hour') ?? 0)),
    storage: parsePlanetStorage(storage),
    buildings: Array.isArray(payload.buildings)
      ? payload.buildings
          .filter(isJsonObject)
          .map(parsePlanetBuilding)
          .filter((building): building is PlanetProductionSummary['buildings'][number] => building !== null)
      : [],
  };
}

function parsePlanetStorage(payload: JsonObject): PlanetStorageSummary {
  return {
    planet_id: stringField(payload, 'planet_id') ?? '',
    used_units: Math.max(0, Math.round(numberField(payload, 'used_units') ?? 0)),
    free_units: Math.max(0, Math.round(numberField(payload, 'free_units') ?? 0)),
    capacity_units: Math.max(0, Math.round(numberField(payload, 'capacity_units') ?? 0)),
    updated_at: Math.max(0, Math.round(numberField(payload, 'updated_at') ?? 0)),
    items: Array.isArray(payload.items)
      ? payload.items
          .filter(isJsonObject)
          .map((item) => ({
            item_id: stringField(item, 'item_id') ?? '',
            quantity: Math.max(0, Math.round(numberField(item, 'quantity') ?? 0)),
          }))
          .filter((item) => item.item_id !== '' && item.quantity > 0)
      : [],
  };
}

function applyPlanetStorageSummary(state: ClientState, storage: PlanetStorageSummary): ClientState {
  if (!storage.planet_id) {
    return state;
  }

  const production = state.production
    ? {
        planets: state.production.planets.map((planet) =>
          planet.planet_id === storage.planet_id ? { ...planet, storage } : planet,
        ),
      }
    : state.production;

  const currentPlanetIntel = state.planetIntel;
  const selectedPlanet = currentPlanetIntel?.selectedPlanet;
  const planetIntel =
    currentPlanetIntel && selectedPlanet && selectedPlanet.planet_id === storage.planet_id
      ? {
          ...currentPlanetIntel,
          selectedPlanet: {
            ...selectedPlanet,
            production: selectedPlanet.production
              ? { ...selectedPlanet.production, storage }
              : storageOnlyProductionSummary(storage),
          },
        }
      : currentPlanetIntel;

  return {
    ...state,
    production,
    planetIntel,
  };
}

function storageOnlyProductionSummary(storage: PlanetStorageSummary): PlanetProductionSummary {
  return {
    planet_id: storage.planet_id,
    production_enabled: false,
    last_calculated_at: storage.updated_at,
    energy_capacity_per_hour: 0,
    energy_reserved_per_hour: 0,
    storage,
    buildings: [],
  };
}

function parsePlanetBuilding(payload: JsonObject): PlanetProductionSummary['buildings'][number] | null {
  const buildingID = stringField(payload, 'building_id') ?? '';
  if (!buildingID) {
    return null;
  }
  return {
    building_id: buildingID,
    building_type: stringField(payload, 'building_type') ?? '',
    category: stringField(payload, 'category') ?? '',
    level: Math.max(0, Math.round(numberField(payload, 'level') ?? 0)),
    state: stringField(payload, 'state') ?? '',
    updated_at: Math.max(0, Math.round(numberField(payload, 'updated_at') ?? 0)),
  };
}

function parseRouteList(payload: JsonObject, fallback: RouteListSummary | null): RouteListSummary {
  const routes = Array.isArray(payload.routes)
    ? payload.routes
        .filter(isJsonObject)
        .map(parseRoute)
        .filter((route): route is RouteSummary => route !== null)
    : fallback?.routes ?? [];
  return { routes };
}

function parseRoute(payload: JsonObject): RouteSummary | null {
  const routeID = stringField(payload, 'route_id') ?? '';
  const destination = objectField(payload, 'destination');
  if (!routeID || !destination) {
    return null;
  }
  const risk = objectField(payload, 'risk') ?? {};
  return {
    route_id: routeID,
    source_planet_id: stringField(payload, 'source_planet_id') ?? '',
    destination: {
      type: stringField(destination, 'type') ?? '',
      id: stringField(destination, 'id') ?? '',
    },
    resource_item_id: stringField(payload, 'resource_item_id') ?? '',
    amount_per_hour: Math.max(0, Math.round(numberField(payload, 'amount_per_hour') ?? 0)),
    energy_cost_per_hour: Math.max(0, Math.round(numberField(payload, 'energy_cost_per_hour') ?? 0)),
    enabled: booleanField(payload, 'enabled') ?? false,
    risk: {
      loss_chance: Math.max(0, numberField(risk, 'loss_chance') ?? 0),
      min_loss_percent: Math.max(0, numberField(risk, 'min_loss_percent') ?? 0),
      max_loss_percent: Math.max(0, numberField(risk, 'max_loss_percent') ?? 0),
    },
    last_calculated_at: Math.max(0, Math.round(numberField(payload, 'last_calculated_at') ?? 0)),
    updated_at: Math.max(0, Math.round(numberField(payload, 'updated_at') ?? 0)),
  };
}

function applyRouteSnapshot(state: ClientState, route: RouteSummary): ClientState {
  const routes = { routes: upsertRoute(state.routes?.routes ?? [], route) };
  const currentPlanetIntel = state.planetIntel;
  const selectedPlanet = currentPlanetIntel?.selectedPlanet;
  const shouldUpdateSelected =
    selectedPlanet &&
    (selectedPlanet.planet_id === route.source_planet_id ||
      selectedPlanet.routes.some((existingRoute) => existingRoute.route_id === route.route_id));
  const planetIntel =
    currentPlanetIntel && selectedPlanet && shouldUpdateSelected
      ? {
          ...currentPlanetIntel,
          selectedPlanet: {
            ...selectedPlanet,
            routes: upsertRoute(selectedPlanet.routes, route),
          },
        }
      : currentPlanetIntel;

  return {
    ...state,
    routes,
    planetIntel,
  };
}

function upsertRoute(routes: RouteSummary[], route: RouteSummary): RouteSummary[] {
  const index = routes.findIndex((existingRoute) => existingRoute.route_id === route.route_id);
  if (index === -1) {
    return [...routes, route];
  }
  return routes.map((existingRoute, routeIndex) => (routeIndex === index ? route : existingRoute));
}

function parseMarketSummary(payload: JsonObject, fallback: MarketSummary | null): MarketSummary {
  const listings = Array.isArray(payload.listings)
    ? payload.listings
        .filter(isJsonObject)
        .map(parseMarketListing)
        .filter((listing): listing is MarketListingSummary => listing !== null)
    : fallback?.listings ?? [];
  const counts = objectField(payload, 'counts') ?? {};
  return {
    listings,
    counts: {
      active: Math.max(0, Math.round(numberField(counts, 'active') ?? fallback?.counts.active ?? 0)),
      mine: Math.max(0, Math.round(numberField(counts, 'mine') ?? fallback?.counts.mine ?? 0)),
    },
  };
}

function parseMarketListing(payload: JsonObject): MarketListingSummary | null {
  const listingID = stringField(payload, 'listing_id') ?? '';
  const itemID = stringField(payload, 'item_id') ?? '';
  if (!listingID || !itemID) {
    return null;
  }
  const estimate = objectField(payload, 'estimated_unit_purchase') ?? {};
  return {
    listing_id: listingID,
    item_id: itemID,
    display_name: stringField(payload, 'display_name') ?? itemID,
    rarity: stringField(payload, 'rarity') ?? '',
    remaining_quantity: Math.max(0, Math.round(numberField(payload, 'remaining_quantity') ?? 0)),
    unit_price: Math.max(0, Math.round(numberField(payload, 'unit_price') ?? 0)),
    currency_type: stringField(payload, 'currency_type') ?? 'credits',
    status: stringField(payload, 'status') ?? '',
    expires_at: optionalRoundedNumber(payload, 'expires_at', undefined),
    owned_by_you: booleanField(payload, 'owned_by_you') ?? false,
    final_price_pending:
      booleanField(payload, 'final_price_pending') ?? booleanField(payload, 'server_recalculates') ?? true,
    estimated_unit_purchase: {
      quantity: Math.max(0, Math.round(numberField(estimate, 'quantity') ?? 0)),
      subtotal: Math.max(0, Math.round(numberField(estimate, 'subtotal') ?? 0)),
      currency_type: stringField(estimate, 'currency_type') ?? stringField(payload, 'currency_type') ?? 'credits',
      pending: booleanField(estimate, 'pending') ?? true,
    },
  };
}

function parseAuctionSummary(payload: JsonObject, fallback: AuctionSummary | null): AuctionSummary {
  const lots = Array.isArray(payload.lots)
    ? payload.lots
        .filter(isJsonObject)
        .map(parseAuctionLot)
        .filter((lot): lot is AuctionLotSummary => lot !== null)
    : fallback?.lots ?? [];
  const grants = Array.isArray(payload.grants)
    ? payload.grants
        .filter(isJsonObject)
        .map(parseAuctionGrant)
        .filter((grant): grant is AuctionGrantSummary => grant !== null)
    : fallback?.grants ?? [];
  return { lots, grants };
}

function parseAuctionLot(payload: JsonObject): AuctionLotSummary | null {
  const auctionID = stringField(payload, 'auction_id') ?? '';
  if (!auctionID) {
    return null;
  }
  return {
    auction_id: auctionID,
    payload_type: stringField(payload, 'payload_type') ?? '',
    definition_id: stringField(payload, 'definition_id') ?? '',
    quantity: Math.max(0, Math.round(numberField(payload, 'quantity') ?? 0)),
    currency_type: stringField(payload, 'currency_type') ?? 'credits',
    start_price: Math.max(0, Math.round(numberField(payload, 'start_price') ?? 0)),
    current_bid: Math.max(0, Math.round(numberField(payload, 'current_bid') ?? 0)),
    has_bid: booleanField(payload, 'has_bid') ?? false,
    leading: booleanField(payload, 'leading') ?? false,
    buy_now_price: optionalRoundedNumber(payload, 'buy_now_price', undefined),
    status: stringField(payload, 'status') ?? '',
    starts_at: Math.max(0, Math.round(numberField(payload, 'starts_at') ?? 0)),
    ends_at: Math.max(0, Math.round(numberField(payload, 'ends_at') ?? 0)),
    final_price_pending:
      booleanField(payload, 'final_price_pending') ?? booleanField(payload, 'server_recalculates') ?? true,
  };
}

function parseAuctionGrant(payload: JsonObject): AuctionGrantSummary | null {
  const auctionID = stringField(payload, 'auction_id') ?? '';
  if (!auctionID) {
    return null;
  }
  return {
    auction_id: auctionID,
    payload_type: stringField(payload, 'payload_type') ?? '',
    definition_id: stringField(payload, 'definition_id') ?? '',
    quantity: Math.max(0, Math.round(numberField(payload, 'quantity') ?? 0)),
    reason: stringField(payload, 'reason') ?? '',
    granted_at: Math.max(0, Math.round(numberField(payload, 'granted_at') ?? 0)),
  };
}

function parsePremiumSummary(payload: JsonObject, fallback: PremiumSummary | null): PremiumSummary {
  const entitlements = Array.isArray(payload.entitlements)
    ? payload.entitlements
        .filter(isJsonObject)
        .map(parsePremiumEntitlement)
        .filter((entitlement): entitlement is PremiumEntitlementSummary => entitlement !== null)
    : fallback?.entitlements ?? [];
  const stock = Array.isArray(payload.stock)
    ? payload.stock
        .filter(isJsonObject)
        .map(parsePremiumStock)
        .filter((record): record is PremiumStockSummary => record !== null)
    : fallback?.stock ?? [];
  const purchases = Array.isArray(payload.purchases)
    ? payload.purchases
        .filter(isJsonObject)
        .map(parsePremiumPurchase)
        .filter((purchase): purchase is PremiumPurchaseSummary => purchase !== null)
    : fallback?.purchases ?? [];
  return { entitlements, stock, purchases };
}

function parsePremiumEntitlement(payload: JsonObject): PremiumEntitlementSummary | null {
  const entitlementID = stringField(payload, 'entitlement_id') ?? '';
  const grant = objectField(payload, 'payload') ?? {};
  if (!entitlementID) {
    return null;
  }
  return {
    entitlement_id: entitlementID,
    type: stringField(payload, 'type') ?? '',
    state: stringField(payload, 'state') ?? '',
    payload: {
      currency_bucket: stringField(grant, 'currency_bucket') ?? undefined,
      amount: optionalRoundedNumber(grant, 'amount', undefined),
      loadout_slot_scope: stringField(grant, 'loadout_slot_scope') ?? undefined,
      loadout_slot_count: optionalRoundedNumber(grant, 'loadout_slot_count', undefined),
      period_key: stringField(grant, 'period_key') ?? undefined,
      cosmetic_id: stringField(grant, 'cosmetic_id') ?? undefined,
      badge_id: stringField(grant, 'badge_id') ?? undefined,
    },
    created_at: Math.max(0, Math.round(numberField(payload, 'created_at') ?? 0)),
    claimed_at: optionalRoundedNumber(payload, 'claimed_at', undefined),
  };
}

function parsePremiumStock(payload: JsonObject): PremiumStockSummary | null {
  const periodKey = stringField(payload, 'period_key') ?? '';
  if (!periodKey) {
    return null;
  }
  return {
    period_key: periodKey,
    stock_total: Math.max(0, Math.round(numberField(payload, 'stock_total') ?? 0)),
    stock_remaining: Math.max(0, Math.round(numberField(payload, 'stock_remaining') ?? 0)),
    price_amount: Math.max(0, Math.round(numberField(payload, 'price_amount') ?? 0)),
    payment_currency: stringField(payload, 'payment_currency') ?? 'premium_paid',
  };
}

function parsePremiumPurchase(payload: JsonObject): PremiumPurchaseSummary | null {
  const periodKey = stringField(payload, 'period_key') ?? '';
  if (!periodKey) {
    return null;
  }
  return {
    period_key: periodKey,
    payment_currency: stringField(payload, 'payment_currency') ?? 'premium_paid',
    granted_at: Math.max(0, Math.round(numberField(payload, 'granted_at') ?? 0)),
  };
}

function parseQuestBoardSummary(payload: JsonObject, fallback: QuestBoardSummary | null): QuestBoardSummary {
  const offers = Array.isArray(payload.offers)
    ? payload.offers
        .filter(isJsonObject)
        .map(parseQuestOffer)
        .filter((offer): offer is QuestOfferSummary => offer !== null)
    : fallback?.offers ?? [];
  const active = Array.isArray(payload.active)
    ? payload.active
        .filter(isJsonObject)
        .map((quest) => parseQuestSummary(quest, null))
        .filter((quest): quest is QuestSummary => quest !== null)
    : fallback?.active ?? [];
  const counts = objectField(payload, 'counts') ?? {};
  const rerollCost = objectField(payload, 'reroll_cost') ?? {};
  return {
    offers,
    active,
    counts: {
      offers: Math.max(0, Math.round(numberField(counts, 'offers') ?? fallback?.counts.offers ?? offers.length)),
      active: Math.max(0, Math.round(numberField(counts, 'active') ?? fallback?.counts.active ?? countQuests(active, 'accepted'))),
      completed: Math.max(0, Math.round(numberField(counts, 'completed') ?? fallback?.counts.completed ?? countQuests(active, 'completed'))),
      claimable: Math.max(0, Math.round(numberField(counts, 'claimable') ?? fallback?.counts.claimable ?? active.filter((quest) => quest.can_claim).length)),
      claimed: Math.max(0, Math.round(numberField(counts, 'claimed') ?? fallback?.counts.claimed ?? countQuests(active, 'claimed'))),
    },
    reroll_cost: {
      currency_type: stringField(rerollCost, 'currency_type') ?? fallback?.reroll_cost.currency_type ?? 'credits',
      amount: Math.max(0, Math.round(numberField(rerollCost, 'amount') ?? fallback?.reroll_cost.amount ?? 0)),
    },
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

function parseQuestOffer(payload: JsonObject): QuestOfferSummary | null {
  const offerID = stringField(payload, 'offer_id') ?? '';
  if (!offerID) {
    return null;
  }
  return {
    offer_id: offerID,
    quest_type: stringField(payload, 'quest_type') ?? '',
    title: stringField(payload, 'title') ?? offerID,
    description: stringField(payload, 'description') ?? '',
    objectives: parseQuestObjectives(payload),
    rewards: parseQuestRewards(payload),
    expires_at: Math.max(0, Math.round(numberField(payload, 'expires_at') ?? 0)),
  };
}

function parseQuestSummary(payload: JsonObject, fallback: QuestSummary | null): QuestSummary | null {
  const questID = stringField(payload, 'quest_id') ?? fallback?.quest_id ?? '';
  if (!questID) {
    return null;
  }
  return {
    quest_id: questID,
    quest_type: stringField(payload, 'quest_type') ?? fallback?.quest_type ?? '',
    title: stringField(payload, 'title') ?? fallback?.title ?? questID,
    description: stringField(payload, 'description') ?? fallback?.description ?? '',
    state: stringField(payload, 'state') ?? fallback?.state ?? '',
    objectives: Array.isArray(payload.objectives) ? parseQuestObjectives(payload) : fallback?.objectives ?? [],
    rewards: Array.isArray(payload.rewards) ? parseQuestRewards(payload) : fallback?.rewards ?? [],
    accepted_at: Math.max(0, Math.round(numberField(payload, 'accepted_at') ?? fallback?.accepted_at ?? 0)),
    completed_at: optionalRoundedNumber(payload, 'completed_at', fallback?.completed_at),
    claimed_at: optionalRoundedNumber(payload, 'claimed_at', fallback?.claimed_at),
    can_claim: booleanField(payload, 'can_claim') ?? fallback?.can_claim ?? false,
  };
}

function parseQuestObjectives(payload: JsonObject): QuestObjectiveSummary[] {
  return Array.isArray(payload.objectives)
    ? payload.objectives
        .filter(isJsonObject)
        .map(parseQuestObjective)
        .filter((objective): objective is QuestObjectiveSummary => objective !== null)
    : [];
}

function parseQuestObjective(payload: JsonObject): QuestObjectiveSummary | null {
  const id = stringField(payload, 'id') ?? '';
  if (!id) {
    return null;
  }
  return {
    id,
    kind: stringField(payload, 'kind') ?? '',
    target: stringField(payload, 'target') ?? undefined,
    current: Math.max(0, Math.round(numberField(payload, 'current') ?? 0)),
    required: Math.max(0, Math.round(numberField(payload, 'required') ?? 0)),
    completed: booleanField(payload, 'completed') ?? false,
  };
}

function parseQuestRewards(payload: JsonObject): QuestRewardSummary[] {
  return Array.isArray(payload.rewards)
    ? payload.rewards
        .filter(isJsonObject)
        .map(parseQuestReward)
        .filter((reward): reward is QuestRewardSummary => reward !== null)
    : [];
}

function parseQuestReward(payload: JsonObject): QuestRewardSummary | null {
  const kind = stringField(payload, 'kind') ?? '';
  const amount = numberField(payload, 'amount') ?? 0;
  if (!kind || amount <= 0) {
    return null;
  }
  return {
    kind,
    currency_type: stringField(payload, 'currency_type') ?? undefined,
    item_id: stringField(payload, 'item_id') ?? undefined,
    role: stringField(payload, 'role') ?? undefined,
    amount: Math.max(0, Math.round(amount)),
  };
}

function parseAdminInspection(payload: JsonObject, fallback: AdminInspectionSummary | null): AdminInspectionSummary {
  const inventory = objectField(payload, 'inventory') ?? {};
  const wallet = objectField(payload, 'wallet') ?? {};
  return {
    target: stringField(payload, 'target') ?? fallback?.target ?? '',
    inventory: {
      stackable_items: Math.max(0, Math.round(numberField(inventory, 'stackable_items') ?? fallback?.inventory.stackable_items ?? 0)),
      instance_items: Math.max(0, Math.round(numberField(inventory, 'instance_items') ?? fallback?.inventory.instance_items ?? 0)),
      item_ledger: Array.isArray(inventory.item_ledger)
        ? inventory.item_ledger
            .filter(isJsonObject)
            .map((entry) => ({
              ledger_id: stringField(entry, 'ledger_id') ?? '',
              item_id: stringField(entry, 'item_id') ?? '',
              quantity: Math.round(numberField(entry, 'quantity') ?? 0),
              action: stringField(entry, 'action') ?? '',
              balance_after: Math.round(numberField(entry, 'balance_after') ?? 0),
              location: stringField(entry, 'location') ?? '',
              reason: stringField(entry, 'reason') ?? '',
              created_at: Math.max(0, Math.round(numberField(entry, 'created_at') ?? 0)),
            }))
            .filter((entry) => entry.ledger_id !== '')
        : fallback?.inventory.item_ledger ?? [],
    },
    wallet: {
      balances: Array.isArray(wallet.balances)
        ? wallet.balances
            .filter(isJsonObject)
            .map((balance) => ({
              currency_type: stringField(balance, 'currency_type') ?? '',
              balance: Math.round(numberField(balance, 'balance') ?? 0),
            }))
            .filter((balance) => balance.currency_type !== '')
        : fallback?.wallet.balances ?? [],
      ledger: Array.isArray(wallet.ledger)
        ? wallet.ledger
            .filter(isJsonObject)
            .map((entry) => ({
              ledger_id: stringField(entry, 'ledger_id') ?? '',
              currency_type: stringField(entry, 'currency_type') ?? '',
              amount: Math.round(numberField(entry, 'amount') ?? 0),
              action: stringField(entry, 'action') ?? '',
              balance_after: Math.round(numberField(entry, 'balance_after') ?? 0),
              reason: stringField(entry, 'reason') ?? '',
              created_at: Math.max(0, Math.round(numberField(entry, 'created_at') ?? 0)),
            }))
            .filter((entry) => entry.ledger_id !== '')
        : fallback?.wallet.ledger ?? [],
    },
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

function parseAdminRepairCraftJob(payload: JsonObject, fallback: AdminRepairCraftJobSummary | null): AdminRepairCraftJobSummary {
  return {
    accepted: booleanField(payload, 'accepted') ?? fallback?.accepted ?? false,
    job_id: stringField(payload, 'job_id') ?? fallback?.job_id,
    status: stringField(payload, 'status') ?? fallback?.status ?? '',
    already_complete: booleanField(payload, 'already_complete') ?? fallback?.already_complete,
    message: stringField(payload, 'message') ?? fallback?.message,
  };
}

function parseCommandLogSummary(payload: JsonObject, fallback: CommandLogSummary | null): CommandLogSummary {
  return {
    entries: Array.isArray(payload.entries)
      ? payload.entries
          .filter(isJsonObject)
          .map((entry) => ({
            request_id: stringField(entry, 'request_id') ?? '',
            operation: stringField(entry, 'operation') ?? '',
            status: stringField(entry, 'status') ?? '',
            error_code: stringField(entry, 'error_code') ?? undefined,
            duration_ms: Math.max(0, Math.round(numberField(entry, 'duration_ms') ?? 0)),
            timestamp: Math.max(0, Math.round(numberField(entry, 'timestamp') ?? 0)),
          }))
          .filter((entry) => entry.request_id !== '')
      : fallback?.entries ?? [],
    total: Math.max(0, Math.round(numberField(payload, 'total') ?? fallback?.total ?? 0)),
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

function parseMetricsSummary(payload: JsonObject, fallback: MetricsSummary | null): MetricsSummary {
  const snapshot = objectField(payload, 'snapshot') ?? {};
  return {
    snapshot: {
      counters: Array.isArray(snapshot.counters)
        ? snapshot.counters.filter(isJsonObject).map(parseMetricCounter)
        : fallback?.snapshot.counters ?? [],
      gauges: Array.isArray(snapshot.gauges)
        ? snapshot.gauges.filter(isJsonObject).map(parseMetricCounter)
        : fallback?.snapshot.gauges ?? [],
      durations: Array.isArray(snapshot.durations)
        ? snapshot.durations.filter(isJsonObject).map(parseMetricDuration)
        : fallback?.snapshot.durations ?? [],
    },
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

function parseMetricCounter(payload: JsonObject): { name: string; value: number; labels: Array<{ name: string; value: string }> } {
  return {
    name: stringField(payload, 'name') ?? '',
    value: Math.round(numberField(payload, 'value') ?? 0),
    labels: parseMetricLabels(payload),
  };
}

function parseMetricDuration(payload: JsonObject): MetricsSummary['snapshot']['durations'][number] {
  return {
    name: stringField(payload, 'name') ?? '',
    labels: parseMetricLabels(payload),
    count: Math.max(0, Math.round(numberField(payload, 'count') ?? 0)),
    total: Math.max(0, Math.round(numberField(payload, 'total') ?? 0)),
    minimum: Math.max(0, Math.round(numberField(payload, 'minimum') ?? 0)),
    maximum: Math.max(0, Math.round(numberField(payload, 'maximum') ?? 0)),
    p50: Math.max(0, Math.round(numberField(payload, 'p50') ?? 0)),
    p95: Math.max(0, Math.round(numberField(payload, 'p95') ?? 0)),
    p99: Math.max(0, Math.round(numberField(payload, 'p99') ?? 0)),
  };
}

function parseMetricLabels(payload: JsonObject): Array<{ name: string; value: string }> {
  return Array.isArray(payload.labels)
    ? payload.labels
        .filter(isJsonObject)
        .map((label) => ({
          name: stringField(label, 'name') ?? '',
          value: stringField(label, 'value') ?? '',
        }))
        .filter((label) => label.name !== '')
    : [];
}

function parseReleaseGateSummary(payload: JsonObject, fallback: ReleaseGateSummary | null): ReleaseGateSummary {
  const report = objectField(payload, 'report') ?? {};
  return {
    report: {
      covered: booleanField(report, 'covered') ?? fallback?.report.covered ?? false,
      passed: booleanField(report, 'passed') ?? fallback?.report.passed ?? false,
      missing: Array.isArray(report.missing)
        ? report.missing
            .filter(isJsonObject)
            .map((item) => ({
              module: stringField(item, 'module') ?? '',
              check: stringField(item, 'check') ?? '',
            }))
            .filter((item) => item.module !== '' && item.check !== '')
        : fallback?.report.missing ?? [],
    },
    coverage: Array.isArray(payload.coverage)
      ? payload.coverage
          .filter(isJsonObject)
          .map((item) => ({
            module: stringField(item, 'module') ?? '',
            passed: booleanField(item, 'passed') ?? false,
            missing: Array.isArray(item.missing) ? item.missing.filter((check): check is string => typeof check === 'string') : [],
            evidence: Math.max(0, Math.round(numberField(item, 'evidence') ?? 0)),
          }))
          .filter((item) => item.module !== '')
      : fallback?.coverage ?? [],
    evidence: Math.max(0, Math.round(numberField(payload, 'evidence') ?? fallback?.evidence ?? 0)),
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

function parseAbuseCoverageSummary(payload: JsonObject, fallback: AbuseCoverageSummary | null): AbuseCoverageSummary {
  const report = objectField(payload, 'report') ?? {};
  return {
    report: {
      passed: booleanField(report, 'passed') ?? fallback?.report.passed ?? false,
      missing: Array.isArray(report.missing) ? report.missing.filter((item): item is string => typeof item === 'string') : fallback?.report.missing ?? [],
    },
    coverage: Array.isArray(payload.coverage)
      ? payload.coverage
          .filter(isJsonObject)
          .map((item) => ({
            case: stringField(item, 'case') ?? '',
            evidence: Array.isArray(item.evidence)
              ? item.evidence
                  .filter(isJsonObject)
                  .map((evidence) => ({
                    package: stringField(evidence, 'package') ?? '',
                    test_name: stringField(evidence, 'test_name') ?? '',
                    note: stringField(evidence, 'note') ?? '',
                  }))
                  .filter((evidence) => evidence.test_name !== '')
              : [],
          }))
          .filter((item) => item.case !== '')
      : fallback?.coverage ?? [],
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
  };
}

function parseEconomyDashboard(payload: JsonObject, fallback: EconomyDashboardSummary | null): EconomyDashboardSummary {
  const wallets = objectField(payload, 'wallets') ?? {};
  const market = objectField(payload, 'market') ?? {};
  const auction = objectField(payload, 'auction') ?? {};
  const premium = objectField(payload, 'premium') ?? {};
  return {
    wallets: {
      credits: Math.max(0, Math.round(numberField(wallets, 'credits') ?? fallback?.wallets.credits ?? 0)),
      premium_paid: Math.max(0, Math.round(numberField(wallets, 'premium_paid') ?? fallback?.wallets.premium_paid ?? 0)),
      premium_earned: Math.max(0, Math.round(numberField(wallets, 'premium_earned') ?? fallback?.wallets.premium_earned ?? 0)),
    },
    market: {
      active_listings: Math.max(0, Math.round(numberField(market, 'active_listings') ?? fallback?.market.active_listings ?? 0)),
      sold_listings: Math.max(0, Math.round(numberField(market, 'sold_listings') ?? fallback?.market.sold_listings ?? 0)),
      volume_credits: Math.max(0, Math.round(numberField(market, 'volume_credits') ?? fallback?.market.volume_credits ?? 0)),
    },
    auction: {
      active_lots: Math.max(0, Math.round(numberField(auction, 'active_lots') ?? fallback?.auction.active_lots ?? 0)),
      closed_lots: Math.max(0, Math.round(numberField(auction, 'closed_lots') ?? fallback?.auction.closed_lots ?? 0)),
      grants: Math.max(0, Math.round(numberField(auction, 'grants') ?? fallback?.auction.grants ?? 0)),
    },
    premium: {
      pending_entitlements: Math.max(0, Math.round(numberField(premium, 'pending_entitlements') ?? fallback?.premium.pending_entitlements ?? 0)),
      claimed_entitlements: Math.max(0, Math.round(numberField(premium, 'claimed_entitlements') ?? fallback?.premium.claimed_entitlements ?? 0)),
      weekly_stock_remaining: Math.max(0, Math.round(numberField(premium, 'weekly_stock_remaining') ?? fallback?.premium.weekly_stock_remaining ?? 0)),
    },
    generated_at: Math.max(0, Math.round(numberField(payload, 'generated_at') ?? fallback?.generated_at ?? 0)),
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
    ? payload.remembered
        .filter(isJsonObject)
        .map(parseMinimapMemory)
        .filter((memory): memory is MinimapMemory => memory !== null)
    : fallback?.remembered ?? [];

  return {
    radar_range: Math.max(0, numberField(payload, 'radar_range') ?? fallback?.radar_range ?? 0),
    projection_window_size: optionalRoundedNumber(payload, 'projection_window_size', fallback?.projection_window_size),
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
    status_flags: parsePublicStatusFlags(payload.status_flags),
  };
}

function parseMinimapMemory(payload: JsonObject): MinimapMemory | null {
  if (!isVec2(payload.position)) {
    return null;
  }
  return {
    kind: stringField(payload, 'kind') ?? '',
    planet_id: stringField(payload, 'planet_id') ?? undefined,
    detail_id: stringField(payload, 'detail_id') ?? undefined,
    label: stringField(payload, 'label') ?? '',
    position: payload.position,
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

function parseEntityMovement(payload: JsonObject): EntityPayload['movement'] {
  const movement = objectField(payload, 'movement');
  if (!movement) {
    return undefined;
  }

  const moving = booleanField(movement, 'moving');
  const origin = isVec2(movement.origin) ? movement.origin : null;
  const target = isVec2(movement.target) ? movement.target : null;
  const speed = numberField(movement, 'speed');
  const startedAt = numberField(movement, 'started_at_ms');
  const arriveAt = numberField(movement, 'arrive_at_ms');
  if (moving !== true || !origin || !target || speed === null || startedAt === null || arriveAt === null) {
    throw new Error('Invalid entity movement payload.');
  }
  if (speed <= 0 || arriveAt < startedAt) {
    throw new Error('Invalid entity movement timing.');
  }

  return {
    moving,
    origin,
    target,
    speed,
    started_at_ms: startedAt,
    arrive_at_ms: arriveAt,
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

function roundedOptional(payload: JsonObject, key: string): number | undefined {
  const value = numberField(payload, key);
  return value === null ? undefined : Math.max(0, Math.round(value));
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

function emptyPlanetIntel(): PlanetIntelSummary {
  return {
    knownSignals: 0,
    staleIntel: null,
    ownedPlanets: 0,
    planets: [],
    selectedPlanet: null,
    lastScan: null,
  };
}

function updateVisibleSignalCount(fallback: PlanetIntelSummary | null, visibleSignals: number): PlanetIntelSummary {
  const next = fallback ?? emptyPlanetIntel();
  return {
    ...next,
    knownSignals: Math.max(visibleSignals, next.planets.length, next.knownSignals),
  };
}

function applyScanPulse(fallback: PlanetIntelSummary | null, scan: ScanPulseSummary): PlanetIntelSummary {
  const next = fallback ?? emptyPlanetIntel();
  return {
    ...next,
    knownSignals: scan.planet_id ? Math.max(next.knownSignals, next.planets.length, 1) : next.knownSignals,
    lastScan: scan,
  };
}

function initialScanMode(): ScanModeState {
  return {
    enabled: false,
    nextPulseAt: null,
    lastRejectedAt: null,
    lastError: null,
  };
}

function scanModeAfterPulseSummary(mode: ScanModeState, scan: ScanPulseSummary): ScanModeState {
  if (scan.status === 'started') {
    return scanModeAfterPulseStarted(mode, scan);
  }
  if (scan.status === 'resolved' || scan.status === 'planet_discovered' || scan.status === 'no_signal') {
    return scanModeAfterPulseResolved(mode);
  }
  return mode;
}

function scanModeAfterPulseStarted(mode: ScanModeState, scan: ScanPulseSummary): ScanModeState {
  if (!mode.enabled) {
    return mode;
  }
  return {
    ...mode,
    nextPulseAt: scanResolveWakeAt(scan.resolve_after),
    lastError: null,
  };
}

function scanModeAfterPulseResolved(mode: ScanModeState): ScanModeState {
  if (!mode.enabled) {
    return mode;
  }
  return {
    ...mode,
    nextPulseAt: Date.now() + SCAN_REPEAT_DELAY_MS,
    lastError: null,
  };
}

function scanResolveWakeAt(resolveAfter: number | undefined): number {
  const now = Date.now();
  if (Number.isFinite(resolveAfter) && typeof resolveAfter === 'number' && resolveAfter > now) {
    return Math.round(resolveAfter);
  }
  return now + SCAN_STARTED_RECHECK_MS;
}

function applyPlanetDetail(fallback: PlanetIntelSummary | null, detail: PlanetDetailSummary): PlanetIntelSummary {
  const next = fallback ?? emptyPlanetIntel();
  const planets = next.planets.some((planet) => planet.planet_id === detail.planet_id)
    ? next.planets.map((planet) => (planet.planet_id === detail.planet_id ? { ...planet, ...detail } : planet))
    : detail.planet_id
      ? [...next.planets, detail]
      : next.planets;
  return {
    ...next,
    planets,
    knownSignals: Math.max(next.knownSignals, planets.length),
    selectedPlanet: detail.planet_id ? detail : next.selectedPlanet,
  };
}

function applyQuestUpdate(board: QuestBoardSummary | null, quest: QuestSummary): QuestBoardSummary | null {
  if (!board) {
    return null;
  }
  const active = board.active.some((item) => item.quest_id === quest.quest_id)
    ? board.active.map((item) => (item.quest_id === quest.quest_id ? quest : item))
    : [...board.active, quest];
  return {
    ...board,
    active,
    counts: {
      offers: board.offers.length,
      active: countQuests(active, 'accepted'),
      completed: countQuests(active, 'completed'),
      claimable: active.filter((item) => item.can_claim).length,
      claimed: countQuests(active, 'claimed'),
    },
  };
}

function countQuests(quests: QuestSummary[], state: string): number {
  return quests.filter((quest) => quest.state === state).length;
}

function scanLogLine(scan: ScanPulseSummary): string {
  if (scan.status === 'planet_discovered') {
    return `Scanner resolved ${scan.signal?.signal_band ?? 'unknown'} ${scan.signal?.biome ?? 'signal'}.`;
  }
  if (scan.status === 'started') {
    return 'Scanner pulse started.';
  }
  if (scan.status === 'player_revealed') {
    return scan.message || 'Scanner revealed a radar contact.';
  }
  return scan.message || 'Scanner pulse resolved with no signal.';
}

function questEventLog(eventType: string): string {
  switch (eventType) {
    case CLIENT_EVENTS.questAccepted:
      return 'Quest accepted.';
    case CLIENT_EVENTS.questProgressed:
      return 'Quest progress updated.';
    case CLIENT_EVENTS.questCompleted:
      return 'Quest completed.';
    case CLIENT_EVENTS.questRewardClaimed:
      return 'Quest reward claimed.';
    case CLIENT_EVENTS.questAbandoned:
      return 'Quest abandoned.';
    default:
      return 'Quest update received.';
  }
}

function economyEventLog(eventType: string): string {
  switch (eventType) {
    case CLIENT_EVENTS.marketListingCreated:
      return 'Market listing created.';
    case CLIENT_EVENTS.marketSaleCompleted:
      return 'Market sale completed.';
    case CLIENT_EVENTS.marketListingCancelled:
      return 'Market listing cancelled.';
    case CLIENT_EVENTS.auctionBidPlaced:
      return 'Auction bid placed.';
    case CLIENT_EVENTS.auctionClosed:
      return 'Auction lot closed.';
    case CLIENT_EVENTS.premiumEntitlementClaimed:
      return 'Premium entitlement claimed.';
    case CLIENT_EVENTS.premiumStockConsumed:
      return 'Premium weekly stock consumed.';
    case CLIENT_EVENTS.economyFlowUpdated:
      return 'Economy flow updated.';
    default:
      return 'Economy update received.';
  }
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
    knownLoot: {},
    worldEffects: [],
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
    market: null,
    auction: null,
    premium: null,
    economyDashboard: null,
    adminInspection: null,
    adminRepair: null,
    commandLogSummary: null,
    metrics: null,
    releaseGate: null,
    abuseCoverage: null,
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

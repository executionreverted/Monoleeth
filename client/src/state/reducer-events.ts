import { CLIENT_EVENTS, EventEnvelope, OPERATIONS, rejectForbiddenPayloadKeys } from '../protocol/envelope';
import type { ClientState, MapTransferState } from './types';
import { appendLog, numberField, objectField, stringField } from './reducer-helpers';
import {
  applyPlanetDetail,
  applyPlanetClaimed,
  applyPlanetStorageSummary,
  applyRouteSnapshot,
  applyScanPulse,
  countPlanetSignals,
  parseKnownPlanets,
  parseMinimapSummary,
  parsePlanetDetail,
  parsePlanetStorage,
  parseProductionCollection,
  parseRoute,
  parseRouteList,
  parseScanPulse,
  scanLogLine,
  scanModeAfterPulseResolved,
  scanModeAfterPulseStarted,
  updateVisibleSignalCount,
} from './reducer-discovery';
import { economyEventLog } from './reducer-economy';
import {
  applyDeathShipDisabled,
  parseCargoSummary,
  parseCraftingSummary,
  parseHangarSummary,
  parseInventorySummary,
  parseLoadoutSummary,
  parseProgressionSummary,
  parseSessionReady,
  parseShipSummary,
  parseStatSummary,
  parseWalletSummary,
} from './reducer-player-parsers';
import { applyQuestUpdate, parseAdminRepairCraftJob, parseQuestBoardSummary, parseQuestSummary, questEventLog } from './reducer-quests-admin';
import {
  applyMapPolicyUpdatedPayload,
  applyMapSnapshotPayload,
  applyPlayerProtectionUpdatedPayload,
  mapSubscriptionEpochFromPayload,
} from './reducer-map';
import { applySnapshotPayload } from './reducer-snapshot';
import {
  appendWorldEffect,
  applyCorrection,
  applyTargetUpdated,
  clearOriginMapLiveState,
  displayNameForEntity,
  feedbackEffect,
  movementTargetFromAuthoritativeSelf,
  parseEntityMovement,
  parseEntityPayload,
  parseKnownLootDrop,
  parseSnapshotEntities,
  removeMinimapContact,
  replaceVisibleEntities,
  requireEntityID,
  requirePosition,
  upsertMinimapContact,
  withoutPendingOperations,
} from './reducer-world';

export function applyEvent(state: ClientState, envelope: EventEnvelope): ClientState {
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

    case CLIENT_EVENTS.planetClaimed: {
      const nextState = withoutPendingPlanetClaim(state, envelope.payload);
      return {
        ...nextState,
        planetIntel: applyPlanetClaimed(nextState.planetIntel, envelope.payload),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        commandLog: appendLog(nextState.commandLog, 'info', 'Planet claim accepted.'),
      };
    }

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

    case CLIENT_EVENTS.marketListingCreated: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.marketCreateListing]);
      return {
        ...nextState,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        commandLog: appendLog(nextState.commandLog, 'info', economyEventLog(envelope.type)),
      };
    }

    case CLIENT_EVENTS.marketListingUpdated:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', economyEventLog(envelope.type)),
      };

    case CLIENT_EVENTS.marketSaleCompleted: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.marketBuy]);
      return {
        ...nextState,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        commandLog: appendLog(nextState.commandLog, 'info', economyEventLog(envelope.type)),
      };
    }

    case CLIENT_EVENTS.marketListingCancelled: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.marketCancel]);
      return {
        ...nextState,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        commandLog: appendLog(nextState.commandLog, 'info', economyEventLog(envelope.type)),
      };
    }

    case CLIENT_EVENTS.auctionLotUpdated:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', economyEventLog(envelope.type)),
      };

    case CLIENT_EVENTS.auctionClosed: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.auctionBid, OPERATIONS.auctionBuyNow]);
      return {
        ...nextState,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        commandLog: appendLog(nextState.commandLog, 'info', economyEventLog(envelope.type)),
      };
    }

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
    case CLIENT_EVENTS.economyFlowUpdated:
      return {
        ...state,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', economyEventLog(envelope.type)),
      };

    case CLIENT_EVENTS.premiumEntitlementClaimed: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.premiumClaim]);
      return {
        ...nextState,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        commandLog: appendLog(nextState.commandLog, 'info', economyEventLog(envelope.type)),
      };
    }

    case CLIENT_EVENTS.premiumStockConsumed: {
      const nextState = withoutPendingOperations(state, [OPERATIONS.premiumPurchaseWeeklyXCore]);
      return {
        ...nextState,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(nextState.lastSequence, envelope.seq),
        commandLog: appendLog(nextState.commandLog, 'info', economyEventLog(envelope.type)),
      };
    }

    case CLIENT_EVENTS.questBoardGenerated:
      return {
        ...state,
        questBoard: parseQuestBoardSummary(envelope.payload, state.questBoard, envelope.server_time),
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
        commandLog: appendLog(state.commandLog, 'info', 'Quest board refreshed.'),
      };

    case CLIENT_EVENTS.questBoardRerolled: {
      const board = objectField(envelope.payload, 'quest_board');
      return {
        ...state,
        questBoard: board ? parseQuestBoardSummary(board, state.questBoard, envelope.server_time) : state.questBoard,
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

    case CLIENT_EVENTS.mapSnapshot: {
      const withSnapshotPayload = applySnapshotPayload(state, envelope.payload);
      return {
        ...withSnapshotPayload,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(withSnapshotPayload.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.mapChanged: {
      const withSnapshotPayload = applyMapSnapshotPayload(state, envelope.payload, {
        clearMapScopedState: clearOriginMapLiveState,
        forceClearMapScopedState: true,
      });
      return {
        ...withSnapshotPayload,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(withSnapshotPayload.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.portalCooldownStarted: {
      const portalID = stringField(envelope.payload, 'portal_id')?.trim();
      const readyAt =
        numberField(envelope.payload, 'cooldown_ready_at_ms') ??
        numberField(envelope.payload, 'ready_at_ms') ??
        numberField(envelope.payload, 'expires_at');
      return {
        ...state,
        portalCooldowns:
          portalID && readyAt !== null
            ? { ...state.portalCooldowns, [portalID]: Math.max(0, Math.round(readyAt)) }
            : state.portalCooldowns,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.mapPolicyUpdated: {
      const withPolicyUpdate = applyMapPolicyUpdatedPayload(state, envelope.payload);
      return {
        ...withPolicyUpdate,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(withPolicyUpdate.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.playerProtectionUpdated: {
      const withProtectionUpdate = applyPlayerProtectionUpdatedPayload(state, envelope.payload);
      return {
        ...withProtectionUpdate,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(withProtectionUpdate.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.mapTransferStarted: {
      const eventEpoch = mapSubscriptionEpochFromPayload(envelope.payload);
      if (eventEpoch !== null && state.mapSubscriptionEpoch !== null && eventEpoch < state.mapSubscriptionEpoch) {
        return {
          ...state,
          lastServerTime: envelope.server_time,
          lastSequence: Math.max(state.lastSequence, envelope.seq),
        };
      }
      return {
        ...state,
        mapTransfer: parseMapTransferStarted(envelope.payload, envelope.server_time),
        mapSubscriptionEpoch: eventEpoch ?? state.mapSubscriptionEpoch,
        lastServerTime: envelope.server_time,
        lastSequence: Math.max(state.lastSequence, envelope.seq),
      };
    }

    case CLIENT_EVENTS.mapTransferCompleted: {
      const snapshotPayload = objectField(envelope.payload, 'snapshot');
      if (!snapshotPayload) {
        return {
          ...state,
          lastServerTime: envelope.server_time,
          lastSequence: Math.max(state.lastSequence, envelope.seq),
        };
      }
      const entities = parseSnapshotEntities(snapshotPayload);
      if (!entities) {
        return {
          ...state,
          lastServerTime: envelope.server_time,
          lastSequence: Math.max(state.lastSequence, envelope.seq),
        };
      }
      const cleared = clearOriginMapLiveState(state);
      const withSnapshotPayload = applySnapshotPayload(cleared, snapshotPayload);
      return {
        ...replaceVisibleEntities(withSnapshotPayload, entities, envelope.server_time, envelope.seq),
        mapTransfer: null,
        mapSubscriptionEpoch:
          mapSubscriptionEpochFromPayload(envelope.payload) ??
          mapSubscriptionEpochFromPayload(snapshotPayload) ??
          withSnapshotPayload.mapSubscriptionEpoch,
        connectionStatus: state.auth.mode === 'real' && state.auth.session ? 'connected' : state.connectionStatus,
      };
    }

    case CLIENT_EVENTS.mapTransferFailed:
      return {
        ...state,
        mapTransfer: parseMapTransferFailed(envelope.payload, envelope.server_time),
        lastError: {
          code: 'ERR_FORBIDDEN',
          message: stringField(envelope.payload, 'reason') ?? 'Map transfer failed.',
          retryable: false,
        },
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

function withoutPendingPlanetClaim(state: ClientState, payload: EventEnvelope['payload']): ClientState {
  const planetPayload = objectField(payload, 'planet');
  const planetID = planetPayload ? stringField(planetPayload, 'planet_id') : null;
  if (!planetID) {
    return state;
  }
  const pendingCommands: ClientState['pendingCommands'] = {};
  let changed = false;
  for (const [requestID, pending] of Object.entries(state.pendingCommands)) {
    if (pending.op === OPERATIONS.discoveryClaimPlanet && pending.payload?.planet_id === planetID) {
      changed = true;
      continue;
    }
    pendingCommands[requestID] = pending;
  }
  return changed ? { ...state, pendingCommands } : state;
}

function parseMapTransferStarted(payload: EventEnvelope['payload'], serverTime: number): MapTransferState {
  return {
    state: 'started',
    portal_id: stringField(payload, 'portal_id') ?? undefined,
    from_public_map_key: stringField(payload, 'from_public_map_key') ?? undefined,
    to_public_map_key: stringField(payload, 'to_public_map_key') ?? undefined,
    started_at: serverTime,
  };
}

function parseMapTransferFailed(payload: EventEnvelope['payload'], serverTime: number): MapTransferState {
  return {
    state: 'failed',
    portal_id: stringField(payload, 'portal_id') ?? undefined,
    from_public_map_key: stringField(payload, 'from_public_map_key') ?? undefined,
    reason: stringField(payload, 'reason') ?? 'Map transfer failed.',
    started_at: serverTime,
  };
}

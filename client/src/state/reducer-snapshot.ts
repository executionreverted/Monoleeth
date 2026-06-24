import {
  adminContentResponseAllowedPayloadKeys,
  JsonObject,
  playerContentCatalogResponseAllowedPayloadKeys,
  rejectForbiddenPayloadKeys,
} from '../protocol/envelope';
import type { ClientState } from './types';
import { isJsonObject, objectField } from './reducer-helpers';
import { hasPlayerContentCatalogPayload, parseContentCatalogSummary } from './reducer-content';
import {
  applyScanPulse,
  applyPlanetDetail,
  applyPlanetStorageSummary,
  applyRouteList,
  applyRouteSettlementSnapshot,
  applyRouteSnapshot,
  parseKnownPlanets,
  parseMinimapSummary,
  parsePlanetDetail,
  parsePlanetStorage,
  parseProductionCollection,
  parseRoute,
  parseRouteList,
  parseRouteSettlement,
  parseScanPulse,
  parseSectorSummary,
  scanModeAfterPulseSummary,
} from './reducer-discovery';
import {
  parseAuctionSummary,
  parseEconomyDashboard,
  parseMarketSummary,
  parsePremiumSummary,
  parseShopCatalogSummary,
} from './reducer-economy';
import {
  parseCargoSummary,
  parseCraftingSummary,
  parseHangarSummary,
  parseInventorySummary,
  parseLoadoutSummary,
  parseProgressionSummary,
  parseRepairQuote,
  parseSessionReady,
  parseShipSummary,
  parseStatSummary,
  parseWalletSummary,
} from './reducer-player-parsers';
import { withoutPendingCoordinateItemUse } from './reducer-pending';
import { applyMapSnapshotPayload } from './reducer-map';
import {
  parseAbuseCoverageSummary,
  parseAdminInspection,
  parseAdminRepairCraftJob,
  parseCommandLogSummary,
  parseMetricsSummary,
  parseQuestBoardSummary,
  parseReleaseGateSummary,
} from './reducer-quests-admin';
import { clearOriginMapLiveState } from './reducer-world';
import { applyAdminContentPayload, hasAdminContentPayload } from './reducer-content-admin';

export function applySnapshotPayload(state: ClientState, payload: JsonObject): ClientState {
  const adminContentPayload = hasAdminContentPayload(payload);
  const playerContentCatalogPayload = hasPlayerContentCatalogPayload(payload);
  rejectForbiddenPayloadKeys(payload, {
    allowedKeys: adminContentPayload
      ? adminContentResponseAllowedPayloadKeys
      : playerContentCatalogPayload
        ? playerContentCatalogResponseAllowedPayloadKeys
        : undefined,
  });

  let next = adminContentPayload
    ? state
    : applyMapSnapshotPayload(state, payload, {
        clearMapScopedState: clearOriginMapLiveState,
      });

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
    const parsedInventory = parseInventorySummary(inventory, next.inventory);
    next = withoutPendingCoordinateItemUse(next, parsedInventory, Array.isArray(inventory.instances));
    next = {
      ...next,
      inventory: parsedInventory,
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
    next = applyRouteList(next, parseRouteList(routes, next.routes));
  }

  const route = objectField(payload, 'route');
  if (route) {
    const parsedRoute = parseRoute(route);
    if (parsedRoute) {
      next = applyRouteSnapshot(next, parsedRoute);
    }
  }

  const settlementPayloads = [
    objectField(payload, 'settlement'),
    ...(Array.isArray(payload.settlements) ? payload.settlements.filter(isJsonObject) : []),
  ].filter((settlement): settlement is Record<string, unknown> => !!settlement);
  for (const settlement of settlementPayloads) {
    const parsedSettlement = parseRouteSettlement(settlement);
    const currentRoute = parsedSettlement
      ? next.routes?.routes.find((candidate) => candidate.route_id === parsedSettlement.route_id)
      : null;
    if (parsedSettlement) {
      next = applyRouteSettlementSnapshot(next, parsedSettlement);
      if (currentRoute) {
        next = applyRouteSnapshot(next, { ...currentRoute, last_settlement: parsedSettlement });
      }
    }
  }

  const market = objectField(payload, 'market');
  if (market) {
    next = {
      ...next,
      market: parseMarketSummary(market, next.market),
    };
  }

  const shop = objectField(payload, 'shop') ?? objectField(payload, 'shop_catalog');
  if (shop) {
    next = {
      ...next,
      shopCatalog: parseShopCatalogSummary(shop, next.shopCatalog),
    };
  }

  const contentCatalog = objectField(payload, 'content_catalog');
  if (contentCatalog) {
    next = {
      ...next,
      contentCatalog: parseContentCatalogSummary(contentCatalog, next.contentCatalog),
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
      questBoard: parseQuestBoardSummary(questBoard, next.questBoard, next.lastServerTime ?? undefined),
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

  next = applyAdminContentPayload(next, payload);

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

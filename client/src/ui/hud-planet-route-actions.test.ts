import { describe, expect, test, vi } from 'vitest';

import { dispatchPlanetRouteButtonAction } from './hud-planet-route-actions';
import type { HUDHandlers } from './hud-types';

describe('planet route HUD action dispatch', () => {
  test('dispatches intel share with planet id and visible entity id only', () => {
    const handlers = testHandlers();
    const control = {
      dataset: { planetId: 'planet-eris' },
      querySelector: vi.fn(() => ({ value: 'entity_pilot_2' })),
    };
    const button = {
      dataset: { action: 'intel-share', planetId: 'planet-eris' },
      closest: vi.fn(() => control),
    } as unknown as HTMLButtonElement;

    const handled = dispatchPlanetRouteButtonAction(button, handlers, () => {});

    expect(handled).toBe(true);
    expect(handlers.onIntelShareToEntity).toHaveBeenCalledWith('planet-eris', 'entity_pilot_2');
    expect(handlers.onIntelShareToEntity).toHaveBeenCalledTimes(1);
  });

  test('dispatches rendered coordinate item create with only planet id', () => {
    const handlers = testHandlers();
    const button = {
      dataset: { action: 'coordinate-item-create', planetId: 'planet-eris' },
    } as unknown as HTMLButtonElement;

    const handled = dispatchPlanetRouteButtonAction(button, handlers, () => {});

    expect(handled).toBe(true);
    expect(handlers.onCoordinateItemCreate).toHaveBeenCalledWith('planet-eris');
    expect(handlers.onCoordinateItemCreate).toHaveBeenCalledTimes(1);
    expect(handlers.onIntelCoordinateItemCreate).not.toHaveBeenCalled();
  });

  test('dispatches storage route create with typed endpoint intent only', () => {
    const handlers = testHandlers();
    const control = routeControl({
      '[data-route-create-destination]': 'storage:route-storage-endpoint',
      '[data-route-create-resource]': 'refined_alloy',
      '[data-route-rate]': '40',
    });
    const button = {
      dataset: { action: 'route-create', sourcePlanetId: 'planet-source' },
      closest: vi.fn(() => control),
    } as unknown as HTMLButtonElement;

    const handled = dispatchPlanetRouteButtonAction(button, handlers, () => {});

    expect(handled).toBe(true);
    expect(handlers.onRouteCreate).toHaveBeenCalledWith({
      sourcePlanetID: 'planet-source',
      destinationPlanetID: undefined,
      destination: { type: 'storage', id: 'route-storage-endpoint' },
      resourceItemID: 'refined_alloy',
      amountPerHour: 40,
    });
  });

  test('dispatches rendered building build with server-safe intent only', () => {
    const handlers = testHandlers();
    const control = routeControl({
      '[data-building-build-type]': 'alloy_foundry',
      '[data-building-build-slot]': 'beta',
    });
    const button = {
      dataset: { action: 'planet-building-build', planetId: 'planet-source' },
      closest: vi.fn(() => control),
    } as unknown as HTMLButtonElement;

    const handled = dispatchPlanetRouteButtonAction(button, handlers, () => {});

    expect(handled).toBe(true);
    expect(handlers.onPlanetBuildingBuild).toHaveBeenCalledWith({
      planetID: 'planet-source',
      buildingType: 'alloy_foundry',
      slot: 'beta',
    });
  });
});

function routeControl(values: Record<string, string>): HTMLElement {
  return {
    querySelector: vi.fn((selector: string) => ({ value: values[selector] ?? '' })),
  } as unknown as HTMLElement;
}

function testHandlers(): HUDHandlers {
  return {
    onConnect: vi.fn(),
    onDisconnect: vi.fn(),
    onLogout: vi.fn(),
    onStop: vi.fn(),
    onSync: vi.fn(),
    onFire: vi.fn(),
    onLoot: vi.fn(),
    onRepairQuote: vi.fn(),
    onRepair: vi.fn(),
    onScan: vi.fn(),
    onStealthToggle: vi.fn(),
    onSelectTarget: vi.fn(),
    onCycleTarget: vi.fn(),
    onPortalEnter: vi.fn(),
    onPlanetDetail: vi.fn(),
    onPlanetNavigate: vi.fn(),
    onPlanetClaim: vi.fn(),
    onIntelShare: vi.fn(),
    onIntelShareToEntity: vi.fn(),
    onCoordinateItemCreate: vi.fn(),
    onCoordinateItemUse: vi.fn(),
    onIntelCoordinateItemCreate: vi.fn(),
    onIntelCoordinateItemUse: vi.fn(),
    onPlanetBuildingBuild: vi.fn(),
    onPlanetBuildingUpgrade: vi.fn(),
    onCraftingStart: vi.fn(),
    onCraftingComplete: vi.fn(),
    onCraftingCancel: vi.fn(),
    onRouteCreate: vi.fn(),
    onRouteUpdate: vi.fn(),
    onRouteEnable: vi.fn(),
    onRouteDisable: vi.fn(),
    onRouteSettle: vi.fn(),
    onHangarActivateShip: vi.fn(),
    onLoadoutEquipModule: vi.fn(),
    onLoadoutUnequipModule: vi.fn(),
    onMarketCreateListing: vi.fn(),
    onShopBuyProduct: vi.fn(),
    onMarketBuy: vi.fn(),
    onMarketCancel: vi.fn(),
    onAuctionBid: vi.fn(),
    onAuctionBuyNow: vi.fn(),
    onAuctionGrants: vi.fn(),
    onPremiumClaim: vi.fn(),
    onPremiumWeeklyXCore: vi.fn(),
    onQuestAccept: vi.fn(),
    onQuestClaim: vi.fn(),
    onQuestReroll: vi.fn(),
    onAdminRefresh: vi.fn(),
    onAdminRepairCraftJob: vi.fn(),
  };
}

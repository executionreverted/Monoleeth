import { describe, expect, test, vi } from 'vitest';

import { dispatchCraftingButtonAction } from './hud-crafting-actions';
import { dispatchInventoryButtonAction } from './hud-inventory-actions';
import type { HUDHandlers } from './hud-types';

describe('inventory and crafting HUD action dispatch', () => {
  test('dispatches coordinate item use with only the item instance id', () => {
    const handlers = testHandlers();
    const button = testButton('intel-coordinate-use', { itemInstanceId: 'coord-scroll-1' });

    const handled = dispatchInventoryButtonAction(button, handlers, () => {});

    expect(handled).toBe(true);
    expect(handlers.onIntelCoordinateItemUse).toHaveBeenCalledWith('coord-scroll-1');
    expect(handlers.onIntelCoordinateItemUse).toHaveBeenCalledTimes(1);
  });

  test('dispatches crafting controls through the existing helper', () => {
    const handlers = testHandlers();

    expect(dispatchCraftingButtonAction(testButton('crafting-start', { recipeId: 'alloy', locationType: 'station' }), handlers)).toBe(true);
    expect(dispatchCraftingButtonAction(testButton('crafting-complete', { jobId: 'job-1' }), handlers)).toBe(true);
    expect(dispatchCraftingButtonAction(testButton('crafting-cancel', { jobId: 'job-2' }), handlers)).toBe(true);

    expect(handlers.onCraftingStart).toHaveBeenCalledWith('alloy', 'station');
    expect(handlers.onCraftingComplete).toHaveBeenCalledWith('job-1');
    expect(handlers.onCraftingCancel).toHaveBeenCalledWith('job-2');
  });
});

function testButton(action: string, dataset: Record<string, string>): HTMLButtonElement {
  return {
    dataset: { action, ...dataset },
  } as unknown as HTMLButtonElement;
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

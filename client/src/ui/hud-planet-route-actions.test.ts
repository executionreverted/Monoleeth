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
});

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
    onIntelShareToEntity: vi.fn(),
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

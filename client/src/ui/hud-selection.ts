import type { InventoryTabID, ModuleFilterID, ShopCategoryID } from './hud-types';

export const hudSelection = {
  selectedQuestKey: null as string | null,
  selectedShopCategory: 'ships' as ShopCategoryID,
  selectedShopKey: null as string | null,
  selectedShopQuantity: 1,
  selectedInventoryTab: 'equipment' as InventoryTabID,
  selectedModuleFilter: 'all' as ModuleFilterID,
  selectedModuleInstanceID: null as string | null,
  selectedHangarShipID: null as string | null,
  selectedPortalID: null as string | null,
  selectedPortalScope: null as string | null,
};

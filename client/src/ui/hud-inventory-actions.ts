import { hudSelection } from './hud-selection';
import type { HUDHandlers } from './hud-types';
import { isInventoryTabID, isModuleFilterID } from './hud-formatters';

export function dispatchInventoryButtonAction(
  button: HTMLButtonElement,
  handlers: HUDHandlers,
  renderCurrentState: () => void,
): boolean {
  switch (button.dataset.action) {
    case 'inventory-tab':
      if (isInventoryTabID(button.dataset.inventoryTab)) {
        hudSelection.selectedInventoryTab = button.dataset.inventoryTab;
        renderCurrentState();
      }
      return true;
    case 'module-filter':
      if (isModuleFilterID(button.dataset.moduleFilter)) {
        hudSelection.selectedModuleFilter = button.dataset.moduleFilter;
        hudSelection.selectedModuleInstanceID = null;
        renderCurrentState();
      }
      return true;
    case 'loadout-equip':
      if (button.dataset.slotId && button.dataset.itemInstanceId) {
        handlers.onLoadoutEquipModule(button.dataset.slotId, button.dataset.itemInstanceId);
      }
      return true;
    case 'loadout-unequip':
      if (button.dataset.slotId) {
        handlers.onLoadoutUnequipModule(button.dataset.slotId);
      }
      return true;
    case 'module-select':
      if (button.dataset.moduleInstanceId) {
        hudSelection.selectedModuleInstanceID = button.dataset.moduleInstanceId;
        renderCurrentState();
      }
      return true;
    case 'coordinate-item-use':
    case 'intel-coordinate-use':
      if (button.dataset.itemInstanceId) {
        handlers.onCoordinateItemUse(button.dataset.itemInstanceId);
      }
      return true;
    default:
      return false;
  }
}

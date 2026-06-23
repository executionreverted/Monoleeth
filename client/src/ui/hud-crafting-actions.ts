import type { HUDHandlers } from './hud-types';

export function dispatchCraftingButtonAction(button: HTMLButtonElement, handlers: HUDHandlers): boolean {
  switch (button.dataset.action) {
    case 'crafting-start':
      if (button.dataset.recipeId) {
        handlers.onCraftingStart(button.dataset.recipeId);
      }
      return true;
    case 'crafting-complete':
      if (button.dataset.jobId) {
        handlers.onCraftingComplete(button.dataset.jobId);
      }
      return true;
    default:
      return false;
  }
}

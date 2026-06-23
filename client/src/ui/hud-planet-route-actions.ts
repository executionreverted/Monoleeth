import type { RouteDestinationInput } from '../protocol/commands';
import { dispatchCraftingButtonAction } from './hud-crafting-actions';
import { hudSelection } from './hud-selection';
import type { HUDHandlers } from './hud-types';

export function dispatchPlanetRouteButtonAction(
  button: HTMLButtonElement,
  handlers: HUDHandlers,
  rerender: () => void,
): boolean {
  switch (button.dataset.action) {
    case 'planet-select':
      if (button.dataset.planetId) {
        handlers.onPlanetDetail(button.dataset.planetId);
      }
      return true;
    case 'planet-navigate':
      if (button.dataset.planetId) {
        handlers.onPlanetNavigate(button.dataset.planetId);
      }
      return true;
    case 'planet-claim':
      if (button.dataset.planetId) {
        handlers.onPlanetClaim(button.dataset.planetId);
      }
      return true;
    case 'intel-share': {
      const control = button.closest<HTMLElement>('[data-intel-share-control]');
      handlers.onIntelShareToEntity(
        button.dataset.planetId ?? control?.dataset.planetId ?? '',
        routeControlValue(control, '[data-intel-share-target]'),
      );
      return true;
    }
    case 'coordinate-item-create':
    case 'intel-coordinate-create':
      if (button.dataset.planetId) {
        handlers.onCoordinateItemCreate(button.dataset.planetId);
      }
      return true;
    case 'planet-building-build': {
      const control = button.closest<HTMLElement>('[data-building-build-control]');
      handlers.onPlanetBuildingBuild({
        planetID: button.dataset.planetId ?? control?.dataset.planetId ?? '',
        buildingType: routeControlValue(control, '[data-building-type]'),
        slot: routeControlValue(control, '[data-building-slot]'),
      });
      return true;
    }
    case 'planet-building-upgrade':
      if (button.dataset.planetId && button.dataset.buildingId) {
        handlers.onPlanetBuildingUpgrade({
          planetID: button.dataset.planetId,
          buildingID: button.dataset.buildingId,
          targetLevel: Number(button.dataset.targetLevel ?? '0'),
        });
      }
      return true;
    case 'crafting-start':
    case 'crafting-complete':
    case 'crafting-cancel':
      return dispatchCraftingButtonAction(button, handlers);
    case 'route-select':
      if (button.dataset.routeId) {
        hudSelection.selectedRouteID = button.dataset.routeId;
        rerender();
      }
      return true;
    case 'route-create': {
      const control = button.closest<HTMLElement>('[data-route-create-control]');
      const destination = routeDestinationFromControlValue(routeControlValue(control, '[data-route-create-destination]'));
      handlers.onRouteCreate({
        sourcePlanetID: button.dataset.sourcePlanetId ?? control?.dataset.routeSourcePlanetId ?? '',
        destinationPlanetID: destination.type === 'planet' ? destination.id : undefined,
        destination,
        resourceItemID: routeControlValue(control, '[data-route-create-resource]'),
        amountPerHour: Number(routeControlValue(control, '[data-route-rate]')),
      });
      return true;
    }
    case 'route-update': {
      const control = button.closest<HTMLElement>('[data-route-update-control]');
      const destination = routeDestinationFromControlValue(routeControlValue(control, '[data-route-update-destination]'));
      handlers.onRouteUpdate({
        routeID: button.dataset.routeId ?? control?.dataset.routeId ?? '',
        destinationPlanetID: destination.type === 'planet' ? destination.id : undefined,
        destination,
        resourceItemID: routeControlValue(control, '[data-route-update-resource]'),
        amountPerHour: Number(routeControlValue(control, '[data-route-rate]')),
      });
      return true;
    }
    case 'route-enable':
      if (button.dataset.routeId) {
        handlers.onRouteEnable(button.dataset.routeId);
      }
      return true;
    case 'route-disable':
      if (button.dataset.routeId) {
        handlers.onRouteDisable(button.dataset.routeId);
      }
      return true;
    case 'route-settle':
      handlers.onRouteSettle(button.dataset.routeId || undefined);
      return true;
    default:
      return false;
  }
}

function routeControlValue(container: HTMLElement | null | undefined, selector: string): string {
  const control = container?.querySelector<HTMLInputElement | HTMLSelectElement>(selector);
  return control?.value ?? '';
}

function routeDestinationFromControlValue(value: string): RouteDestinationInput {
  const [rawType, ...rest] = value.split(':');
  const typedID = rest.join(':');
  if ((rawType === 'storage' || rawType === 'station') && typedID) {
    return { type: rawType, id: typedID };
  }
  if (rawType === 'planet' && typedID) {
    return { type: 'planet', id: typedID };
  }
  return { type: 'planet', id: value };
}

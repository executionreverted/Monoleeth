import { OPERATIONS } from '../protocol/envelope';
import type { ClientState, InventorySummary } from './types';
import { stringField } from './reducer-helpers';

export function withoutPendingCoordinateItemUse(
  state: ClientState,
  inventory: InventorySummary,
  hasAuthoritativeInstances: boolean,
): ClientState {
  if (!hasAuthoritativeInstances) {
    return state;
  }
  const itemInstanceIDs = new Set(inventory.instances.map((item) => item.item_instance_id));
  const pendingCommands: ClientState['pendingCommands'] = {};
  let changed = false;
  for (const [requestID, pending] of Object.entries(state.pendingCommands)) {
    const itemInstanceID = stringField(pending.payload ?? {}, 'item_instance_id');
    if (
      pending.op === OPERATIONS.intelCoordinateItemUse &&
      itemInstanceID &&
      !itemInstanceIDs.has(itemInstanceID)
    ) {
      changed = true;
      continue;
    }
    pendingCommands[requestID] = pending;
  }
  return changed ? { ...state, pendingCommands } : state;
}

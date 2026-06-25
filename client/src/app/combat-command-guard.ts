import type { ClientState } from '../state/types';

export const SHIP_DISABLED_COMBAT_MESSAGE = 'Fire rejected: ship disabled.';

export function combatCommandsDisabled(state: Pick<ClientState, 'ship'>): boolean {
  return state.ship?.disabled === true;
}

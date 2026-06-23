import type { ClientState } from '../state/types';

type RealtimeCommandGateState = Pick<ClientState, 'auth' | 'connectionStatus'>;

export function canSendRealtimeCommand(state: RealtimeCommandGateState): boolean {
  return state.auth.mode === 'demo' || state.connectionStatus === 'connected';
}

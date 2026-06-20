import type { ClientState } from '../state/types';

export function canSendRealtimeCommand(authMode: ClientState['auth']['mode'], status: ClientState['connectionStatus']): boolean {
  return authMode !== 'real' || status === 'connected';
}

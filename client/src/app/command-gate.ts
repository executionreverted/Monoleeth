import type { ClientState } from '../state/types';

export function canSendRealtimeCommand(status: ClientState['connectionStatus']): boolean {
  return status === 'connected';
}

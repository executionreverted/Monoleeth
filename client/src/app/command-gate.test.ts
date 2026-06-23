import { describe, expect, test } from 'vitest';

import { canSendRealtimeCommand } from './command-gate';
import type { ClientState, ConnectionStatus } from '../state/types';

describe('canSendRealtimeCommand', () => {
  test('blocks real-mode gameplay commands until session.ready promotes the socket to connected', () => {
    const blockedStatuses: ConnectionStatus[] = [
      'restoring',
      'logged_out',
      'authenticated_pending_socket',
      'connecting',
      'reconnecting',
      'auth_expired',
      'offline',
      'error',
    ];

    for (const status of blockedStatuses) {
      expect(canSendRealtimeCommand(gateState('real', status)), status).toBe(false);
    }
  });

  test('enables real-mode gameplay commands only after server session readiness', () => {
    expect(canSendRealtimeCommand(gateState('real', 'connected'))).toBe(true);
  });

  test('keeps explicit demo fixture mode isolated from real auth gating', () => {
    expect(canSendRealtimeCommand(gateState('demo', 'offline'))).toBe(true);
    expect(canSendRealtimeCommand(gateState('demo', 'authenticated_pending_socket'))).toBe(true);
  });
});

function gateState(mode: ClientState['auth']['mode'], connectionStatus: ConnectionStatus): Pick<ClientState, 'auth' | 'connectionStatus'> {
  return {
    auth: {
      mode,
      session: null,
      submitting: false,
      error: null,
    },
    connectionStatus,
  };
}

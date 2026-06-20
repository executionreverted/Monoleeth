import { describe, expect, test } from 'vitest';

import { canSendRealtimeCommand } from './command-gate';
import type { ClientState } from '../state/types';

describe('canSendRealtimeCommand', () => {
  test('blocks real-mode commands until the server-owned session.ready bootstrap reaches connected', () => {
    const blockedStatuses: ClientState['connectionStatus'][] = [
      'restoring',
      'logged_out',
      'offline',
      'connecting',
      'authenticated_pending_socket',
      'reconnecting',
      'error',
      'auth_expired',
    ];

    for (const status of blockedStatuses) {
      expect(canSendRealtimeCommand('real', status)).toBe(false);
    }

    expect(canSendRealtimeCommand('real', 'connected')).toBe(true);
  });

  test('keeps demo mode able to exercise local offline intents explicitly', () => {
    expect(canSendRealtimeCommand('demo', 'offline')).toBe(true);
    expect(canSendRealtimeCommand('demo', 'connected')).toBe(true);
  });
});

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
      expect(canSendRealtimeCommand(status)).toBe(false);
    }

    expect(canSendRealtimeCommand('connected')).toBe(true);
  });
});

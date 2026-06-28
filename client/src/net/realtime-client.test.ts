import { describe, expect, test } from 'vitest';

import { statusForCloseEvent } from './realtime-client';

describe('statusForCloseEvent', () => {
  test('maps only terminal auth policy closes to auth_expired', () => {
    expect(statusForCloseEvent(1008, 'session invalid')).toBe('auth_expired');
  });

  test('lets slow-client policy closes reconnect instead of clearing auth', () => {
    expect(statusForCloseEvent(1008, 'client too slow')).toBe('offline');
    expect(statusForCloseEvent(1008, '')).toBe('offline');
  });
});

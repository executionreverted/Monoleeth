import { describe, expect, test } from 'vitest';

import { isDemoModeEnabled } from './client-app-core';

describe('isDemoModeEnabled', () => {
  test('requires both an explicit demo flag and a development build', () => {
    expect(isDemoModeEnabled('?demo=1', true)).toBe(true);
    expect(isDemoModeEnabled('?demo=1', false)).toBe(false);
    expect(isDemoModeEnabled('?demo=0', true)).toBe(false);
    expect(isDemoModeEnabled('', true)).toBe(false);
  });
});

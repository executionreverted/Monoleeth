import { describe, expect, test } from 'vitest';

import { hudShellHTML } from './hud-render-shell';

describe('hudShellHTML', () => {
  test('keeps the topbar as a compact status-only row', () => {
    const html = hudShellHTML();

    expect(html).toContain('class="hud__topbar"');
    expect(html).toContain('class="top-status"');
    expect(html).not.toContain('class="toolbar"');
    expect(html).not.toContain('data-action="stop"');
    expect(html).not.toContain('data-action="sync"');
    expect(html).not.toContain('data-panel-id="chat"');
    expect(html).not.toContain('data-panel-id="social"');
    expect(html).not.toContain('data-action="logout"');
  });
});

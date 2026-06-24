import { beforeEach, describe, expect, test } from 'vitest';

import { createInitialState } from '../state/reducer';
import type { AdminContentState, ClientState } from '../state/types';
import { adminContentBlock } from './hud-render-admin-content';
import { opsPanel } from './hud-render-panels';
import { hudSelection } from './hud-selection';

describe('admin content HUD render', () => {
  beforeEach(() => {
    hudSelection.selectedAdminContentType = 'module';
    hudSelection.selectedAdminContentID = null;
  });

  test('hides CMS controls from non-admin sessions', () => {
    const state: ClientState = {
      ...createInitialState(),
      auth: {
        mode: 'real',
        session: {
          authenticated: true,
          account: { email: 'pilot@example.com', admin: false },
          player: { callsign: 'Pilot' },
          server_time: 1000,
        },
        submitting: false,
        error: null,
      },
    };

    expect(adminContentBlock(state)).toBe('');
    expect(opsPanel(state)).toContain('Admin session required.');
    expect(opsPanel(state)).not.toContain('admin-content-publish');
  });

  test('renders admin CMS actions, versions, module rows, and selected stats', () => {
    const state = adminState();

    const html = adminContentBlock(state);

    expect(html).toContain('data-action="admin-content-refresh"');
    expect(html).toContain('data-action="admin-content-validate"');
    expect(html).toContain('data-action="admin-content-publish"');
    expect(html).toContain('data-action="admin-content-audit"');
    expect(html).toContain('data-action="admin-content-rollback"');
    expect(html).toContain('content_balance_v2');
    expect(html).toContain('Laser Alpha T1');
    expect(html).toContain('laser_alpha_t1');
    expect(html).toContain('Weapon');
    expect(html).toContain('8');
    expect(html).toContain('Cooldown');
    expect(html).toContain('1200ms');
    expect(html).toContain('Energy');
    expect(html).toContain('4');
  });

  test('selects module draft rows without rendering hidden loot internals', () => {
    hudSelection.selectedAdminContentID = 'shield_alpha_t1';

    const html = adminContentBlock(adminState());

    expect(html).toContain('Shield Alpha T1');
    expect(html).toContain('data-selected="true"');
    expect(html).not.toContain('loot_table');
    expect(html).not.toContain('spawn_candidates');
  });

  test('disables publish when validation has issues', () => {
    const state = adminState({
      validation: {
        valid: false,
        version: 'draft_invalid',
        checked_at: 1600,
        issues: [{ path: 'modules[laser_alpha_t1].stat_modifiers[0].value', code: 'negative_damage', message: 'damage must be positive' }],
      },
    });

    const html = adminContentBlock(state);

    expect(html).toContain('data-action="admin-content-publish" disabled');
    expect(html).toContain('negative_damage');
    expect(html).toContain('damage must be positive');
  });
});

function adminState(contentOverride: Partial<AdminContentState> = {}): ClientState {
  return {
    ...createInitialState(),
    auth: {
      mode: 'real',
      session: {
        authenticated: true,
        account: { email: 'admin@example.com', admin: true },
        player: { callsign: 'Admin' },
        server_time: 1000,
      },
      submitting: false,
      error: null,
    },
    adminContent: {
      versions: {
        versions: [
          {
            id: '22222222-2222-5222-8222-222222222222',
            version: 'content_balance_v2',
            status: 'published',
            current: true,
            created_at: 1500,
            published_at: 1500,
          },
          {
            id: '11111111-1111-5111-8111-111111111111',
            version: 'content_balance_v1',
            status: 'archived',
            current: false,
            created_at: 1000,
            published_at: 1000,
          },
        ],
        total: 2,
        limit: 50,
        offset: 0,
        generated_at: 1550,
      },
      rowsByType: {
        module: {
          content_type: 'module',
          rows: [
            {
              content_type: 'module',
              content_id: 'laser_alpha_t1',
              enabled: true,
              display_json: { name: 'Laser Alpha T1' },
              data_json: {
                rarity: 'common',
                required_rank: 1,
                tier: 1,
                slot_type: 'weapon',
                stat_modifiers: [
                  { stat: 'weapon_damage', value: 8 },
                  { stat: 'shield_damage', value: 5 },
                ],
                cooldowns: [{ duration_ms: 1200 }],
                energy: { activation_cost: 4 },
              },
            },
            {
              content_type: 'module',
              content_id: 'shield_alpha_t1',
              enabled: true,
              display_json: { name: 'Shield Alpha T1' },
              data_json: {
                rarity: 'common',
                required_rank: 1,
                tier: 1,
                slot_type: 'shield',
                stat_modifiers: [{ stat: 'shield_capacity', value: 40 }],
              },
            },
          ],
          total: 2,
          limit: 50,
          offset: 0,
          generated_at: 1551,
        },
      },
      selectedRow: null,
      validation: {
        valid: true,
        version: 'content_balance_v2',
        checked_at: 1552,
        issues: [],
      },
      publish: null,
      rollback: null,
      auditLog: {
        entries: [
          {
            id: '33333333-3333-5333-8333-333333333333',
            content_type: 'module',
            content_id: 'laser_alpha_t1',
            field_path: '$.stat_modifiers',
            created_at: 1553,
          },
        ],
        total: 1,
        limit: 12,
        offset: 0,
        generated_at: 1554,
      },
      ...contentOverride,
    },
  };
}

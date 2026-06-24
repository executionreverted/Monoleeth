import { beforeEach, describe, expect, test, vi } from 'vitest';

import { createInitialState } from '../state/reducer';
import type { AdminContentState, ClientState } from '../state/types';
import { HUD } from './hud';
import {
  adminContentBlock,
  adminModuleEditFields,
  adminModuleEditModal,
  buildAdminModuleDraftUpdate,
} from './hud-render-admin-content';
import { opsPanel } from './hud-render-panels';
import { hudSelection } from './hud-selection';
import type { HUDHandlers } from './hud-types';

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

  test('keeps stale admin content hidden from non-admin render paths', () => {
    const state = adminState();
    state.auth = {
      ...state.auth,
      session: {
        ...state.auth.session!,
        account: { email: 'pilot@example.com', admin: false },
      },
    };
    const staleRow = state.adminContent!.rowsByType.module.rows[0];
    staleRow.data_json = {
      ...staleRow.data_json,
      loot_table: 'hidden_drop_rate_sentinel',
      spawn_candidates: ['hidden_spawn_window_sentinel'],
      admin_notes: 'hidden_admin_note_sentinel',
    };

    const html = opsPanel(state);

    expect(adminContentBlock(state)).toBe('');
    expect(html).toContain('Admin session required.');
    expect(html).not.toContain('admin-content-refresh');
    expect(html).not.toContain('hidden_drop_rate_sentinel');
    expect(html).not.toContain('hidden_spawn_window_sentinel');
    expect(html).not.toContain('hidden_admin_note_sentinel');
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
    expect(html).toContain('data-action="admin-content-module-edit"');
  });

  test('selects module draft rows without rendering hidden loot internals', () => {
    hudSelection.selectedAdminContentID = 'shield_alpha_t1';

    const html = adminContentBlock(adminState());

    expect(html).toContain('Shield Alpha T1');
    expect(html).toContain('data-selected="true"');
    expect(html).not.toContain('loot_table');
    expect(html).not.toContain('spawn_candidates');
    expect(html).not.toContain('rare_drop_sentinel');
    expect(html).not.toContain('spawn_sentinel');
  });

  test('renders compact module edit modal with only allowlisted editable fields', () => {
    const html = adminModuleEditModal(adminState(), 'laser_alpha_t1');

    expect(html).toContain('data-admin-content-module-form="true"');
    expect(html).toContain('data-admin-module-field="weapon_damage"');
    expect(html).toContain('data-admin-module-field="shield_damage"');
    expect(html).toContain('data-admin-module-field="range"');
    expect(html).toContain('data-admin-module-field="cooldown"');
    expect(html).toContain('data-admin-module-field="energy"');
    expect(html).toContain('data-admin-module-field="rank"');
    expect(html).toContain('data-admin-module-field="rarity"');
    expect(html).not.toContain('loot_table');
    expect(html).not.toContain('spawn_candidates');
    expect(html).not.toContain('rare_drop_sentinel');
    expect(html).not.toContain('spawn_sentinel');
  });

  test('builds update draft payload by preserving row identity and patching module stats', () => {
    const row = adminState().adminContent?.rowsByType.module.rows.find((item) => item.content_id === 'laser_alpha_t1');
    expect(row).toBeTruthy();

    const update = buildAdminModuleDraftUpdate(row!, {
      weapon_damage: 11,
      shield_damage: 7,
      range: 520,
      cooldown: 900,
      energy: 6,
      rank: 2,
      rarity: 'rare',
    });
    const data = update.dataJSON;
    const modifiers = data.stat_modifiers as Array<{ stat: string; value: number }>;

    expect(update).toMatchObject({
      contentType: 'module',
      contentID: 'laser_alpha_t1',
      enabled: true,
      displayJSON: { name: 'Laser Alpha T1' },
    });
    expect(modifiers.find((modifier) => modifier.stat === 'weapon_damage')?.value).toBe(11);
    expect(modifiers.find((modifier) => modifier.stat === 'shield_damage')?.value).toBe(7);
    expect(modifiers.find((modifier) => modifier.stat === 'range')?.value).toBe(520);
    expect((data.cooldowns as Array<{ duration_ms: number }>)[0].duration_ms).toBe(900);
    expect((data.energy as { activation_cost: number }).activation_cost).toBe(6);
    expect(data.required_rank).toBe(2);
    expect(data.rarity).toBe('rare');
    expect(data.loot_table).toBe('rare_drop_sentinel');
    expect(data.spawn_candidates).toEqual(['spawn_sentinel']);
    expect(row!.data_json.required_rank).toBe(1);
  });

  test('does not add module edit fields that are absent from draft data', () => {
    const row = adminState().adminContent?.rowsByType.module.rows.find((item) => item.content_id === 'shield_alpha_t1');
    expect(row).toBeTruthy();

    const fields = adminModuleEditFields(row!);
    const update = buildAdminModuleDraftUpdate(row!, {
      weapon_damage: 99,
      cooldown: 500,
      energy: 2,
      rank: 3,
    });
    const data = update.dataJSON;

    expect(fields.map((field) => field.id)).toEqual(['rank', 'rarity']);
    expect(data.required_rank).toBe(3);
    expect(data.stat_modifiers).toEqual([{ stat: 'shield_capacity', value: 40 }]);
    expect(data.cooldowns).toBeUndefined();
    expect(data.energy).toBeUndefined();
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

  test('dispatches admin CMS refresh, validate, publish, audit, rollback, and select actions', () => {
    const handlers = adminContentHandlers();

    dispatchHUDButton('admin-content-refresh', handlers);
    dispatchHUDButton('admin-content-validate', handlers);
    dispatchHUDButton('admin-content-publish', handlers);
    dispatchHUDButton('admin-content-audit', handlers);

    const rollback = dispatchHUDButton('admin-content-rollback', handlers, {
      versionId: '11111111-1111-5111-8111-111111111111',
    });
    const select = dispatchHUDButton('admin-content-select', handlers, {
      contentType: 'module',
      contentId: 'shield_alpha_t1',
    });

    expect(handlers.onAdminContentRefresh).toHaveBeenCalledTimes(1);
    expect(handlers.onAdminContentValidate).toHaveBeenCalledTimes(1);
    expect(handlers.onAdminContentPublish).toHaveBeenCalledTimes(1);
    expect(handlers.onAdminContentAudit).toHaveBeenCalledTimes(1);
    expect(handlers.onAdminContentRollback).toHaveBeenCalledWith('11111111-1111-5111-8111-111111111111');
    expect(handlers.onAdminContentRollback).toHaveBeenCalledTimes(1);
    expect(hudSelection.selectedAdminContentType).toBe('module');
    expect(hudSelection.selectedAdminContentID).toBe('shield_alpha_t1');
    expect(rollback.rerenderCurrent).not.toHaveBeenCalled();
    expect(select.rerenderCurrent).toHaveBeenCalledTimes(1);
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
                  { stat: 'range', value: 420 },
                ],
                cooldowns: [{ duration_ms: 1200 }],
                energy: { activation_cost: 4 },
                loot_table: 'rare_drop_sentinel',
                spawn_candidates: ['spawn_sentinel'],
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
                loot_table: 'rare_drop_sentinel',
                spawn_candidates: ['spawn_sentinel'],
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

type AdminContentHandlerSpies = ReturnType<typeof adminContentHandlers>;

type HUDDispatchHarness = {
  handlers: HUDHandlers;
  currentState: ClientState | null;
  dispatchAction(action: string | undefined): boolean;
  rerenderCurrent(): void;
};

type HUDDispatchPrototype = {
  dispatchAction(this: HUDDispatchHarness, action: string | undefined): boolean;
  dispatchButtonAction(this: HUDDispatchHarness, button: HTMLButtonElement): void;
};

function dispatchHUDButton(
  action: string,
  handlers: AdminContentHandlerSpies,
  dataset: Record<string, string> = {},
): { rerenderCurrent: ReturnType<typeof vi.fn> } {
  const hudDispatch = HUD.prototype as unknown as HUDDispatchPrototype;
  const rerenderCurrent = vi.fn();
  const harness: HUDDispatchHarness = {
    handlers: handlers as unknown as HUDHandlers,
    currentState: null,
    dispatchAction: hudDispatch.dispatchAction,
    rerenderCurrent,
  };

  hudDispatch.dispatchButtonAction.call(harness, testButton(action, dataset));

  return { rerenderCurrent };
}

function testButton(action: string, dataset: Record<string, string>): HTMLButtonElement {
  return {
    dataset: { action, ...dataset },
  } as unknown as HTMLButtonElement;
}

function adminContentHandlers() {
  return {
    onAdminContentRefresh: vi.fn(),
    onAdminContentValidate: vi.fn(),
    onAdminContentPublish: vi.fn(),
    onAdminContentRollback: vi.fn(),
    onAdminContentAudit: vi.fn(),
    onAdminContentUpdateDraft: vi.fn(),
  };
}

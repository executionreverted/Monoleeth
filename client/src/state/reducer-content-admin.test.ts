import { describe, expect, test } from 'vitest';

import { OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';

describe('admin content reducer', () => {
  test('stores admin versions and module draft rows from real admin responses', () => {
    const state = createInitialState();
    const withVersions = reduceClientState(state, {
      type: 'responseReceived',
      envelope: {
        request_id: 'request-content-versions',
        ok: true,
        payload: {
          content_versions: {
            versions: [
              {
                id: '22222222-2222-5222-8222-222222222222',
                version: 'content_balance_v2',
                status: 'published',
                current: true,
                created_at: 1_000,
                published_at: 1_000,
              },
            ],
            total: 1,
            limit: 50,
            offset: 0,
            generated_at: 1_001,
          },
        },
        server_time: 1_001,
        v: 1,
      },
    });

    const withRows = reduceClientState(withVersions, {
      type: 'responseReceived',
      envelope: {
        request_id: 'request-content-list',
        ok: true,
        payload: {
          content: {
            content_type: 'module',
            rows: [
              {
                content_type: 'module',
                content_id: 'laser_alpha_t1',
                enabled: true,
                display_json: { name: 'LC1' },
                data_json: {
                  damage: 9,
                  cooldown: { duration_ms: 1200 },
                  map_id: 'admin-visible-only',
                },
              },
            ],
            total: 1,
            limit: 50,
            offset: 0,
            generated_at: 1_002,
          },
        },
        server_time: 1_002,
        v: 1,
      },
    });

    expect(withRows.adminContent?.versions?.versions[0].version).toBe('content_balance_v2');
    expect(withRows.adminContent?.rowsByType.module.rows[0].content_id).toBe('laser_alpha_t1');
    expect(withRows.adminContent?.rowsByType.module.rows[0].data_json).toMatchObject({
      damage: 9,
      map_id: 'admin-visible-only',
    });
  });

  test('stores validation, publish, rollback, and audit payloads', () => {
    const state = {
      ...createInitialState(),
      pendingCommands: {
        publish: { requestID: 'publish', op: OPERATIONS.adminContentPublish, queuedAt: 1, payload: {} },
      },
    };

    const next = reduceClientState(state, {
      type: 'responseReceived',
      envelope: {
        request_id: 'publish',
        ok: true,
        payload: {
          content_publish: {
            published: true,
            idempotent: false,
            row_count: 12,
            version: {
              id: '33333333-3333-5333-8333-333333333333',
              version: 'content_balance_v3',
              status: 'published',
              current: true,
              created_at: 2_000,
              published_at: 2_000,
            },
            validation: {
              valid: true,
              version: 'content_balance_v3',
              checked_at: 1_999,
              issues: [],
            },
          },
          content_audit_log: {
            entries: [
              {
                id: '44444444-4444-5444-8444-444444444444',
                content_type: 'module',
                content_id: 'laser_alpha_t1',
                field_path: '$',
                new_value_json: { content_id: 'laser_alpha_t1', data_json: { damage: 9 } },
                actor_ref: 'account-admin',
                created_at: 2_000,
              },
            ],
            total: 1,
            limit: 50,
            offset: 0,
            generated_at: 2_001,
          },
        },
        server_time: 2_001,
        v: 1,
      },
    });

    expect(next.adminContent?.publish?.published).toBe(true);
    expect(next.adminContent?.validation?.valid).toBe(true);
    expect(next.adminContent?.auditLog?.entries[0].actor_ref).toBe('account-admin');
    expect(next.pendingCommands.publish).toBeUndefined();
  });

  test('rejects secret-bearing admin content reducer payloads', () => {
    expect(() =>
      reduceClientState(createInitialState(), {
        type: 'responseReceived',
        envelope: {
          request_id: 'request-content-secret',
          ok: true,
          payload: {
            content: {
              content_type: 'module',
              rows: [{ content_id: 'laser_alpha_t1', enabled: true, display_json: {}, data_json: { token: 'nope' } }],
            },
          },
          server_time: 1,
          v: 1,
        },
      }),
    ).toThrow(/Forbidden server payload rejected/);
  });
});

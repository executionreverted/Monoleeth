import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';

describe('crafting reducer reconciliation', () => {
  test('crafting event recovers lost mutation responses and clears matching pending commands', () => {
    const state = createInitialState();
    state.pendingCommands = {
      'craft-start-1': {
        requestID: 'craft-start-1',
        op: OPERATIONS.craftingStart,
        queuedAt: 1,
        payload: { recipe_id: 'refined_alloy_batch' },
      },
      'craft-complete-1': {
        requestID: 'craft-complete-1',
        op: OPERATIONS.craftingComplete,
        queuedAt: 1,
        payload: { job_id: 'craft-job-completed' },
      },
      'craft-cancel-1': {
        requestID: 'craft-cancel-1',
        op: OPERATIONS.craftingCancel,
        queuedAt: 1,
        payload: { job_id: 'craft-job-cancelled' },
      },
      'craft-start-other': {
        requestID: 'craft-start-other',
        op: OPERATIONS.craftingStart,
        queuedAt: 1,
        payload: { recipe_id: 'ship_frame' },
      },
    };

    const next = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.craftingRecipes,
        {
          active_jobs: [
            {
              job_id: 'craft-job-started',
              recipe_id: 'refined_alloy_batch',
              state: 'running',
              started_at: 1000,
              completes_at: 31000,
            },
          ],
        },
        2,
      ),
    });

    expect(next.pendingCommands['craft-start-1']).toBeUndefined();
    expect(next.pendingCommands['craft-complete-1']).toBeUndefined();
    expect(next.pendingCommands['craft-cancel-1']).toBeUndefined();
    expect(next.pendingCommands['craft-start-other']).toMatchObject({ op: OPERATIONS.craftingStart });
    expect(next.crafting?.active_jobs).toEqual([
      {
        job_id: 'craft-job-started',
        recipe_id: 'refined_alloy_batch',
        state: 'running',
        started_at: 1000,
        completes_at: 31000,
      },
    ]);
  });
});

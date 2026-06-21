import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event, expectServerOwnedGameplayCleared, stateWithServerOwnedGameplay } from './reducer-fixtures.test-support';
import { isWithinMinimapProjectionWindow, worldMapMemoryMarkers } from './world-memory';

describe('reduceClientState', () => {
  test('phase 09 quest, admin, and observability payloads reconcile server-owned state', () => {
    const questBoard = {
      offers: [
        {
          offer_id: 'offer-1',
          quest_type: 'kill',
          title: 'Training Sweep',
          description: 'Destroy hostile targets confirmed by the server.',
          objectives: [{ id: 'kill', kind: 'kill', target: 'pirate_drone', current: 0, required: 2, completed: false }],
          rewards: [{ kind: 'currency', currency_type: 'credits', amount: 100 }],
          expires_at: 5000,
          can_accept: true,
        },
      ],
      active: [],
      counts: { offers: 1, active: 0, completed: 0, claimable: 0, claimed: 0 },
      reroll_cost: { currency_type: 'credits', amount: 25 },
      can_reroll: true,
      reset_at: 5000,
      generated_at: 1000,
      revision: 1000,
    };
    const withBoard = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'quest-board',
        ok: true,
        payload: { quest_board: questBoard },
        server_time: 1001,
        v: 1,
      },
    });

    const acceptedQuest = {
      quest_id: 'quest-1',
      accepted_offer_id: 'offer-1',
      quest_type: 'kill',
      title: 'Training Sweep',
      description: 'Destroy hostile targets confirmed by the server.',
      state: 'accepted',
      objectives: [{ id: 'kill', kind: 'kill', target: 'pirate_drone', current: 1, required: 2, completed: false }],
      rewards: [{ kind: 'currency', currency_type: 'credits', amount: 100 }],
      accepted_at: 1002,
      can_claim: false,
    };
    const withAccepted = reduceClientState(withBoard, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.questAccepted, acceptedQuest, 2),
    });
    const withProgress = reduceClientState(withAccepted, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.questProgressed,
        {
          ...acceptedQuest,
          state: 'completed',
          objectives: [{ id: 'kill', kind: 'kill', target: 'pirate_drone', current: 2, required: 2, completed: true }],
          completed_at: 1100,
          can_claim: true,
        },
        3,
      ),
    });

    const afterClaim = reduceClientState(withProgress, {
      type: 'responseReceived',
      envelope: {
        request_id: 'quest-claim',
        ok: true,
        payload: {
          quest_board: {
            ...questBoard,
            active: [
              {
                ...acceptedQuest,
                state: 'claimed',
                completed_at: 1100,
                claimed_at: 1200,
                can_claim: false,
              },
            ],
            counts: { offers: 1, active: 0, completed: 0, claimable: 0, claimed: 1 },
            generated_at: 1200,
            revision: 1200,
          },
          wallet: { credits: 600, premium_paid: 0, premium_earned: 0 },
          inventory: {
            stackable: [{ item_id: 'iron_ore', quantity: 4, location: 'account_inventory' }],
            instances: [],
            counts: { cargo_stacks: 0, storage_stacks: 1, equipped_instances: 0 },
          },
          progression: { main_level: 1, main_xp: 25, rank: 1, combat_xp: 0 },
        },
        server_time: 1004,
        v: 1,
      },
    });

    const withAdmin = reduceClientState(afterClaim, {
      type: 'responseReceived',
      envelope: {
        request_id: 'ops',
        ok: true,
        payload: {
          admin: {
            target: 'self',
            inventory: { stackable_items: 1, instance_items: 0, item_ledger: [] },
            wallet: { balances: [{ currency_type: 'credits', balance: 600 }], ledger: [] },
            generated_at: 1300,
          },
          admin_repair: { accepted: false, job_id: 'craft-job-1', status: 'unavailable' },
          command_log: {
            entries: [{ request_id: 'quest-board', operation: 'quest.board', status: 'success', duration_ms: 2, timestamp: 1300 }],
            total: 1,
            generated_at: 1300,
          },
          metrics: { snapshot: { counters: [{ name: 'commands_per_sec', value: 1 }], gauges: [], durations: [] }, generated_at: 1300 },
          release_gate: {
            report: { covered: true, passed: true },
            coverage: [{ module: '10-quest-board-generation', passed: true, evidence: 3 }],
            evidence: 3,
            generated_at: 1300,
          },
          abuse_coverage: { report: { passed: true }, coverage: [{ case: 'negative_amounts', evidence: [] }], generated_at: 1300 },
        },
        server_time: 1005,
        v: 1,
      },
    });

    expect(withBoard.questBoard?.offers[0].offer_id).toBe('offer-1');
    expect(withAccepted.questBoard?.counts.active).toBe(1);
    expect(withAccepted.questBoard?.offers).toHaveLength(0);
    expect(withAccepted.questBoard?.counts.offers).toBe(0);
    expect(withProgress.questBoard?.counts.claimable).toBe(1);
    expect(afterClaim.questBoard?.counts.claimed).toBe(1);
    expect(afterClaim.wallet?.credits).toBe(600);
    expect(afterClaim.inventory?.stackable[0].item_id).toBe('iron_ore');
    expect(afterClaim.progression?.main_xp).toBe(25);
    expect(withAdmin.adminInspection?.wallet.balances[0].balance).toBe(600);
    expect(withAdmin.adminRepair?.status).toBe('unavailable');
    expect(withAdmin.commandLogSummary?.entries[0].operation).toBe('quest.board');
    expect(withAdmin.metrics?.snapshot.counters[0].name).toBe('commands_per_sec');
    expect(withAdmin.releaseGate?.report.passed).toBe(true);
    expect(withAdmin.abuseCoverage?.report.passed).toBe(true);
  });

  test('phase 09 quest board rejects stale revisions and fails closed on expired offers', () => {
    const freshBoard = {
      offers: [
        {
          offer_id: 'offer-fresh',
          quest_type: 'collect',
          title: 'Cargo Recovery',
          description: 'Recover cargo marked by the server.',
          objectives: [
            {
              id: 'collect',
              kind: 'collect',
              target: 'iron_ore',
              display_name: 'Iron Ore',
              catalog_ref: 'item:iron_ore',
              art_key: 'item.iron_ore',
              current: 0,
              required: 2,
              completed: false,
            },
          ],
          rewards: [
            {
              kind: 'item',
              item_id: 'scanner_circuit',
              display_name: 'Scanner Circuit',
              catalog_ref: 'item:scanner_circuit',
              art_key: 'item.scanner_circuit',
              amount: 1,
            },
          ],
          expires_at: 3000,
          can_accept: true,
        },
      ],
      active: [],
      counts: { offers: 1, active: 0, completed: 0, claimable: 0, claimed: 0 },
      reroll_cost: { currency_type: 'credits', amount: 25 },
      can_reroll: true,
      reset_at: 3000,
      generated_at: 2000,
      revision: 2000,
    };
    const withFresh = reduceClientState(createInitialState(), {
      type: 'responseReceived',
      envelope: {
        request_id: 'quest-board-fresh',
        ok: true,
        payload: { quest_board: freshBoard },
        server_time: 2001,
        v: 1,
      },
    });
    const stale = reduceClientState(withFresh, {
      type: 'responseReceived',
      envelope: {
        request_id: 'quest-board-stale',
        ok: true,
        payload: {
          quest_board: {
            ...freshBoard,
            offers: [],
            counts: { offers: 0, active: 0, completed: 0, claimable: 0, claimed: 0 },
            generated_at: 1000,
            revision: 1000,
          },
        },
        server_time: 2002,
        v: 1,
      },
    });
    const expired = reduceClientState(withFresh, {
      type: 'responseReceived',
      envelope: {
        request_id: 'quest-board-expired',
        ok: true,
        payload: {
          quest_board: {
            ...freshBoard,
            offers: [{ ...freshBoard.offers[0], offer_id: 'offer-expired', expires_at: 2500, can_accept: true }],
            reset_at: 2500,
            generated_at: 2000,
            revision: 3000,
          },
        },
        server_time: 3001,
        v: 1,
      },
    });

    expect(withFresh.questBoard?.offers[0].objectives[0].display_name).toBe('Iron Ore');
    expect(withFresh.questBoard?.offers[0].rewards[0].display_name).toBe('Scanner Circuit');
    expect(stale.questBoard?.offers).toHaveLength(1);
    expect(stale.questBoard?.generated_at).toBe(2000);
    expect(stale.questBoard?.revision).toBe(2000);
    expect(expired.questBoard?.offers[0].can_accept).toBe(false);
    expect(expired.questBoard?.offers[0].locked_reason).toBe('Offer expired.');
  });
});

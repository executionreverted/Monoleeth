import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';

describe('social reducer behavior', () => {
  test('party and clan events accept server-owned player ids', () => {
    const state = createInitialState();
    const withParty = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.partyUpdated,
        {
          party_id: 'party-1',
          members: [{ player_id: 'player-1', joined_at: '2026-06-27T10:00:00Z', is_leader: true }],
          created_at: '2026-06-27T10:00:00Z',
        },
        1,
      ),
    });

    const withClan = reduceClientState(withParty, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.clanUpdated,
        {
          clan: {
            clan_id: 'clan-1',
            name: 'Alpha Fleet',
            tag: 'ALF',
            owner_id: 'player-1',
            created_at: '2026-06-27T10:00:00Z',
          },
          membership: {
            clan_id: 'clan-1',
            player_id: 'player-1',
            rank: 'owner',
            joined_at: '2026-06-27T10:00:00Z',
          },
          members: [{ clan_id: 'clan-1', player_id: 'player-1', rank: 'owner', joined_at: '2026-06-27T10:00:00Z' }],
        },
        2,
      ),
    });

    expect(withClan.social.party?.members[0]?.playerID).toBe('player-1');
    expect(withClan.social.clan?.tag).toBe('ALF');
    expect(withClan.social.clanMembers).toHaveLength(1);
  });
});

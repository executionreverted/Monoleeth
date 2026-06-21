import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event } from './reducer-fixtures.test-support';

describe('reduceClientState world AOI and movement', () => {
  test('AOI diff events reconcile minimap live contacts', () => {
    const state = {
      ...createInitialState(),
      minimap: { radar_range: 1000, projection_window_size: 2000, live_contacts: [], remembered: [] },
    };
    const entered = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
        display: { label: 'Training Drone', disposition: 'hostile' },
        status_flags: ['hostile', 'hidden', 'scan_revealed', 'stealthed'],
        projection_source: 'worker_projection',
      }),
    });

    expect(entered.visibleEntities['npc-1']).toMatchObject({
      entity_id: 'npc-1',
      position: { x: 10, y: 20 },
    });
    expect(entered.minimap?.live_contacts).toEqual([
      {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 10, y: 20 },
        disposition: 'hostile',
        status_flags: ['hostile', 'scan_revealed'],
        projection_source: 'worker_projection',
      },
    ]);
    expect(entered.visibleEntities['npc-1'].status_flags).toEqual(['hostile', 'scan_revealed']);

    const selfState = reduceClientState(state, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'player-local',
        entity_type: 'player',
        position: { x: 0, y: 0 },
        display: { label: 'Pilot', disposition: 'self' },
        status_flags: ['self', 'hidden', 'stealthed'],
      }),
    });
    expect(selfState.visibleEntities['player-local'].status_flags).toEqual(['self', 'stealthed']);

    const updated = reduceClientState(entered, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityUpdated, {
        entity_id: 'npc-1',
        entity_type: 'npc',
        position: { x: 15, y: 25 },
        display: { label: 'Training Drone', disposition: 'hostile' },
      }, 2),
    });
    expect(updated.minimap?.live_contacts[0]).toMatchObject({
      entity_id: 'npc-1',
      position: { x: 15, y: 25 },
    });

    const left = reduceClientState(updated, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityLeft, { entity_id: 'npc-1' }, 3),
    });

    expect(left.visibleEntities['npc-1']).toBeUndefined();
    expect(left.minimap?.live_contacts).toEqual([]);
  });

  test('server correction updates authoritative entity position and clears local target', () => {
    const state = reduceClientState({
      ...createInitialState(),
      minimap: { radar_range: 1000, projection_window_size: 2000, live_contacts: [], remembered: [] },
    }, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'player-1',
        entity_type: 'player',
        position: { x: 0, y: 0 },
      }),
    });

    const corrected = reduceClientState(
      {
        ...state,
        movementTarget: { x: 100, y: 100 },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.positionCorrected, {
          entity_id: 'player-1',
          position: { x: 12, y: 16 },
        }, 2),
      },
    );

    expect(corrected.visibleEntities['player-1'].position).toEqual({ x: 12, y: 16 });
    expect(corrected.minimap?.live_contacts[0].position).toEqual({ x: 12, y: 16 });
    expect(corrected.movementTarget).toBeNull();
    expect(corrected.lastCorrection).toEqual({ entityID: 'player-1', position: { x: 12, y: 16 } });
  });

  test('server correction preserves authoritative movement route target', () => {
    const state = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityEntered, {
        entity_id: 'player-1',
        entity_type: 'player',
        position: { x: 0, y: 0 },
        status_flags: ['self'],
      }),
    });

    const corrected = reduceClientState(
      {
        ...state,
        movementTarget: { x: 100, y: 100 },
      },
      {
        type: 'eventReceived',
        envelope: event(CLIENT_EVENTS.positionCorrected, {
          entity_id: 'player-1',
          position: { x: 12, y: 16 },
          movement: {
            moving: true,
            origin: { x: 9, y: 12 },
            target: { x: 80, y: 120 },
            speed: 180,
            started_at_ms: 1000,
            arrive_at_ms: 1600,
          },
        }, 2),
      },
    );

    expect(corrected.visibleEntities['player-1'].movement).toMatchObject({
      origin: { x: 9, y: 12 },
      target: { x: 80, y: 120 },
      speed: 180,
    });
    expect(corrected.movementTarget).toEqual({ x: 80, y: 120 });
  });

  test('ignores duplicate or stale realtime events by sequence', () => {
    const entered = reduceClientState(createInitialState(), {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.entityEntered,
        {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 10, y: 20 },
          display: { label: 'Training Drone', disposition: 'hostile' },
        },
        5,
      ),
    });

    const staleUpdate = reduceClientState(entered, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.entityUpdated,
        {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 999, y: 999 },
          display: { label: 'Stale Drone', disposition: 'hostile' },
        },
        4,
      ),
    });
    const duplicateLeave = reduceClientState(staleUpdate, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.entityLeft, { entity_id: 'npc-1' }, 5),
    });
    const freshUpdate = reduceClientState(duplicateLeave, {
      type: 'eventReceived',
      envelope: event(
        CLIENT_EVENTS.entityUpdated,
        {
          entity_id: 'npc-1',
          entity_type: 'npc',
          position: { x: 30, y: 40 },
          display: { label: 'Fresh Drone', disposition: 'hostile' },
        },
        6,
      ),
    });

    expect(entered.lastSequence).toBe(5);
    expect(staleUpdate.visibleEntities['npc-1'].position).toEqual({ x: 10, y: 20 });
    expect(duplicateLeave.visibleEntities['npc-1']).toBeDefined();
    expect(freshUpdate.visibleEntities['npc-1'].position).toEqual({ x: 30, y: 40 });
    expect(freshUpdate.lastSequence).toBe(6);
  });
});

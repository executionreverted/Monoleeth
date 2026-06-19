import { describe, expect, test } from 'vitest';

import { activeEntityMovement, currentEntityPosition, estimateServerTime, movementTiming, selfEntity, serverClockOffset } from './movement';
import type { EntityPayload } from '../protocol/envelope';

describe('movement helpers', () => {
  test('estimates server time from monotonic local offset', () => {
    const offset = serverClockOffset(2500, 10_000);

    expect(offset).toBe(-7500);
    expect(estimateServerTime(2750, offset)).toBe(10_250);
  });

  test('interpolates server-owned movement and remaining time', () => {
    const timing = movementTiming(
      {
        moving: true,
        origin: { x: 10, y: 20 },
        target: { x: 110, y: 220 },
        speed: 180,
        started_at_ms: 1000,
        arrive_at_ms: 3000,
      },
      2000,
    );

    expect(timing.current).toEqual({ x: 60, y: 120 });
    expect(timing.distance).toBeCloseTo(223.607, 3);
    expect(timing.durationMs).toBe(2000);
    expect(timing.elapsedMs).toBe(1000);
    expect(timing.remainingMs).toBe(1000);
    expect(timing.progress).toBe(0.5);
  });

  test('does not expose an active ETA after server arrival time', () => {
    const entity: EntityPayload = {
      entity_id: 'player-1',
      entity_type: 'player',
      position: { x: 0, y: 0 },
      status_flags: ['self'],
      movement: {
        moving: true,
        origin: { x: 0, y: 0 },
        target: { x: 100, y: 0 },
        speed: 100,
        started_at_ms: 1000,
        arrive_at_ms: 2000,
      },
    };

    expect(currentEntityPosition(entity, 1500)).toEqual({ x: 50, y: 0 });
    expect(currentEntityPosition(entity, 2500)).toEqual({ x: 100, y: 0 });
    expect(activeEntityMovement(entity, 2500)).toBeNull();
  });

  test('selects explicit self entity before generic player fallback', () => {
    const fallback: EntityPayload = {
      entity_id: 'other-player',
      entity_type: 'player',
      position: { x: 10, y: 0 },
    };
    const self: EntityPayload = {
      entity_id: 'local-player',
      entity_type: 'player',
      position: { x: 0, y: 0 },
      status_flags: ['self'],
    };

    expect(selfEntity([fallback, self])?.entity_id).toBe('local-player');
  });
});

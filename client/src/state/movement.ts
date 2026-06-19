import { EntityMovementPayload, EntityPayload, Vec2 } from '../protocol/envelope';

export const LONG_RANGE_MOVE_STEP_UNITS = 900;

export interface MovementTiming {
  origin: Vec2;
  target: Vec2;
  current: Vec2;
  distance: number;
  durationMs: number;
  elapsedMs: number;
  remainingMs: number;
  progress: number;
}

export function serverClockOffset(localNow: number, serverTime: number): number {
  return localNow - serverTime;
}

export function estimateServerTime(localNow: number, offset: number): number {
  return localNow - offset;
}

export function movementTiming(movement: EntityMovementPayload, serverNow: number): MovementTiming {
  const durationMs = Math.max(1, movement.arrive_at_ms - movement.started_at_ms);
  const elapsedMs = clamp(serverNow - movement.started_at_ms, 0, durationMs);
  const progress = elapsedMs / durationMs;
  const distance = distanceBetween(movement.origin, movement.target);
  return {
    origin: movement.origin,
    target: movement.target,
    current: {
      x: lerp(movement.origin.x, movement.target.x, progress),
      y: lerp(movement.origin.y, movement.target.y, progress),
    },
    distance,
    durationMs,
    elapsedMs,
    remainingMs: Math.max(0, movement.arrive_at_ms - serverNow),
    progress,
  };
}

export function currentEntityPosition(entity: EntityPayload, serverNow: number): Vec2 {
  const movement = entity.movement;
  if (!movement?.moving || movement.arrive_at_ms <= movement.started_at_ms) {
    return entity.position;
  }
  return movementTiming(movement, serverNow).current;
}

export function activeEntityMovement(entity: EntityPayload, serverNow: number): MovementTiming | null {
  const movement = entity.movement;
  if (!movement?.moving || movement.arrive_at_ms <= movement.started_at_ms) {
    return null;
  }
  const timing = movementTiming(movement, serverNow);
  return timing.remainingMs > 0 ? timing : null;
}

export function selfEntity(entities: Record<string, EntityPayload> | EntityPayload[]): EntityPayload | null {
  const list = Array.isArray(entities) ? entities : Object.values(entities);
  return list.find(isSelfEntity) ?? list.find((entity) => entity.entity_type === 'player') ?? null;
}

export function isSelfEntity(entity: EntityPayload): boolean {
  return entity.status_flags?.includes('self') || entity.status_flags?.includes('local') || false;
}

export function distanceBetween(a: Vec2, b: Vec2): number {
  return Math.hypot(b.x - a.x, b.y - a.y);
}

export function boundedMovementTarget(origin: Vec2, target: Vec2, maxDistance = LONG_RANGE_MOVE_STEP_UNITS): Vec2 {
  const distance = distanceBetween(origin, target);
  if (!Number.isFinite(distance) || !Number.isFinite(maxDistance) || maxDistance <= 0 || distance <= maxDistance) {
    return { ...target };
  }

  const ratio = maxDistance / distance;
  return {
    x: origin.x + (target.x - origin.x) * ratio,
    y: origin.y + (target.y - origin.y) * ratio,
  };
}

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

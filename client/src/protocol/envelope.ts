export const PROTOCOL_VERSION = 1 as const;

export const OPERATIONS = {
  sessionSnapshot: 'session.snapshot',
  worldSnapshot: 'world.snapshot',
  moveTo: 'move_to',
  stop: 'stop',
  debugSpawnNPC: 'debug_spawn_npc',
  debugSnapshot: 'debug_snapshot',
  combatUseSkill: 'combat.use_skill',
  lootPickup: 'loot.pickup',
  scanPulse: 'scan.pulse',
} as const;

export type Operation = (typeof OPERATIONS)[keyof typeof OPERATIONS];

export const CLIENT_EVENTS = {
  sessionReady: 'session.ready',
  playerSnapshot: 'player.snapshot',
  shipSnapshot: 'ship.snapshot',
  cargoSnapshot: 'cargo.snapshot',
  walletSnapshot: 'wallet.snapshot',
  statsSnapshot: 'stats.updated',
  worldSnapshot: 'world.snapshot',
  entityEntered: 'aoi.entity_entered',
  entityUpdated: 'aoi.entity_updated',
  entityLeft: 'aoi.entity_left',
  positionCorrected: 'position.corrected',
  movementStopped: 'movement.stopped',
  serverNotice: 'server.notice',
} as const;

export type ClientEventType = (typeof CLIENT_EVENTS)[keyof typeof CLIENT_EVENTS] | string;

export interface Vec2 {
  x: number;
  y: number;
}

export type EntityType = 'player' | 'npc' | 'loot' | 'planet_signal';

export interface EntityDisplay {
  label?: string;
  disposition?: string;
}

export interface EntityPayload {
  entity_id: string;
  entity_type: EntityType;
  position: Vec2;
  status_flags?: string[];
  display?: EntityDisplay;
}

export interface RequestEnvelope<TPayload extends JsonObject = JsonObject> {
  request_id: string;
  op: Operation;
  payload: TPayload;
  client_seq: number;
  v: number;
}

export interface ResponseEnvelope<TPayload extends JsonObject = JsonObject> {
  request_id: string;
  ok: true;
  payload: TPayload;
  server_time: number;
  v: number;
}

export interface ErrorPayload {
  code: string;
  message: string;
  retryable: boolean;
}

export interface ErrorEnvelope {
  request_id: string;
  ok: false;
  error: ErrorPayload;
  server_time: number;
  v: number;
}

export interface EventEnvelope<TPayload extends JsonObject = JsonObject> {
  event_id: string;
  type: ClientEventType;
  payload: TPayload;
  server_time: number;
  seq: number;
  v: number;
}

export type ServerMessage = ResponseEnvelope | ErrorEnvelope | EventEnvelope;

export type JsonValue = unknown;
export type JsonObject = Record<string, unknown>;

const forbiddenPayloadKeys = new Set([
  'account_id',
  'session_id',
  'world_id',
  'zone_id',
  'client_player_id',
  'player_id',
  'damage',
  'xp',
  'loot',
  'cooldown',
  'wallet_amount',
  'hidden',
  'internal',
  'internal_metadata',
  'gameplay_seed',
  'future_spawn',
  'future_spawn_data',
  'spawn_candidates',
  'loot_roll',
  'loot_table',
]);

export function parseServerMessage(raw: string): ServerMessage {
  const parsed = parseJson(raw);
  assertJsonObject(parsed, 'server message');
  assertVersion(parsed);

  if ('ok' in parsed) {
    if (parsed.ok === true) {
      const payload = requireObject(parsed.payload, 'response payload');
      rejectForbiddenPayloadKeys(payload);
      return {
        request_id: requireString(parsed.request_id, 'request_id'),
        ok: true,
        payload,
        server_time: requireNumber(parsed.server_time, 'server_time'),
        v: requireNumber(parsed.v, 'v'),
      };
    }

    if (parsed.ok === false) {
      const error = requireObject(parsed.error, 'error');
      return {
        request_id: requireString(parsed.request_id, 'request_id'),
        ok: false,
        error: {
          code: requireString(error.code, 'error.code'),
          message: requireString(error.message, 'error.message'),
          retryable: requireBoolean(error.retryable, 'error.retryable'),
        },
        server_time: requireNumber(parsed.server_time, 'server_time'),
        v: requireNumber(parsed.v, 'v'),
      };
    }
  }

  const payload = requireObject(parsed.payload, 'event payload');
  rejectForbiddenPayloadKeys(payload);

  return {
    event_id: requireString(parsed.event_id, 'event_id'),
    type: requireString(parsed.type, 'type'),
    payload,
    server_time: requireNumber(parsed.server_time, 'server_time'),
    seq: requireNumber(parsed.seq, 'seq'),
    v: requireNumber(parsed.v, 'v'),
  };
}

export function rejectForbiddenPayloadKeys(value: JsonValue): void {
  const found = findForbiddenPayloadKey(value);
  if (found) {
    throw new Error('Forbidden server payload rejected.');
  }
}

export function findForbiddenPayloadKey(value: JsonValue): string | null {
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findForbiddenPayloadKey(item);
      if (found) {
        return found;
      }
    }
    return null;
  }

  if (!isJsonObject(value)) {
    return null;
  }

  for (const [key, child] of Object.entries(value)) {
    const normalized = key.toLowerCase();
    if (forbiddenPayloadKeys.has(normalized)) {
      return key;
    }
    const childFound = findForbiddenPayloadKey(child);
    if (childFound) {
      return childFound;
    }
  }

  return null;
}

export function isJsonObject(value: unknown): value is JsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function parseJson(raw: string): unknown {
  try {
    return JSON.parse(raw) as unknown;
  } catch (error) {
    throw new Error(`Invalid JSON message: ${error instanceof Error ? error.message : String(error)}`);
  }
}

function assertJsonObject(value: unknown, label: string): asserts value is JsonObject {
  if (!isJsonObject(value)) {
    throw new Error(`${label} must be a JSON object.`);
  }
}

function requireObject(value: unknown, label: string): JsonObject {
  assertJsonObject(value, label);
  return value;
}

function requireString(value: unknown, label: string): string {
  if (typeof value !== 'string' || value.trim() === '') {
    throw new Error(`${label} must be a non-empty string.`);
  }
  return value;
}

function requireNumber(value: unknown, label: string): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new Error(`${label} must be a finite number.`);
  }
  return value;
}

function requireBoolean(value: unknown, label: string): boolean {
  if (typeof value !== 'boolean') {
    throw new Error(`${label} must be a boolean.`);
  }
  return value;
}

function assertVersion(value: JsonObject): void {
  const version = value.v;
  if (version !== undefined && version !== PROTOCOL_VERSION) {
    throw new Error(`Unsupported protocol version: ${String(version)}`);
  }
}

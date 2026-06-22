import { EntityPayload, JsonObject, JsonValue, Vec2 } from '../protocol/envelope';
import type { ClientState, LogLine, ScanModeState } from './types';

export function isVec2(value: JsonValue | unknown): value is Vec2 {
  return (
    typeof value === 'object' &&
    value !== null &&
    !Array.isArray(value) &&
    typeof (value as Vec2).x === 'number' &&
    Number.isFinite((value as Vec2).x) &&
    typeof (value as Vec2).y === 'number' &&
    Number.isFinite((value as Vec2).y)
  );
}

export function isJsonObject(value: JsonValue | unknown): value is JsonObject {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export function objectField(payload: JsonObject, key: string): JsonObject | null {
  const value = payload[key];
  return isJsonObject(value) ? value : null;
}

export function stringField(payload: JsonObject, key: string): string | null {
  const value = payload[key];
  return typeof value === 'string' ? value : null;
}

export function numberField(payload: JsonObject, key: string): number | null {
  const value = payload[key];
  return typeof value === 'number' && Number.isFinite(value) ? value : null;
}

export function roundedOptional(payload: JsonObject, key: string): number | undefined {
  const value = numberField(payload, key);
  return value === null ? undefined : Math.max(0, Math.round(value));
}

export function booleanField(payload: JsonObject, key: string): boolean | null {
  const value = payload[key];
  return typeof value === 'boolean' ? value : null;
}

export function optionalRoundedNumber(payload: JsonObject, key: string, fallback: number | undefined): number | undefined {
  const value = numberField(payload, key);
  if (value === null) {
    return fallback;
  }
  return Math.max(0, Math.round(value));
}

export function isKnownEntityType(entityType: string): entityType is EntityPayload['entity_type'] {
  return (
    entityType === 'player' ||
    entityType === 'npc' ||
    entityType === 'loot' ||
    entityType === 'planet_signal'
  );
}

const safeStatusFlags = new Set([
  'damaged',
  'friendly',
  'hostile',
  'known_intel',
  'load_smoke',
  'local',
  'loot',
  'moving',
  'neutral',
  'scan_revealed',
  'scannable',
  'self',
  'stealthed',
  'unknown_signal',
  'visible',
]);

export function parsePublicStatusFlags(value: unknown): string[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const flags = value.filter((flag): flag is string => typeof flag === 'string' && safeStatusFlags.has(flag));
  if (!flags.includes('self')) {
    const selfOnly = flags.indexOf('stealthed');
    if (selfOnly >= 0) {
      flags.splice(selfOnly, 1);
    }
  }
  return flags.length > 0 ? flags : undefined;
}

export function initialScanMode(): ScanModeState {
  return {
    enabled: false,
    nextPulseAt: null,
    lastRejectedAt: null,
    lastError: null,
  };
}

export function appendLog(lines: LogLine[], level: LogLine['level'], text: string): LogLine[] {
  return [...lines.slice(-39), newLog(level, text)];
}

export function clearGameplay(state: ClientState): ClientState {
  return {
    ...state,
    lastServerTime: null,
    lastSequence: 0,
    mapSubscriptionEpoch: null,
    mapTransfer: null,
    currentMap: null,
    portalCooldowns: {},
    playerSnapshot: null,
    sector: null,
    minimap: null,
    visibleEntities: {},
    selectedTargetID: null,
    movementTarget: null,
    lastCorrection: null,
    knownLoot: {},
    worldEffects: [],
    pendingCommands: {},
    commandLog: [],
    combatLog: [],
    cargo: null,
    wallet: null,
    ship: null,
    stats: null,
    progression: null,
    inventory: null,
    hangar: null,
    loadout: null,
    crafting: null,
    repairQuote: null,
    skillCooldowns: {},
    questBoard: null,
    planetIntel: null,
    scanMode: initialScanMode(),
    production: null,
    routes: null,
    shopCatalog: null,
    market: null,
    auction: null,
    premium: null,
    economyDashboard: null,
    adminInspection: null,
    adminRepair: null,
    commandLogSummary: null,
    metrics: null,
    releaseGate: null,
    abuseCoverage: null,
    lastError: null,
  };
}

export function newLog(level: LogLine['level'], text: string): LogLine {
  return {
    id: `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`,
    level,
    text,
    at: Date.now(),
  };
}

export function defaultSocketURL(): string {
  if (typeof window === 'undefined') {
    return 'ws://127.0.0.1:5173/ws';
  }

  const { protocol, host } = window.location;
  const wsProtocol = protocol === 'https:' ? 'wss:' : 'ws:';
  return `${wsProtocol}//${host}/ws`;
}

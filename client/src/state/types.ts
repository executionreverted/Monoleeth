import { EntityPayload, ErrorPayload, EventEnvelope, JsonObject, RequestEnvelope, ResponseEnvelope, Vec2 } from '../protocol/envelope';

export type ConnectionStatus = 'offline' | 'connecting' | 'connected' | 'reconnecting' | 'error';

export interface PlayerSnapshot extends JsonObject {
  hp?: number;
  shield?: number;
  energy?: number;
  max_hp?: number;
  max_shield?: number;
  max_energy?: number;
  rank?: number;
  callsign?: string;
}

export interface LogLine {
  id: string;
  level: 'info' | 'warn' | 'error';
  text: string;
  at: number;
}

export interface CargoSummary {
  used: number;
  capacity: number;
  items: Array<{ item_id: string; quantity: number }>;
}

export interface PendingCommand {
  requestID: string;
  op: string;
  queuedAt: number;
}

export interface ClientState {
  connectionStatus: ConnectionStatus;
  socketURL: string;
  lastServerTime: number | null;
  lastSequence: number;
  playerSnapshot: PlayerSnapshot;
  visibleEntities: Record<string, EntityPayload>;
  selectedTargetID: string | null;
  movementTarget: Vec2 | null;
  lastCorrection: { entityID: string; position: Vec2 } | null;
  pendingCommands: Record<string, PendingCommand>;
  commandLog: LogLine[];
  combatLog: LogLine[];
  cargo: CargoSummary;
  questBoard: { available: number; active: number };
  inventory: { equipped: number; storage: number };
  planetIntel: { knownSignals: number; staleIntel: number };
  lastError: ErrorPayload | null;
}

export type ClientAction =
  | { type: 'connectionChanged'; status: ConnectionStatus; socketURL?: string }
  | { type: 'requestQueued'; envelope: RequestEnvelope }
  | {
      type: 'responseReceived';
      envelope: ResponseEnvelope | { ok: false; error: ErrorPayload; request_id: string; server_time: number; v?: number };
    }
  | { type: 'replaceVisibleEntities'; entities: EntityPayload[]; serverTime?: number | null; sequence?: number }
  | { type: 'eventReceived'; envelope: EventEnvelope }
  | { type: 'serverCorrection'; entityID: string; position: Vec2; serverTime?: number }
  | { type: 'selectTarget'; entityID: string | null }
  | { type: 'appendLog'; level: LogLine['level']; text: string };

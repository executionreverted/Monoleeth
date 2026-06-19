import { EntityPayload, ErrorPayload, EventEnvelope, JsonObject, RequestEnvelope, ResponseEnvelope, Vec2 } from '../protocol/envelope';

export type ConnectionStatus =
  | 'restoring'
  | 'logged_out'
  | 'authenticated_pending_socket'
  | 'connecting'
  | 'connected'
  | 'reconnecting'
  | 'auth_expired'
  | 'offline'
  | 'error';

export type ClientMode = 'real' | 'demo';

export interface PublicAccount {
  email: string;
  admin: boolean;
}

export interface PublicPlayer {
  callsign: string;
}

export interface PublicSession extends JsonObject {
  authenticated: boolean;
  account?: PublicAccount;
  player?: PublicPlayer;
  roles?: string[];
  expires_at?: number;
  server_time: number;
}

export interface ClientAuthState {
  mode: ClientMode;
  session: PublicSession | null;
  submitting: boolean;
  error: string | null;
}

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

export interface WalletSummary {
  credits: number;
  premium_paid: number;
  premium_earned: number;
}

export interface ShipSummary {
  active_ship_id: string;
  display_name: string;
  hull: number;
  max_hull: number;
  shield: number;
  max_shield: number;
  capacitor: number;
  max_capacitor: number;
  disabled: boolean;
  repair_state: string;
}

export interface StatSummary {
  speed: number;
  radar_range: number;
  weapon_range: number;
  cargo_capacity: number;
}

export interface ProgressionSummary {
  main_level: number;
  main_xp: number;
  rank: number;
  combat_level?: number;
  combat_xp?: number;
}

export interface RepairQuote {
  ship_id: string;
  currency: string;
  cost: number;
  disabled: boolean;
}

export interface SectorSummary {
  name: string;
  region: string;
  danger: string;
  contested: boolean;
}

export interface MinimapContact {
  entity_id: string;
  entity_type: EntityPayload['entity_type'];
  position: Vec2;
  disposition?: string;
  status_flags?: string[];
}

export interface MinimapMemory {
  kind: string;
  label: string;
  position: Vec2;
  freshness: string;
}

export interface MinimapSummary {
  radar_range: number;
  live_contacts: MinimapContact[];
  remembered: MinimapMemory[];
}

export interface PendingCommand {
  requestID: string;
  op: string;
  queuedAt: number;
}

export interface ClientState {
  auth: ClientAuthState;
  connectionStatus: ConnectionStatus;
  socketURL: string;
  lastServerTime: number | null;
  lastSequence: number;
  playerSnapshot: PlayerSnapshot | null;
  sector: SectorSummary | null;
  minimap: MinimapSummary | null;
  visibleEntities: Record<string, EntityPayload>;
  selectedTargetID: string | null;
  movementTarget: Vec2 | null;
  lastCorrection: { entityID: string; position: Vec2 } | null;
  pendingCommands: Record<string, PendingCommand>;
  commandLog: LogLine[];
  combatLog: LogLine[];
  cargo: CargoSummary | null;
  wallet: WalletSummary | null;
  ship: ShipSummary | null;
  stats: StatSummary | null;
  progression: ProgressionSummary | null;
  repairQuote: RepairQuote | null;
  skillCooldowns: Record<string, number>;
  questBoard: { available: number; active: number } | null;
  inventory: { equipped: number; storage: number } | null;
  planetIntel: { knownSignals: number; staleIntel: number | null } | null;
  lastError: ErrorPayload | null;
}

export type ClientAction =
  | { type: 'demoModeStarted' }
  | { type: 'authRestoreStarted' }
  | { type: 'authSubmitStarted' }
  | { type: 'authSessionLoaded'; session: PublicSession }
  | { type: 'authLoggedOut' }
  | { type: 'authExpired'; message?: string }
  | { type: 'authFailed'; message: string }
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

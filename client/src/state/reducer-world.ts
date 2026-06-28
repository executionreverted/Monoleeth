import {
  CLIENT_EVENTS,
  EntityPayload,
  EventEnvelope,
  JsonObject,
  rejectForbiddenPayloadKeys,
  Vec2,
} from '../protocol/envelope';
import type {
  ClientState,
  KnownLootDrop,
  MinimapContact,
  MinimapSummary,
  WorldFeedbackEffect,
} from './types';
import {
  booleanField,
  initialCombatEngagement,
  isJsonObject,
  initialScanMode,
  isKnownEntityType,
  isVec2,
  numberField,
  objectField,
  parsePublicStatusFlags,
  roundedOptional,
  stringField,
} from './reducer-helpers';
import { countPlanetSignals, updateVisibleSignalCount } from './reducer-discovery';
import { mapSubscriptionEpochFromPayload } from './reducer-map';

export function replaceVisibleEntities(
  state: ClientState,
  entities: EntityPayload[],
  serverTime: number | null,
  sequence?: number,
): ClientState {
  const visibleEntities: Record<string, EntityPayload> = {};
  for (const entity of entities) {
    rejectForbiddenPayloadKeys(entity);
    visibleEntities[entity.entity_id] = entity;
  }

  return {
    ...state,
    visibleEntities,
    selectedTargetID:
      state.selectedTargetID && visibleEntities[state.selectedTargetID] ? state.selectedTargetID : null,
    movementTarget: movementTargetFromAuthoritativeSelf(visibleEntities, null),
    lastCorrection: null,
    knownLoot: retainVisibleLootDrops(state.knownLoot, visibleEntities),
    minimap: rebuildMinimapLiveContacts(state.minimap, visibleEntities),
    planetIntel: updateVisibleSignalCount(state.planetIntel, countPlanetSignals(visibleEntities)),
    lastServerTime: serverTime,
    lastSequence: sequence ? Math.max(state.lastSequence, sequence) : state.lastSequence,
  };
}

export function clearOriginMapLiveState(state: ClientState): ClientState {
  return {
    ...state,
    visibleEntities: {},
    selectedTargetID: null,
    movementTarget: null,
    lastCorrection: null,
    mapTransfer: null,
    currentMap: null,
    portalCooldowns: {},
    knownLoot: {},
    worldEffects: [],
    combatEngagement: initialCombatEngagement(),
    skillCooldowns: {},
    planetIntel: state.planetIntel
      ? {
          ...state.planetIntel,
          knownSignals: 0,
          selectedPlanet: null,
          lastScan: null,
        }
      : null,
    scanMode: initialScanMode(),
    minimap: state.minimap
      ? {
          ...state.minimap,
          public_map_key: undefined,
          bounds: undefined,
          visible_portals: [],
          safe_zones: [],
          live_contacts: [],
        }
      : null,
  };
}

function contactFromEntity(entity: EntityPayload, fallback?: MinimapContact): MinimapContact {
  const contact: MinimapContact = {
    entity_id: entity.entity_id,
    entity_type: entity.entity_type,
    position: entity.position,
    disposition: entity.display?.disposition,
    status_flags: entity.status_flags ? [...entity.status_flags] : undefined,
  };
  if (entity.projection_source) {
    contact.projection_source = entity.projection_source;
    return contact;
  }
  if (fallback?.projection_source) {
    contact.projection_source = fallback.projection_source;
  }
  return contact;
}

export function upsertMinimapContact(minimap: MinimapSummary | null, entity: EntityPayload): MinimapSummary | null {
  if (!minimap) {
    return minimap;
  }
  const previous = minimap.live_contacts.find((entry) => entry.entity_id === entity.entity_id);
  const contact = contactFromEntity(entity, previous);
  const nextContacts = minimap.live_contacts.filter((entry) => entry.entity_id !== entity.entity_id);
  nextContacts.push(contact);
  nextContacts.sort((a, b) => a.entity_id.localeCompare(b.entity_id));
  return {
    ...minimap,
    live_contacts: nextContacts,
  };
}

function rebuildMinimapLiveContacts(
  minimap: MinimapSummary | null,
  visibleEntities: Record<string, EntityPayload>,
): MinimapSummary | null {
  if (!minimap) {
    return minimap;
  }
  const previousByID = new Map(minimap.live_contacts.map((contact) => [contact.entity_id, contact]));
  const liveContacts = Object.values(visibleEntities)
    .map((entity) => contactFromEntity(entity, previousByID.get(entity.entity_id)))
    .sort((a, b) => a.entity_id.localeCompare(b.entity_id));
  return {
    ...minimap,
    live_contacts: liveContacts,
  };
}

export function removeMinimapContact(minimap: MinimapSummary | null, entityID: string): MinimapSummary | null {
  if (!minimap) {
    return minimap;
  }
  const nextContacts = minimap.live_contacts.filter((entry) => entry.entity_id !== entityID);
  if (nextContacts.length === minimap.live_contacts.length) {
    return minimap;
  }
  return {
    ...minimap,
    live_contacts: nextContacts,
  };
}

export function isStaleEvent(state: ClientState, envelope: EventEnvelope): boolean {
  if (envelope.seq > 0 && state.lastSequence > 0 && envelope.seq <= state.lastSequence) {
    return true;
  }

  const eventEpoch = mapSubscriptionEpochFromPayload(envelope.payload);
  if (eventEpoch === null || state.mapSubscriptionEpoch === null) {
    return false;
  }
  if (envelope.type === CLIENT_EVENTS.mapTransferStarted) {
    return false;
  }
  if (
    envelope.type === CLIENT_EVENTS.mapTransferCompleted ||
    envelope.type === CLIENT_EVENTS.worldSnapshot ||
    envelope.type === CLIENT_EVENTS.mapSnapshot ||
    envelope.type === CLIENT_EVENTS.mapChanged
  ) {
    return eventEpoch < state.mapSubscriptionEpoch;
  }
  if (!isMapScopedClientEvent(envelope.type)) {
    return false;
  }
  return eventEpoch !== state.mapSubscriptionEpoch;
}

function isMapScopedClientEvent(eventType: string): boolean {
  return (
    eventType === CLIENT_EVENTS.entityEntered ||
    eventType === CLIENT_EVENTS.entityUpdated ||
    eventType === CLIENT_EVENTS.entityLeft ||
    eventType === CLIENT_EVENTS.positionCorrected ||
    eventType === CLIENT_EVENTS.movementStopped ||
    eventType === CLIENT_EVENTS.targetUpdated ||
    eventType === CLIENT_EVENTS.combatDamage ||
    eventType === CLIENT_EVENTS.combatMiss ||
    eventType === CLIENT_EVENTS.combatCooldownStarted ||
    eventType === CLIENT_EVENTS.combatAttackStarted ||
    eventType === CLIENT_EVENTS.combatAttackStopped ||
    eventType === CLIENT_EVENTS.combatShotStarted ||
    eventType === CLIENT_EVENTS.combatShotResolved ||
    eventType === CLIENT_EVENTS.combatStateSnapshot ||
    eventType === CLIENT_EVENTS.combatNPCKilled ||
    eventType === CLIENT_EVENTS.lootCreated ||
    eventType === CLIENT_EVENTS.lootUpdated ||
    eventType === CLIENT_EVENTS.lootRemoved ||
    eventType === CLIENT_EVENTS.lootPickedUp ||
    eventType === CLIENT_EVENTS.scanPulseStarted ||
    eventType === CLIENT_EVENTS.scanPulseResolved ||
    eventType === CLIENT_EVENTS.scanPlanetDiscovered ||
    eventType === CLIENT_EVENTS.knownPlanets ||
    eventType === CLIENT_EVENTS.planetDetail ||
    eventType === CLIENT_EVENTS.planetClaimed ||
    eventType === CLIENT_EVENTS.productionSummary ||
    eventType === CLIENT_EVENTS.planetStorageSummary ||
    eventType === CLIENT_EVENTS.routeList ||
    eventType === CLIENT_EVENTS.routeSnapshot ||
    eventType === CLIENT_EVENTS.worldSnapshot ||
    eventType === CLIENT_EVENTS.mapSnapshot ||
    eventType === CLIENT_EVENTS.mapChanged ||
    eventType === CLIENT_EVENTS.portalCooldownStarted ||
    eventType === CLIENT_EVENTS.mapPolicyUpdated ||
    eventType === CLIENT_EVENTS.playerProtectionUpdated ||
    eventType === CLIENT_EVENTS.mapTransferStarted ||
    eventType === CLIENT_EVENTS.mapTransferCompleted ||
    eventType === CLIENT_EVENTS.mapTransferFailed
  );
}

export function resetsRealtimeStream(status: ClientState['connectionStatus']): boolean {
  return status === 'authenticated_pending_socket' || status === 'connected' || status === 'reconnecting';
}

export function withoutPendingOperations(state: ClientState, operations: readonly string[]): ClientState {
  if (operations.length === 0) {
    return state;
  }

  const operationSet = new Set(operations);
  let changed = false;
  const pendingCommands: ClientState['pendingCommands'] = {};
  for (const [requestID, pending] of Object.entries(state.pendingCommands)) {
    if (operationSet.has(pending.op)) {
      changed = true;
      continue;
    }
    pendingCommands[requestID] = pending;
  }

  return changed ? { ...state, pendingCommands } : state;
}

export function applyCorrection(
  state: ClientState,
  entityID: string,
  position: Vec2,
  movement: EntityPayload['movement'],
  serverTime: number | null,
  sequence?: number,
): ClientState {
  const entity = state.visibleEntities[entityID];
  let visibleEntities = state.visibleEntities;
  if (entity) {
    const correctedEntity: EntityPayload = {
      ...entity,
      position,
      movement,
    };
    if (!movement) {
      delete correctedEntity.movement;
    }
    visibleEntities = {
      ...state.visibleEntities,
      [entityID]: correctedEntity,
    };
  }

  return {
    ...state,
    visibleEntities,
    minimap: entity
      ? upsertMinimapContact(state.minimap, { ...entity, position, movement })
      : state.minimap,
    movementTarget: movement?.moving ? movement.target : null,
    lastCorrection: { entityID, position },
    lastServerTime: serverTime,
    lastSequence: sequence ? Math.max(state.lastSequence, sequence) : state.lastSequence,
  };
}

export function applyTargetUpdated(state: ClientState, envelope: EventEnvelope): ClientState {
  const entityID = requireEntityID(envelope.payload);
  const combat = parseEntityCombat(envelope.payload);
  const entity = state.visibleEntities[entityID];
  const visibleEntities =
    entity && combat
      ? {
          ...state.visibleEntities,
          [entityID]: {
            ...entity,
            combat,
          },
        }
      : state.visibleEntities;

  return {
    ...state,
    visibleEntities,
    lastServerTime: envelope.server_time,
    lastSequence: Math.max(state.lastSequence, envelope.seq),
  };
}

export function appendWorldEffect(state: ClientState, effect: WorldFeedbackEffect | null): ClientState {
  if (!effect) {
    return state;
  }
  const now = Date.now();
  if (state.worldEffects.some((entry) => entry.id === effect.id)) {
    return state;
  }
  return {
    ...state,
    worldEffects: [...state.worldEffects.filter((entry) => entry.expiresAt > now).slice(-17), effect],
  };
}

export function feedbackEffect(
  state: ClientState,
  envelope: EventEnvelope,
  kind: WorldFeedbackEffect['kind'],
): WorldFeedbackEffect | null {
  const targetID =
    stringField(envelope.payload, 'target_id') ??
    stringField(envelope.payload, 'entity_id') ??
    stringField(envelope.payload, 'drop_id') ??
    undefined;
  const sourceID = stringField(envelope.payload, 'source_id') ?? selfEntityID(state);
  const position =
    (isVec2(envelope.payload.position) ? envelope.payload.position : null) ??
    (targetID ? state.visibleEntities[targetID]?.position : undefined) ??
    (targetID ? state.knownLoot[targetID]?.position : undefined);

  if (!targetID && !position) {
    return null;
  }

  const createdAt = Date.now();
  const sourcePosition = sourceID ? state.visibleEntities[sourceID]?.position : null;
  return {
    id: `${envelope.event_id}:${kind}`,
    kind,
    phase: feedbackPhase(envelope.type, kind),
    damageKind: kind === 'damage' ? damageKindFromAmounts(envelope.payload) : undefined,
    targetID,
    targetEntityID: targetID,
    sourceID: sourceID ?? undefined,
    sourceEntityID: sourceID ?? undefined,
    position: position ? { ...position } : undefined,
    sourcePosition: kind === 'laser' && sourcePosition ? { ...sourcePosition } : undefined,
    amount: roundedOptional(envelope.payload, 'amount'),
    shieldAmount: roundedOptional(envelope.payload, 'shield_amount'),
    hullAmount: roundedOptional(envelope.payload, 'hull_amount'),
    itemID: stringField(envelope.payload, 'item_id') ?? (targetID ? state.knownLoot[targetID]?.item_id : undefined),
    quantity: roundedOptional(envelope.payload, 'quantity') ?? (targetID ? state.knownLoot[targetID]?.quantity : undefined),
    createdAt,
    expiresAt: createdAt + feedbackDuration(kind),
  };
}

function feedbackPhase(eventType: string, kind: WorldFeedbackEffect['kind']): WorldFeedbackEffect['phase'] | undefined {
  if (eventType === CLIENT_EVENTS.combatShotStarted) {
    return 'started';
  }
  if (
    eventType === CLIENT_EVENTS.combatShotResolved ||
    eventType === CLIENT_EVENTS.combatDamage ||
    eventType === CLIENT_EVENTS.combatMiss ||
    kind === 'damage' ||
    kind === 'miss'
  ) {
    return 'resolved';
  }
  return undefined;
}

function damageKindFromAmounts(payload: JsonObject): WorldFeedbackEffect['damageKind'] | undefined {
  const shield = roundedOptional(payload, 'shield_amount') ?? 0;
  const hull = roundedOptional(payload, 'hull_amount') ?? 0;
  if (shield > 0 && hull > 0) {
    return 'mixed';
  }
  if (shield > 0) {
    return 'shield';
  }
  if (hull > 0 || (roundedOptional(payload, 'amount') ?? 0) > 0) {
    return 'hull';
  }
  return undefined;
}

function feedbackDuration(kind: WorldFeedbackEffect['kind']): number {
  switch (kind) {
    case 'laser':
      return 700;
    case 'damage':
    case 'miss':
      return 2200;
    case 'destroyed':
    case 'loot_spawn':
    case 'loot_pickup':
      return 3200;
  }
}

export function parseKnownLootDrop(payload: JsonObject): KnownLootDrop | null {
  const dropID = stringField(payload, 'drop_id') ?? stringField(payload, 'entity_id');
  const itemID = stringField(payload, 'item_id');
  const quantity = numberField(payload, 'quantity');
  if (!dropID || !itemID || quantity === null || quantity <= 0) {
    return null;
  }

  return {
    drop_id: dropID,
    item_id: itemID,
    quantity: Math.round(quantity),
    state: stringField(payload, 'state') ?? undefined,
    expires_at: numberField(payload, 'expires_at') ?? undefined,
    position: isVec2(payload.position) ? { ...payload.position } : undefined,
  };
}

function retainVisibleLootDrops(
  knownLoot: Record<string, KnownLootDrop>,
  visibleEntities: Record<string, EntityPayload>,
): Record<string, KnownLootDrop> {
  const retained: Record<string, KnownLootDrop> = {};
  for (const [dropID, drop] of Object.entries(knownLoot)) {
    if (visibleEntities[dropID]?.entity_type === 'loot') {
      retained[dropID] = drop;
    }
  }
  return retained;
}

function selfEntityID(state: ClientState): string | null {
  return Object.values(state.visibleEntities).find(isSelfEntity)?.entity_id ?? null;
}

export function displayNameForEntity(state: ClientState, entityID: string | null): string {
  if (!entityID) {
    return 'target';
  }
  return state.visibleEntities[entityID]?.display?.label ?? entityID;
}

export function parseSnapshotEntities(payload: JsonObject): EntityPayload[] | null {
  if (!('entities' in payload)) {
    return null;
  }

  rejectForbiddenPayloadKeys(payload);

  if (!Array.isArray(payload.entities)) {
    throw new Error('Snapshot entities must be an array.');
  }

  return payload.entities.map((entity) => {
    if (!isJsonObject(entity)) {
      throw new Error('Snapshot entity must be an object.');
    }
    return parseEntityPayload(entity);
  });
}

export function parseEntityPayload(payload: JsonObject): EntityPayload {
  const source = isJsonObject(payload.entity) ? payload.entity : payload;
  const entityID = typeof source.entity_id === 'string' ? source.entity_id : '';
  const entityType = typeof source.entity_type === 'string' ? source.entity_type : '';
  const position = isVec2(source.position) ? source.position : null;

  if (!entityID || !isKnownEntityType(entityType) || !position) {
    throw new Error('Invalid entity payload.');
  }

  return {
    entity_id: entityID,
    entity_type: entityType,
    position,
    status_flags: parsePublicStatusFlags(source.status_flags),
    display: parseEntityDisplay(source),
    combat: parseEntityCombat(source),
    movement: parseEntityMovement(source),
    projection_source: stringField(source, 'projection_source') ?? undefined,
  };
}

export function movementTargetFromAuthoritativeSelf(
  visibleEntities: Record<string, EntityPayload>,
  fallback: Vec2 | null,
): Vec2 | null {
  const self = Object.values(visibleEntities).find(isSelfEntity);
  if (!self) {
    return fallback;
  }
  return self.movement?.moving ? self.movement.target : null;
}

function isSelfEntity(entity: EntityPayload): boolean {
  return entity.status_flags?.includes('self') || entity.status_flags?.includes('local') || false;
}

export function requireEntityID(payload: JsonObject): string {
  if (typeof payload.entity_id === 'string') {
    return payload.entity_id;
  }
  if (typeof payload.id === 'string') {
    return payload.id;
  }
  throw new Error('Missing entity id.');
}

export function requirePosition(payload: JsonObject): Vec2 {
  if (isVec2(payload.position)) {
    return payload.position;
  }
  throw new Error('Missing correction position.');
}

function parseEntityDisplay(payload: JsonObject): EntityPayload['display'] {
  const display = objectField(payload, 'display');
  if (!display) {
    return undefined;
  }
  const label = stringField(display, 'label') ?? undefined;
  const disposition = stringField(display, 'disposition') ?? undefined;
  return label || disposition ? { label, disposition } : undefined;
}

export function parseEntityCombat(payload: JsonObject): EntityPayload['combat'] {
  const combat = objectField(payload, 'combat') ?? payload;
  const hp = numberField(combat, 'hp');
  const maxHP = numberField(combat, 'max_hp');
  const shield = numberField(combat, 'shield');
  const maxShield = numberField(combat, 'max_shield');
  if (hp === null && maxHP === null && shield === null && maxShield === null) {
    return undefined;
  }
  return {
    hp: Math.max(0, Math.round(hp ?? 0)),
    max_hp: Math.max(0, Math.round(maxHP ?? 0)),
    shield: Math.max(0, Math.round(shield ?? 0)),
    max_shield: Math.max(0, Math.round(maxShield ?? 0)),
    status: stringField(combat, 'status') ?? undefined,
  };
}

export function parseEntityMovement(payload: JsonObject): EntityPayload['movement'] {
  const movement = objectField(payload, 'movement');
  if (!movement) {
    return undefined;
  }

  const moving = booleanField(movement, 'moving');
  const origin = isVec2(movement.origin) ? movement.origin : null;
  const target = isVec2(movement.target) ? movement.target : null;
  const speed = numberField(movement, 'speed');
  const startedAt = numberField(movement, 'started_at_ms');
  const arriveAt = numberField(movement, 'arrive_at_ms');
  if (moving !== true || !origin || !target || speed === null || startedAt === null || arriveAt === null) {
    throw new Error('Invalid entity movement payload.');
  }
  if (speed <= 0 || arriveAt < startedAt) {
    throw new Error('Invalid entity movement timing.');
  }

  return {
    moving,
    origin,
    target,
    speed,
    started_at_ms: startedAt,
    arrive_at_ms: arriveAt,
  };
}

import { createRequestId } from './request-id';
import { JsonObject, OPERATIONS, Operation, PROTOCOL_VERSION, RequestEnvelope, Vec2 } from './envelope';

type CommandPayload = JsonObject;

export class CommandBuilder {
  private clientSeq = 0;

  sessionSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.sessionSnapshot, {});
  }

  worldSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.worldSnapshot, {});
  }

  moveTo(target: Vec2): RequestEnvelope<{ target: Vec2 }> {
    return this.build(OPERATIONS.moveTo, { target });
  }

  stop(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.stop, {});
  }

  debugSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.debugSnapshot, {});
  }

  combatUseSkill(targetID: string, skillID = 'basic_laser'): RequestEnvelope<{ target_id: string; skill_id: string }> {
    return this.build(OPERATIONS.combatUseSkill, {
      target_id: targetID,
      skill_id: skillID,
    });
  }

  lootPickup(dropID: string): RequestEnvelope<{ drop_id: string }> {
    return this.build(OPERATIONS.lootPickup, {
      drop_id: dropID,
    });
  }

  deathRepairQuote(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.deathRepairQuote, {});
  }

  deathRepairShip(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.deathRepairShip, {});
  }

  progressionSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.progressionSnapshot, {});
  }

  inventorySnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.inventorySnapshot, {});
  }

  hangarSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.hangarSnapshot, {});
  }

  loadoutSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.loadoutSnapshot, {});
  }

  statsSnapshot(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.statsSnapshot, {});
  }

  craftingRecipes(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.craftingRecipes, {});
  }

  scanPulse(): RequestEnvelope<Record<string, never>> {
    return this.build(OPERATIONS.scanPulse, {});
  }

  debugSpawnNPC(entityID: string, position: Vec2): RequestEnvelope<{ entity_id: string; position: Vec2 }> {
    return this.build(OPERATIONS.debugSpawnNPC, {
      entity_id: entityID,
      position,
    });
  }

  build<TPayload extends CommandPayload>(op: Operation, payload: TPayload): RequestEnvelope<TPayload> {
    assertClientSafePayload(payload);
    this.clientSeq += 1;
    return {
      request_id: createRequestId(),
      op,
      payload,
      client_seq: this.clientSeq,
      v: PROTOCOL_VERSION,
    };
  }
}

export function assertClientSafePayload(payload: CommandPayload): void {
  const forbidden = findTrustedClientField(payload);
  if (forbidden) {
    throw new Error(`Command payload must not include trusted field: ${forbidden}`);
  }
}

function findTrustedClientField(value: unknown): string | null {
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findTrustedClientField(item);
      if (found) {
        return found;
      }
    }
    return null;
  }

  if (typeof value !== 'object' || value === null) {
    return null;
  }

  for (const [key, child] of Object.entries(value)) {
    const normalized = key.toLowerCase();
    if (
      normalized === 'player_id' ||
      normalized === 'account_id' ||
      normalized === 'session_id' ||
      normalized === 'world_id' ||
      normalized === 'zone_id' ||
      normalized === 'damage' ||
      normalized === 'xp' ||
      normalized === 'main_xp' ||
      normalized === 'combat_xp' ||
      normalized === 'role_xp' ||
      normalized === 'rank' ||
      normalized === 'skill_points' ||
      normalized === 'loot' ||
      normalized === 'cooldown' ||
      normalized === 'wallet_amount' ||
      normalized === 'hit' ||
      normalized === 'crit'
    ) {
      return key;
    }

    const childFound = findTrustedClientField(child);
    if (childFound) {
      return childFound;
    }
  }

  return null;
}

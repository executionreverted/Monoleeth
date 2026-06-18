import { EntityPayload, Vec2 } from '../protocol/envelope';

export interface WorldViewState {
  entities: EntityPayload[];
  selectedTargetID: string | null;
  movementTarget: Vec2 | null;
  lastCorrection: { entityID: string; position: Vec2 } | null;
}

export interface WorldInputHandlers {
  onMoveIntent(target: Vec2): void;
  onSelectTarget(entityID: string | null): void;
}

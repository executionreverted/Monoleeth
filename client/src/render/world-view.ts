import { EntityPayload, Vec2 } from '../protocol/envelope';
import { MinimapSummary, SectorSummary, WorldFeedbackEffect } from '../state/types';

export interface WorldViewState {
  entities: EntityPayload[];
  sector: SectorSummary | null;
  minimap: MinimapSummary | null;
  selectedTargetID: string | null;
  movementTarget: Vec2 | null;
  lastCorrection: { entityID: string; position: Vec2 } | null;
  worldEffects: WorldFeedbackEffect[];
  lastServerTime: number | null;
}

export interface WorldInputHandlers {
  onMoveIntent(target: Vec2): void;
  onSelectTarget(entityID: string | null): void;
}

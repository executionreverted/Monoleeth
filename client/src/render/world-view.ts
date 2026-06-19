import { EntityPayload, Vec2 } from '../protocol/envelope';
import { MinimapSummary, ScanModeState, SectorSummary, WorldFeedbackEffect, WorldMapMemoryMarker } from '../state/types';

export interface WorldViewState {
  entities: EntityPayload[];
  sector: SectorSummary | null;
  minimap: MinimapSummary | null;
  selectedTargetID: string | null;
  movementTarget: Vec2 | null;
  lastCorrection: { entityID: string; position: Vec2 } | null;
  memoryMarkers: WorldMapMemoryMarker[];
  worldEffects: WorldFeedbackEffect[];
  scanMode: ScanModeState;
  lastServerTime: number | null;
}

export interface WorldInputHandlers {
  onMoveIntent(target: Vec2): void;
  onSelectTarget(entityID: string | null): void;
  onSelectMemoryMarker(marker: WorldMapMemoryMarker): void;
}

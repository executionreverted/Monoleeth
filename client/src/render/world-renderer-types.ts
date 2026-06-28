import { Graphics, Sprite } from 'pixi.js';

import { EntityPayload, Vec2 } from '../protocol/envelope';
import { isSelfEntity } from '../state/movement';
import { WorldFeedbackEffect, WorldMapMemoryMarker } from '../state/types';
import { isUnknownSignalAsset, worldAssetForEntity } from './world-renderer-assets';

export const entityColors: Record<EntityPayload['entity_type'], number> = {
  player: worldAssetForEntity({ entity_id: 'sample-player', entity_type: 'player', position: { x: 0, y: 0 } }).accentColor,
  npc: worldAssetForEntity({ entity_id: 'sample-npc', entity_type: 'npc', position: { x: 0, y: 0 } }).accentColor,
  loot: worldAssetForEntity({ entity_id: 'sample-loot', entity_type: 'loot', position: { x: 0, y: 0 } }).accentColor,
  planet_signal: worldAssetForEntity({ entity_id: 'sample-signal', entity_type: 'planet_signal', position: { x: 0, y: 0 } }).accentColor,
};

export const hudColors = {
  cyan: 0x2bdfff,
  cyanSoft: 0x8af5ff,
  green: 0x44e878,
  amber: 0xf4c95d,
  red: 0xff4236,
  redSoft: 0xff5c7a,
  line: 0xd8ddde,
  muted: 0x7c8a90,
  panel: 0x03080a,
};

export const npcSwarmOffsets = [
  { x: -20, y: -10, r: 7 },
  { x: -10, y: 13, r: 5 },
  { x: 16, y: -16, r: 6 },
  { x: 21, y: 9, r: 5 },
  { x: 3, y: -2, r: 8 },
];
export const PROJECTILE_TRAVEL_MS = 260;
export const STARFIELD_TILE = { width: 2048, height: 1152 };
export const STARFIELD_GRID_RADIUS = 3;
export const starfieldLayerConfigs = [
  { id: 'far', depth: 0.018, scale: 1.02, alpha: 0.88 },
  { id: 'mid', depth: 0.052, scale: 0.68, alpha: 0.22 },
] as const;

export type StarfieldLayerID = (typeof starfieldLayerConfigs)[number]['id'];

export interface StarfieldTile {
  sprite: Sprite;
  layerID: StarfieldLayerID;
  column: number;
  row: number;
}

export interface StarfieldDebugState {
  assetLoaded: boolean;
  tileCount: number;
  mirroredTiles: number;
  farOffset: Vec2;
  midOffset: Vec2;
  sampleTiles: Array<{ layer: StarfieldLayerID; mirrorX: boolean; mirrorY: boolean; screen: Vec2 }>;
}

export interface ProjectileDebugState {
  id: string;
  source: Vec2;
  target: Vec2;
  head: Vec2;
  progress: number;
  active: boolean;
  alpha: number;
  phase?: WorldFeedbackEffect['phase'];
  sourceEntityID?: string;
  targetEntityID?: string;
}

export function damageKindForEffect(effect: WorldFeedbackEffect): NonNullable<WorldFeedbackEffect['damageKind']> {
  if (effect.damageKind) {
    return effect.damageKind;
  }
  const shield = effect.shieldAmount ?? 0;
  const hull = effect.hullAmount ?? 0;
  if (shield > 0 && hull > 0) {
    return 'mixed';
  }
  if (shield > 0) {
    return 'shield';
  }
  return 'hull';
}

export function projectileDebugFromEffect(
  effect: WorldFeedbackEffect,
  now: number,
  source: Vec2,
  target: Vec2,
): ProjectileDebugState | null {
  if (effect.kind !== 'laser' || effect.expiresAt <= now) {
    return null;
  }
  const progress = clamp((now - effect.createdAt) / PROJECTILE_TRAVEL_MS, 0, 1);
  const totalProgress = clamp((now - effect.createdAt) / Math.max(1, effect.expiresAt - effect.createdAt), 0, 1);
  return {
    id: effect.id,
    source,
    target,
    head: lerpVec(source, target, progress),
    progress,
    active: progress < 1,
    alpha: clamp(1 - totalProgress, 0, 1),
    phase: effect.phase,
    sourceEntityID: effect.sourceEntityID ?? effect.sourceID,
    targetEntityID: effect.targetEntityID ?? effect.targetID,
  };
}

export function damageLabel(effect: WorldFeedbackEffect): string {
  if (typeof effect.amount === 'number') {
    return `-${effect.amount}`;
  }
  if (typeof effect.hullAmount === 'number' || typeof effect.shieldAmount === 'number') {
    return `-${(effect.hullAmount ?? 0) + (effect.shieldAmount ?? 0)}`;
  }
  return 'HIT';
}

export function pickupLabel(effect: WorldFeedbackEffect): string {
  const itemID = effect.itemID ?? 'item';
  return `+${effect.quantity ?? 0} ${itemID}`;
}

export function labelForEntity(entity: EntityPayload): string {
  if (isSelfEntity(entity)) {
    return 'YOU';
  }
  const label = entity.display?.label?.trim();
  if (!label) {
    return '';
  }
  const detail = labelDetailForEntity(entity);
  return detail ? `${label.toUpperCase()}\n${detail}` : label.toUpperCase();
}

export function memoryMarkerLabel(marker: WorldMapMemoryMarker): string {
  const detail = marker.state ? marker.state.toUpperCase() : 'KNOWN';
  return `${marker.label.toUpperCase()}\n${detail} MEMORY`;
}

function labelDetailForEntity(entity: EntityPayload): string {
  if (entity.entity_type === 'npc') {
    return 'HOSTILE';
  }
  if (entity.entity_type === 'loot') {
    return 'DROP';
  }
  if (entity.entity_type === 'planet_signal') {
    return isUnknownSignal(entity) ? 'UNKNOWN SIGNAL' : (entity.display?.disposition ?? 'SIGNAL').toUpperCase();
  }
  return entity.display?.disposition?.toUpperCase() ?? '';
}

export function labelColorForEntity(entity: EntityPayload): number {
  if (isSelfEntity(entity)) {
    return hudColors.cyan;
  }
  if (entity.entity_type === 'npc') {
    return hudColors.redSoft;
  }
  if (entity.entity_type === 'loot' || isUnknownSignal(entity)) {
    return worldAssetForEntity(entity).accentColor;
  }
  if (entity.display?.disposition === 'friendly') {
    return worldAssetForEntity(entity).accentColor;
  }
  return worldAssetForEntity(entity, isSelfEntity(entity)).accentColor;
}

export function labelOffsetForEntity(entity: EntityPayload): { x: number; y: number; anchorX: number; anchorY: number } {
  if (isSelfEntity(entity)) {
    return { x: 0, y: -44, anchorX: 0.5, anchorY: 0.5 };
  }
  switch (entity.entity_type) {
    case 'player':
      return { x: -40, y: -12, anchorX: 1, anchorY: 0.5 };
    case 'npc':
      return { x: 34, y: -12, anchorX: 0, anchorY: 0.5 };
    case 'loot':
      return { x: 32, y: 18, anchorX: 0, anchorY: 0.5 };
    case 'planet_signal':
      return { x: 34, y: 18, anchorX: 0, anchorY: 0.5 };
    default:
      return { x: 0, y: -36, anchorX: 0.5, anchorY: 0.5 };
  }
}

export function markerHitRadius(entity: EntityPayload): number {
  switch (entity.entity_type) {
    case 'npc':
    case 'loot':
    case 'planet_signal':
      return 34;
    default:
      return 28;
  }
}

export function isUnknownSignal(entity: EntityPayload): boolean {
  return isUnknownSignalAsset(entity);
}

export function drawDiamond(view: Graphics, radius: number, color: number, fillAlpha: number, strokeAlpha: number): void {
  if (fillAlpha > 0) {
    view.moveTo(0, -radius).lineTo(radius, 0).lineTo(0, radius).lineTo(-radius, 0).closePath().fill({ color, alpha: fillAlpha });
  }
  view.moveTo(0, -radius).lineTo(radius, 0).lineTo(0, radius).lineTo(-radius, 0).closePath().stroke({
    color,
    width: 2,
    alpha: strokeAlpha,
  });
}

export function drawAsteroidShard(view: Graphics, x: number, y: number, radius: number, accent: number, alpha: number): void {
  view
    .moveTo(x - radius, y - 2)
    .lineTo(x - radius * 0.38, y - radius)
    .lineTo(x + radius * 0.72, y - radius * 0.42)
    .lineTo(x + radius, y + radius * 0.35)
    .lineTo(x + radius * 0.08, y + radius)
    .lineTo(x - radius * 0.78, y + radius * 0.45)
    .closePath()
    .fill({ color: 0x9c8f80, alpha: 0.22 * alpha })
    .stroke({ color: hudColors.line, width: 1, alpha: 0.32 * alpha });
  view.circle(x, y, Math.max(1.3, radius * 0.22)).fill({ color: accent, alpha: 0.62 * alpha });
}

export function drawIsometricCrate(view: Graphics, color: number): void {
  view
    .moveTo(0, -19)
    .lineTo(18, -9)
    .lineTo(18, 12)
    .lineTo(0, 22)
    .lineTo(-18, 12)
    .lineTo(-18, -9)
    .closePath()
    .fill({ color: hudColors.panel, alpha: 0.72 })
    .stroke({ color, width: 2, alpha: 0.82 });
  view
    .moveTo(0, -19)
    .lineTo(0, 1)
    .lineTo(18, -9)
    .moveTo(0, 1)
    .lineTo(-18, -9)
    .moveTo(0, 1)
    .lineTo(0, 22)
    .stroke({ color: hudColors.line, width: 1, alpha: 0.42 });
  view.rect(-7, -6, 14, 8).stroke({ color, width: 1, alpha: 0.5 });
}

export function lerp(from: number, to: number, amount: number): number {
  return from + (to - from) * amount;
}

export function lerpVec(from: Vec2, to: Vec2, amount: number): Vec2 {
  return {
    x: lerp(from.x, to.x, amount),
    y: lerp(from.y, to.y, amount),
  };
}

export function snapClose(current: Vec2, target: Vec2): Vec2 {
  const dx = current.x - target.x;
  const dy = current.y - target.y;
  if (dx * dx + dy * dy < 0.25) {
    return { ...target };
  }
  return current;
}

export function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

export function isFiniteVec(value: Vec2): boolean {
  return Number.isFinite(value.x) && Number.isFinite(value.y);
}

export function isOddTile(value: number): boolean {
  return Math.abs(value % 2) === 1;
}

export function wrap(value: number, size: number): number {
  return ((value % size) + size) % size;
}

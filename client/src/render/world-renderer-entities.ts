import { Graphics, Sprite, Text } from 'pixi.js';

import { EntityPayload, Vec2 } from '../protocol/envelope';
import {
  clearHUDInputSuppression,
  pointerTargetOwnsUI,
  releaseTransientHUDControlFocus,
  worldCanvasInputBlocked,
} from '../input/world-input-authority';
import { isSelfEntity } from '../state/movement';
import { WorldMapMemoryMarker } from '../state/types';
import { WorldViewState } from './world-view';
import { worldAssetForEntity, worldAssetForMemoryMarker } from './world-renderer-assets';
import { WorldRendererEffects } from './world-renderer-effects';
import { spriteAlphaForEntity, spriteScaleForEntity } from './world-renderer-sprites';
import {
  clamp,
  drawAsteroidShard,
  drawDiamond,
  drawIsometricCrate,
  hudColors,
  isUnknownSignal,
  labelColorForEntity,
  labelForEntity,
  labelOffsetForEntity,
  lerp,
  markerHitRadius,
  memoryMarkerLabel,
  npcSwarmOffsets,
  snapClose,
} from './world-renderer-types';

export abstract class WorldRendererEntities extends WorldRendererEffects {
  protected bindInput(): void {
    if (!this.app) {
      return;
    }

    window.addEventListener(
      'pointerdown',
      (event) => {
        if (event.target === this.app?.canvas) {
          if (this.shouldIgnoreWorldClick()) {
            this.ignoreWorldInputUntil = Math.max(this.ignoreWorldInputUntil, performance.now() + 80);
            releaseTransientHUDControlFocus(document.activeElement);
            return;
          }
          this.ignoreWorldInputUntil = 0;
          clearHUDInputSuppression();
          return;
        }
        if (pointerTargetOwnsUI(event.target)) {
          this.ignoreWorldInputUntil = performance.now() + 220;
        }
      },
      true,
    );

    this.app.canvas.addEventListener('click', (event) => {
      if (event.target !== this.app?.canvas || this.shouldIgnoreWorldClick()) {
        return;
      }
      const rect = this.app?.canvas.getBoundingClientRect();
      if (!rect) {
        return;
      }
      const screen = {
        x: event.clientX - rect.left,
        y: event.clientY - rect.top,
      };
      const clickedEntity = this.findEntityAtScreen(screen);
      if (clickedEntity) {
        this.handlers.onSelectTarget(clickedEntity.entity_id);
        return;
      }
      const clickedMemoryMarker = this.findMemoryMarkerAtScreen(screen);
      if (clickedMemoryMarker) {
        this.handlers.onSelectMemoryMarker(clickedMemoryMarker);
        return;
      }
      this.handlers.onMoveIntent(this.screenToWorld(screen));
    });
  }

  protected shouldIgnoreWorldClick(): boolean {
    if (performance.now() < this.ignoreWorldInputUntil) {
      return true;
    }
    return worldCanvasInputBlocked(document.activeElement);
  }

  protected createEntityView(entity: EntityPayload): Graphics {
    const view = new Graphics();
    const asset = worldAssetForEntity(entity, isSelfEntity(entity));
    view.label = `${entity.entity_id}:${asset.key}`;
    return view;
  }

  protected createEntitySprite(entity: EntityPayload): Sprite | null {
    const asset = worldAssetForEntity(entity, isSelfEntity(entity));
    const texture = this.worldAssetTextures.get(asset.key);
    if (!texture) {
      return null;
    }
    const sprite = new Sprite(texture);
    sprite.label = `${asset.key}:sprite:${entity.entity_id}`;
    sprite.anchor.set(0.5);
    sprite.tint = asset.accentColor;
    sprite.alpha = spriteAlphaForEntity(entity);
    sprite.scale.set(spriteScaleForEntity(entity, this.scale));
    return sprite;
  }

  protected createEntityLabel(entity: EntityPayload): Text {
    const label = new Text({
      text: labelForEntity(entity),
      style: {
        fontFamily: 'IBM Plex Mono, Aptos Mono, monospace',
        fontSize: 11,
        fontWeight: '700',
        fill: labelColorForEntity(entity),
        stroke: { color: '#050709', width: 3 },
        align: 'left',
        lineHeight: 14,
      },
      anchor: 0.5,
    });
    return label;
  }

  protected drawEntity(view: Graphics, entity: EntityPayload, selected: boolean, self: boolean): void {
    view.clear();
    const asset = worldAssetForEntity(entity, self);
    view.label = `${entity.entity_id}:${asset.key}`;
    this.updateEntitySprite(entity, self);
    const hasSpriteBody = this.hasLoadedEntitySpriteBody(entity, self);

    if (selected) {
      this.drawSelectedReticle(view, entity);
    }

    switch (entity.entity_type) {
      case 'player':
        if (!hasSpriteBody) {
          this.drawPlayerShip(view, self, asset.accentColor, asset.glowColor);
        }
        break;
      case 'npc':
        if (!hasSpriteBody) {
          this.drawNpcSwarm(view, entity, asset.accentColor, asset.glowColor);
        }
        this.drawCombatBars(view, entity);
        break;
      case 'loot':
        if (!hasSpriteBody) {
          this.drawLootCache(view, asset.accentColor);
        }
        break;
      case 'planet_signal':
        this.drawPlanetSignal(view, entity, asset.accentColor, asset.glowColor);
        break;
    }
  }

  protected hasLoadedEntitySpriteBody(entity: EntityPayload, self: boolean): boolean {
    const asset = worldAssetForEntity(entity, self);
    return Boolean(this.entitySprites.get(entity.entity_id) && this.worldAssetTextures.get(asset.key));
  }

  protected updateEntitySprite(entity: EntityPayload, self: boolean): void {
    const sprite = this.entitySprites.get(entity.entity_id);
    if (!sprite) {
      return;
    }
    const asset = worldAssetForEntity(entity, self);
    const texture = this.worldAssetTextures.get(asset.key);
    if (texture && sprite.texture !== texture) {
      sprite.texture = texture;
    }
    sprite.label = `${asset.key}:sprite:${entity.entity_id}`;
    sprite.tint = asset.accentColor;
    sprite.alpha = spriteAlphaForEntity(entity);
    sprite.scale.set(spriteScaleForEntity(entity, this.scale));
  }

  protected drawSelectedReticle(view: Graphics, entity: EntityPayload): void {
    const asset = worldAssetForEntity(entity, isSelfEntity(entity));
    const lockColor = asset.glowColor;
    view.circle(0, 0, 31).stroke({ color: lockColor, width: 1, alpha: 0.46 });
    view.circle(0, 0, 42).stroke({ color: lockColor, width: 1, alpha: 0.16 });
    view.moveTo(0, -42).lineTo(0, -28).moveTo(0, 28).lineTo(0, 42).stroke({ color: lockColor, width: 2, alpha: 0.88 });
    view.moveTo(-42, 0).lineTo(-28, 0).moveTo(28, 0).lineTo(42, 0).stroke({ color: lockColor, width: 2, alpha: 0.88 });
    view
      .moveTo(-30, -18)
      .lineTo(-30, -30)
      .lineTo(-18, -30)
      .moveTo(18, -30)
      .lineTo(30, -30)
      .lineTo(30, -18)
      .moveTo(30, 18)
      .lineTo(30, 30)
      .lineTo(18, 30)
      .moveTo(-18, 30)
      .lineTo(-30, 30)
      .lineTo(-30, 18)
      .stroke({ color: lockColor, width: 2, alpha: 0.82 });
  }

  protected drawPlayerShip(view: Graphics, self: boolean, accent: number, glow: number): void {
    if (self) {
      view.circle(0, 0, 52).stroke({ color: accent, width: 1, alpha: 0.28 });
      view.circle(0, 0, 76).stroke({ color: accent, width: 1, alpha: 0.14 });
      view.moveTo(-92, 0).lineTo(-58, 0).moveTo(58, 0).lineTo(92, 0).stroke({ color: accent, width: 1, alpha: 0.12 });
      view.moveTo(0, -92).lineTo(0, -58).moveTo(0, 58).lineTo(0, 92).stroke({ color: accent, width: 1, alpha: 0.12 });
    }

    view.circle(0, 22, 9).fill({ color: accent, alpha: 0.12 });
    view.circle(-5, 24, 3).fill({ color: accent, alpha: 0.82 });
    view.circle(5, 24, 3).fill({ color: accent, alpha: 0.82 });
    view.moveTo(0, -25).lineTo(16, 18).lineTo(4, 10).lineTo(0, 26).lineTo(-4, 10).lineTo(-16, 18).closePath().fill(hudColors.line);
    view.moveTo(0, -25).lineTo(16, 18).lineTo(4, 10).lineTo(0, 26).lineTo(-4, 10).lineTo(-16, 18).closePath().stroke({
      color: glow,
      width: 1,
      alpha: 0.82,
    });
    view.moveTo(0, -14).lineTo(6, 12).lineTo(0, 7).lineTo(-6, 12).closePath().fill({ color: hudColors.panel, alpha: 0.62 });
    view.moveTo(0, -35).lineTo(0, -48).stroke({ color: accent, width: 2, alpha: 0.85 });
  }

  protected drawNpcSwarm(view: Graphics, entity: EntityPayload, accent: number, glow: number): void {
    view.circle(0, 0, 28).stroke({ color: accent, width: 1, alpha: 0.18 });
    for (const [index, rock] of npcSwarmOffsets.entries()) {
      const alpha = index === npcSwarmOffsets.length - 1 ? 0.82 : 0.54;
      drawAsteroidShard(view, rock.x, rock.y, rock.r, accent, alpha);
    }

    drawDiamond(view, 17, accent, 0.16, 0.92);
    view.moveTo(-25, 0).lineTo(-10, 0).moveTo(10, 0).lineTo(25, 0).stroke({ color: glow, width: 2, alpha: 0.76 });
    view.moveTo(0, -25).lineTo(0, -10).moveTo(0, 10).lineTo(0, 25).stroke({ color: glow, width: 2, alpha: 0.76 });
    if (entity.status_flags?.includes('damaged')) {
      view.circle(0, 0, 34).stroke({ color: hudColors.amber, width: 1, alpha: 0.22 });
    }
  }

  protected drawLootCache(view: Graphics, accent: number): void {
    view.circle(0, 0, 29).stroke({ color: accent, width: 1, alpha: 0.18 });
    drawDiamond(view, 24, accent, 0.08, 0.48);
    drawIsometricCrate(view, accent);
    view.moveTo(-16, 22).lineTo(16, 22).stroke({ color: accent, width: 1, alpha: 0.22 });
    view.circle(0, 33, 3).fill({ color: accent, alpha: 0.78 });
  }

  protected drawPlanetSignal(view: Graphics, entity: EntityPayload, accent: number, glow: number): void {
    if (isUnknownSignal(entity)) {
      view.circle(0, 0, 27).stroke({ color: accent, width: 1, alpha: 0.18 });
      view
        .moveTo(-20, -4)
        .lineTo(-18, -14)
        .moveTo(-14, -20)
        .lineTo(-4, -22)
        .moveTo(4, -22)
        .lineTo(14, -18)
        .moveTo(20, -10)
        .lineTo(22, 0)
        .moveTo(18, 14)
        .lineTo(10, 21)
        .moveTo(-10, 21)
        .lineTo(-18, 14)
        .stroke({ color: accent, width: 2, alpha: 0.72 });
      view
        .moveTo(-7, -8)
        .lineTo(-1, -14)
        .lineTo(8, -10)
        .lineTo(9, -2)
        .lineTo(1, 5)
        .lineTo(1, 10)
        .stroke({ color: accent, width: 3, alpha: 0.86 });
      view.circle(1, 17, 2.5).fill({ color: accent, alpha: 0.92 });
      return;
    }

    view.circle(0, 0, 23).fill({ color: 0x17252a, alpha: 0.92 });
    view.circle(-7, -7, 9).fill({ color: hudColors.line, alpha: 0.16 });
    view.circle(4, 4, 18).stroke({ color: accent, width: 1, alpha: 0.52 });
    view.moveTo(-27, 4).lineTo(27, -8).stroke({ color: hudColors.muted, width: 2, alpha: 0.46 });
    drawDiamond(view, 31, accent, 0, 0.58);
  }


  protected renderMemoryMarkers(state: WorldViewState): void {
    const activeIDs = new Set(state.memoryMarkers.map((marker) => marker.id));
    for (const [markerID, view] of this.memoryMarkerViews) {
      if (!activeIDs.has(markerID)) {
        view.destroy();
        this.memoryMarkerViews.delete(markerID);
        this.memoryMarkerLabels.get(markerID)?.destroy();
        this.memoryMarkerLabels.delete(markerID);
        this.memoryMarkerTargets.delete(markerID);
      }
    }

    for (const marker of state.memoryMarkers) {
      this.memoryMarkerTargets.set(marker.id, marker);
      let view = this.memoryMarkerViews.get(marker.id);
      if (!view) {
        view = new Graphics();
        view.label = marker.id;
        this.memoryMarkerViews.set(marker.id, view);
        this.memoryMarkerLayer.addChild(view);
      }
      let label = this.memoryMarkerLabels.get(marker.id);
      if (!label) {
        label = this.createMemoryMarkerLabel(marker);
        this.memoryMarkerLabels.set(marker.id, label);
        this.memoryMarkerLayer.addChild(label);
      }

      label.text = memoryMarkerLabel(marker);
      this.drawMemoryPlanetMarker(view, marker);
      this.positionMemoryMarker(marker.id);
    }
  }

  protected createMemoryMarkerLabel(marker: WorldMapMemoryMarker): Text {
    return new Text({
      text: memoryMarkerLabel(marker),
      style: {
        fontFamily: 'IBM Plex Mono, Aptos Mono, monospace',
        fontSize: 11,
        fontWeight: '700',
        fill: hudColors.green,
        stroke: { color: '#050709', width: 3 },
        lineHeight: 14,
      },
      anchor: { x: 0, y: 0.5 },
    });
  }

  protected drawMemoryPlanetMarker(view: Graphics, marker: WorldMapMemoryMarker): void {
    const asset = worldAssetForMemoryMarker(marker);
    view.clear();
    view.label = `${marker.id}:${asset.key}`;
    view.circle(0, 0, 33).stroke({ color: asset.accentColor, width: 1, alpha: 0.24 });
    view.circle(0, 0, 47).stroke({ color: asset.glowColor, width: 1, alpha: 0.12 });
    view.circle(0, 0, 21).fill({ color: 0x101b19, alpha: 0.92 });
    view.circle(-7, -8, 9).fill({ color: asset.accentColor, alpha: 0.2 });
    view.circle(7, 7, 11).fill({ color: asset.glowColor, alpha: 0.12 });
    view.moveTo(-28, 5).lineTo(28, -7).stroke({ color: hudColors.line, width: 2, alpha: 0.42 });
    drawDiamond(view, 34, asset.accentColor, 0, 0.68);
    view
      .moveTo(-43, 0)
      .lineTo(-30, 0)
      .moveTo(30, 0)
      .lineTo(43, 0)
      .moveTo(0, -43)
      .lineTo(0, -30)
      .moveTo(0, 30)
      .lineTo(0, 43)
      .stroke({ color: asset.accentColor, width: 1, alpha: 0.5 });
    if (marker.state === 'owned') {
      view.circle(0, 0, 5).fill({ color: asset.accentColor, alpha: 0.9 });
    }
  }


  protected updateInterpolatedEntities(): void {
    for (const [entityID, entity] of this.entityTargets) {
      const view = this.entityViews.get(entityID);
      if (!view) {
        continue;
      }
      const label = this.entityLabels.get(entityID);

      this.entityWorldPositions.set(entityID, this.nextDisplayPosition(entity));
      this.positionEntityView(entityID, view);
      if (label) {
        this.positionEntityLabel(entityID, label);
      }
    }

    const local = this.state?.entities.find(isSelfEntity) ?? this.state?.entities.find((entity) => entity.entity_type === 'player');
    if (local) {
      this.center = this.entityWorldPositions.get(local.entity_id) ?? this.authoritativeDisplayPosition(local);
    }
  }

  protected updateMemoryMarkerPositions(): void {
    for (const markerID of this.memoryMarkerTargets.keys()) {
      this.positionMemoryMarker(markerID);
    }
  }

  protected drawCombatBars(view: Graphics, entity: EntityPayload): void {
    if (!entity.combat || entity.combat.max_hp <= 0) {
      return;
    }
    const width = 36;
    const hpRatio = clamp(entity.combat.hp / entity.combat.max_hp, 0, 1);
    view.rect(-width / 2, 25, width, 4).fill({ color: 0x21050a, alpha: 0.86 });
    view.rect(-width / 2, 25, width * hpRatio, 4).fill({ color: 0xff5c7a, alpha: 0.92 });
    if (entity.combat.max_shield > 0) {
      const shieldRatio = clamp(entity.combat.shield / entity.combat.max_shield, 0, 1);
      view.rect(-width / 2, 20, width, 3).fill({ color: 0x061821, alpha: 0.78 });
      view.rect(-width / 2, 20, width * shieldRatio, 3).fill({ color: 0x2bdfff, alpha: 0.9 });
    }
  }


  protected positionEntityView(entityID: string, view: Graphics): void {
    const world = this.entityWorldPositions.get(entityID) ?? this.entityTargets.get(entityID)?.position;
    if (!world) {
      return;
    }
    const screen = this.worldToScreen(world);
    view.position.set(screen.x, screen.y);
    const sprite = this.entitySprites.get(entityID);
    if (sprite) {
      sprite.position.set(screen.x, screen.y);
    }
  }

  protected positionEntityLabel(entityID: string, label: Text): void {
    const world = this.entityWorldPositions.get(entityID) ?? this.entityTargets.get(entityID)?.position;
    const entity = this.entityTargets.get(entityID);
    if (!world) {
      return;
    }
    const screen = this.worldToScreen(world);
    const offset = entity ? labelOffsetForEntity(entity) : { x: 0, y: -34, anchorX: 0.5, anchorY: 0.5 };
    label.anchor.set(offset.anchorX, offset.anchorY);
    label.position.set(screen.x + offset.x, screen.y + offset.y);
  }

  protected positionMemoryMarker(markerID: string): void {
    const marker = this.memoryMarkerTargets.get(markerID);
    const view = this.memoryMarkerViews.get(markerID);
    if (!marker || !view) {
      return;
    }
    const screen = this.worldToScreen(marker.position);
    view.position.set(screen.x, screen.y);
    const label = this.memoryMarkerLabels.get(markerID);
    if (label) {
      label.position.set(screen.x + 38, screen.y + 16);
    }
  }

  protected findEntityAtScreen(screen: Vec2): EntityPayload | null {
    if (!this.state) {
      return null;
    }

    return (
      this.state.entities.find((entity) => {
        const entityWorld = this.entityWorldPositions.get(entity.entity_id) ?? this.authoritativeDisplayPosition(entity);
        const entityScreen = this.worldToScreen(entityWorld);
        const dx = entityScreen.x - screen.x;
        const dy = entityScreen.y - screen.y;
        const radius = markerHitRadius(entity);
        return dx * dx + dy * dy <= radius * radius;
      }) ?? null
    );
  }

  protected findMemoryMarkerAtScreen(screen: Vec2): WorldMapMemoryMarker | null {
    if (!this.state) {
      return null;
    }

    return (
      this.state.memoryMarkers.find((marker) => {
        const markerScreen = this.worldToScreen(marker.position);
        const dx = markerScreen.x - screen.x;
        const dy = markerScreen.y - screen.y;
        return dx * dx + dy * dy <= 42 * 42;
      }) ?? null
    );
  }


  protected nextDisplayPosition(entity: EntityPayload): Vec2 {
    const authoritative = this.authoritativeDisplayPosition(entity);
    if (entity.movement?.moving || isSelfEntity(entity)) {
      return authoritative;
    }

    const current = this.entityWorldPositions.get(entity.entity_id) ?? authoritative;
    return snapClose(
      {
        x: lerp(current.x, authoritative.x, 0.16),
        y: lerp(current.y, authoritative.y, 0.16),
      },
      authoritative,
    );
  }

}

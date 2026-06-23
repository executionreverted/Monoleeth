import { Graphics, Text } from 'pixi.js';

import { Vec2 } from '../protocol/envelope';
import { isSelfEntity } from '../state/movement';
import { WorldFeedbackEffect } from '../state/types';
import { emptyMapOverlayDebug, MapOverlayFrameDebug, MapOverlayPortalDebug, MapOverlaySafeZoneDebug, summarizeMapOverlay } from './map-overlay';
import { WorldViewState } from './world-view';
import { WORLD_RENDER_ASSETS, worldAssetForPortal, worldAssetForSafeZone } from './world-renderer-assets';
import { WorldRendererStarfield } from './world-renderer-starfield';
import {
  clamp,
  damageLabel,
  hudColors,
  lerpVec,
  pickupLabel,
  PROJECTILE_TRAVEL_MS,
} from './world-renderer-types';

export abstract class WorldRendererEffects extends WorldRendererStarfield {
  protected drawMapOverlay(state: WorldViewState): void {
    this.mapOverlayLayer.clear();
    if (!this.app) {
      this.mapOverlayDebug = emptyMapOverlayDebug();
      return;
    }

    const overlay = summarizeMapOverlay(
      { currentMap: state.currentMap, minimap: state.minimap },
      {
        center: this.center,
        screen: { width: this.app.screen.width, height: this.app.screen.height },
        scale: this.scale,
      },
    );
    this.mapOverlayDebug = overlay;
    if (!overlay.active || !overlay.bounds) {
      return;
    }

    this.drawMapBoundsFrame(overlay.bounds);
    for (const safeZone of overlay.safeZones) {
      this.drawSafeZoneHint(safeZone);
    }
    for (const portal of overlay.portalMarkers) {
      this.drawPortalMarker(portal);
    }
  }

  protected drawMapBoundsFrame(bounds: MapOverlayFrameDebug): void {
    if (bounds.width <= 0 || bounds.height <= 0) {
      return;
    }

    const corner = clamp(Math.min(bounds.width, bounds.height) * 0.08, 24, 72);
    this.mapOverlayLayer
      .rect(bounds.topLeft.x, bounds.topLeft.y, bounds.width, bounds.height)
      .fill({ color: hudColors.cyan, alpha: 0.012 })
      .stroke({ color: hudColors.cyan, width: 2, alpha: 0.24 });
    this.mapOverlayLayer
      .moveTo(bounds.topLeft.x, bounds.topLeft.y + corner)
      .lineTo(bounds.topLeft.x, bounds.topLeft.y)
      .lineTo(bounds.topLeft.x + corner, bounds.topLeft.y)
      .moveTo(bounds.topRight.x - corner, bounds.topRight.y)
      .lineTo(bounds.topRight.x, bounds.topRight.y)
      .lineTo(bounds.topRight.x, bounds.topRight.y + corner)
      .moveTo(bounds.bottomRight.x, bounds.bottomRight.y - corner)
      .lineTo(bounds.bottomRight.x, bounds.bottomRight.y)
      .lineTo(bounds.bottomRight.x - corner, bounds.bottomRight.y)
      .moveTo(bounds.bottomLeft.x + corner, bounds.bottomLeft.y)
      .lineTo(bounds.bottomLeft.x, bounds.bottomLeft.y)
      .lineTo(bounds.bottomLeft.x, bounds.bottomLeft.y - corner)
      .stroke({ color: hudColors.cyanSoft, width: 3, alpha: 0.42 });
  }

  protected drawPortalMarker(portal: MapOverlayPortalDebug): void {
    const asset = worldAssetForPortal(portal);
    const radius = clamp(portal.screenRadius, 10, 30);
    const x = portal.screen.x;
    const y = portal.screen.y;
    this.mapOverlayLayer.circle(x, y, radius + 10).stroke({ color: asset.glowColor, width: 1, alpha: 0.18 });
    this.mapOverlayLayer
      .moveTo(x, y - radius)
      .lineTo(x + radius, y)
      .lineTo(x, y + radius)
      .lineTo(x - radius, y)
      .closePath()
      .fill({ color: hudColors.panel, alpha: 0.58 })
      .stroke({ color: asset.accentColor, width: 2, alpha: 0.76 });
    this.mapOverlayLayer
      .moveTo(x - radius - 7, y)
      .lineTo(x - radius + 3, y)
      .moveTo(x + radius - 3, y)
      .lineTo(x + radius + 7, y)
      .moveTo(x, y - radius - 7)
      .lineTo(x, y - radius + 3)
      .moveTo(x, y + radius - 3)
      .lineTo(x, y + radius + 7)
      .stroke({ color: asset.glowColor, width: 1, alpha: 0.58 });
  }

  protected drawSafeZoneHint(zone: MapOverlaySafeZoneDebug): void {
    const asset = worldAssetForSafeZone(zone);
    const radius = clamp(zone.screenRadius, 12, 1600);
    const color = asset.accentColor;
    this.mapOverlayLayer
      .circle(zone.screen.x, zone.screen.y, radius)
      .fill({ color, alpha: zone.blocksPVP ? 0.026 : 0.018 })
      .stroke({ color, width: 2, alpha: zone.blocksPVP ? 0.28 : 0.22 });
    this.mapOverlayLayer.circle(zone.screen.x, zone.screen.y, clamp(radius * 0.075, 4, 12)).fill({ color, alpha: 0.34 });
  }

  protected drawMarkers(state: WorldViewState): void {
    const staleMarkers = this.markerLayer.removeChildren();
    for (const marker of staleMarkers) {
      marker.destroy();
    }

    if (state.movementTarget) {
      const asset = WORLD_RENDER_ASSETS['marker.movement.target'];
      const target = this.worldToScreen(state.movementTarget);
      const marker = new Graphics();
      marker.label = asset.key;
      marker.circle(0, 0, 16).stroke({ color: asset.accentColor, width: 2, alpha: 0.7 });
      marker.moveTo(-22, 0).lineTo(-8, 0).moveTo(8, 0).lineTo(22, 0).stroke({ color: asset.accentColor, width: 2 });
      marker.moveTo(0, -22).lineTo(0, -8).moveTo(0, 8).lineTo(0, 22).stroke({ color: asset.accentColor, width: 2 });
      marker.position.set(target.x, target.y);
      this.markerLayer.addChild(marker);
    }

    this.drawFeedbackEffects(state);
  }

  protected drawScanWaves(state: WorldViewState): void {
    this.scanLayer.clear();
    this.scanDebug = { active: false, screen: null, rings: [] };
    if (!state.scanMode.enabled) {
      return;
    }

    const local = state.entities.find(isSelfEntity) ?? state.entities.find((entity) => entity.entity_type === 'player');
    if (!local) {
      return;
    }

    const world = this.entityWorldPositions.get(local.entity_id) ?? this.authoritativeDisplayPosition(local);
    const screen = this.worldToScreen(world);
    const phase = (performance.now() % 2400) / 2400;
    const color = state.scanMode.lastError ? hudColors.amber : hudColors.cyan;
    const rings: Array<{ radius: number; alpha: number }> = [];

    this.scanLayer.circle(screen.x, screen.y, 43).stroke({ color, width: 1, alpha: 0.2 });
    for (let index = 0; index < 3; index += 1) {
      const progress = (phase + index / 3) % 1;
      const radius = 54 + progress * 138 * this.scale;
      const alpha = (1 - progress) * 0.32;
      rings.push({ radius, alpha });
      this.scanLayer.circle(screen.x, screen.y, radius).stroke({ color, width: 2, alpha });
      this.scanLayer.circle(screen.x, screen.y, radius + 7).stroke({ color: hudColors.cyanSoft, width: 1, alpha: alpha * 0.32 });
    }
    const sweep = phase * Math.PI * 2;
    this.scanLayer
      .moveTo(screen.x, screen.y)
      .lineTo(screen.x + Math.cos(sweep) * 104 * this.scale, screen.y + Math.sin(sweep) * 104 * this.scale)
      .stroke({ color: hudColors.cyanSoft, width: 2, alpha: 0.34 });
    this.scanDebug = { active: true, screen, rings };
  }


  protected drawFeedbackEffects(state: WorldViewState): void {
    const now = Date.now();
    for (const effect of state.worldEffects) {
      if (effect.expiresAt <= now) {
        continue;
      }
      switch (effect.kind) {
        case 'laser':
          this.drawLaserEffect(effect, now);
          break;
        case 'damage':
          this.drawFloatingText(effect, now, damageLabel(effect), 0xff8c9c);
          break;
        case 'miss':
          this.drawFloatingText(effect, now, 'MISS', 0xf4c95d);
          break;
        case 'destroyed':
          this.drawBurstEffect(effect, now, 0xff5c7a);
          break;
        case 'loot_spawn':
          this.drawBurstEffect(effect, now, 0xf4c95d);
          this.drawFloatingText(effect, now, 'DROP', 0xf4c95d);
          break;
        case 'loot_pickup':
          this.drawFloatingText(effect, now, pickupLabel(effect), 0x7cff9b);
          break;
      }
    }
  }

  protected drawLaserEffect(effect: WorldFeedbackEffect, now: number): void {
    const asset = WORLD_RENDER_ASSETS['projectile.laser.basic'];
    const target = this.effectScreenPosition(effect);
    if (!target) {
      return;
    }
    const source = this.effectSourceScreenPosition(effect);
    const alpha = this.effectAlpha(effect, now);
    const progress = this.projectileProgress(effect, now);
    const marker = new Graphics();
    marker.label = asset.key;
    if (source) {
      const head = lerpVec(source, target, progress);
      const tail = lerpVec(source, target, Math.max(0, progress - 0.2));
      marker
        .moveTo(source.x, source.y)
        .lineTo(target.x, target.y)
        .stroke({ color: asset.glowColor, width: 1, alpha: 0.12 * alpha })
        .circle(source.x, source.y, 6 + (1 - progress) * 5)
        .stroke({ color: asset.glowColor, width: 1, alpha: 0.42 * alpha })
        .moveTo(tail.x, tail.y)
        .lineTo(head.x, head.y)
        .stroke({ color: asset.glowColor, width: 7, alpha: 0.22 * alpha })
        .moveTo(tail.x, tail.y)
        .lineTo(head.x, head.y)
        .stroke({ color: asset.accentColor, width: 2, alpha: 0.92 * alpha })
        .circle(head.x, head.y, 5)
        .fill({ color: 0xfff0a8, alpha: 0.95 * alpha })
        .circle(head.x, head.y, 11)
        .stroke({ color: 0x2bdfff, width: 2, alpha: 0.55 * alpha });
    }
    const flashAlpha = alpha * (source ? clamp((progress - 0.72) / 0.28, 0, 1) : 1);
    if (flashAlpha > 0) {
      if (source) {
        marker.moveTo(source.x, source.y).lineTo(target.x, target.y).stroke({ color: 0xf4c95d, width: 1, alpha: 0.16 * flashAlpha });
      }
      marker.circle(target.x, target.y, 18 + (1 - flashAlpha) * 10).stroke({ color: 0xf4c95d, width: 2, alpha: 0.78 * flashAlpha });
      marker
        .moveTo(target.x - 8, target.y)
        .lineTo(target.x + 8, target.y)
        .moveTo(target.x, target.y - 8)
        .lineTo(target.x, target.y + 8)
        .stroke({
          color: 0xfff0a8,
          width: 2,
          alpha: 0.86 * flashAlpha,
        });
    }
    this.markerLayer.addChild(marker);
  }

  protected drawBurstEffect(effect: WorldFeedbackEffect, now: number, color: number): void {
    const target = this.effectScreenPosition(effect);
    if (!target) {
      return;
    }
    const progress = this.effectProgress(effect, now);
    const alpha = this.effectAlpha(effect, now);
    const radius = 18 + progress * 30;
    const marker = new Graphics();
    marker.circle(target.x, target.y, radius).stroke({ color, width: 2, alpha: 0.64 * alpha });
    marker
      .moveTo(target.x - radius, target.y)
      .lineTo(target.x - radius * 0.45, target.y)
      .moveTo(target.x + radius * 0.45, target.y)
      .lineTo(target.x + radius, target.y)
      .moveTo(target.x, target.y - radius)
      .lineTo(target.x, target.y - radius * 0.45)
      .moveTo(target.x, target.y + radius * 0.45)
      .lineTo(target.x, target.y + radius)
      .stroke({ color, width: 2, alpha: 0.48 * alpha });
    this.markerLayer.addChild(marker);
  }

  protected drawFloatingText(effect: WorldFeedbackEffect, now: number, text: string, color: number): void {
    const target = this.effectScreenPosition(effect);
    if (!target) {
      return;
    }
    const progress = this.effectProgress(effect, now);
    const label = new Text({
      text,
      style: {
        fontFamily: 'IBM Plex Mono, Aptos Mono, monospace',
        fontSize: 14,
        fontWeight: '700',
        fill: color,
        stroke: { color: '#050709', width: 4 },
      },
      anchor: 0.5,
    });
    label.alpha = this.effectAlpha(effect, now);
    label.position.set(target.x, target.y - 34 - progress * 26);
    this.markerLayer.addChild(label);
  }

  protected effectScreenPosition(effect: WorldFeedbackEffect): Vec2 | null {
    if (effect.targetID) {
      const world = this.entityWorldPositions.get(effect.targetID) ?? this.entityTargets.get(effect.targetID)?.position;
      if (world) {
        return this.worldToScreen(world);
      }
    }
    return effect.position ? this.worldToScreen(effect.position) : null;
  }

  protected effectSourceScreenPosition(effect: WorldFeedbackEffect): Vec2 | null {
    if (effect.sourceID) {
      const world = this.entityWorldPositions.get(effect.sourceID) ?? this.entityTargets.get(effect.sourceID)?.position;
      if (world) {
        return this.worldToScreen(world);
      }
    }
    return effect.sourcePosition ? this.worldToScreen(effect.sourcePosition) : null;
  }

  protected projectileDebugSnapshot(
    effect: WorldFeedbackEffect,
    now: number,
  ): Array<{ id: string; source: Vec2; target: Vec2; head: Vec2; progress: number; active: boolean; alpha: number }> {
    if (effect.kind !== 'laser' || effect.expiresAt <= now) {
      return [];
    }
    const source = this.effectSourceScreenPosition(effect);
    const target = this.effectScreenPosition(effect);
    if (!source || !target) {
      return [];
    }
    const progress = this.projectileProgress(effect, now);
    return [
      {
        id: effect.id,
        source,
        target,
        head: lerpVec(source, target, progress),
        progress,
        active: progress < 1,
        alpha: this.effectAlpha(effect, now),
      },
    ];
  }

  protected effectProgress(effect: WorldFeedbackEffect, now: number): number {
    return clamp((now - effect.createdAt) / Math.max(1, effect.expiresAt - effect.createdAt), 0, 1);
  }

  protected projectileProgress(effect: WorldFeedbackEffect, now: number): number {
    return clamp((now - effect.createdAt) / PROJECTILE_TRAVEL_MS, 0, 1);
  }

  protected effectAlpha(effect: WorldFeedbackEffect, now: number): number {
    return clamp(1 - this.effectProgress(effect, now), 0, 1);
  }

}

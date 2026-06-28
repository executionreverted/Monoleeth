import { Application, Assets, Text, Texture } from 'pixi.js';

import { Vec2 } from '../protocol/envelope';
import { isSelfEntity, serverClockOffset } from '../state/movement';
import { cloneMapOverlayDebug, MapOverlayDebugState } from './map-overlay';
import { WorldInputHandlers, WorldViewState } from './world-view';
import type { EntityAssetDirectionCode } from './world-entity-asset-catalog';
import { WORLD_RENDER_ASSETS, worldTextureKeyForAsset } from './world-renderer-assets';
import { WorldRendererEntities } from './world-renderer-entities';
import { labelColorForEntity, labelForEntity, StarfieldDebugState } from './world-renderer-types';

export class WorldRenderer extends WorldRendererEntities {
  constructor(handlers: WorldInputHandlers) {
    super(handlers);
  }

  async mount(container: HTMLElement): Promise<void> {
    const app = new Application();
    await app.init({
      resizeTo: container,
      background: '#050709',
      antialias: true,
      autoDensity: true,
      resolution: Math.min(window.devicePixelRatio || 1, 2),
    });

    this.app = app;
    app.canvas.className = 'world-canvas';
    container.appendChild(app.canvas);
    app.stage.addChild(this.backgroundLayer);
    this.backgroundLayer.addChild(this.starfieldLayer);
    this.backgroundLayer.addChild(this.nebulaLayer);
    this.backgroundLayer.addChild(this.gridLayer);
    app.stage.addChild(this.mapOverlayLayer);
    app.stage.addChild(this.mapOverlaySpriteLayer);
    app.stage.addChild(this.scanLayer);
    app.stage.addChild(this.worldLayer);
    app.stage.addChild(this.memoryMarkerLayer);
    app.stage.addChild(this.markerLayer);

    await this.loadWorldAssetTextures();
    this.starfieldTexture = this.worldAssetTextures.get('background.starfield') ?? (await Assets.load<Texture>(WORLD_RENDER_ASSETS['background.starfield'].assetURL));
    this.createStarfield();
    this.emptyLabel = new Text({
      text: 'AWAITING SERVER SNAPSHOT',
      style: {
        fontFamily: 'IBM Plex Mono, Aptos Mono, monospace',
        fontSize: 13,
        fill: 0x8af5ff,
        stroke: { color: '#050709', width: 4 },
      },
      anchor: 0.5,
    });
    this.emptyLabel.alpha = 0.72;
    app.stage.addChild(this.emptyLabel);
    this.bindInput();

    app.ticker.add((ticker) => {
      const pulse = 0.92 + Math.sin(performance.now() / 650) * 0.08;
      for (const star of this.stars) {
        star.view.alpha = 0.32 + star.depth * 0.28 + pulse * 0.16;
      }
      this.updateInterpolatedEntities();
      this.updateMemoryMarkerPositions();
      this.updateBackground();
      if (this.state) {
        this.drawMapOverlay(this.state);
        this.drawScanWaves(this.state);
        this.drawMarkers(this.state);
      }
    });
  }

  render(state: WorldViewState): void {
    if (!this.app) {
      return;
    }

    this.state = state;
    if (state.lastServerTime !== null && state.lastServerTime !== this.serverClockTime) {
      this.serverClockOffset = serverClockOffset(performance.now(), state.lastServerTime);
      this.serverClockTime = state.lastServerTime;
    }
    if (this.emptyLabel) {
      this.emptyLabel.visible = state.entities.length === 0;
      this.emptyLabel.position.set(this.app.screen.width / 2, this.app.screen.height / 2);
    }
    const local = state.entities.find(isSelfEntity) ?? state.entities.find((entity) => entity.entity_type === 'player');
    this.center = local ? this.authoritativeDisplayPosition(local) : this.center;
    this.scale = this.app.screen.width < 700 ? 0.78 : 1;
    this.updateBackground();
    this.drawMapOverlay(state);

    const activeIDs = new Set(state.entities.map((entity) => entity.entity_id));
    for (const [entityID, view] of this.entityViews) {
      if (!activeIDs.has(entityID)) {
        view.destroy();
        this.entityViews.delete(entityID);
        this.entitySprites.get(entityID)?.destroy();
        this.entitySprites.delete(entityID);
        this.entitySpriteDirections.delete(entityID);
        this.entityLabels.get(entityID)?.destroy();
        this.entityLabels.delete(entityID);
        this.entityTargets.delete(entityID);
        this.entityWorldPositions.delete(entityID);
      }
    }

    for (const entity of state.entities) {
      this.entityTargets.set(entity.entity_id, entity);
      let view = this.entityViews.get(entity.entity_id);
      if (!view) {
        view = this.createEntityView(entity);
        this.entityViews.set(entity.entity_id, view);
        this.entityWorldPositions.set(entity.entity_id, { ...entity.position });
        const sprite = this.createEntitySprite(entity);
        if (sprite) {
          this.entitySprites.set(entity.entity_id, sprite);
          this.worldLayer.addChild(sprite);
        }
        this.worldLayer.addChild(view);
      }
      let label = this.entityLabels.get(entity.entity_id);
      if (!label) {
        label = this.createEntityLabel(entity);
        this.entityLabels.set(entity.entity_id, label);
        this.worldLayer.addChild(label);
      }

      label.text = labelForEntity(entity);
      label.style.fill = labelColorForEntity(entity);
      label.visible = label.text !== '';
      this.drawEntity(view, entity, state.selectedTargetID === entity.entity_id, isSelfEntity(entity));
      this.entityWorldPositions.set(entity.entity_id, this.nextDisplayPosition(entity));
      this.positionEntityView(entity.entity_id, view);
      this.positionEntityLabel(entity.entity_id, label);
    }

    this.drawMarkers(state);
    this.drawScanWaves(state);
    this.renderMemoryMarkers(state);
  }

  destroy(): void {
    this.app?.destroy(true);
    this.app = null;
  }

  private async loadWorldAssetTextures(): Promise<void> {
    const descriptors = Object.values(WORLD_RENDER_ASSETS);
    await Promise.all(
      descriptors.map(async (descriptor) => {
        const texture = await Assets.load<Texture>(descriptor.assetURL);
        this.worldAssetTextures.set(descriptor.key, texture);
        await Promise.all(
          Object.entries(descriptor.directionURLs ?? {}).map(async ([direction, url]) => {
            const directionalTexture = await Assets.load<Texture>(url);
            this.worldAssetTextures.set(worldTextureKeyForAsset(descriptor, direction as EntityAssetDirectionCode), directionalTexture);
          }),
        );
      }),
    );
  }

  debugSnapshot(): {
    center: Vec2;
    displayPositions: Record<string, Vec2>;
    memoryMarkers: Array<{ id: string; detailID: string; label: string; position: Vec2; screen: Vec2; state: string }>;
    scanWaves: { active: boolean; screen: Vec2 | null; rings: Array<{ radius: number; alpha: number }> };
    projectiles: Array<{ id: string; source: Vec2; target: Vec2; head: Vec2; progress: number; active: boolean; alpha: number }>;
    renderedAssets: {
      loadedTextures: number;
      entitySprites: Array<{ entityID: string; assetKey: string; visible: boolean }>;
      overlaySprites: Array<{ assetKey: string; visible: boolean }>;
    };
    background: StarfieldDebugState;
    mapOverlay: MapOverlayDebugState;
  } {
    const displayPositions: Record<string, Vec2> = {};
    for (const [entityID, position] of this.entityWorldPositions) {
      displayPositions[entityID] = { ...position };
    }
    const now = Date.now();
    return {
      center: { ...this.center },
      displayPositions,
      memoryMarkers: [...this.memoryMarkerTargets.values()].map((marker) => ({
        id: marker.id,
        detailID: marker.detailID,
        label: marker.label,
        position: { ...marker.position },
        screen: this.worldToScreen(marker.position),
        state: marker.state,
      })),
      scanWaves: {
        active: this.scanDebug.active,
        screen: this.scanDebug.screen ? { ...this.scanDebug.screen } : null,
        rings: this.scanDebug.rings.map((ring) => ({ ...ring })),
      },
      projectiles: (this.state?.worldEffects ?? []).flatMap((effect) => this.projectileDebugSnapshot(effect, now)),
      renderedAssets: {
        loadedTextures: this.worldAssetTextures.size,
        entitySprites: [...this.entitySprites.entries()].map(([entityID, sprite]) => ({
          entityID,
          assetKey: String(sprite.label ?? '').split(':')[0] ?? '',
          visible: sprite.visible && sprite.alpha > 0,
        })),
        overlaySprites: this.mapOverlaySpriteLayer.children.map((child) => ({
          assetKey: String(child.label ?? '').split(':')[0] ?? '',
          visible: child.visible && child.alpha > 0,
        })),
      },
      background: {
        assetLoaded: this.starfieldDebug.assetLoaded,
        tileCount: this.starfieldDebug.tileCount,
        mirroredTiles: this.starfieldDebug.mirroredTiles,
        farOffset: { ...this.starfieldDebug.farOffset },
        midOffset: { ...this.starfieldDebug.midOffset },
        sampleTiles: this.starfieldDebug.sampleTiles.map((tile) => ({ ...tile, screen: { ...tile.screen } })),
      },
      mapOverlay: cloneMapOverlayDebug(this.mapOverlayDebug),
    };
  }

}

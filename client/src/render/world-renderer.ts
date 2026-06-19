import { Application, Assets, Container, Graphics, Sprite, Text, Texture } from 'pixi.js';

import { EntityPayload, Vec2 } from '../protocol/envelope';
import { currentEntityPosition, estimateServerTime, isSelfEntity, serverClockOffset } from '../state/movement';
import { WorldFeedbackEffect, WorldMapMemoryMarker } from '../state/types';
import { WorldInputHandlers, WorldViewState } from './world-view';
import starfieldURL from '../assets/starfield_2048x1152.png?url';

const entityColors: Record<EntityPayload['entity_type'], number> = {
  player: 0x2bdfff,
  npc: 0xff4236,
  loot: 0xf4c95d,
  planet_signal: 0xf4c95d,
};

const hudColors = {
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

const npcSwarmOffsets = [
  { x: -20, y: -10, r: 7 },
  { x: -10, y: 13, r: 5 },
  { x: 16, y: -16, r: 6 },
  { x: 21, y: 9, r: 5 },
  { x: 3, y: -2, r: 8 },
];
const PROJECTILE_TRAVEL_MS = 260;
const STARFIELD_TILE = { width: 2048, height: 1152 };
const STARFIELD_GRID_RADIUS = 3;
const starfieldLayerConfigs = [
  { id: 'far', depth: 0.018, scale: 1.02, alpha: 0.88 },
  { id: 'mid', depth: 0.052, scale: 0.68, alpha: 0.22 },
] as const;

type StarfieldLayerID = (typeof starfieldLayerConfigs)[number]['id'];

interface StarfieldTile {
  sprite: Sprite;
  layerID: StarfieldLayerID;
  column: number;
  row: number;
}

interface StarfieldDebugState {
  assetLoaded: boolean;
  tileCount: number;
  mirroredTiles: number;
  farOffset: Vec2;
  midOffset: Vec2;
  sampleTiles: Array<{ layer: StarfieldLayerID; mirrorX: boolean; mirrorY: boolean; screen: Vec2 }>;
}

interface FogDebugState {
  active: boolean;
  revealCenter: Vec2 | null;
  revealRadius: number;
  rememberedPockets: number;
  overlayAlpha: number;
}

export class WorldRenderer {
  private app: Application | null = null;
  private readonly backgroundLayer = new Container();
  private readonly starfieldLayer = new Container();
  private readonly fogLayer = new Graphics();
  private readonly scanLayer = new Graphics();
  private readonly worldLayer = new Container();
  private readonly memoryMarkerLayer = new Container();
  private readonly markerLayer = new Container();
  private readonly nebulaLayer = new Graphics();
  private readonly gridLayer = new Graphics();
  private readonly entityViews = new Map<string, Graphics>();
  private readonly entityLabels = new Map<string, Text>();
  private readonly entityTargets = new Map<string, EntityPayload>();
  private readonly entityWorldPositions = new Map<string, Vec2>();
  private readonly memoryMarkerViews = new Map<string, Graphics>();
  private readonly memoryMarkerLabels = new Map<string, Text>();
  private readonly memoryMarkerTargets = new Map<string, WorldMapMemoryMarker>();
  private readonly starfieldTiles: StarfieldTile[] = [];
  private readonly stars: Array<{ view: Graphics; base: Vec2; depth: number }> = [];
  private starfieldTexture: Texture | null = null;
  private starfieldDebug: StarfieldDebugState = {
    assetLoaded: false,
    tileCount: 0,
    mirroredTiles: 0,
    farOffset: { x: 0, y: 0 },
    midOffset: { x: 0, y: 0 },
    sampleTiles: [],
  };
  private emptyLabel: Text | null = null;
  private state: WorldViewState | null = null;
  private center: Vec2 = { x: 0, y: 0 };
  private scale = 1;
  private serverClockOffset: number | null = null;
  private serverClockTime: number | null = null;
  private ignoreWorldInputUntil = 0;
  private scanDebug: {
    active: boolean;
    screen: Vec2 | null;
    rings: Array<{ radius: number; alpha: number }>;
  } = { active: false, screen: null, rings: [] };
  private fogDebug: FogDebugState = { active: false, revealCenter: null, revealRadius: 0, rememberedPockets: 0, overlayAlpha: 0 };

  constructor(private readonly handlers: WorldInputHandlers) {}

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
    app.stage.addChild(this.fogLayer);
    app.stage.addChild(this.scanLayer);
    app.stage.addChild(this.worldLayer);
    app.stage.addChild(this.memoryMarkerLayer);
    app.stage.addChild(this.markerLayer);

    this.starfieldTexture = await Assets.load<Texture>(starfieldURL);
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
        this.drawFog(this.state);
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
    this.drawFog(state);

    const activeIDs = new Set(state.entities.map((entity) => entity.entity_id));
    for (const [entityID, view] of this.entityViews) {
      if (!activeIDs.has(entityID)) {
        view.destroy();
        this.entityViews.delete(entityID);
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

  debugSnapshot(): {
    center: Vec2;
    displayPositions: Record<string, Vec2>;
    memoryMarkers: Array<{ id: string; detailID: string; label: string; position: Vec2; screen: Vec2; state: string }>;
    scanWaves: { active: boolean; screen: Vec2 | null; rings: Array<{ radius: number; alpha: number }> };
    projectiles: Array<{ id: string; source: Vec2; target: Vec2; head: Vec2; progress: number; active: boolean; alpha: number }>;
    background: StarfieldDebugState;
    fog: FogDebugState;
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
      background: {
        assetLoaded: this.starfieldDebug.assetLoaded,
        tileCount: this.starfieldDebug.tileCount,
        mirroredTiles: this.starfieldDebug.mirroredTiles,
        farOffset: { ...this.starfieldDebug.farOffset },
        midOffset: { ...this.starfieldDebug.midOffset },
        sampleTiles: this.starfieldDebug.sampleTiles.map((tile) => ({ ...tile, screen: { ...tile.screen } })),
      },
      fog: {
        active: this.fogDebug.active,
        revealCenter: this.fogDebug.revealCenter ? { ...this.fogDebug.revealCenter } : null,
        revealRadius: this.fogDebug.revealRadius,
        rememberedPockets: this.fogDebug.rememberedPockets,
        overlayAlpha: this.fogDebug.overlayAlpha,
      },
    };
  }

  private createStarfield(): void {
    if (!this.app || !this.starfieldTexture) {
      return;
    }

    const width = Math.max(this.app.screen.width, 1600);
    const height = Math.max(this.app.screen.height, 1000);
    for (const tile of this.starfieldTiles) {
      tile.sprite.destroy();
    }
    this.starfieldTiles.length = 0;
    this.starfieldLayer.removeChildren();
    for (const layer of starfieldLayerConfigs) {
      for (let row = -STARFIELD_GRID_RADIUS; row <= STARFIELD_GRID_RADIUS; row += 1) {
        for (let column = -STARFIELD_GRID_RADIUS; column <= STARFIELD_GRID_RADIUS; column += 1) {
          const sprite = new Sprite(this.starfieldTexture);
          sprite.alpha = layer.alpha;
          sprite.label = `starfield-${layer.id}-${column}-${row}`;
          this.starfieldTiles.push({ sprite, layerID: layer.id, column, row });
          this.starfieldLayer.addChild(sprite);
        }
      }
    }

    this.nebulaLayer.clear();
    for (let index = 0; index < 160; index += 1) {
      const star = new Graphics();
      const radius = index % 19 === 0 ? 1.55 : index % 5 === 0 ? 1 : 0.55;
      const color = index % 7 === 0 ? 0xf4c95d : index % 5 === 0 ? 0x7cff9b : 0xd7f7ff;
      star.circle(0, 0, radius).fill(color);
      const base = { x: (index * 97) % width, y: (index * 53) % height };
      const depth = index % 11 === 0 ? 0.88 : index % 3 === 0 ? 0.58 : 0.32;
      star.position.set(base.x, base.y);
      star.alpha = 0.12 + depth * 0.16;
      this.stars.push({ view: star, base, depth });
      this.backgroundLayer.addChild(star);
    }
    this.updateBackground();
  }

  private bindInput(): void {
    if (!this.app) {
      return;
    }

    window.addEventListener(
      'pointerdown',
      (event) => {
        if (event.target === this.app?.canvas) {
          if (this.shouldIgnoreWorldClick()) {
            this.ignoreWorldInputUntil = Math.max(this.ignoreWorldInputUntil, performance.now() + 80);
            return;
          }
          this.ignoreWorldInputUntil = 0;
          (window as Window & { __SPACE_MORPG_HUD_INPUT_UNTIL__?: number }).__SPACE_MORPG_HUD_INPUT_UNTIL__ = 0;
          return;
        }
        if (blocksWorldCanvasInput(event.target)) {
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

  private shouldIgnoreWorldClick(): boolean {
    const hudBlockUntil = (window as Window & { __SPACE_MORPG_HUD_INPUT_UNTIL__?: number }).__SPACE_MORPG_HUD_INPUT_UNTIL__ ?? 0;
    if (performance.now() < Math.max(this.ignoreWorldInputUntil, hudBlockUntil)) {
      return true;
    }
    return document.activeElement instanceof HTMLElement && Boolean(document.activeElement.closest('input, textarea, select, [role="dialog"]'));
  }

  private createEntityView(entity: EntityPayload): Graphics {
    const view = new Graphics();
    view.label = entity.entity_id;
    return view;
  }

  private createEntityLabel(entity: EntityPayload): Text {
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

  private drawEntity(view: Graphics, entity: EntityPayload, selected: boolean, self: boolean): void {
    view.clear();

    if (selected) {
      this.drawSelectedReticle(view, entity);
    }

    switch (entity.entity_type) {
      case 'player':
        this.drawPlayerShip(view, self);
        break;
      case 'npc':
        this.drawNpcSwarm(view, entity);
        this.drawCombatBars(view, entity);
        break;
      case 'loot':
        this.drawLootCache(view);
        break;
      case 'planet_signal':
        this.drawPlanetSignal(view, entity);
        break;
    }
  }

  private drawSelectedReticle(view: Graphics, entity: EntityPayload): void {
    const lockColor = entity.entity_type === 'npc' ? hudColors.redSoft : entity.entity_type === 'loot' ? hudColors.amber : hudColors.cyanSoft;
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

  private drawPlayerShip(view: Graphics, self: boolean): void {
    if (self) {
      view.circle(0, 0, 52).stroke({ color: hudColors.cyan, width: 1, alpha: 0.28 });
      view.circle(0, 0, 76).stroke({ color: hudColors.cyan, width: 1, alpha: 0.14 });
      view.moveTo(-92, 0).lineTo(-58, 0).moveTo(58, 0).lineTo(92, 0).stroke({ color: hudColors.cyan, width: 1, alpha: 0.12 });
      view.moveTo(0, -92).lineTo(0, -58).moveTo(0, 58).lineTo(0, 92).stroke({ color: hudColors.cyan, width: 1, alpha: 0.12 });
    }

    view.circle(0, 22, 9).fill({ color: hudColors.cyan, alpha: 0.12 });
    view.circle(-5, 24, 3).fill({ color: hudColors.cyan, alpha: 0.82 });
    view.circle(5, 24, 3).fill({ color: hudColors.cyan, alpha: 0.82 });
    view.moveTo(0, -25).lineTo(16, 18).lineTo(4, 10).lineTo(0, 26).lineTo(-4, 10).lineTo(-16, 18).closePath().fill(hudColors.line);
    view.moveTo(0, -25).lineTo(16, 18).lineTo(4, 10).lineTo(0, 26).lineTo(-4, 10).lineTo(-16, 18).closePath().stroke({
      color: self ? hudColors.cyanSoft : hudColors.green,
      width: 1,
      alpha: 0.82,
    });
    view.moveTo(0, -14).lineTo(6, 12).lineTo(0, 7).lineTo(-6, 12).closePath().fill({ color: hudColors.panel, alpha: 0.62 });
    view.moveTo(0, -35).lineTo(0, -48).stroke({ color: self ? hudColors.cyan : hudColors.green, width: 2, alpha: 0.85 });
  }

  private drawNpcSwarm(view: Graphics, entity: EntityPayload): void {
    view.circle(0, 0, 28).stroke({ color: hudColors.red, width: 1, alpha: 0.18 });
    for (const [index, rock] of npcSwarmOffsets.entries()) {
      const alpha = index === npcSwarmOffsets.length - 1 ? 0.82 : 0.54;
      drawAsteroidShard(view, rock.x, rock.y, rock.r, hudColors.red, alpha);
    }

    drawDiamond(view, 17, hudColors.red, 0.16, 0.92);
    view.moveTo(-25, 0).lineTo(-10, 0).moveTo(10, 0).lineTo(25, 0).stroke({ color: 0xffd9df, width: 2, alpha: 0.76 });
    view.moveTo(0, -25).lineTo(0, -10).moveTo(0, 10).lineTo(0, 25).stroke({ color: 0xffd9df, width: 2, alpha: 0.76 });
    if (entity.status_flags?.includes('damaged')) {
      view.circle(0, 0, 34).stroke({ color: hudColors.amber, width: 1, alpha: 0.22 });
    }
  }

  private drawLootCache(view: Graphics): void {
    view.circle(0, 0, 29).stroke({ color: hudColors.amber, width: 1, alpha: 0.18 });
    drawDiamond(view, 24, hudColors.amber, 0.08, 0.48);
    drawIsometricCrate(view, hudColors.amber);
    view.moveTo(-16, 22).lineTo(16, 22).stroke({ color: hudColors.amber, width: 1, alpha: 0.22 });
    view.circle(0, 33, 3).fill({ color: hudColors.amber, alpha: 0.78 });
  }

  private drawPlanetSignal(view: Graphics, entity: EntityPayload): void {
    if (isUnknownSignal(entity)) {
      view.circle(0, 0, 27).stroke({ color: hudColors.amber, width: 1, alpha: 0.18 });
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
        .stroke({ color: hudColors.amber, width: 2, alpha: 0.72 });
      view
        .moveTo(-7, -8)
        .lineTo(-1, -14)
        .lineTo(8, -10)
        .lineTo(9, -2)
        .lineTo(1, 5)
        .lineTo(1, 10)
        .stroke({ color: hudColors.amber, width: 3, alpha: 0.86 });
      view.circle(1, 17, 2.5).fill({ color: hudColors.amber, alpha: 0.92 });
      return;
    }

    view.circle(0, 0, 23).fill({ color: 0x17252a, alpha: 0.92 });
    view.circle(-7, -7, 9).fill({ color: hudColors.line, alpha: 0.16 });
    view.circle(4, 4, 18).stroke({ color: hudColors.cyan, width: 1, alpha: 0.52 });
    view.moveTo(-27, 4).lineTo(27, -8).stroke({ color: hudColors.muted, width: 2, alpha: 0.46 });
    drawDiamond(view, 31, hudColors.cyan, 0, 0.58);
  }

  private drawFog(state: WorldViewState): void {
    if (!this.app || !state.minimap) {
      this.fogLayer.clear();
      this.fogDebug = { active: false, revealCenter: null, revealRadius: 0, rememberedPockets: 0, overlayAlpha: 0 };
      return;
    }

    const local = state.entities.find(isSelfEntity) ?? state.entities.find((entity) => entity.entity_type === 'player');
    if (!local) {
      this.fogLayer.clear();
      this.fogDebug = { active: false, revealCenter: null, revealRadius: 0, rememberedPockets: 0, overlayAlpha: 0 };
      return;
    }

    const revealWorld = this.entityWorldPositions.get(local.entity_id) ?? this.authoritativeDisplayPosition(local);
    const revealCenter = this.worldToScreen(revealWorld);
    const revealRadius = clamp(state.minimap.radar_range * this.scale, 128, Math.max(this.app.screen.width, this.app.screen.height) * 0.72);
    const overlayAlpha = 0.58;
    const padding = revealRadius + 260;
    const pockets = this.fogMemoryPockets(state);

    this.fogLayer.clear();
    this.fogLayer.rect(-padding, -padding, this.app.screen.width + padding * 2, this.app.screen.height + padding * 2).fill({
      color: 0x010406,
      alpha: overlayAlpha,
    });
    this.fogLayer.circle(revealCenter.x, revealCenter.y, revealRadius).cut();
    for (const pocket of pockets) {
      this.fogLayer.circle(pocket.screen.x, pocket.screen.y, pocket.radius).cut();
    }

    this.fogLayer.circle(revealCenter.x, revealCenter.y, revealRadius + 18).stroke({ color: hudColors.cyan, width: 2, alpha: 0.15 });
    this.fogLayer.circle(revealCenter.x, revealCenter.y, revealRadius + 62).stroke({ color: 0x071219, width: 46, alpha: 0.22 });
    for (const pocket of pockets) {
      const color = pocket.freshness === 'stale' ? hudColors.amber : hudColors.green;
      this.fogLayer.circle(pocket.screen.x, pocket.screen.y, pocket.radius + 10).stroke({ color, width: 1, alpha: 0.2 });
      this.fogLayer.circle(pocket.screen.x, pocket.screen.y, pocket.radius + 32).stroke({ color, width: 18, alpha: 0.06 });
    }

    this.fogDebug = {
      active: true,
      revealCenter,
      revealRadius,
      rememberedPockets: pockets.length,
      overlayAlpha,
    };
  }

  private fogMemoryPockets(state: WorldViewState): Array<{ screen: Vec2; radius: number; freshness: string }> {
    if (!this.app) {
      return [];
    }
    const seen = new Set<string>();
    const pockets: Array<{ screen: Vec2; radius: number; freshness: string }> = [];
    const add = (position: Vec2, freshness: string): void => {
      if (!isFiniteVec(position)) {
        return;
      }
      const key = `${Math.round(position.x)}:${Math.round(position.y)}`;
      if (seen.has(key)) {
        return;
      }
      seen.add(key);
      const screen = this.worldToScreen(position);
      const margin = 190;
      if (screen.x < -margin || screen.y < -margin || screen.x > this.app!.screen.width + margin || screen.y > this.app!.screen.height + margin) {
        return;
      }
      pockets.push({
        screen,
        radius: clamp((state.minimap?.radar_range ?? 360) * this.scale * 0.18, 58, 126),
        freshness,
      });
    };

    for (const memory of state.minimap?.remembered ?? []) {
      add(memory.position, memory.freshness);
    }
    for (const marker of state.memoryMarkers) {
      add(marker.position, marker.state);
    }
    return pockets.slice(0, 12);
  }

  private renderMemoryMarkers(state: WorldViewState): void {
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

  private createMemoryMarkerLabel(marker: WorldMapMemoryMarker): Text {
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

  private drawMemoryPlanetMarker(view: Graphics, marker: WorldMapMemoryMarker): void {
    view.clear();
    view.circle(0, 0, 33).stroke({ color: hudColors.green, width: 1, alpha: 0.24 });
    view.circle(0, 0, 47).stroke({ color: hudColors.cyan, width: 1, alpha: 0.12 });
    view.circle(0, 0, 21).fill({ color: 0x101b19, alpha: 0.92 });
    view.circle(-7, -8, 9).fill({ color: hudColors.green, alpha: 0.2 });
    view.circle(7, 7, 11).fill({ color: hudColors.cyanSoft, alpha: 0.12 });
    view.moveTo(-28, 5).lineTo(28, -7).stroke({ color: hudColors.line, width: 2, alpha: 0.42 });
    drawDiamond(view, 34, hudColors.green, 0, 0.68);
    view
      .moveTo(-43, 0)
      .lineTo(-30, 0)
      .moveTo(30, 0)
      .lineTo(43, 0)
      .moveTo(0, -43)
      .lineTo(0, -30)
      .moveTo(0, 30)
      .lineTo(0, 43)
      .stroke({ color: hudColors.green, width: 1, alpha: 0.5 });
    if (marker.state === 'owned') {
      view.circle(0, 0, 5).fill({ color: hudColors.green, alpha: 0.9 });
    }
  }

  private drawMarkers(state: WorldViewState): void {
    const staleMarkers = this.markerLayer.removeChildren();
    for (const marker of staleMarkers) {
      marker.destroy();
    }

    if (state.movementTarget) {
      const target = this.worldToScreen(state.movementTarget);
      const marker = new Graphics();
      marker.circle(0, 0, 16).stroke({ color: 0xf4c95d, width: 2, alpha: 0.7 });
      marker.moveTo(-22, 0).lineTo(-8, 0).moveTo(8, 0).lineTo(22, 0).stroke({ color: 0xf4c95d, width: 2 });
      marker.moveTo(0, -22).lineTo(0, -8).moveTo(0, 8).lineTo(0, 22).stroke({ color: 0xf4c95d, width: 2 });
      marker.position.set(target.x, target.y);
      this.markerLayer.addChild(marker);
    }

    this.drawFeedbackEffects(state);
  }

  private drawScanWaves(state: WorldViewState): void {
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

  private updateBackground(): void {
    if (!this.app) {
      return;
    }

    const width = this.app.screen.width;
    const height = this.app.screen.height;
    const gridSize = width < 700 ? 52 : 96;
    const offsetX = wrap(-this.center.x * 0.18 * this.scale, gridSize);
    const offsetY = wrap(-this.center.y * 0.18 * this.scale, gridSize);

    this.updateStarfieldTiles(width, height);
    this.gridLayer.clear();
    for (let x = offsetX - gridSize; x <= width + gridSize; x += gridSize) {
      this.gridLayer.moveTo(x, 0).lineTo(x, height).stroke({ color: 0x2bdfff, width: 1, alpha: 0.1 });
    }
    for (let y = offsetY - gridSize; y <= height + gridSize; y += gridSize) {
      this.gridLayer.moveTo(0, y).lineTo(width, y).stroke({ color: 0x2bdfff, width: 1, alpha: 0.1 });
    }

    this.nebulaLayer.position.set(0, 0);
    for (const star of this.stars) {
      const parallaxX = star.base.x - this.center.x * star.depth * 0.055;
      const parallaxY = star.base.y - this.center.y * star.depth * 0.055;
      star.view.position.set(wrap(parallaxX, width + 80) - 40, wrap(parallaxY, height + 80) - 40);
    }
  }

  private updateStarfieldTiles(width: number, height: number): void {
    if (!this.starfieldTexture) {
      this.starfieldDebug = {
        assetLoaded: false,
        tileCount: 0,
        mirroredTiles: 0,
        farOffset: { x: 0, y: 0 },
        midOffset: { x: 0, y: 0 },
        sampleTiles: [],
      };
      return;
    }

    const sampleTiles: StarfieldDebugState['sampleTiles'] = [];
    let mirroredTiles = 0;
    const offsets: Record<StarfieldLayerID, Vec2> = {
      far: { x: 0, y: 0 },
      mid: { x: 0, y: 0 },
    };
    for (const layer of starfieldLayerConfigs) {
      const tileWidth = STARFIELD_TILE.width * layer.scale;
      const tileHeight = STARFIELD_TILE.height * layer.scale;
      const parallaxX = this.center.x * layer.depth * this.scale;
      const parallaxY = this.center.y * layer.depth * this.scale;
      const originTileX = Math.floor((parallaxX - width / 2) / tileWidth) - STARFIELD_GRID_RADIUS;
      const originTileY = Math.floor((parallaxY - height / 2) / tileHeight) - STARFIELD_GRID_RADIUS;
      offsets[layer.id] = { x: parallaxX, y: parallaxY };

      for (const tile of this.starfieldTiles.filter((entry) => entry.layerID === layer.id)) {
        const tileX = originTileX + tile.column + STARFIELD_GRID_RADIUS;
        const tileY = originTileY + tile.row + STARFIELD_GRID_RADIUS;
        const mirrorX = isOddTile(tileX);
        const mirrorY = isOddTile(tileY);
        if (mirrorX || mirrorY) {
          mirroredTiles += 1;
        }
        const x = width / 2 + tileX * tileWidth - parallaxX;
        const y = height / 2 + tileY * tileHeight - parallaxY;

        tile.sprite.alpha = layer.alpha;
        tile.sprite.scale.set((mirrorX ? -1 : 1) * layer.scale, (mirrorY ? -1 : 1) * layer.scale);
        tile.sprite.position.set(x + (mirrorX ? tileWidth : 0), y + (mirrorY ? tileHeight : 0));

        if (tile.column === 0 && tile.row === 0) {
          sampleTiles.push({ layer: layer.id, mirrorX, mirrorY, screen: { x, y } });
        }
      }
    }

    this.starfieldDebug = {
      assetLoaded: true,
      tileCount: this.starfieldTiles.length,
      mirroredTiles,
      farOffset: offsets.far,
      midOffset: offsets.mid,
      sampleTiles,
    };
  }

  private updateInterpolatedEntities(): void {
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

  private updateMemoryMarkerPositions(): void {
    for (const markerID of this.memoryMarkerTargets.keys()) {
      this.positionMemoryMarker(markerID);
    }
  }

  private drawCombatBars(view: Graphics, entity: EntityPayload): void {
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

  private drawFeedbackEffects(state: WorldViewState): void {
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

  private drawLaserEffect(effect: WorldFeedbackEffect, now: number): void {
    const target = this.effectScreenPosition(effect);
    if (!target) {
      return;
    }
    const source = this.effectSourceScreenPosition(effect);
    const alpha = this.effectAlpha(effect, now);
    const progress = this.projectileProgress(effect, now);
    const marker = new Graphics();
    if (source) {
      const head = lerpVec(source, target, progress);
      const tail = lerpVec(source, target, Math.max(0, progress - 0.2));
      marker
        .moveTo(source.x, source.y)
        .lineTo(target.x, target.y)
        .stroke({ color: 0x2bdfff, width: 1, alpha: 0.12 * alpha })
        .circle(source.x, source.y, 6 + (1 - progress) * 5)
        .stroke({ color: 0x8af5ff, width: 1, alpha: 0.42 * alpha })
        .moveTo(tail.x, tail.y)
        .lineTo(head.x, head.y)
        .stroke({ color: 0x2bdfff, width: 7, alpha: 0.22 * alpha })
        .moveTo(tail.x, tail.y)
        .lineTo(head.x, head.y)
        .stroke({ color: 0xf4c95d, width: 2, alpha: 0.92 * alpha })
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

  private drawBurstEffect(effect: WorldFeedbackEffect, now: number, color: number): void {
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

  private drawFloatingText(effect: WorldFeedbackEffect, now: number, text: string, color: number): void {
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

  private effectScreenPosition(effect: WorldFeedbackEffect): Vec2 | null {
    if (effect.targetID) {
      const world = this.entityWorldPositions.get(effect.targetID) ?? this.entityTargets.get(effect.targetID)?.position;
      if (world) {
        return this.worldToScreen(world);
      }
    }
    return effect.position ? this.worldToScreen(effect.position) : null;
  }

  private effectSourceScreenPosition(effect: WorldFeedbackEffect): Vec2 | null {
    if (effect.sourceID) {
      const world = this.entityWorldPositions.get(effect.sourceID) ?? this.entityTargets.get(effect.sourceID)?.position;
      if (world) {
        return this.worldToScreen(world);
      }
    }
    return effect.sourcePosition ? this.worldToScreen(effect.sourcePosition) : null;
  }

  private projectileDebugSnapshot(
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

  private effectProgress(effect: WorldFeedbackEffect, now: number): number {
    return clamp((now - effect.createdAt) / Math.max(1, effect.expiresAt - effect.createdAt), 0, 1);
  }

  private projectileProgress(effect: WorldFeedbackEffect, now: number): number {
    return clamp((now - effect.createdAt) / PROJECTILE_TRAVEL_MS, 0, 1);
  }

  private effectAlpha(effect: WorldFeedbackEffect, now: number): number {
    return clamp(1 - this.effectProgress(effect, now), 0, 1);
  }

  private positionEntityView(entityID: string, view: Graphics): void {
    const world = this.entityWorldPositions.get(entityID) ?? this.entityTargets.get(entityID)?.position;
    if (!world) {
      return;
    }
    const screen = this.worldToScreen(world);
    view.position.set(screen.x, screen.y);
  }

  private positionEntityLabel(entityID: string, label: Text): void {
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

  private positionMemoryMarker(markerID: string): void {
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

  private findEntityAtScreen(screen: Vec2): EntityPayload | null {
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

  private findMemoryMarkerAtScreen(screen: Vec2): WorldMapMemoryMarker | null {
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

  private worldToScreen(world: Vec2): Vec2 {
    if (!this.app) {
      return world;
    }

    return {
      x: this.app.screen.width / 2 + (world.x - this.center.x) * this.scale,
      y: this.app.screen.height / 2 + (world.y - this.center.y) * this.scale,
    };
  }

  private screenToWorld(screen: Vec2): Vec2 {
    if (!this.app) {
      return screen;
    }

    return {
      x: Math.round(this.center.x + (screen.x - this.app.screen.width / 2) / this.scale),
      y: Math.round(this.center.y + (screen.y - this.app.screen.height / 2) / this.scale),
    };
  }

  private nextDisplayPosition(entity: EntityPayload): Vec2 {
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

  private authoritativeDisplayPosition(entity: EntityPayload): Vec2 {
    return currentEntityPosition(entity, this.estimatedServerTime());
  }

  private estimatedServerTime(): number {
    return this.serverClockOffset === null ? Date.now() : estimateServerTime(performance.now(), this.serverClockOffset);
  }
}

function damageLabel(effect: WorldFeedbackEffect): string {
  if (typeof effect.amount === 'number') {
    return `-${effect.amount}`;
  }
  if (typeof effect.hullAmount === 'number' || typeof effect.shieldAmount === 'number') {
    return `-${(effect.hullAmount ?? 0) + (effect.shieldAmount ?? 0)}`;
  }
  return 'HIT';
}

function pickupLabel(effect: WorldFeedbackEffect): string {
  const itemID = effect.itemID ?? 'item';
  return `+${effect.quantity ?? 0} ${itemID}`;
}

function labelForEntity(entity: EntityPayload): string {
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

function memoryMarkerLabel(marker: WorldMapMemoryMarker): string {
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

function labelColorForEntity(entity: EntityPayload): number {
  if (isSelfEntity(entity)) {
    return hudColors.cyan;
  }
  if (entity.entity_type === 'npc') {
    return hudColors.redSoft;
  }
  if (entity.entity_type === 'loot' || isUnknownSignal(entity)) {
    return hudColors.amber;
  }
  if (entity.display?.disposition === 'friendly') {
    return hudColors.green;
  }
  return entityColors[entity.entity_type];
}

function labelOffsetForEntity(entity: EntityPayload): { x: number; y: number; anchorX: number; anchorY: number } {
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

function markerHitRadius(entity: EntityPayload): number {
  switch (entity.entity_type) {
    case 'npc':
    case 'loot':
    case 'planet_signal':
      return 34;
    default:
      return 28;
  }
}

function isUnknownSignal(entity: EntityPayload): boolean {
  return (
    entity.entity_type === 'planet_signal' &&
    (entity.status_flags?.includes('unknown_signal') || entity.display?.disposition === 'unknown' || /unknown/i.test(entity.display?.label ?? ''))
  );
}

function blocksWorldCanvasInput(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    !target.classList.contains('world-canvas') &&
    Boolean(target.closest('.hud, .auth-panel, .hud-modal, .hud-window, button, input, select, textarea, [role="dialog"]'))
  );
}

function drawDiamond(view: Graphics, radius: number, color: number, fillAlpha: number, strokeAlpha: number): void {
  if (fillAlpha > 0) {
    view.moveTo(0, -radius).lineTo(radius, 0).lineTo(0, radius).lineTo(-radius, 0).closePath().fill({ color, alpha: fillAlpha });
  }
  view.moveTo(0, -radius).lineTo(radius, 0).lineTo(0, radius).lineTo(-radius, 0).closePath().stroke({
    color,
    width: 2,
    alpha: strokeAlpha,
  });
}

function drawAsteroidShard(view: Graphics, x: number, y: number, radius: number, accent: number, alpha: number): void {
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

function drawIsometricCrate(view: Graphics, color: number): void {
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

function lerp(from: number, to: number, amount: number): number {
  return from + (to - from) * amount;
}

function lerpVec(from: Vec2, to: Vec2, amount: number): Vec2 {
  return {
    x: lerp(from.x, to.x, amount),
    y: lerp(from.y, to.y, amount),
  };
}

function snapClose(current: Vec2, target: Vec2): Vec2 {
  const dx = current.x - target.x;
  const dy = current.y - target.y;
  if (dx * dx + dy * dy < 0.25) {
    return { ...target };
  }
  return current;
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function isFiniteVec(value: Vec2): boolean {
  return Number.isFinite(value.x) && Number.isFinite(value.y);
}

function isOddTile(value: number): boolean {
  return Math.abs(value % 2) === 1;
}

function wrap(value: number, size: number): number {
  return ((value % size) + size) % size;
}

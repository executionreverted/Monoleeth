import { Application, Container, Graphics, Text, Texture } from 'pixi.js';

import { EntityPayload, Vec2 } from '../protocol/envelope';
import { currentEntityPosition, estimateServerTime } from '../state/movement';
import { WorldMapMemoryMarker } from '../state/types';
import { WorldInputHandlers, WorldViewState } from './world-view';
import { FogDebugState, StarfieldDebugState, StarfieldTile } from './world-renderer-types';

export abstract class WorldRendererBase {
  protected app: Application | null = null;
  protected readonly backgroundLayer = new Container();
  protected readonly starfieldLayer = new Container();
  protected readonly fogLayer = new Graphics();
  protected readonly scanLayer = new Graphics();
  protected readonly worldLayer = new Container();
  protected readonly memoryMarkerLayer = new Container();
  protected readonly markerLayer = new Container();
  protected readonly nebulaLayer = new Graphics();
  protected readonly gridLayer = new Graphics();
  protected readonly entityViews = new Map<string, Graphics>();
  protected readonly entityLabels = new Map<string, Text>();
  protected readonly entityTargets = new Map<string, EntityPayload>();
  protected readonly entityWorldPositions = new Map<string, Vec2>();
  protected readonly memoryMarkerViews = new Map<string, Graphics>();
  protected readonly memoryMarkerLabels = new Map<string, Text>();
  protected readonly memoryMarkerTargets = new Map<string, WorldMapMemoryMarker>();
  protected readonly starfieldTiles: StarfieldTile[] = [];
  protected readonly stars: Array<{ view: Graphics; base: Vec2; depth: number }> = [];
  protected starfieldTexture: Texture | null = null;
  protected starfieldDebug: StarfieldDebugState = {
    assetLoaded: false,
    tileCount: 0,
    mirroredTiles: 0,
    farOffset: { x: 0, y: 0 },
    midOffset: { x: 0, y: 0 },
    sampleTiles: [],
  };
  protected emptyLabel: Text | null = null;
  protected state: WorldViewState | null = null;
  protected center: Vec2 = { x: 0, y: 0 };
  protected scale = 1;
  protected serverClockOffset: number | null = null;
  protected serverClockTime: number | null = null;
  protected ignoreWorldInputUntil = 0;
  protected scanDebug: {
    active: boolean;
    screen: Vec2 | null;
    rings: Array<{ radius: number; alpha: number }>;
  } = { active: false, screen: null, rings: [] };
  protected fogDebug: FogDebugState = { active: false, revealCenter: null, revealRadius: 0, rememberedPockets: 0, overlayAlpha: 0 };

  protected constructor(protected readonly handlers: WorldInputHandlers) {}

  protected worldToScreen(world: Vec2): Vec2 {
    if (!this.app) {
      return world;
    }

    return {
      x: this.app.screen.width / 2 + (world.x - this.center.x) * this.scale,
      y: this.app.screen.height / 2 + (world.y - this.center.y) * this.scale,
    };
  }

  protected screenToWorld(screen: Vec2): Vec2 {
    if (!this.app) {
      return screen;
    }

    return {
      x: Math.round(this.center.x + (screen.x - this.app.screen.width / 2) / this.scale),
      y: Math.round(this.center.y + (screen.y - this.app.screen.height / 2) / this.scale),
    };
  }


  protected authoritativeDisplayPosition(entity: EntityPayload): Vec2 {
    return currentEntityPosition(entity, this.estimatedServerTime());
  }

  protected estimatedServerTime(): number {
    return this.serverClockOffset === null ? Date.now() : estimateServerTime(performance.now(), this.serverClockOffset);
  }}

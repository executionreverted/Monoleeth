import { Application, Container, Graphics } from 'pixi.js';

import { EntityPayload, Vec2 } from '../protocol/envelope';
import { WorldInputHandlers, WorldViewState } from './world-view';

const entityColors: Record<EntityPayload['entity_type'], number> = {
  player: 0x8af5ff,
  npc_placeholder: 0xff5c7a,
  loot_placeholder: 0xf4c95d,
  planet_signal_placeholder: 0x7cff9b,
};

export class WorldRenderer {
  private app: Application | null = null;
  private readonly worldLayer = new Container();
  private readonly markerLayer = new Container();
  private readonly entityViews = new Map<string, Graphics>();
  private readonly entityTargets = new Map<string, EntityPayload>();
  private readonly entityWorldPositions = new Map<string, Vec2>();
  private readonly stars: Graphics[] = [];
  private state: WorldViewState | null = null;
  private center: Vec2 = { x: 0, y: 0 };
  private scale = 1;

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
    app.stage.addChild(this.worldLayer);
    app.stage.addChild(this.markerLayer);

    this.createStarfield();
    this.bindInput();

    app.ticker.add((ticker) => {
      const pulse = 0.92 + Math.sin(performance.now() / 650) * 0.08;
      for (const star of this.stars) {
        star.alpha = 0.42 + pulse * 0.24;
      }
      this.updateInterpolatedEntities();
      this.markerLayer.rotation += 0.0007 * ticker.deltaTime;
    });
  }

  render(state: WorldViewState): void {
    if (!this.app) {
      return;
    }

    this.state = state;
    const local = state.entities.find((entity) => entity.entity_type === 'player');
    this.center = local?.position ?? this.center;
    this.scale = this.app.screen.width < 700 ? 0.78 : 1;

    const activeIDs = new Set(state.entities.map((entity) => entity.entity_id));
    for (const [entityID, view] of this.entityViews) {
      if (!activeIDs.has(entityID)) {
        view.destroy();
        this.entityViews.delete(entityID);
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

      this.drawEntity(view, entity, state.selectedTargetID === entity.entity_id);
      if (entity.entity_type === 'player') {
        this.entityWorldPositions.set(entity.entity_id, { ...entity.position });
      }
      this.positionEntityView(entity.entity_id, view);
    }

    this.drawMarkers(state);
  }

  destroy(): void {
    this.app?.destroy(true);
    this.app = null;
  }

  private createStarfield(): void {
    if (!this.app) {
      return;
    }

    const width = Math.max(this.app.screen.width, 1280);
    const height = Math.max(this.app.screen.height, 900);
    for (let index = 0; index < 140; index += 1) {
      const star = new Graphics();
      const radius = index % 13 === 0 ? 1.8 : 0.9;
      const color = index % 7 === 0 ? 0xf4c95d : index % 5 === 0 ? 0x7cff9b : 0xd7f7ff;
      star.circle(0, 0, radius).fill(color);
      star.position.set((index * 97) % width, (index * 53) % height);
      star.alpha = 0.35 + (index % 5) * 0.08;
      this.stars.push(star);
      this.app.stage.addChild(star);
    }
  }

  private bindInput(): void {
    if (!this.app) {
      return;
    }

    this.app.canvas.addEventListener('click', (event) => {
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
      this.handlers.onMoveIntent(this.screenToWorld(screen));
    });
  }

  private createEntityView(entity: EntityPayload): Graphics {
    const view = new Graphics();
    view.label = entity.entity_id;
    return view;
  }

  private drawEntity(view: Graphics, entity: EntityPayload, selected: boolean): void {
    const color = entityColors[entity.entity_type];
    view.clear();

    if (selected) {
      view.circle(0, 0, 22).stroke({ color: 0xf4c95d, width: 2, alpha: 0.86 });
    }

    switch (entity.entity_type) {
      case 'player':
        view.moveTo(0, -18).lineTo(14, 14).lineTo(0, 7).lineTo(-14, 14).closePath().fill(color);
        view.moveTo(0, -25).lineTo(0, -38).stroke({ color: 0x7cff9b, width: 2, alpha: 0.85 });
        break;
      case 'npc_placeholder':
        view.circle(0, 0, 13).fill({ color, alpha: 0.82 });
        view.circle(0, 0, 18).stroke({ color, width: 1, alpha: 0.35 });
        break;
      case 'loot_placeholder':
        view.rect(-9, -9, 18, 18).fill({ color, alpha: 0.9 });
        view.rect(-13, -13, 26, 26).stroke({ color: 0xffffff, width: 1, alpha: 0.28 });
        break;
      case 'planet_signal_placeholder':
        view.circle(0, 0, 15).stroke({ color, width: 3, alpha: 0.78 });
        view.circle(0, 0, 4).fill(color);
        break;
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

    if (state.lastCorrection) {
      const corrected = this.worldToScreen(state.lastCorrection.position);
      const marker = new Graphics();
      marker.circle(0, 0, 24).stroke({ color: 0x8af5ff, width: 1, alpha: 0.58 });
      marker.position.set(corrected.x, corrected.y);
      this.markerLayer.addChild(marker);
    }
  }

  private updateInterpolatedEntities(): void {
    for (const [entityID, entity] of this.entityTargets) {
      const view = this.entityViews.get(entityID);
      if (!view) {
        continue;
      }

      const current = this.entityWorldPositions.get(entityID) ?? entity.position;
      const next =
        entity.entity_type === 'player'
          ? entity.position
          : {
              x: lerp(current.x, entity.position.x, 0.16),
              y: lerp(current.y, entity.position.y, 0.16),
            };
      this.entityWorldPositions.set(entityID, snapClose(next, entity.position));
      this.positionEntityView(entityID, view);
    }
  }

  private positionEntityView(entityID: string, view: Graphics): void {
    const world = this.entityWorldPositions.get(entityID) ?? this.entityTargets.get(entityID)?.position;
    if (!world) {
      return;
    }
    const screen = this.worldToScreen(world);
    view.position.set(screen.x, screen.y);
  }

  private findEntityAtScreen(screen: Vec2): EntityPayload | null {
    if (!this.state) {
      return null;
    }

    return (
      this.state.entities.find((entity) => {
        const entityScreen = this.worldToScreen(entity.position);
        const dx = entityScreen.x - screen.x;
        const dy = entityScreen.y - screen.y;
        return dx * dx + dy * dy <= 26 * 26;
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
}

function lerp(from: number, to: number, amount: number): number {
  return from + (to - from) * amount;
}

function snapClose(current: Vec2, target: Vec2): Vec2 {
  const dx = current.x - target.x;
  const dy = current.y - target.y;
  if (dx * dx + dy * dy < 0.25) {
    return { ...target };
  }
  return current;
}

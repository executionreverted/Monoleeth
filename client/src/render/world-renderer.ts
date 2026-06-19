import { Application, Container, Graphics, Text } from 'pixi.js';

import { EntityPayload, Vec2 } from '../protocol/envelope';
import { WorldInputHandlers, WorldViewState } from './world-view';

const entityColors: Record<EntityPayload['entity_type'], number> = {
  player: 0x8af5ff,
  npc: 0xff5c7a,
  loot: 0xf4c95d,
  planet_signal: 0xf4c95d,
};

export class WorldRenderer {
  private app: Application | null = null;
  private readonly backgroundLayer = new Container();
  private readonly worldLayer = new Container();
  private readonly markerLayer = new Container();
  private readonly nebulaLayer = new Graphics();
  private readonly gridLayer = new Graphics();
  private readonly entityViews = new Map<string, Graphics>();
  private readonly entityLabels = new Map<string, Text>();
  private readonly entityTargets = new Map<string, EntityPayload>();
  private readonly entityWorldPositions = new Map<string, Vec2>();
  private readonly stars: Array<{ view: Graphics; base: Vec2; depth: number }> = [];
  private emptyLabel: Text | null = null;
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
    app.stage.addChild(this.backgroundLayer);
    this.backgroundLayer.addChild(this.nebulaLayer);
    this.backgroundLayer.addChild(this.gridLayer);
    app.stage.addChild(this.worldLayer);
    app.stage.addChild(this.markerLayer);

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
      this.updateBackground();
      this.updateInterpolatedEntities();
      this.markerLayer.rotation += 0.0007 * ticker.deltaTime;
    });
  }

  render(state: WorldViewState): void {
    if (!this.app) {
      return;
    }

    this.state = state;
    if (this.emptyLabel) {
      this.emptyLabel.visible = state.entities.length === 0;
      this.emptyLabel.position.set(this.app.screen.width / 2, this.app.screen.height / 2);
    }
    const local = state.entities.find(isSelfEntity) ?? state.entities.find((entity) => entity.entity_type === 'player');
    this.center = local?.position ?? this.center;
    this.scale = this.app.screen.width < 700 ? 0.78 : 1;
    this.updateBackground();

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
      label.style.fill = entityColors[entity.entity_type];
      label.visible = label.text !== '';
      this.drawEntity(view, entity, state.selectedTargetID === entity.entity_id, isSelfEntity(entity));
      if (isSelfEntity(entity)) {
        this.entityWorldPositions.set(entity.entity_id, { ...entity.position });
      }
      this.positionEntityView(entity.entity_id, view);
      this.positionEntityLabel(entity.entity_id, label);
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

    const width = Math.max(this.app.screen.width, 1600);
    const height = Math.max(this.app.screen.height, 1000);
    this.nebulaLayer
      .circle(width * 0.28, height * 0.22, 220)
      .fill({ color: 0x6f3d8f, alpha: 0.1 })
      .circle(width * 0.74, height * 0.72, 260)
      .fill({ color: 0x2bdfff, alpha: 0.065 })
      .circle(width * 0.52, height * 0.45, 180)
      .fill({ color: 0xff5c7a, alpha: 0.045 });
    for (let index = 0; index < 220; index += 1) {
      const star = new Graphics();
      const radius = index % 17 === 0 ? 1.8 : index % 5 === 0 ? 1.15 : 0.75;
      const color = index % 7 === 0 ? 0xf4c95d : index % 5 === 0 ? 0x7cff9b : 0xd7f7ff;
      star.circle(0, 0, radius).fill(color);
      const base = { x: (index * 97) % width, y: (index * 53) % height };
      const depth = index % 11 === 0 ? 0.72 : index % 3 === 0 ? 0.44 : 0.22;
      star.position.set(base.x, base.y);
      star.alpha = 0.3 + depth * 0.28;
      this.stars.push({ view: star, base, depth });
      this.backgroundLayer.addChild(star);
    }
    this.updateBackground();
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

  private createEntityLabel(entity: EntityPayload): Text {
    const label = new Text({
      text: labelForEntity(entity),
      style: {
        fontFamily: 'IBM Plex Mono, Aptos Mono, monospace',
        fontSize: 12,
        fill: entityColors[entity.entity_type],
        stroke: { color: '#050709', width: 3 },
      },
      anchor: 0.5,
    });
    return label;
  }

  private drawEntity(view: Graphics, entity: EntityPayload, selected: boolean, self: boolean): void {
    const color = entityColors[entity.entity_type];
    view.clear();

    if (selected) {
      view.circle(0, 0, 22).stroke({ color: 0xf4c95d, width: 2, alpha: 0.86 });
    }
    if (self) {
      view.circle(0, 0, 48).stroke({ color: 0x2bdfff, width: 1, alpha: 0.28 });
      view.circle(0, 0, 72).stroke({ color: 0x2bdfff, width: 1, alpha: 0.16 });
    }

    switch (entity.entity_type) {
      case 'player':
        view.moveTo(0, -18).lineTo(14, 14).lineTo(0, 7).lineTo(-14, 14).closePath().fill(color);
        view.moveTo(0, -25).lineTo(0, -38).stroke({ color: self ? 0x2bdfff : 0x7cff9b, width: 2, alpha: 0.85 });
        break;
      case 'npc':
        view.circle(0, 0, 13).fill({ color, alpha: 0.82 });
        view.circle(0, 0, 18).stroke({ color, width: 1, alpha: 0.35 });
        break;
      case 'loot':
        view.rect(-9, -9, 18, 18).fill({ color, alpha: 0.9 });
        view.rect(-13, -13, 26, 26).stroke({ color: 0xffffff, width: 1, alpha: 0.28 });
        break;
      case 'planet_signal':
        view.circle(0, 0, 15).stroke({ color, width: 3, alpha: 0.78 });
        view.moveTo(-7, -7).lineTo(7, 7).moveTo(7, -7).lineTo(-7, 7).stroke({ color, width: 2, alpha: 0.72 });
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

  private updateBackground(): void {
    if (!this.app) {
      return;
    }

    const width = this.app.screen.width;
    const height = this.app.screen.height;
    const gridSize = width < 700 ? 52 : 96;
    const offsetX = wrap(-this.center.x * 0.18 * this.scale, gridSize);
    const offsetY = wrap(-this.center.y * 0.18 * this.scale, gridSize);

    this.gridLayer.clear();
    for (let x = offsetX - gridSize; x <= width + gridSize; x += gridSize) {
      this.gridLayer.moveTo(x, 0).lineTo(x, height).stroke({ color: 0x2bdfff, width: 1, alpha: 0.08 });
    }
    for (let y = offsetY - gridSize; y <= height + gridSize; y += gridSize) {
      this.gridLayer.moveTo(0, y).lineTo(width, y).stroke({ color: 0x2bdfff, width: 1, alpha: 0.08 });
    }

    this.nebulaLayer.position.set(wrap(-this.center.x * 0.025, 120) - 60, wrap(-this.center.y * 0.025, 120) - 60);
    for (const star of this.stars) {
      const parallaxX = star.base.x - this.center.x * star.depth * 0.055;
      const parallaxY = star.base.y - this.center.y * star.depth * 0.055;
      star.view.position.set(wrap(parallaxX, width + 80) - 40, wrap(parallaxY, height + 80) - 40);
    }
  }

  private updateInterpolatedEntities(): void {
    for (const [entityID, entity] of this.entityTargets) {
      const view = this.entityViews.get(entityID);
      if (!view) {
        continue;
      }
      const label = this.entityLabels.get(entityID);

      const current = this.entityWorldPositions.get(entityID) ?? entity.position;
      const next =
        isSelfEntity(entity)
          ? entity.position
          : {
              x: lerp(current.x, entity.position.x, 0.16),
              y: lerp(current.y, entity.position.y, 0.16),
            };
      this.entityWorldPositions.set(entityID, snapClose(next, entity.position));
      this.positionEntityView(entityID, view);
      if (label) {
        this.positionEntityLabel(entityID, label);
      }
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

  private positionEntityLabel(entityID: string, label: Text): void {
    const world = this.entityWorldPositions.get(entityID) ?? this.entityTargets.get(entityID)?.position;
    if (!world) {
      return;
    }
    const screen = this.worldToScreen(world);
    label.position.set(screen.x, screen.y - 34);
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

function isSelfEntity(entity: EntityPayload): boolean {
  return entity.status_flags?.includes('self') || entity.status_flags?.includes('local') || false;
}

function labelForEntity(entity: EntityPayload): string {
  if (isSelfEntity(entity)) {
    return 'YOU';
  }
  return entity.display?.label ?? '';
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

function wrap(value: number, size: number): number {
  return ((value % size) + size) % size;
}

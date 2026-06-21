import { Graphics, Sprite } from 'pixi.js';

import { Vec2 } from '../protocol/envelope';
import { WorldRendererBase } from './world-renderer-base';
import {
  isOddTile,
  StarfieldDebugState,
  StarfieldLayerID,
  starfieldLayerConfigs,
  STARFIELD_GRID_RADIUS,
  STARFIELD_TILE,
  wrap,
} from './world-renderer-types';

export abstract class WorldRendererStarfield extends WorldRendererBase {
  protected createStarfield(): void {
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


  protected updateBackground(): void {
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

  protected updateStarfieldTiles(width: number, height: number): void {
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

}

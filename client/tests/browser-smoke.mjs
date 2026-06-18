import { mkdir } from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';

import { chromium } from 'playwright';

const url = readArg('--url') ?? 'http://127.0.0.1:5173';
const outputDir = path.resolve('tmp', 'smoke');

const forbiddenText = ['gameplay_seed', 'future_spawn', 'internal_metadata', 'loot_table'];

const browser = await chromium.launch({ headless: true });

try {
  await mkdir(outputDir, { recursive: true });
  await verifyViewport({ width: 1440, height: 900 }, 'desktop');
  await verifyViewport({ width: 390, height: 844 }, 'mobile');
} finally {
  await browser.close();
}

async function verifyViewport(viewport, label) {
  const page = await browser.newPage({ viewport });
  await page.goto(url, { waitUntil: 'networkidle' });
  await page.waitForSelector('canvas.world-canvas', { timeout: 10000 });
  await page.waitForTimeout(350);

  const stats = await page.evaluate(() => {
    const canvas = document.querySelector('canvas.world-canvas');
    if (!(canvas instanceof HTMLCanvasElement)) {
      return { samples: 0, nonBlank: 0, width: 0, height: 0, scrollWidth: document.body.scrollWidth };
    }

    const context = canvas.getContext('2d');
    if (!context) {
      return { samples: 0, nonBlank: 1, width: canvas.width, height: canvas.height, scrollWidth: document.body.scrollWidth };
    }

    const width = canvas.width;
    const height = canvas.height;
    let samples = 0;
    let nonBlank = 0;
    for (let y = 0; y < height; y += Math.max(1, Math.floor(height / 12))) {
      for (let x = 0; x < width; x += Math.max(1, Math.floor(width / 12))) {
        const [r, g, b, a] = context.getImageData(x, y, 1, 1).data;
        samples += 1;
        if (a > 0 && (r > 8 || g > 8 || b > 8)) {
          nonBlank += 1;
        }
      }
    }
    return { samples, nonBlank, width, height, scrollWidth: document.body.scrollWidth };
  });

  if (stats.width === 0 || stats.height === 0) {
    throw new Error(`${label}: canvas has no size`);
  }
  if (stats.nonBlank === 0) {
    throw new Error(`${label}: canvas appears blank`);
  }
  if (stats.scrollWidth > viewport.width + 1) {
    throw new Error(`${label}: layout has horizontal overflow (${stats.scrollWidth} > ${viewport.width})`);
  }

  const text = await page.locator('body').innerText();
  for (const forbidden of forbiddenText) {
    if (text.includes(forbidden)) {
      throw new Error(`${label}: forbidden debug text leaked: ${forbidden}`);
    }
  }

  await page.screenshot({ path: path.join(outputDir, `${label}.png`), fullPage: true });
  await page.close();
}

function readArg(name) {
  const index = process.argv.indexOf(name);
  if (index === -1) {
    return null;
  }
  return process.argv[index + 1] ?? null;
}

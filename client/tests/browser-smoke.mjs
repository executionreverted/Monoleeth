import { createHash } from 'node:crypto';
import { mkdir } from 'node:fs/promises';
import net from 'node:net';
import path from 'node:path';
import process from 'node:process';

import { chromium } from 'playwright';
import { createServer as createViteServer } from 'vite';

const explicitURL = readArg('--url');
const outputDir = path.resolve('tmp', 'smoke');

const forbiddenText = ['gameplay_seed', 'future_spawn', 'internal_metadata', 'loot_table'];
let eventSequence = 100;

const appServer = explicitURL ? null : await startViteAppServer();
const url = explicitURL ?? appServer.url;
let browser;

try {
  browser = await chromium.launch({ headless: true });
  await mkdir(outputDir, { recursive: true });
  await verifyViewport({ width: 1440, height: 900 }, 'desktop');
  await verifyViewport({ width: 390, height: 844 }, 'mobile');
} finally {
  await browser?.close();
  await appServer?.close();
}

async function verifyViewport(viewport, label) {
  const realtime = await startMockRealtimeServer();
  const page = await browser.newPage({ viewport });
  try {
    await page.goto(withSmokeParam(url), { waitUntil: 'networkidle' });
    await page.waitForSelector('canvas.world-canvas', { timeout: 10000 });
    await page.waitForFunction(() => Boolean(window.__SPACE_MORPG_SMOKE_STATE__), null, { timeout: 10000 });
    await page.waitForTimeout(350);

    await page.locator('.socket-field__input').fill(realtime.url);
    await page.locator('[data-action="connect"]').click();
    await realtime.waitForOp('debug_snapshot');
    await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.connectionStatus === 'connected');
    await page.waitForFunction(() => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      return (
        state?.playerSnapshot?.callsign === 'Server-Pilot' &&
        state?.cargo?.capacity === 80 &&
        state?.wallet?.credits === 1250 &&
        state?.stats?.radar_range === 420 &&
        state?.commandLog?.some((line) => line.text === 'Forbidden server payload rejected.') &&
        !state?.visibleEntities?.['hidden-planet']
      );
    });

    await clickWorldPosition(page, { x: 150, y: -250 });
    await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'npc-rake-01');
    await page.locator('[data-action="fire"]').click();
    await realtime.waitForOp('combat.use_skill');
    await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.playerSnapshot?.energy === 58);

    await clickWorldPosition(page, { x: -110, y: -220 });
    await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'loot-scrap-01');
    await page.locator('[data-action="loot"]').click();
    await realtime.waitForOp('loot.pickup');
    await page.waitForFunction(() => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      return state?.cargo?.used === 21 && state?.cargo?.items?.some((item) => item.item_id === 'raw_ore' && item.quantity === 15);
    });

    await page.locator('[data-action="scan"]').click();
    await realtime.waitForOp('scan.pulse');

    await clickWorldPosition(page, { x: 40, y: -320 });
    await realtime.waitForOp('move_to');
    await page.waitForFunction(() => {
      const player = window.__SPACE_MORPG_SMOKE_STATE__?.visibleEntities?.['player-local'];
      return Math.abs((player?.position?.x ?? 9999) - 40) <= 1 && Math.abs((player?.position?.y ?? 9999) + 320) <= 1;
    });
    await page.waitForTimeout(150);

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
    assertNoForbiddenText(text, `${label} body`);
    const smokeState = await page.evaluate(() => JSON.stringify(window.__SPACE_MORPG_SMOKE_STATE__));
    assertNoForbiddenText(smokeState, `${label} smoke state`);

    await page.screenshot({ path: path.join(outputDir, `${label}.png`), fullPage: true });
  } finally {
    await page.close();
    await realtime.close();
  }
}

async function clickWorldPosition(page, world) {
  const point = await page.evaluate((target) => {
    const canvas = document.querySelector('canvas.world-canvas');
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    if (!(canvas instanceof HTMLCanvasElement) || !state) {
      return null;
    }
    const rect = canvas.getBoundingClientRect();
    const player = Object.values(state.visibleEntities ?? {}).find((entity) => entity.entity_type === 'player');
    const center = player?.position ?? { x: 0, y: 0 };
    const scale = rect.width < 700 ? 0.78 : 1;
    return {
      x: rect.left + rect.width / 2 + (target.x - center.x) * scale,
      y: rect.top + rect.height / 2 + (target.y - center.y) * scale,
      element: document.elementFromPoint(
        rect.left + rect.width / 2 + (target.x - center.x) * scale,
        rect.top + rect.height / 2 + (target.y - center.y) * scale,
      )?.className,
    };
  }, world);

  if (!point) {
    throw new Error('Could not map world position to canvas point.');
  }
  if (point.element !== 'world-canvas') {
    throw new Error(`World click at ${Math.round(point.x)},${Math.round(point.y)} hit ${String(point.element)}`);
  }
  await page.mouse.click(point.x, point.y);
}

function assertNoForbiddenText(text, label) {
  for (const forbidden of forbiddenText) {
    if (text.includes(forbidden)) {
      throw new Error(`${label}: forbidden debug text leaked: ${forbidden}`);
    }
  }
}

async function startMockRealtimeServer() {
  const received = [];
  const waiters = new Map();
  const sockets = new Set();
  const server = net.createServer((socket) => {
    sockets.add(socket);
    let buffer = Buffer.alloc(0);
    let handshaken = false;

    socket.on('data', (chunk) => {
      buffer = Buffer.concat([buffer, chunk]);
      if (!handshaken) {
        const end = buffer.indexOf('\r\n\r\n');
        if (end === -1) {
          return;
        }
        const request = buffer.subarray(0, end + 4).toString('utf8');
        buffer = buffer.subarray(end + 4);
        socket.write(webSocketHandshakeResponse(request));
        handshaken = true;
      }

      for (;;) {
        const frame = readClientFrame(buffer);
        if (!frame) {
          return;
        }
        buffer = buffer.subarray(frame.bytesRead);
        if (frame.opcode === 8) {
          socket.end();
          return;
        }
        if (frame.opcode === 1) {
          handleClientMessage(socket, frame.payload, received, waiters);
        }
      }
    });

    socket.on('close', () => sockets.delete(socket));
    socket.on('error', () => sockets.delete(socket));
  });

  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  if (!address || typeof address === 'string') {
    throw new Error('Mock realtime server did not expose a TCP port.');
  }

  return {
    url: `ws://127.0.0.1:${address.port}/ws`,
    waitForOp: (op) => waitForOp(received, waiters, op),
    close: async () => {
      for (const socket of sockets) {
        socket.destroy();
      }
      await new Promise((resolve) => server.close(resolve));
    },
  };
}

async function startViteAppServer() {
  const port = await findFreePort();
  const server = await createViteServer({
    logLevel: 'error',
    server: {
      host: '127.0.0.1',
      port,
      strictPort: true,
    },
  });
  await server.listen();

  return {
    url: `http://127.0.0.1:${port}`,
    close: () => server.close(),
  };
}

function findFreePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.unref();
    server.on('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      server.close((error) => {
        if (error) {
          reject(error);
          return;
        }
        if (!address || typeof address === 'string') {
          reject(new Error('Could not reserve a local smoke app port.'));
          return;
        }
        resolve(address.port);
      });
    });
  });
}

function webSocketHandshakeResponse(request) {
  const key = request.match(/^Sec-WebSocket-Key:\s*(.+)$/im)?.[1]?.trim();
  if (!key) {
    throw new Error('Missing Sec-WebSocket-Key.');
  }
  const accept = createHash('sha1')
    .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
    .digest('base64');
  return [
    'HTTP/1.1 101 Switching Protocols',
    'Upgrade: websocket',
    'Connection: Upgrade',
    `Sec-WebSocket-Accept: ${accept}`,
    '',
    '',
  ].join('\r\n');
}

function readClientFrame(buffer) {
  if (buffer.length < 2) {
    return null;
  }
  const opcode = buffer[0] & 0x0f;
  const masked = (buffer[1] & 0x80) !== 0;
  let length = buffer[1] & 0x7f;
  let offset = 2;
  if (length === 126) {
    if (buffer.length < offset + 2) {
      return null;
    }
    length = buffer.readUInt16BE(offset);
    offset += 2;
  } else if (length === 127) {
    if (buffer.length < offset + 8) {
      return null;
    }
    const high = buffer.readUInt32BE(offset);
    const low = buffer.readUInt32BE(offset + 4);
    length = high * 2 ** 32 + low;
    offset += 8;
  }
  const maskOffset = offset;
  if (masked) {
    offset += 4;
  }
  if (buffer.length < offset + length) {
    return null;
  }
  const payload = Buffer.from(buffer.subarray(offset, offset + length));
  if (masked) {
    const mask = buffer.subarray(maskOffset, maskOffset + 4);
    for (let index = 0; index < payload.length; index += 1) {
      payload[index] ^= mask[index % 4];
    }
  }
  return {
    opcode,
    payload: payload.toString('utf8'),
    bytesRead: offset + length,
  };
}

function handleClientMessage(socket, raw, received, waiters) {
  const request = JSON.parse(raw);
  received.push(request);
  const waiter = waiters.get(request.op);
  if (waiter) {
    waiters.delete(request.op);
    waiter.resolve(request);
  }

  switch (request.op) {
    case 'debug_snapshot':
      sendMessage(socket, response(request.request_id, snapshotPayload()));
      sendMessage(
        socket,
        event('hidden-rejected', 'aoi.entity_entered', {
          entity_id: 'hidden-planet',
          entity_type: 'planet_signal_placeholder',
          position: { x: 9000, y: 9000 },
          gameplay_seed: 'server-only',
        }),
      );
      break;
    case 'move_to':
      sendMessage(socket, response(request.request_id, { accepted: true }));
      sendMessage(socket, event('move-correction', 'position.corrected', { entity_id: 'player-local', position: request.payload.target }));
      break;
    case 'combat.use_skill':
      sendMessage(socket, response(request.request_id, { accepted: true }));
      sendMessage(socket, event('combat-energy', 'player.snapshot', { energy: 58, shield: 61, hp: 84 }));
      break;
    case 'loot.pickup':
      sendMessage(
        socket,
        response(request.request_id, {
          cargo: {
            used: 21,
            capacity: 80,
            items: [
              { item_id: 'raw_ore', quantity: 15 },
              { item_id: 'salvage_thread', quantity: 6 },
            ],
          },
        }),
      );
      break;
    case 'scan.pulse':
      sendMessage(socket, response(request.request_id, { accepted: true, resolve_after_ms: 1000 }));
      break;
    default:
      sendMessage(socket, errorResponse(request.request_id, 'ERR_INVALID_PAYLOAD', 'Unsupported operation.'));
  }
}

function waitForOp(received, waiters, op) {
  const found = received.find((message) => message.op === op);
  if (found) {
    return Promise.resolve(found);
  }
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      waiters.delete(op);
      reject(new Error(`Timed out waiting for ${op}; received ops: ${received.map((message) => message.op).join(', ')}`));
    }, 5000);
    waiters.set(op, {
      resolve: (message) => {
        clearTimeout(timeout);
        resolve(message);
      },
    });
  });
}

function sendMessage(socket, message) {
  socket.write(encodeServerFrame(JSON.stringify(message)));
}

function encodeServerFrame(text) {
  const payload = Buffer.from(text, 'utf8');
  if (payload.length < 126) {
    return Buffer.concat([Buffer.from([0x81, payload.length]), payload]);
  }
  if (payload.length <= 0xffff) {
    const header = Buffer.alloc(4);
    header[0] = 0x81;
    header[1] = 126;
    header.writeUInt16BE(payload.length, 2);
    return Buffer.concat([header, payload]);
  }
  throw new Error('Smoke fixture frame is too large.');
}

function snapshotPayload() {
  return {
    entities: [
      {
        entity_id: 'player-local',
        entity_type: 'player',
        position: { x: 0, y: 0 },
        status_flags: ['local'],
      },
      {
        entity_id: 'npc-rake-01',
        entity_type: 'npc_placeholder',
        position: { x: 150, y: -250 },
        status_flags: ['visible', 'hostile'],
      },
      {
        entity_id: 'loot-scrap-01',
        entity_type: 'loot_placeholder',
        position: { x: -110, y: -220 },
        status_flags: ['visible'],
      },
      {
        entity_id: 'signal-eris-04',
        entity_type: 'planet_signal_placeholder',
        position: { x: 260, y: 150 },
        status_flags: ['known_intel'],
      },
    ],
    player: {
      callsign: 'Server-Pilot',
      hp: 84,
      shield: 61,
      energy: 72,
      max_hp: 100,
      max_shield: 100,
      max_energy: 100,
      rank: 2,
    },
    cargo: {
      used: 17,
      capacity: 80,
      items: [
        { item_id: 'raw_ore', quantity: 11 },
        { item_id: 'salvage_thread', quantity: 6 },
      ],
    },
    wallet: {
      credits: 1250,
      premium_paid: 0,
      premium_earned: 25,
    },
    stats: {
      speed: 180,
      radar_range: 420,
      weapon_range: 260,
      cargo_capacity: 80,
    },
  };
}

function response(requestID, payload) {
  return {
    request_id: requestID,
    ok: true,
    payload,
    server_time: Date.now(),
    v: 1,
  };
}

function errorResponse(requestID, code, message) {
  return {
    request_id: requestID,
    ok: false,
    error: { code, message, retryable: false },
    server_time: Date.now(),
    v: 1,
  };
}

function event(eventID, type, payload) {
  return {
    event_id: eventID,
    type,
    payload,
    server_time: Date.now(),
    seq: eventSequence += 1,
    v: 1,
  };
}

function withSmokeParam(rawURL) {
  const parsed = new URL(rawURL);
  parsed.searchParams.set('smoke', '1');
  return parsed.toString();
}

function readArg(name) {
  const index = process.argv.indexOf(name);
  if (index === -1) {
    return null;
  }
  return process.argv[index + 1] ?? null;
}

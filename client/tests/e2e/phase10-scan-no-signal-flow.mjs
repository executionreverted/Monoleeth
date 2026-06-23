#!/usr/bin/env node
import { spawn } from 'node:child_process';
import net from 'node:net';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const clientDir = resolve(scriptDir, '../..');
const repoRoot = resolve(clientDir, '..');
const viewport = { width: 1440, height: 900 };
const commandTimeoutMS = 12000;
const maxProcessLogLines = 3000;
const planetScannerPoint = { x: 0, y: 0 };
const scannerPoint = { x: 0, y: 0 };
const hiddenTargetPoint = { x: 1400, y: 0 };

const leakTokens = [
  'map_1_1',
  'map_1_2',
  'map_1_3',
  'internal_map_id',
  'source_map_id',
  'destination_map_id',
  'worker_id',
  'map_worker_id',
  'spawn_id',
  'spawn_area_id',
  'enemy_pool_id',
  'stat_template_id',
  'drop_profile_id',
  'aggro_profile_id',
  'leash_profile_id',
  'border_raider_drone_pool',
  'border_raider_drone_area',
  'loot_table',
  'scan_roll',
  'scan_cell',
  'scan_candidate',
  'scan_candidates',
  'planet_candidate',
  'gameplay_seed',
  'procedural_seed',
  'world_seed',
  'player_id',
  'session_id',
  'password_hash',
  'session_token',
  'raw_token',
  'reset_secret',
  'mock_npc',
  'fixture_npc',
  'fake_npc',
  'mock_wallet',
  'mock_cargo',
];

async function main() {
  const serverPort = await freePort();
  const origin = `http://127.0.0.1:${serverPort}`;
  const goServer = child('go-server', 'go', ['run', './cmd/game-server'], repoRoot, {
    GAME_SERVER_ADDR: `127.0.0.1:${serverPort}`,
    GAME_CLIENT_STATIC_DIR: 'client/dist',
    GAME_DEV_MODE: '1',
    GAME_E2E_SCAN_NO_PLANET_SEED: '1',
  });
  let browser;
  const clients = [];

  try {
    await waitHTTP(`${origin}/healthz`, 'Go server', goServer);
    await waitHTTP(`${origin}/?smoke=1`, 'built client', goServer);

    browser = await chromium.launch();
    const nonce = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const planetScanner = await newClient(browser, origin, {
      label: 'planet-scanner',
      email: `phase10-nosignal-planet-${nonce}@example.test`,
      callsign: `P10NP-${nonce.slice(-6)}`,
    });
    clients.push(planetScanner);
    const scanner = await newClient(browser, origin, {
      label: 'scanner',
      email: `phase10-nosignal-scanner-${nonce}@example.test`,
      callsign: `P10NS-${nonce.slice(-6)}`,
    });
    clients.push(scanner);
    const target = await newClient(browser, origin, {
      label: 'target',
      email: `phase10-nosignal-target-${nonce}@example.test`,
      callsign: `P10NT-${nonce.slice(-6)}`,
    });
    clients.push(target);

    await Promise.all(clients.map(openCommandSocket));
    await moveToPosition(planetScanner, planetScannerPoint, 35, 'planet-candidate no-signal point', 60000);
    const planetNoSignal = await pulseNoSignal(planetScanner, 'planet-candidate');
    const planetScannerState = await waitSmoke(
      planetScanner,
      (state) => state.currentMap?.public_map_key === '1-1' && state.planetIntel?.lastScan?.status === 'no_signal' && (state.planetIntel?.planets ?? []).length === 0,
      'planet-candidate no-signal reconciliation',
      10000,
    );
    await assertNoLeak(planetScanner, planetScannerState, 'planet-candidate no-signal state');
    await assertWebSocketCanary(planetScanner, 'planet-candidate no-signal');

    await Promise.all([
      moveToPosition(scanner, scannerPoint, 35, 'scanner no-signal point', 60000),
      moveToPosition(target, hiddenTargetPoint, 35, 'hidden target no-signal point', 60000),
    ]);
    await waitSmoke(
      target,
      (state) => {
        const self = selfEntity(state);
        return self && !self.movement?.moving && distance(self.position, hiddenTargetPoint) <= 45;
      },
      'hidden target authoritative no-signal position',
      15000,
    );

    await toggleStealth(target, true);
    const targetState = await smoke(target);
    assert(selfEntity(targetState)?.status_flags?.includes('stealthed'), 'target stealth flag missing before scan');

    const hiddenNoSignal = await pulseNoSignal(scanner, 'hidden-player');

    const scannerState = await waitSmoke(
      scanner,
      (state) => state.currentMap?.public_map_key === '1-1' && state.planetIntel?.lastScan?.status === 'no_signal',
      'scanner no-signal reconciliation',
      10000,
    );
    assert(!Object.values(scannerState.visibleEntities ?? {}).some((entity) => entity?.callsign === target.callsign), 'hidden target became visible to scanner');
    await assertNoLeak(scanner, scannerState, 'scanner no-signal state');
    await assertNoLeak(target, targetState, 'hidden target state');
    await assertWebSocketCanary(scanner, 'scanner no-signal');
    await assertStorageCanary(clients);
    assertProcessLogCanary([goServer]);

    console.log(
      `phase10-scan-no-signal smoke ok map=1-1 planet=${planetNoSignal.scan.status} hidden=${hiddenNoSignal.scan.status} distance=${Math.round(distance(scannerPoint, hiddenTargetPoint))}`,
    );
  } finally {
    for (const client of clients) {
      await client.page.evaluate(() => window.__phase10NoSignalCommandSocket?.close()).catch(() => {});
      await client.context.close().catch(() => {});
    }
    if (browser) await browser.close().catch(() => {});
    await stop(goServer);
  }
}

async function newClient(browser, origin, { label, email, callsign }) {
  const context = await browser.newContext({ viewport });
  const page = await context.newPage();
  const client = { context, page, label, email, callsign, seq: 1 };
  await installWebSocketCanary(client);
  await page.goto(`${origin}/?smoke=1`, { waitUntil: 'domcontentloaded' });
  await register(client, email, 'correct-password', callsign);
  const state = await waitSmoke(client, originReady, `${label} authenticated origin`, 30000);
  assert(state.auth?.session?.authenticated === true, `${label} missing authenticated session`);
  assertMap(state, '1-1', 'Origin', `${label} origin`);
  await assertNoLeak(client, state, `${label} origin`);
  return client;
}

async function installWebSocketCanary(client) {
  await client.page.addInitScript(() => {
    if (window.__phase10NoSignalWebSocketCanaryInstalled) return;
    window.__phase10NoSignalWebSocketCanaryInstalled = true;
    const NativeWebSocket = window.WebSocket;
    const frames = [];
    const state = { nextSocketID: 1, nextFrameIndex: 0 };
    window.__phase10NoSignalWebSocketFrames = frames;

    function safePath(url) {
      try {
        return new URL(String(url), window.location.href).pathname;
      } catch {
        return '';
      }
    }

    function capture(direction, socketID, url, data) {
      frames.push({
        direction,
        index: state.nextFrameIndex++,
        socket_id: socketID,
        path: safePath(url),
        text: typeof data === 'string' ? data.slice(0, 1_000_000) : '',
        truncated: typeof data === 'string' && data.length > 1_000_000,
      });
    }

    class Phase10NoSignalWebSocket extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        const socketID = state.nextSocketID++;
        this.__phase10NoSignalSocketID = socketID;
        this.addEventListener('message', (event) => capture('in', socketID, this.url || args[0], event.data));
      }

      send(data) {
        capture('out', this.__phase10NoSignalSocketID ?? 0, this.url, data);
        return super.send(data);
      }
    }

    window.WebSocket = Phase10NoSignalWebSocket;
  });
}

async function register(client, email, password, callsign) {
  const { page } = client;
  await page.waitForSelector('.auth-panel input[name="email"]', { timeout: 20000 });
  await page.click('.auth-panel [data-toggle]');
  await page.waitForSelector('.auth-panel[data-mode="register"] input[name="callsign"]');
  await page.fill('.auth-panel input[name="email"]', email);
  await page.fill('.auth-panel input[name="password"]', password);
  await page.fill('.auth-panel input[name="callsign"]', callsign);
  await page.click('.auth-panel [data-submit]');
}

function originReady(state) {
  return state?.connectionStatus === 'connected' && state.auth?.session?.authenticated === true && state.currentMap?.public_map_key === '1-1';
}

async function openCommandSocket(client) {
  await client.page.evaluate(
    () =>
      new Promise((resolve, reject) => {
        if (window.__phase10NoSignalCommandSocket?.readyState === WebSocket.OPEN) return resolve(true);
        const socket = new WebSocket(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`);
        window.__phase10NoSignalCommandSocket = socket;
        const timeout = window.setTimeout(() => reject(new Error('command WebSocket open timeout')), 10000);
        socket.addEventListener('open', () => {
          window.clearTimeout(timeout);
          resolve(true);
        });
        socket.addEventListener('error', () => {
          window.clearTimeout(timeout);
          reject(new Error('command WebSocket error'));
        });
      }),
  );
}

async function send(client, op, payload) {
  await openCommandSocket(client);
  const request = {
    request_id: `phase10-nosignal-${client.label}-${op.replace(/[^a-z0-9]+/gi, '-')}-${Date.now()}-${client.seq}`,
    op,
    payload,
    client_seq: client.seq++,
    v: 1,
  };
  return client.page.evaluate(
    ({ message, timeoutMS }) =>
      new Promise((resolve, reject) => {
        const socket = window.__phase10NoSignalCommandSocket;
        if (!socket || socket.readyState !== WebSocket.OPEN) return reject(new Error('command WebSocket is not open'));
        const timeout = window.setTimeout(() => {
          socket.removeEventListener('message', onMessage);
          reject(new Error(`command response timeout: ${message.request_id}`));
        }, timeoutMS);
        function onMessage(event) {
          let data;
          try {
            data = JSON.parse(String(event.data));
          } catch {
            return;
          }
          if (data.request_id !== message.request_id) return;
          window.clearTimeout(timeout);
          socket.removeEventListener('message', onMessage);
          resolve(data);
        }
        socket.addEventListener('message', onMessage);
        socket.send(JSON.stringify(message));
      }),
    { message: request, timeoutMS: commandTimeoutMS },
  );
}

async function moveToPosition(client, targetPosition, arriveDistance, label, timeoutMS) {
  const deadline = Date.now() + timeoutMS;
  while (Date.now() < deadline) {
    await waitSmoke(client, (state) => state.connectionStatus === 'connected' && Object.keys(state.pendingCommands ?? {}).length === 0, 'pending commands clear', 10000);
    const state = await smoke(client);
    const self = selfEntity(state);
    const position = positionNow(self, state);
    if (distance(position, targetPosition) <= arriveDistance) return state;

    const target = step(position, targetPosition, 500);
    const response = await send(client, 'move_to', { target });
    assertNoLeakPayload(response, `${client.label} move_to ${label} response`);
    if (response.ok !== true && response.error?.code === 'ERR_RATE_LIMITED') {
      await delay(125);
      continue;
    }
    ok(response, `${client.label} move_to`);
    const speed = self?.movement?.speed ?? state.stats?.speed ?? 180;
    const eta = Math.ceil((distance(position, target) / Math.max(1, speed)) * 1000);
    await waitSmoke(
      client,
      (candidate) => {
        const pos = positionNow(selfEntity(candidate), candidate);
        return distance(pos, target) <= 45 || distance(pos, targetPosition) <= arriveDistance;
      },
      `${client.label} movement to ${fmt(target)}`,
      Math.max(6000, eta + 8000),
    );
  }
  throw new Error(`${client.label} timed out before reaching ${label} at ${fmt(targetPosition)}`);
}

async function toggleStealth(client, enabled) {
  const response = await send(client, 'stealth.toggle', { enabled });
  assertNoLeakPayload(response, `${client.label} stealth.toggle response`);
  const payload = payloadOf(response, `${client.label} stealth.toggle`);
  assert(payload.accepted === true, `${client.label} stealth accepted ${compact(payload)}`);
  assert(payload.stealth?.enabled === enabled, `${client.label} stealth enabled ${compact(payload)}`);
  await waitSmoke(client, (state) => selfEntity(state)?.status_flags?.includes('stealthed') === enabled, `${client.label} stealth ${enabled}`, 10000);
}

async function pulseNoSignal(client, label) {
  await resetWebSocketFrames(client);
  const scanResponse = await send(client, 'scan.pulse', {});
  const scanPayload = payloadOf(scanResponse, `${label} scan.pulse`);
  assert(scanPayload.scan?.status === 'no_signal', `${label} scan status ${compact(scanPayload.scan)}, want no_signal`);
  assert(scanPayload.scan?.pulse_reference, `${label} scan pulse reference missing ${compact(scanPayload.scan)}`);
  assert(!scanPayload.scan?.planet_id, `${label} no_signal leaked planet id ${compact(scanPayload.scan)}`);
  assert(!scanPayload.scan?.signal, `${label} no_signal leaked planet signal ${compact(scanPayload.scan)}`);
  assert((scanPayload.known_planets?.planets ?? []).length === 0, `${label} no_signal leaked known planets ${compact(scanPayload.known_planets)}`);
  assertNoLeakPayload(scanPayload, `${label} scan no-signal response`);
  return scanPayload;
}

async function smoke(client) {
  return client.page.evaluate(() => window.__SPACE_MORPG_SMOKE_STATE__ ?? null);
}

async function waitSmoke(client, predicate, label, timeoutMS) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMS) {
    last = await smoke(client);
    if (last && predicate(last)) return last;
    await delay(100);
  }
  throw new Error(`Timed out waiting for ${label}. Last state: ${compact(last)}`);
}

function assertMap(state, publicKey, display, label) {
  const map = state.currentMap;
  assert(map?.public_map_key === publicKey, `${label} map key ${map?.public_map_key}`);
  assert(new RegExp(display, 'i').test(map.display_name ?? ''), `${label} map display ${map?.display_name}`);
  assert(map.bounds?.min_x === 0 && map.bounds?.min_y === 0 && map.bounds?.max_x === 10000 && map.bounds?.max_y === 10000, `${label} bounds ${compact(map.bounds)}`);
}

function selfEntity(state) {
  const entities = Object.values(state?.visibleEntities ?? {});
  return entities.find((entity) => entity.status_flags?.includes('self')) ?? entities.find((entity) => entity.entity_type === 'player') ?? null;
}

function positionNow(entity, state) {
  assert(entity?.position, 'self position present');
  const movement = entity.movement;
  if (!movement?.moving || !movement.origin || !movement.target || !state?.serverNow) return entity.position;
  const start = Date.parse(movement.started_at ?? '');
  const arrive = Date.parse(movement.arrive_at ?? '');
  if (!Number.isFinite(start) || !Number.isFinite(arrive) || arrive <= start) return entity.movement.target;
  const t = Math.min(1, Math.max(0, (state.serverNow - start) / (arrive - start)));
  return {
    x: movement.origin.x + (movement.target.x - movement.origin.x) * t,
    y: movement.origin.y + (movement.target.y - movement.origin.y) * t,
  };
}

function step(from, to, maxDistance) {
  const total = distance(from, to);
  if (total <= maxDistance) return to;
  const ratio = maxDistance / total;
  return { x: from.x + (to.x - from.x) * ratio, y: from.y + (to.y - from.y) * ratio };
}

function distance(a, b) {
  const dx = Number(a?.x ?? 0) - Number(b?.x ?? 0);
  const dy = Number(a?.y ?? 0) - Number(b?.y ?? 0);
  return Math.hypot(dx, dy);
}

function payloadOf(response, label) {
  ok(response, label);
  return response.payload ?? {};
}

function ok(response, label) {
  assert(response?.ok === true, `${label} failed ${compact(response)}`);
}

async function resetWebSocketFrames(client) {
  await client.page.evaluate(() => {
    if (Array.isArray(window.__phase10NoSignalWebSocketFrames)) window.__phase10NoSignalWebSocketFrames.length = 0;
  });
}

async function assertWebSocketCanary(client, label) {
  const frames = await client.page.evaluate(() => window.__phase10NoSignalWebSocketFrames ?? []);
  assert(frames.length > 0, `${label} websocket canary captured no frames`);
  for (const frame of frames) {
    assert(frame.truncated !== true, `${label} websocket frame ${frame.index} exceeded scan limit`);
    for (const token of leakTokens) {
      assert(!frame.text.toLowerCase().includes(token.toLowerCase()), `${label} websocket leaked ${token} in frame ${frame.index}: ${frame.text.slice(0, 300)}`);
    }
  }
}

async function assertStorageCanary(clients) {
  for (const client of clients) {
    const leak = await client.page.evaluate((tokens) => {
      const scanText = (surface, text, key = '') => {
        const lower = String(text ?? '').toLowerCase();
        for (const token of tokens) {
          if (lower.includes(String(token).toLowerCase())) return `${surface}${key ? ` ${key}` : ''} leaked ${token}`;
        }
        return '';
      };
      const scanStorage = (surface, storage) => {
        for (let i = 0; i < storage.length; i++) {
          const key = storage.key(i) ?? '';
          const keyLeak = scanText(`${surface}.key`, key);
          if (keyLeak) return keyLeak;
          const valueLeak = scanText(`${surface}.value`, storage.getItem(key), key);
          if (valueLeak) return valueLeak;
        }
        return '';
      };
      return scanStorage('localStorage', window.localStorage) || scanStorage('sessionStorage', window.sessionStorage) || scanText('document.cookie', document.cookie);
    }, leakTokens);
    assert(!leak, `${client.label} storage leak: ${leak}`);
  }
}

async function assertNoLeak(client, state, label) {
  assertNoLeakPayload(state, label);
  const html = await client.page.locator('body').innerText({ timeout: 5000 }).catch(() => '');
  for (const token of leakTokens) {
    assert(!html.toLowerCase().includes(token.toLowerCase()), `${label} DOM leaked ${token}`);
  }
}

function assertNoLeakPayload(payload, label) {
  const json = JSON.stringify(payload ?? {});
  for (const token of leakTokens) {
    assert(!json.toLowerCase().includes(token.toLowerCase()), `${label} leaked ${token}: ${json.slice(0, 400)}`);
  }
}

function assertProcessLogCanary(processes) {
  for (const proc of processes) {
    for (const line of proc.lines) {
      for (const token of leakTokens) {
        assert(!line.toLowerCase().includes(token.toLowerCase()), `${proc.label} log leaked ${token}: ${line}`);
      }
    }
  }
}

function child(label, command, args, cwd, env = {}) {
  const proc = spawn(command, args, {
    cwd,
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
    detached: process.platform !== 'win32',
  });
  proc.label = label;
  proc.lines = [];
  const capture = (chunk) => {
    for (const line of String(chunk).split(/\r?\n/).filter(Boolean)) {
      proc.lines.push(line);
      if (proc.lines.length > maxProcessLogLines) proc.lines.shift();
    }
  };
  proc.stdout.on('data', capture);
  proc.stderr.on('data', capture);
  proc.on('exit', (code, signal) => {
    if (code && code !== 0) proc.lines.push(`${label} exited code=${code} signal=${signal ?? ''}`);
  });
  return proc;
}

async function waitHTTP(url, label, proc) {
  const deadline = Date.now() + 60000;
  while (Date.now() < deadline) {
    if (proc.exitCode !== null) throw new Error(`${label} exited before ready:\n${proc.lines.join('\n')}`);
    try {
      const response = await fetch(url);
      if (response.ok) return;
    } catch {
      // keep polling
    }
    await delay(250);
  }
  throw new Error(`${label} did not become ready:\n${proc.lines.join('\n')}`);
}

async function stop(proc) {
  if (!proc || proc.exitCode !== null) return;
  killProcess(proc, 'SIGTERM');
  await new Promise((resolve) => {
    const timeout = setTimeout(resolve, 5000);
    proc.once('exit', () => {
      clearTimeout(timeout);
      resolve();
    });
  });
  if (proc.exitCode === null) {
    killProcess(proc, 'SIGKILL');
    await delay(250);
  }
}

function killProcess(proc, signal) {
  if (!proc || proc.exitCode !== null) return;
  try {
    if (process.platform !== 'win32' && proc.pid) {
      process.kill(-proc.pid, signal);
      return;
    }
  } catch {
    // Fall back to killing the direct child.
  }
  proc.kill(signal);
}

async function freePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      server.close(() => resolve(address.port));
    });
    server.on('error', reject);
  });
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function compact(value) {
  return JSON.stringify(value, (_key, entry) => (typeof entry === 'number' ? Number(entry.toFixed?.(3) ?? entry) : entry));
}

function fmt(point) {
  return `${Math.round(point?.x ?? 0)},${Math.round(point?.y ?? 0)}`;
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});

#!/usr/bin/env node
import { spawn } from 'node:child_process';
import { mkdir } from 'node:fs/promises';
import net from 'node:net';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const clientDir = resolve(scriptDir, '../..');
const repoRoot = resolve(clientDir, '..');
const screenshotDir = resolve(repoRoot, 'output/screenshots/ui-implementation/09');
const gateTarget = { x: 9800, y: 5000 };
const viewport = { width: 1440, height: 900 };
let clientSeq = 1;

async function main() {
  const serverPort = await freePort();
  const clientPort = await freePort();
  const clientOrigin = `http://127.0.0.1:${clientPort}`;
  const serverTarget = `http://127.0.0.1:${serverPort}`;
  await mkdir(screenshotDir, { recursive: true });

  const goServer = child('go-server', 'go', ['run', './cmd/game-server'], repoRoot, {
    GAME_SERVER_ADDR: `127.0.0.1:${serverPort}`,
    GAME_ALLOWED_ORIGINS: clientOrigin,
  });
  let viteServer;
  let browser;
  try {
    await waitHTTP(`${serverTarget}/healthz`, 'Go server', goServer);
    viteServer = child(
      'vite',
      'npm',
      ['--cache', '/tmp/gameproject-npm-cache', 'run', 'dev', '--', '--port', String(clientPort), '--strictPort'],
      clientDir,
      { GAME_CLIENT_PROXY_TARGET: serverTarget },
    );
    await waitHTTP(`${clientOrigin}/?smoke=1`, 'Vite client', viteServer);

    browser = await chromium.launch();
    const page = await browser.newPage({ viewport });
    await page.goto(`${clientOrigin}/?smoke=1`, { waitUntil: 'domcontentloaded' });

    const nonce = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    await register(page, `phase09-${nonce}@example.test`, 'correct-password', `P09-${nonce.slice(-8)}`);
    const origin = await waitSmoke(page, originReady, 'authenticated Origin map state', 30000);
    assertOrigin(origin);
    await assertNoLeak(page, origin, 'origin');
    await page.screenshot({ path: resolve(screenshotDir, 'map-origin-desktop.png'), fullPage: true });

    await openCommandSocket(page);
    await moveToGate(page);
    ok(await send(page, 'portal.enter', { portal_id: 'east_gate' }), 'portal.enter');

    const outer = await waitSmoke(page, (s) => s.currentMap?.public_map_key === '1-2', 'Outer Ring map state', 15000);
    assertOuter(outer);
    assertNoOriginMapLeakage(origin, outer);
    await assertNoLeak(page, outer, 'outer-ring');
    await page.screenshot({ path: resolve(screenshotDir, 'map-outer-ring-desktop.png'), fullPage: true });
    await page.evaluate(() => window.__phase09CommandSocket?.close());

    console.log(
      `phase09-map smoke ok origin=${origin.currentMap.public_map_key} destination=${outer.currentMap.public_map_key} screenshots=${resolve(screenshotDir, 'map-origin-desktop.png')},${resolve(screenshotDir, 'map-outer-ring-desktop.png')}`,
    );
  } finally {
    if (browser) await browser.close().catch(() => {});
    if (viteServer) await stop(viteServer);
    await stop(goServer);
  }
}

async function register(page, email, password, callsign) {
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

function assertOrigin(state) {
  const map = needMap(state);
  assert(map.public_map_key === '1-1', `origin map key ${map.public_map_key}`);
  assert(/Origin|Fringe/i.test(map.display_name ?? ''), `origin display ${map.display_name}`);
  assertBounds(map.bounds);
  assert(map.visible_portals?.some((portal) => portal.portal_id === 'east_gate'), 'east_gate portal visible');
  assert((map.safe_zones?.length ?? 0) > 0, 'origin safe zones visible');
}

function assertOuter(state) {
  const map = needMap(state);
  assert(map.public_map_key === '1-2', `destination map key ${map.public_map_key}`);
  assert(/Outer Ring/i.test(map.display_name ?? ''), `destination display ${map.display_name}`);
  assertBounds(map.bounds);
  const portals = new Set((map.visible_portals ?? []).map((portal) => portal.portal_id));
  assert(portals.has('west_gate'), 'west_gate portal visible');
  assert(!portals.has('east_gate'), 'east_gate portal absent');
  if (map.safe_zone) {
    assert(map.safe_zone.inside === true, 'destination safe-zone inside');
    assert(map.safe_zone.blocks_pvp === true, 'destination safe-zone blocks PvP');
  } else {
    assert(map.protection?.blocks_pvp === true, 'destination protection blocks PvP');
  }
}

function assertNoOriginMapLeakage(origin, outer) {
  const originNonSelfIDs = entityIDs(origin).filter((id) => id !== selfEntity(origin)?.entity_id);
  const outerEntityIDs = new Set(entityIDs(outer));
  for (const id of originNonSelfIDs) {
    assert(!outerEntityIDs.has(id), `origin entity leaked after transfer: ${id}`);
  }

  const outerContactIDs = new Set((outer.minimap?.live_contacts ?? []).map((contact) => contact.entity_id).filter(Boolean));
  for (const id of originNonSelfIDs) {
    assert(!outerContactIDs.has(id), `origin minimap contact leaked after transfer: ${id}`);
  }

  const originLootIDs = new Set(Object.keys(origin.knownLoot ?? {}));
  for (const id of originLootIDs) {
    assert(!(id in (outer.knownLoot ?? {})), `origin loot leaked after transfer: ${id}`);
  }

  const minimapPortalIDs = new Set((outer.minimap?.visible_portals ?? []).map((portal) => portal.portal_id));
  assert(!minimapPortalIDs.has('east_gate'), 'origin east_gate leaked into destination minimap');
  assert(outer.selectedTargetID === null || !originNonSelfIDs.includes(outer.selectedTargetID), 'origin selected target survived transfer');
  assert(outer.movementTarget === null, 'origin movement target survived transfer');
  assert((outer.mapSubscriptionEpoch ?? 0) > (origin.mapSubscriptionEpoch ?? 0), 'destination epoch did not advance');
}

function needMap(state) {
  assert(state?.currentMap, 'currentMap present');
  return state.currentMap;
}

function assertBounds(bounds) {
  assert(bounds?.min_x === 0 && bounds?.min_y === 0 && bounds?.max_x === 10000 && bounds?.max_y === 10000, `bounds ${JSON.stringify(bounds)}`);
}

async function moveToGate(page) {
  const deadline = Date.now() + 90000;
  while (Date.now() < deadline) {
    await waitSmoke(page, (s) => s.connectionStatus === 'connected' && Object.keys(s.pendingCommands ?? {}).length === 0, 'pending commands clear', 10000);
    const state = await smoke(page);
    const self = selfEntity(state);
    const position = positionNow(self, state);
    if (distance(position, gateTarget) <= 120) return;

    const target = step(position, gateTarget, 1100);
    const response = await send(page, 'move_to', { target });
    if (response.ok !== true && response.error?.code === 'ERR_RATE_LIMITED') {
      await delay(125);
      continue;
    }
    ok(response, 'move_to');
    const eta = Math.ceil((distance(position, target) / Math.max(1, self?.movement?.speed ?? 180)) * 1000);
    await waitSmoke(
      page,
      (candidate) => {
        const pos = positionNow(selfEntity(candidate), candidate);
        return distance(pos, target) <= 35 || distance(pos, gateTarget) <= 120;
      },
      `movement to ${fmt(target)}`,
      Math.max(5000, eta + 5000),
    );
  }
  throw new Error(`Timed out before reaching east_gate at ${fmt(gateTarget)}`);
}

async function openCommandSocket(page) {
  await page.evaluate(
    () =>
      new Promise((resolve, reject) => {
        if (window.__phase09CommandSocket?.readyState === WebSocket.OPEN) return resolve(true);
        const socket = new WebSocket(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`);
        window.__phase09CommandSocket = socket;
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

async function send(page, op, payload) {
  const request = {
    request_id: `phase09-${op.replace(/[^a-z0-9]+/gi, '-')}-${Date.now()}-${clientSeq}`,
    op,
    payload,
    client_seq: clientSeq++,
    v: 1,
  };
  return page.evaluate(
    (message) =>
      new Promise((resolve, reject) => {
        const socket = window.__phase09CommandSocket;
        if (!socket || socket.readyState !== WebSocket.OPEN) return reject(new Error('command WebSocket is not open'));
        const timeout = window.setTimeout(() => {
          socket.removeEventListener('message', onMessage);
          reject(new Error(`command response timeout: ${message.request_id}`));
        }, 10000);
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
    request,
  );
}

async function smoke(page) {
  return page.evaluate(() => window.__SPACE_MORPG_SMOKE_STATE__ ?? null);
}

async function waitSmoke(page, predicate, description, timeoutMS) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMS) {
    last = await smoke(page);
    if (last && predicate(last)) return last;
    await delay(100);
  }
  throw new Error(`Timed out waiting for ${description}. Last state: ${compact(last)}`);
}

function selfEntity(state) {
  const entities = Object.values(state?.visibleEntities ?? {});
  return entities.find((entity) => entity.status_flags?.includes('self')) ?? entities.find((entity) => entity.entity_type === 'player') ?? null;
}

function entityIDs(state) {
  return Object.keys(state?.visibleEntities ?? {});
}

function positionNow(entity, state) {
  assert(entity?.position, 'self position present');
  const movement = entity.movement;
  if (!movement?.moving || !movement.origin || !movement.target || !state?.serverNow) return entity.position;
  const duration = movement.arrive_at_ms - movement.started_at_ms;
  if (duration <= 0) return movement.target;
  const progress = Math.max(0, Math.min(1, (state.serverNow - movement.started_at_ms) / duration));
  return {
    x: movement.origin.x + (movement.target.x - movement.origin.x) * progress,
    y: movement.origin.y + (movement.target.y - movement.origin.y) * progress,
  };
}

function step(from, to, maxDistance) {
  const total = distance(from, to);
  if (total <= maxDistance) return round(to);
  const scale = maxDistance / total;
  return round({ x: from.x + (to.x - from.x) * scale, y: from.y + (to.y - from.y) * scale });
}

function round(vec) {
  return { x: Math.round(vec.x), y: Math.round(vec.y) };
}

function distance(a, b) {
  return !a || !b ? Number.POSITIVE_INFINITY : Math.hypot(a.x - b.x, a.y - b.y);
}

async function assertNoLeak(page, state, label) {
  const body = await page.locator('body').innerText({ timeout: 5000 });
  const json = JSON.stringify(state);
  assert(!body.includes('Unhandled event'), `${label} DOM has unhandled event log`);
  for (const token of leakTokens) {
    assert(!body.includes(token), `${label} DOM leaked ${token}`);
    assert(!json.includes(token), `${label} smoke state leaked ${token}`);
  }
  const key = forbiddenKey(state);
  assert(!key, `${label} smoke state leaked forbidden key ${key}`);
}

const leakTokens = [
  'map_1_1', 'map_1_2', 'internal_map_id', 'destination_map_id', 'destination_spawn_id', 'source_map_id',
  'source_spawn_id', 'spawn_id', 'worker_id', 'map_worker_id', 'destination_worker', 'origin_worker',
  'destination_id', 'destination_key', 'destination_map_key', 'destination_position', 'destination_public_key',
  'destination_public_map_key', 'from_map_key', 'from_public_map_key', 'source_map_key', 'source_public_map_key',
  'source_position', 'spawn_map_key', 'spawn_point', 'spawn_position', 'spawn_public_map_key', 'to_map_key',
  'to_public_map_key',
  'gameplay_seed', 'procedural_seed', 'world_seed', 'spawn_candidates', 'planet_candidate', 'scan_roll',
  'loot_roll', 'loot_table',
];

function forbiddenKey(value, path = []) {
  if (Array.isArray(value)) {
    for (let i = 0; i < value.length; i += 1) {
      const found = forbiddenKey(value[i], [...path, String(i)]);
      if (found) return found;
    }
    return null;
  }
  if (!value || typeof value !== 'object') return null;
  const forbidden = new Set(leakTokens);
  for (const [key, child] of Object.entries(value)) {
    if (forbidden.has(key.toLowerCase())) return [...path, key].join('.');
    const found = forbiddenKey(child, [...path, key]);
    if (found) return found;
  }
  return null;
}

function child(name, command, args, cwd, env) {
  const proc = spawn(command, args, {
    cwd,
    detached: true,
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  proc.output = [];
  const collect = (chunk, tag) => {
    for (const line of chunk.toString().split(/\r?\n/).filter(Boolean)) proc.output.push(`[${name}:${tag}] ${line}`);
    if (proc.output.length > 80) proc.output.splice(0, proc.output.length - 80);
  };
  proc.stdout.on('data', (chunk) => collect(chunk, 'out'));
  proc.stderr.on('data', (chunk) => collect(chunk, 'err'));
  return proc;
}

async function waitHTTP(url, label, proc) {
  const deadline = Date.now() + 30000;
  while (Date.now() < deadline) {
    if (proc.exitCode !== null) throw new Error(`${label} exited early:\n${proc.output.join('\n')}`);
    try {
      if ((await fetch(url)).ok) return;
    } catch {
      // listener not ready
    }
    await delay(150);
  }
  throw new Error(`${label} did not become ready:\n${proc.output.join('\n')}`);
}

async function stop(proc) {
  if (!proc || proc.exitCode !== null || proc.signalCode !== null) return;
  signal(proc, 'SIGTERM');
  const exited = await waitExit(proc, 3000);
  if (!exited) {
    signal(proc, 'SIGKILL');
    await waitExit(proc, 2000);
  }
}

function signal(proc, sig) {
  try {
    process.kill(-proc.pid, sig);
    return;
  } catch {
    // Fall back to the direct child if the process group is already gone.
  }
  try {
    proc.kill(sig);
  } catch {
    // Already exited.
  }
}

async function waitExit(proc, timeoutMS) {
  if (proc.exitCode !== null || proc.signalCode !== null) return true;
  return Promise.race([
    new Promise((resolve) => proc.once('exit', () => resolve(true))),
    delay(timeoutMS).then(() => false),
  ]);
}

async function freePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.on('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      const port = typeof address === 'object' && address ? address.port : null;
      server.close(() => (port ? resolve(port) : reject(new Error('failed to allocate port'))));
    });
  });
}

function ok(response, label) {
  assert(response?.ok === true, `${label} failed: ${JSON.stringify(response)}`);
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function fmt(vec) {
  return `${Math.round(vec.x)},${Math.round(vec.y)}`;
}

function compact(state) {
  if (!state) return 'null';
  return JSON.stringify({
    connectionStatus: state.connectionStatus,
    currentMap: state.currentMap,
    self: selfEntity(state),
    pending: Object.keys(state.pendingCommands ?? {}),
    transfer: state.mapTransfer,
    lastError: state.lastError,
  });
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

main().catch((error) => {
  console.error(error?.stack ?? error);
  process.exitCode = 1;
});

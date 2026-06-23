#!/usr/bin/env node
import { spawn } from 'node:child_process';
import net from 'node:net';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const clientDir = resolve(scriptDir, '../..');
const repoRoot = resolve(clientDir, '..');
const maxProcessLogLines = 3000;
const viewport = { width: 1440, height: 900 };
const eastGateTarget = { x: 9800, y: 5000 };
const skirmishGateTarget = { x: 9800, y: 5000 };
const borderRaiderApproachTarget = { x: 5400, y: 5200 };

const leakTokens = [
  'map_1_1',
  'map_1_2',
  'map_1_3',
  'internal_map_id',
  'source_map_id',
  'destination_map_id',
  'spawn_id',
  'spawn_area_id',
  'enemy_pool_id',
  'stat_template_id',
  'drop_profile_id',
  'aggro_profile_id',
  'leash_profile_id',
  'border_raider_drone_pool',
  'border_raider_drone_area',
  'border_raider_drone_level_2',
  'border_raider_drone_salvage',
  'border_raider_salvage',
  'loot_table',
  'loot_roll',
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
];

const forbiddenFieldNames = new Set([
  'account_id',
  'session_id',
  'world_id',
  'zone_id',
  'map_id',
  'internal_map_id',
  'source_map_id',
  'destination_map_id',
  'owner_player_id',
  'player_id',
  'spawn_id',
  'spawn_area_id',
  'enemy_pool_id',
  'stat_template_id',
  'drop_profile_id',
  'aggro_profile_id',
  'leash_profile_id',
  'loot_roll',
  'loot_table',
  'password_hash',
  'token',
  'session_token',
  'reset_secret',
]);

async function main() {
  const serverPort = await freePort();
  const origin = `http://127.0.0.1:${serverPort}`;
  const goServer = child('go-server', 'go', ['run', './cmd/game-server'], repoRoot, {
    GAME_SERVER_ADDR: `127.0.0.1:${serverPort}`,
    GAME_CLIENT_STATIC_DIR: 'client/dist',
  });
  let browser;
  let context;

  try {
    await waitHTTP(`${origin}/healthz`, 'Go server', goServer);
    await waitHTTP(`${origin}/?smoke=1`, 'built client', goServer);

    browser = await chromium.launch();
    context = await browser.newContext({ viewport });
    const page = await context.newPage();
    const client = { page, seq: 1 };
    await installWebSocketCanary(client);

    const nonce = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    await page.goto(`${origin}/?smoke=1`, { waitUntil: 'domcontentloaded' });
    await register(client, `phase10-pvp-drop-${nonce}@example.test`, 'correct-password', `P10D-${nonce.slice(-7)}`);
    const originState = await waitSmoke(client, originReady, 'authenticated Origin state', 30000);
    assert(originState.auth?.session?.authenticated === true, 'real authenticated session missing');
    assertMap(originState, '1-1', 'Origin', 'origin');
    await assertNoLeak(client, originState, 'origin');

    await openCommandSocket(client);
    await enterPortalViaPosition(client, 'east_gate', eastGateTarget, '1-2', 'Outer Ring');
    await enterPortalViaPosition(client, 'skirmish_gate', skirmishGateTarget, '1-3', 'Border Skirmish');

    await moveToPosition(client, borderRaiderApproachTarget, 220, 'border raider approach', 90000);
    const withNPC = await waitSmoke(
      client,
      (state) => state.currentMap?.public_map_key === '1-3' && findLiveNPC(state),
      'visible Border Skirmish NPC',
      20000,
    );
    const npc = findLiveNPC(withNPC);
    assert(npc.entity_id, `pvp-map npc missing public entity id ${compact(npc)}`);
    assert(npc.position, `pvp-map npc missing position ${compact(npc)}`);
    assertNoPayloadLeak(npc, 'pvp-map visible npc');

    await moveToPosition(client, npc.position, Math.max(80, Math.min(220, (withNPC.stats?.weapon_range ?? 260) - 40)), `pvp-map npc ${npc.entity_id}`, 40000);
    await resetWebSocketFrames(client);
    const combatPayload = await fightNPCUntilKilled(client, npc.entity_id);
    const expectedDrop = responseDrop(combatPayload, 'pvp-map combat response');
    assert(expectedDrop.item_id === 'carbon_shards', `pvp-map drop item ${expectedDrop.item_id}, want carbon_shards`);
    assert(expectedDrop.quantity >= 2, `pvp-map drop quantity ${expectedDrop.quantity}, want at least 2`);

    const withDrop = await waitSmoke(
      client,
      (state) => state.currentMap?.public_map_key === '1-3' && findKnownDrop(state, expectedDrop),
      'pvp-map server-created loot drop',
      15000,
    );
    const drop = findKnownDrop(withDrop, expectedDrop);
    assertDropMatches(drop, expectedDrop, 'pvp-map loot drop');
    assertNoPayloadLeak({ drop }, 'pvp-map smoke loot drop');

    const beforeCargo = withDrop.cargo;
    const pickupDropID = drop.drop_id ?? expectedDrop.drop_id ?? expectedDrop.entity_id;
    await moveToPosition(client, drop.position, Math.max(45, (withDrop.stats?.loot_pickup_range ?? 120) - 25), `pvp-map loot ${pickupDropID}`, 30000);
    const pickupPayload = payloadOf(await send(client, 'loot.pickup', { drop_id: pickupDropID }), 'loot.pickup');
    assert(pickupPayload.accepted === true, `loot.pickup accepted ${compact(pickupPayload)}`);
    assertCargoPickup(pickupPayload.cargo, beforeCargo, drop, 'pvp-map loot.pickup response cargo');

    const finalState = await waitSmoke(
      client,
      (state) =>
        state.currentMap?.public_map_key === '1-3' &&
        cargoIncludesPickup(state.cargo, beforeCargo, drop) &&
        !state.knownLoot?.[pickupDropID] &&
        !hasUnhandledEventLog(state),
      'pvp-map cargo reconciliation after loot pickup',
      15000,
    );
    await assertNoLeak(client, finalState, 'pvp-map final');
    await assertWebSocketCanary(client, 'pvp-map drop');
    assertProcessLogCanary([goServer]);

    console.log(`phase10-pvp-map-drop smoke ok map=1-3 npc=${npc.entity_id} drop=${pickupDropID} item=${drop.item_id}x${drop.quantity}`);
  } finally {
    if (context) await context.close().catch(() => {});
    if (browser) await browser.close().catch(() => {});
    await stop(goServer);
  }
}

function originReady(state) {
  return state?.connectionStatus === 'connected' && state.auth?.session?.authenticated === true && state.currentMap?.public_map_key === '1-1';
}

async function installWebSocketCanary(client) {
  await client.page.addInitScript(() => {
    if (window.__phase10PVPDropWebSocketCanaryInstalled) return;
    window.__phase10PVPDropWebSocketCanaryInstalled = true;
    const NativeWebSocket = window.WebSocket;
    const frames = [];
    const state = { nextSocketID: 1, nextFrameIndex: 0 };
    window.__phase10PVPDropWebSocketFrames = frames;

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

    class Phase10PVPDropWebSocket extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        const socketID = state.nextSocketID++;
        this.__phase10PVPDropSocketID = socketID;
        this.addEventListener('message', (event) => capture('in', socketID, this.url || args[0], event.data));
      }

      send(data) {
        capture('out', this.__phase10PVPDropSocketID ?? 0, this.url, data);
        return super.send(data);
      }
    }

    window.WebSocket = Phase10PVPDropWebSocket;
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

async function openCommandSocket(client) {
  await client.page.evaluate(
    () =>
      new Promise((resolve, reject) => {
        if (window.__phase10PVPDropCommandSocket?.readyState === WebSocket.OPEN) return resolve(true);
        const socket = new WebSocket(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`);
        window.__phase10PVPDropCommandSocket = socket;
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
    request_id: `phase10-pvp-drop-${op.replace(/[^a-z0-9]+/gi, '-')}-${Date.now()}-${client.seq}`,
    op,
    payload,
    client_seq: client.seq++,
    v: 1,
  };
  return client.page.evaluate(
    ({ message }) =>
      new Promise((resolve, reject) => {
        const socket = window.__phase10PVPDropCommandSocket;
        if (!socket || socket.readyState !== WebSocket.OPEN) return reject(new Error('command WebSocket is not open'));
        const timeout = window.setTimeout(() => {
          socket.removeEventListener('message', onMessage);
          reject(new Error(`command response timeout: ${message.request_id}`));
        }, 12000);
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
    { message: request },
  );
}

async function enterPortalViaPosition(client, portalID, position, expectedMapKey, expectedDisplay) {
  await moveToPosition(client, position, 120, `${portalID} portal`, 90000);
  await resetWebSocketFrames(client);
  const payload = payloadOf(await send(client, 'portal.enter', { portal_id: portalID }), `portal.enter ${portalID}`);
  assert(payload.accepted === true, `portal accepted missing ${compact(payload)}`);
  assert(payload.to_public_map_key === expectedMapKey, `portal destination ${payload.to_public_map_key}, want ${expectedMapKey}`);
  const state = await waitSmoke(client, (candidate) => candidate.currentMap?.public_map_key === expectedMapKey, `${expectedDisplay} map state`, 15000);
  assertMap(state, expectedMapKey, expectedDisplay, expectedDisplay);
  await assertNoLeak(client, state, `${expectedMapKey} arrival`);
  return state;
}

async function moveToPosition(client, targetPosition, arriveDistance, label, timeoutMS) {
  assert(targetPosition, `${label} target position present`);
  const deadline = Date.now() + timeoutMS;
  while (Date.now() < deadline) {
    await waitSmoke(client, (state) => state.connectionStatus === 'connected' && Object.keys(state.pendingCommands ?? {}).length === 0, 'pending commands clear', 10000);
    const state = await smoke(client);
    const self = selfEntity(state);
    const position = positionNow(self, state);
    if (distance(position, targetPosition) <= arriveDistance) return state;

    const target = step(position, targetPosition, 1100);
    const response = await send(client, 'move_to', { target });
    if (response.ok !== true && response.error?.code === 'ERR_RATE_LIMITED') {
      await delay(125);
      continue;
    }
    payloadOf(response, 'move_to');
    const eta = Math.ceil((distance(position, target) / Math.max(1, self?.movement?.speed ?? state.stats?.speed ?? 180)) * 1000);
    await waitSmoke(
      client,
      (candidate) => {
        const pos = positionNow(selfEntity(candidate), candidate);
        return distance(pos, target) <= 35 || distance(pos, targetPosition) <= arriveDistance;
      },
      `movement to ${fmt(target)}`,
      Math.max(5000, eta + 5000),
    );
  }
  throw new Error(`Timed out before reaching ${label} at ${fmt(targetPosition)}`);
}

async function fightNPCUntilKilled(client, targetID) {
  let lastPayload = null;
  for (let attempt = 1; attempt <= 5; attempt += 1) {
    const payload = payloadOf(await send(client, 'combat.use_skill', { skill_id: 'basic_laser', target_id: targetID }), 'combat.use_skill');
    assert(payload.accepted === true, `combat.use_skill accepted ${compact(payload)}`);
    lastPayload = payload;
    if (payload.killed === true) return payload;
    await waitCombatCooldown(client, payload);
  }
  throw new Error(`combat.use_skill did not kill PvP-map NPC: ${compact(lastPayload)}`);
}

async function waitCombatCooldown(client, payload) {
  const readyAt = Number(payload.cooldown_ready_at_ms ?? 0);
  const delayMS = readyAt > Date.now() ? readyAt - Date.now() + 75 : 100;
  await delay(Math.min(Math.max(delayMS, 100), 5000));
  await waitSmoke(
    client,
    (state) => state.connectionStatus === 'connected' && (readyAt <= 0 || Date.now() >= readyAt || (state.serverNow ?? 0) >= readyAt),
    'basic_laser cooldown',
    Math.max(1500, Math.min(delayMS + 1500, 6500)),
  );
}

function payloadOf(response, label) {
  assert(response?.ok === true, `${label} failed: ${compact(response)}`);
  const payload = typeof response.payload === 'string' ? JSON.parse(response.payload) : response.payload;
  assert(payload && typeof payload === 'object', `${label} payload missing`);
  assertNoPayloadLeak(payload, `${label} payload`);
  return payload;
}

function responseDrop(combatPayload, label) {
  const drop = Array.isArray(combatPayload.drops)
    ? combatPayload.drops.find((entry) => entry?.item_id && Number(entry.quantity) > 0 && (entry.drop_id || entry.entity_id))
    : null;
  assert(drop, `${label} includes server loot drop ${compact(combatPayload)}`);
  assertNoPayloadLeak({ drop }, `${label} drop`);
  return {
    drop_id: drop.drop_id,
    entity_id: drop.entity_id,
    item_id: drop.item_id,
    quantity: Number(drop.quantity),
  };
}

function findLiveNPC(state) {
  return Object.values(state?.visibleEntities ?? {}).find((entity) => {
    if (entity?.entity_type !== 'npc' || !entity.position) return false;
    const hp = Number(entity.combat?.hp ?? 1);
    return hp > 0 && !['dead', 'destroyed', 'disabled'].includes(String(entity.combat?.status ?? 'active').toLowerCase());
  });
}

function findKnownDrop(state, expectedDrop) {
  const dropID = expectedDrop.drop_id ?? expectedDrop.entity_id;
  if (dropID && state?.knownLoot?.[dropID]) return state.knownLoot[dropID];
  return Object.values(state?.knownLoot ?? {}).find(
    (drop) => drop.item_id === expectedDrop.item_id && Number(drop.quantity) >= expectedDrop.quantity,
  );
}

function assertDropMatches(drop, expectedDrop, label) {
  assert(drop, `${label} present`);
  assert(drop.drop_id ?? drop.entity_id, `${label} has public drop id`);
  assert(drop.item_id === expectedDrop.item_id, `${label} item ${drop.item_id}, want ${expectedDrop.item_id}`);
  assert(Number(drop.quantity) >= expectedDrop.quantity, `${label} quantity ${drop.quantity}, want ${expectedDrop.quantity}`);
  assert(drop.position, `${label} position present`);
}

function assertCargoPickup(cargo, beforeCargo, drop, label) {
  assert(cargoIncludesPickup(cargo, beforeCargo, drop), `${label} includes picked ${drop.item_id} x${drop.quantity}`);
  assert((cargo.used ?? 0) > (beforeCargo?.used ?? 0), `${label} used cargo ${cargo.used}, before ${beforeCargo?.used ?? 0}`);
}

function cargoIncludesPickup(cargo, beforeCargo, drop) {
  const quantity = Number(drop.quantity);
  return drop.item_id && quantity > 0 && cargoQuantity(cargo, drop.item_id) >= cargoQuantity(beforeCargo, drop.item_id) + quantity;
}

function cargoQuantity(cargo, itemID) {
  return Number(cargo?.items?.find((item) => item.item_id === itemID)?.quantity ?? 0);
}

function hasUnhandledEventLog(state) {
  return (state?.commandLog ?? []).some((line) => typeof line?.text === 'string' && line.text.includes('Unhandled event'));
}

function assertMap(state, mapKey, display, label) {
  const map = state?.currentMap;
  assert(map?.public_map_key === mapKey, `${label} map ${map?.public_map_key}, want ${mapKey}`);
  assert(new RegExp(display, 'i').test(map.display_name ?? ''), `${label} display ${map.display_name}`);
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

function fmt(vec) {
  return `${Math.round(vec?.x ?? 0)},${Math.round(vec?.y ?? 0)}`;
}

async function smoke(client) {
  return client.page.evaluate(() => window.__SPACE_MORPG_SMOKE_STATE__ ?? null);
}

async function waitSmoke(client, predicate, description, timeoutMS) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < timeoutMS) {
    last = await smoke(client);
    if (last && predicate(last)) return last;
    await delay(100);
  }
  throw new Error(`Timed out waiting for ${description}. Last state: ${compact(last)}`);
}

async function resetWebSocketFrames(client) {
  await client.page.evaluate(() => {
    if (Array.isArray(window.__phase10PVPDropWebSocketFrames)) window.__phase10PVPDropWebSocketFrames.length = 0;
  });
}

async function webSocketFrames(client) {
  return client.page.evaluate(() => window.__phase10PVPDropWebSocketFrames ?? []);
}

async function assertWebSocketCanary(client, label) {
  const frames = await webSocketFrames(client);
  assert(frames.some((frame) => frame.path === '/ws' && frame.direction === 'in'), `${label} missing inbound /ws frames`);
  for (const frame of frames) {
    assert(frame.truncated !== true, `${label} websocket frame ${frame.index} exceeded scan limit`);
    if (frame.text) assertNoPayloadLeak(frame.text, `${label} ${frame.direction} websocket frame`);
  }
}

async function assertNoLeak(client, state, label) {
  assertNoPayloadLeak(state, `${label} smoke state`);
  const storageLeak = await browserStorageLeak(client);
  assert(!storageLeak, `${label} browser storage leaked ${storageLeak}`);
}

function assertNoPayloadLeak(value, label) {
  const text = typeof value === 'string' ? value : JSON.stringify(value);
  for (const token of leakTokens) {
    assert(!text.includes(token), `${label} leaked token ${token}`);
  }
  const key = forbiddenKey(value);
  assert(!key, `${label} leaked forbidden key ${key}`);
}

function forbiddenKey(value, path = []) {
  if (typeof value === 'string') {
    try {
      return forbiddenKey(JSON.parse(value), path);
    } catch {
      return '';
    }
  }
  if (Array.isArray(value)) {
    for (let index = 0; index < value.length; index += 1) {
      const found = forbiddenKey(value[index], path.concat(String(index)));
      if (found) return found;
    }
    return '';
  }
  if (!value || typeof value !== 'object') return '';
  for (const [key, child] of Object.entries(value)) {
    if (forbiddenFieldNames.has(key)) return path.concat(key).join('.');
    const found = forbiddenKey(child, path.concat(key));
    if (found) return found;
  }
  return '';
}

async function browserStorageLeak(client) {
  return client.page.evaluate((tokens) => {
    const haystacks = [document.body?.innerText ?? '', document.cookie ?? ''];
    for (let index = 0; index < localStorage.length; index += 1) {
      const key = localStorage.key(index);
      haystacks.push(`${key}=${key ? localStorage.getItem(key) : ''}`);
    }
    for (let index = 0; index < sessionStorage.length; index += 1) {
      const key = sessionStorage.key(index);
      haystacks.push(`${key}=${key ? sessionStorage.getItem(key) : ''}`);
    }
    const text = haystacks.join('\n');
    return tokens.find((token) => text.includes(token)) ?? '';
  }, leakTokens);
}

function assertProcessLogCanary(processes) {
  for (const proc of processes) {
    for (const line of proc.lines ?? []) {
      for (const token of leakTokens) {
        assert(!line.includes(token), `${proc.name} log leaked ${token}: ${line}`);
      }
    }
  }
}

async function waitHTTP(url, label, proc) {
  const started = Date.now();
  let lastError = null;
  while (Date.now() - started < 30000) {
    if (proc.exitCode !== null) {
      throw new Error(`${label} exited before ready with code ${proc.exitCode}: ${proc.lines.join('\n')}`);
    }
    try {
      const response = await fetch(url);
      if (response.ok) return;
    } catch (error) {
      lastError = error;
    }
    await delay(100);
  }
  throw new Error(`${label} not ready at ${url}: ${lastError?.message ?? 'timeout'}`);
}

function child(name, command, args, cwd, env) {
  const proc = spawn(command, args, {
    cwd,
    detached: true,
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  proc.name = name;
  proc.lines = [];
  const capture = (stream, prefix) => {
    stream.setEncoding('utf8');
    stream.on('data', (chunk) => {
      for (const line of String(chunk).split(/\r?\n/)) {
        if (!line) continue;
        proc.lines.push(`[${name}:${prefix}] ${line}`);
        if (proc.lines.length > maxProcessLogLines) proc.lines.shift();
      }
    });
  };
  capture(proc.stdout, 'stdout');
  capture(proc.stderr, 'stderr');
  return proc;
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

function freePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.unref();
    server.on('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      const port = typeof address === 'object' && address ? address.port : 0;
      server.close(() => resolve(port));
    });
  });
}

function compact(value) {
  return JSON.stringify(value).slice(0, 2000);
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});

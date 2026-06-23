#!/usr/bin/env node
import { spawn } from 'node:child_process';
import net from 'node:net';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const clientDir = resolve(scriptDir, '../..');
const repoRoot = resolve(clientDir, '..');
const maxProcessLogLines = 5000;
const commandTimeoutMS = 12000;
const desktopViewport = { width: 1440, height: 900 };
const useBuiltClientServer = process.env.PHASE10_BUILT_CLIENT === '1';
const eastGateTarget = { x: 9800, y: 5000 };
const skirmishGateTarget = { x: 9800, y: 5000 };
const seedOrigin = { x: 5400, y: 5200 };
const observerStealthStaging = { x: 4800, y: 5200 };
const observerPoint = { x: 5520, y: 5200 };
const lureChasePoint = { x: 5700, y: 5200 };
const lureLeashBreakPoint = { x: 6350, y: 5200 };
const seededNPCSpeed = 90;
const seededAggroRadius = 520;
const seededLeashDistance = 900;

const leakTokens = [
  'map_1_1',
  'map_1_2',
  'map_1_3',
  'internal_map_id',
  'destination_map_id',
  'source_map_id',
  'worker_id',
  'map_worker_id',
  'destination_worker',
  'origin_worker',
  'spawn_id',
  'destination_spawn_id',
  'source_spawn_id',
  'spawn_point',
  'spawn_position',
  'gameplay_seed',
  'procedural_seed',
  'world_seed',
  'spawn_candidates',
  'future_spawn',
  'enemy_pool_id',
  'spawn_area_id',
  'stat_template_id',
  'drop_profile_id',
  'aggro_profile_id',
  'leash_profile_id',
  'enemy_pool',
  'spawn_area',
  'stat_template',
  'drop_profile',
  'aggro_profile',
  'leash_profile',
  'border_raider_drone_pool',
  'border_raider_drone_area',
  'border_raider_drone_level_2',
  'border_raider_drone_salvage',
  'border_raider_drone_hunter',
  'border_raider_drone_patrol',
  'border_raider_salvage',
  'aggro_target_entity_id',
  'aggro_acquired_at',
  'aggro_target_last_seen_at',
  'last_aggro_tick_at',
  'leash_origin',
  'aggro_radius',
  'assist_radius',
  'target_memory',
  'safe_zone_attack_policy',
  'leash_distance',
  'reset_on_break',
  'map_max_alive',
  'pool_max_alive',
  'kill_respawn_delay',
  'loot_table',
  'scan_roll',
  'loot_roll',
  'player_id',
  'session_id',
  'password',
  'password_hash',
  'session_token',
  'raw_token',
  'reset_secret',
  'correct-password',
  'demo_npc',
  'fixture_npc',
  'mock_npc',
  'fake_npc',
  'mock_wallet',
  'mock_cargo',
];

async function main() {
  const serverPort = await freePort();
  const serverTarget = `http://127.0.0.1:${serverPort}`;
  const clientPort = useBuiltClientServer ? 0 : await freePort();
  const clientOrigin = useBuiltClientServer ? serverTarget : `http://127.0.0.1:${clientPort}`;
  const goEnv = {
    GAME_SERVER_ADDR: `127.0.0.1:${serverPort}`,
    GAME_ALLOWED_ORIGINS: clientOrigin,
  };
  if (useBuiltClientServer) {
    goEnv.GAME_CLIENT_STATIC_DIR = 'client/dist';
  }
  const goServer = child('go-server', 'go', ['run', './cmd/game-server'], repoRoot, goEnv);
  let viteServer;
  let browser;
  const clients = [];

  try {
    await waitHTTP(`${serverTarget}/healthz`, 'Go server', goServer);
    if (useBuiltClientServer) {
      await waitHTTP(`${clientOrigin}/?smoke=1`, 'built client', goServer);
    } else {
      viteServer = child(
        'vite',
        'npm',
        ['--cache', '/tmp/gameproject-npm-cache', 'run', 'dev', '--', '--port', String(clientPort), '--strictPort'],
        clientDir,
        { GAME_CLIENT_PROXY_TARGET: serverTarget },
      );
      await waitHTTP(`${clientOrigin}/?smoke=1`, 'Vite client', viteServer);
    }

    browser = await chromium.launch();
    const nonce = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const observer = await newClient(browser, clientOrigin, {
      label: 'observer',
      email: `phase10-enemy-observer-${nonce}@example.test`,
      callsign: `P10EO-${nonce.slice(-6)}`,
    });
    clients.push(observer);
    const lure = await newClient(browser, clientOrigin, {
      label: 'lure',
      email: `phase10-enemy-lure-${nonce}@example.test`,
      callsign: `P10EL-${nonce.slice(-6)}`,
    });
    clients.push(lure);

    await assertAuthenticatedOrigin(observer);
    await assertAuthenticatedOrigin(lure);
    await openCommandSocket(observer);
    await openCommandSocket(lure);

    await travelToBorderSkirmish([observer, lure]);

    await moveToPosition(observer, observerStealthStaging, 70, 'observer stealth staging', 60000);
    await toggleStealth(observer, true);
    await moveToPosition(observer, observerPoint, 45, 'stealthed observer seed watch', 30000);
    const observerAtSeed = await waitSmoke(
      observer,
      (state) => state.currentMap?.public_map_key === '1-3' && findSeedHostileNPC(state),
      'hostile NPC near Border Skirmish seed area',
      15000,
    );
    const npc = findSeedHostileNPC(observerAtSeed);
    assertSeedNPCPublicShape(npc);
    await assertNoLeak(observer, observerAtSeed, 'observer seeded hostile NPC');
    await assertStealthedObserverNotTargeted(observer, npc.entity_id);

    await moveToPosition(lure, lureChasePoint, 35, 'lure inside seeded aggro radius', 70000);
    const chase = await waitForNPCChasingLure(observer, lure, npc.entity_id);
    assert(distance(chase.lurePosition, seedOrigin) < seededAggroRadius, `lure position ${fmt(chase.lurePosition)} outside seeded aggro radius`);
    assertNoLeakPayload({ npc: chase.npc, lure_position: chase.lurePosition }, 'browser chase proof');

    await waitForNPCAwayFromOrigin(observer, npc.entity_id, 180, 12000);
    await moveToPosition(lure, lureLeashBreakPoint, 35, 'lure beyond seeded leash distance', 20000);
    const reset = await waitForNPCLeashReset(observer, lure, npc.entity_id);
    assert(distance(reset.lurePosition, seedOrigin) > seededLeashDistance, `lure position ${fmt(reset.lurePosition)} did not break seeded leash`);
    assertNoLeakPayload({ npc: reset.npc, lure_position: reset.lurePosition }, 'browser leash reset proof');

    await assertNoLeak(observer, reset.observerState, 'observer leash reset state');
    await assertNoLeak(lure, reset.lureState, 'lure leash reset state');
    await assertWebSocketCanary([observer, lure]);
    await assertStorageCanary([observer, lure]);
    assertProcessLogCanary([goServer, viteServer]);

    console.log(
      `phase10-enemy-aggro smoke ok map=1-3 npc=${npc.entity_id} chase_target=${fmt(chase.npc.movement.target)} leash_reset_target=${fmt(
        reset.npc.movement?.target ?? reset.npc.position,
      )}`,
    );
  } finally {
    for (const client of clients) {
      await client.page.evaluate(() => window.__phase10EnemyCommandSocket?.close()).catch(() => {});
      await client.context.close().catch(() => {});
    }
    if (browser) await browser.close().catch(() => {});
    if (viteServer) await stop(viteServer);
    await stop(goServer);
  }
}

async function newClient(browser, clientOrigin, { label, email, callsign }) {
  const context = await browser.newContext({ viewport: desktopViewport });
  const page = await context.newPage();
  const client = { context, page, label, email, callsign, seq: 1 };
  await installWebSocketCanary(client);
  await page.goto(`${clientOrigin}/?smoke=1`, { waitUntil: 'domcontentloaded' });
  await register(client, email, 'correct-password', callsign);
  return client;
}

async function installWebSocketCanary(client) {
  await client.page.addInitScript((clientLabel) => {
    if (window.__phase10EnemyWebSocketCanaryInstalled) return;
    window.__phase10EnemyWebSocketCanaryInstalled = true;

    const NativeWebSocket = window.WebSocket;
    const maxTextLength = 1_000_000;
    const frames = [];
    const state = { nextSocketID: 1, nextFrameIndex: 0 };
    window.__phase10EnemyWebSocketFrames = frames;

    function safePath(url) {
      try {
        return new URL(String(url), window.location.href).pathname;
      } catch {
        return '';
      }
    }

    function captureText(data) {
      if (typeof data === 'string') {
        return {
          kind: 'text',
          text: data.length > maxTextLength ? data.slice(0, maxTextLength) : data,
          text_length: data.length,
          truncated: data.length > maxTextLength,
        };
      }
      if (data instanceof ArrayBuffer) {
        return { kind: 'arraybuffer', text: '', text_length: 0, byte_length: data.byteLength, truncated: false };
      }
      if (ArrayBuffer.isView(data)) {
        return { kind: 'arraybuffer-view', text: '', text_length: 0, byte_length: data.byteLength, truncated: false };
      }
      return { kind: Object.prototype.toString.call(data), text: '', text_length: 0, byte_length: 0, truncated: false };
    }

    function capture(direction, socketID, url, data) {
      const frameText = captureText(data);
      frames.push({
        client_label: clientLabel,
        direction,
        index: state.nextFrameIndex++,
        socket_id: socketID,
        path: safePath(url),
        ...frameText,
      });
    }

    class Phase10EnemyWebSocket extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        const socketID = state.nextSocketID++;
        this.__phase10EnemySocketID = socketID;
        this.addEventListener('message', (event) => capture('in', socketID, this.url || args[0], event.data));
      }

      send(data) {
        capture('out', this.__phase10EnemySocketID ?? 0, this.url, data);
        return super.send(data);
      }
    }

    window.WebSocket = Phase10EnemyWebSocket;
  }, client.label);
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

async function assertAuthenticatedOrigin(client) {
  const state = await waitSmoke(client, originReady, 'authenticated Origin state', 30000);
  assertRealAuthenticated(state, client.callsign, client.label);
  assertMap(state, '1-1', 'Origin', `${client.label} origin`);
  await assertNoLeak(client, state, `${client.label} origin`);
}

function originReady(state) {
  return state?.connectionStatus === 'connected' && state.auth?.session?.authenticated === true && state.currentMap?.public_map_key === '1-1';
}

function assertRealAuthenticated(state, callsign, label) {
  assert(state.auth?.mode === 'real', `${label} auth mode ${state.auth?.mode}`);
  assert(state.auth?.session?.authenticated === true, `${label} session not authenticated`);
  assert(state.auth?.session?.player?.callsign === callsign, `${label} callsign ${JSON.stringify(state.auth?.session?.player)}`);
  assert(!state.auth?.session?.player?.player_id, `${label} auth leaked player_id`);
}

function assertMap(state, mapKey, display, label) {
  const map = needMap(state);
  assert(map.public_map_key === mapKey, `${label} map ${map.public_map_key}, want ${mapKey}`);
  assert(new RegExp(display, 'i').test(map.display_name ?? ''), `${label} display ${map.display_name}`);
  assertBounds(map.bounds);
}

function assertBounds(bounds) {
  assert(bounds?.min_x === 0 && bounds?.min_y === 0 && bounds?.max_x === 10000 && bounds?.max_y === 10000, `bounds ${JSON.stringify(bounds)}`);
}

function needMap(state) {
  assert(state?.currentMap, 'currentMap present');
  return state.currentMap;
}

async function openCommandSocket(client) {
  await client.page.evaluate(
    () =>
      new Promise((resolve, reject) => {
        if (window.__phase10EnemyCommandSocket?.readyState === WebSocket.OPEN) return resolve(true);
        const socket = new WebSocket(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`);
        window.__phase10EnemyCommandSocket = socket;
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
    request_id: `phase10-enemy-${client.label}-${op.replace(/[^a-z0-9]+/gi, '-')}-${Date.now()}-${client.seq}`,
    op,
    payload,
    client_seq: client.seq++,
    v: 1,
  };
  return client.page.evaluate(
    ({ message, timeoutMS }) =>
      new Promise((resolve, reject) => {
        const socket = window.__phase10EnemyCommandSocket;
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

async function travelToBorderSkirmish(clients) {
  await Promise.all(clients.map((client) => moveToPosition(client, eastGateTarget, 120, `${client.label} east_gate`, 90000)));
  await Promise.all(clients.map((client) => enterPortal(client, 'east_gate', '1-2', 'Outer Ring')));
  await assertBothOnMap(clients, '1-2', 'Outer Ring');

  await Promise.all(clients.map((client) => moveToPosition(client, skirmishGateTarget, 120, `${client.label} skirmish_gate`, 90000)));
  await Promise.all(clients.map((client) => enterPortal(client, 'skirmish_gate', '1-3', 'Border Skirmish')));
  await assertBothOnMap(clients, '1-3', 'Border Skirmish');
}

async function enterPortal(client, portalID, expectedMapKey, expectedDisplay) {
  const before = await smoke(client);
  const portal = findVisiblePortal(before, portalID);
  assert(portal, `${client.label} visible portal ${portalID}`);
  const response = await send(client, 'portal.enter', { portal_id: portalID });
  assertNoLeakPayload(response, `${client.label} portal ${portalID} response`);
  const payload = payloadOf(response, `${client.label} portal.enter ${portalID}`);
  assert(payload.accepted === true, `${client.label} portal accepted ${JSON.stringify(payload)}`);
  assert(payload.to_public_map_key === expectedMapKey, `${client.label} portal destination ${payload.to_public_map_key}`);
  assert(payload.snapshot?.map?.public_map_key === expectedMapKey, `${client.label} portal snapshot map ${JSON.stringify(payload.snapshot?.map)}`);
  const transferred = await waitSmoke(
    client,
    (state) => state.currentMap?.public_map_key === expectedMapKey,
    `${expectedDisplay} map state`,
    15000,
  );
  assertMap(transferred, expectedMapKey, expectedDisplay, `${client.label} ${expectedDisplay}`);
  return transferred;
}

async function assertBothOnMap(clients, mapKey, display) {
  await Promise.all(
    clients.map(async (client) => {
      const state = await waitSmoke(client, (candidate) => candidate.currentMap?.public_map_key === mapKey, `on ${mapKey}`, 10000);
      assertMap(state, mapKey, display, `${client.label} ${display}`);
      await assertNoLeak(client, state, `${client.label} ${mapKey}`);
    }),
  );
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
    assertNoLeakPayload(response, `${client.label} move_to ${label} response`);
    if (response.ok !== true && response.error?.code === 'ERR_RATE_LIMITED') {
      await delay(125);
      continue;
    }
    ok(response, `${client.label} move_to`);
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
  throw new Error(`${client.label} timed out before reaching ${label} at ${fmt(targetPosition)}`);
}

async function toggleStealth(client, enabled) {
  const response = await send(client, 'stealth.toggle', { enabled });
  assertNoLeakPayload(response, `${client.label} stealth.toggle response`);
  const payload = payloadOf(response, `${client.label} stealth.toggle`);
  assert(payload.accepted === true, `${client.label} stealth accepted ${JSON.stringify(payload)}`);
  assert(payload.stealth?.enabled === enabled, `${client.label} stealth enabled ${JSON.stringify(payload)}`);
  await waitSmoke(
    client,
    (state) => selfEntity(state)?.status_flags?.includes('stealthed') === enabled,
    `stealth ${enabled ? 'enabled' : 'disabled'} status flag`,
    10000,
  );
}

async function assertStealthedObserverNotTargeted(observer, npcID) {
  await delay(1500);
  const state = await smoke(observer);
  const npc = state?.visibleEntities?.[npcID];
  const observerPosition = positionNow(selfEntity(state), state);
  assert(isLiveHostileNPC(npc), `seed NPC ${npcID} visible after observer stealth`);
  assert(
    !npc.movement?.moving || !near(npc.movement.target, observerPosition, 140),
    `stealthed observer was targeted by seed NPC movement ${JSON.stringify(npc.movement)}`,
  );
}

async function waitForNPCChasingLure(observer, lure, npcID) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < 15000) {
    const [observerState, lureState] = await Promise.all([smoke(observer), smoke(lure)]);
    const npc = observerState?.visibleEntities?.[npcID];
    const lureSelf = selfEntity(lureState);
    const lurePosition = lureSelf?.position ? positionNow(lureSelf, lureState) : null;
    last = { observerState, lureState, npc, lurePosition };
    if (!lureState || !lureSelf?.position) {
      await delay(100);
      continue;
    }
    if (
      observerState?.currentMap?.public_map_key === '1-3' &&
      lureState?.currentMap?.public_map_key === '1-3' &&
      isLiveHostileNPC(npc) &&
      npc.movement?.moving === true &&
      nearNumber(npc.movement.speed, seededNPCSpeed, 0.001) &&
      near(npc.movement.target, lurePosition, 170)
    ) {
      return { observerState, lureState, npc, lurePosition };
    }
    await delay(100);
  }
  throw new Error(`Timed out waiting for seed NPC ${npcID} to chase lure. Last state: ${compactPair(last)}`);
}

async function waitForNPCAwayFromOrigin(observer, npcID, minDistance, timeoutMS) {
  return waitSmoke(
    observer,
    (state) => {
      const npc = state.visibleEntities?.[npcID];
      return isLiveHostileNPC(npc) && distance(positionNow(npc, state), seedOrigin) >= minDistance;
    },
    `seed NPC ${npcID} ${minDistance}u away from leash origin`,
    timeoutMS,
  );
}

async function waitForNPCLeashReset(observer, lure, npcID) {
  const started = Date.now();
  let last = null;
  while (Date.now() - started < 15000) {
    const [observerState, lureState] = await Promise.all([smoke(observer), smoke(lure)]);
    const npc = observerState?.visibleEntities?.[npcID];
    const lureSelf = selfEntity(lureState);
    const lurePosition = lureSelf?.position ? positionNow(lureSelf, lureState) : null;
    last = { observerState, lureState, npc, lurePosition };
    if (!lureState || !lureSelf?.position) {
      await delay(100);
      continue;
    }
    const npcTarget = npc?.movement?.target ?? null;
    const npcPosition = isLiveHostileNPC(npc) ? positionNow(npc, observerState) : null;
    const returningToOrigin =
      npc?.movement?.moving === true &&
      nearNumber(npc.movement.speed, seededNPCSpeed, 0.001) &&
      near(npcTarget, seedOrigin, 35) &&
      !near(npcTarget, lurePosition, 300);
    const alreadyHome = !npc?.movement?.moving && near(npcPosition, seedOrigin, 45) && !near(npcPosition, lurePosition, 300);
    if (
      observerState?.currentMap?.public_map_key === '1-3' &&
      lureState?.currentMap?.public_map_key === '1-3' &&
      isLiveHostileNPC(npc) &&
      distance(lurePosition, seedOrigin) > seededLeashDistance &&
      (returningToOrigin || alreadyHome)
    ) {
      return { observerState, lureState, npc, lurePosition };
    }
    await delay(100);
  }
  throw new Error(`Timed out waiting for seed NPC ${npcID} leash reset. Last state: ${compactPair(last)}`);
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
  throw new Error(`Timed out waiting for ${client.label} ${description}. Last state: ${compact(last)}`);
}

function selfEntity(state) {
  const entities = Object.values(state?.visibleEntities ?? {});
  return entities.find((entity) => entity.status_flags?.includes('self')) ?? entities.find((entity) => entity.entity_type === 'player') ?? null;
}

function findVisiblePortal(state, portalID) {
  return (state?.currentMap?.visible_portals ?? []).find((portal) => portal.portal_id === portalID) ?? null;
}

function findSeedHostileNPC(state) {
  return (
    Object.values(state?.visibleEntities ?? {}).find(
      (entity) => isLiveHostileNPC(entity) && near(entity.position, seedOrigin, 320),
    ) ?? null
  );
}

function isLiveHostileNPC(entity) {
  if (entity?.entity_type !== 'npc' || !entity.position) return false;
  if (!entity.status_flags?.includes('hostile') && entity.display?.disposition !== 'hostile') return false;
  if (!entity.combat) return true;
  const hp = Number(entity.combat.hp ?? 0);
  if (!(hp > 0)) return false;
  return !['dead', 'destroyed', 'disabled'].includes(String(entity.combat.status ?? 'active').toLowerCase());
}

function assertSeedNPCPublicShape(npc) {
  assert(isLiveHostileNPC(npc), `seed hostile NPC present ${JSON.stringify(npc)}`);
  assert(npc.entity_id, 'seed NPC has public entity id');
  assert(near(npc.position, seedOrigin, 320), `seed NPC position ${fmt(npc.position)}, want near ${fmt(seedOrigin)}`);
  assertNoLeakPayload({ npc }, 'seed NPC public payload');
}

function positionNow(entity, state) {
  assert(entity?.position, 'entity position present');
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

function near(a, b, tolerance) {
  return distance(a, b) <= tolerance;
}

function nearNumber(actual, expected, tolerance) {
  return Math.abs(Number(actual) - expected) <= tolerance;
}

function payloadOf(response, label) {
  ok(response, label);
  const payload = typeof response.payload === 'string' ? JSON.parse(response.payload) : response.payload;
  assert(payload && typeof payload === 'object', `${label} payload present`);
  return payload;
}

async function assertNoLeak(client, state, label) {
  const body = await client.page.locator('body').innerText({ timeout: 5000 });
  assert(!body.includes('Unhandled event'), `${label} DOM has unhandled event log`);
  for (const token of leakTokens) {
    assert(!body.includes(token), `${label} DOM leaked ${token}`);
  }
  assertNoLeakPayload(state, `${label} smoke state`);
  const browserLeak = await browserStorageLeak(client, leakTokens);
  assert(!browserLeak, `${label} browser storage leaked ${browserLeak}`);
}

async function assertWebSocketCanary(clients) {
  for (const client of clients) {
    const frames = await websocketFrames(client);
    const inbound = frames.filter((frame) => frame.direction === 'in').length;
    const outbound = frames.filter((frame) => frame.direction === 'out').length;
    assert(inbound > 0, `${client.label} WebSocket canary captured no inbound frames`);
    assert(outbound > 0, `${client.label} WebSocket canary captured no outbound frames`);
    for (const frame of frames) assertNoWebSocketFrameLeak(frame);
  }
}

async function assertStorageCanary(clients) {
  for (const client of clients) {
    const browserLeak = await browserStorageLeak(client, leakTokens);
    assert(!browserLeak, `${client.label} browser storage or cookie leaked ${browserLeak}`);
  }
}

async function websocketFrames(client) {
  return client.page.evaluate(() =>
    (window.__phase10EnemyWebSocketFrames ?? []).map((frame) => ({
      client_label: frame.client_label,
      direction: frame.direction,
      index: frame.index,
      socket_id: frame.socket_id,
      path: frame.path,
      kind: frame.kind,
      text: frame.text,
      text_length: frame.text_length,
      truncated: frame.truncated,
    })),
  );
}

function assertNoWebSocketFrameLeak(frame) {
  const surface = `${frame.client_label}.websocket.${frame.direction}[${frame.index}]`;
  assert(frame.truncated !== true, `${surface} text exceeded canary scan limit`);
  if (!frame.text) return;
  for (const token of leakTokens) {
    assert(!frame.text.includes(token), `${surface} leaked token ${token}`);
  }
  const parsed = parseFrameJSON(frame.text);
  if (parsed === null) return;
  const key = forbiddenKey(parsed);
  assert(!key, `${surface} leaked forbidden key ${key}`);
}

function assertNoLeakPayload(payload, label) {
  const json = JSON.stringify(payload);
  for (const token of leakTokens) {
    assert(!json.includes(token), `${label} leaked ${token}`);
  }
  const key = forbiddenKey(payload);
  assert(!key, `${label} leaked forbidden key ${key}`);
}

function forbiddenKey(value, path = []) {
  if (Array.isArray(value)) {
    for (let i = 0; i < value.length; i += 1) {
      const found = forbiddenKey(value[i], [...path, String(i)]);
      if (found) return found;
    }
    return null;
  }
  if (!value || typeof value !== 'object') return null;
  const forbidden = new Set(leakTokens.map((token) => token.toLowerCase()));
  for (const [key, child] of Object.entries(value)) {
    if (forbidden.has(key.toLowerCase())) return [...path, key].join('.');
    const found = forbiddenKey(child, [...path, key]);
    if (found) return found;
  }
  return null;
}

function parseFrameJSON(text) {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}

async function browserStorageLeak(client, tokens) {
  const storageLeak = await client.page.evaluate((scanTokens) => {
    const scanText = (surface, text, key = '') => {
      const haystack = String(text ?? '');
      for (const token of scanTokens) {
        if (haystack.includes(token)) return `${surface}${key ? `:${key}` : ''}:${token}`;
      }
      return null;
    };
    const scanStorage = (surface, storage) => {
      for (let i = 0; i < storage.length; i += 1) {
        const key = storage.key(i) ?? '';
        const keyLeak = scanText(`${surface}.key`, key);
        if (keyLeak) return keyLeak;
        const valueLeak = scanText(`${surface}.value`, storage.getItem(key), key);
        if (valueLeak) return valueLeak;
      }
      return null;
    };
    return scanStorage('localStorage', window.localStorage) ?? scanStorage('sessionStorage', window.sessionStorage) ?? scanText('document.cookie', document.cookie);
  }, tokens);
  if (storageLeak) return storageLeak;

  const cookies = await client.context.cookies();
  for (const cookie of cookies) {
    for (const token of tokens) {
      if (cookie.name.includes(token)) return `cookie.name:${cookie.name}:${token}`;
      if (cookie.value.includes(token)) return `cookie.value:${cookie.name}:${token}`;
    }
  }
  return null;
}

function assertProcessLogCanary(processes) {
  for (const proc of processes.filter(Boolean)) {
    for (const line of proc.logLines ?? []) {
      for (const token of leakTokens) {
        if (line.text.includes(token)) {
          throw new Error(`${line.process}:${line.tag}[${line.index}] leaked token ${token}`);
        }
      }
    }
  }
}

function child(name, command, args, cwd, env) {
  const proc = spawn(command, args, {
    cwd,
    detached: true,
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  proc.output = [];
  proc.logLines = [];
  proc.nextLogLineIndex = 0;
  const collect = (chunk, tag) => {
    for (const line of chunk.toString().split(/\r?\n/).filter(Boolean)) {
      proc.output.push(`[${name}:${tag}] ${line}`);
      proc.logLines.push({ process: name, tag, index: proc.nextLogLineIndex++, text: line });
    }
    if (proc.output.length > 80) proc.output.splice(0, proc.output.length - 80);
    if (proc.logLines.length > maxProcessLogLines) proc.logLines.splice(0, proc.logLines.length - maxProcessLogLines);
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
      // Listener not ready.
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
  return Promise.race([new Promise((resolve) => proc.once('exit', () => resolve(true))), delay(timeoutMS).then(() => false)]);
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
  if (!vec) return 'null';
  return `${Math.round(vec.x)},${Math.round(vec.y)}`;
}

function compact(state) {
  if (!state) return 'null';
  return JSON.stringify({
    connectionStatus: state.connectionStatus,
    currentMap: state.currentMap,
    self: selfEntity(state),
    entities: Object.values(state.visibleEntities ?? {}).map((entity) => ({
      id: entity.entity_id,
      type: entity.entity_type,
      flags: entity.status_flags,
      label: entity.display?.label,
      position: entity.position,
      movement: entity.movement,
      hp: entity.combat?.hp,
      status: entity.combat?.status,
    })),
    pending: Object.keys(state.pendingCommands ?? {}),
    transfer: state.mapTransfer,
    lastError: state.lastError,
  });
}

function compactPair(last) {
  if (!last) return 'null';
  return JSON.stringify({
    observer: compact(last.observerState),
    lure: compact(last.lureState),
    npc: last.npc,
    lurePosition: last.lurePosition,
  });
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

main().catch((error) => {
  console.error(error?.stack ?? error);
  process.exitCode = 1;
});

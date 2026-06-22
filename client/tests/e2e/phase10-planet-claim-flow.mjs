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
const desktopViewport = { width: 1440, height: 900 };
const commandTimeoutMS = 12000;
const claimRangeArriveDistance = 220;

const leakTokens = [
  'map_1_1',
  'map_1_2',
  'map_1_3',
  'internal_map_id',
  'destination_map_id',
  'source_map_id',
  'spawn_id',
  'candidate_key',
  'scan_roll',
  'scan_cell',
  'scan_candidate',
  'scan_candidates',
  'candidate_data',
  'planet_candidate',
  'procedural_seed',
  'gameplay_seed',
  'world_seed',
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
  const clientPort = await freePort();
  const clientOrigin = `http://127.0.0.1:${clientPort}`;
  const serverTarget = `http://127.0.0.1:${serverPort}`;
  const goServer = child('go-server', 'go', ['run', './cmd/game-server'], repoRoot, {
    GAME_SERVER_ADDR: `127.0.0.1:${serverPort}`,
    GAME_ALLOWED_ORIGINS: clientOrigin,
    GAME_DEV_MODE: '1',
    GAME_E2E_PLANET_CLAIM_SEED: '1',
  });
  let viteServer;
  let browser;
  let client;

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
    const context = await browser.newContext({ viewport: desktopViewport });
    const page = await context.newPage();
    const nonce = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    client = {
      context,
      page,
      label: 'claim',
      email: `phase10-claim-${nonce}@example.test`,
      callsign: `P10C-${nonce.slice(-8)}`,
      seq: 1,
    };
    await installWebSocketCanary(client);
    await page.goto(`${clientOrigin}/?smoke=1`, { waitUntil: 'domcontentloaded' });
    await register(client, client.email, 'correct-password', client.callsign);

    const origin = await waitSmoke(client, originReady, 'authenticated Origin state', 30000);
    assert(origin.auth?.session?.authenticated === true, 'authenticated session visible in smoke state');
    assert(origin.auth?.session?.player?.callsign === client.callsign, 'callsign reconciled from server session');
    assert(origin.currentMap?.public_map_key === '1-1', `origin map = ${origin.currentMap?.public_map_key}`);
    await waitSmoke(client, (state) => inventoryQuantity(state.inventory, 'x_core') === 1, 'E2E X Core inventory seed', 20000);
    await assertNoLeak(client, await smoke(client), 'origin-before-scan');

    await openCommandSocket(client);
    await resetWebSocketFrames(client);
    await clickFirstEnabled(client, 'button[data-action="scan"]', 'Scan button');
    const withPlanet = await waitSmoke(
      client,
      (state) => discoveredPlanetID(state) !== '',
      'browser scan discovered planet',
      30000,
    );
    const planetID = discoveredPlanetID(withPlanet);
    assert(planetID, `discovered planet id missing in ${compact(withPlanet.planetIntel)}`);
    assertNoPayloadLeak(withPlanet.planetIntel, 'planet intel after scan');

    const detailButton = `button[data-action="planet-detail"][data-planet-id=${cssString(planetID)}]`;
    await client.page.locator(detailButton).first().click({ timeout: 10000 });
    const withDetail = await waitSmoke(
      client,
      (state) => state.planetIntel?.selectedPlanet?.planet_id === planetID && state.planetIntel.selectedPlanet.coordinates,
      'selected planet detail coordinates',
      15000,
    );
    const coordinates = withDetail.planetIntel.selectedPlanet.coordinates;
    assert(coordinates, `selected planet coordinates missing ${compact(withDetail.planetIntel.selectedPlanet)}`);
    await moveToPosition(client, coordinates, claimRangeArriveDistance, `planet ${planetID}`, 90000);
    await waitSmoke(
      client,
      (state) => distance(positionNow(selfEntity(state), state), coordinates) <= claimRangeArriveDistance,
      'ship near claimed planet',
      10000,
    );

    await resetWebSocketFrames(client);
    await clickFirstEnabled(
      client,
      `button[data-action="planet-claim"][data-planet-id=${cssString(planetID)}]`,
      'Claim button',
    );
    const claimFrames = await waitForUIClaimResponse(client, planetID, 15000);
    assertClaimRequestPayload(claimFrames.request, planetID);
    assertClaimResponsePayload(claimFrames.response, planetID);
    assertNoPayloadLeak(claimFrames.response, 'claim response frame');
    assertNoPayloadLeak(claimFrames.event, 'planet.claimed frame');

    const claimed = await waitSmoke(
      client,
      (state) =>
        planetOwnerStatus(state, planetID) === 'owned_by_you' &&
        selectedPlanetOwnerStatus(state, planetID) === 'owned_by_you' &&
        productionInitialized(state, planetID) &&
        inventoryQuantity(state.inventory, 'x_core') === 0 &&
        !hasPendingOp(state, 'discovery.claim_planet') &&
        !hasUnhandledEventLog(state),
      'claimed planet reconciliation',
      15000,
    );
    assert(productionInitialized(claimed, planetID), `production missing for ${planetID}`);
    assert(inventoryQuantity(claimed.inventory, 'x_core') === 0, `x_core still present ${compact(claimed.inventory)}`);
    await assertNoLeak(client, claimed, 'claimed');
    await assertWebSocketCanary(client, 'claim');
    assertProcessLogCanary([goServer, viteServer]);

    console.log(`phase10-planet-claim smoke ok planet=${planetID} owner=${planetOwnerStatus(claimed, planetID)}`);
  } finally {
    if (client) {
      await client.page.evaluate(() => window.__phase10ClaimCommandSocket?.close()).catch(() => {});
      await client.context.close().catch(() => {});
    }
    if (browser) await browser.close().catch(() => {});
    if (viteServer) await stop(viteServer);
    await stop(goServer);
  }
}

async function installWebSocketCanary(client) {
  await client.page.addInitScript(() => {
    if (window.__phase10ClaimWebSocketCanaryInstalled) return;
    window.__phase10ClaimWebSocketCanaryInstalled = true;

    const NativeWebSocket = window.WebSocket;
    const maxTextLength = 1_000_000;
    const frames = [];
    const clicks = [];
    const state = { nextSocketID: 1, nextFrameIndex: 0 };
    window.__phase10ClaimWebSocketFrames = frames;
    window.__phase10ClaimClicks = clicks;

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
        direction,
        index: state.nextFrameIndex++,
        socket_id: socketID,
        path: safePath(url),
        ...frameText,
      });
    }

    class Phase10ClaimWebSocket extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        const socketID = state.nextSocketID++;
        this.__phase10ClaimSocketID = socketID;
        this.addEventListener('message', (event) => capture('in', socketID, this.url || args[0], event.data));
      }

      send(data) {
        capture('out', this.__phase10ClaimSocketID ?? 0, this.url, data);
        return super.send(data);
      }
    }

    window.WebSocket = Phase10ClaimWebSocket;

    document.addEventListener(
      'click',
      (event) => {
        const button = event.target instanceof Element ? event.target.closest('button[data-action]') : null;
        if (!button) return;
        clicks.push({
          action: button.dataset.action || '',
          planet_id: button.dataset.planetId || '',
          disabled: button.disabled === true,
          text: button.textContent || '',
          connected: document.querySelector('.hud')?.getAttribute('data-connection') || '',
          default_prevented: event.defaultPrevented === true,
          at: Date.now(),
        });
        if (clicks.length > 50) clicks.shift();
      },
      true,
    );
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
        if (window.__phase10ClaimCommandSocket?.readyState === WebSocket.OPEN) return resolve(true);
        const socket = new WebSocket(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`);
        window.__phase10ClaimCommandSocket = socket;
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
  const request = {
    request_id: `phase10-claim-${op.replace(/[^a-z0-9]+/gi, '-')}-${Date.now()}-${client.seq}`,
    op,
    payload,
    client_seq: client.seq++,
    v: 1,
  };
  return client.page.evaluate(
    ({ message, timeoutMS }) =>
      new Promise((resolve, reject) => {
        const socket = window.__phase10ClaimCommandSocket;
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
  assert(targetPosition, `${label} target position present`);
  const deadline = Date.now() + timeoutMS;
  while (Date.now() < deadline) {
    const state = await waitSmoke(client, (s) => s.connectionStatus === 'connected', 'connected movement state', 10000);
    const self = selfEntity(state);
    const position = positionNow(self, state);
    if (distance(position, targetPosition) <= arriveDistance) return state;

    const target = step(position, targetPosition, 1000);
    const response = await send(client, 'move_to', { target });
    if (response.ok !== true && response.error?.code === 'ERR_RATE_LIMITED') {
      await delay(150);
      continue;
    }
    ok(response, 'move_to');
    const eta = Math.ceil((distance(position, target) / Math.max(1, state.stats?.speed ?? self?.movement?.speed ?? 180)) * 1000);
    await waitSmoke(
      client,
      (candidate) => {
        const pos = positionNow(selfEntity(candidate), candidate);
        return distance(pos, target) <= 40 || distance(pos, targetPosition) <= arriveDistance;
      },
      `movement to ${fmt(target)}`,
      Math.max(5000, eta + 5000),
    );
  }
  throw new Error(`Timed out before reaching ${label} at ${fmt(targetPosition)}`);
}

async function clickFirstEnabled(client, selector, label) {
  const locator = client.page.locator(selector).first();
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    if ((await locator.count()) > 0 && (await locator.isEnabled().catch(() => false))) {
      await locator.click();
      return;
    }
    await delay(100);
  }
  throw new Error(`${label} was not enabled for selector ${selector}`);
}

async function waitForUIClaimResponse(client, planetID, timeoutMS) {
  const started = Date.now();
  let lastFrames = [];
  while (Date.now() - started < timeoutMS) {
    lastFrames = await webSocketFrames(client);
    const parsedFrames = lastFrames
      .map((frame) => ({ frame, parsed: parseFrameJSON(frame.text) }))
      .filter((entry) => entry.parsed);
    const requestEntry = parsedFrames.find(
      (entry) =>
        entry.frame.direction === 'out' &&
        entry.parsed.op === 'discovery.claim_planet' &&
        entry.parsed.payload?.planet_id === planetID,
    );
      if (requestEntry) {
        const responseEntry = parsedFrames.find(
          (entry) => entry.frame.direction === 'in' && entry.parsed.request_id === requestEntry.parsed.request_id,
        );
        const eventEntry = parsedFrames.find((entry) => entry.frame.direction === 'in' && entry.parsed.type === 'planet.claimed');
        if (responseEntry?.parsed?.ok === false) {
          throw new Error(
            `Planet claim response rejected. Request: ${compact(requestEntry.parsed)} Response: ${compact(responseEntry.parsed)} Diagnostics: ${compact(await claimDiagnostics(client, planetID))}`,
          );
        }
        if (responseEntry?.parsed?.ok === true && eventEntry) {
          return {
            request: requestEntry.parsed,
          response: responseEntry.parsed,
          event: eventEntry.parsed,
        };
      }
    }
    await delay(100);
  }
  const state = await smoke(client);
  const diagnostics = await claimDiagnostics(client, planetID);
  throw new Error(
    `Timed out waiting for UI claim response. Diagnostics: ${compact(diagnostics)} State: ${compact(state)} Frames: ${compact(lastFrames)}`,
  );
}

async function claimDiagnostics(client, planetID) {
  return client.page.evaluate((id) => {
    const buttons = [...document.querySelectorAll(`button[data-action="planet-claim"][data-planet-id="${CSS.escape(id)}"]`)].map(
      (button, index) => {
        const rect = button.getBoundingClientRect();
        return {
          index,
          text: button.textContent || '',
          disabled: button.disabled === true,
          visible: rect.width > 0 && rect.height > 0,
          rect: { x: Math.round(rect.x), y: Math.round(rect.y), width: Math.round(rect.width), height: Math.round(rect.height) },
          connection: document.querySelector('.hud')?.getAttribute('data-connection') || '',
          title: button.getAttribute('title') || '',
        };
      },
    );
    return {
      buttons,
      clicks: window.__phase10ClaimClicks ?? [],
      pending_history: window.__SPACE_MORPG_SMOKE_STATE__?.pendingHistory ?? [],
      command_log: window.__SPACE_MORPG_SMOKE_STATE__?.commandLog ?? [],
      connection_status: window.__SPACE_MORPG_SMOKE_STATE__?.connectionStatus ?? '',
      pending_commands: window.__SPACE_MORPG_SMOKE_STATE__?.pendingCommands ?? {},
    };
  }, planetID);
}

function assertClaimRequestPayload(request, planetID) {
  assert(request.op === 'discovery.claim_planet', `claim op = ${request.op}`);
  assert(request.payload?.planet_id === planetID, `claim planet_id = ${compact(request.payload)}`);
  assert(Object.keys(request.payload ?? {}).join(',') === 'planet_id', `claim payload keys = ${Object.keys(request.payload ?? {})}`);
  for (const field of ['player_id', 'map_id', 'position', 'coordinates', 'owner', 'owner_player_id', 'x_core', 'production', 'inventory', 'storage', 'claim_reference']) {
    assert(!(field in request.payload), `claim request leaked trusted field ${field}`);
  }
}

function assertClaimResponsePayload(response, planetID) {
  assert(response.ok === true, `claim response not ok ${compact(response)}`);
  const payload = response.payload ?? {};
  assert(payload.claim?.accepted === true, `claim accepted missing ${compact(payload.claim)}`);
  assert(payload.claim?.planet?.planet_id === planetID, `claim planet mismatch ${compact(payload.claim)}`);
  assert(payload.claim?.planet?.owner_status === 'owned_by_you', `claim owner status ${compact(payload.claim?.planet)}`);
  assert((payload.production?.planets ?? []).some((planet) => planet.planet_id === planetID), 'claim response missing production');
  assert(inventoryQuantity(payload.inventory, 'x_core') === 0, `claim response inventory still has x_core ${compact(payload.inventory)}`);
}

async function assertNoLeak(client, state, label) {
  const body = await client.page.locator('body').innerText({ timeout: 5000 });
  const json = JSON.stringify(state);
  assert(!body.includes('Unhandled event'), `${label} DOM has unhandled event log`);
  for (const token of leakTokens) {
    assert(!body.includes(token), `${label} DOM leaked ${token}`);
    assert(!json.includes(token), `${label} smoke state leaked ${token}`);
  }
  const key = forbiddenKey(state);
  assert(!key, `${label} smoke state leaked forbidden key ${key}`);
  const storageLeak = await browserStorageLeak(client, leakTokens);
  assert(!storageLeak, `${label} browser storage leaked ${storageLeak}`);
}

async function assertWebSocketCanary(client, label) {
  const frames = await webSocketFrames(client);
  const inbound = frames.filter((frame) => frame.direction === 'in').length;
  const outbound = frames.filter((frame) => frame.direction === 'out').length;
  assert(inbound > 0, `${label} WebSocket canary captured no inbound frames`);
  assert(outbound > 0, `${label} WebSocket canary captured no outbound frames`);
  for (const frame of frames) {
    assert(frame.truncated !== true, `${label} websocket frame ${frame.index} exceeded scan limit`);
    if (!frame.text) continue;
    for (const token of leakTokens) {
      assert(!frame.text.includes(token), `${label} websocket frame ${frame.index} leaked ${token}`);
    }
    const parsed = parseFrameJSON(frame.text);
    if (parsed) {
      const key = forbiddenKey(parsed);
      assert(!key, `${label} websocket frame ${frame.index} leaked forbidden key ${key}`);
    }
  }
}

function assertProcessLogCanary(processes) {
  for (const proc of processes) {
    const text = proc.lines.join('\n');
    for (const token of leakTokens) {
      assert(!text.includes(token), `${proc.name} log leaked ${token}`);
    }
  }
}

async function browserStorageLeak(client, tokens) {
  return client.page.evaluate((forbiddenTokens) => {
    const surfaces = [];
    for (const storage of [window.localStorage, window.sessionStorage]) {
      for (let index = 0; index < storage.length; index += 1) {
        const key = storage.key(index) ?? '';
        surfaces.push(`${key}:${storage.getItem(key) ?? ''}`);
      }
    }
    surfaces.push(document.cookie || '');
    for (const surface of surfaces) {
      for (const token of forbiddenTokens) {
        if (surface.includes(token)) return token;
      }
    }
    return '';
  }, tokens);
}

async function resetWebSocketFrames(client) {
  await client.page.evaluate(() => {
    if (Array.isArray(window.__phase10ClaimWebSocketFrames)) {
      window.__phase10ClaimWebSocketFrames.length = 0;
    }
  });
}

async function webSocketFrames(client) {
  return client.page.evaluate(() =>
    (window.__phase10ClaimWebSocketFrames ?? []).map((frame) => ({
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

function discoveredPlanetID(state) {
  return state?.planetIntel?.lastScan?.planet_id || state?.planetIntel?.planets?.[0]?.planet_id || '';
}

function planetOwnerStatus(state, planetID) {
  return state?.planetIntel?.planets?.find((planet) => planet.planet_id === planetID)?.owner_status ?? '';
}

function selectedPlanetOwnerStatus(state, planetID) {
  const selected = state?.planetIntel?.selectedPlanet;
  return selected?.planet_id === planetID ? selected.owner_status : '';
}

function productionInitialized(state, planetID) {
  return (
    state?.production?.planets?.some((planet) => planet.planet_id === planetID && planet.storage && planet.energy_capacity_per_hour > 0) ||
    state?.planetIntel?.selectedPlanet?.production?.planet_id === planetID
  );
}

function inventoryQuantity(inventory, itemID) {
  return (inventory?.stackable ?? [])
    .filter((item) => item.item_id === itemID)
    .reduce((total, item) => total + Number(item.quantity ?? 0), 0);
}

function hasPendingOp(state, op) {
  return Object.values(state?.pendingCommands ?? {}).some((command) => command.op === op);
}

function hasUnhandledEventLog(state) {
  return [...(state?.commandLog ?? []), ...(state?.combatLog ?? [])].some((line) => String(line.text ?? '').includes('Unhandled event'));
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

function cssString(value) {
  return JSON.stringify(String(value));
}

function forbiddenKey(value, path = []) {
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

const forbiddenFieldNames = new Set([
  'account_id',
  'session_id',
  'world_id',
  'zone_id',
  'map_id',
  'internal_map_id',
  'worker_id',
  'map_worker_id',
  'destination_map_id',
  'destination_spawn_id',
  'client_player_id',
  'player_id',
  'owner_player_id',
  'candidate_key',
  'planet_candidate',
  'candidate_data',
  'scan_roll',
  'scan_cell',
  'scan_result',
  'scan_candidate',
  'scan_candidates',
  'procedural_seed',
  'gameplay_seed',
  'world_seed',
  'password',
  'password_hash',
  'token',
  'session_token',
  'reset_secret',
  'auth_header',
  'cookie',
]);

function assertNoPayloadLeak(value, label) {
  const text = JSON.stringify(value);
  for (const token of leakTokens) {
    assert(!text.includes(token), `${label} leaked token ${token}`);
  }
  const key = forbiddenKey(value);
  assert(!key, `${label} leaked forbidden key ${key}`);
}

function parseFrameJSON(text) {
  try {
    return JSON.parse(text);
  } catch {
    return null;
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

function ok(response, label) {
  assert(response?.ok === true, `${label} failed: ${compact(response)}`);
  return response.payload ?? {};
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function compact(value) {
  return JSON.stringify(value, null, 0)?.slice(0, 5000);
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});

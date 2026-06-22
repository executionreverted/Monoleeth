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
const viewport = { width: 1440, height: 900 };

const leakTokens = [
  'map_1_1',
  'map_1_2',
  'map_1_3',
  'internal_map_id',
  'source_map_id',
  'destination_map_id',
  'owner_player_id',
  'player_id',
  'session_id',
  'password',
  'password_hash',
  'session_token',
  'raw_token',
  'reset_secret',
  'procedural_seed',
  'gameplay_seed',
  'world_seed',
  'candidate_key',
  'scan_roll',
  'demo_npc',
  'fixture_npc',
  'mock_npc',
  'fake_npc',
  'mock_wallet',
  'mock_cargo',
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
  'client_player_id',
  'candidate_key',
  'planet_candidate',
  'candidate_data',
  'scan_roll',
  'scan_cell',
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

async function main() {
  const serverPort = await freePort();
  const clientPort = await freePort();
  const clientOrigin = `http://127.0.0.1:${clientPort}`;
  const serverTarget = `http://127.0.0.1:${serverPort}`;
  const goServer = child('go-server', 'go', ['run', './cmd/game-server'], repoRoot, {
    GAME_SERVER_ADDR: `127.0.0.1:${serverPort}`,
    GAME_ALLOWED_ORIGINS: clientOrigin,
    GAME_DEV_MODE: '1',
    GAME_E2E_ROUTE_SEED: '1',
  });
  let viteServer;
  let browser;
  let context;

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
    context = await browser.newContext({ viewport });
    const page = await context.newPage();
    const nonce = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const client = {
      context,
      page,
      email: `phase10-route-${nonce}@example.test`,
      callsign: `P10R-${nonce.slice(-8)}`,
      capturedWebSocketFrames: [],
    };
    await installWebSocketCanary(client);
    await page.goto(`${clientOrigin}/?smoke=1`, { waitUntil: 'domcontentloaded' });
    await register(client, client.email, 'correct-password', client.callsign);

    const seeded = await waitSmoke(client, routeSeedReady, 'E2E owned route planets and storage', 30000);
    const sourceID = routeSourceID(seeded);
    const destinationID = routeDestinationID(seeded, sourceID);
    assert(sourceID && destinationID, `route seed planets missing ${compact(seeded.planetIntel)}`);
    assertRouteSeedIDOpaque(sourceID, 'source route seed');
    assertRouteSeedIDOpaque(destinationID, 'destination route seed');
    await assertNoLeak(client, seeded, 'route-seed');

    await client.page.locator(`button[data-action="planet-detail"][data-planet-id=${cssString(sourceID)}]`).first().click();
    await waitSmoke(
      client,
      (state) => state.planetIntel?.selectedPlanet?.planet_id === sourceID && state.planetIntel.selectedPlanet.production,
      'source planet detail',
      15000,
    );

    await resetWebSocketFrames(client);
    await setRouteCreateControls(client, destinationID, 'refined_alloy', 40);
    await clickFirstEnabled(client, `button[data-action="route-create"][data-source-planet-id=${cssString(sourceID)}]`, 'Route create');
    const createFrames = await waitForOperation(client, 'route.create', (payload) => payload.source_planet_id === sourceID, 15000);
    assertExactKeys(createFrames.request.payload, ['source_planet_id', 'destination_planet_id', 'resource_item_id', 'amount_per_hour'], 'route.create request');
    assert(createFrames.request.payload.destination_planet_id === destinationID, `route.create destination ${compact(createFrames.request.payload)}`);
    assert(createFrames.request.payload.resource_item_id === 'refined_alloy', `route.create resource ${compact(createFrames.request.payload)}`);
    assert(createFrames.request.payload.amount_per_hour === 40, `route.create rate ${compact(createFrames.request.payload)}`);
    assertNoPayloadLeak(createFrames.response, 'route.create response');
    const withRoute = await waitSmoke(
      client,
      (state) => (state.routes?.routes ?? []).length === 1 && !hasPendingOp(state, 'route.create') && !hasUnhandledEventLog(state),
      'created route reconciliation',
      15000,
    );
    const routeID = withRoute.routes.routes[0].route_id;
    assert(routeID, `created route id missing ${compact(withRoute.routes)}`);

    await resetWebSocketFrames(client);
    await setRouteUpdateControls(client, routeID, destinationID, 'refined_alloy', 75);
    await clickFirstEnabled(client, `button[data-action="route-update"][data-route-id=${cssString(routeID)}]`, 'Route update');
    const updateFrames = await waitForOperation(client, 'route.update', (payload) => payload.route_id === routeID, 15000);
    assertExactKeys(updateFrames.request.payload, ['route_id', 'destination_planet_id', 'resource_item_id', 'amount_per_hour'], 'route.update request');
    assert(updateFrames.request.payload.destination_planet_id === destinationID, `route.update destination ${compact(updateFrames.request.payload)}`);
    assert(updateFrames.request.payload.resource_item_id === 'refined_alloy', `route.update resource ${compact(updateFrames.request.payload)}`);
    assert(updateFrames.request.payload.amount_per_hour === 75, `route.update rate ${compact(updateFrames.request.payload)}`);
    await waitSmoke(
      client,
      (state) => routeByID(state, routeID)?.resource_item_id === 'refined_alloy' && routeByID(state, routeID)?.amount_per_hour === 75 && !hasPendingOp(state, 'route.update'),
      'updated route reconciliation',
      15000,
    );

    await resetWebSocketFrames(client);
    await clickFirstEnabled(client, `button[data-action="route-disable"][data-route-id=${cssString(routeID)}]`, 'Route disable');
    const disableFrames = await waitForOperation(client, 'route.disable', (payload) => payload.route_id === routeID, 15000);
    assertExactKeys(disableFrames.request.payload, ['route_id'], 'route.disable request');
    await waitSmoke(client, (state) => routeByID(state, routeID)?.enabled === false && !hasPendingOp(state, 'route.disable'), 'disabled route reconciliation', 15000);

    await resetWebSocketFrames(client);
    await clickFirstEnabled(client, `button[data-action="route-enable"][data-route-id=${cssString(routeID)}]`, 'Route enable');
    const enableFrames = await waitForOperation(client, 'route.enable', (payload) => payload.route_id === routeID, 15000);
    assertExactKeys(enableFrames.request.payload, ['route_id'], 'route.enable request');
    await waitSmoke(client, (state) => routeByID(state, routeID)?.enabled === true && !hasPendingOp(state, 'route.enable'), 'enabled route reconciliation', 15000);

    await resetWebSocketFrames(client);
    await clickFirstEnabled(client, `button[data-action="route-settle"][data-route-id=${cssString(routeID)}]`, 'Route settle');
    const settleFrames = await waitForOperation(client, 'route.settle', (payload) => payload.route_id === routeID, 15000);
    assertExactKeys(settleFrames.request.payload, ['route_id'], 'route.settle request');
    assertNoPayloadLeak(settleFrames.response, 'route.settle response');
    await waitSmoke(client, (state) => !hasPendingOp(state, 'route.settle') && !hasUnhandledEventLog(state), 'single route settlement reconciliation', 15000);

    await resetWebSocketFrames(client);
    await clickFirstEnabled(client, 'button[data-action="route-settle"]:not([data-route-id])', 'Route settle all');
    const reconcileFrames = await waitForOperation(client, 'route.settle', (payload) => Object.keys(payload ?? {}).length === 0, 15000);
    assertExactKeys(reconcileFrames.request.payload, [], 'route.settle reconcile request');
    assertNoPayloadLeak(reconcileFrames.response, 'route.settle reconcile response');

    const finalState = await waitSmoke(client, (state) => !hasPendingOp(state, 'route.settle') && !hasUnhandledEventLog(state), 'route reconcile final state', 15000);
    await assertNoLeak(client, finalState, 'route-final');
    await assertWebSocketCanary(client, 'route');
    assertProcessLogCanary([goServer, viteServer]);
    assertNoForbiddenOutboundFrames(client.capturedWebSocketFrames);

    console.log(`phase10-route smoke ok source=${sourceID} destination=${destinationID} route=${routeID}`);
  } finally {
    if (context) await context.close().catch(() => {});
    if (browser) await browser.close().catch(() => {});
    if (viteServer) await stop(viteServer);
    await stop(goServer);
  }
}

async function installWebSocketCanary(client) {
  await client.page.addInitScript(() => {
    if (window.__phase10RouteWebSocketCanaryInstalled) return;
    window.__phase10RouteWebSocketCanaryInstalled = true;
    const NativeWebSocket = window.WebSocket;
    const frames = [];
    const maxTextLength = 1_000_000;
    const state = { nextSocketID: 1, nextFrameIndex: 0 };
    window.__phase10RouteWebSocketFrames = frames;

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
      return { kind: Object.prototype.toString.call(data), text: '', text_length: 0, truncated: false };
    }

    function capture(direction, socketID, url, data) {
      frames.push({
        direction,
        index: state.nextFrameIndex++,
        socket_id: socketID,
        path: safePath(url),
        ...captureText(data),
      });
    }

    class Phase10RouteWebSocket extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        const socketID = state.nextSocketID++;
        this.__phase10RouteSocketID = socketID;
        this.addEventListener('message', (event) => capture('in', socketID, this.url || args[0], event.data));
      }

      send(data) {
        capture('out', this.__phase10RouteSocketID ?? 0, this.url, data);
        return super.send(data);
      }
    }

    window.WebSocket = Phase10RouteWebSocket;
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

function routeSeedReady(state) {
  return (
    state?.connectionStatus === 'connected' &&
    state.auth?.session?.authenticated === true &&
    state.currentMap?.public_map_key === '1-1' &&
    routeSourceID(state) &&
    routeDestinationID(state, routeSourceID(state)) &&
    sourceStorageHas(state, 'refined_alloy')
  );
}

function routeSourceID(state) {
  return (
    state?.planetIntel?.planets?.find(
      (planet) => planet.planet_id.includes('planet-e2e-route-source-') && planet.owner_status === 'owned_by_you',
    )?.planet_id ?? ''
  );
}

function routeDestinationID(state, sourceID) {
  return (
    state?.planetIntel?.planets?.find(
      (planet) =>
        planet.planet_id !== sourceID &&
        planet.planet_id.includes('planet-e2e-route-destination-') &&
        planet.owner_status === 'owned_by_you',
    )?.planet_id ?? ''
  );
}

function assertRouteSeedIDOpaque(planetID, label) {
  assert(!planetID.includes('player-'), `${label} planet id leaked player id prefix: ${planetID}`);
}

function sourceStorageHas(state, itemID) {
  const sourceID = routeSourceID(state);
  const source = state?.production?.planets?.find((planet) => planet.planet_id === sourceID);
  return (source?.storage?.items ?? []).some((item) => item.item_id === itemID && Number(item.quantity ?? 0) > 0);
}

async function setRouteCreateControls(client, destinationID, resourceItemID, amountPerHour) {
  await client.page.locator('[data-route-create-control="true"]').first().waitFor({ timeout: 10000 });
  await client.page.locator('[data-route-create-control="true"] [data-route-create-destination]').first().selectOption(destinationID);
  await client.page.locator('[data-route-create-control="true"] [data-route-create-resource]').first().selectOption(resourceItemID);
  await client.page.locator('[data-route-create-control="true"] [data-route-rate]').first().fill(String(amountPerHour));
}

async function setRouteUpdateControls(client, routeID, destinationID, resourceItemID, amountPerHour) {
  const root = client.page.locator(`[data-route-update-control="true"][data-route-id=${cssString(routeID)}]`).first();
  await root.waitFor({ timeout: 10000 });
  await root.locator('[data-route-update-destination]').selectOption(destinationID);
  await root.locator('[data-route-update-resource]').selectOption(resourceItemID);
  await root.locator('[data-route-rate]').fill(String(amountPerHour));
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

async function waitForOperation(client, op, payloadPredicate, timeoutMS) {
  const started = Date.now();
  let lastFrames = [];
  while (Date.now() - started < timeoutMS) {
    lastFrames = await webSocketFrames(client);
    const parsed = lastFrames.map((frame) => ({ frame, parsed: parseFrameJSON(frame.text) })).filter((entry) => entry.parsed);
    const requestEntry = parsed.find(
      (entry) => entry.frame.direction === 'out' && entry.parsed.op === op && payloadPredicate(entry.parsed.payload ?? {}),
    );
    if (requestEntry) {
      const responseEntry = parsed.find(
        (entry) => entry.frame.direction === 'in' && entry.parsed.request_id === requestEntry.parsed.request_id,
      );
      if (responseEntry?.parsed?.ok === false) {
        throw new Error(`${op} rejected. Request: ${compact(requestEntry.parsed)} Response: ${compact(responseEntry.parsed)}`);
      }
      if (responseEntry?.parsed?.ok === true) {
        return { request: requestEntry.parsed, response: responseEntry.parsed };
      }
    }
    await delay(100);
  }
  throw new Error(`Timed out waiting for ${op}. Frames: ${compact(lastFrames)} State: ${compact(await smoke(client))}`);
}

function routeByID(state, routeID) {
  return state?.routes?.routes?.find((route) => route.route_id === routeID) ?? null;
}

function hasPendingOp(state, op) {
  return Object.values(state?.pendingCommands ?? {}).some((command) => command.op === op);
}

function hasUnhandledEventLog(state) {
  return [...(state?.commandLog ?? []), ...(state?.combatLog ?? [])].some((line) => String(line.text ?? '').includes('Unhandled event'));
}

function assertExactKeys(payload, keys, label) {
  const got = Object.keys(payload ?? {});
  assert(got.length === keys.length && got.every((key, index) => key === keys[index]), `${label} keys = ${got.join(',')}, want ${keys.join(',')}`);
}

function assertNoForbiddenOutboundFrames(frames) {
  const trustedRouteFields = [
    'owner',
    'owner_player_id',
    'player_id',
    'session_id',
    'source',
    'destination',
    'source_map_id',
    'destination_map_id',
    'map_id',
    'from_public_map_key',
    'to_public_map_key',
    'enabled',
    'settlement',
    'storage',
    'energy_cost_per_hour',
    'risk',
    'loss_chance',
    'last_calculated_at',
    'position',
    'coordinates',
  ];
  for (const frame of frames) {
    if (frame.direction !== 'out' || !frame.text) continue;
    const parsed = parseFrameJSON(frame.text);
    if (!parsed?.op?.startsWith?.('route.')) continue;
    for (const field of trustedRouteFields) {
      assert(!(field in (parsed.payload ?? {})), `${parsed.op} outbound payload leaked ${field}: ${compact(parsed.payload)}`);
    }
  }
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
  const frames = await captureWebSocketFrames(client);
  assert(frames.some((frame) => frame.direction === 'in'), `${label} WebSocket canary captured no inbound frames`);
  assert(frames.some((frame) => frame.direction === 'out'), `${label} WebSocket canary captured no outbound frames`);
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
  if (client.captureWebSocketFramesStarted) {
    await captureWebSocketFrames(client);
  } else {
    client.captureWebSocketFramesStarted = true;
  }
  await client.page.evaluate(() => {
    if (Array.isArray(window.__phase10RouteWebSocketFrames)) {
      window.__phase10RouteWebSocketFrames.length = 0;
    }
  });
}

async function captureWebSocketFrames(client) {
  const frames = await webSocketFrames(client);
  client.capturedWebSocketFrames ??= [];
  client.capturedWebSocketFrames.push(...frames);
  return client.capturedWebSocketFrames;
}

async function webSocketFrames(client) {
  return client.page.evaluate(() =>
    (window.__phase10RouteWebSocketFrames ?? []).map((frame) => ({
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

function cssString(value) {
  return JSON.stringify(String(value));
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

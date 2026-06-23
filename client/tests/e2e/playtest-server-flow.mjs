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
  'password_hash',
  'session_token',
  'raw_token',
  'reset_secret',
  'procedural_seed',
  'gameplay_seed',
  'world_seed',
  'candidate_key',
  'scan_roll',
  'loot_table',
  'loot_roll',
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
  'candidate_key',
  'scan_roll',
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
    GAME_PLAYTEST_SEED: 'true',
  });
  let browser;
  let context;

  try {
    await waitHTTP(`${origin}/healthz`, 'Go server', goServer);
    await waitHTTP(`${origin}/?smoke=1`, 'built client', goServer);

    browser = await chromium.launch();
    context = await browser.newContext({ viewport });
    const page = await context.newPage();
    const client = { page };
    await installWebSocketCanary(client);

    const nonce = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    await page.goto(`${origin}/?smoke=1`, { waitUntil: 'domcontentloaded' });
    await register(client, `playtest-${nonce}@example.test`, 'correct-password', `PT-${nonce.slice(-8)}`);

    const seeded = await waitSmoke(client, playtestSeedReady, 'playtest onboarding seed', 30000);
    assert(seeded.auth?.session?.authenticated === true, 'authenticated real session missing');
    assert(seeded.currentMap?.public_map_key === '1-1', `current map ${seeded.currentMap?.public_map_key}, want 1-1`);
    assert((seeded.currentMap?.visible_portals ?? []).some((portal) => portal.portal_id === 'east_gate'), 'east_gate portal missing');
    assert(inventoryQuantity(seeded.inventory, 'x_core') === 1, 'playtest X Core seed missing');
    await assertNoLeak(client, seeded, 'seeded playtest state');

    const sourceID = routeSourceID(seeded);
    const destinationID = routeDestinationID(seeded, sourceID);
    assert(sourceID && destinationID, `playtest route planets missing ${compact(seeded.planetIntel)}`);
    assertRouteSeedIDOpaque(sourceID, 'playtest source');
    assertRouteSeedIDOpaque(destinationID, 'playtest destination');

    await page.locator(`button[data-action="planet-detail"][data-planet-id=${cssString(sourceID)}]`).first().click();
    await waitSmoke(
      client,
      (state) => state.planetIntel?.selectedPlanet?.planet_id === sourceID && state.planetIntel.selectedPlanet.production,
      'playtest source planet detail',
      15000,
    );

    await resetWebSocketFrames(client);
    await setRouteCreateControls(client, destinationID, 'refined_alloy', 40);
    await clickFirstEnabled(client, `button[data-action="route-create"][data-source-planet-id=${cssString(sourceID)}]`, 'Route create');
    const createFrames = await waitForOperation(client, 'route.create', (payload) => payload.source_planet_id === sourceID, 15000);
    assertExactKeys(createFrames.request.payload, ['source_planet_id', 'destination_planet_id', 'resource_item_id', 'amount_per_hour'], 'route.create request');
    assert(createFrames.request.payload.destination_planet_id === destinationID, `route.create destination ${compact(createFrames.request.payload)}`);
    assert(createFrames.request.payload.resource_item_id === 'refined_alloy', `route.create resource ${compact(createFrames.request.payload)}`);
    assertNoPayloadLeak(createFrames.response, 'route.create response');

    const withRoute = await waitSmoke(
      client,
      (state) => (state.routes?.routes ?? []).length === 1 && !hasPendingOp(state, 'route.create') && !hasUnhandledEventLog(state),
      'playtest route create reconciliation',
      15000,
    );
    const routeID = withRoute.routes.routes[0].route_id;
    assert(routeID, `created route missing id ${compact(withRoute.routes)}`);

    await resetWebSocketFrames(client);
    await clickFirstEnabled(client, `button[data-action="route-settle"][data-route-id=${cssString(routeID)}]`, 'Route settle');
    const settleFrames = await waitForOperation(client, 'route.settle', (payload) => payload.route_id === routeID, 15000);
    assertExactKeys(settleFrames.request.payload, ['route_id'], 'route.settle request');
    assertNoPayloadLeak(settleFrames.response, 'route.settle response');
    const finalState = await waitSmoke(client, (state) => !hasPendingOp(state, 'route.settle') && !hasUnhandledEventLog(state), 'playtest route settle reconciliation', 15000);
    await assertNoLeak(client, finalState, 'playtest route final');
    await assertWebSocketCanary(client, 'playtest');
    assertProcessLogCanary([goServer]);

    console.log(`playtest-server smoke ok source=${sourceID} destination=${destinationID} route=${routeID}`);
  } finally {
    if (context) await context.close().catch(() => {});
    if (browser) await browser.close().catch(() => {});
    await stop(goServer);
  }
}

async function installWebSocketCanary(client) {
  await client.page.addInitScript(() => {
    if (window.__playtestServerWebSocketCanaryInstalled) return;
    window.__playtestServerWebSocketCanaryInstalled = true;
    const NativeWebSocket = window.WebSocket;
    const frames = [];
    const state = { nextSocketID: 1, nextFrameIndex: 0 };
    window.__playtestServerWebSocketFrames = frames;

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
      });
    }

    class PlaytestServerWebSocket extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        const socketID = state.nextSocketID++;
        this.__playtestServerSocketID = socketID;
        this.addEventListener('message', (event) => capture('in', socketID, this.url || args[0], event.data));
      }

      send(data) {
        capture('out', this.__playtestServerSocketID ?? 0, this.url, data);
        return super.send(data);
      }
    }

    window.WebSocket = PlaytestServerWebSocket;
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

function playtestSeedReady(state) {
  const sourceID = routeSourceID(state);
  return (
    state?.connectionStatus === 'connected' &&
    state.auth?.session?.authenticated === true &&
    state.currentMap?.public_map_key === '1-1' &&
    inventoryQuantity(state.inventory, 'x_core') === 1 &&
    sourceID &&
    routeDestinationID(state, sourceID) &&
    sourceStorageHas(state, 'refined_alloy')
  );
}

function routeSourceID(state) {
  return (
    state?.planetIntel?.planets?.find(
      (planet) => planet.planet_id.includes('planet-playtest-route-source-') && planet.owner_status === 'owned_by_you',
    )?.planet_id ?? ''
  );
}

function routeDestinationID(state, sourceID) {
  return (
    state?.planetIntel?.planets?.find(
      (planet) =>
        planet.planet_id !== sourceID &&
        planet.planet_id.includes('planet-playtest-route-destination-') &&
        planet.owner_status === 'owned_by_you',
    )?.planet_id ?? ''
  );
}

function sourceStorageHas(state, itemID) {
  const source = state?.production?.planets?.find((planet) => planet.planet_id === routeSourceID(state));
  return (source?.storage?.items ?? []).some((item) => item.item_id === itemID && Number(item.quantity ?? 0) > 0);
}

function inventoryQuantity(inventory, itemID) {
  return (inventory?.stackable ?? []).filter((item) => item.item_id === itemID).reduce((sum, item) => sum + Number(item.quantity ?? 0), 0);
}

async function setRouteCreateControls(client, destinationID, resourceItemID, amountPerHour) {
  await client.page.locator('[data-route-create-control="true"]').first().waitFor({ timeout: 10000 });
  await client.page.locator('[data-route-create-control="true"] [data-route-create-destination]').first().selectOption(destinationID);
  await client.page.locator('[data-route-create-control="true"] [data-route-create-resource]').first().selectOption(resourceItemID);
  await client.page.locator('[data-route-create-control="true"] [data-route-rate]').first().fill(String(amountPerHour));
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

async function webSocketFrames(client) {
  return client.page.evaluate(() => window.__playtestServerWebSocketFrames ?? []);
}

async function resetWebSocketFrames(client) {
  await client.page.evaluate(() => {
    if (Array.isArray(window.__playtestServerWebSocketFrames)) window.__playtestServerWebSocketFrames.length = 0;
  });
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

async function assertNoLeak(client, state, label) {
  assertNoPayloadLeak(state, `${label} smoke state`);
  const storageLeak = await browserStorageLeak(client);
  assert(!storageLeak, `${label} browser storage leaked ${storageLeak}`);
}

function assertNoPayloadLeak(value, label) {
  const text = JSON.stringify(value);
  for (const token of leakTokens) {
    assert(!text.includes(token), `${label} leaked token ${token}`);
  }
  const key = forbiddenKey(value);
  assert(!key, `${label} leaked forbidden key ${key}`);
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

async function assertWebSocketCanary(client, label) {
  const frames = await webSocketFrames(client);
  assert(frames.some((frame) => frame.path === '/ws' && frame.direction === 'in'), `${label} missing inbound /ws frames`);
  for (const frame of frames) {
    if (frame.text) assertNoPayloadLeak(frame.text, `${label} ${frame.direction} websocket frame`);
  }
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

function assertRouteSeedIDOpaque(planetID, label) {
  assert(!planetID.includes('player-'), `${label} planet id leaked player id prefix: ${planetID}`);
}

function assertExactKeys(payload, keys, label) {
  const got = Object.keys(payload ?? {}).sort();
  const want = [...keys].sort();
  assert(JSON.stringify(got) === JSON.stringify(want), `${label} keys = ${got.join(',')}, want ${want.join(',')}`);
}

function hasPendingOp(state, op) {
  return Object.values(state?.pendingCommands ?? {}).some((pending) => pending?.op === op);
}

function hasUnhandledEventLog(state) {
  return (state?.commandLog ?? []).some((line) => typeof line?.text === 'string' && line.text.includes('Unhandled event'));
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

#!/usr/bin/env node
import { spawn } from 'node:child_process';
import { mkdir } from 'node:fs/promises';
import net from 'node:net';
import { basename, dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const clientDir = resolve(scriptDir, '../..');
const repoRoot = resolve(clientDir, '..');
const screenshotDir = resolve(repoRoot, 'output/screenshots/ui-implementation/playtest');
const magickBin = process.env.PLAYTEST_MAGICK_BIN || 'magick';
const tesseractBin = process.env.PLAYTEST_TESSERACT_BIN || '/opt/homebrew/bin/tesseract';
const screenshotOCRTimeoutMS = 30000;
const maxProcessLogLines = 3000;
const viewport = { width: 1440, height: 900 };
const starterNpcApproachTarget = { x: 800, y: 400 };
const outerNpcApproachTarget = { x: 1800, y: 5400 };
const claimRangeArriveDistance = 220;
const eastGateTarget = { x: 9800, y: 5000 };

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
  await mkdir(screenshotDir, { recursive: true });
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
    const client = { page, seq: 1 };
    await installWebSocketCanary(client);

    const nonce = `${Date.now()}-${Math.random().toString(16).slice(2)}`;
    await page.goto(`${origin}/?smoke=1`, { waitUntil: 'domcontentloaded' });
    await register(client, `playtest-${nonce}@example.test`, 'correct-password', `PT-${nonce.slice(-8)}`);

    const seeded = await waitSmoke(
      client,
      (state) => playtestSeedReady(state) && hasRenderedEntityAsset(state, 'ship.player.self'),
      'playtest onboarding seed with player sprite',
      30000,
    );
    assert(seeded.auth?.session?.authenticated === true, 'authenticated real session missing');
    assert(seeded.currentMap?.public_map_key === '1-1', `current map ${seeded.currentMap?.public_map_key}, want 1-1`);
    assert((seeded.currentMap?.visible_portals ?? []).some((portal) => portal.portal_id === 'east_gate'), 'east_gate portal missing');
    assert(inventoryQuantity(seeded.inventory, 'x_core') === 1, 'playtest X Core seed missing');
    assertWorldAssetTexturesLoaded(seeded);
    assert(hasRenderedOverlayAsset(seeded, 'portal.gate.visible'), 'portal gate overlay sprite missing');
    assert(hasRenderedOverlayAsset(seeded, 'zone.safe.pvp-blocked'), 'safe-zone overlay sprite missing');
    const assetScreenshotPath = await captureAssetSpriteProof(client, 'asset-sprites-desktop');
    await assertNoLeak(client, seeded, 'seeded playtest state');

    const sourceID = routeSourceID(seeded);
    const destinationID = routeDestinationID(seeded, sourceID);
    assert(sourceID && destinationID, `playtest route planets missing ${compact(seeded.planetIntel)}`);
    assertRouteSeedIDOpaque(sourceID, 'playtest source');
    assertRouteSeedIDOpaque(destinationID, 'playtest destination');

    await openCommandSocket(client);
    const afterLoot = await completeFightLootLoop(client);
    await assertNoLeak(client, afterLoot, 'playtest combat loot');

    await resetWebSocketFrames(client);
    await clickFirstEnabled(client, 'button[data-action="scan"]', 'Scan');
    const scanFrames = await waitForOperation(client, 'scan.pulse', (payload) => Object.keys(payload ?? {}).length === 0, 30000);
    assertExactKeys(scanFrames.request.payload, [], 'scan.pulse request');
    assertSafeScanPayload(scanFrames.response.payload, 'scan.pulse response');
    const withPlanet = await waitSmoke(client, (state) => discoveredPlanetID(state) !== '', 'playtest scanned planet discovery', 30000);
    const planetID = discoveredPlanetID(withPlanet);
    await page.locator(`button[data-action="planet-detail"][data-planet-id=${cssString(planetID)}]`).first().click();
    const withDetail = await waitSmoke(
      client,
      (state) => state.planetIntel?.selectedPlanet?.planet_id === planetID && state.planetIntel.selectedPlanet.coordinates,
      'playtest discovered planet detail',
      15000,
    );
    const claimCoordinates = withDetail.planetIntel.selectedPlanet.coordinates;
    await moveToPosition(client, claimCoordinates, claimRangeArriveDistance, `claim planet ${planetID}`, 90000);

    await resetWebSocketFrames(client);
    await clickFirstEnabled(client, `button[data-action="planet-claim"][data-planet-id=${cssString(planetID)}]`, 'Claim');
    const claimFrames = await waitForOperation(client, 'discovery.claim_planet', (payload) => payload.planet_id === planetID, 20000);
    assertExactKeys(claimFrames.request.payload, ['planet_id'], 'discovery.claim_planet request');
    assertClaimResponsePayload(claimFrames.response.payload, planetID);
    const claimed = await waitSmoke(
      client,
      (state) =>
        planetOwnerStatus(state, planetID) === 'owned_by_you' &&
        selectedPlanetOwnerStatus(state, planetID) === 'owned_by_you' &&
        productionInitialized(state, planetID) &&
        inventoryQuantity(state.inventory, 'x_core') === 0 &&
        !hasPendingOp(state, 'discovery.claim_planet') &&
        !hasUnhandledEventLog(state),
      'playtest claimed planet reconciliation',
      20000,
    );
    await assertNoLeak(client, claimed, 'playtest claim');

    await resetWebSocketFrames(client);
    await clickFirstEnabled(client, `button[data-action="planet-building-build"][data-planet-id=${cssString(planetID)}]`, 'Building build');
    const buildingID = `${planetID}-building-iron_extractor-alpha`;
    const buildingFrames = await waitForOperation(
      client,
      'planet.building_build',
      (payload) => payload.planet_id === planetID && payload.building_type === 'iron_extractor' && payload.slot === 'alpha',
      15000,
    );
    assertExactKeys(buildingFrames.request.payload, ['planet_id', 'building_type', 'slot'], 'planet.building_build request');
    assertBuildingBuildResponsePayload(buildingFrames.response.payload, planetID, buildingID);
    const withBuilding = await waitSmoke(
      client,
      (state) =>
        productionBuilding(state, planetID, buildingID)?.level === 1 &&
        !hasPendingOp(state, 'planet.building_build') &&
        !hasUnhandledEventLog(state),
      'playtest building build reconciliation',
      15000,
    );
    await assertNoLeak(client, withBuilding, 'playtest building build');

    await closeModalIfOpen(client);
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

    await closeModalIfOpen(client);
    await moveToPosition(client, eastGateTarget, 120, 'east_gate portal', 90000);
    await resetWebSocketFrames(client);
    const portalResponse = payloadOf(await send(client, 'portal.enter', { portal_id: 'east_gate' }), 'portal.enter');
    const portalFrames = await waitForOperation(client, 'portal.enter', (payload) => payload.portal_id === 'east_gate', 15000);
    assertExactKeys(portalFrames.request.payload, ['portal_id'], 'portal.enter request');
    assertPortalResponsePayload(portalResponse, '1-2', 'Outer Ring');
    assertNoPayloadLeak(portalFrames.response, 'portal.enter response frame');
    const outer = await waitSmoke(client, (state) => state.currentMap?.public_map_key === '1-2', 'playtest Outer Ring map state', 15000);
    assertOuterMap(outer);
    assertNoOriginMapLeakage(finalState, outer);
    await assertNoLeak(client, outer, 'playtest outer ring');
    await resetWebSocketFrames(client);
    const outerAfterLoot = await completeFightLootLoop(client, {
      mapKey: '1-2',
      approachTarget: outerNpcApproachTarget,
      label: 'playtest outer ring',
    });
    await assertNoLeak(client, outerAfterLoot, 'playtest outer ring combat loot');

    await assertWebSocketCanary(client, 'playtest');
    assertProcessLogCanary([goServer]);

    console.log(
      `playtest-server smoke ok source=${sourceID} destination=${destinationID} building=${buildingID} route=${routeID} portal=1-2 destination_drop=ok screenshot=${assetScreenshotPath}`,
    );
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

function assertWorldAssetTexturesLoaded(state) {
  const loaded = state?.worldView?.renderedAssets?.loadedTextures ?? 0;
  assert(loaded >= 10, `world asset textures not loaded: ${loaded}`);
}

function hasRenderedEntityAsset(state, assetKey) {
  return (state?.worldView?.renderedAssets?.entitySprites ?? []).some((sprite) => sprite.assetKey === assetKey && sprite.visible === true);
}

function hasRenderedOverlayAsset(state, assetKey) {
  return (state?.worldView?.renderedAssets?.overlaySprites ?? []).some((sprite) => sprite.assetKey === assetKey && sprite.visible === true);
}

async function captureAssetSpriteProof(client, name) {
  const screenshotPath = resolve(screenshotDir, `${name}.png`);
  await client.page.locator('canvas.world-canvas').screenshot({ path: screenshotPath });
  const pixelProof = await imagePixelProof(screenshotPath);
  assert(pixelProof.ok, `asset canvas pixel proof failed ${compact(pixelProof)}`);
  await assertScreenshotOCRCanary(screenshotPath);
  return screenshotPath;
}

async function imagePixelProof(screenshotPath) {
  const proc = spawn(magickBin, [screenshotPath, '-resize', '96x96!', 'txt:-'], { stdio: ['ignore', 'pipe', 'pipe'] });
  let stdout = '';
  let stderr = '';
  proc.stdout.on('data', (chunk) => {
    stdout += chunk.toString();
  });
  proc.stderr.on('data', (chunk) => {
    stderr += chunk.toString();
  });
  const code = await new Promise((resolve) => proc.on('exit', resolve));
  assert(code === 0, `ImageMagick pixel proof failed (${magickBin}) code=${code} stderr=${stderr.trim()}`);

  let brightPixels = 0;
  const buckets = new Set();
  for (const line of stdout.split('\n')) {
    const match = line.match(/\((\d+(?:\.\d+)?),(\d+(?:\.\d+)?),(\d+(?:\.\d+)?)(?:,|\))/);
    if (!match) continue;
    const red = Number(match[1]);
    const green = Number(match[2]);
    const blue = Number(match[3]);
    if (red + green + blue > 80) {
      brightPixels += 1;
      buckets.add(`${Math.floor(red) >> 5}:${Math.floor(green) >> 5}:${Math.floor(blue) >> 5}`);
    }
  }
  return { ok: brightPixels > 120 && buckets.size >= 4, brightPixels, colorBuckets: buckets.size };
}

async function assertScreenshotOCRCanary(screenshotPath) {
  const ocrText = await runScreenshotOCR(screenshotPath);
  assert(ocrText.trim().length > 0, `${basename(screenshotPath)} OCR produced no text`);
  const token = screenshotLeakToken(ocrText);
  assert(!token, `${basename(screenshotPath)} OCR leaked token ${token}`);
}

async function runScreenshotOCR(screenshotPath) {
  const proc = spawn(tesseractBin, [screenshotPath, 'stdout'], {
    cwd: repoRoot,
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  const stdout = [];
  let missingTesseract = false;
  const exit = new Promise((resolve) => {
    proc.on('error', (error) => {
      missingTesseract = error?.code === 'ENOENT';
      resolve({ code: -1, signal: null });
    });
    proc.on('exit', (code, signal) => resolve({ code, signal }));
  });
  proc.stdout.on('data', (chunk) => stdout.push(chunk));
  proc.stderr.resume();

  const timed = await Promise.race([exit, delay(screenshotOCRTimeoutMS).then(() => ({ timedOut: true }))]);
  if (timed.timedOut) {
    signal(proc, 'SIGKILL');
    await waitExit(proc, 2000);
    throw new Error(`${basename(screenshotPath)} OCR timed out after ${screenshotOCRTimeoutMS}ms`);
  }
  if (missingTesseract) {
    throw new Error(`Tesseract OCR is required for playtest screenshot leak canary but was not found at ${tesseractBin}`);
  }
  if (timed.code !== 0) {
    throw new Error(`${basename(screenshotPath)} OCR failed with exit code ${timed.code ?? 'null'}${timed.signal ? ` signal ${timed.signal}` : ''}`);
  }
  return Buffer.concat(stdout).toString('utf8');
}

function screenshotLeakToken(text) {
  const haystack = text.toLowerCase();
  return leakTokens.find((token) => haystack.includes(token.toLowerCase())) ?? null;
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

async function openCommandSocket(client) {
  await client.page.evaluate(
    () =>
      new Promise((resolve, reject) => {
        if (window.__playtestServerCommandSocket?.readyState === WebSocket.OPEN) return resolve(true);
        const socket = new WebSocket(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`);
        window.__playtestServerCommandSocket = socket;
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
    request_id: `playtest-${op.replace(/[^a-z0-9]+/gi, '-')}-${Date.now()}-${client.seq}`,
    op,
    payload,
    client_seq: client.seq++,
    v: 1,
  };
  return client.page.evaluate(
    ({ message }) =>
      new Promise((resolve, reject) => {
        const socket = window.__playtestServerCommandSocket;
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

async function completeFightLootLoop(client, options = {}) {
  const mapKey = options.mapKey ?? '1-1';
  const approachTarget = options.approachTarget ?? starterNpcApproachTarget;
  const label = options.label ?? 'playtest';
  if (!findHostileNPC(await smoke(client))) {
    await moveToPosition(client, approachTarget, 260, `${label} hostile radar approach`, 30000);
  }
  const withNPC = await waitSmoke(client, (state) => state.currentMap?.public_map_key === mapKey && findHostileNPC(state), `${label} visible NPC`, 15000);
  assert(hasRenderedEntityAsset(withNPC, 'npc.swarm.hostile'), `${label} hostile NPC sprite asset missing`);
  const npc = findHostileNPC(withNPC);
  const killedNPCID = npc.entity_id;
  await moveToPosition(client, npc.position, Math.max(80, Math.min(220, (withNPC.stats?.weapon_range ?? 260) - 40)), `combat target ${killedNPCID}`, 30000);
  const combatPayload = await fightNPCUntilKilled(client, killedNPCID);
  const expectedDrop = responseDrop(combatPayload, `${label} combat response`);
  const withDrop = await waitSmoke(client, (state) => state.currentMap?.public_map_key === mapKey && findKnownDrop(state, expectedDrop), `${label} server-created loot drop`, 15000);
  const drop = findKnownDrop(withDrop, expectedDrop);
  assertDropMatches(drop, expectedDrop, `${label} loot drop`);
  assertNoPayloadLeak({ drop }, `${label} smoke loot drop`);

  const pickupDropID = drop.drop_id ?? expectedDrop.drop_id ?? expectedDrop.entity_id;
  const beforeCargo = withDrop.cargo;
  await moveToPosition(client, drop.position, Math.max(45, (withDrop.stats?.loot_pickup_range ?? 120) - 25), `loot drop ${pickupDropID}`, 30000);
  const pickupPayload = payloadOf(await send(client, 'loot.pickup', { drop_id: pickupDropID }), 'loot.pickup');
  assert(pickupPayload.accepted === true, `loot.pickup accepted ${compact(pickupPayload)}`);
  assertCargoPickup(pickupPayload.cargo, beforeCargo, drop, 'loot.pickup response cargo');
  assertNoPayloadLeak(pickupPayload, 'loot.pickup response');

  const withCargo = await waitSmoke(
    client,
    (state) => state.currentMap?.public_map_key === mapKey && cargoIncludesPickup(state.cargo, beforeCargo, drop) && !state.knownLoot?.[pickupDropID],
    `${label} cargo reconciliation after loot pickup`,
    15000,
  );
  assertCargoPickup(withCargo.cargo, beforeCargo, drop, `${label} smoke cargo`);
  return withCargo;
}

async function fightNPCUntilKilled(client, targetID) {
  let lastPayload = null;
  for (let attempt = 1; attempt <= 4; attempt += 1) {
    const combatPayload = payloadOf(await send(client, 'combat.use_skill', { skill_id: 'basic_laser', target_id: targetID }), 'combat.use_skill');
    assert(combatPayload.accepted === true, `combat.use_skill accepted ${compact(combatPayload)}`);
    assertNoPayloadLeak(combatPayload, `combat.use_skill response ${attempt}`);
    lastPayload = combatPayload;
    if (combatPayload.killed === true) return combatPayload;
    await waitCombatCooldown(client, combatPayload);
  }
  throw new Error(`combat.use_skill did not kill target: ${compact(lastPayload)}`);
}

async function waitCombatCooldown(client, combatPayload) {
  const readyAt = Number(combatPayload.cooldown_ready_at_ms ?? 0);
  const delayMS = readyAt > Date.now() ? readyAt - Date.now() + 75 : 100;
  await delay(Math.min(Math.max(delayMS, 100), 5000));
  await waitSmoke(
    client,
    (state) => state.connectionStatus === 'connected' && (readyAt <= 0 || Date.now() >= readyAt || (state.serverNow ?? 0) >= readyAt),
    'basic_laser cooldown',
    Math.max(1500, Math.min(delayMS + 1500, 6500)),
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
    if (response.ok !== true && response.error?.code === 'ERR_RATE_LIMITED') {
      await delay(125);
      continue;
    }
    payloadOf(response, 'move_to');
    const eta = Math.ceil((distance(position, target) / Math.max(1, self?.movement?.speed ?? 180)) * 1000);
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

function payloadOf(response, label) {
  assert(response?.ok === true, `${label} failed: ${compact(response)}`);
  const payload = typeof response.payload === 'string' ? JSON.parse(response.payload) : response.payload;
  assert(payload && typeof payload === 'object', `${label} payload missing`);
  assertNoPayloadLeak(payload, `${label} payload`);
  return payload;
}

function findHostileNPC(state) {
  return Object.values(state?.visibleEntities ?? {}).find(isLiveNPC) ?? null;
}

function isLiveNPC(entity) {
  if (entity?.entity_type !== 'npc' || !entity.position) return false;
  if (!entity.combat) return true;
  const hp = Number(entity.combat.hp ?? 0);
  return hp > 0 && !['dead', 'destroyed', 'disabled'].includes(String(entity.combat.status ?? 'active').toLowerCase());
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
  const itemID = drop.item_id;
  const quantity = Number(drop.quantity);
  if (!itemID || !(quantity > 0)) return false;
  return cargoQuantity(cargo, itemID) >= cargoQuantity(beforeCargo, itemID) + quantity;
}

function cargoQuantity(cargo, itemID) {
  return Number(cargo?.items?.find((item) => item.item_id === itemID)?.quantity ?? 0);
}

function assertSafeScanPayload(payload, label) {
  assert(payload?.scan, `${label} missing scan payload ${compact(payload)}`);
  assert(['started', 'no_signal', 'planet_discovered', 'player_revealed'].includes(payload.scan.status), `${label} scan status ${compact(payload.scan)}`);
  assert(payload.scan.pulse_reference, `${label} missing pulse reference ${compact(payload.scan)}`);
  assertNoPayloadLeak(payload, label);
}

function assertClaimResponsePayload(payload, planetID) {
  assert(payload?.claim?.accepted === true, `claim accepted missing ${compact(payload)}`);
  assert(payload.claim?.planet?.planet_id === planetID, `claim planet mismatch ${compact(payload.claim)}`);
  assert(payload.claim?.planet?.owner_status === 'owned_by_you', `claim owner status ${compact(payload.claim?.planet)}`);
  assert((payload.production?.planets ?? []).some((planet) => planet.planet_id === planetID), `claim response missing production ${compact(payload.production)}`);
  assert(inventoryQuantity(payload.inventory, 'x_core') === 0, `claim response inventory still has x_core ${compact(payload.inventory)}`);
  assertNoPayloadLeak(payload, 'claim response');
}

function assertBuildingBuildResponsePayload(payload, planetID, buildingID) {
  assert(payload?.building?.planet_id === planetID, `building planet mismatch ${compact(payload)}`);
  assert(payload.building?.building_id === buildingID, `building id mismatch ${compact(payload.building)}`);
  assert(payload.building?.building_type === 'iron_extractor', `building type mismatch ${compact(payload.building)}`);
  assert(payload.building?.level === 1, `building level mismatch ${compact(payload.building)}`);
  assert((payload.production?.planets ?? []).some((planet) => productionBuilding({ production: payload.production }, planetID, buildingID)), `building response missing production ${compact(payload.production)}`);
  assert(payload.wallet && typeof payload.wallet.credits === 'number', `building response missing wallet ${compact(payload.wallet)}`);
  assertNoPayloadLeak(payload, 'building build response');
}

function assertPortalResponsePayload(payload, expectedMapKey, expectedDisplay) {
  assert(payload.accepted === true, `portal accepted missing ${compact(payload)}`);
  assert(payload.to_public_map_key === expectedMapKey, `portal destination ${payload.to_public_map_key}, want ${expectedMapKey}`);
  assert(payload.snapshot?.map?.public_map_key === expectedMapKey, `portal snapshot map ${compact(payload.snapshot?.map)}`);
  assert(new RegExp(expectedDisplay, 'i').test(payload.snapshot?.map?.display_name ?? ''), `portal snapshot display ${compact(payload.snapshot?.map)}`);
  assertNoPayloadLeak(payload, 'portal response');
}

function assertOuterMap(state) {
  const map = state?.currentMap;
  assert(map?.public_map_key === '1-2', `outer map key ${map?.public_map_key}`);
  assert(/Outer Ring/i.test(map.display_name ?? ''), `outer display ${map.display_name}`);
  assertBounds(map.bounds);
  const portals = new Set((map.visible_portals ?? []).map((portal) => portal.portal_id));
  assert(portals.has('west_gate'), 'west_gate portal visible');
  assert(!portals.has('east_gate'), 'east_gate portal absent after transfer');
  const self = selfEntity(state);
  assert(self?.entity_type === 'player', 'outer self entity visible');
}

function assertBounds(bounds) {
  assert(bounds?.min_x === 0 && bounds?.min_y === 0 && bounds?.max_x === 10000 && bounds?.max_y === 10000, `bounds ${compact(bounds)}`);
}

function assertNoOriginMapLeakage(origin, outer) {
  const originSelfID = selfEntity(origin)?.entity_id;
  const originNonSelfIDs = entityIDs(origin).filter((id) => id !== originSelfID);
  const outerEntityIDs = new Set(entityIDs(outer));
  for (const id of originNonSelfIDs) {
    assert(!outerEntityIDs.has(id), `origin entity leaked after transfer: ${id}`);
  }

  const outerContactIDs = new Set((outer.minimap?.live_contacts ?? []).map((contact) => contact.entity_id).filter(Boolean));
  for (const id of originNonSelfIDs) {
    assert(!outerContactIDs.has(id), `origin minimap contact leaked after transfer: ${id}`);
  }

  for (const id of Object.keys(origin.knownLoot ?? {})) {
    assert(!(id in (outer.knownLoot ?? {})), `origin loot leaked after transfer: ${id}`);
  }

  const minimapPortalIDs = new Set((outer.minimap?.visible_portals ?? []).map((portal) => portal.portal_id));
  assert(!minimapPortalIDs.has('east_gate'), 'origin east_gate leaked into destination minimap');
  assert(outer.selectedTargetID === null || !originNonSelfIDs.includes(outer.selectedTargetID), 'origin selected target survived transfer');
  assert(outer.movementTarget === null, 'origin movement target survived transfer');
  assert((outer.mapSubscriptionEpoch ?? 0) > (origin.mapSubscriptionEpoch ?? 0), 'destination epoch did not advance');
}

function entityIDs(state) {
  return Object.keys(state?.visibleEntities ?? {});
}

function discoveredPlanetID(state) {
  return state?.planetIntel?.lastScan?.planet_id || state?.planetIntel?.planets?.find((planet) => planet.owner_status !== 'owned_by_you')?.planet_id || '';
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

function productionBuilding(state, planetID, buildingID) {
  return state?.production?.planets
    ?.find((planet) => planet.planet_id === planetID)
    ?.buildings?.find((building) => building.building_id === buildingID) ?? null;
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

async function closeModalIfOpen(client) {
  const backdrop = client.page.locator('[data-modal-close="backdrop"]').first();
  if ((await backdrop.count()) > 0) {
    await backdrop.click({ timeout: 1000 }).catch(() => client.page.keyboard.press('Escape'));
  } else {
    await client.page.keyboard.press('Escape');
  }
  await client.page
    .waitForFunction(() => !document.querySelector('.hud__modal-layer[data-open="true"]'), null, { timeout: 5000 })
    .catch(() => {});
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

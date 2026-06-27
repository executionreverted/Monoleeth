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
const useBuiltClientServer = process.env.PHASE10_BUILT_CLIENT === '1';
const originNpcApproachTarget = { x: 800, y: 400 };
const eastGateTarget = { x: 9800, y: 5000 };
const skirmishGateTarget = { x: 9800, y: 5000 };
const pvpAttackerTarget = { x: 900, y: 5000 };
const pvpTargetTarget = { x: 980, y: 5000 };
const pvpRespawnCheckpoint = { x: 400, y: 5000 };
const safeZoneRadius = 260;
const protectedUIClickOffset = { x: pvpRespawnCheckpoint.x + 170, y: pvpRespawnCheckpoint.y };
const commandTimeoutMS = 12000;

const strictLeakTokens = [
  'map_1_1',
  'map_1_2',
  'map_1_3',
  'internal_map_id',
  'destination_map_id',
  'source_map_id',
  'spawn_id',
  'death_id',
  'respawn_location_id',
  'lethal_event_key',
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

const broadLeakTokens = strictLeakTokens;

async function main() {
  const serverPort = await freePort();
  const serverTarget = `http://127.0.0.1:${serverPort}`;
  const clientPort = useBuiltClientServer ? 0 : await freePort();
  const clientOrigin = useBuiltClientServer ? serverTarget : `http://127.0.0.1:${clientPort}`;
  const goEnv = {
    GAME_SERVER_ADDR: `127.0.0.1:${serverPort}`,
    GAME_ALLOWED_ORIGINS: clientOrigin,
    GAME_DEV_MODE: '1',
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
    const attacker = await newClient(browser, clientOrigin, {
      label: 'attacker',
      email: `phase10-attacker-${nonce}@example.test`,
      callsign: `P10A-${nonce.slice(-7)}`,
    });
    clients.push(attacker);
    const target = await newClient(browser, clientOrigin, {
      label: 'target',
      email: `phase10-target-${nonce}@example.test`,
      callsign: `P10T-${nonce.slice(-7)}`,
    });
    clients.push(target);

    const attackerOrigin = await waitSmoke(attacker, originReady, 'attacker authenticated Origin state', 30000);
    const targetOrigin = await waitSmoke(target, originReady, 'target authenticated Origin state', 30000);
    assertRealAuthenticated(attackerOrigin, attacker.callsign, 'attacker');
    assertRealAuthenticated(targetOrigin, target.callsign, 'target');
    assertMap(attackerOrigin, '1-1', 'Origin', 'attacker origin');
    assertMap(targetOrigin, '1-1', 'Origin', 'target origin');
    await assertNoBroadLeak(attacker, attackerOrigin, 'attacker origin');
    await assertNoBroadLeak(target, targetOrigin, 'target origin');

    await openCommandSocket(attacker);
    await openCommandSocket(target);

    const targetWithCargo = await completeFightLootLoop(target, {
      mapLabel: 'Origin',
      approachTarget: originNpcApproachTarget,
      expectedMapKey: '1-1',
    });
    const deathCargo = cargoStack(targetWithCargo.cargo, 'raw_ore');
    assert(deathCargo.quantity >= 3, `target real cargo before PvP death = ${JSON.stringify(targetWithCargo.cargo)}`);
    await assertNoBroadLeak(target, targetWithCargo, 'target origin cargo');

    await moveBothToPortal([attacker, target], eastGateTarget, 'east_gate');
    await Promise.all([
      enterPortal(attacker, 'east_gate', '1-2', 'Outer Ring'),
      enterPortal(target, 'east_gate', '1-2', 'Outer Ring'),
    ]);
    await assertBothOnMap([attacker, target], '1-2', 'Outer Ring');

    await moveBothToPortal([attacker, target], skirmishGateTarget, 'skirmish_gate');
    await Promise.all([
      enterPortal(attacker, 'skirmish_gate', '1-3', 'Border Skirmish'),
      enterPortal(target, 'skirmish_gate', '1-3', 'Border Skirmish'),
    ]);
    await assertBothOnMap([attacker, target], '1-3', 'Border Skirmish');

    const safeTarget = await waitForVisiblePlayer(attacker, target.callsign, 'target visible at protected PvP spawn');
    const protectedPVP = await send(attacker, 'combat.use_skill', {
      skill_id: 'basic_laser',
      target_id: safeTarget.entity_id,
    });
    assert(protectedPVP?.ok === false, `protected PvP command accepted: ${JSON.stringify(protectedPVP)}`);
    assert(protectedPVP.error?.code === 'ERR_PVP_BLOCKED', `protected PvP error = ${JSON.stringify(protectedPVP.error)}`);
    assertNoStrictLeak(protectedPVP, 'protected PvP rejection');
    await moveToPosition(attacker, protectedUIClickOffset, 35, 'attacker protected UI click offset', 30000);
    const uiSafeTarget = await waitForVisiblePlayer(attacker, target.callsign, 'target visible for protected PvP UI click');
    await proveProtectedPVPUIClickRejected(attacker, uiSafeTarget.entity_id);

    await waitOutProtections([attacker, target]);
    await Promise.all([
      moveToPosition(attacker, pvpAttackerTarget, 35, 'attacker outside PvP safe zone', 30000),
      moveToPosition(target, pvpTargetTarget, 35, 'target outside PvP safe zone', 30000),
    ]);
    const attackerReady = await waitSmoke(
      attacker,
      (state) => state.currentMap?.public_map_key === '1-3' && distance(positionNow(selfEntity(state), state), pvpRespawnCheckpoint) > safeZoneRadius,
      'attacker outside PvP safe radius',
      10000,
    );
    const targetReady = await waitSmoke(
      target,
      (state) => state.currentMap?.public_map_key === '1-3' && distance(positionNow(selfEntity(state), state), pvpRespawnCheckpoint) > safeZoneRadius,
      'target outside PvP safe radius',
      10000,
    );
    assert(distance(positionNow(selfEntity(attackerReady), attackerReady), positionNow(selfEntity(targetReady), targetReady)) <= 160, 'PvP ships in weapon range');

    await resetWebSocketFrames(attacker);
    await resetWebSocketFrames(target);

    const deathPayload = await fightPVPUntilDisabled(attacker, target, deathCargo);
    const deathDrop = responseDrop(deathPayload, 'PvP death response');
    assert(deathDrop.item_id === 'raw_ore', `PvP death drop item ${deathDrop.item_id}`);
    assert(deathDrop.quantity >= deathCargo.quantity, `PvP death drop quantity ${deathDrop.quantity}, cargo ${deathCargo.quantity}`);
    assertNoStrictLeak(deathPayload, 'PvP death response');

    const attackerWithDrop = await waitSmoke(
      attacker,
      (state) => state.currentMap?.public_map_key === '1-3' && findKnownDrop(state, deathDrop),
      'attacker-owned PvP death cargo drop',
      15000,
    );
    assertDropMatches(findKnownDrop(attackerWithDrop, deathDrop), deathDrop, 'attacker death cargo smoke drop');

    const targetDisabled = await waitSmoke(
      target,
      (state) =>
        state.currentMap?.public_map_key === '1-3' &&
        state.ship?.disabled === true &&
        isDisabledRepairState(state.ship?.repair_state) &&
        state.repairQuote?.disabled === true &&
        cargoQuantity(state.cargo, deathCargo.item_id) === 0,
      'target disabled ship and cargo reconciliation',
      15000,
    );
    assertDeathStateSurfaceSafe(targetDisabled, 'target disabled smoke state');

    const blockedAfterDeath = await send(target, 'combat.use_skill', {
      skill_id: 'basic_laser',
      target_id: selfEntity(targetDisabled).entity_id,
    });
    assert(blockedAfterDeath?.ok === false, `target post-death command accepted: ${JSON.stringify(blockedAfterDeath)}`);
    assert(blockedAfterDeath.error?.code === 'ERR_SHIP_DISABLED', `target post-death error = ${JSON.stringify(blockedAfterDeath.error)}`);
    assertNoStrictLeak(blockedAfterDeath, 'target post-death action block');

    const quotePayload = payloadOf(await send(target, 'death.repair_quote', {}), 'death.repair_quote');
    assert(quotePayload.disabled === true, `repair quote disabled = ${JSON.stringify(quotePayload)}`);
    assert(quotePayload.ship_id, `repair quote ship_id missing ${JSON.stringify(quotePayload)}`);
    assert(quotePayload.quote_id, `repair quote quote_id missing ${JSON.stringify(quotePayload)}`);
    assertNoStrictLeak(quotePayload, 'repair quote response');

    const repairResponse = await send(target, 'death.repair_ship', quotePayload);
    assert(repairResponse?.ok === true, `death.repair_ship failed with quote ${JSON.stringify(quotePayload)}: ${JSON.stringify(repairResponse)}`);
    const repairPayload = payloadOf(repairResponse, 'death.repair_ship');
    assertRepairPayload(repairPayload);
    assertNoStrictLeak(repairPayload, 'repair ship response');

    const targetRepaired = await waitSmoke(
      target,
      (state) =>
        state.currentMap?.public_map_key === '1-3' &&
        state.ship?.disabled === false &&
        state.ship?.repair_state === 'ready' &&
        state.repairQuote === null &&
        near(positionNow(selfEntity(state), state), pvpRespawnCheckpoint, 3) &&
        state.currentMap?.protection?.reason === 'respawn' &&
        state.currentMap?.protection?.blocks_pvp === true,
      'target repaired checkpoint/protection reconciliation',
      15000,
    );
    assert(targetRepaired.lastCorrection?.entityID === selfEntity(targetRepaired).entity_id, 'repair position correction targets self');
    assert(near(targetRepaired.lastCorrection?.position, pvpRespawnCheckpoint, 3), `repair correction = ${JSON.stringify(targetRepaired.lastCorrection)}`);
    assertDeathStateSurfaceSafe(targetRepaired, 'target repaired smoke state');
    await assertStrictDeathStorageCanary([attacker, target]);
    await assertStrictDeathWebSocketCanary([attacker, target]);
    assertProcessLogCanary([goServer, viteServer]);

    console.log(
      `phase10-pvp-death smoke ok target_cargo=${deathCargo.item_id}x${deathCargo.quantity} death_drop=${deathDrop.drop_id} repair_map=${repairPayload.public_map_key} repair_position=${fmt(repairPayload.position)}`,
    );
  } finally {
    for (const client of clients) {
      await client.page.evaluate(() => window.__phase10CommandSocket?.close()).catch(() => {});
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
    if (window.__phase10WebSocketCanaryInstalled) return;
    window.__phase10WebSocketCanaryInstalled = true;

    const NativeWebSocket = window.WebSocket;
    const maxTextLength = 1_000_000;
    const frames = [];
    const state = { nextSocketID: 1, nextFrameIndex: 0 };
    window.__phase10WebSocketFrames = frames;

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

    class Phase10WebSocket extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        const socketID = state.nextSocketID++;
        this.__phase10SocketID = socketID;
        this.addEventListener('message', (event) => capture('in', socketID, this.url || args[0], event.data));
      }

      send(data) {
        capture('out', this.__phase10SocketID ?? 0, this.url, data);
        return super.send(data);
      }
    }

    window.WebSocket = Phase10WebSocket;
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
        if (window.__phase10CommandSocket?.readyState === WebSocket.OPEN) return resolve(true);
        const socket = new WebSocket(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`);
        window.__phase10CommandSocket = socket;
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
    request_id: `phase10-${client.label}-${op.replace(/[^a-z0-9]+/gi, '-')}-${Date.now()}-${client.seq}`,
    op,
    payload,
    client_seq: client.seq++,
    v: 1,
  };
  return client.page.evaluate(
    ({ message, timeoutMS }) =>
      new Promise((resolve, reject) => {
        const socket = window.__phase10CommandSocket;
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

async function completeFightLootLoop(client, { mapLabel, approachTarget, expectedMapKey }) {
  if (!findHostileNPC(await smoke(client))) {
    await moveToPosition(client, approachTarget, 260, `${mapLabel} hostile radar approach`, 30000);
  }
  const withNPC = await waitSmoke(
    client,
    (state) => state.currentMap?.public_map_key === expectedMapKey && findHostileNPC(state),
    `visible ${mapLabel} hostile NPC`,
    15000,
  );
  const npc = findHostileNPC(withNPC);
  assert(npc, `${mapLabel} NPC target present`);

  await moveToPosition(client, npc.position, Math.max(80, Math.min(220, (withNPC.stats?.weapon_range ?? 260) - 40)), `combat target ${npc.entity_id}`, 30000);
  const combatPayload = await fightNPCUntilKilled(client, npc.entity_id, mapLabel);

  const expectedDrop = responseDrop(combatPayload, `${mapLabel} combat response`);
  const withDrop = await waitSmoke(
    client,
    (state) => state.currentMap?.public_map_key === expectedMapKey && findKnownDrop(state, expectedDrop),
    `server-created ${mapLabel} loot drop`,
    15000,
  );
  const drop = findKnownDrop(withDrop, expectedDrop);
  assertDropMatches(drop, expectedDrop, `${mapLabel} loot drop`);

  const pickupDropID = drop.drop_id ?? expectedDrop.drop_id ?? expectedDrop.entity_id;
  assert(pickupDropID, `${mapLabel} pickup drop id present`);
  const beforeCargo = withDrop.cargo;
  await moveToPosition(client, drop.position, Math.max(45, (withDrop.stats?.loot_pickup_range ?? 120) - 25), `loot drop ${pickupDropID}`, 30000);
  const pickupPayload = payloadOf(await send(client, 'loot.pickup', { drop_id: pickupDropID }), 'loot.pickup');
  assert(pickupPayload.accepted === true, `${mapLabel} loot pickup accepted ${JSON.stringify(pickupPayload)}`);
  assertCargoPickup(pickupPayload.cargo, beforeCargo, drop, `${mapLabel} pickup response cargo`);
  assertNoBroadLeakPayload(pickupPayload, `${mapLabel} loot pickup response`);

  const withCargo = await waitSmoke(
    client,
    (state) => state.currentMap?.public_map_key === expectedMapKey && cargoIncludesPickup(state.cargo, beforeCargo, drop) && !state.knownLoot?.[pickupDropID],
    `${mapLabel} cargo reconciliation after loot pickup`,
    15000,
  );
  assertCargoPickup(withCargo.cargo, beforeCargo, drop, `${mapLabel} smoke cargo`);
  return withCargo;
}

async function fightNPCUntilKilled(client, targetID, mapLabel) {
  let lastPayload = null;
  for (let attempt = 1; attempt <= 5; attempt += 1) {
    const combatPayload = payloadOf(await send(client, 'combat.use_skill', { skill_id: 'basic_laser', target_id: targetID }), 'combat.use_skill');
    assert(combatPayload.accepted === true, `${mapLabel} combat accepted ${JSON.stringify(combatPayload)}`);
    assertNoBroadLeakPayload(combatPayload, `${mapLabel} combat response ${attempt}`);
    lastPayload = combatPayload;
    if (combatPayload.killed === true) {
      assert(combatPayload.amount > 0, `${mapLabel} lethal combat amount ${JSON.stringify(combatPayload)}`);
      return combatPayload;
    }
    await waitCombatCooldown(client, combatPayload, mapLabel);
  }
  throw new Error(`${mapLabel} combat did not kill target after retries: ${JSON.stringify(lastPayload)}`);
}

async function fightPVPUntilDisabled(attacker, target, targetCargo) {
  let targetEntity = await waitForVisiblePlayer(attacker, target.callsign, 'target visible before lethal PvP');
  let lastPayload = null;
  for (let attempt = 1; attempt <= 24; attempt += 1) {
    const combatPayload = payloadOf(
      await send(attacker, 'combat.use_skill', { skill_id: 'basic_laser', target_id: targetEntity.entity_id }),
      `PvP combat attempt ${attempt}`,
    );
    assert(combatPayload.accepted === true, `PvP combat accepted ${JSON.stringify(combatPayload)}`);
    assertNoStrictLeak(combatPayload, `PvP combat response ${attempt}`);
    lastPayload = combatPayload;
    if (combatPayload.killed === true) {
      assert(responseDrop(combatPayload, 'PvP lethal response').quantity >= targetCargo.quantity, `PvP lethal drop did not include target cargo ${JSON.stringify(combatPayload)}`);
      return combatPayload;
    }
    assert(combatPayload.target?.status === 'active', `PvP target status after nonlethal hit ${JSON.stringify(combatPayload)}`);
    await waitCombatCooldown(attacker, combatPayload, 'PvP');
    targetEntity = findPlayerByCallsign(await smoke(attacker), target.callsign) ?? targetEntity;
  }
  throw new Error(`PvP combat did not disable target after retries: ${JSON.stringify(lastPayload)}`);
}

async function waitCombatCooldown(client, combatPayload, label) {
  const readyAt = Number(combatPayload.cooldown_ready_at_ms ?? 0);
  const delayMS = readyAt > Date.now() ? readyAt - Date.now() + 75 : 100;
  await delay(Math.min(Math.max(delayMS, 100), 5000));
  await waitSmoke(
    client,
    (state) => state.connectionStatus === 'connected' && (readyAt <= 0 || Date.now() >= readyAt || (state.serverNow ?? 0) >= readyAt),
    `${label} basic_laser cooldown`,
    Math.max(1500, Math.min(delayMS + 1500, 6500)),
  );
}

async function moveBothToPortal(clients, position, portalID) {
  await Promise.all(clients.map((client) => moveToPosition(client, position, 120, `${client.label} ${portalID}`, 90000)));
}

async function enterPortal(client, portalID, expectedMapKey, expectedDisplay) {
  const before = await smoke(client);
  const portal = findVisiblePortal(before, portalID);
  assert(portal, `${client.label} visible portal ${portalID}`);
  const response = await send(client, 'portal.enter', { portal_id: portalID });
  const payload = payloadOf(response, `${client.label} portal.enter ${portalID}`);
  assert(payload.accepted === true, `${client.label} portal payload ${JSON.stringify(payload)}`);
  assert(payload.to_public_map_key === expectedMapKey, `${client.label} portal destination ${payload.to_public_map_key}`);
  assert(payload.snapshot?.map?.public_map_key === expectedMapKey, `${client.label} portal snapshot map ${JSON.stringify(payload.snapshot?.map)}`);
  assertNoBroadLeakPayload(payload, `${client.label} portal response`);
  const transferred = await waitSmoke(
    client,
    (state) => state.currentMap?.public_map_key === expectedMapKey,
    `${client.label} ${expectedDisplay} map state`,
    15000,
  );
  assertMap(transferred, expectedMapKey, expectedDisplay, `${client.label} ${expectedDisplay}`);
  return transferred;
}

async function assertBothOnMap(clients, mapKey, display) {
  await Promise.all(
    clients.map(async (client) => {
      const state = await waitSmoke(client, (candidate) => candidate.currentMap?.public_map_key === mapKey, `${client.label} on ${mapKey}`, 10000);
      assertMap(state, mapKey, display, `${client.label} ${display}`);
      await assertNoBroadLeak(client, state, `${client.label} ${mapKey}`);
    }),
  );
}

async function moveToPosition(client, targetPosition, arriveDistance, label, timeoutMS) {
  assert(targetPosition, `${label} target position present`);
  const deadline = Date.now() + timeoutMS;
  while (Date.now() < deadline) {
    await waitSmoke(client, (state) => state.connectionStatus === 'connected' && Object.keys(state.pendingCommands ?? {}).length === 0, `${client.label} pending commands clear`, 10000);
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
    ok(response, `${client.label} move_to`);
    const eta = Math.ceil((distance(position, target) / Math.max(1, self?.movement?.speed ?? 180)) * 1000);
    await waitSmoke(
      client,
      (candidate) => {
        const pos = positionNow(selfEntity(candidate), candidate);
        return distance(pos, target) <= 35 || distance(pos, targetPosition) <= arriveDistance;
      },
      `${client.label} movement to ${fmt(target)}`,
      Math.max(5000, eta + 5000),
    );
  }
  throw new Error(`${client.label} timed out before reaching ${label} at ${fmt(targetPosition)}`);
}

async function waitForVisiblePlayer(client, callsign, description) {
  const state = await waitSmoke(client, (candidate) => findPlayerByCallsign(candidate, callsign), description, 15000);
  return findPlayerByCallsign(state, callsign);
}

async function proveProtectedPVPUIClickRejected(client, targetID) {
  await resetWebSocketFrames(client);
  const selectedByTargetSelect = await clickTargetSelect(client, targetID);
  assert(selectedByTargetSelect === true, `${client.label} target-select click did not select ${targetID}`);
  await waitSmoke(client, (state) => state.selectedTargetID === targetID, 'protected PvP UI target selection', 5000);
  const fireButton = client.page.locator('button[data-action="fire"][data-quick-action="laser"]');
  await fireButton.waitFor({ state: 'visible', timeout: 5000 });
  const readiness = await fireButtonReadiness(client, targetID);
  assert(readiness.disabled === false, `${client.label} protected PvP Fire button disabled before UI proof: ${JSON.stringify(readiness)}`);
  await fireButton.click();
  const exchange = await waitForUICombatError(client, targetID, 'ERR_PVP_BLOCKED', 'protected PvP UI click rejection', 10000);
  assertNoStrictLeak(exchange.outbound, 'protected PvP UI outbound command');
  assertNoStrictLeak(exchange.inbound, 'protected PvP UI inbound rejection');
}

async function clickTargetSelect(client, targetID) {
  const deadline = Date.now() + 5000;
  let observed = [];
  while (Date.now() < deadline) {
    const buttons = await client.page.locator('button[data-action="target-select"]').elementHandles();
    observed = [];
    for (const button of buttons) {
      const info = await button.evaluate((node) => {
        const rect = node.getBoundingClientRect();
        return {
          entityID: node.getAttribute('data-entity-id'),
          entityType: node.getAttribute('data-entity-type'),
          targetSource: node.getAttribute('data-target-source'),
          disabled: node.disabled === true,
          title: node.getAttribute('title'),
          text: node.textContent?.replace(/\s+/g, ' ').trim() ?? '',
          visible: rect.width > 0 && rect.height > 0,
          rect: {
            x: Math.round(rect.x),
            y: Math.round(rect.y),
            width: Math.round(rect.width),
            height: Math.round(rect.height),
          },
        };
      });
      observed.push(info);
      if (info.entityID === targetID) {
        assert(info.disabled === false, `${client.label} target-select ${targetID} is disabled. Observed: ${JSON.stringify(observed)}`);
        try {
          await button.click({ timeout: 5000 });
        } catch (error) {
          throw new Error(
            `${client.label} failed to click real target-select button for ${targetID}: ${error?.message ?? error}. Observed: ${JSON.stringify(observed)}`,
          );
        }
        try {
          await waitSmoke(client, (state) => state.selectedTargetID === targetID, 'real target-select button selection', 2000);
        } catch (error) {
          const state = await smoke(client);
          throw new Error(
            `${client.label} clicked target-select ${targetID} but selectedTargetID is ${state?.selectedTargetID ?? null}. Observed: ${JSON.stringify(
              observed,
            )}. ${error?.message ?? error}`,
          );
        }
        return true;
      }
    }
    await delay(100);
  }
  throw new Error(`${client.label} missing real target-select button for ${targetID}. Observed: ${JSON.stringify(observed)}`);
}

async function waitForUICombatError(client, targetID, errorCode, description, timeoutMS) {
  const started = Date.now();
  let lastFrames = [];
  while (Date.now() - started < timeoutMS) {
    const parsedFrames = (await websocketFrames(client)).map((frame) => ({ ...frame, parsed: parseFrameJSON(frame.text) }));
    lastFrames = parsedFrames;
    const outbound = parsedFrames.find(
      (frame) =>
        frame.direction === 'out' &&
        frame.parsed?.op === 'combat.use_skill' &&
        frame.parsed?.payload?.skill_id === 'basic_laser' &&
        frame.parsed?.payload?.target_id === targetID,
    );
    if (outbound) {
      assert(outbound.parsed.request_id, `${client.label} UI combat outbound missing request_id`);
      assert(outbound.parsed.v === 1, `${client.label} UI combat outbound version ${outbound.parsed.v}`);
      const inbound = parsedFrames.find(
        (frame) =>
          frame.direction === 'in' &&
          frame.parsed?.request_id === outbound.parsed.request_id &&
          frame.parsed?.ok === false,
      );
      if (inbound) {
        assert(inbound.parsed.error?.code === errorCode, `${client.label} ${description} error = ${JSON.stringify(inbound.parsed.error)}`);
        return { outbound: outbound.parsed, inbound: inbound.parsed };
      }
    }
    await delay(100);
  }
  throw new Error(`${client.label} timed out waiting for ${description}. Frames: ${JSON.stringify(lastFrames.slice(-12))}`);
}

async function fireButtonReadiness(client, targetID) {
  return client.page.evaluate((entityID) => {
    const button = document.querySelector('button[data-action="fire"][data-quick-action="laser"]');
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return {
      disabled: button?.disabled === true,
      title: button?.getAttribute('title') ?? null,
      text: button?.textContent?.replace(/\s+/g, ' ').trim() ?? null,
      selectedTargetID: state?.selectedTargetID ?? null,
      target: state?.visibleEntities?.[entityID] ?? null,
      ship: state?.ship
        ? {
            disabled: state.ship.disabled,
            capacitor: state.ship.capacitor,
            max_capacitor: state.ship.max_capacitor,
            repair_state: state.ship.repair_state,
          }
        : null,
      stats: state?.stats
        ? {
            basic_laser_energy_cost: state.stats.basic_laser_energy_cost,
            basic_laser_cooldown_ms: state.stats.basic_laser_cooldown_ms,
            weapon_range: state.stats.weapon_range,
          }
        : null,
      connectionStatus: state?.connectionStatus ?? null,
      pending: Object.values(state?.pendingCommands ?? {}).map((entry) => entry.op),
      skillCooldown: state?.skillCooldowns?.basic_laser ?? null,
      serverNow: state?.serverNow ?? null,
    };
  }, targetID);
}

async function websocketFrames(client) {
  return client.page.evaluate(() =>
    (window.__phase10WebSocketFrames ?? []).map((frame) => ({
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

async function waitOutProtections(clients) {
  const states = await Promise.all(clients.map((client) => smoke(client)));
  const expiresAt = Math.max(0, ...states.map((state) => Number(state?.currentMap?.protection?.expires_at ?? 0)));
  const delayMS = expiresAt > 0 ? expiresAt - Date.now() + 300 : 10500;
  if (delayMS > 0) await delay(Math.min(delayMS, 12000));
}

async function resetWebSocketFrames(client) {
  await client.page.evaluate(() => {
    if (Array.isArray(window.__phase10WebSocketFrames)) {
      window.__phase10WebSocketFrames.splice(0, window.__phase10WebSocketFrames.length);
    }
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
  throw new Error(`Timed out waiting for ${client.label} ${description}. Last state: ${compact(last)}`);
}

function selfEntity(state) {
  const entities = Object.values(state?.visibleEntities ?? {});
  return entities.find((entity) => entity.status_flags?.includes('self')) ?? entities.find((entity) => entity.entity_type === 'player') ?? null;
}

function findPlayerByCallsign(state, callsign) {
  return (
    Object.values(state?.visibleEntities ?? {}).find(
      (entity) =>
        entity.entity_type === 'player' &&
        !entity.status_flags?.includes('self') &&
        entity.display?.label === callsign &&
        entity.position,
    ) ?? null
  );
}

function findHostileNPC(state) {
  return Object.values(state?.visibleEntities ?? {}).find((entity) => entity.entity_type === 'npc' && entity.position && (entity.combat?.hp ?? 1) > 0) ?? null;
}

function findVisiblePortal(state, portalID) {
  return (state?.currentMap?.visible_portals ?? []).find((portal) => portal.portal_id === portalID) ?? null;
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

function near(a, b, tolerance) {
  return distance(a, b) <= tolerance;
}

function responseDrop(combatPayload, label) {
  const drop = Array.isArray(combatPayload.drops)
    ? combatPayload.drops.find((entry) => entry?.item_id && Number(entry.quantity) > 0 && (entry.drop_id || entry.entity_id))
    : null;
  assert(drop, `${label} includes server loot drop ${JSON.stringify(combatPayload)}`);
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
  assert(Number(drop.quantity) >= expectedDrop.quantity, `${label} quantity ${drop.quantity}, want at least ${expectedDrop.quantity}`);
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

function cargoStack(cargo, itemID) {
  const item = cargo?.items?.find((entry) => entry.item_id === itemID);
  return { item_id: itemID, quantity: Number(item?.quantity ?? 0) };
}

function cargoQuantity(cargo, itemID) {
  return Number(cargo?.items?.find((item) => item.item_id === itemID)?.quantity ?? 0);
}

function assertRepairPayload(payload) {
  assert(payload.accepted === true, `repair accepted ${JSON.stringify(payload)}`);
  assert(payload.repaired === true, `repair repaired ${JSON.stringify(payload)}`);
  assert(payload.public_map_key === '1-3', `repair public_map_key ${payload.public_map_key}`);
  assert(near(payload.position, pvpRespawnCheckpoint, 3), `repair position ${JSON.stringify(payload.position)}`);
  assert(payload.protection?.reason === 'respawn', `repair protection ${JSON.stringify(payload.protection)}`);
  assert(payload.protection?.blocks_pvp === true, `repair protection blocks_pvp ${JSON.stringify(payload.protection)}`);
  assert(payload.protection?.break_on_pvp_action === true, `repair protection break_on_pvp_action ${JSON.stringify(payload.protection)}`);
  assert(payload.ship?.disabled === false, `repair ship disabled ${JSON.stringify(payload.ship)}`);
  assert(payload.ship?.repair_state === 'ready', `repair ship state ${JSON.stringify(payload.ship)}`);
}

function isDisabledRepairState(state) {
  return state === 'disabled' || state === 'death';
}

function payloadOf(response, label) {
  ok(response, label);
  const payload = typeof response.payload === 'string' ? JSON.parse(response.payload) : response.payload;
  assert(payload && typeof payload === 'object', `${label} payload present`);
  return payload;
}

async function assertNoBroadLeak(client, state, label) {
  const body = await client.page.locator('body').innerText({ timeout: 5000 });
  assert(!body.includes('Unhandled event'), `${label} DOM has unhandled event log`);
  for (const token of broadLeakTokens) {
    assert(!body.includes(token), `${label} DOM leaked ${token}`);
  }
  assertNoBroadLeakPayload(state, `${label} smoke state`);
  const browserLeak = await browserStorageLeak(client, broadLeakTokens);
  assert(!browserLeak, `${label} browser storage leaked ${browserLeak}`);
}

function assertNoBroadLeakPayload(payload, label) {
  assertNoLeakPayload(payload, label, broadLeakTokens);
}

function assertNoStrictLeak(payload, label) {
  assertNoLeakPayload(payload, label, strictLeakTokens);
}

function assertNoLeakPayload(payload, label, tokens) {
  const json = JSON.stringify(payload);
  for (const token of tokens) {
    assert(!json.includes(token), `${label} leaked ${token}`);
  }
  const key = forbiddenKey(payload, [], tokens);
  assert(!key, `${label} leaked forbidden key ${key}`);
}

function assertDeathStateSurfaceSafe(state, label) {
  assertNoStrictLeak(
    {
      currentMap: {
        public_map_key: state.currentMap?.public_map_key,
        display_name: state.currentMap?.display_name,
        risk_band: state.currentMap?.risk_band,
        pvp_policy: state.currentMap?.pvp_policy,
        safe_zone: state.currentMap?.safe_zone,
        protection: state.currentMap?.protection,
        bounds: state.currentMap?.bounds,
      },
      ship: state.ship,
      repairQuote: state.repairQuote,
      cargo: state.cargo,
      knownLoot: state.knownLoot,
      lastCorrection: state.lastCorrection,
      visibleEntities: state.visibleEntities,
    },
    label,
  );
}

async function assertStrictDeathWebSocketCanary(clients) {
  for (const client of clients) {
    const frames = await client.page.evaluate(() =>
      (window.__phase10WebSocketFrames ?? []).map((frame) => ({
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
    const inbound = frames.filter((frame) => frame.direction === 'in').length;
    const outbound = frames.filter((frame) => frame.direction === 'out').length;
    assert(inbound > 0, `${client.label} death WebSocket canary captured no inbound frames`);
    assert(outbound > 0, `${client.label} death WebSocket canary captured no outbound frames`);
    for (const frame of frames) assertNoWebSocketFrameLeak(frame);
  }
}

async function assertStrictDeathStorageCanary(clients) {
  for (const client of clients) {
    const browserLeak = await browserStorageLeak(client, strictLeakTokens);
    assert(!browserLeak, `${client.label} death/repair browser storage or cookie leaked ${browserLeak}`);
  }
}

function assertNoWebSocketFrameLeak(frame) {
  const surface = `${frame.client_label}.websocket.${frame.direction}[${frame.index}]`;
  assert(frame.truncated !== true, `${surface} text exceeded canary scan limit`);
  if (!frame.text) return;
  for (const token of strictLeakTokens) {
    assert(!frame.text.includes(token), `${surface} leaked token ${token}`);
  }
  const parsed = parseFrameJSON(frame.text);
  if (parsed === null) return;
  const key = forbiddenKey(parsed, [], strictLeakTokens);
  assert(!key, `${surface} leaked forbidden key ${key}`);
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

function forbiddenKey(value, path = [], tokens = strictLeakTokens) {
  if (Array.isArray(value)) {
    for (let i = 0; i < value.length; i += 1) {
      const found = forbiddenKey(value[i], [...path, String(i)], tokens);
      if (found) return found;
    }
    return null;
  }
  if (!value || typeof value !== 'object') return null;
  const forbidden = new Set(tokens.map((token) => token.toLowerCase()));
  for (const [key, child] of Object.entries(value)) {
    if (forbidden.has(key.toLowerCase())) return [...path, key].join('.');
    const found = forbiddenKey(child, [...path, key], tokens);
    if (found) return found;
  }
  return null;
}

function assertProcessLogCanary(processes) {
  for (const proc of processes.filter(Boolean)) {
    for (const line of proc.logLines ?? []) {
      for (const token of strictLeakTokens) {
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
    ship: state.ship,
    repairQuote: state.repairQuote,
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

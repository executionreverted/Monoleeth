#!/usr/bin/env node
import { spawn } from 'node:child_process';
import { mkdir, writeFile } from 'node:fs/promises';
import net from 'node:net';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const clientDir = resolve(scriptDir, '../..');
const repoRoot = resolve(clientDir, '..');
const screenshotDir = resolve(repoRoot, 'output/screenshots/ui-implementation/darkorbit-feel');
const postgresContainer = 'gameproject-postgres';
const postgresUser = process.env.POSTGRES_USER || 'gameproject';
const postgresPassword = process.env.POSTGRES_PASSWORD || 'gameproject_dev_password';
const postgresBaseDB = process.env.POSTGRES_DB || 'gameproject';
const viewport = { width: 1440, height: 900 };
const maxProcessLogLines = 5000;

const leakTokens = [
  'internal_map_id',
  'destination_map_id',
  'source_map_id',
  'spawn_area_id',
  'enemy_pool_id',
  'stat_template_id',
  'drop_profile_id',
  'aggro_profile_id',
  'leash_profile_id',
  'loot_table',
  'loot_roll',
  'gameplay_seed',
  'procedural_seed',
  'world_seed',
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
  const nonce = `${Date.now()}${Math.random().toString(16).slice(2)}`;
  const dbName = `gameproject_darkorbit_feel_${nonce}`.slice(0, 60);
  let databaseURL = process.env.DARKORBIT_FEEL_DATABASE_URL || '';
  let ownsDatabase = false;
  let goServer;
  let browser;
  let context;
  let client;

  try {
    await mkdir(screenshotDir, { recursive: true });
    if (!databaseURL) {
      const port = await ensurePostgresDB(dbName);
      ownsDatabase = true;
      databaseURL = `postgres://${postgresUser}:${postgresPassword}@127.0.0.1:${port}/${dbName}?sslmode=disable`;
    }

    const serverPort = await freePort();
    const origin = `http://127.0.0.1:${serverPort}`;
    goServer = child('go-server', 'go', ['run', './cmd/game-server'], repoRoot, {
      GAME_SERVER_ADDR: `127.0.0.1:${serverPort}`,
      GAME_ALLOWED_ORIGINS: origin,
      GAME_CLIENT_STATIC_DIR: 'client/dist',
      GAME_CONTENT_DATABASE_URL: databaseURL,
      GAME_DEV_MODE: '1',
    });
    await waitHTTP(`${origin}/healthz`, 'Go server', goServer);
    await waitHTTP(`${origin}/?smoke=1`, 'built client', goServer);

    browser = await chromium.launch();
    context = await browser.newContext({ viewport });
    const page = await context.newPage();
    client = { page, seq: 1, diagnostics: [], processes: [goServer], runNotes: [], startedAt: Date.now() };
    page.on('console', (message) => {
      if (message.type() === 'error' || message.type() === 'warning') {
        client.diagnostics.push(`[console:${message.type()}] ${message.text()}`);
      }
    });
    page.on('pageerror', (error) => {
      client.diagnostics.push(`[pageerror] ${error?.stack ?? error}`);
    });
    await installWebSocketCanary(client);

    await page.goto(`${origin}/?smoke=1`, { waitUntil: 'domcontentloaded' });
    await register(client, `darkorbit-feel-${nonce}@example.test`, 'correct-password', `DO-${nonce.slice(-7)}`);
    const originState = await waitSmoke(client, originReady, 'authenticated Origin state', 30000);
    console.log('darkorbit-feel phase=origin-ready');
    note(client, 'origin-ready', {
      map: originState.currentMap?.public_map_key,
      portals: (originState.currentMap?.visible_portals ?? []).length,
    });
    assert(originState.auth?.session?.authenticated === true, 'real authenticated session missing');
    assert(originState.currentMap?.public_map_key === '1-1', `map ${originState.currentMap?.public_map_key}, want 1-1`);
    assert((originState.currentMap?.visible_portals ?? []).length > 0, 'Origin visible portal missing');
    await assertNoLeak(client, originState, 'origin bootstrap');

    await moveToPosition(client, { x: 5200, y: 4096 }, 1300, 'Origin Streuner band', 70000);
    const originCombatState = await waitSmoke(client, (state) => findLiveNPC(state), 'Origin live NPC after moving to spawn band', 20000);
    let originNPC = findLiveNPC(originCombatState);
    await moveToPosition(client, originNPC.position, Math.max(80, Math.min(220, (originCombatState.stats?.weapon_range ?? 260) - 40)), 'Origin attack target', 45000);
    const originAttackState = await waitSmoke(client, (state) => findLiveNPC(state), 'fresh Origin attack target', 10000);
    originNPC = findNearestLiveNPC(originAttackState);
    await resetWebSocketFrames(client);
    const startPayload = payloadOf(await sendDriverCommand(client, 'combatStartAttack', [originNPC.entity_id], 'combat.start_attack'), 'combat.start_attack');
    assert(startPayload.accepted === true, `combat.start_attack rejected ${compact(startPayload)}`);
    await assertExactOutboundPayload(client, 'combat.start_attack', ['target_id']);
    await waitForInboundEventCount(client, 'combat.shot_started', 2, 35000);
    const beforeMoveShotCount = inboundEventCount(await frames(client), 'combat.shot_started');
    const beforeMove = await smoke(client);
    const beforeMoveSelf = selfEntity(beforeMove);
    const nudge = combatNudge(positionNow(beforeMoveSelf, beforeMove), originNPC.position, beforeMove.stats?.weapon_range ?? 260, beforeMove.currentMap?.bounds);
    await sendDriverCommand(client, 'moveTo', [nudge], 'move_to');
    await waitForInboundEventCount(client, 'combat.shot_started', beforeMoveShotCount + 1, 35000);
    await killAndPickupOriginLoot(client, originNPC.entity_id);
    console.log('darkorbit-feel phase=origin-loot-picked');
    note(client, 'origin-loot-picked', {
      target_id: originNPC.entity_id,
      shots: inboundEventCount(await frames(client), 'combat.shot_started'),
    });
    await assertNoLeak(client, await smoke(client), 'post-origin-loot state');

    await enterForwardPortal(client, '1-2');
    console.log('darkorbit-feel phase=map-1-2');
    note(client, 'map-1-2');
    await enterForwardPortal(client, '1-3');
    console.log('darkorbit-feel phase=map-1-3');
    note(client, 'map-1-3');
    const borderArrivalState = await waitSmoke(client, (state) => state.currentMap?.public_map_key === '1-3', 'Border Skirmish map reached', 10000);
    const borderArrivalPosition = positionNow(selfEntity(borderArrivalState), borderArrivalState);
    const borderBands = [
      { x: 5200, y: 4096 },
      { x: 10400, y: 4096 },
      { x: 15600, y: 4096 },
    ].sort((a, b) => distance(borderArrivalPosition, a) - distance(borderArrivalPosition, b));
    await resetWebSocketFrames(client);
    const borderState = await moveUntilNPC(
      client,
      borderBands,
      'Border Skirmish NPC band',
      260,
    );
    let borderNPC = findLiveNPCByID(borderState, borderState.__preferredTargetID) ?? findNearestLiveNPC(borderState);
    await moveToPosition(client, borderNPC.position, Math.max(80, Math.min(220, (borderState.stats?.weapon_range ?? 260) - 40)), 'Border attack target', 45000);
    const borderAttackState = await waitSmoke(client, (state) => findLiveNPCByID(state, borderNPC.entity_id), 'fresh Border attack target', 10000);
    borderNPC = findLiveNPCByID(borderAttackState, borderNPC.entity_id);
    const borderSelf = selfEntity(borderAttackState);
    const borderSelfID = borderSelf.entity_id;
    assert(!borderAttackState.ship?.disabled, `ship disabled before Border attack ${compact(borderAttackState.ship)}`);
    const beforeReturnFireShield = Number(borderSelf.combat?.shield ?? borderAttackState.ship?.shield ?? 0);
    const beforeReturnFireHP = Number(borderSelf.combat?.hp ?? borderAttackState.ship?.hull ?? 0);
    await resetWebSocketFrames(client);
    const borderStartPayload = payloadOf(
      await sendDriverCommand(client, 'combatStartAttack', [borderNPC.entity_id], 'combat.start_attack'),
      `border combat.start_attack ${borderNPC.entity_id} ${compact({ target: borderNPC, self: borderAttackState.ship })}`,
    );
    assert(borderStartPayload.accepted === true, `border combat.start_attack rejected ${compact(borderStartPayload)}`);
    await waitForInboundEvent(
      client,
      'combat.shot_started',
      (message) => message.payload?.target_id === borderNPC.entity_id && message.payload?.skill_id === 'basic_laser',
      'Border player shot',
      35000,
    );
    await waitForInboundEvent(
      client,
      'combat.shot_started',
      (message) => message.payload?.target_id === borderSelfID && message.payload?.source_id !== borderSelfID,
      'Border NPC return-fire shot',
      45000,
    );
    await waitForInboundEvent(
      client,
      'combat.damage',
      (message) => message.payload?.target_id === borderSelfID && Number(message.payload?.amount ?? 0) > 0,
      'Border NPC return-fire damage',
      45000,
    );
    await waitSmoke(
      client,
      (state) => {
        const self = selfEntity(state);
        const shield = Number(self?.combat?.shield ?? state.ship?.shield ?? beforeReturnFireShield);
        const hp = Number(self?.combat?.hp ?? state.ship?.hull ?? beforeReturnFireHP);
        return shield < beforeReturnFireShield || hp < beforeReturnFireHP;
      },
      'Border return-fire reduced player shield or hull',
      15000,
    );
    await sendDriverCommand(client, 'combatStopAttack', [], 'combat.stop_attack');
    console.log('darkorbit-feel phase=border-return-fire');
    note(client, 'border-return-fire', {
      target_id: borderNPC.entity_id,
      self_id: borderSelfID,
      return_fire_damage_events: inboundMessages(await frames(client), 'combat.damage').filter(
        (message) => message.payload?.target_id === borderSelfID && Number(message.payload?.amount ?? 0) > 0,
      ).length,
    });
    await assertNoLeak(client, await smoke(client), 'final state');

    const desktopScreenshotPath = resolve(screenshotDir, `darkorbit-feel-desktop-${nonce}.png`);
    await page.screenshot({ path: desktopScreenshotPath, fullPage: false });
    const mobileScreenshotPath = resolve(screenshotDir, `darkorbit-feel-mobile-${nonce}.png`);
    await page.setViewportSize({ width: 390, height: 844 });
    await waitSmoke(client, (state) => state.connectionStatus === 'connected', 'mobile screenshot connected state', 10000);
    await page.screenshot({ path: mobileScreenshotPath, fullPage: false });
    note(client, 'screenshots-captured', {
      desktop: desktopScreenshotPath,
      mobile: mobileScreenshotPath,
    });
    const longRunMS = Number(process.env.DARKORBIT_FEEL_LONG_RUN_MS || 0);
    if (longRunMS > 0) {
      await observeLongRun(client, longRunMS);
    }
    const notesPath = resolve(screenshotDir, `darkorbit-feel-notes-${nonce}.json`);
    await writeRunNotes(client, notesPath, {
      desktopScreenshotPath,
      mobileScreenshotPath,
      longRunMS,
    });
    await assertWebSocketCanary(client);
    assertProcessLogCanary([goServer]);
    console.log(
      `darkorbit-feel e2e ok shots=${inboundEventCount(await frames(client), 'combat.shot_started')} desktop=${desktopScreenshotPath} mobile=${mobileScreenshotPath} notes=${notesPath}`,
    );
  } catch (error) {
    if (client?.page) {
      await writeFailureArtifact(client, nonce, error).catch((artifactError) => {
        console.error(`darkorbit-feel failure artifact error: ${artifactError?.stack ?? artifactError}`);
      });
    }
    throw error;
  } finally {
    if (context) await context.close().catch(() => {});
    if (browser) await browser.close().catch(() => {});
    if (goServer) await stop(goServer);
    if (ownsDatabase) {
      await run('docker', ['exec', postgresContainer, 'dropdb', '-U', postgresUser, '--force', '--if-exists', dbName], repoRoot).catch(() => {});
    }
  }
}

function originReady(state) {
  return state?.connectionStatus === 'connected' && state.auth?.session?.authenticated === true && state.currentMap?.public_map_key === '1-1';
}

function note(client, phase, details = {}) {
  client.runNotes.push({
    phase,
    at_ms: Date.now() - client.startedAt,
    ...details,
  });
}

async function observeLongRun(client, durationMS) {
  const deadline = Date.now() + durationMS;
  note(client, 'long-run-started', { duration_ms: durationMS });
  while (Date.now() < deadline) {
    const state = await smoke(client);
    note(client, 'long-run-sample', {
      map: state.currentMap?.public_map_key,
      visible_npcs: Object.values(state.visibleEntities ?? {}).filter((entity) => entity?.entity_type === 'npc').length,
      live_npcs: Object.values(state.visibleEntities ?? {}).filter((entity) => entity?.entity_type === 'npc' && Number(entity.combat?.hp ?? 1) > 0).length,
      ship_disabled: Boolean(state.ship?.disabled),
      pending_commands: Object.keys(state.pendingCommands ?? {}).length,
    });
    if (Object.keys(state.pendingCommands ?? {}).length === 0) {
      await sendDriverCommand(client, 'combatState', [], 'combat.state').catch((error) => {
        note(client, 'long-run-keepalive-error', { message: String(error?.message ?? error) });
      });
    }
    await delay(Math.min(15000, Math.max(250, deadline - Date.now())));
  }
  note(client, 'long-run-complete', { duration_ms: durationMS });
}

async function writeRunNotes(client, notesPath, artifacts) {
  const frameList = await frames(client);
  const state = await smoke(client);
  const notes = {
    generated_at: new Date().toISOString(),
    artifacts,
    summary: {
      map: state.currentMap?.public_map_key,
      authenticated: state.auth?.session?.authenticated === true,
      shot_started_events: inboundEventCount(frameList, 'combat.shot_started'),
      damage_events: inboundEventCount(frameList, 'combat.damage'),
      loot_created_events: inboundEventCount(frameList, 'loot.created'),
      outbound_ops: Array.from(new Set(outboundMessages(frameList).map((message) => message.op))).sort(),
      diagnostics: client.diagnostics,
    },
    phases: client.runNotes,
  };
  await writeFile(notesPath, `${JSON.stringify(notes, null, 2)}\n`);
}

async function writeFailureArtifact(client, nonce, error) {
  const failureScreenshotPath = resolve(screenshotDir, `darkorbit-feel-failure-${nonce}.png`);
  const failureNotesPath = resolve(screenshotDir, `darkorbit-feel-failure-${nonce}.json`);
  const frameList = await frames(client).catch(() => []);
  const state = await smoke(client).catch(() => null);
  await client.page.screenshot({ path: failureScreenshotPath, fullPage: false }).catch(() => {});
  await writeFile(
    failureNotesPath,
    `${JSON.stringify(
      {
        generated_at: new Date().toISOString(),
        error: String(error?.stack ?? error),
        screenshot: failureScreenshotPath,
        state: state ? smokeSummary(state) : null,
        combat_engagement: state?.combatEngagement ?? null,
        recent_inbound_events: inboundMessages(frameList).slice(-30),
        recent_outbound_ops: outboundMessages(frameList)
          .slice(-30)
          .map((message) => ({ op: message.op, request_id: message.request_id, payload: message.payload })),
        phases: client.runNotes,
        diagnostics: client.diagnostics,
      },
      null,
      2,
    )}\n`,
  );
  console.error(`darkorbit-feel failure artifacts screenshot=${failureScreenshotPath} notes=${failureNotesPath}`);
}

async function installWebSocketCanary(client) {
  await client.page.addInitScript(() => {
    if (window.__darkOrbitFeelCanaryInstalled) return;
    window.__darkOrbitFeelCanaryInstalled = true;
    const NativeWebSocket = window.WebSocket;
    const frames = [];
    const state = { nextSocketID: 1, nextFrameIndex: 0 };
    window.__darkOrbitFeelFrames = frames;
    window.__darkOrbitFeelSockets = {};
    class DarkOrbitFeelWebSocket extends NativeWebSocket {
      constructor(...args) {
        super(...args);
        const socketID = state.nextSocketID++;
        this.__darkOrbitFeelSocketID = socketID;
        window.__darkOrbitFeelSockets[socketID] = this;
        this.addEventListener('open', () => {
          if (safePath(this.url || args[0]) === '/ws') window.__darkOrbitFeelCommandSocket = this;
        });
        this.addEventListener('close', () => {
          if (window.__darkOrbitFeelSockets?.[socketID] === this) delete window.__darkOrbitFeelSockets[socketID];
          if (window.__darkOrbitFeelCommandSocket === this) window.__darkOrbitFeelCommandSocket = null;
        });
        this.addEventListener('message', (event) => capture('in', socketID, this.url || args[0], event.data));
      }
      send(data) {
        capture('out', this.__darkOrbitFeelSocketID ?? 0, this.url, data);
        return super.send(data);
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
    function safePath(url) {
      try {
        return new URL(String(url), window.location.href).pathname;
      } catch {
        return '';
      }
    }
    window.WebSocket = DarkOrbitFeelWebSocket;
  });
}

async function register(client, email, password, callsign) {
  const { page } = client;
  const result = await authFetch(page, '/api/auth/register', { email, password, callsign });
  assert(result.authenticated === true, `register failed ${compact(result)}`);
  await page.reload({ waitUntil: 'domcontentloaded' });
}

async function authFetch(page, path, payload) {
  return page.evaluate(async ({ path, payload }) => {
    const response = await fetch(path, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(payload),
    });
    const body = await response.json().catch(() => ({}));
    if (!response.ok) {
      throw new Error(`${path} ${response.status} ${JSON.stringify(body)}`);
    }
    return body;
  }, { path, payload });
}

async function openCommandSocket(client) {
  await client.page.evaluate(
    () =>
      new Promise((resolve, reject) => {
        if (window.__darkOrbitFeelCommandSocket?.readyState === WebSocket.OPEN) return resolve(true);
        const existing = Object.values(window.__darkOrbitFeelSockets ?? {}).find(
          (socket) => socket?.readyState === WebSocket.OPEN && new URL(socket.url, window.location.href).pathname === '/ws',
        );
        if (existing) {
          window.__darkOrbitFeelCommandSocket = existing;
          return resolve(true);
        }
        const socket = new WebSocket(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`);
        window.__darkOrbitFeelCommandSocket = socket;
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
    request_id: `darkorbit-feel-${op.replace(/[^a-z0-9]+/gi, '-')}-${Date.now()}-${client.seq}`,
    op,
    payload,
    client_seq: client.seq++,
    v: 1,
  };
  return client.page.evaluate(
    ({ message }) =>
      new Promise((resolve, reject) => {
        const socket = window.__darkOrbitFeelCommandSocket;
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

async function sendDriverCommand(client, method, args, op) {
  const startIndex = await lastFrameIndex(client);
  await client.page.evaluate(
    ({ method, args }) => {
      const driver = window.__SPACE_MORPG_TEST_DRIVER__;
      if (!driver || typeof driver[method] !== 'function') {
        throw new Error(`smoke test driver method ${method} missing`);
      }
      driver[method](...args);
    },
    { method, args },
  );
  const outbound = await waitForFrameMessage(
    client,
    (message, frame) => frame.direction === 'out' && frame.index > startIndex && message.op === op,
    `${op} outbound`,
    12000,
  );
  return waitForFrameMessage(
    client,
    (message, frame) => frame.direction === 'in' && frame.index > outbound.frame.index && message.request_id === outbound.message.request_id,
    `${op} response`,
    15000,
  ).then((entry) => entry.message);
}

async function waitForFrameMessage(client, predicate, label, timeoutMS) {
  let found = null;
  await waitFor(async () => {
    for (const frame of await frames(client)) {
      if (!frame.text) continue;
      let message;
      try {
        message = JSON.parse(frame.text);
      } catch {
        continue;
      }
      if (predicate(message, frame)) {
        found = { message, frame };
        return;
      }
    }
    assert(false, `${label} frame missing`);
  }, timeoutMS, label);
  return found;
}

async function lastFrameIndex(client) {
  const frameList = await frames(client);
  return frameList.reduce((max, frame) => Math.max(max, Number(frame.index ?? -1)), -1);
}

async function enterForwardPortal(client, expectedMapKey) {
  const state = await smoke(client);
  const portals = state.currentMap?.visible_portals ?? [];
  assert(portals.length > 0, `no visible portals from ${state.currentMap?.public_map_key}`);
  const portal = portals.slice().sort((a, b) => (b.position?.x ?? 0) - (a.position?.x ?? 0) || (a.position?.y ?? 0) - (b.position?.y ?? 0))[0];
  await moveToPosition(client, portal.position, Math.max(120, portal.interaction_radius ?? 180), `portal ${portal.portal_id}`, 120000);
  const payload = payloadOf(await sendDriverCommand(client, 'portalEnter', [portal.portal_id], 'portal.enter'), `portal.enter ${portal.portal_id}`);
  assert(payload.accepted === true, `portal rejected ${compact(payload)}`);
  return waitSmoke(client, (candidate) => candidate.currentMap?.public_map_key === expectedMapKey, `map ${expectedMapKey}`, 20000);
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
    const target = step(position, targetPosition, 1200);
    const response = await sendDriverCommand(client, 'moveTo', [target], 'move_to');
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
        return distance(pos, target) <= 40 || distance(pos, targetPosition) <= arriveDistance;
      },
      `movement to ${fmt(target)}`,
      Math.max(5000, eta + 5000),
    );
  }
  throw new Error(`Timed out before reaching ${label} at ${fmt(targetPosition)}`);
}

async function moveUntilNPC(client, candidates, label, arriveDistance = 1400) {
  let lastState = null;
  for (const candidate of candidates) {
    await resetWebSocketFrames(client);
    await moveToPosition(client, candidate, arriveDistance, `${label} ${fmt(candidate)}`, 90000);
    lastState = await waitSmoke(
      client,
      (state) => state.currentMap?.public_map_key === '1-3',
      `${label} map ready`,
      10000,
    );
    const freshNPC = findRecentLiveNPC(lastState, await frames(client));
    if (freshNPC) return { ...lastState, __preferredTargetID: freshNPC.entity_id };
  }
  throw new Error(`${label} live NPC missing after candidates; last=${compact(smokeSummary(lastState))}`);
}

async function killAndPickupOriginLoot(client, targetID) {
  await sustainAttackUntilKilled(client, targetID, 150000);
  await waitForInboundEvent(
    client,
    'combat.attack_stopped',
    (message) => message.payload?.active === false && message.payload?.last_stop_reason === 'target_destroyed',
    `Origin combat stopped after kill ${targetID}`,
    15000,
  );
  const lootEvent = await waitForInboundEvent(
    client,
    'loot.created',
    (message) => Boolean(message.payload?.drop_id && message.payload?.item_id && message.payload?.position),
    'Origin loot.created',
    15000,
  );
  const drop = lootEvent.payload;
  const lootState = await waitSmoke(
    client,
    (state) => Boolean(findKnownDrop(state, drop.drop_id)),
    `Origin known loot ${drop.drop_id}`,
    15000,
  );
  const knownDrop = findKnownDrop(lootState, drop.drop_id);
  const beforeCargo = lootState.cargo;
  await moveToPosition(
    client,
    knownDrop.position ?? drop.position,
    Math.max(60, Math.min(120, (lootState.stats?.loot_pickup_range ?? 120) - 10)),
    `Origin loot ${drop.drop_id}`,
    45000,
  );
  const pickupPayload = payloadOf(
    await sendDriverCommand(client, 'lootPickup', [drop.drop_id], 'loot.pickup'),
    `loot.pickup ${drop.drop_id}`,
  );
  assert(pickupPayload.accepted === true, `loot.pickup rejected ${compact(pickupPayload)}`);
  assert(cargoIncludesPickup(pickupPayload.cargo, beforeCargo, drop), `pickup cargo ${compact(pickupPayload.cargo)} missing ${compact(drop)}`);
  await waitSmoke(
    client,
    (state) => cargoIncludesPickup(state.cargo, beforeCargo, drop) && !findKnownDrop(state, drop.drop_id),
    `Origin loot ${drop.drop_id} removed and cargo updated`,
    15000,
  );
}

async function sustainAttackUntilKilled(client, targetID, timeoutMS) {
  const deadline = Date.now() + timeoutMS;
  let lastShotCount = inboundEventCount(await frames(client), 'combat.shot_started');
  let lastShotAt = Date.now();
  let lastKeepaliveAt = Date.now();
  let restarts = 0;
  let lastSummary = null;
  while (Date.now() < deadline) {
    const killed = inboundMessages(await frames(client), 'combat.npc_killed').find((message) => message.payload?.entity_id === targetID);
    if (killed) return killed;
    const state = await smoke(client);
    const target = findLiveNPCByID(state, targetID);
    lastSummary = {
      targetID,
      target: target
        ? { hp: target.combat?.hp, shield: target.combat?.shield, status: target.combat?.status, position: target.position }
        : null,
      self: selfEntity(state)?.position,
      pendingCommands: state.pendingCommands,
      combatEngagement: state.combatEngagement,
      restarts,
      eventTypes: Array.from(new Set(inboundMessages(await frames(client)).map((message) => message.type))).slice(-20),
    };
    if (!target) {
      await delay(250);
      continue;
    }
    const self = selfEntity(state);
    const selfPosition = positionNow(self, state);
    const targetPosition = positionNow(target, state);
    const weaponRange = state.stats?.weapon_range ?? 260;
    if (distance(selfPosition, targetPosition) > Math.max(80, weaponRange - 70)) {
      await moveToPosition(client, targetPosition, Math.max(80, Math.min(180, weaponRange - 80)), 'Origin sustained target range', 45000);
      lastShotAt = Date.now();
    }
    const shotCount = inboundEventCount(await frames(client), 'combat.shot_started');
    if (shotCount > lastShotCount) {
      lastShotCount = shotCount;
      lastShotAt = Date.now();
    }
    if (Date.now() - lastKeepaliveAt > 12000 && Object.keys(state.pendingCommands ?? {}).length === 0) {
      const response = await sendDriverCommand(client, 'combatState', [], 'combat.state');
      payloadOf(response, `combat.state keepalive ${targetID}`);
      lastKeepaliveAt = Date.now();
    }
    if (Date.now() - lastShotAt > 9000 && Object.keys(state.pendingCommands ?? {}).length === 0) {
      const response = await sendDriverCommand(client, 'combatUseSkill', [targetID], 'combat.use_skill');
      if (response.ok !== true) {
        if (['ERR_RATE_LIMITED', 'ERR_COOLDOWN_ACTIVE', 'ERR_OUT_OF_RANGE'].includes(response.error?.code)) {
          lastShotAt = Date.now() - 6000;
          await delay(500);
          continue;
        }
        payloadOf(response, `fallback combat.use_skill ${targetID}`);
      }
      const payload = payloadOf(response, `fallback combat.use_skill ${targetID}`);
      assert(payload.accepted === true, `fallback combat.use_skill rejected ${compact(payload)}`);
      restarts++;
      lastShotAt = Date.now();
    }
    await delay(250);
  }
  throw new Error(`Origin NPC killed ${targetID} timed out; last=${compact(lastSummary)}`);
}

function firstKnownDrop(state) {
  return Object.values(state.knownLoot ?? {}).find((drop) => drop?.position && (drop.drop_id || drop.entity_id));
}

function findKnownDrop(state, dropID) {
  if (!dropID) return null;
  return Object.values(state.knownLoot ?? {}).find((drop) => (drop?.drop_id || drop?.entity_id) === dropID) ?? null;
}

function targetDestroyed(state, targetID) {
  const target = state.visibleEntities?.[targetID];
  if (!target) return true;
  const hp = Number(target.combat?.hp ?? 0);
  return hp <= 0 || ['dead', 'destroyed', 'disabled'].includes(String(target.combat?.status ?? '').toLowerCase());
}

function cargoIncludesPickup(cargo, beforeCargo, drop) {
  const before = cargoQuantity(beforeCargo, drop.item_id);
  const after = cargoQuantity(cargo, drop.item_id);
  return after >= before + Number(drop.quantity ?? 0);
}

function cargoQuantity(cargo, itemID) {
  return (cargo?.items ?? []).filter((item) => item.item_id === itemID).reduce((sum, item) => sum + Number(item.quantity ?? 0), 0);
}

function findLiveNPC(state) {
  return Object.values(state.visibleEntities ?? {})
    .filter((entity) => {
      if (entity?.entity_type !== 'npc' || !entity.position) return false;
      const hp = Number(entity.combat?.hp ?? 1);
      return hp > 0 && !['dead', 'destroyed', 'disabled'].includes(String(entity.combat?.status ?? 'active').toLowerCase());
    })
    .sort((a, b) => combatDurability(a) - combatDurability(b))[0];
}

function findLiveNPCByID(state, entityID) {
  if (!entityID) return null;
  const entity = state.visibleEntities?.[entityID];
  if (!entity || entity.entity_type !== 'npc' || !entity.position) return null;
  const hp = Number(entity.combat?.hp ?? 1);
  if (hp <= 0 || ['dead', 'destroyed', 'disabled'].includes(String(entity.combat?.status ?? 'active').toLowerCase())) return null;
  return entity;
}

function findRecentLiveNPC(state, frameList) {
  const ids = [];
  for (const message of inboundMessages(frameList, 'aoi.entity_entered').concat(inboundMessages(frameList, 'aoi.entity_updated'))) {
    if (message.payload?.entity_type === 'npc' && message.payload?.entity_id) ids.push(message.payload.entity_id);
  }
  for (const entityID of ids.reverse()) {
    const entity = findLiveNPCByID(state, entityID);
    if (entity) return entity;
  }
  return null;
}

function combatDurability(entity) {
  return Number(entity?.combat?.hp ?? 0) + Number(entity?.combat?.shield ?? 0);
}

function findNearestLiveNPC(state) {
  const self = selfEntity(state);
  const position = self ? positionNow(self, state) : null;
  const live = Object.values(state.visibleEntities ?? {}).filter((entity) => {
    if (entity?.entity_type !== 'npc' || !entity.position) return false;
    const hp = Number(entity.combat?.hp ?? 1);
    return hp > 0 && !['dead', 'destroyed', 'disabled'].includes(String(entity.combat?.status ?? 'active').toLowerCase());
  });
  if (!position) return live.sort((a, b) => combatDurability(a) - combatDurability(b))[0];
  return live.sort((a, b) => distance(position, a.position) - distance(position, b.position))[0];
}

function selfEntity(state) {
  const entities = Object.values(state.visibleEntities ?? {});
  return entities.find((entity) => (entity.status_flags ?? []).includes('self')) ?? entities.find((entity) => entity.entity_type === 'player');
}

function positionNow(entity, state) {
  assert(entity?.position, `self position missing ${compact(state?.visibleEntities)}`);
  const movement = entity.movement;
  if (!movement?.moving || !movement.origin || !movement.target || !state?.serverNow) return entity.position;
  const start = Number(movement.started_at_ms ?? 0);
  const arrive = Number(movement.arrive_at_ms ?? 0);
  const duration = arrive - start;
  if (duration <= 0) return movement.target;
  const progress = Math.max(0, Math.min(1, (Number(state.serverNow) - start) / duration));
  return {
    x: movement.origin.x + (movement.target.x - movement.origin.x) * progress,
    y: movement.origin.y + (movement.target.y - movement.origin.y) * progress,
  };
}

function step(from, to, maxDistance) {
  const total = distance(from, to);
  if (total <= maxDistance) return { ...to };
  const scale = maxDistance / total;
  return { x: from.x + (to.x - from.x) * scale, y: from.y + (to.y - from.y) * scale };
}

function combatNudge(position, target, weaponRange, bounds) {
  const dx = Number(position.x) - Number(target.x);
  const dy = Number(position.y) - Number(target.y);
  const length = Math.sqrt(dx * dx + dy * dy);
  const moveDistance = 80;
  let candidate;
  if (length < 1) {
    candidate = { x: target.x + Math.min(120, weaponRange - 50), y: target.y };
  } else {
    candidate = {
      x: position.x + (-dy / length) * moveDistance,
      y: position.y + (dx / length) * moveDistance,
    };
    if (distance(candidate, target) > weaponRange - 35) {
      candidate = step(position, target, moveDistance);
    }
  }
  return clampToBounds(candidate, bounds);
}

function clampToBounds(position, bounds) {
  if (!bounds) return position;
  return {
    x: Math.min(Math.max(position.x, bounds.min_x ?? 0), bounds.max_x ?? position.x),
    y: Math.min(Math.max(position.y, bounds.min_y ?? 0), bounds.max_y ?? position.y),
  };
}

async function waitForInboundEventCount(client, type, count, timeoutMS) {
  return waitFor(async () => {
    const got = inboundEventCount(await frames(client), type);
    assert(got >= count, `${type} count ${got}, want ${count}`);
  }, timeoutMS, `${type} x${count}`);
}

async function waitForInboundEvent(client, type, predicate, label, timeoutMS) {
  let found = null;
  let lastSeen = [];
  await waitFor(async () => {
    const candidates = inboundMessages(await frames(client), type);
    lastSeen = candidates.slice(-8).map((message) => ({ type: message.type, payload: message.payload }));
    for (const message of candidates) {
      if (predicate(message)) {
        found = message;
        return;
      }
    }
    assert(false, `${label} event missing; recent ${compact(lastSeen)}`);
  }, timeoutMS, label);
  return found;
}

function inboundMessages(frameList, type = '') {
  const messages = [];
  for (const frame of frameList ?? []) {
    if (frame.direction !== 'in' || !frame.text) continue;
    let parsed;
    try {
      parsed = JSON.parse(frame.text);
    } catch {
      continue;
    }
    if (!type || parsed.type === type) messages.push(parsed);
  }
  return messages;
}

function outboundMessages(frameList, op = '') {
  const messages = [];
  for (const frame of frameList ?? []) {
    if (frame.direction !== 'out' || !frame.text) continue;
    let parsed;
    try {
      parsed = JSON.parse(frame.text);
    } catch {
      continue;
    }
    if (!op || parsed.op === op) messages.push(parsed);
  }
  return messages;
}

function inboundEventCount(frameList, type) {
  return inboundMessages(frameList, type).length;
}

function assertExactOutboundPayload(client, op, keys) {
  return client.page.evaluate(({ op, keys }) => {
    const frames = window.__darkOrbitFeelFrames ?? [];
    const outbound = frames
      .filter((frame) => frame.direction === 'out' && frame.text)
      .map((frame) => {
        try {
          return JSON.parse(frame.text);
        } catch {
          return null;
        }
      })
      .filter((message) => message?.op === op);
    if (outbound.length !== 1) throw new Error(`${op} outbound count ${outbound.length}, want 1`);
    const got = Object.keys(outbound[0].payload ?? {}).sort();
    const want = keys.slice().sort();
    if (JSON.stringify(got) !== JSON.stringify(want)) throw new Error(`${op} payload keys ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
    return true;
  }, { op, keys });
}

async function resetWebSocketFrames(client) {
  await client.page.evaluate(() => {
    if (Array.isArray(window.__darkOrbitFeelFrames)) window.__darkOrbitFeelFrames.length = 0;
  });
}

async function frames(client) {
  return client.page.evaluate(() => window.__darkOrbitFeelFrames ?? []);
}

async function smoke(client) {
  const state = await client.page.evaluate(() => {
    const smokeState = window.__SPACE_MORPG_SMOKE_STATE__ ?? null;
    if (smokeState && Array.isArray(window.__darkOrbitFeelFrames)) {
      return { ...smokeState, __frames: window.__darkOrbitFeelFrames };
    }
    return smokeState;
  });
  assert(state, 'smoke state missing');
  return state;
}

async function waitSmoke(client, predicate, description, timeoutMS) {
  let lastState = null;
  try {
    await waitFor(async () => {
      lastState = await smoke(client);
      assert(predicate(lastState), `${description} not ready`);
    }, timeoutMS, description);
  } catch (error) {
    const summary = lastState ? smokeSummary(lastState) : null;
    const diagnostics = (client.diagnostics ?? []).slice(-12);
    const processLogs = (client.processes ?? [])
      .flatMap((proc) => (proc?.logs ?? []).slice(-80).map((line) => `[${proc.label}] ${line}`))
      .join('\n');
    throw new Error(`${error.message}\nlast smoke: ${compact(summary)}\ndiagnostics: ${diagnostics.join('\n')}\nprocess logs:\n${processLogs}`);
  }
  return lastState;
}

function smokeSummary(state) {
  return {
    connectionStatus: state?.connectionStatus,
    auth: state?.auth,
    currentMap: state?.currentMap,
    entityCount: Object.keys(state?.visibleEntities ?? {}).length,
    npcCount: Object.values(state?.visibleEntities ?? {}).filter((entity) => entity?.entity_type === 'npc').length,
    pendingCommands: state?.pendingCommands,
    commandLogSummary: state?.commandLogSummary,
  };
}

async function assertNoLeak(client, state, label) {
  assertNoPayloadLeak(state, `${label} smoke state`);
  const storageLeak = await client.page.evaluate((tokens) => {
    const haystack = `${document.body.innerText}\n${document.cookie}\n${JSON.stringify(window.localStorage)}\n${JSON.stringify(window.sessionStorage)}`.toLowerCase();
    return tokens.find((token) => haystack.includes(String(token).toLowerCase())) ?? '';
  }, leakTokens);
  assert(!storageLeak, `${label} leaked token ${storageLeak}`);
}

function assertNoPayloadLeak(payload, label) {
  const raw = compact(payload).toLowerCase();
  for (const token of leakTokens) {
    assert(!raw.includes(String(token).toLowerCase()), `${label} leaked ${token}: ${raw.slice(0, 1500)}`);
  }
}

async function assertWebSocketCanary(client) {
  const frameList = await frames(client);
  assert(frameList.some((frame) => frame.direction === 'out' && frame.text.includes('combat.start_attack')), 'combat.start_attack websocket frame missing');
  for (const frame of frameList) {
    if (frame.truncated) throw new Error(`truncated websocket frame ${frame.index}`);
    if (frame.text) assertNoPayloadLeak(frame.text, `websocket frame ${frame.index}`);
  }
}

function assertProcessLogCanary(processes) {
  for (const proc of processes) {
    const text = proc?.logs?.join('\n') ?? '';
    assertNoPayloadLeak(text, `${proc?.label ?? 'process'} log`);
  }
}

function payloadOf(response, label) {
  if (response?.ok !== true) {
    throw new Error(`${label} failed: ${compact(response)}`);
  }
  assertNoPayloadLeak(response.payload ?? {}, `${label} response`);
  return response.payload ?? {};
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function compact(value) {
  return JSON.stringify(value, (_key, val) => (typeof val === 'number' && Number.isFinite(val) ? Math.round(val * 1000) / 1000 : val));
}

function fmt(vec) {
  return `${Math.round(vec?.x ?? 0)},${Math.round(vec?.y ?? 0)}`;
}

function distance(a, b) {
  const dx = Number(a?.x ?? 0) - Number(b?.x ?? 0);
  const dy = Number(a?.y ?? 0) - Number(b?.y ?? 0);
  return Math.sqrt(dx * dx + dy * dy);
}

async function ensurePostgresDB(dbName) {
  await run('docker', ['compose', 'up', '-d', 'postgres'], repoRoot, { POSTGRES_PORT: process.env.POSTGRES_PORT || '55432' });
  await waitFor(async () => {
    await run('docker', ['exec', postgresContainer, 'pg_isready', '-U', postgresUser, '-d', postgresBaseDB], repoRoot);
  }, 30000, 'postgres health');
  await run('docker', ['exec', postgresContainer, 'createdb', '-U', postgresUser, dbName], repoRoot);
  const port = (await run('docker', ['port', postgresContainer, '5432'], repoRoot)).trim().split(':').pop();
  assert(port, 'postgres host port unavailable');
  return port;
}

async function freePort() {
  return new Promise((resolvePort, reject) => {
    const server = net.createServer();
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      server.close(() => resolvePort(address.port));
    });
    server.on('error', reject);
  });
}

function child(label, command, args, cwd, env = {}) {
  const proc = spawn(command, args, { cwd, detached: true, env: { ...process.env, ...env }, stdio: ['ignore', 'pipe', 'pipe'] });
  proc.label = label;
  proc.logs = [];
  const collect = (streamName, chunk) => {
    for (const line of String(chunk).split(/\r?\n/).filter(Boolean)) {
      proc.logs.push(`[${streamName}] ${line}`);
      if (proc.logs.length > maxProcessLogLines) proc.logs.shift();
    }
  };
  proc.stdout.on('data', (chunk) => collect('stdout', chunk));
  proc.stderr.on('data', (chunk) => collect('stderr', chunk));
  return proc;
}

async function waitHTTP(url, label, proc) {
  await waitFor(async () => {
    const response = await fetch(url);
    assert(response.ok, `${label} HTTP ${response.status}`);
  }, 45000, `${label} ${url}`, () => {
    if (proc?.exitCode !== null) throw new Error(`${label} exited:\n${proc.logs.join('\n')}`);
  });
}

async function waitFor(fn, timeoutMS, label, tick) {
  const deadline = Date.now() + timeoutMS;
  let lastError;
  while (Date.now() < deadline) {
    try {
      if (tick) tick();
      await fn();
      return;
    } catch (error) {
      lastError = error;
      await delay(150);
    }
  }
  throw new Error(`${label} timed out: ${lastError?.message ?? lastError}`);
}

function delay(ms) {
  return new Promise((resolveDelay) => setTimeout(resolveDelay, ms));
}

async function stop(proc) {
  if (!proc || proc.exitCode !== null) return;
  try {
    process.kill(-proc.pid, 'SIGTERM');
  } catch {
    proc.kill('SIGTERM');
  }
  await new Promise((resolveStop) => {
    const timer = setTimeout(() => {
      try {
        process.kill(-proc.pid, 'SIGKILL');
      } catch {
        proc.kill('SIGKILL');
      }
      resolveStop();
    }, 3000);
    proc.once('exit', () => {
      clearTimeout(timer);
      resolveStop();
    });
  });
}

async function run(command, args, cwd, env = {}) {
  return new Promise((resolveRun, reject) => {
    const proc = spawn(command, args, { cwd, env: { ...process.env, ...env }, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '';
    let stderr = '';
    proc.stdout.on('data', (chunk) => {
      stdout += String(chunk);
    });
    proc.stderr.on('data', (chunk) => {
      stderr += String(chunk);
    });
    proc.on('error', reject);
    proc.on('exit', (code) => {
      if (code === 0) return resolveRun(stdout);
      reject(new Error(`${command} ${args.join(' ')} exited ${code}\n${stdout}\n${stderr}`));
    });
  });
}

main().catch((error) => {
  console.error(error?.stack ?? error);
  process.exit(1);
});

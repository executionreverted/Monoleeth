#!/usr/bin/env node
import { spawn } from 'node:child_process';
import net from 'node:net';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { chromium } from 'playwright';

const scriptDir = dirname(fileURLToPath(import.meta.url));
const clientDir = resolve(scriptDir, '../..');
const repoRoot = resolve(clientDir, '..');
const postgresContainer = 'gameproject-postgres';
const postgresUser = process.env.POSTGRES_USER || 'gameproject';
const postgresPassword = process.env.POSTGRES_PASSWORD || 'gameproject_dev_password';
const postgresBaseDB = process.env.POSTGRES_DB || 'gameproject';
const forbiddenLeakTokens = [
  'loot_table',
  'loot_tables',
  'drop_profile',
  'enemy_pool',
  'spawn_area',
  'procedural_seed',
  'gameplay_seed',
  'snapshot_json',
  'data_json',
  'display_json',
  'audit_log',
];

async function main() {
  const nonce = `${Date.now()}${Math.random().toString(16).slice(2)}`;
  const adminEmail = `cms-admin-${nonce}@example.test`;
  const adminPassword = `admin-password-${nonce}`;
  const dbName = `gameproject_cms_smoke_${nonce}`.slice(0, 60);
  let databaseURL = process.env.CMS_SMOKE_DATABASE_URL || '';
  let ownsDatabase = false;
  let firstServer;
  let secondServer;
  let browser;

  try {
    if (!databaseURL) {
      const port = await ensurePostgresDB(dbName);
      ownsDatabase = true;
      databaseURL = `postgres://${postgresUser}:${postgresPassword}@127.0.0.1:${port}/${dbName}?sslmode=disable`;
    }

    const firstPort = await freePort();
    const firstOrigin = `http://127.0.0.1:${firstPort}`;
    firstServer = startGameServer(firstPort, firstOrigin, databaseURL, adminEmail, adminPassword);
    await waitHTTP(`${firstOrigin}/healthz`, 'first Go server', firstServer);

    browser = await chromium.launch();
    const adminContext = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    const adminPage = await adminContext.newPage();
    await adminPage.goto(`${firstOrigin}/?smoke=1`, { waitUntil: 'domcontentloaded' });
    await login(adminPage, adminEmail, adminPassword);
    await openSocket(adminPage);

    const moduleRow = await contentGet(adminPage, 'module', 'laser_alpha_t1');
    setModuleStat(moduleRow.data_json, 'weapon_damage', 99);
    setOptionalModuleStat(moduleRow.data_json, 'shield_damage', 99);
    await contentUpdate(adminPage, 'module', 'laser_alpha_t1', moduleRow);

    const shopRow = await contentGet(adminPage, 'shop_product', 'product_module_laser_alpha_t1');
    shopRow.data_json.price_policy.amount = 321;
    await contentUpdate(adminPage, 'shop_product', 'product_module_laser_alpha_t1', shopRow);

    const lootRow = await contentGet(adminPage, 'loot_table', 'training_drone_salvage');
    for (const row of lootRow.data_json.Rows) {
      if (row.ItemDefinition?.item_id === 'raw_ore') {
        row.MinQuantity = 6;
        row.MaxQuantity = 6;
        row.Chance = 1;
      } else {
        row.Chance = 0;
      }
    }
    await contentUpdate(adminPage, 'loot_table', 'training_drone_salvage', lootRow);

    const validation = await send(adminPage, 'admin.content.validate_draft', { version: `cms_smoke_${nonce}` });
    assert(validation.payload.validation.valid, `draft invalid ${JSON.stringify(validation.payload.validation.issues)}`);
    const publish = await send(adminPage, 'admin.content.publish', {
      version: `cms_smoke_${nonce}`,
      notes: 'cms playable browser smoke',
      balance_tag: 'cms_playable_smoke',
    });
    assert(publish.payload.content_publish.published, `publish failed ${JSON.stringify(publish.payload)}`);
    await adminContext.close();
    await stop(firstServer);
    firstServer = null;

    const secondPort = await freePort();
    const secondOrigin = `http://127.0.0.1:${secondPort}`;
    secondServer = startGameServer(secondPort, secondOrigin, databaseURL, adminEmail, adminPassword);
    await waitHTTP(`${secondOrigin}/healthz`, 'second Go server', secondServer);

    const playerContext = await browser.newContext({ viewport: { width: 1280, height: 800 } });
    const playerPage = await playerContext.newPage();
    await playerPage.goto(`${secondOrigin}/?smoke=1`, { waitUntil: 'domcontentloaded' });
    await register(playerPage, `cms-player-${nonce}@example.test`, 'correct-password', `CMS-${nonce.slice(-6)}`);
    await openSocket(playerPage);

    const catalog = await send(playerPage, 'content.catalog', {});
    assertNoLeak(catalog.payload, 'content.catalog');
    const projectedModule = catalog.payload.content_catalog.modules.find((row) => row.item_id === 'laser_alpha_t1');
    assert(projectedModule, 'projected laser module missing');
    assert(statValue(projectedModule.stat_modifiers, 'weapon_damage') === 99, 'projected laser damage not published');

    const shop = await send(playerPage, 'shop.catalog', {});
    assertNoLeak(shop.payload, 'shop.catalog');
    const product = shop.payload.shop.products.find((row) => row.product_id === 'product_module_laser_alpha_t1');
    assert(product, 'laser shop product missing');
    assert(product.price.amount === 321, `shop price ${product.price.amount}, want 321`);

    const buy = await send(playerPage, 'shop.buy_product', { product_id: 'product_module_laser_alpha_t1', quantity: 1 });
    assert(buy.payload.accepted && buy.payload.server_total === 321, `shop buy payload ${JSON.stringify(buy.payload)}`);

    await send(playerPage, 'move_to', { target: { x: 800, y: 400 } });
    await delay(3500);
    const combat = await send(playerPage, 'combat.use_skill', { skill_id: 'basic_laser', target_id: 'entity_training_npc' });
    assert(combat.payload.accepted && combat.payload.killed, `combat payload ${JSON.stringify(combat.payload)}`);
    const drop = (combat.payload.drops || []).find((row) => row.item_id === 'raw_ore');
    assert(drop && drop.quantity === 6, `loot drops ${JSON.stringify(combat.payload.drops)}, want raw_ore x6`);
    assertNoLeak(combat.payload, 'combat.use_skill');

    await playerContext.close();
    console.log(`cms-playable smoke ok db=${dbName} version=cms_smoke_${nonce} shop_price=321 laser_damage=99 raw_ore_drop=6`);
  } finally {
    if (browser) await browser.close().catch(() => {});
    if (firstServer) await stop(firstServer);
    if (secondServer) await stop(secondServer);
    if (ownsDatabase) {
      await run('docker', ['exec', postgresContainer, 'dropdb', '-U', postgresUser, '--force', '--if-exists', dbName], repoRoot).catch(() => {});
    }
  }
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

function startGameServer(port, origin, databaseURL, adminEmail, adminPassword) {
  return child('go-server', 'go', ['run', './cmd/game-server'], repoRoot, {
    GAME_SERVER_ADDR: `127.0.0.1:${port}`,
    GAME_ALLOWED_ORIGINS: origin,
    GAME_CLIENT_STATIC_DIR: 'client/dist',
    GAME_CONTENT_DATABASE_URL: databaseURL,
    GAME_ADMIN_EMAIL: adminEmail,
    GAME_ADMIN_PASSWORD: adminPassword,
    GAME_ADMIN_CALLSIGN: 'CMS Admin',
  });
}

async function login(page, email, password) {
  const result = await authFetch(page, '/api/auth/login', { email, password });
  assert(result.authenticated, `login failed ${JSON.stringify(result)}`);
}

async function register(page, email, password, callsign) {
  const result = await authFetch(page, '/api/auth/register', { email, password, callsign });
  assert(result.authenticated, `register failed ${JSON.stringify(result)}`);
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

async function openSocket(page) {
  await page.evaluate(() => {
    window.__cmsSmoke = { messages: [], seq: 1 };
    const socket = new WebSocket(`${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.host}/ws`);
    window.__cmsSmoke.socket = socket;
    socket.addEventListener('message', (event) => {
      window.__cmsSmoke.messages.push(JSON.parse(event.data));
    });
  });
  await page.waitForFunction(() => window.__cmsSmoke?.socket?.readyState === WebSocket.OPEN, null, { timeout: 15000 });
}

async function send(page, op, payload) {
  const requestID = `cms-smoke-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  await page.evaluate(({ op, payload, requestID }) => {
    const state = window.__cmsSmoke;
    state.socket.send(JSON.stringify({ request_id: requestID, op, payload, client_seq: state.seq++, v: 1 }));
  }, { op, payload, requestID });
  await page.waitForFunction((id) => window.__cmsSmoke.messages.some((message) => message.request_id === id), requestID, { timeout: 15000 });
  const response = await page.evaluate((id) => {
    const messages = window.__cmsSmoke.messages;
    const index = messages.findIndex((message) => message.request_id === id);
    return messages.splice(index, 1)[0];
  }, requestID);
  if (!response.ok) {
    throw new Error(`${op} failed ${JSON.stringify(response.error || response)}`);
  }
  return response;
}

async function contentGet(page, contentType, contentID) {
  const response = await send(page, 'admin.content.get', { content_type: contentType, content_id: contentID });
  return response.payload.content_row;
}

async function contentUpdate(page, contentType, contentID, row) {
  const response = await send(page, 'admin.content.update_draft', {
    content_type: contentType,
    content_id: contentID,
    enabled: row.enabled,
    display_json: row.display_json,
    data_json: row.data_json,
  });
  return response.payload.content_row;
}

function setModuleStat(data, stat, value) {
  const modifier = (data.stat_modifiers || []).find((row) => row.stat === stat);
  assert(modifier, `module stat ${stat} missing`);
  modifier.value = value;
}

function setOptionalModuleStat(data, stat, value) {
  const modifier = (data.stat_modifiers || []).find((row) => row.stat === stat);
  if (modifier) {
    modifier.value = value;
  }
}

function statValue(modifiers, stat) {
  return (modifiers || []).find((row) => row.stat === stat)?.value;
}

function assertNoLeak(payload, label) {
  const raw = JSON.stringify(payload).toLowerCase();
  for (const token of forbiddenLeakTokens) {
    assert(!raw.includes(token), `${label} leaked ${token}: ${raw}`);
  }
}

function child(label, command, args, cwd, env = {}) {
  const proc = spawn(command, args, {
    cwd,
    env: { ...process.env, ...env },
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  proc.label = label;
  proc.stdout.setEncoding('utf8');
  proc.stderr.setEncoding('utf8');
  proc.stdout.on('data', (chunk) => process.stdout.write(`[${label}] ${chunk}`));
  proc.stderr.on('data', (chunk) => process.stderr.write(`[${label}] ${chunk}`));
  return proc;
}

async function run(command, args, cwd, env = {}) {
  return new Promise((resolve, reject) => {
    const proc = spawn(command, args, { cwd, env: { ...process.env, ...env }, stdio: ['ignore', 'pipe', 'pipe'] });
    let stdout = '';
    let stderr = '';
    const timer = setTimeout(() => {
      proc.kill('SIGKILL');
      reject(new Error(`${command} ${args.join(' ')} timed out`));
    }, 30000);
    proc.stdout.on('data', (chunk) => (stdout += chunk));
    proc.stderr.on('data', (chunk) => (stderr += chunk));
    proc.on('error', (error) => {
      clearTimeout(timer);
      reject(error);
    });
    proc.on('exit', (code) => {
      clearTimeout(timer);
      if (code === 0) {
        resolve(stdout);
        return;
      }
      reject(new Error(`${command} ${args.join(' ')} exited ${code}\n${stdout}\n${stderr}`));
    });
  });
}

async function stop(proc) {
  if (!proc || proc.exitCode !== null) return;
  proc.kill('SIGTERM');
  await new Promise((resolve) => {
    const timer = setTimeout(() => {
      proc.kill('SIGKILL');
      resolve();
    }, 5000);
    proc.once('exit', () => {
      clearTimeout(timer);
      resolve();
    });
  });
}

async function waitHTTP(url, label, proc) {
  await waitFor(async () => {
    if (proc.exitCode !== null) throw new Error(`${label} exited ${proc.exitCode}`);
    const response = await fetch(url);
    if (!response.ok) throw new Error(`${label} ${response.status}`);
  }, 30000, label);
}

async function waitFor(fn, timeoutMS, label) {
  const deadline = Date.now() + timeoutMS;
  let lastError;
  while (Date.now() < deadline) {
    try {
      return await fn();
    } catch (error) {
      lastError = error;
      await new Promise((resolve) => setTimeout(resolve, 250));
    }
  }
  throw new Error(`${label} timed out: ${lastError?.message || lastError}`);
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

async function delay(ms) {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

function assert(condition, message) {
  if (!condition) {
    throw new Error(message);
  }
}

main().then(() => {
  process.exit(0);
}).catch((error) => {
  console.error(error);
  process.exit(1);
});

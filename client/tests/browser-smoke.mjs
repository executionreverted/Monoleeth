import { spawn } from 'node:child_process';
import { createHash } from 'node:crypto';
import { mkdir } from 'node:fs/promises';
import { dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import net from 'node:net';
import path from 'node:path';
import process from 'node:process';

import { chromium } from 'playwright';
import { createServer as createViteServer } from 'vite';

const explicitURL = readArg('--url');
const useFixture = process.argv.includes('--fixture');
const thisDir = dirname(fileURLToPath(import.meta.url));
const clientRoot = path.resolve(thisDir, '..');
const repoRoot = path.resolve(clientRoot, '..');
const outputDir = path.resolve(repoRoot, 'output', 'screenshots', 'ui-implementation', '10');
const adminEmail = 'smoke-admin@example.com';
const adminPassword = 'correct-admin-password';
const adminCallsign = 'Smoke-Admin';
const forbiddenText = [
  'gameplay_seed',
  'future_spawn',
  'internal_metadata',
  'loot_table',
  'account_id',
  'player_id',
  'session_id',
  'generated_payload',
  'generated_seed',
  'rare_cap',
  'password_hash',
  'session_token',
  'reset_secret',
  'auth_header',
  'world_seed',
  'npc_placeholder',
  'loot_placeholder',
  'planet_signal_placeholder',
];
const fakeSocialCountPatterns = [
  { label: 'unread mail count', pattern: /\b(?:mail|inbox|unread)\b[^\n]{0,24}\b\d+\b/i },
  { label: 'friend count', pattern: /\bfriends?\b[^\n]{0,24}\b\d+\b/i },
  { label: 'party count', pattern: /\bparty\b[^\n]{0,24}\b\d+\b/i },
  { label: 'menu notification count', pattern: /\bmenu\b[^\n]{0,24}\b(?:notification|badge|count)?s?\s*\d+\b/i },
  { label: 'social notification count', pattern: /\bsocial\b[^\n]{0,24}\b(?:notification|badge|count)?s?\s*\d+\b/i },
  { label: 'notification count', pattern: /\bnotifications?\b[^\n]{0,24}\b\d+\b/i },
];
const unimplementedMutationOps = [
  'loadout.equip_module',
  'loadout.unequip_module',
  'crafting.start',
  'crafting.complete',
  'crafting.cancel',
  'discovery.claim_planet',
  'planet.building_build',
  'planet.building_upgrade',
  'route.create',
  'route.update',
  'route.enable',
  'route.disable',
  'route.settle',
];
const unimplementedMutationControlPatterns = [
  {
    label: 'loadout module mutation',
    pattern: /\b(?:loadout|module)\b[^\n]{0,32}\b(?:equip|unequip)\b|\b(?:equip|unequip)\b[^\n]{0,32}\b(?:loadout|module)\b/i,
  },
  {
    label: 'crafting mutation',
    pattern: /\b(?:craft|crafting|recipe)\b[^\n]{0,32}\b(?:start|complete|cancel)\b|\b(?:start|complete|cancel)\b[^\n]{0,32}\b(?:craft|crafting|recipe)\b/i,
  },
  {
    label: 'planet claim mutation',
    pattern: /\b(?:discovery|planet)\b[^\n]{0,32}\bclaim\b|\bclaim\b[^\n]{0,32}\bplanet\b/i,
  },
  {
    label: 'planet building mutation',
    pattern: /\b(?:planet|building)\b[^\n]{0,32}\b(?:build|upgrade)\b|\b(?:build|upgrade)\b[^\n]{0,32}\b(?:planet|building)\b/i,
  },
  {
    label: 'route mutation',
    pattern: /\broute\b[^\n]{0,32}\b(?:create|update|enable|disable|settle)\b|\b(?:create|update|enable|disable|settle)\b[^\n]{0,32}\broute\b/i,
  },
];
let eventSequence = 100;

const appPort = explicitURL ? null : await findFreePort();
const gamePort = explicitURL || useFixture ? null : await findFreePort();
const origin = appPort ? `http://127.0.0.1:${appPort}` : null;
const gameServer = gamePort && origin ? await startGameServer(gamePort, origin) : null;
const appServer = explicitURL ? null : await startViteAppServer({ port: appPort, gamePort });
const url = explicitURL ?? appServer.url;
let browser;

try {
  browser = await chromium.launch({ headless: true });
  await mkdir(outputDir, { recursive: true });
  if (useFixture) {
    await verifyFixtureViewport({ width: 1440, height: 900 }, 'fixture-desktop');
    await verifyFixtureViewport({ width: 390, height: 844 }, 'fixture-mobile');
  } else {
    await verifyRealViewport({ width: 390, height: 844 }, 'mobile');
    await verifyRealViewport({ width: 1024, height: 768 }, 'tablet');
    await verifyRealViewport({ width: 1440, height: 900 }, 'desktop');
    await verifyAdminViewport({ width: 1440, height: 900 }, 'admin-desktop');
  }
} finally {
  await browser?.close();
  await appServer?.close();
  await gameServer?.close();
}

async function verifyRealViewport(viewport, label) {
  const page = await browser.newPage({ viewport });
  const suffix = `${Date.now()}-${label}`.replace(/[^a-z0-9-]/gi, '-').toLowerCase();
  const email = `smoke-${suffix}@example.com`;
  const password = 'correct-password';
  const callsign = `Smoke-${label}`;
  try {
    await page.goto(withSmokeParam(url), { waitUntil: 'networkidle' });
    await page.waitForSelector('.auth-card', { timeout: 10000 });
    await page.waitForFunction(() => Boolean(window.__SPACE_MORPG_SMOKE_STATE__), null, { timeout: 10000 });
    await page.waitForFunction(() => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      return (
        state?.connectionStatus === 'logged_out' &&
        state?.playerSnapshot === null &&
        state?.cargo === null &&
        state?.wallet === null &&
        state?.stats === null &&
        state?.inventory === null &&
        state?.hangar === null &&
        state?.loadout === null &&
        state?.crafting === null &&
        Object.keys(state?.visibleEntities ?? {}).length === 0
      );
    });
    await assertNoFakeTopbarCounts(page, `${label} unauthenticated`);
    await assertNoUnimplementedMutationControls(page, `${label} unauthenticated`);
    await page.screenshot({ path: path.join(outputDir, `unauth-${label}.png`), fullPage: true });

    await page.locator('input[name="email"]').fill(email);
    await page.locator('input[name="password"]').fill(password);
    await page.locator('[data-submit]').click();
    await page.waitForFunction(() => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      return (
        state?.connectionStatus === 'logged_out' &&
        state?.auth?.error === 'Email or password is invalid.' &&
        state?.playerSnapshot === null &&
        Object.keys(state?.visibleEntities ?? {}).length === 0
      );
    });
    await assertNoFakeTopbarCounts(page, `${label} invalid-login`);
    await assertNoUnimplementedMutationControls(page, `${label} invalid-login`);

    await page.locator('[data-toggle]').click();
    await page.locator('input[name="email"]').fill(email);
    await page.locator('input[name="password"]').fill(password);
    await page.locator('input[name="callsign"]').fill(callsign);
    await page.locator('[data-submit]').click();

    await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.connectionStatus === 'connected', null, {
      timeout: 10000,
    });
    try {
      await page.waitForFunction(
        (expectedCallsign) => {
          const state = window.__SPACE_MORPG_SMOKE_STATE__;
          const entities = state?.visibleEntities ?? {};
          const hasPlayer = Object.values(entities).some((entity) => entity.entity_type === 'player');
          const hasVisibleNPC = Object.values(entities).some(
            (entity) => entity.entity_id === 'entity_training_npc' && entity.entity_type === 'npc',
          );
          const self = Object.values(entities).find((entity) => entity.status_flags?.includes('self'));
          return (
            state?.auth?.session?.authenticated === true &&
            state?.playerSnapshot?.callsign === expectedCallsign &&
            state?.sector?.name === 'Origin Fringe' &&
            state?.minimap?.live_contacts?.some((contact) => contact.entity_id === 'entity_training_npc') &&
            state?.cargo?.capacity === 60 &&
            state?.wallet?.credits === 1200 &&
            state?.wallet?.premium_paid === 300 &&
            state?.ship?.active_ship_id === 'starter_ship' &&
            state?.ship?.disabled === false &&
            state?.progression?.rank >= 1 &&
            state?.stats?.radar_range === 420 &&
            state?.inventory?.counts?.cargo_stacks === 0 &&
            state?.hangar?.active_ship_id === 'starter_ship' &&
            state?.loadout?.slots?.length === 3 &&
            state?.crafting?.recipes?.length >= 3 &&
            state?.planetIntel?.knownSignals === 0 &&
            state?.production?.planets?.length === 0 &&
            state?.routes?.routes?.length === 0 &&
            state?.market?.listings?.some(
              (listing) => listing.status === 'active' && !listing.owned_by_you && listing.server_recalculates === true,
            ) &&
            state?.auction?.lots?.some((lot) => lot.status === 'active' && lot.server_recalculates === true) &&
            state?.premium?.entitlements?.length === 1 &&
            state?.premium?.stock?.length === 1 &&
            hasPlayer &&
            self?.entity_type === 'player' &&
            hasVisibleNPC &&
            !entities.entity_hidden_planet_signal
          );
        },
        callsign,
        { timeout: 10000 },
      );
    } catch (error) {
      console.error(`${label} bootstrap state`, JSON.stringify(await bootstrapDiagnostics(page, callsign), null, 2));
      throw error;
    }
    await assertNoUnimplementedMutationControls(page, `${label} authenticated bootstrap`);

    await verifyRealEconomy(page);

    await page.waitForFunction(() => {
      const button = document.querySelector('[data-panel="intel"] [data-action="scan"]');
      return button instanceof HTMLButtonElement && !button.disabled;
    }, null, { timeout: 10000 });
    await page.locator('[data-panel="intel"] [data-action="scan"]').dispatchEvent('click');
    await page.waitForFunction(() => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      return (
        state?.planetIntel?.lastScan?.status === 'planet_discovered' &&
        state?.planetIntel?.lastScan?.signal?.signal_band &&
        state?.planetIntel?.knownSignals === 1 &&
        state?.planetIntel?.planets?.length === 1 &&
        state?.progression?.main_xp >= 25
      );
    }, null, { timeout: 10000 });

    await assertCanvasAndLayout(page, viewport, label);
    await assertNoForbiddenLeaks(page, label);
    await assertNoFakeTopbarCounts(page, `${label} authenticated`);
    await assertNoUnimplementedMutationControls(page, `${label} authenticated`);
    await page.screenshot({ path: path.join(outputDir, `live-${label}.png`), fullPage: true });

    if (label === 'desktop') {
      await verifyReconnectReconciliation(page, callsign);
      await verifyRealCombatLoot(page);

      await clickWorldPosition(page, { x: 40, y: 40 });
      await page.waitForFunction(() => {
        const player = Object.values(window.__SPACE_MORPG_SMOKE_STATE__?.visibleEntities ?? {}).find(
          (entity) => entity.status_flags?.includes('self'),
        );
        return Math.abs(player?.position?.x ?? 0) > 0 || Math.abs(player?.position?.y ?? 0) > 0;
      });
    }

    await page.locator('[data-action="logout"]').click();
    await page.waitForSelector('.auth-card', { timeout: 10000 });
    await page.waitForFunction(() => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      return (
        state?.connectionStatus === 'logged_out' &&
        state?.playerSnapshot === null &&
        state?.cargo === null &&
        state?.inventory === null &&
        state?.hangar === null &&
        state?.loadout === null &&
        state?.crafting === null &&
        Object.keys(state?.visibleEntities ?? {}).length === 0
      );
    });
    await assertNoFakeTopbarCounts(page, `${label} logout`);
    await assertNoUnimplementedMutationControls(page, `${label} logout`);
  } finally {
    await page.close();
  }
}

async function verifyReconnectReconciliation(page, expectedCallsign) {
  const before = await page.evaluate(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return {
      walletCredits: state?.wallet?.credits,
      premiumPaid: state?.wallet?.premium_paid,
      questOffers: state?.questBoard?.offers?.length,
      marketListings: state?.market?.listings?.length,
      auctionLots: state?.auction?.lots?.length,
      premiumEntitlements: state?.premium?.entitlements?.length,
    };
  });

  await page.reload({ waitUntil: 'networkidle' });
  await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.connectionStatus === 'connected', null, {
    timeout: 10000,
  });
  await page.waitForFunction(
    ({ callsign, snapshot }) => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      return (
        state?.auth?.session?.authenticated === true &&
        state?.playerSnapshot?.callsign === callsign &&
        state?.sector?.name === 'Origin Fringe' &&
        state?.wallet?.credits === snapshot.walletCredits &&
        state?.wallet?.premium_paid === snapshot.premiumPaid &&
        state?.questBoard?.offers?.length === snapshot.questOffers &&
        state?.market?.listings?.length === snapshot.marketListings &&
        state?.auction?.lots?.length === snapshot.auctionLots &&
        state?.premium?.entitlements?.length === snapshot.premiumEntitlements
      );
    },
    { callsign: expectedCallsign, snapshot: before },
    { timeout: 10000 },
  );
  await assertNoForbiddenLeaks(page, 'desktop-reconnect');
  await assertNoFakeTopbarCounts(page, 'desktop-reconnect');
  await assertNoUnimplementedMutationControls(page, 'desktop-reconnect');
}

async function verifyAdminViewport(viewport, label) {
  const page = await browser.newPage({ viewport });
  try {
    await page.goto(withSmokeParam(url), { waitUntil: 'networkidle' });
    await page.waitForSelector('.auth-card', { timeout: 10000 });
    await page.locator('input[name="email"]').fill(adminEmail);
    await page.locator('input[name="password"]').fill(adminPassword);
    await page.locator('[data-submit]').click();

    await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.connectionStatus === 'connected', null, {
      timeout: 10000,
    });
    await page.waitForFunction(
      (expectedCallsign) => {
        const state = window.__SPACE_MORPG_SMOKE_STATE__;
        const opsText = document.querySelector('[data-panel="systems"]')?.textContent ?? '';
        return (
          state?.auth?.session?.account?.admin === true &&
          state?.playerSnapshot?.callsign === expectedCallsign &&
          state?.questBoard?.offers?.length === 10 &&
          state?.economyDashboard?.wallets &&
          state?.adminInspection?.wallet?.balances?.length >= 1 &&
          state?.commandLogSummary?.total >= 1 &&
          state?.metrics?.snapshot &&
          state?.releaseGate?.report?.passed === true &&
          state?.abuseCoverage?.report?.passed === true &&
          opsText.includes('Ops') &&
          opsText.includes('Gate')
        );
      },
      adminCallsign,
      { timeout: 10000 },
    );

    await assertCanvasAndLayout(page, viewport, label);
    await assertNoForbiddenLeaks(page, label);
    await assertNoFakeTopbarCounts(page, label);
    await assertNoUnimplementedMutationControls(page, label);
    await page.screenshot({ path: path.join(outputDir, `live-${label}.png`), fullPage: true });
    await page.locator('[data-action="logout"]').click();
    await page.waitForSelector('.auth-card', { timeout: 10000 });
    await assertNoFakeTopbarCounts(page, `${label} logout`);
    await assertNoUnimplementedMutationControls(page, `${label} logout`);
  } finally {
    await page.close();
  }
}

async function verifyFixtureViewport(viewport, label) {
  const realtime = await startMockRealtimeServer();
  const page = await browser.newPage({ viewport });
  try {
    await page.goto(withSmokeParam(url, { demo: '1' }), { waitUntil: 'networkidle' });
    await page.waitForSelector('canvas.world-canvas', { timeout: 10000 });
    await page.waitForFunction(() => Boolean(window.__SPACE_MORPG_SMOKE_STATE__), null, { timeout: 10000 });
    await page.waitForTimeout(350);

    await page.locator('.socket-field__input').fill(realtime.url);
    await page.locator('[data-action="connect"]').click();
    await realtime.waitForOp('debug_snapshot');
    await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.connectionStatus === 'connected');
    await page.waitForFunction(() => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      return (
        state?.playerSnapshot?.callsign === 'Server-Pilot' &&
        state?.cargo?.capacity === 80 &&
        state?.wallet?.credits === 1250 &&
        state?.stats?.radar_range === 420 &&
        state?.commandLog?.some((line) => line.text === 'Forbidden server payload rejected.') &&
        !state?.visibleEntities?.['hidden-planet']
      );
    });

    if (label === 'fixture-desktop') {
      await clickWorldPosition(page, { x: 150, y: -250 });
      await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'npc-rake-01');
      await page.locator('.hud__actionbar [data-action="fire"]').click();
      await realtime.waitForOp('combat.use_skill');
      await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.playerSnapshot?.energy === 58);

      await clickWorldPosition(page, { x: -110, y: -220 });
      await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'loot-scrap-01');
      await page.locator('.hud__actionbar [data-action="loot"]').click();
      await realtime.waitForOp('loot.pickup');
      await page.waitForFunction(() => {
        const state = window.__SPACE_MORPG_SMOKE_STATE__;
        return state?.cargo?.used === 21 && state?.cargo?.items?.some((item) => item.item_id === 'raw_ore' && item.quantity === 15);
      });

      await clickWorldPosition(page, { x: 40, y: -220 });
      await realtime.waitForOp('move_to');
      await page.waitForFunction(() => {
        const player = window.__SPACE_MORPG_SMOKE_STATE__?.visibleEntities?.['player-local'];
        return Math.abs((player?.position?.x ?? 9999) - 40) <= 1 && Math.abs((player?.position?.y ?? 9999) + 220) <= 1;
      });
    }

    await assertCanvasAndLayout(page, viewport, label);
    await assertNoForbiddenLeaks(page, label);
    await assertNoFakeTopbarCounts(page, label);
    await page.screenshot({ path: path.join(outputDir, `${label}.png`), fullPage: true });
  } finally {
    await page.close();
    await realtime.close();
  }
}

async function verifyRealCombatLoot(page) {
  await clickWorldPosition(page, { x: 80, y: 0 });
  await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'entity_training_npc', null, {
    timeout: 10000,
  });
  await page.locator('.hud__actionbar [data-action="fire"]').click();
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const entities = state?.visibleEntities ?? {};
    const lootDrop = Object.values(entities).find((entity) => entity.entity_type === 'loot');
    return (
      !entities.entity_training_npc &&
      Boolean(lootDrop) &&
      state?.combatLog?.some((line) => /destroyed/i.test(line.text))
    );
  }, null, { timeout: 10000 });

  const dropPosition = await page.evaluate(() => {
    const drop = Object.values(window.__SPACE_MORPG_SMOKE_STATE__?.visibleEntities ?? {}).find(
      (entity) => entity.entity_type === 'loot',
    );
    return drop?.position ?? null;
  });
  if (!dropPosition) {
    throw new Error('No server-created loot drop was visible after combat.');
  }

  await clickWorldPosition(page, dropPosition);
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const target = state?.selectedTargetID ? state.visibleEntities?.[state.selectedTargetID] : null;
    return target?.entity_type === 'loot';
  }, null, { timeout: 10000 });
  await page.locator('.hud__actionbar [data-action="loot"]').click();
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const entities = state?.visibleEntities ?? {};
    return (
      state?.cargo?.used === 6 &&
      state?.cargo?.items?.some((item) => item.item_id === 'raw_ore' && item.quantity === 3) &&
      state?.inventory?.stackable?.some((item) => item.item_id === 'raw_ore' && item.quantity === 3 && item.location === 'ship_cargo') &&
      !Object.values(entities).some((entity) => entity.entity_type === 'loot')
    );
  }, null, { timeout: 10000 });
}

async function verifyRealEconomy(page) {
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const button = document.querySelector('[data-panel="economy"] [data-action="market-buy"]');
    const activeListing = state?.market?.listings?.find((listing) => listing.status === 'active' && !listing.owned_by_you);
    return activeListing?.remaining_quantity > 0 && button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-panel="economy"] [data-action="market-buy"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return (
      state?.wallet?.credits === 1175 &&
      state?.inventory?.stackable?.some((item) => item.item_id === 'raw_ore' && item.quantity === 1 && item.location === 'account_inventory')
    );
  }, null, { timeout: 10000 });

  await page.waitForFunction(() => {
    const button = document.querySelector('[data-panel="economy"] [data-action="market-create"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-panel="economy"] [data-action="market-create"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return (
      state?.market?.listings?.some((listing) => listing.owned_by_you && listing.status === 'active') &&
      !state?.inventory?.stackable?.some((item) => item.item_id === 'raw_ore' && item.location === 'account_inventory')
    );
  }, null, { timeout: 10000 });

  await page.waitForFunction(() => {
    const button = document.querySelector('[data-panel="economy"] [data-action="market-cancel"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-panel="economy"] [data-action="market-cancel"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return (
      !state?.market?.listings?.some((listing) => listing.owned_by_you && listing.status === 'active') &&
      state?.inventory?.stackable?.some((item) => item.item_id === 'raw_ore' && item.quantity === 1 && item.location === 'account_inventory')
    );
  }, null, { timeout: 10000 });

  await page.waitForFunction(() => {
    const button = document.querySelector('[data-panel="economy"] [data-action="auction-bid"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-panel="economy"] [data-action="auction-bid"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return state?.auction?.lots?.[0]?.leading === true && state?.wallet?.credits < 1175;
  }, null, { timeout: 10000 });

  await page.waitForFunction(() => {
    const button = document.querySelector('[data-panel="economy"] [data-action="premium-claim"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-panel="economy"] [data-action="premium-claim"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return state?.wallet?.premium_earned === 50 && state?.premium?.entitlements?.[0]?.state === 'claimed';
  }, null, { timeout: 10000 });

  await page.waitForFunction(() => {
    const button = document.querySelector('[data-panel="economy"] [data-action="premium-weekly-xcore"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-panel="economy"] [data-action="premium-weekly-xcore"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return state?.wallet?.premium_paid === 200 && state?.premium?.purchases?.length === 1;
  }, null, { timeout: 10000 });
}

async function bootstrapDiagnostics(page, expectedCallsign) {
  return page.evaluate((callsign) => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const entities = state?.visibleEntities ?? {};
    const self = Object.values(entities).find((entity) => entity.status_flags?.includes('self'));
    return {
      expectedCallsign: callsign,
      connectionStatus: state?.connectionStatus,
      auth: state?.auth,
      playerSnapshot: state?.playerSnapshot,
      sector: state?.sector,
      minimapContacts: state?.minimap?.live_contacts?.map((contact) => contact.entity_id),
      entityIDs: Object.keys(entities),
      self,
      cargo: state?.cargo,
      wallet: state?.wallet,
      ship: state?.ship,
      progression: state?.progression,
      stats: state?.stats,
      inventoryCounts: state?.inventory?.counts,
      hangar: state?.hangar,
      loadoutSlots: state?.loadout?.slots?.length,
      craftingRecipes: state?.crafting?.recipes?.length,
      planetIntel: state?.planetIntel,
      productionPlanets: state?.production?.planets?.length,
      routes: state?.routes,
      market: state?.market,
      auction: state?.auction,
      premium: state?.premium,
      commandLog: state?.commandLog,
    };
  }, expectedCallsign);
}

async function assertCanvasAndLayout(page, viewport, label) {
  const stats = await page.evaluate(() => {
    const canvas = document.querySelector('canvas.world-canvas');
    if (!(canvas instanceof HTMLCanvasElement)) {
      return { samples: 0, nonBlank: 0, width: 0, height: 0, scrollWidth: document.body.scrollWidth };
    }

    const context = canvas.getContext('2d');
    if (!context) {
      return { samples: 0, nonBlank: 1, width: canvas.width, height: canvas.height, scrollWidth: document.body.scrollWidth };
    }

    const width = canvas.width;
    const height = canvas.height;
    let samples = 0;
    let nonBlank = 0;
    for (let y = 0; y < height; y += Math.max(1, Math.floor(height / 12))) {
      for (let x = 0; x < width; x += Math.max(1, Math.floor(width / 12))) {
        const [r, g, b, a] = context.getImageData(x, y, 1, 1).data;
        samples += 1;
        if (a > 0 && (r > 8 || g > 8 || b > 8)) {
          nonBlank += 1;
        }
      }
    }
    return { samples, nonBlank, width, height, scrollWidth: document.body.scrollWidth };
  });

  if (stats.width === 0 || stats.height === 0) {
    throw new Error(`${label}: canvas has no size`);
  }
  if (stats.nonBlank === 0) {
    throw new Error(`${label}: canvas appears blank`);
  }
  if (stats.scrollWidth > viewport.width + 1) {
    throw new Error(`${label}: layout has horizontal overflow (${stats.scrollWidth} > ${viewport.width})`);
  }
}

async function assertNoForbiddenLeaks(page, label) {
  const text = await page.locator('body').innerText();
  assertNoForbiddenText(text, `${label} body`);
  const smokeState = await page.evaluate(() => JSON.stringify(window.__SPACE_MORPG_SMOKE_STATE__));
  assertNoForbiddenText(smokeState, `${label} smoke state`);
  const browserStorage = await page.evaluate(() => {
    const local = {};
    const session = {};
    for (let index = 0; index < window.localStorage.length; index += 1) {
      const key = window.localStorage.key(index);
      if (key) {
        local[key] = window.localStorage.getItem(key);
      }
    }
    for (let index = 0; index < window.sessionStorage.length; index += 1) {
      const key = window.sessionStorage.key(index);
      if (key) {
        session[key] = window.sessionStorage.getItem(key);
      }
    }
    return JSON.stringify({ cookie: document.cookie, local, session });
  });
  assertNoForbiddenText(browserStorage, `${label} browser storage`);
}

async function assertNoFakeTopbarCounts(page, label) {
  const violations = await page.evaluate((patterns) => {
    const compiled = patterns.map((entry) => ({ label: entry.label, pattern: new RegExp(entry.source, entry.flags) }));
    const isVisible = (element) => {
      const style = window.getComputedStyle(element);
      return style.display !== 'none' && style.visibility !== 'hidden' && element.getClientRects().length > 0;
    };
    const elementText = (element) => (element.innerText || element.textContent || '').replace(/\s+/g, ' ').trim();
    const elementMetadata = (element) =>
      Array.from(element.attributes)
        .filter((attribute) => attribute.name === 'aria-label' || attribute.name === 'title' || attribute.name.startsWith('data-'))
        .map((attribute) => `${attribute.name}=${attribute.value}`)
        .join(' ');
    const socialTermPattern = /\b(mail|inbox|unread|friends?|social|party|menu|notifications?)\b/i;
    const candidates = Array.from(document.querySelectorAll('.hud__topbar, .hud__topbar *, .toolbar, .toolbar *, [aria-label], [title]')).filter(
      (element) => element instanceof HTMLElement && isVisible(element),
    );
    const matches = [];

    for (const element of candidates) {
      const haystack = `${elementText(element)} ${elementMetadata(element)}`.trim();
      if (!socialTermPattern.test(haystack)) {
        continue;
      }
      for (const entry of compiled) {
        if (entry.pattern.test(haystack)) {
          matches.push({ kind: entry.label, text: haystack });
        }
      }
    }

    return matches;
  }, fakeSocialCountPatterns.map((entry) => ({ label: entry.label, source: entry.pattern.source, flags: entry.pattern.flags })));

  if (violations.length > 0) {
    throw new Error(
      `${label}: fake mail/social/menu notification count visible: ${violations
        .map((violation) => `${violation.kind} in "${violation.text}"`)
        .join('; ')}`,
    );
  }
}

async function assertNoUnimplementedMutationControls(page, label) {
  const violations = await page.evaluate(
    ({ operations, patterns }) => {
      const compiled = patterns.map((entry) => ({ label: entry.label, pattern: new RegExp(entry.source, entry.flags) }));
      const aliases = operations.flatMap((operation) => [
        operation,
        operation.replace(/\./g, '-'),
        operation.replace(/\./g, '_'),
        operation.replace(/[._]/g, '-'),
        operation.replace(/[._]/g, '_'),
      ]);
      const uniqueAliases = Array.from(new Set(aliases.map((alias) => alias.toLowerCase())));
      const isVisible = (element) => {
        const style = window.getComputedStyle(element);
        return style.display !== 'none' && style.visibility !== 'hidden' && element.getClientRects().length > 0;
      };
      const isEnabledAction = (element) => {
        if (!(element instanceof HTMLElement) || !isVisible(element)) {
          return false;
        }
        if (element.matches(':disabled') || element.hasAttribute('disabled') || element.getAttribute('aria-disabled') === 'true') {
          return false;
        }
        return window.getComputedStyle(element).pointerEvents !== 'none';
      };
      const elementText = (element) =>
        [element.innerText, element.textContent, element instanceof HTMLInputElement ? element.value : '']
          .filter(Boolean)
          .join(' ')
          .replace(/\s+/g, ' ')
          .trim();
      const elementMetadata = (element) =>
        Array.from(element.attributes)
          .filter(
            (attribute) =>
              attribute.name === 'aria-label' ||
              attribute.name === 'title' ||
              attribute.name === 'name' ||
              attribute.name === 'value' ||
              attribute.name === 'data-action' ||
              attribute.name === 'data-op' ||
              attribute.name === 'data-command',
          )
          .map((attribute) => `${attribute.name}=${attribute.value}`)
          .join(' ');
      const candidates = Array.from(
        document.querySelectorAll('button, a[href], input[type="button"], input[type="submit"], [role="button"], [data-action]'),
      ).filter(isEnabledAction);
      const matches = [];

      for (const element of candidates) {
        const haystack = `${elementText(element)} ${elementMetadata(element)}`.replace(/\s+/g, ' ').trim();
        const normalized = haystack.toLowerCase();
        const alias = uniqueAliases.find((entry) => normalized.includes(entry));
        if (alias) {
          matches.push({ kind: alias, text: haystack });
          continue;
        }
        for (const entry of compiled) {
          if (entry.pattern.test(haystack)) {
            matches.push({ kind: entry.label, text: haystack });
          }
        }
      }

      return matches;
    },
    {
      operations: unimplementedMutationOps,
      patterns: unimplementedMutationControlPatterns.map((entry) => ({
        label: entry.label,
        source: entry.pattern.source,
        flags: entry.pattern.flags,
      })),
    },
  );

  if (violations.length > 0) {
    throw new Error(
      `${label}: unimplemented mutation control is visible and enabled: ${violations
        .map((violation) => `${violation.kind} in "${violation.text}"`)
        .join('; ')}`,
    );
  }
}

async function clickWorldPosition(page, world) {
  const point = await page.evaluate((target) => {
    const canvas = document.querySelector('canvas.world-canvas');
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    if (!(canvas instanceof HTMLCanvasElement) || !state) {
      return null;
    }
    const rect = canvas.getBoundingClientRect();
    const player =
      Object.values(state.visibleEntities ?? {}).find((entity) => entity.status_flags?.includes('self')) ??
      Object.values(state.visibleEntities ?? {}).find((entity) => entity.entity_type === 'player');
    const center = player?.position ?? { x: 0, y: 0 };
    const scale = rect.width < 700 ? 0.78 : 1;
    return {
      x: rect.left + rect.width / 2 + (target.x - center.x) * scale,
      y: rect.top + rect.height / 2 + (target.y - center.y) * scale,
      target,
      center,
      rect: { left: rect.left, top: rect.top, width: rect.width, height: rect.height },
      element: document.elementFromPoint(
        rect.left + rect.width / 2 + (target.x - center.x) * scale,
        rect.top + rect.height / 2 + (target.y - center.y) * scale,
      )?.className,
    };
  }, world);

  if (!point) {
    throw new Error('Could not map world position to canvas point.');
  }
  if (point.element !== 'world-canvas') {
    throw new Error(`World click at ${Math.round(point.x)},${Math.round(point.y)} hit ${String(point.element)} (${JSON.stringify(point)})`);
  }
  await page.mouse.click(point.x, point.y);
}

function assertNoForbiddenText(text, label) {
  for (const forbidden of forbiddenText) {
    if (text.includes(forbidden)) {
      throw new Error(`${label}: forbidden debug text leaked: ${forbidden}`);
    }
  }
}

async function startGameServer(port, allowedOrigin) {
  const child = spawn('go', ['run', './cmd/game-server'], {
    cwd: repoRoot,
    env: {
      ...process.env,
      GOCACHE: process.env.GOCACHE ?? '/tmp/gameproject-go-cache',
      GAME_SERVER_ADDR: `127.0.0.1:${port}`,
      GAME_ALLOWED_ORIGINS: allowedOrigin,
      GAME_ADMIN_EMAIL: adminEmail,
      GAME_ADMIN_PASSWORD: adminPassword,
      GAME_ADMIN_CALLSIGN: adminCallsign,
    },
    detached: true,
    stdio: ['ignore', 'pipe', 'pipe'],
  });
  const logs = [];
  child.stdout.on('data', (chunk) => logs.push(chunk.toString('utf8')));
  child.stderr.on('data', (chunk) => logs.push(chunk.toString('utf8')));
  await waitForHealth(`http://127.0.0.1:${port}/healthz`, child, logs);
  return {
    close: async () => {
      if (child.exitCode !== null) {
        return;
      }
      killProcessGroup(child, 'SIGTERM');
      await waitForExit(child, 3000).catch(() => killProcessGroup(child, 'SIGKILL'));
    },
  };
}

async function waitForHealth(healthURL, child, logs) {
  const deadline = Date.now() + 20000;
  while (Date.now() < deadline) {
    if (child.exitCode !== null) {
      throw new Error(`game server exited early with ${child.exitCode}:\n${logs.join('')}`);
    }
    try {
      const response = await fetch(healthURL);
      if (response.ok) {
        return;
      }
    } catch {
      // Server is still booting.
    }
    await delay(100);
  }
  throw new Error(`timed out waiting for game server:\n${logs.join('')}`);
}

async function startMockRealtimeServer() {
  const received = [];
  const waiters = new Map();
  const sockets = new Set();
  const server = net.createServer((socket) => {
    sockets.add(socket);
    let buffer = Buffer.alloc(0);
    let handshaken = false;

    socket.on('data', (chunk) => {
      buffer = Buffer.concat([buffer, chunk]);
      if (!handshaken) {
        const end = buffer.indexOf('\r\n\r\n');
        if (end === -1) {
          return;
        }
        const request = buffer.subarray(0, end + 4).toString('utf8');
        buffer = buffer.subarray(end + 4);
        socket.write(webSocketHandshakeResponse(request));
        handshaken = true;
      }

      for (;;) {
        const frame = readClientFrame(buffer);
        if (!frame) {
          return;
        }
        buffer = buffer.subarray(frame.bytesRead);
        if (frame.opcode === 8) {
          socket.end();
          return;
        }
        if (frame.opcode === 1) {
          handleClientMessage(socket, frame.payload, received, waiters);
        }
      }
    });

    socket.on('close', () => sockets.delete(socket));
    socket.on('error', () => sockets.delete(socket));
  });

  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  if (!address || typeof address === 'string') {
    throw new Error('Mock realtime server did not expose a TCP port.');
  }

  return {
    url: `ws://127.0.0.1:${address.port}/ws`,
    waitForOp: (op) => waitForOp(received, waiters, op),
    close: async () => {
      for (const socket of sockets) {
        socket.destroy();
      }
      await new Promise((resolve) => server.close(resolve));
    },
  };
}

async function startViteAppServer({ port, gamePort }) {
  const server = await createViteServer({
    root: clientRoot,
    logLevel: 'error',
    server: {
      host: '127.0.0.1',
      port,
      strictPort: true,
      proxy: gamePort
        ? {
            '/api': {
              target: `http://127.0.0.1:${gamePort}`,
              changeOrigin: true,
            },
            '/ws': {
              target: `ws://127.0.0.1:${gamePort}`,
              ws: true,
            },
          }
        : undefined,
    },
  });
  await server.listen();

  return {
    url: `http://127.0.0.1:${port}`,
    close: () => server.close(),
  };
}

function findFreePort() {
  return new Promise((resolve, reject) => {
    const server = net.createServer();
    server.unref();
    server.on('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      server.close((error) => {
        if (error) {
          reject(error);
          return;
        }
        if (!address || typeof address === 'string') {
          reject(new Error('Could not reserve a local smoke app port.'));
          return;
        }
        resolve(address.port);
      });
    });
  });
}

function webSocketHandshakeResponse(request) {
  const key = request.match(/^Sec-WebSocket-Key:\s*(.+)$/im)?.[1]?.trim();
  if (!key) {
    throw new Error('Missing Sec-WebSocket-Key.');
  }
  const accept = createHash('sha1')
    .update(`${key}258EAFA5-E914-47DA-95CA-C5AB0DC85B11`)
    .digest('base64');
  return [
    'HTTP/1.1 101 Switching Protocols',
    'Upgrade: websocket',
    'Connection: Upgrade',
    `Sec-WebSocket-Accept: ${accept}`,
    '',
    '',
  ].join('\r\n');
}

function readClientFrame(buffer) {
  if (buffer.length < 2) {
    return null;
  }
  const opcode = buffer[0] & 0x0f;
  const masked = (buffer[1] & 0x80) !== 0;
  let length = buffer[1] & 0x7f;
  let offset = 2;
  if (length === 126) {
    if (buffer.length < offset + 2) {
      return null;
    }
    length = buffer.readUInt16BE(offset);
    offset += 2;
  } else if (length === 127) {
    if (buffer.length < offset + 8) {
      return null;
    }
    const high = buffer.readUInt32BE(offset);
    const low = buffer.readUInt32BE(offset + 4);
    length = high * 2 ** 32 + low;
    offset += 8;
  }
  const maskOffset = offset;
  if (masked) {
    offset += 4;
  }
  if (buffer.length < offset + length) {
    return null;
  }
  const payload = Buffer.from(buffer.subarray(offset, offset + length));
  if (masked) {
    const mask = buffer.subarray(maskOffset, maskOffset + 4);
    for (let index = 0; index < payload.length; index += 1) {
      payload[index] ^= mask[index % 4];
    }
  }
  return {
    opcode,
    payload: payload.toString('utf8'),
    bytesRead: offset + length,
  };
}

function handleClientMessage(socket, raw, received, waiters) {
  const request = JSON.parse(raw);
  received.push(request);
  const waiter = waiters.get(request.op);
  if (waiter) {
    waiters.delete(request.op);
    waiter.resolve(request);
  }

  switch (request.op) {
    case 'debug_snapshot':
      sendMessage(socket, response(request.request_id, snapshotPayload()));
      sendMessage(
        socket,
        event('hidden-rejected', 'aoi.entity_entered', {
          entity_id: 'hidden-planet',
          entity_type: 'planet_signal',
          position: { x: 9000, y: 9000 },
          gameplay_seed: 'server-only',
        }),
      );
      break;
    case 'move_to':
      sendMessage(socket, response(request.request_id, { accepted: true }));
      sendMessage(socket, event('move-correction', 'position.corrected', { entity_id: 'player-local', position: request.payload.target }));
      break;
    case 'combat.use_skill':
      sendMessage(socket, response(request.request_id, { accepted: true }));
      sendMessage(socket, event('combat-energy', 'player.snapshot', { energy: 58, shield: 61, hp: 84 }));
      break;
    case 'loot.pickup':
      sendMessage(
        socket,
        response(request.request_id, {
          cargo: {
            used: 21,
            capacity: 80,
            items: [
              { item_id: 'raw_ore', quantity: 15 },
              { item_id: 'salvage_thread', quantity: 6 },
            ],
          },
        }),
      );
      break;
    default:
      sendMessage(socket, errorResponse(request.request_id, 'ERR_INVALID_PAYLOAD', 'Unsupported operation.'));
  }
}

function waitForOp(received, waiters, op) {
  const found = received.find((message) => message.op === op);
  if (found) {
    return Promise.resolve(found);
  }
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      waiters.delete(op);
      reject(new Error(`Timed out waiting for ${op}; received ops: ${received.map((message) => message.op).join(', ')}`));
    }, 5000);
    waiters.set(op, {
      resolve: (message) => {
        clearTimeout(timeout);
        resolve(message);
      },
    });
  });
}

function sendMessage(socket, message) {
  socket.write(encodeServerFrame(JSON.stringify(message)));
}

function encodeServerFrame(text) {
  const payload = Buffer.from(text, 'utf8');
  if (payload.length < 126) {
    return Buffer.concat([Buffer.from([0x81, payload.length]), payload]);
  }
  if (payload.length <= 0xffff) {
    const header = Buffer.alloc(4);
    header[0] = 0x81;
    header[1] = 126;
    header.writeUInt16BE(payload.length, 2);
    return Buffer.concat([header, payload]);
  }
  throw new Error('Smoke fixture frame is too large.');
}

function snapshotPayload() {
  return {
    entities: [
      {
        entity_id: 'player-local',
        entity_type: 'player',
        position: { x: 0, y: 0 },
        status_flags: ['local', 'self'],
        display: { label: 'Server-Pilot', disposition: 'self' },
      },
      {
        entity_id: 'npc-rake-01',
        entity_type: 'npc',
        display: { label: 'Drone Rake', disposition: 'hostile' },
        position: { x: 150, y: -250 },
        status_flags: ['visible', 'hostile'],
      },
      {
        entity_id: 'loot-scrap-01',
        entity_type: 'loot',
        display: { label: 'Scrap Cache', disposition: 'neutral' },
        position: { x: -110, y: -220 },
        status_flags: ['visible'],
      },
      {
        entity_id: 'signal-eris-04',
        entity_type: 'planet_signal',
        display: { label: 'Unknown Signal', disposition: 'unknown' },
        position: { x: 260, y: 150 },
        status_flags: ['known_intel'],
      },
    ],
    sector: {
      name: 'Fixture Fringe',
      region: 'Fixture Belt',
      danger: 'locked',
      contested: false,
    },
    minimap: {
      radar_range: 420,
      live_contacts: [
        {
          entity_id: 'player-local',
          entity_type: 'player',
          position: { x: 0, y: 0 },
          disposition: 'self',
          status_flags: ['self'],
        },
        {
          entity_id: 'npc-rake-01',
          entity_type: 'npc',
          position: { x: 150, y: -250 },
          disposition: 'hostile',
          status_flags: ['hostile'],
        },
        {
          entity_id: 'loot-scrap-01',
          entity_type: 'loot',
          position: { x: -110, y: -220 },
          disposition: 'neutral',
          status_flags: ['loot'],
        },
        {
          entity_id: 'signal-eris-04',
          entity_type: 'planet_signal',
          position: { x: 260, y: 150 },
          disposition: 'unknown',
          status_flags: ['unknown_signal'],
        },
      ],
      remembered: [],
    },
    player: {
      callsign: 'Server-Pilot',
      hp: 84,
      shield: 61,
      energy: 72,
      max_hp: 100,
      max_shield: 100,
      max_energy: 100,
      rank: 2,
    },
    cargo: {
      used: 17,
      capacity: 80,
      items: [
        { item_id: 'raw_ore', quantity: 11 },
        { item_id: 'salvage_thread', quantity: 6 },
      ],
    },
    wallet: {
      credits: 1250,
      premium_paid: 0,
      premium_earned: 25,
    },
    stats: {
      speed: 180,
      radar_range: 420,
      weapon_range: 260,
      cargo_capacity: 80,
    },
  };
}

function response(requestID, payload) {
  return {
    request_id: requestID,
    ok: true,
    payload,
    server_time: Date.now(),
    v: 1,
  };
}

function errorResponse(requestID, code, message) {
  return {
    request_id: requestID,
    ok: false,
    error: { code, message, retryable: false },
    server_time: Date.now(),
    v: 1,
  };
}

function event(eventID, type, payload) {
  eventSequence += 1;
  return {
    event_id: eventID,
    type,
    payload,
    server_time: Date.now(),
    seq: eventSequence,
    v: 1,
  };
}

function withSmokeParam(rawURL, extra = {}) {
  const parsed = new URL(rawURL);
  parsed.searchParams.set('smoke', '1');
  for (const [key, value] of Object.entries(extra)) {
    parsed.searchParams.set(key, value);
  }
  return parsed.toString();
}

function readArg(name) {
  const index = process.argv.indexOf(name);
  if (index === -1) {
    return null;
  }
  return process.argv[index + 1] ?? null;
}

function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function waitForExit(child, timeoutMs) {
  if (child.exitCode !== null) {
    return Promise.resolve();
  }
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('process exit timed out')), timeoutMs);
    child.once('exit', () => {
      clearTimeout(timeout);
      resolve();
    });
  });
}

function killProcessGroup(child, signal) {
  if (!child.pid) {
    return;
  }
  try {
    process.kill(-child.pid, signal);
  } catch {
    child.kill(signal);
  }
}

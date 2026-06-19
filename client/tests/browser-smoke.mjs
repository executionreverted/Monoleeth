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
const phaseOutputDir = path.resolve(repoRoot, 'output', 'screenshots', 'ui-patch-2', '01');
const phase02OutputDir = path.resolve(repoRoot, 'output', 'screenshots', 'ui-patch-2', '02');
const phase03OutputDir = path.resolve(repoRoot, 'output', 'screenshots', 'ui-patch-2', '03');
const phase04OutputDir = path.resolve(repoRoot, 'output', 'screenshots', 'ui-patch-2', '04');
const phase05OutputDir = path.resolve(repoRoot, 'output', 'screenshots', 'ui-patch-2', '05');
const phase06OutputDir = path.resolve(repoRoot, 'output', 'screenshots', 'ui-patch-2', '06');
const phase07OutputDir = path.resolve(repoRoot, 'output', 'screenshots', 'ui-patch-2', '07');
const phase08OutputDir = path.resolve(repoRoot, 'output', 'screenshots', 'ui-patch-2', '08');
const phasePatch3OutputDir = path.resolve(repoRoot, 'output', 'screenshots', 'ui-patch-3');
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
  await mkdir(phaseOutputDir, { recursive: true });
  await mkdir(phase02OutputDir, { recursive: true });
  await mkdir(phase03OutputDir, { recursive: true });
  await mkdir(phase04OutputDir, { recursive: true });
  await mkdir(phase05OutputDir, { recursive: true });
  await mkdir(phase06OutputDir, { recursive: true });
  await mkdir(phase07OutputDir, { recursive: true });
  await mkdir(phase08OutputDir, { recursive: true });
  await mkdir(phasePatch3OutputDir, { recursive: true });
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
    if (label === 'mobile' || label === 'desktop') {
      await page.screenshot({ path: path.join(phase08OutputDir, `unauth-${label}.png`), fullPage: true });
    }

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
            state?.ship?.active_ship_id === 'starter' &&
            state?.ship?.disabled === false &&
            state?.progression?.rank >= 1 &&
            state?.stats?.radar_range === 420 &&
            state?.stats?.loot_pickup_range === 120 &&
            state?.stats?.basic_laser_energy_cost === 10 &&
            state?.inventory?.counts?.cargo_stacks === 0 &&
            state?.inventory?.counts?.equipped_instances === 1 &&
            state?.inventory?.instances?.some((item) => item.item_id === 'scanner_t1' && item.location === 'ship_equipped') &&
            state?.inventory?.instances?.some((item) => item.item_id === 'laser_alpha_t1' && item.location === 'account_inventory') &&
            state?.hangar?.active_ship_id === 'starter' &&
            state?.loadout?.slots?.length === 3 &&
            state?.loadout?.slots?.some((slot) => slot.slot_id === 'utility_1' && slot.module_item_id === 'scanner_t1') &&
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
    await verifyQuickActionContracts(page, viewport, label);
    await verifyInventoryLoadout(page, viewport, label);
    await verifyHangarSurface(page, viewport, label);

    await verifyRealEconomy(page, viewport, label);

    await verifyScanModeAutomation(page, viewport, label);
    await verifyPlanetMemoryMarker(page, label);
    await verifyPlanetCatalogSurface(page, viewport, label);
    await verifyQuestBoardSurface(page, viewport, label);

    await verifyPanelModalChrome(page, viewport, label);
    await assertCanvasAndLayout(page, viewport, label);
    await verifyStarfieldBackground(page, label);
    await verifyFogOfWar(page, label);
    await assertMockupParityShell(page, viewport, label);
    await assertNoForbiddenLeaks(page, label);
    await assertNoFakeTopbarCounts(page, `${label} authenticated`);
    await assertNoUnimplementedMutationControls(page, `${label} authenticated`);
    await page.screenshot({ path: path.join(outputDir, `live-${label}.png`), fullPage: true });
    await page.screenshot({ path: path.join(phase07OutputDir, `live-${label}.png`), fullPage: true });
    await page.screenshot({ path: path.join(phase08OutputDir, `live-${label}.png`), fullPage: true });

    if (label === 'desktop') {
      await verifyReconnectReconciliation(page, callsign);
      await verifyRealCombatLoot(page);
      await verifyRealMovementInterpolation(page);
      await verifyPlanetNavigateAction(page, label);
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

async function verifyPlanetMemoryMarker(page, label) {
  await page.waitForFunction(() => {
    const button = document.querySelector('[data-panel="planets"] [data-action="planet-detail"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });

  const moveCountBeforeDetail = await commandLogCount(page, 'Sent move_to.');
  await page.locator('[data-panel="planets"] [data-action="planet-detail"]').first().dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const detail = state?.planetIntel?.selectedPlanet;
    const marker = state?.worldView?.memoryMarkers?.[0];
    const detailPanel = document.querySelector('[data-modal="planet-detail"] [data-planet-detail]');
    const inlineDetail = document.querySelector('[data-panel="planets"] [data-planet-detail]');
    return (
      detail?.planet_id &&
      Number.isFinite(detail.coordinates?.x) &&
      Number.isFinite(detail.coordinates?.y) &&
      marker?.detailID === detail.planet_id &&
      marker?.position?.x === detail.coordinates.x &&
      marker?.position?.y === detail.coordinates.y &&
      detailPanel instanceof HTMLElement &&
      inlineDetail === null
    );
  }, null, { timeout: 10000 });

  const moveCountAfterDetail = await commandLogCount(page, 'Sent move_to.');
  if (moveCountAfterDetail !== moveCountBeforeDetail) {
    throw new Error(`${label}: requesting planet detail emitted move_to`);
  }

  await page.screenshot({ path: path.join(phaseOutputDir, `planet-memory-${label}.png`), fullPage: true });

  if (label !== 'desktop') {
    await page.locator('[data-modal-close="button"]').click();
    await page.waitForFunction(() => !document.querySelector('[data-modal]'), null, { timeout: 10000 });
    await syncRememberedMinimap(page, label);
    return;
  }

  await page.locator('[data-modal-close="button"]').click();
  await page.waitForFunction(() => !document.querySelector('[data-modal]'), null, { timeout: 10000 });
  await syncRememberedMinimap(page, label);

  await page.waitForTimeout(250);
  await clickWorldPosition(page, { x: 80, y: 0 });
  await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'entity_training_npc', null, {
    timeout: 10000,
  });

  const markerWorld = await page.evaluate(() => {
    const marker = window.__SPACE_MORPG_SMOKE_STATE__?.worldView?.memoryMarkers?.[0];
    return marker?.position ?? null;
  });
  if (!markerWorld) {
    throw new Error(`${label}: planet memory marker was missing before marker click`);
  }

  await ensureWorldPositionClickable(page, markerWorld);
  const moveCountBeforeMarker = await commandLogCount(page, 'Sent move_to.');
  await clickWorldPosition(page, markerWorld);
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return (
      state?.selectedTargetID === null &&
      state?.commandLog?.some((line) => /Selected known planet/i.test(line.text)) &&
      state?.commandLog?.some((line) => line.text === 'Sent discovery.planet_detail.')
    );
  }, null, { timeout: 10000 });
  const moveCountAfterMarker = await commandLogCount(page, 'Sent move_to.');
  if (moveCountAfterMarker !== moveCountBeforeMarker) {
    throw new Error(`${label}: planet memory marker click emitted move_to`);
  }
  await page.screenshot({ path: path.join(phase08OutputDir, 'selected-planet-desktop.png'), fullPage: true });
}

async function verifyPlanetNavigateAction(page, label) {
  await page.locator('[data-panel="planets"] [data-action="planet-detail"]').first().dispatchEvent('click');
  await page.waitForSelector('[data-modal="planet-detail"] [data-action="planet-navigate"]:not([disabled])', { timeout: 10000 });
  const moveCountBeforeNavigate = await commandLogCount(page, 'Sent move_to.');
  await page.locator('[data-modal="planet-detail"] [data-action="planet-navigate"]').click();
  await page.waitForFunction(
    (before) => {
      const lines = window.__SPACE_MORPG_SMOKE_STATE__?.commandLog ?? [];
      return lines.filter((line) => line.text === 'Sent move_to.').length > before;
    },
    moveCountBeforeNavigate,
    { timeout: 10000 },
  );
  await page.locator('[data-modal-close="button"]').click();
  await page.waitForFunction(() => !document.querySelector('[data-modal]'), null, { timeout: 10000 });
}

async function verifyPlanetCatalogSurface(page, viewport, label) {
  await page.locator('[data-panel-toggle="intel"]').click();
  await page.waitForSelector('[data-window-panel="intel"][data-focused="true"] .planet-catalog', { timeout: 10000 });
  await page.waitForSelector('[data-window-panel="intel"] .planet-catalog-row', { timeout: 10000 });

  const moveCountBeforeSelect = await commandLogCount(page, 'Sent move_to.');
  await page.locator('[data-window-panel="intel"] .planet-catalog-row').first().dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const detail = state?.planetIntel?.selectedPlanet;
    const selectedRow = document.querySelector('[data-window-panel="intel"] .planet-catalog-row[data-selected="true"]');
    const sections = [...document.querySelectorAll('[data-window-panel="intel"] .planet-section-grid h3')].map((node) => node.textContent?.trim());
    return (
      detail?.planet_id &&
      Number.isFinite(detail.coordinates?.x) &&
      Number.isFinite(detail.coordinates?.y) &&
      selectedRow instanceof HTMLElement &&
      ['Overview', 'Production', 'Storage', 'Routes', 'Intel'].every((section) => sections.includes(section))
    );
  }, null, { timeout: 10000 });
  const moveCountAfterSelect = await commandLogCount(page, 'Sent move_to.');
  if (moveCountAfterSelect !== moveCountBeforeSelect) {
    throw new Error(`${label}: selecting a planet catalog row emitted move_to`);
  }

  const summary = await page.evaluate(() => {
    const actions = [...document.querySelectorAll('[data-window-panel="intel"] .planet-catalog__actions button')];
    return {
      selectedPlanetID: window.__SPACE_MORPG_SMOKE_STATE__?.planetIntel?.selectedPlanet?.planet_id ?? '',
      rowCount: document.querySelectorAll('[data-window-panel="intel"] .planet-catalog-row').length,
      navigateEnabled: actions[0] instanceof HTMLButtonElement ? !actions[0].disabled : false,
      lockedDisabled: actions.slice(1).every((button) => button instanceof HTMLButtonElement && button.disabled),
      hasProductionSection: Boolean(
        [...document.querySelectorAll('[data-window-panel="intel"] .planet-section-grid h3')].find(
          (node) => node.textContent?.trim() === 'Production',
        ),
      ),
    };
  });
  if (!summary.selectedPlanetID || summary.rowCount < 1 || !summary.navigateEnabled || !summary.lockedDisabled || !summary.hasProductionSection) {
    throw new Error(`${label}: planet catalog surface mismatch ${JSON.stringify(summary)}`);
  }

  await page.screenshot({ path: path.join(phasePatch3OutputDir, `planets-catalog-${label}.png`), fullPage: true });
  if (viewport.width < 768) {
    const hasHorizontalScroll = await page.evaluate(() => document.scrollingElement.scrollWidth > window.innerWidth + 1);
    if (hasHorizontalScroll) {
      throw new Error(`${label}: planets catalog created horizontal body scroll`);
    }
  }
  await page.locator('[data-window-panel="intel"] [data-panel-close="intel"]').click();
  await page.waitForFunction(() => !document.querySelector('[data-window-panel="intel"]'), null, { timeout: 10000 });
}

async function verifyQuestBoardSurface(page, viewport, label) {
  await page.locator('[data-panel-toggle="quests"]').click();
  await page.waitForSelector('[data-window-panel="quests"][data-focused="true"] .quest-board', { timeout: 10000 });
  await page.waitForSelector('[data-window-panel="quests"] .quest-row', { timeout: 10000 });

  const initial = await page.evaluate(() => {
    const navText = document.querySelector('[data-panel-toggle="quests"]')?.textContent ?? '';
    const detail = document.querySelector('[data-window-panel="quests"] [data-quest-detail]');
    return {
      navText,
      detailKey: detail instanceof HTMLElement ? detail.dataset.questDetail ?? '' : '',
      rowCount: document.querySelectorAll('[data-window-panel="quests"] .quest-row').length,
      sectionLabels: [...document.querySelectorAll('[data-window-panel="quests"] .quest-section header strong')].map((node) =>
        node.textContent?.trim(),
      ),
      hasObjective: Boolean(document.querySelector('[data-window-panel="quests"] .quest-objective-row')),
      hasReward: Boolean(document.querySelector('[data-window-panel="quests"] .quest-reward-row')),
      offers: window.__SPACE_MORPG_SMOKE_STATE__?.questBoard?.offers?.length ?? 0,
      active: window.__SPACE_MORPG_SMOKE_STATE__?.questBoard?.active?.length ?? 0,
    };
  });
  if (!/quests/i.test(initial.navText) || /galaxy/i.test(initial.navText)) {
    throw new Error(`${label}: quest nav label mismatch ${JSON.stringify(initial)}`);
  }
  if (
    initial.rowCount < 2 ||
    !['Offers', 'Active', 'Claimable', 'Completed'].every((section) => initial.sectionLabels.includes(section)) ||
    !initial.hasObjective ||
    !initial.hasReward
  ) {
    throw new Error(`${label}: quest board surface incomplete ${JSON.stringify(initial)}`);
  }

  const rowCount = await page.locator('[data-window-panel="quests"] .quest-row').count();
  if (rowCount > 1) {
    await page.locator('[data-window-panel="quests"] .quest-row').nth(1).click();
    await page.waitForFunction(
      (previousKey) => {
        const detail = document.querySelector('[data-window-panel="quests"] [data-quest-detail]');
        return detail instanceof HTMLElement && detail.dataset.questDetail && detail.dataset.questDetail !== previousKey;
      },
      initial.detailKey,
      { timeout: 10000 },
    );
  }

  const acceptBefore = await commandLogCount(page, 'Sent quest.accept.');
  await page.waitForSelector('[data-window-panel="quests"] [data-action="quest-accept"]:not([disabled])', { timeout: 10000 });
  await page.locator('[data-window-panel="quests"] [data-action="quest-accept"]').first().click();
  await page.waitForFunction(
    ({ before, activeBefore, offersBefore }) => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      const accepted = (state?.commandLog ?? []).filter((line) => line.text === 'Sent quest.accept.').length > before;
      return (
        accepted &&
        ((state?.questBoard?.active?.length ?? 0) > activeBefore || (state?.questBoard?.offers?.length ?? 0) < offersBefore)
      );
    },
    { before: acceptBefore, activeBefore: initial.active, offersBefore: initial.offers },
    { timeout: 10000 },
  );
  await page.waitForSelector('[data-window-panel="quests"] .quest-section[data-quest-section="active"] .quest-row', { timeout: 10000 });

  const actionState = await page.evaluate(() => {
    const reroll = document.querySelector('[data-window-panel="quests"] [data-action="quest-reroll"]');
    const claim = document.querySelector('[data-window-panel="quests"] [data-action="quest-claim"]');
    return {
      acceptedCommands: (window.__SPACE_MORPG_SMOKE_STATE__?.commandLog ?? []).filter((line) => line.text === 'Sent quest.accept.').length,
      hasRerollContract: reroll instanceof HTMLButtonElement,
      hasClaimContract: claim instanceof HTMLButtonElement,
      activeRows: document.querySelectorAll('[data-window-panel="quests"] .quest-section[data-quest-section="active"] .quest-row').length,
      detailKey: document.querySelector('[data-window-panel="quests"] [data-quest-detail]')?.getAttribute('data-quest-detail') ?? '',
    };
  });
  if (actionState.acceptedCommands <= acceptBefore || !actionState.hasRerollContract || !actionState.hasClaimContract || actionState.activeRows < 1) {
    throw new Error(`${label}: quest action wiring incomplete ${JSON.stringify(actionState)}`);
  }

  await page.screenshot({ path: path.join(phasePatch3OutputDir, `quests-${label}.png`), fullPage: true });
  if (viewport.width < 768) {
    const hasHorizontalScroll = await page.evaluate(() => document.scrollingElement.scrollWidth > window.innerWidth + 1);
    if (hasHorizontalScroll) {
      throw new Error(`${label}: quest board created horizontal body scroll`);
    }
  }
  await page.locator('[data-window-panel="quests"] [data-panel-close="quests"]').click();
  await page.waitForFunction(() => !document.querySelector('[data-window-panel="quests"]'), null, { timeout: 10000 });
}

async function syncRememberedMinimap(page, label) {
  await page.locator('[data-action="sync"]').click();
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return (state?.minimap?.remembered ?? []).length >= 1 && document.querySelectorAll('.minimap__memory').length >= 1;
  }, null, { timeout: 10000 });
  const leaked = await page.evaluate(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return {
      remembered: state?.minimap?.remembered ?? [],
      hiddenVisible: Boolean(state?.visibleEntities?.entity_hidden_planet_signal),
      hiddenContact: state?.minimap?.live_contacts?.some((contact) => contact.entity_id === 'entity_hidden_planet_signal') ?? false,
    };
  });
  if (leaked.hiddenVisible || leaked.hiddenContact) {
    throw new Error(`${label}: hidden planet signal leaked while syncing remembered minimap ${JSON.stringify(leaked)}`);
  }
}

async function verifyFogOfWar(page, label) {
  await page.waitForFunction(() => {
    const fog = window.__SPACE_MORPG_SMOKE_STATE__?.worldView?.fog;
    return (
      fog?.active === true &&
      Number.isFinite(fog.revealCenter?.x) &&
      Number.isFinite(fog.revealCenter?.y) &&
      fog.revealRadius > 100 &&
      fog.overlayAlpha >= 0.45 &&
      fog.rememberedPockets >= 1
    );
  }, null, { timeout: 10000 });
  const memoryCount = await page.locator('.minimap__memory').count();
  if (memoryCount < 1) {
    throw new Error(`${label}: minimap did not render remembered fog memory`);
  }
}

async function verifyQuickActionContracts(page, viewport, label) {
  await page.waitForFunction(() => document.querySelectorAll('.hud__actionbar [data-quick-action]').length === 6, null, {
    timeout: 10000,
  });
  const summary = await page.evaluate(() =>
    Array.from(document.querySelectorAll('.hud__actionbar [data-quick-action]')).map((button) => {
      const element = button instanceof HTMLButtonElement ? button : null;
      const slot = element?.closest('[data-quick-action-slot]');
      const icon = element?.querySelector('.action-button__icon');
      return {
        id: element?.dataset.quickAction ?? '',
        action: element?.dataset.action ?? '',
        state: element?.dataset.state ?? '',
        disabled: element?.disabled ?? false,
        slot: slot?.getAttribute('data-slot') ?? '',
        commandOp: slot?.getAttribute('data-command-op') ?? '',
        label: element?.querySelector('.action-button__label')?.textContent?.trim() ?? '',
        detail: element?.querySelector('small')?.textContent?.trim() ?? '',
        iconSource: icon instanceof HTMLImageElement ? icon.getAttribute('src') ?? '' : '',
      };
    }),
  );
  const expected = [
    { id: 'laser', action: 'fire', slot: '1', commandOp: 'combat.use_skill', locked: false },
    { id: 'rocket', action: 'rocket', slot: '2', commandOp: '', locked: true },
    { id: 'scan', action: 'scan', slot: '3', commandOp: 'scan.pulse', locked: false },
    { id: 'shield', action: 'shield', slot: '4', commandOp: '', locked: true },
    { id: 'warp', action: 'warp', slot: '5', commandOp: '', locked: true },
    { id: 'gather', action: 'loot', slot: '6', commandOp: 'loot.pickup', locked: false },
  ];
  for (const [index, want] of expected.entries()) {
    const got = summary[index];
    if (
      !got ||
      got.id !== want.id ||
      got.action !== want.action ||
      got.slot !== want.slot ||
      got.commandOp !== want.commandOp ||
      !/(?:\.svg(?:\?|$)|data:image\/svg\+xml)/.test(got.iconSource) ||
      (want.locked && (!got.disabled || got.state !== 'locked' || got.detail !== 'Locked')) ||
      (!want.locked && got.label.length === 0)
    ) {
      throw new Error(`${label}: quick action slot ${index + 1} contract mismatch ${JSON.stringify({ got, want, summary })}`);
    }
  }

  const sentBeforeLocked = await commandLogSentCount(page);
  await page.evaluate(() => {
    for (const id of ['rocket', 'shield', 'warp']) {
      document.querySelector(`.hud__actionbar [data-quick-action="${id}"]`)?.dispatchEvent(
        new MouseEvent('click', { bubbles: true, cancelable: true }),
      );
    }
  });
  await page.keyboard.press('2');
  await page.keyboard.press('4');
  await page.keyboard.press('5');
  await page.waitForTimeout(80);
  const sentAfterLocked = await commandLogSentCount(page);
  if (sentAfterLocked !== sentBeforeLocked) {
    throw new Error(`${label}: locked quick actions emitted command log entries`);
  }

  if (label === 'desktop') {
    await page.locator('[data-panel-toggle="cargo"]').click();
    await page.waitForSelector('[data-window-panel="cargo"][data-focused="true"]', { timeout: 10000 });
    await page.locator('[data-window-panel="cargo"]').focus();
    const sentBeforeFocus = await commandLogSentCount(page);
    await page.keyboard.press('3');
    await page.keyboard.press('1');
    await page.waitForTimeout(120);
    const sentAfterFocus = await commandLogSentCount(page);
    if (sentAfterFocus !== sentBeforeFocus) {
      throw new Error(`${label}: quick action shortcut fired while a HUD window owned focus`);
    }
    await page.locator('[data-window-panel="cargo"] [data-panel-close="cargo"]').click();
    await page.waitForFunction(() => !document.querySelector('[data-window-panel="cargo"]'), null, { timeout: 10000 });
  }

  await page.screenshot({ path: path.join(phase03OutputDir, `quick-actions-${label}.png`), fullPage: true });
  if (viewport.width < 768) {
    const hasHorizontalScroll = await page.evaluate(() => document.scrollingElement.scrollWidth > window.innerWidth + 1);
    if (hasHorizontalScroll) {
      throw new Error(`${label}: quick action bar created horizontal body scroll`);
    }
  }
}

async function verifyInventoryLoadout(page, viewport, label) {
  await page.locator('[data-panel-toggle="cargo"]').click();
  await page.waitForSelector('[data-window-panel="cargo"][data-focused="true"] .loadout-console', { timeout: 10000 });
  await page.waitForSelector('[data-loadout-slot-id="offensive_1"]', { timeout: 10000 });
  await page.waitForSelector('[data-module-slot-type="offensive"]', { timeout: 10000 });

  const initial = await page.evaluate(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const laser = state?.inventory?.instances?.find((item) => item.item_id === 'laser_alpha_t1');
    return {
      laserID: laser?.item_instance_id ?? '',
      laserLocation: laser?.location ?? '',
      equipped: state?.loadout?.slots?.find((slot) => slot.slot_id === 'offensive_1')?.item_instance_id ?? '',
      slotCount: document.querySelectorAll('[data-loadout-slot-id]').length,
      moduleCards: document.querySelectorAll('[data-module-instance-id]').length,
      moduleDetail: document.querySelector('[data-window-panel="cargo"] [data-module-detail]')?.getAttribute('data-module-detail') ?? '',
      moduleSelectButtons: document.querySelectorAll('[data-window-panel="cargo"] [data-action="module-select"]').length,
    };
  });
  if (
    !initial.laserID ||
    initial.laserLocation !== 'account_inventory' ||
    initial.equipped ||
    initial.slotCount < 3 ||
    initial.moduleCards < 3 ||
    !initial.moduleDetail ||
    initial.moduleSelectButtons < 1
  ) {
    throw new Error(`${label}: inventory loadout initial state mismatch ${JSON.stringify(initial)}`);
  }
  await page.locator(`[data-window-panel="cargo"] [data-action="module-select"][data-module-instance-id="${initial.laserID}"]`).click();
  await page.waitForFunction(
    (laserID) => document.querySelector('[data-window-panel="cargo"] [data-module-detail]')?.getAttribute('data-module-detail') === laserID,
    initial.laserID,
    { timeout: 10000 },
  );

  await dispatchLoadoutDrop(page, `[data-module-instance-id="${initial.laserID}"]`, '[data-loadout-slot-id="offensive_1"]');
  await page.waitForFunction(
    (laserID) => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      return (
        state?.loadout?.slots?.some((slot) => slot.slot_id === 'offensive_1' && slot.item_instance_id === laserID) &&
        state?.inventory?.instances?.some((item) => item.item_instance_id === laserID && item.location === 'ship_equipped') &&
        (state?.commandLog ?? []).some((line) => line.text === 'Sent loadout.equip_module.')
      );
    },
    initial.laserID,
    { timeout: 10000 },
  );

  await dispatchLoadoutDrop(
    page,
    `[data-equipped-slot-id="offensive_1"][data-module-instance-id="${initial.laserID}"]`,
    '.module-bay[data-loadout-inventory-drop="true"]',
  );
  await page.waitForFunction(
    (laserID) => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      return (
        state?.loadout?.slots?.some((slot) => slot.slot_id === 'offensive_1' && !slot.item_instance_id) &&
        state?.inventory?.instances?.some((item) => item.item_instance_id === laserID && item.location === 'account_inventory') &&
        (state?.commandLog ?? []).some((line) => line.text === 'Sent loadout.unequip_module.')
      );
    },
    initial.laserID,
    { timeout: 10000 },
  );

  await page.screenshot({ path: path.join(outputDir, `inventory-loadout-${label}.png`), fullPage: true });
  if (viewport.width < 768) {
    const hasHorizontalScroll = await page.evaluate(() => document.scrollingElement.scrollWidth > window.innerWidth + 1);
    if (hasHorizontalScroll) {
      throw new Error(`${label}: inventory loadout created horizontal body scroll`);
    }
  }
  await page.locator('[data-window-panel="cargo"] [data-panel-close="cargo"]').click();
  await page.waitForFunction(() => !document.querySelector('[data-window-panel="cargo"]'), null, { timeout: 10000 });
}

async function verifyHangarSurface(page, viewport, label) {
  await page.locator('[data-panel-toggle="systems"]').click();
  await page.waitForSelector('[data-window-panel="systems"][data-focused="true"] .hangar-console', { timeout: 10000 });
  await page.waitForSelector('.hangar-row[data-active="true"]', { timeout: 10000 });
  await page.waitForSelector('.hangar-detail .hangar-preview', { timeout: 10000 });

  const summary = await page.evaluate(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const active = state?.hangar?.ships?.find((ship) => ship.ship_id === state?.hangar?.active_ship_id);
    const activateButton = document.querySelector('[data-window-panel="systems"] [data-action="hangar-activate"]');
    return {
      activeShipID: state?.hangar?.active_ship_id ?? '',
      rowCount: state?.hangar?.ships?.length ?? 0,
      activeRow: active
        ? {
            shipID: active.ship_id,
            active: active.active,
            cargo: active.cargo_capacity,
            speed: active.speed,
            radar: active.radar,
            utility: active.slot_utility,
          }
        : null,
      activateDisabled: activateButton instanceof HTMLButtonElement ? activateButton.disabled : null,
      commandSent: (state?.commandLog ?? []).some((line) => line.text === 'Sent hangar.activate_ship.'),
      selectedRow: document.querySelector('.hangar-row[data-selected="true"]')?.getAttribute('data-ship-id') ?? '',
      detailTitle: document.querySelector('.hangar-detail .hangar-preview strong')?.textContent?.trim() ?? '',
    };
  });
  if (
    summary.activeShipID !== 'starter' ||
    summary.rowCount < 1 ||
    summary.activeRow?.shipID !== 'starter' ||
    summary.activeRow.active !== true ||
    summary.activeRow.cargo <= 0 ||
    summary.activeRow.utility !== 1 ||
    summary.activateDisabled !== true ||
    summary.commandSent ||
    summary.selectedRow !== 'starter' ||
    !summary.detailTitle
  ) {
    throw new Error(`${label}: hangar surface mismatch ${JSON.stringify(summary)}`);
  }

  await page.screenshot({ path: path.join(phasePatch3OutputDir, `hangar-${label}.png`), fullPage: true });
  if (viewport.width < 768) {
    const hasHorizontalScroll = await page.evaluate(() => document.scrollingElement.scrollWidth > window.innerWidth + 1);
    if (hasHorizontalScroll) {
      throw new Error(`${label}: hangar surface created horizontal body scroll`);
    }
  }
  await page.locator('[data-window-panel="systems"] [data-panel-close="systems"]').click();
  await page.waitForFunction(() => !document.querySelector('[data-window-panel="systems"]'), null, { timeout: 10000 });
}

async function dispatchLoadoutDrop(page, sourceSelector, targetSelector) {
  await page.evaluate(
    ({ sourceSelector, targetSelector }) => {
      const source = document.querySelector(sourceSelector);
      const target = document.querySelector(targetSelector);
      if (!source || !target) {
        throw new Error(`Missing drag source or target: ${sourceSelector} -> ${targetSelector}`);
      }
      const dataTransfer = new DataTransfer();
      source.dispatchEvent(new DragEvent('dragstart', { bubbles: true, cancelable: true, dataTransfer }));
      target.dispatchEvent(new DragEvent('dragover', { bubbles: true, cancelable: true, dataTransfer }));
      target.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer }));
      source.dispatchEvent(new DragEvent('dragend', { bubbles: true, cancelable: true, dataTransfer }));
    },
    { sourceSelector, targetSelector },
  );
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
        const opsText = document.querySelector('[data-panel="ops"]')?.textContent ?? '';
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
      const entityTypes = new Set(Object.values(state?.visibleEntities ?? {}).map((entity) => entity.entity_type));
      const minimapTypes = new Set(
        Array.from(document.querySelectorAll('.minimap__point')).map((point) => point.getAttribute('data-entity-type')),
      );
      return (
        state?.playerSnapshot?.callsign === 'Server-Pilot' &&
        state?.cargo?.capacity === 80 &&
        state?.wallet?.credits === 1250 &&
        state?.stats?.radar_range === 420 &&
        state?.stats?.loot_pickup_range === 120 &&
        state?.stats?.basic_laser_energy_cost === 10 &&
        entityTypes.has('player') &&
        entityTypes.has('npc') &&
        entityTypes.has('loot') &&
        entityTypes.has('planet_signal') &&
        minimapTypes.has('player') &&
        minimapTypes.has('npc') &&
        minimapTypes.has('loot') &&
        minimapTypes.has('planet_signal') &&
        state?.commandLog?.some((line) => line.text === 'Forbidden server payload rejected.') &&
        !state?.visibleEntities?.['hidden-planet']
      );
    });

    if (label === 'fixture-desktop') {
      await clickWorldPosition(page, { x: 150, y: -250 });
      await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'npc-rake-01');
      await page.waitForSelector('[data-panel="target"] .target-lock[data-target-kind="npc"]', { timeout: 10000 });
      await page.locator('.hud__actionbar [data-action="fire"]').click();
      await realtime.waitForOp('combat.use_skill');
      await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.playerSnapshot?.energy === 58);

      await clickWorldPosition(page, { x: -110, y: -220 });
      await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'loot-scrap-01');
      await page.waitForSelector('[data-panel="target"] .target-lock[data-target-kind="loot"]', { timeout: 10000 });
      await realtime.waitForOp('move_to');
      await realtime.waitForOp('loot.pickup');
      await page.waitForFunction(() => {
        const state = window.__SPACE_MORPG_SMOKE_STATE__;
        return state?.cargo?.used === 21 && state?.cargo?.items?.some((item) => item.item_id === 'raw_ore' && item.quantity === 15);
      });

      await clickWorldPosition(page, { x: 260, y: 150 });
      await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'signal-eris-04');
      await page.waitForSelector('[data-panel="target"] .target-lock[data-target-kind="planet_signal"]', { timeout: 10000 });

      await clickWorldPosition(page, { x: 40, y: -220 });
      await realtime.waitForOp('move_to');
      await page.waitForFunction(() => {
        const player = window.__SPACE_MORPG_SMOKE_STATE__?.visibleEntities?.['player-local'];
        return Math.abs((player?.position?.x ?? 9999) - 40) <= 1 && Math.abs((player?.position?.y ?? 9999) + 220) <= 1;
      });

      await clickWorldPosition(page, { x: 150, y: -250 });
      await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'npc-rake-01');
    }

    if (label === 'fixture-desktop') {
      await verifyFixtureScanLoop(page, realtime, label);
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

async function verifyScanModeAutomation(page, viewport, label) {
  await page.waitForFunction(() => {
    const button = document.querySelector('.hud__actionbar [data-quick-action="scan"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });

  const sentBefore = await commandLogCount(page, 'Sent scan.pulse.');
  await page.locator('.hud__actionbar [data-quick-action="scan"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const button = document.querySelector('.hud__actionbar [data-quick-action="scan"]');
    return (
      state?.scanMode?.enabled === true &&
      button instanceof HTMLButtonElement &&
      button.dataset.state === 'scanning' &&
      /scanning/i.test(button.textContent ?? '')
    );
  }, null, { timeout: 10000 });
  await page.waitForFunction(() => {
    const waves = window.__SPACE_MORPG_SMOKE_STATE__?.worldView?.scanWaves;
    return (
      waves?.active === true &&
      Number.isFinite(waves.screen?.x) &&
      Number.isFinite(waves.screen?.y) &&
      Array.isArray(waves.rings) &&
      waves.rings.length >= 3 &&
      waves.rings.every((ring) => ring.radius > 40 && ring.alpha > 0)
    );
  }, null, { timeout: 5000 });
  await page.screenshot({ path: path.join(phase04OutputDir, `scan-mode-${label}.png`), fullPage: true });
  await page.screenshot({ path: path.join(phase08OutputDir, `scan-mode-${label}.png`), fullPage: true });

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
  const sentAfter = await commandLogCount(page, 'Sent scan.pulse.');
  if (sentAfter <= sentBefore) {
    throw new Error(`${label}: scan mode did not send a real scan.pulse command`);
  }

  await page.locator('.hud__actionbar [data-quick-action="scan"]').dispatchEvent('click');
  await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.scanMode?.enabled === false, null, {
    timeout: 10000,
  });
  if (viewport.width < 768) {
    const hasHorizontalScroll = await page.evaluate(() => document.scrollingElement.scrollWidth > window.innerWidth + 1);
    if (hasHorizontalScroll) {
      throw new Error(`${label}: scan mode action state created horizontal body scroll`);
    }
  }
}

async function verifyFixtureScanLoop(page, realtime, label) {
  await page.waitForFunction(() => {
    const button = document.querySelector('.hud__actionbar [data-quick-action="scan"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('.hud__actionbar [data-quick-action="scan"]').dispatchEvent('click');
  await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.scanMode?.enabled === true, null, { timeout: 10000 });
  await realtime.waitForOpCount('scan.pulse', 2);
  await page.locator('.hud__actionbar [data-quick-action="scan"]').dispatchEvent('click');
  await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.scanMode?.enabled === false, null, { timeout: 10000 });
}

async function verifyRealCombatLoot(page) {
  await clickWorldPosition(page, { x: 80, y: 0 });
  await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.selectedTargetID === 'entity_training_npc', null, {
    timeout: 10000,
  });
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const target = state?.visibleEntities?.[state.selectedTargetID];
    const targetPanelText = document.querySelector('[data-panel="target"]')?.textContent ?? '';
    return (
      target?.entity_type === 'npc' &&
      target?.combat?.hp > 0 &&
      target?.combat?.max_hp >= target.combat.hp &&
      /Hull/i.test(targetPanelText) &&
      /Range/i.test(targetPanelText)
    );
  }, null, { timeout: 10000 });
  await page.locator('.hud__actionbar [data-action="fire"]').click();
  try {
    await page.waitForFunction(() => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      const effects = state?.worldEffects ?? [];
      const projectile = state?.worldView?.projectiles?.find((entry) => entry.active === true && entry.progress > 0 && entry.progress < 1);
      return (
        effects.some((effect) => effect.kind === 'laser' && effect.targetID === 'entity_training_npc' && effect.sourceID) &&
        projectile &&
        Number.isFinite(projectile.source?.x) &&
        Number.isFinite(projectile.target?.x) &&
        Number.isFinite(projectile.head?.x) &&
        Math.hypot(projectile.head.x - projectile.source.x, projectile.head.y - projectile.source.y) > 4 &&
        Math.hypot(projectile.target.x - projectile.head.x, projectile.target.y - projectile.head.y) > 4
      );
    }, null, { timeout: 3000 });
  } catch (error) {
    console.error('desktop projectile state', JSON.stringify(await projectileDiagnostics(page), null, 2));
    throw error;
  }
  await page.screenshot({ path: path.join(phase06OutputDir, 'projectile-desktop.png'), fullPage: true });
  await page.screenshot({ path: path.join(phase08OutputDir, 'projectile-desktop.png'), fullPage: true });
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const entities = state?.visibleEntities ?? {};
    const lootDrop = Object.values(entities).find((entity) => entity.entity_type === 'loot');
    const effects = state?.worldEffects ?? [];
    const knownDrops = Object.values(state?.knownLoot ?? {});
    return (
      !entities.entity_training_npc &&
      Boolean(lootDrop) &&
      effects.some((effect) => effect.kind === 'damage' && effect.targetID === 'entity_training_npc') &&
      effects.some((effect) => effect.kind === 'loot_spawn') &&
      knownDrops.some((drop) => drop.item_id === 'raw_ore' && drop.quantity === 3) &&
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
  try {
    await page.waitForFunction(() => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      const entities = state?.visibleEntities ?? {};
      const effects = state?.worldEffects ?? [];
      return (
        state?.cargo?.used === 6 &&
        state?.cargo?.items?.some((item) => item.item_id === 'raw_ore' && item.quantity === 3) &&
        state?.inventory?.stackable?.some((item) => item.item_id === 'raw_ore' && item.quantity === 3 && item.location === 'ship_cargo') &&
        effects.some((effect) => effect.kind === 'loot_pickup' && effect.itemID === 'raw_ore' && effect.quantity === 3) &&
        !Object.values(entities).some((entity) => entity.entity_type === 'loot')
      );
    }, null, { timeout: 10000 });
  } catch (error) {
    console.error('desktop loot pickup state', JSON.stringify(await lootDiagnostics(page, dropPosition), null, 2));
    throw error;
  }
}

async function lootDiagnostics(page, expectedDropPosition) {
  return page.evaluate((dropPosition) => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const entities = state?.visibleEntities ?? {};
    const self = Object.values(entities).find((entity) => entity.status_flags?.includes('self'));
    const loot = Object.values(entities).find((entity) => entity.entity_type === 'loot');
    return {
      expectedDropPosition: dropPosition,
      selectedTargetID: state?.selectedTargetID,
      self,
      loot,
      cargo: state?.cargo,
      inventory: state?.inventory,
      effects: state?.worldEffects,
      recentCommandLog: state?.commandLog?.slice(-10),
      worldView: state?.worldView,
      activeElement: document.activeElement?.outerHTML?.slice(0, 220),
      windows: Array.from(document.querySelectorAll('[data-window-panel]')).map((element) =>
        element instanceof HTMLElement ? { id: element.dataset.windowPanel, focused: element.dataset.focused, display: getComputedStyle(element).display } : null,
      ),
    };
  }, expectedDropPosition);
}

async function projectileDiagnostics(page) {
  return page.evaluate(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return {
      selectedTargetID: state?.selectedTargetID,
      target: state?.selectedTargetID ? state?.visibleEntities?.[state.selectedTargetID] : null,
      effects: state?.worldEffects,
      projectiles: state?.worldView?.projectiles,
      displayPositions: state?.worldView?.displayPositions,
      recentCommandLog: state?.commandLog?.slice(-10),
      recentCombatLog: state?.combatLog?.slice(-10),
    };
  });
}

async function verifyStarfieldBackground(page, label) {
  await page.waitForFunction(
    () => {
      const background = window.__SPACE_MORPG_SMOKE_STATE__?.worldView?.background;
      return background?.assetLoaded === true && background.tileCount >= 90 && background.mirroredTiles > 0;
    },
    null,
    { timeout: 10000 },
  );
  const background = await backgroundDebug(page);
  if (!background.assetLoaded) {
    throw new Error(`${label} starfield asset was not loaded`);
  }
  if (background.tileCount < 90) {
    throw new Error(`${label} expected mirrored starfield tile grid, got ${background.tileCount}`);
  }
  if (background.mirroredTiles <= 0) {
    throw new Error(`${label} starfield did not report mirrored tiles`);
  }
  if (!isFiniteVec(background.farOffset) || !isFiniteVec(background.midOffset)) {
    throw new Error(`${label} starfield offsets were invalid: ${JSON.stringify(background)}`);
  }
  if ((background.sampleTiles ?? []).length < 2) {
    throw new Error(`${label} starfield sample tiles missing both parallax layers: ${JSON.stringify(background)}`);
  }
}

async function backgroundDebug(page) {
  return page.evaluate(() => window.__SPACE_MORPG_SMOKE_STATE__?.worldView?.background ?? null);
}

async function verifyRealMovementInterpolation(page) {
  const initialBackground = await backgroundDebug(page);
  const initial = await selfMovementSample(page);
  const firstTarget = await movementTargetAwayFromMemory(page, initial.entity.position, [
    { x: 320, y: -220 },
    { x: -320, y: -220 },
    { x: 260, y: 260 },
    { x: -260, y: 260 },
  ]);
  await clickWorldPosition(page, firstTarget);
  try {
    await page.waitForFunction(
      (target) => {
        const self = Object.values(window.__SPACE_MORPG_SMOKE_STATE__?.visibleEntities ?? {}).find((entity) =>
          entity.status_flags?.includes('self'),
        );
        return (
          self?.movement?.moving === true &&
          Math.hypot(self.movement.target.x - target.x, self.movement.target.y - target.y) < 16
        );
      },
      firstTarget,
      { timeout: 10000 },
    );
  } catch (error) {
    console.error('desktop first movement state', JSON.stringify(await movementDiagnostics(page, firstTarget), null, 2));
    throw error;
  }
  await page.waitForFunction(() => document.querySelector('[data-movement-eta-active="true"]'), null, { timeout: 3000 });
  await page.waitForFunction(
    () =>
      window.__SPACE_MORPG_SMOKE_STATE__?.movementEta?.active === true &&
      window.__SPACE_MORPG_SMOKE_STATE__?.commandLog?.some((line) => /^Move -?\d+,-?\d+ -> -?\d+,-?\d+, \d+u, eta/.test(line.text)) &&
      window.__SPACE_MORPG_SMOKE_STATE__?.commandLog?.some((line) => /^Route -?\d+,-?\d+ -> -?\d+,-?\d+, \d+u, eta/.test(line.text)),
    null,
    { timeout: 3000 },
  );
  const afterFirstBackground = await backgroundDebug(page);
  const parallaxDelta = backgroundOffsetDelta(initialBackground, afterFirstBackground);
  if (parallaxDelta <= 0.15) {
    throw new Error(
      `desktop starfield parallax offset did not change during movement: ${JSON.stringify({
        before: initialBackground,
        after: afterFirstBackground,
        delta: parallaxDelta,
      })}`,
    );
  }
  const etaBefore = await movementEtaSample(page);
  await page.waitForTimeout(180);
  const etaAfter = await movementEtaSample(page);
  if (!etaBefore.active || !etaAfter.active || etaAfter.remainingMs >= etaBefore.remainingMs) {
    throw new Error(`desktop movement ETA did not count down: ${JSON.stringify({ before: etaBefore, after: etaAfter })}`);
  }
  await page.waitForTimeout(80);
  const first = await selfMovementSample(page);
  const firstDisplayDistance = distance(first.display, firstTarget);
  if (firstDisplayDistance < 20) {
    throw new Error(
      `desktop movement display jumped to first target ${JSON.stringify({
        firstDisplayDistance,
        firstTarget,
        sample: first,
        etaBefore,
        etaAfter,
        diagnostics: await movementDiagnostics(page, firstTarget),
      })}`,
    );
  }
  if (distance(first.display, first.entity.movement.origin) <= 0.5) {
    throw new Error('desktop movement display did not advance from server route origin');
  }
  await page.screenshot({ path: path.join(phase05OutputDir, 'movement-eta-desktop.png'), fullPage: true });
  await page.screenshot({ path: path.join(phase08OutputDir, 'movement-eta-desktop.png'), fullPage: true });

  await page.waitForTimeout(90);
  const beforeSecond = await selfMovementSample(page);
  const secondTarget = await movementTargetAwayFromMemory(page, beforeSecond.entity.position, [
    { x: -320, y: 220 },
    { x: 320, y: 220 },
    { x: -260, y: -260 },
    { x: 260, y: -260 },
  ]);
  await clickWorldPosition(page, secondTarget);
  try {
    await page.waitForFunction(
      ({ firstTarget: previousTarget, target: expectedTarget }) => {
        const self = Object.values(window.__SPACE_MORPG_SMOKE_STATE__?.visibleEntities ?? {}).find((entity) =>
          entity.status_flags?.includes('self'),
        );
        const target = self?.movement?.target;
        return (
          self?.movement?.moving === true &&
          target &&
          Math.hypot(target.x - expectedTarget.x, target.y - expectedTarget.y) < 16 &&
          Math.hypot(target.x - previousTarget.x, target.y - previousTarget.y) > 20
        );
      },
      { firstTarget, target: secondTarget },
      { timeout: 10000 },
    );
  } catch (error) {
    console.error('desktop movement reclick state', JSON.stringify(await movementDiagnostics(page, secondTarget), null, 2));
    throw error;
  }
  const second = await selfMovementSample(page);
  if (distance(second.entity.movement.origin, first.entity.movement.origin) <= 1) {
    throw new Error(
      `desktop reclick movement reused old origin ${JSON.stringify(second.entity.movement.origin)} from ${JSON.stringify(
        first.entity.movement.origin,
      )}`,
    );
  }
  if (distance(second.entity.movement.target, secondTarget) > 16) {
    throw new Error(`desktop reclick movement target = ${JSON.stringify(second.entity.movement.target)}, want near ${JSON.stringify(secondTarget)}`);
  }
  if (distance(second.entity.movement.origin, beforeSecond.entity.position) > 25) {
    throw new Error(
      `desktop reclick origin ${JSON.stringify(second.entity.movement.origin)} not near server in-flight position ${JSON.stringify(
        beforeSecond.entity.position,
      )}`,
    );
  }
  const moveIntentCount = await commandLogMatchCount(page, '^Move -?\\d+,-?\\d+ -> -?\\d+,-?\\d+, \\d+u, eta');
  if (moveIntentCount < 2) {
    throw new Error(`desktop movement expected two debug move logs, got ${moveIntentCount}`);
  }

  const spamOne = await movementTargetAwayFromMemory(page, second.entity.position, [{ x: 120, y: -120 }]);
  const spamTwo = await movementTargetAwayFromMemory(page, second.entity.position, [{ x: -140, y: 110 }]);
  const rejectionBefore = await commandLogMatchCount(page, '^Move rejected: Movement intent rate limit exceeded\\.');
  await clickWorldPosition(page, spamOne);
  await page.waitForFunction(
    ({ before, target }) => {
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      const self = Object.values(state?.visibleEntities ?? {}).find((entity) => entity.status_flags?.includes('self'));
      const rejections = (state?.commandLog ?? []).filter((line) => /^Move rejected: Movement intent rate limit exceeded\./.test(line.text)).length;
      const accepted = self?.movement?.target && Math.hypot(self.movement.target.x - target.x, self.movement.target.y - target.y) < 16;
      return rejections > before || accepted;
    },
    { before: rejectionBefore, target: spamOne },
    { timeout: 5000 },
  );
  const rejectionAfterFirst = await commandLogMatchCount(page, '^Move rejected: Movement intent rate limit exceeded\\.');
  if (rejectionAfterFirst === rejectionBefore) {
    await clickWorldPositionsRapid(page, [spamTwo, { x: spamTwo.x + 40, y: spamTwo.y - 30 }, { x: spamTwo.x - 35, y: spamTwo.y + 45 }]);
    await page.waitForFunction(
      (before) =>
        (window.__SPACE_MORPG_SMOKE_STATE__?.commandLog ?? []).filter((line) =>
          /^Move rejected: Movement intent rate limit exceeded\./.test(line.text),
        ).length > before,
      rejectionAfterFirst,
      { timeout: 5000 },
    );
  }
  const afterRejected = await selfMovementSample(page);
  if (
    afterRejected.entity.movement?.moving !== true ||
    !Number.isFinite(afterRejected.entity.movement.target?.x) ||
    !Number.isFinite(afterRejected.entity.movement.target?.y)
  ) {
    throw new Error(`desktop movement spam corrupted route state: ${JSON.stringify(afterRejected.entity.movement)}`);
  }
  await page.waitForFunction(
    () => window.__SPACE_MORPG_SMOKE_STATE__?.movementEta?.active === false && !document.querySelector('[data-movement-eta-active="true"]'),
    null,
    { timeout: 10000 },
  );
  await verifyHudInputIsolationDuringMovement(page);
}

async function verifyHudInputIsolationDuringMovement(page) {
  await page.waitForTimeout(1200);
  const initial = await selfMovementSample(page);
  const target = await movementTargetAwayFromMemory(page, initial.entity.position, [
    { x: 340, y: -210 },
    { x: -340, y: -210 },
    { x: 300, y: 240 },
    { x: -300, y: 240 },
  ]);
  await clickWorldPosition(page, target);
  await page.waitForFunction(() => window.__SPACE_MORPG_SMOKE_STATE__?.movementEta?.active === true, null, { timeout: 3000 });
  const moveBefore = await commandLogCount(page, 'Sent move_to.');
  const skillBefore = await commandLogCount(page, 'Sent combat.use_skill.');

  for (const panel of ['cargo', 'systems', 'quests', 'intel', 'economy']) {
    await page.locator(`[data-panel-toggle="${panel}"]`).click();
    await page.waitForSelector(`[data-window-panel="${panel}"][data-focused="true"]`, { timeout: 10000 });
  }

  await page.locator('[data-window-panel="economy"]').focus();
  await page.locator('[data-window-panel="economy"] .hud-window__body').click({ position: { x: 20, y: 20 } });
  await page.keyboard.press('1');
  await page.waitForTimeout(80);
  const skillAfterWindowKey = await commandLogCount(page, 'Sent combat.use_skill.');
  if (skillAfterWindowKey !== skillBefore) {
    throw new Error('desktop movement HUD window focus allowed a quick action command');
  }

  const beforeDrag = await windowRect(page, 'economy');
  const header = await page.locator('[data-window-panel="economy"] .hud-window__header').boundingBox();
  if (!beforeDrag || !header) {
    throw new Error('desktop movement economy window was not draggable');
  }
  await page.mouse.move(header.x + header.width / 2, header.y + header.height / 2);
  await page.mouse.down();
  await page.mouse.move(header.x + header.width / 2 + 72, header.y + header.height / 2 + 36, { steps: 5 });
  await page.mouse.up();
  const afterDrag = await windowRect(page, 'economy');
  if (!afterDrag || Math.hypot(afterDrag.left - beforeDrag.left, afterDrag.top - beforeDrag.top) < 25) {
    throw new Error('desktop movement economy window did not drag while moving');
  }

  await page.locator('[data-window-panel="economy"] [data-modal-open="economy"]').click();
  await page.waitForSelector('[data-modal="economy"][role="dialog"]', { timeout: 10000 });
  await page.locator('[data-modal="economy"] .hud-modal__body').click({ position: { x: 24, y: 24 } });
  await page.keyboard.press('1');
  await page.waitForTimeout(80);
  const skillAfterModalKey = await commandLogCount(page, 'Sent combat.use_skill.');
  if (skillAfterModalKey !== skillBefore) {
    throw new Error('desktop movement modal focus allowed a quick action command');
  }

  const modalBefore = await page.locator('[data-modal="economy"]').boundingBox();
  const modalHeader = await page.locator('[data-modal="economy"] .hud-modal__header').boundingBox();
  if (!modalBefore || !modalHeader) {
    throw new Error('desktop movement economy modal was not draggable');
  }
  await page.mouse.move(modalHeader.x + modalHeader.width / 2, modalHeader.y + modalHeader.height / 2);
  await page.mouse.down();
  await page.mouse.move(modalHeader.x + modalHeader.width / 2 - 64, modalHeader.y + modalHeader.height / 2 + 42, { steps: 5 });
  await page.mouse.up();
  const modalAfter = await page.locator('[data-modal="economy"]').boundingBox();
  if (!modalAfter || Math.hypot(modalAfter.x - modalBefore.x, modalAfter.y - modalBefore.y) < 25) {
    throw new Error('desktop movement economy modal did not drag while moving');
  }

  await page.locator('[data-modal-close="button"]').click();
  await page.waitForFunction(() => !document.querySelector('[data-modal]'), null, { timeout: 10000 });
  for (const panel of ['economy', 'intel', 'quests', 'systems', 'cargo']) {
    await page.locator(`[data-window-panel="${panel}"] [data-panel-close="${panel}"]`).click();
  }
  await page.waitForFunction(() => !document.querySelector('[data-window-panel]'), null, { timeout: 10000 });

  const moveAfter = await commandLogCount(page, 'Sent move_to.');
  if (moveAfter !== moveBefore) {
    throw new Error('desktop movement HUD/modal click leaked into move_to');
  }
}

function backgroundOffsetDelta(before, after) {
  if (!before || !after) {
    return 0;
  }
  return (
    distance(before.farOffset ?? { x: 0, y: 0 }, after.farOffset ?? { x: 0, y: 0 }) +
    distance(before.midOffset ?? { x: 0, y: 0 }, after.midOffset ?? { x: 0, y: 0 })
  );
}

async function movementTargetAwayFromMemory(page, origin, offsets) {
  return page.evaluate(
    ({ start, candidates }) => {
      const markers = window.__SPACE_MORPG_SMOKE_STATE__?.worldView?.memoryMarkers ?? [];
      const targets = candidates.map((offset) => ({ x: Math.round(start.x + offset.x), y: Math.round(start.y + offset.y) }));
      return (
        targets.find((target) =>
          markers.every((marker) => Math.hypot((marker.position?.x ?? 0) - target.x, (marker.position?.y ?? 0) - target.y) > 90),
        ) ?? targets[0]
      );
    },
    { start: origin, candidates: offsets },
  );
}

async function movementDiagnostics(page, expectedTarget) {
  return page.evaluate((target) => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const self = Object.values(state?.visibleEntities ?? {}).find((entity) => entity.status_flags?.includes('self'));
    return {
      expectedTarget: target,
      self,
      movementTarget: state?.movementTarget,
      movementEta: state?.movementEta,
      worldView: state?.worldView,
      recentCommandLog: state?.commandLog?.slice(-8),
    };
  }, expectedTarget);
}

async function verifyRealEconomy(page, viewport, label) {
  await page.locator('[data-panel-toggle="economy"]').click();
  await page.waitForSelector('[data-window-panel="economy"][data-focused="true"] .shop-console', { timeout: 10000 });
  await page.waitForFunction(() => {
    const labels = [...document.querySelectorAll('[data-window-panel="economy"] .shop-category span')].map((node) =>
      node.textContent?.trim(),
    );
    const detail = document.querySelector('[data-window-panel="economy"] [data-shop-detail]');
    return (
      ['Market', 'Sell', 'Auction', 'Premium'].every((label) => labels.includes(label)) &&
      detail instanceof HTMLElement &&
      document.querySelector('[data-window-panel="economy"] [data-shop-quantity]') &&
      document.querySelector('[data-window-panel="economy"] [data-action="market-buy"]')
    );
  }, null, { timeout: 10000 });
  await page.screenshot({ path: path.join(phasePatch3OutputDir, `shop-${label}.png`), fullPage: true });
  if (viewport.width < 768) {
    const hasHorizontalScroll = await page.evaluate(() => document.scrollingElement.scrollWidth > window.innerWidth + 1);
    if (hasHorizontalScroll) {
      throw new Error(`${label}: shop catalog created horizontal body scroll`);
    }
  }

  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const button = document.querySelector('[data-window-panel="economy"] [data-action="market-buy"]');
    const activeListing = state?.market?.listings?.find((listing) => listing.status === 'active' && !listing.owned_by_you);
    return activeListing?.remaining_quantity > 0 && button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-window-panel="economy"] [data-action="market-buy"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return (
      state?.wallet?.credits === 1175 &&
      state?.inventory?.stackable?.some((item) => item.item_id === 'raw_ore' && item.quantity === 1 && item.location === 'account_inventory')
    );
  }, null, { timeout: 10000 });

  await page.locator('[data-window-panel="economy"] [data-shop-category="sell"]').click();
  await page.waitForFunction(() => {
    const button = document.querySelector('[data-window-panel="economy"] [data-action="market-create"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-window-panel="economy"] [data-action="market-create"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return (
      state?.market?.listings?.some((listing) => listing.owned_by_you && listing.status === 'active') &&
      !state?.inventory?.stackable?.some((item) => item.item_id === 'raw_ore' && item.location === 'account_inventory')
    );
  }, null, { timeout: 10000 });

  await page.waitForFunction(() => {
    const button = document.querySelector('[data-window-panel="economy"] [data-action="market-cancel"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-window-panel="economy"] [data-action="market-cancel"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return (
      !state?.market?.listings?.some((listing) => listing.owned_by_you && listing.status === 'active') &&
      state?.inventory?.stackable?.some((item) => item.item_id === 'raw_ore' && item.quantity === 1 && item.location === 'account_inventory')
    );
  }, null, { timeout: 10000 });

  await page.locator('[data-window-panel="economy"] [data-shop-category="auction"]').click();
  await page.waitForFunction(() => {
    const button = document.querySelector('[data-window-panel="economy"] [data-action="auction-bid"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-window-panel="economy"] [data-action="auction-bid"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return state?.auction?.lots?.[0]?.leading === true && state?.wallet?.credits < 1175;
  }, null, { timeout: 10000 });

  await page.locator('[data-window-panel="economy"] [data-shop-category="premium"]').click();
  await page.locator('[data-window-panel="economy"] [data-shop-kind="premium_entitlement"]').first().click();
  await page.waitForFunction(() => {
    const button = document.querySelector('[data-window-panel="economy"] [data-action="premium-claim"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-window-panel="economy"] [data-action="premium-claim"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return state?.wallet?.premium_earned === 50 && state?.premium?.entitlements?.[0]?.state === 'claimed';
  }, null, { timeout: 10000 });

  await page.locator('[data-window-panel="economy"] [data-shop-kind="premium_stock"]').first().click();
  await page.waitForFunction(() => {
    const button = document.querySelector('[data-window-panel="economy"] [data-action="premium-weekly-xcore"]');
    return button instanceof HTMLButtonElement && !button.disabled;
  }, null, { timeout: 10000 });
  await page.locator('[data-window-panel="economy"] [data-action="premium-weekly-xcore"]').dispatchEvent('click');
  await page.waitForFunction(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    return state?.wallet?.premium_paid === 200 && state?.premium?.purchases?.length === 1;
  }, null, { timeout: 10000 });
  await page.locator('[data-window-panel="economy"] [data-shop-category="market"]').click();
  await page.locator('[data-window-panel="economy"] [data-panel-close="economy"]').click();
  await page.waitForFunction(() => !document.querySelector('[data-window-panel="economy"]'), null, { timeout: 10000 });
}

async function verifyPanelModalChrome(page, viewport, label) {
  const featurePanels = ['cargo', 'systems', 'quests', 'intel', 'economy'];
  for (const panel of featurePanels) {
    await page.locator(`[data-panel-toggle="${panel}"]`).click();
    await page.waitForSelector(`[data-window-panel="${panel}"][data-open="true"][data-focused="true"]`, { timeout: 10000 });
  }
  await page.waitForSelector('[data-window-panel="economy"][data-open="true"][data-focused="true"]', { timeout: 10000 });

  const opened = await page.evaluate(() => {
    const hud = document.querySelector('.hud');
    const toggle = document.querySelector('[data-panel-toggle="economy"]');
    const windowPanel = document.querySelector('[data-window-panel="economy"]');
    const action = windowPanel?.querySelector('[data-action="market-buy"], [data-action="market-cancel"], [data-action="premium-claim"]');
    const rect = windowPanel instanceof HTMLElement ? windowPanel.getBoundingClientRect() : null;
    const windows = Array.from(document.querySelectorAll('[data-window-panel]')).map((panel) => {
      const element = panel instanceof HTMLElement ? panel : null;
      const bounds = element?.getBoundingClientRect();
      return {
        id: element?.dataset.windowPanel,
        focused: element?.dataset.focused,
        z: Number(element?.style.getPropertyValue('--window-z') ?? '0'),
        rect: bounds ? { left: bounds.left, right: bounds.right, top: bounds.top, bottom: bounds.bottom, width: bounds.width, height: bounds.height } : null,
        visible: Boolean(bounds && bounds.width > 0 && bounds.height > 0 && getComputedStyle(element).display !== 'none'),
      };
    });
    return {
      activePanel: hud instanceof HTMLElement ? hud.dataset.activePanel : null,
      toggleActive: toggle instanceof HTMLElement ? toggle.dataset.active : null,
      togglePressed: toggle instanceof HTMLElement ? toggle.getAttribute('aria-pressed') : null,
      focused: windowPanel instanceof HTMLElement ? windowPanel.dataset.focused : null,
      hasServerAction: Boolean(action),
      rect: rect ? { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom } : null,
      windows,
      scrollWidth: document.body.scrollWidth,
    };
  });

  if (opened.activePanel !== 'economy' || opened.toggleActive !== 'true' || opened.togglePressed !== 'true') {
    throw new Error(`${label}: economy window did not become the active HUD panel`);
  }
  if (opened.focused !== 'true' || !opened.hasServerAction) {
    throw new Error(`${label}: economy window is missing focus state or server-backed controls`);
  }
  const openedIDs = new Set(opened.windows.map((windowPanel) => windowPanel.id));
  for (const panel of featurePanels) {
    if (!openedIDs.has(panel)) {
      throw new Error(`${label}: ${panel} feature window did not open`);
    }
  }
  if (opened.scrollWidth > viewport.width + 1) {
    throw new Error(`${label}: panel window caused horizontal overflow (${opened.scrollWidth} > ${viewport.width})`);
  }
  if (viewport.width >= 768) {
    for (const windowPanel of opened.windows) {
      if (!windowPanel.visible || !windowPanel.rect) {
        throw new Error(`${label}: ${windowPanel.id} feature window is not visible on desktop/tablet`);
      }
      const centerX = windowPanel.rect.left + windowPanel.rect.width / 2;
      const centerY = windowPanel.rect.top + windowPanel.rect.height / 2;
      if (Math.abs(centerX - viewport.width / 2) > 90 || Math.abs(centerY - viewport.height / 2) > 110) {
        throw new Error(`${label}: ${windowPanel.id} did not open centered (${JSON.stringify(windowPanel.rect)})`);
      }
    }
  } else if (opened.rect) {
    const withinViewport =
      opened.rect.left >= -1 &&
      opened.rect.right <= viewport.width + 1 &&
      opened.rect.top >= -1 &&
      opened.rect.bottom <= viewport.height + 1;
    if (!withinViewport) {
      throw new Error(`${label}: mobile panel window is outside viewport ${JSON.stringify(opened.rect)}`);
    }
  }

  if (viewport.width >= 768) {
    const beforeDrag = await windowRect(page, 'economy');
    const header = await page.locator('[data-window-panel="economy"] .hud-window__header').boundingBox();
    if (!header || !beforeDrag) {
      throw new Error(`${label}: economy drag handle was not measurable`);
    }
    await page.mouse.move(header.x + header.width / 2, header.y + header.height / 2);
    await page.mouse.down();
    await page.mouse.move(header.x + header.width / 2 + 130, header.y + header.height / 2 + 74, { steps: 6 });
    await page.mouse.up();
    const afterDrag = await windowRect(page, 'economy');
    if (!afterDrag) {
      throw new Error(`${label}: economy window disappeared after drag`);
    }
    if (Math.hypot(afterDrag.left - beforeDrag.left, afterDrag.top - beforeDrag.top) < 40) {
      throw new Error(`${label}: economy window did not move after drag`);
    }
    if (afterDrag.left < -1 || afterDrag.right > viewport.width + 1 || afterDrag.top < -1 || afterDrag.top > viewport.height - 36) {
      throw new Error(`${label}: dragged economy window was not clamped (${JSON.stringify(afterDrag)})`);
    }

    await page.locator('[data-panel-toggle="cargo"]').click();
    await page.waitForSelector('[data-window-panel="cargo"][data-focused="true"]', { timeout: 10000 });
    const cargoWindows = await page.locator('[data-window-panel="cargo"]').count();
    if (cargoWindows !== 1) {
      throw new Error(`${label}: nav focus duplicated cargo windows (${cargoWindows})`);
    }

    const moveCountBeforeWindowClick = await commandLogCount(page, 'Sent move_to.');
    await page.locator('[data-window-panel="cargo"] .hud-window__body').click({ position: { x: 18, y: 18 } });
    await page.locator('[data-panel-toggle="systems"]').click();
    const moveCountAfterWindowClick = await commandLogCount(page, 'Sent move_to.');
    if (moveCountAfterWindowClick !== moveCountBeforeWindowClick) {
      throw new Error(`${label}: HUD window/nav click leaked into canvas move_to`);
    }
  }

  await page.screenshot({ path: path.join(phase02OutputDir, `windows-${label}.png`), fullPage: true });
  await page.screenshot({ path: path.join(phase08OutputDir, `window-${label}.png`), fullPage: true });

  await page.locator('[data-panel-toggle="economy"]').click();
  await page.waitForSelector('[data-window-panel="economy"][data-focused="true"]', { timeout: 10000 });
  await page.locator('[data-window-panel="economy"] [data-modal-open="economy"]').click();
  await page.waitForSelector('[data-modal="economy"][role="dialog"][aria-modal="true"]', { timeout: 10000 });
  await page.evaluate(() => {
    document.querySelector('[data-modal="economy"] .hud-modal__body')?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  });
  if ((await page.locator('[data-modal="economy"]').count()) !== 1) {
    throw new Error(`${label}: modal closed when clicking inside modal body`);
  }

  await page.locator('[data-modal-close="button"]').click();
  await page.waitForFunction(() => !document.querySelector('[data-modal]'), null, { timeout: 10000 });

  await page.locator('[data-window-panel="economy"] [data-modal-open="economy"]').click();
  await page.waitForSelector('[data-modal="economy"]', { timeout: 10000 });
  await page.keyboard.press('Escape');
  await page.waitForFunction(() => !document.querySelector('[data-modal]'), null, { timeout: 10000 });

  await page.locator('[data-window-panel="economy"] [data-modal-open="economy"]').click();
  await page.waitForSelector('[data-modal="economy"]', { timeout: 10000 });
  await page.locator('[data-modal-close="backdrop"]').click({ position: { x: 4, y: 4 } });
  await page.waitForFunction(() => !document.querySelector('[data-modal]'), null, { timeout: 10000 });

  for (const panel of featurePanels.toReversed()) {
    await page.locator(`[data-panel-toggle="${panel}"]`).click();
    await page.waitForSelector(`[data-window-panel="${panel}"][data-focused="true"]`, { timeout: 10000 });
    await page.locator(`[data-window-panel="${panel}"] [data-panel-close="${panel}"]`).click();
  }
  await page.waitForFunction(() => !document.querySelector('[data-window-panel]') && document.querySelector('.hud')?.dataset.activePanel === 'none', null, {
    timeout: 10000,
  });
}

async function windowRect(page, panel) {
  return page.evaluate((panelID) => {
    const element = document.querySelector(`[data-window-panel="${panelID}"]`);
    if (!(element instanceof HTMLElement)) {
      return null;
    }
    const rect = element.getBoundingClientRect();
    return { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom, width: rect.width, height: rect.height };
  }, panel);
}

async function selfMovementSample(page) {
  return page.evaluate(() => {
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    const entity = Object.values(state?.visibleEntities ?? {}).find((candidate) => candidate.status_flags?.includes('self'));
    if (!entity) {
      throw new Error('Missing self entity.');
    }
    const display = state?.worldView?.displayPositions?.[entity.entity_id] ?? entity.position;
    return { entity, display };
  });
}

async function movementEtaSample(page) {
  return page.evaluate(() => {
    const eta = window.__SPACE_MORPG_SMOKE_STATE__?.movementEta;
    const element = document.querySelector('[data-movement-eta-active="true"]');
    return {
      active: eta?.active === true,
      remainingMs: eta?.remainingMs ?? null,
      progress: eta?.progress ?? null,
      text: element?.textContent ?? '',
    };
  });
}

function distance(a, b) {
  const dx = (a?.x ?? 0) - (b?.x ?? 0);
  const dy = (a?.y ?? 0) - (b?.y ?? 0);
  return Math.hypot(dx, dy);
}

function isFiniteVec(value) {
  return Number.isFinite(value?.x) && Number.isFinite(value?.y);
}

async function commandLogCount(page, text) {
  return page.evaluate((needle) => {
    const lines = window.__SPACE_MORPG_SMOKE_STATE__?.commandLog ?? [];
    return lines.filter((line) => line.text === needle).length;
  }, text);
}

async function commandLogMatchCount(page, pattern) {
  return page.evaluate((source) => {
    const regex = new RegExp(source);
    const lines = window.__SPACE_MORPG_SMOKE_STATE__?.commandLog ?? [];
    return lines.filter((line) => regex.test(line.text)).length;
  }, pattern);
}

async function commandLogSentCount(page) {
  return page.evaluate(() => {
    const lines = window.__SPACE_MORPG_SMOKE_STATE__?.commandLog ?? [];
    return lines.filter((line) => /^Sent /.test(line.text)).length;
  });
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

async function assertMockupParityShell(page, viewport, label) {
  const shell = await page.evaluate(() => {
    const rectFor = (selector) => {
      const element = document.querySelector(selector);
      if (!(element instanceof HTMLElement)) {
        return null;
      }
      const rect = element.getBoundingClientRect();
      const style = window.getComputedStyle(element);
      return {
        left: rect.left,
        right: rect.right,
        top: rect.top,
        bottom: rect.bottom,
        width: rect.width,
        height: rect.height,
        visible: style.display !== 'none' && style.visibility !== 'hidden' && rect.width > 0 && rect.height > 0,
      };
    };
    const visibleCount = (selector) =>
      Array.from(document.querySelectorAll(selector)).filter((element) => {
        if (!(element instanceof HTMLElement)) {
          return false;
        }
        const rect = element.getBoundingClientRect();
        const style = window.getComputedStyle(element);
        return style.display !== 'none' && style.visibility !== 'hidden' && rect.width > 0 && rect.height > 0;
      }).length;
    const iconWidths = Array.from(document.querySelectorAll('.action-button__icon'))
      .map((element) => (element instanceof HTMLElement ? element.getBoundingClientRect().width : 0))
      .filter((width) => width > 0);

    return {
      scrollWidth: document.scrollingElement?.scrollWidth ?? document.body.scrollWidth,
      topbar: rectFor('.hud__topbar'),
      topCells: visibleCount('.top-status__cell'),
      topIcons: visibleCount('.top-status__icon'),
      nav: rectFor('.hud__nav'),
      navButtons: visibleCount('.hud-nav-button'),
      navIcons: visibleCount('.hud-nav-button__icon'),
      rightRail: rectFor('.hud__rail--right'),
      rightPanels: visibleCount('.hud__rail--right .panel'),
      actionbar: rectFor('.hud__actionbar'),
      actionSlots: visibleCount('.hud__actionbar [data-quick-action-slot]'),
      actionIconMinWidth: iconWidths.length > 0 ? Math.min(...iconWidths) : 0,
      log: rectFor('.hud__log'),
      minimap: rectFor('.minimap'),
    };
  });

  if (shell.scrollWidth > viewport.width + 1) {
    throw new Error(`${label}: mockup shell has horizontal overflow (${shell.scrollWidth} > ${viewport.width})`);
  }
  if (!shell.topbar?.visible || shell.topCells !== 6 || shell.topIcons !== 6) {
    throw new Error(`${label}: top status bar lost mockup icon/value structure ${JSON.stringify(shell)}`);
  }
  if (!shell.nav?.visible || shell.navButtons < 5 || shell.navIcons < 5) {
    throw new Error(`${label}: left nav rail lost icon button structure ${JSON.stringify(shell)}`);
  }
  if (!shell.actionbar?.visible || shell.actionSlots !== 6) {
    throw new Error(`${label}: action rail lost six-slot structure ${JSON.stringify(shell)}`);
  }
  const minActionIcon = viewport.width < 768 ? 18 : viewport.width < 1100 ? 22 : 28;
  if (shell.actionIconMinWidth < minActionIcon) {
    throw new Error(`${label}: action slot icons are too small for mockup parity ${JSON.stringify(shell)}`);
  }
  if (!shell.rightRail?.visible || shell.rightPanels < 3 || !shell.minimap?.visible) {
    throw new Error(`${label}: right rail/minimap structure is missing ${JSON.stringify(shell)}`);
  }
  if (viewport.width >= 1100 && !shell.log?.visible) {
    throw new Error(`${label}: desktop log panel is missing from the mockup shell ${JSON.stringify(shell)}`);
  }
  if (viewport.width >= 768 && (shell.topbar.height > 62 || shell.actionbar.bottom > viewport.height + 1)) {
    throw new Error(`${label}: desktop/tablet HUD chrome is not framed like the mockup ${JSON.stringify(shell)}`);
  }
  if (viewport.width >= 768 && viewport.width < 1100 && shell.actionbar.right > shell.rightRail.left + 1) {
    throw new Error(`${label}: tablet action rail overlaps the right rail ${JSON.stringify(shell)}`);
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
  const point = await worldClickPoint(page, world);

  if (!point) {
    throw new Error('Could not map world position to canvas point.');
  }
  if (point.element !== 'world-canvas') {
    throw new Error(`World click at ${Math.round(point.x)},${Math.round(point.y)} hit ${String(point.element)} (${JSON.stringify(point)})`);
  }
  await page.mouse.click(point.x, point.y);
}

async function clickWorldPositionsRapid(page, worlds) {
  const points = [];
  for (const world of worlds) {
    const point = await worldClickPoint(page, world);
    if (!point) {
      throw new Error('Could not map world position to canvas point.');
    }
    if (point.element !== 'world-canvas') {
      throw new Error(`World click at ${Math.round(point.x)},${Math.round(point.y)} hit ${String(point.element)} (${JSON.stringify(point)})`);
    }
    points.push(point);
  }
  for (const point of points) {
    await page.mouse.click(point.x, point.y);
  }
}

async function ensureWorldPositionClickable(page, world) {
  const point = await worldClickPoint(page, world);
  if (!point) {
    throw new Error('Could not map world position to canvas point.');
  }
  if (point.element === 'world-canvas') {
    return;
  }

  const targetCenter = { x: world.x, y: world.y - 180 };
  await clickWorldPosition(page, targetCenter);
  await page.waitForFunction(
    (target) => {
      const canvas = document.querySelector('canvas.world-canvas');
      const state = window.__SPACE_MORPG_SMOKE_STATE__;
      if (!(canvas instanceof HTMLCanvasElement) || !state) {
        return false;
      }
      const rect = canvas.getBoundingClientRect();
      const center = state.worldView?.center ?? { x: 0, y: 0 };
      const scale = rect.width < 700 ? 0.78 : 1;
      const x = rect.left + rect.width / 2 + (target.x - center.x) * scale;
      const y = rect.top + rect.height / 2 + (target.y - center.y) * scale;
      return document.elementFromPoint(x, y)?.className === 'world-canvas';
    },
    world,
    { timeout: 10000 },
  );
}

async function worldClickPoint(page, world) {
  return page.evaluate((target) => {
    const canvas = document.querySelector('canvas.world-canvas');
    const state = window.__SPACE_MORPG_SMOKE_STATE__;
    if (!(canvas instanceof HTMLCanvasElement) || !state) {
      return null;
    }
    const rect = canvas.getBoundingClientRect();
    const player =
      Object.values(state.visibleEntities ?? {}).find((entity) => entity.status_flags?.includes('self')) ??
      Object.values(state.visibleEntities ?? {}).find((entity) => entity.entity_type === 'player');
    const center = state.worldView?.center ?? player?.position ?? { x: 0, y: 0 };
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
    waitForOpCount: (op, count) => waitForOpCount(received, op, count),
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
    case 'scan.pulse': {
      const scan = {
        pulse_reference: `fixture-${request.request_id}`,
        status: 'started',
        resolve_after: Date.now() + 90,
      };
      sendMessage(socket, response(request.request_id, { scan }));
      sendMessage(socket, event(`scan-start-${request.request_id}`, 'scan.pulse_started', scan));
      setTimeout(() => {
        sendMessage(
          socket,
          event(`scan-resolve-${request.request_id}`, 'scan.pulse_resolved', {
            pulse_reference: scan.pulse_reference,
            status: 'no_signal',
            message: 'Fixture scan resolved with no signal.',
          }),
        );
      }, 90);
      break;
    }
    default:
      sendMessage(socket, errorResponse(request.request_id, 'ERR_INVALID_PAYLOAD', 'Unsupported operation.'));
  }
}

function waitForOpCount(received, op, count) {
  const matching = () => received.filter((message) => message.op === op);
  const found = matching();
  if (found.length >= count) {
    return Promise.resolve(found);
  }
  return new Promise((resolve, reject) => {
    const startedAt = Date.now();
    const timer = setInterval(() => {
      const current = matching();
      if (current.length >= count) {
        clearInterval(timer);
        resolve(current);
        return;
      }
      if (Date.now() - startedAt > 9000) {
        clearInterval(timer);
        reject(new Error(`Timed out waiting for ${count} ${op} ops; received ops: ${received.map((message) => message.op).join(', ')}`));
      }
    }, 50);
  });
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
    ship: {
      active_ship_id: 'starter',
      display_name: 'Sparrow',
      hull: 84,
      max_hull: 100,
      shield: 61,
      max_shield: 100,
      capacitor: 72,
      max_capacitor: 100,
      disabled: false,
      repair_state: 'ready',
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
      loot_pickup_range: 120,
      basic_laser_energy_cost: 10,
      basic_laser_cooldown_ms: 350,
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

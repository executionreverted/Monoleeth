import {
  capacityIconURL,
  cargoIconURL,
  creditsIconURL,
  dangerIconURL,
  energyIconURL,
  sectorIconURL,
} from './hud-icons';
import { escapeHTML, lockedValue } from './hud-formatters';

export function hudShellHTML(): string {
  return `
      <header class="hud__topbar">
        <div class="top-status" aria-label="Pilot status">
          <div class="top-status__cell" data-icon="sector"><img class="top-status__icon" src="${escapeHTML(sectorIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Sector</span><strong data-top-sector>${lockedValue()}</strong></div>
          <div class="top-status__cell top-status__cell--danger" data-icon="danger"><img class="top-status__icon" src="${escapeHTML(dangerIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Danger</span><strong data-top-danger>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="energy"><img class="top-status__icon" src="${escapeHTML(energyIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Energy</span><strong data-top-energy>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="cargo"><img class="top-status__icon" src="${escapeHTML(cargoIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Cargo</span><strong data-top-cargo>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="credits"><img class="top-status__icon" src="${escapeHTML(creditsIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Credits</span><strong data-top-credits>${lockedValue()}</strong></div>
          <div class="top-status__cell" data-icon="cap"><img class="top-status__icon" src="${escapeHTML(capacityIconURL)}" alt="" aria-hidden="true" draggable="false" /><span>Cap</span><strong data-top-cap>${lockedValue()}</strong></div>
        </div>
      </header>
      <div class="hud__movement-eta" data-movement-eta></div>
      <aside class="hud__rail hud__rail--left">
        <div class="panel panel--status" data-panel="status"></div>
        <nav class="panel hud__nav" data-hud-nav aria-label="HUD panels"></nav>
      </aside>
      <div class="hud__window-layer" data-window-layer></div>
      <div class="hud__aux-panels" aria-hidden="true">
        <div class="panel panel--cargo" data-panel="cargo"></div>
        <div class="panel panel--economy" data-panel="economy"></div>
        <div class="panel panel--systems" data-panel="systems"></div>
        <div class="panel panel--quests" data-panel="quests"></div>
        <div class="panel panel--chat" data-panel="chat"></div>
        <div class="panel panel--social" data-panel="social"></div>
        <div class="panel panel--ops" data-panel="ops"></div>
        <div class="panel panel--drawer" data-panel="drawer" data-open="false"></div>
      </div>
      <aside class="hud__rail hud__rail--right">
        <div class="panel panel--planets" data-panel="planets"></div>
        <div class="panel" data-panel="target"></div>
        <div class="panel" data-panel="ship"></div>
        <div class="panel" data-panel="intel"></div>
      </aside>
      <footer class="hud__actionbar panel" data-panel="actions"></footer>
      <footer class="hud__log panel" data-panel="log"></footer>
      <div class="hud-floating-tooltip" id="module-floating-tooltip" data-module-tooltip-layer role="tooltip" aria-hidden="true"></div>
      <div class="hud__modal-layer" data-modal-layer></div>
      <div class="toast" role="status" aria-live="polite"></div>
    `;
}

export function collectHUDPanels(root: HTMLElement): Record<string, HTMLElement> {
  return {
    status: panel(root, 'status'),
    cargo: panel(root, 'cargo'),
    economy: panel(root, 'economy'),
    systems: panel(root, 'systems'),
    quests: panel(root, 'quests'),
    chat: panel(root, 'chat'),
    ops: panel(root, 'ops'),
    drawer: panel(root, 'drawer'),
    planets: panel(root, 'planets'),
    target: panel(root, 'target'),
    ship: panel(root, 'ship'),
    intel: panel(root, 'intel'),
    actions: panel(root, 'actions'),
    log: panel(root, 'log'),
  };
}

function panel(root: HTMLElement, name: string): HTMLElement {
  const element = root.querySelector<HTMLElement>(`[data-panel="${name}"]`);
  if (!element) {
    throw new Error(`Missing HUD panel ${name}`);
  }
  return element;
}

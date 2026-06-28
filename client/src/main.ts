import './styles.css';

import { ClientApp } from './app/client-app';
import type { Vec2 } from './protocol/envelope';

declare global {
  interface Window {
    __SPACE_MORPG_TEST_DRIVER__?: {
      moveTo(target: Vec2): void;
      combatStartAttack(targetID: string): void;
      combatStopAttack(): void;
      portalEnter(portalID: string): void;
      lootPickup(dropID: string): void;
    };
  }
}

const root = document.querySelector<HTMLDivElement>('#app');

if (!root) {
  throw new Error('Missing #app root.');
}

const app = new ClientApp(root);

if (new URLSearchParams(window.location.search).has('smoke')) {
  const testApp = app as unknown as {
    sendMove(target: Vec2): void;
    sendCombatStartAttack(targetID: string): void;
    sendCombatStopAttack(): void;
    sendPortalEnter(portalID: string): void;
    sendCommand(envelope: unknown): void;
    commandBuilder: { lootPickup(dropID: string): unknown };
  };
  window.__SPACE_MORPG_TEST_DRIVER__ = {
    moveTo: (target) => testApp.sendMove(target),
    combatStartAttack: (targetID) => testApp.sendCombatStartAttack(targetID),
    combatStopAttack: () => testApp.sendCombatStopAttack(),
    portalEnter: (portalID) => testApp.sendPortalEnter(portalID),
    lootPickup: (dropID) => testApp.sendCommand(testApp.commandBuilder.lootPickup(dropID)),
  };
}

app.start().catch((error: unknown) => {
  console.error('Failed to start client app', error);
  root.innerHTML = '<div class="boot-error">Client boot failed.</div>';
});

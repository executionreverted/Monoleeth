import './styles.css';

import { ClientApp } from './app/client-app';
import type { Vec2 } from './protocol/envelope';

declare global {
  interface Window {
    __SPACE_MORPG_TEST_DRIVER__?: {
      moveTo(target: Vec2): void;
      combatUseSkill(targetID: string): void;
      combatStartAttack(targetID: string): void;
      combatStopAttack(): void;
      combatState(): void;
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
    commandBuilder: {
      combatUseSkill(targetID: string): unknown;
      combatState(): unknown;
      lootPickup(dropID: string): unknown;
    };
  };
  window.__SPACE_MORPG_TEST_DRIVER__ = {
    moveTo: (target) => testApp.sendMove(target),
    combatUseSkill: (targetID) => testApp.sendCommand(testApp.commandBuilder.combatUseSkill(targetID)),
    combatStartAttack: (targetID) => testApp.sendCombatStartAttack(targetID),
    combatStopAttack: () => testApp.sendCombatStopAttack(),
    combatState: () => testApp.sendCommand(testApp.commandBuilder.combatState()),
    portalEnter: (portalID) => testApp.sendPortalEnter(portalID),
    lootPickup: (dropID) => testApp.sendCommand(testApp.commandBuilder.lootPickup(dropID)),
  };
}

app.start().catch((error: unknown) => {
  console.error('Failed to start client app', error);
  root.innerHTML = '<div class="boot-error">Client boot failed.</div>';
});

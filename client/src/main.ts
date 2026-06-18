import './styles.css';

import { ClientApp } from './app/client-app';

const root = document.querySelector<HTMLDivElement>('#app');

if (!root) {
  throw new Error('Missing #app root.');
}

const app = new ClientApp(root);

app.start().catch((error: unknown) => {
  console.error('Failed to start client app', error);
  root.innerHTML = '<div class="boot-error">Client boot failed.</div>';
});

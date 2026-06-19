import { ClientState } from '../state/types';

export interface AuthPanelHandlers {
  onLogin(email: string, password: string): void;
  onRegister(email: string, password: string, callsign: string): void;
}

type AuthMode = 'login' | 'register';

export class AuthPanel {
  private readonly root: HTMLElement;
  private mode: AuthMode = 'login';

  constructor(container: HTMLElement, private readonly handlers: AuthPanelHandlers) {
    this.root = document.createElement('section');
    this.root.className = 'auth-panel';
    this.root.innerHTML = `
      <form class="auth-card" autocomplete="on">
        <div class="auth-card__header">
          <span class="auth-card__mark"></span>
          <div>
            <h1>Frontier Console</h1>
            <p data-copy>Authenticate to open the live sector link.</p>
          </div>
        </div>
        <label>
          <span>Email</span>
          <input name="email" type="email" autocomplete="email" required />
        </label>
        <label>
          <span>Password</span>
          <input name="password" type="password" autocomplete="current-password" required />
        </label>
        <label data-callsign>
          <span>Callsign</span>
          <input name="callsign" type="text" autocomplete="nickname" minlength="2" maxlength="32" />
        </label>
        <div class="auth-actions">
          <button class="primary-action" type="submit" data-submit>Login</button>
          <button class="ghost-action" type="button" data-toggle>Create account</button>
        </div>
        <p class="auth-message" role="status" aria-live="polite"></p>
      </form>
    `;
    container.appendChild(this.root);
    this.bindEvents();
    this.syncMode();
  }

  render(state: ClientState): void {
    this.root.hidden = state.auth.mode !== 'real' || Boolean(state.auth.session);
    this.root.dataset.status = state.connectionStatus;
    const form = this.form();
    const disabled = state.auth.submitting || state.connectionStatus === 'restoring';
    for (const control of Array.from(form.elements)) {
      if (control instanceof HTMLInputElement || control instanceof HTMLButtonElement) {
        control.disabled = disabled;
      }
    }
    const message = this.root.querySelector<HTMLElement>('.auth-message');
    if (message) {
      message.textContent =
        state.connectionStatus === 'restoring'
          ? 'Restoring session...'
          : state.auth.error ?? (state.auth.submitting ? 'Authenticating...' : '');
    }
  }

  private bindEvents(): void {
    this.root.addEventListener('submit', (event) => {
      event.preventDefault();
      const data = new FormData(this.form());
      const email = String(data.get('email') ?? '').trim();
      const password = String(data.get('password') ?? '');
      const callsign = String(data.get('callsign') ?? '').trim();
      if (this.mode === 'register') {
        this.handlers.onRegister(email, password, callsign);
        return;
      }
      this.handlers.onLogin(email, password);
    });
    this.root.querySelector('[data-toggle]')?.addEventListener('click', () => {
      this.mode = this.mode === 'login' ? 'register' : 'login';
      this.syncMode();
    });
  }

  private syncMode(): void {
    this.root.dataset.mode = this.mode;
    const callsign = this.root.querySelector<HTMLElement>('[data-callsign]');
    const callsignInput = this.root.querySelector<HTMLInputElement>('input[name="callsign"]');
    const submit = this.root.querySelector<HTMLButtonElement>('[data-submit]');
    const toggle = this.root.querySelector<HTMLButtonElement>('[data-toggle]');
    const password = this.root.querySelector<HTMLInputElement>('input[name="password"]');
    if (callsign) {
      callsign.hidden = this.mode !== 'register';
    }
    if (callsignInput) {
      callsignInput.required = this.mode === 'register';
    }
    if (password) {
      password.autocomplete = this.mode === 'register' ? 'new-password' : 'current-password';
    }
    if (submit) {
      submit.textContent = this.mode === 'register' ? 'Create' : 'Login';
    }
    if (toggle) {
      toggle.textContent = this.mode === 'register' ? 'Use login' : 'Create account';
    }
  }

  private form(): HTMLFormElement {
    const form = this.root.querySelector<HTMLFormElement>('form');
    if (!form) {
      throw new Error('Auth form is missing.');
    }
    return form;
  }
}

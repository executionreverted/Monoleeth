import { PublicSession } from '../state/types';

export interface RegisterInput {
  email: string;
  password: string;
  callsign: string;
}

export interface LoginInput {
  email: string;
  password: string;
}

export class AuthClientError extends Error {
  constructor(
    message: string,
    readonly code: string,
  ) {
    super(message);
    this.name = 'AuthClientError';
  }
}

export class AuthClient {
  async loadSession(): Promise<PublicSession> {
    return this.request('/api/session', { method: 'GET' });
  }

  async register(input: RegisterInput): Promise<PublicSession> {
    return this.request('/api/auth/register', {
      method: 'POST',
      body: JSON.stringify(input),
    });
  }

  async login(input: LoginInput): Promise<PublicSession> {
    return this.request('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify(input),
    });
  }

  async logout(): Promise<PublicSession> {
    return this.request('/api/auth/logout', { method: 'POST' });
  }

  private async request(path: string, init: RequestInit): Promise<PublicSession> {
    const response = await fetch(path, {
      ...init,
      credentials: 'include',
      headers: {
        ...(init.body ? { 'Content-Type': 'application/json' } : {}),
        ...init.headers,
      },
    });
    const body = (await response.json().catch(() => null)) as unknown;
    if (!response.ok) {
      throw parseAuthError(body);
    }
    return parsePublicSession(body);
  }
}

function parseAuthError(body: unknown): AuthClientError {
  if (isRecord(body) && isRecord(body.error)) {
    const code = typeof body.error.code === 'string' ? body.error.code : 'ERR_AUTH';
    const message = typeof body.error.message === 'string' ? body.error.message : 'Authentication failed.';
    return new AuthClientError(message, code);
  }
  return new AuthClientError('Authentication failed.', 'ERR_AUTH');
}

function parsePublicSession(body: unknown): PublicSession {
  if (!isRecord(body) || typeof body.authenticated !== 'boolean') {
    throw new AuthClientError('Session response is invalid.', 'ERR_INVALID_SESSION');
  }
  const account = isRecord(body.account)
    ? {
        email: typeof body.account.email === 'string' ? body.account.email : '',
        admin: body.account.admin === true,
      }
    : undefined;
  const player = isRecord(body.player)
    ? {
        callsign: typeof body.player.callsign === 'string' ? body.player.callsign : '',
      }
    : undefined;
  const roles = Array.isArray(body.roles) ? body.roles.filter((role): role is string => typeof role === 'string') : undefined;
  return {
    authenticated: body.authenticated,
    account,
    player,
    roles,
    expires_at: typeof body.expires_at === 'number' ? body.expires_at : undefined,
    server_time: typeof body.server_time === 'number' ? body.server_time : Date.now(),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

import { describe, expect, test } from 'vitest';

import { CLIENT_EVENTS, OPERATIONS } from '../protocol/envelope';
import { createInitialState, reduceClientState } from './reducer';
import { event, expectServerOwnedGameplayCleared, stateWithServerOwnedGameplay } from './reducer-fixtures.test-support';
import { isWithinMinimapProjectionWindow, worldMapMemoryMarkers } from './world-memory';

describe('reduceClientState', () => {
  test('initial state has no fake gameplay values', () => {
    const state = createInitialState();

    expect(state.connectionStatus).toBe('restoring');
    expectServerOwnedGameplayCleared(state);
  });

  test('logout and auth expiry clear gameplay state', () => {
    const withGameplay = stateWithServerOwnedGameplay();
    expect(withGameplay.playerSnapshot).not.toBeNull();
    expect(withGameplay.wallet).not.toBeNull();
    expect(Object.keys(withGameplay.visibleEntities)).toEqual(['npc-1']);

    const loggedOut = reduceClientState(withGameplay, { type: 'authLoggedOut' });
    expect(loggedOut.connectionStatus).toBe('logged_out');
    expectServerOwnedGameplayCleared(loggedOut);

    const expired = reduceClientState(withGameplay, { type: 'authExpired', message: 'Session expired.' });
    expect(expired.connectionStatus).toBe('auth_expired');
    expectServerOwnedGameplayCleared(expired);
    expect(expired.auth.error).toBe('Session expired.');
  });

  test('loaded real session clears stale gameplay and waits for session.ready before connected', () => {
    const loaded = reduceClientState(stateWithServerOwnedGameplay(), {
      type: 'authSessionLoaded',
      session: {
        authenticated: true,
        account: { email: 'pilot@example.com', admin: false },
        player: { callsign: 'Pilot' },
        server_time: 2000,
      },
    });

    expect(loaded.connectionStatus).toBe('authenticated_pending_socket');
    expect(loaded.pendingCommands).toEqual({});
    expect(loaded.playerSnapshot).toBeNull();
    expect(loaded.visibleEntities).toEqual({});
    expect(loaded.wallet).toBeNull();
    expect(loaded.inventory).toBeNull();
    expect(loaded.market).toBeNull();
    expect(loaded.auth.session?.authenticated).toBe(true);
    expect(loaded.lastServerTime).toBe(2000);

    const ready = reduceClientState(loaded, {
      type: 'eventReceived',
      envelope: event(CLIENT_EVENTS.sessionReady, { authenticated: true }, 1),
    });

    expect(ready.connectionStatus).toBe('connected');
    expect(ready.auth.session?.authenticated).toBe(true);
  });

  test('demo mode is explicit and isolated from real auth session state', () => {
    const demo = reduceClientState(stateWithServerOwnedGameplay(), { type: 'demoModeStarted' });

    expect(demo.auth.mode).toBe('demo');
    expect(demo.auth.session).toBeNull();
    expect(demo.connectionStatus).toBe('offline');
    expectServerOwnedGameplayCleared(demo);
    expect(demo.commandLog.some((line) => line.text.includes('Demo mode'))).toBe(true);
  });
});

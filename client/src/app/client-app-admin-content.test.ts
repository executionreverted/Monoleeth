import { describe, expect, test } from 'vitest';

import { OPERATIONS, type EntityPayload, type RequestEnvelope, type ServerMessage, type Vec2 } from '../protocol/envelope';
import { createInitialState } from '../state/reducer';
import type { ClientAction, ClientState, WorldMapMemoryMarker } from '../state/types';
import {
  ADMIN_CONTENT_DEFAULT_BALANCE_TAG,
  ADMIN_CONTENT_DEFAULT_PUBLISH_NOTES,
  ADMIN_CONTENT_DEFAULT_ROLLBACK_NOTES,
  ClientAppCommands,
} from './client-app-commands';

describe('ClientApp admin content commands', () => {
  test('publishes with non-empty notes and balance tag', () => {
    const app = new AdminContentCommandHarness();

    app.publish();

    expect(app.sent).toHaveLength(1);
    expect(app.sent[0]).toMatchObject({
      op: OPERATIONS.adminContentPublish,
      payload: {
        notes: ADMIN_CONTENT_DEFAULT_PUBLISH_NOTES,
        balance_tag: ADMIN_CONTENT_DEFAULT_BALANCE_TAG,
      },
    });
  });

  test('rolls back with non-empty notes and balance tag', () => {
    const app = new AdminContentCommandHarness();

    app.rollback('11111111-1111-5111-8111-111111111111');

    expect(app.sent).toHaveLength(1);
    expect(app.sent[0]).toMatchObject({
      op: OPERATIONS.adminContentRollback,
      payload: {
        target_version_id: '11111111-1111-5111-8111-111111111111',
        notes: ADMIN_CONTENT_DEFAULT_ROLLBACK_NOTES,
        balance_tag: ADMIN_CONTENT_DEFAULT_BALANCE_TAG,
      },
    });
  });
});

class AdminContentCommandHarness extends ClientAppCommands {
  readonly sent: RequestEnvelope[] = [];

  constructor() {
    super({} as HTMLElement);
    this.state = {
      ...createInitialState(),
      connectionStatus: 'connected',
      auth: {
        mode: 'real',
        session: {
          authenticated: true,
          account: { email: 'admin@example.com', admin: true },
          player: { callsign: 'Admin' },
          server_time: 1000,
        },
        submitting: false,
        error: null,
      },
    };
  }

  publish(): void {
    this.sendAdminContentPublish();
  }

  rollback(versionID: string): void {
    this.sendAdminContentRollback(versionID);
  }

  protected sendCommand(envelope: RequestEnvelope): boolean {
    this.sent.push(envelope);
    return true;
  }

  protected activateLootTarget(_target: EntityPayload, _source: 'click' | 'action'): void {}
  protected cancelNavigation(): void {}
  protected estimatedServerTime(): number | null {
    return null;
  }
  protected findLocalPlayerID(): string | null {
    return null;
  }
  protected hasPendingOperation(_op: string): boolean {
    return false;
  }
  protected scheduleNavigationLoop(_serverNow?: number | null): void {}
  protected selectedTarget(): EntityPayload | null {
    return null;
  }
  protected selfEntity(): EntityPayload | null {
    return null;
  }
  protected selfStealthEnabled(): boolean {
    return false;
  }
  protected applyServerMessage(_message: ServerMessage): void {}
  protected dispatch(_action: ClientAction): void {}
  protected handleRealtimeStatus(_status: ClientState['connectionStatus']): void {}
  protected selectEntity(_entityID: string | null): void {}
  protected selectMemoryMarker(_marker: WorldMapMemoryMarker): void {}
  protected handleWorldMoveIntent(_target: Vec2): void {}
}

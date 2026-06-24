import { parseServerMessage, RequestEnvelope, ServerMessage } from '../protocol/envelope';
import { ConnectionStatus } from '../state/types';

export interface RealtimeClientOptions {
  onStatus(status: ConnectionStatus, detail?: { code?: number; reason?: string }): void;
  onMessage(message: ServerMessage): void;
  onError(message: string): void;
}

export class RealtimeClient {
  private socket: WebSocket | null = null;
  private generation = 0;
  private readonly requestOperations = new Map<string, string>();

  constructor(private readonly options: RealtimeClientOptions) {}

  connect(url: string): void {
    this.disconnect();
    this.options.onStatus('connecting');

    let generation: number;
    let socket: WebSocket;
    try {
      this.generation += 1;
      generation = this.generation;
      socket = new WebSocket(url);
      this.socket = socket;
    } catch (error) {
      this.options.onStatus('error');
      this.options.onError(error instanceof Error ? error.message : String(error));
      return;
    }

    const isCurrentSocket = (): boolean => this.socket === socket && this.generation === generation;

    socket.addEventListener('open', () => {
      if (!isCurrentSocket()) {
        return;
      }
      this.options.onStatus('connected');
    });

    socket.addEventListener('close', (event) => {
      if (!isCurrentSocket()) {
        return;
      }
      this.socket = null;
      this.options.onStatus(event.code === 1008 ? 'auth_expired' : 'offline', {
        code: event.code,
        reason: event.reason,
      });
    });

    socket.addEventListener('error', () => {
      if (!isCurrentSocket()) {
        return;
      }
      this.options.onStatus('error');
      this.options.onError('WebSocket connection failed.');
    });

    socket.addEventListener('message', (event) => {
      if (!isCurrentSocket()) {
        return;
      }
      if (typeof event.data !== 'string') {
        this.options.onError('Ignored non-JSON realtime message.');
        return;
      }

      try {
        const message = parseServerMessage(event.data, {
          operationForRequestID: (requestID) => this.requestOperations.get(requestID) ?? null,
        });
        if ('ok' in message) {
          this.requestOperations.delete(message.request_id);
        }
        this.options.onMessage(message);
      } catch (error) {
        this.options.onError(error instanceof Error ? error.message : String(error));
      }
    });
  }

  disconnect(): void {
    if (!this.socket) {
      return;
    }

    const socket = this.socket;
    this.socket = null;
    this.generation += 1;
    this.requestOperations.clear();
    socket.close();
    this.options.onStatus('offline');
  }

  send(envelope: RequestEnvelope): boolean {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      return false;
    }

    this.socket.send(JSON.stringify(envelope));
    this.requestOperations.set(envelope.request_id, envelope.op);
    return true;
  }

  isConnected(): boolean {
    return this.socket?.readyState === WebSocket.OPEN;
  }
}

import { parseServerMessage, RequestEnvelope, ServerMessage } from '../protocol/envelope';
import { ConnectionStatus } from '../state/types';

export interface RealtimeClientOptions {
  onStatus(status: ConnectionStatus): void;
  onMessage(message: ServerMessage): void;
  onError(message: string): void;
}

export class RealtimeClient {
  private socket: WebSocket | null = null;

  constructor(private readonly options: RealtimeClientOptions) {}

  connect(url: string): void {
    this.disconnect();
    this.options.onStatus('connecting');

    try {
      this.socket = new WebSocket(url);
    } catch (error) {
      this.options.onStatus('error');
      this.options.onError(error instanceof Error ? error.message : String(error));
      return;
    }

    this.socket.addEventListener('open', () => {
      this.options.onStatus('connected');
    });

    this.socket.addEventListener('close', () => {
      this.socket = null;
      this.options.onStatus('offline');
    });

    this.socket.addEventListener('error', () => {
      this.options.onStatus('error');
      this.options.onError('WebSocket connection failed.');
    });

    this.socket.addEventListener('message', (event) => {
      if (typeof event.data !== 'string') {
        this.options.onError('Ignored non-JSON realtime message.');
        return;
      }

      try {
        this.options.onMessage(parseServerMessage(event.data));
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
    socket.close();
    this.options.onStatus('offline');
  }

  send(envelope: RequestEnvelope): boolean {
    if (!this.socket || this.socket.readyState !== WebSocket.OPEN) {
      return false;
    }

    this.socket.send(JSON.stringify(envelope));
    return true;
  }

  isConnected(): boolean {
    return this.socket?.readyState === WebSocket.OPEN;
  }
}
